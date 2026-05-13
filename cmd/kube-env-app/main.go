package main

import (
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"

	"kube-env/internal/appinfo"
	"kube-env/internal/repository"
	"kube-env/internal/webapp"
)

func main() {
	var listen string
	var root string
	var showVersion bool
	flag.StringVar(&listen, "listen", "127.0.0.1:8080", "HTTP listen address")
	flag.StringVar(&root, "root", "", "environment repository root")
	flag.BoolVar(&showVersion, "version", false, "print version information")
	flag.Parse()

	info := appinfo.For(appinfo.EnvAppName)
	if showVersion {
		slog.Info("version", "name", info.Name, "version", info.Version, "commit", info.Commit, "date", info.Date)
		return
	}

	var repo *repository.Repository
	if root != "" {
		var err error
		repo, err = repository.New(root)
		if err != nil {
			slog.Error("invalid repository root", "root", root, "error", err)
			os.Exit(2)
		}
	}

	server := webapp.NewServer(info, repo)
	if err := server.ListenAndServe(listen); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("web UI failed", "error", err)
		os.Exit(1)
	}
}
