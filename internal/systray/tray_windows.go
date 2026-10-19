//go:build windows

package systray

import (
	"fmt"
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NOTIFYICONDATAW (Windows Vista и новее, версия 4 уведомлений). Размер 976 байт на amd64
// проверяет тест.
type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32 // в объединении с uTimeout
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         windows.GUID
	hBalloonIcon     uintptr
}

// notifyIdentifier — NOTIFYICONIDENTIFIER для Shell_NotifyIconGetRect.
type notifyIdentifier struct {
	cbSize   uint32
	hWnd     uintptr
	uID      uint32
	guidItem windows.GUID
}

// iconInfo — ICONINFO для CreateIconIndirect.
type iconInfo struct {
	fIcon    uint32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

const (
	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4

	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04
	nifShowTip = 0x80

	notifyVersion4 = 4
	ninSelect      = 0x0400 // WM_USER: щелчок мышью по значку (версия 4)
	ninKeySelect   = 0x0401 // выбор значка с клавиатуры (версия 4)

	trayIconID = 1
	tipMaxChar = 127 // szTip вмещает 127 знаков и завершающий ноль
)

// trayIcon — значок в области уведомлений. Работает только в потоке окон.
type trayIcon struct {
	hwnd     uintptr
	callback uint32 // сообщение, которое оболочка шлёт окну при действиях со значком
	added    bool
	v4       bool
	key      string // что нарисовано сейчас: ничего не меняется, пока ключ тот же
}

func newTrayIcon(hwnd uintptr, callback uint32) *trayIcon {
	return &trayIcon{hwnd: hwnd, callback: callback}
}

func (t *trayIcon) data(flags uint32, hIcon uintptr, tip string) notifyIconData {
	var d notifyIconData
	d.cbSize = uint32(unsafe.Sizeof(d))
	d.hWnd = t.hwnd
	d.uID = trayIconID
	d.uFlags = flags
	d.uCallbackMessage = t.callback
	d.hIcon = hIcon
	units := windows.StringToUTF16(tip)
	if len(units) > tipMaxChar+1 {
		units = append(units[:tipMaxChar:tipMaxChar], 0)
	}
	copy(d.szTip[:], units)
	return d
}

func (t *trayIcon) notify(msg uintptr, d *notifyIconData) bool {
	r, _, _ := procShellNotifyIcon.Call(msg, uintptr(unsafe.Pointer(d)))
	return r != 0
}

// apply рисует значок и подсказку. Если значка в области уведомлений ещё нет (оболочка
// не запущена, перезапущена), пробует его добавить; не получилось, повторит при следующем
// вызове.
func (t *trayIcon) apply(d Display) {
	key := fmt.Sprintf("%s|%d|%s", d.Label, d.Status, d.Tooltip)
	if t.added && key == t.key {
		return
	}
	hIcon, err := makeIcon(d.Label, d.Status)
	if err != nil {
		return
	}
	defer procDestroyIcon.Call(hIcon) // оболочка копирует значок

	flags := uint32(nifMessage | nifIcon | nifTip | nifShowTip)
	nd := t.data(flags, hIcon, d.Tooltip)
	if t.added {
		if t.notify(nimModify, &nd) {
			t.key = key
			return
		}
		t.added = false // оболочка потеряла значок: добавим заново
	}
	if !t.notify(nimAdd, &nd) {
		return
	}
	t.added, t.key = true, key
	nd.uVersion = notifyVersion4
	t.v4 = t.notify(nimSetVersion, &nd)
}

// forget помечает значок потерянным: оболочка перезапустилась (сообщение TaskbarCreated).
func (t *trayIcon) forget() { t.added, t.key = false, "" }

// remove убирает значок из области уведомлений.
func (t *trayIcon) remove() {
	if !t.added {
		return
	}
	nd := t.data(0, 0, "")
	t.notify(nimDelete, &nd)
	t.added = false
}

// rect возвращает прямоугольник значка на экране (куда прижать панель).
func (t *trayIcon) rect() (Rect, bool) {
	if procShellNotifyIconGetRect.Find() != nil {
		return Rect{}, false
	}
	id := notifyIdentifier{hWnd: t.hwnd, uID: trayIconID}
	id.cbSize = uint32(unsafe.Sizeof(id))
	var rc rect
	hr, _, _ := procShellNotifyIconGetRect.Call(uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&rc)))
	if hr != 0 || rc.right <= rc.left || rc.bottom <= rc.top {
		return Rect{}, false
	}
	return rc.toRect(), true
}

// clicked: сообщение оболочки означает щелчок по значку (левой кнопкой или с клавиатуры). В версии 4
// событие лежит в младшем слове lParam; в старой версии приходит само сообщение мыши.
func (t *trayIcon) clicked(lParam uintptr) bool {
	ev := uint32(lParam & 0xFFFF)
	if t.v4 {
		return ev == ninSelect || ev == ninKeySelect || ev == wmContextMenu
	}
	return ev == wmLButtonUp || ev == wmRButtonUp
}

// makeIcon рисует значок функциями icon.go, упаковывает в ICO (PNG внутри) и достаёт из
// него картинку нужной стороны. Если система не приняла PNG, значок собирается из пикселей.
func makeIcon(label string, st Status) (uintptr, error) {
	px := int(metric(smCXSmIcon))
	if px <= 0 {
		px = size100
	}
	side := PickSide(px)
	ico, err := BuildICO(label, st)
	if err != nil {
		return 0, err
	}
	data, err := ICOImage(ico, side)
	if err != nil {
		return 0, err
	}
	h, _, _ := procCreateIconFromResourceEx.Call(
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 1, 0x00030000, uintptr(px), uintptr(px), 0)
	if h != 0 {
		return h, nil
	}
	return iconFromPixels(RenderIcon(label, st, side))
}

// iconFromPixels собирает значок из пикселей картинки: цветная часть с прозрачностью (BGRA)
// и пустая маска.
func iconFromPixels(img *image.RGBA) (uintptr, error) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	bgra := make([]byte, 0, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := img.RGBAAt(x, y) // уже умноженный на прозрачность
			bgra = append(bgra, p.B, p.G, p.R, p.A)
		}
	}
	color, _, _ := procCreateBitmap.Call(uintptr(w), uintptr(h), 1, 32, uintptr(unsafe.Pointer(&bgra[0])))
	if color == 0 {
		return 0, fmt.Errorf("CreateBitmap(цвет) не удался")
	}
	defer procDeleteObject.Call(color)
	mask := make([]byte, ((w+15)/16*2)*h)
	maskBmp, _, _ := procCreateBitmap.Call(uintptr(w), uintptr(h), 1, 1, uintptr(unsafe.Pointer(&mask[0])))
	if maskBmp == 0 {
		return 0, fmt.Errorf("CreateBitmap(маска) не удался")
	}
	defer procDeleteObject.Call(maskBmp)
	ii := iconInfo{fIcon: 1, hbmMask: maskBmp, hbmColor: color}
	hIcon, _, err := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	if hIcon == 0 {
		return 0, fmt.Errorf("CreateIconIndirect: %w", err)
	}
	return hIcon, nil
}
