package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The golden files were produced by history-to-md.js from the same fixture (user label
// replaced with "User"), so these tests pin byte-for-byte compatibility with the JS tool.
const fixture = "testdata/abcdef12-0000-0000-0000-000000000000.jsonl"

func TestRenderMatchesJS(t *testing.T) {
	for _, tc := range []struct {
		golden string
		full   bool
		want   Stats
	}{
		{"testdata/dialog.golden.md", false, Stats{User: 3, Assistant: 2}},
		{"testdata/full.golden.md", true, Stats{User: 3, Assistant: 3, ToolResults: 3}},
	} {
		got, st, err := Render(fixture, Options{Full: tc.full, UserLabel: "User"})
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(tc.golden)
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Errorf("%s: output differs from JS golden\n--- got ---\n%s", tc.golden, got)
		}
		st.Bytes = 0
		if st != tc.want {
			t.Errorf("%s: stats = %+v, want %+v", tc.golden, st, tc.want)
		}
	}
}

func TestUserLabel(t *testing.T) {
	got, _, _ := Render(fixture, Options{UserLabel: "Me"})
	if !strings.Contains(got, "## Me\n") || strings.Contains(got, "## User") {
		t.Errorf("custom user label not applied")
	}
}

func TestOutPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "abcdef12-3456-7890.jsonl")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mt := time.Date(2026, 9, 21, 23, 30, 0, 0, time.UTC)
	if err := os.Chtimes(file, mt, mt); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		folder, custom string
		full           bool
		want           string
	}{
		{"my-app", "", false, "/h/my-app/2026-09-21-abcdef12.md"},
		{"my-app", "", true, "/h/my-app/2026-09-21-abcdef12.full.md"},
		{"my-app", "notes.md", true, "/h/my-app/2026-09-21-notes.md"},
		{"my-app", "notes", false, "/h/my-app/2026-09-21-notes.md"},
		{"my-app", "2020-01-01-x.md", false, "/h/my-app/2020-01-01-x.md"},
		{"my-app", "/notes/x.md", false, "/notes/2026-09-21-x.md"},
	} {
		if got := OutPath("/h", tc.folder, file, tc.custom, tc.full); got != tc.want {
			t.Errorf("OutPath(%q, %q, %v) = %s, want %s", tc.folder, tc.custom, tc.full, got, tc.want)
		}
	}
}
