package domain

import (
	"fmt"
	"sort"
	"time"
)

type AnnotationValue struct {
	SegmentID           string      `json:"segmentId"`
	SensitivityTags     []string    `json:"sensitivityTags"`
	ProposedAccessLevel AccessLevel `json:"proposedAccessLevel"`
	EmbargoUntil        *time.Time  `json:"embargoUntil,omitempty"`
}

type ValidationIssue struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Field     string `json:"field"`
	SegmentID string `json:"segmentId,omitempty"`
}

type AnnotationSummary struct {
	BySensitivityTag map[string]int `json:"bySensitivityTag"`
	ByAccessLevel    map[string]int `json:"byAccessLevel"`
	Embargoed        int            `json:"embargoed"`
}

type AnnotationPreflight struct {
	CanSubmit  bool                       `json:"canSubmit"`
	Summary    AnnotationSummary          `json:"summary"`
	Issues     []ValidationIssue          `json:"issues"`
	Normalized map[string]AnnotationValue `json:"-"`
}

func PreflightAnnotations(segments []TranscriptSegment, values []AnnotationValue, consent AccessLevel, now time.Time) AnnotationPreflight {
	result := AnnotationPreflight{Summary: AnnotationSummary{BySensitivityTag: map[string]int{}, ByAccessLevel: map[string]int{}}, Normalized: map[string]AnnotationValue{}}
	segmentIDs := make(map[string]bool, len(segments))
	for _, segment := range segments {
		segmentIDs[segment.SegmentID] = true
	}
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value.SegmentID] {
			result.Issues = append(result.Issues, ValidationIssue{"duplicate_annotation", "同一段落不能重复标注", "annotations", value.SegmentID})
			continue
		}
		seen[value.SegmentID] = true
		if !segmentIDs[value.SegmentID] {
			result.Issues = append(result.Issues, ValidationIssue{"unknown_annotation_segment", "标注段落不属于当前修订", "annotations", value.SegmentID})
			continue
		}
		tags, err := NormalizeTags(value.SensitivityTags)
		if err != nil {
			result.Issues = append(result.Issues, ValidationIssue{"invalid_sensitivity_tag", err.Error(), "sensitivityTags", value.SegmentID})
		} else {
			value.SensitivityTags = tags
		}
		if err := value.ProposedAccessLevel.Validate(); err != nil {
			result.Issues = append(result.Issues, ValidationIssue{"invalid_access_level", "建议开放级别无效", "proposedAccessLevel", value.SegmentID})
		} else if !value.ProposedAccessLevel.WithinConsent(consent) {
			result.Issues = append(result.Issues, ValidationIssue{"consent_mismatch", "建议开放级别超出档案整体授权", "proposedAccessLevel", value.SegmentID})
		}
		if value.EmbargoUntil != nil {
			if value.ProposedAccessLevel == AccessClosed {
				result.Issues = append(result.Issues, ValidationIssue{"closed_embargo_invalid", "不开放段落不能设置延迟日期", "embargoUntil", value.SegmentID})
			} else if !afterCalendarDate(*value.EmbargoUntil, now) {
				result.Issues = append(result.Issues, ValidationIssue{"embargo_not_future", "延迟开放日期必须晚于当前日期", "embargoUntil", value.SegmentID})
			}
		}
		result.Normalized[value.SegmentID] = value
	}
	for _, segment := range segments {
		if !seen[segment.SegmentID] {
			result.Issues = append(result.Issues, ValidationIssue{"annotation_missing", "段落尚未覆盖标注", "annotations", segment.SegmentID})
		}
	}
	for _, segment := range segments {
		value, exists := result.Normalized[segment.SegmentID]
		if !exists {
			continue
		}
		if tags, err := NormalizeTags(value.SensitivityTags); err == nil {
			for _, tag := range tags {
				result.Summary.BySensitivityTag[tag]++
			}
		}
		if err := value.ProposedAccessLevel.Validate(); err == nil {
			result.Summary.ByAccessLevel[string(value.ProposedAccessLevel)]++
		}
		if value.EmbargoUntil != nil && value.ProposedAccessLevel != AccessClosed && afterCalendarDate(*value.EmbargoUntil, now) {
			result.Summary.Embargoed++
		}
	}
	sort.SliceStable(result.Issues, func(i, j int) bool {
		left, right := result.Issues[i], result.Issues[j]
		if left.SegmentID == right.SegmentID {
			return left.Code < right.Code
		}
		return left.SegmentID < right.SegmentID
	})
	result.CanSubmit = len(result.Issues) == 0
	return result
}

func afterCalendarDate(candidate, now time.Time) bool {
	candidateDate := candidate.UTC().Format("2006-01-02")
	return candidateDate > now.UTC().Format("2006-01-02")
}

func (p AnnotationPreflight) FirstError() error {
	if p.CanSubmit {
		return nil
	}
	issue := p.Issues[0]
	return Invalid(issue.Code, fmt.Sprintf("段落 %s：%s", issue.SegmentID, issue.Message), issue.Field)
}
