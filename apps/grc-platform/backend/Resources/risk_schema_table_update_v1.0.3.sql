-- Updated 07/10/2026 (v1.0.3)
--
-- Adds risk_ai_suggestion: one row per AI suggestion shown to a user, for
-- every feature that shares this shape (one suggested value, a confidence,
-- accept-or-override) — auto-categorisation, likelihood prediction, and
-- action plan description suggestion. Additive only — no existing table is
-- altered. The statement below is already present in risk_schema.sql (the
-- full, current schema); this file is the standalone copy for running
-- against a database that already has every other Risk module table.
--
-- suggested_value is TEXT, not VARCHAR, so it can hold a full drafted action
-- plan description as well as a short category id or 1-3 likelihood score.
-- decided_by stores the actor's UUID (Asgardeo `sub` claim), the same
-- convention every other created_by/updated_by in this schema uses.
--
-- Rollback: DROP TABLE risk_ai_suggestion; — safe only before this table has
-- accumulated real suggestion history, i.e. before any of the three features
-- above have run in this environment. Once it has, dropping and recreating
-- the table discards every past suggestion and decision on record; there is
-- no way to recover that history once dropped.

USE grc_platform;

CREATE TABLE IF NOT EXISTS risk_ai_suggestion (
  id                BIGINT        NOT NULL AUTO_INCREMENT,
  risk_id           INT           NOT NULL,
  feature           ENUM('CATEGORY','LIKELIHOOD','ACTION_PLAN') NOT NULL,
  suggested_value   TEXT          NOT NULL,
  suggested_reason  TEXT          NULL,
  confidence        ENUM('HIGH','MEDIUM','LOW') NULL,
  status            ENUM('SUGGESTED','ACCEPTED','OVERRIDDEN') NOT NULL DEFAULT 'SUGGESTED',
  override_reason   TEXT          NULL,
  decided_by        VARCHAR(255)  NULL COMMENT 'Actor UUID (Asgardeo sub claim) who accepted/overrode the suggestion',
  decided_at        DATETIME      NULL,
  created_at        DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_ras_risk (risk_id),
  KEY idx_ras_feature (feature),
  CONSTRAINT fk_ras_risk FOREIGN KEY (risk_id) REFERENCES risk(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
