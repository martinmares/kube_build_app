package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kube-env/internal/appinfo"
)

func TestNoArgsPrintsHelp(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommand(appinfo.For(appinfo.EditAppName), &cliOptions{})
	cmd.SetArgs(nil)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	oldArgs := os.Args
	os.Args = []string{"kube-edit-app"}
	defer func() { os.Args = oldArgs }()

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute without args failed: %v\n%s", err, out.String())
	}
	for _, expected := range []string{
		"Web editor for kube environment repositories",
		"Available Commands:",
		"serve",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("help output missing %q:\n%s", expected, out.String())
		}
	}
}

func TestVersionPrintsJSON(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommand(appinfo.For(appinfo.EditAppName), &cliOptions{})
	cmd.SetArgs([]string{"--version"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), `"name": "kube-edit-app"`) {
		t.Fatalf("version output missing app name:\n%s", out.String())
	}
}

func TestServeRequiresRoot(t *testing.T) {
	t.Setenv("ENVIRONMENTS_ROOT", "")
	var out bytes.Buffer
	cmd := newRootCommand(appinfo.For(appinfo.EditAppName), &cliOptions{})
	cmd.SetArgs([]string{"serve"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("serve without root succeeded, want error")
	}
	if !strings.Contains(err.Error(), "--root is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServeRejectsConflictingWriteFlags(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "test"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(appinfo.For(appinfo.EditAppName), &cliOptions{})
	cmd.SetArgs([]string{"serve", "--root", root, "--read-only", "--allow-write"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("serve with conflicting flags succeeded, want error")
	}
	if !strings.Contains(err.Error(), "--read-only and --allow-write cannot be used together") {
		t.Fatalf("unexpected error: %v", err)
	}
}
