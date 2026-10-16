# risk-register-migration

One-time migration that loads the historical **Managed Services** risk register
into the platform database by driving the **compliance-entity HTTP API**.
Deployed as a **Choreo Manual Task** (Go).

It imports the Managed Services register only. The original register migration
is done, and WSO2 Cloud has no existing risks, so a row on any other register is
rejected. The original behaviour is in git history. Design:
`RISK_MODULE_DESIGN.md` §14 (Phase 2).

Full design: `RISK_REGISTER_CSV_MIGRATION_PLAN.md` (a planning doc kept outside
this repo). Section references (§n) in the code comments point there.

## What it does

1. **Preflight** (§7) — entity health, SCIM reachable, the three risk roles
   `ACTIVE`, reference data populated (including customers, products and
   deployment types), at least one register on the `MANAGED_SERVICES` template,
   CSV headers present. Any failure aborts before a single write.
2. **Parse + map** the CSV (§4, §6) — header-name matched, legend/blank rows
   skipped, values transformed, bad rows → `REJECT` findings.
3. **Resolve people** (§6) — work email → uuid via the SCIM org snapshot →
   `user.id` via `GET /users/by-uuid` (else `POST /users`, never mutating an
   existing row).
4. **Dry run** (default) stops here and prints the report.
5. **Reconstruct resume state** from the entity (§8) — no ledger file.
6. **Write** each row in `Migration ID` order (§5):
   - `POST /risks` (born `PENDING_RISK_OWNER_APPROVAL`, `created_by` = marker;
     `likelihood`/`impact` in this call are the row's **Gross** values, and
     set the immutable `gross_score_id`; also `customerId`, `deploymentTypeId`,
     `productIds` and `environments`. The entity assigns the risk code and
     bumps the per-customer counter in the same transaction)
   - Residual differs from Gross → `POST /risks/{id}/assessments` with the
     row's **Residual** values, so the CSV's current-state numbers show up
     as a real reassessment (skipped entirely when Residual == Gross)
   - walk `workflow_status` to the row's bucket via `PATCH` (no-op when
     `from == to`)
   - `APPROVED` + overdue → `POST /risks/{id}/escalations` to suppress the
     nightly escalation job
   - `APPROVED` → ensure the §6 role grants (the ACCEPT+HIGH management
     grant is gated on **Gross** likelihood × impact ≥ 7, matching the live
     backend's gross-based approval rule)
   - `CLOSED` → also `PATCH` the action plan to `COMPLETED`
7. **Verify** (real run only, `verify.go`) — re-reads every migratable row back
   from the entity (`GET /risks/{id}/detail` + escalations + grants, plus
   `GET /risks/{id}/assessments` for any row where Residual differs from
   Gross) and diffs it, field by field, against the CSV. That includes:
   - customer and deployment type (by id), products and environments (as sets),
     and that no compliance reference is present;
   - the entity-assigned risk code: it must start with the row's own
     `YEAR-REGISTER-CUSTOMER-QUARTER-` followed by a number. The number itself
     is not checked, because a row the entity rejected earlier shifts it;
   - unexpected grants: only RISK_TEAM grants on teams this sheet touches are
     inspected, because the original migration's grants carry the same marker.
     Stale GLOBAL grants go unflagged.

   It also confirms every rejected row still has no matching risk. It runs
   unconditionally, covering every migratable row — including ones already
   complete from an earlier run and `Skipped` this time — not just what this
   invocation wrote. A disagreement becomes a `MISMATCH` finding in the same
   report as `REJECT`/`WARN`.

## The Managed Services sheet

The CSV format handed to the sheet owner is
`RISK_REGISTER_CSV_FORMAT.md` (kept outside this repo, in the planning-docs
`handoff/managed-services-risk-register/` folder, with an example CSV and the
allowed-values list). In short:

- **Columns:** the original columns **minus** `Security Compliance Reference`,
  **plus** `Customer`, `Deployment Type`, `Product` and `Environment`. All four
  new ones are mandatory on every row. The compliance column is optional: if it
  is left in, every cell must be blank.
- **Register:** `Source Register` must be the Managed Services register (name or
  code). `Assignment Team` is any existing active team, as everywhere else (the
  entity only checks that it exists); for this sheet it is always the Managed
  Services team.
- **Lookups:** `Customer`, `Product` and `Deployment Type` are matched by name
  (case and spacing ignored) against the entity's lists. An unknown or inactive
  value rejects the row; the tool never creates lookup values. `Product` and
  `Environment` take several values separated by `;` (`,` and line breaks are
  tolerated). `Environment` is exactly `Production`, `Non-Production` or `DR`.
- **Row key:** title + source register + **customer** + year + quarter, used for
  the duplicate check, resume matching and verification. Two customers may
  share a title in one quarter. The entity's search returns the customer *name*,
  so a customer renamed between a run and its resume would stop matching.
- **Risk numbers:** assigned by the entity, per (register, customer), in the
  order the tool writes rows, which is Migration ID order. The sheet owner sorts
  the sheet by Year, Quarter, then original order before numbering Migration ID.
  A row rejected now and fixed in a later run takes the next number then, so get
  the dry run clean before the real run.

### Before a run (per environment)

1. Phase 1 deployed: the register-template schema, the Compliance Entity, the
   backend and the frontend.
2. The `Managed Services` register exists with the `MANAGED_SERVICES` template
   (Admin Console). Staging must have no leftover test risks on it, because the
   template is locked once a risk uses it.
3. The lookup seed script has run (customers with codes, deployment types,
   products). Customer codes are frozen once a risk uses them; check them first.
4. The assignment team (Managed Services) exists. The tool creates its Risk
   Owner grants itself for every `APPROVED` row.

### What the dry run prints

A dry run reaches the same row-level verdicts as the real run, so a clean dry
run is not followed by a real run that rejects rows: besides everything the
parser rejects, it REJECTs rows that share a natural key with another row in the
sheet, and rows that already match two marker risks in the entity.

Besides `errors.csv`, `report.txt` gains, when there is something to say:

- **unknown values to add in the Admin Console** — each distinct unknown
  Customer / Deployment Type / Product, with its row count and first Migration
  IDs; **inactive values to reactivate**; and **invalid environments** (fix in
  the sheet).
- **per-customer summary** — risks by status, how many are new, and the first
  and last risk code the run would assign. The next number is read once from
  `GET /risks/next-sequence-number`, so nobody should raise a risk for these
  customers during the import. Check the codes before the real run.

## Buckets

Every row's added `Workflow Status` column is exactly `APPROVED` or
`CLOSED`. Anything else rejects the row. There is no brand-new / `DRAFT` /
`CANCELLED` bucket.

## Run it

Local (against reachable staging URLs) — copy `.env.example` to `.env`, fill in
the values, then:

```bash
set -a && source .env && set +a

go run . -input ./risks.csv -migration-date 2026-09-15          # dry run

# ... fix everything the report REJECTs, re-export, repeat ...

go run . -input ./risks.csv -migration-date 2026-09-15 -dry-run=false
```

`-input`, `-migration-date` and `-dry-run` stay explicit flags on every
invocation rather than living in `.env` — which CSV, which date, and whether
this run writes should never be a leftover value from an old file. `go run .
-h` prints the full flag surface; every `.choreo/component.yaml` config key
maps to a `Config` field in `main.go`, and `.env`/`.env.example` use the same
names.

In Choreo: set the config keys in `.choreo/component.yaml`, upload the CSV as
the `INPUT_PATH` file mount, leave `DRY_RUN=true`, trigger, read the logs; then
flip `DRY_RUN=false` and trigger again.

Exit codes: `0` clean · `2` completed with findings · `1` structural abort.

## Fully local end-to-end

Two ways to resolve identity locally, against the same local entity + local
MySQL either way — pick one:

- **`-scim-snapshot <file>`** — an `email,uuid` CSV read instead of calling
  Asgardeo. No Asgardeo credentials needed at all. **Local testing only —
  never set `SCIM_SNAPSHOT_FILE` in Choreo**, and pass it as a flag, not a
  `.env` var, so it can't leak into a staging/Choreo run by accident. When
  it's set, the `SCIM_INTERNAL_*` config becomes optional.
- **A real Asgardeo org** (your own personal/dev org is fine) — set
  `SCIM_BASE_URL` / `SCIM_INTERNAL_ORG` / `SCIM_INTERNAL_CLIENT_ID` /
  `SCIM_INTERNAL_CLIENT_SECRET` / `SCIM_INTERNAL_SCOPES`
  (`internal_user_mgt_view internal_user_mgt_list` — only those two) for real,
  set `SCIM_DOMAIN` to the **email suffix your org's users actually carry**
  (not the org name — they're different fields: `SCIM_INTERNAL_ORG` is the
  Asgardeo tenant path segment that goes in the URL), and drop
  `-scim-snapshot`. This exercises the real OAuth2 client-credentials +
  SCIM2 Users API path, not the fake.

Either way, the CSV's four person columns need emails that actually resolve —
either present in the snapshot file, or real users in the org/domain above.

```bash
# 1. local MySQL. shared.sql/risk_schema.sql both open with USE grc_platform
# and no CREATE DATABASE, so the database has to exist first.
mysql -uroot -p -e "CREATE DATABASE IF NOT EXISTS grc_platform"
mysql -uroot -p grc_platform < ../../apps/grc-platform/backend/Resources/shared.sql
mysql -uroot -p grc_platform < ../../apps/grc-platform/backend/Resources/risk_schema.sql
# risk roles. Must be this file, not the older root staging_shared_seed_data.sql:
# that one predates the management-role split and still carries the pre-split
# name grc-platform-management, so preflight aborts on the missing
# grc-platform-risk-management. Re-running is safe (ON DUPLICATE KEY UPDATE on
# uq_role_name, and the renames are no-ops once applied), and it fixes a DB
# seeded from the old file by renaming the role in place, keeping its role_id.
mysql -uroot -p grc_platform < ../../apps/grc-platform/backend/Resources/shared_seed_data.sql
# teams / categories / compliance refs / scores. NOT risk_module_data_schema.sql
# — that file seeds neither risk_team nor risk_score, and buildRefData aborts
# preflight when any of the four is empty.
mysql -uroot -p grc_platform < <path-to>/staging_risk_seed_data.sql
# Managed Services lookups (customers, deployment types, products, platforms).
mysql -uroot -p grc_platform < <path-to>/managed_services_lookup_seed_data.sql
# Then give the Managed Services register its template
# (risk_team.register_template = 'MANAGED_SERVICES'), as the Admin Console does;
# preflight aborts when no register is on that template.

# Run the seed files from the mysql CLI, not MySQL Workbench: Workbench's safe
# update mode rejects shared_seed_data.sql's `WHERE role_name COLLATE
# utf8mb4_bin IN (...)` with error 1175 (the COLLATE hides the uq_role_name
# index from it). `SET SQL_SAFE_UPDATES = 0;` in the same session also works.

# 2. compliance-entity (separate shell, leave running).
# Back up any .env you already have — this overwrites it.
cd ../../entity/compliance-entity
[ -f .env ] && cp .env .env.bak
# &tls=false is required: internal/db/mysql.go defaults a DSN with no tls= to
# verified TLS and will not fall back to plaintext, so a local MySQL without
# TLS just fails to connect.
printf 'DB_DSN=root:<password>@tcp(127.0.0.1:3306)/grc_platform?parseTime=true&tls=false\nSERVER_PORT=8080\n' > .env
go run ./cmd/api

# 3. confirm the entity is up before every run in this tool — cheap, and it's
# the fastest way to tell "the entity isn't running" apart from a real config
# problem when preflight fails.
curl -s localhost:8080/health
curl -s localhost:8080/risk/scores | head -c 200

# 4. this tool. Copy .env.example to .env, fill in COMPLIANCE_ENTITY_BASE_URL
# plus whichever identity path you picked above.
cd ../../operations/risk-register-migration
set -a && source .env && set +a
IN=<path-to-your-prepared-register>.csv

go run . -input "$IN" -migration-date 2026-09-15          # dry run
echo "exit=$?"

# ... fix everything the report REJECTs, re-run the dry run, repeat ...

go run . -input "$IN" -migration-date 2026-09-15 -dry-run=false   # real run
echo "exit=$?"
```

The tool now verifies itself automatically (see "Verification" above) — the
manual SQL below is a direct-to-DB cross-check, useful when you want to see
the raw rows yourself or double another way, not a required step:

```sql
SELECT risk_code, workflow_status, treatment_strategy
FROM risk WHERE created_by='risk-sheet-migration' ORDER BY risk_code;

SELECT COUNT(*) FROM `user`         WHERE created_by='risk-sheet-migration';
SELECT COUNT(*) FROM risk_escalation  WHERE created_by='risk-sheet-migration';
SELECT COUNT(*) FROM user_role_grant  WHERE created_by='risk-sheet-migration';
SELECT COUNT(*) FROM risk_action_plan WHERE created_by='risk-sheet-migration' AND status='COMPLETED';
```

Then re-run the exact same real-run command once more. Every row must come
back **Skipped** (`migrated=0 skipped(resume)=N`), and none of the counts
above may change — this is the resume path, and the property a retriggered
Choreo Manual Task actually depends on. Reset with `rollback.sql` between
attempts (it deliberately leaves `user` rows alone — see its header comment;
delete those separately if you're switching identity source and want a fully
clean slate).

A `testdata/risks.csv`-shaped register (5 data rows: 3 `APPROVED`, 2
`CLOSED`, one `ACCEPT`/high row that also gets the management grant, across two
customers and three assignment teams) is a good size for a first local pass before
trying the real register export.

## Report

The run prints two blocks to stdout in a single write (captured by Choreo —
see the note in `report.go` on why it's one `Write` call, not several):

- **`errors.csv`** — one row per finding: `migration_id, csv_row, risk_title,
  severity (REJECT|WARN|MISMATCH), failure, detail`. `MISMATCH` findings come
  from the post-write verification pass (`verify.go`), not the write pipeline
  itself — see below.
- **`report.txt`** — findings-by-code counts, the distinct unresolved people,
  the Migration IDs that got a suppressing escalation, the grant count, and
  (real run) per-bucket migrated counts.

A real run also prints a third block, **`risk_codes.csv`** (`migration_id,
customer, risk_code, risk_title`, sorted by Migration ID): the code the entity
assigned to every risk the verification pass matched, including ones written by
an earlier run. Send it to the sheet owner so she can match her rows to the
numbers, which are permanent and go out in emails and Git issues. It is not
printed on a dry run, because there are no codes yet.

A real run additionally logs one `verifying` progress line every 10 rows and
one `verification complete` summary line (verified-ok / mismatch / confirmed-
absent-rejected counts) via `slog`, from the verification pass described next.

## Verification

Every real run ends with an automatic, read-only pass (`verify.go`) that
re-reads the entity and checks it agrees with the CSV — reusing the same
compliance-entity API this tool writes through, never MySQL directly:

- Every **migratable** row (not just ones this run wrote — a resumed run's
  already-`Skipped` rows are re-checked too) gets its risk fetched via
  `GET /risks/{id}/detail` and diffed field by field: title, description,
  register/team/category/compliance-ref ids, owner/assigner/mgmt-approver/
  action-owner ids, dates, gross likelihood/impact, treatment strategy,
  workflow status, and the action plan (status, description, steps, and —
  for `CLOSED` — its completed date). Escalations and grants are checked both
  ways: missing (expected but absent) and unexpected (a marker-created
  escalation/grant present that no migratable row calls for).
- Every row whose Residual differs from its Gross also gets a
  `GET /risks/{id}/assessments` call: a marker-authored entry must exist with
  residual likelihood/impact matching the CSV, or it's a `MISMATCH` — either
  "no residual assessment" (missing) or a value diff (wrong).
- Every **rejected** row gets a cheap negative check: no marker-created risk
  should exist for it.

A disagreement is a `MISMATCH` finding in the same report as `REJECT`/`WARN`
and trips the same `exitFindings` (2) exit code — there's no separate flag to
disable verification and no new exit code.

## Rollback

`rollback.sql`, run by hand against the target DB. The tool never deletes.

The `risk-sheet-migration` marker is **not unique to this import**: the original
register migration wrote its risks and grants under the same marker in the same
database, so deleting by marker alone would delete those too. The script
selects this import's risks as those with the marker **and** a
`risk_managed_service_detail` row, and deletes their children by risk id.

Grants are handled more carefully, because a grant row can be shared. The entity
creates grants with `ON DUPLICATE KEY`, which never changes `created_by`: when
the original migration and this import grant the same person the same role on
the same team there is one row, still carrying the marker. So the script deletes
a marker RISK_TEAM grant only on a team that **no other risk uses** (as source
register or assignment team). Grants on a team another risk also uses are never
deleted: the preview lists them with a `used_by_other_risks` flag, and you decide
by hand. If real Managed Services risks already exist on the Managed Services
team, its grants therefore stay. GLOBAL grants (the management-approver grant)
cannot be attributed to one migration, so they are listed, never deleted. The
preview also counts the marker risks it will leave alone.

The Managed Services template rows go with their risks (cascade).

**The per-customer sequence counters are left alone by default.** They carry no
marker and are not deleted by cascade. A risk code is a permanent identifier
(Git issues, emails, the sheet owner's records) and the counter never moves
backward (RISK_MODULE_DESIGN.md §12); lowering it would let the next risk reuse
the code of a risk the rollback just deleted. The cost is a gap in the numbering
after a re-import. For a staging rehearsal, where no code was ever released,
`rollback.sql` has a separate **OPTIONAL** block that resets each affected
counter to the highest number still in use (0 when the customer has no risk
left). Do not use it on production, or wherever `risk_codes.csv` has been sent
to anyone.

Tested against the real schema on MySQL with the original migration's data
alongside, including a team both migrations use: the default run leaves every
counter untouched (the next BankOne number stays 5, so no deleted code is
reused) and is a safe no-op when run twice; the optional block resets them
(BankOne back to its real risk's 1, Bank Of China to 0, a customer the import
never touched unchanged). The original migration's risks, plans, history and
grants survive, including the shared-team grant, as does an admin-created grant
on the same team.

## Status

Implemented (T1–T9, T11): HTTP + SCIM clients, preflight + reference data, CSV
value mapping, identity resolution, resume-state reconstruction, the write
pipeline, the post-write verification pass, the report, and a table-driven
test suite including an end-to-end pass over `testdata/risks.csv` (a Managed
Services register, derived from the original "Risk Form Structure" tab) against
a stateful in-memory fake of the entity — including a second run that must be a clean no-op, and a verification
pass over that same clean run that must report zero mismatches. `go test ./...`
is green (the gap is `main`/`run`/`loadConfig` CLI bootstrap).

Not yet done: `.choreo/component.yaml` confirmation as a real Manual Task in the
Choreo console (T10) and the staging/production dry-run → real-run passes (T12).
