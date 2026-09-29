package app

import (
	"fmt"
	"os"
	"strings"
)

// Column describes one table column. Width 0 = flexible (takes the remaining space;
// the last flexible column wins). Right aligns right; TailFirst keeps the end of the
// value when truncating (paths).
type Column struct {
	Title     string
	Width     int
	Right     bool
	TailFirst bool
}

// FormatTable renders a header line and one line per row, fitted to total width.
func FormatTable(cols []Column, rows [][]string, total int) (header string, lines []string) {
	widths := columnWidths(cols, total)
	cell := func(c Column, w int, s string) string {
		switch {
		case c.Right:
			return FitRight(s, w)
		case c.TailFirst:
			return FitLeft(s, w)
		}
		return Fit(s, w)
	}
	join := func(vals []string) string {
		parts := make([]string, len(cols))
		for i, c := range cols {
			v := ""
			if i < len(vals) {
				v = vals[i]
			}
			parts[i] = cell(c, widths[i], v)
		}
		return strings.TrimRight(strings.Join(parts, "  "), " ")
	}
	titles := make([]string, len(cols))
	for i, c := range cols {
		titles[i] = c.Title
	}
	header = join(titles)
	for _, r := range rows {
		lines = append(lines, join(r))
	}
	return header, lines
}

func columnWidths(cols []Column, total int) []int {
	widths := make([]int, len(cols))
	used, flex := 0, -1
	for i, c := range cols {
		if c.Width == 0 {
			flex = i
			continue
		}
		widths[i] = c.Width
		used += c.Width
	}
	used += 2 * (len(cols) - 1)
	if flex >= 0 {
		widths[flex] = max(10, total-used)
	}
	return widths
}

// Pick is the Picker result: either an Extra action (index into Picker.Extras) or a Row.
type Pick struct {
	Extra int // -1 when a row was picked
	Row   int // -1 when an extra was picked
}

// Picker is a full-screen scrollable table: ↑↓ PgUp/PgDn Home/End, Enter picks,
// "/" filters rows by substring, q/ESC goes back. Extras are action lines shown above
// the table (e.g. "» Зберегти всі"). State (cursor, filter) survives repeated Run calls,
// so a caller can loop: pick → do something → Run again at the same place.
type Picker struct {
	Title   string // context lines above the table (may contain \n)
	Columns []Column
	Rows    [][]string
	Extras  []string
	Detail  func(row int) string // optional line under the table for the selected row
	Empty   string               // message when there are no rows

	cursor    int // over extras+visible rows
	offset    int // first visible row (in filtered space)
	filter    string
	filtering bool
}

// Run shows the table until the user picks something (ok=true) or goes back (ok=false).
func (p *Picker) Run() (Pick, bool) {
	if err := Raw(); err != nil {
		fmt.Fprintln(os.Stderr, "raw mode failed:", err)
		return Pick{}, false
	}
	defer Cooked()
	HideCursor()
	defer ShowCursor()
	Clear()

	for {
		visible := p.visibleRows()
		total := len(p.Extras) + len(visible)
		if p.cursor >= total {
			p.cursor = max(0, total-1)
		}
		page := p.frame(visible)

		k := ReadKey()
		if p.filtering {
			switch k.Code {
			case KeyEnter:
				p.filtering = false
			case KeyEsc:
				p.filtering, p.filter = false, ""
				p.cursor, p.offset = 0, 0
			case KeyBackspace:
				if r := []rune(p.filter); len(r) > 0 {
					p.filter = string(r[:len(r)-1])
					p.cursor, p.offset = len(p.Extras), 0
				}
			case KeyCtrlU:
				p.filter, p.cursor, p.offset = "", len(p.Extras), 0
			case KeyText:
				p.filter += k.Text
				p.cursor, p.offset = len(p.Extras), 0
			case KeyCtrlC:
				Exit()
			case KeyUp, KeyDown:
				p.filtering = false
				p.move(k.Code, total, page)
			}
			continue
		}

		switch k.Code {
		case KeyCtrlC:
			Exit()
		case KeyEsc:
			if p.filter != "" {
				p.filter, p.cursor, p.offset = "", 0, 0
				continue
			}
			return Pick{}, false
		case KeyEnter:
			if total == 0 {
				continue
			}
			if p.cursor < len(p.Extras) {
				return Pick{Extra: p.cursor, Row: -1}, true
			}
			return Pick{Extra: -1, Row: visible[p.cursor-len(p.Extras)]}, true
		case KeyText:
			switch k.Text {
			case "q", "Q", "й", "Й":
				return Pick{}, false
			case "/":
				p.filtering = true
			case "j":
				p.move(KeyDown, total, page)
			case "k":
				p.move(KeyUp, total, page)
			}
		default:
			p.move(k.Code, total, page)
		}
	}
}

func (p *Picker) move(code KeyCode, total, page int) {
	if total == 0 {
		return
	}
	switch code {
	case KeyUp:
		p.cursor--
	case KeyDown:
		p.cursor++
	case KeyPgUp, KeyLeft:
		p.cursor -= page
	case KeyPgDn, KeyRight:
		p.cursor += page
	case KeyHome:
		p.cursor = 0
	case KeyEnd:
		p.cursor = total - 1
	}
	p.cursor = min(max(p.cursor, 0), total-1)
}

// visibleRows returns indexes of rows matching the filter (all rows when no filter).
func (p *Picker) visibleRows() []int {
	f := strings.ToLower(strings.TrimSpace(p.filter))
	out := make([]int, 0, len(p.Rows))
	for i, r := range p.Rows {
		if f == "" || strings.Contains(strings.ToLower(strings.Join(r, " ")), f) {
			out = append(out, i)
		}
	}
	return out
}

// frame draws the screen and returns the page size (rows that fit).
func (p *Picker) frame(visible []int) int {
	tw, th := Size()
	inner := tw - 5 // "  " margin + "▶ " marker + 1 spare cell (avoid auto-wrap)

	var out []string
	if p.Title != "" {
		for _, t := range strings.Split(p.Title, "\n") {
			out = append(out, "  "+Fit(t, tw-4))
		}
		out = append(out, "")
	}

	for i, e := range p.Extras {
		if i == p.cursor {
			out = append(out, "  \033[7m"+Fit("▶ "+e, inner+2)+"\033[0m")
		} else {
			out = append(out, "    "+e)
		}
	}
	if len(p.Extras) > 0 {
		out = append(out, "")
	}

	header, lines := FormatTable(p.Columns, pick(p.Rows, visible), inner)
	out = append(out, "    \033[1m"+header+"\033[0m")
	out = append(out, "  "+strings.Repeat("─", tw-4))

	var detail string
	if p.Detail != nil && p.cursor >= len(p.Extras) && len(visible) > 0 {
		detail = p.Detail(visible[p.cursor-len(p.Extras)])
	}

	// footer: rule + detail + status/help
	reserved := len(out) + 1 + 2 + 1
	if p.Detail != nil {
		reserved++
	}
	page := max(3, th-reserved)

	rowCursor := p.cursor - len(p.Extras)
	if rowCursor >= 0 {
		if rowCursor < p.offset {
			p.offset = rowCursor
		}
		if rowCursor >= p.offset+page {
			p.offset = rowCursor - page + 1
		}
	}
	p.offset = min(p.offset, max(0, len(lines)-page))

	if len(lines) == 0 {
		msg := p.Empty
		if msg == "" {
			msg = "(порожньо)"
		}
		if p.filter != "" {
			msg = "нічого не знайдено за фільтром"
		}
		out = append(out, "    "+msg)
	}
	for i := p.offset; i < len(lines) && i < p.offset+page; i++ {
		if i == rowCursor {
			out = append(out, "  \033[7m"+Fit("▶ "+lines[i], inner+2)+"\033[0m")
		} else {
			out = append(out, "    "+lines[i])
		}
	}
	out = append(out, "  "+strings.Repeat("─", tw-4))
	if p.Detail != nil {
		out = append(out, "  "+Fit(detail, tw-4))
	}

	status := fmt.Sprintf("%d/%d", len(visible), len(p.Rows))
	if len(visible) > 0 && rowCursor >= 0 {
		status = fmt.Sprintf("%d · %s", rowCursor+1, status)
	}
	help := "↑↓ PgUp/PgDn · Enter вибрати · / фільтр · q назад"
	if p.filtering {
		help = "фільтр: " + p.filter + "▏  (Enter — готово, Esc — скинути)"
	} else if p.filter != "" {
		help = "фільтр: «" + p.filter + "» (Esc — скинути) · " + help
	}
	out = append(out, "  \033[2m"+Fit(help+"   ["+status+"]", tw-4)+"\033[0m")

	Frame(out)
	return page
}

func pick(rows [][]string, idx []int) [][]string {
	out := make([][]string, len(idx))
	for i, j := range idx {
		out[i] = rows[j]
	}
	return out
}
