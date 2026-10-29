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
//   - известные игры и браузеры по каталогу до scanCatalogDepth уровней (не больше maxFiles
//     прочитанных файлов).
//
// Папки Windows* в Program Files (Windows Defender, Windows Media Player, Windows NT) не
// пропускаются: класс «система» их не покрывает (programs.Roots.IsSystem), и без записи в снимке
// их программы закрывались бы как незнакомые. Пропускаются WindowsApps (у неё свой разбор) и
// каталог установки PC Block. Ссылки и junction не обходятся (scanFS их не отдаёт как папки).
//
// Сканирование ограничено по времени: первый этап (корни приложений и пакеты Store) обязан
// закончиться, иначе снимок неполный и пропущенные программы закрывались бы как незнакомые, —
// тогда ошибка, служба повторит снимок позже и до того ничего не закрывает (R18). Второй этап
// (каталог на третьем уровне) по таймауту просто останавливается. Зависшее чтение папки
// (сетевая папка) не держит Scan дольше timeout+grace.

// scanEntry — запись папки: имя, папка ли, размер файла.
type scanEntry struct {
	name string
	dir  bool
	size int64
}

// scanFS — чтение папок и файлов для сканера. Пути — пути Windows с «\».
type scanFS interface {
	// ReadDir возвращает обычные файлы и папки (ссылки и junction не возвращаются).
	ReadDir(dir string) ([]scanEntry, error)
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
	scanTimeout      = 10 * time.Second // общий таймаут сканирования
	scanGrace        = 2 * time.Second  // сверх таймаута на зависшее чтение папки
	scanMaxEntries   = 400              // записей в снимке
	scanMaxFiles     = 5000             // прочитанных файлов, после которых поиск по каталогу останавливается
	scanMainDepth    = 2                // уровней вглубь корня приложения для главного exe
	scanCatalogDepth = 3                // уровней вглубь корня приложения для каталога игр
	manifestMaxBytes = 1 << 20          // AppxManifest.xml длиннее — не читается
	storeManifest    = "AppxManifest.xml"
)

// scanContainerDirs — общие папки Program Files, в которых корень приложения — следующая папка
// (совпадает с programs: Roots.AppRoot, тест сверяет).
var scanContainerDirs = map[string]bool{"microsoft": true, "common files": true}

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
	timeout      time.Duration
	grace        time.Duration
	maxEntries   int
	maxFiles     int
}

// scanState — то, что обход успел найти; общий для обхода и Scan (обход может пережить Scan,
// если чтение папки зависло).
type scanState struct {
	mu      sync.Mutex
	mains   []string // главные exe корней приложений
	store   []string // exe пакетов Store
	extras  []string // находки по каталогу
	deep    []string // папки третьего уровня для каталога
	files   int      // прочитано файлов
	rootsOK int      // прочитано корней (Program Files, WindowsApps)
	phase1  bool     // первый этап закончен
	err     error
}

// Scan обходит корни и возвращает снимок, отсортированный по пути; см. описание сканера.
func (s *programScanner) Scan() ([]service.FoundProgram, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	st := &scanState{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if p := recover(); p != nil {
				st.mu.Lock()
				st.err = fmt.Errorf("сбой сканера программ: %v", p)
				st.mu.Unlock()
			}
		}()
		s.walk(ctx, st)
	}()
	timer := time.NewTimer(s.timeout + s.grace)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
	paths, err := st.result(s.maxEntries, s.timeout)
	if err != nil {
		return nil, err
	}
	return s.describe(paths), nil
}

// result собирает снимок: главные exe, затем пакеты Store, затем каталог; без повторов, не
// больше max записей, по порядку нормализованного пути.
func (st *scanState) result(max int, timeout time.Duration) ([]string, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	switch {
	case st.err != nil:
		return nil, st.err
	case !st.phase1:
		return nil, fmt.Errorf("сканирование программ не уложилось в %d с", int(timeout.Seconds()))
	case st.rootsOK == 0:
		return nil, fmt.Errorf("не прочитана ни одна папка программ (Program Files)")
	}
	type item struct{ path, norm string }
	items := []item{}
	seen := map[string]bool{}
	for _, group := range [][]string{st.mains, st.store, st.extras} {
		for _, p := range group {
			n := programs.Normalize(p)
			if n == "" || seen[n] || len(items) >= max {
				continue
			}
			seen[n] = true
			items = append(items, item{p, n})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].norm < items[j].norm })
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.path)
	}
	return out, nil
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

// read читает папку, если время не вышло, и считает прочитанные файлы.
func (s *programScanner) read(ctx context.Context, st *scanState, dir string) ([]scanEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ents, err := s.fs.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	n := 0
	for _, e := range ents {
		if !e.dir {
			n++
		}
	}
	st.mu.Lock()
	st.files += n
	st.mu.Unlock()
	return ents, nil
}

// skipDir: каталог установки PC Block не сканируется (его программы — система).
func (s *programScanner) skipDir(dir string) bool {
	return s.installDir != "" && programs.Normalize(dir) == s.installDir
}

// walk — оба этапа обхода.
func (s *programScanner) walk(ctx context.Context, st *scanState) {
	for _, pf := range s.programFiles {
		if pf == "" {
			continue
		}
		ents, err := s.read(ctx, st, pf)
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
				s.app(ctx, st, dir, e.name)
				continue
			}
			sub, err := s.read(ctx, st, dir)
			if err != nil {
				continue
			}
			for _, se := range sub {
				if p := dir + `\` + se.name; se.dir && !s.skipDir(p) {
					s.app(ctx, st, p, se.name)
				}
			}
		}
	}
	if s.store != "" {
		s.walkStore(ctx, st)
	}
	if ctx.Err() != nil {
		return // первый этап не закончен: снимок неполный
	}
	st.mu.Lock()
	st.phase1 = true
	deep := st.deep
	st.mu.Unlock()

	for _, dir := range deep {
		st.mu.Lock()
		files := st.files
		st.mu.Unlock()
		if ctx.Err() != nil || files >= s.maxFiles {
			return
		}
		ents, err := s.read(ctx, st, dir)
		if err != nil {
			continue
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
	}
}

// app обходит корень приложения dir (имя папки name) до scanMainDepth уровней: главный exe,
// находки по каталогу и папки следующего уровня для второго этапа.
func (s *programScanner) app(ctx context.Context, st *scanState, dir, name string) {
	var levels [scanMainDepth + 1][]scanFile
	var extras, deep []string
	cur := []string{dir}
	for d := 0; d <= scanMainDepth && len(cur) > 0; d++ {
		var next []string
		for _, folder := range cur {
			ents, err := s.read(ctx, st, folder)
			if err != nil {
				continue
			}
			for _, e := range ents {
				p := folder + `\` + e.name
				if e.dir {
					switch {
					case s.skipDir(p):
					case d < scanMainDepth:
						next = append(next, p)
					case d+1 <= scanCatalogDepth:
						deep = append(deep, p)
					}
					continue
				}
				if !isExe(e.name) {
					continue
				}
				levels[d] = append(levels[d], scanFile{path: p, name: e.name, size: e.size})
				if _, ok := s.cat.Lookup(e.name); ok {
					extras = append(extras, p)
				}
			}
		}
		cur = next
	}
	main, ok := pickMainExe(name, levels)
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
	ents, err := s.read(ctx, st, s.store)
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
