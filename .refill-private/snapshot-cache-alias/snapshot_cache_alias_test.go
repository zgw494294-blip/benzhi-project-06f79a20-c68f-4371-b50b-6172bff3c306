package snapshot_cache_alias_test

import (
	"context"
	"testing"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
)

func TestCachedSnapshotOwnsNestedSegmentState(t *testing.T) {
	ctx := context.Background()
	repo, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repo.Close()

	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	embargo := now.Add(72 * time.Hour)
	segments, digest, err := domain.PrepareSegments("revision-cache", []domain.TranscriptSegment{{
		SegmentID: "S001",
		Sequence:  1,
		Text:      "包含第三方姓名的访谈段落",
	}})
	if err != nil {
		t.Fatalf("prepare segments: %v", err)
	}
	segments[0].SensitivityTags = []string{"name"}
	segments[0].ProposedAccessLevel = domain.AccessResearcher
	segments[0].EmbargoUntil = &embargo
	snapshot := store.Snapshot{
		Dossier: domain.InterviewDossier{
			DossierID:       "dossier-cache-alias",
			Title:           "缓存所有权测试",
			ParticipantName: "参与者",
			InterviewerName: "访谈员",
			SessionDate:     "2026-08-27",
			MaterialSummary: "敏感段落摘要",
			ConsentScope:    domain.AccessResearcher,
			Status:          domain.StatusParticipantReview,
			Version:         3,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		Revisions: []domain.TranscriptRevision{{
			RevisionID:     "revision-cache",
			DossierID:      "dossier-cache-alias",
			RevisionNumber: 1,
			SourceDigest:   "source",
			ContentDigest:  digest,
			SubmittedBy:    "访谈员",
			SubmittedAt:    now,
		}},
		Segments: map[string][]domain.TranscriptSegment{"revision-cache": segments},
	}
	if err := repo.Create(ctx, snapshot); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	first, err := repo.Get(ctx, snapshot.Dossier.DossierID)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	firstSegment := &first.Segments["revision-cache"][0]
	firstSegment.SensitivityTags[0] = "polluted-without-save"
	pollutedEmbargo := embargo.Add(365 * 24 * time.Hour)
	*firstSegment.EmbargoUntil = pollutedEmbargo

	second, err := repo.Get(ctx, snapshot.Dossier.DossierID)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	secondSegment := second.Segments["revision-cache"][0]
	if got := secondSegment.SensitivityTags[0]; got != "name" {
		t.Fatalf("未保存的标签修改污染缓存：got %q", got)
	}
	if secondSegment.EmbargoUntil == nil || !secondSegment.EmbargoUntil.Equal(embargo) {
		t.Fatalf("未保存的延迟期修改污染缓存：got %v", secondSegment.EmbargoUntil)
	}
}
