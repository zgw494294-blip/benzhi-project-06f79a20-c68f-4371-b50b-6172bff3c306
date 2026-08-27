package audit

import (
	"fmt"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

type TimelineEntry struct {
	Sequence     int64                `json:"sequence"`
	EventType    string               `json:"eventType"`
	Title        string               `json:"title"`
	Actor        string               `json:"actor"`
	Reason       string               `json:"reason"`
	BeforeStatus domain.DossierStatus `json:"beforeStatus"`
	AfterStatus  domain.DossierStatus `json:"afterStatus"`
	OccurredAt   time.Time            `json:"occurredAt"`
	Digest       string               `json:"digest"`
}

type Timeline struct {
	DossierID string          `json:"dossierId"`
	Entries   []TimelineEntry `json:"entries"`
	Count     int             `json:"count"`
	Head      string          `json:"headDigest"`
	Valid     bool            `json:"valid"`
}

var eventTitles = map[string]string{
	EventDossierCreated: "建立访谈档案", EventTranscriptRevised: "提交转写修订",
	EventDossierRevised:    "校订草稿档案信息",
	EventSegmentsAnnotated: "完成敏感标注", EventReviewConfirmed: "参与者确认",
	EventReviewRequested: "参与者提出修订请求", EventReviewResolved: "编研员处理异议",
	EventReviewDraftSaved: "保存参与者审阅草稿",
	EventDossierSealed:    "封存授权清单", EventDossierReleased: "生成分级阅览副本",
}

func BuildTimeline(events []domain.AuditEvent) (Timeline, error) {
	verified, err := Verify(events)
	if err != nil {
		return Timeline{}, err
	}
	timeline := Timeline{Count: verified.Count, Head: verified.HeadDigest, Valid: verified.Valid}
	if len(events) == 0 {
		return timeline, nil
	}
	timeline.DossierID = events[0].DossierID
	timeline.Entries = make([]TimelineEntry, 0, len(events))
	for _, event := range events {
		if event.DossierID != timeline.DossierID {
			return Timeline{}, fmt.Errorf("审计时间线混入其他档案事件")
		}
		title := eventTitles[event.EventType]
		if title == "" {
			title = event.EventType
		}
		timeline.Entries = append(timeline.Entries, TimelineEntry{
			Sequence: event.Sequence, EventType: event.EventType, Title: title,
			Actor: event.Actor, Reason: event.Reason, BeforeStatus: event.BeforeStatus,
			AfterStatus: event.AfterStatus, OccurredAt: event.OccurredAt, Digest: event.EventDigest,
		})
	}
	return timeline, nil
}
