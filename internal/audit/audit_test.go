package audit

import (
	"testing"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

func TestAuditChainDetectsMissingAndTamperedEvents(t *testing.T) {
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	b := NewBuilder(func() time.Time { now = now.Add(time.Second); return now })
	var events []domain.AuditEvent
	for i, eventType := range []string{"dossier.created", "transcript.revised", "segments.annotated"} {
		e, err := b.Next(events, Change{EventID: eventType, DossierID: "d1", EventType: eventType, Actor: "测试员", Reason: "测试", BeforeStatus: domain.StatusDraft, AfterStatus: domain.StatusPendingAnnotate})
		if err != nil {
			t.Fatal(err)
		}
		if e.Sequence != int64(i+1) {
			t.Fatalf("序号错误: %d", e.Sequence)
		}
		events = append(events, e)
	}
	if result, err := Verify(events); err != nil || !result.Valid || result.Count != 3 {
		t.Fatalf("完整链验证失败: %#v %v", result, err)
	}
	missing := []domain.AuditEvent{events[0], events[2]}
	if _, err := Verify(missing); err == nil {
		t.Fatal("遗漏事件未被发现")
	}
	tampered := append([]domain.AuditEvent(nil), events...)
	tampered[1].Reason = "被改写"
	if _, err := Verify(tampered); err == nil {
		t.Fatal("事件改写未被发现")
	}
}
