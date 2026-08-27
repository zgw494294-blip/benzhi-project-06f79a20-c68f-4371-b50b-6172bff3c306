package audit

import (
	"fmt"
	"strings"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

type Builder struct{ now func() time.Time }

func NewBuilder(now func() time.Time) *Builder {
	if now == nil {
		now = time.Now
	}
	return &Builder{now: now}
}

type Change struct {
	EventID, DossierID, EventType, Actor, Reason string
	BeforeStatus, AfterStatus                    domain.DossierStatus
}

func (b *Builder) Next(history []domain.AuditEvent, change Change) (domain.AuditEvent, error) {
	if strings.TrimSpace(change.EventID) == "" || strings.TrimSpace(change.DossierID) == "" {
		return domain.AuditEvent{}, domain.Invalid("audit_identity_required", "审计事件标识不完整", "eventId")
	}
	if strings.TrimSpace(change.EventType) == "" || strings.TrimSpace(change.Actor) == "" {
		return domain.AuditEvent{}, domain.Invalid("audit_actor_required", "审计事件类型和操作者不能为空", "actor")
	}
	if !IsKnownEventType(change.EventType) {
		return domain.AuditEvent{}, domain.Invalid("audit_event_type_invalid", "审计事件类型不受支持", "eventType")
	}
	if strings.TrimSpace(change.Reason) == "" {
		return domain.AuditEvent{}, domain.Invalid("audit_reason_required", "审计事件必须记录处置理由", "reason")
	}
	if change.AfterStatus == "" {
		return domain.AuditEvent{}, domain.Invalid("audit_after_status_required", "审计事件必须记录变更后状态", "afterStatus")
	}
	var previous string
	sequence := int64(1)
	if len(history) > 0 {
		last := history[len(history)-1]
		if last.DossierID != change.DossierID {
			return domain.AuditEvent{}, fmt.Errorf("审计历史属于其他档案")
		}
		previous, sequence = last.EventDigest, last.Sequence+1
	}
	e := domain.AuditEvent{EventID: change.EventID, DossierID: change.DossierID, Sequence: sequence, EventType: change.EventType, Actor: strings.TrimSpace(change.Actor), Reason: strings.TrimSpace(change.Reason), BeforeStatus: change.BeforeStatus, AfterStatus: change.AfterStatus, OccurredAt: b.now().UTC(), PreviousDigest: previous}
	e.EventDigest = eventDigest(e)
	return e, nil
}

func eventDigest(e domain.AuditEvent) string {
	return domain.DigestText(e.EventID, e.DossierID, fmt.Sprint(e.Sequence), e.EventType, e.Actor, e.Reason, string(e.BeforeStatus), string(e.AfterStatus), e.OccurredAt.Format(time.RFC3339Nano), e.PreviousDigest)
}
