package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
)

func TestCompleteNegotiationAndTieredRelease(t *testing.T) {
	ctx := context.Background()
	repo, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	service := NewService(repo)
	view, err := service.CreateDossier(ctx, CreateDossierCommand{Title: "厂史", ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-27", MaterialSummary: "社区记忆", ConsentScope: domain.AccessPublic, Actor: "乙"})
	if err != nil {
		t.Fatal(err)
	}
	id := view.Snapshot.Dossier.DossierID
	view, err = service.SubmitRevision(ctx, id, SubmitRevisionCommand{Version: view.Snapshot.Dossier.Version, SourceDigest: "source", SubmittedBy: "乙", Segments: []SegmentInput{{"S001", 1, "公开正文"}, {"S002", 2, "张三参与往事"}}})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.Annotate(ctx, id, AnnotateCommand{Version: view.Snapshot.Dossier.Version, Actor: "乙", Annotations: []AnnotationInput{{SegmentID: "S001", ProposedAccessLevel: domain.AccessPublic}, {SegmentID: "S002", SensitivityTags: []string{"name"}, ProposedAccessLevel: domain.AccessResearcher}}})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.Review(ctx, id, ReviewCommand{Version: view.Snapshot.Dossier.Version, RevisionID: view.CurrentRevision.RevisionID, Participant: "甲", Decisions: []DecisionInput{{SegmentID: "S001", DecisionType: domain.DecisionConfirm}, {SegmentID: "S002", DecisionType: domain.DecisionTextChange, RequestedText: "一位同事参与往事", Reason: "匿名处理"}}})
	if err != nil {
		t.Fatal(err)
	}
	if view.Snapshot.Dossier.Status != domain.StatusDispute {
		t.Fatalf("状态=%s", view.Snapshot.Dossier.Status)
	}
	var decisionID string
	for _, d := range view.Snapshot.Decisions {
		if d.DecisionType == domain.DecisionTextChange {
			decisionID = d.DecisionID
		}
	}
	view, err = service.Resolve(ctx, id, decisionID, ResolveCommand{Version: view.Snapshot.Dossier.Version, Actor: "编研员", Resolution: "接受", ReplacementText: "一位同事参与往事"})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.RevisionDiff) != 1 {
		t.Fatalf("修订差异=%#v", view.RevisionDiff)
	}
	view, err = service.Review(ctx, id, ReviewCommand{Version: view.Snapshot.Dossier.Version, RevisionID: view.CurrentRevision.RevisionID, Participant: "甲", Decisions: []DecisionInput{{SegmentID: "S001", DecisionType: domain.DecisionConfirm}, {SegmentID: "S002", DecisionType: domain.DecisionConfirm}}})
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := service.SealPreflight(ctx, id)
	if err != nil || !preflight.CanSeal {
		t.Fatalf("封存预检失败: %v %#v", err, preflight.Blockers)
	}
	view, err = service.Seal(ctx, id, SealCommand{Version: view.Snapshot.Dossier.Version, ConfirmationDigest: preflight.ConfirmationDigest, Actor: "编研员", Reason: "核验完成"})
	if err != nil {
		t.Fatal(err)
	}
	if view.Snapshot.Dossier.Status != domain.StatusReleased {
		t.Fatalf("状态=%s", view.Snapshot.Dossier.Status)
	}
	publicCopy, err := service.ReadingCopy(ctx, id, domain.AccessPublic)
	if err != nil {
		t.Fatal(err)
	}
	if len(publicCopy.Segments) != 1 || strings.Contains(publicCopy.Segments[0].Text, "同事") {
		t.Fatalf("公开副本泄露: %#v", publicCopy)
	}
	researchCopy, err := service.ReadingCopy(ctx, id, domain.AccessResearcher)
	if err != nil {
		t.Fatal(err)
	}
	if len(researchCopy.Segments) != 2 || !researchCopy.AuditValid {
		t.Fatalf("研究者副本错误: %#v", researchCopy)
	}
	_, err = service.SubmitRevision(ctx, id, SubmitRevisionCommand{Version: view.Snapshot.Dossier.Version, SourceDigest: "late", SubmittedBy: "乙", Segments: []SegmentInput{{"S001", 1, "修改"}}})
	if !errors.Is(err, domain.ErrSealed) {
		t.Fatalf("封存后修改应失败: %v", err)
	}
}

func TestVersionConflictIsActionable(t *testing.T) {
	ctx := context.Background()
	repo, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	service := NewService(repo)
	view, err := service.CreateDossier(ctx, CreateDossierCommand{Title: "档案", ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-27", MaterialSummary: "摘要", ConsentScope: domain.AccessPublic, Actor: "乙"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SubmitRevision(ctx, view.Snapshot.Dossier.DossierID, SubmitRevisionCommand{Version: 0, SourceDigest: "source", SubmittedBy: "乙", Segments: []SegmentInput{{"S001", 1, "正文"}}})
	if !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("应返回版本冲突: %v", err)
	}
}
