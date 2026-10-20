// Package ui — окно программы на Wails: запуск окна и методы, доступные интерфейсу.
// Логики правил здесь нет: только связка окна со службой через канал связи (Ruling 14).
package ui

import (
	"context"
	"encoding/json"
	"io/fs"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"pcblock/internal/ipc"
	"pcblock/internal/launch"
	"pcblock/internal/service"
	"pcblock/internal/version"
)

// App — методы, которые вызывает интерфейс окна (биндинги Wails: frontend/wailsjs/go/ui/App).
// Ровно четыре открытых метода: Call, Finish, Mode, Version. Любой новый открытый метод
// попадает в биндинги, а windows.yml сверяет их с закоммиченными, поэтому вспомогательные
// методы пишутся с маленькой буквы.
type App struct {
	mode   launch.Mode
	opts   launch.Options
	client *service.WindowClient

	mu       sync.Mutex
	ctx      context.Context
	exitCode int
}

// NewApp создаёт связку окна для режима запуска. Канал связи при этом не открывается:
// клиент подключается при первом Call (Ruling 22: запуск без ключа не трогает службу, его
// выполняет и генератор биндингов Wails).
func NewApp(mode launch.Mode, opts launch.Options) *App {
	return &App{
		mode:     mode,
		opts:     opts,
		client:   service.NewWindowClient(dialService, opts["handover"]),
		exitCode: defaultExitCode(mode),
	}
}

// defaultExitCode — код выхода, если окно закрыли крестиком: у сброса пароля и вопроса перед
// удалением это отмена (1), у остальных окон обычное закрытие (0).
func defaultExitCode(mode launch.Mode) int {
	switch mode {
	case launch.ModeResetPassword, launch.ModeUninstallGate:
		return 1
	}
	return 0
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
}

// shutdown сообщает службе о штатном закрытии окна: это не обход защиты (FR-27), и родительская
// сессия, открытая этим окном, заканчивается. Службы нет: ничего страшного.
func (a *App) shutdown(_ context.Context) {
	if kind, ok := windowKind(a.mode); ok {
		if body, err := json.Marshal(ipc.WindowClosingRequest{Kind: kind}); err == nil {
			_, _ = a.client.Call(ipc.CmdWindowClosing, string(body))
		}
	}
	a.client.Close()
}

// windowKind — вид окна для службы (OpenWindow, WindowClosing); у остальных режимов его нет.
func windowKind(mode launch.Mode) (string, bool) {
	switch mode {
	case launch.ModeParent:
		return ipc.WindowParent, true
	case launch.ModeMyStats:
		return ipc.WindowMyStats, true
	case launch.ModeFirstRun:
		return ipc.WindowFirstRun, true
	}
	return "", false
}

// Call отправляет команду службе и возвращает ответ канала связи в JSON как есть. Ошибка
// возвращается, только если связи со службой нет: текст готов для показа человеку.
func (a *App) Call(command string, payload string) (string, error) {
	reply, err := a.client.Call(command, payload)
	if err == nil {
		a.noteReply(command, reply)
	}
	return reply, err
}

// noteReply запоминает итог команды, от которого зависит код выхода окна. Сброс пароля удался:
// окно остаётся на экране с надписью «Пароль изменён», а код выхода уже 0, чем бы его ни закрыли
// (FR-26). Остальные команды кода выхода не меняют.
func (a *App) noteReply(command, reply string) {
	if a.mode != launch.ModeResetPassword || command != ipc.CmdResetPassword {
		return
	}
	var rep ipc.Reply
	if json.Unmarshal([]byte(reply), &rep) == nil && rep.OK {
		a.setExitCode(0)
	}
}

func (a *App) setExitCode(code int) {
	a.mu.Lock()
	a.exitCode = code
	a.mu.Unlock()
}

// Finish закрывает окно, и программа завершается с этим кодом. Окно удаления: 0 удалить и
// оставить статистику, 2 удалить вместе со статистикой, 1 отмена. Окно сброса пароля: 0
// пароль изменён, 1 отмена.
func (a *App) Finish(code int) {
	if code < 0 || code > 255 {
		code = 1
	}
	a.setExitCode(code)
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		runtime.Quit(ctx)
	}
}

// Mode возвращает режим запуска окна: window, parent, mystats, firstrun, wizard,
// reset-password или uninstall-gate. По нему интерфейс выбирает экран.
func (a *App) Mode() string {
	return a.mode.String()
}

// Version возвращает номер версии программы для показа в окне.
func (a *App) Version() string {
	return version.Version
}

func (a *App) code() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.exitCode
}

// windowSpec — заголовок и размер окна режима. Окна с вопросом (сброс пароля, удаление)
// фиксированного размера.
type windowSpec struct {
	title               string
	width, height       int
	minWidth, minHeight int
	fixed               bool
}

func specFor(mode launch.Mode) windowSpec {
	switch mode {
	case launch.ModeParent:
		return windowSpec{title: "PC Block: настройки для родителя", width: 1040, height: 700, minWidth: 800, minHeight: 560}
	case launch.ModeMyStats:
		return windowSpec{title: "Моя статистика", width: 800, height: 640, minWidth: 640, minHeight: 520}
	case launch.ModeFirstRun:
		return windowSpec{title: "Добро пожаловать в PC Block", width: 700, height: 600, minWidth: 560, minHeight: 480}
	case launch.ModeWizard:
		return windowSpec{title: "PC Block: первая настройка", width: 880, height: 680, minWidth: 720, minHeight: 560}
	case launch.ModeResetPassword:
		return windowSpec{title: "PC Block: сброс пароля родителя", width: 560, height: 520, minWidth: 560, minHeight: 520, fixed: true}
	case launch.ModeUninstallGate:
		return windowSpec{title: "PC Block: удаление программы", width: 560, height: 520, minWidth: 560, minHeight: 520, fixed: true}
	}
	return windowSpec{title: "PC Block", width: 960, height: 640, minWidth: 720, minHeight: 480}
}

// Run открывает окно программы в режиме mode и ждёт его закрытия. Возвращает код выхода
// программы: его задал Finish, а без него действует код закрытия окна крестиком.
func Run(assets fs.FS, mode launch.Mode, opts launch.Options) (int, error) {
	app := NewApp(mode, opts)
	sp := specFor(mode)
	err := wails.Run(&options.App{
		Title:            sp.title,
		Width:            sp.width,
		Height:           sp.height,
		MinWidth:         sp.minWidth,
		MinHeight:        sp.minHeight,
		DisableResize:    sp.fixed,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 243, G: 244, B: 246, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
	})
	return app.code(), err
}
