package application

import (
	"context"
	"strings"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
)

func (s *Service) Review(ctx context.Context, dossierID string, cmd ReviewCommand) (DossierView, error) {
	var view DossierView
	err := s.coordinator.WithKey(dossierID, func() error {
		snap, err := s.repo.Get(ctx, dossierID)
		if err != nil {
			return err
		}
		if err = checkVersion(snap, cmd.Version); err != nil {
			return err
		}
		if strings.TrimSpace(cmd.RevisionID) == "" {
			return domain.Invalid("revision_required", "正式审阅必须指定当前修订", "revisionId")
		}
		revision, segments, participant, err := reviewContext(snap, cmd.RevisionID, cmd.Participant)
		if err != nil {
			return err
		}
		merged := map[string]domain.ReviewDraftDecision{}
		for _, draft := range snap.ReviewDrafts {
			if draft.RevisionID == revision.RevisionID && draft.Participant == participant {
				for _, decision := range draft.Decisions {
					merged[decision.SegmentID] = decision
				}
			}
		}
		provided, err := validateReviewInputs(cmd.Decisions, segments)
		if err != nil {
			return err
		}
		for id, decision := range provided {
			merged[id] = decision
		}
		if len(merged) != len(segments) {
			return domain.Invalid("review_incomplete", "参与者必须逐段作出有效决定", "decisions")
		}
		newDecisions := make([]domain.ReviewDecision, 0, len(segments))
		hasRequest := false
		for _, segment := range segments {
			input, ok := merged[segment.SegmentID]
			if !ok {
				return domain.Invalid("review_incomplete", "参与者必须逐段作出有效决定", "decisions")
			}
			d, err := domain.NewDecision(domain.ReviewDecision{DecisionID: s.nextID("dec"), DossierID: dossierID, RevisionID: revision.RevisionID, Participant: participant, SegmentID: input.SegmentID, DecisionType: input.DecisionType, RequestedText: input.RequestedText, RequestedAccessLevel: input.RequestedAccessLevel, Reason: input.Reason, DecidedAt: s.now()})
			if err != nil {
				return err
			}
			if d.DecisionType == domain.DecisionRestrict {
				items := snap.Segments[revision.RevisionID]
				for i := range items {
					if items[i].SegmentID == d.SegmentID {
						items[i].ProposedAccessLevel = d.RequestedAccessLevel
					}
				}
				snap.Segments[revision.RevisionID] = items
			}
			if d.IsRequest() {
				hasRequest = true
			}
			newDecisions = append(newDecisions, d)
		}
		snap.Decisions = append(snap.Decisions, newDecisions...)
		keptDrafts := snap.ReviewDrafts[:0]
		for _, draft := range snap.ReviewDrafts {
			if draft.RevisionID != revision.RevisionID || draft.Participant != participant {
				keptDrafts = append(keptDrafts, draft)
			}
		}
		snap.ReviewDrafts = keptDrafts
		before := snap.Dossier.Status
		now := s.now()
		target := domain.StatusReadyApproval
		if hasRequest {
			target = domain.StatusDispute
		}
		if err = snap.Dossier.Transition(target, now); err != nil {
			return err
		}
		touch(&snap, now)
		event := "review.confirmed"
		reason := "参与者完成逐段确认"
		if hasRequest {
			event = "review.requested"
			reason = "参与者提出收窄范围或文字修订请求"
		}
		if err = s.appendAudit(&snap, event, participant, reason, before, target); err != nil {
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

func (s *Service) SaveReviewDraft(ctx context.Context, dossierID string, cmd SaveReviewDraftCommand) (DossierView, error) {
	var view DossierView
	err := s.coordinator.WithKey(dossierID, func() error {
		snap, err := s.repo.Get(ctx, dossierID)
		if err != nil {
			return err
		}
		if err = checkVersion(snap, cmd.Version); err != nil {
			return err
		}
		if strings.TrimSpace(cmd.RevisionID) == "" {
			return domain.Invalid("revision_required", "保存草稿必须指定当前修订", "revisionId")
		}
		revision, segments, participant, err := reviewContext(snap, cmd.RevisionID, cmd.Participant)
		if err != nil {
			return err
		}
		validated, err := validateReviewInputs(cmd.Decisions, segments)
		if err != nil {
			return err
		}
		if len(validated) == 0 {
			return domain.Invalid("review_draft_empty", "审阅草稿至少包含一项决定", "decisions")
		}
		decisions := make([]domain.ReviewDraftDecision, 0, len(validated))
		for _, segment := range segments {
			if decision, ok := validated[segment.SegmentID]; ok {
				decisions = append(decisions, decision)
			}
		}
		now := s.now()
		draft := domain.ReviewDraft{DossierID: dossierID, RevisionID: revision.RevisionID, Participant: participant, Decisions: decisions, SavedAt: now.UTC()}
		replaced := false
		for i := range snap.ReviewDrafts {
			if snap.ReviewDrafts[i].RevisionID == revision.RevisionID && snap.ReviewDrafts[i].Participant == participant {
				snap.ReviewDrafts[i], replaced = draft, true
			}
		}
		if !replaced {
			snap.ReviewDrafts = append(snap.ReviewDrafts, draft)
		}
		touch(&snap, now)
		reason := "保存参与者审阅草稿；已完成=" + fmtInt(len(decisions)) + "，待决定=" + fmtInt(len(segments)-len(decisions))
		if err = s.appendAudit(&snap, "review.draft_saved", participant, reason, snap.Dossier.Status, snap.Dossier.Status); err != nil {
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

func reviewContext(snap store.Snapshot, requestedRevision, participant string) (domain.TranscriptRevision, []domain.TranscriptSegment, string, error) {
	if snap.Dossier.Status != domain.StatusParticipantReview {
		return domain.TranscriptRevision{}, nil, "", domain.ErrInvalidTransition
	}
	participant = strings.TrimSpace(participant)
	if participant == "" {
		return domain.TranscriptRevision{}, nil, "", domain.Invalid("participant_required", "参与者不能为空", "participant")
	}
	if participant != snap.Dossier.ParticipantName {
		return domain.TranscriptRevision{}, nil, "", domain.Invalid("participant_mismatch", "参与者与当前档案不匹配", "participant")
	}
	revision, ok := snap.CurrentRevision()
	if !ok {
		return domain.TranscriptRevision{}, nil, "", domain.Invalid("revision_required", "当前审阅清单没有修订", "revisionId")
	}
	if requestedRevision != "" && requestedRevision != revision.RevisionID {
		return domain.TranscriptRevision{}, nil, "", domain.ErrRevisionStale
	}
	return revision, snap.Segments[revision.RevisionID], participant, nil
}

func validateReviewInputs(inputs []DecisionInput, segments []domain.TranscriptSegment) (map[string]domain.ReviewDraftDecision, error) {
	bySegment := make(map[string]domain.TranscriptSegment, len(segments))
	for _, segment := range segments {
		bySegment[segment.SegmentID] = segment
	}
	result := make(map[string]domain.ReviewDraftDecision, len(inputs))
	for _, input := range inputs {
		segment, ok := bySegment[input.SegmentID]
		if !ok {
			return nil, domain.Invalid("invalid_review_segment", "审阅段落不属于当前修订", "segmentId")
		}
		if _, exists := result[input.SegmentID]; exists {
			return nil, domain.Invalid("duplicate_review_segment", "同一段落不能重复审阅", "segmentId")
		}
		validated, err := domain.ValidateDraftDecision(domain.ReviewDraftDecision{SegmentID: input.SegmentID, DecisionType: input.DecisionType, RequestedText: input.RequestedText, RequestedAccessLevel: input.RequestedAccessLevel, Reason: input.Reason}, segment)
		if err != nil {
			return nil, err
		}
		result[input.SegmentID] = validated
	}
	return result, nil
}

func (s *Service) Resolve(ctx context.Context, dossierID, decisionID string, cmd ResolveCommand) (DossierView, error) {
	var view DossierView
	err := s.coordinator.WithKey(dossierID, func() error {
		snap, err := s.repo.Get(ctx, dossierID)
		if err != nil {
			return err
		}
		if err = checkVersion(snap, cmd.Version); err != nil {
			return err
		}
		if snap.Dossier.Status != domain.StatusDispute {
			return domain.ErrInvalidTransition
		}
		index := -1
		for i := range snap.Decisions {
			if snap.Decisions[i].DecisionID == decisionID {
				index = i
				break
			}
		}
		if index < 0 {
			return domain.ErrNotFound
		}
		decision := &snap.Decisions[index]
		if decision.DecisionType == domain.DecisionTextChange && strings.TrimSpace(cmd.ReplacementText) == "" {
			return domain.Invalid("replacement_required", "文字修订请求必须提交替代段落", "replacementText")
		}
		if err = domain.ResolveDecision(decision, cmd.Resolution, cmd.Actor, s.now()); err != nil {
			return err
		}
		if decision.DecisionType == domain.DecisionTextChange {
			last, _ := snap.CurrentRevision()
			current := snap.CurrentSegments()
			revisionID := s.nextID("rev")
			raw := make([]domain.TranscriptSegment, len(current))
			for i, segment := range current {
				segment.RevisionID = ""
				if segment.SegmentID == decision.SegmentID {
					segment.Text = strings.TrimSpace(cmd.ReplacementText)
				}
				raw[i] = segment
			}
			prepared, digest, err := domain.PrepareSegments(revisionID, raw)
			if err != nil {
				return err
			}
			revision := domain.TranscriptRevision{RevisionID: revisionID, DossierID: dossierID, RevisionNumber: last.RevisionNumber + 1, SourceDigest: last.ContentDigest, ContentDigest: digest, SubmittedBy: cmd.Actor, SubmittedAt: s.now().UTC(), SupersedesRevisionID: last.RevisionID}
			for i := range prepared {
				for _, old := range current {
					if old.SegmentID == prepared[i].SegmentID {
						prepared[i].SensitivityTags = old.SensitivityTags
						prepared[i].ProposedAccessLevel = old.ProposedAccessLevel
						prepared[i].EmbargoUntil = old.EmbargoUntil
					}
				}
			}
			snap.Revisions = append(snap.Revisions, revision)
			snap.Segments[revisionID] = prepared
		}
		before := snap.Dossier.Status
		now := s.now()
		if domain.AllRequestsClosed(snap.Decisions) {
			if err = snap.Dossier.Transition(domain.StatusParticipantReview, now); err != nil {
				return err
			}
		}
		touch(&snap, now)
		if err = s.appendAudit(&snap, "review.resolved", cmd.Actor, cmd.Resolution, before, snap.Dossier.Status); err != nil {
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
