package webapp

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"kube-env/internal/appinfo"
	"kube-env/internal/buildapp"
	"kube-env/internal/opsapp/cluster"
	"kube-env/internal/repository"
)

//go:embed static/* templates/*
var contentFiles embed.FS

var indexTemplate = template.Must(template.ParseFS(contentFiles, "templates/index.html"))

type indexPageData struct {
	AppName string
}

type Server struct {
	info             appinfo.Info
	repo             *repository.Repository
	options          Options
	buildPreviewMu   sync.Mutex
	buildPreviewDirs map[string]buildPreviewSnapshot
}

type apiError struct {
	Error  string `json:"error"`
	Status int    `json:"status"`
	Code   string `json:"code,omitempty"`
}

type buildPreviewFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

type buildPreview struct {
	ID        string                `json:"id"`
	Env       string                `json:"env"`
	Files     []buildPreviewFile    `json:"files"`
	Events    []buildapp.BuildEvent `json:"events"`
	Totals    buildPreviewTotals    `json:"totals"`
	ExpiresAt time.Time             `json:"expires_at"`
}

type buildPreviewTotals struct {
	Files       int `json:"files"`
	Deployments int `json:"deployments"`
	Services    int `json:"services"`
	Assets      int `json:"assets"`
}

type buildPreviewSnapshot struct {
	Env       string
	Root      string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type buildPreviewContent struct {
	Path        string `json:"path"`
	SizeBytes   int64  `json:"size_bytes"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
	Binary      bool   `json:"binary"`
	Truncated   bool   `json:"truncated"`
}

type Options struct {
	BasePath          string
	ReadOnly          bool
	EncjsonPath       string
	EncjsonLegacyPath string
	EncjsonKeydir     string
	ClusterStatus     bool
	Kubeconfig        string
	KubeContext       string
}

func NewServer(info appinfo.Info, repo *repository.Repository, options ...Options) *Server {
	opts := Options{BasePath: "/", ReadOnly: true}
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.BasePath == "" {
		opts.BasePath = "/"
	}
	return &Server{info: info, repo: repo, options: opts, buildPreviewDirs: map[string]buildPreviewSnapshot{}}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(mustSubFS(contentFiles, "static")))))
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/v1/info", s.handleInfo)
	mux.HandleFunc("GET /api/v1/git/status", s.handleGitStatus)
	mux.HandleFunc("GET /api/v1/git/diff/{file_path...}", s.handleGitDiff)
	mux.HandleFunc("POST /api/v1/git/restore/{file_path...}", s.handleGitRestore)
	mux.HandleFunc("POST /api/v1/git/commit", s.handleGitCommit)
	mux.HandleFunc("GET /api/v1/envs", s.handleEnvironments)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps", s.handleApps)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps/{app_file}", s.handleAppDetail)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps/{app_file}/rendered", s.handleAppRendered)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps/{app_file}/vars", s.handleAppVars)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/vars", s.handleAppVarsUpdate)
	mux.HandleFunc("GET /api/v1/envs/{env}/defaults", s.handleDefaults)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/defaults/vars", s.handleDefaultsVarsUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/defaults/container-envs", s.handleDefaultsContainerEnvsUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/replicas", s.handleAppReplicasUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/autoscaling", s.handleAppAutoscalingUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/resources", s.handleAppContainerResourcesUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/envs", s.handleAppContainerEnvsUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/runtime/java", s.handleAppContainerRuntimeUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/ports", s.handleAppContainerPortsUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/probes", s.handleAppContainerProbesUpdate)
	mux.HandleFunc("POST /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/probes/fix-legacy", s.handleAppContainerLegacyProbesFix)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps/{app_file}/model", s.handleAppModel)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets", s.handleAssets)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets/content/{asset_path...}", s.handleAssetContent)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets/special/{special_file}/entries", s.handleSpecialEntries)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets/special/{special_file}/decrypted-entries", s.handleSpecialDecryptedEntries)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/assets/special/{special_file}/entries", s.handleSpecialEntriesUpdate)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets/special/{special_file}/preflight", s.handleSpecialPreflight)
	mux.HandleFunc("POST /api/v1/envs/{env}/validate", s.handleBuildValidate)
	mux.HandleFunc("POST /api/v1/envs/{env}/summary", s.handleBuildSummary)
	mux.HandleFunc("POST /api/v1/envs/{env}/inventory", s.handleBuildInventory)
	mux.HandleFunc("POST /api/v1/envs/{env}/preview", s.handleBuildPreview)
	mux.HandleFunc("GET /api/v1/envs/{env}/preview/{preview_id}/content/{file_path...}", s.handleBuildPreviewContent)
	mux.HandleFunc("GET /api/v1/envs/{env}/cluster", s.handleClusterStatus)
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
	if err := indexTemplate.Execute(w, indexPageData{AppName: s.info.Name}); err != nil {
		slog.Error("render index failed", "error", err)
	}
}

func mustSubFS(source embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(source, dir)
	if err != nil {
		panic(err)
	}
	return sub
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
		"cluster_status": map[string]any{
			"enabled":      s.options.ClusterStatus,
			"kubeconfig":   s.options.Kubeconfig != "",
			"kube_context": s.options.KubeContext,
		},
	})
}

func (s *Server) handleGitStatus(w http.ResponseWriter, _ *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	writeJSON(w, http.StatusOK, s.repo.GitStatus())
}

func (s *Server) handleGitDiff(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	diff := s.repo.GitDiff(r.PathValue("file_path"))
	if !diff.Available && diff.Error != "" {
		writeJSON(w, http.StatusBadRequest, diff)
		return
	}
	writeJSON(w, http.StatusOK, diff)
}

func (s *Server) handleGitRestore(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	var payload struct {
		DeleteUntracked bool `json:"delete_untracked"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	result, err := s.repo.GitRestore(r.PathValue("file_path"), payload.DeleteUntracked)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGitCommit(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	var payload struct {
		Paths   []string `json:"paths"`
		Message string   `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.repo.GitCommitSelected(payload.Paths, payload.Message)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
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

func (s *Server) handleAppVarsUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	var payload struct {
		ExpectedHash string               `json:"expected_hash"`
		Items        []repository.VarItem `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	vars, err := s.repo.UpdateAppVars(r.PathValue("env"), r.PathValue("app_file"), payload.Items, payload.ExpectedHash)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

func (s *Server) handleDefaults(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	defaults, err := s.repo.Defaults(r.PathValue("env"))
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, defaults)
}

func (s *Server) handleDefaultsVarsUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	var payload struct {
		ExpectedHash string               `json:"expected_hash"`
		Items        []repository.VarItem `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defaults, err := s.repo.UpdateDefaultsVars(r.PathValue("env"), payload.Items, payload.ExpectedHash)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, defaults)
}

func (s *Server) handleDefaultsContainerEnvsUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	var payload struct {
		ExpectedHash string                               `json:"expected_hash"`
		Groups       []repository.ContainerEnvGroupUpdate `json:"groups"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defaults, err := s.repo.UpdateDefaultsContainerEnvs(r.PathValue("env"), payload.Groups, payload.ExpectedHash)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, defaults)
}

func (s *Server) handleAppReplicasUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	var payload struct {
		ExpectedHash string `json:"expected_hash"`
		Replicas     int    `json:"replicas"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	replicas, err := s.repo.UpdateAppReplicas(r.PathValue("env"), r.PathValue("app_file"), payload.Replicas, payload.ExpectedHash)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, replicas)
}

func (s *Server) handleAppAutoscalingUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	var payload struct {
		ExpectedHash string                       `json:"expected_hash"`
		Autoscaling  repository.AutoscalingUpdate `json:"autoscaling"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	autoscaling, err := s.repo.UpdateAppAutoscaling(r.PathValue("env"), r.PathValue("app_file"), payload.Autoscaling, payload.ExpectedHash)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, autoscaling)
}

func (s *Server) handleAppContainerResourcesUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	containerIndex, err := strconv.Atoi(r.PathValue("container_index"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "container index must be an integer")
		return
	}
	var payload struct {
		ExpectedHash string                    `json:"expected_hash"`
		Resources    repository.ResourceUpdate `json:"resources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resources, err := s.repo.UpdateAppContainerResources(r.PathValue("env"), r.PathValue("app_file"), containerIndex, payload.Resources, payload.ExpectedHash)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resources)
}

func (s *Server) handleAppContainerEnvsUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	containerIndex, err := strconv.Atoi(r.PathValue("container_index"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "container index must be an integer")
		return
	}
	var payload struct {
		ExpectedHash string               `json:"expected_hash"`
		Items        []repository.VarItem `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	vars, err := s.repo.UpdateAppContainerEnvs(r.PathValue("env"), r.PathValue("app_file"), containerIndex, payload.Items, payload.ExpectedHash)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

func (s *Server) handleAppContainerRuntimeUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	containerIndex, err := strconv.Atoi(r.PathValue("container_index"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "container index must be an integer")
		return
	}
	var payload struct {
		ExpectedHash string                       `json:"expected_hash"`
		Runtime      repository.JavaRuntimeUpdate `json:"runtime"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	runtime, err := s.repo.UpdateAppContainerRuntime(r.PathValue("env"), r.PathValue("app_file"), containerIndex, payload.Runtime, payload.ExpectedHash)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runtime)
}

func (s *Server) handleAppContainerPortsUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	containerIndex, err := strconv.Atoi(r.PathValue("container_index"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "container index must be an integer")
		return
	}
	var payload struct {
		ExpectedHash string                  `json:"expected_hash"`
		Ports        []repository.PortUpdate `json:"ports"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ports, err := s.repo.UpdateAppContainerPorts(r.PathValue("env"), r.PathValue("app_file"), containerIndex, payload.Ports, payload.ExpectedHash)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ports)
}

func (s *Server) handleAppContainerProbesUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	containerIndex, err := strconv.Atoi(r.PathValue("container_index"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "container index must be an integer")
		return
	}
	var payload struct {
		ExpectedHash string                 `json:"expected_hash"`
		Probes       repository.ProbeUpdate `json:"probes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	probes, err := s.repo.UpdateAppContainerProbes(r.PathValue("env"), r.PathValue("app_file"), containerIndex, payload.Probes, payload.ExpectedHash)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, probes)
}

func (s *Server) handleAppContainerLegacyProbesFix(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	containerIndex, err := strconv.Atoi(r.PathValue("container_index"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "container index must be an integer")
		return
	}
	var payload struct {
		ExpectedHash string `json:"expected_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	probes, err := s.repo.FixAppContainerLegacyProbes(r.PathValue("env"), r.PathValue("app_file"), containerIndex, payload.ExpectedHash)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, probes)
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

func (s *Server) handleSpecialDecryptedEntries(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	env := r.PathValue("env")
	specialFile := r.PathValue("special_file")
	if !isSecuredSpecialFile(specialFile) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "special file is not secured"})
		return
	}
	detail, err := s.repo.AssetDetail(env, specialFile)
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	decrypted, err := s.decryptSpecialFile(detail.Path, detail.Content)
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	editable := specialFile == "env.secured.json"
	entries, err := specialEntriesFromContent(env, specialFile, detail.ContentHash, detail.IsDirty, string(decrypted), editable)
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	warning := "decrypted values; saving re-encrypts the file through EncJson"
	if !editable {
		warning = "decrypted read-only preview; write/encrypt flow is not enabled yet"
	}
	entries.Warning = &warning
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleSpecialEntriesUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if s.options.ReadOnly {
		writeError(w, http.StatusForbidden, "mutating API endpoints are disabled")
		return
	}
	var payload struct {
		ExpectedHash string                    `json:"expected_hash"`
		Entries      []repository.SpecialEntry `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.PathValue("special_file") == "env.secured.json" {
		entries, err := s.updateSecuredEnvEntries(r.PathValue("env"), payload.Entries, payload.ExpectedHash)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, entries)
		return
	}
	entries, err := s.repo.UpdateSpecialEntries(r.PathValue("env"), r.PathValue("special_file"), payload.Entries, payload.ExpectedHash)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) updateSecuredEnvEntries(env string, entries []repository.SpecialEntry, expectedHash string) (repository.SpecialEntries, error) {
	detail, err := s.repo.AssetDetail(env, "env.secured.json")
	if err != nil {
		return repository.SpecialEntries{}, err
	}
	if expectedHash != "" && expectedHash != detail.ContentHash {
		return repository.SpecialEntries{}, repository.NewConflictError("special file changed before save; refresh and apply the edit again")
	}
	decrypted, err := s.decryptSpecialFile(detail.Path, detail.Content)
	if err != nil {
		return repository.SpecialEntries{}, err
	}
	decryptedEntries, err := specialEntriesFromContent(env, "env.secured.json", detail.ContentHash, detail.IsDirty, string(decrypted), true)
	if err != nil {
		return repository.SpecialEntries{}, err
	}
	encryptedEntries, err := specialEntriesFromContent(env, "env.secured.json", detail.ContentHash, detail.IsDirty, detail.Content, false)
	if err != nil {
		return repository.SpecialEntries{}, err
	}
	mixedEntries := securedMixedEntries(entries, decryptedEntries.Entries, encryptedEntries.Entries)
	mixedContent, err := repository.PatchSpecialEntriesContent(detail.Content, "environment", mixedEntries)
	if err != nil {
		return repository.SpecialEntries{}, err
	}
	encrypted, err := s.encryptSpecialFile(detail.Path, detail.Content, mixedContent)
	if err != nil {
		return repository.SpecialEntries{}, err
	}
	finalEntries, err := specialEntriesFromContent(env, "env.secured.json", detail.ContentHash, detail.IsDirty, string(encrypted), false)
	if err != nil {
		return repository.SpecialEntries{}, err
	}
	return s.repo.UpdateSecuredSpecialEntriesEncrypted(env, "env.secured.json", finalEntries.Entries, expectedHash)
}

func securedMixedEntries(submitted []repository.SpecialEntry, decrypted []repository.SpecialEntry, encrypted []repository.SpecialEntry) []repository.SpecialEntry {
	decryptedByKey := map[string]repository.SpecialEntry{}
	for _, entry := range decrypted {
		decryptedByKey[entry.Key] = entry
	}
	encryptedByKey := map[string]repository.SpecialEntry{}
	for _, entry := range encrypted {
		encryptedByKey[entry.Key] = entry
	}
	mixed := make([]repository.SpecialEntry, 0, len(submitted))
	for _, entry := range submitted {
		if decryptedEntry, ok := decryptedByKey[entry.Key]; ok && specialEntriesEqual(entry, decryptedEntry) {
			if encryptedEntry, ok := encryptedByKey[entry.Key]; ok {
				mixed = append(mixed, encryptedEntry)
				continue
			}
		}
		mixed = append(mixed, entry)
	}
	return mixed
}

func specialEntriesEqual(a repository.SpecialEntry, b repository.SpecialEntry) bool {
	return strings.TrimSpace(a.ValueType) == strings.TrimSpace(b.ValueType) && a.ValueText == b.ValueText
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
	decryptOK := false
	if isSecuredSpecialFile(specialFile) {
		bin := s.encjsonBinForMode(mode)
		if bin == "" {
			if mode == "legacy" {
				issues = append(issues, "missing ENCJSON_LEGACY_PATH or --encjson-legacy-path")
			} else {
				issues = append(issues, "missing ENCJSON_PATH or --encjson-path")
			}
		} else if info, err := os.Stat(bin); err != nil || info.IsDir() {
			issues = append(issues, "encjson binary is not accessible: "+bin)
		} else if _, err := s.decryptSpecialFile(detail.Path, detail.Content); err != nil {
			issues = append(issues, err.Error())
		} else {
			decryptOK = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"env":          env,
		"special_file": specialFile,
		"mode":         mode,
		"ok":           len(issues) == 0,
		"decrypt_ok":   decryptOK,
		"issues":       issues,
	})
}

func (s *Server) handleBuildValidate(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	env := r.PathValue("env")
	err := buildapp.Validate(s.buildOptions(env))
	writeJSON(w, http.StatusOK, map[string]any{
		"env":     env,
		"ok":      err == nil,
		"message": validationMessage(err),
	})
}

func (s *Server) handleBuildSummary(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	summary, err := buildapp.ResourceSummary(s.buildOptions(r.PathValue("env")))
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleBuildInventory(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	inventory, err := buildapp.Inventory(s.buildOptions(r.PathValue("env")))
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, inventory)
}

func (s *Server) handleBuildPreview(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	env := r.PathValue("env")
	tmpDir, err := os.MkdirTemp("", "kube-edit-build-preview-*")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	opts := s.buildOptions(env)
	opts.Target = tmpDir
	result, err := buildapp.Build(opts)
	if err != nil {
		os.RemoveAll(tmpDir)
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	files, err := previewFiles(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	id, err := randomID()
	if err != nil {
		os.RemoveAll(tmpDir)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	now := time.Now()
	expiresAt := now.Add(30 * time.Minute)
	s.rememberBuildPreview(id, buildPreviewSnapshot{Env: env, Root: tmpDir, CreatedAt: now, ExpiresAt: expiresAt})
	writeJSON(w, http.StatusOK, buildPreview{
		ID:        id,
		Env:       env,
		Files:     files,
		Events:    result.Events,
		ExpiresAt: expiresAt,
		Totals: buildPreviewTotals{
			Files:       len(files),
			Deployments: len(result.Deployments) + len(result.Budgets) + len(result.Autoscaling),
			Services:    len(result.Services) + len(result.Externals),
			Assets:      len(result.Assets),
		},
	})
}

func (s *Server) handleBuildPreviewContent(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository root is not configured"})
		return
	}
	env := r.PathValue("env")
	id := r.PathValue("preview_id")
	filePath := r.PathValue("file_path")
	snapshot, ok := s.buildPreviewSnapshot(id, env)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "build preview snapshot not found or expired"})
		return
	}
	fullPath, err := safeJoin(snapshot.Root, filePath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	if info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "preview path is a directory"})
		return
	}
	const maxPreviewContentBytes = 1024 * 1024
	data, err := os.ReadFile(fullPath)
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	truncated := false
	if len(data) > maxPreviewContentBytes {
		data = data[:maxPreviewContentBytes]
		truncated = true
	}
	contentType := http.DetectContentType(data)
	binary := !utf8.Valid(data)
	content := ""
	if !binary {
		content = string(data)
	}
	writeJSON(w, http.StatusOK, buildPreviewContent{
		Path:        filepath.ToSlash(filepath.Clean(filePath)),
		SizeBytes:   info.Size(),
		Content:     content,
		ContentType: contentType,
		Binary:      binary,
		Truncated:   truncated,
	})
}

func (s *Server) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository root is not configured")
		return
	}
	if !s.options.ClusterStatus {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	namespace, err := s.environmentNamespace(r.PathValue("env"))
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	summary, err := cluster.InspectNamespace(r.Context(), namespace, cluster.Options{Kubeconfig: s.options.Kubeconfig, Context: s.options.KubeContext})
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "updated_at": time.Now(), "cluster": summary})
}

func (s *Server) environmentNamespace(env string) (string, error) {
	env = strings.TrimSpace(env)
	if env == "" {
		return "", errors.New("environment is required")
	}
	envDir := filepath.Join(s.repo.Root(), env)
	if !strings.HasPrefix(envDir, s.repo.Root()+string(os.PathSeparator)) {
		return "", errors.New("invalid environment path")
	}
	content, err := os.ReadFile(filepath.Join(envDir, "env.unsecured.json"))
	if err != nil {
		return "", err
	}
	var payload struct {
		Environment map[string]any `json:"environment"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		return "", err
	}
	namespace := strings.TrimSpace(fmt.Sprint(payload.Environment["NAMESPACE"]))
	if namespace == "" || namespace == "<nil>" {
		return "", fmt.Errorf("environment %q does not define NAMESPACE in env.unsecured.json", env)
	}
	return namespace, nil
}

func previewFiles(root string) ([]buildPreviewFile, error) {
	files := []buildPreviewFile{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, buildPreviewFile{Path: filepath.ToSlash(rel), SizeBytes: info.Size()})
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func (s *Server) decryptSpecialFile(path string, content string) ([]byte, error) {
	mode := detectEncjsonMode(content)
	bin := s.encjsonBinForMode(mode)
	if bin == "" {
		if mode == "legacy" {
			return nil, errors.New("missing ENCJSON_LEGACY_PATH or --encjson-legacy-path")
		}
		return nil, errors.New("missing ENCJSON_PATH or --encjson-path")
	}
	args := []string{"decrypt"}
	if keydir := strings.TrimSpace(s.options.EncjsonKeydir); keydir != "" {
		args = append(args, "-k", keydir)
	}
	args = append(args, "-f", path)
	cmd := exec.Command(bin, args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return nil, fmt.Errorf("EncJson decrypt failed: %w: %s", err, stderr)
			}
		}
		return nil, fmt.Errorf("EncJson decrypt failed: %w", err)
	}
	return out, nil
}

func (s *Server) encryptSpecialFile(path string, originalContent string, plainContent string) ([]byte, error) {
	mode := detectEncjsonMode(originalContent)
	bin := s.encjsonBinForMode(mode)
	if bin == "" {
		if mode == "legacy" {
			return nil, errors.New("missing ENCJSON_LEGACY_PATH or --encjson-legacy-path")
		}
		return nil, errors.New("missing ENCJSON_PATH or --encjson-path")
	}
	tempDir, err := os.MkdirTemp("", "kube-edit-encjson-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	tempPath := filepath.Join(tempDir, filepath.Base(path))
	if err := os.WriteFile(tempPath, []byte(plainContent), 0o600); err != nil {
		return nil, err
	}
	args := []string{"encrypt"}
	if keydir := strings.TrimSpace(s.options.EncjsonKeydir); keydir != "" {
		args = append(args, "-k", keydir)
	}
	args = append(args, "-f", tempPath)
	cmd := exec.Command(bin, args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return nil, fmt.Errorf("EncJson encrypt failed: %w: %s", err, stderr)
			}
		}
		return nil, fmt.Errorf("EncJson encrypt failed: %w", err)
	}
	if len(strings.TrimSpace(string(out))) > 0 {
		return out, nil
	}
	return os.ReadFile(tempPath)
}

func (s *Server) encjsonBinForMode(mode string) string {
	if mode == "legacy" {
		return strings.TrimSpace(s.options.EncjsonLegacyPath)
	}
	return strings.TrimSpace(s.options.EncjsonPath)
}

func specialEntriesFromContent(env string, specialFile string, contentHash string, isDirty bool, content string, editable bool) (repository.SpecialEntries, error) {
	rootKey := "environment"
	if specialFile == "assets.secured.json" || specialFile == "assets.unsecured.json" {
		rootKey = "assets"
	}
	out := repository.SpecialEntries{Env: env, SpecialFile: specialFile, ContentHash: contentHash, Editable: editable, IsDirty: isDirty}
	if rootKey == "environment" {
		entries, err := environmentEntriesFromContent(content)
		if err != nil {
			return repository.SpecialEntries{}, err
		}
		out.Entries = entries
		return out, nil
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return repository.SpecialEntries{}, err
	}
	rawSection, ok := root[rootKey].(map[string]any)
	if !ok {
		return repository.SpecialEntries{}, fmt.Errorf("missing object at .%s", rootKey)
	}
	keys := make([]string, 0, len(rawSection))
	for key := range rawSection {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out.Entries = append(out.Entries, specialEntryFromValue(key, rawSection[key], rootKey))
	}
	return out, nil
}

func environmentEntriesFromContent(content string) ([]repository.SpecialEntry, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("expected JSON object")
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("expected JSON object key")
		}
		if key != "environment" {
			var ignored any
			if err := decoder.Decode(&ignored); err != nil {
				return nil, err
			}
			continue
		}
		token, err = decoder.Token()
		if err != nil {
			return nil, err
		}
		if delim, ok := token.(json.Delim); !ok || delim != '{' {
			return nil, errors.New("missing object at .environment")
		}
		var entries []repository.SpecialEntry
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			entryKey, ok := token.(string)
			if !ok {
				return nil, errors.New("expected environment key")
			}
			var value any
			if err := decoder.Decode(&value); err != nil {
				return nil, err
			}
			entries = append(entries, specialEntryFromValue(entryKey, value, "environment"))
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return entries, nil
	}
	return nil, errors.New("missing object at .environment")
}

func specialEntryFromValue(key string, value any, rootKey string) repository.SpecialEntry {
	if rootKey == "assets" {
		if object, ok := value.(map[string]any); ok {
			return repository.SpecialEntry{
				Key:       key,
				ValueType: firstNonEmpty(stringValue(object["kind"]), "unknown"),
				ValueText: stringValue(object["content"]),
			}
		}
		return repository.SpecialEntry{Key: key, ValueType: "legacy", ValueText: stringValue(value)}
	}
	switch value.(type) {
	case string:
		return repository.SpecialEntry{Key: key, ValueType: "string", ValueText: stringValue(value)}
	case bool:
		return repository.SpecialEntry{Key: key, ValueType: "bool", ValueText: stringValue(value)}
	case float64, int, int64:
		return repository.SpecialEntry{Key: key, ValueType: "number", ValueText: stringValue(value)}
	case nil:
		return repository.SpecialEntry{Key: key, ValueType: "null", ValueText: ""}
	default:
		raw, _ := json.Marshal(value)
		return repository.SpecialEntry{Key: key, ValueType: "json", ValueText: string(raw)}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func (s *Server) rememberBuildPreview(id string, snapshot buildPreviewSnapshot) {
	s.buildPreviewMu.Lock()
	defer s.buildPreviewMu.Unlock()
	now := time.Now()
	for existingID, existing := range s.buildPreviewDirs {
		if now.After(existing.ExpiresAt) || existing.Env == snapshot.Env {
			os.RemoveAll(existing.Root)
			delete(s.buildPreviewDirs, existingID)
		}
	}
	s.buildPreviewDirs[id] = snapshot
}

func (s *Server) buildPreviewSnapshot(id string, env string) (buildPreviewSnapshot, bool) {
	s.buildPreviewMu.Lock()
	defer s.buildPreviewMu.Unlock()
	snapshot, ok := s.buildPreviewDirs[id]
	if !ok || snapshot.Env != env || time.Now().After(snapshot.ExpiresAt) {
		if ok {
			os.RemoveAll(snapshot.Root)
			delete(s.buildPreviewDirs, id)
		}
		return buildPreviewSnapshot{}, false
	}
	return snapshot, true
}

func safeJoin(root string, relPath string) (string, error) {
	if relPath == "" {
		return "", errors.New("preview path is empty")
	}
	clean := filepath.Clean(filepath.FromSlash(relPath))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("preview path escapes preview root")
	}
	fullPath := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, fullPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("preview path escapes preview root")
	}
	return fullPath, nil
}

func randomID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func (s *Server) buildOptions(env string) buildapp.Options {
	return buildapp.Options{
		Environment: env,
		Root:        s.repo.Root(),
	}
}

func validationMessage(err error) string {
	if err == nil {
		return "Validation OK"
	}
	return err.Error()
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
	if repository.IsConflictError(err) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	code := ""
	if status == http.StatusConflict {
		code = "conflict"
	}
	writeJSON(w, status, apiError{Error: message, Status: status, Code: code})
}
