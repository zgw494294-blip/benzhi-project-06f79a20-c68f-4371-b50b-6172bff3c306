package store

import (
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

type Snapshot struct {
	Dossier      domain.InterviewDossier               `json:"dossier"`
	Revisions    []domain.TranscriptRevision           `json:"revisions"`
	Segments     map[string][]domain.TranscriptSegment `json:"segmentsByRevision"`
	Decisions    []domain.ReviewDecision               `json:"decisions"`
	ReviewDrafts []domain.ReviewDraft                  `json:"-"`
	Manifest     *domain.ReleaseManifest               `json:"manifest,omitempty"`
	AuditEvents  []domain.AuditEvent                   `json:"auditEvents"`
}

func (s Snapshot) CurrentRevision() (domain.TranscriptRevision, bool) {
	if len(s.Revisions) == 0 {
		return domain.TranscriptRevision{}, false
	}
	return s.Revisions[len(s.Revisions)-1], true
}

func (s Snapshot) CurrentSegments() []domain.TranscriptSegment {
	r, ok := s.CurrentRevision()
	if !ok {
		return nil
	}
	return append([]domain.TranscriptSegment(nil), s.Segments[r.RevisionID]...)
}

func (s Snapshot) Clone() Snapshot {
	c := s
	c.Revisions = append([]domain.TranscriptRevision(nil), s.Revisions...)
	c.Decisions = append([]domain.ReviewDecision(nil), s.Decisions...)
	for i := range c.Decisions {
		c.Decisions[i].ResolvedAt = cloneTimePtr(s.Decisions[i].ResolvedAt)
	}
	c.ReviewDrafts = append([]domain.ReviewDraft(nil), s.ReviewDrafts...)
	for i := range c.ReviewDrafts {
		c.ReviewDrafts[i].Decisions = append([]domain.ReviewDraftDecision(nil), s.ReviewDrafts[i].Decisions...)
	}
	c.AuditEvents = append([]domain.AuditEvent(nil), s.AuditEvents...)
	c.Segments = make(map[string][]domain.TranscriptSegment, len(s.Segments))
	for k, v := range s.Segments {
		copies := make([]domain.TranscriptSegment, len(v))
		for i := range v {
			copies[i] = v[i]
			copies[i].SensitivityTags = append([]string(nil), v[i].SensitivityTags...)
			copies[i].EmbargoUntil = cloneTimePtr(v[i].EmbargoUntil)
		}
		c.Segments[k] = copies
	}
	if s.Manifest != nil {
		m := *s.Manifest
		m.SegmentEntries = append([]domain.ManifestSegmentEntry(nil), s.Manifest.SegmentEntries...)
		for i := range m.SegmentEntries {
			m.SegmentEntries[i].EmbargoUntil = cloneTimePtr(s.Manifest.SegmentEntries[i].EmbargoUntil)
		}
		m.ReleasedAt = cloneTimePtr(s.Manifest.ReleasedAt)
		c.Manifest = &m
	}
	return c
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}
