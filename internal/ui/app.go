// Package ui — окно программы на Wails: запуск окна и методы, доступные интерфейсу.
// Логики правил здесь нет: только связка окна с ядром.
package ui

import (
	"context"
	"io/fs"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"pcblock/internal/version"
)

// App — методы, которые вызывает интерфейс окна (биндинги Wails: frontend/wailsjs/go/ui/App).
type App struct {
	ctx context.Context
}

// NewApp создаёт связку окна.
func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Version возвращает номер версии программы для показа в окне.
func (a *App) Version() string {
	return version.Version
}

// Run открывает окно программы и ждёт его закрытия.
func Run(assets fs.FS) error {
	app := NewApp()
	return wails.Run(&options.App{
		Title:            "PC Block",
		Width:            960,
		Height:           640,
		MinWidth:         720,
		MinHeight:        480,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 243, G: 244, B: 246, A: 255},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
	})
}
