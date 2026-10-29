//go:build windows

package guard

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Общие вызовы Win32 для окон стража (экран блокировки, плашка, скрытое окно сообщений).
// Базовые процедуры и константы лежат в lockwindow_windows.go.
var (
	procGetDC                    = user32.NewProc("GetDC")
	procReleaseDC                = user32.NewProc("ReleaseDC")
	procMoveWindow               = user32.NewProc("MoveWindow")
	procDestroyWindow            = user32.NewProc("DestroyWindow")
	procEnableWindow             = user32.NewProc("EnableWindow")
	procSetFocus                 = user32.NewProc("SetFocus")
	procGetFocus                 = user32.NewProc("GetFocus")
	procInvalidateRect           = user32.NewProc("InvalidateRect")
	procPostMessage              = user32.NewProc("PostMessageW")
	procSetWindowText            = user32.NewProc("SetWindowTextW")
	procGetWindowTextLength      = user32.NewProc("GetWindowTextLengthW")
	procGetWindowText            = user32.NewProc("GetWindowTextW")
	procIsDialogMessage          = user32.NewProc("IsDialogMessageW")
	procGetDlgCtrlID             = user32.NewProc("GetDlgCtrlID")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procSystemParametersInfo     = user32.NewProc("SystemParametersInfoW")
	procGetDpiForSystem          = user32.NewProc("GetDpiForSystem")
	procIsChild                  = user32.NewProc("IsChild")
	procDrawFocusRect            = user32.NewProc("DrawFocusRect")
	procSetBkColor               = gdi32.NewProc("SetBkColor")

	procRoundRect = gdi32.NewProc("RoundRect")
	procCreatePen = gdi32.NewProc("CreatePen")

	procBeep = kernel32.NewProc("Beep")
)

// Сообщения, стили и флаги, которых нет в lockwindow_windows.go.
const (
	wmSize            = 0x0005
	wmSetText         = 0x000C
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmMouseActivate   = 0x0021
	wmDrawItem        = 0x002B
	wmDisplayChange   = 0x007E
	wmCommand         = 0x0111
	wmCtlColorEdit    = 0x0133
	wmCtlColorStatic  = 0x0138
	wmApp             = 0x8000

	bmClick       = 0x00F5
	emSetLimitTxt = 0x00C5
	// EM_SETPASSWORDCHAR / EM_GETPASSWORDCHAR: точки в поле пароля; 0 — текст виден (промокод).
	emSetPasswordChar = 0x00CC
	emGetPasswordChar = 0x00D2
	enChange          = 0x0300
	bnClicked         = 0
	maNoActivate      = 3

	vkReturn = 0x0D

	wsClipChildren = 0x02000000
	wsTabStop      = 0x00010000
	wsDisabled     = 0x08000000
	wsExNoActivate = 0x08000000
	bsOwnerDraw    = 0x0000000B
	esPassword     = 0x0020
	esAutoHScroll  = 0x0080

	swpNoActivate = 0x0010
	swpShowWindow = 0x0040

	odsSelected = 0x0001
	odsDisabled = 0x0004
	odsFocus    = 0x0010

	dtVCenter   = 0x0004
	dtWordBreak = 0x0010
	dtCalcRect  = 0x0400
	dtNoPrefix  = 0x0800

	errClassExists = syscall.Errno(1410) // ERROR_CLASS_ALREADY_EXISTS
	spiGetWorkArea = 0x0030
	psSolid        = 0
	defaultCharset = 1
	clearType      = 5
)

// Цвета тёмного экрана блокировки сверх тех, что заданы в lockwindow_windows.go (ТЗ 8.0 п.9).
var (
	colorSurface  = rgb(0x1F, 0x23, 0x27) // поверхность: поле пароля, второстепенные кнопки, плашка
	colorLine     = rgb(0x3A, 0x41, 0x48) // тонкая линия на тёмной поверхности
	colorGraphite = rgb(0x2B, 0x31, 0x38) // главная кнопка (ТЗ 8.0 п.3)
	colorWhite    = rgb(0xFF, 0xFF, 0xFF)
	colorDimText  = rgb(0x6E, 0x76, 0x7E) // недоступная кнопка
	colorAmber    = rgb(0xA1, 0x5C, 0x00) // «осталось мало»: полоса плашки предупреждения
	// colorErrorText — красный для текста и рамки на тёмном фоне. Красный из палитры
	// (#B3261E) на #15181B даёт контраст около 2:1, поэтому на тёмном берётся его светлый тон.
	colorErrorText = rgb(0xF2, 0x8B, 0x82)
)

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

// ptrTo превращает адрес из lParam в указатель на структуру Windows. Перевод идёт через
// память, а не через unsafe.Pointer(uintptr), поэтому go vet не видит здесь подозрительного
// приведения; адрес живёт, пока обрабатывается сообщение.
func ptrTo[T any](addr uintptr) *T {
	return (*T)(*(*unsafe.Pointer)(unsafe.Pointer(&addr)))
}

func utf16(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
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

// makeFont создаёт шрифт Segoe UI высотой px пикселей.
func makeFont(px int32, weight int) uintptr {
	f, _, _ := procCreateFont.Call(
		uintptr(-px), 0, 0, 0, uintptr(weight), 0, 0, 0,
		defaultCharset, 0, 0, clearType, 0,
		uintptr(unsafe.Pointer(utf16("Segoe UI"))),
	)
	return f
}

// fontSet — шрифты одного окна, размеры уже с учётом dpi.
type fontSet struct {
	title, body, small, button uintptr
}

func newFontSet(dpi int) fontSet {
	return fontSet{
		title:  makeFont(Scaled(40, dpi), 600),
		body:   makeFont(Scaled(22, dpi), 400),
		small:  makeFont(Scaled(17, dpi), 400),
		button: makeFont(Scaled(20, dpi), 600),
	}
}

func (f fontSet) free() {
	for _, h := range []uintptr{f.title, f.body, f.small, f.button} {
		if h != 0 {
			procDeleteObject.Call(h)
		}
	}
}

// measureHeight возвращает высоту текста, разбитого по словам в ширину width.
func measureHeight(hdc, font uintptr, text string, width int32) int32 {
	old, _, _ := procSelectObject.Call(hdc, font)
	rc := rect{0, 0, width, 0}
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(utf16(text))), ^uintptr(0),
		uintptr(unsafe.Pointer(&rc)), dtCalcRect|dtWordBreak|dtCenter|dtNoPrefix)
	procSelectObject.Call(hdc, old)
	return rc.bottom - rc.top
}

// drawWrapped рисует текст в прямоугольнике с переносом по словам; align это dtCenter или 0
// (по левому краю).
func drawWrapped(hdc, font, color uintptr, text string, rc rect, align uintptr) {
	old, _, _ := procSelectObject.Call(hdc, font)
	procSetTextColor.Call(hdc, color)
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(utf16(text))), ^uintptr(0),
		uintptr(unsafe.Pointer(&rc)), align|dtWordBreak|dtNoPrefix)
	procSelectObject.Call(hdc, old)
}

// roundBox рисует прямоугольник со скруглёнными углами: заливка, рамка, радиус в пикселях.
func roundBox(hdc uintptr, rc rect, fill, line uintptr, lineW, radius int32) {
	brush, _, _ := procCreateSolidBr.Call(fill)
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

// buttonLook — вид кнопки, которую рисует окно само.
type buttonLook struct {
	fill, line, text uintptr
	lineW            int32
}

// lookOf выбирает вид кнопки: главная графитовая с белым текстом (ТЗ 8.0 п.3), остальные на
// поверхности; нажатая светлее, недоступная серая.
func lookOf(main, enabled, pressed bool) buttonLook {
	switch {
	case !enabled:
		return buttonLook{fill: colorBg, line: colorLine, text: colorDimText, lineW: 1}
	case main && pressed:
		return buttonLook{fill: colorLine, line: colorSecondary, text: colorWhite, lineW: 2}
	case main:
		return buttonLook{fill: colorGraphite, line: colorSecondary, text: colorWhite, lineW: 2}
	case pressed:
		return buttonLook{fill: colorGraphite, line: colorLine, text: colorText, lineW: 1}
	}
	return buttonLook{fill: colorSurface, line: colorLine, text: colorText, lineW: 1}
}

// drawButton рисует кнопку по DRAWITEMSTRUCT: подложка цвета родителя (back), скруглённый
// прямоугольник, подпись по центру, рамка фокуса при выборе с клавиатуры.
func drawButton(di *drawItemStruct, label string, font uintptr, look buttonLook, radius int32, back uintptr) {
	rc := di.rcItem
	procFillRect.Call(di.hdc, uintptr(unsafe.Pointer(&rc)), back)
	roundBox(di.hdc, rect{rc.left + 1, rc.top + 1, rc.right - 1, rc.bottom - 1}, look.fill, look.line, look.lineW, radius)
	old, _, _ := procSelectObject.Call(di.hdc, font)
	procSetBkMode.Call(di.hdc, transparent)
	procSetTextColor.Call(di.hdc, look.text)
	procDrawText.Call(di.hdc, uintptr(unsafe.Pointer(utf16(label))), ^uintptr(0),
		uintptr(unsafe.Pointer(&rc)), dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
	procSelectObject.Call(di.hdc, old)
	if di.itemState&odsFocus != 0 {
		focus := rect{rc.left + 5, rc.top + 5, rc.right - 5, rc.bottom - 5}
		procDrawFocusRect.Call(di.hdc, uintptr(unsafe.Pointer(&focus)))
	}
}

// createChild создаёт дочернее окно стандартного класса.
func createChild(parent, hInst uintptr, class, text string, style uintptr, id int, rc rect) uintptr {
	h, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(utf16(class))),
		uintptr(unsafe.Pointer(utf16(text))),
		wsChild|wsVisible|style,
		uintptr(rc.left), uintptr(rc.top), uintptr(rc.right-rc.left), uintptr(rc.bottom-rc.top),
		parent, uintptr(id), hInst, 0,
	)
	return h
}

func moveWindow(hwnd uintptr, rc rect) {
	procMoveWindow.Call(hwnd, uintptr(rc.left), uintptr(rc.top), uintptr(rc.right-rc.left), uintptr(rc.bottom-rc.top), 1)
}

func setFont(hwnd, font uintptr) { procSendMessage.Call(hwnd, wmSetFont, font, 1) }

func setWindowText(hwnd uintptr, s string) {
	procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(utf16(s))))
}

// windowText читает текст окна (поле пароля). Пароль остаётся в памяти ровно на время вызова
// службе; на диск и в журнал он не попадает.
func windowText(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLength.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	s := windows.UTF16ToString(buf)
	for i := range buf {
		buf[i] = 0
	}
	return s
}

func invalidate(hwnd uintptr) { procInvalidateRect.Call(hwnd, 0, 1) }

// forceForeground делает окно активным. Процесс системы в чужом сеансе не всегда может
// просто вызвать SetForegroundWindow: поток временно подключается к потоку окна, которое
// сейчас на переднем плане.
func forceForeground(hwnd uintptr) {
	fg, _, _ := procGetForegroundWin.Call()
	if fg == hwnd {
		return
	}
	cur := windows.GetCurrentThreadId()
	var fgThread uint32
	if fg != 0 {
		t, _, _ := procGetWindowThreadProcessID.Call(fg, 0)
		fgThread = uint32(t)
	}
	attached := false
	if fgThread != 0 && fgThread != cur {
		r, _, _ := procAttachThreadInput.Call(uintptr(cur), uintptr(fgThread), 1)
		attached = r != 0
	}
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWin.Call(hwnd)
	if attached {
		procAttachThreadInput.Call(uintptr(cur), uintptr(fgThread), 0)
	}
}

// workArea возвращает рабочую область основного монитора (без панели задач).
func workArea() rect {
	var rc rect
	r, _, _ := procSystemParametersInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&rc)), 0)
	if r == 0 {
		w, _, _ := procGetSystemMetrics.Call(0)
		h, _, _ := procGetSystemMetrics.Call(1)
		return rect{0, 0, int32(w), int32(h)}
	}
	return rc
}

// registerPlainClass регистрирует простой класс окна со своей оконной процедурой.
func registerPlainClass(name string, proc, hInst, brush uintptr) (*uint16, error) {
	cls := utf16(name)
	cursor, _, _ := procLoadCursor.Call(0, uintptr(idcArrow))
	wc := wndClassEx{
		style:         0x0003,
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
			return cls, nil // класс уже зарегистрирован этим процессом
		}
		return nil, err
	}
	return cls, nil
}
