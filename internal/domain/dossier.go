package domain

import (
	"fmt"
	"strings"
	"time"
)

type DossierStatus string

const (
	StatusDraft             DossierStatus = "draft"
	StatusPendingAnnotate   DossierStatus = "pending_annotation"
	StatusParticipantReview DossierStatus = "participant_review"
	StatusDispute           DossierStatus = "dispute_resolution"
	StatusReadyApproval     DossierStatus = "ready_approval"
	StatusSealed            DossierStatus = "sealed"
	StatusReleased          DossierStatus = "released"
)

type InterviewDossier struct {
	DossierID       string        `json:"dossierId"`
	Title           string        `json:"title"`
	ParticipantName string        `json:"participantName"`
	InterviewerName string        `json:"interviewerName"`
	SessionDate     string        `json:"sessionDate"`
	MaterialSummary string        `json:"materialSummary"`
	ConsentScope    AccessLevel   `json:"consentScope"`
	Status          DossierStatus `json:"status"`
	Version         int64         `json:"version"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}

type CreateDossierInput struct {
	DossierID, Title, ParticipantName, InterviewerName, SessionDate, MaterialSummary string
	ConsentScope                                                                     AccessLevel
	Now                                                                              time.Time
}

type DossierDetails struct {
	Title           string      `json:"title"`
	ParticipantName string      `json:"participantName"`
	InterviewerName string      `json:"interviewerName"`
	SessionDate     string      `json:"sessionDate"`
	MaterialSummary string      `json:"materialSummary"`
	ConsentScope    AccessLevel `json:"consentScope"`
}

func validateDossierDetails(in DossierDetails) (DossierDetails, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.ParticipantName = strings.TrimSpace(in.ParticipantName)
	in.InterviewerName = strings.TrimSpace(in.InterviewerName)
	in.SessionDate = strings.TrimSpace(in.SessionDate)
	in.MaterialSummary = strings.TrimSpace(in.MaterialSummary)
	checks := []struct{ value, code, message, field string }{
		{in.Title, "title_required", "档案标题不能为空", "title"},
		{in.ParticipantName, "participant_required", "参与者姓名不能为空", "participantName"},
		{in.InterviewerName, "interviewer_required", "访谈员姓名不能为空", "interviewerName"},
		{in.SessionDate, "session_date_required", "访谈日期不能为空", "sessionDate"},
		{in.MaterialSummary, "summary_required", "材料摘要不能为空", "materialSummary"},
	}
	for _, check := range checks {
		if check.value == "" {
			return in, Invalid(check.code, check.message, check.field)
		}
	}
	if parsed, err := time.Parse("2006-01-02", in.SessionDate); err != nil || parsed.Format("2006-01-02") != in.SessionDate {
		return in, Invalid("invalid_session_date", "访谈日期必须是有效的 YYYY-MM-DD 日期", "sessionDate")
	}
	if err := in.ConsentScope.Validate(); err != nil {
		return in, Invalid("invalid_consent_scope", "整体授权范围无效", "consentScope")
	}
	return in, nil
}

func NewDossier(in CreateDossierInput) (*InterviewDossier, error) {
	if strings.TrimSpace(in.DossierID) == "" {
		return nil, Invalid("dossier_id_required", "档案编号不能为空", "dossierId")
	}
	details, err := validateDossierDetails(DossierDetails{in.Title, in.ParticipantName, in.InterviewerName, in.SessionDate, in.MaterialSummary, in.ConsentScope})
	if err != nil {
		return nil, err
	}
	now := in.Now.UTC()
	return &InterviewDossier{
		DossierID: in.DossierID, Title: details.Title,
		ParticipantName: details.ParticipantName, InterviewerName: details.InterviewerName,
		SessionDate: details.SessionDate, MaterialSummary: details.MaterialSummary,
		ConsentScope: details.ConsentScope, Status: StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (d *InterviewDossier) ReviseDetails(details DossierDetails, now time.Time) ([]string, error) {
	if d.Status != StatusDraft {
		if err := d.EnsureMutable(); err != nil {
			return nil, err
		}
		return nil, ErrInvalidTransition
	}
	validated, err := validateDossierDetails(details)
	if err != nil {
		return nil, err
	}
	changes := make([]string, 0, 6)
	if d.Title != validated.Title {
		changes = append(changes, "title")
	}
	if d.ParticipantName != validated.ParticipantName {
		changes = append(changes, "participantName")
	}
	if d.InterviewerName != validated.InterviewerName {
		changes = append(changes, "interviewerName")
	}
	if d.SessionDate != validated.SessionDate {
		changes = append(changes, "sessionDate")
	}
	if d.MaterialSummary != validated.MaterialSummary {
		changes = append(changes, "materialSummary")
	}
	if d.ConsentScope != validated.ConsentScope {
		changes = append(changes, "consentScope")
	}
	if len(changes) == 0 {
		return nil, Invalid("dossier_unchanged", "校订内容没有实际变化", "form")
	}
	d.Title, d.ParticipantName, d.InterviewerName = validated.Title, validated.ParticipantName, validated.InterviewerName
	d.SessionDate, d.MaterialSummary, d.ConsentScope = validated.SessionDate, validated.MaterialSummary, validated.ConsentScope
	d.UpdatedAt = now.UTC()
	return changes, nil
}

func FormatChangedFields(fields []string) string {
	return fmt.Sprintf("变更字段=%s", strings.Join(fields, ","))
}

func (d *InterviewDossier) EnsureMutable() error {
	if d.Status == StatusSealed || d.Status == StatusReleased {
		return ErrSealed
	}
	return nil
}

func (d *InterviewDossier) Transition(to DossierStatus, now time.Time) error {
	if err := d.EnsureMutable(); err != nil {
		return err
	}
	allowed := map[DossierStatus][]DossierStatus{
		StatusDraft:             {StatusPendingAnnotate},
		StatusPendingAnnotate:   {StatusParticipantReview},
		StatusParticipantReview: {StatusDispute, StatusReadyApproval},
		StatusDispute:           {StatusParticipantReview, StatusReadyApproval},
		StatusReadyApproval:     {StatusSealed},
	}
	for _, candidate := range allowed[d.Status] {
		if candidate == to {
			d.Status = to
			d.UpdatedAt = now.UTC()
			return nil
		}
	}
	return ErrInvalidTransition
}

func (d *InterviewDossier) MarkReleased(now time.Time) error {
	if d.Status != StatusSealed {
		return ErrInvalidTransition
	}
	d.Status, d.UpdatedAt = StatusReleased, now.UTC()
	return nil
}

func (d InterviewDossier) NextTodo() string {
	switch d.Status {
	case StatusDraft:
		return "提交首个转写修订"
	case StatusPendingAnnotate:
		return "完成全部段落的敏感标注"
	case StatusParticipantReview:
		return "等待参与者逐段审阅"
	case StatusDispute:
		return "编研员处理参与者异议"
	case StatusReadyApproval:
		return "核验授权边界并封存"
	case StatusSealed:
		return "生成分级阅览副本"
	case StatusReleased:
		return "档案已开放"
	default:
		return "检查档案状态"
	}
}
