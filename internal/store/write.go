package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

func timeString(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return timeString(*t)
}

func insertDossier(ctx context.Context, tx *sql.Tx, d domain.InterviewDossier) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO dossiers(dossier_id,title,participant_name,interviewer_name,session_date,material_summary,consent_scope,status,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, d.DossierID, d.Title, d.ParticipantName, d.InterviewerName, d.SessionDate, d.MaterialSummary, string(d.ConsentScope), string(d.Status), d.Version, timeString(d.CreatedAt), timeString(d.UpdatedAt))
	return err
}

func writeChildren(ctx context.Context, tx *sql.Tx, s Snapshot) error {
	for _, r := range s.Revisions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO revisions VALUES(?,?,?,?,?,?,?,?)`, r.RevisionID, r.DossierID, r.RevisionNumber, r.SourceDigest, r.ContentDigest, r.SubmittedBy, timeString(r.SubmittedAt), r.SupersedesRevisionID); err != nil {
			return err
		}
		for _, segment := range s.Segments[r.RevisionID] {
			tags, _ := json.Marshal(segment.SensitivityTags)
			if _, err := tx.ExecContext(ctx, `INSERT INTO segments VALUES(?,?,?,?,?,?,?,?)`, segment.SegmentID, segment.RevisionID, segment.Sequence, segment.Text, string(tags), string(segment.ProposedAccessLevel), nullableTime(segment.EmbargoUntil), segment.SegmentDigest); err != nil {
				return err
			}
		}
	}
	for _, d := range s.Decisions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO review_decisions(decision_id,dossier_id,segment_id,decision_type,requested_text,requested_access_level,reason,resolution,resolved_by,decided_at,resolved_at,revision_id,participant) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, d.DecisionID, d.DossierID, d.SegmentID, string(d.DecisionType), d.RequestedText, string(d.RequestedAccessLevel), d.Reason, d.Resolution, d.ResolvedBy, timeString(d.DecidedAt), nullableTime(d.ResolvedAt), d.RevisionID, d.Participant); err != nil {
			return err
		}
	}
	for _, draft := range s.ReviewDrafts {
		decisions, err := json.Marshal(draft.Decisions)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO review_drafts(dossier_id,revision_id,participant,decisions,saved_at) VALUES(?,?,?,?,?)`, draft.DossierID, draft.RevisionID, draft.Participant, string(decisions), timeString(draft.SavedAt)); err != nil {
			return err
		}
	}
	if s.Manifest != nil {
		entries, _ := json.Marshal(s.Manifest.SegmentEntries)
		if _, err := tx.ExecContext(ctx, `INSERT INTO manifests VALUES(?,?,?,?,?,?,?,?,?)`, s.Manifest.ManifestID, s.Manifest.DossierID, s.Manifest.RevisionID, s.Manifest.ConsentDigest, s.Manifest.DecisionDigest, string(entries), s.Manifest.ManifestDigest, timeString(s.Manifest.SealedAt), nullableTime(s.Manifest.ReleasedAt)); err != nil {
			return err
		}
	}
	for _, e := range s.AuditEvents {
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events VALUES(?,?,?,?,?,?,?,?,?,?,?)`, e.EventID, e.DossierID, e.Sequence, e.EventType, e.Actor, e.Reason, string(e.BeforeStatus), string(e.AfterStatus), timeString(e.OccurredAt), e.PreviousDigest, e.EventDigest); err != nil {
			return err
		}
	}
	return nil
}
