package diff

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Result struct {
	Changed bool       `json:"changed"`
	Files   []FileDiff `json:"files"`
}

type FileDiff struct {
	Path       string   `json:"path"`
	Status     string   `json:"status"`
	OldContent string   `json:"old_content,omitempty"`
	NewContent string   `json:"new_content,omitempty"`
	Unified    []string `json:"unified,omitempty"`
}

func Directories(oldRoot string, newRoot string) (Result, error) {
	oldFiles, err := readFiles(oldRoot)
	if err != nil {
		return Result{}, err
	}
	newFiles, err := readFiles(newRoot)
	if err != nil {
		return Result{}, err
	}
	pathsMap := map[string]bool{}
	for path := range oldFiles {
		pathsMap[path] = true
	}
	for path := range newFiles {
		pathsMap[path] = true
	}
	paths := make([]string, 0, len(pathsMap))
	for path := range pathsMap {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	result := Result{}
	for _, path := range paths {
		oldContent, oldOK := oldFiles[path]
		newContent, newOK := newFiles[path]
		if oldOK && newOK && bytes.Equal(oldContent, newContent) {
			continue
		}
		file := FileDiff{Path: path, OldContent: string(oldContent), NewContent: string(newContent)}
		switch {
		case !oldOK && newOK:
			file.Status = "added"
		case oldOK && !newOK:
			file.Status = "deleted"
		default:
			file.Status = "modified"
		}
		file.Unified = unified(path, string(oldContent), string(newContent), file.Status)
		result.Files = append(result.Files, file)
	}
	result.Changed = len(result.Files) > 0
	return result, nil
}

func readFiles(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	if strings.TrimSpace(root) == "" {
		return files, nil
	}
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
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
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = content
		return nil
	}); err != nil {
		return nil, err
	}
	return files, nil
}

func unified(path string, oldContent string, newContent string, status string) []string {
	lines := []string{fmt.Sprintf("--- applied/%s", path), fmt.Sprintf("+++ desired/%s", path)}
	if status == "added" {
		for _, line := range splitLines(newContent) {
			lines = append(lines, "+"+line)
		}
		return lines
	}
	if status == "deleted" {
		for _, line := range splitLines(oldContent) {
			lines = append(lines, "-"+line)
		}
		return lines
	}
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)
	lines = append(lines, "@@")
	for _, line := range oldLines {
		lines = append(lines, "-"+line)
	}
	for _, line := range newLines {
		lines = append(lines, "+"+line)
	}
	return lines
}

func splitLines(content string) []string {
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}
