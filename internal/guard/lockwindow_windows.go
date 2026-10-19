//go:build windows

package guard

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Ленивые процедуры Win32 (Ruling 3: напрямую через x/sys/windows, без сторонних
// библиотек окон). user32/gdi32 — рисование и сообщения, kernel32 — модуль.
var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassEx    = user32.NewProc("RegisterClassExW")
	procCreateWindowEx     = user32.NewProc("CreateWindowExW")
	procDefWindowProc      = user32.NewProc("DefWindowProcW")
	procShowWindow         = user32.NewProc("ShowWindow")
	procUpdateWindow       = user32.NewProc("UpdateWindow")
	procGetMessage         = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessage    = user32.NewProc("DispatchMessageW")
	procSendMessage        = user32.NewProc("SendMessageW")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procSetWindowPos       = user32.NewProc("SetWindowPos")
	procGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	procSetForegroundWin   = user32.NewProc("SetForegroundWindow")
	procGetForegroundWin   = user32.NewProc("GetForegroundWindow")
	procIsWindowVisible    = user32.NewProc("IsWindowVisible")
	procSetTimer           = user32.NewProc("SetTimer")
	procKillTimer          = user32.NewProc("KillTimer")
	procBeginPaint         = user32.NewProc("BeginPaint")
	procEndPaint           = user32.NewProc("EndPaint")
	procFillRect           = user32.NewProc("FillRect")
	procGetClientRect      = user32.NewProc("GetClientRect")
	procDrawText           = user32.NewProc("DrawTextW")
	procLoadCursor         = user32.NewProc("LoadCursorW")
	procSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	procSetWindowsHookEx   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHook  = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx     = user32.NewProc("CallNextHookEx")

	procSetBkMode     = gdi32.NewProc("SetBkMode")
	procSetTextColor  = gdi32.NewProc("SetTextColor")
	procCreateSolidBr = gdi32.NewProc("CreateSolidBrush")
	procCreateFont    = gdi32.NewProc("CreateFontW")
	procSelectObject  = gdi32.NewProc("SelectObject")
	procDeleteObject  = gdi32.NewProc("DeleteObject")

	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
)

// Сообщения и константы Win32.
const (
	wmDestroy    = 0x0002
	wmClose      = 0x0010
	wmPaint      = 0x000F
	wmSetFont    = 0x0030
	wmKeyDown    = 0x0100
	wmSysCommand = 0x0112
	wmTimer      = 0x0113
	scClose      = 0xF060
	vkEscape     = 0x1B
	vkTab        = 0x09
	vkF4         = 0x73
	vkLWin       = 0x5B
	vkRWin       = 0x5C

	wsPopup     = 0x80000000
	wsVisible   = 0x10000000
	wsChild     = 0x40000000
	wsExTopmost = 0x00000008
	wsExToolWin = 0x00000080

	swHide        = 0
	swShow        = 5
	swpNoMove     = 0x0002
	swpNoSize     = 0x0001
	hwndTopmost   = ^uintptr(0) // (HWND)-1
	smXVirtScreen = 76
	smYVirtScreen = 77
	smCXVirtScr   = 78
	smCYVirtScr   = 79

	dtCenter     = 0x0001
	dtSingleLine = 0x0020
	transparent  = 1
	whKeyboardLL = 13
	llkhfAltDown = 0x20
	idcArrow     = 32512
)

func rgb(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }

// Цвета тёмного экрана блокировки (ТЗ 8.0 п.9).
var (
	colorBg        = rgb(0x15, 0x18, 0x1B)
	colorText      = rgb(0xEE, 0xF1, 0xF4)
	colorSecondary = rgb(0xA3, 0xAB, 0xB3)
)

type rect struct{ left, top, right, bottom int32 }

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
	pt      struct{ x, y int32 }
}

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

// kbdllHookStruct — данные низкоуровневого хука клавиатуры. Используем только
// vkCode и flags для решения «глушить/пропустить»; нажатия никуда не пишем (R23).
type kbdllHookStruct struct {
	vkCode      uint32
	scanCode    uint32
	flags       uint32
	time        uint32
	dwExtraInfo uintptr
}

// LockWindow — нативное окно экрана блокировки.
type LockWindow struct {
	hwnd     uintptr
	hInst    uintptr
	bgBrush  uintptr
	font     uintptr
	hook     uintptr
	view     LockView
	hookDone bool

	// onMsg получает сообщения, которые окно само не разбирает (кнопки, рисование кнопок,
	// цвета поля пароля, смена разрешения): lockscreen_windows.go. Закрытие, Esc и Alt+F4
	// окно отклоняет раньше и сюда не передаёт. onPaint рисует содержимое вместо заглушки.
	onMsg   func(msg uint32, wParam, lParam uintptr) (uintptr, bool)
	onPaint func(hdc uintptr, rc *rect)
}

var (
	active        *LockWindow
	wndProcPtr    uintptr
	hookProcPtr   uintptr
	classReg      bool
	classMu       sync.Mutex
	classNamePtr  *uint16
	buttonClass   *uint16
	hookInstalled bool
)

func init() {
	classNamePtr, _ = windows.UTF16PtrFromString("PCBlockLockWindow")
	buttonClass, _ = windows.UTF16PtrFromString("BUTTON")
	wndProcPtr = windows.NewCallback(wndProc)
	hookProcPtr = windows.NewCallback(lowLevelKeyboardProc)
}

// NewLockWindow создаёт и показывает окно на весь виртуальный экран, поверх всех
// окон (WS_EX_TOPMOST). WndProc не закрывает окно на WM_CLOSE, SC_CLOSE, Esc.
func NewLockWindow(v LockView) (*LockWindow, error) {
	procSetProcessDPIAware.Call()

	hInst, _, _ := procGetModuleHandle.Call(0)
	cursor, _, _ := procLoadCursor.Call(0, uintptr(idcArrow))
	brush, _, _ := procCreateSolidBr.Call(colorBg)

	w := &LockWindow{hInst: hInst, bgBrush: brush, view: v}
	active = w

	if err := registerClass(hInst, cursor, brush); err != nil {
		return nil, err
	}

	x, _, _ := procGetSystemMetrics.Call(smXVirtScreen)
	y, _, _ := procGetSystemMetrics.Call(smYVirtScreen)
	cx, _, _ := procGetSystemMetrics.Call(smCXVirtScr)
	cy, _, _ := procGetSystemMetrics.Call(smCYVirtScr)

	title, _ := windows.UTF16PtrFromString("PC Block")
	hwnd, _, err := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWin,
		uintptr(unsafe.Pointer(classNamePtr)),
		uintptr(unsafe.Pointer(title)),
		wsPopup|wsVisible|wsClipChildren,
		x, y, cx, cy,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowEx: %w", err)
	}
	w.hwnd = hwnd
	w.font = createFont()

	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	procSetForegroundWin.Call(hwnd)
	procSetTimer.Call(hwnd, 1, 500, 0) // раз в 500 мс возвращать окно наверх
	return w, nil
}

func registerClass(hInst, cursor, brush uintptr) error {
	classMu.Lock()
	defer classMu.Unlock()
	if classReg {
		return nil
	}
	wc := wndClassEx{
		style:         0x0003, // CS_HREDRAW|CS_VREDRAW
		lpfnWndProc:   wndProcPtr,
		hInstance:     hInst,
		hCursor:       cursor,
		hbrBackground: brush,
		lpszClassName: classNamePtr,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	atom, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return fmt.Errorf("RegisterClassEx: %w", err)
	}
	classReg = true
	return nil
}

func createFont() uintptr {
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	// Высота -40 (логические пиксели), остальное по умолчанию.
	f, _, _ := procCreateFont.Call(
		^uintptr(0)-39, 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(face)),
	)
	return f
}

// InstallKeyboardGuard ставит низкоуровневый перехват клавиатуры, глушащий
// Win, Alt+Tab, Alt+F4, Esc (и Ctrl+Esc / Ctrl+Shift+Esc как Esc), пока показан
// экран. Возвращает ошибку, если хук не установился.
func (w *LockWindow) InstallKeyboardGuard() error {
	h, _, err := procSetWindowsHookEx.Call(whKeyboardLL, hookProcPtr, w.hInst, 0)
	if h == 0 {
		return fmt.Errorf("SetWindowsHookEx(WH_KEYBOARD_LL): %w", err)
	}
	w.hook = h
	w.hookDone = true
	hookInstalled = true
	return nil
}

// KeyboardGuardInstalled сообщает, установлен ли перехват клавиатуры.
func (w *LockWindow) KeyboardGuardInstalled() bool { return w.hookDone }

// RemoveKeyboardGuard снимает перехват клавиатуры: сочетания Windows снова работают. Зовётся,
// когда экран спрятан (родительская сессия, нет связи со службой): глушить клавиши без экрана
// нельзя (R23, FR-6).
func (w *LockWindow) RemoveKeyboardGuard() {
	if w.hook != 0 {
		procUnhookWindowsHook.Call(w.hook)
		w.hook = 0
	}
	w.hookDone = false
	hookInstalled = false
}

// Hwnd возвращает ручку окна.
func (w *LockWindow) Hwnd() uintptr { return w.hwnd }

// Show показывает спрятанное окно и возвращает таймер «поверх всех окон».
func (w *LockWindow) Show() {
	procShowWindow.Call(w.hwnd, swShow)
	procSetWindowPos.Call(w.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize)
	procSetTimer.Call(w.hwnd, 1, 500, 0)
}

// Hide прячет окно и останавливает таймер: спрятанное окно не должно возвращаться наверх.
func (w *LockWindow) Hide() {
	procKillTimer.Call(w.hwnd, 1)
	procShowWindow.Call(w.hwnd, swHide)
}

// Visible сообщает, видно ли окно.
func (w *LockWindow) Visible() bool { return w.visible() }

// FitVirtualScreen растягивает окно на весь виртуальный экран; зовётся при смене разрешения
// или набора мониторов.
func (w *LockWindow) FitVirtualScreen() {
	x, _, _ := procGetSystemMetrics.Call(smXVirtScreen)
	y, _, _ := procGetSystemMetrics.Call(smYVirtScreen)
	cx, _, _ := procGetSystemMetrics.Call(smCXVirtScr)
	cy, _, _ := procGetSystemMetrics.Call(smCYVirtScr)
	procSetWindowPos.Call(w.hwnd, hwndTopmost, x, y, cx, cy, 0)
}

// SelfTest посылает окну WM_CLOSE, WM_SYSCOMMAND/SC_CLOSE и WM_KEYDOWN VK_ESCAPE
// и после каждого проверяет IsWindowVisible. Вызывается из отдельной горутины,
// пока Run крутит цикл сообщений на своём потоке.
func (w *LockWindow) SelfTest() SelfTestResult {
	var r SelfTestResult
	procSendMessage.Call(w.hwnd, wmClose, 0, 0)
	r.VisibleAfterWMClose = w.visible()
	procSendMessage.Call(w.hwnd, wmSysCommand, scClose, 0)
	r.VisibleAfterSCClose = w.visible()
	procSendMessage.Call(w.hwnd, wmKeyDown, vkEscape, 0)
	r.VisibleAfterEsc = w.visible()
	r.Passed = r.VisibleAfterWMClose && r.VisibleAfterSCClose && r.VisibleAfterEsc
	return r
}

func (w *LockWindow) visible() bool {
	r, _, _ := procIsWindowVisible.Call(w.hwnd)
	return r != 0
}

// IsForeground сообщает, является ли окно активным (передний план).
func (w *LockWindow) IsForeground() bool {
	fg, _, _ := procGetForegroundWin.Call()
	return fg == w.hwnd
}

// Run крутит цикл сообщений окна. Должен вызываться на потоке ОС, на котором
// создано окно и установлен хук (runtime.LockOSThread у вызывающего).
func (w *LockWindow) Run() {
	var m tagMSG
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break // 0 = WM_QUIT, -1 = ошибка
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// Close снимает хук и закрывает окно (используется администратором/службой).
func (w *LockWindow) Close() {
	if w.hook != 0 {
		procUnhookWindowsHook.Call(w.hook)
		w.hook = 0
	}
	if w.font != 0 {
		procDeleteObject.Call(w.font)
	}
	if w.bgBrush != 0 {
		procDeleteObject.Call(w.bgBrush)
	}
}

func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case wmClose:
		return 0 // экран блокировки не закрывается по WM_CLOSE
	case wmSysCommand:
		if wParam&0xFFF0 == scClose {
			return 0 // Alt+F4 / системное «Закрыть» не закрывают окно
		}
	case wmKeyDown:
		if wParam == vkEscape {
			return 0 // Esc не закрывает окно
		}
	case wmTimer:
		if active == nil || active.hwnd != hwnd || !active.visible() {
			return 0 // спрятанное окно наверх не возвращается
		}
		procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize)
		procSetForegroundWin.Call(hwnd)
		return 0
	case wmPaint:
		paint(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	if active != nil && active.onMsg != nil {
		if r, handled := active.onMsg(uint32(msg), wParam, lParam); handled {
			return r
		}
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}

func paint(hwnd uintptr) {
	if active == nil {
		return
	}
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), active.bgBrush)
	procSetBkMode.Call(hdc, transparent)
	if active.onPaint != nil {
		active.onPaint(hdc, &rc)
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return
	}
	old, _, _ := procSelectObject.Call(hdc, active.font)

	drawCentered(hdc, &rc, active.view.Reason, colorText, 0.30)
	if active.view.Detail != "" {
		drawCentered(hdc, &rc, active.view.Detail, colorSecondary, 0.38)
	}
	drawCentered(hdc, &rc, active.view.Until, colorSecondary, 0.44)
	drawCentered(hdc, &rc, active.view.Used, colorSecondary, 0.50)

	procSelectObject.Call(hdc, old)
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

func drawCentered(hdc uintptr, full *rect, s string, color uintptr, frac float64) {
	if s == "" {
		return
	}
	procSetTextColor.Call(hdc, color)
	h := full.bottom - full.top
	y := full.top + int32(float64(h)*frac)
	band := rect{full.left, y, full.right, y + 64}
	p, _ := windows.UTF16PtrFromString(s)
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(p)), ^uintptr(0),
		uintptr(unsafe.Pointer(&band)), dtCenter|dtSingleLine)
}

// lowLevelKeyboardProc — колбэк WH_KEYBOARD_LL. lParam объявлен unsafe.Pointer
// (Windows передаёт указатель на KBDLLHOOKSTRUCT): так обходится предупреждение
// vet о приведении uintptr к указателю. Нажатия никуда не записываются (R23).
func lowLevelKeyboardProc(nCode, wParam uintptr, lParam unsafe.Pointer) uintptr {
	if nCode == 0 && hookInstalled { // HC_ACTION
		k := (*kbdllHookStruct)(lParam)
		alt := k.flags&llkhfAltDown != 0
		switch k.vkCode {
		case vkLWin, vkRWin, vkEscape:
			return 1 // проглотить
		case vkTab:
			if alt {
				return 1
			}
		case vkF4:
			if alt {
				return 1
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, nCode, wParam, uintptr(lParam))
	return r
}
