//go:build windows

package winhost

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"pcblock/internal/programs"
	"pcblock/internal/service"
)

// Тесты адаптера наблюдателя на настоящей Windows (windows-latest, M3-T4b). Завершают только
// собственные дочерние процессы — копии notepad.exe (или cmd.exe, или самого теста) во временной
// папке; системные процессы только проверяются на отказ, Kill к ним не зовётся, если ручку на
// завершение открыть удалось бы.

const helperEnv = "PCB_WATCH_HELPER_SLEEP"

// TestHelperSleep — запасной дочерний процесс: копия теста, запущенная с этим тестом, спит.
// Без переменной окружения ничего не делает.
func TestHelperSleep(t *testing.T) {
	if os.Getenv(helperEnv) == "" {
		return
	}
	time.Sleep(60 * time.Second)
}

// testChild — дочерний процесс теста.
type testChild struct {
	cmd  *exec.Cmd
	done chan struct{}
	path string // путь копии exe как создан (длинный)
	kind string
}

func (c *testChild) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func currentSession(t *testing.T) uint32 {
	t.Helper()
	var s uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &s); err != nil {
		t.Fatalf("ProcessIdToSessionId: %v", err)
	}
	return s
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("копия %s: %v", src, err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("копия в %s: %v", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatalf("копия в %s: %v", dst, err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("копия в %s: %v", dst, err)
	}
}

// shortPathOf — 8.3-вариант пути (если на томе есть короткие имена), иначе сам путь.
func shortPathOf(p string) string {
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return p
	}
	buf := make([]uint16, 1024)
	n, err := windows.GetShortPathName(name, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) >= len(buf) {
		return p
	}
	return windows.UTF16ToString(buf[:n])
}

// startChild копирует notepad.exe (не запустился — cmd.exe, затем сам тест) в dir под именем name
// и запускает копию в текущем сеансе; viaShort — запуск по 8.3-пути. Процесс завершается в Cleanup.
func startChild(t *testing.T, dir, name string, viaShort bool) *testChild {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("папка %s: %v", dir, err)
	}
	sys := os.Getenv("SystemRoot")
	if sys == "" {
		sys = `C:\Windows`
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	type variant struct {
		kind, src string
		args      []string
		env       []string
		stdin     bool
	}
	variants := []variant{
		{kind: "notepad", src: filepath.Join(sys, "System32", "notepad.exe")},
		{kind: "cmd", src: filepath.Join(sys, "System32", "cmd.exe"), stdin: true},
		{kind: "test", src: self, args: []string{"-test.run=^TestHelperSleep$"}, env: []string{helperEnv + "=1"}},
	}
	for _, v := range variants {
		if _, err := os.Stat(v.src); err != nil {
			continue
		}
		dst := filepath.Join(dir, name)
		_ = os.Remove(dst)
		copyFile(t, v.src, dst)
		run := dst
		if viaShort {
			run = shortPathOf(dst)
		}
		cmd := exec.Command(run, v.args...)
		cmd.Env = append(os.Environ(), v.env...)
		var stdin io.WriteCloser
		if v.stdin {
			if stdin, err = cmd.StdinPipe(); err != nil {
				t.Fatalf("stdin: %v", err)
			}
		}
		if err := cmd.Start(); err != nil {
			t.Logf("%s не запустился: %v", v.kind, err)
			continue
		}
		c := &testChild{cmd: cmd, done: make(chan struct{}), path: dst, kind: v.kind}
		go func() { _ = cmd.Wait(); close(c.done) }()
		t.Cleanup(func() {
			if c.alive() {
				_ = cmd.Process.Kill() // свой дочерний процесс
			}
			if stdin != nil {
				_ = stdin.Close()
			}
			<-c.done
		})
		select {
		case <-c.done:
			t.Logf("%s завершился сразу после запуска, беру следующий вариант", v.kind)
			continue
		case <-time.After(700 * time.Millisecond):
		}
		return c
	}
	t.Fatal("не удалось запустить ни копию notepad.exe, ни cmd.exe, ни копию теста")
	return nil
}

// expectedPath — длинный путь файла, независимо от адаптера: filepath.EvalSymlinks разворачивает
// ссылки и 8.3-имена по FindFirstFile.
func expectedPath(t *testing.T, p string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", p, err)
	}
	return programs.Normalize(real)
}

func findPID(list []service.ProcInfo, pid uint32) (service.ProcInfo, bool) {
	for _, p := range list {
		if p.PID == pid {
			return p, true
		}
	}
	return service.ProcInfo{}, false
}

// Ловит: наблюдатель не видит процесс сеанса, путь приходит в 8.3-виде («RUNNER~1») или с
// регистром и ссылками как попало (тогда ключ таблицы программ не совпал бы, RM3-1); Kill
// завершает процесс с чужим моментом создания или другим путём (PL-9: номер процесса повторяется);
// повторный Kill уже завершённого процесса отвечает «завершён» (служба записала бы лишнее).
func TestProcessWatcherListsAndKillsChild(t *testing.T) {
	session := currentSession(t)
	dir := filepath.Join(t.TempDir(), "Игры и программы")
	c := startChild(t, dir, "pcb-тест игра.exe", true)
	want := expectedPath(t, c.path)
	t.Logf("дочерний процесс %s (pid %d), запущен по пути %s, ожидаемый путь %s", c.kind, c.cmd.Process.Pid, c.cmd.Path, want)
	if hasShortName(want) {
		t.Fatalf("ожидаемый путь %s сам содержит 8.3-имя: EvalSymlinks его не развернул", want)
	}

	w := newProcessWatcher()
	list, err := w.List(session)
	if err != nil {
		t.Fatalf("List(%d): %v", session, err)
	}
	pid := uint32(c.cmd.Process.Pid)
	p, ok := findPID(list, pid)
	if !ok {
		t.Fatalf("List(%d) не нашёл дочерний процесс %d среди %d процессов", session, pid, len(list))
	}
	if !p.PathOK || p.Stamp == 0 || p.SessionID != session {
		t.Fatalf("дочерний процесс: %+v; ждали PathOK, момент создания и сеанс %d", p, session)
	}
	if got := programs.Normalize(p.Path); got != want {
		t.Fatalf("путь %q (нормализованный %q); ждали %q (запуск по %q)", p.Path, got, want, c.cmd.Path)
	}
	for _, q := range list {
		if q.SessionID != session {
			t.Errorf("процесс %d из сеанса %d в списке сеанса %d", q.PID, q.SessionID, session)
		}
		if q.PID == 0 || q.PID == 4 || q.PID == uint32(os.Getpid()) {
			t.Errorf("в списке процесс %d (бездействие, System или сам тест)", q.PID)
		}
	}
	// Второй List берёт путь из памяти и даёт тот же ответ.
	list2, err := w.List(session)
	if err != nil {
		t.Fatalf("второй List: %v", err)
	}
	if p2, ok := findPID(list2, pid); !ok || p2 != p {
		t.Fatalf("второй List: %+v %v; ждали %+v", p2, ok, p)
	}

	wrongStamp := p
	wrongStamp.Stamp++
	if w.Kill(wrongStamp) {
		t.Fatal("Kill с чужим моментом создания завершил процесс")
	}
	wrongPath := p
	wrongPath.Path = filepath.Join(filepath.Dir(p.Path), "другая.exe")
	if w.Kill(wrongPath) {
		t.Fatal("Kill с другим путём завершил процесс")
	}
	unread := p
	unread.PathOK = false
	if w.Kill(unread) {
		t.Fatal("Kill процесса с непрочитанным путём завершил процесс")
	}
	time.Sleep(100 * time.Millisecond)
	if !c.alive() {
		t.Fatal("дочерний процесс завершился после отказов Kill")
	}

	if !w.Kill(p) {
		t.Fatal("Kill не завершил дочерний процесс")
	}
	select {
	case <-c.done:
	case <-time.After(time.Second):
		t.Fatal("Kill ответил «завершён», а процесс жив через 1 с")
	}
	if w.Kill(p) {
		t.Fatal("повторный Kill завершённого процесса ответил «завершён»")
	}
	list3, err := w.List(session)
	if err != nil {
		t.Fatalf("List после Kill: %v", err)
	}
	if _, ok := findPID(list3, pid); ok {
		t.Fatal("завершённый процесс остался в списке")
	}
}

// Ловит: цикл «List + Kill» раз в 250 мс (как служба, RM3-11) убирает незнакомую программу дольше
// 1 с (NFR-6). Допуск раннера — 1500 мс; в сообщении — фактическое время.
func TestProcessWatcherClosesWithinOneSecond(t *testing.T) {
	session := currentSession(t)
	w := newProcessWatcher()
	c := startChild(t, filepath.Join(t.TempDir(), "pcb-programs"), "pcb-быстро.exe", false)
	pid := uint32(c.cmd.Process.Pid)
	start := time.Now()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	var gone time.Duration
	kills := 0
	for time.Since(start) < 5*time.Second {
		list, err := w.List(session)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		p, ok := findPID(list, pid)
		if !ok {
			gone = time.Since(start)
			break
		}
		if w.Kill(p) {
			kills++
		}
		<-tick.C
	}
	if gone == 0 {
		t.Fatalf("процесс %d не исчез за 5 с цикла List+Kill (успешных Kill %d)", pid, kills)
	}
	t.Logf("процесс исчез через %d мс после начала цикла, успешных Kill %d", gone.Milliseconds(), kills)
	if gone > 1500*time.Millisecond {
		t.Errorf("процесс исчез через %d мс; ждали не больше 1000 мс (допуск раннера 1500 мс)", gone.Milliseconds())
	}
	if kills != 1 {
		t.Errorf("успешных Kill %d; ждали 1", kills)
	}
}

// Ловит: Kill защищённого процесса (csrss) или System паникует, отвечает «завершён» или вообще
// пытается завершить (отказ в доступе — штатный ответ false, Review Focus 4). Перед Kill тест сам
// проверяет, что ручку на завершение открыть нельзя: иначе Kill не зовётся (тест не трогает систему).
func TestProcessWatcherRefusesProtected(t *testing.T) {
	session := currentSession(t)
	w := newProcessWatcher()
	list, err := w.List(session)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	refuse := func(name string, p service.ProcInfo) {
		t.Helper()
		if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, p.PID); err == nil {
			_ = windows.CloseHandle(h)
			t.Fatalf("%s (pid %d) открывается на завершение: Kill не вызываю, проверка отказа невозможна", name, p.PID)
		}
		if w.Kill(p) {
			t.Errorf("Kill(%s, pid %d) ответил «завершён»", name, p.PID)
		}
	}
	var csrss service.ProcInfo
	for _, p := range list {
		if strings.EqualFold(filepath.Base(p.Path), "csrss.exe") {
			csrss = p
			break
		}
	}
	if csrss.PID == 0 {
		t.Fatalf("csrss.exe не найден в сеансе %d (процессов %d) или его путь не прочитан", session, len(list))
	}
	refuse("csrss", csrss)
	refuse("System", service.ProcInfo{PID: 4, Stamp: 1, SessionID: 0, Path: `C:\Windows\System32\ntoskrnl.exe`, PathOK: true})

	if got, err := w.List(0x7ffffff0); err != nil || len(got) != 0 {
		t.Errorf("List несуществующего сеанса: %d процессов, %v; ждали пусто без ошибки", len(got), err)
	}
}

// Ловит: junction, 8.3-имена и путь длиннее MAX_PATH не разворачиваются к одному виду (тогда
// ребёнок обошёл бы классификацию ссылкой на папку или коротким именем, Review Focus 2).
func TestCanonicalPathJunctionShortAndLong(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "Настоящая папка")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(target, "игра.exe")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := expectedPath(t, file)

	got, ok := canonicalPath(shortPathOf(file))
	if !ok || programs.Normalize(got) != want {
		t.Errorf("8.3-путь %s: %q %v; ждали %q", shortPathOf(file), got, ok, want)
	}

	link := filepath.Join(base, "ссылка")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("mklink /J: %v: %s", err, out)
	}
	got, ok = canonicalPath(filepath.Join(link, "игра.exe"))
	if !ok || programs.Normalize(got) != want {
		t.Errorf("через junction: %q %v; ждали %q", got, ok, want)
	}

	deep := filepath.Join(base, strings.Repeat("длинная папка-", 4), strings.Repeat("ещё одна папка-", 4),
		strings.Repeat("и ещё-", 8), strings.Repeat("последняя-", 8))
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	longFile := filepath.Join(deep, "игра.exe")
	if err := os.WriteFile(longFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(longFile) < maxPathChars {
		t.Fatalf("путь %d символов: короче MAX_PATH, тест проверяет не то", len(longFile))
	}
	wantLong := expectedPath(t, longFile)
	got, ok = canonicalPath(longFile)
	if !ok || programs.Normalize(got) != wantLong {
		t.Errorf("длинный путь (%d символов): %q %v; ждали %q", len(longFile), got, ok, wantLong)
	}

	if _, ok := canonicalPath(filepath.Join(base, `NOSUCH~1`, "x.exe")); ok {
		t.Error("неразвёрнутое 8.3-имя несуществующего файла принято за пригодный путь")
	}
}

// Ловит: издатель и описание не читаются из ресурса версии (в списке программ все были бы «Без
// подписи издателя»), отпечаток не в формате «размер:время», а у файла без ресурса версии
// подставляется что-то вместо пустых строк.
func TestFileMetaReadsVersionResource(t *testing.T) {
	sys := os.Getenv("SystemRoot")
	if sys == "" {
		sys = `C:\Windows`
	}
	exe := filepath.Join(sys, "explorer.exe")
	pub, descr, fp := fileMeta(exe)
	if pub != "Microsoft Corporation" || descr == "" {
		t.Errorf("%s: издатель %q, описание %q; ждали Microsoft Corporation и непустое описание", exe, pub, descr)
	}
	fi, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if want := fingerprintOf(fi.Size(), fi.ModTime()); fp != want {
		t.Errorf("отпечаток %q; ждали %q", fp, want)
	}

	plain := filepath.Join(t.TempDir(), "без версии.exe")
	if err := os.WriteFile(plain, []byte("не PE"), 0o644); err != nil {
		t.Fatal(err)
	}
	pfi, err := os.Stat(plain)
	if err != nil {
		t.Fatal(err)
	}
	pub, descr, fp = fileMeta(plain)
	if want := fingerprintOf(pfi.Size(), pfi.ModTime()); pub != "" || descr != "" || fp != want {
		t.Errorf("файл без ресурса версии: %q, %q, %q; ждали пустые издателя и описание и отпечаток %q", pub, descr, fp, want)
	}
	if pub, descr, fp := fileMeta(filepath.Join(t.TempDir(), "нет.exe")); pub != "" || descr != "" || fp != "" {
		t.Errorf("нет файла: %q, %q, %q; ждали пустые строки", pub, descr, fp)
	}
}

// Ловит: корни компьютера не из KnownFolders или не нормализованы (тогда класс «система» и
// доверенные каталоги не совпали бы с путями процессов).
func TestMachineFoldersRoots(t *testing.T) {
	f := readMachineFolders()
	r := f.roots()
	sys := programs.Normalize(os.Getenv("SystemRoot"))
	if r.SystemRoot == "" || r.SystemRoot != sys {
		t.Errorf("SystemRoot %q; ждали %q", r.SystemRoot, sys)
	}
	if pf := programs.Normalize(os.Getenv("ProgramFiles")); r.ProgramFiles != pf {
		t.Errorf("ProgramFiles %q; ждали %q", r.ProgramFiles, pf)
	}
	if pf := programs.Normalize(os.Getenv("ProgramFiles(x86)")); r.ProgramFilesX86 != pf {
		t.Errorf("ProgramFilesX86 %q; ждали %q", r.ProgramFilesX86, pf)
	}
	self, _ := os.Executable()
	if want := expectedPath(t, filepath.Dir(self)); r.InstallDir != want {
		t.Errorf("InstallDir %q; ждали %q", r.InstallDir, want)
	}
	if pd := expectedPath(t, os.Getenv("ProgramData")); r.ProgramData == "" || r.ProgramData != pd {
		t.Errorf("ProgramData %q; ждали %q", r.ProgramData, pd)
	}
	if up := expectedPath(t, filepath.Dir(os.Getenv("USERPROFILE"))); r.UserProfiles == "" || r.UserProfiles != up {
		t.Errorf("UserProfiles %q; ждали %q (папка над USERPROFILE)", r.UserProfiles, up)
	}
	if !r.IsSystem(filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Windows Defender", "Platform", "4.18.1-0", "MsMpEng.exe")) {
		t.Error("Защитник Windows не система при настоящих корнях")
	}
	if !r.IsSystem(filepath.Join(os.Getenv("SystemRoot"), "System32", "svchost.exe")) {
		t.Error("svchost.exe не система при настоящих корнях")
	}
}

// Сухой прогон (RM3-8): процессы сеанса раннера через настоящий снимок и классификатор. Kill не
// вызывается. Итог (со статистикой снимка, и при его ошибке) — пометка ::notice в файле $RUNNER_TEMP/pcb-watch-dryrun.txt (его выводит шаг
// windows.yml) и в журнале теста.
//
// Ловит: процесс папки Windows, Program Files или каталога установки после снимка «незнакомый»
// (на компьютере ребёнка он закрывался бы); WatchDir не отделяет тестовые программы от раннера;
// копия notepad в WatchDir не считается закрываемой (приёмка S12…S15 тогда ничего не проверила бы).
func TestWatcherDryRun(t *testing.T) {
	session := currentSession(t)
	folders := readMachineFolders()
	roots := folders.roots()
	writeNotice := func(notice string) {
		t.Log(notice)
		if dir := os.Getenv("RUNNER_TEMP"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, "pcb-watch-dryrun.txt"), []byte(notice+"\n"), 0o644); err != nil {
				t.Errorf("итог сухого прогона не записан: %v", err)
			}
		}
	}
	// Тест даёт сканеру 90 с, чтобы увидеть настоящее время, и отдельно сверяет его с бюджетом
	// службы (первый этап 45 с).
	sc := newWinScanner(folders)
	sc.phase1, sc.timeout = 90*time.Second, 90*time.Second
	found, stats, err := sc.scan()
	if err != nil {
		writeNotice(scanFailedNotice(err))
		t.Fatalf("снимок программ раннера: %v", err)
	}
	t.Logf("снимок: %s", stats)
	if stats.Phase1 > scanPhase1Timeout {
		t.Errorf("первый этап снимка шёл %.1f с, бюджет службы %d с: у заказчика снимок не сделался бы (%s)",
			stats.Phase1.Seconds(), int(scanPhase1Timeout.Seconds()), stats)
	}
	c := programs.NewClassifier(roots, programs.DefaultCatalog)
	c.Reload(snapshotEntries(c, found))

	watchDir := filepath.Join(t.TempDir(), "pcb-programs")
	child := startChild(t, watchDir, "pcb-test.exe", false)
	watchNorm := expectedPath(t, watchDir)

	list, err := newProcessWatcher().List(session)
	if err != nil {
		t.Fatalf("List(%d): %v", session, err)
	}
	safe := []string{os.Getenv("RUNNER_TOOL_CACHE"), `C:\hostedtoolcache`, `C:\actions-runner`, os.Getenv("RUNNER_WORKSPACE")}
	r := dryRun(list, c, watchNorm, safe)
	r.Scan = stats.String()
	writeNotice(r.notice())
	if len(r.Misjudged) > 0 {
		t.Errorf("после снимка «незнакомые» в доверенных каталогах (закрывались бы у ребёнка): %s", strings.Join(r.Misjudged, "; "))
	}
	if len(r.Exposed) > 0 {
		t.Errorf("WatchDir %s покрывает защищённые каталоги, закрываемыми стали бы: %s", watchNorm, strings.Join(r.Exposed, "; "))
	}
	if want := []string{expectedPath(t, child.path)}; !slices.Equal(r.Closable, want) {
		t.Errorf("закрываемые внутри WatchDir %q; ждали только тестовую копию %q", r.Closable, want)
	}
	if !child.alive() {
		t.Error("сухой прогон завершил тестовую копию: Kill в нём вызываться не должен")
	}
}
