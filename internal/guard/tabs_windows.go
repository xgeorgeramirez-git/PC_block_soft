//go:build windows

package guard

import (
	"fmt"

	"golang.org/x/sys/windows"

	"pcblock/internal/ipc"
)

// winTabs — исполнитель закрытия вкладки и окна Brave (TabActor, RM4-2). Look читает настоящий
// передний план заново: окно, путь его exe (окна PC Block и стража — «»), домен активной вкладки и
// число вкладок через тот же braveReader, что и датчики (braveMu). Подмен тестового режима здесь
// нет: их накладывает контроллер.
type winTabs struct{}

func (winTabs) Look() TabLook {
	fg, _, _ := procGetForegroundWin.Call()
	look := TabLook{Window: fg}
	if fg == 0 || !isBravePath(foregroundPath(fg)) {
		return look
	}
	look.Brave = true
	look.Domain, look.Unread, look.NoAddress = braveDomain(windows.HWND(fg))
	if !look.Unread {
		look.Tabs = braveTabCount(windows.HWND(fg))
	}
	return look
}

// braveTabCount — число вкладок окна Brave hwnd; не посчитано — 0 (nextClose считает, что вкладок
// не стало меньше).
func braveTabCount(hwnd windows.HWND) int {
	braveMu.Lock()
	defer braveMu.Unlock()
	if braveRd == nil {
		braveRd = newBraveReader()
	}
	n, err := countTabs(braveRd, hwnd)
	if err != nil {
		return 0
	}
	return n
}

// Close закрывает вкладку или окно Brave window способом way. Ctrl+W уходит окну переднего плана,
// только если это всё ещё window (контроллер только что увидел там window); WM_CLOSE — PostMessage, а не SendMessage: зависший
// Brave не держит опрос стража.
func (winTabs) Close(way string, window uintptr) error {
	switch way {
	case ipc.CloseTabCtrlW:
		return closeActiveTab(window)
	case ipc.CloseTabInvoke:
		braveMu.Lock()
		defer braveMu.Unlock()
		if braveRd == nil {
			braveRd = newBraveReader()
		}
		return closeTabButton(braveRd, windows.HWND(window))
	case ipc.CloseTabWMClose:
		if r, _, err := procPostMessage.Call(window, wmClose, 0, 0); r == 0 {
			return fmt.Errorf("PostMessage(WM_CLOSE): %v", err)
		}
		return nil
	}
	return fmt.Errorf("неизвестный способ закрытия %q", way)
}
