package repository

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type GitStatus struct {
	Available  bool            `json:"available"`
	Root       string          `json:"root,omitempty"`
	Branch     string          `json:"branch,omitempty"`
	Commit     string          `json:"commit,omitempty"`
	Dirty      bool            `json:"dirty"`
	DirtyCount int             `json:"dirty_count"`
	Files      []GitFileStatus `json:"files"`
	Error      string          `json:"error,omitempty"`
}

type GitFileStatus struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

type GitDiff struct {
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Content   string `json:"content,omitempty"`
	Error     string `json:"error,omitempty"`
}

type GitRestoreResult struct {
	Restored bool      `json:"restored"`
	Path     string    `json:"path"`
	Status   GitStatus `json:"status"`
}

type GitCommitResult struct {
	Committed bool      `json:"committed"`
	Commit    string    `json:"commit,omitempty"`
	Paths     []string  `json:"paths"`
	Status    GitStatus `json:"status"`
}

func (r *Repository) GitStatus() GitStatus {
	topLevel, err := gitOutput(r.root, "rev-parse", "--show-toplevel")
	if err != nil {
		return GitStatus{Available: false, Error: "repository root is not inside a Git work tree"}
	}
	status := GitStatus{Available: true, Root: topLevel}
	status.Branch, _ = gitOutput(r.root, "branch", "--show-current")
	status.Commit, _ = gitOutput(r.root, "rev-parse", "--short", "HEAD")

	raw, err := gitOutput(r.root, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		status.Error = err.Error()
		return status
	}
	rootForRel := r.root
	if resolved, err := filepath.EvalSymlinks(r.root); err == nil {
		rootForRel = resolved
	}
	topLevelForRel := topLevel
	if resolved, err := filepath.EvalSymlinks(topLevel); err == nil {
		topLevelForRel = resolved
	}
	prefix, err := filepath.Rel(topLevelForRel, rootForRel)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	prefix = filepath.ToSlash(prefix)
	if prefix == "." {
		prefix = ""
	}
	files := parseGitStatusFiles(raw, prefix)
	status.Files = files
	status.DirtyCount = len(files)
	status.Dirty = len(files) > 0
	return status
}

func (r *Repository) GitDiff(relativePath string) GitDiff {
	rel, err := validateRelativePath(relativePath)
	if err != nil {
		return GitDiff{Available: false, Error: err.Error()}
	}
	if _, err := gitOutput(r.root, "rev-parse", "--show-toplevel"); err != nil {
		return GitDiff{Available: false, Path: rel, Error: "repository root is not inside a Git work tree"}
	}

	diff := GitDiff{Available: true, Path: rel}
	staged, stagedErr := gitOutputRaw(r.root, "diff", "--cached", "--", rel)
	unstaged, unstagedErr := gitOutputRaw(r.root, "diff", "--", rel)
	if stagedErr != nil && unstagedErr != nil {
		diff.Error = stagedErr.Error()
		return diff
	}
	diff.Content = strings.TrimSpace(strings.TrimSpace(staged) + "\n" + strings.TrimSpace(unstaged))
	if diff.Content == "" && r.gitStatusCode(rel) == "??" {
		content, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(rel)))
		if err != nil {
			diff.Error = err.Error()
			return diff
		}
		diff.Content = untrackedFileDiff(rel, string(content))
	}
	return diff
}

func (r *Repository) GitRestore(relativePath string, deleteUntracked bool) (GitRestoreResult, error) {
	rel, err := validateRelativePath(relativePath)
	if err != nil {
		return GitRestoreResult{}, err
	}
	if _, err := gitOutput(r.root, "rev-parse", "--show-toplevel"); err != nil {
		return GitRestoreResult{}, errors.New("repository root is not inside a Git work tree")
	}
	code := r.gitStatusCode(rel)
	if code == "" {
		return GitRestoreResult{Path: rel, Status: r.GitStatus()}, nil
	}
	if code == "??" {
		if !deleteUntracked {
			return GitRestoreResult{}, errors.New("untracked file restore requires delete_untracked=true")
		}
		path := filepath.Join(r.root, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if err != nil {
			return GitRestoreResult{}, err
		}
		if info.IsDir() {
			return GitRestoreResult{}, errors.New("refusing to delete untracked directory")
		}
		if err := os.Remove(path); err != nil {
			return GitRestoreResult{}, err
		}
		return GitRestoreResult{Restored: true, Path: rel, Status: r.GitStatus()}, nil
	}
	if _, err := gitOutputRaw(r.root, "restore", "--staged", "--worktree", "--", rel); err != nil {
		return GitRestoreResult{}, err
	}
	return GitRestoreResult{Restored: true, Path: rel, Status: r.GitStatus()}, nil
}

func (r *Repository) GitCommitSelected(relativePaths []string, message string) (GitCommitResult, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return GitCommitResult{}, errors.New("commit message is required")
	}
	if _, err := gitOutput(r.root, "rev-parse", "--show-toplevel"); err != nil {
		return GitCommitResult{}, errors.New("repository root is not inside a Git work tree")
	}
	paths, err := r.validateDirtyPaths(relativePaths)
	if err != nil {
		return GitCommitResult{}, err
	}
	addArgs := append([]string{"add", "--"}, paths...)
	if _, err := gitOutputRaw(r.root, addArgs...); err != nil {
		return GitCommitResult{}, err
	}
	commitArgs := append([]string{"commit", "-m", message, "--"}, paths...)
	if _, err := gitOutputRaw(r.root, commitArgs...); err != nil {
		return GitCommitResult{}, err
	}
	commit, _ := gitOutput(r.root, "rev-parse", "--short", "HEAD")
	return GitCommitResult{Committed: true, Commit: commit, Paths: paths, Status: r.GitStatus()}, nil
}

func (r *Repository) validateDirtyPaths(relativePaths []string) ([]string, error) {
	seen := map[string]bool{}
	var paths []string
	status := r.GitStatus()
	if !status.Available {
		return nil, errors.New("repository root is not inside a Git work tree")
	}
	dirty := map[string]bool{}
	for _, file := range status.Files {
		dirty[file.Path] = true
	}
	for _, rawPath := range relativePaths {
		path, err := validateRelativePath(rawPath)
		if err != nil {
			return nil, err
		}
		if seen[path] {
			continue
		}
		if !dirty[path] {
			return nil, errors.New("selected path is not dirty: " + path)
		}
		seen[path] = true
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, errors.New("at least one changed file must be selected")
	}
	return paths, nil
}

func (r *Repository) dirtyPathSet() map[string]bool {
	status := r.GitStatus()
	out := map[string]bool{}
	if !status.Available {
		return out
	}
	for _, file := range status.Files {
		out[file.Path] = true
	}
	return out
}

func (r *Repository) gitStatusCode(path string) string {
	for _, file := range r.GitStatus().Files {
		if file.Path == path {
			return file.Code
		}
	}
	return ""
}

func (r *Repository) isDirtyPath(path string) bool {
	return r.dirtyPathSet()[path]
}

func parseGitStatusFiles(raw string, prefix string) []GitFileStatus {
	files := []GitFileStatus{}
	for _, line := range strings.Split(raw, "\n") {
		if len(line) < 4 {
			continue
		}
		code := strings.TrimSpace(line[:2])
		path := strings.TrimSpace(strings.TrimLeft(line[2:], " \t"))
		if strings.Contains(path, " -> ") {
			parts := strings.Split(path, " -> ")
			path = strings.TrimSpace(parts[len(parts)-1])
		}
		path = strings.Trim(path, `"`)
		path = filepath.ToSlash(path)
		if prefix != "" {
			if path == prefix {
				path = ""
			} else if strings.HasPrefix(path, prefix+"/") {
				path = strings.TrimPrefix(path, prefix+"/")
			} else if strings.HasPrefix(path, "../") || path == ".." {
				continue
			}
		}
		if path == "" {
			continue
		}
		files = append(files, GitFileStatus{Path: path, Code: code})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

func hasDirtyPrefix(dirty map[string]bool, prefix string) bool {
	for path := range dirty {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func gitOutput(dir string, args ...string) (string, error) {
	out, err := gitOutputRaw(dir, args...)
	return strings.TrimSpace(out), err
}

func gitOutputRaw(dir string, args ...string) (string, error) {
	if _, err := os.Stat(dir); err != nil {
		return "", err
	}
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", errors.New(strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

func untrackedFileDiff(path string, content string) string {
	lines := strings.Split(content, "\n")
	var builder strings.Builder
	builder.WriteString("--- /dev/null\n")
	builder.WriteString("+++ b/")
	builder.WriteString(path)
	builder.WriteString("\n@@\n")
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			continue
		}
		builder.WriteByte('+')
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	return strings.TrimRight(builder.String(), "\n")
}
