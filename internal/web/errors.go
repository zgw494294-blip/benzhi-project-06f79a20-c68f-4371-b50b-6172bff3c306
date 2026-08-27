package web

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"benzhi-project-06f79a20-c68f-4371-b50b-6172bff3c306/internal/domain"
)

type errorBody struct {
	Error apiError `json:"error"`
}
type apiError struct {
	Code    string                   `json:"code"`
	Message string                   `json:"message"`
	Field   string                   `json:"field,omitempty"`
	Issues  []domain.ValidationIssue `json:"issues,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	message := "服务暂时无法完成请求"
	var rule *domain.RuleError
	var preflight *domain.PreflightError
	switch {
	case errors.As(err, &preflight):
		status = http.StatusUnprocessableEntity
		code = "seal_preflight_blocked"
		message = preflight.Error()
	case errors.As(err, &rule):
		status = http.StatusUnprocessableEntity
		code = rule.Code
		message = rule.Message
		if rule.Code == "revision_not_found" {
			status = http.StatusNotFound
		}
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
		code = "not_found"
		message = err.Error()
	case errors.Is(err, domain.ErrVersionConflict):
		status = http.StatusConflict
		code = "version_conflict"
		message = err.Error()
	case errors.Is(err, domain.ErrSealed):
		status = http.StatusConflict
		code = "dossier_sealed"
		message = err.Error()
	case errors.Is(err, domain.ErrInvalidTransition):
		status = http.StatusConflict
		code = "invalid_transition"
		message = err.Error()
	case errors.Is(err, domain.ErrOpenRequests):
		status = http.StatusConflict
		code = "open_requests"
		message = err.Error()
	case errors.Is(err, domain.ErrConsentMismatch):
		status = http.StatusUnprocessableEntity
		code = "consent_mismatch"
		message = err.Error()
	case errors.Is(err, domain.ErrIntegrity):
		status = http.StatusConflict
		code = "integrity_error"
		message = err.Error()
	case errors.Is(err, domain.ErrRevisionStale):
		status = http.StatusConflict
		code = "revision_stale"
		message = err.Error()
	case errors.Is(err, domain.ErrPreflightStale):
		status = http.StatusConflict
		code = "seal_preflight_stale"
		message = err.Error()
	}
	field := ""
	if rule != nil {
		field = rule.Field
	}
	issues := []domain.ValidationIssue(nil)
	if preflight != nil {
		issues = preflight.Issues
	}
	writeJSON(w, status, errorBody{apiError{Code: code, Message: message, Field: field, Issues: issues}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	mediaType, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if parseErr != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, errorBody{apiError{Code: "content_type_required", Message: "Content-Type 必须为 application/json"}})
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{apiError{Code: "invalid_json", Message: "JSON 请求体无效：" + err.Error()}})
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody{apiError{Code: "invalid_json_tail", Message: "JSON 对象后存在无效内容"}})
			return false
		}
		writeJSON(w, http.StatusBadRequest, errorBody{apiError{Code: "multiple_json_values", Message: "请求体只能包含一个 JSON 对象"}})
		return false
	}
	return true
}
