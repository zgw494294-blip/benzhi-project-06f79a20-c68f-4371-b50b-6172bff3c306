package audit

import (
	"fmt"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

const (
	EventDossierCreated    = "dossier.created"
	EventDossierRevised    = "dossier.revised"
	EventTranscriptRevised = "transcript.revised"
	EventSegmentsAnnotated = "segments.annotated"
	EventReviewConfirmed   = "review.confirmed"
	EventReviewRequested   = "review.requested"
	EventReviewResolved    = "review.resolved"
	EventReviewDraftSaved  = "review.draft_saved"
	EventDossierSealed     = "dossier.sealed"
	EventDossierReleased   = "dossier.released"
)

var knownEventTypes = map[string]bool{
	EventDossierCreated: true, EventTranscriptRevised: true,
	EventDossierRevised:    true,
	EventSegmentsAnnotated: true, EventReviewConfirmed: true,
	EventReviewRequested: true, EventReviewResolved: true,
	EventReviewDraftSaved: true,
	EventDossierSealed:    true, EventDossierReleased: true,
}

// Chain 封装单一档案按顺序连接的审计事件，并在追加前验证已有历史。
type Chain struct {
	dossierID string
	events    []domain.AuditEvent
}

func NewChain(dossierID string, events []domain.AuditEvent) (*Chain, error) {
	if dossierID == "" {
		return nil, domain.Invalid("audit_dossier_required", "审计链必须指定档案", "dossierId")
	}
	copyEvents := append([]domain.AuditEvent(nil), events...)
	for _, event := range copyEvents {
		if event.DossierID != dossierID {
			return nil, fmt.Errorf("审计事件 %s 不属于档案 %s", event.EventID, dossierID)
		}
	}
	if _, err := Verify(copyEvents); err != nil {
		return nil, err
	}
	return &Chain{dossierID: dossierID, events: copyEvents}, nil
}

func (c *Chain) Append(builder *Builder, change Change) (domain.AuditEvent, error) {
	if change.DossierID != c.dossierID {
		return domain.AuditEvent{}, domain.Invalid("audit_dossier_mismatch", "审计变更属于其他档案", "dossierId")
	}
	event, err := builder.Next(c.events, change)
	if err != nil {
		return domain.AuditEvent{}, err
	}
	c.events = append(c.events, event)
	return event, nil
}

func (c *Chain) Events() []domain.AuditEvent {
	return append([]domain.AuditEvent(nil), c.events...)
}

func (c *Chain) Head() string {
	if len(c.events) == 0 {
		return ""
	}
	return c.events[len(c.events)-1].EventDigest
}

func IsKnownEventType(eventType string) bool { return knownEventTypes[eventType] }
