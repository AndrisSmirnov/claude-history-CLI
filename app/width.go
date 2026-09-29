package app

import "strings"

// RuneWidth approximates how many terminal cells r occupies (0, 1 or 2).
// Good enough for Latin/Cyrillic text, CJK and emoji — no external runewidth dependency.
func RuneWidth(r rune) int {
	switch {
	case r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFE0F || (r >= 0x0300 && r <= 0x036F):
		return 0
	case r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE4F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1FAFF,
		r >= 0x20000 && r <= 0x3FFFD:
		return 2
	}
	return 1
}

// Width returns the display width of s.
func Width(s string) int {
	w := 0
	for _, r := range s {
		w += RuneWidth(r)
	}
	return w
}

// Fit truncates s to w cells (ending with "…" when cut) and pads it with spaces to exactly w.
func Fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s + strings.Repeat(" ", w-Width(s))
	}
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := RuneWidth(r)
		if cur+rw > w-1 {
			break
		}
		b.WriteRune(r)
		cur += rw
	}
	b.WriteString("…")
	cur++
	return b.String() + strings.Repeat(" ", w-cur)
}

// FitLeft is Fit that keeps the tail of s ("…/projects/my-app") — for paths.
func FitLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s + strings.Repeat(" ", w-Width(s))
	}
	rs := []rune(s)
	cur := 1 // "…"
	i := len(rs)
	for i > 0 && cur+RuneWidth(rs[i-1]) <= w {
		i--
		cur += RuneWidth(rs[i])
	}
	return "…" + string(rs[i:]) + strings.Repeat(" ", w-cur)
}

// FitRight right-aligns s in w cells.
func FitRight(s string, w int) string {
	if Width(s) >= w {
		return Fit(s, w)
	}
	return strings.Repeat(" ", w-Width(s)) + s
}
