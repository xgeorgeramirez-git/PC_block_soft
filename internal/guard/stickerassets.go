package guard

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"
)

// Кадры стикеров промокода для экрана блокировки (RM5b-9, R26): спрайт PNG на стикер,
// кадры слева направо, сторона кадра StickerFramePx, палитра 256 цветов с прозрачностью.
// Файлы делает tools/sticker-frames.py из assets/stickers/<n>.tgs и коммитит разработчик;
// число кадров каждого стикера лежит в frames.json. Здесь только встраивание и разбор,
// показ (масштаб под dpi, таймер) - в коде экрана блокировки.

//go:embed assets/stickers/*.png assets/stickers/frames.json
var stickerFS embed.FS

// StickerFramePx - сторона кадра спрайта (при 150 %; меньше - усреднением).
const StickerFramePx = 300

// StickerFPS - кадров в секунду в спрайтах (каждый 5-й кадр исходных 60):
// при 15 кадрах/с 17 стикеров не помещались в бюджет 4 МБ (4,17 МБ, Ruling T7).
const StickerFPS = 12

var (
	stickerCountsOnce sync.Once
	stickerCounts     map[string]int
	stickerCountsErr  error
)

// stickerFrameCounts - число кадров каждого встроенного стикера (разбор один раз).
func stickerFrameCounts() (map[string]int, error) {
	stickerCountsOnce.Do(func() {
		data, err := stickerFS.ReadFile("assets/stickers/frames.json")
		if err != nil {
			stickerCountsErr = err
			return
		}
		stickerCounts, stickerCountsErr = parseStickerFrames(data)
	})
	return stickerCounts, stickerCountsErr
}

// parseStickerFrames разбирает frames.json: {"017": 33, ...}; номер - три цифры, кадров > 0.
func parseStickerFrames(data []byte) (map[string]int, error) {
	var m map[string]int
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	for n, c := range m {
		if !validStickerNumber(n) {
			return nil, fmt.Errorf("стикер %q: номер не из трёх цифр", n)
		}
		if c <= 0 {
			return nil, fmt.Errorf("стикер %s: кадров %d", n, c)
		}
	}
	return m, nil
}

func validStickerNumber(n string) bool {
	if len(n) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		if n[i] < '0' || n[i] > '9' {
			return false
		}
	}
	return true
}

// stickerSprite - встроенный PNG-спрайт стикера n.
func stickerSprite(n string) ([]byte, error) {
	if !validStickerNumber(n) {
		return nil, fmt.Errorf("стикер %q: номер не из трёх цифр", n)
	}
	return stickerFS.ReadFile("assets/stickers/" + n + ".png")
}
