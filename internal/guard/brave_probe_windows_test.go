//go:build windows

package guard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Проба T0 вехи M4 (RM4-1, RM4-2, RM4-10): на раннере windows-latest от имени пользователя раннера
// настоящий Brave открывает адреса probe-*.test (в hosts раннера они ведут на 127.0.0.1, из сети
// ничего не грузится), а тест читает домен через UI Automation, закрывает вкладку и смотрит, действуют
// ли политики Brave из HKCU. Каждый шаг даёт одну строку-факт ::notice в $RUNNER_TEMP/pcb-brave-probe.txt
// (его выводит шаг windows.yml «Проба Brave (пометки)»): журналы Actions из песочницы не скачиваются
// (PL-1). Пометок одного вида у шага GitHub показывает не больше 10, поэтому факты шага собраны в одну
// строку. Тест падает только на шаге (б): без чтения домена дальше смотреть нечего.
//
// Запуск только при PCB_BRAVE_PROBE=1 (windows.yml ставит, если Brave установлен).

const (
	probeTitle   = "Проба Brave"
	probeSite    = "probe-site.test"
	probeTwo     = "probe-two.test"
	probeApp     = "probe-app.test"
	probeBlocked = "blocked-probe.test"
	bravePolicy  = `Software\Policies\BraveSoftware\Brave`
)

// probeLog пишет факты пробы: в журнал теста и строкой ::notice в файл.
type probeLog struct {
	t    *testing.T
	path string
}

func newProbeLog(t *testing.T) *probeLog {
	p := &probeLog{t: t}
	if dir := os.Getenv("RUNNER_TEMP"); dir != "" {
		p.path = filepath.Join(dir, "pcb-brave-probe.txt")
		_ = os.Remove(p.path)
	}
	return p
}

func (p *probeLog) fact(step string, format string, args ...any) {
	text := step + ": " + fmt.Sprintf(format, args...)
	text = strings.NewReplacer("\r", " ", "\n", " | ").Replace(text)
	p.t.Log(text)
	if p.path == "" {
		return
	}
	f, err := os.OpenFile(p.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		p.t.Errorf("факт пробы не записан: %v", err)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "::notice title=%s::%s\n", probeTitle, text)
}

// findBrave — путь brave.exe: Program Files, Program Files (x86), LOCALAPPDATA.
func findBrave() string {
	rel := `BraveSoftware\Brave-Browser\Application\brave.exe`
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if dir := os.Getenv(env); dir != "" {
			p := filepath.Join(dir, rel)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

func fileVersion(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return "версия не прочитана"
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return "версия не прочитана"
	}
	var fi *windows.VS_FIXEDFILEINFO
	var n uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&fi), &n); err != nil || fi == nil {
		return "версия не прочитана"
	}
	return fmt.Sprintf("%d.%d.%d.%d", fi.FileVersionMS>>16, fi.FileVersionMS&0xffff, fi.FileVersionLS>>16, fi.FileVersionLS&0xffff)
}

func processPath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, fgPathBuf)
	size := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &size) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// braveWindows — видимые окна верхнего уровня процессов brave.exe (класс Chrome_WidgetWin_1, шире
// 100 точек: у Chromium есть и скрытые служебные окна того же класса). Обратная функция одна на
// весь тест: windows.NewCallback ограничен числом вызовов на процесс.
var (
	enumMu   sync.Mutex
	enumOut  []windows.HWND
	enumOnce sync.Once
	enumCB   uintptr
)

func braveWindows() []windows.HWND {
	enumOnce.Do(func() {
		enumCB = windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
			if vis, _, _ := procIsWindowVisible.Call(hwnd); vis == 0 {
				return 1
			}
			if windowClass(hwnd) != "Chrome_WidgetWin_1" {
				return 1
			}
			var r rect
			if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 || r.right-r.left < 100 {
				return 1
			}
			var pid uint32
			procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			if isBravePath(processPath(pid)) {
				enumOut = append(enumOut, windows.HWND(hwnd))
			}
			return 1
		})
	})
	enumMu.Lock()
	defer enumMu.Unlock()
	enumOut = nil
	_ = windows.EnumWindows(enumCB, nil)
	return append([]windows.HWND(nil), enumOut...)
}

// braveProcs — PID всех процессов brave.exe.
func braveProcs() []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []uint32
	for err := windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), "brave.exe") {
			out = append(out, e.ProcessID)
		}
	}
	return out
}

func cpuTime(h windows.Handle) time.Duration {
	var c, e, k, u windows.Filetime
	if windows.GetProcessTimes(h, &c, &e, &k, &u) != nil {
		return 0
	}
	return time.Duration(k.Nanoseconds() + u.Nanoseconds())
}

// braveCPU — процессорное время всех процессов brave.exe по PID (процесс, которого нет, — 0).
func braveCPU() map[uint32]time.Duration {
	out := map[uint32]time.Duration{}
	for _, pid := range braveProcs() {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			continue
		}
		out[pid] = cpuTime(h)
		windows.CloseHandle(h)
	}
	return out
}

// Процессы Brave, запущенные тестом (PID процесса, который запустил exec), и их потомки — по дереву
// процессов (ParentProcessID). Чужие brave.exe тест не трогает.
var (
	startedMu   sync.Mutex
	startedPIDs = map[uint32]bool{}
)

// testBraveProcs — живые brave.exe, запущенные тестом, и их потомки (по снимку процессов).
func testBraveProcs() []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	type proc struct {
		parent uint32
		brave  bool
	}
	all := map[uint32]proc{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err := windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		all[e.ProcessID] = proc{parent: e.ParentProcessID, brave: strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), "brave.exe")}
	}
	startedMu.Lock()
	ours := map[uint32]bool{}
	for pid := range startedPIDs {
		ours[pid] = true
	}
	startedMu.Unlock()
	for grew := true; grew; {
		grew = false
		for pid, p := range all {
			if !ours[pid] && p.brave && ours[p.parent] {
				ours[pid], grew = true, true
			}
		}
	}
	var out []uint32
	for pid := range ours {
		if p, ok := all[pid]; ok && p.brave {
			out = append(out, pid)
		}
	}
	return out
}

// killTestBrave завершает brave.exe, запущенные тестом, с потомками и ждёт их выхода не дольше
// 10 с (профиль во временной папке иначе занят и не удаляется). Возвращает, сколько осталось.
func killTestBrave() int {
	end := time.Now().Add(10 * time.Second)
	for {
		left := testBraveProcs()
		if len(left) == 0 {
			return 0
		}
		if time.Now().After(end) {
			return len(left)
		}
		for _, pid := range left {
			if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid); err == nil {
				_ = windows.TerminateProcess(h, 1)
				windows.CloseHandle(h)
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// newProfile — временная папка профиля Brave. Очистка теста завершает Brave до удаления папки:
// t.Cleanup идёт в обратном порядке, эта регистрируется после t.TempDir.
func newProfile(t *testing.T) string {
	dir := t.TempDir()
	t.Cleanup(func() {
		if n := killTestBrave(); n > 0 {
			t.Logf("за 10 с не завершились %d процессов Brave, папка профиля может остаться", n)
		}
	})
	return dir
}

func startBrave(t *testing.T, exe string, args ...string) {
	cmd := exec.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		t.Logf("запуск Brave %v: %v", args, err)
		return
	}
	startedMu.Lock()
	startedPIDs[uint32(cmd.Process.Pid)] = true
	startedMu.Unlock()
	go func() { _ = cmd.Wait() }()
}

// waitWindow ждёт новое окно Brave, которого нет в old.
func waitWindow(old []windows.HWND, d time.Duration) windows.HWND {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		for _, h := range braveWindows() {
			known := false
			for _, o := range old {
				known = known || o == h
			}
			if !known {
				return h
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return 0
}

// readDomain — один Read окна: домен, источник, фокус и ошибка (текст поля дальше не идёт).
type readOut struct {
	domain, source string
	focused        bool
	err            error
}

func readOnce(r *braveReader, hwnd windows.HWND) readOut {
	text, focused, source, err := r.Read(hwnd)
	return readOut{domain: omniboxDomain(text), source: source, focused: focused, err: err}
}

// waitDomain читает домен окна, пока он не станет want, не дольше d.
func waitDomain(r *braveReader, hwnd windows.HWND, want string, d time.Duration) (readOut, time.Duration) {
	start := time.Now()
	var last readOut
	for time.Since(start) < d {
		last = readOnce(r, hwnd)
		if last.err == nil && last.domain == want {
			return last, time.Since(start)
		}
		time.Sleep(500 * time.Millisecond) // Chromium строит дерево доступности лениво: повтор раз в 500 мс
	}
	return last, time.Since(start)
}

func (o readOut) String() string {
	if o.err != nil {
		return "ошибка " + o.err.Error()
	}
	return fmt.Sprintf("домен %q, источник %q, фокус %v", o.domain, o.source, o.focused)
}

// uiaNames — Name элементов окна hwnd (все потомки или только данного типа), не больше max.
func uiaNames(r *braveReader, hwnd windows.HWND, controlType int32, max int) ([]string, error) {
	var names []string
	err := r.th.do(func(u uia) error {
		root, err := u.fromHandle(hwnd)
		if err != nil {
			return err
		}
		defer release(root)
		var cond comObj
		if controlType == 0 {
			cond, err = u.trueCond()
		} else {
			cond, err = u.intCond(uiaControlTypePropertyID, controlType)
		}
		if err != nil {
			return err
		}
		defer release(cond)
		els, err := findAll(root, treeScopeDescendants, cond, max)
		if err != nil {
			return err
		}
		for _, el := range els {
			if n, err := elemName(el); err == nil && n != "" {
				names = append(names, n)
			}
			release(el)
		}
		return nil
	})
	return names, err
}

// waitNames ждёт, пока среди Name элементов окна появится строка с want, не дольше d.
func waitNames(r *braveReader, hwnd windows.HWND, want string, d time.Duration) ([]string, bool) {
	end := time.Now().Add(d)
	var names []string
	for time.Now().Before(end) {
		names, _ = uiaNames(r, hwnd, 0, 3000)
		for _, n := range names {
			if strings.Contains(n, want) {
				return names, true
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return names, false
}

// cut — не больше n знаков строки.
func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// uiaTree — дерево UIA окна до глубины 6, не больше 60 узлов, не заходя внутрь Document: глубина,
// ControlType, ClassName, Name, AutomationId, FrameworkId, есть ли ValuePattern.
func uiaTree(r *braveReader, hwnd windows.HWND) ([]string, error) {
	var lines []string
	err := r.th.do(func(u uia) error {
		root, err := u.fromHandle(hwnd)
		if err != nil {
			return err
		}
		defer release(root)
		return walkTree(u, root, 6, 60, func(el comObj, depth int) (bool, bool) {
			ct, _ := elemControlType(el)
			cls, _ := elemClassName(el)
			name, _ := elemName(el)
			id, _ := elemString(el, uiaAutomationIDPropertyID)
			fw, _ := elemString(el, uiaFrameworkIDPropertyID)
			vp, _ := elemBool(el, uiaIsValuePatternAvailablePropID)
			lines = append(lines, fmt.Sprintf("%d:%d cls=%q name=%q id=%q fw=%q vp=%v", depth, ct, cut(cls, 30), cut(name, 30), cut(id, 20), fw, vp))
			return false, false
		})
	})
	return lines, err
}

// docValues — что дают для первого Document окна ValuePattern, свойство Value (30045) и
// LegacyIAccessible Value (30093); на раннере адреса — только probe-*.test.
func docValues(r *braveReader, hwnd windows.HWND) string {
	var out string
	err := r.th.do(func(u uia) error {
		root, err := u.fromHandle(hwnd)
		if err != nil {
			return err
		}
		defer release(root)
		cond, err := u.intCond(uiaControlTypePropertyID, uiaDocumentControlTypeID)
		if err != nil {
			return err
		}
		doc, err := findFirst(root, treeScopeDescendants, cond)
		release(cond)
		if err != nil {
			return err
		}
		defer release(doc)
		vp, vpErr := elemValue(doc)
		pv, pvErr := elemString(doc, uiaValueValuePropertyID)
		lv, lvErr := elemString(doc, uiaLegacyValuePropertyID)
		out = fmt.Sprintf("ValuePattern %q (ошибка %v); Value 30045 %q (ошибка %v); LegacyIAccessible Value %q (ошибка %v)",
			cut(vp, 80), vpErr, cut(pv, 80), pvErr, cut(lv, 80), lvErr)
		return nil
	})
	if err != nil {
		return "Document: " + err.Error()
	}
	return out
}

// treeFacts пишет дерево UIA окна тремя пометками.
func treeFacts(log *probeLog, step string, r *braveReader, hwnd windows.HWND) {
	lines, err := uiaTree(r, hwnd)
	if err != nil {
		log.fact(step, "дерево UIA не прочитано: %v (узлов %d)", err, len(lines))
		return
	}
	if len(lines) == 0 {
		log.fact(step, "в дереве UIA окна нет ни одного узла")
		return
	}
	const parts = 3
	per := (len(lines) + parts - 1) / parts
	for i := 0; i*per < len(lines); i++ {
		end := min((i+1)*per, len(lines))
		log.fact(fmt.Sprintf("%s %d/%d", step, i+1, (len(lines)+per-1)/per), "узлов %d: %s", len(lines), strings.Join(lines[i*per:end], " · "))
	}
}

func setBravePolicies() error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, bravePolicy, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer k.Close()
	for name, v := range map[string]uint32{"TorDisabled": 1, "BraveVPNDisabled": 1} {
		if err := k.SetDWordValue(name, v); err != nil {
			return err
		}
	}
	if err := k.SetStringValue("DnsOverHttpsMode", "off"); err != nil {
		return err
	}
	for sub, v := range map[string]string{"ExtensionInstallBlocklist": "*", "URLBlocklist": probeBlocked} {
		sk, _, err := registry.CreateKey(k, sub, registry.ALL_ACCESS)
		if err != nil {
			return err
		}
		err = sk.SetStringValue("1", v)
		sk.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func removeBravePolicies() {
	for _, sub := range []string{"ExtensionInstallBlocklist", "URLBlocklist", ""} {
		p := bravePolicy
		if sub != "" {
			p += `\` + sub
		}
		_ = registry.DeleteKey(registry.CURRENT_USER, p)
	}
}

func shortList(xs []string, max int) string {
	if len(xs) > max {
		return strings.Join(xs[:max], "; ") + fmt.Sprintf("; … ещё %d", len(xs)-max)
	}
	return strings.Join(xs, "; ")
}

func TestBraveProbe(t *testing.T) {
	if os.Getenv("PCB_BRAVE_PROBE") != "1" {
		t.Skip("проба Brave только при PCB_BRAVE_PROBE=1 (windows.yml, Brave установлен)")
	}
	log := newProbeLog(t)
	t.Cleanup(removeBravePolicies)

	// (а) Где Brave и какой версии.
	exe := findBrave()
	if exe == "" {
		log.fact("(а) установка", "brave.exe не найден в Program Files, Program Files (x86), LOCALAPPDATA")
		t.Fatal("brave.exe не найден")
	}
	log.fact("(а) установка", "%s, версия файла %s", exe, fileVersion(exe))

	common := func(profile string) []string {
		return []string{"--no-first-run", "--no-default-browser-check", "--user-data-dir=" + profile}
	}
	r := newBraveReader()

	// (б) Окно Brave на probe-site.test: поле адреса за 10 с, повтор раз в 500 мс. Без поля адреса
	// проба идёт дальше (факты (в)…(е)), а тест падает в конце.
	var readOK bool // поле адреса нашлось при обычном запуске (без флагов доступности)
	defer func() {
		if !readOK {
			t.Errorf("домен %q не прочитан из поля адреса Brave при обычном запуске (факт (б))", probeSite)
		}
	}()
	open := func(step, profile string, extra ...string) (windows.HWND, bool) {
		args := append(common(profile), extra...)
		startBrave(t, exe, append(args, "--new-window", "http://"+probeSite+"/a?b=c")...)
		hwnd := waitWindow(nil, 20*time.Second)
		if hwnd == 0 {
			log.fact(step, "окно Brave не появилось за 20 с")
			return 0, false
		}
		forceForeground(uintptr(hwnd))
		got, took := waitDomain(r, hwnd, probeSite, 10*time.Second)
		desc, derr := r.Describe(hwnd)
		fg, _, _ := procGetForegroundWin.Call()
		ok := got.err == nil && got.domain == probeSite && got.source == braveSourceOmnibox
		log.fact(step, "через %.1f с: %s; элемент: %s (ошибка %v); окно Brave на переднем плане: %v; поле адреса найдено: %v",
			took.Seconds(), got, desc, derr, fg == uintptr(hwnd), ok)
		if !ok {
			treeFacts(log, step+" дерево UIA", r, hwnd)
			log.fact(step+" Document", "%s", docValues(r, hwnd))
		}
		return hwnd, ok
	}
	profile := newProfile(t)
	hwnd, readOK := open("(б) чтение домена", profile)
	label := ""
	if !readOK && hwnd != 0 {
		// Для сравнения: тот же запуск с --force-renderer-accessibility (в бою флагов запуска нет).
		killTestBrave()
		profile = newProfile(t)
		var ok bool
		hwnd, ok = open("(б2) с --force-renderer-accessibility", profile, "--force-renderer-accessibility")
		if ok {
			label = " (окно с --force-renderer-accessibility)"
		} else {
			hwnd = 0
		}
	}
	const skipped = "пропущено: нет поля адреса"

	// (в) 60 чтений раз в секунду: процессор теста и всех процессов Brave.
	if hwnd == 0 {
		log.fact("(в) процессор за 60 с", skipped)
	} else {
		selfBefore, braveBefore := cpuTime(windows.CurrentProcess()), braveCPU()
		errs, wrong := 0, 0
		var slowest time.Duration
		for i := 0; i < 60; i++ {
			s := time.Now()
			o := readOnce(r, hwnd)
			if d := time.Since(s); d > slowest {
				slowest = d
			}
			if o.err != nil {
				errs++
			} else if o.domain != probeSite {
				wrong++
			}
			time.Sleep(time.Second - time.Since(s))
		}
		selfAfter, braveAfter := cpuTime(windows.CurrentProcess()), braveCPU()
		var braveDelta time.Duration
		for pid, after := range braveAfter {
			braveDelta += after - braveBefore[pid] // новый процесс: всё его время
		}
		log.fact("(в) процессор за 60 с", "тест %d мс, все процессы Brave (%d) %d мс; самое долгое чтение %d мс; ошибок %d, чужой домен %d%s",
			(selfAfter - selfBefore).Milliseconds(), len(braveAfter), braveDelta.Milliseconds(), slowest.Milliseconds(), errs, wrong, label)
	}

	// (г) Вторая вкладка в том же окне, Ctrl+W, затем кнопка закрытия выбранной вкладки.
	if hwnd == 0 {
		log.fact("(г) закрытие вкладки", skipped)
	} else {
		var g []string
		startBrave(t, exe, append(common(profile), "http://"+probeTwo+"/")...)
		two, took := waitDomain(r, hwnd, probeTwo, 10*time.Second)
		g = append(g, fmt.Sprintf("вторая вкладка: %s за %.1f с; окон Brave %d", two, took.Seconds(), len(braveWindows())))
		if two.domain == probeTwo {
			forceForeground(uintptr(hwnd))
			time.Sleep(300 * time.Millisecond)
			fg, _, _ := procGetForegroundWin.Call()
			cerr := closeActiveTab()
			back, took := waitDomain(r, hwnd, probeSite, 2*time.Second)
			g = append(g, fmt.Sprintf("Ctrl+W (окно на переднем плане %v, ошибка %v): через %.1f с %s", fg == uintptr(hwnd), cerr, took.Seconds(), back))
		}
		startBrave(t, exe, append(common(profile), "http://"+probeTwo+"/")...)
		if two, _ := waitDomain(r, hwnd, probeTwo, 10*time.Second); two.domain == probeTwo {
			berr := closeTabButton(r, hwnd)
			back, took := waitDomain(r, hwnd, probeSite, 2*time.Second)
			g = append(g, fmt.Sprintf("кнопка закрытия вкладки UIA (ошибка %v): через %.1f с %s", berr, took.Seconds(), back))
		} else {
			g = append(g, "кнопка закрытия: вторая вкладка не открылась повторно")
		}
		if tabs, err := uiaNames(r, hwnd, uiaTabItemControlTypeID, 20); err == nil {
			g = append(g, fmt.Sprintf("вкладок TabItem %d", len(tabs)))
		} else {
			g = append(g, "TabItem: "+err.Error())
		}
		g = append(g, "закреплённая вкладка не проверена: закрепить можно только меню вкладки")
		log.fact("(г) закрытие вкладки", "%s%s", strings.Join(g, "; "), label)
	}

	// (д) Окно сайта-приложения без поля адреса: Read даёт document?
	before := braveWindows()
	startBrave(t, exe, append(common(profile), "--app=http://"+probeApp+"/")...)
	if app := waitWindow(before, 15*time.Second); app == 0 {
		log.fact("(д) окно приложения", "окно --app не появилось за 15 с")
	} else {
		o, took := waitDomain(r, app, probeApp, 10*time.Second)
		desc, derr := r.Describe(app)
		log.fact("(д) окно приложения", "через %.1f с %s; элемент: %s (ошибка %v); Document: %s", took.Seconds(), o, desc, derr, docValues(r, app))
	}

	// (е) Политики Brave из HKCU: новый профиль, запрещённый домен, brave://policy, кнопка VPN.
	killTestBrave()
	if err := setBravePolicies(); err != nil {
		log.fact("(е) политики", "ключ %s не записан: %v", bravePolicy, err)
		return
	}
	pol := newProfile(t)
	startBrave(t, exe, append(common(pol), "--new-window", "http://"+probeBlocked+"/")...)
	bw := waitWindow(nil, 20*time.Second)
	if bw == 0 {
		log.fact("(е) политики", "окно Brave с политиками не появилось за 20 с")
		return
	}
	forceForeground(uintptr(bw))
	names, blocked := waitNames(r, bw, "ERR_BLOCKED_BY_ADMINISTRATOR", 10*time.Second)
	var errLines []string
	for _, n := range names {
		if strings.Contains(n, "ERR_") {
			errLines = append(errLines, n)
		}
	}
	log.fact("(е1) запрещённый домен", "страница блокировки ERR_BLOCKED_BY_ADMINISTRATOR: %v; строки с ERR_: %s; элементов с Name %d; Document: %s",
		blocked, shortList(errLines, 5), len(names), docValues(r, bw))

	buttons, berr := uiaNames(r, bw, uiaButtonControlTypeID, 200)
	var vpn []string
	for _, b := range buttons {
		if strings.Contains(strings.ToUpper(b), "VPN") {
			vpn = append(vpn, b)
		}
	}
	log.fact("(е2) кнопка VPN", "кнопок в окне %d (ошибка %v); с VPN в Name: %d [%s]; кнопки: %s",
		len(buttons), berr, len(vpn), shortList(vpn, 5), shortList(buttons, 25))

	startBrave(t, exe, append(common(pol), "brave://policy")...)
	pnames, ok := waitNames(r, bw, "TorDisabled", 10*time.Second)
	want := []string{"TorDisabled", "BraveVPNDisabled", "DnsOverHttpsMode", "ExtensionInstallBlocklist", "URLBlocklist"}
	var seen []string
	for i, n := range pnames {
		for _, w := range want {
			if strings.Contains(n, w) {
				// Имя политики и несколько следующих строк (значение, источник, статус).
				end := min(i+6, len(pnames))
				seen = append(seen, strings.Join(pnames[i:end], " / "))
			}
		}
	}
	log.fact("(е3) brave://policy", "страница с TorDisabled: %v; строки: %s", ok, shortList(seen, 5))
}
