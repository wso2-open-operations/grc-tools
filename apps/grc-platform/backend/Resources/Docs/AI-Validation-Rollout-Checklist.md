# AI Evidence Validation — Phase 5 Rollout Checklist

| | |
|---|---|
| **Status** | Ready to execute — code Phases 0–4 complete (2026-07-12) |
| **Scope** | Deploy the two new Choreo components, wire secrets, enable per environment |
| **Design** | `docs/AI-Validation-Agent-MCP-Design.md` (§3.1 console values, §9 env tables) |
| **Applies to** | `ai-validation-agent`, `ai-validation-mcp`, `grc-platform-backend`, `compliance-entity`, MySQL |

Do these **in order**. Nothing here changes evidence behaviour until step 6 flips `AI_VALIDATION_ENABLED=true`, so it is safe to deploy fully-disabled first and enable per environment.

---

## 0. Prerequisites (must be done first)

- [ ] **DB migration** — run both `ALTER`s against each environment's DB (dev → staging → prod). They are additive and safe on existing rows:
  ```sql
  ALTER TABLE audit_ai_validation_log
    MODIFY result ENUM('PASS','FAIL','UNCERTAIN','PENDING','ERROR') NOT NULL;
  ALTER TABLE audit_ai_validation_log
    ADD COLUMN feedback TEXT NULL AFTER gaps_found;
  ```
- [ ] **Redeploy the Compliance Entity** so the new `GET /evidence-files/{fileId}/content` route + PENDING/ERROR enum acceptance are live. Verify:
  ```
  curl -i https://<entity-internal-url>/evidence-files/<knownFileId>/content
  # expect 200 + Content-Type + X-File-Name  (404 = still the old binary)
  ```

---

## 1. Generate the shared secrets (once per environment)

Three random secrets. Generate each with:
```bash
openssl rand -hex 32
```

| Secret you generate | Used as… | Must be identical in… |
|---|---|---|
| **S1** | `MCP_SHARED_SECRET` | `ai-validation-agent` **and** `ai-validation-mcp` |
| **S2** | `AGENT_API_KEY` (agent) = `AI_AGENT_API_KEY` (backend) | `ai-validation-agent` **and** `grc-platform-backend` |
| **S3** | `ANTHROPIC_API_KEY` | `ai-validation-agent` only — from the Anthropic console (enterprise tier + DPA, open question O-1) |

> Keep a per-env record (dev/staging/prod each get their own S1/S2). Never reuse across environments.

---

## 2. Create the MCP Server component (`ai-validation-mcp`)

Console → **Create Component → Service**, values from design §3.1:

| Field | Value |
|---|---|
| Type | Service |
| Component directory | `apps/grc-platform/ai-validation/cmd/mcpserver` |
| Buildpack | Dockerfile |
| Dockerfile path | `apps/grc-platform/ai-validation/docker/mcpserver.Dockerfile` |
| Docker context | `apps/grc-platform/ai-validation` |
| Endpoint visibility | **Project** (internal — no public endpoint) |

**Configs & Secrets** (per environment):

| Variable | Kind | Value |
|---|---|---|
| `MCP_SHARED_SECRET` | **Secret** | **S1** |
| `COMPLIANCE_ENTITY_BASE_URL` | Config | internal URL of the `compliance-entity` component (Endpoints tab) |
| `SESSION_TTL_SECONDS` | Config | `600` |
| `MAX_FILE_BYTES_TO_LLM` | Config | `10485760` |
| `LOG_LEVEL` | Config | `info` |

- Leave `PORT` unset (defaults to `:8091`, matches `component.yaml`).
- [ ] Deploy. Health check: `GET /healthz` → `200 ok`.
- [ ] **Copy this component's internal URL** — you need it for the agent's `MCP_BASE_URL`.

---

## 3. Create the Validation Agent component (`ai-validation-agent`)

Console → **Create Component → Service**:

| Field | Value |
|---|---|
| Type | Service |
| Component directory | `apps/grc-platform/ai-validation/cmd/agent` |
| Buildpack | Dockerfile |
| Dockerfile path | `apps/grc-platform/ai-validation/docker/agent.Dockerfile` |
| Docker context | `apps/grc-platform/ai-validation` |
| Endpoint visibility | **Project** |

**Configs & Secrets** (per environment):

| Variable | Kind | Value |
|---|---|---|
| `ANTHROPIC_API_KEY` | **Secret** | **S3** |
| `AGENT_API_KEY` | **Secret** | **S2** |
| `MCP_SHARED_SECRET` | **Secret** | **S1** (same as MCP) |
| `MCP_BASE_URL` | Config | internal URL of `ai-validation-mcp` (from step 2) |
| `ANTHROPIC_MODEL` | Config | `claude-sonnet-4-6` |
| `VALIDATION_TIMEOUT_SECONDS` | Config | `300` |
| `MAX_LOOP_ITERATIONS` | Config | `12` |
| `MAX_FILES_PER_JOB` | Config | `12` |
| `LOG_LEVEL` | Config | `info` |

- Leave `PORT` unset (defaults to `:8090`).
- [ ] **Egress:** allow outbound `api.anthropic.com:443` for this component only. (MCP needs intra-project egress to the entity only; the agent additionally needs Anthropic.)
- [ ] Deploy. Health check: `GET /healthz` → `200 ok`.
- [ ] **Copy this component's internal URL** — you need it for the backend's `AI_AGENT_BASE_URL`.

---

## 4. Wire the GRC Backend

Add to the `grc-platform-backend` Configs & Secrets (per environment). Keep `AI_VALIDATION_ENABLED=false` for now:

| Variable | Kind | Value |
|---|---|---|
| `AI_AGENT_BASE_URL` | Config | internal URL of `ai-validation-agent` (from step 3) |
| `AI_AGENT_API_KEY` | **Secret** | **S2** (same as the agent's `AGENT_API_KEY`) |
| `AI_VALIDATION_ENABLED` | Config | `false` ← flip to `true` in step 6 |

- [ ] Redeploy the backend (still a no-op path while disabled).

---

## 5. Pre-enable smoke test (agent + MCP wiring, no submission needed)

From inside the project network (or a debug shell in a component):

- [ ] Agent rejects unauthenticated triggers:
  ```
  curl -s -o /dev/null -w "%{http_code}\n" -XPOST https://<agent-internal-url>/api/v1/validations -d '{}'
  # expect 401
  ```
- [ ] Agent accepts an authenticated trigger and returns a jobId (this WILL create a PENDING then a real result if scope ids exist, so use a throwaway/known control):
  ```
  curl -s -XPOST -H "Authorization: Bearer <S2>" https://<agent-internal-url>/api/v1/validations \
    -d '{"task":"validate_evidence","scope":{"auditId":<A>,"controlId":<C>,"evidenceId":<E>},"requestedBy":"rollout@wso2.com"}'
  # expect 202 {"jobId":"vj_..."}
  ```
- [ ] Check the agent logs for `llm turn` with a non-zero `cacheReadTokens` on the **second** job (confirms prompt caching), and no `session bootstrap`/`mcp connect` errors.

---

## 6. Enable and verify end-to-end (per environment: dev → staging → prod)

- [ ] Set `AI_VALIDATION_ENABLED=true` on the backend and redeploy.
- [ ] Submit real evidence for a control in the UI. Expected sequence:
  1. Backend logs the fire-and-forget trigger (no error).
  2. UI evidence card shows **"Analyzing evidence…"** (PENDING) within a few seconds.
  3. Within ~1–2 min it resolves to **PASS / Gaps Found / Needs Human Review**, or **"AI validation unavailable"** on ERROR.
- [ ] Confirm the advisory caption is present and that Approve/Reject still work regardless of the verdict (results must never gate the workflow).
- [ ] Reviewer account: the compact "AI Pre-Review" strip appears above Approve/Reject (feedback items hidden).

---

## 7. Post-rollout follow-ups

- [ ] **Threat model [04]** — update the wording: file access is **fully proxied** through the entity (the agent/MCP never receive an Azure credential or SAS URL) — strictly stronger than the reviewed per-blob-SAS design (design §8, R-3).
- [ ] **Cost baseline (R-4)** — after the first week, pull token usage per validation from the agent logs (`inputTokens` / `outputTokens` / `cacheReadTokens`); tune `ANTHROPIC_MODEL` or `MAX_LOOP_ITERATIONS` if needed.
- [ ] **Open questions** — resolve O-1 (Anthropic enterprise tier + DPA), O-2 (PENDING staleness threshold), O-4 (should ERROR rows alert anyone).

---

## Quick reference — which secret goes where

```
        ┌──────────────────────┐         ┌───────────────────────┐
S2 ───► │ grc-platform-backend │ ──S2──► │  ai-validation-agent  │
        │  AI_AGENT_API_KEY    │  (auth) │  AGENT_API_KEY (=S2)  │
        │  AI_AGENT_BASE_URL ──┼────────►│  ANTHROPIC_API_KEY=S3 │
        └──────────────────────┘         │  MCP_SHARED_SECRET=S1 │
                                          │  MCP_BASE_URL ───────┐│
                                          └──────────────────────┼┘
                                                          S1 (auth)│
                                          ┌───────────────────────▼┐
                                          │   ai-validation-mcp    │
                                          │  MCP_SHARED_SECRET=S1  │
                                          │  COMPLIANCE_ENTITY_... ─┼──► compliance-entity
                                          └────────────────────────┘
```
- **S1** (`MCP_SHARED_SECRET`) = agent ↔ MCP bootstrap/lifecycle auth.
- **S2** = backend → agent trigger auth (`AI_AGENT_API_KEY` on the backend == `AGENT_API_KEY` on the agent).
- **S3** (`ANTHROPIC_API_KEY`) lives **only** in the agent.
- Both new components are **Project-internal** — never expose a public endpoint.
