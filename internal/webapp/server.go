package webapp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"kube-env/internal/appinfo"
	"kube-env/internal/repository"
)

type Server struct {
	info    appinfo.Info
	repo    *repository.Repository
	options Options
}

type Options struct {
	BasePath          string
	ReadOnly          bool
	EncjsonPath       string
	EncjsonLegacyPath string
	EncjsonKeydir     string
}

func NewServer(info appinfo.Info, repo *repository.Repository, options ...Options) *Server {
	opts := Options{BasePath: "/", ReadOnly: true}
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.BasePath == "" {
		opts.BasePath = "/"
	}
	return &Server{info: info, repo: repo, options: opts}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/v1/info", s.handleInfo)
	mux.HandleFunc("GET /api/v1/envs", s.handleEnvironments)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps", s.handleApps)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets", s.handleAssets)
	return mux
}

func (s *Server) ListenAndServe(addr string) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	slog.Info("starting web UI", "name", s.info.Name, "addr", addr)
	return server.ListenAndServe()
}

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>%s</title>
</head>
<body>
  <main>
    <h1>%s</h1>
    <p>Go web UI skeleton is running.</p>
  </main>
</body>
</html>
`, s.info.Name, s.info.Name)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "UP"})
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":      s.info.Name,
		"version":   s.info.Version,
		"commit":    s.info.Commit,
		"date":      s.info.Date,
		"base_path": s.options.BasePath,
		"read_only": s.options.ReadOnly,
	})
}

func (s *Server) handleEnvironments(w http.ResponseWriter, _ *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	environments, err := s.repo.Environments()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"root":  s.repo.Root(),
		"items": environments,
	})
}

func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	env := r.PathValue("env")
	apps, err := s.repo.Apps(env)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"env":   env,
		"items": apps,
	})
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	env := r.PathValue("env")
	assets, err := s.repo.Assets(env)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"env":   env,
		"items": assets,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
