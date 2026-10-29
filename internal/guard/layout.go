package guard

// Раскладка экрана блокировки и плашки в один столбец по центру. Размеры текста измеряет
// Windows, а положение блоков считается здесь: чистые числа, которые проверяются тестами на
// любой системе (блоки не налезают друг на друга и не выходят за область).

// FlowItem — блок столбца: ширина, высота и отступ над ним (у первого блока отступ не учитывается).
type FlowItem struct {
	W, H int32
	Gap  int32
}

// Scaled переводит размер в пикселях при 96 dpi в пиксели при заданном dpi (масштаб Windows
// 100 % это 96, 150 % это 144). Нулевой и отрицательный dpi считаются 96.
func Scaled(px int, dpi int) int32 {
	if dpi <= 0 {
		dpi = 96
	}
	return int32((px*dpi + 48) / 96)
}

// FlowHeight — высота столбца вместе с отступами.
func FlowHeight(items []FlowItem) int32 {
	var h int32
	for i, it := range items {
		h += it.H
		if i > 0 {
			h += it.Gap
		}
	}
	return h
}

// FlowTop выбирает верх столбца высотой total в области area: чуть выше середины (40 %
// свободного места сверху), но не ближе margin к верхнему краю.
func FlowTop(area Rect, total, margin int32) int32 {
	free := area.Bottom - area.Top - total
	top := area.Top + free*2/5
	if top < area.Top+margin {
		top = area.Top + margin
	}
	return top
}

// Flow раскладывает блоки сверху вниз, начиная с y = top; каждый блок стоит по центру области
// по горизонтали, блок шире области сжимается до её ширины.
func Flow(area Rect, top int32, items []FlowItem) []Rect {
	out := make([]Rect, len(items))
	width := area.Right - area.Left
	y := top
	for i, it := range items {
		if i > 0 {
			y += it.Gap
		}
		w := it.W
		if w > width {
			w = width
		}
		left := area.Left + (width-w)/2
		out[i] = Rect{Left: left, Top: y, Right: left + w, Bottom: y + it.H}
		y += it.H
	}
	return out
}

// PlateLayout — раскладка текстовой плашки справа внизу (плашка 1 мин, «Добавлено N минут»,
// плашки о закрытых программах): размер окна, область текста, крестик в правом верхнем углу и
// кнопки под текстом. Координаты клиентские, в пикселях экрана.
type PlateLayout struct {
	W, H    int32
	Text    Rect
	Close   Rect   // пустой, если крестика нет
	Buttons []Rect // кнопки сверху вниз во всю ширину текста и полосы (подпись «Взять из копилки (30 мин)» не влезла бы в половину); не nil
}

// PlateTextWidth — ширина области текста плашки при dpi: окно знает её до измерения текста.
func PlateTextWidth(dpi int, hasClose bool) int32 {
	s := func(px int) int32 { return Scaled(px, dpi) }
	w := s(400) - s(6) - 2*s(18)
	if hasClose {
		w -= s(28) + s(6)
	}
	return w
}

// NewPlateLayout считает плашку для текста высотой textH и n кнопок столбцом (крестик отдельно).
// Ширина 400 px при 96 dpi, полоса цвета слева, отступы 18 px; крестик 28×28 в 8 px от угла;
// кнопки высотой 40 px через 8 px. Блоки не налезают друг на друга (layout_test.go).
func NewPlateLayout(dpi int, textH int32, n int, hasClose bool) PlateLayout {
	s := func(px int) int32 { return Scaled(px, dpi) }
	width, pad, bar := s(400), s(18), s(6)
	left := bar + pad
	l := PlateLayout{W: width, Buttons: []Rect{}}
	textTop := pad
	if hasClose {
		cs, in := s(28), s(8)
		l.Close = Rect{Left: width - in - cs, Top: in, Right: width - in, Bottom: in + cs}
		if textH < cs {
			textTop += (cs - textH) / 2
		}
	}
	l.Text = Rect{Left: left, Top: textTop, Right: left + PlateTextWidth(dpi, hasClose), Bottom: textTop + textH}
	bottom := l.Text.Bottom
	if l.Close.Bottom > bottom {
		bottom = l.Close.Bottom
	}
	gap, bh := s(8), s(40)
	top := bottom + s(12) - gap
	for i := 0; i < n; i++ {
		top += gap
		l.Buttons = append(l.Buttons, Rect{Left: left, Top: top, Right: width - pad, Bottom: top + bh})
		top += bh
		bottom = top
	}
	l.H = bottom + pad
	return l
}

// Hit возвращает номер кнопки под точкой, -1 — крестик, -2 — мимо.
func (l PlateLayout) Hit(x, y int32) int {
	in := func(r Rect) bool { return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom }
	if l.Close.Right > l.Close.Left && in(l.Close) {
		return -1
	}
	for i, r := range l.Buttons {
		if in(r) {
			return i
		}
	}
	return -2
}
