package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func parseTime(v string) time.Time { t, _ := time.Parse(time.RFC3339Nano, v); return t }
func parseNullable(v sql.NullString) *time.Time {
	if !v.Valid {
		return nil
	}
	t := parseTime(v.String)
	return &t
}

func loadRevisions(ctx context.Context, q queryer, s *Snapshot) error {
	rows, err := q.QueryContext(ctx, `SELECT revision_id,dossier_id,revision_number,source_digest,content_digest,submitted_by,submitted_at,supersedes_revision_id FROM revisions WHERE dossier_id=? ORDER BY revision_number`, s.Dossier.DossierID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var r domain.TranscriptRevision
		var submitted string
		if err = rows.Scan(&r.RevisionID, &r.DossierID, &r.RevisionNumber, &r.SourceDigest, &r.ContentDigest, &r.SubmittedBy, &submitted, &r.SupersedesRevisionID); err != nil {
			rows.Close()
			return err
		}
		r.SubmittedAt = parseTime(submitted)
		s.Revisions = append(s.Revisions, r)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, revision := range s.Revisions {
		if err = loadSegments(ctx, q, s, revision.RevisionID); err != nil {
			return err
		}
	}
	return nil
}

func loadSegments(ctx context.Context, q queryer, s *Snapshot, revisionID string) error {
	rows, err := q.QueryContext(ctx, `SELECT segment_id,revision_id,sequence,text,sensitivity_tags,proposed_access_level,embargo_until,segment_digest FROM segments WHERE revision_id=? ORDER BY sequence`, revisionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var segment domain.TranscriptSegment
		var tags, level string
		var embargo sql.NullString
		if err = rows.Scan(&segment.SegmentID, &segment.RevisionID, &segment.Sequence, &segment.Text, &tags, &level, &embargo, &segment.SegmentDigest); err != nil {
			return err
		}
		json.Unmarshal([]byte(tags), &segment.SensitivityTags)
		segment.ProposedAccessLevel = domain.AccessLevel(level)
		segment.EmbargoUntil = parseNullable(embargo)
		s.Segments[revisionID] = append(s.Segments[revisionID], segment)
	}
	return rows.Err()
}

func loadDecisions(ctx context.Context, q queryer, s *Snapshot) error {
	rows, err := q.QueryContext(ctx, `SELECT decision_id,dossier_id,revision_id,participant,segment_id,decision_type,requested_text,requested_access_level,reason,resolution,resolved_by,decided_at,resolved_at FROM review_decisions WHERE dossier_id=? ORDER BY decided_at,decision_id`, s.Dossier.DossierID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var d domain.ReviewDecision
		var kind, level, decided string
		var resolved sql.NullString
		if err = rows.Scan(&d.DecisionID, &d.DossierID, &d.RevisionID, &d.Participant, &d.SegmentID, &kind, &d.RequestedText, &level, &d.Reason, &d.Resolution, &d.ResolvedBy, &decided, &resolved); err != nil {
			return err
		}
		d.DecisionType = domain.DecisionType(kind)
		d.RequestedAccessLevel = domain.AccessLevel(level)
		d.DecidedAt = parseTime(decided)
		d.ResolvedAt = parseNullable(resolved)
		s.Decisions = append(s.Decisions, d)
	}
	return rows.Err()
}

func loadReviewDrafts(ctx context.Context, q queryer, s *Snapshot) error {
	rows, err := q.QueryContext(ctx, `SELECT dossier_id,revision_id,participant,decisions,saved_at FROM review_drafts WHERE dossier_id=? ORDER BY revision_id,participant`, s.Dossier.DossierID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var draft domain.ReviewDraft
		var decisions, savedAt string
		if err = rows.Scan(&draft.DossierID, &draft.RevisionID, &draft.Participant, &decisions, &savedAt); err != nil {
			return err
		}
		if err = json.Unmarshal([]byte(decisions), &draft.Decisions); err != nil {
			return err
		}
		draft.SavedAt = parseTime(savedAt)
		s.ReviewDrafts = append(s.ReviewDrafts, draft)
	}
	return rows.Err()
}

func loadManifest(ctx context.Context, q queryer, s *Snapshot) error {
	var m domain.ReleaseManifest
	var entries, sealed string
	var released sql.NullString
	err := q.QueryRowContext(ctx, `SELECT manifest_id,dossier_id,revision_id,consent_digest,decision_digest,segment_entries,manifest_digest,sealed_at,released_at FROM manifests WHERE dossier_id=?`, s.Dossier.DossierID).Scan(&m.ManifestID, &m.DossierID, &m.RevisionID, &m.ConsentDigest, &m.DecisionDigest, &entries, &m.ManifestDigest, &sealed, &released)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	json.Unmarshal([]byte(entries), &m.SegmentEntries)
	m.SealedAt = parseTime(sealed)
	m.ReleasedAt = parseNullable(released)
	s.Manifest = &m
	return nil
}

func loadAudit(ctx context.Context, q queryer, s *Snapshot) error {
	rows, err := q.QueryContext(ctx, `SELECT event_id,dossier_id,sequence,event_type,actor,reason,before_status,after_status,occurred_at,previous_digest,event_digest FROM audit_events WHERE dossier_id=? ORDER BY sequence`, s.Dossier.DossierID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e domain.AuditEvent
		var before, after, occurred string
		if err = rows.Scan(&e.EventID, &e.DossierID, &e.Sequence, &e.EventType, &e.Actor, &e.Reason, &before, &after, &occurred, &e.PreviousDigest, &e.EventDigest); err != nil {
			return err
		}
		e.BeforeStatus = domain.DossierStatus(before)
		e.AfterStatus = domain.DossierStatus(after)
		e.OccurredAt = parseTime(occurred)
		s.AuditEvents = append(s.AuditEvents, e)
	}
	return rows.Err()
}
