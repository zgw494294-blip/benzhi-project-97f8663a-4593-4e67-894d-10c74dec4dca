package web

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"seed-vigor-workbench/internal/application"
)

type Server struct {
	service *application.Service
	static  http.Handler
}

func NewServer(service *application.Service, staticDir string) *Server {
	if staticDir == "" {
		staticDir = filepath.Join("web", "static")
	}
	return &Server{service: service, static: http.FileServer(http.Dir(staticDir))}
}

func (s *Server) Handler() http.Handler {
	return securityHeaders(http.HandlerFunc(s.route))
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" && r.Method == http.MethodGet {
		s.HandleIndex(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/assets/") && r.Method == http.MethodGet {
		s.HandleAsset(w, r)
		return
	}
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		s.HandleHealth(w, r)
		return
	}
	if r.URL.Path == "/api/assays" {
		switch r.Method {
		case http.MethodGet:
			s.HandleListAssays(w, r)
		case http.MethodPost:
			s.HandleCreateAssay(w, r)
		default:
			methodNotAllowed(w)
		}
		return
	}
	parts := splitPath(r.URL.Path)
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "assays" {
		id := parts[2]
		if len(parts) == 3 && r.Method == http.MethodGet {
			s.HandleGetAssay(w, r, id)
			return
		}
		if len(parts) == 4 && parts[3] == "readiness" && r.Method == http.MethodGet {
			s.HandleDraftReadiness(w, r, id)
			return
		}
		if len(parts) == 4 && r.Method == http.MethodPost {
			switch parts[3] {
			case "freeze":
				s.HandleFreezeProtocol(w, r, id)
			case "observations":
				s.HandleRecordObservation(w, r, id)
			case "draft":
				s.HandleReviseDraft(w, r, id)
			case "seal":
				s.HandleSealObservation(w, r, id)
			default:
				notFound(w)
			}
			return
		}
		if len(parts) == 5 && parts[3] == "observations" && parts[4] == "day" && r.Method == http.MethodPost {
			s.HandleRecordDailyObservations(w, r, id)
			return
		}
		if len(parts) == 6 && parts[3] == "deviations" && parts[5] == "resolve" && r.Method == http.MethodPost {
			s.HandleResolveDeviation(w, r, id, parts[4])
			return
		}
		if len(parts) == 5 && parts[3] == "review" && r.Method == http.MethodPost {
			switch parts[4] {
			case "return":
				s.HandleReturnReview(w, r, id)
			case "resubmit":
				s.HandleResubmitReview(w, r, id)
			case "approve":
				s.HandleApproveReview(w, r, id)
			default:
				notFound(w)
			}
			return
		}
	}
	notFound(w)
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func NewHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Code: "method_not_allowed", Message: "请求方法不允许"})
}
func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, errorResponse{Code: "not_found", Message: "资源不存在"})
}
