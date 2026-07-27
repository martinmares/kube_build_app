package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"kube-env/internal/importapp"
)

type importOptions struct {
	file       string
	output     string
	report     string
	namespace  string
	deployment string
	kubeconfig string
	context    string
	timeout    time.Duration
	force      bool
}

type importSource struct {
	deployment  []byte
	services    []byte
	description string
}

func newImportCommand() *cobra.Command {
	opts := &importOptions{}
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a Kubernetes Deployment into an app model",
		Long: `Import an apps/v1 Deployment into a kube-build-app app model.

Use --file for an offline YAML file (or "-" for stdin). Without --file, the
Deployment and Services are read through kubectl from the current Kubernetes
context. Matching Services are converted to named ports and expose_as entries.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(cmd, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.file, "file", "f", "", `Deployment YAML file; use "-" for stdin`)
	cmd.Flags().StringVarP(&opts.output, "output", "o", "-", `app model output file; use "-" for stdout`)
	cmd.Flags().StringVar(&opts.report, "report", "", "optional JSON import report file")
	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "", "source namespace for cluster import")
	cmd.Flags().StringVar(&opts.deployment, "deployment", "", "source Deployment name for cluster import")
	cmd.Flags().StringVar(&opts.kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "kubeconfig path for cluster import")
	cmd.Flags().StringVar(&opts.context, "context", "", "kubeconfig context for cluster import")
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 15*time.Second, "kubectl timeout for cluster import")
	cmd.Flags().BoolVar(&opts.force, "force", false, "overwrite existing output and report files")
	return cmd
}

func runImport(cmd *cobra.Command, opts *importOptions) error {
	if opts.output == "-" && opts.report == "-" {
		return errors.New("import failed: --output - and --report - cannot share stdout")
	}
	if strings.TrimSpace(opts.report) == "-" {
		return errors.New("import failed: --report must be a file path")
	}
	if err := validateImportDestinations(opts.output, opts.report, opts.force); err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	source, err := readImportSource(cmd, opts)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}
	result, err := importapp.ImportDeployment(source.deployment, source.description)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}
	if len(source.services) > 0 {
		if err := importapp.EnrichWithServices(&result, source.services); err != nil {
			return fmt.Errorf("import failed: %w", err)
		}
	}
	appYAML, err := importapp.MarshalApp(result.App)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	var reportJSON []byte
	if strings.TrimSpace(opts.report) != "" {
		reportJSON, err = importapp.MarshalReport(result.Report)
		if err != nil {
			return fmt.Errorf("import failed: %w", err)
		}
	}

	if opts.output == "-" {
		if _, err := cmd.OutOrStdout().Write(appYAML); err != nil {
			return fmt.Errorf("import failed: write app model: %w", err)
		}
	} else {
		if err := writeImportFile(opts.output, appYAML, opts.force); err != nil {
			return fmt.Errorf("import failed: %w", err)
		}
	}
	if len(reportJSON) > 0 {
		if err := writeImportFile(opts.report, reportJSON, opts.force); err != nil {
			return fmt.Errorf("import failed: write report: %w", err)
		}
	}

	if opts.output != "-" {
		fmt.Fprintf(cmd.OutOrStdout(), "Imported Deployment %s -> %s\n", result.Report.Name, opts.output)
		if opts.report != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Report: %s\n", opts.report)
		}
	}
	return nil
}

func readImportSource(cmd *cobra.Command, opts *importOptions) (importSource, error) {
	file := strings.TrimSpace(opts.file)
	if file != "" {
		if strings.TrimSpace(opts.deployment) != "" || strings.TrimSpace(opts.namespace) != "" {
			return importSource{}, errors.New("--file conflicts with --deployment and --namespace")
		}
		if file == "-" {
			content, err := io.ReadAll(cmd.InOrStdin())
			return importSource{deployment: content, description: "stdin"}, err
		}
		content, err := os.ReadFile(file)
		return importSource{deployment: content, description: file}, err
	}

	kubectlOptions := importapp.KubectlOptions{
		Namespace:  opts.namespace,
		Deployment: opts.deployment,
		Kubeconfig: opts.kubeconfig,
		Context:    opts.context,
		Timeout:    opts.timeout,
	}
	deployment, err := importapp.ReadDeploymentFromKubectl(cmd.Context(), kubectlOptions)
	if err != nil {
		return importSource{}, err
	}
	services, err := importapp.ReadServicesFromKubectl(cmd.Context(), kubectlOptions)
	if err != nil {
		return importSource{}, err
	}
	return importSource{
		deployment:  deployment,
		services:    services,
		description: fmt.Sprintf("kubectl:%s/deployment/%s", opts.namespace, opts.deployment),
	}, nil
}

func writeImportFile(path string, content []byte, force bool) error {
	path = strings.TrimSpace(path)
	if path == "" || path == "-" {
		return errors.New("output file path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileNoClobber(path, content, force)
}

func validateImportDestinations(output string, report string, force bool) error {
	var paths []string
	if output = strings.TrimSpace(output); output != "" && output != "-" {
		paths = append(paths, output)
	}
	if report = strings.TrimSpace(report); report != "" && report != "-" {
		paths = append(paths, report)
	}
	if len(paths) == 2 {
		outputPath, outputErr := filepath.Abs(paths[0])
		reportPath, reportErr := filepath.Abs(paths[1])
		if outputErr == nil && reportErr == nil && outputPath == reportPath {
			return errors.New("--output and --report must use different files")
		}
	}
	if force {
		return nil
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists; use --force to overwrite", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
