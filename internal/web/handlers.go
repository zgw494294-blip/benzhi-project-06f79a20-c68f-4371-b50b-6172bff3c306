package web

import (
	"net/http"
	"strconv"
	"strings"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/application"
	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

func (s *Server) ListDossiers(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.ListDossiers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dossiers": items})
}

func (s *Server) ReviseDossier(w http.ResponseWriter, r *http.Request) {
	var cmd application.ReviseDossierCommand
	if !decodeJSON(w, r, &cmd) {
		return
	}
	view, err := s.app.ReviseDossier(r.Context(), r.PathValue("dossierId"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) CreateDossier(w http.ResponseWriter, r *http.Request) {
	var cmd application.CreateDossierCommand
	if !decodeJSON(w, r, &cmd) {
		return
	}
	view, err := s.app.CreateDossier(r.Context(), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", "/api/dossiers/"+view.Snapshot.Dossier.DossierID)
	writeJSON(w, http.StatusCreated, view)
}

func (s *Server) GetDossier(w http.ResponseWriter, r *http.Request) {
	view, err := s.app.GetDossier(r.Context(), r.PathValue("dossierId"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) SubmitRevision(w http.ResponseWriter, r *http.Request) {
	var cmd application.SubmitRevisionCommand
	if !decodeJSON(w, r, &cmd) {
		return
	}
	view, err := s.app.SubmitRevision(r.Context(), r.PathValue("dossierId"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) RevisionHistory(w http.ResponseWriter, r *http.Request) {
	from, to, err := comparisonNumbers(r)
	if err != nil {
		writeError(w, err)
		return
	}
	history, err := s.app.RevisionHistory(r.Context(), r.PathValue("dossierId"), from, to)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func comparisonNumbers(r *http.Request) (int, int, error) {
	fromRaw, toRaw := strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("to"))
	if fromRaw == "" && toRaw == "" {
		return 0, 0, nil
	}
	if fromRaw == "" || toRaw == "" {
		return 0, 0, domain.Invalid("comparison_pair_required", "比较参数 from 和 to 必须同时提供", "from")
	}
	from, fromErr := strconv.Atoi(fromRaw)
	to, toErr := strconv.Atoi(toRaw)
	if fromErr != nil || toErr != nil || from < 1 || to < 1 {
		return 0, 0, domain.Invalid("invalid_revision_number", "比较修订编号必须为正整数", "from")
	}
	return from, to, nil
}

func (s *Server) Annotate(w http.ResponseWriter, r *http.Request) {
	var cmd application.AnnotateCommand
	if !decodeJSON(w, r, &cmd) {
		return
	}
	view, err := s.app.Annotate(r.Context(), r.PathValue("dossierId"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) AnnotationPreflight(w http.ResponseWriter, r *http.Request) {
	var cmd application.AnnotateCommand
	if !decodeJSON(w, r, &cmd) {
		return
	}
	result, err := s.app.AnnotationPreflight(r.Context(), r.PathValue("dossierId"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) Review(w http.ResponseWriter, r *http.Request) {
	var cmd application.ReviewCommand
	if !decodeJSON(w, r, &cmd) {
		return
	}
	view, err := s.app.Review(r.Context(), r.PathValue("dossierId"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) SaveReviewDraft(w http.ResponseWriter, r *http.Request) {
	var cmd application.SaveReviewDraftCommand
	if !decodeJSON(w, r, &cmd) {
		return
	}
	view, err := s.app.SaveReviewDraft(r.Context(), r.PathValue("dossierId"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) Resolve(w http.ResponseWriter, r *http.Request) {
	var cmd application.ResolveCommand
	if !decodeJSON(w, r, &cmd) {
		return
	}
	view, err := s.app.Resolve(r.Context(), r.PathValue("dossierId"), r.PathValue("decisionId"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) Seal(w http.ResponseWriter, r *http.Request) {
	var cmd application.SealCommand
	if !decodeJSON(w, r, &cmd) {
		return
	}
	view, err := s.app.Seal(r.Context(), r.PathValue("dossierId"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) SealPreflight(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.SealPreflight(r.Context(), r.PathValue("dossierId"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) ReadingCopy(w http.ResponseWriter, r *http.Request) {
	level, err := domain.ParseAccessLevel(strings.TrimSpace(r.URL.Query().Get("level")))
	if err != nil {
		writeError(w, err)
		return
	}
	copy, err := s.app.ReadingCopy(r.Context(), r.PathValue("dossierId"), level)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, copy)
}
