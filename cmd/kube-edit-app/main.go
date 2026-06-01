package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"kube-env/internal/appinfo"
	"kube-env/internal/repository"
	"kube-env/internal/webapp"
)

type cliOptions struct {
	listen            string
	root              string
	basePath          string
	readOnly          bool
	allowWrite        bool
	encjsonPath       string
	encjsonLegacyPath string
	encjsonKeydir     string
	clusterStatus     bool
	kubeconfig        string
	kubeContext       string
	showVersion       bool
}

func main() {
	info := appinfo.For(appinfo.EditAppName)
	opts := &cliOptions{}
	cmd := newRootCommand(info, opts)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", info.Name, err)
		os.Exit(1)
	}
}

func newRootCommand(info appinfo.Info, opts *cliOptions) *cobra.Command {
	root := &cobra.Command{
		Use:           "kube-edit-app",
		Short:         "Web editor for kube environment repositories",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(os.Args) == 1 {
				return cmd.Help()
			}
			if opts.showVersion {
				return printJSON(cmd, info)
			}
			return cmd.Help()
		},
	}

	root.PersistentFlags().BoolVar(&opts.showVersion, "version", false, "print version information as JSON")
	root.AddCommand(newServeCommand(info, opts))
	return root
}

func newServeCommand(info appinfo.Info, opts *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start web editor server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, info, opts)
		},
	}
	cmd.Flags().StringVar(&opts.listen, "listen", "127.0.0.1:8183", "HTTP listen address")
	cmd.Flags().StringVar(&opts.root, "root", os.Getenv("ENVIRONMENTS_ROOT"), "environment repository root")
	cmd.Flags().StringVar(&opts.basePath, "base-path", "/", "reverse proxy base path")
	cmd.Flags().BoolVar(&opts.readOnly, "read-only", false, "disable mutating API endpoints")
	cmd.Flags().BoolVar(&opts.allowWrite, "allow-write", false, "explicitly allow mutating API endpoints")
	cmd.Flags().StringVar(&opts.encjsonPath, "encjson-path", os.Getenv("ENCJSON_PATH"), "modern EncJson binary path")
	cmd.Flags().StringVar(&opts.encjsonLegacyPath, "encjson-legacy-path", os.Getenv("ENCJSON_LEGACY_PATH"), "legacy EncJson binary path")
	cmd.Flags().StringVar(&opts.encjsonKeydir, "encjson-keydir", os.Getenv("ENCJSON_KEYDIR"), "optional EncJson key directory")
	cmd.Flags().BoolVar(&opts.clusterStatus, "cluster-status", false, "enable read-only Kubernetes namespace status panel")
	cmd.Flags().StringVar(&opts.kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "optional kubeconfig path for read-only cluster status")
	cmd.Flags().StringVar(&opts.kubeContext, "context", "", "optional kubeconfig context for read-only cluster status")
	return cmd
}

func runServe(_ *cobra.Command, info appinfo.Info, opts *cliOptions) error {
	if opts.readOnly && opts.allowWrite {
		return errors.New("--read-only and --allow-write cannot be used together")
	}
	if opts.root == "" {
		return errors.New("--root is required or ENVIRONMENTS_ROOT must be set")
	}

	repo, err := repository.New(opts.root)
	if err != nil {
		return fmt.Errorf("invalid repository root: %w", err)
	}

	serverOpts := webapp.Options{
		BasePath:          opts.basePath,
		ReadOnly:          opts.readOnly || !opts.allowWrite,
		EncjsonPath:       opts.encjsonPath,
		EncjsonLegacyPath: opts.encjsonLegacyPath,
		EncjsonKeydir:     opts.encjsonKeydir,
		ClusterStatus:     opts.clusterStatus,
		Kubeconfig:        opts.kubeconfig,
		KubeContext:       opts.kubeContext,
	}
	server := webapp.NewServer(info, repo, serverOpts)
	if err := server.ListenAndServe(opts.listen); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("web editor failed", "error", err)
		return err
	}
	return nil
}

func printJSON(cmd *cobra.Command, payload any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		return errors.Join(errors.New("failed to write JSON"), err)
	}
	return nil
}
