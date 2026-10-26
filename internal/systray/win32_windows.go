//go:build windows

package systray

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Вызовы Win32 для значка и мини-панели: напрямую через x/sys/windows, без сторонних
// библиотек окон (Ruling 3).
var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassEx          = user32.NewProc("RegisterClassExW")
	procCreateWindowEx           = user32.NewProc("CreateWindowExW")
	procDefWindowProc            = user32.NewProc("DefWindowProcW")
	procDestroyWindow            = user32.NewProc("DestroyWindow")
	procShowWindow               = user32.NewProc("ShowWindow")
	procGetMessage               = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessage          = user32.NewProc("DispatchMessageW")
	procIsDialogMessage          = user32.NewProc("IsDialogMessageW")
	procPostMessage              = user32.NewProc("PostMessageW")
	procPostQuitMessage          = user32.NewProc("PostQuitMessage")
	procSetTimer                 = user32.NewProc("SetTimer")
	procKillTimer                = user32.NewProc("KillTimer")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procMoveWindow               = user32.NewProc("MoveWindow")
	procGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procGetDC                    = user32.NewProc("GetDC")
	procReleaseDC                = user32.NewProc("ReleaseDC")
	procBeginPaint               = user32.NewProc("BeginPaint")
	procEndPaint                 = user32.NewProc("EndPaint")
	procFillRect                 = user32.NewProc("FillRect")
	procDrawText                 = user32.NewProc("DrawTextW")
	procLoadCursor               = user32.NewProc("LoadCursorW")
	procSetProcessDPIAware       = user32.NewProc("SetProcessDPIAware")
	procGetDpiForSystem          = user32.NewProc("GetDpiForSystem")
	procInvalidateRect           = user32.NewProc("InvalidateRect")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procSetFocus                 = user32.NewProc("SetFocus")
	procGetFocus                 = user32.NewProc("GetFocus")
	procRegisterWindowMessage    = user32.NewProc("RegisterWindowMessageW")
	procCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
	procCreateIconIndirect       = user32.NewProc("CreateIconIndirect")
	procDestroyIcon              = user32.NewProc("DestroyIcon")
	procIsChild                  = user32.NewProc("IsChild")
	procMonitorFromRect          = user32.NewProc("MonitorFromRect")
	procGetMonitorInfo           = user32.NewProc("GetMonitorInfoW")
	procSendMessage              = user32.NewProc("SendMessageW")
	procDrawFocusRect            = user32.NewProc("DrawFocusRect")
	procUpdateLayeredWindow      = user32.NewProc("UpdateLayeredWindow")
	procGetKeyState              = user32.NewProc("GetKeyState")
	procTrackMouseEvent          = user32.NewProc("TrackMouseEvent")
	procSetCapture               = user32.NewProc("SetCapture")
	procReleaseCapture           = user32.NewProc("ReleaseCapture")
	procSetCursor                = user32.NewProc("SetCursor")

	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procCreateFont       = gdi32.NewProc("CreateFontW")
	procSelectObject     = gdi32.NewProc("SelectObject")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procSetTextColor     = gdi32.NewProc("SetTextColor")
	procSetBkMode        = gdi32.NewProc("SetBkMode")
	procRoundRect        = gdi32.NewProc("RoundRect")
	procCreatePen        = gdi32.NewProc("CreatePen")
	procCreateBitmap     = gdi32.NewProc("CreateBitmap")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procGetTextFace        = gdi32.NewProc("GetTextFaceW")

	procShellNotifyIcon        = shell32.NewProc("Shell_NotifyIconW")
	procShellNotifyIconGetRect = shell32.NewProc("Shell_NotifyIconGetRect")

	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
)

// Сообщения, стили и флаги.
const (
	wmDestroy         = 0x0002
	wmActivate        = 0x0006
	wmPaint           = 0x000F
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmDrawItem        = 0x002B
	wmSetFont         = 0x0030
	wmContextMenu     = 0x007B
	wmSetCursor       = 0x0020
	wmKeyDown         = 0x0100
	wmCommand         = 0x0111
	wmTimer           = 0x0113
	wmMouseMove       = 0x0200
	wmLButtonDown     = 0x0201
	wmLButtonUp       = 0x0202
	wmMouseLeave      = 0x02A3
	wmRButtonUp       = 0x0205
	wmApp             = 0x8000

	waInactive = 0
	vkEscape   = 0x1B
	vkReturn   = 0x0D
	vkTab      = 0x09
	vkSpace    = 0x20
	vkShift    = 0x10
	vkLeft     = 0x25
	vkUp       = 0x26
	vkRight    = 0x27
	vkDown     = 0x28

	wsPopup        = 0x80000000
	wsVisible      = 0x10000000
	wsChild        = 0x40000000
	wsClipChildren = 0x02000000
	wsTabStop      = 0x00010000
	wsExTopmost    = 0x00000008
	wsExToolWin    = 0x00000080
	wsExLayered    = 0x00080000
	bsOwnerDraw    = 0x0000000B
	bnClicked      = 0

	swHide      = 0
	swShow      = 5
	swpNoMove   = 0x0002
	swpNoSize   = 0x0001
	swpNoActive = 0x0010
	hwndTopmost = ^uintptr(0) // (HWND)-1: поверх всех окон

	smCXSmIcon = 49
	idcArrow   = 32512

	odsSelected = 0x0001
	odsFocus    = 0x0010

	dtCenter     = 0x0001
	dtVCenter    = 0x0004
	dtWordBreak  = 0x0010
	dtSingleLine = 0x0020
	dtCalcRect   = 0x0400
	dtNoPrefix   = 0x0800
	transparent  = 1
	psSolid      = 0

	defaultCharset = 1
	clearType      = 5

	monitorDefaultToNearest = 2

	errClassExists = syscall.Errno(1410) // ERROR_CLASS_ALREADY_EXISTS
)

type rect struct{ left, top, right, bottom int32 }

func (r rect) toRect() Rect { return Rect{int(r.left), int(r.top), int(r.right), int(r.bottom)} }

type point struct{ x, y int32 }

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type tagMSG struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

// drawItemStruct — DRAWITEMSTRUCT: описание кнопки, которую рисует родитель (WM_DRAWITEM).
type drawItemStruct struct {
	ctlType    uint32
	ctlID      uint32
	itemID     uint32
	itemAction uint32
	itemState  uint32
	hwndItem   uintptr
	hdc        uintptr
	rcItem     rect
	itemData   uintptr
}

type monitorInfo struct {
	size    uint32
	monitor rect
	work    rect
	flags   uint32
}

func rgb(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }

// Светлая тема мини-панели (ТЗ 8.0 п.9).
var (
	uiSurface   = rgb(0xFF, 0xFF, 0xFF)
	uiLine      = rgb(0xD8, 0xDC, 0xE0)
	uiText      = rgb(0x1B, 0x1F, 0x23)
	uiSecondary = rgb(0x59, 0x61, 0x6A)
	uiGraphite  = rgb(0x2B, 0x31, 0x38)
	uiWhite     = rgb(0xFF, 0xFF, 0xFF)
	uiRed       = rgb(0xB3, 0x26, 0x1E)
)

// ptrTo превращает адрес из lParam в указатель на структуру Windows (go vet не видит здесь
// подозрительного приведения; адрес живёт, пока обрабатывается сообщение).
func ptrTo[T any](addr uintptr) *T {
	return (*T)(*(*unsafe.Pointer)(unsafe.Pointer(&addr)))
}

func wstr(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

func metric(index uintptr) int32 {
	v, _, _ := procGetSystemMetrics.Call(index)
	return int32(v)
}

// systemDPI возвращает dpi системы (96 при масштабе 100 %).
func systemDPI() int {
	if procGetDpiForSystem.Find() != nil {
		return 96
	}
	d, _, _ := procGetDpiForSystem.Call()
	if d < 96 || d > 960 {
		return 96
	}
	return int(d)
}

// scaled переводит пиксели при 96 dpi в пиксели при dpi системы.
func scaled(px, dpi int) int32 { return int32((px*dpi + 48) / 96) }

func makeFont(px int32, weight int) uintptr {
	f, _, _ := procCreateFont.Call(
		uintptr(-px), 0, 0, 0, uintptr(weight), 0, 0, 0,
		defaultCharset, 0, 0, clearType, 0,
		uintptr(unsafe.Pointer(wstr("Segoe UI"))),
	)
	return f
}

// measureHeight возвращает высоту текста, разбитого по словам в ширину width.
func measureHeight(hdc, font uintptr, text string, width int32) int32 {
	old, _, _ := procSelectObject.Call(hdc, font)
	rc := rect{0, 0, width, 0}
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(wstr(text))), ^uintptr(0),
		uintptr(unsafe.Pointer(&rc)), dtCalcRect|dtWordBreak|dtNoPrefix)
	procSelectObject.Call(hdc, old)
	return rc.bottom - rc.top
}

func drawWrapped(hdc, font, color uintptr, text string, rc rect) {
	old, _, _ := procSelectObject.Call(hdc, font)
	procSetBkMode.Call(hdc, transparent)
	procSetTextColor.Call(hdc, color)
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(wstr(text))), ^uintptr(0),
		uintptr(unsafe.Pointer(&rc)), dtWordBreak|dtNoPrefix)
	procSelectObject.Call(hdc, old)
}

// roundBox рисует прямоугольник со скруглёнными углами: заливка, рамка, радиус в пикселях.
func roundBox(hdc uintptr, rc rect, fill, line uintptr, lineW, radius int32) {
	brush, _, _ := procCreateSolidBrush.Call(fill)
	pen, _, _ := procCreatePen.Call(psSolid, uintptr(lineW), line)
	oldBrush, _, _ := procSelectObject.Call(hdc, brush)
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	procRoundRect.Call(hdc, uintptr(rc.left), uintptr(rc.top), uintptr(rc.right), uintptr(rc.bottom),
		uintptr(radius*2), uintptr(radius*2))
	procSelectObject.Call(hdc, oldBrush)
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
}

func fillSolid(hdc uintptr, rc rect, color uintptr) {
	brush, _, _ := procCreateSolidBrush.Call(color)
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), brush)
	procDeleteObject.Call(brush)
}

// createChild создаёт дочернее окно стандартного класса.
func createChild(parent, hInst uintptr, class, text string, style uintptr, id int, rc rect) uintptr {
	h, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(wstr(class))),
		uintptr(unsafe.Pointer(wstr(text))),
		wsChild|wsVisible|style,
		uintptr(rc.left), uintptr(rc.top), uintptr(rc.right-rc.left), uintptr(rc.bottom-rc.top),
		parent, uintptr(id), hInst, 0,
	)
	return h
}

// registerPlainClass регистрирует простой класс окна со своей оконной процедурой; повторная
// регистрация тем же процессом не ошибка.
func registerPlainClass(name string, proc, hInst, brush uintptr) (*uint16, error) {
	cls := wstr(name)
	cursor, _, _ := procLoadCursor.Call(0, uintptr(idcArrow))
	wc := wndClassEx{
		style:         0x0003, // CS_HREDRAW | CS_VREDRAW
		lpfnWndProc:   proc,
		hInstance:     hInst,
		hCursor:       cursor,
		hbrBackground: brush,
		lpszClassName: cls,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	atom, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		if errno, ok := err.(syscall.Errno); ok && errno == errClassExists {
			return cls, nil
		}
		return nil, err
	}
	return cls, nil
}

// monitorWork возвращает рабочую область монитора, на котором лежит прямоугольник.
func monitorWork(rc rect) rect {
	m, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&rc)), monitorDefaultToNearest)
	var mi monitorInfo
	mi.size = uint32(unsafe.Sizeof(mi))
	if m != 0 {
		if r, _, _ := procGetMonitorInfo.Call(m, uintptr(unsafe.Pointer(&mi))); r != 0 {
			return mi.work
		}
	}
	return rect{0, 0, metric(0), metric(1)}
}
