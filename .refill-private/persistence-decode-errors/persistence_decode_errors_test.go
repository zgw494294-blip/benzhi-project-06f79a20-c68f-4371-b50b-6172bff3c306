package persistence_decode_errors_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
	_ "modernc.org/sqlite"
)

func createStoredSnapshot(t *testing.T, path, id string, withManifest bool) {
	t.Helper()
	ctx := context.Background()
	repo, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	dossier := domain.InterviewDossier{DossierID: id, Title: "持久化解码", ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-27", MaterialSummary: "摘要", ConsentScope: domain.AccessPublic, Status: domain.StatusPendingAnnotate, Version: 1, CreatedAt: now, UpdatedAt: now}
	segments, digest, err := domain.PrepareSegments("r1-"+id, []domain.TranscriptSegment{{SegmentID: "S1", Sequence: 1, Text: "正文"}})
	if err != nil {
		t.Fatal(err)
	}
	segments[0].ProposedAccessLevel = domain.AccessPublic
	revision := domain.TranscriptRevision{RevisionID: "r1-" + id, DossierID: id, RevisionNumber: 1, SourceDigest: "source", ContentDigest: digest, SubmittedBy: "乙", SubmittedAt: now}
	snapshot := store.Snapshot{Dossier: dossier, Revisions: []domain.TranscriptRevision{revision}, Segments: map[string][]domain.TranscriptSegment{revision.RevisionID: segments}}
	if withManifest {
		dossier.Status = domain.StatusReadyApproval
		manifest, buildErr := domain.BuildManifest("m1-"+id, dossier, revision, segments, nil, now)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		dossier.Status = domain.StatusReleased
		snapshot.Dossier, snapshot.Manifest = dossier, &manifest
	}
	if err = repo.Create(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	if err = repo.Close(); err != nil {
		t.Fatal(err)
	}
}

func corruptAndRead(t *testing.T, path, id, statement string) error {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(statement); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	repo, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	_, err = repo.Get(context.Background(), id)
	return err
}

func TestCorruptPersistedEncodingsAreRejected(t *testing.T) {
	t.Run("segment-json", func(t *testing.T) {
		path, id := t.TempDir()+"/segments.db", "bad-segments"
		createStoredSnapshot(t, path, id, false)
		if err := corruptAndRead(t, path, id, "UPDATE segments SET sensitivity_tags='{bad json'"); err == nil {
			t.Error("损坏的 sensitivity_tags JSON 被静默接受")
		}
	})
	t.Run("manifest-json", func(t *testing.T) {
		path, id := t.TempDir()+"/manifest.db", "bad-manifest"
		createStoredSnapshot(t, path, id, true)
		if err := corruptAndRead(t, path, id, "UPDATE manifests SET segment_entries='{bad json'"); err == nil {
			t.Error("损坏的 segment_entries JSON 被静默接受")
		}
	})
	t.Run("timestamp", func(t *testing.T) {
		path, id := t.TempDir()+"/time.db", "bad-time"
		createStoredSnapshot(t, path, id, false)
		if err := corruptAndRead(t, path, id, "UPDATE dossiers SET updated_at='not-a-time'"); err == nil {
			t.Error("损坏的持久化时间被转换为零值并静默接受")
		}
	})
}
