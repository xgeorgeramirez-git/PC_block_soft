package guard

import (
	"errors"
	"time"

	"pcblock/internal/ipc"
)

// Закрытие вкладки и окна Brave (RM4-2, RM4-1). Решение «закрыть» принимает служба по снимку
// датчиков; страж исполняет его только после того, как заново прочитал передний план (TabActor.Look)
// и увидел там то же окно Brave с тем же доменом. Здесь нет Windows: исполнитель — winTabs
// (tabs_windows.go), в тестах — подделка.

// TabLook — передний план в момент действия, прочитанный заново (не из снимка службе).
type TabLook struct {
	Window uintptr // окно переднего плана; 0 — нет
	Brave  bool    // настоящий путь окна — brave.exe (isBravePath), не подмена TestForeground
	Domain string  // домен активной вкладки (после подмены TestDomain в тестовом режиме)
	Unread bool    // адрес не прочитан (domainMemo.Read)
	// NoAddress — адрес не прочитан, потому что у окна его нет (поле адреса и Document не нашлись,
	// адрес документа пуст), а не из-за сбоя UIA. Только такое окно закрывается WM_CLOSE (PL-9).
	NoAddress bool
	Tabs      int // вкладок в окне (UIA TabItem вне Document); 0 — не посчитаны
}

// TabActor читает настоящий передний план и закрывает вкладку или окно Brave.
type TabActor interface {
	Look() TabLook
	// Close — way: ipc.CloseTabCtrlW, ipc.CloseTabInvoke или ipc.CloseTabWMClose; window — окно из Look.
	Close(way string, window uintptr) error
}

// errModifierHeld — зажат Shift, Alt или Win: Ctrl+W стал бы другим сочетанием (Ctrl+Shift+W
// закрывает всё окно), шаг откладывается до следующего опроса и не засчитывается.
var errModifierHeld = errors.New("зажата клавиша-модификатор")

// tabRetryAfter — через сколько после шага лестницы, если вкладка того же домена ещё открыта и
// вкладок не стало меньше, пробуется следующий шаг (RM4-2); раньше — ничего: Ctrl+W мог ещё не
// дойти, и второй закрыл бы соседнюю вкладку.
const tabRetryAfter = 2 * time.Second

// tabLadder — где лестница закрытия: окно и домен, шаг (1 ctrlW, 2 invoke, 3 wmClose; 0 — не
// начата), когда сделан шаг и сколько тогда было вкладок. Закрытие окна без адреса — domain «».
type tabLadder struct {
	window uintptr
	domain string
	step   int
	at     time.Time
	tabs   int
}

// ladderWays — способ каждого шага лестницы; дальше третьего — WM_CLOSE раз в tabRetryAfter.
var ladderWays = [...]string{1: ipc.CloseTabCtrlW, 2: ipc.CloseTabInvoke, 3: ipc.CloseTabWMClose}

// nextClose — что сделать с командой службы: domain — домен из closeTab («» — команды нет), closeWin —
// команда closeBraveWindow; look — передний план, прочитанный сейчас. way «» — ничего не делать;
// next — лестница после шага (контроллер сохраняет её, если Close не вернул errModifierHeld).
//
// Вкладка закрывается, только если на переднем плане Brave, адрес прочитан и домен вкладки тот же.
// Шаг повышается, только если прошло tabRetryAfter, а вкладок не стало меньше (не посчитаны —
// не стало); стало меньше — прошлое действие удалось, следующая вкладка того же сайта снова
// закрывается Ctrl+W. Окно без адреса (NoAddress, не сбой UIA) закрывается WM_CLOSE не чаще раза в
// tabRetryAfter.
func nextClose(l tabLadder, domain string, closeWin bool, look TabLook, now time.Time) (way string, next tabLadder) {
	if look.Window == 0 || !look.Brave {
		return "", l
	}
	if closeWin && look.Unread && look.NoAddress { // сбой UIA — ничего не закрывать (PL-9)
		if l.step > 0 && l.domain == "" && l.window == look.Window && now.Sub(l.at) < tabRetryAfter {
			return "", l
		}
		return ipc.CloseTabWMClose, tabLadder{window: look.Window, step: 3, at: now}
	}
	if domain == "" || look.Unread || look.Domain != domain {
		return "", l
	}
	same := l.step > 0 && l.window == look.Window && l.domain == domain
	if same && l.tabs > 0 && look.Tabs > 0 && look.Tabs < l.tabs {
		same = false
	}
	if !same {
		return ipc.CloseTabCtrlW, tabLadder{window: look.Window, domain: domain, step: 1, at: now, tabs: look.Tabs}
	}
	if now.Sub(l.at) < tabRetryAfter {
		return "", l
	}
	step := min(l.step+1, len(ladderWays)-1)
	return ladderWays[step], tabLadder{window: look.Window, domain: domain, step: step, at: now, tabs: look.Tabs}
}

// isSitePlate — плашка о сайте (RM4-9): уступает плашке 1 мин, сильнее плашки отдыха.
func isSitePlate(kind string) bool {
	switch kind {
	case ipc.BlockedSite, ipc.BlockedAdult, ipc.BlockedVideoOver, ipc.BlockedBrowser:
		return true
	}
	return false
}
