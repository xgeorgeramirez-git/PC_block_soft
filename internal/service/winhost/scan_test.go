package winhost

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"pcblock/internal/programs"
	"pcblock/internal/service"
)

// fakeFS — файловая система для сканера: файлы с размерами, папки выводятся из путей.
// Регистр путей сохраняется, поиск без учёта регистра (как в Windows).
type fakeFS struct {
	mu      sync.Mutex
	dirs    map[string]map[string]scanEntry // папка (нижний регистр) → имя (нижний регистр) → запись
	files   map[string][]byte               // содержимое файлов (манифесты), нижний регистр
	denied  map[string]bool                 // папки, которые не читаются
	hang    map[string]chan struct{}        // папки, чтение которых висит до закрытия канала
	readDir int                             // сколько раз читались папки
}

func newFakeFS(sizes map[string]int64) *fakeFS {
	f := &fakeFS{dirs: map[string]map[string]scanEntry{}, files: map[string][]byte{}, denied: map[string]bool{}, hang: map[string]chan struct{}{}}
	for p, size := range sizes {
		f.add(p, size)
	}
	return f
}

// add добавляет файл (size >= 0) или пустую папку (size < 0) со всеми папками над ним.
func (f *fakeFS) add(p string, size int64) {
	parts := strings.Split(p, `\`)
	for i := 1; i < len(parts); i++ {
		dir := strings.ToLower(strings.Join(parts[:i], `\`))
		if f.dirs[dir] == nil {
			f.dirs[dir] = map[string]scanEntry{}
		}
		name := parts[i]
		isDir := i < len(parts)-1 || size < 0
		e := scanEntry{name: name, dir: isDir}
		if !isDir {
			e.size = size
		}
		if old, ok := f.dirs[dir][strings.ToLower(name)]; !ok || !old.dir {
			f.dirs[dir][strings.ToLower(name)] = e
		}
	}
	if size < 0 {
		if d := strings.ToLower(p); f.dirs[d] == nil {
			f.dirs[d] = map[string]scanEntry{}
		}
	}
}

func (f *fakeFS) addFile(p string, data string) {
	f.add(p, int64(len(data)))
	f.files[strings.ToLower(p)] = []byte(data)
}

func (f *fakeFS) ReadDir(dir string) ([]scanEntry, error) {
	key := strings.ToLower(dir)
	f.mu.Lock()
	f.readDir++
	ch := f.hang[key]
	denied := f.denied[key]
	ents, ok := f.dirs[key]
	f.mu.Unlock()
	if ch != nil {
		<-ch
	}
	if denied {
		return nil, errors.New("отказано в доступе")
	}
	if !ok {
		return nil, errors.New("нет такой папки")
	}
	out := make([]scanEntry, 0, len(ents))
	for _, e := range ents {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

func (f *fakeFS) ReadFile(p string, max int64) ([]byte, error) {
	data, ok := f.files[strings.ToLower(p)]
	if !ok {
		return nil, errors.New("нет такого файла")
	}
	if int64(len(data)) > max {
		return nil, errors.New("файл слишком большой")
	}
	return data, nil
}

const (
	tPF    = `C:\Program Files`
	tPF86  = `C:\Program Files (x86)`
	tStore = `C:\Program Files\WindowsApps`
)

func testScanner(fsys scanFS) *programScanner {
	return &programScanner{
		programFiles: []string{tPF, tPF86},
		store:        tStore,
		installDir:   programs.Normalize(tPF + `\PCBlock`),
		cat:          programs.DefaultCatalog,
		fs:           fsys,
		meta: func(p string) (string, string, string) {
			if strings.HasSuffix(strings.ToLower(p), `brave.exe`) {
				return "Brave Software Inc", "Brave Browser", "1:1"
			}
			return "", "", ""
		},
		timeout:    time.Second,
		grace:      200 * time.Millisecond,
		maxEntries: 400,
		maxFiles:   5000,
	}
}

func foundPaths(list []service.FoundProgram) []string {
	out := make([]string, 0, len(list))
	for _, f := range list {
		out = append(out, f.Path)
	}
	return out
}

// Ловит: главный exe корня приложения выбран не по правилу (имя как у папки → единственный в
// корне → самый большой в корне → то же глубже, не глубже 2 уровней): без записи корня каждый
// вспомогательный exe этой программы стал бы «незнакомым» и закрывался бы (RM3-1 п.3, RM3-10).
func TestScanPicksMainExePerAppRoot(t *testing.T) {
	fsys := newFakeFS(map[string]int64{
		tPF + `\Notepad++\notepad++.exe`:                                4000,
		tPF + `\Notepad++\updater\GUP.exe`:                              9000,
		tPF + `\7-Zip\7zFM.exe`:                                         1000,
		tPF + `\7-Zip\7zG.exe`:                                          2000,
		tPF + `\7-Zip\Uninstall.exe`:                                    9000,
		tPF + `\Git\git-bash.exe`:                                       100,
		tPF + `\Git\git-cmd.exe`:                                        200,
		tPF + `\Git\usr\bin\bash.exe`:                                   5000,
		tPF + `\Single App\tool.exe`:                                    10,
		tPF + `\Brave Software\Brave-Browser\Application\brave.exe`:     300,
		tPF + `\Brave Software\Brave-Browser\Application\chrome_pr.exe`: 20,
		tPF + `\Deep\a\b\c\deep.exe`:                                    50, // глубже 2 уровней: главного нет
		tPF + `\Empty Folder\readme.txt`:                                5,
		tPF + `\Office\root\Office16\WINWORD.EXE`:                       700,
		tPF86 + `\Steam\steam.exe`:                                      100,
		tPF86 + `\Steam\steamerrorreporter.exe`:                         500,
	})
	got, err := testScanner(fsys).Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	want := []string{
		tPF86 + `\Steam\steam.exe`,
		tPF + `\7-Zip\7zG.exe`,
		tPF + `\Brave Software\Brave-Browser\Application\brave.exe`,
		tPF + `\Git\git-cmd.exe`,
		tPF + `\Notepad++\notepad++.exe`,
		tPF + `\Office\root\Office16\WINWORD.EXE`,
		tPF + `\Single App\tool.exe`,
	}
	if g := foundPaths(got); !slices.Equal(g, want) {
		t.Fatalf("найдено:\n%s\nждали:\n%s", strings.Join(g, "\n"), strings.Join(want, "\n"))
	}
	for _, f := range got {
		if strings.HasSuffix(f.Path, `brave.exe`) && (f.Name != "Brave Browser" || f.Publisher != "Brave Software Inc") {
			t.Errorf("brave: имя %q, издатель %q; ждали описание и CompanyName из свойств файла", f.Name, f.Publisher)
		}
		if strings.HasSuffix(f.Path, `tool.exe`) && (f.Name != "tool" || f.Publisher != "") {
			t.Errorf("tool: имя %q, издатель %q; ждали имя файла и пустого издателя", f.Name, f.Publisher)
		}
	}
}

// Ловит: в общих папках Microsoft и Common Files вся папка стала бы одним приложением (или была
// бы пропущена): корень приложения там — следующая папка (programs.Roots.AppRoot), и без её
// главного exe экранная клавиатура или обновление Edge закрывались бы как «незнакомые».
func TestScanDescendsIntoContainerDirs(t *testing.T) {
	fsys := newFakeFS(map[string]int64{
		tPF + `\Common Files\microsoft shared\ink\TabTip.exe`:   100,
		tPF + `\Common Files\Other Shared\helper.exe`:           100,
		tPF86 + `\Microsoft\EdgeUpdate\MicrosoftEdgeUpdate.exe`: 100,
		tPF86 + `\Microsoft\Edge\Application\msedge.exe`:        100,
		tPF86 + `\Microsoft\Edge\Application\setup.exe`:         10,
	})
	got, err := testScanner(fsys).Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	want := []string{
		tPF86 + `\Microsoft\Edge\Application\msedge.exe`,
		tPF86 + `\Microsoft\EdgeUpdate\MicrosoftEdgeUpdate.exe`,
		tPF + `\Common Files\microsoft shared\ink\TabTip.exe`,
		tPF + `\Common Files\Other Shared\helper.exe`,
	}
	if g := foundPaths(got); !slices.Equal(g, want) {
		t.Fatalf("найдено:\n%s\nждали:\n%s", strings.Join(g, "\n"), strings.Join(want, "\n"))
	}
	// Корни приложений у найденного те же, что считает классификатор.
	roots := programs.Roots{ProgramFiles: programs.Normalize(tPF), ProgramFilesX86: programs.Normalize(tPF86)}
	for _, f := range got {
		if roots.AppRoot(f.Path) == "" {
			t.Errorf("%s: у найденного нет корня приложения", f.Path)
		}
	}
	for name := range scanContainerDirs {
		p := tPF + `\` + name + `\app\x.exe`
		if got, want := roots.AppRoot(p), programs.Normalize(tPF+`\`+name+`\app`); got != want {
			t.Errorf("общая папка %q: AppRoot = %q, сканер считает корнем %q", name, got, want)
		}
	}
}

// Ловит: сканер заходит в WindowsApps обычным обходом (там тысячи файлов пакетов и нет доступа)
// и в каталог установки PC Block; известная игра глубже главного exe не находится.
func TestScanSkipsStoreAndInstallDirAndFindsCatalogGames(t *testing.T) {
	fsys := newFakeFS(map[string]int64{
		tPF + `\PCBlock\PCBlock.exe`:                                100,
		tPF + `\WindowsApps\Junk_1.0_x64__abc\junk.exe`:             100,
		tPF86 + `\Steam\steam.exe`:                                  100,
		tPF86 + `\Steam\steamapps\common\Terraria\Terraria.exe`:     100, // 3 уровня: только по каталогу
		tPF86 + `\Steam\steamapps\common\Unknown\unknown.exe`:       100,
		tPF + `\Roblox\Versions\version-1\RobloxPlayerBeta.exe`:     100,
		tPF + `\Roblox\Versions\version-1\RobloxPlayerLauncher.exe`: 10,
		tPF + `\Games\a\b\c\d\minecraft.exe`:                        100, // глубже 3 уровней: не ищем
	})
	got, err := testScanner(fsys).Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	want := []string{
		tPF86 + `\Steam\steam.exe`,
		tPF86 + `\Steam\steamapps\common\Terraria\Terraria.exe`,
		tPF + `\Roblox\Versions\version-1\RobloxPlayerBeta.exe`,
		tPF + `\Roblox\Versions\version-1\RobloxPlayerLauncher.exe`, // по каталогу, не главный
	}
	if g := foundPaths(got); !slices.Equal(g, want) {
		t.Fatalf("найдено:\n%s\nждали:\n%s", strings.Join(g, "\n"), strings.Join(want, "\n"))
	}
	if _, ok := programs.DefaultCatalog.Lookup("terraria.exe"); !ok {
		t.Fatal("terraria.exe нет в каталоге: тест проверяет не то")
	}
}

const manifestApp = `<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10" xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10">
  <Identity Name="Microsoft.MinecraftUWP" Publisher="CN=Microsoft" Version="1.20.0.0"/>
  <Properties><DisplayName>Minecraft</DisplayName></Properties>
  <Applications>
    <Application Id="App" Executable="Minecraft.Windows.exe" EntryPoint="Minecraft.App"/>
    <Application Id="Helper" Executable="bin/Helper.exe"/>
    <Application Id="Web" StartPage="index.html"/>
    <Application Id="Bad1" Executable="..\..\evil.exe"/>
    <Application Id="Bad2" Executable="C:\Windows\evil.exe"/>
    <Application Id="Bad3" Executable="$targetnametoken$.exe"/>
  </Applications>
</Package>`

const manifestFramework = `<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10">
  <Properties><Framework>true</Framework></Properties>
  <Applications><Application Id="X" Executable="fw.exe"/></Applications>
</Package>`

const manifestResource = `<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10">
  <Properties><ResourcePackage>true</ResourcePackage></Properties>
</Package>`

// Ловит: пакеты Microsoft Store не попадают в снимок (приложение Store, поставленное до PC Block,
// стало бы «незнакомым»), в снимок попадают фреймворки и ресурсные пакеты, а путь exe из
// манифеста выходит за папку пакета (Д-3, дополнения оркестратора).
func TestScanStorePackagesFromManifest(t *testing.T) {
	fsys := newFakeFS(nil)
	mc := tStore + `\Microsoft.MinecraftUWP_1.20.0.0_x64__8wekyb3d8bbwe`
	fsys.addFile(mc+`\AppxManifest.xml`, manifestApp)
	fsys.addFile(tStore+`\Contoso.Tool_2.0.0.0_x64__abcdef\AppxManifest.xml`, strings.ReplaceAll(manifestApp, "Minecraft.Windows.exe", "Tool.exe"))
	fsys.addFile(tStore+`\Microsoft.VCLibs.140.00_14.0.0.0_x64__8wekyb3d8bbwe\AppxManifest.xml`, manifestApp)
	fsys.addFile(tStore+`\Microsoft.NET.Native.Runtime.2.2_2.2.0.0_x64__8wekyb3d8bbwe\AppxManifest.xml`, manifestApp)
	fsys.addFile(tStore+`\Microsoft.UI.Xaml.2.8_8.0.0.0_x64__8wekyb3d8bbwe\AppxManifest.xml`, manifestApp)
	fsys.addFile(tStore+`\Contoso.Runtime.Framework_1.0.0.0_x64__abcdef\AppxManifest.xml`, manifestApp)
	fsys.addFile(tStore+`\Contoso.Fw_1.0.0.0_x64__abcdef\AppxManifest.xml`, manifestFramework)
	fsys.addFile(tStore+`\Contoso.Tool_2.0.0.0_neutral_split.scale-100_abcdef\AppxManifest.xml`, manifestResource)
	fsys.add(tStore+`\Deleted`, -1)
	fsys.addFile(tStore+`\Broken_1.0.0.0_x64__abcdef\AppxManifest.xml`, "<Package><Applications><Application")
	fsys.add(tPF86, -1)

	got, err := testScanner(fsys).Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	want := []string{
		tStore + `\Contoso.Tool_2.0.0.0_x64__abcdef\bin\Helper.exe`,
		tStore + `\Contoso.Tool_2.0.0.0_x64__abcdef\Tool.exe`,
		mc + `\bin\Helper.exe`,
		mc + `\Minecraft.Windows.exe`,
	}
	if g := foundPaths(got); !slices.Equal(g, want) {
		t.Fatalf("найдено:\n%s\nждали:\n%s", strings.Join(g, "\n"), strings.Join(want, "\n"))
	}
}

// Ловит: одна нечитаемая папка роняет весь снимок (тогда наблюдатель не заработал бы никогда),
// или, наоборот, снимок без единого прочитанного корня считается удачным: компьютер выглядел бы
// пустым, и все программы стали бы «незнакомыми» (RM3-10, R18).
func TestScanErrorsPerFolderAndAllRoots(t *testing.T) {
	fsys := newFakeFS(map[string]int64{
		tPF + `\Good\good.exe`:    100,
		tPF + `\Locked\x.exe`:     100,
		tPF86 + `\Other\o.exe`:    100,
		tStore + `\..placeholder`: 1,
	})
	fsys.denied[strings.ToLower(tPF+`\Locked`)] = true
	fsys.denied[strings.ToLower(tStore)] = true
	got, err := testScanner(fsys).Scan()
	if err != nil {
		t.Fatalf("Scan с одной нечитаемой папкой: %v", err)
	}
	if g, want := foundPaths(got), []string{tPF86 + `\Other\o.exe`, tPF + `\Good\good.exe`}; !slices.Equal(g, want) {
		t.Fatalf("найдено %q; ждали %q", g, want)
	}

	none := newFakeFS(nil)
	if got, err := testScanner(none).Scan(); err == nil {
		t.Fatalf("ни один корень не прочитан, а ошибки нет: %v", got)
	}
}

// Ловит: снимок длиннее 400 записей (мастер и база раздуваются) или при обрезке теряются главные
// exe корней приложений, а остаются находки по каталогу; порядок зависит от порядка обхода.
func TestScanLimitsEntriesMainsFirstAndSorted(t *testing.T) {
	sizes := map[string]int64{}
	for i := range 30 {
		sizes[fmt.Sprintf(`%s\App%02d\app.exe`, tPF, i)] = 100
	}
	for i := range 30 {
		sizes[fmt.Sprintf(`%s\Aaa\sub%02d\x\y\terraria.exe`, tPF, i)] = 100 // по каталогу, 3 уровня
	}
	s := testScanner(newFakeFS(sizes))
	s.maxEntries = 31
	got, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 31 {
		t.Fatalf("записей %d; ждали 31", len(got))
	}
	mains := 0
	for _, f := range got {
		if strings.HasSuffix(f.Path, `\app.exe`) {
			mains++
		}
	}
	if mains != 30 {
		t.Errorf("главных exe %d из 30: при обрезке потеряны корни приложений", mains)
	}
	if !sort.SliceIsSorted(got, func(i, j int) bool {
		return programs.Normalize(got[i].Path) < programs.Normalize(got[j].Path)
	}) {
		t.Errorf("результат не отсортирован: %q", foundPaths(got))
	}
	s2 := testScanner(newFakeFS(sizes))
	s2.maxEntries = 31
	again, _ := s2.Scan()
	if !slices.Equal(foundPaths(got), foundPaths(again)) {
		t.Errorf("два сканирования одного дерева дали разный результат")
	}
}

// Ловит: поиск по каталогу не ограничен числом файлов (на компьютере с играми и SDK обход шёл бы
// минутами), а предел срезает и главные exe корней.
func TestScanFileLimitStopsCatalogOnly(t *testing.T) {
	sizes := map[string]int64{tPF + `\Zeta\zeta.exe`: 100}
	for i := range 50 {
		sizes[fmt.Sprintf(`%s\Big\a\b\c%02d\f.dll`, tPF, i)] = 1
		sizes[fmt.Sprintf(`%s\Big\a\b\c%02d\terraria.exe`, tPF, i)] = 1
	}
	s := testScanner(newFakeFS(sizes))
	s.maxFiles = 10
	got, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !slices.Contains(foundPaths(got), tPF+`\Zeta\zeta.exe`) {
		t.Errorf("главный exe потерян из-за предела файлов: %q", foundPaths(got))
	}
	games := 0
	for _, f := range got {
		if strings.HasSuffix(f.Path, `terraria.exe`) {
			games++
		}
	}
	if games > 10 {
		t.Errorf("по каталогу найдено %d при пределе 10 файлов", games)
	}
}

// Ловит: зависшее чтение папки (сетевая папка, заблокированный диск) держит сканер дольше
// общего таймаута; зависание на первом этапе (главные exe) выдаётся за удачный неполный снимок,
// и все пропущенные программы стали бы «незнакомыми».
func TestScanHangingFolder(t *testing.T) {
	sizes := map[string]int64{
		tPF + `\Good\good.exe`:           100,
		tPF + `\Hang\x.exe`:              100,
		tPF + `\Late\a\b\c\terraria.exe`: 100,
		tPF + `\Late\late.exe`:           100,
	}
	t.Run("главные exe", func(t *testing.T) {
		fsys := newFakeFS(sizes)
		ch := make(chan struct{})
		defer close(ch)
		fsys.hang[strings.ToLower(tPF+`\Hang`)] = ch
		s := testScanner(fsys)
		s.timeout, s.grace = 100*time.Millisecond, 100*time.Millisecond
		start := time.Now()
		got, err := s.Scan()
		if d := time.Since(start); d > time.Second {
			t.Fatalf("Scan шёл %v при таймауте 100 мс", d)
		}
		if err == nil {
			t.Fatalf("первый этап не закончен, а ошибки нет: %q", foundPaths(got))
		}
	})
	t.Run("каталог", func(t *testing.T) {
		fsys := newFakeFS(sizes)
		ch := make(chan struct{})
		defer close(ch)
		fsys.hang[strings.ToLower(tPF+`\Late\a\b\c`)] = ch
		s := testScanner(fsys)
		s.timeout, s.grace = 100*time.Millisecond, 100*time.Millisecond
		start := time.Now()
		got, err := s.Scan()
		if d := time.Since(start); d > time.Second {
			t.Fatalf("Scan шёл %v при таймауте 100 мс", d)
		}
		if err != nil {
			t.Fatalf("главные exe найдены, зависла только папка третьего уровня: %v", err)
		}
		if g, want := foundPaths(got), []string{tPF + `\Good\good.exe`, tPF + `\Hang\x.exe`, tPF + `\Late\late.exe`}; !slices.Equal(g, want) {
			t.Fatalf("найдено %q; ждали %q", g, want)
		}
	})
}

// Ловит: паника сканера (неожиданные данные папки) роняет службу вместо ошибки снимка.
func TestScanPanicIsError(t *testing.T) {
	s := testScanner(panicFS{})
	if _, err := s.Scan(); err == nil {
		t.Fatal("паника сканера без ошибки")
	}
}

type panicFS struct{}

func (panicFS) ReadDir(string) ([]scanEntry, error)    { panic("сбой") }
func (panicFS) ReadFile(string, int64) ([]byte, error) { panic("сбой") }

// Ловит: выбор главного exe зависит от регистра и пробелов в имени папки («Notepad++» и
// «notepad++.exe», «Some App» и «SomeApp.exe»), установщики удаления выигрывают по размеру.
func TestPickMainExe(t *testing.T) {
	f := func(path string, size int64) scanFile {
		return scanFile{path: path, name: path[strings.LastIndex(path, `\`)+1:], size: size}
	}
	cases := []struct {
		name   string
		folder string
		levels [scanMainDepth + 1][]scanFile
		want   string
	}{
		{"имя папки", "Some App", [3][]scanFile{{f(`a\big.exe`, 900), f(`a\SomeApp.EXE`, 1)}}, `a\SomeApp.EXE`},
		{"имя папки глубже", "Zoom", [3][]scanFile{{f(`a\x.exe`, 9), f(`a\y.exe`, 8)}, {f(`a\b\zoom.exe`, 1)}}, `a\b\zoom.exe`},
		{"единственный", "Tool", [3][]scanFile{{f(`a\run.exe`, 1)}}, `a\run.exe`},
		{"самый большой", "Tool", [3][]scanFile{{f(`a\a.exe`, 1), f(`a\b.exe`, 3), f(`a\c.exe`, 2)}}, `a\b.exe`},
		{"равные по размеру", "Tool", [3][]scanFile{{f(`a\b.exe`, 3), f(`a\a.exe`, 3)}}, `a\a.exe`},
		{"удаление не главное", "Tool", [3][]scanFile{{f(`a\unins000.exe`, 9), f(`a\Uninstall.exe`, 9), f(`a\t.exe`, 1)}}, `a\t.exe`},
		{"только удаление", "Tool", [3][]scanFile{{f(`a\unins000.exe`, 9)}}, `a\unins000.exe`},
		{"следующий уровень", "Tool", [3][]scanFile{nil, nil, {f(`a\b\c\x.exe`, 1)}}, `a\b\c\x.exe`},
		{"пусто", "Tool", [3][]scanFile{}, ""},
	}
	for _, c := range cases {
		got, ok := pickMainExe(c.folder, c.levels)
		if got.path != c.want || ok != (c.want != "") {
			t.Errorf("%s: %q %v; want %q", c.name, got.path, ok, c.want)
		}
	}
}
