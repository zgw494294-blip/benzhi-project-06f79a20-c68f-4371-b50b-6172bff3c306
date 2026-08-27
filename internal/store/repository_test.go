package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

func TestRepositoryRoundTripAndOptimisticVersion(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	dossier := domain.InterviewDossier{DossierID: "d-store", Title: "档案", ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-27", MaterialSummary: "摘要", ConsentScope: domain.AccessPublic, Status: domain.StatusPendingAnnotate, Version: 1, CreatedAt: now, UpdatedAt: now}
	segments, digest, err := domain.PrepareSegments("r1", []domain.TranscriptSegment{{SegmentID: "S001", Sequence: 1, Text: "正文"}})
	if err != nil {
		t.Fatal(err)
	}
	revision := domain.TranscriptRevision{RevisionID: "r1", DossierID: dossier.DossierID, RevisionNumber: 1, SourceDigest: "source", ContentDigest: digest, SubmittedBy: "乙", SubmittedAt: now}
	snap := Snapshot{Dossier: dossier, Revisions: []domain.TranscriptRevision{revision}, Segments: map[string][]domain.TranscriptSegment{"r1": segments}}
	if err = repo.Create(ctx, snap); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.Get(ctx, dossier.DossierID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CurrentSegments()[0].Text != "正文" {
		t.Fatalf("重建正文错误: %#v", loaded)
	}
	loaded.Dossier.Version = 2
	loaded.Dossier.Status = domain.StatusParticipantReview
	if err = repo.Save(ctx, loaded, 99); !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("过期版本应冲突: %v", err)
	}
	if err = repo.Save(ctx, loaded, 1); err != nil {
		t.Fatal(err)
	}
	again, err := repo.Get(ctx, dossier.DossierID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Dossier.Version != 2 || again.Dossier.Status != domain.StatusParticipantReview {
		t.Fatalf("更新未保存: %#v", again.Dossier)
	}
}

func TestMigrationIsRepeatable(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/test.db"
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var version int
	if err = second.db.QueryRowContext(ctx, "SELECT schema_version FROM schema_meta").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("schemaVersion=%d", version)
	}
}
