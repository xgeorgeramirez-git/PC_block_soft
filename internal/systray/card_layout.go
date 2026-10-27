package systray

import "math"

// Геометрия карточки-табло (D-53, макет заказчика). Всё считается в пикселях экрана по
// масштабу dpi/96 и проверяется тестами на Linux; окно только рисует и ловит нажатия.

// Размеры карточки при 100 % (пропорции макета 1,6 : 1), px.
const (
	cardBaseW      = 480
	cardBaseH      = 290
	cardBaseMargin = 14 // поля вокруг карточки под мягкую тень; окно целиком выходит около 1,6 : 1
	cardBasePad    = 26
	cardBaseRadius = 54
	cardBaseArrow  = 50 // диаметр круглой кнопки
	cardBaseTileW  = 54
	cardBaseTileH  = 88  // ближе к квадрату, чем 50x104 в первой версии
	cardBasePair   = 8   // зазор между плитками одной пары
	cardBaseColon  = 40  // ширина между парами: тут двоеточие, с воздухом до плиток
	cardBaseTop    = 112 // верх плиток
	cardBaseLabelH = 32
	cardBaseBtnW   = 176
	cardBaseBtnH   = 44
)

// CardTarget — что на карточке нажимается мышью и клавишами.
type CardTarget int

const (
	TargetNone    CardTarget = iota
	TargetArrow              // круглая кнопка со стрелкой: открыть «Мою статистику»
	TargetRefresh            // «Обновить» под текстом ошибки
)

// Contains: точка внутри прямоугольника (правая и нижняя границы не входят).
func (r Rect) Contains(x, y int) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

// Dx и Dy — ширина и высота.
func (r Rect) Dx() int { return r.Right - r.Left }
func (r Rect) Dy() int { return r.Bottom - r.Top }

// CardLayout — места всех частей карточки в координатах окна. Окно больше карточки на поля
// для тени, поэтому Card лежит внутри W x H со сдвигом Margin.
type CardLayout struct {
	Scale  float64
	W, H   int // окно целиком
	Margin int
	Card   Rect
	Radius int
	Title  Rect
	Arrow  Rect // квадрат вокруг круглой кнопки
	Tiles  [6]Rect
	Colons [2]Rect // области между парами плиток; двоеточие по их центру
	Labels [3]Rect // подписи «часы», «минуты», «секунды»
	Body   Rect    // вся зона плиток: на ней крупная строка или ошибка
	Text   Rect    // место текста ошибки и крупной строки
	Button Rect    // «Обновить»
}

// CardScale переводит dpi в масштаб карточки: от 100 % до 400 %.
func CardScale(dpi int) float64 {
	s := float64(dpi) / 96
	if s < 1 {
		return 1
	}
	if s > 4 {
		return 4
	}
	return s
}

// NewCardLayout считает места по масштабу.
func NewCardLayout(scale float64) CardLayout {
	px := func(v int) int { return int(math.Round(float64(v) * scale)) }
	l := CardLayout{Scale: scale, Margin: px(cardBaseMargin), Radius: px(cardBaseRadius)}
	cw, ch := px(cardBaseW), px(cardBaseH)
	l.W, l.H = cw+2*l.Margin, ch+2*l.Margin
	m := l.Margin
	l.Card = Rect{m, m, m + cw, m + ch}
	pad := px(cardBasePad)
	arrow := px(cardBaseArrow)
	l.Arrow = Rect{l.Card.Right - pad - arrow, l.Card.Top + px(22), l.Card.Right - pad, l.Card.Top + px(22) + arrow}
	l.Title = Rect{l.Card.Left + pad, l.Card.Top + px(24), l.Arrow.Left - px(12), l.Card.Top + px(24) + arrow}

	tw, th := px(cardBaseTileW), px(cardBaseTileH)
	pair, colon := px(cardBasePair), px(cardBaseColon)
	top := l.Card.Top + px(cardBaseTop)
	x := l.Card.Left + pad
	for g := 0; g < 3; g++ {
		for k := 0; k < 2; k++ {
			l.Tiles[g*2+k] = Rect{x, top, x + tw, top + th}
			x += tw
			if k == 0 {
				x += pair
			}
		}
		pairRect := Rect{l.Tiles[g*2].Left, top, l.Tiles[g*2+1].Right, top + th}
		l.Labels[g] = Rect{pairRect.Left, top + th + px(14), pairRect.Right, top + th + px(14) + px(cardBaseLabelH)}
		if g < 2 {
			l.Colons[g] = Rect{x, top, x + colon, top + th}
			x += colon
		}
	}
	// Правый край последней плитки совпал бы с краем внутренней области; округление может
	// сдвинуть его на пиксель, поэтому зона плиток берётся по фактическим краям.
	l.Body = Rect{l.Card.Left + pad, top, l.Tiles[5].Right, l.Labels[2].Bottom}
	btnW, btnH := px(cardBaseBtnW), px(cardBaseBtnH)
	cx := (l.Body.Left + l.Body.Right) / 2
	l.Button = Rect{cx - btnW/2, l.Body.Bottom - px(14) - btnH, cx - btnW/2 + btnW, l.Body.Bottom - px(14)}
	l.Text = Rect{l.Body.Left + px(14), l.Body.Top + px(10), l.Body.Right - px(14), l.Body.Bottom - px(10)}
	return l
}

// ErrorText — место текста ошибки: над кнопкой «Обновить».
func (l CardLayout) ErrorText() Rect {
	t := l.Text
	t.Bottom = l.Button.Top - int(math.Round(8*l.Scale))
	return t
}

// HitTest: что под точкой окна (x, y). Кнопка со стрелкой круглая и есть всегда; «Обновить»
// только на экране ошибки.
func (l CardLayout) HitTest(x, y int, kind CountdownKind) CardTarget {
	a := l.Arrow
	cx, cy := float64(a.Left+a.Right)/2, float64(a.Top+a.Bottom)/2
	r := float64(a.Dx()) / 2
	if dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy; dx*dx+dy*dy <= r*r {
		return TargetArrow
	}
	if kind == CountdownError && l.Button.Contains(x, y) {
		return TargetRefresh
	}
	return TargetNone
}

// FocusOrder — что получает фокус клавиатуры и в каком порядке (Tab). На экране ошибки первой
// идёт «Обновить»: это то, что нужно нажать.
func FocusOrder(kind CountdownKind) []CardTarget {
	if kind == CountdownError {
		return []CardTarget{TargetRefresh, TargetArrow}
	}
	return []CardTarget{TargetArrow}
}

// NextFocus переходит по Tab (back: Shift+Tab) по кругу; нет текущего или он не из списка:
// первый (при back последний).
func NextFocus(order []CardTarget, cur CardTarget, back bool) CardTarget {
	if len(order) == 0 {
		return TargetNone
	}
	at := -1
	for i, t := range order {
		if t == cur {
			at = i
		}
	}
	switch {
	case at < 0 && back:
		return order[len(order)-1]
	case at < 0:
		return order[0]
	case back:
		return order[(at+len(order)-1)%len(order)]
	}
	return order[(at+1)%len(order)]
}
