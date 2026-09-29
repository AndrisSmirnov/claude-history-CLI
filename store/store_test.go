package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRoot builds a ~/.claude/projects look-alike and points Root at it.
func fakeRoot(t *testing.T) (projectPath string) {
	t.Helper()
	old := Root
	Root = t.TempDir()
	t.Cleanup(func() { Root = old })

	projectPath = filepath.Join(t.TempDir(), "My Proj")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(Root, Sanitize(projectPath))
	write := func(id, body string, age time.Duration) {
		f := filepath.Join(dir, id+".jsonl")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := time.Now().Add(-age)
		_ = os.Chtimes(f, mt, mt)
	}
	cwd := `"cwd":"` + projectPath + `"`
	write("aaaa1111-0000", `{"type":"user","isMeta":true,"message":{"content":"<caveat>x</caveat>"},`+cwd+`}
{"type":"user","message":{"content":"Перше питання про Widget"},`+cwd+`,"gitBranch":"dev"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Відповідь: widget це CLI. WIDGET!"}]}}
{"type":"ai-title","aiTitle":"Про widget"}
`, time.Hour)
	write("aaaa2222-0000", `{"type":"user","message":{"content":"друга сесія"},`+cwd+`}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"widget"}}]}}
`, time.Minute)
	if err := os.MkdirAll(filepath.Join(dir, "aaaa1111-0000", "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	return projectPath
}

func TestSanitize(t *testing.T) {
	if got := Sanitize("/Users/me/Code/my-app copy"); got != "-Users-me-Code-my-app-copy" {
		t.Errorf("Sanitize = %s", got)
	}
}

func TestResolveProjectAndMeta(t *testing.T) {
	path := fakeRoot(t)

	for _, in := range []string{path, strings.ToUpper(path[:len(path)-2]) + path[len(path)-2:], Sanitize(path)} {
		p, err := ResolveProject(in)
		if err != nil {
			t.Fatalf("ResolveProject(%q): %v", in, err)
		}
		if p.Path != path || p.Sessions != 2 {
			t.Errorf("ResolveProject(%q) = %+v", in, p)
		}
	}
	if _, err := ResolveProject(filepath.Join(path, "nope")); err == nil {
		t.Error("expected error for a project without sessions")
	}

	ss := ListSessions(filepath.Join(Root, Sanitize(path)))
	if len(ss) != 2 || ss[0].ID != "aaaa2222-0000" {
		t.Fatalf("ListSessions: newest first expected, got %+v", ss)
	}
	LoadMeta(ss)
	s := ss[1]
	if s.Title != "Про widget" || s.FirstLine != "Перше питання про Widget" || s.Branch != "dev" || s.Cwd != path {
		t.Errorf("meta = %+v", s)
	}
}

func TestFindByID(t *testing.T) {
	fakeRoot(t)
	if hits, err := FindByID("aaaa"); err != nil || len(hits) != 2 {
		t.Errorf("ambiguous prefix: %v %d", err, len(hits))
	}
	if hits, err := FindByID("aaaa1"); err != nil || len(hits) != 1 || hits[0].ID != "aaaa1111-0000" {
		t.Errorf("unique prefix: %v %+v", err, hits)
	}
	if _, err := FindByID("abc"); err == nil {
		t.Error("too short prefix must fail")
	}
	if _, err := FindByID("ffff"); err == nil {
		t.Error("unknown id must fail")
	}
}

func TestSearch(t *testing.T) {
	fakeRoot(t)
	hits := Search(AllSessions(true), "WIDGET")
	// Session 2 mentions widget only in a tool call — dialog-only search skips it.
	if len(hits) != 1 {
		t.Fatalf("hits = %+v", hits)
	}
	h := hits[0]
	if h.Session.ID != "aaaa1111-0000" || h.Count != 4 { // 1 user + 2 assistant + 1 title
		t.Errorf("hit = %+v", h)
	}
	if !strings.Contains(h.Snippet, "Widget") || h.Session.Title == "" {
		t.Errorf("snippet/meta = %q / %q", h.Snippet, h.Session.Title)
	}
}
