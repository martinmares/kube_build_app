package webapp

import (
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"kube-env/internal/appinfo"
	"kube-env/internal/buildapp"
	"kube-env/internal/repository"
)

//go:embed static/* templates/*
var contentFiles embed.FS

var indexTemplate = template.Must(template.ParseFS(contentFiles, "templates/index.html"))

type indexPageData struct {
	AppName string
}

type Server struct {
	info    appinfo.Info
	repo    *repository.Repository
	options Options
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
	Env    string                `json:"env"`
	Files  []buildPreviewFile    `json:"files"`
	Events []buildapp.BuildEvent `json:"events"`
	Totals buildPreviewTotals    `json:"totals"`
}

type buildPreviewTotals struct {
	Files       int `json:"files"`
	Deployments int `json:"deployments"`
	Services    int `json:"services"`
	Assets      int `json:"assets"`
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
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(mustSubFS(contentFiles, "static")))))
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/v1/info", s.handleInfo)
	mux.HandleFunc("GET /api/v1/git/status", s.handleGitStatus)
	mux.HandleFunc("GET /api/v1/git/diff/{file_path...}", s.handleGitDiff)
	mux.HandleFunc("POST /api/v1/git/restore/{file_path...}", s.handleGitRestore)
	mux.HandleFunc("GET /api/v1/envs", s.handleEnvironments)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps", s.handleApps)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps/{app_file}", s.handleAppDetail)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps/{app_file}/rendered", s.handleAppRendered)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps/{app_file}/vars", s.handleAppVars)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/vars", s.handleAppVarsUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/replicas", s.handleAppReplicasUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/autoscaling", s.handleAppAutoscalingUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/resources", s.handleAppContainerResourcesUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/vars", s.handleAppContainerVarsUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/runtime/java", s.handleAppContainerRuntimeUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/ports", s.handleAppContainerPortsUpdate)
	mux.HandleFunc("PATCH /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/probes", s.handleAppContainerProbesUpdate)
	mux.HandleFunc("POST /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/probes/fix-legacy", s.handleAppContainerLegacyProbesFix)
	mux.HandleFunc("GET /api/v1/envs/{env}/apps/{app_file}/model", s.handleAppModel)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets", s.handleAssets)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets/content/{asset_path...}", s.handleAssetContent)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets/special/{special_file}/entries", s.handleSpecialEntries)
	mux.HandleFunc("GET /api/v1/envs/{env}/assets/special/{special_file}/preflight", s.handleSpecialPreflight)
	mux.HandleFunc("POST /api/v1/envs/{env}/validate", s.handleBuildValidate)
	mux.HandleFunc("POST /api/v1/envs/{env}/summary", s.handleBuildSummary)
	mux.HandleFunc("POST /api/v1/envs/{env}/inventory", s.handleBuildInventory)
	mux.HandleFunc("POST /api/v1/envs/{env}/preview", s.handleBuildPreview)
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

func (s *Server) handleAppContainerVarsUpdate(w http.ResponseWriter, r *http.Request) {
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
	vars, err := s.repo.UpdateAppContainerVars(r.PathValue("env"), r.PathValue("app_file"), containerIndex, payload.Items, payload.ExpectedHash)
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
	defer os.RemoveAll(tmpDir)

	opts := s.buildOptions(env)
	opts.Target = tmpDir
	result, err := buildapp.Build(opts)
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": err.Error()})
		return
	}
	files, err := previewFiles(tmpDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, buildPreview{
		Env:    env,
		Files:  files,
		Events: result.Events,
		Totals: buildPreviewTotals{
			Files:       len(files),
			Deployments: len(result.Deployments) + len(result.Budgets) + len(result.Autoscaling),
			Services:    len(result.Services) + len(result.Externals),
			Assets:      len(result.Assets),
		},
	})
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
