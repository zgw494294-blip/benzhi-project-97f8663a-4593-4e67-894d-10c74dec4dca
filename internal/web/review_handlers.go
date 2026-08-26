package web

import (
	"net/http"

	"seed-vigor-workbench/internal/application"
)

func (s *Server) HandleReturnReview(w http.ResponseWriter, r *http.Request, id string) {
	var command application.ReviewCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, err)
		return
	}
	view, err := s.service.ReturnReview(r.Context(), id, command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) HandleResubmitReview(w http.ResponseWriter, r *http.Request, id string) {
	var command application.ResubmitCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, err)
		return
	}
	view, err := s.service.ResubmitReview(r.Context(), id, command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) HandleApproveReview(w http.ResponseWriter, r *http.Request, id string) {
	var command application.ReviewCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, err)
		return
	}
	view, err := s.service.ApproveAndArchive(r.Context(), id, command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
