package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"kube-env/internal/appinfo"
	opsconfig "kube-env/internal/opsapp/config"
	"kube-env/internal/opsapp/render"
)

type cliOptions struct {
	configPath  string
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

func printJSON(cmd *cobra.Command, payload any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		return errors.Join(errors.New("failed to write JSON"), err)
	}
	return nil
}
