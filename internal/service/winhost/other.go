//go:build !windows

package winhost

import (
	"errors"

	"pcblock/internal/launch"
)

// errOnlyWindows — служба и установка работают только в Windows; на других системах пакет
// собирается, чтобы go vet и тесты остальных пакетов не падали.
var errOnlyWindows = errors.New("Служба PC Block работает только в Windows")

// RunService запускает программу как службу Windows.
func RunService() error { return errOnlyWindows }

// Install ставит и запускает службу (--install-service).
func Install(opts launch.Options) error { return errOnlyWindows }

// Remove останавливает и удаляет службу (--remove-service).
func Remove(opts launch.Options) error { return errOnlyWindows }

// TestSetup делает первичную настройку для приёмки (--test-setup).
func TestSetup(opts launch.Options) error { return errOnlyWindows }

// TestIPC отправляет один запрос в канал связи и пишет ответ (--test-ipc).
func TestIPC(opts launch.Options) error { return errOnlyWindows }

// TestRunAs запускает программу от имени другой учётки (--test-runas).
func TestRunAs(opts launch.Options) error { return errOnlyWindows }

// UninstallGate решает вопрос об удалении без окна (--uninstall-gate); вне Windows отказ.
func UninstallGate(opts launch.Options) (int, bool) { return 1, true }

// ResetPasswordNoWindow сбрасывает пароль без окна (--reset-password --new-password=…).
func ResetPasswordNoWindow(opts launch.Options) error { return errOnlyWindows }
