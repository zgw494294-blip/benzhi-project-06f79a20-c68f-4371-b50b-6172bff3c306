package split_save_atomicity_test

import (
	"context"
	"testing"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
)

func TestFailedChildRewriteRollsBackDossierVersion(t *testing.T) {
	ctx := context.Background()
	repo, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repo.Close()

	now := time.Date(2026, 8, 27, 11, 0, 0, 0, time.UTC)
	original := store.Snapshot{
		Dossier: domain.InterviewDossier{
			DossierID:       "dossier-atomic-save",
			Title:           "提交前标题",
			ParticipantName: "参与者",
			InterviewerName: "访谈员",
			SessionDate:     "2026-08-27",
			MaterialSummary: "提交前摘要",
			ConsentScope:    domain.AccessPublic,
			Status:          domain.StatusDraft,
			Version:         1,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		Segments: map[string][]domain.TranscriptSegment{},
	}
	if err := repo.Create(ctx, original); err != nil {
		t.Fatalf("create original snapshot: %v", err)
	}

	broken := original.Clone()
	broken.Dossier.Title = "不应提交的标题"
	broken.Dossier.Version = 2
	broken.Dossier.UpdatedAt = now.Add(time.Minute)
	duplicate := domain.TranscriptRevision{
		RevisionID:     "duplicate-revision",
		DossierID:      original.Dossier.DossierID,
		RevisionNumber: 1,
		SourceDigest:   "source",
		ContentDigest:  domain.DigestText("content"),
		SubmittedBy:    "访谈员",
		SubmittedAt:    now,
	}
	broken.Revisions = []domain.TranscriptRevision{duplicate, duplicate}
	broken.Segments[duplicate.RevisionID] = nil

	if err := repo.Save(ctx, broken, original.Dossier.Version); err == nil {
		t.Fatal("duplicate child rewrite unexpectedly succeeded")
	}
	loaded, err := repo.Get(ctx, original.Dossier.DossierID)
	if err != nil {
		t.Fatalf("reload after failed Save: %v", err)
	}
	if loaded.Dossier.Version != original.Dossier.Version || loaded.Dossier.Title != original.Dossier.Title {
		t.Fatalf("failed Save committed dossier header: version=%d title=%q", loaded.Dossier.Version, loaded.Dossier.Title)
	}
}
