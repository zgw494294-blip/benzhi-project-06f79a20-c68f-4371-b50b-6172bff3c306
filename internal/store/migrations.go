package store

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 2

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS schema_meta (schema_version INTEGER NOT NULL);`,
	`CREATE TABLE IF NOT EXISTS dossiers (
 dossier_id TEXT PRIMARY KEY,title TEXT NOT NULL,participant_name TEXT NOT NULL,interviewer_name TEXT NOT NULL,
 session_date TEXT NOT NULL,material_summary TEXT NOT NULL,consent_scope TEXT NOT NULL,status TEXT NOT NULL,
 version INTEGER NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);`,
	`CREATE TABLE IF NOT EXISTS revisions (
 revision_id TEXT PRIMARY KEY,dossier_id TEXT NOT NULL,revision_number INTEGER NOT NULL,source_digest TEXT NOT NULL,
 content_digest TEXT NOT NULL,submitted_by TEXT NOT NULL,submitted_at TEXT NOT NULL,supersedes_revision_id TEXT NOT NULL,
 UNIQUE(dossier_id,revision_number),FOREIGN KEY(dossier_id) REFERENCES dossiers(dossier_id));`,
	`CREATE TABLE IF NOT EXISTS segments (
 segment_id TEXT NOT NULL,revision_id TEXT NOT NULL,sequence INTEGER NOT NULL,text TEXT NOT NULL,sensitivity_tags TEXT NOT NULL,
 proposed_access_level TEXT NOT NULL,embargo_until TEXT,segment_digest TEXT NOT NULL,
 PRIMARY KEY(segment_id,revision_id),FOREIGN KEY(revision_id) REFERENCES revisions(revision_id));`,
	`CREATE TABLE IF NOT EXISTS review_decisions (
 decision_id TEXT PRIMARY KEY,dossier_id TEXT NOT NULL,segment_id TEXT NOT NULL,decision_type TEXT NOT NULL,
 requested_text TEXT NOT NULL,requested_access_level TEXT NOT NULL,reason TEXT NOT NULL,resolution TEXT NOT NULL,
 resolved_by TEXT NOT NULL,decided_at TEXT NOT NULL,resolved_at TEXT,FOREIGN KEY(dossier_id) REFERENCES dossiers(dossier_id));`,
	`CREATE TABLE IF NOT EXISTS manifests (
 manifest_id TEXT PRIMARY KEY,dossier_id TEXT NOT NULL UNIQUE,revision_id TEXT NOT NULL,consent_digest TEXT NOT NULL,
 decision_digest TEXT NOT NULL,segment_entries TEXT NOT NULL,manifest_digest TEXT NOT NULL,sealed_at TEXT NOT NULL,released_at TEXT,
 FOREIGN KEY(dossier_id) REFERENCES dossiers(dossier_id));`,
	`CREATE TABLE IF NOT EXISTS audit_events (
 event_id TEXT PRIMARY KEY,dossier_id TEXT NOT NULL,sequence INTEGER NOT NULL,event_type TEXT NOT NULL,actor TEXT NOT NULL,
 reason TEXT NOT NULL,before_status TEXT NOT NULL,after_status TEXT NOT NULL,occurred_at TEXT NOT NULL,
 previous_digest TEXT NOT NULL,event_digest TEXT NOT NULL,UNIQUE(dossier_id,sequence),FOREIGN KEY(dossier_id) REFERENCES dossiers(dossier_id));`,
	`CREATE INDEX IF NOT EXISTS idx_revisions_dossier ON revisions(dossier_id,revision_number);`,
	`CREATE INDEX IF NOT EXISTS idx_decisions_dossier ON review_decisions(dossier_id,decided_at);`,
	`CREATE INDEX IF NOT EXISTS idx_audit_dossier ON audit_events(dossier_id,sequence);`,
}

var version2Migrations = []string{
	`ALTER TABLE review_decisions ADD COLUMN revision_id TEXT NOT NULL DEFAULT '';`,
	`ALTER TABLE review_decisions ADD COLUMN participant TEXT NOT NULL DEFAULT '';`,
	`UPDATE review_decisions SET revision_id=(SELECT revision_id FROM revisions WHERE revisions.dossier_id=review_decisions.dossier_id ORDER BY revision_number DESC LIMIT 1), participant=(SELECT participant_name FROM dossiers WHERE dossiers.dossier_id=review_decisions.dossier_id) WHERE revision_id='';`,
	`CREATE TABLE IF NOT EXISTS review_drafts (
 dossier_id TEXT NOT NULL,revision_id TEXT NOT NULL,participant TEXT NOT NULL,decisions TEXT NOT NULL,saved_at TEXT NOT NULL,
 PRIMARY KEY(dossier_id,revision_id,participant),FOREIGN KEY(dossier_id) REFERENCES dossiers(dossier_id),FOREIGN KEY(revision_id) REFERENCES revisions(revision_id));`,
	`CREATE INDEX IF NOT EXISTS idx_review_drafts_dossier ON review_drafts(dossier_id,revision_id,participant);`,
}

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range migrations {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("执行数据库迁移: %w", err)
		}
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_meta").Scan(&count); err != nil {
		return err
	}
	currentVersion := 1
	if count > 0 {
		if err = tx.QueryRowContext(ctx, "SELECT schema_version FROM schema_meta LIMIT 1").Scan(&currentVersion); err != nil {
			return err
		}
	}
	if currentVersion < 2 {
		for _, statement := range version2Migrations {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("执行数据库版本 2 迁移: %w", err)
			}
		}
	}
	if count == 0 {
		_, err = tx.ExecContext(ctx, "INSERT INTO schema_meta(schema_version) VALUES (?)", schemaVersion)
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE schema_meta SET schema_version=?", schemaVersion)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
