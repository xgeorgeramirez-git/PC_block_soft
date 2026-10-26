//go:build windows

package systray

import (
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
)

func testModeOn() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, testModeKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	_ = k.Close()
	return true
}

// startTestHook запускает ожидание события, если включён тестовый режим; возвращает функцию
// остановки.
func (a *app) startTestHook(stop <-chan struct{}) func() {
	if !testModeOn() {
		return func() {}
	}
	h, err := windows.CreateEvent(nil, 0, 0, windows.StringToUTF16Ptr(testCardEvent))
	if err != nil && h == 0 {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = recover() }()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if ev, _ := windows.WaitForSingleObject(h, 500); ev == windows.WAIT_OBJECT_0 {
				procPostMessage.Call(a.host, wmTestOpenCard, 0, 0)
			}
		}
	}()
	return func() {
		<-done
		_ = windows.CloseHandle(h)
	}
}
