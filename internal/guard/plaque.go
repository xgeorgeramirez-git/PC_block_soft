package guard

import (
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/sound"
)

// Плашка «Игрового лобби» про программы и сайты (ТЗ 8.0 п. 1, 8.2.4, 8.2.5; D-111, D-112, D-114; макет —
// plans/design-0.8.4/plaques.py): тёмно-синяя панель с обводкой и бликом сверху, сплошная тень под ней,
// слева движущийся стикер заказчика, справа заголовок крупно с обводкой цветом вида, текст и, у «ждёт
// разрешения», золотая кнопка «Попросить сейчас»; крестик в правом верхнем углу. Окно в правом нижнем
// углу рабочей области, звук — из схемы Windows («Восклицание» или «Уведомление»). Здесь — чистые
// числа (вид, раскладка, память), их проверяют тесты на любой системе; рисунок — plaquerender.go,
// окно — plaque_windows.go.

// Цвета плашки, 0xRRGGBB (kit.py: панель, обводка, блик; plaques.py: текст и крестик).
const (
	PlaqueBgRGB    = 0x132C8A // панель
	PlaqueHiRGB    = 0x3A5DD6 // блик сверху 3 px
	PlaqueInkRGB   = 0x0B1D5C // обводка, тень, обводка заголовка, подпись кнопки
	PlaqueTextRGB  = 0xDCE4FF // текст
	PlaqueCloseRGB = 0x22388F // крестик
	plaqueCloseHot = 0x3552B8 // крестик под мышью

	plaqueBtnTop    = 0xFFE066 // золотая кнопка: верх градиента
	plaqueBtnMid    = 0xFFB800 // с 55 % высоты
	plaqueBtnLow    = 0xD98200 // нижняя полоса 6 px
	plaqueBtnHotTop = 0xFFEC99 // под мышью и нажатая: светлее
	plaqueBtnHotMid = 0xFFC93D

	plaqueAccentWait = 0xFFE066 // заголовок «Ждёт разрешения»
	plaqueAccentOK   = 0x9FF0C4 // «Можно играть!»
	plaqueAccentStop = 0xFFB199 // остальные
)

// PlaqueStyle — вид плашки «Игрового лобби» по виду плашки службы (ТЗ 8.2.4, 8.2.5, D-111, D-112).
type PlaqueStyle struct {
	Sticker string // "031" ждёт, "110" запрещено (программа, сайт, «18+», браузер, решение «Запретить»), "048" видео, игры на сегодня, только учёба, "033" разрешил
	Accent  uint32 // цвет заголовка: 0xFFE066 ждёт, 0x9FF0C4 разрешил, 0xFFB199 остальные
	Sound   string // sound.AliasExclamation или sound.AliasNotify (разрешил)
}

// PlaqueStyleOf — вид плашки kind службы (ipc.Blocked*) «Игровым лобби»; allowed — у решения родителя
// «разрешил». false — прежняя плашка: сбой кнопки (ipc.PlateNotice), предупреждения о времени и
// незнакомый вид (ТЗ 3.2, D-114: gamesOver и studyOnly — тоже «Лобби», стикер 048).
func PlaqueStyleOf(kind string, allowed bool) (PlaqueStyle, bool) {
	stop := PlaqueStyle{Sticker: "110", Accent: plaqueAccentStop, Sound: sound.AliasExclamation}
	switch kind {
	case ipc.BlockedUnknown:
		return PlaqueStyle{Sticker: "031", Accent: plaqueAccentWait, Sound: sound.AliasExclamation}, true
	case ipc.BlockedForbidden, ipc.BlockedSite, ipc.BlockedAdult, ipc.BlockedBrowser:
		return stop, true
	case ipc.BlockedVideoOver, ipc.BlockedGamesOver, ipc.BlockedStudyOnly:
		return PlaqueStyle{Sticker: "048", Accent: plaqueAccentStop, Sound: sound.AliasExclamation}, true
	case ipc.BlockedDecided:
		if allowed {
			return PlaqueStyle{Sticker: "033", Accent: plaqueAccentOK, Sound: sound.AliasNotify}, true
		}
		return stop, true
	}
	return PlaqueStyle{}, false
}

// SplitPlaque делит плашку предупреждения по окнам: табло за 30/15/5 мин — общая карточка (board),
// плашки «Игрового лобби» — своё окно (plaque), остальное — текстовая плашка (text).
func SplitPlaque(w *WarningView) (board, text, plaque *WarningView) {
	board, text = SplitPlate(w)
	if text != nil && text.Sticker != "" {
		return board, nil, text
	}
	return board, text, nil
}

// PlaqueStickerFrame — кадр стикера плашки w в момент now: StickerFPS кадров в секунду от показа по
// кругу; при выключенных анимациях Windows — первый (PL-89). Окно зовёт по своему тикеру без Model.
func PlaqueStickerFrame(w *WarningView, now time.Time) int {
	if w == nil || w.Sticker == "" || w.Frames <= 0 || w.Still {
		return 0
	}
	return stickerFrameAt(w.Started, now, w.Frames, false)
}

// Размеры плашки при 96 dpi (plaques.py): ширина 470 с рамкой, поля 18/54/18/16, стикер 108, зазор 14,
// заголовок 21 (900, обводка 3), текст 15 (600), кнопка 46 (скругление 14, текст 16, поля 16, нижняя
// полоса 6, блик 3, тень 4), крестик 34 (скругление 10, справа 12, сверху 12), скругление панели 24,
// рамка 3, блик 3, тень — сдвиг 5 сплошная; от краёв рабочей области 16, как у прежней плашки.
const (
	plaqueWidthPx     = 470
	plaqueBorderPx    = 3
	plaquePadTopPx    = 18
	plaquePadRightPx  = 54
	plaquePadBottomPx = 18
	plaquePadLeftPx   = 16
	plaqueStickerPx   = 108
	plaqueGapPx       = 14
	plaqueTitleGapPx  = 8
	plaqueBtnGapPx    = 12 // зазор 8 и отступ кнопки 4
	plaqueBtnHPx      = 46
	plaqueBtnRadiusPx = 14
	plaqueBtnPadPx    = 16
	plaqueBtnStripPx  = 6
	plaqueBtnShadowPx = 4
	plaqueHiPx        = 3
	plaqueClosePx     = 34
	plaqueCloseRadPx  = 10
	plaqueCloseInPx   = 12
	plaqueRadiusPx    = 24
	plaqueShadowPx    = 5
	plaqueMarginPx    = 16

	PlaqueTitlePx = 21 // заголовок, при 96 dpi
	PlaqueTextPx  = 15 // текст
	PlaqueBtnPx   = 16 // подпись кнопки
)

// PlaqueLayout — раскладка плашки при dpi в клиентских координатах окна. W, H — окно целиком с тенью
// (её пиксели ловят клики, PL-86). Без кнопки Btn пустой; без заголовка Title нулевой высоты.
type PlaqueLayout struct {
	W, H                                            int32
	Panel, Shadow, Sticker, Title, Text, Btn, Close Rect
	TitlePx, TextPx, BtnPx                          int   // размеры шрифтов при 96 dpi
	Radius, Border, BtnRadius, CloseRadius          int32 // скругления и рамка
	Hi, BtnStrip, BtnShadow                         int32 // блик панели, нижняя полоса и тень кнопки
}

// PlaqueMeasure — ширина и высота текста s шрифтом px (размер при 96 dpi) жирности weight в ширину
// w: меряет Windows тем же переносом, что рисует (DT_WORDBREAK|DT_EDITCONTROL).
type PlaqueMeasure func(px, weight int, w int32, s string) (width, height int32)

// FitPlaque раскладывает плашку с заголовком title, текстом text и кнопкой button («» — без кнопки)
// при dpi и ставит её в правый нижний угол рабочей области area (Rect — место окна с тенью). Текст не
// обрезается: длинный переносится, плашка выше; стикер и столбец текста по центру по высоте.
func FitPlaque(dpi int, area Rect, title, text, button string, measure PlaqueMeasure) (PlaqueLayout, Rect) {
	s := func(px int) int32 { return Scaled(px, dpi) }
	l := PlaqueLayout{W: s(plaqueWidthPx), TitlePx: PlaqueTitlePx, TextPx: PlaqueTextPx, BtnPx: PlaqueBtnPx,
		Radius: s(plaqueRadiusPx), Border: s(plaqueBorderPx), BtnRadius: s(plaqueBtnRadiusPx), CloseRadius: s(plaqueCloseRadPx),
		Hi: s(plaqueHiPx), BtnStrip: s(plaqueBtnStripPx), BtnShadow: s(plaqueBtnShadowPx)}
	side := min(s(plaqueStickerPx), StickerFramePx)
	left := l.Border + s(plaquePadLeftPx)
	colL := left + side + s(plaqueGapPx)
	colR := l.W - l.Border - s(plaquePadRightPx)
	colW := colR - colL
	var titleH, textH, btnW int32
	if title != "" {
		_, titleH = measure(PlaqueTitlePx, 900, colW, title)
	}
	_, textH = measure(PlaqueTextPx, 600, colW, text)
	colH := titleH + textH
	if title != "" {
		colH += s(plaqueTitleGapPx)
	}
	if button != "" {
		bw, _ := measure(PlaqueBtnPx, 900, colW, button)
		btnW = min(colW, bw+2*s(plaqueBtnPadPx)+2*l.Border)
		colH += s(plaqueBtnGapPx) + s(plaqueBtnHPx)
	}
	contentH := max(side, colH)
	contentTop := l.Border + s(plaquePadTopPx)
	panelH := contentTop + contentH + s(plaquePadBottomPx) + l.Border
	l.Panel = Rect{Left: 0, Top: 0, Right: l.W, Bottom: panelH}
	l.Shadow = Rect{Left: 0, Top: s(plaqueShadowPx), Right: l.W, Bottom: panelH + s(plaqueShadowPx)}
	l.H = l.Shadow.Bottom
	st := contentTop + (contentH-side)/2
	l.Sticker = Rect{Left: left, Top: st, Right: left + side, Bottom: st + side}
	y := contentTop + (contentH-colH)/2
	l.Title = Rect{Left: colL, Top: y, Right: colR, Bottom: y + titleH}
	if title != "" {
		y = l.Title.Bottom + s(plaqueTitleGapPx)
	}
	l.Text = Rect{Left: colL, Top: y, Right: colR, Bottom: y + textH}
	if button != "" {
		top := l.Text.Bottom + s(plaqueBtnGapPx)
		l.Btn = Rect{Left: colL, Top: top, Right: colL + btnW, Bottom: top + s(plaqueBtnHPx)}
	}
	cr := l.W - l.Border - s(plaqueCloseInPx)
	ct := l.Border + s(plaqueCloseInPx)
	l.Close = Rect{Left: cr - s(plaqueClosePx), Top: ct, Right: cr, Bottom: ct + s(plaqueClosePx)}
	place := Rect{Left: area.Right - s(plaqueMarginPx) - l.W, Top: area.Bottom - s(plaqueMarginPx) - l.H,
		Right: area.Right - s(plaqueMarginPx), Bottom: area.Bottom - s(plaqueMarginPx)}
	return l, place
}

// Hit — что под точкой (x, y) окна: кнопка «Попросить сейчас», крестик или ничего.
func (l PlaqueLayout) Hit(x, y int32) Button {
	in := func(r Rect) bool { return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom }
	switch {
	case in(l.Close):
		return ButtonPlateClose
	case l.Btn != (Rect{}) && in(l.Btn):
		return ButtonAskNow
	}
	return ButtonNone
}

// PlaqueMemoryBudget — предел памяти стикера и окна плашки (NFR-5: страж ≤ 40 МБ; вместе с окном
// сообщения ≤ 26 МБ, B-84). Под экраном блокировки плашки нет — с полосой промокода она не встречается.
const PlaqueMemoryBudget = 10 << 20

// PlaqueMemory — сколько держит окно плашки со стикером из frames кадров в раскладке l: спрайт
// frames×300² (номера палитры, как у сообщения), кадр BGRA и заплатка основы стороной стикера, DIB окна
// W×H×4 (копии основы целиком нет: RenderPlaqueInto рисует прямо в DIB).
func PlaqueMemory(frames int, l PlaqueLayout) int {
	side, win := int(l.Sticker.Right-l.Sticker.Left), int(l.W)*int(l.H)*4
	return frames*StickerFramePx*StickerFramePx + 2*side*side*4 + win
}
