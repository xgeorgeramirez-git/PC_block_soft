//go:build windows

package winhost

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// enablePrivilege включает привилегию в токене своего процесса (она у процесса есть, но
// выключена: так выдаются SeDebugPrivilege администратору и SeBackup/SeRestore системе).
// Если привилегии в токене нет, ошибки нет, а нужное действие потом откажет само.
func enablePrivilege(name string) error {
	var tok windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok)
	if err != nil {
		return fmt.Errorf("токен процесса: %w", err)
	}
	defer tok.Close()
	namep, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, namep, &luid); err != nil {
		return fmt.Errorf("привилегия %s: %w", name, err)
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	if err := windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil); err != nil {
		return fmt.Errorf("привилегия %s не включена: %w", name, err)
	}
	return nil
}
