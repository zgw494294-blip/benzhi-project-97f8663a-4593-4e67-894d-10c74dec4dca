package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"seed-vigor-workbench/internal/domain"
	"seed-vigor-workbench/internal/persistence"
)

type errorResponse struct {
	Code            string                   `json:"code"`
	Message         string                   `json:"message"`
	Field           string                   `json:"field,omitempty"`
	CurrentRevision int64                    `json:"current_revision,omitempty"`
	Errors          []domain.ValidationError `json:"errors,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	var validation domain.ValidationError
	var validations domain.ValidationErrors
	var conflict domain.ConflictError
	switch {
	case errors.As(err, &validations):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Code: "validation_error", Message: validations.Error(), Errors: validations.Issues})
	case errors.As(err, &validation):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Code: "validation_error", Message: validation.Message, Field: validation.Field})
	case errors.As(err, &conflict):
		writeJSON(w, http.StatusConflict, errorResponse{Code: "revision_conflict", Message: conflict.Error(), CurrentRevision: conflict.Current})
	case errors.Is(err, persistence.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Code: "not_found", Message: "检验批次不存在"})
	default:
		writeJSON(w, http.StatusBadRequest, errorResponse{Code: "request_rejected", Message: err.Error()})
	}
}
