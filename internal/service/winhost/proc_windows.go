//go:build windows

package winhost

import (
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

// proc — ручка запущенного службой процесса (service.Proc). Alive отвечает по самой ручке, а
// не по номеру процесса, поэтому номер, занятый другим процессом после выхода нашего, службу
// не обманет.
type proc struct {
	pid uint32

	mu sync.Mutex
	h  windows.Handle // 0: процесса уже нет, ручку открыть не удалось
}

// openProc открывает ручку только что запущенного процесса. Не открылась (процесс успел
// завершиться): возвращается «мёртвый» процесс, и служба увидит его смерть при ближайшей
// проверке, как и у любого другого упавшего процесса.
func openProc(pid uint32) *proc {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return &proc{pid: pid}
	}
	return &proc{pid: pid, h: h}
}

func (p *proc) PID() uint32 { return p.pid }

// Alive: процесс ещё работает (ручка не сигналит).
func (p *proc) Alive() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.h == 0 {
		return false
	}
	ev, err := windows.WaitForSingleObject(p.h, 0)
	return err == nil && ev == uint32(windows.WAIT_TIMEOUT)
}

// Terminate завершает процесс и ждёт до двух секунд, пока он исчезнет. Уже завершённый
// процесс ошибкой не считается.
func (p *proc) Terminate() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.h == 0 {
		return nil
	}
	if ev, err := windows.WaitForSingleObject(p.h, 0); err == nil && ev == windows.WAIT_OBJECT_0 {
		return nil
	}
	if err := windows.TerminateProcess(p.h, 1); err != nil {
		return fmt.Errorf("процесс %d не завершён: %w", p.pid, err)
	}
	_, _ = windows.WaitForSingleObject(p.h, 2000)
	return nil
}

// Close отпускает ручку; повторный вызов безопасен.
func (p *proc) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.h != 0 {
		_ = windows.CloseHandle(p.h)
		p.h = 0
	}
}
