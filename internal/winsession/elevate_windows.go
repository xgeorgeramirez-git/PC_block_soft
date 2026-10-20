//go:build windows

package winsession

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// ErrElevationDeclined — человек отказался в окне UAC (или не ввёл пароль администратора).
var ErrElevationDeclined = errors.New("Повышение прав отклонено: окно сброса пароля не открыто")

// IsElevatedAdmin отвечает, что процесс запущен с повышенными правами (UAC подтверждён) и его токен
// входит в Администраторы. Это то же условие, по которому служба пускает сброс пароля и проверку
// перед удалением (R20): без повышения группа Администраторов в токене «только для запрета»,
// и такой процесс считается обычным.
func IsElevatedAdmin() bool {
	tok := windows.GetCurrentProcessToken()
	if !tok.IsElevated() {
		return false
	}
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false
	}
	groups, err := tok.GetTokenGroups()
	if err != nil {
		return false
	}
	for _, g := range groups.AllGroups() {
		if g.Attributes&windows.SE_GROUP_ENABLED != 0 && g.Sid.Equals(admins) {
			return true
		}
	}
	return false
}

// RelaunchElevated запускает эту же программу ещё раз с ключами args через ShellExecuteW и глаголом
// runas: Windows показывает окно UAC. В детской учётке UAC просит пароль администратора, без него
// программа не запустится (FR-26). Функция возвращается, когда окно UAC закрыто; запущенный процесс
// она не ждёт. Отказ в окне UAC даёт ErrElevationDeclined.
func RelaunchElevated(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не найден путь к программе: %w", err)
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	var params *uint16
	if len(args) > 0 {
		if params, err = windows.UTF16PtrFromString(windows.ComposeCommandLine(args)); err != nil {
			return err
		}
	}
	err = windows.ShellExecute(0, verb, file, params, nil, windows.SW_SHOWNORMAL)
	if errors.Is(err, windows.ERROR_CANCELLED) {
		return ErrElevationDeclined
	}
	if err != nil {
		return fmt.Errorf("программа не запущена с повышением прав: %w", err)
	}
	return nil
}
