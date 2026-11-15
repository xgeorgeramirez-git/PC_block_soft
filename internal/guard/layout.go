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

// Ответ на промокод на экране блокировки (R26, 8.2.3 п.3): стикер, под ним надпись.
const (
	StickerGapPx     = 24 // от блока выше до стикера
	StickerTextGapPx = 8  // от стикера до надписи
	stickerMinPx     = 96 // стикер меньше этого (при 100 %) не показывается: остаётся надпись
	stickerSideGapPx = 24 // от места сбоку до столбца поля и кнопок
	stickerMarginPx  = 24 // поле столбца сверху и снизу (как FlowTop экрана блокировки)
)

// PromoStickerSize — сторона стикера в пикселях: StickerPx(dpi), если столбец экрана без стикера
// (columnH, с надписью и кнопками) вместе с ним помещается в высоту areaH с полями; иначе меньше;
// меньше 96 px при 100 % — в столбец не ставится (0): тогда стикер и надпись встают сбоку (PromoSide).
func PromoStickerSize(dpi int, areaH, columnH int32) int32 {
	free := areaH - 2*Scaled(stickerMarginPx, dpi) - columnH - Scaled(StickerGapPx, dpi)
	size := min(StickerPx(dpi), free)
	if size < Scaled(stickerMinPx, dpi) {
		return 0
	}
	return size
}

// PromoSideWidth — ширина места слева от столбца шириной colW (поле и кнопки стоят по центру области),
// с полем у края экрана и отступом до столбца; 0 — места нет.
func PromoSideWidth(dpi int, area Rect, colW int32) int32 {
	colLeft := area.Left + (area.Right-area.Left-colW)/2
	return max(0, colLeft-Scaled(stickerSideGapPx, dpi)-area.Left-Scaled(stickerMarginPx, dpi))
}

// PromoSide ставит ответ на промокод слева от столбца, когда в столбце места нет (низкий экран:
// 1366×768 при 125 %, 1280×720 при 150 %): стикер верхом на уровне anchorTop (поле или первая кнопка),
// под ним надпись высотой textH во всю ширину места. Стикер — сколько помещается по ширине и до низа
// экрана, не больше StickerPx; меньше 96 px при 100 % — без стикера (size 0, sticker пустой), надпись
// стоит на его месте.
func PromoSide(dpi int, area Rect, colW, anchorTop, textH int32) (size int32, sticker, text Rect) {
	w := PromoSideWidth(dpi, area, colW)
	left := area.Left + Scaled(stickerMarginPx, dpi)
	gap := Scaled(StickerTextGapPx, dpi)
	avail := area.Bottom - Scaled(stickerMarginPx, dpi) - anchorTop - gap - textH
	size = min(StickerPx(dpi), w, avail)
	if size < Scaled(stickerMinPx, dpi) {
		size = 0
	}
	top := anchorTop
	if size > 0 {
		sl := left + (w-size)/2
		sticker = Rect{Left: sl, Top: top, Right: sl + size, Bottom: top + size}
		top += size + gap
	}
	return size, sticker, Rect{Left: left, Top: top, Right: left + w, Bottom: top + textH}
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
