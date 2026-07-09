-- =============================================================================
-- GRC Platform — Mock Data
-- Run AFTER all three module schemas (shared.sql, audit_schema.sql, risk_schema.sql).
-- Safe to re-run: all INSERTs use ON DUPLICATE KEY UPDATE.
-- =============================================================================

USE grc_platform_dev;

SET FOREIGN_KEY_CHECKS = 0;

-- =============================================================================
-- AUDIT PRIVILEGES
-- Names must match privilege_name constants in
-- backend/internal/shared/privilege/privilege.go exactly.
-- =============================================================================

INSERT INTO privilege (privilege_name, module, status) VALUES
  ('VIEW_AUDITS',             'AUDIT', 'ACTIVE'),
  ('CREATE_AUDIT',            'AUDIT', 'ACTIVE'),
  ('UPDATE_AUDIT',            'AUDIT', 'ACTIVE'),
  ('MOVE_AUDIT_TO_FIELDWORK', 'AUDIT', 'ACTIVE'),
  ('SUBMIT_AUDIT_FOR_REVIEW', 'AUDIT', 'ACTIVE'),
  ('COMPLETE_AUDIT',          'AUDIT', 'ACTIVE'),
  ('MANAGE_CONTROLS',         'AUDIT', 'ACTIVE'),
  ('SUBMIT_EVIDENCE',         'AUDIT', 'ACTIVE'),
  ('REVIEW_EVIDENCE',         'AUDIT', 'ACTIVE'),
  ('MANAGE_POPULATION',       'AUDIT', 'ACTIVE'),
  ('ADD_COMMENT',             'AUDIT', 'ACTIVE'),
  ('MANAGE_ASSIGNMENTS',      'AUDIT', 'ACTIVE'),
  ('VIEW_TRAIL',              'AUDIT', 'ACTIVE'),
  ('MANAGE_FRAMEWORKS',       'AUDIT', 'ACTIVE'),
  ('MANAGE_USERS',            'AUDIT', 'ACTIVE'),
  ('EXPORT_REPORT',           'AUDIT', 'ACTIVE')
ON DUPLICATE KEY UPDATE module = VALUES(module), status = VALUES(status);

-- =============================================================================
-- AUDIT ROLES
-- role_name must match the Asgardeo group name from the JWT groups claim exactly.
-- =============================================================================

INSERT INTO `role` (role_name, description, status) VALUES
  ('grc-platform-compliance-audit-admin',
   'Full access to all audit capabilities.',
   'ACTIVE'),
  ('grc-platform-compliance-audit-team',
   'Reviews and approves evidence; manages workflow. No evidence upload (SoD).',
   'ACTIVE'),
  ('grc-platform-internal-team',
   'Uploads evidence for assigned controls.',
   'ACTIVE'),
  ('grc-platform-external-auditor',
   'Final auditor sign-off; selects OE samples; reviews evidence.',
   'ACTIVE'),
  ('grc-platform-internal-auditor',
   'Read-only access with commenting and export rights.',
   'ACTIVE'),
  ('grc-platform-management',
   'Read-only dashboard and export access.',
   'ACTIVE'),
  ('wso2-everyone',
   'Testing catch-all — receives all audit privileges for local development.',
   'ACTIVE')
ON DUPLICATE KEY UPDATE description = VALUES(description), status = VALUES(status);

-- =============================================================================
-- AUDIT ROLE → PRIVILEGE MAPPINGS
-- =============================================================================

-- compliance-audit-admin → all audit privileges
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.module = 'AUDIT' AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-compliance-audit-admin'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- compliance-audit-team → review + workflow management; NO SUBMIT_EVIDENCE (SoD)
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'VIEW_AUDITS', 'UPDATE_AUDIT', 'MANAGE_CONTROLS',
  'REVIEW_EVIDENCE', 'ADD_COMMENT', 'VIEW_TRAIL',
  'EXPORT_REPORT', 'MOVE_AUDIT_TO_FIELDWORK'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-compliance-audit-team'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- internal-team → upload evidence only
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'VIEW_AUDITS', 'SUBMIT_EVIDENCE', 'ADD_COMMENT', 'VIEW_TRAIL'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-internal-team'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- external-auditor → final sign-off, sample selection, evidence review
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'VIEW_AUDITS', 'REVIEW_EVIDENCE', 'MANAGE_POPULATION',
  'SUBMIT_AUDIT_FOR_REVIEW', 'ADD_COMMENT', 'VIEW_TRAIL', 'EXPORT_REPORT'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-external-auditor'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- internal-auditor → read, comment, export
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'VIEW_AUDITS', 'ADD_COMMENT', 'VIEW_TRAIL', 'EXPORT_REPORT'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-internal-auditor'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- management → read-only dashboard and export
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'VIEW_AUDITS', 'VIEW_TRAIL', 'EXPORT_REPORT'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-management'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- wso2-everyone → all audit privileges (testing catch-all)
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.module = 'AUDIT' AND p.status = 'ACTIVE'
WHERE  r.role_name = 'wso2-everyone'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- =============================================================================
-- AUDIT TEAMS
-- =============================================================================

INSERT INTO audit_team (id, name, status) VALUES
  (1, 'Identity & Access Management', 'ACTIVE'),
  (2, 'Security Engineering',         'ACTIVE'),
  (3, 'Platform Engineering',         'ACTIVE'),
  (4, 'Cloud Operations',             'ACTIVE')
ON DUPLICATE KEY UPDATE name = VALUES(name), status = VALUES(status);

-- =============================================================================
-- USERS
-- =============================================================================

INSERT INTO `user` (id, email, display_name, audit_team_id, status) VALUES
  (1, 'alice.silva@wso2.com',  'Alice Silva',  1, 'ACTIVE'),  -- compliance team lead
  (2, 'bob.mendez@wso2.com',   'Bob Mendez',   2, 'ACTIVE'),  -- security engineer
  (3, 'charlie.ng@wso2.com',   'Charlie Ng',   1, 'ACTIVE'),  -- process owner (IAM)
  (4, 'diana.patel@wso2.com',  'Diana Patel',  3, 'ACTIVE'),  -- process owner (Platform)
  (5, 'eve.johnson@wso2.com',  'Eve Johnson',  2, 'ACTIVE'),  -- security engineer
  (6, 'frank.lee@wso2.com',    'Frank Lee',    4, 'ACTIVE'),  -- cloud ops engineer
  (7, 'grace.kim@wso2.com',    'Grace Kim',    3, 'ACTIVE'),  -- compliance admin
  (8, 'henry.white@wso2.com',  'Henry White',  NULL, 'ACTIVE')-- external auditor
ON DUPLICATE KEY UPDATE display_name = VALUES(display_name), audit_team_id = VALUES(audit_team_id), status = VALUES(status);

-- =============================================================================
-- FRAMEWORKS
-- =============================================================================

INSERT INTO audit_framework (id, name, status) VALUES
  (1, 'SOC 2',     'ACTIVE'),
  (2, 'HIPAA',     'ACTIVE'),
  (3, 'ISO 27001', 'ACTIVE')
ON DUPLICATE KEY UPDATE name = VALUES(name), status = VALUES(status);

-- =============================================================================
-- PRODUCTS
-- =============================================================================

INSERT INTO audit_product (id, name, status) VALUES
  (1, 'Asgardeo',  'ACTIVE'),
  (2, 'Choreo',    'ACTIVE'),
  (3, 'Ballerina', 'ACTIVE')
ON DUPLICATE KEY UPDATE name = VALUES(name), status = VALUES(status);

-- =============================================================================
-- FRAMEWORK CONTROL LIBRARY
-- =============================================================================

-- ── SOC 2 Controls ────────────────────────────────────────────────────────────

INSERT INTO audit_framework_control
  (id, framework_id, control_number, description, evidence_requirement, requirement_type, control_type, scope, version, is_current)
VALUES
  -- CC1 — Control Environment
  (1, 1, 'CC1.1',
   'The entity demonstrates a commitment to integrity and ethical values.',
   'Code of conduct sign-off records, ethics training completion report for the period.',
   'DESIGN', 'NON_CONFIG', 'COMMON', 1, TRUE),

  (2, 1, 'CC1.2',
   'The board of directors demonstrates independence from management and exercises oversight of internal control.',
   'Board meeting minutes, governance charter, audit-committee composition records.',
   'DESIGN', 'NON_CONFIG', 'COMMON', 1, TRUE),

  (3, 1, 'CC1.3',
   'Management establishes structures, reporting lines, and appropriate authorities and responsibilities in pursuit of objectives.',
   'Organisation chart, RACI matrix, job descriptions for key security roles.',
   'DESIGN', 'NON_CONFIG', 'COMMON', 1, TRUE),

  -- CC6 — Logical and Physical Access Controls
  (4, 1, 'CC6.1',
   'The entity implements logical access security software, infrastructure, and architectures over protected information assets to protect them from security events.',
   'Access control policy, IAM configuration screenshots, MFA enrollment report.',
   'DESIGN', 'CONFIG', 'COMMON', 1, TRUE),

  (5, 1, 'CC6.2',
   'Prior to issuing system credentials and granting system access, the entity registers and authorises new internal and external users whose access is administered by the entity.',
   'User provisioning SOP, sample access request tickets (population required), HR onboarding checklist.',
   'OE', 'CONFIG', 'COMMON', 1, TRUE),

  (6, 1, 'CC6.3',
   'The entity authorises, modifies, or removes access to data, software, functions, and other protected information assets based on approved and documented access requests.',
   'Quarterly access review report, de-provisioning tickets for all departures in the period.',
   'OE', 'CONFIG', 'COMMON', 1, TRUE),

  (7, 1, 'CC6.6',
   'The entity implements controls to prevent or detect and act upon the introduction of unauthorised or malicious software.',
   'Endpoint protection policy, full-scope AV/EDR scan report, SIEM alert summary for the period.',
   'DESIGN', 'CONFIG', 'COMMON', 1, TRUE),

  (8, 1, 'CC6.7',
   'The entity restricts the transmission, movement, and removal of information to authorised internal and external users and processes.',
   'DLP policy, data classification policy, network segmentation diagram.',
   'DESIGN', 'CONFIG', 'COMMON', 1, TRUE),

  -- CC7 — System Operations
  (9, 1, 'CC7.1',
   'The entity uses detection and monitoring procedures to identify changes to configurations or new vulnerabilities that could impact the entity\'s ability to meet its objectives.',
   'Vulnerability scan reports for the period, configuration drift alerts, patch management schedule.',
   'OE', 'CONFIG', 'COMMON', 1, TRUE),

  (10, 1, 'CC7.2',
   'The entity monitors system components and the operation of those components for anomalies that are indicative of malicious acts, natural disasters, and errors affecting the entity\'s ability to meet its objectives.',
   'SIEM dashboard screenshots, alert thresholds configuration, on-call runbook.',
   'OE', 'CONFIG', 'COMMON', 1, TRUE),

  -- CC8 — Change Management
  (11, 1, 'CC8.1',
   'The entity authorises, designs, develops or acquires, configures, documents, tests, approves, and implements changes to infrastructure, data, software, and procedures.',
   'Change management policy, approved change tickets for the period, pre/post deployment verification evidence.',
   'OE', 'CONFIG', 'COMMON', 1, TRUE),

  -- CC9 — Risk Mitigation
  (12, 1, 'CC9.2',
   'The entity assesses and manages risks associated with vendors and business partners.',
   'Vendor risk assessment records, third-party security questionnaires, vendor contract excerpts (confidentiality/security clauses).',
   'DESIGN', 'NON_CONFIG', 'COMMON', 1, TRUE),

  -- A-series — Availability
  (13, 1, 'A1.1',
   'The entity maintains, monitors, and evaluates current processing capacity and use of system components to manage capacity demand.',
   'Capacity planning report, auto-scaling configuration screenshots, load test results.',
   'DESIGN', 'CONFIG', 'PRODUCT_SPECIFIC', 1, TRUE),

  (14, 1, 'A1.2',
   'The entity authorises, designs, develops or acquires, implements, operates, approves, maintains, and monitors environmental protections, software, data back-up processes, and recovery infrastructure.',
   'DR/BCP policy, automated backup logs for the period, RTO/RPO test results.',
   'OE', 'CONFIG', 'PRODUCT_SPECIFIC', 1, TRUE),

  -- ── HIPAA Controls ─────────────────────────────────────────────────────────
  (15, 2, 'HIPAA-164.308(a)(1)',
   'Security Management Process: Implement policies and procedures to prevent, detect, contain, and correct security violations.',
   'Risk analysis report, risk management policy, sanction policy.',
   'DESIGN', 'NON_CONFIG', 'COMMON', 1, TRUE),

  (16, 2, 'HIPAA-164.308(a)(3)',
   'Workforce Security: Implement policies and procedures to ensure workforce members have appropriate access to ePHI, and to prevent access by those who should not have it.',
   'Access authorisation policy, clearance procedures, workforce termination/transfer procedures.',
   'OE', 'NON_CONFIG', 'COMMON', 1, TRUE),

  (17, 2, 'HIPAA-164.312(a)(1)',
   'Access Control: Implement technical policies and procedures that allow only authorised persons or software programs to access ePHI.',
   'Unique user ID evidence, emergency access procedure, automatic logoff configuration.',
   'DESIGN', 'CONFIG', 'COMMON', 1, TRUE),

  (18, 2, 'HIPAA-164.312(b)',
   'Audit Controls: Implement hardware, software, and/or procedural mechanisms that record and examine activity in information systems that contain or use ePHI.',
   'Audit log configuration, log retention policy, sample log review records.',
   'OE', 'CONFIG', 'COMMON', 1, TRUE),

  (19, 2, 'HIPAA-164.312(e)(1)',
   'Transmission Security: Implement technical security measures to guard against unauthorised access to ePHI being transmitted over an electronic communications network.',
   'TLS configuration evidence, network security assessment, VPN policy.',
   'DESIGN', 'CONFIG', 'COMMON', 1, TRUE),

  -- ── ISO 27001 Controls ─────────────────────────────────────────────────────
  (20, 3, 'A.9.1.1',
   'Access Control Policy: An access control policy shall be established, documented and reviewed based on business and information security requirements.',
   'Access control policy document (with review date within the period), approval sign-off.',
   'DESIGN', 'NON_CONFIG', 'COMMON', 1, TRUE),

  (21, 3, 'A.9.4.2',
   'Secure Log-on Procedures: Where required by the access control policy, access to systems and applications shall be controlled by a secure log-on procedure.',
   'MFA configuration evidence, failed-login lockout configuration, SSO integration screenshot.',
   'DESIGN', 'CONFIG', 'COMMON', 1, TRUE),

  (22, 3, 'A.12.4.1',
   'Event Logging: Event logs recording user activities, exceptions, faults and information security events shall be produced, kept and regularly reviewed.',
   'Log management solution configuration, retention schedule, sample review evidence.',
   'OE', 'CONFIG', 'COMMON', 1, TRUE),

  (23, 3, 'A.14.2.2',
   'System Change Control Procedures: Changes to systems within the development lifecycle shall be controlled by the use of formal change control procedures.',
   'Change control procedure, sample approved change records for the period, SDLC documentation.',
   'OE', 'NON_CONFIG', 'COMMON', 1, TRUE),

  (24, 3, 'A.17.1.1',
   'Planning Information Security Continuity: The organisation shall determine its requirements for information security and the continuity of information security management in adverse situations.',
   'BCP/DR plan, information security continuity objectives, annual test evidence.',
   'DESIGN', 'NON_CONFIG', 'PRODUCT_SPECIFIC', 1, TRUE)

ON DUPLICATE KEY UPDATE is_current = VALUES(is_current);

-- =============================================================================
-- AUDITS
-- =============================================================================

INSERT INTO audit (id, name, framework_id, product_id, period_start, period_end, status, scope_description, copied_from_audit_id, created_by) VALUES
  (1, 'SOC 2 Asgardeo 2026', 1, 1, '2026-01-01', '2026-12-31', 'ACTIVE',
   'Annual SOC 2 Type II audit covering the Asgardeo identity platform.',
   NULL, 'alice.silva@wso2.com'),
  (2, 'SOC 2 Choreo 2026',   1, 2, '2026-01-01', '2026-12-31', 'ACTIVE',
   'Annual SOC 2 Type II audit covering the Choreo integration platform.',
   NULL, 'alice.silva@wso2.com'),
  (3, 'SOC 2 Asgardeo 2025', 1, 1, '2025-01-01', '2025-12-31', 'COMPLETED',
   'Annual SOC 2 Type II audit covering the Asgardeo identity platform (prior year).',
   NULL, 'alice.silva@wso2.com'),
  (4, 'HIPAA Asgardeo 2026', 2, 1, '2026-01-01', '2026-12-31', 'ACTIVE',
   'HIPAA Security Rule compliance audit for Asgardeo handling of ePHI.',
   NULL, 'grace.kim@wso2.com')
ON DUPLICATE KEY UPDATE status = VALUES(status), scope_description = VALUES(scope_description);

-- =============================================================================
-- AUDIT CONTROLS
-- For template-linked controls: framework_control_id is set, definition cols are NULL.
-- For manual controls: framework_control_id is NULL, all definition cols are set.
-- =============================================================================

-- ── SOC 2 Asgardeo 2026 (audit_id = 1) ───────────────────────────────────────

INSERT INTO audit_control
  (id, audit_id, framework_control_id, owner_id, team_id, auditor_id, due_date, status, control_source, created_by)
VALUES
  (1,  1,  1, 3, 1, 8, '2026-09-30', 'COMPLETE',                    'COPIED', 'alice.silva@wso2.com'),
  (2,  1,  2, 3, 1, 8, '2026-09-30', 'EVIDENCE_INTERNAL_REVIEW',    'COPIED', 'alice.silva@wso2.com'),
  (3,  1,  3, 7, 1, 8, '2026-09-30', 'EVIDENCE_PENDING',            'COPIED', 'alice.silva@wso2.com'),
  (4,  1,  4, 5, 2, 8, '2026-08-31', 'EVIDENCE_UNDER_VALIDATION',   'COPIED', 'alice.silva@wso2.com'),
  (5,  1,  5, 4, 3, 8, '2026-08-31', 'POPULATION_PENDING',          'COPIED', 'alice.silva@wso2.com'),
  (6,  1,  6, 4, 3, 8, '2026-08-31', 'POPULATION_INTERNAL_REVIEW',  'COPIED', 'alice.silva@wso2.com'),
  (7,  1,  7, 5, 2, 8, '2026-09-30', 'EVIDENCE_NEED_CLARIFICATION', 'COPIED', 'alice.silva@wso2.com'),
  (8,  1,  9, 6, 4, 8, '2026-08-31', 'POPULATION_UNDER_VALIDATION', 'COPIED', 'alice.silva@wso2.com'),
  (9,  1, 10, 6, 4, 8, '2026-08-31', 'POPULATION_COMPLETE',         'COPIED', 'alice.silva@wso2.com'),
  (10, 1, 11, 5, 3, 8, '2026-10-31', 'AWAITING_SAMPLE',             'COPIED', 'alice.silva@wso2.com'),
  (11, 1, 12, 3, 1, 8, '2026-09-30', 'SUBMITTED_SAMPLE',            'COPIED', 'alice.silva@wso2.com'),
  (12, 1, 13, 4, 3, 8, '2026-10-31', 'EVIDENCE_PENDING',            'COPIED', 'alice.silva@wso2.com'),
  (13, 1, 14, 4, 3, 8, '2026-10-31', 'EVIDENCE_PENDING',            'COPIED', 'alice.silva@wso2.com')
ON DUPLICATE KEY UPDATE status = VALUES(status), owner_id = VALUES(owner_id);

-- Manual control in audit 1 (CUSTOM — no template row)
INSERT INTO audit_control
  (id, audit_id, framework_control_id,
   control_number, description, evidence_requirement,
   requirement_type, control_type, scope,
   owner_id, team_id, auditor_id, due_date, status, control_source, created_by)
VALUES
  (14, 1, NULL,
   'CUSTOM-01',
   'MFA enforcement rate must exceed 98% across all enterprise tenants throughout the audit period.',
   'MFA adoption dashboard screenshot, per-tenant enforcement report showing ≥98% compliance.',
   'OE', 'CONFIG', 'PRODUCT_SPECIFIC',
   5, 2, 8, '2026-08-31', 'POPULATION_PENDING', 'MANUAL', 'alice.silva@wso2.com')
ON DUPLICATE KEY UPDATE status = VALUES(status);

-- ── SOC 2 Choreo 2026 (audit_id = 2) ─────────────────────────────────────────

INSERT INTO audit_control
  (id, audit_id, framework_control_id, owner_id, team_id, auditor_id, due_date, status, control_source, created_by)
VALUES
  (15, 2,  1, 3, 1, 8, '2026-09-30', 'EVIDENCE_PENDING',         'COPIED', 'alice.silva@wso2.com'),
  (16, 2,  4, 5, 2, 8, '2026-08-31', 'EVIDENCE_INTERNAL_REVIEW', 'COPIED', 'alice.silva@wso2.com'),
  (17, 2,  7, 5, 2, 8, '2026-09-30', 'EVIDENCE_PENDING',         'COPIED', 'alice.silva@wso2.com'),
  (18, 2, 11, 4, 3, 8, '2026-10-31', 'EVIDENCE_PENDING',         'COPIED', 'alice.silva@wso2.com'),
  (19, 2, 13, 6, 4, 8, '2026-10-31', 'EVIDENCE_PENDING',         'COPIED', 'alice.silva@wso2.com')
ON DUPLICATE KEY UPDATE status = VALUES(status), owner_id = VALUES(owner_id);

-- ── SOC 2 Asgardeo 2025 (audit_id = 3, COMPLETED) ────────────────────────────

INSERT INTO audit_control
  (id, audit_id, framework_control_id, owner_id, team_id, auditor_id, due_date, status, control_source, created_by)
VALUES
  (20, 3,  1, 3, 1, 8, '2025-09-30', 'COMPLETE', 'COPIED', 'alice.silva@wso2.com'),
  (21, 3,  4, 5, 2, 8, '2025-08-31', 'COMPLETE', 'COPIED', 'alice.silva@wso2.com'),
  (22, 3,  7, 5, 2, 8, '2025-09-30', 'COMPLETE', 'COPIED', 'alice.silva@wso2.com'),
  (23, 3,  9, 6, 4, 8, '2025-08-31', 'COMPLETE', 'COPIED', 'alice.silva@wso2.com'),
  (24, 3, 11, 5, 3, 8, '2025-10-31', 'COMPLETE', 'COPIED', 'alice.silva@wso2.com')
ON DUPLICATE KEY UPDATE status = VALUES(status);

-- ── HIPAA Asgardeo 2026 (audit_id = 4) ───────────────────────────────────────

INSERT INTO audit_control
  (id, audit_id, framework_control_id, owner_id, team_id, auditor_id, due_date, status, control_source, created_by)
VALUES
  (25, 4, 15, 3, 1, 6, '2026-09-30', 'EVIDENCE_PENDING',          'COPIED', 'grace.kim@wso2.com'),
  (26, 4, 16, 4, 3, 6, '2026-08-31', 'EVIDENCE_PENDING',          'COPIED', 'grace.kim@wso2.com'),
  (27, 4, 17, 5, 2, 6, '2026-08-31', 'EVIDENCE_INTERNAL_REVIEW',  'COPIED', 'grace.kim@wso2.com'),
  (28, 4, 18, 6, 4, 6, '2026-08-31', 'EVIDENCE_UNDER_VALIDATION', 'COPIED', 'grace.kim@wso2.com'),
  (29, 4, 19, 5, 2, 6, '2026-09-30', 'EVIDENCE_PENDING',          'COPIED', 'grace.kim@wso2.com')
ON DUPLICATE KEY UPDATE status = VALUES(status), owner_id = VALUES(owner_id);

-- =============================================================================
-- POPULATIONS  (OE controls only)
-- =============================================================================

INSERT INTO audit_population
  (id, control_id, owner_id, team_id, reference_number, description, status, due_date, created_by)
VALUES
  -- CC6.2 (control 5) — user provisioning events
  (1, 5, 4, 3, 247,
   'All user provisioning events for Jan 1 – Jun 30 2026 (247 events). '
   'Each row includes request ticket, approver email, effective date, and access level granted.',
   'PENDING', '2026-08-01', 'diana.patel@wso2.com'),

  -- CC6.3 (control 6) — quarterly access review
  (2, 6, 4, 3, 183,
   'Q1 2026 access review: 183 users reviewed. '
   'Evidence includes review completion email chain and updated access matrix.',
   'SUBMITTED', '2026-08-01', 'diana.patel@wso2.com'),

  -- CC7.1 (control 8) — vulnerability scan results
  (3, 8, 6, 4, 91,
   'Vulnerability scan results for Q1–Q2 2026: 91 unique findings, '
   'severity breakdown included, all critical/high remediated within SLA.',
   'COMPLIANCE_APPROVED', '2026-07-31', 'frank.lee@wso2.com'),

  -- CC7.2 (control 9) — SIEM alert log
  (4, 9, 6, 4, 312,
   'SIEM alert export for Q1–Q2 2026: 312 alerts generated, 12 escalated, all resolved. '
   'Alert rule configuration and on-call response records included.',
   'APPROVED', '2026-07-31', 'frank.lee@wso2.com'),

  -- CUSTOM-01 (control 14) — MFA enforcement
  (5, 14, 5, 2, 44,
   'MFA enforcement sample: 44 enterprise tenants sampled. '
   'Adoption rate 98.6%. Non-compliant tenants listed with remediation actions taken.',
   'PENDING', '2026-08-15', 'eve.johnson@wso2.com')

ON DUPLICATE KEY UPDATE status = VALUES(status), description = VALUES(description);

-- =============================================================================
-- EVIDENCE
-- =============================================================================

INSERT INTO audit_evidence
  (id, control_id, submitted_by, status, folder_path, created_by)
VALUES
  (1,  1,  3, 'APPROVED',            'audits/1/controls/1',  'charlie.ng@wso2.com'),
  (2,  2,  3, 'COMPLIANCE_APPROVED', 'audits/1/controls/2',  'charlie.ng@wso2.com'),
  (3,  4,  5, 'COMPLIANCE_APPROVED', 'audits/1/controls/4',  'eve.johnson@wso2.com'),
  (4,  7,  5, 'COMPLIANCE_REJECTED', 'audits/1/controls/7',  'eve.johnson@wso2.com'),
  (5, 16,  5, 'COMPLIANCE_APPROVED', 'audits/2/controls/16', 'eve.johnson@wso2.com'),
  (6, 27,  5, 'SUBMITTED',           'audits/4/controls/27', 'eve.johnson@wso2.com'),
  (7, 28,  6, 'COMPLIANCE_APPROVED', 'audits/4/controls/28', 'frank.lee@wso2.com')
ON DUPLICATE KEY UPDATE status = VALUES(status);

-- =============================================================================
-- EVIDENCE FILES
-- =============================================================================

INSERT INTO audit_evidence_file
  (id, evidence_id, population_id, file_kind, uploaded_by, file_name, file_path, file_type, file_size, created_by)
VALUES
  -- CC1.1 evidence files (evidence_id=1)
  (1, 1, NULL, NULL, 3,
   'code-of-conduct-acknowledgements-2026.pdf',
   'grc-evidence/audits/1/controls/1/code-of-conduct-acknowledgements-2026.pdf',
   'application/pdf', 245760, 'charlie.ng@wso2.com'),

  (2, 1, NULL, NULL, 3,
   'ethics-training-completion-jan-jun-2026.xlsx',
   'grc-evidence/audits/1/controls/1/ethics-training-completion-jan-jun-2026.xlsx',
   'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', 98304, 'charlie.ng@wso2.com'),

  -- CC1.2 evidence file (evidence_id=2)
  (3, 2, NULL, NULL, 3,
   'board-meeting-minutes-q1-2026.pdf',
   'grc-evidence/audits/1/controls/2/board-meeting-minutes-q1-2026.pdf',
   'application/pdf', 512000, 'charlie.ng@wso2.com'),

  -- CC6.1 evidence files (evidence_id=3)
  (4, 3, NULL, NULL, 5,
   'iam-config-screenshot-2026-06.png',
   'grc-evidence/audits/1/controls/4/iam-config-screenshot-2026-06.png',
   'image/png', 184320, 'eve.johnson@wso2.com'),

  (5, 3, NULL, NULL, 5,
   'mfa-enrollment-report-q2-2026.xlsx',
   'grc-evidence/audits/1/controls/4/mfa-enrollment-report-q2-2026.xlsx',
   'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', 73728, 'eve.johnson@wso2.com'),

  -- CC6.6 evidence file (evidence_id=4, rejected — resubmission pending)
  (6, 4, NULL, NULL, 5,
   'endpoint-protection-policy-v3.pdf',
   'grc-evidence/audits/1/controls/7/endpoint-protection-policy-v3.pdf',
   'application/pdf', 307200, 'eve.johnson@wso2.com'),

  -- Population files
  (7, NULL, 2, 'POPULATION', 4,
   'access-review-q1-2026.xlsx',
   'grc-evidence/audits/1/controls/6/population/access-review-q1-2026.xlsx',
   'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', 153600, 'diana.patel@wso2.com'),

  (8, NULL, 3, 'POPULATION', 6,
   'vuln-scan-q2-2026.pdf',
   'grc-evidence/audits/1/controls/8/population/vuln-scan-q2-2026.pdf',
   'application/pdf', 409600, 'frank.lee@wso2.com')

ON DUPLICATE KEY UPDATE file_name = VALUES(file_name);

-- =============================================================================
-- COMMENTS
-- =============================================================================

INSERT INTO audit_comment
  (id, evidence_id, author_id, parent_comment_id, content, is_internal, created_by)
VALUES
  (1, 2, 1, NULL,
   'Board minutes look complete — all required elements present. Approved at compliance review.',
   FALSE, 'alice.silva@wso2.com'),

  (2, 4, 8, NULL,
   'The AV scan report is missing the batch processing cluster (10.20.30.0/24). '
   'Please resubmit with a full-scope scan covering all subnets.',
   FALSE, 'henry.white@wso2.com'),

  (3, 4, 5, 2,
   'Understood — resubmitting with the batch cluster included. Will have it by EOD tomorrow.',
   FALSE, 'eve.johnson@wso2.com'),

  (4, 3, 1, NULL,
   'IAM config looks good. Forwarding to auditor for review.',
   TRUE, 'alice.silva@wso2.com'),

  (5, 6, 1, NULL,
   'Pending compliance review. Please include the data flow diagram alongside the access control matrix.',
   FALSE, 'alice.silva@wso2.com')

ON DUPLICATE KEY UPDATE content = VALUES(content);

-- =============================================================================
-- AI VALIDATION LOG
-- =============================================================================

INSERT INTO audit_ai_validation_log
  (id, evidence_id, control_id, result, gaps_found, summary, confidence_score, created_by)
VALUES
  (1, 1, 1, 'PASS', NULL,
   'Evidence covers all required elements: code of conduct signatures and training completion '
   'records are present and dated within the audit period.',
   0.9400, 'ai-validator'),

  (2, 3, 4, 'PASS', NULL,
   'IAM configuration screenshot shows MFA enabled system-wide. Access policies are consistent '
   'with the stated access control policy. MFA enrollment report covers the full audit period.',
   0.8800, 'ai-validator'),

  (3, 4, 7, 'FAIL',
   'AV/EDR scan does not include the batch processing cluster (10.20.30.0/24 subnet). '
   'Scope appears limited to web and app tiers only.',
   'Partial scope: scan report covers web and app tiers but excludes the batch cluster. '
   'Control CC6.6 requires full-scope endpoint protection evidence.',
   0.9200, 'ai-validator'),

  (4, 6, 27, 'UNCERTAIN',
   'Access control policy document present but the most recent review date is 18 months ago. '
   'HIPAA 164.312(a)(1) requires policy review at least annually.',
   'Policy document uploaded but may be outdated relative to the audit period. '
   'Verify the review date and resubmit a current version if required.',
   0.7100, 'ai-validator')

ON DUPLICATE KEY UPDATE result = VALUES(result), summary = VALUES(summary);

-- =============================================================================
-- NOTIFICATIONS
-- =============================================================================

INSERT INTO audit_notification
  (id, recipient_id, audit_id, control_id, evidence_id, type, channel, message, is_read, created_by)
VALUES
  (1, 5, 1,  7, 4, 'REJECTION', 'IN_APP',
   'Your evidence for CC6.6 (Malicious Software Prevention) was rejected. See comment for details.',
   FALSE, 'system'),

  (2, 3, 1,  2, 2, 'APPROVAL', 'IN_APP',
   'Your evidence for CC1.2 (Board Oversight) has been approved by the compliance team.',
   TRUE, 'system'),

  (3, 5, 1,  4, 3, 'APPROVAL', 'IN_APP',
   'Your evidence for CC6.1 (Logical Access Security) has been approved and forwarded to the auditor.',
   TRUE, 'system'),

  (4, 4, 1,  5, NULL, 'REMINDER', 'EMAIL',
   'Reminder: Population submission for CC6.2 (User Registration & Authorisation) is due in 7 days.',
   FALSE, 'system'),

  (5, 8, 1, 11, NULL, 'REMINDER', 'IN_APP',
   'Sample selection for CC9.2 (Vendor Risk) is awaiting your input.',
   FALSE, 'system')

ON DUPLICATE KEY UPDATE message = VALUES(message);

-- =============================================================================
-- AUDIT TRAIL
-- =============================================================================

INSERT INTO audit_trail
  (id, actor_id, audit_id, control_id, evidence_id, action, details, created_by)
VALUES
  (1,  1, 1, NULL, NULL, 'CREATED',   '{"name":"SOC 2 Asgardeo 2026"}',                                             'alice.silva@wso2.com'),
  (2,  1, 2, NULL, NULL, 'CREATED',   '{"name":"SOC 2 Choreo 2026"}',                                               'alice.silva@wso2.com'),
  (3,  7, 4, NULL, NULL, 'CREATED',   '{"name":"HIPAA Asgardeo 2026"}',                                             'grace.kim@wso2.com'),
  (4,  3, 1, 1,  1, 'UPLOADED',  '{"file":"code-of-conduct-acknowledgements-2026.pdf","control":"CC1.1"}',          'charlie.ng@wso2.com'),
  (5,  3, 1, 1,  1, 'UPLOADED',  '{"file":"ethics-training-completion-jan-jun-2026.xlsx","control":"CC1.1"}',       'charlie.ng@wso2.com'),
  (6,  1, 1, 1,  1, 'APPROVED',  '{"stage":"COMPLIANCE","control":"CC1.1"}',                                       'alice.silva@wso2.com'),
  (7,  8, 1, 1,  1, 'APPROVED',  '{"stage":"AUDITOR","control":"CC1.1"}',                                          'henry.white@wso2.com'),
  (8,  3, 1, 2,  2, 'UPLOADED',  '{"file":"board-meeting-minutes-q1-2026.pdf","control":"CC1.2"}',                  'charlie.ng@wso2.com'),
  (9,  1, 1, 2,  2, 'APPROVED',  '{"stage":"COMPLIANCE","control":"CC1.2"}',                                       'alice.silva@wso2.com'),
  (10, 5, 1, 4,  3, 'UPLOADED',  '{"file":"iam-config-screenshot-2026-06.png","control":"CC6.1"}',                  'eve.johnson@wso2.com'),
  (11, 5, 1, 4,  3, 'UPLOADED',  '{"file":"mfa-enrollment-report-q2-2026.xlsx","control":"CC6.1"}',                 'eve.johnson@wso2.com'),
  (12, 1, 1, 4,  3, 'APPROVED',  '{"stage":"COMPLIANCE","control":"CC6.1"}',                                       'alice.silva@wso2.com'),
  (13, 5, 1, 7,  4, 'UPLOADED',  '{"file":"endpoint-protection-policy-v3.pdf","control":"CC6.6"}',                  'eve.johnson@wso2.com'),
  (14, 8, 1, 7,  4, 'COMMENTED', '{"comment":"AV scan missing batch cluster","control":"CC6.6"}',                   'henry.white@wso2.com'),
  (15, 1, 1, 7,  4, 'REJECTED',  '{"stage":"COMPLIANCE","reason":"Missing batch cluster scan","control":"CC6.6"}',  'alice.silva@wso2.com'),
  (16, 5, 1, 7,  4, 'COMMENTED', '{"comment":"Acknowledged, resubmitting with full scope"}',                        'eve.johnson@wso2.com'),
  (17, 4, 1, 5, NULL,'CREATED',  '{"populationRef":247,"control":"CC6.2"}',                                        'diana.patel@wso2.com'),
  (18, 4, 1, 6, NULL,'UPLOADED', '{"file":"access-review-q1-2026.xlsx","control":"CC6.3"}',                         'diana.patel@wso2.com'),
  (19, 1, 1, 6, NULL,'APPROVED', '{"stage":"COMPLIANCE_POPULATION","control":"CC6.3"}',                             'alice.silva@wso2.com'),
  (20, 6, 1, 8, NULL,'UPLOADED', '{"file":"vuln-scan-q2-2026.pdf","control":"CC7.1"}',                              'frank.lee@wso2.com'),
  (21, 1, 1, 8, NULL,'APPROVED', '{"stage":"COMPLIANCE_POPULATION","control":"CC7.1"}',                             'alice.silva@wso2.com'),
  (22, 8, 1, 8, NULL,'APPROVED', '{"stage":"AUDITOR_POPULATION","control":"CC7.1"}',                                'henry.white@wso2.com')

ON DUPLICATE KEY UPDATE action = VALUES(action);

SET FOREIGN_KEY_CHECKS = 1;
