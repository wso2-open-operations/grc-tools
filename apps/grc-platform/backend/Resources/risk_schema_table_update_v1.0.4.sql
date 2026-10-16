-- Updated 16/10/2026 (v1.0.4)
--
-- Renames the risk workflow status IN_REMEDIATION to APPROVED. Run after v1.0.2.
-- risk_change_log is not touched; old history keeps the IN_REMEDIATION text.
--
-- Run in order, one step at a time:
--   STEP 1  Add APPROVED to the ENUM (before deploying the new backend)
--   DEPLOY  the new backend
--   STEP 2  Move IN_REMEDIATION risks to APPROVED (right after the deploy)
--   STEP 3  Drop IN_REMEDIATION from the ENUM (only once STEP 2 shows 0 left)
--
-- Rollback: UPDATE risk SET workflow_status = 'IN_REMEDIATION'
--   WHERE workflow_status = 'APPROVED'; then redeploy the previous backend.
--   After STEP 3, re-add IN_REMEDIATION to the ENUM first (STEP 1 with the
--   value kept).

USE grc_platform;

SELECT workflow_status, COUNT(*) AS risks
  FROM risk
 GROUP BY workflow_status
 ORDER BY workflow_status;

-- STEP 1
ALTER TABLE risk
  MODIFY COLUMN workflow_status ENUM(
      'DRAFT',
      'PENDING_RISK_OWNER_APPROVAL',
      'PENDING_MANAGEMENT_APPROVAL',
      'PENDING_COMPLIANCE_REVIEW',
      'IN_REMEDIATION',
      'PENDING_OWNER_COMPLETION_APPROVAL',
      'PENDING_MANAGEMENT_CLOSURE_APPROVAL',
      'PENDING_COMPLIANCE_CLOSURE',
      'PENDING_AMENDMENT',
      'PENDING_REVISION',
      'ESCALATED',
      'CLOSED',
      'CANCELLED',
      'APPROVED'
  ) NOT NULL DEFAULT 'PENDING_RISK_OWNER_APPROVAL';

-- DEPLOY the new backend now.

-- STEP 2
SELECT COUNT(*) AS in_remediation_before
  FROM risk
 WHERE workflow_status = 'IN_REMEDIATION';

START TRANSACTION;

UPDATE risk
   SET workflow_status = 'APPROVED'
 WHERE workflow_status = 'IN_REMEDIATION';

-- Expect 0 here, and approved_after equal to in_remediation_before.
SELECT COUNT(*) AS in_remediation_after
  FROM risk
 WHERE workflow_status = 'IN_REMEDIATION';

SELECT COUNT(*) AS approved_after
  FROM risk
 WHERE workflow_status = 'APPROVED';

COMMIT;

-- STEP 3
ALTER TABLE risk
  MODIFY COLUMN workflow_status ENUM(
      'DRAFT',
      'PENDING_RISK_OWNER_APPROVAL',
      'PENDING_MANAGEMENT_APPROVAL',
      'PENDING_COMPLIANCE_REVIEW',
      'APPROVED',
      'PENDING_OWNER_COMPLETION_APPROVAL',
      'PENDING_MANAGEMENT_CLOSURE_APPROVAL',
      'PENDING_COMPLIANCE_CLOSURE',
      'PENDING_AMENDMENT',
      'PENDING_REVISION',
      'ESCALATED',
      'CLOSED',
      'CANCELLED'
  ) NOT NULL DEFAULT 'PENDING_RISK_OWNER_APPROVAL';

SELECT workflow_status, COUNT(*) AS risks
  FROM risk
 GROUP BY workflow_status
 ORDER BY workflow_status;
