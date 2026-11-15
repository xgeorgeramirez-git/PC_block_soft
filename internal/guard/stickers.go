package guard

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
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

// stickerDuration — один круг стикера из frames кадров при StickerFPS.
func stickerDuration(frames int) time.Duration {
	return time.Duration(frames) * time.Second / StickerFPS
}

// stickerFrameAt — номер кадра в момент now показа, начатого в started: кадры идут StickerFPS в
// секунду по кругу (после последнего снова первый, D-84). still — анимации Windows выключены:
// стикер стоит на последнем кадре.
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
	return int(d * StickerFPS / time.Second % time.Duration(frames))
}

// stickerCacheSize — сколько наборов разобранных кадров держать: один. Стикер из 90 кадров — 14 МБ
// при 100 % (200 px) и 32 МБ при 150 % (300 px); окно, сложив кадры в свою полосу, кэш очищает
// (ClearStickerCache), так что кадры лежат в памяти один раз.
const stickerCacheSize = 1

// StickerStripCols — кадров в ряду полосы кадров окна: 90 кадров по 300 px в одну строку — рисунок
// шириной 27000 px, сеткой по 8 — 2400×3600.
const StickerStripCols = 8

// StickerStripSize — размер полосы для frames кадров, в кадрах: колонок и рядов.
func StickerStripSize(frames int) (cols, rows int) {
	if frames <= 0 {
		return 0, 0
	}
	return min(frames, StickerStripCols), (frames + StickerStripCols - 1) / StickerStripCols
}

// StickerStripCell — клетка кадра frame в полосе: колонка и ряд.
func StickerStripCell(frame int) (col, row int) {
	return frame % StickerStripCols, frame / StickerStripCols
}

type stickerKey struct {
	n  string
	px int
}

var (
	stickerCacheMu sync.Mutex
	stickerCache   []stickerCacheEntry
	// stickerDecoding — разборы, которые идут сейчас: второй запрос того же ждёт первый, а не разбирает
	// заново. Сам разбор идёт без stickerCacheMu: проверка кэша потоком окон его не ждёт.
	stickerDecoding = map[stickerKey]chan struct{}{}
)

type stickerCacheEntry struct {
	key    stickerKey
	frames [][]byte
}

// StickerStripOver складывает кадры (BGRA с предумноженной прозрачностью, сторона px) в полосу-сетку
// на сплошном фоне (r, g, b): непрозрачные BGRA w×h, строки сверху вниз, кадр f — в клетке
// StickerStripCell(f). Клетки без кадров залиты фоном. Окну остаётся положить полосу в память
// одним вызовом и выводить кадры копированием прямоугольника.
func StickerStripOver(frames [][]byte, px int, r, g, b byte) (strip []byte, w, h int) {
	cols, rows := StickerStripSize(len(frames))
	if cols == 0 || px < 1 {
		return []byte{}, 0, 0
	}
	w, h = cols*px, rows*px
	strip = make([]byte, w*h*4)
	for i := 0; i < len(strip); i += 4 {
		strip[i], strip[i+1], strip[i+2], strip[i+3] = b, g, r, 255
	}
	for f, fr := range frames {
		if len(fr) < px*px*4 {
			continue
		}
		col, row := StickerStripCell(f)
		for y := 0; y < px; y++ {
			src := fr[y*px*4 : (y+1)*px*4]
			dst := strip[((row*px+y)*w+col*px)*4:]
			for i := 0; i < len(src); i += 4 {
				rest := 255 - uint32(src[i+3])
				dst[i] = byte(uint32(src[i]) + uint32(b)*rest/255)
				dst[i+1] = byte(uint32(src[i+1]) + uint32(g)*rest/255)
				dst[i+2] = byte(uint32(src[i+2]) + uint32(r)*rest/255)
			}
		}
	}
	return strip, w, h
}

// StickerFramesBGRA возвращает кадры стикера n стороной px (больше StickerFramePx не бывает) в
// пикселях BGRA с предумноженной прозрачностью, строки сверху вниз — как RestCatBGRA. Уменьшение
// усреднением по площади. Последний набор кэшируется (stickerCacheSize): окно берёт его без разбора.
// px < 1 — пустой срез.
func StickerFramesBGRA(n string, px int) ([][]byte, error) {
	if px < 1 {
		return [][]byte{}, nil
	}
	px = min(px, StickerFramePx)
	key := stickerKey{n, px}
	for {
		stickerCacheMu.Lock()
		for i, e := range stickerCache {
			if e.key == key {
				if i > 0 { // свежий — в начало
					stickerCache = append([]stickerCacheEntry{e}, append(stickerCache[:i:i], stickerCache[i+1:]...)...)
				}
				stickerCacheMu.Unlock()
				return e.frames, nil
			}
		}
		wait, busy := stickerDecoding[key]
		if !busy {
			break // разбирает этот вызов; stickerCacheMu ещё взят
		}
		stickerCacheMu.Unlock()
		<-wait // тот же разбор идёт в другой горутине: дождаться и взять из кэша
		if _, ok := stickerFramesCached(n, px); !ok {
			return decodeStickerFrames(n, px) // тот разбор не удался: ошибка своя
		}
	}
	done := make(chan struct{})
	stickerDecoding[key] = done
	stickerCacheMu.Unlock()

	frames, err := decodeStickerFrames(n, px)

	stickerCacheMu.Lock()
	delete(stickerDecoding, key)
	close(done)
	if err == nil {
		stickerCache = append([]stickerCacheEntry{{key, frames}}, stickerCache...)
		if len(stickerCache) > stickerCacheSize {
			stickerCache = stickerCache[:stickerCacheSize]
		}
	}
	stickerCacheMu.Unlock()
	return frames, err
}

// stickerFramesCached — кадры стикера n стороной px, если они уже в кэше (не разбирает PNG: потоку
// окон нельзя ждать разбора долю секунды, пока держится перехват клавиш).
func stickerFramesCached(n string, px int) ([][]byte, bool) {
	px = min(px, StickerFramePx)
	stickerCacheMu.Lock()
	defer stickerCacheMu.Unlock()
	for _, e := range stickerCache {
		if e.key == (stickerKey{n, px}) {
			return e.frames, true
		}
	}
	return nil, false
}

// ClearStickerCache забывает разобранные кадры: экран блокировки снят, память не держится.
func ClearStickerCache() {
	stickerCacheMu.Lock()
	stickerCache = nil
	stickerCacheMu.Unlock()
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
	out := make([][]byte, count)
	if pal, ok := img.(*image.Paletted); ok && pal.Rect.Min == (image.Point{}) {
		// Спрайты палитровые (tools/sticker-frames.py): кадры считаются прямо из палитры, без копии
		// всего спрайта в NRGBA (90 кадров по 300 px — лишние 32 МБ).
		for f := range out {
			out[f] = scalePalettedBGRA(pal, f*StickerFramePx, StickerFramePx, px)
		}
		return out, nil
	}
	src := image.NewNRGBA(img.Bounds())
	draw.Draw(src, src.Bounds(), img, img.Bounds().Min, draw.Src)
	for f := range out {
		out[f] = scaleBGRA(src, f*StickerFramePx, StickerFramePx, px)
	}
	return out, nil
}

// scalePalettedBGRA — то же, что scaleBGRA, для палитрового спрайта: цвета палитры предумножаются
// так же попиксельно, пиксели складываются по номерам палитры.
func scalePalettedBGRA(src *image.Paletted, x0, side, size int) []byte {
	var pr, pg, pb, pa [256]uint32
	for i, c := range src.Palette {
		if i > 255 {
			break
		}
		n := color.NRGBAModel.Convert(c).(color.NRGBA)
		a := uint32(n.A)
		pr[i], pg[i], pb[i], pa[i] = uint32(n.R)*a/255, uint32(n.G)*a/255, uint32(n.B)*a/255, a
	}
	out := make([]byte, size*size*4)
	for y := 0; y < size; y++ {
		y0, y1 := y*side/size, max((y+1)*side/size, y*side/size+1)
		for x := 0; x < size; x++ {
			sx0, sx1 := x0+x*side/size, x0+max((x+1)*side/size, x*side/size+1)
			var r, g, b, a, cnt uint32
			for sy := y0; sy < y1; sy++ {
				row := src.Pix[sy*src.Stride+sx0 : sy*src.Stride+sx1]
				for _, i := range row {
					r += pr[i]
					g += pg[i]
					b += pb[i]
					a += pa[i]
				}
				cnt += uint32(len(row))
			}
			o := (y*size + x) * 4
			out[o], out[o+1], out[o+2], out[o+3] = byte(b/cnt), byte(g/cnt), byte(r/cnt), byte(a/cnt)
		}
	}
	return out
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
