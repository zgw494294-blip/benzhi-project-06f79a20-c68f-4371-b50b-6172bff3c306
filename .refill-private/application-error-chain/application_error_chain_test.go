package application_error_chain_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/application"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/web"
)

type missingRepository struct{}

func (missingRepository) Create(context.Context, store.Snapshot) error {
	return fmt.Errorf("unexpected Create call")
}

func (missingRepository) Save(context.Context, store.Snapshot, int64) error {
	return fmt.Errorf("unexpected Save call")
}

func (missingRepository) Get(context.Context, string) (store.Snapshot, error) {
	return store.Snapshot{}, fmt.Errorf("sqlite lookup failed: %w", domain.ErrNotFound)
}

func (missingRepository) List(context.Context) ([]domain.InterviewDossier, error) {
	return nil, fmt.Errorf("sqlite list failed: %w", domain.ErrNotFound)
}

func TestApplicationQueriesPreserveRepositoryErrorIdentity(t *testing.T) {
	ctx := context.Background()
	service := application.NewService(missingRepository{})
	queries := []struct {
		name string
		call func() error
	}{
		{"GetDossier", func() error {
			_, err := service.GetDossier(ctx, "missing")
			return err
		}},
		{"ListDossiers", func() error {
			_, err := service.ListDossiers(ctx)
			return err
		}},
		{"RevisionHistory", func() error {
			_, err := service.RevisionHistory(ctx, "missing", 0, 0)
			return err
		}},
		{"AnnotationPreflight", func() error {
			_, err := service.AnnotationPreflight(ctx, "missing", application.AnnotateCommand{})
			return err
		}},
		{"SealPreflight", func() error {
			_, err := service.SealPreflight(ctx, "missing")
			return err
		}},
		{"ReadingCopy", func() error {
			_, err := service.ReadingCopy(ctx, "missing", domain.AccessPublic)
			return err
		}},
	}

	for _, query := range queries {
		if err := query.call(); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s 丢失 Repository error identity: %v", query.name, err)
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/dossiers/missing", nil)
	web.NewServer(service).ServeHTTP(recorder, request)
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode HTTP response: %v", err)
	}
	if recorder.Code != http.StatusNotFound || response.Error.Code != "not_found" {
		t.Errorf("HTTP error mapping lost cause: status=%d code=%q", recorder.Code, response.Error.Code)
	}
}
