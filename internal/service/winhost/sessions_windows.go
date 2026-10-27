//go:build windows

package winhost

import (
	"fmt"

	"golang.org/x/sys/windows"

	"pcblock/internal/service"
	"pcblock/internal/winsession"
)

var (
	wtsapi32                    = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSLogoffSessionDLL     = wtsapi32.NewProc("WTSLogoffSession")
	procWTSDisconnectSessionDLL = wtsapi32.NewProc("WTSDisconnectSession")
	user32                      = windows.NewLazySystemDLL("user32.dll")
	procExitWindowsExDLL        = user32.NewProc("ExitWindowsEx")
)

const (
	ewxPowerOff     = 0x00000008 // EWX_POWEROFF: выключить питание, если оборудование это умеет
	ewxForceIfHung  = 0x00000010 // EWX_FORCEIFHUNG: завершить только зависшие программы
	shutdownPlanned = 0x80000000 // SHTDN_REASON_FLAG_PLANNED с причиной «другое»
)

// launcher запускает страж, значок и окна в сеансах Windows (service.SessionLauncher над
// internal/winsession). Страж идёт от имени системы: обычный пользователь не может его
// завершить; значок и окна от имени пользователя сеанса.
type launcher struct {
	exe string
}

// Sessions перечисляет интерактивные сеансы с вошедшим пользователем, в том числе отключённые
// (смена пользователя), с признаком, на экране ли сеанс (FB-3): служебный сеанс 0 и экран входа
// без пользователя не в счёт.
func (l launcher) Sessions() ([]service.SessionInfo, error) {
	list, err := winsession.ActiveSessions()
	if err != nil {
		return nil, err
	}
	return sessionInfos(list), nil
}

// Launch запускает программу с ключами args в сеансе и возвращает ручку процесса.
func (l launcher) Launch(kind service.ProcKind, sessionID uint32, args []string) (service.Proc, error) {
	var (
		pid uint32
		err error
	)
	switch kind {
	case service.ProcGuard:
		pid, err = winsession.LaunchAsSystem(sessionID, l.exe, args)
	case service.ProcTray, service.ProcWindow:
		pid, err = winsession.LaunchAsUser(sessionID, l.exe, args)
	default:
		return nil, fmt.Errorf("неизвестный вид процесса %v", kind)
	}
	if err != nil {
		return nil, err
	}
	return openProc(pid), nil
}

// Logoff выводит пользователя из сеанса (WTSLogoffSession): из процесса системы
// ExitWindowsEx пользователя из его сеанса не выводит.
func (l launcher) Logoff(sessionID uint32) error {
	// 0 — WTS_CURRENT_SERVER_HANDLE; последний аргумент FALSE: не ждать завершения выхода.
	r, _, err := procWTSLogoffSessionDLL.Call(0, uintptr(sessionID), 0)
	if r == 0 {
		return fmt.Errorf("WTSLogoffSession: %w", err)
	}
	return nil
}

// Shutdown штатно выключает компьютер: программы получают просьбу закрыться (несохранённое
// предлагают сохранить), зависшие завершаются. Служба работает от имени системы: привилегия
// SeShutdownPrivilege в её токене есть, но выключена, и включается здесь.
func (l launcher) Shutdown() error {
	if err := enablePrivilege("SeShutdownPrivilege"); err != nil {
		return err
	}
	r, _, err := procExitWindowsExDLL.Call(uintptr(ewxPowerOff|ewxForceIfHung), uintptr(shutdownPlanned))
	if r == 0 {
		return fmt.Errorf("ExitWindowsEx: %w", err)
	}
	return nil
}

// SwitchUser отключает сеанс без завершения (WTSDisconnectSession): Windows показывает экран
// выбора пользователя, программы и несохранённые данные ребёнка остаются в сеансе.
func (l launcher) SwitchUser(sessionID uint32) error {
	// 0 — WTS_CURRENT_SERVER_HANDLE; последний аргумент FALSE: не ждать завершения отключения.
	r, _, err := procWTSDisconnectSessionDLL.Call(0, uintptr(sessionID), 0)
	if r == 0 {
		return fmt.Errorf("WTSDisconnectSession: %w", err)
	}
	return nil
}

// Stamp возвращает момент создания процесса: пара (номер, момент) не повторяется, в отличие
// от номера. Процесса нет или он уже завершён: ok = false.
func (l launcher) Stamp(pid uint32) (uint64, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return 0, false
	}
	if exited.HighDateTime != 0 || exited.LowDateTime != 0 {
		return 0, false
	}
	return uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), true
}

// KillStale завершает процесс, оставшийся от прошлого запуска службы, если это тот же
// процесс: совпали номер и момент создания. Другой процесс с тем же номером не трогается.
func (l launcher) KillStale(pid uint32, stamp uint64) bool {
	if got, ok := l.Stamp(pid); !ok || got != stamp {
		return false
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	if err := windows.TerminateProcess(h, 1); err != nil {
		return false
	}
	_, _ = windows.WaitForSingleObject(h, 3000)
	return true
}

var _ service.SessionLauncher = launcher{}
