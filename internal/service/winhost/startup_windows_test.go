//go:build windows

package winhost

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"pcblock/internal/service"
	"pcblock/internal/startup"
)

// Источник автозагрузки испытывается на настоящих местах текущего пользователя раннера (его
// SID — «ребёнок»): уникальные имена значений, ярлыка и задания, уборка в t.Cleanup. Тестовую
// ветку вместо Run подставить нельзя: проверяется именно то место, которое читает Windows.

func currentSID(t *testing.T) string {
	t.Helper()
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("SID процесса: %v", err)
	}
	return tu.User.Sid.String()
}

func testStartupSource() *startupSource {
	return newStartupSource(readMachineFolders(), fileMeta)
}

func findEntry(t *testing.T, s *startupSource, sid, source, name string) (startup.Entry, bool) {
	t.Helper()
	list, err := s.List(sid)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, e := range list {
		if e.Source == source && strings.EqualFold(e.Name, name) {
			return e, true
		}
	}
	return startup.Entry{}, false
}

func setUserValue(t *testing.T, sid, path string, set func(registry.Key) error, name string) {
	t.Helper()
	k, _, err := registry.CreateKey(registry.USERS, sid+`\`+path, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("ключ %s: %v", path, err)
	}
	defer k.Close()
	if err := set(k); err != nil {
		t.Fatalf("значение %s\\%s: %v", path, name, err)
	}
	t.Cleanup(func() { deleteUserValue(sid, path, name) })
}

func deleteUserValue(sid, path, name string) {
	k, err := registry.OpenKey(registry.USERS, sid+`\`+path, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.DeleteValue(name)
}

func approvedByte(t *testing.T, sid, sub, name string) byte {
	t.Helper()
	k, err := registry.OpenKey(registry.USERS, sid+`\`+approvedPath+sub, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("StartupApproved\\%s: %v", sub, err)
	}
	defer k.Close()
	b, _, err := k.GetBinaryValue(name)
	if err != nil || len(b) != 12 {
		t.Fatalf("StartupApproved\\%s\\%s = %x, %v; want 12 байт", sub, name, b, err)
	}
	return b[0]
}

func powershell(t *testing.T, script string) string {
	t.Helper()
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		t.Fatalf("powershell: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// Ловит (RM5b-1, RM5b-2, RM5b-7): своя запись Run не видна или не своя, команда и exe не те,
// %SystemRoot% не раскрыт и компонент Windows показан, выключение удаляет значение Run вместо
// StartupApproved, не тот байт состояния, состояние не читается обратно.
func TestStartupSourceOwnRun(t *testing.T) {
	sid := currentSID(t)
	s := testStartupSource()
	name := "PCBlockTestRun-" + strconv.Itoa(os.Getpid())
	hidden := name + "-win"
	exe := filepath.Join(t.TempDir(), "pcb startup.exe")
	cmd := `"` + exe + `" --tray`
	t.Cleanup(func() { deleteUserValue(sid, approvedPath+approvedRun, name) })
	setUserValue(t, sid, runPath, func(k registry.Key) error { return k.SetStringValue(name, cmd) }, name)
	setUserValue(t, sid, runPath, func(k registry.Key) error {
		return k.SetExpandStringValue(hidden, `%SystemRoot%\System32\notepad.exe`)
	}, hidden)

	e, ok := findEntry(t, s, sid, startup.SrcRun, name)
	if !ok {
		t.Fatalf("List не видит своё значение Run %s", name)
	}
	if e.Scope != startup.ScopeOwn || e.Command != cmd || !strings.EqualFold(e.Exe, exe) || !e.Enabled {
		t.Errorf("запись = %+v; want own, %q, exe %q, включена", e, cmd, exe)
	}
	if _, ok := findEntry(t, s, sid, startup.SrcRun, hidden); ok {
		t.Errorf("компонент Windows %s показан (RM5b-7)", hidden)
	}

	if err := s.SetEnabled(sid, e, false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if b := approvedByte(t, sid, approvedRun, name); b != 0x03 {
		t.Errorf("после выключения первый байт = %#x, want 0x03", b)
	}
	if regMissing(sid+`\`+runPath, name) {
		t.Error("выключение удалило значение Run")
	}
	if e2, ok := findEntry(t, s, sid, startup.SrcRun, name); !ok || e2.Enabled {
		t.Errorf("после выключения List: %+v, %v; want выключена", e2, ok)
	}
	if err := s.SetEnabled(sid, e, true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if b := approvedByte(t, sid, approvedRun, name); b != 0x02 {
		t.Errorf("после включения первый байт = %#x, want 0x02", b)
	}
	if e3, ok := findEntry(t, s, sid, startup.SrcRun, name); !ok || !e3.Enabled {
		t.Errorf("после включения List: %+v, %v; want включена", e3, ok)
	}
}

// Ловит (RM5b-2, уточнение 1): служба меняет общую запись в Windows; RunOnce «выключается»
// без штатного выключателя.
func TestStartupSourceSetEnabledRefuses(t *testing.T) {
	s := testStartupSource()
	sid := currentSID(t)
	shared := startup.Entry{Scope: startup.ScopeShared, Source: startup.SrcRun, Name: "X", Command: "x.exe"}
	if err := s.SetEnabled(sid, shared, false); !errors.Is(err, service.ErrSharedNotWritable) {
		t.Errorf("общая: %v; want ErrSharedNotWritable", err)
	}
	once := startup.Entry{Scope: startup.ScopeOwn, Source: startup.SrcRunOnce, Name: "X", Command: "x.exe"}
	if err := s.SetEnabled(sid, once, false); !errors.Is(err, service.ErrNotSwitchable) {
		t.Errorf("RunOnce: %v; want ErrNotSwitchable", err)
	}
}

// Ловит (RM5b-1, RM5b-2): ярлык папки «Автозагрузка» не виден, цель ярлыка не разобрана
// (Exe — сам ярлык), выключение не в StartupApproved\StartupFolder.
func TestStartupSourceOwnFolder(t *testing.T) {
	sid := currentSID(t)
	s := testStartupSource()
	dir, ok, err := profileDir(sid)
	if err != nil || !ok {
		t.Fatalf("профиль %s: %q, %v, %v", sid, dir, ok, err)
	}
	folder := filepath.Join(dir, userStartupDir)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "PCBlockTestLnk-" + strconv.Itoa(os.Getpid()) + ".lnk"
	lnk := filepath.Join(folder, name)
	target := filepath.Join(t.TempDir(), "pcbfolder.exe")
	if err := os.WriteFile(target, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(lnk)
		deleteUserValue(sid, approvedPath+approvedFolder, name)
	})
	powershell(t, `$s=(New-Object -ComObject WScript.Shell).CreateShortcut('`+lnk+`'); $s.TargetPath='`+target+`'; $s.Arguments='--min'; $s.Save()`)

	e, ok := findEntry(t, s, sid, startup.SrcFolder, name)
	if !ok {
		t.Fatalf("List не видит ярлык %s", lnk)
	}
	want, _ := canonicalPath(target)
	got, _ := canonicalPath(e.Exe)
	if e.Scope != startup.ScopeOwn || !strings.EqualFold(got, want) || !strings.HasSuffix(e.Command, " --min") || !e.Enabled {
		t.Errorf("ярлык = %+v; want own, exe %q, аргументы --min, включён", e, want)
	}
	if err := s.SetEnabled(sid, e, false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if b := approvedByte(t, sid, approvedFolder, name); b != 0x03 {
		t.Errorf("первый байт = %#x, want 0x03", b)
	}
	if _, err := os.Stat(lnk); err != nil {
		t.Errorf("выключение тронуло ярлык: %v", err)
	}
}

// Ловит (RM5b-1, RM5b-2, уточнение 7): задание «при входе» текущего пользователя с
// интерактивным входом не видно или не своё, путь задания не тот, /DISABLE не выключает.
func TestStartupSourceTask(t *testing.T) {
	sid := currentSID(t)
	s := testStartupSource()
	task := "PCBlockTestTask-" + strconv.Itoa(os.Getpid())
	exe := filepath.Join(t.TempDir(), "pcbtask.exe")
	t.Cleanup(func() {
		_ = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			`Unregister-ScheduledTask -TaskName '`+task+`' -Confirm:$false -ErrorAction SilentlyContinue`).Run()
	})
	powershell(t, `$a=New-ScheduledTaskAction -Execute '`+exe+`' -Argument '--x'; `+
		`$tr=New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME; `+
		`$p=New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive; `+
		`Register-ScheduledTask -TaskName '`+task+`' -Action $a -Trigger $tr -Principal $p | Out-Null`)

	e, ok := findEntry(t, s, sid, startup.SrcTask, `\`+task)
	if !ok {
		t.Fatalf("List не видит задание %s", task)
	}
	if e.Scope != startup.ScopeOwn || e.Task != `\`+task || !strings.EqualFold(e.Exe, exe) || !e.Enabled {
		t.Errorf("задание = %+v; want own, путь \\%s, exe %q, включено", e, task, exe)
	}
	if err := s.SetEnabled(sid, e, false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if st := powershell(t, `(Get-ScheduledTask -TaskName '`+task+`').State`); st != "Disabled" {
		t.Errorf("состояние задания = %q, want Disabled", st)
	}
	if e2, ok := findEntry(t, s, sid, startup.SrcTask, `\`+task); !ok || e2.Enabled {
		t.Errorf("после выключения List: %+v, %v; want выключено", e2, ok)
	}
	if err := s.SetEnabled(sid, e, true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if st := powershell(t, `(Get-ScheduledTask -TaskName '`+task+`').State`); st != "Ready" {
		t.Errorf("после включения состояние = %q, want Ready", st)
	}
}

// Ловит (RM5b-1, RM5b-2, уточнение 6): ключ приложения магазина в ветке классов не виден, нет
// State — показан, выключение пишет не 1, включение — не 2.
func TestStartupSourceStore(t *testing.T) {
	sid := currentSID(t)
	classes := sid + classesSuffix
	if ok, err := hiveLoaded(classes); err != nil || !ok {
		t.Skipf("ветка %s не загружена (%v): раннер без ветки классов", classes, err)
	}
	s := testStartupSource()
	pkg := "PCBlockTest.Fake" + strconv.Itoa(os.Getpid()) + "_test"
	base := classes + `\` + appDataPath + `\` + pkg
	t.Cleanup(func() { _ = deleteTree(registry.USERS, base) })
	k, _, err := registry.CreateKey(registry.USERS, base+`\App`, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("ключ %s: %v", base, err)
	}
	err = k.SetDWordValue(stateValue, 2)
	k.Close()
	if err != nil {
		t.Fatal(err)
	}
	k, _, err = registry.CreateKey(registry.USERS, base+`\NoState`, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	k.Close()

	e, ok := findEntry(t, s, sid, startup.SrcStore, pkg+"!App")
	if !ok {
		t.Fatalf("List не видит приложение магазина %s", pkg)
	}
	if e.Scope != startup.ScopeOwn || e.Package != pkg || e.Task != "App" || !e.Enabled {
		t.Errorf("приложение = %+v; want own, %s, App, включено", e, pkg)
	}
	if _, ok := findEntry(t, s, sid, startup.SrcStore, pkg+"!NoState"); ok {
		t.Error("ключ без State показан")
	}
	if err := s.SetEnabled(sid, e, false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if n := regDWord(t, base+`\App`, stateValue); n != 1 {
		t.Errorf("State после выключения = %d, want 1", n)
	}
	if err := s.SetEnabled(sid, e, true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if n := regDWord(t, base+`\App`, stateValue); n != 2 {
		t.Errorf("State после включения = %d, want 2", n)
	}
}
