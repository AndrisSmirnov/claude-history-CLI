package app

import (
	"fmt"
	"os"
	"strings"
)

const minBoxWidth = 52

// Item is a single selectable entry in a Menu.
// If Handler is nil the item is a no-op placeholder.
// If Separator is true a ├─┤ divider is drawn after this item.
type Item struct {
	Label     string
	Handler   func()
	Separator bool
}

// Menu is an interactive list navigable with ↑↓ and Enter.
// Nest menus by setting Handler to another Menu's Run method.
//
// Title is optional and may span several lines (context: project, session…).
// Rebuild refreshes Title and Items after every handler call.
// BackLabel overrides the auto-appended back item ("← Назад").
// ExitOnSelect closes the menu right after a handler runs.
// DefaultIndex is where the cursor starts.
type Menu struct {
	Title        string
	Items        []Item
	BackLabel    string
	Rebuild      func() (title string, items []Item)
	ExitOnSelect bool
	DefaultIndex int
}

// Run renders the menu and blocks until the user leaves it (q / ESC / back item).
func (m *Menu) Run() {
	if len(m.Items) == 0 {
		return
	}
	selected := 0
	if m.DefaultIndex > 0 && m.DefaultIndex < len(m.Items) {
		selected = m.DefaultIndex
	}

	for {
		if err := Raw(); err != nil {
			fmt.Fprintln(os.Stderr, "raw mode failed:", err)
			return
		}
		HideCursor()
		Clear()
		total := len(m.Items) + 1 // + back item

		for picked := false; !picked; {
			Frame(m.lines(selected))
			k := ReadKey()
			switch k.Code {
			case KeyUp:
				if selected > 0 {
					selected--
				} else {
					selected = total - 1
				}
			case KeyDown:
				if selected < total-1 {
					selected++
				} else {
					selected = 0
				}
			case KeyHome:
				selected = 0
			case KeyEnd:
				selected = total - 1
			case KeyCtrlC:
				Exit()
			case KeyEsc:
				Cooked()
				return
			case KeyText:
				if k.Text == "q" || k.Text == "Q" || k.Text == "й" || k.Text == "Й" {
					Cooked()
					return
				}
				// digits jump straight to an item
				if len(k.Text) == 1 && k.Text[0] >= '1' && k.Text[0] <= '9' {
					if i := int(k.Text[0] - '1'); i < len(m.Items) {
						selected = i
					}
				}
			case KeyEnter:
				if selected == len(m.Items) {
					Cooked()
					return
				}
				if m.Items[selected].Handler != nil {
					picked = true
				}
			}
		}

		// Run the handler in cooked mode on a clean screen.
		Cooked()
		ShowCursor()
		Clear()
		m.Items[selected].Handler()

		if m.ExitOnSelect {
			return
		}
		if m.Rebuild != nil {
			m.Title, m.Items = m.Rebuild()
			if selected > len(m.Items) {
				selected = len(m.Items)
			}
		}
	}
}

func (m *Menu) backLabel() string {
	if m.BackLabel != "" {
		return m.BackLabel
	}
	return "← Назад"
}

func (m *Menu) lines(selected int) []string {
	tw, _ := Size()
	width := minBoxWidth
	for _, it := range m.Items {
		width = max(width, Width(it.Label)+6)
	}
	for _, t := range strings.Split(m.Title, "\n") {
		width = max(width, Width(t)+4)
	}
	width = min(width, tw-4)

	var out []string
	bar := strings.Repeat("─", width)
	out = append(out, "  ┌"+bar+"┐")
	if m.Title != "" {
		for _, t := range strings.Split(m.Title, "\n") {
			out = append(out, "  │ "+Fit(" "+t, width-2)+" │")
		}
		out = append(out, "  ├"+bar+"┤")
	}

	row := func(i int, label string) string {
		if i == selected {
			return "  │\033[7m" + Fit(" ▶ "+label, width) + "\033[0m│"
		}
		return "  │" + Fit("   "+label, width) + "│"
	}
	for i, it := range m.Items {
		out = append(out, row(i, it.Label))
		if it.Separator {
			out = append(out, "  ├"+bar+"┤")
		}
	}
	out = append(out, "  ├"+bar+"┤")
	out = append(out, row(len(m.Items), m.backLabel()))
	out = append(out, "  ├"+bar+"┤")
	out = append(out, "  │ "+Fit("  ↑↓ вибір · Enter відкрити · q/Esc назад", width-2)+" │")
	out = append(out, "  └"+bar+"┘")
	return out
}

// Choose shows a one-shot menu and returns the picked option index, or ok=false on back/ESC.
func Choose(title string, options []string, defaultIndex int) (index int, ok bool) {
	index = -1
	m := &Menu{Title: title, ExitOnSelect: true, DefaultIndex: defaultIndex}
	for i, o := range options {
		m.Items = append(m.Items, Item{Label: o, Handler: func() { index = i }})
	}
	m.Run()
	return index, index >= 0
}
