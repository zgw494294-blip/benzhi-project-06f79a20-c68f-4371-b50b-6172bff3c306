package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/application"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/store"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	repo, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	return NewServer(application.NewService(repo))
}

func TestWorkbenchAndStrictJSON(t *testing.T) {
	handler := testHandler(t)
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest("GET", "/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "<body>") {
		t.Fatalf("工作台页面无效: %d", page.Code)
	}
	bad := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/dossiers", strings.NewReader(`{"title":"档案","unknown":true}`))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(bad, req)
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "invalid_json") {
		t.Fatalf("严格解码未拒绝未知字段: %d %s", bad.Code, bad.Body.String())
	}
}

func TestJSONContentTypeRequired(t *testing.T) {
	handler := testHandler(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "/api/dossiers", strings.NewReader(`{}`)))
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("状态码=%d", response.Code)
	}
}
