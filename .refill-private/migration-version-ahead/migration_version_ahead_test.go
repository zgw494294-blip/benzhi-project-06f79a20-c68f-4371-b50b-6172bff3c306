package migration_version_ahead_test

import (
	"context"
	"database/sql"
	"testing"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
	_ "modernc.org/sqlite"
)

func TestRestartDoesNotAcceptPartiallyMigratedSchema(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/partial-migration.db"
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	statements := []string{
		`CREATE TABLE schema_meta (schema_version INTEGER NOT NULL)`,
		`INSERT INTO schema_meta(schema_version) VALUES (1)`,
		`CREATE TABLE review_decisions (
 decision_id TEXT PRIMARY KEY,dossier_id TEXT NOT NULL,segment_id TEXT NOT NULL,decision_type TEXT NOT NULL,
 requested_text TEXT NOT NULL,requested_access_level TEXT NOT NULL,reason TEXT NOT NULL,resolution TEXT NOT NULL,
 resolved_by TEXT NOT NULL,decided_at TEXT NOT NULL,resolved_at TEXT,revision_id TEXT NOT NULL DEFAULT '')`,
	}
	for _, statement := range statements {
		if _, err := legacy.ExecContext(ctx, statement); err != nil {
			legacy.Close()
			t.Fatalf("prepare partial schema: %v", err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	if first, err := store.Open(ctx, path); err == nil {
		first.Close()
		t.Fatal("first Open unexpectedly accepted partial v2 schema")
	}
	second, err := store.Open(ctx, path)
	if err != nil {
		return
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close reopened repository: %v", err)
	}

	probe, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open schema probe: %v", err)
	}
	defer probe.Close()
	rows, err := probe.QueryContext(ctx, `SELECT participant FROM review_decisions`)
	if err != nil {
		t.Fatalf("restart accepted schemaVersion 2 without participant column: %v", err)
	}
	rows.Close()
}
