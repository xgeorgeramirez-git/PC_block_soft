package service

import (
	"fmt"
	"strings"
	"time"

	"pcblock/internal/programs"
)

// Ручной или автоматический запуск программы (R13, D-65, FB-18). Плашку «запрещена родителем» и
// нарушение «попытка открыть запрещённое» получает только ручной запуск; запрещённая программа,
// которая запустилась сама (автозагрузка, планировщик, служба, обновлятор, перезапуск самой себя),
// закрывается молча, а родителю раз в день пишется «запускалась сама и закрыта». Решение —
// чистая функция launchKind над фактами launchFacts; факты собирает наблюдатель (watch.go) в
// момент, когда впервые видит процесс. Windows-адаптер только сообщает номер родителя (ParentPID).

const (
	// manualInputWindow — ребёнок пользовался клавиатурой или мышью не раньше чем за столько до
	// запуска: иначе запуск не ручной (включительно: ровно 10 с — ручной).
	manualInputWindow = 10 * time.Second
	// manualLogonGrace — сколько после входа в учётку запуск из Проводника считается автозагрузкой
	// (включительно: ровно через 60 с — уже ручной).
	manualLogonGrace = 60 * time.Second
)

// launchFacts — что известно о запуске процесса, когда наблюдатель увидел его впервые. Пути —
// нормализованные (programs.Normalize); пустой путь и нулевой момент — «неизвестно».
type launchFacts struct {
	At         time.Time // момент, когда наблюдатель впервые увидел процесс (запуск с точностью до опроса)
	LogonAt    time.Time // когда служба увидела ребёнка в этом сеансе (вход в учётку)
	InputAt    time.Time // последний ввод ребёнка по снимку стража
	Self       string    // путь процесса
	Parent     string    // путь родителя; «» — родителя нет в сеансе ребёнка (launchParentOf)
	Shell      string    // путь Проводника сеанса: %SystemRoot%\explorer.exe
	Foreground string    // путь программы на переднем плане по снимку стража
	Store      bool      // процесс — приложение Магазина (Program Files\WindowsApps)
	Broker     string    // путь svchost.exe: через него Windows запускает приложения Магазина
}

// launchKind решает, ручной ли запуск, и объясняет решение простыми словами (для подробного
// журнала работы). Ручной: ввод ребёнка не раньше manualInputWindow до запуска и (родитель —
// Проводник, и с входа прошло не меньше manualLogonGrace, или родитель — программа на переднем
// плане, или это приложение Магазина без родителя в сеансе либо с родителем svchost, и с входа
// прошло не меньше manualLogonGrace). Для Проводника передний план не в счёт: в первые секунды после входа на переднем плане
// он сам, а запускает автозагрузку. «Не знаю» — автоматический: ребёнку не показывается плашка
// о том, чего он не делал.
func launchKind(f launchFacts) (bool, string) {
	if f.InputAt.IsZero() {
		return false, "нет данных о вводе ребёнка"
	}
	if ago := f.At.Sub(f.InputAt); ago > manualInputWindow {
		return false, fmt.Sprintf("ребёнок ничего не нажимал %s до запуска", ago.Round(time.Millisecond))
	}
	if f.Store && (f.Parent == "" || (f.Broker != "" && f.Parent == f.Broker)) {
		// Приложение Магазина Windows запускает через svchost вне сеанса ребёнка: родителя в сеансе
		// нет и при ручном запуске. Решают свежий ввод и то, что это не первые 60 с после входа
		// (задачи автозагрузки Магазина, как у Проводника).
		if f.LogonAt.IsZero() {
			return false, "приложение Магазина, момент входа неизвестен"
		}
		if since := f.At.Sub(f.LogonAt); since < manualLogonGrace {
			return false, fmt.Sprintf("приложение Магазина через %s после входа (автозагрузка)", since.Round(time.Millisecond))
		}
		return true, "приложение Магазина, запущенное системой при вводе ребёнка"
	}
	switch {
	case f.Parent == "":
		return false, "запустившей программы нет в сеансе ребёнка"
	case f.Parent == f.Self:
		return false, "программа перезапустила сама себя"
	case f.Shell != "" && f.Parent == f.Shell:
		if f.LogonAt.IsZero() {
			return false, "запустил Проводник, момент входа неизвестен"
		}
		if since := f.At.Sub(f.LogonAt); since < manualLogonGrace {
			return false, fmt.Sprintf("запустил Проводник через %s после входа (автозагрузка)", since.Round(time.Millisecond))
		}
		return true, "запустил Проводник"
	case f.Foreground != "" && f.Parent == f.Foreground:
		return true, "запустила программа на переднем плане: " + f.Parent
	}
	return false, "запустила программа не на переднем плане: " + f.Parent
}

// launchParentOf — нормализованный путь родителя процесса p среди процессов сеанса by; «» —
// родитель не узнан. Номер родителя мог достаться другому процессу (PL-9): родитель должен быть
// создан не позже p. Родитель другого пользователя, в другом сеансе или с непрочитанными
// путём, пользователем или моментом создания — не узнан (PL-19).
func launchParentOf(p ProcInfo, by map[uint32]ProcInfo, sid string) string {
	if p.ParentPID == 0 {
		return ""
	}
	q, ok := by[p.ParentPID]
	if !ok || q.SessionID != p.SessionID || !q.PathOK || !q.UserOK || !strings.EqualFold(q.UserSID, sid) {
		return ""
	}
	if q.Stamp == 0 || p.Stamp == 0 || q.Stamp > p.Stamp {
		return ""
	}
	return programs.Normalize(q.Path)
}

// launchShell — нормализованный путь Проводника по папке Windows; «» — папка не определена.
func launchShell(systemRoot string) string {
	if systemRoot == "" {
		return ""
	}
	return programs.Normalize(strings.TrimSuffix(systemRoot, `\`) + `\explorer.exe`)
}

// launchStoreApp — путь path лежит в папке приложений Магазина Program Files\WindowsApps (строго
// глубже). Папка Program Files не определена — «не знаю», не Магазин.
func launchStoreApp(path, programFiles string) bool {
	if programFiles == "" {
		return false
	}
	root := programs.Normalize(strings.TrimSuffix(programFiles, `\`) + `\windowsapps`)
	p := programs.Normalize(path)
	return root != "" && strings.HasPrefix(p, root+`\`) && len(p) > len(root)+1
}

// launchBroker — нормализованный путь svchost.exe по папке Windows; «» — папка не определена.
func launchBroker(systemRoot string) string {
	if systemRoot == "" {
		return ""
	}
	return programs.Normalize(strings.TrimSuffix(systemRoot, `\`) + `\system32\svchost.exe`)
}
