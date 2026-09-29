package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"history-cli/app"
	"history-cli/export"
	"history-cli/prompt"
	"history-cli/store"
)

var modes = []string{
	"Повний (з інструментами)",
	"Діалог (лише текст)",
	"Обидва варіанти",
}

// exportSessions is the shared "save" screen: mode → folder → preview → confirm → write.
// All sessions are expected to come from one project (the folder default is taken from the first).
func (t *Tools) exportSessions(ss []store.Session) {
	if len(ss) == 0 {
		return
	}
	store.LoadMeta(ss)
	info := describe(ss)

	mode, ok := app.Choose(info+"\n \nЩо зберегти?", modes, 0)
	if !ok {
		return
	}
	var fulls []bool
	switch mode {
	case 0:
		fulls = []bool{true}
	case 1:
		fulls = []bool{false}
	default:
		fulls = []bool{false, true}
	}

	app.Clear()
	fmt.Println()
	for _, l := range strings.Split(info, "\n") {
		fmt.Println("  " + l)
	}
	fmt.Printf("\n  Режим: %s\n", modes[mode])
	fmt.Printf("  Тека: назва всередині %s або повний шлях (Tab — доповнення, Esc — назад)\n\n", store.ShortPath(t.cfg.HistoryRoot))

	project := ss[0].ProjectPath()
	folder, err := prompt.Path("Тека", t.cfg.FolderFor(project))
	if err != nil {
		return
	}
	folder = strings.TrimSpace(folder)
	if folder == "" {
		folder = t.cfg.FolderFor(project)
	}
	root := t.cfg.HistoryRoot
	absolute := strings.HasPrefix(folder, "/") || strings.HasPrefix(folder, "~")
	if absolute {
		root, folder = "", prompt.ExpandHome(folder)
	}

	type job struct {
		s    store.Session
		full bool
		out  string
	}
	var jobs []job
	for _, s := range ss {
		for _, f := range fulls {
			jobs = append(jobs, job{s, f, export.OutPath(root, folder, s.File, "", f)})
		}
	}

	fmt.Printf("\n  Буде записано файлів: %d\n", len(jobs))
	for i, j := range jobs {
		if i == 8 {
			fmt.Printf("    … ще %d\n", len(jobs)-i)
			break
		}
		mark := ""
		if _, err := os.Stat(j.out); err == nil {
			mark = "  (існує — перезапишу)"
		}
		fmt.Printf("    %s%s\n", store.ShortPath(j.out), mark)
	}
	fmt.Println()
	yes, err := prompt.Bool("Зберегти?", true)
	if err != nil || !yes {
		return
	}

	fmt.Println()
	var failed int
	for _, j := range jobs {
		st, err := export.Convert(j.s.File, j.out, export.Options{Full: j.full, UserLabel: t.cfg.UserLabel})
		if err != nil {
			failed++
			fmt.Printf("  ✗ %s: %v\n", j.s.Short(), err)
			continue
		}
		fmt.Printf("  ✓ %s  (%s)\n", store.ShortPath(j.out), st.String(j.full))
	}
	if !absolute {
		t.cfg.Remember(project, folder)
	}
	if failed > 0 {
		fmt.Printf("\n  Помилок: %d\n", failed)
	}

	dir := filepath.Dir(jobs[0].out)
	k := app.WaitKey("o — відкрити теку · будь-яка інша клавіша — назад")
	if k.Code == app.KeyText && (k.Text == "o" || k.Text == "O" || k.Text == "щ" || k.Text == "Щ") {
		openDir(dir)
	}
}

// describe is the info block shown above the export dialog.
func describe(ss []store.Session) string {
	if len(ss) == 1 {
		s := ss[0]
		lines := []string{
			"Сесія:      " + s.ID,
			"Проєкт:     " + store.ShortPath(s.ProjectPath()),
		}
		if s.Title != "" {
			lines = append(lines, "Назва:      "+s.Title)
		}
		if s.FirstLine != "" {
			lines = append(lines, "Початок:    "+transcriptCut(s.FirstLine, 90))
		}
		act := fmtTime(s) + fmt.Sprintf(" · %d KB", s.Size/1024)
		if s.Branch != "" {
			act += " · " + s.Branch
		}
		return strings.Join(append(lines, "Активність: "+act), "\n")
	}
	var size int64
	for _, s := range ss {
		size += s.Size
	}
	return strings.Join([]string{
		fmt.Sprintf("Сесій:      %d (%d KB)", len(ss), size/1024),
		"Проєкт:     " + store.ShortPath(ss[0].ProjectPath()),
		"Період:     " + ss[len(ss)-1].ModTime.Local().Format("2006-01-02") + " … " + ss[0].ModTime.Local().Format("2006-01-02"),
	}, "\n")
}

func transcriptCut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// openDir shows dir in the system file manager (Finder on macOS, xdg-open elsewhere).
func openDir(dir string) {
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	_ = exec.Command(cmd, dir).Start()
}
