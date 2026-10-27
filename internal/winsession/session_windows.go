//go:build windows

package winsession

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Session описывает интерактивный сеанс Windows (номер, пользователь, SID, активность).
type Session struct {
	ID     uint32
	User   string
	SID    string
	Active bool
	State  uint32 // состояние WTS: 0 активен, 4 отключён
}

// ActiveSessions перечисляет сеансы терминальной службы (WTSEnumerateSessions)
// и возвращает только активные (WTSActive). Учётка и SID сеанса берутся из
// токена пользователя (WTSQueryUserToken), доступного службе от имени системы.
func ActiveSessions() ([]Session, error) {
	var p *windows.WTS_SESSION_INFO
	var count uint32
	// 0 = WTS_CURRENT_SERVER_HANDLE, версия структуры = 1.
	if err := windows.WTSEnumerateSessions(0, 0, 1, &p, &count); err != nil {
		return nil, fmt.Errorf("WTSEnumerateSessions: %w", err)
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(p)))

	size := unsafe.Sizeof(windows.WTS_SESSION_INFO{})
	out := make([]Session, 0, count)
	for i := uint32(0); i < count; i++ {
		si := (*windows.WTS_SESSION_INFO)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + uintptr(i)*size))
		s := Session{ID: si.SessionID, Active: si.State == windows.WTSActive, State: uint32(si.State)}
		if user, sid, err := sessionUser(si.SessionID); err == nil {
			s.User, s.SID = user, sid
		}
		out = append(out, s)
	}
	return out, nil
}

func sessionUser(session uint32) (user, sid string, err error) {
	var token windows.Token
	if err = windows.WTSQueryUserToken(session, &token); err != nil {
		return "", "", err
	}
	defer token.Close()
	tu, err := token.GetTokenUser()
	if err != nil {
		return "", "", err
	}
	account, domain, _, err := tu.User.Sid.LookupAccount("")
	if err != nil {
		return "", "", err
	}
	return domain + "\\" + account, tu.User.Sid.String(), nil
}

// LaunchAsSystem запускает exe в сеансе sessionID от имени системы: копирует
// первичный токен своего процесса (служба работает как LocalSystem), переносит
// его в нужный сеанс (TokenSessionId) и создаёт процесс на рабочем столе
// winsta0\default. Такой процесс обычный пользователь завершить не может.
func LaunchAsSystem(sessionID uint32, exe string, args []string) (uint32, error) {
	access := uint32(windows.TOKEN_DUPLICATE | windows.TOKEN_QUERY |
		windows.TOKEN_ASSIGN_PRIMARY | windows.TOKEN_ADJUST_DEFAULT | windows.TOKEN_ADJUST_SESSIONID)
	var self windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), access, &self); err != nil {
		return 0, fmt.Errorf("OpenProcessToken: %w", err)
	}
	defer self.Close()

	var dup windows.Token
	if err := windows.DuplicateTokenEx(self, windows.MAXIMUM_ALLOWED, nil,
		windows.SecurityImpersonation, windows.TokenPrimary, &dup); err != nil {
		return 0, fmt.Errorf("DuplicateTokenEx: %w", err)
	}
	defer dup.Close()

	s := sessionID
	if err := windows.SetTokenInformation(dup, windows.TokenSessionId,
		(*byte)(unsafe.Pointer(&s)), uint32(unsafe.Sizeof(s))); err != nil {
		return 0, fmt.Errorf("SetTokenInformation(TokenSessionId): %w", err)
	}
	return startProcess(dup, exe, args, false)
}

// LaunchAsUser запускает exe в сеансе sessionID от имени пользователя сеанса
// (токен сеанса из WTSQueryUserToken, блок окружения пользователя).
func LaunchAsUser(sessionID uint32, exe string, args []string) (uint32, error) {
	var token windows.Token
	if err := windows.WTSQueryUserToken(sessionID, &token); err != nil {
		return 0, fmt.Errorf("WTSQueryUserToken: %w", err)
	}
	defer token.Close()
	return startProcess(token, exe, args, true)
}

func startProcess(token windows.Token, exe string, args []string, withEnv bool) (uint32, error) {
	cmdPtr, err := windows.UTF16PtrFromString(buildCmdLine(exe, args))
	if err != nil {
		return 0, err
	}
	desktop, err := windows.UTF16PtrFromString(`winsta0\default`)
	if err != nil {
		return 0, err
	}

	flags := uint32(windows.CREATE_NO_WINDOW)
	var env *uint16
	if withEnv {
		var block *uint16
		if err := windows.CreateEnvironmentBlock(&block, token, false); err != nil {
			return 0, fmt.Errorf("CreateEnvironmentBlock: %w", err)
		}
		defer windows.DestroyEnvironmentBlock(block)
		env = block
		flags |= windows.CREATE_UNICODE_ENVIRONMENT
	}

	si := &windows.StartupInfo{}
	si.Cb = uint32(unsafe.Sizeof(*si))
	si.Desktop = desktop
	var pi windows.ProcessInformation
	if err := windows.CreateProcessAsUser(token, nil, cmdPtr, nil, nil, false,
		flags, env, nil, si, &pi); err != nil {
		return 0, fmt.Errorf("CreateProcessAsUser: %w", err)
	}
	windows.CloseHandle(pi.Thread)
	windows.CloseHandle(pi.Process)
	return pi.ProcessId, nil
}

func buildCmdLine(exe string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteArg(exe))
	for _, a := range args {
		parts = append(parts, quoteArg(a))
	}
	return strings.Join(parts, " ")
}

func quoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
