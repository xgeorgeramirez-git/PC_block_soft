package winhost

import (
	"context"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"pcblock/internal/programs"
	"pcblock/internal/service"
)

// Сканер программ компьютера (M3: RM3-1, RM3-10, FR-2, дополнения T4b): снимок для первого
// старта службы и шага мастера «Программы». Обход папок идёт через scanFS, поэтому логика
// проверяется тестами на любой системе; на Windows scanFS — это os.ReadDir (scan_windows.go).
//
// Что находится:
//   - корни приложений Program Files и Program Files (x86): каждая первая подпапка, в общих папках
//     Microsoft и Common Files — следующая (как programs.Roots.AppRoot). В каждой — один «главный»
//     exe (pickMainExe), не глубже scanMainDepth уровней: его запись в списке даёт категорию всему
//     корню, и вспомогательные exe и новые версии программы не становятся «незнакомыми»;
//   - пакеты Microsoft Store в %ProgramFiles%\WindowsApps: exe из AppxManifest.xml, без
//     фреймворков и ресурсных пакетов;
//   - известные игры и браузеры по каталогу до scanCatalogDepth уровней (третий уровень — только
//     имена, не больше maxFiles файлов).
//
// Папки Windows* в Program Files (Windows Defender, Windows Media Player, Windows NT) не
// пропускаются: класс «система» их не покрывает (programs.Roots.IsSystem), и без записи в снимке
// их программы закрывались бы как незнакомые. Пропускаются WindowsApps (у неё свой разбор) и
// каталог установки PC Block. Ссылки и junction не обходятся (scanFS их не отдаёт как папки).
// В заведомо большие служебные папки внутри корня (isBulkyDir: node_modules, кэши, ресурсы)
// сканер не спускается: читается только их список, и exe оттуда — запасной главный, если в
// корне другого нет.
//
// Корни приложений обходятся пулом из workers горутин; результат сортируется, поэтому не зависит
// от порядка. Бюджет: первый этап (корни приложений и пакеты Store) — phase1; он обязан
// закончиться, иначе снимок неполный и пропущенные программы закрывались бы как незнакомые, —
// тогда ошибка со статистикой, служба повторит снимок позже и до того ничего не закрывает (R18).
// Второй этап (каталог на третьем уровне) идёт в оставшееся до timeout время и по нему просто
// останавливается. Зависшее чтение папки (сетевая папка) не держит Scan дольше бюджета + grace.

// scanEntry — запись папки: имя, папка ли, размер файла.
type scanEntry struct {
	name string
	dir  bool
	size int64
}

// scanFS — чтение папок и файлов для сканера. Пути — пути Windows с «\».
type scanFS interface {
	// ReadDir возвращает обычные файлы и папки (ссылки и junction не возвращаются); sizes —
	// нужны ли размеры exe (без них size = 0).
	ReadDir(dir string, sizes bool) ([]scanEntry, error)
	// ReadFile читает файл не длиннее max байт; длиннее — ошибка.
	ReadFile(path string, max int64) ([]byte, error)
}

// scanFile — найденный exe.
type scanFile struct {
	path string
	name string
	size int64
}

const (
	scanPhase1Timeout = 45 * time.Second // бюджет первого этапа: корни приложений и пакеты Store
	scanTimeout       = 60 * time.Second // бюджет всего сканирования (второй этап — остаток)
	scanGrace         = 2 * time.Second  // сверх бюджета на зависшее чтение папки
	scanWorkers       = 8                // горутин обхода корней приложений
	scanMaxEntries    = 400              // записей в снимке
	scanMaxFiles      = 5000             // файлов второго этапа, после которых поиск по каталогу останавливается
	scanMainDepth     = 2                // уровней вглубь корня приложения для главного exe
	scanCatalogDepth  = 3                // уровней вглубь корня приложения для каталога игр
	manifestMaxBytes  = 1 << 20          // AppxManifest.xml длиннее — не читается
	storeManifest     = "AppxManifest.xml"
)

// scanContainerDirs — общие папки Program Files, в которых корень приложения — следующая папка
// (совпадает с programs: Roots.AppRoot, тест сверяет).
var scanContainerDirs = map[string]bool{"microsoft": true, "common files": true}

// bulkyDirs — служебные папки внутри корня приложения (нижний регистр), в которые сканер не
// спускается: зависимости, исходники, кэши, локализации, справка, библиотеки.
var bulkyDirs = map[string]bool{
	"node_modules": true, ".git": true, ".svn": true, "packages": true, "installer": true,
	"temp": true, "tmp": true, "logs": true, "locales": true, "locale": true, "resources": true,
	"__pycache__": true, "site-packages": true, "share": true, "lib": true, "lib64": true,
	"include": true, "doc": true, "docs": true, "help": true, "licenses": true, "samples": true,
	"examples": true, "symbols": true,
}

// isBulkyDir: папка внутри корня приложения, в которую сканер не спускается (bulkyDirs и всё
// с «cache» в начале или в конце имени: Cache, CacheStorage, Code Cache, GPUCache).
func isBulkyDir(name string) bool {
	n := strings.ToLower(name)
	return bulkyDirs[n] || strings.HasPrefix(n, "cache") || strings.HasSuffix(n, "cache")
}

// storeFrameworkPrefixes — имена пакетов Store (нижний регистр), которые не приложения:
// библиотеки, которые ставит сама Windows.
var storeFrameworkPrefixes = []string{"microsoft.vclibs", "microsoft.net.", "microsoft.ui.xaml"}

// programScanner — service.Scanner над scanFS.
type programScanner struct {
	programFiles []string // папки Program Files как есть («C:\Program Files»); пустые пропускаются
	store        string   // папка пакетов Store («C:\Program Files\WindowsApps»); "" — не сканировать
	installDir   string   // каталог установки PC Block (programs.Normalize): не сканируется
	cat          programs.Catalog
	fs           scanFS
	meta         service.FileMeta // свойства файла: издатель и описание; nil — без них
	phase1       time.Duration    // бюджет первого этапа
	timeout      time.Duration    // бюджет всего сканирования
	grace        time.Duration
	workers      int
	maxEntries   int
	maxFiles     int
}

// newProgramScanner — сканер с бюджетами и пределами программы.
func newProgramScanner(programFiles []string, store, installDir string, meta service.FileMeta, fsys scanFS) *programScanner {
	return &programScanner{
		programFiles: programFiles,
		store:        store,
		installDir:   installDir,
		cat:          programs.DefaultCatalog,
		fs:           fsys,
		meta:         meta,
		phase1:       scanPhase1Timeout,
		timeout:      scanTimeout,
		grace:        scanGrace,
		workers:      scanWorkers,
		maxEntries:   scanMaxEntries,
		maxFiles:     scanMaxFiles,
	}
}

// scanStats — диагностика сканирования: где уходит время (журнал работы, сухой прогон).
type scanStats struct {
	Phase1, Phase2         time.Duration
	Phase1Done, Phase2Done bool
	AppRoots, StorePkgs    int // корней приложений к обходу, пакетов Store с приложениями
	Dirs, Files            int // прочитано папок и файлов в них
	Found                  int // записей в снимке
	SlowestDir             string
	Slowest                time.Duration
}

// String — статистика одной строкой.
func (st scanStats) String() string {
	state := func(done bool) string {
		if done {
			return "закончен"
		}
		return "не закончен"
	}
	return fmt.Sprintf("этап 1 %s за %.1f с, этап 2 %s за %.1f с; корней приложений %d, пакетов Store %d, папок %d, файлов %d, найдено %d; самая долгая папка %s за %d мс",
		state(st.Phase1Done), st.Phase1.Seconds(), state(st.Phase2Done), st.Phase2.Seconds(),
		st.AppRoots, st.StorePkgs, st.Dirs, st.Files, st.Found, st.SlowestDir, st.Slowest.Milliseconds())
}

// scanState — то, что обход успел найти, и статистика; общий для горутин обхода и Scan (обход
// может пережить Scan, если чтение папки зависло).
type scanState struct {
	mu        sync.Mutex
	start     time.Time
	mains     []string // главные exe корней приложений
	store     []string // exe пакетов Store
	extras    []string // находки по каталогу
	deep      []string // папки третьего уровня для каталога
	files2    int      // прочитано файлов на втором этапе
	rootsOK   int      // прочитано корней (Program Files, WindowsApps)
	phase1End time.Time
	phase2End time.Time
	phase2Ok  bool
	stats     scanStats
	err       error
}

// guard выполняет f; паника — ошибка сканирования, а не падение службы.
func (st *scanState) guard(f func()) {
	defer func() {
		if p := recover(); p != nil {
			st.mu.Lock()
			if st.err == nil {
				st.err = fmt.Errorf("сбой сканера программ: %v", p)
			}
			st.mu.Unlock()
		}
	}()
	f()
}

func (st *scanState) phase1Done() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return !st.phase1End.IsZero()
}

// Scan обходит корни и возвращает снимок, отсортированный по пути; ошибка содержит статистику.
func (s *programScanner) Scan() ([]service.FoundProgram, error) {
	found, _, err := s.scan()
	return found, err
}

// scan — Scan со статистикой (её выводит сухой прогон и при ошибке).
func (s *programScanner) scan() ([]service.FoundProgram, scanStats, error) {
	all, cancelAll := context.WithTimeout(context.Background(), s.timeout)
	defer cancelAll()
	p1, cancel1 := context.WithTimeout(all, s.phase1)
	defer cancel1()
	st := &scanState{start: time.Now()}
	done := make(chan struct{})
	go func() {
		defer close(done)
		st.guard(func() { s.walk(p1, all, st) })
	}()
	t1 := time.NewTimer(s.phase1 + s.grace)
	defer t1.Stop()
	t2 := time.NewTimer(s.timeout + s.grace)
	defer t2.Stop()
wait:
	for {
		select {
		case <-done:
			break wait
		case <-t1.C:
			if !st.phase1Done() {
				break wait // первый этап завис: снимок всё равно неполный
			}
		case <-t2.C:
			break wait
		}
	}
	paths, stats, err := st.result(s.maxEntries, s.phase1)
	if err != nil {
		return nil, stats, fmt.Errorf("%w (%s)", err, stats)
	}
	return s.describe(paths), stats, nil
}

// result собирает снимок: главные exe, затем пакеты Store, затем каталог (каждая группа по
// порядку пути); без повторов, не больше max записей, по порядку нормализованного пути.
func (st *scanState) result(max int, phase1 time.Duration) ([]string, scanStats, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	stats := st.stats
	now := time.Now()
	switch {
	case !st.phase1End.IsZero():
		stats.Phase1, stats.Phase1Done = st.phase1End.Sub(st.start), true
		if st.phase2End.IsZero() {
			stats.Phase2 = now.Sub(st.phase1End)
		} else {
			stats.Phase2, stats.Phase2Done = st.phase2End.Sub(st.phase1End), st.phase2Ok
		}
	default:
		stats.Phase1 = now.Sub(st.start)
	}
	switch {
	case st.err != nil:
		return nil, stats, st.err
	case !stats.Phase1Done:
		return nil, stats, fmt.Errorf("сканирование программ не уложилось в %d с", int(phase1.Seconds()))
	case st.rootsOK == 0:
		return nil, stats, fmt.Errorf("не прочитана ни одна папка программ (Program Files)")
	}
	type item struct{ path, norm string }
	items := []item{}
	seen := map[string]bool{}
	for _, group := range [][]string{st.mains, st.store, st.extras} {
		g := make([]item, 0, len(group))
		for _, p := range group {
			g = append(g, item{p, programs.Normalize(p)})
		}
		sort.Slice(g, func(i, j int) bool { return g[i].norm < g[j].norm })
		for _, it := range g {
			if it.norm == "" || seen[it.norm] || len(items) >= max {
				continue
			}
			seen[it.norm] = true
			items = append(items, it)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].norm < items[j].norm })
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.path)
	}
	stats.Found = len(out)
	return out, stats, nil
}

// describe добавляет к путям имя (FileDescription или имя файла) и издателя (CompanyName).
func (s *programScanner) describe(paths []string) []service.FoundProgram {
	out := make([]service.FoundProgram, 0, len(paths))
	for _, p := range paths {
		var pub, descr string
		if s.meta != nil {
			pub, descr, _ = s.meta(p)
		}
		name := programs.ClipName(descr)
		if name == "" {
			name = programs.NameFromPath(p)
		}
		out = append(out, service.FoundProgram{Path: p, Name: name, Publisher: programs.ClipName(pub)})
	}
	return out
}

// read читает папку, если время не вышло, и ведёт статистику; phase2 — файлы идут в предел
// второго этапа.
func (s *programScanner) read(ctx context.Context, st *scanState, dir string, sizes, phase2 bool) ([]scanEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t := time.Now()
	ents, err := s.fs.ReadDir(dir, sizes)
	d := time.Since(t)
	n := 0
	for _, e := range ents {
		if !e.dir {
			n++
		}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.stats.Dirs++
	st.stats.Files += n
	if phase2 {
		st.files2 += n
	}
	if d > st.stats.Slowest {
		st.stats.Slowest, st.stats.SlowestDir = d, dir
	}
	if err != nil {
		return nil, err
	}
	return ents, nil
}

// skipDir: каталог установки PC Block не сканируется (его программы — система).
func (s *programScanner) skipDir(dir string) bool {
	return s.installDir != "" && programs.Normalize(dir) == s.installDir
}

// appDir — корень приложения к обходу.
type appDir struct{ dir, name string }

// appDirs читает Program Files и общие папки и возвращает корни приложений.
func (s *programScanner) appDirs(ctx context.Context, st *scanState) []appDir {
	var apps []appDir
	for _, pf := range s.programFiles {
		if pf == "" {
			continue
		}
		ents, err := s.read(ctx, st, pf, false, false)
		if err != nil {
			continue
		}
		st.mu.Lock()
		st.rootsOK++
		st.mu.Unlock()
		for _, e := range ents {
			low := strings.ToLower(e.name)
			dir := pf + `\` + e.name
			if !e.dir || low == "windowsapps" || s.skipDir(dir) {
				continue
			}
			if !scanContainerDirs[low] {
				apps = append(apps, appDir{dir, e.name})
				continue
			}
			sub, err := s.read(ctx, st, dir, false, false)
			if err != nil {
				continue
			}
			for _, se := range sub {
				if p := dir + `\` + se.name; se.dir && !s.skipDir(p) {
					apps = append(apps, appDir{p, se.name})
				}
			}
		}
	}
	return apps
}

// pool выполняет f для каждого элемента items в n горутинах, пока ctx не отменён.
func pool[T any](ctx context.Context, st *scanState, n int, items []T, f func(T)) {
	jobs := make(chan T)
	var wg sync.WaitGroup
	for range max(1, n) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st.guard(func() {
				for it := range jobs {
					f(it)
				}
			})
		}()
	}
feed:
	for _, it := range items {
		select {
		case jobs <- it:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
}

// walk — оба этапа обхода: p1 — бюджет первого этапа, all — всего сканирования.
func (s *programScanner) walk(p1, all context.Context, st *scanState) {
	apps := s.appDirs(p1, st)
	st.mu.Lock()
	st.stats.AppRoots = len(apps)
	st.mu.Unlock()
	var storeDone sync.WaitGroup
	if s.store != "" {
		storeDone.Add(1)
		go func() {
			defer storeDone.Done()
			st.guard(func() { s.walkStore(p1, st) })
		}()
	}
	pool(p1, st, s.workers, apps, func(a appDir) { s.app(p1, st, a.dir, a.name) })
	storeDone.Wait()
	if p1.Err() != nil {
		return // первый этап не закончен: снимок неполный
	}
	st.mu.Lock()
	st.phase1End = time.Now()
	deep := append([]string(nil), st.deep...)
	st.mu.Unlock()
	sort.Strings(deep)

	var mu sync.Mutex
	processed := 0
	pool(all, st, s.workers, deep, func(dir string) {
		st.mu.Lock()
		over := st.files2 >= s.maxFiles
		st.mu.Unlock()
		if over || all.Err() != nil {
			return
		}
		ents, err := s.read(all, st, dir, false, true)
		mu.Lock()
		processed++
		mu.Unlock()
		if err != nil {
			return
		}
		var found []string
		for _, e := range ents {
			if !e.dir && isExe(e.name) {
				if _, ok := s.cat.Lookup(e.name); ok {
					found = append(found, dir+`\`+e.name)
				}
			}
		}
		st.mu.Lock()
		st.extras = append(st.extras, found...)
		st.mu.Unlock()
	})
	mu.Lock()
	ok := processed == len(deep)
	mu.Unlock()
	st.mu.Lock()
	st.phase2End, st.phase2Ok = time.Now(), ok
	st.mu.Unlock()
}

// scanFolder — папка внутри корня приложения; bulky — служебная (не спускаться, exe — запасные).
type scanFolder struct {
	path  string
	bulky bool
}

// app обходит корень приложения dir (имя папки name) до scanMainDepth уровней: главный exe,
// находки по каталогу и папки третьего уровня для второго этапа.
func (s *programScanner) app(ctx context.Context, st *scanState, dir, name string) {
	var levels [scanMainDepth + 1][]scanFile
	var spare [scanMainDepth + 1][]scanFile // exe из служебных папок: запасные главные
	var extras, deep []string
	cur := []scanFolder{{path: dir}}
	for d := 0; d <= scanMainDepth && len(cur) > 0; d++ {
		var next []scanFolder
		for _, folder := range cur {
			ents, err := s.read(ctx, st, folder.path, true, false)
			if err != nil {
				continue
			}
			for _, e := range ents {
				p := folder.path + `\` + e.name
				if e.dir {
					switch {
					case folder.bulky, s.skipDir(p):
					case d < scanMainDepth:
						next = append(next, scanFolder{path: p, bulky: isBulkyDir(e.name)})
					case d+1 <= scanCatalogDepth && !isBulkyDir(e.name):
						deep = append(deep, p)
					}
					continue
				}
				if !isExe(e.name) {
					continue
				}
				f := scanFile{path: p, name: e.name, size: e.size}
				if folder.bulky {
					spare[0] = append(spare[0], f)
				} else {
					levels[d] = append(levels[d], f)
				}
				if _, ok := s.cat.Lookup(e.name); ok {
					extras = append(extras, p)
				}
			}
		}
		cur = next
	}
	main, ok := pickMainExe(name, levels)
	if !ok {
		main, ok = pickMainExe(name, spare)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if ok {
		st.mains = append(st.mains, main.path)
	}
	st.extras = append(st.extras, extras...)
	st.deep = append(st.deep, deep...)
}

// walkStore добавляет exe пакетов Microsoft Store из их AppxManifest.xml.
func (s *programScanner) walkStore(ctx context.Context, st *scanState) {
	ents, err := s.read(ctx, st, s.store, false, false)
	if err != nil {
		return
	}
	st.mu.Lock()
	st.rootsOK++
	st.mu.Unlock()
	for _, e := range ents {
		if !e.dir || skipStorePackage(e.name) {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		pkg := s.store + `\` + e.name
		data, err := s.fs.ReadFile(pkg+`\`+storeManifest, manifestMaxBytes)
		if err != nil {
			continue
		}
		exes := manifestExecutables(data)
		st.mu.Lock()
		for _, x := range exes {
			st.store = append(st.store, pkg+`\`+x)
		}
		if len(exes) > 0 {
			st.stats.StorePkgs++
		}
		st.mu.Unlock()
	}
}

// isExe: имя файла оканчивается на «.exe» (регистр не важен).
func isExe(name string) bool {
	return len(name) > 4 && strings.EqualFold(name[len(name)-4:], ".exe")
}

// isUninstaller: программа удаления («unins000.exe», «Uninstall.exe») — главной быть не должна,
// если в той же папке есть что-то ещё.
func isUninstaller(name string) bool {
	n := strings.ToLower(name)
	return strings.HasPrefix(n, "unins") || strings.Contains(n, "uninstall")
}

// squash — имя для сравнения с именем папки: нижний регистр, без пробелов и без «.exe».
func squash(name string) string {
	if isExe(name) {
		name = name[:len(name)-4]
	}
	return strings.ToLower(strings.ReplaceAll(name, " ", ""))
}

// pickMainExe выбирает главный exe корня приложения folder по уровням (0 — сам корень):
// exe с именем как у папки (без пробелов и регистра) на ближайшем уровне; иначе на ближайшем
// уровне с exe — единственный или самый большой (программы удаления — только если других нет;
// при равном размере — первый по пути). Нет exe — ok = false.
func pickMainExe(folder string, levels [scanMainDepth + 1][]scanFile) (scanFile, bool) {
	want := squash(folder)
	byPath := func(a, b scanFile) bool { return strings.ToLower(a.path) < strings.ToLower(b.path) }
	for _, lvl := range levels {
		var match *scanFile
		for i := range lvl {
			if squash(lvl[i].name) == want && (match == nil || byPath(lvl[i], *match)) {
				match = &lvl[i]
			}
		}
		if match != nil {
			return *match, true
		}
	}
	for _, lvl := range levels {
		if len(lvl) == 0 {
			continue
		}
		pool := make([]scanFile, 0, len(lvl))
		for _, f := range lvl {
			if !isUninstaller(f.name) {
				pool = append(pool, f)
			}
		}
		if len(pool) == 0 {
			pool = lvl
		}
		best := pool[0]
		for _, f := range pool[1:] {
			if f.size > best.size || (f.size == best.size && byPath(f, best)) {
				best = f
			}
		}
		return best, true
	}
	return scanFile{}, false
}

// skipStorePackage: папка WindowsApps — фреймворк, а не приложение (VCLibs, .NET Native, UI.Xaml,
// «*.Framework»). Имя пакета — часть имени папки до первого «_».
func skipStorePackage(dir string) bool {
	name := strings.ToLower(dir)
	if i := strings.IndexByte(name, '_'); i >= 0 {
		name = name[:i]
	}
	for _, p := range storeFrameworkPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return strings.HasSuffix(name, ".framework") || strings.Contains(name, ".framework.")
}

// appxManifest — нужная часть AppxManifest.xml (пространства имён не важны).
type appxManifest struct {
	Properties struct {
		Framework       bool `xml:"Framework"`
		ResourcePackage bool `xml:"ResourcePackage"`
	} `xml:"Properties"`
	Applications []struct {
		Executable string `xml:"Executable,attr"`
	} `xml:"Applications>Application"`
}

// manifestExecutables возвращает пути exe приложений пакета относительно его папки («bin\x.exe»).
// Фреймворк, ресурсный пакет и испорченный манифест — пусто. Путь, выходящий за папку пакета
// (абсолютный, с «..»), и шаблоны сборки («$targetnametoken$») отбрасываются.
func manifestExecutables(data []byte) []string {
	var m appxManifest
	if err := xml.Unmarshal(data, &m); err != nil || m.Properties.Framework || m.Properties.ResourcePackage {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	for _, a := range m.Applications {
		x := strings.TrimSpace(strings.ReplaceAll(a.Executable, "/", `\`))
		if x == "" || !isExe(x) || strings.ContainsAny(x, "$:") || strings.HasPrefix(x, `\`) {
			continue
		}
		bad := false
		for _, seg := range strings.Split(x, `\`) {
			if seg == ".." || seg == "." || seg == "" {
				bad = true
			}
		}
		if bad || seen[strings.ToLower(x)] {
			continue
		}
		seen[strings.ToLower(x)] = true
		out = append(out, x)
	}
	return out
}
