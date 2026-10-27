package systray

import (
	"image"
	"image/color"
	"math"
)

// Рисунок карточки-табло без текста: фон с переходом цвета, плитки, двоеточия, кнопка со
// стрелкой. Всё на стандартной библиотеке: сглаженные углы по расстоянию до контура, тень
// размытием по расстоянию, предумноженная прозрачность под UpdateLayeredWindow. Поэтому вид
// карточки проверяется тестами на Linux, а окно Windows только кладёт рисунок на экран и
// дорисовывает текст шрифтом системы (card_windows.go).

// Палитра макета заказчика (docs/mockups/tray-countdown.png).
var (
	cardDark     = color.RGBA{0x3F, 0x56, 0x75, 0xFF} // слева
	cardLight    = color.RGBA{0x7E, 0x98, 0xB4, 0xFF} // справа вверху
	cardTileTop  = color.RGBA{0xF7, 0xFA, 0xFD, 0xFF}
	cardTileBot  = color.RGBA{0xEA, 0xF0, 0xF7, 0xFF} // перепад мал: плитка цельная, без шва
	cardShadowC  = color.RGBA{0x0B, 0x16, 0x26, 0xFF}
	colorNavy    = color.RGBA{0x1E, 0x2B, 0x40, 0xFF} // цифры на плитках
	colorLabel   = color.RGBA{0xD6, 0xDF, 0xEA, 0xFF} // подписи под парами
	colorWhite   = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	cardBtnBase  = color.RGBA{0xF6, 0xF9, 0xFC, 0xFF}
	cardBtnHover = color.RGBA{0xE4, 0xEE, 0xF8, 0xFF}
	cardBtnPress = color.RGBA{0xCF, 0xDD, 0xEB, 0xFF}
)

const (
	cardColonDot  = 20.0 // расстояние от центра плитки до точки двоеточия, px при 100 %
	cardFocusGap  = 4.0  // зазор между кнопкой и кольцом фокуса
	cardDotRadius = 4.6
)

// CardState — что в карточке сейчас подсвечено.
type CardState struct {
	Hover        CardTarget
	Pressed      CardTarget
	Focus        CardTarget
	FocusVisible bool // кольцо фокуса рисуется после первого нажатия клавиши, как :focus-visible
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func mix(a, b color.RGBA, t float64) color.RGBA {
	t = clamp01(t)
	return color.RGBA{
		R: uint8(math.Round(lerp(float64(a.R), float64(b.R), t))),
		G: uint8(math.Round(lerp(float64(a.G), float64(b.G), t))),
		B: uint8(math.Round(lerp(float64(a.B), float64(b.B), t))),
		A: 0xFF,
	}
}

// cardGradient — цвет фона карточки в точке перехода t от 0 (тёмный слева) до 1 (светлый).
func cardGradient(t float64) color.RGBA { return mix(cardDark, cardLight, t) }

// sdfRoundRect — расстояние от точки до скруглённого прямоугольника: меньше нуля внутри.
func sdfRoundRect(px, py, x0, y0, x1, y1, r float64) float64 {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hx, hy := (x1-x0)/2-r, (y1-y0)/2-r
	qx, qy := math.Abs(px-cx)-hx, math.Abs(py-cy)-hy
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}

// coverage переводит расстояние до контура в долю закрытого пикселя (сглаживание на 1 px).
func coverage(d float64) float64 { return clamp01(0.5 - d) }

// blend кладёт цвет c с прозрачностью a поверх пикселя (предумноженная прозрачность).
func blend(img *image.RGBA, x, y int, c color.RGBA, a float64) {
	if a <= 0 || !(image.Point{x, y}).In(img.Rect) {
		return
	}
	if a > 1 {
		a = 1
	}
	i := img.PixOffset(x, y)
	p := img.Pix[i : i+4 : i+4]
	inv := 1 - a
	da := a*255 + float64(p[3])*inv
	ch := func(src uint8, dst uint8) uint8 {
		v := float64(src)*a + float64(dst)*inv
		if v > da {
			v = da
		}
		return uint8(math.Round(v))
	}
	r, g, b := ch(c.R, p[0]), ch(c.G, p[1]), ch(c.B, p[2])
	al := uint8(math.Round(da))
	if r > al {
		r = al
	}
	if g > al {
		g = al
	}
	if b > al {
		b = al
	}
	p[0], p[1], p[2], p[3] = r, g, b, al
}

type paintFn func(x, y float64) (color.RGBA, float64)

func solid(c color.RGBA, a float64) paintFn {
	return func(float64, float64) (color.RGBA, float64) { return c, a }
}

// paintRegion обходит прямоугольник bounds и красит пиксели по доле покрытия cover.
func paintRegion(img *image.RGBA, bounds image.Rectangle, cover func(x, y float64) float64, p paintFn) {
	bounds = bounds.Intersect(img.Rect)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			cv := cover(fx, fy)
			if cv <= 0 {
				continue
			}
			c, a := p(fx, fy)
			blend(img, x, y, c, a*cv)
		}
	}
}

func expand(r Rect, by int) image.Rectangle {
	return image.Rect(r.Left-by, r.Top-by, r.Right+by, r.Bottom+by)
}

func fillRound(img *image.RGBA, r Rect, radius float64, p paintFn) {
	x0, y0, x1, y1 := float64(r.Left), float64(r.Top), float64(r.Right), float64(r.Bottom)
	paintRegion(img, expand(r, 1), func(x, y float64) float64 {
		return coverage(sdfRoundRect(x, y, x0, y0, x1, y1, radius))
	}, p)
}

// shadowRound — мягкая тень скруглённого прямоугольника: сдвиг вниз dy, размытие blur.
func shadowRound(img *image.RGBA, r Rect, radius float64, dy, blur, alpha float64) {
	x0, y0, x1, y1 := float64(r.Left), float64(r.Top)+dy, float64(r.Right), float64(r.Bottom)+dy
	reach := int(math.Ceil(blur + math.Abs(dy)))
	paintRegion(img, expand(r, reach+1), func(x, y float64) float64 {
		d := sdfRoundRect(x, y, x0, y0, x1, y1, radius)
		if d <= 0 {
			return 1
		}
		if d >= blur {
			return 0
		}
		t := d / blur
		return 0.5 * (1 + math.Cos(math.Pi*t))
	}, solid(cardShadowC, alpha))
}

// ringArc рисует кольцо (окружность) радиуса mid и толщины w вокруг (cx, cy).
func ring(img *image.RGBA, cx, cy, mid, w float64, c color.RGBA, a float64) {
	reach := int(math.Ceil(mid + w))
	b := image.Rect(int(cx)-reach, int(cy)-reach, int(cx)+reach+1, int(cy)+reach+1)
	paintRegion(img, b, func(x, y float64) float64 {
		return coverage(math.Abs(math.Hypot(x-cx, y-cy)-mid) - w/2)
	}, solid(c, a))
}

func disc(img *image.RGBA, cx, cy, r float64, c color.RGBA, a float64) {
	reach := int(math.Ceil(r)) + 1
	b := image.Rect(int(cx)-reach, int(cy)-reach, int(cx)+reach+1, int(cy)+reach+1)
	paintRegion(img, b, func(x, y float64) float64 { return coverage(math.Hypot(x-cx, y-cy) - r) }, solid(c, a))
}

func distSegment(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := clamp01(((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

// chevron рисует стрелку «>» двумя отрезками с круглыми концами.
func chevron(img *image.RGBA, cx, cy, d, w float64, c color.RGBA) {
	ax, ay := cx-0.08*d, cy-0.17*d
	tx, ty := cx+0.10*d, cy
	bx, by := cx-0.08*d, cy+0.17*d
	reach := int(0.3*d) + 2
	b := image.Rect(int(cx)-reach, int(cy)-reach, int(cx)+reach+1, int(cy)+reach+1)
	paintRegion(img, b, func(x, y float64) float64 {
		dist := math.Min(distSegment(x, y, ax, ay, tx, ty), distSegment(x, y, tx, ty, bx, by))
		return coverage(dist - w/2)
	}, solid(c, 1))
}

// RenderCardBase рисует карточку без текста. Размер рисунка l.W на l.H; за пределами
// карточки прозрачно, кроме тени.
func RenderCardBase(l CardLayout, v CountdownView, st CardState) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, l.W, l.H))
	s := l.Scale
	radius := float64(l.Radius)

	shadowRound(img, l.Card, radius, 6*s, 16*s, 0.38)
	cw, ch := float64(l.Card.Dx()), float64(l.Card.Dy())
	fillRound(img, l.Card, radius, func(x, y float64) (color.RGBA, float64) {
		// Переход от тёмного слева к светлому справа вверху (макет): основной вклад даёт
		// горизонталь, вертикаль добавляет свет к верхнему краю.
		t := 0.5*(x-float64(l.Card.Left))/cw + 0.5*(1-(y-float64(l.Card.Top))/ch)
		return cardGradient(clamp01(t)), 1
	})

	switch v.Kind {
	case CountdownTime:
		tileR := 18 * s
		for _, t := range l.Tiles {
			shadowRound(img, t, tileR, 4*s, 9*s, 0.34)
		}
		for _, t := range l.Tiles {
			top, h := float64(t.Top), float64(t.Dy())
			fillRound(img, t, tileR, func(x, y float64) (color.RGBA, float64) {
				return mix(cardTileTop, cardTileBot, (y-top)/h), 1
			})
		}
		for _, c := range l.Colons {
			cx := float64(c.Left+c.Right) / 2
			cy := float64(c.Top+c.Bottom) / 2
			disc(img, cx, cy-cardColonDot*s, cardDotRadius*s, colorWhite, 1)
			disc(img, cx, cy+cardColonDot*s, cardDotRadius*s, colorWhite, 1)
		}
	default:
		fillRound(img, l.Body, 18*s, solid(colorWhite, 0.13))
	}

	if v.Kind == CountdownError {
		drawRefresh(img, l, st)
	}
	drawArrow(img, l, st)
	return img
}

func drawRefresh(img *image.RGBA, l CardLayout, st CardState) {
	s := l.Scale
	c := cardBtnBase
	switch {
	case st.Pressed == TargetRefresh:
		c = cardBtnPress
	case st.Hover == TargetRefresh:
		c = cardBtnHover
	}
	b := l.Button
	fillRound(img, b, float64(b.Dy())/2, solid(c, 0.97))
	if st.FocusVisible && st.Focus == TargetRefresh {
		gap := cardFocusGap * s
		o := Rect{b.Left - int(gap), b.Top - int(gap), b.Right + int(gap), b.Bottom + int(gap)}
		x0, y0, x1, y1 := float64(o.Left), float64(o.Top), float64(o.Right), float64(o.Bottom)
		rad := float64(o.Dy()) / 2
		w := 2.6 * s
		paintRegion(img, expand(o, 3), func(x, y float64) float64 {
			return coverage(math.Abs(sdfRoundRect(x, y, x0, y0, x1, y1, rad)) - w/2)
		}, solid(colorWhite, 1))
	}
}

func drawArrow(img *image.RGBA, l CardLayout, st CardState) {
	s := l.Scale
	a := l.Arrow
	cx, cy := float64(a.Left+a.Right)/2, float64(a.Top+a.Bottom)/2
	r := float64(a.Dx()) / 2
	switch {
	case st.Pressed == TargetArrow:
		disc(img, cx, cy, r, colorWhite, 0.30)
	case st.Hover == TargetArrow:
		disc(img, cx, cy, r, colorWhite, 0.16)
	}
	w := 2 * s
	ring(img, cx, cy, r-w/2, w, colorWhite, 0.86)
	chevron(img, cx, cy, float64(a.Dx()), 3.2*s, colorWhite)
	if st.FocusVisible && st.Focus == TargetArrow {
		fw := 2.6 * s
		ring(img, cx, cy, r+cardFocusGap*s, fw, colorWhite, 1)
	}
}

// Роли текста на карточке.
type TextRole int

const (
	RoleTitle TextRole = iota
	RoleDigit
	RoleLabel
	RoleMessage
	RoleError
	RoleButton
)

// Align — выравнивание текста в прямоугольнике.
type Align int

const (
	AlignLeft Align = iota
	AlignCenter
)

// TextRun — одна надпись для шрифта системы: где, каким размером и цветом.
type TextRun struct {
	Role  TextRole
	Text  string
	Rect  Rect
	Px    int // высота шрифта в пикселях экрана
	Bold  bool
	Color color.RGBA
	Align Align
	Wrap  bool // перенос по словам; иначе одна строка по центру высоты
}

// digitColor — цвет цифр по статусу (ТЗ 8.0 п.3): янтарный до 15 минут, красный при «ВРЕМЯ ВЫШЛО».
func digitColor(s Status) color.RGBA {
	switch s {
	case StatusLow:
		return colorAmber
	case StatusOut:
		return colorRed
	}
	return colorNavy
}

// CardTexts перечисляет надписи карточки.
func CardTexts(l CardLayout, v CountdownView) []TextRun {
	px := func(base int) int { return int(math.Round(float64(base) * l.Scale)) }
	runs := []TextRun{{Role: RoleTitle, Text: v.Title, Rect: l.Title, Px: px(28), Bold: true, Color: colorWhite, Align: AlignLeft}}
	switch v.Kind {
	case CountdownTime:
		digits := []rune(v.Hours + v.Minutes + v.Seconds)
		for i, d := range digits {
			if i >= len(l.Tiles) {
				break
			}
			runs = append(runs, TextRun{Role: RoleDigit, Text: string(d), Rect: l.Tiles[i], Px: px(68), Bold: true, Color: digitColor(v.Status), Align: AlignCenter})
		}
		for i, name := range []string{textLabelHours, textLabelMinutes, textLabelSeconds} {
			runs = append(runs, TextRun{Role: RoleLabel, Text: name, Rect: l.Labels[i], Px: px(25), Bold: true, Color: colorLabel, Align: AlignCenter})
		}
	case CountdownMessage:
		runs = append(runs, TextRun{Role: RoleMessage, Text: v.Big, Rect: l.Text, Px: px(28), Bold: true, Color: colorWhite, Align: AlignCenter, Wrap: true})
	case CountdownError:
		runs = append(runs,
			TextRun{Role: RoleError, Text: v.Big, Rect: l.ErrorText(), Px: px(22), Bold: true, Color: colorWhite, Align: AlignCenter, Wrap: true},
			TextRun{Role: RoleButton, Text: v.Button, Rect: l.Button, Px: px(18), Bold: true, Color: colorNavy, Align: AlignCenter},
		)
	}
	return runs
}
