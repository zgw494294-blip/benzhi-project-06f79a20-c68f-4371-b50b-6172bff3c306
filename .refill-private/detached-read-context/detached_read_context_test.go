package detached_read_context_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/application"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
)

type observedContext struct {
	operation string
	err       error
}

type contextRecordingRepository struct {
	observed []observedContext
}

func (r *contextRecordingRepository) Create(context.Context, store.Snapshot) error {
	return fmt.Errorf("unexpected Create call")
}

func (r *contextRecordingRepository) Save(context.Context, store.Snapshot, int64) error {
	return fmt.Errorf("unexpected Save call")
}

func (r *contextRecordingRepository) Get(ctx context.Context, _ string) (store.Snapshot, error) {
	r.observed = append(r.observed, observedContext{operation: "Get", err: ctx.Err()})
	return store.Snapshot{}, domain.ErrNotFound
}

func (r *contextRecordingRepository) List(ctx context.Context) ([]domain.InterviewDossier, error) {
	r.observed = append(r.observed, observedContext{operation: "List", err: ctx.Err()})
	return nil, domain.ErrNotFound
}

func TestCanceledReadQueriesReachRepositoryContext(t *testing.T) {
	repo := &contextRecordingRepository{}
	service := application.NewService(repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	queries := []struct {
		name string
		call func()
	}{
		{name: "GetDossier", call: func() { _, _ = service.GetDossier(ctx, "missing") }},
		{name: "ListDossiers", call: func() { _, _ = service.ListDossiers(ctx) }},
		{name: "RevisionHistory", call: func() { _, _ = service.RevisionHistory(ctx, "missing", 0, 0) }},
		{name: "AnnotationPreflight", call: func() {
			_, _ = service.AnnotationPreflight(ctx, "missing", application.AnnotateCommand{})
		}},
		{name: "SealPreflight", call: func() { _, _ = service.SealPreflight(ctx, "missing") }},
		{name: "ReadingCopy", call: func() {
			_, _ = service.ReadingCopy(ctx, "missing", domain.AccessPublic)
		}},
	}

	for _, query := range queries {
		before := len(repo.observed)
		query.call()
		if len(repo.observed) != before+1 {
			t.Fatalf("%s: expected exactly one repository call, got %d", query.name, len(repo.observed)-before)
		}
		observation := repo.observed[before]
		if !errors.Is(observation.err, context.Canceled) {
			t.Errorf("%s: %s received context error %v, want context.Canceled", query.name, observation.operation, observation.err)
		}
	}
}
