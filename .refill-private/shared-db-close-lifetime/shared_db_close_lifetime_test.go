package shared_db_close_lifetime_test

import (
	"context"
	"path/filepath"
	"testing"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
)

func TestClosingOneRepositoryKeepsPeerHandleUsable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared.db")

	first, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("open first repository: %v", err)
	}
	second, err := store.Open(ctx, path)
	if err != nil {
		_ = first.Close()
		t.Fatalf("open second repository: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	if err = first.Close(); err != nil {
		t.Fatalf("close first repository: %v", err)
	}
	items, err := second.List(ctx)
	if err != nil {
		t.Fatalf("peer repository became unusable after sibling Close: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("new database unexpectedly contains %d dossiers", len(items))
	}
}
