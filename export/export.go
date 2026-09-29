// Package export writes a session transcript as Markdown — the same format as
// ~/Code/history-to-md.js (header, "## <label>" sections, optional tool blocks).
package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"history-cli/transcript"
)

// Options controls rendering.
type Options struct {
	Full      bool   // include tool calls (🔧 TOOL) and their results (↳ РЕЗУЛЬТАТ)
	UserLabel string // heading for the user's messages ("User" by default)
}

// Stats counts the rendered sections.
type Stats struct {
	User, Assistant, ToolResults int
	Bytes                        int
}

func (s Stats) String(full bool) string {
	tr := ""
	if full {
		tr = fmt.Sprintf(", tool-результатів: %d", s.ToolResults)
	}
	return fmt.Sprintf("user: %d, Claude: %d%s, %d KB", s.User, s.Assistant, tr, s.Bytes/1024)
}

// Render converts one .jsonl file into Markdown.
func Render(file string, opt Options) (string, Stats, error) {
	if opt.UserLabel == "" {
		opt.UserLabel = "User"
	}
	var md strings.Builder
	md.WriteString("# Історія сесії")
	if opt.Full {
		md.WriteString(" (повна, з інструментами)")
	}
	md.WriteString("\n\n_файл: " + filepath.Base(file) + "_\n")

	var st Stats
	data, err := os.ReadFile(file)
	if err != nil {
		return "", st, err
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r transcript.Record
		if json.Unmarshal(line, &r) != nil || !r.IsDialog() {
			continue
		}
		text, hasText := transcript.Render(r.Content(), opt.Full)
		if strings.TrimSpace(text) == "" {
			continue
		}
		var label string
		switch {
		case r.Type == "assistant":
			label = "Claude"
			st.Assistant++
		case hasText:
			label = opt.UserLabel
			st.User++
		default: // a user record without own text = tool_result
			label = "⚙️ Результат інструмента"
			st.ToolResults++
		}
		md.WriteString("\n\n## " + label + "\n\n" + text)
	}
	st.Bytes = md.Len()
	return md.String(), st, nil
}

// Convert renders file and writes it to out (creating parent folders).
func Convert(file, out string, opt Options) (Stats, error) {
	md, st, err := Render(file, opt)
	if err != nil {
		return st, err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return st, err
	}
	return st, os.WriteFile(out, []byte(md), 0o644)
}

var datePrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

// OutPath is where a session export goes:
//
//	<root>/<folder>/YYYY-MM-DD-<id8>[.full].md
//
// The date is the session's last activity (UTC, as in the JS tool). custom, when set,
// replaces the file name ("notes.md" → YYYY-MM-DD-notes.md); a custom value with
// a directory ("~/notes/x.md") is written to that directory instead of <root>/<folder>.
func OutPath(root, folder, file, custom string, full bool) string {
	date := "0000-00-00"
	if st, err := os.Stat(file); err == nil {
		date = st.ModTime().UTC().Format("2006-01-02")
	}
	id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
	if len(id) > 8 {
		id = id[:8]
	}

	base := id
	if full {
		base += ".full"
	}
	base += ".md"
	dir := filepath.Join(root, folder)
	if custom != "" {
		base = filepath.Base(custom)
		if filepath.Ext(base) == "" {
			base += ".md"
		}
		if d := filepath.Dir(custom); d != "." {
			dir = d
		}
	}
	if !datePrefix.MatchString(base) {
		base = date + "-" + base
	}
	return filepath.Join(dir, base)
}
