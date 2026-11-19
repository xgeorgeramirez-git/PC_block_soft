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

// NoteLayout — раскладка плашки «Сообщение от родителя» (FR-41): заголовок, текст крупно с переносами,
// большая кнопка «Понял» по центру внизу. Координаты клиентские, в пикселях экрана.
type NoteLayout struct {
	W, H   int32
	Title  Rect
	Text   Rect
	Button Rect
}

// Размеры плашки сообщения при 96 dpi: ширина 560, поля 24, кнопка 200×56. Рамка нейтральная, как у
// остальных плашек стража, без цветной полосы (ТЗ 8.0: цвет — только для статусов). Тесная плашка
// (над экраном блокировки на низком экране) — поля 12, кнопка 160×44, заголовок мельче.
const (
	noteWidthPx    = 560
	noteMaxWPct    = 80 // длинному тексту плашка шире, до 80 % ширины области
	noteWidthStep  = 40 // шаг расширения при 96 dpi
	noteMarginPx   = 24 // от верхнего края рабочей области
	noteMinSidePx  = 16 // от боковых краёв на узком экране
	noteLockGapPx  = 12 // от заголовка и столбца кнопок экрана блокировки
	noteLockEdgePx = 16 // столбец экрана блокировки, раздвинутый ради плашки, не ближе к краю экрана
	noteMaxPct     = 40 // плашка не выше 40 % рабочей области

	// NoteFontPx — текст сообщения при 96 dpi; не помещается и в плашку шириной 80 % (над экраном
	// блокировки — и раздвинув его) — мельче шагом noteFontStep, не меньше NoteFontMinPx. Текст не
	// обрезается никогда (FR-41: до 300 знаков и 5 переводов строк видны целиком).
	NoteFontPx    = 28
	NoteFontMinPx = 14
	noteFontStep  = 2
)

// NoteDensity — отступы и размеры плашки сообщения при 96 dpi.
type NoteDensity struct {
	Pad, Gap, BtnGap, BtnW, BtnH int
	TitlePx                      int // заголовок «Сообщение от родителя»
}

var (
	// NoteNormal — обычная плашка.
	NoteNormal = NoteDensity{Pad: 24, Gap: 10, BtnGap: 20, BtnW: 200, BtnH: 56, TitlePx: 24}
	// NoteCompact — тесная плашка: место между заголовком и кнопками экрана блокировки на низком экране.
	NoteCompact = NoteDensity{Pad: 12, Gap: 4, BtnGap: 10, BtnW: 160, BtnH: 44, TitlePx: 18}
)

// NoteWidth — ширина плашки сообщения при dpi на экране шириной areaW (узкий экран — уже).
func NoteWidth(dpi int, areaW int32) int32 {
	w := Scaled(noteWidthPx, dpi)
	if limit := areaW - 2*Scaled(noteMinSidePx, dpi); limit > 0 && w > limit {
		w = limit
	}
	return w
}

// noteWidths — ширины плашки по возрастанию: от NoteWidth до 80 % ширины области шагом noteWidthStep.
func noteWidths(dpi int, areaW int32) []int32 {
	w0 := NoteWidth(dpi, areaW)
	wMax := max(w0, min(areaW*noteMaxWPct/100, areaW-2*Scaled(noteMinSidePx, dpi)))
	out := []int32{w0}
	for w := w0 + Scaled(noteWidthStep, dpi); w < wMax; w += Scaled(noteWidthStep, dpi) {
		out = append(out, w)
	}
	if wMax > w0 {
		out = append(out, wMax)
	}
	return out
}

// NoteTextWidth — ширина области заголовка и текста плашки шириной w: окно знает её до измерения.
func NoteTextWidth(dpi int, w int32, d NoteDensity) int32 {
	return w - 2*Scaled(d.Pad, dpi)
}

// NewNoteLayout считает плашку ширины w для заголовка высотой titleH и текста высотой textH.
func NewNoteLayout(dpi int, d NoteDensity, w, titleH, textH int32) NoteLayout {
	s := func(px int) int32 { return Scaled(px, dpi) }
	left, pad := s(d.Pad), s(d.Pad)
	l := NoteLayout{W: w}
	tw := NoteTextWidth(dpi, w, d)
	l.Title = Rect{Left: left, Top: pad, Right: left + tw, Bottom: pad + titleH}
	l.Text = Rect{Left: left, Top: l.Title.Bottom + s(d.Gap), Right: left + tw, Bottom: l.Title.Bottom + s(d.Gap) + textH}
	bw := min(s(d.BtnW), tw)
	bl := left + (tw-bw)/2
	bt := l.Text.Bottom + s(d.BtnGap)
	l.Button = Rect{Left: bl, Top: bt, Right: bl + bw, Bottom: bt + s(d.BtnH)}
	l.H = l.Button.Bottom + pad
	return l
}

// NoteLock — экран блокировки PC Block под плашкой сообщения, в тех же координатах, что рабочая
// область: заголовок («Время вышло…») и верх столбца поля и кнопок (подпись поля, поле или первая
// кнопка) — как стоят без плашки. Плашка не закрывает ни то, ни другое (NFR-10: «Пароль родителя» и
// аварийный выход видны). Не хватает места между ними — экран блокировки раздвигается: столбец поля и
// кнопок вниз не больше MaxDown (низ последней кнопки остаётся на экране), заголовок вверх не больше MaxUp.
type NoteLock struct {
	Title       Rect
	ControlsTop int32
	MaxUp       int32
	MaxDown     int32
}

// NoteMeasure — высота текста s шрифтом fontPx (размер при 96 dpi) в ширину width; меряет Windows
// тем же переносом, что рисует.
type NoteMeasure func(fontPx int, width int32, s string) int32

// NoteFit — плашка сообщения, разложенная под место: плотность, размер шрифта текста (при 96 dpi),
// текст (всегда целиком), раскладка, окно в координатах области и сдвиг экрана блокировки под ней:
// ShiftDown — столбец поля и кнопок вниз, ShiftUp — заголовок и всё над столбцом вверх.
type NoteFit struct {
	Density            NoteDensity
	FontPx             int
	Text               string
	Layout             NoteLayout
	Place              Rect
	ShiftUp, ShiftDown int32
}

// noteSpot — место для плашки высотой h: верх и сдвиг экрана блокировки; ok — поместилась. Плашка не
// выше noteMaxPct % рабочей области. На рабочем столе — сверху рабочей области. Над экраном блокировки —
// выше его заголовка, если там помещается, иначе между заголовком и столбцом кнопок; spread — можно
// раздвинуть экран: сначала столбец кнопок вниз, потом заголовок вверх, ровно на нехватку.
func noteSpot(dpi int, area Rect, lock *NoteLock, h int32, spread bool) (top, up, down int32, ok bool) {
	s := func(px int) int32 { return Scaled(px, dpi) }
	if h > (area.Bottom-area.Top)*noteMaxPct/100 {
		return 0, 0, 0, false
	}
	top = area.Top + s(noteMarginPx)
	if lock == nil {
		return top, 0, 0, top+h <= area.Bottom-s(noteMarginPx)
	}
	gap := s(noteLockGapPx)
	if top+h <= lock.Title.Top-gap {
		return top, 0, 0, true
	}
	midTop := lock.Title.Bottom + gap
	need := h - (lock.ControlsTop - gap - midTop)
	if need <= 0 {
		return midTop, 0, 0, true
	}
	if !spread {
		return 0, 0, 0, false
	}
	down = min(need, max(0, lock.MaxDown))
	up = need - down
	if up > max(0, lock.MaxUp) {
		return 0, 0, 0, false
	}
	return midTop - up, up, down, true
}

// FitNote раскладывает плашку с заголовком title и текстом text в области area (рабочая область
// монитора) над экраном блокировки lock (nil — рабочий стол). Шрифт текста от NoteFontPx вниз до
// NoteFontMinPx; на каждом размере — обычная плашка, потом тесная, каждая от NoteWidth шире до 80 %
// области; не поместилось — над экраном блокировки та же широкая плашка, раздвинув экран (начиная со
// шрифта, которым встаёт одна строка без раздвижки: короткое сообщение экран не двигает, длинное не
// мельче короткого); и только потом шрифт мельче. Текст не
// обрезается. Не поместилось и мельчайшим шрифтом (крошечный экран) — тесная широкая плашка с
// раздвинутым экраном прижимается к столбцу кнопок сверху: кнопки и поле пароля важнее заголовка.
func FitNote(dpi int, area Rect, lock *NoteLock, title, text string, measure NoteMeasure) NoteFit {
	widths := noteWidths(dpi, area.Right-area.Left)
	wMax := widths[len(widths)-1]
	build := func(d NoteDensity, font int, w int32) NoteLayout {
		tw := NoteTextWidth(dpi, w, d)
		return NewNoteLayout(dpi, d, w, measure(d.TitlePx, tw, title), measure(font, tw, text))
	}
	place := func(f NoteFit, top int32) NoteFit {
		left := area.Left + (area.Right-area.Left-f.Layout.W)/2
		f.Place = Rect{Left: left, Top: top, Right: left + f.Layout.W, Bottom: top + f.Layout.H}
		return f
	}
	densities := []NoteDensity{NoteNormal, NoteCompact}
	fits := func(d NoteDensity, font int, w int32, t string) (NoteLayout, int32, bool) {
		tw := NoteTextWidth(dpi, w, d)
		l := NewNoteLayout(dpi, d, w, measure(d.TitlePx, tw, title), measure(font, tw, t))
		top, _, _, ok := noteSpot(dpi, area, lock, l.H, false)
		return l, top, ok
	}
	// Шрифт короткого сообщения (одна строка) на этом месте: экран раздвигается только с него и мельче,
	// так что длинный текст не мельче короткого, а короткий экран не двигает.
	oneLine := NoteFontMinPx
	for font := NoteFontPx; font > NoteFontMinPx && oneLine == NoteFontMinPx; font -= noteFontStep {
		for _, d := range densities {
			if _, _, ok := fits(d, font, widths[0], "Ж"); ok {
				oneLine = font
				break
			}
		}
	}
	for font := NoteFontPx; font >= NoteFontMinPx; font -= noteFontStep {
		for _, d := range densities {
			for _, w := range widths {
				if l, top, ok := fits(d, font, w, text); ok {
					return place(NoteFit{Density: d, FontPx: font, Text: text, Layout: l}, top)
				}
			}
		}
		if lock == nil || font > oneLine {
			continue
		}
		for _, d := range densities {
			l := build(d, font, wMax)
			if top, up, down, ok := noteSpot(dpi, area, lock, l.H, true); ok {
				return place(NoteFit{Density: d, FontPx: font, Text: text, Layout: l, ShiftUp: up, ShiftDown: down}, top)
			}
		}
	}
	f := NoteFit{Density: NoteCompact, FontPx: NoteFontMinPx, Text: text, Layout: build(NoteCompact, NoteFontMinPx, wMax)}
	h := f.Layout.H
	top := area.Top + Scaled(noteMarginPx, dpi)
	if lock != nil {
		f.ShiftUp, f.ShiftDown = max(0, lock.MaxUp), max(0, lock.MaxDown)
		top = lock.ControlsTop + f.ShiftDown - Scaled(noteLockGapPx, dpi) - h
	}
	top = max(area.Top, min(top, area.Bottom-h))
	return place(f, top)
}
