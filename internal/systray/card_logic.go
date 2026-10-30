package systray

import (
	"fmt"
	"image"
	"time"
)

// Решения окна табло без вызовов Windows (card_windows.go только применяет их): что делает
// клавиша, нажатие и отпускание мыши, когда пересчитывать рисунок, где поставить окно, как
// рисовать надпись, когда прятать окно. Вынесены сюда, чтобы проверяться тестами и на Linux, и в
// покрытии на Windows без рабочего стола.

// Коды сообщений Windows, нужные решениям (значения из WinUser.h).
const (
	waInactive = 0 // WA_INACTIVE в младшем слове wParam сообщения WM_ACTIVATE

	vkEscape = 0x1B
	vkReturn = 0x0D
	vkTab    = 0x09
	vkSpace  = 0x20
	vkShift  = 0x10
	vkLeft   = 0x25
	vkUp     = 0x26
	vkRight  = 0x27
	vkDown   = 0x28

	dtCenter     = 0x0001
	dtVCenter    = 0x0004
	dtWordBreak  = 0x0010
	dtSingleLine = 0x0020
	dtCalcRect   = 0x0400
	dtNoPrefix   = 0x0800
)

// cardKeyAct — что окно делает с клавишей.
type cardKeyAct int

const (
	keyIgnored cardKeyAct = iota // клавиша не табло: окно передаёт её дальше
	keyHide                      // Esc: закрыть
	keyMove                      // Tab и стрелки: фокус на соседнюю кнопку
	keyPress                     // Enter и пробел: нажать кнопку с фокусом
)

// cardKey решает, что делает клавиша vk на табло вида kind с подсветкой st. shift спрашивается
// только для Tab (Shift+Tab идёт назад). Возвращается новая подсветка: кольцо фокуса видно
// после первой клавиши.
func cardKey(vk uintptr, kind CountdownKind, st CardState, shift func() bool) (CardState, cardKeyAct) {
	order := FocusOrder(kind)
	st.FocusVisible = true
	switch vk {
	case vkEscape:
		return st, keyHide
	case vkTab, vkRight, vkDown, vkLeft, vkUp:
		back := vk == vkLeft || vk == vkUp
		if vk == vkTab {
			back = shift()
		}
		st.Focus = NextFocus(order, st.Focus, back)
		return st, keyMove
	case vkReturn, vkSpace:
		if !contains(order, st.Focus) {
			st.Focus = order[0]
		}
		return st, keyPress
	}
	return st, keyIgnored
}

// cardDown — кнопка мыши нажата над t: нажатие запоминается, и мышь захватывается (capture),
// чтобы отпускание пришло этому окну.
func cardDown(st CardState, t CardTarget) (CardState, bool) {
	if t == TargetNone {
		return st, false
	}
	st.Pressed = t
	return st, true
}

// cardUp — кнопка мыши отпущена над t: нажатие снимается; срабатывает только кнопка, над которой
// мышь и нажали, и отпустили.
func cardUp(st CardState, t CardTarget) (CardState, CardTarget) {
	pressed := st.Pressed
	st.Pressed = TargetNone
	if t != pressed {
		return st, TargetNone
	}
	return st, pressed // без нажатия это TargetNone: щелчка нет
}

// pointFromLParam достаёт точку окна из lParam сообщения мыши (координаты со знаком: за левым
// и верхним краем они отрицательные).
func pointFromLParam(lParam uintptr) (x, y int) {
	return int(int16(lParam & 0xFFFF)), int(int16((lParam >> 16) & 0xFFFF))
}

// lParamFromPoint упаковывает точку окна в lParam сообщения мыши.
func lParamFromPoint(x, y int) uintptr {
	return uintptr(uint32(y)<<16 | uint32(x)&0xFFFF)
}

func contains(order []CardTarget, t CardTarget) bool {
	for _, o := range order {
		if o == t {
			return true
		}
	}
	return false
}

// focusFor — фокус после смены вида: остаётся на месте, если кнопка есть и в новом виде, иначе
// переходит на первую.
func focusFor(kind CountdownKind, cur CardTarget) CardTarget {
	if order := FocusOrder(kind); !contains(order, cur) {
		return order[0]
	}
	return cur
}

// cardBaseKey — от чего зависит рисунок без текста: вид, подсветка, цвет. Цифр в ключе нет:
// секундная смена цифр рисунок не пересчитывает.
func cardBaseKey(v CountdownView, st CardState) string {
	return fmt.Sprintf("%d|%v|%d", v.Kind, st, v.Status)
}

// toBGRA переставляет красный и синий байты: DIB Windows хранит пиксель как B, G, R, A.
func toBGRA(img *image.RGBA, dst []byte) []byte {
	if len(dst) != len(img.Pix) {
		dst = make([]byte, len(img.Pix))
	}
	for i := 0; i+3 < len(img.Pix); i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3]
	}
	return dst
}

// cardOriginAt — левый верхний угол окна табло над значком anchor: карточку ставит PlacePanel,
// окно больше неё на поля под тень.
func cardOriginAt(l CardLayout, anchor, work Rect) (x, y int) {
	x, y = PlacePanel(anchor, work, l.Card.Dx(), l.Card.Dy())
	return x - l.Margin, y - l.Margin
}

// cardOriginCorner — левый верхний угол окна плашки-табло в правом нижнем углу рабочей области.
func cardOriginCorner(l CardLayout, work Rect, gap int) (x, y int) {
	x, y = PlaceCorner(work, l.Card.Dx(), l.Card.Dy(), gap)
	return x - l.Margin, y - l.Margin
}

// textRunFlags — флаги DrawText для надписи: без «&» как подчёркивания, выравнивание, одна
// строка по центру высоты или перенос по словам.
func textRunFlags(r TextRun) uintptr {
	flags := uintptr(dtNoPrefix)
	if r.Align == AlignCenter {
		flags |= dtCenter
	}
	if r.Wrap {
		return flags | dtWordBreak
	}
	return flags | dtSingleLine | dtVCenter
}

// centerRows ставит текст высотой h по центру места top..bottom; выше места не сдвигает.
func centerRows(top, bottom, h int32) (int32, int32) {
	if h < bottom-top {
		top += (bottom - top - h) / 2
		bottom = top + h
	}
	return top, bottom
}

// cardHandCursor: над кнопкой указатель рукой, пока команда службе не в пути.
func cardHandCursor(st CardState, busy bool) bool {
	return st.Hover != TargetNone && !busy
}

// shownTooLong: открытое окно показано дольше предельного срока limit (забытое закрывается само).
func shownTooLong(visible bool, shownAt, now time.Time, limit time.Duration) bool {
	return visible && now.Sub(shownAt) > limit
}

// hidesOnDeactivate: окно прячется, когда теряет активацию (wParam сообщения WM_ACTIVATE).
// Плашка стража (noActivate) активной не бывает и не прячется: иначе её спрятала бы игра.
func hidesOnDeactivate(noActivate bool, wParam uintptr) bool {
	return !noActivate && wParam&0xFFFF == waInactive
}

// canPress: нажатие доходит, если команда не в пути, нажата кнопка и есть кому её выполнить.
func canPress(busy bool, t CardTarget, hasHandler bool) bool {
	return !busy && t != TargetNone && hasHandler
}

// tooSoon: окно закрылось только что (щелчком по значку через потерю активации) и сразу не
// открывается: для человека это одно нажатие.
func tooSoon(hiddenAt, now time.Time, guard time.Duration) bool {
	return now.Sub(hiddenAt) < guard
}
