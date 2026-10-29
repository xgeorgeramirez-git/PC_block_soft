package winhost

import (
	"fmt"
	"sort"
	"strings"

	"pcblock/internal/programs"
	"pcblock/internal/service"
)

// Сухой прогон наблюдателя (RM3-8): диагностика для приёмки на windows-latest, не боевой путь.
// Процессы сеанса раннера прогоняются через классификатор с настоящими корнями и снимком; ничего
// не завершается. Итог — одна строка пометки ::notice для журнала сборки.

// dryRunMaxPaths — сколько путей незнакомых программ попадает в пометку.
const dryRunMaxPaths = 20

// dryRunReport — итог сухого прогона.
type dryRunReport struct {
	Total, Unreadable, System, Allowed, Forbidden, Unknown int
	// UnknownPaths — незнакомые без фильтра WatchDir (нормализованные, по порядку, без повторов):
	// это закрыл бы наблюдатель в обычном режиме.
	UnknownPaths []string
	// Misjudged — процессы из папки Windows, Program Files и каталога установки, которые
	// получили «незнакомая» или «запрещена» после снимка: на компьютере ребёнка они закрывались бы.
	Misjudged []string
	// Closable — что наблюдатель в тестовом режиме считает закрываемым: незнакомое и запрещённое
	// внутри WatchDir (вне WatchDir — «не трогать», как в service.watchDecide).
	Closable []string
	// Exposed — закрываемое, которое лежит в защищённом каталоге (система, Program Files,
	// кэш инструментов и папка раннера): WatchDir указывает не туда.
	Exposed []string
}

// snapshotEntries превращает находки сканера в список классификатора так же, как снимок службы
// (service.snapshotPrograms): только доверенные каталоги вне класса «система», категория по
// каталогу или «Прочее».
func snapshotEntries(c *programs.Classifier, found []service.FoundProgram) []programs.Entry {
	roots := c.Roots()
	out := make([]programs.Entry, 0, len(found))
	for _, f := range found {
		p := programs.Normalize(f.Path)
		if p == "" || !roots.IsTrusted(p) || roots.IsSystem(p) {
			continue
		}
		cat := programs.CatOther
		if cc, ok := c.CatalogCategory(p); ok {
			cat = cc
		}
		out = append(out, programs.Entry{Path: p, Name: f.Name, Category: cat})
	}
	return out
}

// inside: нормализованный путь p внутри нормализованной папки dir (по границе разделителя).
func inside(p, dir string) bool {
	dir = strings.TrimSuffix(dir, `\`)
	return dir != "" && strings.HasPrefix(p, dir+`\`)
}

// dryRun классифицирует процессы list (watchDir — нормализованная папка тестовых программ,
// "" — нет; safe — защищённые каталоги сверх корней классификатора).
func dryRun(list []service.ProcInfo, c *programs.Classifier, watchDir string, safe []string) dryRunReport {
	roots := c.Roots()
	trusted := []string{roots.SystemRoot, roots.ProgramFiles, roots.ProgramFilesX86, roots.InstallDir}
	protected := append([]string{}, trusted...)
	for _, s := range safe {
		if n := programs.Normalize(s); n != "" {
			protected = append(protected, n)
		}
	}
	under := func(p string, dirs []string) bool {
		for _, d := range dirs {
			if inside(p, d) {
				return true
			}
		}
		return false
	}
	var r dryRunReport
	unknown := map[string]bool{}
	misjudged := map[string]bool{}
	closable := map[string]bool{}
	exposed := map[string]bool{}
	for _, p := range list {
		r.Total++
		norm := programs.Normalize(p.Path)
		if !p.PathOK || norm == "" {
			r.Unreadable++
			continue
		}
		res := c.Classify(norm, "")
		switch res.Verdict {
		case programs.VerdictSystem:
			r.System++
		case programs.VerdictAllow:
			r.Allowed++
		case programs.VerdictForbidden:
			r.Forbidden++
		case programs.VerdictUnknown:
			r.Unknown++
			unknown[norm] = true
		}
		if res.Verdict != programs.VerdictUnknown && res.Verdict != programs.VerdictForbidden {
			continue
		}
		if under(norm, trusted) {
			misjudged[norm] = true
		}
		if inside(norm, watchDir) {
			closable[norm] = true
			if under(norm, protected) {
				exposed[norm] = true
			}
		}
	}
	r.UnknownPaths = sortedKeys(unknown)
	r.Misjudged = sortedKeys(misjudged)
	r.Closable = sortedKeys(closable)
	r.Exposed = sortedKeys(exposed)
	return r
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// notice — итог одной строкой пометки GitHub Actions (::notice).
func (r dryRunReport) notice() string {
	var b strings.Builder
	fmt.Fprintf(&b, "::notice title=Сухой прогон наблюдателя::процессов %d, нечитаемых %d, система %d, разрешено %d, запрещено %d, незнакомых %d (без фильтра WatchDir); закрываемых %d (только внутри WatchDir); ошибок классификации в доверенных каталогах %d; защищённых внутри WatchDir %d.",
		r.Total, r.Unreadable, r.System, r.Allowed, r.Forbidden, r.Unknown, len(r.Closable), len(r.Misjudged), len(r.Exposed))
	paths := r.UnknownPaths
	if len(paths) > dryRunMaxPaths {
		paths = paths[:dryRunMaxPaths]
	}
	if len(paths) > 0 {
		fmt.Fprintf(&b, " Незнакомые (первые %d): %s", len(paths), strings.Join(paths, "; "))
	}
	if len(r.Misjudged) > 0 {
		fmt.Fprintf(&b, " Ошибки классификации: %s", strings.Join(r.Misjudged, "; "))
	}
	return strings.NewReplacer("\r", " ", "\n", " ", "%", "%25").Replace(b.String())
}
