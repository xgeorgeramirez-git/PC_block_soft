// PC Block — родительский контроль для Windows 11 (ТЗ: docs/01_ТЗ.md).
package main

import (
	"embed"
	"io/fs"
	"log"

	"pcblock/internal/ui"
)

//go:embed all:frontend/dist
var embedded embed.FS

func main() {
	assets, err := fs.Sub(embedded, "frontend/dist")
	if err != nil {
		log.Fatalf("не найдены файлы интерфейса: %v", err)
	}
	if err := ui.Run(assets); err != nil {
		log.Fatalf("окно программы не запустилось: %v", err)
	}
}
