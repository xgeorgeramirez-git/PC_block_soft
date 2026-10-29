package winhost

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// Тесты возврата T4b после windows-latest: «сканирование программ не уложилось в 10 с» на
// раннере с Visual Studio и SDK. Бюджеты, пул горутин, служебные папки и статистика сканера.

// Ловит: бюджеты сканера не те, что решил оркестратор: первый этап 45 с, всё сканирование 60 с,
// запас на зависшую папку 2 с, пул 8 горутин.
func TestScanBudgets(t *testing.T) {
	if scanPhase1Timeout != 45*time.Second || scanTimeout != 60*time.Second || scanWorkers != 8 || scanGrace != 2*time.Second {
		t.Errorf("бюджеты: этап 1 %v, всего %v, пул %d, запас %v; ждали 45 с, 60 с, 8, 2 с", scanPhase1Timeout, scanTimeout, scanWorkers, scanGrace)
	}
	s := newProgramScanner([]string{tPF}, tStore, "", nil, newFakeFS(nil))
	if s.phase1 != scanPhase1Timeout || s.timeout != scanTimeout || s.grace != scanGrace || s.workers != scanWorkers ||
		s.maxEntries != scanMaxEntries || s.maxFiles != scanMaxFiles {
		t.Errorf("newProgramScanner не берёт бюджеты: этап 1 %v, всего %v, запас %v, пул %d, записей %d, файлов %d",
			s.phase1, s.timeout, s.grace, s.workers, s.maxEntries, s.maxFiles)
	}
}

// Ловит: второй этап (каталог глубже) обрывается вместе с бюджетом первого: ему остаётся время
// до общего бюджета.
func TestScanPhase2UsesRemainingBudget(t *testing.T) {
	fsys := newFakeFS(map[string]int64{
		tPF + `\Late\late.exe`:            100,
		tPF + `\Late\a\b\c1\readme.txt`:   1,
		tPF + `\Late\a\b\c2\terraria.exe`: 100,
	})
	fsys.delay[strings.ToLower(tPF+`\Late\a\b\c1`)] = 150 * time.Millisecond
	s := testScanner(fsys)
	s.phase1, s.timeout, s.workers = 100*time.Millisecond, time.Second, 1
	got, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !slices.Contains(foundPaths(got), tPF+`\Late\a\b\c2\terraria.exe`) {
		t.Errorf("игра третьего уровня не найдена, хотя общий бюджет не вышел: %q", foundPaths(got))
	}
}

// Ловит: статистика сканирования врёт или пуста: следующий прогон на windows-latest не покажет,
// где уходит время (п.1 возврата T4b).
func TestScanStats(t *testing.T) {
	fsys := newFakeFS(map[string]int64{
		tPF + `\A\a.exe`:              10,
		tPF + `\A\x\y\z\deep.txt`:     1,
		tPF + `\Common Files\B\b.exe`: 10,
		tPF86 + `\C\c.dll`:            10,
	})
	fsys.addFile(tStore+`\Pkg_1.0.0.0_x64__abc\AppxManifest.xml`, manifestApp)
	fsys.delay[strings.ToLower(tPF86+`\C`)] = 40 * time.Millisecond
	_, st, err := testScanner(fsys).scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	// Папки этапа 1: Program Files, Program Files (x86), Common Files, A, A\x, A\x\y, B, C,
	// WindowsApps; этап 2: A\x\y\z. Файлы: a.exe, b.exe, c.dll, deep.txt.
	if st.AppRoots != 3 || st.StorePkgs != 1 || st.Dirs != 10 || st.Files != 4 || st.Found != 4 || !st.Phase1Done || !st.Phase2Done {
		t.Errorf("статистика %+v; ждали корней 3, пакетов 1, папок 10, файлов 4, найдено 4, оба этапа закончены", st)
	}
	if st.SlowestDir != tPF86+`\C` || st.Slowest < 40*time.Millisecond {
		t.Errorf("самая долгая папка %q за %v; ждали %s не меньше 40 мс", st.SlowestDir, st.Slowest, tPF86+`\C`)
	}
	if st.Phase1 < 40*time.Millisecond {
		t.Errorf("время этапа 1 %v; ждали не меньше 40 мс", st.Phase1)
	}
	text := st.String()
	for _, w := range []string{"этап 1", "этап 2", "корней приложений 3", "пакетов Store 1", "папок 10", "файлов 4", "найдено 4", tPF86 + `\C`} {
		if !strings.Contains(text, w) {
			t.Errorf("в статистике нет %q: %s", w, text)
		}
	}
	if strings.ContainsAny(text, "\r\n") {
		t.Errorf("статистика не одной строкой: %q", text)
	}
}

// Ловит: корни приложений обходятся по одному (на компьютере с сотнями программ первый этап не
// укладывается в бюджет) или результат зависит от порядка горутин.
func TestScanParallelDeterministic(t *testing.T) {
	sizes := map[string]int64{}
	for i := range 40 {
		sizes[fmt.Sprintf(`%s\App%02d\app%02d.exe`, tPF, i, i)] = int64(i)
	}
	fsys := newFakeFS(sizes)
	for i := range 40 {
		fsys.delay[strings.ToLower(fmt.Sprintf(`%s\App%02d`, tPF, i))] = 25 * time.Millisecond
	}
	start := time.Now()
	par, err := testScanner(fsys).Scan()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	seq := testScanner(newFakeFS(sizes))
	seq.workers = 1
	one, err := seq.Scan()
	if err != nil {
		t.Fatalf("Scan по одному: %v", err)
	}
	if len(par) != 40 || !slices.Equal(foundPaths(par), foundPaths(one)) {
		t.Errorf("параллельно %d записей, по одному %d; списки должны совпасть", len(par), len(one))
	}
	// При обрезке до предела записей остаются одни и те же (а не те, что горутины нашли первыми).
	for range 3 {
		cut, cutOne := testScanner(newFakeFS(sizes)), testScanner(newFakeFS(sizes))
		cut.maxEntries, cutOne.maxEntries, cutOne.workers = 20, 20, 1
		a, _ := cut.Scan()
		b, _ := cutOne.Scan()
		if len(a) != 20 || !slices.Equal(foundPaths(a), foundPaths(b)) {
			t.Fatalf("обрезка до 20: параллельно %q, по одному %q", foundPaths(a), foundPaths(b))
		}
	}
	if elapsed > 600*time.Millisecond {
		t.Errorf("40 корней по 25 мс шли %d мс: пул 8 горутин даёт около 125 мс, по одному 1000 мс", elapsed.Milliseconds())
	}
}

// Ловит: сканер спускается в заведомо большие служебные папки внутри корня (node_modules, .git,
// кэши, ресурсы) и тратит на них бюджет; exe из такой папки выигрывает у настоящей программы.
// Папка-корень с таким именем не пропускается, а exe в служебной папке остаётся запасным
// главным, когда другого нет: иначе корень приложения пропал бы из снимка и его программы
// закрывались бы как незнакомые.
func TestScanBulkyDirs(t *testing.T) {
	for _, n := range []string{"node_modules", ".git", "Packages", "Installer", "Temp", "tmp", "Cache", "CacheStorage", "Code Cache",
		"GPUCache", "locales", "Resources", "__pycache__", "site-packages", "share", "Lib", "include", "Docs"} {
		if !isBulkyDir(n) {
			t.Errorf("isBulkyDir(%q) = false", n)
		}
	}
	for _, n := range []string{"bin", "Application", "Office16", "root", "x64", "Brave-Browser", "Steam", "Common7", "Libraries"} {
		if isBulkyDir(n) {
			t.Errorf("isBulkyDir(%q) = true", n)
		}
	}
	fsys := newFakeFS(map[string]int64{
		tPF + `\Tool\bin\small.exe`:                    1,
		tPF + `\Tool\resources\huge.exe`:               900,
		tPF + `\Tool\node_modules\pkg\x\terraria.exe`:  1,
		tPF + `\Tool\node_modules\pkg\big.exe`:         900,
		tPF + `\VS\Installer\vs_installer.exe`:         5,
		tPF + `\VS\2022\Enterprise\Common7\devenv.txt`: 5,
		tPF + `\Installer\setup.exe`:                   5,
		tPF + `\Tool\bin\x64\cache\terraria.exe`:       1, // служебная папка третьего уровня
	})
	got, err := testScanner(fsys).Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	want := []string{tPF + `\Installer\setup.exe`, tPF + `\Tool\bin\small.exe`, tPF + `\VS\Installer\vs_installer.exe`}
	if g := foundPaths(got); !slices.Equal(g, want) {
		t.Fatalf("найдено %q; ждали %q", g, want)
	}
	for dir := range fsys.reads {
		if strings.Contains(dir, `node_modules\`) {
			t.Errorf("сканер спустился в %s", dir)
		}
	}
}

// Ловит: папки третьего уровня (только имена для каталога игр) читаются с размерами файлов, а
// уровни 0–2 (выбор главного exe по размеру) — без них.
func TestScanSizesOnlyForMainLevels(t *testing.T) {
	fsys := newFakeFS(map[string]int64{
		tPF + `\App\a.exe`:                 1,
		tPF + `\App\l1\l2\l3\terraria.exe`: 1,
	})
	if _, err := testScanner(fsys).Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for dir, sizes := range map[string]bool{
		strings.ToLower(tPF + `\App`):          true,
		strings.ToLower(tPF + `\App\l1\l2`):    true,
		strings.ToLower(tPF + `\App\l1\l2\l3`): false,
	} {
		got, ok := fsys.reads[dir]
		if !ok || got != sizes {
			t.Errorf("%s: прочитана %v, с размерами %v; ждали с размерами %v", dir, ok, got, sizes)
		}
	}
}

// Ловит: предел файлов второго этапа считает и файлы первого: на большом компьютере первый этап
// читает десятки тысяч файлов, и поиск игр по каталогу не начался бы вовсе.
func TestScanFileLimitCountsPhase2Only(t *testing.T) {
	sizes := map[string]int64{tPF + `\Game\a\b\c\terraria.exe`: 1}
	for i := range 20 {
		sizes[fmt.Sprintf(`%s\Big\f%02d.dll`, tPF, i)] = 1
	}
	s := testScanner(newFakeFS(sizes))
	s.maxFiles = 10
	got, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !slices.Contains(foundPaths(got), tPF+`\Game\a\b\c\terraria.exe`) {
		t.Errorf("игра третьего уровня не найдена: предел второго этапа съели файлы первого: %q", foundPaths(got))
	}
}
