-- Updated 23/11/2026 (v1.0.9)
--
-- Adds the RISK_BULK_UPLOAD privilege (bulk risk import) and grants it to
-- grc-platform-risk-compliance-admin. Run after v1.0.8. Data only; no schema
-- change. The backend caches role privileges for up to 15 minutes.
--
-- The backend requires this privilege held GLOBAL, so grant the role GLOBAL to
-- whoever should import.
--
-- Already in shared_seed_data.sql; this is the standalone copy for an existing
-- database. Re-running is a safe no-op.
--
-- Rollback:
--   UPDATE role_privilege rp
--   JOIN   `role` r    ON r.id = rp.role_id
--   JOIN   privilege p ON p.id = rp.privilege_id
--   SET    rp.is_active = FALSE
--   WHERE  r.role_name = 'grc-platform-risk-compliance-admin'
--     AND  p.privilege_name = 'RISK_BULK_UPLOAD';

USE grc_platform;

INSERT INTO privilege (privilege_name, module, status) VALUES
  ('RISK_BULK_UPLOAD', 'RISK', 'ACTIVE')
ON DUPLICATE KEY UPDATE
  module = VALUES(module),
  status = VALUES(status);

INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name = 'RISK_BULK_UPLOAD' AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-risk-compliance-admin'
ON DUPLICATE KEY UPDATE is_active = TRUE;
