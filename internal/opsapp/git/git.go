package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// CheckoutOptions describes a read-only Git checkout used to resolve an environment target revision.
type CheckoutOptions struct {
	Repo     string
	Revision string
	WorkDir  string
	Name     string
}

// Resolution is the Git metadata produced for a checked out target revision.
type Resolution struct {
	Repo           string `json:"repo"`
	Revision       string `json:"revision"`
	ResolvedCommit string `json:"resolved_commit"`
	WorktreePath   string `json:"worktree_path"`
}

// Checkout clones repo into WorkDir/Name, checks out Revision and returns the resolved commit SHA.
func Checkout(opts CheckoutOptions) (Resolution, error) {
	repo := strings.TrimSpace(opts.Repo)
	if repo == "" {
		return Resolution{}, errors.New("git repo is required")
	}
	revision := strings.TrimSpace(opts.Revision)
	if revision == "" {
		revision = "HEAD"
	}
	workDir := strings.TrimSpace(opts.WorkDir)
	if workDir == "" {
		workDir = filepath.Join(".tmp", "kube-ops-work")
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return Resolution{}, err
	}
	target := filepath.Join(workDir, safeName(opts.Name))
	if err := os.RemoveAll(target); err != nil {
		return Resolution{}, err
	}
	cloneArgs := []string{"clone", "--quiet", "--no-checkout"}
	if isLocalRepoPath(repo) {
		// Avoid Git's local clone object optimization. It can expose stale or missing local
		// object references from developer worktrees; upload-pack gives us a safer copy.
		cloneArgs = append(cloneArgs, "--no-local")
	}
	cloneArgs = append(cloneArgs, repo, target)
	if _, err := gitOutput("", cloneArgs...); err != nil {
		return Resolution{}, err
	}
	if _, err := gitOutput(target, "checkout", "--quiet", revision); err != nil {
		return Resolution{}, err
	}
	commit, err := gitOutput(target, "rev-parse", "HEAD")
	if err != nil {
		return Resolution{}, err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{Repo: repo, Revision: revision, ResolvedCommit: commit, WorktreePath: absTarget}, nil
}

func isLocalRepoPath(repo string) bool {
	if strings.Contains(repo, "://") || strings.HasPrefix(repo, "git@") || strings.HasPrefix(repo, "ssh://") {
		return false
	}
	if _, err := os.Stat(repo); err == nil {
		return true
	}
	return false
}

func safeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "repo"
	}
	var builder strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('-')
	}
	result := strings.Trim(builder.String(), ".-")
	if result == "" {
		return "repo"
	}
	return result
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, text)
	}
	return text, nil
}
