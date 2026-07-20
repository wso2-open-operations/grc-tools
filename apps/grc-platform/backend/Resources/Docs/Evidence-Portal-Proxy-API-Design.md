# Design: Evidence Portal Proxy API — Secure Multi-IdP Integration

**Status:** Approved design — ready to implement.
**Supersedes:** `Evidence-App-Proxy-API-Design.md` (kept for history).
**Applies to:** `grc-platform/apps/grc-platform/backend` (main backend) +
`grc-platform/entity/compliance-entity` + Choreo/Asgardeo configuration.
**Threat model:** `[Threat Model] GRC Platform v1.0` — interactions **[01]** (JWT auth) and
**[05]** (Third-party Evidence Portal submits evidence via the public API).

---

## 1. Context / Problem

A second team built an **Evidence Portal** app. Its users are the **same WSO2 team members**
already in the GRC Platform (same email), but the portal authenticates them against a
**second Asgardeo organization (IdP-2)** — so its JWTs have a different `iss`, `aud`, and
signing keys. The portal needs to:

1. Fetch the **audits, products, frameworks, and controls** assigned to the logged-in user.
2. **Submit evidence** files against those controls (through the backend → Compliance Entity
   → Azure Blob proxy chain — no storage credential ever reaches the portal).

The threat model ([05]) already **asserts** these mitigations, which the code must now make true:

> "The backend re-derives the folder path from the route and verifies the user's assignment;
> RBAC enforced; client-supplied paths are not trusted." … "Choreo Gateway rate-limiting plus
> per-principal throttling."

### Current gaps (verified in code)

| # | Gap | Where |
|---|-----|-------|
| 1 | **Single-issuer auth** — IdP-2 tokens rejected before email is read | `backend/internal/middleware/auth.go` (`Config.Issuer/Audience/JWKSEndpoint` are scalars; `ParseWithClaims` at ~line 275 pins one issuer) |
| 2 | **No role mapping for IdP-2** — `user` table has no role column; roles come only from JWT `groups` resolved via `role`/`role_privilege` | `backend/internal/shared/privilege/privilege.go`, `Resources/shared.sql` |
| 3 | **IDOR** — evidence endpoints check the `SUBMIT_EVIDENCE` privilege only; never that the caller is *assigned to this control* | `backend/internal/audit/handler/evidence.go` (`getUploadLink`, `uploadEvidence`, `submitEvidence`) |
| 4 | **Client-controlled `folderPath`** — only a non-empty check; not bound to the route's audit/control | `evidence.go:105-109`, `service/evidence.go` `UploadFile`/`Submit` |
| 5 | **Compliance Entity accepts any blob name** — no path allowlist | `entity/compliance-entity/internal/handler/file_handler.go` |
| 6 | **No per-principal rate limiting** in the backend | (middleware absent) |

### Decisions (agreed 2026-07-09)

1. **IdP-2 role mapping:** *per-issuer group→role mapping with a privilege ceiling* — **not**
   mirroring the same role names in both IdPs. Rationale: if IdP-2 groups mapped 1:1 onto GRC
   role names, whoever administers IdP-2 could assign `audit_compliance_admin` and gain full
   API access. With a per-issuer map + ceiling, IdP-2 tokens can **never** resolve to more
   than evidence-submission privileges, no matter what groups they carry.
2. **Route scoping:** IdP-2 tokens are valid **only** under `/api/v1/evidence-app/*`; 403
   everywhere else. The rest of the GRC API is unreachable from the portal even if privileges
   were misconfigured.
3. **Read API shape:** **one enriched endpoint** (`GET /api/v1/evidence-app/controls`) that
   returns everything the portal team asked for (audit, product, framework, control) in one
   self-scoped query — no enumerable IDs on the read side, so no read-side IDOR surface.

---

## 2. Target request pipeline

```
Evidence Portal (IdP-2 Bearer token, user email)
  │  HTTPS
  ▼
Choreo API Gateway                 TLS termination + perimeter throttling
  ▼
[1] Auth middleware                multi-issuer: pick IdP by `iss`, verify sig (per-IdP JWKS),
                                   iss/aud/exp/RS256; REJECT unknown issuers (401)
                                   per-issuer group→role map + privilege CEILING
                                   ctx gains: email, groups→privileges, issuer scope
  ▼
[2] Scope guard                    issuer scope "evidence-app" ⇒ path must be /api/v1/evidence-app/* (403)
  ▼
[3] Rate limiter                   token bucket per authenticated email ⇒ 429 + Retry-After
  ▼
[4] Handler                        RequirePrivilege(SUBMIT_EVIDENCE)                    (privilege)
                                   + IsAssignedForEvidence(email, controlID)            (resource, 403)
                                   + folderPath == audits/{auditID}/controls/{controlID}/evidence/{ts}/
                                     with auditID re-derived from the control row       (binding, 400)
  ▼
Service → Compliance Entity (/files, blob-path allowlist) → Azure Blob (account key stays in entity)
                            └────→ MySQL (evidence rows + append-only audit_trail)
```

- **Identity** = `email` claim (same person in both IdPs; matches `user.email`).
- **Privilege** = mapped groups → `privilege.Store.Resolve` → intersect with ceiling.
- **Resource** = team assignment: `user.audit_team_id → audit_team → audit_control.team_id`.

---

## 3. API contract (what the portal team gets)

All endpoints: `Authorization: Bearer <IdP-2 access token>` over HTTPS.
`controlId` below is the globally unique `audit_control.id`; the backend derives the audit
from the control row — the portal never supplies an audit id, which removes one
cross-parameter check that could be gotten wrong.

### 3.1 `GET /api/v1/evidence-app/controls`

Returns every control the user's team must act on (audit `ACTIVE`), enriched with
audit/product/framework. Status filter:
- **DESIGN controls:** `EVIDENCE_PENDING`, `EVIDENCE_NEED_CLARIFICATION`, `SUBMITTED_SAMPLE`
- **OE controls:** above **+** `POPULATION_PENDING`, `POPULATION_NEED_CLARIFICATION`

Each item includes `requirementType` (`DESIGN`|`OE`) and a computed `phase`
(`POPULATION`|`EVIDENCE`) so the portal knows which submission flow to show.
`baseFolderPath` is phase-aware (`…/population/` vs `…/evidence/`):

```json
[
  {
    "audit": {
      "id": 5,
      "name": "SOC2 2026",
      "product": "Choreo",
      "framework": "SOC 2",
      "periodStart": "2026-01-01",
      "periodEnd": "2026-12-31"
    },
    "control": {
      "id": 12,
      "number": "CC6.1",
      "description": "Logical access controls restrict access…",
      "evidenceRequirement": "Screenshots of IAM policy…",
      "requirementType": "DESIGN",
      "status": "EVIDENCE_PENDING",
      "phase": "EVIDENCE",
      "dueDate": "2026-08-01"
    },
    "baseFolderPath": "audits/5/controls/12/evidence/"
  },
  {
    "audit": {
      "id": 5,
      "name": "SOC2 FY26",
      "product": "Choreo",
      "framework": "SOC 2",
      "periodStart": "2026-01-01",
      "periodEnd": "2026-12-31"
    },
    "control": {
      "id": 17,
      "number": "CC7.2",
      "description": "System monitoring controls detect anomalous activity…",
      "evidenceRequirement": "Population of all access events for the period…",
      "requirementType": "OE",
      "status": "POPULATION_PENDING",
      "phase": "POPULATION",
      "dueDate": "2026-08-15"
    },
    "baseFolderPath": "audits/5/controls/17/population/"
  }
]
```

### 3.2 `GET /api/v1/evidence-app/controls/{controlId}/upload-link`

Starts an upload session. Returns **only a path string** (no storage credential — per threat
model [05], "no storage credential is issued"):

```json
{ "folderPath": "audits/5/controls/12/evidence/1751500000/", "expiresAt": "2026-07-09T14:00:00Z" }
```

### 3.3 `POST /api/v1/evidence-app/controls/{controlId}/upload`

`multipart/form-data`, one call per file. Fields: `folderPath` (from 3.2, verbatim), `file`.
Max **25 MiB**/file. Content-Type is sniffed server-side; filename is stripped to its base name.
Response `201`: `{ "fileName": "iam-policy.png", "size": 48211 }`

### 3.4 `POST /api/v1/evidence-app/controls/{controlId}/submit`

Body: `{ "folderPath": "audits/5/controls/12/evidence/1751500000/" }`.
Records the uploaded blobs in the DB, writes the audit trail, and advances the control to
`EVIDENCE_INTERNAL_REVIEW`. Response `201` with the evidence record.

---

### 3.5 Population endpoints (OE controls only)

OE-type controls go through a **population phase before evidence**. The team uploads a
population dataset (e.g. full access-event log), compliance reviews it, selects a sample,
and then the evidence phase begins. Use these endpoints when `requirementType == "OE"` and
`phase == "POPULATION"`. Calling any of them on a DESIGN control returns **409**.

#### 3.5.1 `GET /api/v1/evidence-app/controls/{controlId}/population/upload-link`

The backend resolves the active `audit_population` record for the control (status `PENDING`
or `COMPLIANCE_REJECTED`) and returns its folder path. The path segment after `population/`
is the stable `audit_population.id` — not a timestamp:

```json
{ "folderPath": "audits/5/controls/17/population/3/", "expiresAt": "2026-07-09T14:00:00Z" }
```

Use this `folderPath` verbatim in 3.5.2 and 3.5.3.

#### 3.5.2 `POST /api/v1/evidence-app/controls/{controlId}/population/upload`

Same multipart shape as 3.3 (`folderPath` + `file`, max 25 MiB). Files land under
`audits/{auditID}/controls/{controlID}/population/{populationID}/` in Azure.

Response `201`: `{ "fileName": "access-events.csv", "size": 204800 }`

#### 3.5.3 `POST /api/v1/evidence-app/controls/{controlId}/population/submit`

Body: `{ "folderPath": "audits/5/controls/17/population/3/" }`.

The backend:
1. Validates the folderPath matches the derived `auditID` / `controlID` / `populationID`.
2. Lists blobs at that prefix via the Compliance Entity.
3. Creates `audit_evidence_file` rows with `population_id = 3`, `file_kind = 'POPULATION'`.
4. Advances `audit_population.status` → `SUBMITTED`.
5. Advances `audit_control.status` → `POPULATION_INTERNAL_REVIEW`.

Response `201` with the population record. The control will no longer appear in the
`GET /controls` list after this call (status is no longer in the filter).

---

### 3.6 Error contract

| Status | Meaning | Portal action |
|---|---|---|
| **401** | Missing/invalid/expired token, or unknown issuer | Re-authenticate against IdP-2; retry once |
| **403** | Not assigned to this control / group not mapped / route outside evidence-app scope | Do not retry; surface "not authorized" |
| **400** | `folderPath` doesn't match the control's folder, or malformed request | Use the folderPath from 3.2 / 3.5.1 verbatim |
| **409** | Population endpoint called on a DESIGN control | Use evidence endpoints (3.2–3.4) instead |
| **413** | File > 25 MiB | Reject/compress before upload |
| **422** | Submit with no files uploaded in that folder | Upload (3.3 / 3.5.2) must succeed first |
| **429** | Rate limited | Back off; honor `Retry-After` |

### 3.7 Portal must-nots
- Do **not** fabricate or reuse a `folderPath` from another control/session.
- Do **not** submit for controls not returned by 3.1.
- Do **not** send IdP credentials to the GRC backend — Bearer token only.
- Do **not** treat its own IdP roles as GRC authorization — the GRC backend decides.

### 3.8 Keeping the control list fresh (pull model — agreed 2026-07-11)

The integration is **pull-only**: GRC never calls the portal; the portal fetches state
via 3.1 whenever it needs it. There is no push/webhook channel, and the portal team
does not need to host any receiving API. Recommended portal-side refresh patterns:

1. **Fetch on page load** (required) — call 3.1 every time the controls page is
   opened/navigated to. Covers the vast majority of cases.
2. **Auto-refresh** (recommended) — re-call 3.1 every ~60 s while the tab is visible.
   Well within the rate limit (10 r/s per user).
3. **Manual refresh button** (optional) — same call, user-triggered.

The list is self-correcting because 3.1 filters by actionable statuses only:
after a successful submit the control leaves the list on the next fetch
(status advanced to `*_INTERNAL_REVIEW`); if a reviewer sends it back
(`EVIDENCE_NEED_CLARIFICATION` / `POPULATION_NEED_CLARIFICATION`) it reappears.
The portal must **not** cache or persist control data beyond the current view.

---

## 4. Backend changes (`apps/grc-platform/backend`)

### A. Multi-issuer configuration — `internal/config/config.go`

Replace the scalar auth fields with a list of IdPs (keep `ClockSkew`,
`TokenValidatorEnabled` top-level):

```go
type IdPConfig struct {
    Issuer       string
    JWKSEndpoint string
    Audience     string
    Scope        string            // "full" | "evidence-app"
    GroupRoleMap map[string]string // external group -> GRC role name; nil = identity map
}

type AuthConfig struct {
    IdPs                  []IdPConfig
    ClockSkew             time.Duration
    TokenValidatorEnabled bool
}
```

Env parsing:

| Env var | IdP | Notes |
|---|---|---|
| `AUTH_ISSUER` / `AUTH_JWKS_ENDPOINT` / `AUTH_AUDIENCE` | IdP-1 | unchanged; `Scope=full`, `GroupRoleMap=nil` |
| `AUTH_ISSUER_2` / `AUTH_JWKS_ENDPOINT_2` / `AUTH_AUDIENCE_2` | IdP-2 | **optional** — appended only when `AUTH_ISSUER_2` is set, so single-IdP deploys are unchanged; `Scope=evidence-app` |
| `AUTH_GROUP_ROLE_MAP_2` | IdP-2 | e.g. `grc_evidence_submitter=audit_internal_team` (comma-separated `ext=grc` pairs) |

Validation at startup: when `AUTH_ISSUER_2` is set, all of `AUTH_JWKS_ENDPOINT_2`,
`AUTH_AUDIENCE_2`, `AUTH_GROUP_ROLE_MAP_2` must be set (fail fast, same style as existing
required-var checks).

### B. Multi-issuer JWT validation — `internal/middleware/auth.go`

- `Config` takes `IdPs []IdPConfig` instead of the three scalars.
- Build **one `jwksCache` per IdP**, indexed by issuer (reuse the existing `newJWKSCache`
  — do not re-implement the cache).
- In `extractUserInfo`:
  1. `jwt.NewParser().ParseUnverified` **once** to read `iss` only (never trust anything
     else from the unverified parse).
  2. Select the IdP whose `Issuer` matches; **unknown issuer → 401** (same generic
     "invalid token" message; do not leak which issuers are configured).
  3. `jwt.ParseWithClaims` with that IdP's keyFunc + `jwt.WithIssuer(idp.Issuer)`,
     `jwt.WithAudience(idp.Audience)`, `jwt.WithLeeway(cfg.ClockSkew)`,
     `jwt.WithExpirationRequired()`, `jwt.WithValidMethods([]string{"RS256"})` —
     identical hardening to today (auth.go ~275-280).
- `UserInfo` gains two fields: `Issuer string`, `Scope string` (from the matched IdP).
- **Group mapping + ceiling** (before `PrivilegeStore.Resolve`):

```go
groups := info.Groups
if idp.GroupRoleMap != nil {
    mapped := make([]string, 0, len(groups))
    for _, g := range groups {
        if r, ok := idp.GroupRoleMap[g]; ok {   // unmapped groups are DROPPED
            mapped = append(mapped, r)
        }
    }
    groups = mapped
}
privs := cfg.PrivilegeStore.Resolve(groups)
if idp.Scope == config.ScopeEvidenceApp {
    privs = intersect(privs, evidenceAppPrivilegeCeiling) // {SUBMIT_EVIDENCE}
}
ctx = privilege.WithContext(ctx, privs)
```

`evidenceAppPrivilegeCeiling = map[string]bool{privilege.SubmitEvidence: true}` — the single
source of truth for what an evidence-app-scoped token may ever do. Even a bad
`AUTH_GROUP_ROLE_MAP_2` (e.g. someone maps to `audit_compliance_admin`) cannot exceed it.

- `cmd/server/main.go`: pass `IdPs: cfg.Auth.IdPs` into `middleware.Config`.

### C. Scope guard — `internal/middleware/scope.go` (new)

```go
// After Auth. Evidence-app-scoped tokens may only reach /api/v1/evidence-app/*.
func IssuerScope(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        info := UserInfoFromContext(r.Context())
        if info != nil && info.Scope == config.ScopeEvidenceApp &&
            !strings.HasPrefix(r.URL.Path, "/api/v1/evidence-app/") {
            response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

Wire in `cmd/server/main.go` immediately after `middleware.Auth`.

### D. Rate limiting — `internal/middleware/ratelimit.go` (new)

- Token bucket via `golang.org/x/time/rate`, keyed by authenticated email (fallback:
  client IP from `X-Forwarded-For` first hop / `RemoteAddr`).
- `map[string]*bucket` + `sync.Mutex`, idle eviction (goroutine sweeping entries unused
  > 10 min).
- Exceeded → **429** with `Retry-After` header.
- Suggested defaults: **10 req/s, burst 20** per caller (uploads are ≤ 25 MiB each; a whole
  submission session is tens of requests). Tune via env if needed later.
- Applied to the `/api/v1/evidence-app/*` route group only (the web app has its own UX
  pacing; the Choreo gateway still throttles globally per threat model [01]/[05]).
- Per-replica in-memory is acceptable: it sits **behind** the Choreo gateway's perimeter
  limits and is a per-principal fairness control, not the only DoS defense.

### E. Enriched controls query — Compliance Entity + backend passthrough

Extend the existing `ListAssignedForEvidence` join
(`entity/compliance-entity/internal/repository/audit_control_repo.go:48-74`) with
product/framework:

```sql
SELECT a.id, a.name, p.name AS product, f.name AS framework,
       a.period_start, a.period_end,
       c.id, c.control_number, c.description, c.evidence_requirement,
       COALESCE(fc.requirement_type, c.requirement_type) AS requirement_type,
       c.status, c.due_date
FROM audit_control c
LEFT JOIN audit_framework_control fc ON fc.id = c.framework_control_id
JOIN audit           a ON a.id = c.audit_id
JOIN audit_product   p ON p.id = a.product_id
JOIN audit_framework f ON f.id = a.framework_id
JOIN audit_team      t ON t.id = c.team_id
JOIN `user`          u ON u.audit_team_id = t.id
WHERE u.email = ?
  AND a.status = 'ACTIVE'
  AND c.status IN (
    'POPULATION_PENDING','POPULATION_NEED_CLARIFICATION',
    'EVIDENCE_PENDING','EVIDENCE_NEED_CLARIFICATION','SUBMITTED_SAMPLE'
  )
ORDER BY a.id, c.control_number
```

Update the entity DTO + backend `AssignedControlForEvidence` model to the nested `audit`/`control`
shape in §3.1, adding `RequirementType` to the `control` struct. The backend computes two
derived fields before serializing:
- **`phase`**: `"POPULATION"` if `status` starts with `"POPULATION_"`, else `"EVIDENCE"`.
- **`baseFolderPath`**: `"audits/%d/controls/%d/population/"` for population phase,
  `"audits/%d/controls/%d/evidence/"` otherwise — both computed server-side, never trusted
  from the client.

(The web app does not consume this endpoint, so the shape change is safe — verify with a
grep of the frontend before merging.)

### F. Resource-level authorization (IDOR fix) — both route groups

1. **Entity repo** (`audit_control_repo.go`) — add:

```sql
-- IsAssigned(email, controlID): may this user submit for this control right now?
-- Covers both population and evidence phases.
SELECT c.audit_id
FROM audit_control c
JOIN audit      a ON a.id = c.audit_id
JOIN audit_team t ON t.id = c.team_id
JOIN `user`     u ON u.audit_team_id = t.id
WHERE u.email = ? AND c.id = ?
  AND a.status = 'ACTIVE'
  AND c.status IN (
    'POPULATION_PENDING','POPULATION_NEED_CLARIFICATION',
    'EVIDENCE_PENDING','EVIDENCE_NEED_CLARIFICATION','SUBMITTED_SAMPLE'
  )
LIMIT 1
```

   Returning `audit_id` kills two birds: the assignment check **and** the server-side
   derivation of the audit for folderPath binding. Not found → not assigned (403).
2. **Entity handler**: `GET /audit-controls/{controlId}/evidence-assignment?email=…` →
   `200 {"auditId": 5}` or `404` (follow the entity's existing route/DTO conventions).
3. **Backend**: entity-client method + `ControlService.AssignedAuditID(ctx, email, controlID)
   (int, bool, error)`.
4. **Enforce** in upload-link, upload, submit — in the **new** `/evidence-app/*` handlers
   *and* the **existing** web-app evidence handlers (`evidence.go`), since the web-app routes
   have the same IDOR today. Not on `listEvidence`/`downloadEvidenceFile` (those are
   `REVIEW_EVIDENCE` for compliance/auditors) and not on `getAssignedControls`
   (already self-scoped by email).
5. **Entity repo** — add `FindActivePopulation(ctx, controlID)`:

```sql
-- Find the active population record for an OE control (PENDING = first submission;
-- COMPLIANCE_REJECTED = need clarification, team must re-upload).
SELECT id FROM audit_population
WHERE control_id = ? AND status IN ('PENDING','COMPLIANCE_REJECTED')
ORDER BY id DESC LIMIT 1
```

   Entity endpoint: `GET /audit-controls/{controlId}/active-population` →
   `200 {"populationId": 3}` or `404` (no active population / DESIGN control).
   Backend client: `ControlService.ActivePopulationID(ctx, controlID) (int, error)`.
   Used by the population upload-link and population submit handlers to resolve the target
   `audit_population` record and bind the folder path.

### G. folderPath binding — `internal/audit/service/evidence.go`

In `UploadFile` and `Submit`, require the path to exactly match the server-derived value
based on which phase the control is in:

**Evidence phase** (DESIGN controls + OE controls in evidence phase):
```
folderPath == fmt.Sprintf("audits/%d/controls/%d/evidence/%s/", auditID, controlID, sessionTs)
```
`auditID` from §F `AssignedAuditID` (never trusted from client); `sessionTs` is
**digits-only** (`^[0-9]+$`).

**Population phase** (OE controls only):
```
folderPath == fmt.Sprintf("audits/%d/controls/%d/population/%d/", auditID, controlID, populationID)
```
`populationID` from §F `FindActivePopulation` (the stable `audit_population.id`, not a
timestamp). Path segment must be a valid integer matching an active population for this control.

Anything else → 400. Existing guards unchanged: filename `filepath.Base`, no path
separators, 25 MiB `MaxBytesReader`, content-type sniffing.

This closes gap #4 for both phases: a caller cannot aim bytes at another control's folder
or another population's folder, because the path is recomputed from the route + DB exactly
as the threat model claims.

### H. New route group — `internal/audit/handler/routes.go` + `evidence_app.go` (new handler file)

```go
// Evidence Portal proxy API (IdP-2 scope; also callable by IdP-1 users with SUBMIT_EVIDENCE)
mux.Handle("GET  /api/v1/evidence-app/controls",                                    rl(eah.listControls))
// Evidence phase (DESIGN controls + OE controls once past population phase)
mux.Handle("GET  /api/v1/evidence-app/controls/{controlId}/upload-link",            rl(eah.uploadLink))
mux.Handle("POST /api/v1/evidence-app/controls/{controlId}/upload",                 rl(eah.upload))
mux.Handle("POST /api/v1/evidence-app/controls/{controlId}/submit",                 rl(eah.submit))
// Population phase (OE controls only — 409 if control requirement_type is DESIGN)
mux.Handle("GET  /api/v1/evidence-app/controls/{controlId}/population/upload-link", rl(eah.populationUploadLink))
mux.Handle("POST /api/v1/evidence-app/controls/{controlId}/population/upload",      rl(eah.populationUpload))
mux.Handle("POST /api/v1/evidence-app/controls/{controlId}/population/submit",      rl(eah.populationSubmit))
```

(`rl` = rate-limit wrapper from §D.) Handlers are thin: privilege check → assignment check
(§F, which also yields `auditID`) → delegate to the **same** `EvidenceService` methods the
web-app handlers use. The old `GET /api/v1/evidence-app/controls` handler
(`getAssignedControls`) is replaced by the enriched version. The web-app
`/api/v1/audits/{id}/controls/{controlId}/evidence/*` routes stay for the SPA, now with §F/§G
enforcement added; the `{id}` audit param is validated against the derived `auditID`
(mismatch → 404).

### I. Audit trail attribution

On submit, include `"via": "evidence-app"` and the token issuer in the `audit_trail`
`details` JSON when the request came from an evidence-app-scoped token — portal actions stay
distinguishable from web-app actions (supports the threat model's non-repudiation claims,
interaction [02]/[05]).

---

## 5. Compliance Entity changes (`entity/compliance-entity`)

**Blob-path allowlist (defense in depth)** — `internal/handler/file_handler.go` (or a small
`validateBlobName` helper in `storage`): reject any `blobName`/`prefix` that does not match
the known layouts:

```
^audits/[0-9]+/controls/[0-9]+/evidence/[0-9]+/[^/]+$      (evidence file)
^audits/[0-9]+/controls/[0-9]+/evidence/[0-9]+/$            (evidence list prefix)
^audits/[0-9]+/controls/[0-9]+/population/[0-9]+/[^/]+$    (population file)
^audits/[0-9]+/controls/[0-9]+/population/[0-9]+/$          (population list prefix)
+ the existing sample layouts — enumerate every current caller of
  UploadBlob/ListBlobs/ReadBlob/Delete in the backend and add exactly those patterns,
  nothing broader.
```

Non-matching → **400**. Also reject `..`, backslashes, and empty segments outright. The
entity stops blindly trusting callers for paths; even a compromised or buggy backend cannot
write outside the evidence tree. (Closes gap #5.)

No auth change in the entity: it stays internal-only (Choreo network isolation, threat model
[02] threat 6), trusting the backend as attributor.

---

## 6. Asgardeo / Choreo configuration (no code)

**IdP-2 Asgardeo org (their side, with our review):**
1. Register the GRC backend API as an **API resource** and authorize the portal app for it,
   so issued tokens carry `aud = AUTH_AUDIENCE_2`.
2. Create **one group**: `grc_evidence_submitter`. Assign exactly the portal users who submit
   evidence. (They may create other groups for their own app; ours ignores them — unmapped
   groups are dropped.)
3. Ensure tokens include a **verified `email`** claim equal to the user's WSO2 email (must
   match `user.email` in the GRC DB) and the `groups` claim.

**Choreo (GRC backend component → Configs & Secrets), per environment:**

```
AUTH_ISSUER_2=https://api.asgardeo.io/t/<their-org>/oauth2/token
AUTH_JWKS_ENDPOINT_2=https://api.asgardeo.io/t/<their-org>/oauth2/jwks
AUTH_AUDIENCE_2=<grc-backend-api-audience-in-their-org>
AUTH_GROUP_ROLE_MAP_2=grc_evidence_submitter=audit_internal_team
```

**Data prerequisite:** portal users must exist in the `user` table with `audit_team_id` set
(HR-entity sync / admin UI). A user missing there simply gets an empty controls list and 403
on submission — fail closed, no error leakage.

**Choreo gateway:** keep perimeter rate limiting on the backend component (threat model [01]
threat 4); the in-app limiter (§4D) adds per-principal fairness behind it.

---

## 7. Implementation order (steps to do)

| Step | What | Files | Verifiable outcome |
|---|---|---|---|
| 1 | Multi-IdP `AuthConfig` + env parsing + startup validation | `internal/config/config.go` | unit test: 1-IdP and 2-IdP env parse; missing `_2` var fails fast |
| 2 | Multi-issuer auth middleware (per-issuer JWKS cache, `iss` selection, `UserInfo.Issuer/Scope`) | `internal/middleware/auth.go`, `cmd/server/main.go` | IdP-1 regression green; unknown issuer → 401 |
| 3 | Group→role map + privilege ceiling | `internal/middleware/auth.go` (+ helper) | mapped group resolves; unmapped dropped; ceiling caps admin-mapped group |
| 4 | Scope-guard middleware | `internal/middleware/scope.go`, `main.go` | IdP-2 token on `/api/v1/audits` → 403 |
| 5 | Entity: enriched assigned-controls query (+ `requirement_type`, population statuses) + `IsAssigned` (returns `audit_id`) + `FindActivePopulation` + endpoints | `entity/.../audit_control_repo.go`, entity handler/routes | curl entity endpoints directly |
| 6 | Backend entity-client + `ControlService.AssignedAuditID` | `internal/shared/...` client, `internal/audit/service/control.go` | unit test with fake entity |
| 7 | folderPath binding in `UploadFile`/`Submit` | `internal/audit/service/evidence.go` | foreign folderPath → 400 |
| 8 | New `/evidence-app/*` handlers + routes (evidence + OE population); add §F checks to existing web-app evidence handlers | `internal/audit/handler/evidence_app.go`, `evidence.go`, `routes.go` | full portal evidence + OE population flows work; unassigned → 403; DESIGN control on population endpoint → 409 |
| 9 | Rate-limit middleware on the group | `internal/middleware/ratelimit.go` | burst → 429 + Retry-After |
| 10 | Audit-trail `via` tagging | evidence submit path | trail row shows `"via":"evidence-app"` |
| 11 | Entity blob-path allowlist | `entity/.../file_handler.go` | malformed blobName → 400; existing flows unaffected |
| 12 | Choreo/Asgardeo config (both orgs) + deploy | Choreo console | verification matrix below |

Steps 5–8 and 11 also fix the web app's existing IDOR/path gaps — they are not portal-only.

---

## 8. Verification matrix

| # | Test | Expected |
|---|---|---|
| 1 | IdP-1 web-app token: full evidence flow (regression) | unchanged behavior |
| 2 | IdP-2 token, assigned control: controls → upload-link → upload → submit | 200/201s; control → `EVIDENCE_INTERNAL_REVIEW`; trail has `via: evidence-app` |
| 3 | IdP-2 token calls `/api/v1/audits/...` or any non-evidence-app route | **403** |
| 4 | IdP-2 token, control **not** on user's team (upload-link/upload/submit) | **403** |
| 5 | `folderPath` pointing at another control / non-numeric session segment | **400** |
| 6 | Token from a third, unconfigured issuer | **401** |
| 7 | IdP-2 token whose groups are not in `AUTH_GROUP_ROLE_MAP_2` | **403** (no privileges) |
| 8 | IdP-2 group maliciously mapped to an admin role (config error simulation) | still only `SUBMIT_EVIDENCE` (ceiling) |
| 9 | Burst > limit on an evidence-app endpoint | **429** + `Retry-After` |
| 10 | Entity `POST /files` with blobName outside allowed layouts / containing `..` | **400** |
| 11 | Email in IdP-2 token not present in `user` table | empty controls list; submit → 403 |
| 12 | OE control in `POPULATION_PENDING`: population upload-link → upload → submit | `200`/`201`; control → `POPULATION_INTERNAL_REVIEW`; files in `audit_evidence_file` with `population_id` set and `file_kind='POPULATION'` |
| 13 | OE control in `POPULATION_NEED_CLARIFICATION`: re-submit population | resolves `COMPLIANCE_REJECTED` population record; same 201 flow |
| 14 | DESIGN control calls `GET …/population/upload-link` | **409** |
| 15 | `folderPath` with wrong `populationID` in path | **400** |
| 16 | `go build ./... && go vet ./...` (backend + entity) | clean |

---

## 9. Explicitly out of scope (per threat model)

- IdP-2 account security, MFA, token lifetime (Asgardeo-owned).
- Security of the machine running the Evidence Portal (user endpoint, out of scope).
- Stolen/replayed legitimate tokens (out-of-scope interaction; short token TTLs are the
  Asgardeo-side control).
- AV/content scanning of uploads beyond the existing size/type gate (tracked separately in
  threat model [03] threat 2).
