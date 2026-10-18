//go:build windows

package ipc

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32                       = windows.NewLazySystemDLL("advapi32.dll")
	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procImpersonateNamedPipeClient = advapi32.NewProc("ImpersonateNamedPipeClient")
	procWaitNamedPipe              = kernel32.NewProc("WaitNamedPipeW")
)

// identifyPeer узнаёт, кто подключился к ручке h (Ruling 17, NFR-9): PID и сеанс из
// GetNamedPipeClientProcessId, учётку, повышение и группы из токена клиента. Вызывать после
// того, как от клиента прочитан хотя бы один кадр: раньше ImpersonateNamedPipeClient не работает.
//
// Подмена клиента живёт только в отдельной горутине, привязанной к потоку. Если снять её не
// удалось, горутина завершается, не отвязываясь: Go уничтожает поток вместе с подменой.
func identifyPeer(h windows.Handle) (Peer, error) {
	type result struct {
		p   Peer
		err error
	}
	ch := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		p, reverted, err := identifyLocked(h)
		if reverted {
			runtime.UnlockOSThread()
		}
		ch <- result{p, err}
	}()
	r := <-ch
	return r.p, r.err
}

// identifyLocked работает на привязанном потоке. reverted = false, если подмена осталась.
func identifyLocked(h windows.Handle) (p Peer, reverted bool, err error) {
	reverted = true
	var pid uint32
	if err = windows.GetNamedPipeClientProcessId(h, &pid); err != nil {
		return
	}
	p.PID = pid
	if err = windows.ProcessIdToSessionId(pid, &p.SessionID); err != nil {
		return
	}
	if r, _, e := procImpersonateNamedPipeClient.Call(uintptr(h)); r == 0 {
		err = e
		return
	}
	reverted = false
	defer func() {
		if rerr := windows.RevertToSelf(); rerr != nil {
			if err == nil {
				err = rerr
			}
			return
		}
		reverted = true
	}()

	var tok windows.Token
	if err = windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &tok); err != nil {
		return
	}
	defer tok.Close()
	user, uerr := tok.GetTokenUser()
	if uerr != nil {
		err = uerr
		return
	}
	p.SID = user.User.Sid.String()
	p.System = user.User.Sid.IsWellKnown(windows.WinLocalSystemSid)
	p.Elevated = tok.IsElevated()
	p.Admin, err = inAdministrators(tok)
	return
}

// inAdministrators сообщает, входит ли учётка токена в Администраторы. Токен без повышения
// (UAC) держит эту группу «только для запрета»: она есть в списке, но не включена. Это тоже
// администратор, только без повышения: команды настроек ему не положены, а подсказать
// «запустите от имени администратора» можно (Review Focus 7).
func inAdministrators(tok windows.Token) (bool, error) {
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false, err
	}
	groups, err := tok.GetTokenGroups()
	if err != nil {
		return false, err
	}
	for _, g := range groups.AllGroups() {
		if g.Attributes&(windows.SE_GROUP_ENABLED|windows.SE_GROUP_USE_FOR_DENY_ONLY) != 0 && g.Sid.Equals(admins) {
			return true, nil
		}
	}
	return false, nil
}

// Проверки владельца канала подменяются в тестах.
var (
	processOwnerIsSystem = processRunsAsSystem
	serviceProcessID     = servicePID
)

// verifySystemServer — клиент до отправки любых данных проверяет, что канал держит процесс
// системы (Ruling 17): иначе процесс ребёнка мог бы занять имя канала, пока служба
// перезапускается, и получить пароль родителя из окна.
//
// Основной путь: владелец токена процесса, который создал канал (GetNamedPipeServerProcessId),
// должен быть S-1-5-18. Токен чужого процесса обычный пользователь прочитать не может
// (ERROR_ACCESS_DENIED), поэтому тогда проверяется, что этот процесс и есть служба
// serviceName: её PID спрашивается у менеджера служб, он доступен всем интерактивным
// пользователям. Любая другая неудача проверки — отказ.
func verifySystemServer(h windows.Handle, serviceName string) error {
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(h, &pid); err != nil {
		return newError(ErrNotService, err)
	}
	system, err := processOwnerIsSystem(pid)
	if err == nil {
		if system {
			return nil
		}
		return newError(ErrNotService, errors.New("ipc: канал держит не процесс системы"))
	}
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) && serviceName != "" {
		svcPID, serr := serviceProcessID(serviceName)
		if serr != nil {
			return newError(ErrNotService, serr)
		}
		if svcPID != 0 && svcPID == pid {
			return nil
		}
		return newError(ErrNotService, errors.New("ipc: канал держит не служба программы"))
	}
	return newError(ErrNotService, err)
}

// processRunsAsSystem сообщает, работает ли процесс pid от имени системы (S-1-5-18).
func processRunsAsSystem(pid uint32) (bool, error) {
	ph, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(ph)
	var tok windows.Token
	if err := windows.OpenProcessToken(ph, windows.TOKEN_QUERY, &tok); err != nil {
		return false, err
	}
	defer tok.Close()
	user, err := tok.GetTokenUser()
	if err != nil {
		return false, err
	}
	return user.User.Sid.IsWellKnown(windows.WinLocalSystemSid), nil
}

// servicePID возвращает PID процесса службы name по данным менеджера служб (0, если служба не запущена).
func servicePID(name string) (uint32, error) {
	namep, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0, err
	}
	defer windows.CloseServiceHandle(m)
	s, err := windows.OpenService(m, namep, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return 0, err
	}
	defer windows.CloseServiceHandle(s)
	var st windows.SERVICE_STATUS_PROCESS
	var needed uint32
	if err := windows.QueryServiceStatusEx(s, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&st)), uint32(unsafe.Sizeof(st)), &needed); err != nil {
		return 0, err
	}
	return st.ProcessId, nil
}
