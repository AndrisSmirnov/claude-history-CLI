// Package tools holds the interactive screens: every root-menu item is a method on Tools.
// Screens are built from app.Menu / app.Picker / prompt and call store + export for data.
package tools

import (
	"fmt"
	"os"
	"strconv"

	"history-cli/app"
	"history-cli/config"
	"history-cli/store"
)

// Tools is shared state for all screens.
type Tools struct {
	cfg      *config.Config
	startDir string // where the program was launched — default for path prompts
}

func New(cfg *config.Config) *Tools {
	wd, _ := os.Getwd()
	return &Tools{cfg: cfg, startDir: wd}
}

// extra is an action line above a session table ("» Зберегти всі…").
type extra struct {
	label string
	run   func()
}

// browse shows a session table; Enter on a row opens the export screen, then returns here.
func (t *Tools) browse(title string, ss []store.Session, showProject bool, extras ...extra) {
	p := &app.Picker{
		Title:   title,
		Columns: sessionColumns(showProject),
		Rows:    sessionRows(ss, showProject),
		Detail:  func(i int) string { return sessionDetail(ss[i]) },
		Empty:   "сесій немає",
	}
	for _, e := range extras {
		p.Extras = append(p.Extras, e.label)
	}
	for {
		pick, ok := p.Run()
		if !ok {
			return
		}
		app.Clear()
		if pick.Extra >= 0 {
			extras[pick.Extra].run()
			continue
		}
		t.exportSessions([]store.Session{ss[pick.Row]})
	}
}

func sessionColumns(showProject bool) []app.Column {
	cols := []app.Column{
		{Title: "#", Width: 3, Right: true},
		{Title: "АКТИВНІСТЬ", Width: 16},
		{Title: "KB", Width: 6, Right: true},
		{Title: "ID", Width: 8},
	}
	if showProject {
		cols = append(cols, app.Column{Title: "ПРОЄКТ", Width: 30, TailFirst: true})
	}
	return append(cols, app.Column{Title: "НАЗВА / ПЕРШЕ ПОВІДОМЛЕННЯ"})
}

func sessionRows(ss []store.Session, showProject bool) [][]string {
	rows := make([][]string, len(ss))
	for i, s := range ss {
		r := []string{strconv.Itoa(i + 1), fmtTime(s), strconv.FormatInt(s.Size/1024, 10), s.Short()}
		if showProject {
			r = append(r, store.ShortPath(s.ProjectPath()))
		}
		rows[i] = append(r, s.Label())
	}
	return rows
}

func sessionDetail(s store.Session) string {
	d := s.ID
	if s.Branch != "" {
		d += " · " + s.Branch
	}
	return d + " · " + store.ShortPath(s.ProjectPath())
}

func fmtTime(s store.Session) string { return s.ModTime.Local().Format("2006-01-02 15:04") }

// loading prints a status line while something slow happens (screen is in cooked mode).
func loading(msg string) { fmt.Printf("\n  %s…\n", msg) }

// fail shows an error and waits for a key.
func fail(err error) {
	fmt.Printf("\n  ✗ %v\n", err)
	app.WaitKey("Будь-яка клавіша — назад")
}
