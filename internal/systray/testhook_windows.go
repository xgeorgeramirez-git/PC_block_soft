//go:build windows

package systray

import (
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Тестовый способ открыть табло без мыши (ТЗ 10.8, сценарий S34): в тестовом режиме значок
// слушает именованное событие Local\PCBlockTrayCard, приёмка его взводит, и табло открывается
// так же, как по щелчку на значке. Без ключа тестового режима в реестре события нет, и
// открыть табло извне нельзя. Ключ в HKLM пишет только администратор (ТЗ 10.8).
const (
	testModeKey   = `SOFTWARE\PCBlock\TestMode`
	testCardEvent = `Local\PCBlockTrayCard`

	// testHookRecheck: как часто значок смотрит, не включили ли тестовый режим.
	testHookRecheck = 2 * time.Second
)

func testModeOn() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, testModeKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	_ = k.Close()
	return true
}

// startTestHook запускает ожидание события в тестовом режиме; возвращает функцию остановки.
// Ключ тестового режима может появиться уже после запуска значка (приёмка включает его на
// установленной программе), поэтому наличие ключа проверяется, пока события нет.
func (a *app) startTestHook(stop <-chan struct{}) func() {
	done := make(chan struct{})
	var h windows.Handle
	go func() {
		defer close(done)
		defer func() { _ = recover() }()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if h == 0 {
				if testModeOn() {
					if ev, _ := windows.CreateEvent(nil, 0, 0, windows.StringToUTF16Ptr(testCardEvent)); ev != 0 {
						h = ev
					}
				}
				if h == 0 {
					select {
					case <-stop:
						return
					case <-time.After(testHookRecheck):
					}
					continue
				}
			}
			if ev, _ := windows.WaitForSingleObject(h, 500); ev == windows.WAIT_OBJECT_0 {
				procPostMessage.Call(a.host, wmTestOpenCard, 0, 0)
			}
		}
	}()
	return func() {
		<-done
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
	}
}
