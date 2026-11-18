USE grc_platform;

-- Framework archive status: audit_framework.status gains ARCHIVED.
--   ACTIVE   - in use, offered when creating an audit.
--   ARCHIVED - hidden from new audits, existing audits are kept; reversible.
--   INACTIVE - deleted (soft delete); only a framework with no audits can be deleted.
-- The new value is appended to the end of the ENUM, which MySQL applies in place
-- without rebuilding the table or locking it. Existing rows are not changed.
-- Deploy order: run this first, then the Compliance Entity, then the backend.
-- Do not remove ARCHIVED from the ENUM once any row holds it.
ALTER TABLE audit_framework
  MODIFY COLUMN status ENUM('ACTIVE','INACTIVE','ARCHIVED') NOT NULL DEFAULT 'ACTIVE',
  ALGORITHM=INPLACE, LOCK=NONE;
