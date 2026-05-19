package digest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type Result struct {
	Digest string   `json:"digest"`
	Files  int      `json:"files"`
	Bytes  int64    `json:"bytes"`
	Paths  []string `json:"paths,omitempty"`
}

func Directory(root string) (Result, error) {
	paths := []string{}
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
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		return Result{}, err
	}
	sort.Strings(paths)

	hash := sha256.New()
	var totalBytes int64
	for _, rel := range paths {
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if err != nil {
			return Result{}, err
		}
		fmt.Fprintf(hash, "path:%s\nsize:%d\n", rel, info.Size())
		file, err := os.Open(path)
		if err != nil {
			return Result{}, err
		}
		written, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return Result{}, copyErr
		}
		if closeErr != nil {
			return Result{}, closeErr
		}
		fmt.Fprint(hash, "\n")
		totalBytes += written
	}

	return Result{Digest: "sha256:" + hex.EncodeToString(hash.Sum(nil)), Files: len(paths), Bytes: totalBytes, Paths: paths}, nil
}
