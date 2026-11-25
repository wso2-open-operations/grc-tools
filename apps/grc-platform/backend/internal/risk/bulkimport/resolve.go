// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package bulkimport

import (
	"context"
	"fmt"
	"strings"
)

// EmailLookup resolves a work email to an Asgardeo uuid. found is false when
// the directory has no such (enabled) account; an error is a directory outage,
// not an answer, and aborts the run. An email that matches more than one account
// is reported as not found: the platform never picks a winner.
type EmailLookup func(ctx context.Context, email string) (uuid string, found bool, err error)

// cachedLookup remembers every answer for the life of one run, including "no such
// account". A run looks each person up in several passes (validation, the dry-run
// plan, provisioning); sharing one cache makes them agree: a person accepted by
// the validation pass cannot be rejected by the provisioning pass because the
// directory changed in between, which would leave a user row behind for a file
// that then fails. It also asks the directory once per person, not once per pass.
// An error is not remembered, so a failed lookup is tried again.
func cachedLookup(l EmailLookup) EmailLookup {
	type answer struct {
		uuid  string
		found bool
	}
	seen := map[string]answer{}
	return func(ctx context.Context, email string) (string, bool, error) {
		key := strings.ToLower(strings.TrimSpace(email))
		if a, ok := seen[key]; ok {
			return a.uuid, a.found, nil
		}
		uuid, found, err := l(ctx, key)
		if err != nil {
			return "", false, err
		}
		seen[key] = answer{uuid, found}
		return uuid, found, nil
	}
}

// Resolver turns a work email into a compliance-entity user.id, via:
//
//	email --(directory lookup)--> uuid --(GET /users/by-uuid, else POST /users)--> user.id
//
// In a dry run the resolver stops at the uuid: it confirms the email resolves
// to an account but never calls the entity, so a dry run writes nothing (plan
// §9). On a real run it resolves the uuid to a user.id, provisioning a user row
// if the person has none — never mutating an existing row (plan §6).
type Resolver struct {
	entity *EntityClient
	lookup EmailLookup
	dryRun bool
	// readOnly looks existing user rows up but never creates one: an unknown
	// person resolves, with id 0. It lets a dry run tell whether a risk already
	// written by an earlier import is complete (its grants depend on user ids)
	// without writing anything.
	readOnly bool

	uuidByEmail map[string]string // lower(email) -> uuid, "" when the directory has no such account
	idByUUID    map[string]int    // uuid -> user.id (real-run cache)
}

// NewReadOnlyResolver resolves like a real run but never provisions a user.
func NewReadOnlyResolver(entity *EntityClient, lookup EmailLookup) *Resolver {
	r := NewResolver(entity, lookup, false)
	r.readOnly = true
	return r
}

func NewResolver(entity *EntityClient, lookup EmailLookup, dryRun bool) *Resolver {
	return &Resolver{
		entity:      entity,
		lookup:      lookup,
		dryRun:      dryRun,
		uuidByEmail: map[string]string{},
		idByUUID:    map[string]int{},
	}
}

// Apply fills the people id fields on every row (the team / category / ref ids
// are already set by mapRow, T4). It returns a Finding per person that could
// not be resolved and, on a real run, a non-nil error if provisioning hit a
// transport/entity failure — the caller then aborts and re-runs to resume.
//
// Rejection vs. warning by column (plan §6, D4 / D7):
//
//   - Risk Owner / Risk Assigned To / Management Approver — always REJECT
//   - Action Owner — REJECT for APPROVED rows, WARN + null for CLOSED
func (r *Resolver) Apply(ctx context.Context, rows []Row) ([]Finding, error) {
	var fs []Finding
	for i := range rows {
		row := &rows[i]
		reject := func(field, value, detail string) {
			fs = append(fs, Finding{MigrationID: row.MigrationID, CSVRow: row.CSVLine, RiskTitle: row.RiskTitle,
				Severity: SevReject, Failure: field, Detail: detail, Value: value})
		}
		warn := func(field, value, detail string) {
			fs = append(fs, Finding{MigrationID: row.MigrationID, CSVRow: row.CSVLine, RiskTitle: row.RiskTitle,
				Severity: SevWarn, Failure: field, Detail: detail, Value: value})
		}

		// The three NOT NULL people columns: unresolvable → REJECT (D4).
		for _, p := range []struct {
			field string
			email string
			dst   *int
		}{
			{"Risk Assigned To", row.AssignerEmail, &row.AssignerID},
			{"Risk Owner", row.OwnerEmail, &row.OwnerID},
			{"Management Approver", row.ManagementApproverEmail, &row.ManagementApproverID},
		} {
			id, ok, err := r.userID(ctx, p.email)
			if err != nil {
				return nil, fmt.Errorf("%s %q: %w", p.field, p.email, err)
			}
			if !ok {
				reject(p.field, p.email, r.unresolvedReason(p.email))
				continue
			}
			if !r.dryRun {
				*p.dst = id
			}
		}

		// Action Owner: mandatory for APPROVED, nullable for CLOSED (D7).
		approved := row.WorkflowStatus == "APPROVED"
		switch {
		case row.ActionOwnerEmail == "":
			if approved {
				reject("Action Owner", "", "mandatory for an APPROVED row")
			}
		default:
			id, ok, err := r.userID(ctx, row.ActionOwnerEmail)
			if err != nil {
				return nil, fmt.Errorf("Action Owner %q: %w", row.ActionOwnerEmail, err)
			}
			switch {
			case ok:
				if !r.dryRun {
					actionOwnerID := id
					row.ActionOwnerID = &actionOwnerID
				}
			case approved:
				reject("Action Owner", row.ActionOwnerEmail, r.unresolvedReason(row.ActionOwnerEmail))
			default:
				warn("Action Owner", row.ActionOwnerEmail, r.unresolvedReason(row.ActionOwnerEmail)+" — left null on this CLOSED row")
			}
		}
	}
	return fs, nil
}

// unresolvedReason explains why an email did not resolve, for the finding.
func (r *Resolver) unresolvedReason(email string) string {
	key := strings.ToLower(strings.TrimSpace(email))
	switch {
	case key == "":
		return "blank"
	default:
		return fmt.Sprintf("%q has no active account in the directory (or matches more than one)", email)
	}
}

// userID resolves one email to a user.id. ok is true when the email maps to
// exactly one uuid (resolvable); in a dry run it returns (0, true, nil) at that
// point without calling the entity. On a real run it resolves the uuid to a
// user.id, provisioning a user row on a 404 (an upsert on uuid — never a
// mutation of an existing row).
func (r *Resolver) userID(ctx context.Context, email string) (id int, ok bool, err error) {
	key := strings.ToLower(strings.TrimSpace(email))
	if key == "" {
		return 0, false, nil
	}
	uuid, seen := r.uuidByEmail[key]
	if !seen {
		var found bool
		var lerr error
		uuid, found, lerr = r.lookup(ctx, key)
		if lerr != nil {
			return 0, false, lerr
		}
		if !found {
			uuid = ""
		}
		r.uuidByEmail[key] = uuid
	}
	if uuid == "" {
		return 0, false, nil
	}
	if r.dryRun {
		return 0, true, nil
	}
	if cached, hit := r.idByUUID[uuid]; hit {
		return cached, true, nil
	}

	id, exists, err := r.entity.GetUserByUUID(ctx, uuid)
	if err != nil {
		return 0, false, err
	}
	if !exists && r.readOnly {
		return 0, true, nil
	}
	if !exists {
		id, err = r.entity.CreateUser(ctx, CreateUserRequest{
			UUID: uuid, UserType: "INTERNAL", Status: "ACTIVE", CreatedBy: marker,
		})
		if err != nil {
			return 0, false, err
		}
	}
	r.idByUUID[uuid] = id
	return id, true, nil
}
