package application

import (
	"context"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
)

type Repository interface {
	Create(context.Context, store.Snapshot) error
	Save(context.Context, store.Snapshot, int64) error
	Get(context.Context, string) (store.Snapshot, error)
	List(context.Context) ([]domain.InterviewDossier, error)
}

type CreateDossierCommand struct {
	Title           string             `json:"title"`
	ParticipantName string             `json:"participantName"`
	InterviewerName string             `json:"interviewerName"`
	SessionDate     string             `json:"sessionDate"`
	MaterialSummary string             `json:"materialSummary"`
	ConsentScope    domain.AccessLevel `json:"consentScope"`
	Actor           string             `json:"actor"`
}

type ReviseDossierCommand struct {
	Version         int64              `json:"version"`
	Title           string             `json:"title"`
	ParticipantName string             `json:"participantName"`
	InterviewerName string             `json:"interviewerName"`
	SessionDate     string             `json:"sessionDate"`
	MaterialSummary string             `json:"materialSummary"`
	ConsentScope    domain.AccessLevel `json:"consentScope"`
	Actor           string             `json:"actor"`
	Reason          string             `json:"reason"`
}

type SegmentInput struct {
	SegmentID string `json:"segmentId"`
	Sequence  int    `json:"sequence"`
	Text      string `json:"text"`
}
type SubmitRevisionCommand struct {
	Version      int64          `json:"version"`
	SourceDigest string         `json:"sourceDigest"`
	SubmittedBy  string         `json:"submittedBy"`
	Segments     []SegmentInput `json:"segments"`
}
type AnnotationInput struct {
	SegmentID           string             `json:"segmentId"`
	SensitivityTags     []string           `json:"sensitivityTags"`
	ProposedAccessLevel domain.AccessLevel `json:"proposedAccessLevel"`
	EmbargoUntil        *time.Time         `json:"embargoUntil,omitempty"`
}
type AnnotateCommand struct {
	Version     int64             `json:"version"`
	Actor       string            `json:"actor"`
	Annotations []AnnotationInput `json:"annotations"`
}
type DecisionInput struct {
	SegmentID            string              `json:"segmentId"`
	DecisionType         domain.DecisionType `json:"decisionType"`
	RequestedText        string              `json:"requestedText,omitempty"`
	RequestedAccessLevel domain.AccessLevel  `json:"requestedAccessLevel,omitempty"`
	Reason               string              `json:"reason,omitempty"`
}
type ReviewCommand struct {
	Version     int64           `json:"version"`
	RevisionID  string          `json:"revisionId,omitempty"`
	Participant string          `json:"participant"`
	Decisions   []DecisionInput `json:"decisions"`
}
type SaveReviewDraftCommand struct {
	Version     int64           `json:"version"`
	RevisionID  string          `json:"revisionId"`
	Participant string          `json:"participant"`
	Decisions   []DecisionInput `json:"decisions"`
}
type ResolveCommand struct {
	Version         int64  `json:"version"`
	Actor           string `json:"actor"`
	Resolution      string `json:"resolution"`
	ReplacementText string `json:"replacementText,omitempty"`
}
type SealCommand struct {
	Version            int64  `json:"version"`
	ConfirmationDigest string `json:"confirmationDigest"`
	Actor              string `json:"actor"`
	Reason             string `json:"reason"`
}

type ReviewDraftProjection struct {
	RevisionID  string                       `json:"revisionId"`
	Participant string                       `json:"participant"`
	Decisions   []domain.ReviewDraftDecision `json:"decisions"`
	Completed   int                          `json:"completed"`
	Remaining   []string                     `json:"remainingSegmentIds"`
	SavedAt     time.Time                    `json:"savedAt"`
}

type DossierView struct {
	Snapshot        store.Snapshot             `json:"snapshot"`
	CurrentRevision *domain.TranscriptRevision `json:"currentRevision,omitempty"`
	CurrentSegments []domain.TranscriptSegment `json:"currentSegments"`
	RevisionDiff    []domain.SegmentDiff       `json:"revisionDiff"`
	NextTodo        string                     `json:"nextTodo"`
	AuditValid      bool                       `json:"auditValid"`
	AuditHead       string                     `json:"auditHead,omitempty"`
	ReviewDraft     *ReviewDraftProjection     `json:"reviewDraft,omitempty"`
}

type VisibilityPreview struct {
	AccessLevel domain.AccessLevel `json:"accessLevel"`
	Count       int                `json:"count"`
	SegmentIDs  []string           `json:"segmentIds"`
}

type SealPreflight struct {
	DossierID           string                   `json:"dossierId"`
	Version             int64                    `json:"version"`
	RevisionID          string                   `json:"revisionId,omitempty"`
	ContentDigest       string                   `json:"contentDigest,omitempty"`
	ConsentDigest       string                   `json:"consentDigest,omitempty"`
	DecisionDigest      string                   `json:"decisionDigest,omitempty"`
	ManifestDigest      string                   `json:"manifestDigest,omitempty"`
	ConfirmationDigest  string                   `json:"confirmationDigest,omitempty"`
	AuditValid          bool                     `json:"auditValid"`
	CanSeal             bool                     `json:"canSeal"`
	Blockers            []domain.ValidationIssue `json:"blockers"`
	Visibility          []VisibilityPreview      `json:"visibility"`
	EmbargoedSegmentIDs []string                 `json:"embargoedSegmentIds"`
}

type ReadingSegment struct {
	SegmentID   string             `json:"segmentId"`
	Sequence    int                `json:"sequence"`
	Text        string             `json:"text"`
	AccessLevel domain.AccessLevel `json:"accessLevel"`
}
type ReadingCopy struct {
	DossierID      string              `json:"dossierId"`
	Title          string              `json:"title"`
	AccessLevel    domain.AccessLevel  `json:"accessLevel"`
	ManifestDigest string              `json:"manifestDigest"`
	Segments       []ReadingSegment    `json:"segments"`
	Timeline       []domain.AuditEvent `json:"timeline"`
	AuditValid     bool                `json:"auditValid"`
}
