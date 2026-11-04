package service

import (
	"testing"
	"time"
)

// Пути запусков в таблице «ручной или сам» (R13, D-65).
const (
	pathExplorer = `c:\windows\explorer.exe`
	pathSteam    = `c:\program files (x86)\steam\steam.exe`
	pathTool     = `c:\users\миша\downloads\tool.exe`
	pathUpdater  = `c:\program files\vendor\updater.exe`
)

// launchT0 — момент, когда наблюдатель увидел запуск в таблице.
var launchT0 = time.Date(2026, 11, 4, 18, 0, 0, 0, time.UTC)

// manualFacts — ручной запуск из Проводника: вход 5 мин назад, ввод 3 с назад.
func manualFacts() launchFacts {
	return launchFacts{
		At:         launchT0,
		LogonAt:    launchT0.Add(-5 * time.Minute),
		InputAt:    launchT0.Add(-3 * time.Second),
		Self:       pathTool,
		Parent:     pathExplorer,
		Shell:      pathExplorer,
		Foreground: pathExplorer,
	}
}

// Ловит (R13, D-65, FB-18): автозапуск запрещённой программы считается ручным (ребёнку плашка
// «запрещена родителем» и нарушение, хотя он ничего не запускал) или ручной запуск считается
// автоматическим (ребёнок не узнаёт, почему программа закрылась, родитель не видит нарушения).
// Границы 10 с ввода и 60 с после входа включительно: ровно 10 с и ровно 60 с — ручной.
func TestLaunchKind(t *testing.T) {
	cases := []struct {
		name   string
		change func(f *launchFacts)
		manual bool
	}{
		{"Проводник через 5 мин после входа, ввод 3 с назад", func(f *launchFacts) {}, true},
		{"Проводник в первые 60 с после входа (автозагрузка)", func(f *launchFacts) { f.LogonAt = launchT0.Add(-30 * time.Second) }, false},
		{"Проводник ровно через 60 с после входа", func(f *launchFacts) { f.LogonAt = launchT0.Add(-60 * time.Second) }, true},
		{"Проводник через 59,999 с после входа", func(f *launchFacts) { f.LogonAt = launchT0.Add(-60*time.Second + time.Millisecond) }, false},
		{"Проводник на переднем плане в первые 60 с: всё равно автозагрузка", func(f *launchFacts) {
			f.LogonAt = launchT0.Add(-10 * time.Second)
		}, false},
		{"Проводник, момент входа неизвестен", func(f *launchFacts) { f.LogonAt = time.Time{} }, false},
		{"родитель на переднем плане (Steam) и ввод", func(f *launchFacts) { f.Parent, f.Foreground = pathSteam, pathSteam }, true},
		{"родитель на переднем плане в первые 60 с после входа", func(f *launchFacts) {
			f.Parent, f.Foreground, f.LogonAt = pathSteam, pathSteam, launchT0.Add(-5*time.Second)
		}, true},
		{"родитель не на переднем плане (обновлятор)", func(f *launchFacts) { f.Parent = pathUpdater }, false},
		{"передний план не определён", func(f *launchFacts) { f.Parent, f.Foreground = pathSteam, "" }, false},
		{"ввод 30 с назад", func(f *launchFacts) { f.InputAt = launchT0.Add(-30 * time.Second) }, false},
		{"ввод ровно 10 с назад", func(f *launchFacts) { f.InputAt = launchT0.Add(-10 * time.Second) }, true},
		{"ввод 10,001 с назад", func(f *launchFacts) { f.InputAt = launchT0.Add(-10*time.Second - time.Millisecond) }, false},
		{"ввод сразу после запуска", func(f *launchFacts) { f.InputAt = launchT0.Add(200 * time.Millisecond) }, true},
		{"нет данных о вводе", func(f *launchFacts) { f.InputAt = time.Time{} }, false},
		{"Steam на переднем плане, но ввода не было", func(f *launchFacts) {
			f.Parent, f.Foreground, f.InputAt = pathSteam, pathSteam, launchT0.Add(-30*time.Second)
		}, false},
		{"родителя нет в списке сеанса", func(f *launchFacts) { f.Parent = "" }, false},
		{"родитель — тот же exe (перезапуск самой себя)", func(f *launchFacts) { f.Parent, f.Foreground = pathTool, pathTool }, false},
		{"путь Проводника не определён: родитель-Проводник не узнаётся", func(f *launchFacts) {
			f.Shell, f.Foreground = "", pathSteam
		}, false},
	}
	for _, k := range cases {
		t.Run(k.name, func(t *testing.T) {
			f := manualFacts()
			k.change(&f)
			manual, why := launchKind(f)
			if manual != k.manual {
				t.Errorf("launchKind: ручной %v (%q), want %v; родитель %q, передний план %q, ввод за %s до запуска, вход за %s",
					manual, why, k.manual, f.Parent, f.Foreground, agoText(f.At, f.InputAt), agoText(f.At, f.LogonAt))
			}
			if why == "" {
				t.Error("launchKind: пустое объяснение (подробный журнал не скажет, почему)")
			}
		})
	}
}

// agoText — «3s» до момента at; нулевой момент — «нет данных» (PL-32).
func agoText(at, t time.Time) string {
	if t.IsZero() {
		return "нет данных"
	}
	return at.Sub(t).String()
}

// Ловит (PL-9, PL-19): родитель процесса узнаётся по одному номеру — номер уже достался другому
// процессу (родитель завершился, его номер занят программой, созданной позже), родитель идёт от
// имени другого пользователя или его путь не прочитан, а запуск всё равно считается ручным.
func TestLaunchParentOf(t *testing.T) {
	child := ProcInfo{PID: 200, ParentPID: 100, Stamp: 50, SessionID: 2, Path: pathTool, PathOK: true, UserSID: childSID, UserOK: true}
	parent := ProcInfo{PID: 100, Stamp: 40, SessionID: 2, Path: `C:\Windows\Explorer.EXE`, PathOK: true, UserSID: childSID, UserOK: true}
	cases := []struct {
		name   string
		change func(p *ProcInfo)
		want   string
	}{
		{"родитель того же пользователя, создан раньше", func(p *ProcInfo) {}, pathExplorer},
		{"номер родителя занят процессом, созданным позже", func(p *ProcInfo) { p.Stamp = 60 }, ""},
		{"момент создания родителя не прочитан", func(p *ProcInfo) { p.Stamp = 0 }, ""},
		{"родитель от имени другого пользователя", func(p *ProcInfo) { p.UserSID = "S-1-5-21-9" }, ""},
		{"пользователь родителя не прочитан", func(p *ProcInfo) { p.UserOK = false }, ""},
		{"путь родителя не прочитан", func(p *ProcInfo) { p.PathOK = false }, ""},
		{"родитель в другом сеансе", func(p *ProcInfo) { p.SessionID = 0 }, ""},
	}
	for _, k := range cases {
		t.Run(k.name, func(t *testing.T) {
			p := parent
			k.change(&p)
			by := map[uint32]ProcInfo{p.PID: p, child.PID: child}
			if got := launchParentOf(child, by, childSID); got != k.want {
				t.Errorf("launchParentOf: %q, want %q (родитель %+v)", got, k.want, p)
			}
		})
	}
	orphan := child
	orphan.ParentPID = 0
	if got := launchParentOf(orphan, map[uint32]ProcInfo{parent.PID: parent}, childSID); got != "" {
		t.Errorf("launchParentOf без номера родителя: %q, want «»", got)
	}
	gone := child
	gone.ParentPID = 300
	if got := launchParentOf(gone, map[uint32]ProcInfo{parent.PID: parent}, childSID); got != "" {
		t.Errorf("launchParentOf, родителя нет в списке: %q, want «»", got)
	}
}
