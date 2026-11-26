package guard

import "math"

// Рисунок окна «Сообщение от родителя» (FR-41, вид - ТЗ 8.2, макет вариант Б) без текста: мягкая тень,
// скруглённая карточка с синей рамкой, синяя кнопка «Понял»; кадр стикера кладётся поверх отдельно.
// Чистый Go по образцу internal/systray/card_render.go: сглаженные края по расстоянию до контура,
// тень - спад по расстоянию, пиксели BGRA с предумноженной прозрачностью, строки сверху вниз (так их
// ждёт UpdateLayeredWindow). Вне тени окно прозрачно (альфа 0): там клики проходят к окнам под ним.
// Текст рисует окно Windows шрифтом системы поверх основы (note_windows.go).

// Цвета окна сообщения, 0xRRGGBB. Синий - только это окно (ТЗ 8.0 п. 3); контраст синего на фоне
// карточки 4,77:1, белого на синем 5,28:1 (noterender_test.go).
const (
	NoteBlueRGB    = 0x2F6DB5 // рамка, заголовок, кнопка
	NoteBlueHotRGB = 0x24589A // кнопка под мышью и нажатая
	NoteTintRGB    = 0xEEF4FC // фон карточки
	NoteInkRGB     = 0x1B1F23 // текст родителя; тень
	noteShadowA    = 0.28     // плотность тени (макет: rgba(27,31,35,0.28))
)

// noteRGB - составляющие цвета 0xRRGGBB.
func noteRGB(c uint32) (r, g, b float64) {
	return float64(c >> 16 & 0xFF), float64(c >> 8 & 0xFF), float64(c & 0xFF)
}

// noteSDF - расстояние от точки до скруглённого прямоугольника r с радиусом rad: меньше нуля внутри.
func noteSDF(px, py float64, r Rect, rad float64) float64 {
	x0, y0, x1, y1 := float64(r.Left), float64(r.Top), float64(r.Right), float64(r.Bottom)
	rad = math.Max(0, math.Min(rad, math.Min(x1-x0, y1-y0)/2))
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hx, hy := (x1-x0)/2-rad, (y1-y0)/2-rad
	qx, qy := math.Abs(px-cx)-hx, math.Abs(py-cy)-hy
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - rad
}

// noteCover - доля пикселя внутри контура по расстоянию d (сглаживание на 1 px).
func noteCover(d float64) float64 { return math.Max(0, math.Min(1, 0.5-d)) }

// noteBlend кладёт цвет c с прозрачностью a поверх пикселя i буфера (предумноженная прозрачность).
func noteBlend(buf []byte, i int, c uint32, a float64) {
	if a <= 0 {
		return
	}
	a = math.Min(a, 1)
	r, g, b := noteRGB(c)
	inv := 1 - a
	p := buf[i : i+4 : i+4]
	al := math.Round(a*255 + float64(p[3])*inv)
	ch := func(src float64, dst byte) byte { return byte(math.Min(al, math.Round(src*a+float64(dst)*inv))) }
	p[0], p[1], p[2], p[3] = ch(b, p[0]), ch(g, p[1]), ch(r, p[2]), byte(al)
}

// notePaint обходит пиксели прямоугольника bounds (обрезанного окном w×h) и кладёт цвет c с
// прозрачностью a × cover(центр пикселя).
func notePaint(buf []byte, w, h int32, bounds Rect, c uint32, a float64, cover func(x, y float64) float64) {
	x0, y0 := max(bounds.Left, 0), max(bounds.Top, 0)
	x1, y1 := min(bounds.Right, w), min(bounds.Bottom, h)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if cv := cover(float64(x)+0.5, float64(y)+0.5); cv > 0 {
				noteBlend(buf, int((y*w+x)*4), c, a*cv)
			}
		}
	}
}

// noteFillRound заливает скруглённый прямоугольник r радиусом rad цветом c.
func noteFillRound(buf []byte, w, h int32, r Rect, rad int32, c uint32) {
	grow := Rect{Left: r.Left - 1, Top: r.Top - 1, Right: r.Right + 1, Bottom: r.Bottom + 1}
	notePaint(buf, w, h, grow, c, 1, func(x, y float64) float64 { return noteCover(noteSDF(x, y, r, float64(rad))) })
}

// RenderNoteBase рисует основу окна сообщения l: тень, карточка (фон #EEF4FC, рамка Border #2F6DB5,
// скругление Radius), кнопка «Понял» (#2F6DB5, под мышью и нажатая hot - темнее, скругление BtnRadius).
// Буфер l.W×l.H, BGRA с предумноженной прозрачностью, строки сверху вниз; вне тени альфа 0. Текст и
// стикер не рисуются: текст кладёт окно, кадр стикера - NoteStickerOver. Собирается один раз на смену
// раскладки или подсветки кнопки.
func RenderNoteBase(l NoteLayout, hot bool) []byte {
	w, h := l.W, l.H
	buf := make([]byte, int(w)*int(h)*4)
	if w <= 0 || h <= 0 {
		return buf
	}
	// Тень: карточка, сдвинутая на ShadowDY, спадает до нуля на ShadowBlur от края (плавно, по косинусу).
	shifted := l.Card
	shifted.Top, shifted.Bottom = shifted.Top+l.ShadowDY, shifted.Bottom+l.ShadowDY
	blur := math.Max(1, float64(l.ShadowBlur))
	notePaint(buf, w, h, l.Shadow, NoteInkRGB, noteShadowA, func(x, y float64) float64 {
		d := noteSDF(x, y, shifted, float64(l.Radius))
		switch {
		case d <= 0:
			return 1
		case d >= blur:
			return 0
		}
		return 0.5 * (1 + math.Cos(math.Pi*d/blur))
	})
	// Рамка - синяя карточка, поверх неё фон внутри на толщину рамки.
	noteFillRound(buf, w, h, l.Card, l.Radius, NoteBlueRGB)
	inner := Rect{Left: l.Card.Left + l.Border, Top: l.Card.Top + l.Border, Right: l.Card.Right - l.Border, Bottom: l.Card.Bottom - l.Border}
	noteFillRound(buf, w, h, inner, max(0, l.Radius-l.Border), NoteTintRGB)
	btn := uint32(NoteBlueRGB)
	if hot {
		btn = NoteBlueHotRGB
	}
	noteFillRound(buf, w, h, l.Button, l.BtnRadius, btn)
	return buf
}

// NoteStickerOver обновляет в буфере окна dst прямоугольник стикера l.Sticker: берёт пиксели основы
// base (с текстом, если окно его уже нарисовало: текст стикер не задевает) и кладёт поверх кадр frame
// (BGRA с предумноженной прозрачностью, сторона l.StickerPx, строки сверху вниз) - «поверх», альфа
// кадра сохраняется: над карточкой видна только сама картинка, нижняя часть лежит на карточке. Вне
// прямоугольника стикера dst не меняется: окну хватает обновить только его (prcDirty). false - кадр
// не положен: у окна нет стикера, буферы не размера окна или кадр не размера стикера (тогда на месте
// стикера остаётся основа).
func NoteStickerOver(dst, base []byte, l NoteLayout, frame []byte) bool {
	size := int(l.W) * int(l.H) * 4
	if l.StickerPx <= 0 || len(dst) != size || len(base) != size {
		return false
	}
	s := l.Sticker
	x0, y0, x1, y1 := max(s.Left, 0), max(s.Top, 0), min(s.Right, l.W), min(s.Bottom, l.H)
	for y := y0; y < y1; y++ {
		i, j := int((y*l.W+x0)*4), int((y*l.W+x1)*4)
		copy(dst[i:j], base[i:j])
	}
	side := l.StickerPx
	if len(frame) != int(side)*int(side)*4 {
		return false
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			f := int(((y-s.Top)*side + (x - s.Left)) * 4)
			sa := uint32(frame[f+3])
			if sa == 0 {
				continue
			}
			d := int((y*l.W + x) * 4)
			rest := 255 - sa
			for k := 0; k < 4; k++ {
				dst[d+k] = byte(min(255, uint32(frame[f+k])+(uint32(dst[d+k])*rest+127)/255))
			}
		}
	}
	return true
}
