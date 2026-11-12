USE grc_platform;

-- Workflow-aware due dates: two new tables, nothing existing is altered.
-- Deploy order: run this first, then the Compliance Entity, then the backend.

-- Allowed days per audit, per cycle and group. An audit with no rows uses the
-- built-in defaults, so existing audits need no data change.
CREATE TABLE IF NOT EXISTS audit_deadline_days (
  audit_id     INT                          NOT NULL,
  phase        ENUM('EVIDENCE','POPULATION') NOT NULL DEFAULT 'EVIDENCE',
  cycle_type   ENUM('INITIAL','REVISION')    NOT NULL,
  handled_by   ENUM('OWNER','COMPLIANCE','AUDITOR') NOT NULL,
  allowed_days SMALLINT UNSIGNED             NOT NULL,
  PRIMARY KEY (audit_id, phase, cycle_type, handled_by),
  CONSTRAINT fk_deadline_days_audit FOREIGN KEY (audit_id) REFERENCES audit(id) ON DELETE CASCADE,
  CONSTRAINT chk_deadline_days_range CHECK (allowed_days BETWEEN 1 AND 365)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Hand-off history: one row per period a group holds a control. The row with a
-- NULL ended_at is the current holder. The planned_* dates are only set on a
-- control's first row.
CREATE TABLE IF NOT EXISTS audit_control_stage (
  id                      BIGINT       NOT NULL AUTO_INCREMENT,
  control_id              INT          NOT NULL,
  phase                   ENUM('EVIDENCE','POPULATION') NOT NULL DEFAULT 'EVIDENCE',
  handled_by              ENUM('OWNER','COMPLIANCE','AUDITOR') NOT NULL,
  cycle_type              ENUM('INITIAL','REVISION') NOT NULL,
  started_at              DATETIME     NOT NULL,
  due_date                DATE         NOT NULL,
  planned_compliance_date DATE         NULL,
  planned_auditor_date    DATE         NULL,
  ended_at                DATETIME     NULL,
  updated_by              VARCHAR(255) NULL,
  PRIMARY KEY (id),
  CONSTRAINT fk_stage_control FOREIGN KEY (control_id) REFERENCES audit_control(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
