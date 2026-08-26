package web

import (
	"net/http"
	"path"
	"strings"
)

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = "/index.html"
	s.static.ServeHTTP(w, r)
}

func (s *Server) HandleAsset(w http.ResponseWriter, r *http.Request) {
	clean := path.Clean(r.URL.Path)
	if !strings.HasPrefix(clean, "/assets/") {
		notFound(w)
		return
	}
	r.URL.Path = strings.TrimPrefix(clean, "/assets")
	s.static.ServeHTTP(w, r)
}

func (s *Server) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
