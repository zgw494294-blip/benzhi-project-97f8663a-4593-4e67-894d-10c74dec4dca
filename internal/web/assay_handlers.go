package web

import (
	"net/http"

	"seed-vigor-workbench/internal/application"
)

func (s *Server) HandleListAssays(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListAssays(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) HandleCreateAssay(w http.ResponseWriter, r *http.Request) {
	var command application.CreateAssayCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, err)
		return
	}
	assay, err := s.service.CreateAssay(r.Context(), command)
	if err != nil {
		writeError(w, err)
		return
	}
	view, err := s.service.GetAssay(r.Context(), assay.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *Server) HandleGetAssay(w http.ResponseWriter, r *http.Request, id string) {
	view, err := s.service.GetAssay(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) HandleFreezeProtocol(w http.ResponseWriter, r *http.Request, id string) {
	var command application.RevisionCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, err)
		return
	}
	view, err := s.service.FreezeProtocol(r.Context(), id, command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) HandleDraftReadiness(w http.ResponseWriter, r *http.Request, id string) {
	result, err := s.service.DraftReadiness(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) HandleReviseDraft(w http.ResponseWriter, r *http.Request, id string) {
	var command application.ReviseDraftCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, err)
		return
	}
	view, err := s.service.ReviseDraft(r.Context(), id, command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
