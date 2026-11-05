-- Updated 03/11/2026 (v1.0.7)
--
-- Adds sub-platforms and the platforms each register offers: risk_platform.parent_id
-- (a platform with a parent is a sub-platform; NULL is top-level) and the
-- risk_team_platform table. Run after v1.0.6 and before deploying the new
-- compliance-entity, which selects parent_id. Deploy order: this script, then
-- compliance-entity, then backend, then webapp. Create sub-platforms only after
-- all three are deployed.
--
-- Additive only: one nullable column, no default, no backfill, and one new empty
-- table. Every existing platform reads NULL and every existing register has no
-- rows, so it keeps offering every active platform. No row is touched. If MySQL
-- cannot add the column online (ALGORITHM=INPLACE, LOCK=NONE) it errors instead
-- of locking the table, and nothing is changed.
--
-- Already in risk_schema.sql; this is the standalone copy for an existing
-- database. Re-running is a safe no-op: the add is guarded on information_schema
-- and the table uses IF NOT EXISTS. foreign_key_checks is off only for the one
-- ALTER, because MySQL adds a foreign key in place only then.
--
-- Rollback (delete any sub-platform rows first):
--   DROP TABLE risk_team_platform;
--   ALTER TABLE risk_platform DROP FOREIGN KEY fk_rplat_parent, DROP COLUMN parent_id;

USE grc_platform;

SET @platform_has_parent = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'risk_platform'
    AND COLUMN_NAME = 'parent_id'
);
SET @add_parent_sql = IF(@platform_has_parent = 0,
  'ALTER TABLE risk_platform '
  'ADD COLUMN parent_id INT NULL '
  'COMMENT ''Set on a sub-platform: the top-level platform it sits under. NULL for a top-level platform, including every platform that predates the column'' '
  'AFTER name, '
  'ADD CONSTRAINT fk_rplat_parent FOREIGN KEY (parent_id) REFERENCES risk_platform(id) ON DELETE RESTRICT, '
  'ALGORITHM=INPLACE, LOCK=NONE',
  'SELECT 1');

SET @prev_fk_checks = @@FOREIGN_KEY_CHECKS;
SET FOREIGN_KEY_CHECKS = 0;
PREPARE add_parent_stmt FROM @add_parent_sql;
EXECUTE add_parent_stmt;
DEALLOCATE PREPARE add_parent_stmt;
SET FOREIGN_KEY_CHECKS = @prev_fk_checks;

CREATE TABLE IF NOT EXISTS risk_team_platform (
  risk_team_id INT      NOT NULL,
  platform_id  INT      NOT NULL,
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (risk_team_id, platform_id),
  KEY idx_rtplat_platform (platform_id),
  CONSTRAINT fk_rtplat_team     FOREIGN KEY (risk_team_id) REFERENCES risk_team(id)     ON DELETE RESTRICT,
  CONSTRAINT fk_rtplat_platform FOREIGN KEY (platform_id)  REFERENCES risk_platform(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Expect Type int, Null = YES, Default = NULL.
SHOW COLUMNS FROM risk_platform LIKE 'parent_id';

-- Expect with_parent = 0 right after this runs.
SELECT COUNT(*) AS total, COUNT(parent_id) AS with_parent FROM risk_platform;

-- Expect 0 right after this runs.
SELECT COUNT(*) AS register_platform_rows FROM risk_team_platform;
