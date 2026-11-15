package guard

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math/rand/v2"
	"strings"
	"sync"
	"time"
)

// Ответ ребёнку на промокод на экране блокировки (R26, D-83): стикер заказчика и надпись.
// Наборы и надписи — те же, что в окне ребёнка (frontend/src/lib/stickers.ts; тест сверяет
// таблицы). Выбор и кадры — здесь, чистые функции; окно Windows только кладёт пиксели (PL-23).

// Стикеры удачи (верный действующий код) и неудачи (неверный, несуществующий, использованный).
var (
	stickerSuccess = []string{"017", "033", "044", "049", "050", "077", "093", "096", "112"}
	stickerFail    = []string{"023", "028", "048", "069", "082", "084", "095", "110"}
)

// Надписи удачи; {m} — минуты кода со словом «минута» в нужной форме («30 минут»).
var stickerSuccessTexts = []string{
	"Ура! +{m}",
	"Есть! Тебе +{m}",
	"Код сработал! +{m}",
	"Отлично, лови +{m}!",
	"Вот это да! +{m}",
	"Супер! Твои +{m}",
}

// Надписи неудачи.
var stickerFailTexts = []string{
	"Такого кода нет. Проверь буквы и цифры",
	"Хм, код не подходит. Посмотри внимательнее",
	"Не получилось. Проверь, нет ли опечатки",
	"Этот код не работает. Попробуй ещё раз",
	"Ой, мимо! Сверь код с запиской",
	"Код не сработал. Может, его уже использовали?",
}

// StickerPxAt100 — сторона стикера на экране при 100 % (96 dpi); больше кадра спрайта не бывает.
const StickerPxAt100 = 200

// StickerPx — сторона стикера при dpi: 200 px при 100 %, 250 при 125 %, 300 при 150 % и выше
// (кадры нарисованы на 300, увеличение размыло бы их).
func StickerPx(dpi int) int32 { return min(Scaled(StickerPxAt100, dpi), StickerFramePx) }

// stickerSuccessText — надпись удачи номер i с минутами кода: «Ура! +30 минут», «+1 минута».
func stickerSuccessText(i, minutes int) string {
	return strings.Replace(stickerSuccessTexts[i], "{m}", fmt.Sprintf("%d %s", minutes, minutesWord(minutes)), 1)
}

// StickerPick — выбранный ответ: номер стикера, готовая надпись, удача ли это.
type StickerPick struct {
	Sticker string
	Text    string
	OK      bool
}

// lastPick — прошлый показ одного вида: номер стикера и номер надписи (-1 — не было).
type lastPick struct {
	sticker string
	text    int
}

// stickerPicker выбирает стикер и надпись, не повторяя прошлый показ; прошлое помнится отдельно
// для удачи и неудачи (два неверных кода подряд — разные ответы, даже если между ними был верный).
type stickerPicker struct {
	rnd     func(n int) int // случайное число 0…n-1; тесты подменяют
	ok, bad lastPick
}

// newStickerPicker создаёт выбор с источником случайности rnd; nil — math/rand.
func newStickerPicker(rnd func(n int) int) *stickerPicker {
	if rnd == nil {
		rnd = rand.IntN
	}
	return &stickerPicker{rnd: rnd, ok: lastPick{text: -1}, bad: lastPick{text: -1}}
}

// pickIndex — случайный номер 0…n-1, не равный prev (при n == 1 — 0).
func (p *stickerPicker) pickIndex(n, prev int) int {
	if n <= 1 {
		return 0
	}
	if prev < 0 || prev >= n {
		return clampIndex(p.rnd(n), n)
	}
	i := clampIndex(p.rnd(n-1), n-1)
	if i >= prev {
		i++ // пропустить прошлый: остальные равновероятны
	}
	return i
}

func clampIndex(i, n int) int { return max(0, min(i, n-1)) }

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return -1
}

// success — ответ на верный код с minutes минутами.
func (p *stickerPicker) success(minutes int) StickerPick {
	s := stickerSuccess[p.pickIndex(len(stickerSuccess), indexOf(stickerSuccess, p.ok.sticker))]
	t := p.pickIndex(len(stickerSuccessTexts), p.ok.text)
	p.ok = lastPick{sticker: s, text: t}
	return StickerPick{Sticker: s, Text: stickerSuccessText(t, minutes), OK: true}
}

// fail — ответ на неверный, несуществующий или использованный код.
func (p *stickerPicker) fail() StickerPick {
	s := stickerFail[p.pickIndex(len(stickerFail), indexOf(stickerFail, p.bad.sticker))]
	t := p.pickIndex(len(stickerFailTexts), p.bad.text)
	p.bad = lastPick{sticker: s, text: t}
	return StickerPick{Sticker: s, Text: stickerFailTexts[t]}
}

// stickerFrames — число кадров стикера n (0 — стикера нет во встроенных).
func stickerFrames(n string) int {
	counts, err := stickerFrameCounts()
	if err != nil {
		return 0
	}
	return counts[n]
}

// stickerDuration — одно проигрывание стикера из frames кадров при StickerFPS.
func stickerDuration(frames int) time.Duration {
	return time.Duration(frames) * time.Second / StickerFPS
}

// stickerFrameAt — номер кадра в момент now показа, начатого в started: кадры идут StickerFPS в
// секунду и останавливаются на последнем (проигрывается один раз). still — анимации Windows
// выключены: сразу последний кадр.
func stickerFrameAt(started, now time.Time, frames int, still bool) int {
	if frames <= 0 {
		return 0
	}
	if still {
		return frames - 1
	}
	d := now.Sub(started)
	if d < 0 {
		return 0
	}
	return min(int(d*StickerFPS/time.Second), frames-1)
}

// stickerCacheSize — сколько наборов кадров держать: показанный и прошлый (кадры 200 px одного
// стикера — около 5 МБ, всех семнадцати — под сотню).
const stickerCacheSize = 2

type stickerKey struct {
	n  string
	px int
}

var (
	stickerCacheMu sync.Mutex
	stickerCache   []stickerCacheEntry
)

type stickerCacheEntry struct {
	key    stickerKey
	frames [][]byte
}

// StickerFramesBGRA возвращает кадры стикера n стороной px (больше StickerFramePx не бывает) в
// пикселях BGRA с предумноженной прозрачностью, строки сверху вниз — как RestCatBGRA. Уменьшение
// усреднением по площади. Последние наборы кэшируются: повторный показ не разбирает PNG заново.
// px < 1 — пустой срез.
func StickerFramesBGRA(n string, px int) ([][]byte, error) {
	if px < 1 {
		return [][]byte{}, nil
	}
	px = min(px, StickerFramePx)
	key := stickerKey{n, px}
	stickerCacheMu.Lock()
	defer stickerCacheMu.Unlock()
	for i, e := range stickerCache {
		if e.key == key {
			if i > 0 { // свежий — в начало
				stickerCache = append([]stickerCacheEntry{e}, append(stickerCache[:i:i], stickerCache[i+1:]...)...)
			}
			return e.frames, nil
		}
	}
	frames, err := decodeStickerFrames(n, px)
	if err != nil {
		return nil, err
	}
	stickerCache = append([]stickerCacheEntry{{key, frames}}, stickerCache...)
	if len(stickerCache) > stickerCacheSize {
		stickerCache = stickerCache[:stickerCacheSize]
	}
	return frames, nil
}

// decodeStickerFrames разбирает спрайт стикера n и уменьшает каждый кадр до px.
func decodeStickerFrames(n string, px int) ([][]byte, error) {
	count := stickerFrames(n)
	data, err := stickerSprite(n)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, fmt.Errorf("стикер %s: нет в frames.json", n)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("стикер %s: %w", n, err)
	}
	if b := img.Bounds(); b.Dx() != count*StickerFramePx || b.Dy() != StickerFramePx {
		return nil, fmt.Errorf("стикер %s: %dx%d, ждали %d кадров по %d", n, b.Dx(), b.Dy(), count, StickerFramePx)
	}
	src := image.NewNRGBA(img.Bounds())
	draw.Draw(src, src.Bounds(), img, img.Bounds().Min, draw.Src)
	out := make([][]byte, count)
	for f := range out {
		out[f] = scaleBGRA(src, f*StickerFramePx, StickerFramePx, px)
	}
	return out, nil
}

// scaleBGRA уменьшает квадрат src со стороной side от (x0, 0) до size×size усреднением по
// площади: BGRA с предумноженной прозрачностью (как RestCatBGRA).
func scaleBGRA(src *image.NRGBA, x0, side, size int) []byte {
	out := make([]byte, size*size*4)
	for y := 0; y < size; y++ {
		y0, y1 := y*side/size, max((y+1)*side/size, y*side/size+1)
		for x := 0; x < size; x++ {
			sx0, sx1 := x0+x*side/size, x0+max((x+1)*side/size, x*side/size+1)
			var r, g, b, a, cnt uint32
			for sy := y0; sy < y1; sy++ {
				p := src.PixOffset(sx0, sy)
				for sx := sx0; sx < sx1; sx++ {
					pa := uint32(src.Pix[p+3])
					r += uint32(src.Pix[p]) * pa / 255
					g += uint32(src.Pix[p+1]) * pa / 255
					b += uint32(src.Pix[p+2]) * pa / 255
					a += pa
					cnt++
					p += 4
				}
			}
			o := (y*size + x) * 4
			out[o], out[o+1], out[o+2], out[o+3] = byte(b/cnt), byte(g/cnt), byte(r/cnt), byte(a/cnt)
		}
	}
	return out
}
