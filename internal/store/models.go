package store

import "benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"

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
	c.ReviewDrafts = append([]domain.ReviewDraft(nil), s.ReviewDrafts...)
	for i := range c.ReviewDrafts {
		c.ReviewDrafts[i].Decisions = append([]domain.ReviewDraftDecision(nil), s.ReviewDrafts[i].Decisions...)
	}
	c.AuditEvents = append([]domain.AuditEvent(nil), s.AuditEvents...)
	c.Segments = make(map[string][]domain.TranscriptSegment, len(s.Segments))
	for k, v := range s.Segments {
		c.Segments[k] = append([]domain.TranscriptSegment(nil), v...)
	}
	if s.Manifest != nil {
		m := *s.Manifest
		m.SegmentEntries = append([]domain.ManifestSegmentEntry(nil), s.Manifest.SegmentEntries...)
		c.Manifest = &m
	}
	return c
}
