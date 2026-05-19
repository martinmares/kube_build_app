package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"kube-env/internal/appinfo"
	opsconfig "kube-env/internal/opsapp/config"
	opsdiff "kube-env/internal/opsapp/diff"
	opsgit "kube-env/internal/opsapp/git"
	"kube-env/internal/opsapp/render"
	opsserver "kube-env/internal/opsapp/server"
	"kube-env/internal/opsapp/source"
	"kube-env/internal/opsapp/state"
	opsstatus "kube-env/internal/opsapp/status"
)

type cliOptions struct {
	configPath  string
	statePath   string
	workDir     string
	output      string
	fromGit     bool
	showVersion bool
}

func main() {
	info := appinfo.For(appinfo.OpsAppName)
	opts := &cliOptions{}
	cmd := newRootCommand(info, opts)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", info.Name, err)
		os.Exit(1)
	}
}

func newRootCommand(info appinfo.Info, opts *cliOptions) *cobra.Command {
	root := &cobra.Command{
		Use:           "kube-ops-app",
		Short:         "Operations portal and sync runner for kube-build-app environments",
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
	root.PersistentFlags().StringVar(&opts.configPath, "config", os.Getenv("KUBE_OPS_CONFIG"), "kube-ops-app config file path")
	root.PersistentFlags().StringVar(&opts.statePath, "state", os.Getenv("KUBE_OPS_STATE"), "optional local state JSON path for prototype applied status")
	root.PersistentFlags().StringVar(&opts.workDir, "work-dir", os.Getenv("KUBE_OPS_WORK_DIR"), "Git checkout work directory for target revision resolution")
	root.PersistentFlags().StringVarP(&opts.output, "output", "o", "text", "output format: text or json")
	root.PersistentFlags().BoolVar(&opts.fromGit, "from-git", false, "render/status/diff from checked out target revision instead of local root_path")
	root.AddCommand(newEnvCommand(opts))
	root.AddCommand(newServerCommand(info, opts))
	return root
}

func newServerCommand(info appinfo.Info, opts *cliOptions) *cobra.Command {
	var listen string
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start read-only kube-ops-app web UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			addr := strings.TrimSpace(listen)
			if addr == "" {
				addr = cfg.Server.HTTP.Listen
			}
			return opsserver.New(opsserver.Options{
				Info:      info,
				Config:    cfg,
				StatePath: opts.statePath,
				WorkDir:   opts.workDir,
				FromGit:   opts.fromGit,
			}).ListenAndServe(addr)
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "", "HTTP listen address; defaults to config server.http.listen")
	return cmd
}

func newEnvCommand(opts *cliOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "env", Short: "Inspect configured environments"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List configured environments",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			if opts.output == "json" {
				return printJSON(cmd, cfg.Environments)
			}
			for _, env := range cfg.Environments {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", env.Name, env.EnvName, env.Namespace, env.TargetRevision)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "resolve ENV",
		Short: "Checkout an environment target revision and print resolved Git metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			env, ok := cfg.Environment(args[0])
			if !ok {
				return fmt.Errorf("environment %q not found", args[0])
			}
			repo := strings.TrimSpace(env.Repo)
			if repo == "" {
				repo = strings.TrimSpace(env.RootPath)
			}
			result, err := opsgit.Checkout(opsgit.CheckoutOptions{
				Repo:     repo,
				Revision: env.TargetRevision,
				WorkDir:  opts.workDir,
				Name:     env.Name,
			})
			if err != nil {
				return err
			}
			if opts.output == "json" {
				return printJSON(cmd, map[string]any{"environment": env.Name, "git": result})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", env.Name, result.Revision, result.ResolvedCommit, result.WorktreePath)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "render-digest ENV",
		Short: "Render an environment and print deterministic manifest digest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			env, err := preparedEnvironment(cfg, opts, args[0])
			if err != nil {
				return err
			}
			result, err := render.RenderDigest(env.Environment)
			if err != nil {
				return err
			}
			if opts.output == "json" {
				return printJSON(cmd, result)
			}
			if result.ResolvedCommit != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\tcommit=%s\t%d files\t%d bytes\n", result.Environment, result.Digest, result.ResolvedCommit, result.Files, result.Bytes)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%d files\t%d bytes\n", result.Environment, result.Digest, result.Files, result.Bytes)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status ENV",
		Short: "Render desired manifests and compare them with recorded applied state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			env, err := preparedEnvironment(cfg, opts, args[0])
			if err != nil {
				return err
			}
			result, err := opsstatus.ComputeEnvironment(env.Environment, state.NewStore(opts.statePath))
			if err != nil {
				return err
			}
			if opts.output == "json" {
				return printJSON(cmd, result)
			}
			if result.ResolvedCommit != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\tdesired=%s\tapplied=%s\tcommit=%s\n", result.Environment, result.SyncStatus, result.DesiredDigest, emptyDash(result.AppliedDigest), result.ResolvedCommit)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\tdesired=%s\tapplied=%s\n", result.Environment, result.SyncStatus, result.DesiredDigest, emptyDash(result.AppliedDigest))
			}
			if result.Reason != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "reason\t%s\n", result.Reason)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "mark-applied ENV",
		Short: "Record current rendered digest as applied in the prototype local state file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(opts.statePath) == "" {
				return errors.New("--state is required for mark-applied")
			}
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			env, err := preparedEnvironment(cfg, opts, args[0])
			if err != nil {
				return err
			}
			store := state.NewStore(opts.statePath)
			snapshotPath, err := store.SnapshotDir(env.Environment.Name)
			if err != nil {
				return err
			}
			result, err := render.RenderDigestTo(env.Environment, snapshotPath)
			if err != nil {
				return err
			}
			if err := store.SaveEnvironment(result.Environment, state.EnvironmentState{
				AppliedRevision: result.TargetRevision,
				AppliedCommit:   result.ResolvedCommit,
				AppliedDigest:   result.Digest,
				SnapshotPath:    snapshotPath,
				AppliedAt:       time.Now().UTC(),
				AppliedBy:       "kube-ops-app mark-applied",
			}); err != nil {
				return err
			}
			if opts.output == "json" {
				return printJSON(cmd, map[string]any{"environment": result.Environment, "applied_revision": result.TargetRevision, "applied_commit": result.ResolvedCommit, "applied_digest": result.Digest})
			}
			if result.ResolvedCommit != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\tmarked applied\t%s\tcommit=%s\n", result.Environment, result.Digest, result.ResolvedCommit)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\tmarked applied\t%s\n", result.Environment, result.Digest)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "diff ENV",
		Short: "Render desired manifests and diff them against the applied snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(opts.statePath) == "" {
				return errors.New("--state is required for diff")
			}
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			env, err := preparedEnvironment(cfg, opts, args[0])
			if err != nil {
				return err
			}
			store := state.NewStore(opts.statePath)
			applied, found, err := store.Environment(env.Environment.Name)
			if err != nil {
				return err
			}
			if !found || applied.SnapshotPath == "" {
				return fmt.Errorf("environment %q has no applied snapshot; run mark-applied first", env.Environment.Name)
			}
			desiredDir, err := os.MkdirTemp("", "kube-ops-desired-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(desiredDir)
			desired, err := render.RenderDigestTo(env.Environment, desiredDir)
			if err != nil {
				return err
			}
			result, err := opsdiff.Directories(applied.SnapshotPath, desiredDir)
			if err != nil {
				return err
			}
			if opts.output == "json" {
				return printJSON(cmd, map[string]any{"environment": env.Environment.Name, "applied_digest": applied.AppliedDigest, "desired_digest": desired.Digest, "diff": result})
			}
			if !result.Changed {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\tNoDiff\t%s\n", env.Environment.Name, desired.Digest)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\tDiff\tapplied=%s\tdesired=%s\n", env.Environment.Name, applied.AppliedDigest, desired.Digest)
			for _, file := range result.Files {
				fmt.Fprintf(cmd.OutOrStdout(), "file\t%s\t%s\n", file.Status, file.Path)
				for _, line := range file.Unified {
					fmt.Fprintln(cmd.OutOrStdout(), line)
				}
			}
			return nil
		},
	})
	return cmd
}

func loadConfig(opts *cliOptions) (opsconfig.Config, error) {
	if opts.output != "text" && opts.output != "json" {
		return opsconfig.Config{}, fmt.Errorf("unsupported output format %q", opts.output)
	}
	if opts.configPath == "" {
		return opsconfig.Config{}, errors.New("--config is required or KUBE_OPS_CONFIG must be set")
	}
	return opsconfig.LoadFile(opts.configPath)
}

func preparedEnvironment(cfg opsconfig.Config, opts *cliOptions, name string) (source.Prepared, error) {
	env, ok := cfg.Environment(name)
	if !ok {
		return source.Prepared{}, fmt.Errorf("environment %q not found", name)
	}
	return source.Prepare(env, source.Options{FromGit: opts.fromGit, WorkDir: opts.workDir})
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func printJSON(cmd *cobra.Command, payload any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		return errors.Join(errors.New("failed to write JSON"), err)
	}
	return nil
}
