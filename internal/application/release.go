package application

import (
	"context"
	"fmt"
	"sort"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/audit"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
)

func (s *Service) Seal(ctx context.Context, dossierID string, cmd SealCommand) (DossierView, error) {
	var view DossierView
	err := s.coordinator.WithKey(dossierID, func() error {
		snap, err := s.repo.Get(ctx, dossierID)
		if err != nil {
			return err
		}
		if cmd.Version != snap.Dossier.Version {
			return domain.ErrPreflightStale
		}
		preflight, manifest, err := s.buildSealPreflight(snap)
		if err != nil {
			return err
		}
		if !preflight.CanSeal {
			return &domain.PreflightError{Issues: preflight.Blockers}
		}
		if cmd.ConfirmationDigest == "" || cmd.ConfirmationDigest != preflight.ConfirmationDigest {
			return domain.ErrPreflightStale
		}
		now := s.now()
		manifest.SealedAt = now.UTC()
		if err = manifest.Verify(); err != nil {
			return err
		}
		before := snap.Dossier.Status
		if err = snap.Dossier.Transition(domain.StatusSealed, now); err != nil {
			return err
		}
		if err = s.appendAudit(&snap, "dossier.sealed", cmd.Actor, cmd.Reason, before, domain.StatusSealed); err != nil {
			return err
		}
		if err = snap.Dossier.MarkReleased(now); err != nil {
			return err
		}
		released := now.UTC()
		manifest.ReleasedAt = &released
		snap.Manifest = &manifest
		if err = s.appendAudit(&snap, "dossier.released", cmd.Actor, "依据封存清单生成分级阅览副本", domain.StatusSealed, domain.StatusReleased); err != nil {
			return err
		}
		touch(&snap, now)
		if err = s.repo.Save(ctx, snap, cmd.Version); err != nil {
			return err
		}
		view = s.makeView(snap)
		return nil
	})
	return view, err
}

func (s *Service) SealPreflight(ctx context.Context, dossierID string) (SealPreflight, error) {
	snap, err := s.repo.Get(ctx, dossierID)
	if err != nil {
		return SealPreflight{}, fmt.Errorf("读取封存预检档案失败: %w", err)
	}
	result, _, err := s.buildSealPreflight(snap)
	return result, err
}

func (s *Service) buildSealPreflight(snap store.Snapshot) (SealPreflight, domain.ReleaseManifest, error) {
	result := SealPreflight{DossierID: snap.Dossier.DossierID, Version: snap.Dossier.Version}
	if snap.Dossier.Status != domain.StatusReadyApproval {
		return result, domain.ReleaseManifest{}, domain.ErrInvalidTransition
	}
	revision, ok := snap.CurrentRevision()
	if !ok {
		result.Blockers = append(result.Blockers, sealBlocker("revision_required", "没有可封存的最终修订", "revisionId", ""))
		return result, domain.ReleaseManifest{}, nil
	}
	result.RevisionID, result.ContentDigest = revision.RevisionID, revision.ContentDigest
	if _, err := domain.VerifyRevisionHistory(snap.Dossier.DossierID, snap.Revisions, snap.Segments, 0, 0); err != nil {
		result.Blockers = append(result.Blockers, sealBlocker("transcript_integrity", err.Error(), "revisionId", ""))
	}
	verified, auditErr := audit.Verify(snap.AuditEvents)
	if auditErr != nil {
		result.Blockers = append(result.Blockers, sealBlocker("audit_integrity", auditErr.Error(), "auditEvents", ""))
	} else {
		result.AuditValid = verified.Valid
	}
	if !domain.AllRequestsClosed(snap.Decisions) {
		result.Blockers = append(result.Blockers, sealBlocker("open_requests", domain.ErrOpenRequests.Error(), "decisions", ""))
	}
	segments := snap.Segments[revision.RevisionID]
	segmentByID := make(map[string]domain.TranscriptSegment, len(segments))
	for _, segment := range segments {
		segmentByID[segment.SegmentID] = segment
		if !segment.ProposedAccessLevel.WithinConsent(snap.Dossier.ConsentScope) {
			result.Blockers = append(result.Blockers, sealBlocker("consent_mismatch", domain.ErrConsentMismatch.Error(), "proposedAccessLevel", segment.SegmentID))
		}
	}
	revisionSegments := make(map[string]map[string]bool, len(snap.Revisions))
	for _, storedRevision := range snap.Revisions {
		revisionSegments[storedRevision.RevisionID] = map[string]bool{}
		for _, segment := range snap.Segments[storedRevision.RevisionID] {
			revisionSegments[storedRevision.RevisionID][segment.SegmentID] = true
		}
	}
	for _, decision := range snap.Decisions {
		if decision.DossierID != snap.Dossier.DossierID {
			result.Blockers = append(result.Blockers, sealBlocker("decision_dossier_mismatch", "正式决定属于其他档案", "decisions", decision.SegmentID))
		}
		if decision.Participant != snap.Dossier.ParticipantName {
			result.Blockers = append(result.Blockers, sealBlocker("decision_participant_mismatch", "正式决定不属于当前参与者", "decisions", decision.SegmentID))
		}
		ids, revisionExists := revisionSegments[decision.RevisionID]
		if !revisionExists || !ids[decision.SegmentID] {
			result.Blockers = append(result.Blockers, sealBlocker("decision_revision_mismatch", "正式决定没有归属于对应修订段落", "decisions", decision.SegmentID))
		}
	}
	decisions := latestRevisionDecisions(snap.Decisions, revision.RevisionID, snap.Dossier.ParticipantName)
	decisionBySegment := make(map[string]domain.ReviewDecision, len(decisions))
	for _, decision := range decisions {
		if _, exists := segmentByID[decision.SegmentID]; !exists {
			result.Blockers = append(result.Blockers, sealBlocker("decision_segment_mismatch", "正式决定不属于最终修订段落", "decisions", decision.SegmentID))
			continue
		}
		decisionBySegment[decision.SegmentID] = decision
	}
	for _, segment := range segments {
		if _, exists := decisionBySegment[segment.SegmentID]; !exists {
			result.Blockers = append(result.Blockers, sealBlocker("decision_missing", "最终修订段落缺少参与者正式决定", "decisions", segment.SegmentID))
		}
	}
	if len(result.Blockers) > 0 {
		return result, domain.ReleaseManifest{}, nil
	}
	candidateID := "man-" + domain.DigestText(snap.Dossier.DossierID, fmt.Sprint(snap.Dossier.Version), revision.RevisionID)[:28]
	manifest, err := domain.BuildManifest(candidateID, snap.Dossier, revision, segments, decisions, s.now())
	if err != nil {
		result.Blockers = append(result.Blockers, sealBlocker("manifest_invalid", err.Error(), "manifest", ""))
		return result, domain.ReleaseManifest{}, nil
	}
	if err = manifest.Verify(); err != nil {
		result.Blockers = append(result.Blockers, sealBlocker("manifest_integrity", err.Error(), "manifest", ""))
		return result, domain.ReleaseManifest{}, nil
	}
	result.ConsentDigest, result.DecisionDigest, result.ManifestDigest = manifest.ConsentDigest, manifest.DecisionDigest, manifest.ManifestDigest
	levels := []domain.AccessLevel{domain.AccessPublic, domain.AccessResearcher, domain.AccessRestricted, domain.AccessClosed}
	byLevel := map[domain.AccessLevel][]string{}
	now := s.now()
	for _, entry := range manifest.SegmentEntries {
		byLevel[entry.AccessLevel] = append(byLevel[entry.AccessLevel], entry.SegmentID)
		if entry.EmbargoUntil != nil && now.Before(*entry.EmbargoUntil) {
			result.EmbargoedSegmentIDs = append(result.EmbargoedSegmentIDs, entry.SegmentID)
		}
	}
	for _, level := range levels {
		result.Visibility = append(result.Visibility, VisibilityPreview{AccessLevel: level, Count: len(byLevel[level]), SegmentIDs: byLevel[level]})
	}
	result.ConfirmationDigest = domain.SealConfirmationDigest(snap.Dossier, revision, manifest, verified.HeadDigest)
	result.CanSeal = true
	return result, manifest, nil
}

func latestRevisionDecisions(all []domain.ReviewDecision, revisionID, participant string) []domain.ReviewDecision {
	latest := map[string]domain.ReviewDecision{}
	for _, decision := range all {
		if decision.RevisionID != revisionID || decision.Participant != participant {
			continue
		}
		previous, exists := latest[decision.SegmentID]
		if !exists || decision.DecidedAt.After(previous.DecidedAt) || decision.DecidedAt.Equal(previous.DecidedAt) && decision.DecisionID > previous.DecisionID {
			latest[decision.SegmentID] = decision
		}
	}
	out := make([]domain.ReviewDecision, 0, len(latest))
	for _, decision := range latest {
		out = append(out, decision)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SegmentID < out[j].SegmentID })
	return out
}

func sealBlocker(code, message, field, segmentID string) domain.ValidationIssue {
	return domain.ValidationIssue{Code: code, Message: message, Field: field, SegmentID: segmentID}
}

func (s *Service) ReadingCopy(ctx context.Context, dossierID string, viewer domain.AccessLevel) (ReadingCopy, error) {
	if err := viewer.Validate(); err != nil {
		return ReadingCopy{}, err
	}
	snap, err := s.repo.Get(ctx, dossierID)
	if err != nil {
		return ReadingCopy{}, fmt.Errorf("读取分级阅览档案失败: %w", err)
	}
	if snap.Dossier.Status != domain.StatusReleased || snap.Manifest == nil {
		return ReadingCopy{}, domain.ErrInvalidTransition
	}
	if err = snap.Manifest.Verify(); err != nil {
		return ReadingCopy{}, err
	}
	verified, err := audit.Verify(snap.AuditEvents)
	if err != nil {
		return ReadingCopy{}, err
	}
	byID := map[string]domain.TranscriptSegment{}
	for _, segment := range snap.CurrentSegments() {
		byID[segment.SegmentID] = segment
	}
	copy := ReadingCopy{DossierID: dossierID, Title: snap.Dossier.Title, AccessLevel: viewer, ManifestDigest: snap.Manifest.ManifestDigest, Timeline: append([]domain.AuditEvent(nil), snap.AuditEvents...), AuditValid: verified.Valid}
	now := time.Now()
	for _, entry := range snap.Manifest.SegmentEntries {
		if viewer.Allows(entry.AccessLevel, entry.EmbargoUntil, now) {
			segment := byID[entry.SegmentID]
			copy.Segments = append(copy.Segments, ReadingSegment{entry.SegmentID, entry.Sequence, segment.Text, entry.AccessLevel})
		}
	}
	return copy, nil
}
