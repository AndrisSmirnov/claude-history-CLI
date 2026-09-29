// Package store reads Claude Code session transcripts from ~/.claude/projects.
// It has no UI: tools/ and the CLI mode both build on it.
//
// Layout on disk:
//
//	~/.claude/projects/<sanitized project path>/<session uuid>.jsonl
//
// The sanitized name is lossy ("/" and "-" both become "-"), so the real project
// path is read from the "cwd" field of the session records instead.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Root is where Claude Code keeps per-project session folders.
var Root = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}()

// Project is one folder in Root.
type Project struct {
	Key        string    // folder name in Root, e.g. -Users-me-Code-my-app
	Dir        string    // absolute path of that folder
	Path       string    // real project path from session records (falls back to Key)
	Sessions   int       // number of .jsonl files
	LastActive time.Time // newest session mtime
}

// Temporary reports whether the project lives in a temp or hidden dir (eval runs,
// scratch, …/.claude/skills/…/workdir) — such projects are hidden from the default lists.
func (p Project) Temporary() bool {
	if strings.Contains(p.Path, "/.") {
		return true
	}
	for _, pre := range []string{"/private/var/folders/", "/var/folders/", "/private/tmp/", "/tmp/"} {
		if strings.HasPrefix(p.Path+"/", pre) {
			return true
		}
	}
	return strings.HasPrefix(p.Key, "-private-var-folders-") || strings.HasPrefix(p.Key, "-private-tmp-")
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// Sanitize maps a project path to its folder name the way Claude Code does.
func Sanitize(p string) string { return nonAlnum.ReplaceAllString(p, "-") }

// ListProjects returns every project in Root, newest activity first.
func ListProjects() ([]Project, error) {
	entries, err := os.ReadDir(Root)
	if err != nil {
		return nil, err
	}
	var out []Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, ok := loadProject(filepath.Join(Root, e.Name()))
		if ok {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastActive.After(out[j].LastActive) })
	return out, nil
}

// loadProject describes a folder; ok=false when it holds no sessions.
func loadProject(dir string) (Project, bool) {
	sessions := ListSessions(dir)
	if len(sessions) == 0 {
		return Project{}, false
	}
	p := Project{
		Key:        filepath.Base(dir),
		Dir:        dir,
		Path:       filepath.Base(dir),
		Sessions:   len(sessions),
		LastActive: sessions[0].ModTime,
	}
	for _, s := range sessions {
		if cwd := firstCwd(s.File); cwd != "" {
			p.Path = cwd
			break
		}
	}
	return p, true
}

// ErrNoSessions means the project folder was not found or holds no sessions.
var ErrNoSessions = errors.New("сесій не знайдено")

// ResolveProject finds the sessions folder for input, which may be:
//   - a project path (absolute, relative or ~/…), e.g. ~/Code/my-app or "."
//   - a folder name inside Root (-Users-me-Code-…)
//   - a sessions folder itself (a directory that contains .jsonl files)
//
// Matching of project paths is case-insensitive (Backend vs BACKEND), like the JS original.
func ResolveProject(input string) (Project, error) {
	input = expandHome(strings.TrimSpace(input))
	if input == "" {
		input = "."
	}
	if st, err := os.Stat(input); err == nil && st.IsDir() && len(ListSessions(input)) > 0 {
		if p, ok := loadProject(input); ok {
			return p, nil
		}
	}
	if !strings.ContainsRune(input, '/') {
		if p, ok := loadProject(filepath.Join(Root, input)); ok {
			return p, nil
		}
	}

	abs, err := filepath.Abs(input)
	if err != nil {
		return Project{}, err
	}
	want := Sanitize(abs)
	entries, _ := os.ReadDir(Root)
	var ci string
	for _, e := range entries {
		if e.Name() == want {
			ci = e.Name()
			break
		}
		if ci == "" && strings.EqualFold(e.Name(), want) {
			ci = e.Name()
		}
	}
	if ci != "" {
		if p, ok := loadProject(filepath.Join(Root, ci)); ok {
			return p, nil
		}
	}
	return Project{Key: want, Dir: filepath.Join(Root, want), Path: abs}, fmt.Errorf("%w для %s", ErrNoSessions, abs)
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

// ShortPath replaces the home directory with "~" for display.
func ShortPath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
