//go:build windows

// Команда lockspike — проба неубиваемого экрана блокировки (ТЗ 3.2, веха M1,
// задача T1). Удаляется после Ruling. Два режима:
//
//	-service  минимальная служба LocalSystem: запускает себя с -guard в сеансе
//	          пользователя от имени системы и проверяет, что обычная учётка
//	          pcbtest не может завершить процесс стража.
//	-guard    показывает экран блокировки, ставит перехват клавиатуры, через 3 с
//	          выполняет самопроверку (окно не закрывается штатными сообщениями).
//
// Итоги пишутся в C:\ProgramData\PCBlockSpike\spike.json и guard.json; их читает
// шаг spike.yml и печатает фактами.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"

	"pcblock/internal/guard"
	"pcblock/internal/winsession"
)

const dataDir = `C:\ProgramData\PCBlockSpike`

var (
	advapi32        = windows.NewLazySystemDLL("advapi32.dll")
	procLogonUser   = advapi32.NewProc("LogonUserW")
	procImpersonate = advapi32.NewProc("ImpersonateLoggedOnUser")
)

const (
	logon32Interactive = 2
	logon32Network     = 3
	logon32ProvDefault = 0
	errLogonTypeDenied = 1385
)

var (
	svcMode     = flag.Bool("service", false, "запуск как служба")
	guardMode   = flag.Bool("guard", false, "показать экран блокировки")
	sessionFlag = flag.Int("session", -1, "номер сеанса пользователя")
	pwFlag      = flag.String("testuser-password", "", "пароль учётки pcbtest")
)

// spikeResult — итог стороны службы (spike.json).
type spikeResult struct {
	ServicePID       uint32
	GuardPID         uint32
	SessionID        uint32
	SessionSource    string
	LogonType        string
	Impersonated     bool
	OpenProcessOK    bool   // true = удалось открыть с PROCESS_TERMINATE (защита не сработала)
	OpenProcessErrno uint32 // ожидается 5 (ERROR_ACCESS_DENIED)
	OpenProcessErr   string
	Note             string
}

// guardResult — итог стороны стража (guard.json).
type guardResult struct {
	GuardPID     uint32
	SessionID    uint32
	Owner        string
	KeyboardHook bool
	IsForeground bool
	SelfTest     guard.SelfTestResult
	Note         string
}

func main() {
	flag.Parse()
	switch {
	case *guardMode:
		sess := uint32(0)
		if *sessionFlag >= 0 {
			sess = uint32(*sessionFlag)
		}
		runGuard(sess)
	case *svcMode:
		runService()
	default:
		fmt.Println("PCBlockSpike: укажите -service или -guard")
	}
}

func runService() {
	sess, src := resolveSession(*sessionFlag)
	exe, err := os.Executable()
	if err != nil {
		writeResult("spike.json", spikeResult{Note: "os.Executable: " + err.Error()})
		return
	}
	h := &svcHandler{session: sess, sessionSrc: src, password: *pwFlag, exe: exe}
	_ = svc.Run("PCBlockSpike", h)
}

type svcHandler struct {
	session    uint32
	sessionSrc string
	password   string
	exe        string
}

func (h *svcHandler) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown
	s <- svc.Status{State: svc.StartPending}
	go h.work()
	s <- svc.Status{State: svc.Running, Accepts: accepts}
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			s <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			s <- svc.Status{State: svc.StopPending}
			return false, 0
		default:
		}
	}
	return false, 0
}

func (h *svcHandler) work() {
	res := spikeResult{
		ServicePID:    uint32(os.Getpid()),
		SessionID:     h.session,
		SessionSource: h.sessionSrc,
	}
	pid, err := winsession.LaunchAsSystem(h.session, h.exe,
		[]string{"-guard", "-session", fmt.Sprint(h.session)})
	if err != nil {
		res.Note = "LaunchAsSystem: " + err.Error()
		writeResult("spike.json", res)
		return
	}
	res.GuardPID = pid
	time.Sleep(2 * time.Second) // дать окну стража подняться

	lt, imp, openOK, errno, estr := checkCannotTerminate(h.password, pid)
	res.LogonType = lt
	res.Impersonated = imp
	res.OpenProcessOK = openOK
	res.OpenProcessErrno = errno
	res.OpenProcessErr = estr
	writeResult("spike.json", res)
}

// checkCannotTerminate входит под обычной учёткой pcbtest, подменяет ею текущий
// поток и пытается открыть процесс стража с правом PROCESS_TERMINATE. Ожидается
// отказ в доступе (процесс стража работает от имени системы).
func checkCannotTerminate(password string, guardPID uint32) (logonType string, impersonated, openOK bool, errno uint32, errStr string) {
	user, _ := windows.UTF16PtrFromString("pcbtest")
	dot, _ := windows.UTF16PtrFromString(".")
	pw, _ := windows.UTF16PtrFromString(password)

	var tok windows.Handle
	ok, _, e := procLogonUser.Call(
		uintptr(unsafe.Pointer(user)), uintptr(unsafe.Pointer(dot)), uintptr(unsafe.Pointer(pw)),
		logon32Interactive, logon32ProvDefault, uintptr(unsafe.Pointer(&tok)))
	if ok != 0 {
		logonType = "interactive"
	} else if en, isErrno := e.(syscall.Errno); isErrno && en == errLogonTypeDenied {
		ok2, _, e2 := procLogonUser.Call(
			uintptr(unsafe.Pointer(user)), uintptr(unsafe.Pointer(dot)), uintptr(unsafe.Pointer(pw)),
			logon32Network, logon32ProvDefault, uintptr(unsafe.Pointer(&tok)))
		if ok2 == 0 {
			return "failed:" + e2.Error(), false, false, 0, ""
		}
		logonType = "network"
	} else {
		return "failed:" + e.Error(), false, false, 0, ""
	}
	defer windows.CloseHandle(tok)

	// Подмена учётки действует на поток ОС: закрепляем поток.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if ir, _, _ := procImpersonate.Call(uintptr(tok)); ir != 0 {
		impersonated = true
	}
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, guardPID)
	if err == nil {
		openOK = true
		windows.CloseHandle(h)
	} else if en, isErrno := err.(syscall.Errno); isErrno {
		errno = uint32(en)
		errStr = err.Error()
	} else {
		errStr = err.Error()
	}
	windows.RevertToSelf()
	return logonType, impersonated, openOK, errno, errStr
}

func runGuard(session uint32) {
	runtime.LockOSThread() // окно и цикл сообщений и хук — один поток ОС
	view := guard.LockView{
		Reason: "Время на сегодня закончилось",
		Until:  "Можно снова: завтра",
		Used:   "Сегодня: 2 ч 00 мин из 2 ч",
	}
	w, err := guard.NewLockWindow(view)
	if err != nil {
		writeResult("guard.json", guardResult{SessionID: session, Note: "NewLockWindow: " + err.Error()})
		return
	}
	hookErr := w.InstallKeyboardGuard()

	go func() {
		time.Sleep(3 * time.Second)
		writeResult("guard.json", guardResult{
			GuardPID:     uint32(os.Getpid()),
			SessionID:    session,
			Owner:        currentUser(),
			KeyboardHook: w.KeyboardGuardInstalled() && hookErr == nil,
			IsForeground: w.IsForeground(),
			SelfTest:     w.SelfTest(),
		})
	}()
	w.Run()
}

func resolveSession(flagVal int) (uint32, string) {
	if flagVal >= 0 {
		return uint32(flagVal), "-session"
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		id := windows.WTSGetActiveConsoleSessionId()
		if id != 0xFFFFFFFF {
			return id, "WTSGetActiveConsoleSessionId"
		}
		if time.Now().After(deadline) {
			return 0, "нет активной консоли за 30 с"
		}
		time.Sleep(time.Second)
	}
}

func currentUser() string {
	tok := windows.GetCurrentProcessToken()
	tu, err := tok.GetTokenUser()
	if err != nil {
		return ""
	}
	acc, dom, _, err := tu.User.Sid.LookupAccount("")
	if err != nil {
		return tu.User.Sid.String()
	}
	return dom + "\\" + acc
}

func writeResult(name string, v any) {
	_ = os.MkdirAll(dataDir, 0o755)
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dataDir, name), b, 0o644)
}
