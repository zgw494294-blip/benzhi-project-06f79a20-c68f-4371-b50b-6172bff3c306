package application

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/audit"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
)

type Service struct {
	repo        Repository
	coordinator *Coordinator
	auditor     *audit.Builder
	now         func() time.Time
	serial      atomic.Uint64
}

func NewService(repo Repository) *Service {
	now := time.Now
	return &Service{repo: repo, coordinator: NewCoordinator(), auditor: audit.NewBuilder(now), now: now}
}

func (s *Service) nextID(prefix string) string {
	return fmt.Sprintf("%s-%d-%06d", prefix, s.now().UTC().UnixNano(), s.serial.Add(1))
}

func (s *Service) appendAudit(snapshot *store.Snapshot, eventType, actor, reason string, before, after domain.DossierStatus) error {
	chain, err := audit.NewChain(snapshot.Dossier.DossierID, snapshot.AuditEvents)
	if err != nil {
		return err
	}
	e, err := chain.Append(s.auditor, audit.Change{EventID: s.nextID("evt"), DossierID: snapshot.Dossier.DossierID, EventType: eventType, Actor: actor, Reason: reason, BeforeStatus: before, AfterStatus: after})
	if err != nil {
		return err
	}
	snapshot.AuditEvents = append(snapshot.AuditEvents, e)
	return nil
}

func (s *Service) CreateDossier(ctx context.Context, cmd CreateDossierCommand) (DossierView, error) {
	id := s.nextID("dos")
	var view DossierView
	err := s.coordinator.WithKey(id, func() error {
		d, err := domain.NewDossier(domain.CreateDossierInput{DossierID: id, Title: cmd.Title, ParticipantName: cmd.ParticipantName, InterviewerName: cmd.InterviewerName, SessionDate: cmd.SessionDate, MaterialSummary: cmd.MaterialSummary, ConsentScope: cmd.ConsentScope, Now: s.now()})
		if err != nil {
			return err
		}
		snapshot := store.Snapshot{Dossier: *d, Segments: map[string][]domain.TranscriptSegment{}}
		if err = s.appendAudit(&snapshot, "dossier.created", cmd.Actor, "建立访谈档案", "", d.Status); err != nil {
			return err
		}
		if err = s.repo.Create(ctx, snapshot); err != nil {
			return err
		}
		view = s.makeView(snapshot)
		return nil
	})
	return view, err
}

func (s *Service) GetDossier(ctx context.Context, id string) (DossierView, error) {
	snap, err := s.repo.Get(context.WithoutCancel(ctx), id)
	if err != nil {
		return DossierView{}, err
	}
	return s.makeView(snap), nil
}
func (s *Service) ListDossiers(ctx context.Context) ([]domain.InterviewDossier, error) {
	return s.repo.List(context.WithoutCancel(ctx))
}

func (s *Service) makeView(snap store.Snapshot) DossierView {
	v := DossierView{Snapshot: snap, CurrentSegments: snap.CurrentSegments(), NextTodo: snap.Dossier.NextTodo()}
	if r, ok := snap.CurrentRevision(); ok {
		v.CurrentRevision = &r
	}
	if len(snap.Revisions) > 1 {
		previous := snap.Revisions[len(snap.Revisions)-2]
		v.RevisionDiff = domain.CompareSegments(snap.Segments[previous.RevisionID], v.CurrentSegments)
	}
	if result, err := audit.Verify(snap.AuditEvents); err == nil {
		v.AuditValid = result.Valid
		v.AuditHead = result.HeadDigest
	}
	if revision, ok := snap.CurrentRevision(); ok {
		for _, draft := range snap.ReviewDrafts {
			if draft.RevisionID != revision.RevisionID || draft.Participant != snap.Dossier.ParticipantName {
				continue
			}
			completed := make(map[string]bool, len(draft.Decisions))
			for _, decision := range draft.Decisions {
				completed[decision.SegmentID] = true
			}
			projection := &ReviewDraftProjection{RevisionID: draft.RevisionID, Participant: draft.Participant, Decisions: append([]domain.ReviewDraftDecision(nil), draft.Decisions...), Completed: len(draft.Decisions), SavedAt: draft.SavedAt}
			for _, segment := range v.CurrentSegments {
				if !completed[segment.SegmentID] {
					projection.Remaining = append(projection.Remaining, segment.SegmentID)
				}
			}
			v.ReviewDraft = projection
			break
		}
	}
	return v
}

func checkVersion(snap store.Snapshot, version int64) error {
	if version != snap.Dossier.Version {
		return domain.ErrVersionConflict
	}
	return nil
}
func touch(snap *store.Snapshot, now time.Time) {
	snap.Dossier.Version++
	snap.Dossier.UpdatedAt = now.UTC()
}
