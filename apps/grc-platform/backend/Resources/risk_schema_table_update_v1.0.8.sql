-- Updated 17/11/2026 (v1.0.8)
--
-- Adds the view-only Risk Hub role grc-platform-risk-viewer: registers list, risk
-- detail, dashboard and analytics for the registers it is granted on. Run after
-- v1.0.7. Data only (one role row, three role_privilege rows); no code change is
-- needed, the Admin Console role picker reads roles from this table. The backend
-- caches role privileges for up to 15 minutes.
--
-- Deliberately without RISK_VIEW_ALL_RISKS, which would also allow the CSV export
-- and subscribe the holder to overdue-risk reminder emails.
--
-- Already in shared_seed_data.sql; this is the standalone copy for an existing
-- database. Re-running is a safe no-op.
--
-- Rollback: revoke its grants, then
--   UPDATE `role` SET status = 'INACTIVE' WHERE role_name = 'grc-platform-risk-viewer';

USE grc_platform;

INSERT INTO `role` (role_name, description, module, scope_basis, assignable_user_type, status) VALUES
  ('grc-platform-risk-viewer',
   'View-only access to selected Risk Hub registers: registers list, risk detail, dashboard and analytics for the registers it is granted on. No create/edit/approve actions, no CSV export. Grant one register at a time; a GLOBAL grant shows every register.',
   'RISK', 'SOURCE_REGISTER', 'INTERNAL', 'ACTIVE')
ON DUPLICATE KEY UPDATE
  description          = VALUES(description),
  module               = VALUES(module),
  scope_basis          = VALUES(scope_basis),
  assignable_user_type = VALUES(assignable_user_type),
  status               = VALUES(status);

INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'RISK_VIEW_RISKS', 'RISK_VIEW_DASHBOARD', 'RISK_VIEW_ANALYTICS'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-risk-viewer'
ON DUPLICATE KEY UPDATE is_active = TRUE;
