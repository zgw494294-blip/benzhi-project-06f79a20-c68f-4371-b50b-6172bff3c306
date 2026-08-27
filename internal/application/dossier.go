package application

import (
	"context"
	"strings"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

func (s *Service) ReviseDossier(ctx context.Context, dossierID string, cmd ReviseDossierCommand) (DossierView, error) {
	var view DossierView
	err := s.coordinator.WithKey(dossierID, func() error {
		snap, err := s.repo.Get(ctx, dossierID)
		if err != nil {
			return err
		}
		if err = checkVersion(snap, cmd.Version); err != nil {
			return err
		}
		if strings.TrimSpace(cmd.Actor) == "" {
			return domain.Invalid("actor_required", "校订操作者不能为空", "actor")
		}
		if strings.TrimSpace(cmd.Reason) == "" {
			return domain.Invalid("reason_required", "校订理由不能为空", "reason")
		}
		createdAt, before := snap.Dossier.CreatedAt, snap.Dossier.Status
		changed, err := snap.Dossier.ReviseDetails(domain.DossierDetails{Title: cmd.Title, ParticipantName: cmd.ParticipantName, InterviewerName: cmd.InterviewerName, SessionDate: cmd.SessionDate, MaterialSummary: cmd.MaterialSummary, ConsentScope: cmd.ConsentScope}, s.now())
		if err != nil {
			return err
		}
		touch(&snap, s.now())
		snap.Dossier.CreatedAt = createdAt
		reason := strings.TrimSpace(cmd.Reason) + "；" + domain.FormatChangedFields(changed)
		if err = s.appendAudit(&snap, "dossier.revised", cmd.Actor, reason, before, snap.Dossier.Status); err != nil {
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

func (s *Service) RevisionHistory(ctx context.Context, dossierID string, from, to int) (domain.RevisionHistory, error) {
	snap, err := s.repo.Get(ctx, dossierID)
	if err != nil {
		return domain.RevisionHistory{}, err
	}
	return domain.VerifyRevisionHistory(dossierID, snap.Revisions, snap.Segments, from, to)
}
