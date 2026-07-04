#!/usr/bin/env bash
# Full smoke test for the compliance entity service.
# Covers all routes: health + full CRUD for every resource type.
# Requires: curl, jq
# Usage:  ./test.sh [host]   (default host = http://localhost:8080)

BASE="${1:-http://localhost:8080}"
PASS=0
FAIL=0

# Unique suffix so every test run inserts non-conflicting rows
TS=$(date +%s)

# ── Helpers ────────────────────────────────────────────────────────────────────

green() { printf "\033[32m✓ %s\033[0m\n" "$*"; }
red()   { printf "\033[31m✗ %s\033[0m\n" "$*"; }

# check label method url [body] want_status
# Always writes the raw response to /tmp/ce_resp.json so callers can jq it.
check() {
  local label="$1" method="$2" url="$3" body="$4" want="$5"
  if [ -n "$body" ]; then
    got=$(curl -s -o /tmp/ce_resp.json -w "%{http_code}" \
      -X "$method" "$url" -H "Content-Type: application/json" -d "$body")
  else
    got=$(curl -s -o /tmp/ce_resp.json -w "%{http_code}" -X "$method" "$url")
  fi
  if [ "$got" = "$want" ]; then
    green "$label → HTTP $got"
    PASS=$((PASS+1))
  else
    red "$label → got HTTP $got, want $want"
    cat /tmp/ce_resp.json; echo
    FAIL=$((FAIL+1))
  fi
}

# id_from_last_response extracts .id from the last check response.
id() { jq -r '.id // empty' /tmp/ce_resp.json; }

# ── Preflight ──────────────────────────────────────────────────────────────────
if ! command -v jq &>/dev/null; then
  echo "ERROR: jq is required.  brew install jq"
  exit 1
fi

echo "===================================================="
echo " Compliance Entity — full smoke test against $BASE"
echo " Run stamp: $TS"
echo "===================================================="
echo

# ── Health ────────────────────────────────────────────────────────────────────
echo "── Health ──"
check "GET /health" GET "$BASE/health" "" 200
echo

# ══════════════════════════════════════════════════════════════════════════════
#  AUDIT MODULE
# ══════════════════════════════════════════════════════════════════════════════

# ── Audit Teams ───────────────────────────────────────────────────────────────
echo "── Audit Teams ──"
check "POST /audit/teams/search" POST "$BASE/audit/teams/search" \
  '{"pagination":{"limit":10,"offset":0}}' 200

check "POST /audit/teams (create)" POST "$BASE/audit/teams" \
  "{\"name\":\"Test Compliance Team $TS\",\"status\":\"ACTIVE\",\"createdBy\":\"test@example.com\"}" 201
AUDIT_TEAM_ID=$(id)
echo "  → auditTeamId=$AUDIT_TEAM_ID"

check "GET /audit/teams/$AUDIT_TEAM_ID" \
  GET "$BASE/audit/teams/$AUDIT_TEAM_ID" "" 200
check "GET /audit/teams/999999 (404)" \
  GET "$BASE/audit/teams/999999" "" 404
check "PATCH /audit/teams/$AUDIT_TEAM_ID" PATCH "$BASE/audit/teams/$AUDIT_TEAM_ID" \
  '{"name":"Updated Compliance Team","updatedBy":"test@example.com"}' 200
echo

# ── Audit Frameworks ──────────────────────────────────────────────────────────
# audit_framework.name has a UNIQUE constraint → use $TS suffix
echo "── Audit Frameworks ──"
check "POST /audit/frameworks/search" POST "$BASE/audit/frameworks/search" \
  '{"statusKey":"ACTIVE","pagination":{"limit":10,"offset":0}}' 200

check "POST /audit/frameworks (create)" POST "$BASE/audit/frameworks" \
  "{\"name\":\"ISO 27001 $TS\",\"version\":\"2022\",\"status\":\"ACTIVE\",\"createdBy\":\"test@example.com\"}" 201
FRAMEWORK_ID=$(id)
echo "  → frameworkId=$FRAMEWORK_ID"

check "GET /audit/frameworks/$FRAMEWORK_ID" \
  GET "$BASE/audit/frameworks/$FRAMEWORK_ID" "" 200
check "GET /audit/frameworks/999999 (404)" \
  GET "$BASE/audit/frameworks/999999" "" 404
check "PATCH /audit/frameworks/$FRAMEWORK_ID" PATCH "$BASE/audit/frameworks/$FRAMEWORK_ID" \
  '{"version":"2023","updatedBy":"test@example.com"}' 200
echo

# ── Audit Products ────────────────────────────────────────────────────────────
echo "── Audit Products ──"
check "POST /audit/products/search" POST "$BASE/audit/products/search" \
  '{"pagination":{"limit":10,"offset":0}}' 200

check "POST /audit/products (create)" POST "$BASE/audit/products" \
  "{\"name\":\"Core Banking Platform $TS\",\"status\":\"ACTIVE\",\"createdBy\":\"test@example.com\"}" 201
PRODUCT_ID=$(id)
echo "  → productId=$PRODUCT_ID"

check "GET /audit/products/$PRODUCT_ID" \
  GET "$BASE/audit/products/$PRODUCT_ID" "" 200
check "GET /audit/products/999999 (404)" \
  GET "$BASE/audit/products/999999" "" 404
check "PATCH /audit/products/$PRODUCT_ID" PATCH "$BASE/audit/products/$PRODUCT_ID" \
  "{\"name\":\"Core Banking Platform v2 $TS\",\"updatedBy\":\"test@example.com\"}" 200
echo

# ── Users ─────────────────────────────────────────────────────────────────────
# user.email has a UNIQUE constraint → use $TS suffix
echo "── Users ──"
check "POST /users/search (all)" POST "$BASE/users/search" \
  '{"pagination":{"limit":5,"offset":0}}' 200
check "POST /users/search (ACTIVE)" POST "$BASE/users/search" \
  '{"statusKey":"ACTIVE","pagination":{"limit":5,"offset":0}}' 200
check "POST /users/search (invalid statusKey → 400)" POST "$BASE/users/search" \
  '{"statusKey":"BOGUS","pagination":{"limit":5,"offset":0}}' 400

check "POST /users (create)" POST "$BASE/users" \
  "{\"email\":\"smoketest$TS@example.com\",\"displayName\":\"Smoke Tester\",\"auditTeamId\":$AUDIT_TEAM_ID,\"status\":\"ACTIVE\",\"createdBy\":\"test@example.com\"}" 201
USER_ID=$(id)
echo "  → userId=$USER_ID"

check "GET /users/$USER_ID" GET "$BASE/users/$USER_ID" "" 200
check "GET /users/by-email/smoketest$TS@example.com" GET "$BASE/users/by-email/smoketest$TS@example.com" "" 200
check "GET /users/by-email/nobody@notfound.com (404)" GET "$BASE/users/by-email/nobody@notfound.com" "" 404
check "GET /users/999999 (404)" GET "$BASE/users/999999" "" 404
check "GET /users/abc (bad id → 400)" GET "$BASE/users/abc" "" 400
check "PATCH /users/$USER_ID" PATCH "$BASE/users/$USER_ID" \
  '{"displayName":"Smoke Tester Updated","updatedBy":"test@example.com"}' 200
echo

# ── Audits ────────────────────────────────────────────────────────────────────
echo "── Audits ──"
check "POST /audits/search" POST "$BASE/audits/search" \
  '{"pagination":{"limit":5,"offset":0}}' 200
check "POST /audits/search (statusKeys filter)" POST "$BASE/audits/search" \
  '{"statusKeys":["ACTIVE","COMPLETED"],"pagination":{"limit":5,"offset":0}}' 200

check "POST /audits (create)" POST "$BASE/audits" \
  "{\"name\":\"Q2 2026 ISO Audit\",\"frameworkId\":$FRAMEWORK_ID,\"productId\":$PRODUCT_ID,\"periodStart\":\"2026-04-01\",\"periodEnd\":\"2026-06-30\",\"createdBy\":\"test@example.com\"}" 201
AUDIT_ID=$(id)
echo "  → auditId=$AUDIT_ID"

check "GET /audits/$AUDIT_ID" GET "$BASE/audits/$AUDIT_ID" "" 200
check "GET /audits/999999 (404)" GET "$BASE/audits/999999" "" 404
check "PATCH /audits/$AUDIT_ID" PATCH "$BASE/audits/$AUDIT_ID" \
  '{"name":"Q2 2026 ISO Audit (Updated)","updatedBy":"test@example.com"}' 200

# Create a throwaway audit to delete so we don't break downstream tests
check "POST /audits (create for delete test)" POST "$BASE/audits" \
  "{\"name\":\"Delete Me $TS\",\"frameworkId\":$FRAMEWORK_ID,\"productId\":$PRODUCT_ID,\"periodStart\":\"2026-01-01\",\"periodEnd\":\"2026-12-31\",\"createdBy\":\"test@example.com\"}" 201
DELETE_AUDIT_ID=$(id)
check "DELETE /audits/$DELETE_AUDIT_ID" DELETE "$BASE/audits/$DELETE_AUDIT_ID?deletedBy=test@example.com" "" 204
check "DELETE /audits/999999 (404)" DELETE "$BASE/audits/999999?deletedBy=test@example.com" "" 404
echo

# ── Controls ──────────────────────────────────────────────────────────────────
echo "── Controls ──"
check "POST /audits/$AUDIT_ID/controls/search" POST "$BASE/audits/$AUDIT_ID/controls/search" \
  '{"pagination":{"limit":10,"offset":0}}' 200
check "POST /audits/$AUDIT_ID/controls/search (DESIGN only)" POST "$BASE/audits/$AUDIT_ID/controls/search" \
  '{"requirementTypes":["DESIGN"],"pagination":{"limit":10,"offset":0}}' 200
check "POST /audits/$AUDIT_ID/controls/search (bad type → 400)" POST "$BASE/audits/$AUDIT_ID/controls/search" \
  '{"requirementTypes":["BOGUS"],"pagination":{"limit":10,"offset":0}}' 400
check "POST /audits/$AUDIT_ID/controls/search (auditorIds filter)" POST "$BASE/audits/$AUDIT_ID/controls/search" \
  "{\"auditorIds\":[$USER_ID],\"pagination\":{\"limit\":10,\"offset\":0}}" 200
check "POST /audits/$AUDIT_ID/controls/search (ownerIds filter)" POST "$BASE/audits/$AUDIT_ID/controls/search" \
  "{\"ownerIds\":[$USER_ID],\"pagination\":{\"limit\":10,\"offset\":0}}" 200
check "POST /controls/search (cross-audit by auditorId)" POST "$BASE/controls/search" \
  "{\"auditorIds\":[$USER_ID],\"pagination\":{\"limit\":10,\"offset\":0}}" 200
check "POST /controls/search (cross-audit all)" POST "$BASE/controls/search" \
  '{"pagination":{"limit":10,"offset":0}}' 200

check "POST /audits/$AUDIT_ID/controls (DESIGN)" POST "$BASE/audits/$AUDIT_ID/controls" \
  "{\"controlNumber\":\"A.9.1\",\"description\":\"Access control policy must be maintained\",\"requirementType\":\"DESIGN\",\"controlType\":\"CONFIG\",\"scope\":\"COMMON\",\"auditorId\":$USER_ID,\"createdBy\":\"test@example.com\"}" 201
CONTROL_ID=$(id)
echo "  → controlId (DESIGN)=$CONTROL_ID"

check "POST /audits/$AUDIT_ID/controls (OE)" POST "$BASE/audits/$AUDIT_ID/controls" \
  "{\"controlNumber\":\"A.9.2\",\"description\":\"User access provisioning reviewed quarterly\",\"requirementType\":\"OE\",\"controlType\":\"NON_CONFIG\",\"scope\":\"COMMON\",\"auditorId\":$USER_ID,\"createdBy\":\"test@example.com\"}" 201
OE_CONTROL_ID=$(id)
echo "  → controlId (OE)=$OE_CONTROL_ID"

check "GET /audits/$AUDIT_ID/controls/$CONTROL_ID" \
  GET "$BASE/audits/$AUDIT_ID/controls/$CONTROL_ID" "" 200
check "GET /audits/$AUDIT_ID/controls/999999 (404)" \
  GET "$BASE/audits/$AUDIT_ID/controls/999999" "" 404
check "PATCH /audits/$AUDIT_ID/controls/$CONTROL_ID" PATCH "$BASE/audits/$AUDIT_ID/controls/$CONTROL_ID" \
  "{\"teamId\":$AUDIT_TEAM_ID,\"ownerId\":$USER_ID,\"updatedBy\":\"test@example.com\"}" 200

# Bulk add two controls
check "POST /audits/$AUDIT_ID/controls/bulk" POST "$BASE/audits/$AUDIT_ID/controls/bulk" \
  "{\"controls\":[{\"controlNumber\":\"A.9.3\",\"description\":\"Password policy\",\"requirementType\":\"DESIGN\",\"controlType\":\"CONFIG\",\"scope\":\"COMMON\",\"createdBy\":\"test@example.com\"},{\"controlNumber\":\"A.9.4\",\"description\":\"Access review log\",\"requirementType\":\"OE\",\"controlType\":\"NON_CONFIG\",\"scope\":\"COMMON\",\"createdBy\":\"test@example.com\"}]}" 201

# Create a throwaway control to delete so we don't destroy OE_CONTROL_ID (needed for population tests)
check "POST /audits/$AUDIT_ID/controls (create for delete test)" POST "$BASE/audits/$AUDIT_ID/controls" \
  "{\"controlNumber\":\"DEL.$TS\",\"description\":\"To be deleted\",\"requirementType\":\"DESIGN\",\"controlType\":\"CONFIG\",\"scope\":\"COMMON\",\"createdBy\":\"test@example.com\"}" 201
DELETE_CONTROL_ID=$(id)
check "DELETE /audits/$AUDIT_ID/controls/$DELETE_CONTROL_ID" \
  DELETE "$BASE/audits/$AUDIT_ID/controls/$DELETE_CONTROL_ID" "" 204
check "DELETE /audits/$AUDIT_ID/controls/999999 (404)" \
  DELETE "$BASE/audits/$AUDIT_ID/controls/999999" "" 404
echo

# ── Evidence (audit_evidence + audit_evidence_file) ───────────────────────────
echo "── Evidence (audit_evidence + audit_evidence_file) ──"
check "POST /audits/$AUDIT_ID/controls/$CONTROL_ID/evidence (create)" POST \
  "$BASE/audits/$AUDIT_ID/controls/$CONTROL_ID/evidence" \
  "{\"submittedBy\":$USER_ID,\"createdBy\":\"test@example.com\"}" 201
EVIDENCE_ID=$(id)
echo "  → evidenceId=$EVIDENCE_ID"

check "GET /audits/$AUDIT_ID/controls/$CONTROL_ID/evidence (list)" \
  GET "$BASE/audits/$AUDIT_ID/controls/$CONTROL_ID/evidence" "" 200
check "GET /evidence/$EVIDENCE_ID" GET "$BASE/evidence/$EVIDENCE_ID" "" 200
check "GET /evidence/999999 (404)" GET "$BASE/evidence/999999" "" 404
check "PATCH /evidence/$EVIDENCE_ID (compliance approve)" PATCH "$BASE/evidence/$EVIDENCE_ID" \
  '{"status":"COMPLIANCE_APPROVED","updatedBy":"test@example.com"}' 200

check "POST /evidence/$EVIDENCE_ID/files (add file)" POST "$BASE/evidence/$EVIDENCE_ID/files" \
  '{"fileName":"policy_doc.pdf","filePath":"https://storage.blob.core.windows.net/evidence/policy_doc.pdf","fileType":"application/pdf","fileSize":204800,"createdBy":"test@example.com"}' 201
EV_FILE_ID=$(id)
echo "  → evidenceFileId=$EV_FILE_ID"

check "GET /evidence/$EVIDENCE_ID/files" GET "$BASE/evidence/$EVIDENCE_ID/files" "" 200
check "DELETE /evidence/files/$EV_FILE_ID" DELETE "$BASE/evidence/files/$EV_FILE_ID" "" 204
check "DELETE /evidence/files/999999 (404)" DELETE "$BASE/evidence/files/999999" "" 404
echo

# ── Populations (audit_population + population files in audit_evidence_file) ──
echo "── Populations (audit_population + population files) ──"
check "POST /audits/$AUDIT_ID/controls/$OE_CONTROL_ID/populations (create)" POST \
  "$BASE/audits/$AUDIT_ID/controls/$OE_CONTROL_ID/populations" \
  "{\"description\":\"All user access records Q2 2026\",\"ownerId\":$USER_ID,\"createdBy\":\"test@example.com\"}" 201
POP_ID=$(id)
echo "  → populationId=$POP_ID"

check "GET /populations/$POP_ID" GET "$BASE/populations/$POP_ID" "" 200
check "GET /populations/999999 (404)" GET "$BASE/populations/999999" "" 404
check "GET /audits/$AUDIT_ID/controls/$OE_CONTROL_ID/populations (list)" \
  GET "$BASE/audits/$AUDIT_ID/controls/$OE_CONTROL_ID/populations" "" 200
check "PATCH /populations/$POP_ID" PATCH "$BASE/populations/$POP_ID" \
  '{"description":"All user access records Q2 2026 (updated)","updatedBy":"test@example.com"}' 200

check "POST /populations/$POP_ID/files (POPULATION file)" POST "$BASE/populations/$POP_ID/files" \
  '{"fileKind":"POPULATION","fileName":"user_list.xlsx","filePath":"https://storage.blob.core.windows.net/populations/user_list.xlsx","fileType":"application/vnd.ms-excel","createdBy":"test@example.com"}' 201
POP_FILE_ID=$(id)
echo "  → populationFileId=$POP_FILE_ID"

check "GET /populations/$POP_ID/files" GET "$BASE/populations/$POP_ID/files" "" 200
check "DELETE /populations/files/$POP_FILE_ID" DELETE "$BASE/populations/files/$POP_FILE_ID" "" 204
check "DELETE /populations/files/999999 (404)" DELETE "$BASE/populations/files/999999" "" 404
echo

# ══════════════════════════════════════════════════════════════════════════════
#  RISK MODULE
# ══════════════════════════════════════════════════════════════════════════════

# ── Risk Teams ────────────────────────────────────────────────────────────────
# risk_team.code has a UNIQUE constraint → use $TS suffix (truncated to fit VARCHAR(50))
echo "── Risk Teams ──"
check "POST /risk/teams/search" POST "$BASE/risk/teams/search" \
  '{"pagination":{"limit":10,"offset":0}}' 200
check "POST /risk/teams/search (SOURCE_REGISTER)" POST "$BASE/risk/teams/search" \
  '{"teamTypeKeys":["SOURCE_REGISTER"],"pagination":{"limit":10,"offset":0}}' 200

TEAM_CODE="S${TS: -6}"   # e.g. S123456 — unique per run, fits in VARCHAR(50)
check "POST /risk/teams (create)" POST "$BASE/risk/teams" \
  "{\"name\":\"Security Operations $TS\",\"code\":\"$TEAM_CODE\",\"teamType\":\"BOTH\",\"status\":\"ACTIVE\",\"createdBy\":\"test@example.com\"}" 201
RISK_TEAM_ID=$(id)
echo "  → riskTeamId=$RISK_TEAM_ID  code=$TEAM_CODE"

check "GET /risk/teams/$RISK_TEAM_ID" GET "$BASE/risk/teams/$RISK_TEAM_ID" "" 200
check "GET /risk/teams/999999 (404)" GET "$BASE/risk/teams/999999" "" 404
check "PATCH /risk/teams/$RISK_TEAM_ID" PATCH "$BASE/risk/teams/$RISK_TEAM_ID" \
  '{"description":"Security operations and incident response","updatedBy":"test@example.com"}' 200
echo

# ── Risk Scores (read-only reference data) ────────────────────────────────────
echo "── Risk Scores ──"
check "GET /risk/scores" GET "$BASE/risk/scores" "" 200
echo

# ── Risk Compliance References ────────────────────────────────────────────────
echo "── Risk Compliance References ──"
check "POST /risk/compliance-references/search" POST "$BASE/risk/compliance-references/search" \
  '{"pagination":{"limit":10,"offset":0}}' 200

check "POST /risk/compliance-references (create)" POST "$BASE/risk/compliance-references" \
  "{\"name\":\"ISO 27001:2022 $TS\",\"description\":\"Information security management standard\",\"createdBy\":\"test@example.com\"}" 201
REF_ID=$(id)
echo "  → referenceId=$REF_ID"

check "GET /risk/compliance-references/$REF_ID" \
  GET "$BASE/risk/compliance-references/$REF_ID" "" 200
check "GET /risk/compliance-references/999999 (404)" \
  GET "$BASE/risk/compliance-references/999999" "" 404
check "PATCH /risk/compliance-references/$REF_ID" PATCH "$BASE/risk/compliance-references/$REF_ID" \
  '{"description":"Updated: Information security management standard 2022","updatedBy":"test@example.com"}' 200
echo

# ── Risks ─────────────────────────────────────────────────────────────────────
echo "── Risks ──"
check "POST /risks/search" POST "$BASE/risks/search" \
  '{"pagination":{"limit":5,"offset":0}}' 200
check "POST /risks/search (Q2 filter)" POST "$BASE/risks/search" \
  '{"riskQuarterKeys":["Q2"],"pagination":{"limit":5,"offset":0}}' 200
check "POST /risks/search (bad status → 400)" POST "$BASE/risks/search" \
  '{"workflowStatusKeys":["BOGUS"],"pagination":{"limit":5,"offset":0}}' 400

check "POST /risks (create)" POST "$BASE/risks" \
  "{\"riskTitle\":\"Unauthorised data access via misconfigured S3\",\"sourceRegisterId\":$RISK_TEAM_ID,\"assignmentTeamId\":$RISK_TEAM_ID,\"assignerId\":$USER_ID,\"ownerId\":$USER_ID,\"riskYear\":2026,\"riskQuarter\":\"Q2\",\"createdBy\":\"test@example.com\"}" 201
RISK_ID=$(id)
echo "  → riskId=$RISK_ID"

check "GET /risks/$RISK_ID" GET "$BASE/risks/$RISK_ID" "" 200
check "GET /risks/999999 (404)" GET "$BASE/risks/999999" "" 404
check "PATCH /risks/$RISK_ID" PATCH "$BASE/risks/$RISK_ID" \
  '{"riskDescription":"S3 bucket ACLs were found open to the internet","updatedBy":"test@example.com"}' 200
echo

# ── Risk Action Plans ─────────────────────────────────────────────────────────
echo "── Risk Action Plans ──"
check "POST /risks/$RISK_ID/action-plans (create)" POST "$BASE/risks/$RISK_ID/action-plans" \
  "{\"description\":\"Restrict S3 bucket ACLs and enable access logging\",\"actionOwnerId\":$USER_ID,\"planType\":\"STANDARD\",\"createdBy\":\"test@example.com\"}" 201
PLAN_ID=$(id)
echo "  → planId=$PLAN_ID"

check "GET /risks/$RISK_ID/action-plans (list)" GET "$BASE/risks/$RISK_ID/action-plans" "" 200
check "GET /action-plans/$PLAN_ID" GET "$BASE/action-plans/$PLAN_ID" "" 200
check "GET /action-plans/999999 (404)" GET "$BASE/action-plans/999999" "" 404
check "PATCH /action-plans/$PLAN_ID" PATCH "$BASE/action-plans/$PLAN_ID" \
  '{"status":"IN_PROGRESS","updatedBy":"test@example.com"}' 200
echo

# ── Risk Action Steps ────────────────────────────────────────────────────────
echo "── Risk Action Steps ──"
check "POST /action-plans/$PLAN_ID/steps (create)" POST "$BASE/action-plans/$PLAN_ID/steps" \
  '{"stepNo":1,"description":"Audit all S3 bucket ACLs and remove public access","createdBy":"test@example.com"}' 201
STEP_ID=$(id)
echo "  → stepId=$STEP_ID"
check "GET /action-plans/$PLAN_ID/steps (list)" GET "$BASE/action-plans/$PLAN_ID/steps" "" 200
check "GET /action-plans/$PLAN_ID/steps/$STEP_ID" GET "$BASE/action-plans/$PLAN_ID/steps/$STEP_ID" "" 200
check "GET /action-plans/$PLAN_ID/steps/999999 (404)" GET "$BASE/action-plans/$PLAN_ID/steps/999999" "" 404
check "PATCH /action-plans/$PLAN_ID/steps/$STEP_ID" PATCH "$BASE/action-plans/$PLAN_ID/steps/$STEP_ID" \
  '{"status":"IN_PROGRESS","updatedBy":"test@example.com"}' 200
check "DELETE /action-plans/$PLAN_ID/steps/$STEP_ID" \
  DELETE "$BASE/action-plans/$PLAN_ID/steps/$STEP_ID" "" 204
check "DELETE /action-plans/$PLAN_ID/steps/999999 (404)" \
  DELETE "$BASE/action-plans/$PLAN_ID/steps/999999" "" 404
echo

# ── Risk Compliance Reference Links ──────────────────────────────────────────
echo "── Risk Compliance Reference Links ──"
check "POST /risks/$RISK_ID/compliance-references (add)" POST "$BASE/risks/$RISK_ID/compliance-references" \
  "{\"referenceId\":$REF_ID,\"createdBy\":\"test@example.com\"}" 201
check "GET /risks/$RISK_ID/compliance-references (list)" GET "$BASE/risks/$RISK_ID/compliance-references" "" 200
check "DELETE /risks/$RISK_ID/compliance-references/$REF_ID" \
  DELETE "$BASE/risks/$RISK_ID/compliance-references/$REF_ID" "" 204
check "DELETE /risks/$RISK_ID/compliance-references/999999 (404)" \
  DELETE "$BASE/risks/$RISK_ID/compliance-references/999999" "" 404
echo

# ── Risk Escalations ─────────────────────────────────────────────────────────
echo "── Risk Escalations ──"
check "POST /risks/$RISK_ID/escalations (create)" POST "$BASE/risks/$RISK_ID/escalations" \
  "{\"escalatedTo\":$USER_ID,\"reason\":\"Deadline missed; risk level HIGH\",\"createdBy\":\"test@example.com\"}" 201
ESCALATION_ID=$(id)
echo "  → escalationId=$ESCALATION_ID"
check "GET /risks/$RISK_ID/escalations (list)" GET "$BASE/risks/$RISK_ID/escalations" "" 200
check "GET /risks/$RISK_ID/escalations/$ESCALATION_ID" GET "$BASE/risks/$RISK_ID/escalations/$ESCALATION_ID" "" 200
check "GET /risks/$RISK_ID/escalations/999999 (404)" GET "$BASE/risks/$RISK_ID/escalations/999999" "" 404
check "PATCH /risks/$RISK_ID/escalations/$ESCALATION_ID" PATCH "$BASE/risks/$RISK_ID/escalations/$ESCALATION_ID" \
  '{"decision":"Extend deadline by 30 days and add management action plan","status":"RESOLVED","updatedBy":"test@example.com"}' 200
echo

# ── Risk Change Log ───────────────────────────────────────────────────────────
echo "── Risk Change Log ──"
check "POST /risks/$RISK_ID/changes (create)" POST "$BASE/risks/$RISK_ID/changes" \
  '{"createdBy":"test@example.com","action":"UPDATE","fieldChanged":"workflow_status","oldValue":"\"PENDING_RISK_OWNER_APPROVAL\"","newValue":"\"IN_REMEDIATION\""}' 201
check "POST /risks/$RISK_ID/changes (bad action → 400)" POST "$BASE/risks/$RISK_ID/changes" \
  '{"createdBy":"test@example.com","action":"BOGUS"}' 400
check "GET /risks/$RISK_ID/changes" GET "$BASE/risks/$RISK_ID/changes" "" 200
echo

# ── Audit Trail ───────────────────────────────────────────────────────────────
echo "── Audit Trail ──"
check "POST /audits/$AUDIT_ID/trail (create)" POST "$BASE/audits/$AUDIT_ID/trail" \
  "{\"actorId\":$USER_ID,\"controlId\":$CONTROL_ID,\"action\":\"APPROVED\",\"createdBy\":\"test@example.com\"}" 201
check "POST /audits/$AUDIT_ID/trail (bad action → 400)" POST "$BASE/audits/$AUDIT_ID/trail" \
  "{\"actorId\":$USER_ID,\"action\":\"BOGUS\",\"createdBy\":\"test@example.com\"}" 400
check "GET /audits/$AUDIT_ID/trail" GET "$BASE/audits/$AUDIT_ID/trail?limit=10&offset=0" "" 200
echo

# ── Risk Evidence (risk_evidence) ─────────────────────────────────────────────
echo "── Risk Evidence (risk_evidence) ──"
check "POST /risks/$RISK_ID/evidence (create)" POST "$BASE/risks/$RISK_ID/evidence" \
  '{"fileName":"s3_audit_report.pdf","filePath":"https://storage.blob.core.windows.net/risk-evidence/s3_audit_report.pdf","evidenceType":"ACTION_PLAN_ATTACHMENT","createdBy":"test@example.com"}' 201
RISK_FILE_ID=$(id)
echo "  → riskEvidenceId=$RISK_FILE_ID"

check "GET /risks/$RISK_ID/evidence (list)" GET "$BASE/risks/$RISK_ID/evidence" "" 200
check "DELETE /risks/evidence/$RISK_FILE_ID" DELETE "$BASE/risks/evidence/$RISK_FILE_ID" "" 204
check "DELETE /risks/evidence/999999 (404)" DELETE "$BASE/risks/evidence/999999" "" 404
echo

# ── Risk Assessments ──────────────────────────────────────────────────────────
echo "── Risk Assessments ──"
check "POST /risks/$RISK_ID/assessments (create)" POST "$BASE/risks/$RISK_ID/assessments" \
  "{\"scoreId\":1,\"progress\":\"S3 ACLs patched; access logging enabled\",\"reassessmentDate\":\"2026-09-01\",\"assessedBy\":\"test@example.com\",\"createdBy\":\"test@example.com\"}" 201
ASSESSMENT_ID=$(id)
echo "  → assessmentId=$ASSESSMENT_ID"

check "GET /risks/$RISK_ID/assessments (list)" GET "$BASE/risks/$RISK_ID/assessments" "" 200
echo

# ── Summary ───────────────────────────────────────────────────────────────────
echo "===================================================="
printf " Results: \033[32m%d passed\033[0m  \033[31m%d failed\033[0m\n" "$PASS" "$FAIL"
echo "===================================================="
[ "$FAIL" -eq 0 ]
