package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"history-cli/transcript"
)

// Session is one <uuid>.jsonl transcript. Fields below "meta" are filled by LoadMeta.
type Session struct {
	ID         string
	File       string
	ModTime    time.Time // last activity
	Size       int64
	ProjectKey string // folder name in Root

	// meta
	Title     string // last "ai-title" record, if any
	FirstLine string // first real user message, one line
	Cwd       string // project path the session was started in
	Branch    string // git branch at the first user message
	loaded    bool
}

// Short is the 8-char hash shown in tables and used in file names.
func (s Session) Short() string {
	if len(s.ID) > 8 {
		return s.ID[:8]
	}
	return s.ID
}

// ProjectPath is the session's real project path (cwd), or the folder name as a fallback.
func (s Session) ProjectPath() string {
	if s.Cwd != "" {
		return s.Cwd
	}
	return s.ProjectKey
}

// Label is the best human description: AI title, else the first user line.
func (s Session) Label() string {
	switch {
	case s.Title != "" && s.FirstLine != "":
		return s.Title + " — " + s.FirstLine
	case s.Title != "":
		return s.Title
	case s.FirstLine != "":
		return s.FirstLine
	}
	return "(без тексту)"
}

// ListSessions returns the .jsonl sessions of one sessions folder, newest first.
// Subagent transcripts (<uuid>/subagents/…) are not included.
func ListSessions(dir string) []Session {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	key := filepath.Base(dir)
	var out []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Session{
			ID:         strings.TrimSuffix(e.Name(), ".jsonl"),
			File:       filepath.Join(dir, e.Name()),
			ModTime:    info.ModTime(),
			Size:       info.Size(),
			ProjectKey: key,
		})
	}
	sortNewest(out)
	return out
}

// AllSessions returns sessions of every project in Root, newest first.
// includeTemp=false skips projects living in temp dirs (see Project.Temporary).
func AllSessions(includeTemp bool) []Session {
	entries, _ := os.ReadDir(Root)
	var out []Session
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		ss := ListSessions(filepath.Join(Root, e.Name()))
		if len(ss) == 0 {
			continue
		}
		if !includeTemp && (Project{Key: e.Name(), Path: firstCwd(ss[0].File)}).Temporary() {
			continue
		}
		out = append(out, ss...)
	}
	sortNewest(out)
	return out
}

func sortNewest(ss []Session) {
	sort.SliceStable(ss, func(i, j int) bool { return ss[i].ModTime.After(ss[j].ModTime) })
}

// LoadMeta fills title / first line / cwd / branch for every session, in parallel.
func LoadMeta(ss []Session) {
	parallel(len(ss), func(i int) {
		if !ss[i].loaded {
			loadMeta(&ss[i])
		}
	})
}

var (
	tagAITitle = []byte(`"type":"ai-title"`)
	tagUser    = []byte(`"type":"user"`)
	tagCwd     = []byte(`"cwd":`)
)

func loadMeta(s *Session) {
	var fallback string
	_ = transcript.Scan(s.File, func(line []byte) bool {
		switch {
		case bytes.Contains(line, tagAITitle):
			var r transcript.Record
			if json.Unmarshal(line, &r) == nil && r.AITitle != "" {
				s.Title = transcript.OneLine(r.AITitle, 80)
			}
		case s.FirstLine == "" && bytes.Contains(line, tagUser):
			var r transcript.Record
			if json.Unmarshal(line, &r) != nil || !r.IsDialog() || r.Type != "user" {
				return true
			}
			if s.Cwd == "" {
				s.Cwd = r.Cwd
			}
			text, hasText := transcript.Render(r.Content(), false)
			if !hasText {
				return true
			}
			line := transcript.OneLine(text, 200)
			// Slash-command wrappers and meta records are noise for a table label.
			if r.IsMeta || strings.HasPrefix(line, "<") {
				if fallback == "" {
					fallback = line
				}
				return true
			}
			s.FirstLine = line
			if r.Cwd != "" {
				s.Cwd = r.Cwd
			}
			s.Branch = r.GitBranch
		}
		return true
	})
	if s.FirstLine == "" {
		s.FirstLine = fallback
	}
	if s.Cwd == "" {
		s.Cwd = firstCwd(s.File)
	}
	s.loaded = true
}

// firstCwd returns the first "cwd" value found in the file (cheap: stops early).
func firstCwd(file string) string {
	var cwd string
	_ = transcript.Scan(file, func(line []byte) bool {
		if !bytes.Contains(line, tagCwd) {
			return true
		}
		var r transcript.Record
		if json.Unmarshal(line, &r) == nil && r.Cwd != "" {
			cwd = r.Cwd
			return false
		}
		return true
	})
	return cwd
}

// FindByID resolves arg to sessions: a path to a .jsonl file, a full session id,
// or an id prefix (≥ 4 chars) — searched across all projects (temp ones included).
// More than one result means the prefix is ambiguous.
func FindByID(arg string) ([]Session, error) {
	arg = expandHome(strings.TrimSpace(arg))
	if arg == "" {
		return nil, fmt.Errorf("порожній ID")
	}
	if strings.HasSuffix(arg, ".jsonl") || strings.ContainsRune(arg, '/') {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, fmt.Errorf("файл не знайдено: %s", arg)
		}
		abs, _ := filepath.Abs(arg)
		return []Session{{
			ID:         strings.TrimSuffix(filepath.Base(abs), ".jsonl"),
			File:       abs,
			ModTime:    info.ModTime(),
			Size:       info.Size(),
			ProjectKey: filepath.Base(filepath.Dir(abs)),
		}}, nil
	}
	if len(arg) < 4 {
		return nil, fmt.Errorf("ID закороткий: потрібно щонайменше 4 символи")
	}
	var hits []Session
	for _, s := range AllSessions(true) {
		if s.ID == arg {
			return []Session{s}, nil
		}
		if strings.HasPrefix(s.ID, strings.ToLower(arg)) {
			hits = append(hits, s)
		}
	}
	if len(hits) == 0 {
		return nil, fmt.Errorf("сесію %q не знайдено", arg)
	}
	return hits, nil
}

// parallel runs fn(0..n-1) on a bounded worker pool.
func parallel(n int, fn func(i int)) {
	workers := min(runtime.NumCPU(), n)
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				fn(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		next <- i
	}
	close(next)
	wg.Wait()
}
