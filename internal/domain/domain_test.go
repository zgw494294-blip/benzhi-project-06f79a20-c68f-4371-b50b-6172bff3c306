package domain

import (
	"errors"
	"testing"
	"time"
)

func TestDossierStateMachineRejectsSkippingAndSealedMutation(t *testing.T) {
	now := time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC)
	d, err := NewDossier(CreateDossierInput{DossierID: "d1", Title: "访谈", ParticipantName: "甲", InterviewerName: "乙", SessionDate: "2026-08-27", MaterialSummary: "摘要", ConsentScope: AccessPublic, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err = d.Transition(StatusParticipantReview, now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("跳过待标注应失败，得到 %v", err)
	}
	for _, target := range []DossierStatus{StatusPendingAnnotate, StatusParticipantReview, StatusReadyApproval, StatusSealed} {
		if err = d.Transition(target, now); err != nil {
			t.Fatalf("迁移到 %s: %v", target, err)
		}
	}
	if err = d.Transition(StatusDraft, now); !errors.Is(err, ErrSealed) {
		t.Fatalf("封存修改应失败，得到 %v", err)
	}
}

func TestPrepareSegmentsStableDigestAndDiff(t *testing.T) {
	input := []TranscriptSegment{{SegmentID: "S001", Sequence: 1, Text: "第一段"}, {SegmentID: "S002", Sequence: 2, Text: "第二段"}}
	first, digest1, err := PrepareSegments("r1", input)
	if err != nil {
		t.Fatal(err)
	}
	_, digest2, err := PrepareSegments("r2", input)
	if err != nil {
		t.Fatal(err)
	}
	if digest1 != digest2 {
		t.Fatalf("修订 ID 不应影响内容摘要")
	}
	changed := append([]TranscriptSegment(nil), input...)
	changed[1].Text = "第二段修订"
	second, digest3, err := PrepareSegments("r3", changed)
	if err != nil {
		t.Fatal(err)
	}
	if digest1 == digest3 {
		t.Fatal("正文变化后摘要未变化")
	}
	diffs := CompareSegments(first, second)
	if len(diffs) != 1 || diffs[0].SegmentID != "S002" || diffs[0].Kind != "changed" {
		t.Fatalf("差异不正确: %#v", diffs)
	}
}

func TestManifestConsentAndIntegrity(t *testing.T) {
	now := time.Date(2026, 8, 27, 2, 0, 0, 0, time.UTC)
	d := InterviewDossier{DossierID: "d1", ParticipantName: "甲", ConsentScope: AccessResearcher, Status: StatusReadyApproval}
	r := TranscriptRevision{RevisionID: "r1", DossierID: "d1"}
	segments, digest, err := PrepareSegments("r1", []TranscriptSegment{{SegmentID: "S001", Sequence: 1, Text: "正文"}})
	if err != nil {
		t.Fatal(err)
	}
	segments[0].ProposedAccessLevel = AccessPublic
	r.ContentDigest = digest
	if _, err = BuildManifest("m1", d, r, segments, nil, now); !errors.Is(err, ErrConsentMismatch) {
		t.Fatalf("超出授权应失败: %v", err)
	}
	segments[0].ProposedAccessLevel = AccessResearcher
	m, err := BuildManifest("m1", d, r, segments, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Verify(); err != nil {
		t.Fatal(err)
	}
	m.SegmentEntries[0].SegmentDigest = "tampered"
	if err = m.Verify(); err == nil {
		t.Fatal("篡改清单应校验失败")
	}
}

func TestAccessEmbargo(t *testing.T) {
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	if AccessPublic.Allows(AccessResearcher, nil, now) {
		t.Fatal("公开访问者不应看到研究者段落")
	}
	if !AccessResearcher.Allows(AccessResearcher, nil, now) {
		t.Fatal("研究者应看到研究者段落")
	}
	if AccessResearcher.Allows(AccessPublic, &future, now) {
		t.Fatal("延迟开放期内研究者不应看到段落")
	}
	if !AccessRestricted.Allows(AccessPublic, &future, now) {
		t.Fatal("受限阅览员应能审核延迟段落")
	}
}
