//go:build windows

package winhost

import (
	"encoding/binary"
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

// notificationBytes возвращает копию первых 8 байт уведомления WTSSESSION_NOTIFICATION (cbSize и
// dwSessionId), на которое указывает EventData события SessionChange. nil: указателя нет или cbSize
// в самой структуре меньше 8. Единственное место с unsafe.
//
// Ruling P2-R7 (B-21): гарантии, что память ещё жива, нет. golang.org/x/sys/windows/svc (v0.48.0)
// копирует EventData как число в своём HandlerEx и возвращает управление диспетчеру служб раньше,
// чем запрос доходит до Execute; срок жизни lpEventData после возврата HandlerEx Microsoft не
// обещает (golang.org/issue/65832). Копия сразу окно гонки не закрывает (она всё равно после
// возврата), но читает ровно 8 байт, а не срез длиной cbSize поверх чужой памяти. Полное решение —
// свой HandlerEx (бэклог, M7).
func notificationBytes(eventData uintptr) []byte {
	if eventData == 0 {
		return nil
	}
	p := *(*unsafe.Pointer)(unsafe.Pointer(&eventData)) // uintptr от системы, не от Go: vet не жалуется
	const size = 8
	if cb := *(*uint32)(p); cb < size {
		return nil
	}
	b := make([]byte, size)
	copy(b, unsafe.Slice((*byte)(p), size))
	// Длина структуры проверена здесь, дальше — только номер: sessionIDOf сверяет cbSize с длиной копии.
	binary.LittleEndian.PutUint32(b, size)
	return b
}
