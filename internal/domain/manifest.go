package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

type ManifestSegmentEntry struct {
	SegmentID     string      `json:"segmentId"`
	Sequence      int         `json:"sequence"`
	SegmentDigest string      `json:"segmentDigest"`
	AccessLevel   AccessLevel `json:"accessLevel"`
	EmbargoUntil  *time.Time  `json:"embargoUntil,omitempty"`
}

type ReleaseManifest struct {
	ManifestID     string                 `json:"manifestId"`
	DossierID      string                 `json:"dossierId"`
	RevisionID     string                 `json:"revisionId"`
	ConsentDigest  string                 `json:"consentDigest"`
	DecisionDigest string                 `json:"decisionDigest"`
	SegmentEntries []ManifestSegmentEntry `json:"segmentEntries"`
	ManifestDigest string                 `json:"manifestDigest"`
	SealedAt       time.Time              `json:"sealedAt"`
	ReleasedAt     *time.Time             `json:"releasedAt,omitempty"`
}

func BuildManifest(id string, dossier InterviewDossier, revision TranscriptRevision, segments []TranscriptSegment, decisions []ReviewDecision, now time.Time) (ReleaseManifest, error) {
	if dossier.Status != StatusReadyApproval {
		return ReleaseManifest{}, ErrInvalidTransition
	}
	if !AllRequestsClosed(decisions) {
		return ReleaseManifest{}, ErrOpenRequests
	}
	entries := make([]ManifestSegmentEntry, len(segments))
	for i, segment := range segments {
		if !segment.ProposedAccessLevel.WithinConsent(dossier.ConsentScope) {
			return ReleaseManifest{}, ErrConsentMismatch
		}
		entries[i] = ManifestSegmentEntry{segment.SegmentID, segment.Sequence, segment.SegmentDigest, segment.ProposedAccessLevel, segment.EmbargoUntil}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Sequence < entries[j].Sequence })
	decisionParts := make([]string, 0, len(decisions)*4)
	sorted := append([]ReviewDecision(nil), decisions...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].DecisionID < sorted[j].DecisionID })
	for _, d := range sorted {
		decisionParts = append(decisionParts, d.DecisionID, d.RevisionID, d.Participant, d.SegmentID, string(d.DecisionType), string(d.RequestedAccessLevel), d.RequestedText, d.Reason, d.Resolution, d.ResolvedBy)
	}
	consentDigest := DigestText(dossier.DossierID, string(dossier.ConsentScope), dossier.ParticipantName)
	decisionDigest := DigestText(decisionParts...)
	canonical, _ := json.Marshal(struct {
		ID, Dossier, Revision, Consent, Decisions string
		Entries                                   []ManifestSegmentEntry
	}{id, dossier.DossierID, revision.RevisionID, consentDigest, decisionDigest, entries})
	sealed := now.UTC()
	return ReleaseManifest{ManifestID: id, DossierID: dossier.DossierID, RevisionID: revision.RevisionID, ConsentDigest: consentDigest, DecisionDigest: decisionDigest, SegmentEntries: entries, ManifestDigest: DigestText(string(canonical)), SealedAt: sealed}, nil
}

func SealConfirmationDigest(dossier InterviewDossier, revision TranscriptRevision, manifest ReleaseManifest, auditHead string) string {
	return DigestText(dossier.DossierID, fmt.Sprint(dossier.Version), revision.RevisionID, revision.ContentDigest, manifest.ConsentDigest, manifest.DecisionDigest, manifest.ManifestDigest, auditHead)
}

func (m ReleaseManifest) Verify() error {
	if m.ManifestID == "" || m.DossierID == "" || m.RevisionID == "" {
		return Invalid("manifest_identity_missing", "封存清单标识不完整", "manifestId")
	}
	if len(m.ConsentDigest) != 64 || len(m.DecisionDigest) != 64 || len(m.ManifestDigest) != 64 {
		return Invalid("manifest_digest_invalid", "封存清单摘要字段不完整", "manifestDigest")
	}
	if len(m.SegmentEntries) == 0 {
		return Invalid("manifest_empty", "封存清单没有段落", "segmentEntries")
	}
	seen := make(map[string]bool, len(m.SegmentEntries))
	for i, e := range m.SegmentEntries {
		if e.Sequence != i+1 || e.SegmentID == "" || len(e.SegmentDigest) != 64 {
			return fmt.Errorf("清单段落 %d 不完整", i+1)
		}
		if seen[e.SegmentID] {
			return Invalid("manifest_segment_duplicate", "封存清单包含重复段落", "segmentEntries")
		}
		seen[e.SegmentID] = true
		if err := e.AccessLevel.Validate(); err != nil {
			return err
		}
	}
	canonical, _ := json.Marshal(struct {
		ID, Dossier, Revision, Consent, Decisions string
		Entries                                   []ManifestSegmentEntry
	}{m.ManifestID, m.DossierID, m.RevisionID, m.ConsentDigest, m.DecisionDigest, m.SegmentEntries})
	if DigestText(string(canonical)) != m.ManifestDigest {
		return Invalid("manifest_digest_mismatch", "封存清单摘要校验失败", "manifestDigest")
	}
	return nil
}
