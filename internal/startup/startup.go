// Package startup — чистая логика автозагрузки учёток детей (FR-39, RM5b-1…RM5b-7): разбор
// команд записей, значений StartupApproved, XML заданий Планировщика, скрываемое, подписи
// «откуда» и сверка найденного с сохранённым. Только стандартная библиотека; чтение и запись
// Windows — порт StartupSource службы (winhost).
package startup

import "strings"

// Entry — запись автозагрузки, найденная в Windows. Ключ записи — Scope + Source + Name.
type Entry struct {
	Scope, Source, Name, Command, Publisher, Exe string
	Enabled                                      bool
	Task                                         string // путь задания Планировщика (\Папка\Имя)
	Package                                      string // семейство пакета приложения магазина
}

// Области и источники записей (значения startup_items.scope и source, ТЗ раздел 4).
const (
	ScopeOwn    = "own"
	ScopeShared = "shared"
)

const (
	SrcRun     = "run"
	SrcRun32   = "run32" // общая HKLM\SOFTWARE\WOW6432Node\...\Run (состояние — StartupApproved\Run32)
	SrcRunOnce = "runonce"
	SrcFolder  = "startup_folder"
	SrcTask    = "task"
	SrcStore   = "store_app"
)

// FromText — подпись «откуда» в строке списка (ТЗ 8.3.11); незнакомый источник — "".
func FromText(e Entry) string {
	switch e.Source {
	case SrcRun, SrcRun32:
		return "Реестр, при входе"
	case SrcRunOnce:
		return "Реестр, один раз при входе"
	case SrcFolder:
		return "Папка «Автозагрузка»"
	case SrcTask:
		return "Планировщик заданий, при входе"
	case SrcStore:
		return "Приложение из магазина"
	}
	return ""
}

// DisplayName — название записи в строке списка: у приложения магазина — понятное имя по
// семейству пакета (часть до «_» без издательского префикса до первой точки: «SpotifyMusic»
// из «SpotifyAB.SpotifyMusic_zpdnekdrzrea0»; без пакета — по Name до «!»); у остальных — Name.
// Пустое имя не получается: тогда остаётся Name.
func DisplayName(e Entry) string {
	if e.Source != SrcStore {
		return e.Name
	}
	pkg := e.Package
	if pkg == "" {
		pkg, _, _ = strings.Cut(e.Name, "!")
	}
	family, _, _ := strings.Cut(pkg, "_")
	if _, app, ok := strings.Cut(family, "."); ok && app != "" {
		return app
	}
	if family == "" {
		return e.Name
	}
	return family
}

// Hidden — запись не показывается и не выключается (RM5b-7, Global Constraint 8): пустая
// команда, задание под \Microsoft\, exe под windir (кроме windir\Temp, куда пишут установщики),
// exe в папке установки PC Block. Пустой windir или pcblockDir — эта проверка не делается;
// exe не разобран (e.Exe пусто) — запись показывается.
func Hidden(e Entry, windir, pcblockDir string) bool {
	if strings.TrimSpace(e.Command) == "" {
		return true
	}
	if e.Source == SrcTask && hasPrefixFold(e.Task, `\Microsoft\`) {
		return true
	}
	exe := normPath(e.Exe)
	if exe == "" {
		return false
	}
	if under(exe, windir) && !under(exe, windir+`\Temp`) {
		return true
	}
	return under(exe, pcblockDir)
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// normPath — путь для сравнения: обратные косые, нижний регистр, без косой в конце.
func normPath(p string) string {
	p = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(p), "/", `\`))
	return strings.TrimRight(p, `\`)
}

// under — нормализованный путь p лежит внутри папки dir. Корень диска («C:\») папкой не
// считается: иначе под неё попало бы всё.
func under(p, dir string) bool {
	d := normPath(dir)
	if !strings.Contains(d, `\`) {
		return false
	}
	return strings.HasPrefix(p, d+`\`)
}

// Diff — итог сверки сохранённых записей с найденными (RM5b-4).
type Diff struct{ New, Changed, Gone []Entry }

type key struct{ scope, source, name string }

func keyOf(e Entry) key { return key{e.Scope, e.Source, e.Name} }

// Compare сверяет сохранённые записи с найденными по ключу Scope+Source+Name: New — найденные
// без пары (в порядке found), Changed — найденные, у которых сменилась команда или издатель,
// Gone — сохранённые без пары (в порядке stored). Включённость и Exe изменением не считаются.
// Пустые части — [], не nil.
func Compare(stored, found []Entry) Diff {
	d := Diff{New: []Entry{}, Changed: []Entry{}, Gone: []Entry{}}
	was := make(map[key]Entry, len(stored))
	for _, e := range stored {
		was[keyOf(e)] = e
	}
	now := make(map[key]bool, len(found))
	for _, e := range found {
		k := keyOf(e)
		now[k] = true
		old, ok := was[k]
		switch {
		case !ok:
			d.New = append(d.New, e)
		case old.Command != e.Command || old.Publisher != e.Publisher:
			d.Changed = append(d.Changed, e)
		}
	}
	for _, e := range stored {
		if !now[keyOf(e)] {
			d.Gone = append(d.Gone, e)
		}
	}
	return d
}
