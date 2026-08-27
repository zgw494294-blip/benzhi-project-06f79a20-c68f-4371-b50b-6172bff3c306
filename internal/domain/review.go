package domain

import (
	"strings"
	"time"
)

type DecisionType string

const (
	DecisionConfirm    DecisionType = "confirm"
	DecisionRestrict   DecisionType = "restrict"
	DecisionTextChange DecisionType = "text_change"
)

type ReviewDecision struct {
	DecisionID           string       `json:"decisionId"`
	DossierID            string       `json:"dossierId"`
	RevisionID           string       `json:"revisionId"`
	Participant          string       `json:"participant"`
	SegmentID            string       `json:"segmentId"`
	DecisionType         DecisionType `json:"decisionType"`
	RequestedText        string       `json:"requestedText,omitempty"`
	RequestedAccessLevel AccessLevel  `json:"requestedAccessLevel,omitempty"`
	Reason               string       `json:"reason,omitempty"`
	Resolution           string       `json:"resolution,omitempty"`
	ResolvedBy           string       `json:"resolvedBy,omitempty"`
	DecidedAt            time.Time    `json:"decidedAt"`
	ResolvedAt           *time.Time   `json:"resolvedAt,omitempty"`
}

type ReviewDraftDecision struct {
	SegmentID            string       `json:"segmentId"`
	DecisionType         DecisionType `json:"decisionType"`
	RequestedText        string       `json:"requestedText,omitempty"`
	RequestedAccessLevel AccessLevel  `json:"requestedAccessLevel,omitempty"`
	Reason               string       `json:"reason,omitempty"`
}

type ReviewDraft struct {
	DossierID   string                `json:"dossierId"`
	RevisionID  string                `json:"revisionId"`
	Participant string                `json:"participant"`
	Decisions   []ReviewDraftDecision `json:"decisions"`
	SavedAt     time.Time             `json:"savedAt"`
}

func NewDecision(d ReviewDecision) (ReviewDecision, error) {
	if d.DecisionID == "" || d.DossierID == "" || d.RevisionID == "" || d.Participant == "" || d.SegmentID == "" {
		return d, Invalid("decision_identity_required", "审阅决定标识不完整", "decisionId")
	}
	switch d.DecisionType {
	case DecisionConfirm:
	case DecisionRestrict:
		if err := d.RequestedAccessLevel.Validate(); err != nil {
			return d, err
		}
		if strings.TrimSpace(d.Reason) == "" {
			return d, Invalid("reason_required", "收窄开放范围必须填写理由", "reason")
		}
	case DecisionTextChange:
		if strings.TrimSpace(d.RequestedText) == "" || strings.TrimSpace(d.Reason) == "" {
			return d, Invalid("change_detail_required", "文字修订请求必须填写替代文字和理由", "requestedText")
		}
	default:
		return d, Invalid("invalid_decision_type", "审阅决定类型无效", "decisionType")
	}
	d.DecidedAt = d.DecidedAt.UTC()
	return d, nil
}

func ValidateDraftDecision(input ReviewDraftDecision, segment TranscriptSegment) (ReviewDraftDecision, error) {
	input.SegmentID = strings.TrimSpace(input.SegmentID)
	input.RequestedText = strings.TrimSpace(input.RequestedText)
	input.Reason = strings.TrimSpace(input.Reason)
	probe := ReviewDecision{DecisionID: "draft", DossierID: "draft", RevisionID: segment.RevisionID, Participant: "draft", SegmentID: input.SegmentID, DecisionType: input.DecisionType, RequestedText: input.RequestedText, RequestedAccessLevel: input.RequestedAccessLevel, Reason: input.Reason, DecidedAt: time.Unix(0, 0)}
	validated, err := NewDecision(probe)
	if err != nil {
		return input, err
	}
	if input.DecisionType == DecisionRestrict && (input.RequestedAccessLevel == segment.ProposedAccessLevel || !input.RequestedAccessLevel.WithinConsent(segment.ProposedAccessLevel)) {
		return input, Invalid("access_not_narrower", "参与者必须把开放范围收窄到更受限级别", "requestedAccessLevel")
	}
	if input.DecisionType == DecisionConfirm {
		input.RequestedText, input.RequestedAccessLevel, input.Reason = "", "", ""
	}
	if input.DecisionType == DecisionTextChange {
		input.RequestedAccessLevel = ""
	}
	_ = validated
	return input, nil
}

func (d ReviewDecision) IsRequest() bool { return d.DecisionType != DecisionConfirm }
func (d ReviewDecision) IsClosed() bool  { return !d.IsRequest() || d.ResolvedAt != nil }

func AllRequestsClosed(decisions []ReviewDecision) bool {
	for _, decision := range decisions {
		if !decision.IsClosed() {
			return false
		}
	}
	return true
}

func ResolveDecision(decision *ReviewDecision, resolution, actor string, now time.Time) error {
	if !decision.IsRequest() {
		return Invalid("confirmation_not_resolvable", "确认决定无需响应", "decisionId")
	}
	if decision.ResolvedAt != nil {
		return Invalid("already_resolved", "修订请求已经关闭", "decisionId")
	}
	if strings.TrimSpace(resolution) == "" || strings.TrimSpace(actor) == "" {
		return Invalid("resolution_required", "处理说明和处理人不能为空", "resolution")
	}
	decision.Resolution, decision.ResolvedBy = strings.TrimSpace(resolution), strings.TrimSpace(actor)
	t := now.UTC()
	decision.ResolvedAt = &t
	return nil
}
