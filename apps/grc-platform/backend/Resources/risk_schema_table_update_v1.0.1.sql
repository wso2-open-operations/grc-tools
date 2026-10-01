-- Updated 01/10/2026 (v1.0.1)
--
-- Adds risk_reminder: the due-date reminder sweep's de-dup log. Additive
-- only — no existing table is altered. The statement below is already
-- present in risk_schema.sql (the full, current schema); this file is the
-- standalone copy for running against a database that already has every
-- other Risk module table.
--
-- The insert IS the claim: production runs several backend replicas, each
-- firing the same daily sweep, so the unique key below is what makes exactly
-- one of them send a given reminder. FK CASCADE to risk, matching
-- risk_escalation's pattern.
--
-- Rollback: DROP TABLE risk_reminder; — safe only before this table has
-- accumulated real claims, i.e. before the reminder feature has run in this
-- environment. Once it has, dropping and recreating the table discards the
-- record of which reminders already went out; any risk whose tier still
-- matches today's date on the next sweep after that will be reminded again.
-- There is no way to recover that history once dropped.

USE grc_platform;

CREATE TABLE IF NOT EXISTS risk_reminder (
  id                BIGINT       NOT NULL AUTO_INCREMENT,
  risk_id           INT          NOT NULL,
  reminder_type     ENUM('DUE_IN_15_DAYS','DUE_IN_5_DAYS','DUE_TODAY') NOT NULL,
  due_date_snapshot DATE         NOT NULL COMMENT 'The implementation_date this reminder was sent for',
  created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by        VARCHAR(255) NULL COMMENT 'Actor that wrote the row; the daily sweep writes system',
  PRIMARY KEY (id),
  UNIQUE KEY uq_risk_reminder (risk_id, reminder_type, due_date_snapshot),
  CONSTRAINT fk_risk_reminder_risk FOREIGN KEY (risk_id) REFERENCES risk(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
