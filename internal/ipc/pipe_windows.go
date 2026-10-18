//go:build windows

package ipc

import (
	"errors"
	"io"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
)

// Общий слой канала: ввод-вывод с перекрытием (FILE_FLAG_OVERLAPPED), отмена через
// CancelIoEx. Блокирующие ConnectNamedPipe, ReadFile и WriteFile нельзя прервать, а они
// вешают остановку службы; здесь каждая операция ждёт результата вместе с остановкой и сроком.

var (
	errTimedOut = errors.New("ipc: время ожидания вышло")
	errStopped  = errors.New("ipc: остановлено")
)

// waitOverlapped ждёт завершения операции ov на ручке h. stop (может быть nil) или срок
// timeout (0 — без срока) отменяют её через CancelIoEx; тогда возвращаются errStopped или
// errTimedOut. Горутина-сторож завершается до возврата, поэтому запоздалая отмена не заденет
// следующую операцию с тем же ov.
func waitOverlapped(h windows.Handle, ov *windows.Overlapped, stop <-chan struct{}, timeout time.Duration) (uint32, error) {
	done := make(chan struct{})
	finished := make(chan struct{})
	var timedOut atomic.Bool
	go func() {
		defer close(finished)
		var expired <-chan time.Time
		if timeout > 0 {
			tm := time.NewTimer(timeout)
			defer tm.Stop()
			expired = tm.C
		}
		select {
		case <-stop:
			_ = windows.CancelIoEx(h, ov)
		case <-expired:
			timedOut.Store(true)
			_ = windows.CancelIoEx(h, ov)
		case <-done:
		}
	}()
	var n uint32
	err := windows.GetOverlappedResult(h, ov, &n, true)
	close(done)
	<-finished
	if errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
		if timedOut.Load() {
			return n, errTimedOut
		}
		return n, errStopped
	}
	return n, err
}

// pipeConn — один конец канала. Читает и пишет строго по очереди (запрос, ответ), поэтому
// одного события на соединение достаточно.
type pipeConn struct {
	h            windows.Handle
	ev           windows.Handle
	stop         <-chan struct{}
	readTimeout  time.Duration
	writeTimeout time.Duration
}

// newPipeConn берёт ручку h под управление: Close закроет и её.
func newPipeConn(h windows.Handle, stop <-chan struct{}) (*pipeConn, error) {
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	return &pipeConn{h: h, ev: ev, stop: stop}, nil
}

// Close закрывает ручки. Вызывать, когда операции на соединении не идут.
func (c *pipeConn) Close() error {
	if c.h != 0 {
		_ = windows.CloseHandle(c.h)
		c.h = 0
	}
	if c.ev != 0 {
		_ = windows.CloseHandle(c.ev)
		c.ev = 0
	}
	return nil
}

// io выполняет одну операцию чтения или записи с перекрытием.
func (c *pipeConn) io(p []byte, read bool, timeout time.Duration) (int, error) {
	if c.h == 0 {
		return 0, io.ErrClosedPipe
	}
	ov := new(windows.Overlapped)
	ov.HEvent = c.ev
	_ = windows.ResetEvent(c.ev)
	var n uint32
	var err error
	if read {
		err = windows.ReadFile(c.h, p, &n, ov)
	} else {
		err = windows.WriteFile(c.h, p, &n, ov)
	}
	if errors.Is(err, windows.ERROR_IO_PENDING) {
		n, err = waitOverlapped(c.h, ov, c.stop, timeout)
	}
	return int(n), err
}

// Read читает до len(p) байт. Закрытие другой стороны — io.EOF.
func (c *pipeConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n, err := c.io(p, true, c.readTimeout)
	switch {
	case err == nil && n == 0:
		return 0, io.EOF
	case isPipeClosed(err):
		return n, io.EOF
	}
	return n, err
}

// Write пишет p целиком. Закрытие другой стороны — io.ErrClosedPipe.
func (c *pipeConn) Write(p []byte) (int, error) {
	total := 0
	for len(p) > 0 {
		n, err := c.io(p, false, c.writeTimeout)
		total += n
		p = p[n:]
		if isPipeClosed(err) {
			return total, io.ErrClosedPipe
		}
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}

// isPipeClosed — другая сторона закрыла канал.
func isPipeClosed(err error) bool {
	return errors.Is(err, windows.ERROR_BROKEN_PIPE) ||
		errors.Is(err, windows.ERROR_PIPE_NOT_CONNECTED) ||
		errors.Is(err, windows.ERROR_NO_DATA) ||
		errors.Is(err, windows.ERROR_HANDLE_EOF)
}

// isGone — соединение потеряно: запрос, записанный в него, до службы не дошёл.
func isGone(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || isPipeClosed(err)
}
