// hist — browse, search and export Claude Code session history as Markdown.
//
//	hist                      interactive menu (↑↓ Enter q)
//	hist -p <path>            interactive, straight into one project's sessions
//	hist ls|projects|search|export …   non-interactive commands (see `hist help`)
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"history-cli/app"
	"history-cli/config"
	"history-cli/export"
	"history-cli/prompt"
	"history-cli/store"
	"history-cli/tools"
)

// version is injected at build time: -ldflags "-X main.version=v1.0.0"
var version = "dev"

const usage = `hist — історія сесій Claude Code → Markdown

ІНТЕРАКТИВНО
  hist                         меню: ↑↓ вибір · Enter · q/Esc назад · / фільтр у таблицях
  hist -p <шлях>               одразу сесії проєкту (напр. hist -p .)

КОМАНДИ
  hist ls [-p <шлях>] [-n 30] [--temp]
                               таблиця сесій: одного проєкту або останніх з усіх
  hist projects [--temp]       усі проєкти з сесіями
  hist search <запит> [-p <шлях>] [--temp]
                               пошук по тексту діалогів і назвах сесій
  hist export <id|latest|файл.jsonl> [--full|--both] [-p <шлях>] [--dir <тека>] [--out <назва.md>]
  hist export --all [--full|--both] -p <шлях> [--dir <тека>]
  hist version | help

ПРАПОРИ
  -p, --project <шлях>  тека проєкту (як запускали claude), ~/…, "." або назва теки в ~/.claude/projects
  --full                з tool-викликами та результатами;  --both — обидва варіанти
  --dir <тека>          тека всередині history_root (або абсолютний шлях); інакше — запам'ятована/назва проєкту
  --out <назва.md>      власна назва файлу (дата-префікс додається)
  --temp                показувати також тимчасові проєкти (/private/var/folders, /tmp)

КУДИ ПИШЕ
  <history_root>/<тека>/YYYY-MM-DD-<id8>[.full].md   (history_root за замовч. ~/Code/history)
  налаштування: ~/.config/history-cli/config.json`

type flags struct {
	pos     []string
	project string
	full    bool
	both    bool
	all     bool
	temp    bool
	dir     string
	out     string
	n       int
}

func parse(args []string) (flags, error) {
	f := flags{n: 30}
	need := func(i int, name string) (string, error) {
		if i+1 >= len(args) {
			return "", fmt.Errorf("%s: потрібне значення", name)
		}
		return args[i+1], nil
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		var err error
		switch a {
		case "-p", "--project":
			f.project, err = need(i, a)
			i++
		case "--dir":
			f.dir, err = need(i, a)
			i++
		case "--out":
			f.out, err = need(i, a)
			i++
		case "-n":
			var v string
			if v, err = need(i, a); err == nil {
				f.n, err = strconv.Atoi(v)
			}
			i++
		case "--full":
			f.full = true
		case "--both":
			f.both = true
		case "--all":
			f.all = true
		case "--temp":
			f.temp = true
		default:
			f.pos = append(f.pos, a)
		}
		if err != nil {
			return f, err
		}
	}
	return f, nil
}

func main() {
	f, err := parse(os.Args[1:])
	if err != nil {
		die(err)
	}
	cfg := config.Load()

	cmd := ""
	if len(f.pos) > 0 {
		cmd = f.pos[0]
	}
	switch cmd {
	case "":
		interactive(cfg, f.project)
	case "help", "-h", "--help":
		fmt.Println(usage)
	case "version", "--version":
		fmt.Println(version)
	case "ls":
		err = cmdList(f)
	case "projects":
		err = cmdProjects(cfg, f)
	case "search":
		err = cmdSearch(f)
	case "export":
		err = cmdExport(cfg, f)
	default:
		err = fmt.Errorf("невідома команда %q (hist help)", cmd)
	}
	if err != nil {
		die(err)
	}
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "hist:", err)
	os.Exit(1)
}

func interactive(cfg *config.Config, project string) {
	if !app.IsTTY() {
		fmt.Println(usage)
		return
	}
	t := tools.New(cfg)
	app.Start()
	defer app.Stop()

	if project != "" {
		p, err := store.ResolveProject(project)
		if err != nil {
			app.Stop()
			die(err)
		}
		t.ProjectSessions(p)
		return
	}

	wd, _ := os.Getwd()
	build := func() (string, []app.Item) {
		items := []app.Item{}
		if p, err := store.ResolveProject(wd); err == nil {
			items = append(items, app.Item{
				Label:   fmt.Sprintf("Поточна тека: %s (%d)", store.ShortPath(p.Path), p.Sessions),
				Handler: t.Here, Separator: true,
			})
		}
		return "", append(items,
			app.Item{Label: "Останні сесії (всі проєкти)", Handler: t.Latest},
			app.Item{Label: "Проєкти", Handler: t.Projects},
			app.Item{Label: "Проєкт за шляхом…", Handler: t.ProjectByPath},
			app.Item{Label: "Пошук по історіях", Handler: t.Search},
			app.Item{Label: "Експорт за ID", Handler: t.ByID, Separator: true},
			app.Item{Label: "Зберегти всі сесії проєкту…", Handler: t.SaveAll, Separator: true},
			app.Item{Label: "Налаштування", Handler: t.Settings},
		)
	}
	title, items := build()
	root := &app.Menu{Title: title, Items: items, BackLabel: "Вихід", Rebuild: build}
	root.Run()
}

// — non-interactive commands —

func tableWidth() int {
	if app.IsTTY() {
		w, _ := app.Size()
		return w - 1
	}
	return 160
}

func printTable(cols []app.Column, rows [][]string) {
	header, lines := app.FormatTable(cols, rows, tableWidth())
	fmt.Println(header)
	for _, l := range lines {
		fmt.Println(l)
	}
}

func sessionTable(ss []store.Session, extra func(i int) string, extraTitle string) {
	cols := []app.Column{
		{Title: "#", Width: 3, Right: true},
		{Title: "АКТИВНІСТЬ", Width: 16},
		{Title: "KB", Width: 6, Right: true},
	}
	if extra != nil {
		cols = append(cols, app.Column{Title: extraTitle, Width: 6, Right: true})
	}
	cols = append(cols,
		app.Column{Title: "ID", Width: 8},
		app.Column{Title: "ПРОЄКТ", Width: 30, TailFirst: true},
		app.Column{Title: "НАЗВА / ПЕРШЕ ПОВІДОМЛЕННЯ"},
	)
	rows := make([][]string, len(ss))
	for i, s := range ss {
		r := []string{strconv.Itoa(i + 1), s.ModTime.Local().Format("2006-01-02 15:04"), strconv.FormatInt(s.Size/1024, 10)}
		if extra != nil {
			r = append(r, extra(i))
		}
		rows[i] = append(r, s.Short(), store.ShortPath(s.ProjectPath()), s.Label())
	}
	printTable(cols, rows)
}

func sessionsFor(f flags) ([]store.Session, error) {
	if f.project == "" {
		return store.AllSessions(f.temp), nil
	}
	p, err := store.ResolveProject(f.project)
	if err != nil {
		return nil, err
	}
	return store.ListSessions(p.Dir), nil
}

func cmdList(f flags) error {
	ss, err := sessionsFor(f)
	if err != nil {
		return err
	}
	total := len(ss)
	if f.n > 0 && len(ss) > f.n {
		ss = ss[:f.n]
	}
	store.LoadMeta(ss)
	sessionTable(ss, nil, "")
	fmt.Printf("\n%d з %d · експорт: hist export <ID> [--full]\n", len(ss), total)
	return nil
}

func cmdProjects(cfg *config.Config, f flags) error {
	ps, err := store.ListProjects()
	if err != nil {
		return err
	}
	var rows [][]string
	for _, p := range ps {
		if p.Temporary() && !f.temp {
			continue
		}
		rows = append(rows, []string{strconv.Itoa(len(rows) + 1), p.LastActive.Local().Format("2006-01-02 15:04"),
			strconv.Itoa(p.Sessions), cfg.FolderFor(p.Path), store.ShortPath(p.Path)})
	}
	printTable([]app.Column{
		{Title: "#", Width: 3, Right: true},
		{Title: "АКТИВНІСТЬ", Width: 16},
		{Title: "СЕСІЙ", Width: 5, Right: true},
		{Title: "ТЕКА ЕКСПОРТУ", Width: 16},
		{Title: "ШЛЯХ"},
	}, rows)
	return nil
}

func cmdSearch(f flags) error {
	q := strings.Join(f.pos[1:], " ")
	if strings.TrimSpace(q) == "" {
		return fmt.Errorf("search: вкажи запит")
	}
	ss, err := sessionsFor(f)
	if err != nil {
		return err
	}
	hits := store.Search(ss, q)
	found := make([]store.Session, len(hits))
	for i, h := range hits {
		found[i] = h.Session
	}
	sessionTable(found, func(i int) string { return strconv.Itoa(hits[i].Count) }, "ЗБІГІВ")
	fmt.Println()
	for i, h := range hits {
		if i == 20 {
			fmt.Printf("… ще %d\n", len(hits)-i)
			break
		}
		fmt.Printf("%3d. %s  %s\n", i+1, h.Session.Short(), h.Snippet)
	}
	fmt.Printf("\nЗнайдено сесій: %d · експорт: hist export <ID> [--full]\n", len(hits))
	return nil
}

func cmdExport(cfg *config.Config, f flags) error {
	var ss []store.Session
	switch {
	case f.all || (len(f.pos) > 1 && f.pos[1] == "--all"):
		if f.project == "" {
			return fmt.Errorf("export --all: вкажи проєкт через -p")
		}
		p, err := store.ResolveProject(f.project)
		if err != nil {
			return err
		}
		ss = store.ListSessions(p.Dir)
	case len(f.pos) < 2:
		return fmt.Errorf("export: вкажи <id|latest|файл.jsonl> або --all")
	case f.pos[1] == "latest":
		target := f.project
		if target == "" {
			target, _ = os.Getwd()
		}
		p, err := store.ResolveProject(target)
		if err != nil {
			return err
		}
		ss = store.ListSessions(p.Dir)[:1]
	default:
		hits, err := store.FindByID(f.pos[1])
		if err != nil {
			return err
		}
		if len(hits) > 1 {
			store.LoadMeta(hits)
			sessionTable(hits, nil, "")
			return fmt.Errorf("неоднозначний ID %q — уточни", f.pos[1])
		}
		ss = hits
	}
	if len(ss) == 0 {
		return store.ErrNoSessions
	}
	store.LoadMeta(ss)

	fulls := []bool{f.full}
	if f.both {
		fulls = []bool{false, true}
	}
	project := ss[0].ProjectPath()
	root, folder := cfg.HistoryRoot, f.dir
	if folder == "" {
		folder = cfg.FolderFor(project)
	}
	absolute := strings.HasPrefix(folder, "/") || strings.HasPrefix(folder, "~")
	if absolute {
		root, folder = "", prompt.ExpandHome(folder)
	}
	custom := ""
	if len(ss) == 1 {
		custom = f.out
	}

	for _, s := range ss {
		for _, full := range fulls {
			out := export.OutPath(root, folder, s.File, custom, full)
			st, err := export.Convert(s.File, out, export.Options{Full: full, UserLabel: cfg.UserLabel})
			if err != nil {
				return err
			}
			fmt.Printf("OK -> %s  (%s)\n", out, st.String(full))
		}
	}
	if f.dir != "" && !absolute {
		cfg.Remember(project, folder)
	}
	return nil
}
