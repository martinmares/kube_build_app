package repository

import "testing"

func TestParseGitStatusFilesKeepsFirstPathCharacter(t *testing.T) {
	files := parseGitStatusFiles("M test/apps/api.yml\n M test/apps/worker.yml\n?? test/assets.secured.json\n", "")
	if len(files) != 3 {
		t.Fatalf("files len = %d, want 3: %#v", len(files), files)
	}
	want := map[string]string{
		"test/apps/api.yml":        "M",
		"test/apps/worker.yml":     "M",
		"test/assets.secured.json": "??",
	}
	for _, file := range files {
		if want[file.Path] != file.Code {
			t.Fatalf("unexpected parsed file: %#v, all files: %#v", file, files)
		}
		delete(want, file.Path)
	}
	if len(want) > 0 {
		t.Fatalf("missing parsed files: %#v, all files: %#v", want, files)
	}
}
