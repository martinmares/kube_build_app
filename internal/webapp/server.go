package webapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
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
	mux.HandleFunc("GET /api/v1/envs/{env}/apps/{app_file}", s.handleAppDetail)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps/{app_file}/rendered", s.handleAppRendered)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps/{app_file}/vars", s.handleAppVars)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps/{app_file}/model", s.handleAppModel)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets", s.handleAssets)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets/content/{asset_path...}", s.handleAssetContent)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets/special/{special_file}/entries", s.handleSpecialEntries)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets/special/{special_file}/preflight", s.handleSpecialPreflight)
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
		"count": len(assets),
		"items": assets,
	})
}

func (s *Server) handleAppDetail(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	detail, err := s.repo.AppDetail(r.PathValue("env"), r.PathValue("app_file"))
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleAppRendered(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	rendered, err := s.repo.AppRendered(r.PathValue("env"), r.PathValue("app_file"))
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rendered)
}

func (s *Server) handleAppVars(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	vars, err := s.repo.AppVars(r.PathValue("env"), r.PathValue("app_file"))
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

func (s *Server) handleAppModel(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	model, err := s.repo.AppModel(r.PathValue("env"), r.PathValue("app_file"))
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, model)
}

func (s *Server) handleAssetContent(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	detail, err := s.repo.AssetDetail(r.PathValue("env"), r.PathValue("asset_path"))
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleSpecialEntries(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	entries, err := s.repo.SpecialEntries(r.PathValue("env"), r.PathValue("special_file"))
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleSpecialPreflight(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	env := r.PathValue("env")
	specialFile := r.PathValue("special_file")
	detail, err := s.repo.AssetDetail(env, specialFile)
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	mode := detectEncjsonMode(detail.Content)
	issues := []string{}
	if isSecuredSpecialFile(specialFile) {
		bin := s.options.EncjsonPath
		if mode == "legacy" {
			bin = s.options.EncjsonLegacyPath
		}
		if bin == "" {
			if mode == "legacy" {
				issues = append(issues, "missing ENCJSON_LEGACY_PATH or --encjson-legacy-path")
			} else {
				issues = append(issues, "missing ENCJSON_PATH or --encjson-path")
			}
		} else if info, err := os.Stat(bin); err != nil || info.IsDir() {
			issues = append(issues, "encjson binary is not accessible: "+bin)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"env":          env,
		"special_file": specialFile,
		"mode":         mode,
		"ok":           len(issues) == 0,
		"issues":       issues,
	})
}

func detectEncjsonMode(content string) string {
	if strings.Contains(content, "EncJson[@api=1.") {
		return "legacy"
	}
	if strings.Contains(content, "EncJson[@api=2.") {
		return "modern"
	}
	return "unknown"
}

func isSecuredSpecialFile(name string) bool {
	return name == "env.secured.json" || name == "assets.secured.json"
}

func statusForError(err error) int {
	if errors.Is(err, os.ErrNotExist) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
