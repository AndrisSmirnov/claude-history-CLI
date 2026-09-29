# history-cli: архітектура

`hist` — інтерактивний термінальний інструмент (меню ↑↓ / Enter / q) для перегляду, пошуку та експорту сесій Claude Code у Markdown. Це окремий Go-модуль `history-cli`; єдина зовнішня залежність — `golang.org/x/term`.

Формат Markdown — **байт-у-байт** той самий, що дає `~/Code/history-to-md.js`. Це закріплено golden-тестом (`export/testdata/*.golden.md` згенеровані JS-скриптом, підпис користувача замінено на `User`).

## Структура

```text
history-cli/
├── main.go            # розбір аргументів: інтерактивне меню або CLI-команди (ls/projects/search/export)
├── build.sh           # vet + test + build → bin/hist; `install` → ~/go/bin/hist; `dist` → dist/ (macOS/Linux)
├── app/               # UI-примітиви, нічого не знають про сесії
│   ├── term.go        #   raw/cooked режим, alt-screen, ReadKey (стрілки, PgUp/PgDn, UTF-8), Frame, WaitKey
│   ├── width.go       #   ширина символів (кирилиця/емодзі/CJK), Fit / FitLeft / FitRight
│   ├── menu.go        #   Menu: ↑↓ Enter q/Esc, цифри 1-9, Choose() для одноразового вибору
│   └── table.go       #   Picker: таблиця з прокруткою, фільтр "/", Extras (дії над таблицею), Detail-рядок; FormatTable
├── prompt/            # ввід рядка: Edit (з попереднім значенням), Path (Tab-доповнення тек), Ask, Bool (y/N однією клавішею)
├── transcript/        # декодування JSONL-записів і рендер контенту (порт render()/resultText() з JS), Scan()
├── store/             # доступ до ~/.claude/projects (без UI)
│   ├── project.go     #   Project, ListProjects, ResolveProject (шлях / ~ / "." / назва теки), Temporary, ShortPath
│   ├── session.go     #   Session, ListSessions, AllSessions, LoadMeta (паралельно), FindByID (префікс ≥ 4)
│   └── search.go      #   Search: паралельний пошук по тексту діалогів і ai-title, сніпет, кількість збігів
├── export/            # Render / Convert (jsonl → md) і OutPath (куди писати)
├── config/            # ~/.config/history-cli/config.json: history_root, user_label, folders, last_query
└── tools/             # екрани = пункти меню
    ├── tools.go       #   Tools, browse() — спільна таблиця сесій, колонки/рядки
    ├── screens.go     #   Latest, Here, Projects, ProjectSessions, ProjectByPath, Search, ByID, SaveAll, Settings
    └── action.go      #   exportSessions — спільний діалог збереження: режим → тека → превʼю → y/N → запис
```

## Напрям залежностей

```text
main ──► tools ──► store ──► transcript
  │        │  └──► export ──► transcript
  │        ├──► app, prompt
  │        └──► config
  └──► (CLI-режим) store, export, config, app.FormatTable
```

- `transcript`, `store`, `export` **не мають UI** — їх використовують і меню, і CLI-команди, і тести.
- `app` і `prompt` **не знають про сесії** — отримують готові рядки.
- Уся взаємодія з користувачем (порядок кроків, тексти) живе в `tools`.

## Звідки беруться дані

```text
~/.claude/projects/<sanitized-path>/<uuid>.jsonl      ← одна сесія
~/.claude/projects/<sanitized-path>/<uuid>/subagents/ ← ігнорується
```

- `sanitized-path` = шлях проєкту, де все, крім `[a-zA-Z0-9]`, замінено на `-` (`store.Sanitize`). Перетворення незворотне, тому **справжній шлях** береться з поля `cwd` записів (`firstCwd`, `LoadMeta`).
- Метадані сесії (`LoadMeta`): `Title` — останній запис `{"type":"ai-title"}`; `FirstLine` — перше реальне повідомлення користувача (без `isMeta` і службових `<command…>`); `Cwd`, `Branch` — з того самого запису.
- Дата активності — `mtime` файлу. У назві експорту — UTC-дата (як у JS), у таблицях — локальний час.
- «Тимчасові» проєкти (`/private/var/folders`, `/tmp`, шляхи з прихованими теками на кшталт `…/.claude/skills/…/workdir`) приховані в списках; `--temp` або пункт у «Проєкти» їх показує. Пошук за ID шукає всюди.

## Потік експорту

```text
tools.browse / ByID / SaveAll
   └─► tools.exportSessions(ss)
         ├─ app.Choose       режим: повний / діалог / обидва
         ├─ prompt.Path      тека: config.FolderFor(cwd) → можна змінити або дати абсолютний шлях
         ├─ export.OutPath   <history_root>/<тека>/YYYY-MM-DD-<id8>[.full].md  (+ позначка «перезапишу»)
         ├─ prompt.Bool      підтвердження
         ├─ export.Convert   запис файлів
         └─ config.Remember  запамʼятати теку для цього проєкту (наступного разу — за замовчуванням)
```

Спочатку `folders` порожній: тека за замовчуванням — назва теки проєкту (`~/Code/my-app → my-app`). Якщо під час збереження вказати іншу назву, вона запам'ятовується для цього проєкту.

## Модель екранів

- **Menu** (`app.Menu`) — список дій. `Handler` запускається в cooked-режимі на чистому екрані; після повернення меню перемальовується (`Rebuild` оновлює пункти, напр. «Поточна тека»).
- **Picker** (`app.Picker`) — таблиця. `Run()` повертає `Pick{Row}` або `Pick{Extra}`; стан (курсор, фільтр) зберігається між викликами, тож екран робить цикл «вибрав → дія → повернувся на те саме місце».
- Увесь інтерактив іде в alternate screen (`app.Start/Stop`), тож після виходу термінал чистий. Ctrl+C будь-де → `app.Exit()` відновлює термінал.

## Як розширювати

**Новий пункт меню.** Додай метод у `tools` (напр. `func (t *Tools) Stats()`), потім рядок у `build()` в `main.go`:
```go
app.Item{Label: "Статистика", Handler: t.Stats},
```

**Нова дія над таблицею сесій.** Передай `extra` у `browse`:
```go
t.browse(title, ss, false, extra{"» Видалити старі", func() { … }})
```

**Нова колонка.** Додай `app.Column` у `sessionColumns` і значення в `sessionRows` (`tools/tools.go`); для CLI — у `sessionTable` (`main.go`). `Width: 0` — гнучка колонка, `TailFirst` — обрізати з початку (шляхи).

**Нове поле метаданих.** Розшир `transcript.Record` (JSON-тег) і `store.Session`, заповни в `store.loadMeta`. Щоб не парсити кожен рядок, спершу перевіряй `bytes.Contains(line, tag)`.

**Новий формат експорту** (HTML, JSON…). Новий файл у `export/` з функцією на кшталт `RenderHTML(file, opt)`, використовуючи `transcript.Render`; додай режим у `modes` (`tools/action.go`) і прапор у `cmdExport`.

**Нова CLI-команда.** Кейс у `switch cmd` в `main.go` + функція `cmdX(cfg, f)`; прапори — у `parse`.

## Тести

```bash
go test ./...
```
- `export`: golden-порівняння з виводом JS (діалог і повний), `OutPath`.
- `store`: фейковий `Root` у TempDir — resolve (регістр, назва теки), метадані, `FindByID`, пошук.

Якщо зміниться логіка рендеру в JS і треба оновити golden-файли (`LABEL` — підпис користувача, зашитий у JS-скрипті):
```bash
F="$PWD/export/testdata/abcdef12-0000-0000-0000-000000000000.jsonl"; LABEL="…"
node history-to-md.js "$F" /tmp/g/dialog.md && node history-to-md.js "$F" --full /tmp/g/full.md
for m in dialog full; do sed "s/^## $LABEL\$/## User/" /tmp/g/*-$m.md > export/testdata/$m.golden.md; done
```
