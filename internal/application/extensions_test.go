package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
)

func extensionService(t *testing.T) (context.Context, *store.Repository, *Service) {
	t.Helper()
	ctx := context.Background()
	repo, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	return ctx, repo, NewService(repo)
}

func TestDraftDossierCorrectionIsVersionedAndAtomic(t *testing.T) {
	ctx, _, service := extensionService(t)
	view, err := service.CreateDossier(ctx, CreateDossierCommand{Title: "原标题", ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-20", MaterialSummary: "原摘要", ConsentScope: domain.AccessPublic, Actor: "乙"})
	if err != nil {
		t.Fatal(err)
	}
	id, createdAt, oldVersion := view.Snapshot.Dossier.DossierID, view.Snapshot.Dossier.CreatedAt, view.Snapshot.Dossier.Version
	view, err = service.ReviseDossier(ctx, id, ReviseDossierCommand{Version: oldVersion, Title: "原标题", ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-21", MaterialSummary: "校订摘要", ConsentScope: domain.AccessResearcher, Actor: "乙", Reason: "纠正建档笔误"})
	if err != nil {
		t.Fatal(err)
	}
	if view.Snapshot.Dossier.Version != oldVersion+1 || view.Snapshot.Dossier.CreatedAt != createdAt || view.NextTodo != "提交首个转写修订" {
		t.Fatalf("校订投影错误: %#v", view)
	}
	last := view.Snapshot.AuditEvents[len(view.Snapshot.AuditEvents)-1]
	if !strings.Contains(last.Reason, "sessionDate") || !strings.Contains(last.Reason, "materialSummary") || !strings.Contains(last.Reason, "consentScope") {
		t.Fatalf("审计未记录变更字段: %s", last.Reason)
	}
	_, err = service.ReviseDossier(ctx, id, ReviseDossierCommand{Version: oldVersion, Title: "覆盖标题", ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-21", MaterialSummary: "校订摘要", ConsentScope: domain.AccessResearcher, Actor: "乙", Reason: "过期覆盖"})
	if !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("应拒绝过期版本: %v", err)
	}
	unchanged, _ := service.GetDossier(ctx, id)
	if unchanged.Snapshot.Dossier.Title != "原标题" || unchanged.Snapshot.Dossier.Version != oldVersion+1 || len(unchanged.Snapshot.AuditEvents) != 2 {
		t.Fatalf("冲突请求改写了档案: %#v", unchanged.Snapshot)
	}
	_, err = service.ReviseDossier(ctx, id, ReviseDossierCommand{Version: unchanged.Snapshot.Dossier.Version, Title: "原标题", ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-21", MaterialSummary: "校订摘要", ConsentScope: domain.AccessResearcher, Actor: "乙", Reason: "没有变化"})
	var rule *domain.RuleError
	if !errors.As(err, &rule) || rule.Code != "dossier_unchanged" {
		t.Fatalf("应拒绝无变化校订: %v", err)
	}
	unchanged, err = service.SubmitRevision(ctx, id, SubmitRevisionCommand{Version: unchanged.Snapshot.Dossier.Version, SourceDigest: "source", SubmittedBy: "乙", Segments: []SegmentInput{{SegmentID: "S1", Sequence: 1, Text: "正文"}}})
	if err != nil {
		t.Fatal(err)
	}
	versionAfterRevision, auditAfterRevision := unchanged.Snapshot.Dossier.Version, len(unchanged.Snapshot.AuditEvents)
	_, err = service.ReviseDossier(ctx, id, ReviseDossierCommand{Version: versionAfterRevision, Title: "流程后修改", ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-21", MaterialSummary: "校订摘要", ConsentScope: domain.AccessResearcher, Actor: "乙", Reason: "不应允许"})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("进入转写流程后应拒绝校订: %v", err)
	}
	afterRejected, _ := service.GetDossier(ctx, id)
	if afterRejected.Snapshot.Dossier.Version != versionAfterRevision || len(afterRejected.Snapshot.AuditEvents) != auditAfterRevision || afterRejected.Snapshot.Dossier.Title != "原标题" {
		t.Fatalf("状态冲突请求改写了档案: %#v", afterRejected.Snapshot)
	}
}

func TestRevisionHistoryVerifiesChainAndDeterministicThreeWayDiff(t *testing.T) {
	ctx, repo, service := extensionService(t)
	now := time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC)
	dossier := domain.InterviewDossier{DossierID: "history-ok", Title: "履历", ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-20", MaterialSummary: "摘要", ConsentScope: domain.AccessPublic, Status: domain.StatusPendingAnnotate, Version: 1, CreatedAt: now, UpdatedAt: now}
	revisions := make([]domain.TranscriptRevision, 0, 3)
	segmentsByRevision := map[string][]domain.TranscriptSegment{}
	inputs := [][]domain.TranscriptSegment{
		{{SegmentID: "S001", Sequence: 1, Text: "第一段"}, {SegmentID: "S002", Sequence: 2, Text: "将移除"}},
		{{SegmentID: "S001", Sequence: 1, Text: "中间版本"}, {SegmentID: "S002", Sequence: 2, Text: "将移除"}},
		{{SegmentID: "S001", Sequence: 1, Text: "最终版本"}, {SegmentID: "S003", Sequence: 2, Text: "新增段落"}},
	}
	for i, input := range inputs {
		id := "r" + fmtInt(i+1)
		segments, digest, err := domain.PrepareSegments(id, input)
		if err != nil {
			t.Fatal(err)
		}
		supersedes := ""
		if i > 0 {
			supersedes = revisions[i-1].RevisionID
		}
		revisions = append(revisions, domain.TranscriptRevision{RevisionID: id, DossierID: dossier.DossierID, RevisionNumber: i + 1, SourceDigest: "source-" + id, ContentDigest: digest, SubmittedBy: "乙", SubmittedAt: now.Add(time.Duration(i) * time.Hour), SupersedesRevisionID: supersedes})
		segmentsByRevision[id] = segments
	}
	if err := repo.Create(ctx, store.Snapshot{Dossier: dossier, Revisions: revisions, Segments: segmentsByRevision}); err != nil {
		t.Fatal(err)
	}
	first, err := service.RevisionHistory(ctx, dossier.DossierID, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.RevisionHistory(ctx, dossier.DossierID, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Verified || len(first.History) != 3 || first.Comparison == nil || len(first.Comparison.Diffs) != 3 {
		t.Fatalf("履历核验结果错误: %#v", first)
	}
	for i, kind := range []string{"changed", "removed", "added"} {
		if first.Comparison.Diffs[i].Kind != kind {
			t.Fatalf("差异顺序不确定: %#v", first.Comparison.Diffs)
		}
		if first.Comparison.Diffs[i] != second.Comparison.Diffs[i] {
			t.Fatal("重复查询结果不一致")
		}
	}
	corrupt := dossier
	corrupt.DossierID = "history-corrupt"
	corruptSegments, digest, _ := domain.PrepareSegments("bad-r1", []domain.TranscriptSegment{{SegmentID: "S001", Sequence: 1, Text: "原正文"}})
	corruptSegments[0].Text = "损坏正文"
	badRevision := domain.TranscriptRevision{RevisionID: "bad-r1", DossierID: corrupt.DossierID, RevisionNumber: 1, SourceDigest: "source", ContentDigest: digest, SubmittedBy: "乙", SubmittedAt: now}
	if err = repo.Create(ctx, store.Snapshot{Dossier: corrupt, Revisions: []domain.TranscriptRevision{badRevision}, Segments: map[string][]domain.TranscriptSegment{"bad-r1": corruptSegments}}); err != nil {
		t.Fatal(err)
	}
	_, err = service.RevisionHistory(ctx, corrupt.DossierID, 0, 0)
	if !errors.Is(err, domain.ErrIntegrity) || !strings.Contains(err.Error(), "修订 1") {
		t.Fatalf("损坏摘要应返回指向修订的完整性错误: %v", err)
	}
}

func pendingReview(t *testing.T, ctx context.Context, service *Service, title string, segmentCount int) DossierView {
	t.Helper()
	view, err := service.CreateDossier(ctx, CreateDossierCommand{Title: title, ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-20", MaterialSummary: "摘要", ConsentScope: domain.AccessPublic, Actor: "乙"})
	if err != nil {
		t.Fatal(err)
	}
	segments := make([]SegmentInput, segmentCount)
	annotations := make([]AnnotationInput, segmentCount)
	for i := range segments {
		id := "S" + fmtInt(i+1)
		segments[i] = SegmentInput{SegmentID: id, Sequence: i + 1, Text: "正文" + id}
		annotations[i] = AnnotationInput{SegmentID: id, ProposedAccessLevel: domain.AccessPublic}
	}
	view, err = service.SubmitRevision(ctx, view.Snapshot.Dossier.DossierID, SubmitRevisionCommand{Version: view.Snapshot.Dossier.Version, SourceDigest: "source", SubmittedBy: "乙", Segments: segments})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.Annotate(ctx, view.Snapshot.Dossier.DossierID, AnnotateCommand{Version: view.Snapshot.Dossier.Version, Actor: "乙", Annotations: annotations})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestAnnotationPreflightReportsIndependentIssuesWithoutWriting(t *testing.T) {
	ctx, _, service := extensionService(t)
	fixed := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }
	view, err := service.CreateDossier(ctx, CreateDossierCommand{Title: "标注", ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-20", MaterialSummary: "摘要", ConsentScope: domain.AccessResearcher, Actor: "乙"})
	if err != nil {
		t.Fatal(err)
	}
	id := view.Snapshot.Dossier.DossierID
	view, err = service.SubmitRevision(ctx, id, SubmitRevisionCommand{Version: view.Snapshot.Dossier.Version, SourceDigest: "source", SubmittedBy: "乙", Segments: []SegmentInput{{"S1", 1, "甲"}, {"S2", 2, "乙"}}})
	if err != nil {
		t.Fatal(err)
	}
	past := fixed.Add(-24 * time.Hour)
	bad := AnnotateCommand{Version: view.Snapshot.Dossier.Version, Actor: "乙", Annotations: []AnnotationInput{{SegmentID: "S1", ProposedAccessLevel: domain.AccessPublic}, {SegmentID: "S2", ProposedAccessLevel: domain.AccessResearcher, EmbargoUntil: &past}, {SegmentID: "S2", ProposedAccessLevel: domain.AccessResearcher}}}
	preflight, err := service.AnnotationPreflight(ctx, id, bad)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.CanSubmit || len(preflight.Issues) != 3 {
		t.Fatalf("应分别报告授权、日期和重复问题: %#v", preflight.Issues)
	}
	before := view.Snapshot.Dossier.Version
	_, err = service.Annotate(ctx, id, bad)
	if err == nil {
		t.Fatal("有阻断项的标注不应提交")
	}
	unchanged, _ := service.GetDossier(ctx, id)
	if unchanged.Snapshot.Dossier.Version != before || unchanged.Snapshot.Dossier.Status != domain.StatusPendingAnnotate || len(unchanged.Snapshot.AuditEvents) != 2 {
		t.Fatalf("失败预检产生了写入: %#v", unchanged.Snapshot)
	}
}

func TestReviewDraftRestoresMergesAndRejectsOldRevision(t *testing.T) {
	ctx, repo, service := extensionService(t)
	view := pendingReview(t, ctx, service, "断点续审", 3)
	id, revisionID := view.Snapshot.Dossier.DossierID, view.CurrentRevision.RevisionID
	view, err := service.SaveReviewDraft(ctx, id, SaveReviewDraftCommand{Version: view.Snapshot.Dossier.Version, RevisionID: revisionID, Participant: "甲", Decisions: []DecisionInput{{SegmentID: "S1", DecisionType: domain.DecisionConfirm}}})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := service.GetDossier(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ReviewDraft == nil || reopened.ReviewDraft.Completed != 1 || len(reopened.ReviewDraft.Remaining) != 2 {
		t.Fatalf("草稿恢复投影错误: %#v", reopened.ReviewDraft)
	}
	view, err = service.Review(ctx, id, ReviewCommand{Version: reopened.Snapshot.Dossier.Version, RevisionID: revisionID, Participant: "甲", Decisions: []DecisionInput{{SegmentID: "S2", DecisionType: domain.DecisionConfirm}, {SegmentID: "S3", DecisionType: domain.DecisionConfirm}}})
	if err != nil {
		t.Fatal(err)
	}
	if view.Snapshot.Dossier.Status != domain.StatusReadyApproval || view.ReviewDraft != nil || len(view.Snapshot.Decisions) != 3 {
		t.Fatalf("草稿合并提交错误: %#v", view)
	}

	stale := pendingReview(t, ctx, service, "旧修订草稿", 2)
	staleID, oldRevision := stale.Snapshot.Dossier.DossierID, stale.CurrentRevision.RevisionID
	stale, err = service.SaveReviewDraft(ctx, staleID, SaveReviewDraftCommand{Version: stale.Snapshot.Dossier.Version, RevisionID: oldRevision, Participant: "甲", Decisions: []DecisionInput{{SegmentID: "S1", DecisionType: domain.DecisionConfirm}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := repo.Get(ctx, staleID)
	current := snapshot.CurrentSegments()
	newRevisionID := "new-current-revision"
	prepared, digest, err := domain.PrepareSegments(newRevisionID, current)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Revisions = append(snapshot.Revisions, domain.TranscriptRevision{RevisionID: newRevisionID, DossierID: staleID, RevisionNumber: 2, SourceDigest: stale.CurrentRevision.ContentDigest, ContentDigest: digest, SubmittedBy: "编研员", SubmittedAt: time.Now().UTC(), SupersedesRevisionID: oldRevision})
	snapshot.Segments[newRevisionID] = prepared
	expected := snapshot.Dossier.Version
	snapshot.Dossier.Version++
	if err = repo.Save(ctx, snapshot, expected); err != nil {
		t.Fatal(err)
	}
	_, err = service.SaveReviewDraft(ctx, staleID, SaveReviewDraftCommand{Version: expected + 1, RevisionID: oldRevision, Participant: "甲", Decisions: []DecisionInput{{SegmentID: "S2", DecisionType: domain.DecisionConfirm}}})
	if !errors.Is(err, domain.ErrRevisionStale) {
		t.Fatalf("旧修订草稿应被拒绝: %v", err)
	}
	stored, _ := repo.Get(ctx, staleID)
	if len(stored.ReviewDrafts) != 1 || len(stored.ReviewDrafts[0].Decisions) != 1 || len(stored.Decisions) != 0 || stored.Dossier.Status != domain.StatusParticipantReview {
		t.Fatalf("旧草稿失败请求污染了聚合: %#v", stored)
	}
}

func TestSealPreflightDigestExpiresWithoutPartialSeal(t *testing.T) {
	ctx, repo, service := extensionService(t)
	view := pendingReview(t, ctx, service, "封存预检", 2)
	id := view.Snapshot.Dossier.DossierID
	view, err := service.Review(ctx, id, ReviewCommand{Version: view.Snapshot.Dossier.Version, RevisionID: view.CurrentRevision.RevisionID, Participant: "甲", Decisions: []DecisionInput{{SegmentID: "S1", DecisionType: domain.DecisionConfirm}, {SegmentID: "S2", DecisionType: domain.DecisionConfirm}}})
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := service.SealPreflight(ctx, id)
	if err != nil || !preflight.CanSeal || len(preflight.Visibility) != 4 || preflight.ConfirmationDigest == "" {
		t.Fatalf("封存预检错误: %v %#v", err, preflight)
	}
	snapshot, _ := repo.Get(ctx, id)
	expected := snapshot.Dossier.Version
	snapshot.Dossier.MaterialSummary = "预检后发生变化"
	snapshot.Dossier.Version++
	if err = repo.Save(ctx, snapshot, expected); err != nil {
		t.Fatal(err)
	}
	_, err = service.Seal(ctx, id, SealCommand{Version: preflight.Version, ConfirmationDigest: preflight.ConfirmationDigest, Actor: "编研员", Reason: "核验完成"})
	if !errors.Is(err, domain.ErrPreflightStale) {
		t.Fatalf("旧预检摘要应被拒绝: %v", err)
	}
	unchanged, _ := repo.Get(ctx, id)
	if unchanged.Manifest != nil || unchanged.Dossier.Status != domain.StatusReadyApproval || len(unchanged.AuditEvents) != len(snapshot.AuditEvents) {
		t.Fatalf("过期预检产生半封存: %#v", unchanged)
	}
	fresh, err := service.SealPreflight(ctx, id)
	if err != nil || !fresh.CanSeal {
		t.Fatalf("重新预检失败: %v %#v", err, fresh.Blockers)
	}
	sealed, err := service.Seal(ctx, id, SealCommand{Version: fresh.Version, ConfirmationDigest: fresh.ConfirmationDigest, Actor: "编研员", Reason: "重新核验完成"})
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Snapshot.Manifest == nil || sealed.Snapshot.Manifest.ManifestDigest != fresh.ManifestDigest || sealed.Snapshot.Dossier.Status != domain.StatusReleased {
		t.Fatalf("正式清单与预检不一致: %#v", sealed.Snapshot)
	}
}
