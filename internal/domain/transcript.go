package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type TranscriptRevision struct {
	RevisionID           string    `json:"revisionId"`
	DossierID            string    `json:"dossierId"`
	RevisionNumber       int       `json:"revisionNumber"`
	SourceDigest         string    `json:"sourceDigest"`
	ContentDigest        string    `json:"contentDigest"`
	SubmittedBy          string    `json:"submittedBy"`
	SubmittedAt          time.Time `json:"submittedAt"`
	SupersedesRevisionID string    `json:"supersedesRevisionId,omitempty"`
}

type TranscriptSegment struct {
	SegmentID           string      `json:"segmentId"`
	RevisionID          string      `json:"revisionId"`
	Sequence            int         `json:"sequence"`
	Text                string      `json:"text"`
	SensitivityTags     []string    `json:"sensitivityTags"`
	ProposedAccessLevel AccessLevel `json:"proposedAccessLevel"`
	EmbargoUntil        *time.Time  `json:"embargoUntil,omitempty"`
	SegmentDigest       string      `json:"segmentDigest"`
}

func DigestText(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(fmt.Sprintf("%d:", len(part))))
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func NormalizeTags(tags []string) ([]string, error) {
	allowed := map[string]bool{"name": true, "place": true, "third_party": true, "health": true, "political": true, "other": true}
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		if !allowed[tag] {
			return nil, Invalid("invalid_sensitivity_tag", "敏感类别不受支持", "sensitivityTags")
		}
		if !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out, nil
}

func PrepareSegments(revisionID string, segments []TranscriptSegment) ([]TranscriptSegment, string, error) {
	if len(segments) == 0 {
		return nil, "", Invalid("segments_required", "转写至少包含一个段落", "segments")
	}
	out := make([]TranscriptSegment, len(segments))
	parts := make([]string, 0, len(segments)*3)
	ids := map[string]bool{}
	for i, segment := range segments {
		if segment.Sequence != i+1 {
			return nil, "", Invalid("invalid_sequence", "段落 sequence 必须从 1 连续递增", "segments")
		}
		if strings.TrimSpace(segment.SegmentID) == "" || ids[segment.SegmentID] {
			return nil, "", Invalid("invalid_segment_id", "段落编号不能为空且不得重复", "segments")
		}
		if strings.TrimSpace(segment.Text) == "" {
			return nil, "", Invalid("segment_text_required", "段落正文不能为空", "segments")
		}
		ids[segment.SegmentID] = true
		segment.RevisionID = revisionID
		segment.Text = strings.TrimSpace(segment.Text)
		segment.SegmentDigest = DigestText(segment.SegmentID, fmt.Sprint(segment.Sequence), segment.Text)
		out[i] = segment
		parts = append(parts, segment.SegmentID, fmt.Sprint(segment.Sequence), segment.SegmentDigest)
	}
	return out, DigestText(parts...), nil
}

type SegmentDiff struct {
	SegmentID string `json:"segmentId"`
	Kind      string `json:"kind"`
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
}

func CompareSegments(previous, current []TranscriptSegment) []SegmentDiff {
	old := map[string]string{}
	for _, s := range previous {
		old[s.SegmentID] = s.Text
	}
	var diffs []SegmentDiff
	for _, s := range current {
		if before, ok := old[s.SegmentID]; !ok {
			diffs = append(diffs, SegmentDiff{s.SegmentID, "added", "", s.Text})
		} else if before != s.Text {
			diffs = append(diffs, SegmentDiff{s.SegmentID, "changed", before, s.Text})
		}
		delete(old, s.SegmentID)
	}
	for id, before := range old {
		diffs = append(diffs, SegmentDiff{id, "removed", before, ""})
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].SegmentID < diffs[j].SegmentID })
	return diffs
}
