// Package prompt provides interactive line input with ESC-to-cancel,
// a pre-filled editable value and Tab completion for paths.
package prompt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"history-cli/app"
)

// ErrCancelled is returned when the user presses ESC.
var ErrCancelled = errors.New("cancelled")

// Ask reads a required string. Reprompts on empty input. ESC → ErrCancelled.
func Ask(label string) (string, error) {
	for {
		v, err := Edit(label, "")
		if err != nil || strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), err
		}
		fmt.Printf("  %s не може бути порожнім.\n", label)
	}
}

// Edit reads a line pre-filled with initial (editable: Backspace, Ctrl+U clears). ESC → ErrCancelled.
func Edit(label, initial string) (string, error) {
	return readLine(label, initial, nil)
}

// Path is Edit with Tab completion of directories ("~" is expanded on completion).
func Path(label, initial string) (string, error) {
	return readLine(label, initial, completeDir)
}

// Bool asks a yes/no question with a single keypress. Enter = default. ESC → ErrCancelled.
func Bool(label string, defaultYes bool) (bool, error) {
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	fmt.Printf("  %s %s: ", label, hint)

	if err := app.Raw(); err != nil {
		var s string
		fmt.Scanln(&s)
		return s == "y" || s == "Y" || (s == "" && defaultYes), nil
	}
	defer app.Cooked()

	for {
		k := app.ReadKey()
		switch k.Code {
		case app.KeyCtrlC:
			app.Exit()
		case app.KeyEsc:
			fmt.Print("\r\n")
			return false, ErrCancelled
		case app.KeyEnter:
			if defaultYes {
				fmt.Print("y\r\n")
			} else {
				fmt.Print("n\r\n")
			}
			return defaultYes, nil
		case app.KeyText:
			switch k.Text {
			case "y", "Y", "н", "Н": // "н" — the same key on a Ukrainian layout
				fmt.Print("y\r\n")
				return true, nil
			case "n", "N", "т", "Т":
				fmt.Print("n\r\n")
				return false, nil
			}
		}
	}
}

// readLine is a tiny line editor in raw mode. complete (optional) handles Tab.
func readLine(label, initial string, complete func(string) string) (string, error) {
	if err := app.Raw(); err != nil {
		fmt.Printf("  %s: ", label)
		var s string
		fmt.Scanln(&s)
		if s == "" {
			s = initial
		}
		return s, nil
	}
	defer app.Cooked()
	app.ShowCursor()

	buf := []rune(initial)
	// Keep the input on one screen row: show only the tail when it is too long,
	// otherwise a wrapped line would leave garbage behind on redraw.
	redraw := func() {
		tw, _ := app.Size()
		room := tw - app.Width("  "+label+": ") - 2
		shown := string(buf)
		if app.Width(shown) > room {
			shown = strings.TrimRight(app.FitLeft(shown, room), " ")
		}
		fmt.Printf("\r\033[K  %s: %s", label, shown)
	}
	redraw()

	for {
		k := app.ReadKey()
		switch k.Code {
		case app.KeyCtrlC:
			app.Exit()
		case app.KeyEsc:
			fmt.Print("\r\n")
			return "", ErrCancelled
		case app.KeyEnter:
			fmt.Print("\r\n")
			return string(buf), nil
		case app.KeyBackspace:
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
			}
		case app.KeyCtrlU:
			buf = buf[:0]
		case app.KeyTab:
			if complete != nil {
				buf = []rune(complete(string(buf)))
			}
		case app.KeyText:
			buf = append(buf, []rune(k.Text)...)
		}
		redraw()
	}
}

// ExpandHome turns "~" / "~/x" into an absolute path.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

// completeDir extends s to the longest common prefix of matching sub-directories.
// A single match gets a trailing "/" so the next Tab descends into it.
func completeDir(s string) string {
	full := ExpandHome(s)
	dir, prefix := filepath.Split(full)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return s
	}
	var matches []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		if e.IsDir() || isDirLink(filepath.Join(dir, name)) {
			matches = append(matches, name)
		}
	}
	if len(matches) == 0 {
		return s
	}
	sort.Strings(matches)
	common := matches[0]
	for _, m := range matches[1:] {
		common = commonPrefix(common, m)
	}
	if len([]rune(common)) < len([]rune(prefix)) {
		return s
	}
	head := strings.TrimSuffix(s, prefix)
	if strings.HasSuffix(s, "/") || prefix == "" {
		head = s
	}
	if len(matches) == 1 {
		return head + common + "/"
	}
	return head + common
}

func isDirLink(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func commonPrefix(a, b string) string {
	ra, rb := []rune(a), []rune(b)
	i := 0
	for i < len(ra) && i < len(rb) && strings.EqualFold(string(ra[i]), string(rb[i])) {
		i++
	}
	return string(ra[:i])
}
