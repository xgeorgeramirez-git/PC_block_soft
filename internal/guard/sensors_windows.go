//go:build windows

package guard

import (
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"pcblock/internal/ipc"
)

// Датчики сеанса (ТЗ R2, R3). Страж читает только три вещи: сколько прошло с последнего ввода,
// закрывает ли окно переднего плана весь монитор и не заблокирован ли сеанс. Заголовки окон,
// имена программ и нажатия не читаются и не хранятся (R23); перехват клавиатуры включается
// только на время экрана блокировки и ничего не записывает.
var (
	procGetLastInputInfo    = user32.NewProc("GetLastInputInfo")
	procGetClassName        = user32.NewProc("GetClassNameW")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procGetWindowLongPtr    = user32.NewProc("GetWindowLongPtrW")
	procMonitorFromWindow   = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	procIsIconic            = user32.NewProc("IsIconic")
	procOpenInputDesktop    = user32.NewProc("OpenInputDesktop")
	procCloseDesktop        = user32.NewProc("CloseDesktop")
	procGetUserObjectInfoW  = user32.NewProc("GetUserObjectInformationW")
	procGetTickCount64      = kernel32.NewProc("GetTickCount64")
	gwlStyle                = ^uintptr(15) // GWL_STYLE = -16
	monitorDefaultToNearest = uintptr(2)
	uoiName                 = uintptr(2)
	desktopReadObjects      = uintptr(0x0001)
)

type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

// ReadSensors читает датчики сеанса и собирает снимок для службы. speed это ускорение часов
// ядра в тестовом режиме (в обычном 1).
func ReadSensors(now time.Time, speed int) ipc.SensorSample {
	return BuildSample(now, readRaw(), speed)
}

func readRaw() RawSensors {
	return RawSensors{IdleMs: idleMs(), Fullscreen: foregroundIsFullscreen(), Locked: sessionLocked()}
}

// idleMs: сколько миллисекунд прошло с последнего ввода с клавиатуры или мыши в этом сеансе.
func idleMs() int64 {
	li := lastInputInfo{cbSize: uint32(unsafe.Sizeof(lastInputInfo{}))}
	if r, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&li))); r == 0 {
		return 0
	}
	t, _, _ := procGetTickCount64.Call()
	// dwTime это 32-битный счётчик тиков: разность считается по модулю 2^32.
	return int64(uint32(t) - li.dwTime)
}

func windowClass(hwnd uintptr) string {
	buf := make([]uint16, 128)
	n, _, _ := procGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// foregroundIsFullscreen: окно переднего плана закрывает весь монитор и без заголовка.
// Рабочий стол (Progman, WorkerW), свернутые, невидимые окна и окна самого стража не в счёт
// (Ruling 24, ТЗ R2 и D-24).
func foregroundIsFullscreen() bool {
	fg, _, _ := procGetForegroundWin.Call()
	if fg == 0 {
		return false
	}
	var pid uint32
	procGetWindowThreadProcessID.Call(fg, uintptr(unsafe.Pointer(&pid)))
	if pid == windows.GetCurrentProcessId() {
		return false
	}
	if IsDesktopClass(windowClass(fg)) {
		return false
	}
	if vis, _, _ := procIsWindowVisible.Call(fg); vis == 0 {
		return false
	}
	if ic, _, _ := procIsIconic.Call(fg); ic != 0 {
		return false
	}
	var wr rect
	if r, _, _ := procGetWindowRect.Call(fg, uintptr(unsafe.Pointer(&wr))); r == 0 {
		return false
	}
	hmon, _, _ := procMonitorFromWindow.Call(fg, monitorDefaultToNearest)
	mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if r, _, _ := procGetMonitorInfo.Call(hmon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return false
	}
	style, _, _ := procGetWindowLongPtr.Call(fg, gwlStyle)
	return IsFullscreenWindow(
		Rect{wr.left, wr.top, wr.right, wr.bottom},
		Rect{mi.rcMonitor.left, mi.rcMonitor.top, mi.rcMonitor.right, mi.rcMonitor.bottom},
		uint32(style),
	)
}

// sessionLocked: сейчас на экране не рабочий стол сеанса, а защищённый (экран входа Windows
// после блокировки). Страж работает от имени системы и может открыть рабочий стол ввода
// и узнать его имя; не получилось открыть, значит ничего не известно, и сеанс считается
// незаблокированным (лучше засчитать секунду, чем перестать считать совсем).
func sessionLocked() bool {
	h, _, _ := procOpenInputDesktop.Call(0, 0, desktopReadObjects)
	if h == 0 {
		return false
	}
	defer procCloseDesktop.Call(h)
	buf := make([]uint16, 64)
	var need uint32
	r, _, _ := procGetUserObjectInfoW.Call(h, uoiName, uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)*2), uintptr(unsafe.Pointer(&need)))
	if r == 0 {
		return false
	}
	return strings.EqualFold(windows.UTF16ToString(buf), "Winlogon")
}

// testModeKeyPath — ключ реестра тестового режима (ТЗ 10.8); его читает и служба.
const testModeKeyPath = `SOFTWARE\PCBlock\TestMode`

// testClockSpeed читает ускорение часов из ключа тестового режима. Нет ключа, значения или
// оно непонятное: 1. Писать в ключ может только администратор, поэтому ребёнок ускорить
// или замедлить учёт этим путём не может: для службы значение читается так же.
func testClockSpeed() int {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, testModeKeyPath, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return 1
	}
	defer k.Close()
	if n, _, err := k.GetIntegerValue("ClockSpeed"); err == nil {
		return ClampSpeed(int64(n))
	}
	if s, _, err := k.GetStringValue("ClockSpeed"); err == nil {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err == nil {
			return ClampSpeed(int64(n))
		}
	}
	return 1
}
