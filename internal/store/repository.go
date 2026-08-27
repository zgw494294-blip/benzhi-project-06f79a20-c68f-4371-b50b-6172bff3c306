package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
	_ "modernc.org/sqlite"
)

type Repository struct {
	db        *sql.DB
	cacheMu   sync.RWMutex
	snapshots map[string]Snapshot
}

func Open(ctx context.Context, path string) (*Repository, error) {
	if strings.TrimSpace(path) == "" {
		path = ":memory:"
	}
	dsn := path
	if path == ":memory:" {
		dsn = "file:oralhistory?mode=memory&cache=shared"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	if err = migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Repository{db: db, snapshots: make(map[string]Snapshot)}, nil
}

func (r *Repository) Close() error { return r.db.Close() }

func (r *Repository) Create(ctx context.Context, s Snapshot) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = insertDossier(ctx, tx, s.Dossier); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return domain.ErrVersionConflict
		}
		return err
	}
	if err = writeChildren(ctx, tx, s); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) Save(ctx context.Context, s Snapshot, expectedVersion int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE dossiers SET title=?,participant_name=?,interviewer_name=?,session_date=?,material_summary=?,consent_scope=?,status=?,version=?,updated_at=? WHERE dossier_id=? AND version=?`,
		s.Dossier.Title, s.Dossier.ParticipantName, s.Dossier.InterviewerName, s.Dossier.SessionDate, s.Dossier.MaterialSummary, string(s.Dossier.ConsentScope), string(s.Dossier.Status), s.Dossier.Version, timeString(s.Dossier.UpdatedAt), s.Dossier.DossierID, expectedVersion)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return domain.ErrVersionConflict
	}
	for _, table := range []string{"segments", "review_drafts", "revisions", "review_decisions", "manifests", "audit_events"} {
		query := fmt.Sprintf("DELETE FROM %s WHERE dossier_id=?", table)
		if table == "segments" {
			query = "DELETE FROM segments WHERE revision_id IN (SELECT revision_id FROM revisions WHERE dossier_id=?)"
		}
		if _, err = tx.ExecContext(ctx, query, s.Dossier.DossierID); err != nil {
			return err
		}
	}
	if err = writeChildren(ctx, tx, s); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	r.cacheMu.Lock()
	delete(r.snapshots, s.Dossier.DossierID)
	r.cacheMu.Unlock()
	return nil
}

func (r *Repository) Get(ctx context.Context, id string) (Snapshot, error) {
	var s Snapshot
	s.Segments = map[string][]domain.TranscriptSegment{}
	row := r.db.QueryRowContext(ctx, `SELECT dossier_id,title,participant_name,interviewer_name,session_date,material_summary,consent_scope,status,version,created_at,updated_at FROM dossiers WHERE dossier_id=?`, id)
	var consent, status, created, updated string
	err := row.Scan(&s.Dossier.DossierID, &s.Dossier.Title, &s.Dossier.ParticipantName, &s.Dossier.InterviewerName, &s.Dossier.SessionDate, &s.Dossier.MaterialSummary, &consent, &status, &s.Dossier.Version, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return s, domain.ErrNotFound
	}
	if err != nil {
		return s, err
	}
	s.Dossier.ConsentScope = domain.AccessLevel(consent)
	s.Dossier.Status = domain.DossierStatus(status)
	s.Dossier.CreatedAt = parseTime(created)
	s.Dossier.UpdatedAt = parseTime(updated)
	r.cacheMu.RLock()
	cached, cacheHit := r.snapshots[id]
	r.cacheMu.RUnlock()
	if cacheHit && cached.Dossier.Version == s.Dossier.Version {
		return cached.Clone(), nil
	}
	if err = loadRevisions(ctx, r.db, &s); err != nil {
		return s, err
	}
	if err = loadDecisions(ctx, r.db, &s); err != nil {
		return s, err
	}
	if err = loadReviewDrafts(ctx, r.db, &s); err != nil {
		return s, err
	}
	if err = loadManifest(ctx, r.db, &s); err != nil {
		return s, err
	}
	if err = loadAudit(ctx, r.db, &s); err != nil {
		return s, err
	}
	r.cacheMu.Lock()
	r.snapshots[id] = s.Clone()
	r.cacheMu.Unlock()
	return s, nil
}

func (r *Repository) List(ctx context.Context) ([]domain.InterviewDossier, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT dossier_id,title,participant_name,interviewer_name,session_date,material_summary,consent_scope,status,version,created_at,updated_at FROM dossiers ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.InterviewDossier
	for rows.Next() {
		var d domain.InterviewDossier
		var consent, status, created, updated string
		if err = rows.Scan(&d.DossierID, &d.Title, &d.ParticipantName, &d.InterviewerName, &d.SessionDate, &d.MaterialSummary, &consent, &status, &d.Version, &created, &updated); err != nil {
			return nil, err
		}
		d.ConsentScope = domain.AccessLevel(consent)
		d.Status = domain.DossierStatus(status)
		d.CreatedAt = parseTime(created)
		d.UpdatedAt = parseTime(updated)
		out = append(out, d)
	}
	return out, rows.Err()
}
