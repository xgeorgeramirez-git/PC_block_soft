//go:build windows

package winhost

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"

	"pcblock/internal/ipc"
	"pcblock/internal/service"
)

// RunService подключается к диспетчеру служб и работает, пока служба не остановлена.
// Вызывается ключом --service, с которым диспетчер запускает программу.
func RunService() error {
	err := svc.Run(ipc.ServiceName, serviceHandler{})
	if errors.Is(err, windows.ERROR_FAILED_SERVICE_CONTROLLER_CONNECT) {
		return fmt.Errorf("ключ --service запускает диспетчер служб Windows, а не человек: службу ставят ключом --install-service")
	}
	return err
}

// serviceHandler — svc.Handler: тонкая обёртка, вся логика в host и internal/service.
type serviceHandler struct{}

// Execute запускает службу и отвечает на команды диспетчера служб.
func (serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptSessionChange

	status <- svc.Status{State: svc.StartPending, WaitHint: 20000}
	h, err := openHost()
	if err != nil {
		return true, 1 // сбой запуска: диспетчер служб посчитает его сбоем и запустит службу снова
	}
	status <- svc.Status{State: svc.Running, Accepts: accepted}

	for req := range requests {
		switch req.Cmd {
		case svc.Interrogate:
			status <- req.CurrentStatus
		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending, WaitHint: 20000}
			h.stop()
			return false, 0
		case svc.SessionChange:
			if ev, ok := sessionEventOf(req.EventType); ok {
				id, ok := sessionIDOf(notificationBytes(req.EventData))
				if !ok {
					id = service.SessionUnknown
					h.logf("Событие сеанса %v без номера сеанса: служба сверит весь список сеансов", ev)
				}
				h.onSession(ev, id)
			}
		}
	}
	h.stop()
	return false, 0
}

// notificationBytes возвращает данные уведомления WTSSESSION_NOTIFICATION, на которые указывает
// EventData события SessionChange (указатель действителен, пока обрабатывается запрос). nil:
// указателя нет или cbSize в самой структуре меньше её размера. Единственное место с unsafe.
func notificationBytes(eventData uintptr) []byte {
	if eventData == 0 {
		return nil
	}
	p := *(*unsafe.Pointer)(unsafe.Pointer(&eventData)) // uintptr от системы, не от Go: vet не жалуется
	const size = 8
	cb := *(*uint32)(p)
	if cb < size {
		return nil
	}
	return unsafe.Slice((*byte)(p), cb)
}
