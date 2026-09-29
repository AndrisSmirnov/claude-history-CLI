package store

import (
	"bytes"
	"encoding/json"
	"strings"

	"history-cli/transcript"
)

// Hit is one session that matches a search query.
type Hit struct {
	Session Session
	Count   int    // number of matches in dialog text (+1 for a title match)
	Snippet string // context around the first match
}

// Search finds sessions whose dialog text (user/assistant, without tools) or AI title
// contains query, case-insensitively. Results keep the order of sessions (newest first)
// and come with meta loaded.
func Search(sessions []Session, query string) []Hit {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	// Raw-line prefilter is only safe when the query has nothing JSON would escape.
	prefilter := !strings.ContainsAny(q, "\"\\\n\t")
	qb := []byte(q)

	results := make([]*Hit, len(sessions)) // each worker writes only its own index
	parallel(len(sessions), func(i int) {
		s := sessions[i]
		h := Hit{Session: s}
		_ = transcript.Scan(s.File, func(line []byte) bool {
			if prefilter && !bytes.Contains(bytes.ToLower(line), qb) {
				return true
			}
			var r transcript.Record
			if json.Unmarshal(line, &r) != nil {
				return true
			}
			if r.Type == "ai-title" && strings.Contains(strings.ToLower(r.AITitle), q) {
				h.Count++
				return true
			}
			if !r.IsDialog() {
				return true
			}
			text, _ := transcript.Render(r.Content(), false)
			lower := strings.ToLower(text)
			if n := strings.Count(lower, q); n > 0 {
				if h.Snippet == "" {
					h.Snippet = snippet(text, lower, q)
				}
				h.Count += n
			}
			return true
		})
		if h.Count > 0 {
			results[i] = &h
		}
	})

	var hits []Hit
	for _, h := range results {
		if h != nil {
			hits = append(hits, *h)
		}
	}
	metas := make([]Session, len(hits))
	for i := range hits {
		metas[i] = hits[i].Session
	}
	LoadMeta(metas)
	for i := range hits {
		hits[i].Session = metas[i]
		if hits[i].Snippet == "" {
			hits[i].Snippet = "[назва] " + metas[i].Title
		}
	}
	return hits
}

// snippet returns ~50 runes of context on both sides of the first match.
// lower must be strings.ToLower(text); positions are mapped back by rune index.
func snippet(text, lower, q string) string {
	idx := strings.Index(lower, q)
	if idx < 0 {
		return transcript.OneLine(text, 120)
	}
	lr := []rune(lower)
	start := len([]rune(lower[:idx]))
	end := start + len([]rune(q))
	tr := []rune(text)
	if len(tr) != len(lr) { // ToLower changed rune count (rare) — fall back to lower-case text
		tr = lr
	}
	from, to := max(0, start-50), min(len(tr), end+50)
	out := transcript.OneLine(string(tr[from:to]), 200)
	if from > 0 {
		out = "…" + out
	}
	if to < len(tr) {
		out += "…"
	}
	return out
}
