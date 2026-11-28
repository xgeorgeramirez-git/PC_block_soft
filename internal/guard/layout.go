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

// NoteLayout - раскладка окна «Сообщение от родителя» (FR-41, вид - ТЗ 8.2): сверху по центру стикер,
// его нижняя часть (Overlap) лежит на карточке; в карточке по центру заголовок, текст родителя и кнопка
// «Понял» во всю ширину внутреннего поля; вокруг карточки мягкая тень. W, H - окно целиком, со стикером
// и тенью: пиксели тени ловят клики, поэтому место окна (NoteFit.Place) считается с ней. Координаты
// клиентские, в пикселях экрана. Без стикера Sticker пустой, StickerPx и Overlap 0.
type NoteLayout struct {
	W, H      int32
	Card      Rect  // карточка: фон, рамка, скругление Radius
	Shadow    Rect  // где рисуется тень: карточка, сдвинутая на ShadowDY и расширенная на ShadowBlur
	Sticker   Rect  // стикер StickerPx×StickerPx
	StickerPx int32 // сторона стикера
	Overlap   int32 // сколько стикера заходит на карточку
	Title     Rect
	Text      Rect
	Button    Rect
	Radius    int32 // скругление карточки
	Border    int32 // рамка карточки
	BtnRadius int32 // скругление кнопки
	ShadowDY  int32 // сдвиг тени вниз
	// ShadowBlur - размытие тени: тень сходит на нет на этом расстоянии от сдвинутой карточки.
	ShadowBlur int32
}

// Размеры окна сообщения при 96 dpi (макет вариант Б): карточка 520, стикер 170 (на низком экране не
// меньше 96), нахлёст 56 из 170, скругление карточки 20, рамка 2, кнопка скругление 6 (ТЗ 8.0 п. 1).
const (
	noteWidthPx      = 520
	noteStickerMaxPx = 170
	noteStickerMinPx = 96
	noteStickerStep  = 10 // шаг уменьшения стикера при 96 dpi
	noteOverlapPx    = 56 // из noteStickerMaxPx: нахлёст пропорционален стороне стикера
	noteRadiusPx     = 20
	noteBorderPx     = 2
	noteBtnRadiusPx  = 6
	noteMaxWPct      = 80 // длинному тексту карточка шире, окно до 80 % ширины области
	noteWidthStep    = 40 // шаг расширения при 96 dpi
	noteMarginPx     = 24 // от верхнего края рабочей области
	noteMinSidePx    = 16 // от боковых краёв на узком экране
	noteLockGapPx    = 12 // от заголовка и столбца кнопок экрана блокировки
	noteLockEdgePx   = 16 // столбец экрана блокировки, раздвинутый ради окна, не ближе к краю экрана
	noteMaxPct       = 55 // окно с тенью и стикером не выше 55 % рабочей области (Ruling FB28-1)

	// NoteFontPx - текст сообщения при 96 dpi: 42 px (D-107, в 1,5 раза крупнее прежних 28); не
	// помещается и в карточку шириной 80 % (над экраном блокировки - и раздвинув его) - мельче по
	// лестнице noteFontSteps. Длинное сообщение на обычном экране не мельче NoteFontLongPx (21), на очень
	// низком - до NoteFontMinPx (14). Текст не обрезается никогда (FR-41: до 300 знаков и 5 переводов
	// строк видны целиком).
	NoteFontPx     = 42
	NoteFontLongPx = 21
	NoteFontMinPx  = 14
)

// noteFontSteps - размеры текста сообщения при 96 dpi по убыванию (R-8): шаг 2 от 42 не попал бы в 21.
var noteFontSteps = []int{NoteFontPx, 38, 34, 30, 27, 24, NoteFontLongPx, 18, 16, NoteFontMinPx}

// NoteDensity - отступы и размеры окна сообщения при 96 dpi.
type NoteDensity struct {
	PadTop, PadSide, PadBottom int // поля карточки (сверху - без стикера)
	StickerGap                 int // от низа стикера до заголовка
	Gap                        int // от заголовка до текста
	BtnGap                     int // от текста до кнопки
	BtnH                       int
	TitlePx                    int // заголовок «Сообщение от родителя», полужирный синий
	BtnPx                      int // подпись кнопки «Понял»
	ShadowDY, ShadowBlur       int
}

var (
	// NoteNormal - обычное окно (макет): поля 64 со стикером (нахлёст 56 и 8), 32 по бокам, 28 снизу.
	NoteNormal = NoteDensity{PadTop: 28, PadSide: 32, PadBottom: 28, StickerGap: 8, Gap: 12, BtnGap: 24, BtnH: 56,
		TitlePx: 18, BtnPx: 22, ShadowDY: 6, ShadowBlur: 16}
	// NoteCompact - тесное окно: место между заголовком и кнопками экрана блокировки на низком экране.
	NoteCompact = NoteDensity{PadTop: 10, PadSide: 16, PadBottom: 10, StickerGap: 4, Gap: 4, BtnGap: 8, BtnH: 44,
		TitlePx: 16, BtnPx: 18, ShadowDY: 2, ShadowBlur: 6}
)

// NoteStickerPx - сторона стикера сообщения при dpi: 170 px при 100 %, не больше кадра StickerFramePx.
func NoteStickerPx(dpi int) int32 { return min(Scaled(noteStickerMaxPx, dpi), StickerFramePx) }

// NoteStickerMinPx - меньше этого стикер сообщения не бывает: 96 px при 100 %, не больше кадра.
func NoteStickerMinPx(dpi int) int32 { return min(Scaled(noteStickerMinPx, dpi), StickerFramePx) }

// NoteOverlap - сколько стикера стороной px заходит на карточку: 56 из 170, с округлением.
func NoteOverlap(px int32) int32 {
	if px <= 0 {
		return 0
	}
	return (px*noteOverlapPx + noteStickerMaxPx/2) / noteStickerMaxPx
}

// noteStickerSizes - стороны стикера по убыванию: от NoteStickerPx шагом noteStickerStep до
// NoteStickerMinPx включительно.
func noteStickerSizes(dpi int) []int32 {
	hi, lo := NoteStickerPx(dpi), NoteStickerMinPx(dpi)
	out := []int32{}
	for px := hi; px > lo; px -= Scaled(noteStickerStep, dpi) {
		out = append(out, px)
	}
	return append(out, lo)
}

// noteShadowSide - тень сбоку от карточки обычного окна (оно шире тесного).
func noteShadowSide(dpi int) int32 { return Scaled(NoteNormal.ShadowBlur, dpi) }

// noteMaxCardWidth - шире карточка не бывает: окно с тенью не шире 80 % области и не ближе
// noteMinSidePx к краям.
func noteMaxCardWidth(dpi int, areaW int32) int32 {
	return min(areaW*noteMaxWPct/100, areaW-2*Scaled(noteMinSidePx, dpi)) - 2*noteShadowSide(dpi)
}

// NoteWidth - ширина карточки сообщения при dpi на экране шириной areaW (узкий экран - уже).
func NoteWidth(dpi int, areaW int32) int32 {
	w := Scaled(noteWidthPx, dpi)
	if limit := noteMaxCardWidth(dpi, areaW); limit > 0 && w > limit {
		w = limit
	}
	return w
}

// noteWidths - ширины карточки по возрастанию: от NoteWidth до noteMaxCardWidth шагом noteWidthStep.
func noteWidths(dpi int, areaW int32) []int32 {
	w0 := NoteWidth(dpi, areaW)
	wMax := max(w0, noteMaxCardWidth(dpi, areaW))
	out := []int32{w0}
	for w := w0 + Scaled(noteWidthStep, dpi); w < wMax; w += Scaled(noteWidthStep, dpi) {
		out = append(out, w)
	}
	if wMax > w0 {
		out = append(out, wMax)
	}
	return out
}

// NoteTextWidth - ширина заголовка, текста и кнопки в карточке шириной cardW: окно знает её до измерения.
func NoteTextWidth(dpi int, cardW int32, d NoteDensity) int32 {
	return cardW - 2*Scaled(d.PadSide, dpi)
}

// NewNoteLayout считает окно с карточкой шириной cardW, стикером стороной sticker (0 - без стикера),
// заголовком высотой titleH и текстом высотой textH.
func NewNoteLayout(dpi int, d NoteDensity, cardW, sticker, titleH, textH int32) NoteLayout {
	s := func(px int) int32 { return Scaled(px, dpi) }
	dy, blur := s(d.ShadowDY), s(d.ShadowBlur)
	l := NoteLayout{Radius: s(noteRadiusPx), Border: s(noteBorderPx), BtnRadius: s(noteBtnRadiusPx), ShadowDY: dy, ShadowBlur: blur}
	padTop := s(d.PadTop)
	cardTop := max(0, blur-dy) // тень над карточкой
	if sticker > 0 {
		l.StickerPx, l.Overlap = sticker, NoteOverlap(sticker)
		cardTop = max(cardTop, sticker-l.Overlap)
		padTop = max(padTop, l.Overlap+s(d.StickerGap))
	}
	l.Card = Rect{Left: blur, Top: cardTop, Right: blur + cardW}
	l.W = l.Card.Right + blur
	if sticker > 0 {
		left := (l.W - sticker) / 2
		l.Sticker = Rect{Left: left, Top: cardTop - (sticker - l.Overlap), Right: left + sticker, Bottom: cardTop + l.Overlap}
	}
	left := l.Card.Left + s(d.PadSide)
	tw := NoteTextWidth(dpi, cardW, d)
	l.Title = Rect{Left: left, Top: cardTop + padTop, Right: left + tw, Bottom: cardTop + padTop + titleH}
	top := l.Title.Bottom + s(d.Gap)
	l.Text = Rect{Left: left, Top: top, Right: left + tw, Bottom: top + textH}
	top = l.Text.Bottom + s(d.BtnGap)
	l.Button = Rect{Left: left, Top: top, Right: left + tw, Bottom: top + s(d.BtnH)}
	l.Card.Bottom = l.Button.Bottom + s(d.PadBottom)
	l.Shadow = Rect{Left: l.Card.Left - blur, Top: l.Card.Top + dy - blur, Right: l.Card.Right + blur, Bottom: l.Card.Bottom + dy + blur}
	l.H = l.Shadow.Bottom
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

// NoteMeasure - высота текста s шрифтом fontPx (размер при 96 dpi) в ширину width; меряет Windows
// тем же переносом, что рисует.
type NoteMeasure func(fontPx int, width int32, s string) int32

// NoteFit - окно сообщения, разложенное под место: плотность, размер шрифта текста (при 96 dpi),
// текст (всегда целиком), раскладка, окно с тенью в координатах области и сдвиг экрана блокировки под
// ним: ShiftDown - столбец поля и кнопок вниз, ShiftUp - заголовок и всё над столбцом вверх.
type NoteFit struct {
	Density            NoteDensity
	FontPx             int
	Text               string
	Layout             NoteLayout
	Place              Rect
	ShiftUp, ShiftDown int32
}

// noteSpot - место для окна высотой h (со стикером и тенью): верх и сдвиг экрана блокировки; ok -
// поместилось. Окно не выше noteMaxPct % рабочей области. На рабочем столе - сверху рабочей области.
// Над экраном блокировки - выше его заголовка, если там помещается, иначе между заголовком и столбцом
// кнопок; spread - можно раздвинуть экран: сначала столбец кнопок вниз, потом заголовок вверх, ровно
// на нехватку.
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

// FitNote раскладывает окно с заголовком title и текстом text в области area (рабочая область
// монитора) над экраном блокировки lock (nil - рабочий стол); sticker - у сообщения есть стикер.
// Порядок ужатия: шрифт текста по лестнице noteFontSteps от 42 вниз до 14; на каждом размере - обычное
// окно со стикером от 170 px до 96 (при 100 %), потом тесное со стикером 96; каждое - с карточкой от
// NoteWidth шире до 80 % области. Не поместилось - над экраном блокировки то же окно с наименьшим
// стикером, раздвинув экран ровно на нехватку; и только потом шрифт мельче (D-107: размер текста
// важнее неподвижного экрана блокировки, короткое сообщение - 42 px, где 42 вообще помещается; длинное
// поэтому не крупнее короткого).
// Над экраном блокировки стикер остаётся, только если текст с ним не мельче, чем без него; иначе (и
// если стикер не помещается совсем: низкий экран при 150 %) - окно без стикера тем же порядком: текст и
// кнопки экрана блокировки важнее картинки. Текст не обрезается. Не поместилось и
// мельчайшим шрифтом (крошечный экран) - тесное широкое окно без стикера с раздвинутым экраном
// прижимается к столбцу кнопок сверху: кнопки и поле пароля важнее заголовка.
func FitNote(dpi int, area Rect, lock *NoteLock, sticker bool, title, text string, measure NoteMeasure) NoteFit {
	type mkey struct {
		font int
		w    int32
		s    string
	}
	memo := map[mkey]int32{}
	meas := func(font int, w int32, s string) int32 {
		k := mkey{font, w, s}
		h, ok := memo[k]
		if !ok {
			h = measure(font, w, s)
			memo[k] = h
		}
		return h
	}
	widths := noteWidths(dpi, area.Right-area.Left)
	wMax := widths[len(widths)-1]
	build := func(d NoteDensity, font int, w, st int32, t string) NoteLayout {
		tw := NoteTextWidth(dpi, w, d)
		return NewNoteLayout(dpi, d, w, st, meas(d.TitlePx, tw, title), meas(font, tw, t))
	}
	place := func(f NoteFit, top int32) NoteFit {
		left := area.Left + (area.Right-area.Left-f.Layout.W)/2
		f.Place = Rect{Left: left, Top: top, Right: left + f.Layout.W, Bottom: top + f.Layout.H}
		return f
	}
	densities := []NoteDensity{NoteNormal, NoteCompact}
	// stickers - стороны стикера для плотности d: обычное окно - от полного до наименьшего, тесное -
	// наименьший (стикер ужимается раньше отступов); без стикера - 0.
	stickers := func(on bool, d NoteDensity) []int32 {
		if !on {
			return []int32{0}
		}
		if d == NoteCompact {
			return []int32{NoteStickerMinPx(dpi)}
		}
		return noteStickerSizes(dpi)
	}
	try := func(on bool) (NoteFit, bool) {
		for _, font := range noteFontSteps {
			for _, d := range densities {
				for _, st := range stickers(on, d) {
					for _, w := range widths {
						l := build(d, font, w, st, text)
						if top, _, _, ok := noteSpot(dpi, area, lock, l.H, false); ok {
							return place(NoteFit{Density: d, FontPx: font, Text: text, Layout: l}, top), true
						}
					}
				}
			}
			if lock == nil {
				continue
			}
			var best NoteFit
			found := false
			for _, d := range densities {
				st := stickers(on, d)
				// Самая узкая карточка с теми же строками, что у самой широкой: раздвижка наименьшая, а
				// короткое сообщение не растягивается на 80 % экрана.
				l := build(d, font, wMax, st[len(st)-1], text)
				for _, w := range widths {
					if n := build(d, font, w, st[len(st)-1], text); n.Text.Bottom-n.Text.Top == l.Text.Bottom-l.Text.Top {
						l = n
						break
					}
				}
				// Из обычного и тесного окна - то, что раздвигает экран меньше: экран блокировки
				// двигается ровно на нехватку.
				if top, up, down, ok := noteSpot(dpi, area, lock, l.H, true); ok && (!found || up+down < best.ShiftUp+best.ShiftDown) {
					best, found = place(NoteFit{Density: d, FontPx: font, Text: text, Layout: l, ShiftUp: up, ShiftDown: down}, top), true
				}
			}
			if found {
				return best, true
			}
		}
		return NoteFit{}, false
	}
	plain, plainOK := try(false)
	if sticker {
		// Над экраном блокировки стикер не делает текст мельче, чем без него: читать важнее картинки.
		if f, ok := try(true); ok && (lock == nil || !plainOK || f.FontPx >= plain.FontPx) {
			return f
		}
	}
	if plainOK {
		return plain
	}
	f := NoteFit{Density: NoteCompact, FontPx: NoteFontMinPx, Text: text, Layout: build(NoteCompact, NoteFontMinPx, wMax, 0, text)}
	h := f.Layout.H
	top := area.Top + Scaled(noteMarginPx, dpi)
	if lock != nil {
		f.ShiftUp, f.ShiftDown = max(0, lock.MaxUp), max(0, lock.MaxDown)
		top = lock.ControlsTop + f.ShiftDown - Scaled(noteLockGapPx, dpi) - h
	}
	top = max(area.Top, min(top, area.Bottom-h))
	return place(f, top)
}

// NoteTextFits - текст сообщения в раскладке f виден целиком: окно есть (шрифт задан), высота текста,
// намеренная measure шрифтом f.FontPx в ширину прямоугольника текста, не больше высоты Layout.Text, и
// прямоугольник текста на месте окна (Place) не выходит за область area. Обычный путь FitNote строит
// прямоугольник из той же меры, поэтому на деле решает край области: запасное тесное широкое окно на
// крошечном экране бывает выше области (GuardState.NoteTextFits, приёмка S57).
func NoteTextFits(f NoteFit, area Rect, measure NoteMeasure) bool {
	t := f.Layout.Text
	w, h := t.Right-t.Left, t.Bottom-t.Top
	if f.FontPx <= 0 || w <= 0 || measure(f.FontPx, w, f.Text) > h {
		return false
	}
	left, top := f.Place.Left+t.Left, f.Place.Top+t.Top
	return left >= area.Left && left+w <= area.Right && top >= area.Top && top+h <= area.Bottom
}
