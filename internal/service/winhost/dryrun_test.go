package winhost

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"pcblock/internal/programs"
	"pcblock/internal/service"
)

var dryRoots = programs.Roots{
	SystemRoot:      `c:\windows`,
	ProgramFiles:    `c:\program files`,
	ProgramFilesX86: `c:\program files (x86)`,
	InstallDir:      `c:\program files\pcblock`,
}

// Ловит: сухой прогон (RM3-8) считает не то: процесс из Program Files, найденный снимком, или
// процесс Windows выглядит «незнакомым»; процесс вне WatchDir попадает в «закрываемые»; список
// незнакомых не отсортирован или длиннее 20; пометка ::notice занимает несколько строк (тогда
// GitHub её не разберёт).
func TestDryRunReport(t *testing.T) {
	found := []service.FoundProgram{
		{Path: `C:\Program Files\Git\git-cmd.exe`},
		{Path: `C:\Program Files (x86)\Steam\steam.exe`},
		{Path: `C:\Users\runner\evil.exe`}, // вне доверенных каталогов: снимок это не разрешает
		{Path: `C:\Windows\explorer.exe`},  // система: в снимок не идёт
	}
	c := programs.NewClassifier(dryRoots, programs.DefaultCatalog)
	c.Reload(snapshotEntries(c, found))

	watch := `c:\temp\pcb-programs`
	list := []service.ProcInfo{
		{PID: 10, Path: `C:\Windows\System32\svchost.exe`, PathOK: true},
		{PID: 11, Path: `C:\Program Files\Git\usr\bin\bash.exe`, PathOK: true}, // наследует корень Git
		{PID: 12, Path: `C:\Program Files\PCBlock\PCBlock.exe`, PathOK: true},  // PC Block
		{PID: 13, Path: `C:\hostedtoolcache\windows\go\1.27\x64\bin\go.exe`, PathOK: true},
		{PID: 14, Path: `C:\Temp\pcb-programs\pcb-test.exe`, PathOK: true},
		{PID: 15, PathOK: false},
		{PID: 16, Path: `C:\Program Files\Unscanned\x.exe`, PathOK: true}, // корень не в снимке
		{PID: 17, Path: `C:\Users\runner\evil.exe`, PathOK: true},
		{PID: 18, Path: `C:\Program Files (x86)\Steam\steamapps\common\G\g.exe`, PathOK: true},
	}
	safe := []string{`C:\hostedtoolcache`, `C:\actions-runner`}
	r := dryRun(list, c, watch, safe)
	if r.Total != 9 || r.Unreadable != 1 || r.System != 2 || r.Allowed != 2 || r.Unknown != 4 || r.Forbidden != 0 {
		t.Errorf("счётчики %+v; ждали всего 9, нечитаемых 1, система 2, разрешено 2, незнакомых 4", r)
	}
	wantUnknown := []string{
		`c:\hostedtoolcache\windows\go\1.27\x64\bin\go.exe`,
		`c:\program files\unscanned\x.exe`,
		`c:\temp\pcb-programs\pcb-test.exe`,
		`c:\users\runner\evil.exe`,
	}
	if !slices.Equal(r.UnknownPaths, wantUnknown) {
		t.Errorf("незнакомые %q; want %q", r.UnknownPaths, wantUnknown)
	}
	if want := []string{`c:\program files\unscanned\x.exe`}; !slices.Equal(r.Misjudged, want) {
		t.Errorf("ошибки классификации в доверенных каталогах %q; want %q", r.Misjudged, want)
	}
	if want := []string{`c:\temp\pcb-programs\pcb-test.exe`}; !slices.Equal(r.Closable, want) {
		t.Errorf("закрываемые с фильтром WatchDir %q; want %q", r.Closable, want)
	}
	if len(r.Exposed) != 0 {
		t.Errorf("защищённые каталоги внутри WatchDir: %q", r.Exposed)
	}
	n := r.notice()
	if !strings.HasPrefix(n, "::notice title=") || strings.ContainsAny(n, "\r\n") {
		t.Errorf("пометка не одной строкой ::notice: %q", n)
	}
	for _, s := range []string{"процессов 9", "незнакомых 4", `c:\users\runner\evil.exe`, "закрываемых 1"} {
		if !strings.Contains(n, s) {
			t.Errorf("в пометке нет %q: %q", s, n)
		}
	}
}

// Ловит: WatchDir указывает внутрь защищённого каталога (Program Files, кэш инструментов раннера):
// фильтр тестового режима тогда не защищает раннер, и сухой прогон должен это показать.
func TestDryRunExposedWhenWatchDirInsideSafeDir(t *testing.T) {
	c := programs.NewClassifier(dryRoots, programs.DefaultCatalog)
	list := []service.ProcInfo{
		{PID: 13, Path: `C:\hostedtoolcache\windows\go\bin\go.exe`, PathOK: true},
	}
	r := dryRun(list, c, `c:\hostedtoolcache`, []string{`C:\hostedtoolcache`})
	if want := []string{`c:\hostedtoolcache\windows\go\bin\go.exe`}; !slices.Equal(r.Exposed, want) {
		t.Errorf("Exposed = %q; want %q", r.Exposed, want)
	}
	if r := dryRun(list, c, "", nil); len(r.Closable) != 0 {
		t.Errorf("без WatchDir закрываемых %q; наблюдатель в тестовом режиме без WatchDir ничего не закрывает", r.Closable)
	}
}

// Ловит: в пометке больше 20 путей (пометка обрезается GitHub и теряет итог).
func TestDryRunNoticeLimitsPaths(t *testing.T) {
	c := programs.NewClassifier(dryRoots, programs.DefaultCatalog)
	var list []service.ProcInfo
	for i := range 30 {
		list = append(list, service.ProcInfo{PID: uint32(100 + i), Path: `C:\Users\u\` + string(rune('a'+i%26)) + string(rune('a'+i/26)) + `.exe`, PathOK: true})
	}
	r := dryRun(list, c, "", nil)
	if r.Unknown != 30 {
		t.Fatalf("незнакомых %d; want 30", r.Unknown)
	}
	if got := strings.Count(r.notice(), `c:\users\u\`); got != 20 {
		t.Errorf("путей в пометке %d; want 20", got)
	}
}

// Ловит: статистика снимка не попадает в пометку сухого прогона ни при удаче, ни при ошибке
// сканера: прогон на windows-latest не показал бы, где уходит время (возврат T4b, п.1).
func TestDryRunNoticeCarriesScanStats(t *testing.T) {
	st := scanStats{Phase1: 12 * time.Second, Phase1Done: true, AppRoots: 230, Dirs: 5000, SlowestDir: `C:\Program Files\X`}
	r := dryRun(nil, programs.NewClassifier(dryRoots, programs.DefaultCatalog), "", nil)
	r.Scan = st.String()
	if n := r.notice(); !strings.Contains(n, "Снимок: этап 1 закончен за 12.0 с") || !strings.Contains(n, "корней приложений 230") {
		t.Errorf("в пометке нет статистики снимка: %q", n)
	}
	err := fmt.Errorf("сканирование программ не уложилось в 45 с (%s)", st)
	n := scanFailedNotice(err)
	if !strings.HasPrefix(n, "::notice title=Сухой прогон наблюдателя::снимок программ не сделан: ") ||
		!strings.Contains(n, `самая долгая папка C:\Program Files\X`) || strings.ContainsAny(n, "\r\n") {
		t.Errorf("пометка при ошибке снимка: %q", n)
	}
}
