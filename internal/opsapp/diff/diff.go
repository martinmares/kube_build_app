package diff

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	return append(lines, lineUnified(oldLines, newLines)...)
}

func splitLines(content string) []string {
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

type diffOp struct {
	kind string
	line string
}

type diffHunk struct {
	ops      []diffOp
	oldStart int
	newStart int
}

func lineUnified(oldLines []string, newLines []string) []string {
	ops := diffLines(oldLines, newLines)
	hunks := unifiedHunks(ops, 3)
	if len(hunks) == 0 {
		return nil
	}
	lines := []string{}
	for _, hunk := range hunks {
		oldCount, newCount := hunkCounts(hunk.ops)
		lines = append(lines, fmt.Sprintf("@@ -%s +%s @@", rangeText(hunk.oldStart, oldCount), rangeText(hunk.newStart, newCount)))
		for _, op := range hunk.ops {
			switch op.kind {
			case "equal":
				lines = append(lines, " "+op.line)
			case "delete":
				lines = append(lines, "-"+op.line)
			case "insert":
				lines = append(lines, "+"+op.line)
			}
		}
	}
	return lines
}

func diffLines(oldLines []string, newLines []string) []diffOp {
	lcs := make([][]int, len(oldLines)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(newLines)+1)
	}
	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	ops := []diffOp{}
	i, j := 0, 0
	for i < len(oldLines) && j < len(newLines) {
		if oldLines[i] == newLines[j] {
			ops = append(ops, diffOp{kind: "equal", line: oldLines[i]})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			ops = append(ops, diffOp{kind: "delete", line: oldLines[i]})
			i++
		} else {
			ops = append(ops, diffOp{kind: "insert", line: newLines[j]})
			j++
		}
	}
	for i < len(oldLines) {
		ops = append(ops, diffOp{kind: "delete", line: oldLines[i]})
		i++
	}
	for j < len(newLines) {
		ops = append(ops, diffOp{kind: "insert", line: newLines[j]})
		j++
	}
	return ops
}

func unifiedHunks(ops []diffOp, context int) []diffHunk {
	changeIndexes := []int{}
	for i, op := range ops {
		if op.kind != "equal" {
			changeIndexes = append(changeIndexes, i)
		}
	}
	if len(changeIndexes) == 0 {
		return nil
	}
	hunks := []diffHunk{}
	start := max(0, changeIndexes[0]-context)
	end := min(len(ops), changeIndexes[0]+context+1)
	for _, idx := range changeIndexes[1:] {
		nextStart := max(0, idx-context)
		nextEnd := min(len(ops), idx+context+1)
		if nextStart <= end {
			end = max(end, nextEnd)
			continue
		}
		hunks = append(hunks, makeHunk(ops, start, end))
		start, end = nextStart, nextEnd
	}
	hunks = append(hunks, makeHunk(ops, start, end))
	return hunks
}

func makeHunk(ops []diffOp, start int, end int) diffHunk {
	oldLine, newLine := 1, 1
	for _, op := range ops[:start] {
		switch op.kind {
		case "equal":
			oldLine++
			newLine++
		case "delete":
			oldLine++
		case "insert":
			newLine++
		}
	}
	return diffHunk{ops: ops[start:end], oldStart: oldLine, newStart: newLine}
}

func hunkCounts(hunk []diffOp) (int, int) {
	oldCount, newCount := 0, 0
	for _, op := range hunk {
		switch op.kind {
		case "equal":
			oldCount++
			newCount++
		case "delete":
			oldCount++
		case "insert":
			newCount++
		}
	}
	return oldCount, newCount
}

func rangeText(start int, count int) string {
	if count == 1 {
		return strconv.Itoa(start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
