package server

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	"kube-env/internal/appinfo"
	"kube-env/internal/opsapp/cluster"
	"kube-env/internal/opsapp/config"
	opsdiff "kube-env/internal/opsapp/diff"
	"kube-env/internal/opsapp/render"
	"kube-env/internal/opsapp/source"
	"kube-env/internal/opsapp/state"
	opsstatus "kube-env/internal/opsapp/status"
)

//go:embed static/* templates/*
var contentFiles embed.FS

var indexTemplate = template.Must(template.ParseFS(contentFiles, "templates/index.html"))

type Options struct {
	Info        appinfo.Info
	Config      config.Config
	StatePath   string
	WorkDir     string
	FromGit     bool
	Kubeconfig  string
	KubeContext string
}

type Server struct {
	options Options
}

type indexPageData struct {
	AppName string
}

type apiError struct {
	Error  string `json:"error"`
	Status int    `json:"status"`
}

func New(options Options) *Server {
	return &Server{options: options}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(mustSubFS(contentFiles, "static")))))
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/v1/info", s.handleInfo)
	mux.HandleFunc("GET /api/v1/envs", s.handleEnvironments)
	mux.HandleFunc("GET /api/v1/envs/{env}/resolve", s.handleResolve)
	mux.HandleFunc("GET /api/v1/envs/{env}/render-digest", s.handleRenderDigest)
	mux.HandleFunc("GET /api/v1/envs/{env}/status", s.handleStatus)
	mux.HandleFunc("POST /api/v1/envs/{env}/mark-applied", s.handleMarkApplied)
	mux.HandleFunc("GET /api/v1/envs/{env}/diff", s.handleDiff)
	mux.HandleFunc("GET /api/v1/envs/{env}/cluster", s.handleCluster)
	return mux
}

func (s *Server) ListenAndServe(addr string) error {
	server := &http.Server{Addr: addr, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	slog.Info("starting ops UI", "name", s.options.Info.Name, "addr", addr)
	return server.ListenAndServe()
}

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTemplate.Execute(w, indexPageData{AppName: s.options.Info.Name}); err != nil {
		slog.Error("render ops index failed", "error", err)
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "UP"})
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":         s.options.Info.Name,
		"version":      s.options.Info.Version,
		"commit":       s.options.Info.Commit,
		"date":         s.options.Info.Date,
		"from_git":     s.options.FromGit,
		"work_dir":     s.options.WorkDir,
		"state_path":   s.options.StatePath,
		"kubeconfig":   s.options.Kubeconfig,
		"kube_context": s.options.KubeContext,
		"server":       s.options.Config.Server,
	})
}

func (s *Server) handleEnvironments(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"environments": s.options.Config.Environments})
}

func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	prepared, err := s.prepare(r.PathValue("env"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, prepared)
}

func (s *Server) handleRenderDigest(w http.ResponseWriter, r *http.Request) {
	prepared, err := s.prepare(r.PathValue("env"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := render.RenderDigest(prepared.Environment)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	prepared, err := s.prepare(r.PathValue("env"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := opsstatus.ComputeEnvironment(prepared.Environment, state.NewStore(s.options.StatePath))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMarkApplied(w http.ResponseWriter, r *http.Request) {
	if s.options.StatePath == "" {
		writeError(w, http.StatusBadRequest, errors.New("state path is required for mark-applied"))
		return
	}
	prepared, err := s.prepare(r.PathValue("env"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	store := state.NewStore(s.options.StatePath)
	snapshotPath, err := store.SnapshotDir(prepared.Environment.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := render.RenderDigestTo(prepared.Environment, snapshotPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	applied := state.EnvironmentState{
		AppliedRevision: result.TargetRevision,
		AppliedCommit:   result.ResolvedCommit,
		AppliedDigest:   result.Digest,
		SnapshotPath:    snapshotPath,
		AppliedAt:       time.Now().UTC(),
		AppliedBy:       "kube-ops-app server mark-applied",
	}
	if err := store.SaveEnvironment(result.Environment, applied); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"environment": result.Environment, "render": result, "applied": applied})
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	prepared, err := s.prepare(r.PathValue("env"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	store := state.NewStore(s.options.StatePath)
	applied, found, err := store.Environment(prepared.Environment.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !found || applied.SnapshotPath == "" {
		writeError(w, http.StatusNotFound, fmt.Errorf("environment %q has no applied snapshot; run mark-applied first", prepared.Environment.Name))
		return
	}
	desiredDir, err := os.MkdirTemp("", "kube-ops-server-desired-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.RemoveAll(desiredDir)
	desired, err := render.RenderDigestTo(prepared.Environment, desiredDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	diff, err := opsdiff.Directories(applied.SnapshotPath, desiredDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"environment": prepared.Environment.Name, "applied": applied, "desired": desired, "diff": diff})
}

func (s *Server) handleCluster(w http.ResponseWriter, r *http.Request) {
	env, ok := s.options.Config.Environment(r.PathValue("env"))
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("environment %q not found", r.PathValue("env")))
		return
	}
	result, err := cluster.InspectNamespace(r.Context(), env.Namespace, cluster.Options{Kubeconfig: s.options.Kubeconfig, Context: s.options.KubeContext})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) prepare(name string) (source.Prepared, error) {
	env, ok := s.options.Config.Environment(name)
	if !ok {
		return source.Prepared{}, fmt.Errorf("environment %q not found", name)
	}
	return source.Prepare(env, source.Options{FromGit: s.options.FromGit, WorkDir: s.options.WorkDir})
}

func mustSubFS(source embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(source, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		slog.Error("write JSON failed", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	if err == nil {
		err = errors.New(http.StatusText(status))
	}
	writeJSON(w, status, apiError{Error: err.Error(), Status: status})
}
