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
	"errors"
	"testing"
)

// ── review fixes: one directory answer per person for the whole run ──────────

func TestCachedLookup_RemembersAnswersButNotErrors(t *testing.T) {
	calls := map[string]int{}
	fail := true
	l := cachedLookup(func(_ context.Context, email string) (string, bool, error) {
		calls[email]++
		switch email {
		case "known@wso2.com":
			return "uuid-known", true, nil
		case "flaky@wso2.com":
			if fail {
				return "", false, errors.New("directory unreachable")
			}
			return "uuid-flaky", true, nil
		}
		return "", false, nil
	})
	ctx := context.Background()

	for range 3 {
		if u, ok, _ := l(ctx, " Known@WSO2.com "); !ok || u != "uuid-known" {
			t.Fatalf("known = %q, %v", u, ok)
		}
		if _, ok, _ := l(ctx, "ghost@wso2.com"); ok {
			t.Fatal("ghost resolved")
		}
	}
	if got := calls["known@wso2.com"]; got != 1 {
		t.Errorf("known was looked up %d times, want 1 (case and spacing share an entry)", got)
	}
	if calls["ghost@wso2.com"] != 1 {
		t.Errorf("a miss must be remembered too: %d lookups", calls["ghost@wso2.com"])
	}

	if _, _, err := l(ctx, "flaky@wso2.com"); err == nil {
		t.Fatal("the outage must surface")
	}
	fail = false
	if u, ok, err := l(ctx, "flaky@wso2.com"); err != nil || !ok || u != "uuid-flaky" {
		t.Errorf("an error must not be remembered: %q %v %v", u, ok, err)
	}
}

// countingLookup wraps the fixture directory and records how often each email is
// asked about. flipAfter, when set, makes every email vanish from the directory
// after that many lookups in total: the directory changing mid-run.
type countingLookup struct {
	inner     EmailLookup
	calls     map[string]int
	total     int
	flipAfter int
}

func (c *countingLookup) lookup(ctx context.Context, email string) (string, bool, error) {
	c.calls[email]++
	c.total++
	if c.flipAfter > 0 && c.total > c.flipAfter {
		return "", false, nil
	}
	return c.inner(ctx, email)
}

func TestRun_AsksTheDirectoryOncePerPerson(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		fe := newFakeEntity(t)
		svc := newRunService(t, fe)
		cl := &countingLookup{inner: snapshotLookup(fixtureSnapshot()), calls: map[string]int{}}
		svc.lookup = cl.lookup

		res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, dryRun,
			plainRow(map[string]string{"Migration ID": "1"}),
			plainRow(map[string]string{"Risk Title": "Second", "Migration ID": "2"})))
		if err != nil || res.Outcome != OutcomeClean {
			t.Fatalf("dryRun=%v: outcome=%v err=%v findings=%+v", dryRun, res.Outcome, err, res.Report.Findings())
		}
		if len(cl.calls) == 0 {
			t.Fatal("the directory was never asked")
		}
		for email, n := range cl.calls {
			if n != 1 {
				t.Errorf("dryRun=%v: %q was looked up %d times, want 1", dryRun, email, n)
			}
		}
	}
}

// The directory changing between the validation pass and provisioning used to
// let provisioning create user rows and only then reject the file. With one
// shared answer per person the passes cannot disagree.
func TestRun_DirectoryChangingMidRunCannotLeaveUsersBehind(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)
	cl := &countingLookup{inner: snapshotLookup(fixtureSnapshot()), calls: map[string]int{}, flipAfter: 4}
	svc.lookup = cl.lookup

	res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, false, plainRow(nil)))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Outcome != OutcomeClean || len(fe.createdRisks) != 1 {
		t.Errorf("outcome = %v, risks = %d, findings = %+v", res.Outcome, len(fe.createdRisks), res.Report.Findings())
	}
	if res.Report.Counts().Rejects != 0 {
		t.Errorf("a person accepted by validation was rejected later: %+v", res.Report.Findings())
	}
}
