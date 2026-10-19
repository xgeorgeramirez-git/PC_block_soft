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
	"pcblock/internal/ui"
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
		// Временная замена systray.Run (T11): процесс только присутствует в сеансе.
		return exitCode(winhost.Hold("tray", opts))
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

// exitCode превращает итог служебного режима в код выхода: 0 или 1.
func exitCode(err error) int {
	if err != nil {
		log.Print(err)
		return 1
	}
	return 0
}
