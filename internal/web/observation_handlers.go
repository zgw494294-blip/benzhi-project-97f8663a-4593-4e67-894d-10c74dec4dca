package web

import (
	"net/http"

	"seed-vigor-workbench/internal/application"
)

func (s *Server) HandleRecordObservation(w http.ResponseWriter, r *http.Request, id string) {
	var command application.ObservationCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, err)
		return
	}
	view, err := s.service.RecordObservation(r.Context(), id, command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) HandleRecordDailyObservations(w http.ResponseWriter, r *http.Request, id string) {
	var command application.DailyObservationCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, err)
		return
	}
	view, err := s.service.RecordDailyObservations(r.Context(), id, command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) HandleSealObservation(w http.ResponseWriter, r *http.Request, id string) {
	var command application.RevisionCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, err)
		return
	}
	view, err := s.service.SealObservation(r.Context(), id, command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) HandleResolveDeviation(w http.ResponseWriter, r *http.Request, assayID, deviationID string) {
	var command application.ResolveDeviationCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, err)
		return
	}
	view, err := s.service.ResolveDeviation(r.Context(), assayID, deviationID, command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
