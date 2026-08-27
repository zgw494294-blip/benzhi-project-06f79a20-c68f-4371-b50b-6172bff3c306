package web

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"sync/atomic"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/application"
)

type Server struct {
	app             *application.Service
	mux             *http.ServeMux
	requestSequence atomic.Uint64
}

func NewServer(app *application.Service) http.Handler {
	s := &Server{app: app, mux: http.NewServeMux()}
	s.routes()
	return s.recoverRequests(s.securityHeaders(s.mux))
}

func (s *Server) routes() {
	static, _ := fs.Sub(assets, "static")
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(static))))
	s.mux.HandleFunc("GET /", s.WorkbenchPage)
	s.mux.HandleFunc("GET /reading/{dossierId}", s.ReadingPage)
	s.mux.HandleFunc("GET /healthz", s.Health)
	s.mux.HandleFunc("GET /api/dossiers", s.ListDossiers)
	s.mux.HandleFunc("POST /api/dossiers", s.CreateDossier)
	s.mux.HandleFunc("GET /api/dossiers/{dossierId}", s.GetDossier)
	s.mux.HandleFunc("PATCH /api/dossiers/{dossierId}", s.ReviseDossier)
	s.mux.HandleFunc("POST /api/dossiers/{dossierId}/revisions", s.SubmitRevision)
	s.mux.HandleFunc("GET /api/dossiers/{dossierId}/revisions/history", s.RevisionHistory)
	s.mux.HandleFunc("POST /api/dossiers/{dossierId}/annotations", s.Annotate)
	s.mux.HandleFunc("POST /api/dossiers/{dossierId}/annotations/preflight", s.AnnotationPreflight)
	s.mux.HandleFunc("POST /api/dossiers/{dossierId}/reviews", s.Review)
	s.mux.HandleFunc("POST /api/dossiers/{dossierId}/reviews/draft", s.SaveReviewDraft)
	s.mux.HandleFunc("POST /api/dossiers/{dossierId}/decisions/{decisionId}/resolve", s.Resolve)
	s.mux.HandleFunc("POST /api/dossiers/{dossierId}/seal", s.Seal)
	s.mux.HandleFunc("GET /api/dossiers/{dossierId}/seal/preflight", s.SealPreflight)
	s.mux.HandleFunc("GET /api/dossiers/{dossierId}/reading", s.ReadingCopy)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := fmt.Sprintf("req-%08d", s.requestSequence.Add(1))
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("HTTP 请求异常：%v\n%s", recovered, debug.Stack())
				writeJSON(w, http.StatusInternalServerError, errorBody{apiError{Code: "internal_error", Message: "服务暂时无法完成请求"}})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
