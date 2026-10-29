//go:build windows

package guard

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	thePlate     *plateWindow
	plateProcPtr uintptr
)

func init() { plateProcPtr = windows.NewCallback(plateProc) }

// plateWindow — текстовая плашка справа внизу (FR-9, ТЗ 8.2.2): «Осталась 1 минута: 0:59»
// и «Добавлено N минут». Кнопок нет: плашку 1 мин не закрыть (её сменяет экран блокировки или
// «Добавлено»). Окно не берёт фокус: WS_EX_NOACTIVATE, показ без активации, щелчок тоже не
// активирует его, поэтому игра остаётся в фокусе. Плашки 30/15/5 мин рисует общая карточка
// (systray.WarningCard). Что показать и когда убрать, решает Model; звук играет контроллер.
type plateWindow struct {
	hwnd, hInst  uintptr
	dpi          int
	fonts        fontSet
	surfaceBrush uintptr
	accentBrush  uintptr

	text    string
	key     string
	visible bool
	geom    rect

	textRect rect
}

func newPlateWindow() (*plateWindow, error) {
	hInst, _, _ := procGetModuleHandle.Call(0)
	p := &plateWindow{hInst: hInst, dpi: systemDPI()}
	p.surfaceBrush, _, _ = procCreateSolidBr.Call(colorSurface)
	p.accentBrush, _, _ = procCreateSolidBr.Call(colorAmber)
	p.fonts = newFontSet(p.dpi)
	thePlate = p
	cls, err := registerPlainClass("PCBlockGuardPlate", plateProcPtr, hInst, p.surfaceBrush)
	if err != nil {
		return nil, fmt.Errorf("RegisterClassEx(плашка): %w", err)
	}
	hwnd, _, err := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWin|wsExNoActivate,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(utf16("PC Block"))),
		wsPopup|wsClipChildren,
		0, 0, 10, 10,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowEx(плашка): %w", err)
	}
	p.hwnd = hwnd
	return p, nil
}

// apply показывает плашку по View.Warning или прячет её (nil).
func (p *plateWindow) apply(w *WarningView) {
	if w == nil {
		p.hide()
		return
	}
	if w.Text == p.key && p.visible {
		// Поверх полноэкранной игры: «поверх всех» подтверждается без активации каждый кадр.
		procSetWindowPos.Call(p.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
		return
	}
	p.key, p.text = w.Text, w.Text
	p.layout()
	invalidate(p.hwnd)
}

func (p *plateWindow) hide() {
	if !p.visible {
		return
	}
	p.visible, p.key = false, ""
	procShowWindow.Call(p.hwnd, swHide)
}

// layout считает размер по тексту и ставит окно в правый нижний угол рабочей области.
func (p *plateWindow) layout() {
	hdc, _, _ := procGetDC.Call(p.hwnd)
	defer procReleaseDC.Call(p.hwnd, hdc)
	s := func(px int) int32 { return Scaled(px, p.dpi) }
	width, pad, bar := s(400), s(18), s(6)
	textW := width - 2*pad - bar
	textH := measureHeight(hdc, p.fonts.body, p.text, textW)
	h := pad + textH + pad
	p.textRect = rect{bar + pad, pad, bar + pad + textW, pad + textH}
	wa := workArea()
	g := rect{wa.right - width - s(16), wa.bottom - h - s(16), wa.right - s(16), wa.bottom - s(16)}
	if p.visible && g == p.geom {
		return
	}
	p.geom, p.visible = g, true
	procSetWindowPos.Call(p.hwnd, hwndTopmost, uintptr(g.left), uintptr(g.top), uintptr(width), uintptr(h),
		swpNoActivate|swpShowWindow)
}

func (p *plateWindow) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	roundBox(hdc, rc, colorSurface, colorLine, 1, 0)
	bar := rect{rc.left + 1, rc.top + 1, rc.left + 1 + Scaled(6, p.dpi), rc.bottom - 1}
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&bar)), p.accentBrush)
	procSetBkMode.Call(hdc, transparent)
	drawWrapped(hdc, p.fonts.body, colorText, p.text, p.textRect, 0)
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

// close уничтожает окно и освобождает ресурсы.
func (p *plateWindow) close() {
	if p.hwnd != 0 {
		procDestroyWindow.Call(p.hwnd)
		p.hwnd = 0
	}
	p.fonts.free()
	for _, b := range []uintptr{p.surfaceBrush, p.accentBrush} {
		if b != 0 {
			procDeleteObject.Call(b)
		}
	}
	thePlate = nil
}

func plateProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if p := thePlate; p != nil && p.hwnd == hwnd {
		switch uint32(msg) {
		case wmPaint:
			p.paint(hwnd)
			return 0
		case wmMouseActivate:
			return maNoActivate
		}
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}
