//go:build windows

package winhost

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"

	"pcblock/internal/ipc"
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
				h.onSession(ev)
			}
		}
	}
	h.stop()
	return false, 0
}
