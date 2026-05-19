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
	"kube-env/internal/opsapp/render"
	"kube-env/internal/opsapp/state"
	opsstatus "kube-env/internal/opsapp/status"
)

type cliOptions struct {
	configPath  string
	statePath   string
	output      string
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
	root.PersistentFlags().StringVarP(&opts.output, "output", "o", "text", "output format: text or json")
	root.AddCommand(newEnvCommand(opts))
	return root
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
		Use:   "render-digest ENV",
		Short: "Render an environment and print deterministic manifest digest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			result, err := render.RenderDigestByName(cfg, args[0])
			if err != nil {
				return err
			}
			if opts.output == "json" {
				return printJSON(cmd, result)
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
			result, err := opsstatus.Compute(cfg, state.NewStore(opts.statePath), args[0])
			if err != nil {
				return err
			}
			if opts.output == "json" {
				return printJSON(cmd, result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\tdesired=%s\tapplied=%s\n", result.Environment, result.SyncStatus, result.DesiredDigest, emptyDash(result.AppliedDigest))
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
			result, err := render.RenderDigestByName(cfg, args[0])
			if err != nil {
				return err
			}
			if err := state.NewStore(opts.statePath).SaveEnvironment(result.Environment, state.EnvironmentState{
				AppliedRevision: result.TargetRevision,
				AppliedDigest:   result.Digest,
				AppliedAt:       time.Now().UTC(),
				AppliedBy:       "kube-ops-app mark-applied",
			}); err != nil {
				return err
			}
			if opts.output == "json" {
				return printJSON(cmd, map[string]any{"environment": result.Environment, "applied_revision": result.TargetRevision, "applied_digest": result.Digest})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\tmarked applied\t%s\n", result.Environment, result.Digest)
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
