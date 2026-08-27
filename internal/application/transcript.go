package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

func (s *Service) SubmitRevision(ctx context.Context, dossierID string, cmd SubmitRevisionCommand) (DossierView, error) {
	var view DossierView
	err := s.coordinator.WithKey(dossierID, func() error {
		snap, err := s.repo.Get(ctx, dossierID)
		if err != nil {
			return err
		}
		if err = checkVersion(snap, cmd.Version); err != nil {
			return err
		}
		if err = snap.Dossier.EnsureMutable(); err != nil {
			return err
		}
		if snap.Dossier.Status != domain.StatusDraft && snap.Dossier.Status != domain.StatusDispute {
			return domain.ErrInvalidTransition
		}
		if strings.TrimSpace(cmd.SubmittedBy) == "" {
			return domain.Invalid("submitter_required", "提交人不能为空", "submittedBy")
		}
		if strings.TrimSpace(cmd.SourceDigest) == "" {
			return domain.Invalid("source_digest_required", "来源摘要不能为空", "sourceDigest")
		}
		previous, currentNumber, supersedes := snap.CurrentSegments(), 1, ""
		if last, ok := snap.CurrentRevision(); ok {
			currentNumber = last.RevisionNumber + 1
			supersedes = last.RevisionID
		}
		revisionID := s.nextID("rev")
		raw := make([]domain.TranscriptSegment, len(cmd.Segments))
		for i, input := range cmd.Segments {
			raw[i] = domain.TranscriptSegment{SegmentID: input.SegmentID, Sequence: input.Sequence, Text: input.Text}
		}
		segments, digest, err := domain.PrepareSegments(revisionID, raw)
		if err != nil {
			return err
		}
		now := s.now()
		revision := domain.TranscriptRevision{RevisionID: revisionID, DossierID: dossierID, RevisionNumber: currentNumber, SourceDigest: cmd.SourceDigest, ContentDigest: digest, SubmittedBy: cmd.SubmittedBy, SubmittedAt: now.UTC(), SupersedesRevisionID: supersedes}
		before := snap.Dossier.Status
		if before == domain.StatusDraft {
			if err = snap.Dossier.Transition(domain.StatusPendingAnnotate, now); err != nil {
				return err
			}
		} else {
			if err = snap.Dossier.Transition(domain.StatusParticipantReview, now); err != nil {
				return err
			}
		}
		snap.Revisions = append(snap.Revisions, revision)
		snap.Segments[revisionID] = segments
		touch(&snap, now)
		reason := "提交不可覆盖的转写修订"
		if diffs := domain.CompareSegments(previous, segments); len(diffs) > 0 {
			reason = reason + "，差异段落数=" + fmtInt(len(diffs))
		}
		if err = s.appendAudit(&snap, "transcript.revised", cmd.SubmittedBy, reason, before, snap.Dossier.Status); err != nil {
			return err
		}
		if err = s.repo.Save(ctx, snap, cmd.Version); err != nil {
			return err
		}
		view = s.makeView(snap)
		return nil
	})
	return view, err
}

func fmtInt(v int) string {
	if v == 0 {
		return "0"
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}

func (s *Service) Annotate(ctx context.Context, dossierID string, cmd AnnotateCommand) (DossierView, error) {
	var view DossierView
	err := s.coordinator.WithKey(dossierID, func() error {
		snap, err := s.repo.Get(ctx, dossierID)
		if err != nil {
			return err
		}
		if err = checkVersion(snap, cmd.Version); err != nil {
			return err
		}
		if snap.Dossier.Status != domain.StatusPendingAnnotate {
			return domain.ErrInvalidTransition
		}
		if strings.TrimSpace(cmd.Actor) == "" {
			return domain.Invalid("actor_required", "标注人不能为空", "actor")
		}
		revision, ok := snap.CurrentRevision()
		if !ok {
			return domain.Invalid("revision_required", "没有可标注的转写修订", "revisionId")
		}
		segments := snap.Segments[revision.RevisionID]
		preflight := domain.PreflightAnnotations(segments, annotationValues(cmd.Annotations), snap.Dossier.ConsentScope, s.now())
		if err = preflight.FirstError(); err != nil {
			return err
		}
		for i, segment := range segments {
			a := preflight.Normalized[segment.SegmentID]
			segment.SensitivityTags = a.SensitivityTags
			segment.ProposedAccessLevel = a.ProposedAccessLevel
			segment.EmbargoUntil = a.EmbargoUntil
			segments[i] = segment
		}
		snap.Segments[revision.RevisionID] = segments
		before := snap.Dossier.Status
		now := s.now()
		if err = snap.Dossier.Transition(domain.StatusParticipantReview, now); err != nil {
			return err
		}
		touch(&snap, now)
		reason := "完成全量敏感标注并生成参与者审阅清单；" + annotationSummary(preflight.Summary)
		if err = s.appendAudit(&snap, "segments.annotated", cmd.Actor, reason, before, snap.Dossier.Status); err != nil {
			return err
		}
		if err = s.repo.Save(ctx, snap, cmd.Version); err != nil {
			return err
		}
		view = s.makeView(snap)
		return nil
	})
	return view, err
}

func (s *Service) AnnotationPreflight(ctx context.Context, dossierID string, cmd AnnotateCommand) (domain.AnnotationPreflight, error) {
	snap, err := s.repo.Get(context.WithoutCancel(ctx), dossierID)
	if err != nil {
		return domain.AnnotationPreflight{}, err
	}
	if err = checkVersion(snap, cmd.Version); err != nil {
		return domain.AnnotationPreflight{}, err
	}
	if snap.Dossier.Status != domain.StatusPendingAnnotate {
		return domain.AnnotationPreflight{}, domain.ErrInvalidTransition
	}
	revision, ok := snap.CurrentRevision()
	if !ok {
		return domain.AnnotationPreflight{}, domain.Invalid("revision_required", "没有可标注的转写修订", "revisionId")
	}
	return domain.PreflightAnnotations(snap.Segments[revision.RevisionID], annotationValues(cmd.Annotations), snap.Dossier.ConsentScope, s.now()), nil
}

func annotationValues(inputs []AnnotationInput) []domain.AnnotationValue {
	out := make([]domain.AnnotationValue, len(inputs))
	for i, input := range inputs {
		out[i] = domain.AnnotationValue{SegmentID: input.SegmentID, SensitivityTags: input.SensitivityTags, ProposedAccessLevel: input.ProposedAccessLevel, EmbargoUntil: input.EmbargoUntil}
	}
	return out
}

func annotationSummary(summary domain.AnnotationSummary) string {
	parts := []string{fmt.Sprintf("延迟期=%d", summary.Embargoed)}
	for _, values := range []map[string]int{summary.BySensitivityTag, summary.ByAccessLevel} {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", key, values[key]))
		}
	}
	return strings.Join(parts, "，")
}
