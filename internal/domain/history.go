package domain

import (
	"fmt"
	"sort"
)

type RevisionVerification struct {
	Revision     TranscriptRevision `json:"revision"`
	SegmentCount int                `json:"segmentCount"`
	Verified     bool               `json:"verified"`
}

type RevisionComparison struct {
	FromRevision int           `json:"fromRevision"`
	ToRevision   int           `json:"toRevision"`
	Diffs        []SegmentDiff `json:"diffs"`
}

type RevisionHistory struct {
	DossierID  string                 `json:"dossierId"`
	Verified   bool                   `json:"verified"`
	History    []RevisionVerification `json:"history"`
	Comparison *RevisionComparison    `json:"comparison,omitempty"`
}

type IntegrityError struct {
	RevisionNumber int
	Message        string
}

func (e *IntegrityError) Error() string {
	if e.RevisionNumber > 0 {
		return fmt.Sprintf("修订 %d 完整性错误：%s", e.RevisionNumber, e.Message)
	}
	return "修订履历完整性错误：" + e.Message
}

func (e *IntegrityError) Unwrap() error { return ErrIntegrity }

func VerifyRevisionHistory(dossierID string, revisions []TranscriptRevision, segments map[string][]TranscriptSegment, from, to int) (RevisionHistory, error) {
	result := RevisionHistory{DossierID: dossierID}
	sorted := append([]TranscriptRevision(nil), revisions...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].RevisionNumber < sorted[j].RevisionNumber })
	byNumber := make(map[int]TranscriptRevision, len(sorted))
	for i, revision := range sorted {
		number := i + 1
		if revision.DossierID != dossierID {
			return RevisionHistory{}, &IntegrityError{revision.RevisionNumber, "修订属于其他档案"}
		}
		if revision.RevisionNumber != number {
			return RevisionHistory{}, &IntegrityError{revision.RevisionNumber, fmt.Sprintf("修订编号应为 %d", number)}
		}
		if _, exists := byNumber[revision.RevisionNumber]; exists {
			return RevisionHistory{}, &IntegrityError{revision.RevisionNumber, "修订编号重复"}
		}
		if i == 0 && revision.SupersedesRevisionID != "" {
			return RevisionHistory{}, &IntegrityError{revision.RevisionNumber, "首个修订不应指向被替代修订"}
		}
		if i > 0 && revision.SupersedesRevisionID != sorted[i-1].RevisionID {
			return RevisionHistory{}, &IntegrityError{revision.RevisionNumber, "被替代标识没有指向前一修订"}
		}
		stored := segments[revision.RevisionID]
		prepared, digest, err := PrepareSegments(revision.RevisionID, stored)
		if err != nil {
			return RevisionHistory{}, &IntegrityError{revision.RevisionNumber, err.Error()}
		}
		for j := range stored {
			if stored[j].RevisionID != revision.RevisionID {
				return RevisionHistory{}, &IntegrityError{revision.RevisionNumber, "段落属于其他修订"}
			}
			if stored[j].SegmentDigest != prepared[j].SegmentDigest {
				return RevisionHistory{}, &IntegrityError{revision.RevisionNumber, "段落摘要不一致"}
			}
		}
		if digest != revision.ContentDigest {
			return RevisionHistory{}, &IntegrityError{revision.RevisionNumber, "整版内容摘要不一致"}
		}
		byNumber[revision.RevisionNumber] = revision
		result.History = append(result.History, RevisionVerification{Revision: revision, SegmentCount: len(stored), Verified: true})
	}
	if (from == 0) != (to == 0) {
		return RevisionHistory{}, Invalid("comparison_pair_required", "比较参数 from 和 to 必须同时提供", "from")
	}
	if from != 0 {
		fromRevision, fromOK := byNumber[from]
		toRevision, toOK := byNumber[to]
		if !fromOK || !toOK {
			missing := from
			if fromOK {
				missing = to
			}
			return RevisionHistory{}, Invalid("revision_not_found", fmt.Sprintf("比较修订 %d 不存在", missing), "revisionNumber")
		}
		diffs := CompareSegments(segments[fromRevision.RevisionID], segments[toRevision.RevisionID])
		result.Comparison = &RevisionComparison{FromRevision: from, ToRevision: to, Diffs: diffs}
	}
	result.Verified = true
	return result, nil
}
