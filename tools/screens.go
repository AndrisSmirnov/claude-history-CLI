package tools

import (
	"fmt"
	"strconv"

	"history-cli/app"
	"history-cli/config"
	"history-cli/prompt"
	"history-cli/store"
)

const latestLimit = 100

// Latest — newest sessions across all projects (temp/eval projects hidden).
func (t *Tools) Latest() {
	loading("Читаю сесії")
	all := store.AllSessions(false)
	ss := all
	if len(ss) > latestLimit {
		ss = ss[:latestLimit]
	}
	store.LoadMeta(ss)
	title := fmt.Sprintf("Останні сесії — %d з %d (всі проєкти)", len(ss), len(all))
	t.browse(title, ss, true)
}

// Here — sessions of the directory the program was started in.
func (t *Tools) Here() {
	p, err := store.ResolveProject(t.startDir)
	if err != nil {
		fail(err)
		return
	}
	t.ProjectSessions(p)
}

// ProjectByPath asks for a project directory and shows its sessions.
func (t *Tools) ProjectByPath() {
	p, ok := t.askProject("Шлях до проєкту")
	if ok {
		t.ProjectSessions(p)
	}
}

// SaveAll asks for a project and exports all of its sessions.
func (t *Tools) SaveAll() {
	p, ok := t.askProject("Шлях до проєкту")
	if !ok {
		return
	}
	ss := store.ListSessions(p.Dir)
	app.Clear()
	t.exportSessions(ss)
}

func (t *Tools) askProject(label string) (store.Project, bool) {
	fmt.Println()
	fmt.Println("  Тека проєкту, з якої запускали Claude Code (Tab — доповнення, Esc — назад)")
	fmt.Println()
	path, err := prompt.Path(label, store.ShortPath(t.startDir))
	if err != nil {
		return store.Project{}, false
	}
	p, err := store.ResolveProject(path)
	if err != nil {
		fail(err)
		return store.Project{}, false
	}
	return p, true
}

// ProjectSessions lists sessions of one project with "save all" / "search here" actions.
func (t *Tools) ProjectSessions(p store.Project) {
	loading("Читаю сесії")
	ss := store.ListSessions(p.Dir)
	store.LoadMeta(ss)
	path := p.Path
	if len(ss) > 0 {
		path = ss[0].ProjectPath()
	}
	title := fmt.Sprintf("Проєкт: %s\nСесій: %d · експорт у %s/%s",
		store.ShortPath(path), len(ss), store.ShortPath(t.cfg.HistoryRoot), t.cfg.FolderFor(path))
	t.browse(title, ss, false,
		extra{fmt.Sprintf("» Зберегти всі сесії (%d)", len(ss)), func() { t.exportSessions(ss) }},
		extra{"» Пошук у цьому проєкті", func() { t.searchIn(ss, store.ShortPath(path)) }},
	)
}

// Projects — every project with sessions, newest activity first.
func (t *Tools) Projects() {
	loading("Читаю проєкти")
	all, err := store.ListProjects()
	if err != nil {
		fail(err)
		return
	}
	showTemp := false
	build := func() ([]store.Project, *app.Picker) {
		var ps []store.Project
		temp := 0
		for _, p := range all {
			if p.Temporary() {
				temp++
				if !showTemp {
					continue
				}
			}
			ps = append(ps, p)
		}
		rows := make([][]string, len(ps))
		for i, p := range ps {
			rows[i] = []string{strconv.Itoa(i + 1), p.LastActive.Local().Format("2006-01-02 15:04"),
				strconv.Itoa(p.Sessions), t.cfg.FolderFor(p.Path), store.ShortPath(p.Path)}
		}
		toggle := fmt.Sprintf("» Показати тимчасові проєкти (%d)", temp)
		if showTemp {
			toggle = "» Сховати тимчасові проєкти"
		}
		return ps, &app.Picker{
			Title: fmt.Sprintf("Проєкти — %d", len(ps)),
			Columns: []app.Column{
				{Title: "#", Width: 3, Right: true},
				{Title: "АКТИВНІСТЬ", Width: 16},
				{Title: "СЕСІЙ", Width: 5, Right: true},
				{Title: "ТЕКА ЕКСПОРТУ", Width: 16},
				{Title: "ШЛЯХ", TailFirst: true},
			},
			Rows:   rows,
			Extras: []string{toggle},
		}
	}
	ps, picker := build()
	for {
		pick, ok := picker.Run()
		if !ok {
			return
		}
		app.Clear()
		if pick.Extra == 0 {
			showTemp = !showTemp
			ps, picker = build()
			continue
		}
		t.ProjectSessions(ps[pick.Row])
	}
}

// Search asks for a query and searches all (non-temp) projects.
func (t *Tools) Search() {
	t.searchIn(nil, "")
}

// searchIn searches ss (nil = all non-temp projects); scope is shown in the title.
func (t *Tools) searchIn(ss []store.Session, scope string) {
	for {
		app.Clear()
		fmt.Println()
		fmt.Println("  Пошук по тексту діалогів і назвах сесій, без урахування регістру (Esc — назад)")
		fmt.Println()
		q, err := prompt.Edit("Пошук", t.cfg.LastQuery)
		if err != nil || q == "" {
			return
		}
		t.cfg.LastQuery = q
		_ = t.cfg.Save()

		loading("Шукаю")
		pool := ss
		if pool == nil {
			pool = store.AllSessions(false)
			scope = "всі проєкти"
		}
		hits := store.Search(pool, q)
		found := make([]store.Session, len(hits))
		rows := make([][]string, len(hits))
		for i, h := range hits {
			s := h.Session
			found[i] = s
			rows[i] = []string{strconv.Itoa(i + 1), fmtTime(s), strconv.Itoa(h.Count), s.Short(),
				store.ShortPath(s.ProjectPath()), s.Label()}
		}
		p := &app.Picker{
			Title: fmt.Sprintf("Пошук «%s» (%s) — знайдено сесій: %d", q, scope, len(hits)),
			Columns: []app.Column{
				{Title: "#", Width: 3, Right: true},
				{Title: "АКТИВНІСТЬ", Width: 16},
				{Title: "ЗБІГІВ", Width: 6, Right: true},
				{Title: "ID", Width: 8},
				{Title: "ПРОЄКТ", Width: 30, TailFirst: true},
				{Title: "НАЗВА / ПЕРШЕ ПОВІДОМЛЕННЯ"},
			},
			Rows:   rows,
			Extras: []string{"» Новий пошук"},
			Detail: func(i int) string { return hits[i].Session.ID + " · " + hits[i].Snippet },
			Empty:  "нічого не знайдено",
		}
		for again := false; !again; {
			pick, ok := p.Run()
			if !ok {
				return
			}
			app.Clear()
			if pick.Extra == 0 {
				again = true
				continue
			}
			t.exportSessions([]store.Session{found[pick.Row]})
		}
	}
}

// ByID asks for a session hash / full id / .jsonl path and opens the export screen.
func (t *Tools) ByID() {
	fmt.Println()
	fmt.Println("  Хеш зі списку (перші 4+ символи), повний ID або шлях до .jsonl (Esc — назад)")
	fmt.Println()
	id, err := prompt.Ask("ID сесії")
	if err != nil {
		return
	}
	ss, err := store.FindByID(id)
	if err != nil {
		fail(err)
		return
	}
	if len(ss) == 1 {
		app.Clear()
		t.exportSessions(ss)
		return
	}
	store.LoadMeta(ss)
	t.browse(fmt.Sprintf("Кілька сесій з префіксом «%s»", id), ss, true)
}

// Settings edits the exports root and the user label.
func (t *Tools) Settings() {
	for {
		i, ok := app.Choose("Налаштування ("+store.ShortPath(config.File())+")", []string{
			"Тека для історій: " + store.ShortPath(t.cfg.HistoryRoot),
			"Підпис моїх повідомлень: " + t.cfg.UserLabel,
		}, 0)
		if !ok {
			return
		}
		app.Clear()
		fmt.Println()
		switch i {
		case 0:
			v, err := prompt.Path("Тека для історій", store.ShortPath(t.cfg.HistoryRoot))
			if err == nil && v != "" {
				t.cfg.HistoryRoot = prompt.ExpandHome(v)
			}
		case 1:
			v, err := prompt.Edit("Підпис", t.cfg.UserLabel)
			if err == nil && v != "" {
				t.cfg.UserLabel = v
			}
		}
		if err := t.cfg.Save(); err != nil {
			fail(err)
		}
	}
}
