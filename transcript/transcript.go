// Package transcript decodes Claude Code JSONL records and renders message content.
// Rendering rules are a 1:1 port of render()/resultText() from ~/Code/history-to-md.js,
// so the Markdown produced by export/ matches the JS tool byte for byte.
package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strings"
)

// Record is the subset of a JSONL line we care about.
type Record struct {
	Type      string          `json:"type"`
	Message   json.RawMessage `json:"message"`
	Cwd       string          `json:"cwd"`
	GitBranch string          `json:"gitBranch"`
	IsMeta    bool            `json:"isMeta"`
	AITitle   string          `json:"aiTitle"`
	Timestamp string          `json:"timestamp"`
}

// IsDialog reports whether the record is a user/assistant message with a message body.
func (r *Record) IsDialog() bool {
	return (r.Type == "user" || r.Type == "assistant") && len(r.Message) > 0 && string(r.Message) != "null"
}

// Content returns message.content (nil when absent or when message is not an object).
func (r *Record) Content() json.RawMessage {
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(r.Message, &m) != nil {
		return nil
	}
	return m.Content
}

type block struct {
	Type    string          `json:"type"`
	Text    *string         `json:"text"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	Content json.RawMessage `json:"content"`
}

// Render turns message content into text. full adds tool calls and tool results.
// hasText is true when the content has non-blank text of its own (a user message
// that only carries tool_result blocks has hasText=false).
func Render(content json.RawMessage, full bool) (text string, hasText bool) {
	content = bytes.TrimSpace(content)
	if len(content) == 0 {
		return "", false
	}
	switch content[0] {
	case '"':
		var s string
		if json.Unmarshal(content, &s) != nil {
			return "", false
		}
		return s, strings.TrimSpace(s) != ""
	case '[':
	default:
		return "", false
	}

	var raw []json.RawMessage
	if json.Unmarshal(content, &raw) != nil {
		return "", false
	}
	parts := make([]string, 0, len(raw))
	for _, r := range raw {
		var b block
		if json.Unmarshal(r, &b) != nil {
			continue
		}
		switch {
		case b.Type == "text":
			t := ""
			if b.Text != nil {
				t = *b.Text
			}
			parts = append(parts, t)
			if strings.TrimSpace(t) != "" {
				hasText = true
			}
		case full && b.Type == "tool_use":
			parts = append(parts, "```\n🔧 TOOL: "+b.Name+"\n"+indentJSON(b.Input)+"\n```")
		case full && b.Type == "tool_result":
			parts = append(parts, "```\n↳ РЕЗУЛЬТАТ:\n"+ResultText(b.Content)+"\n```")
		}
	}
	return strings.Join(parts, "\n\n"), hasText
}

// ResultText flattens tool_result content (string | array of blocks | anything).
func ResultText(rc json.RawMessage) string {
	rc = bytes.TrimSpace(rc)
	if len(rc) == 0 || string(rc) == "null" {
		return ""
	}
	switch rc[0] {
	case '"':
		var s string
		_ = json.Unmarshal(rc, &s)
		return s
	case '[':
		var items []json.RawMessage
		if json.Unmarshal(rc, &items) != nil {
			return compactJSON(rc)
		}
		out := make([]string, len(items))
		for i, it := range items {
			it = bytes.TrimSpace(it)
			if len(it) > 0 && it[0] == '"' {
				_ = json.Unmarshal(it, &out[i])
				continue
			}
			var b block
			if len(it) > 0 && it[0] == '{' && json.Unmarshal(it, &b) == nil && b.Type == "text" {
				if b.Text != nil {
					out[i] = *b.Text
				}
				continue
			}
			out[i] = compactJSON(it)
		}
		return strings.Join(out, "\n")
	}
	return compactJSON(rc)
}

// indentJSON mirrors JSON.stringify(v, null, 2): key order and escapes are kept as in the source.
func indentJSON(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "undefined"
	}
	var buf bytes.Buffer
	if json.Indent(&buf, raw, "", "  ") != nil {
		return string(raw)
	}
	return buf.String()
}

func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return string(raw)
	}
	return buf.String()
}

// Scan calls fn for every non-blank line of a JSONL file; fn returns false to stop early.
// Lines are only valid during the call.
func Scan(file string, fn func(line []byte) bool) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 512<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if !fn(line) {
			break
		}
	}
	return sc.Err()
}

// OneLine collapses whitespace and cuts s to n runes.
func OneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
