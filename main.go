// PC Block — родительский контроль для Windows 11 (ТЗ: docs/01_ТЗ.md).
//
// Программа одна, режим выбирает ключ запуска (Ruling 2 плана M1, internal/launch): служба,
// установка и удаление службы, страж сеанса, значок, окна Wails, служебные ключи для приёмки.
package main

import (
	"embed"
	"io/fs"
	"log"
	"os"

	"pcblock/internal/guard"
	"pcblock/internal/launch"
	"pcblock/internal/service/winhost"
	"pcblock/internal/systray"
	"pcblock/internal/ui"
	"pcblock/internal/winsession"
)

//go:embed all:frontend/dist
var embedded embed.FS

func main() {
	mode, opts, err := launch.Parse(os.Args[1:])
	if err != nil {
		log.Print(err)
		os.Exit(2)
	}
	os.Exit(run(mode, opts))
}

// run выполняет режим и возвращает код выхода программы.
func run(mode launch.Mode, opts launch.Options) int {
	switch mode {
	case launch.ModeService:
		return exitCode(winhost.RunService())
	case launch.ModeInstallService:
		return exitCode(winhost.Install(opts))
	case launch.ModeRemoveService:
		return exitCode(winhost.Remove(opts))
	case launch.ModeTestSetup:
		return exitCode(winhost.TestSetup(opts))
	case launch.ModeTestIPC:
		return exitCode(winhost.TestIPC(opts))
	case launch.ModeTestRunAs:
		return exitCode(winhost.TestRunAs(opts))
	case launch.ModeGuard:
		return exitCode(guard.Run(opts))
	case launch.ModeTray:
		return exitCode(systray.Run(opts))
	case launch.ModeResetPassword:
		// С ключом --new-password окна нет: так сброс проверяет CI (S25), только в тестовом режиме.
		if _, headless := opts["new-password"]; headless {
			return exitCode(winhost.ResetPasswordNoWindow(opts))
		}
		if code, done := relaunchElevated(opts); done {
			return code
		}
	case launch.ModeUninstallGate:
		// Службы нет или тихий режим: окно не нужно, ответ готов (код 0, 1 или 2 для установщика).
		if code, done := winhost.UninstallGate(opts); done {
			return code
		}
	}

	assets, err := fs.Sub(embedded, "frontend/dist")
	if err != nil {
		log.Printf("не найдены файлы интерфейса: %v", err)
		return 1
	}
	code, err := ui.Run(assets, mode, opts)
	if err != nil {
		log.Printf("окно программы не запустилось: %v", err)
		return 1
	}
	return code
}

// relaunchElevated: сброс пароля доступен только администратору с повышенными правами (R20, FR-26).
// Запущенная из меню «Пуск» программа без повышения перезапускает себя через окно UAC и завершается,
// окно откроется уже в новом процессе. Метка --elevated не даёт просить UAC второй раз.
// Возвращает код выхода и признак «этот процесс окно не открывает».
func relaunchElevated(opts launch.Options) (code int, done bool) {
	if opts.Flag("elevated") || winsession.IsElevatedAdmin() {
		return 0, false
	}
	args := append(append([]string{}, os.Args[1:]...), "--elevated")
	if err := winsession.RelaunchElevated(args); err != nil {
		log.Print(err)
		return 1, true
	}
	return 0, true
}

// exitCode превращает итог служебного режима в код выхода: 0 или 1.
func exitCode(err error) int {
	if err != nil {
		log.Print(err)
		return 1
	}
	return 0
}
