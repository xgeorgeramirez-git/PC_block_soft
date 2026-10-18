//go:build windows

package ipc

import (
	"errors"
	"io"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Права клиента на канал: ровно то, что даёт интерактивным пользователям дескриптор канала.
// GENERIC_WRITE включает FILE_APPEND_DATA, то есть право создать экземпляр канала, и
// CreateFile отказал бы обычному пользователю.
const pipeClientAccess = windows.FILE_READ_DATA | windows.FILE_WRITE_DATA | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE

const defaultRequestTimeout = 10 * time.Second

// DialOptions — настройки клиента.
type DialOptions struct {
	// RequireSystemServer: до отправки данных проверить, что канал держит процесс системы
	// (Ruling 17). Во всей программе true (DefaultDialOptions); false только в тестах,
	// где сервер канала работает не от имени системы.
	RequireSystemServer bool
	// ServiceName — имя службы для запасной проверки владельца; пусто — ServiceName.
	ServiceName string
	// RequestTimeout — срок на запись запроса и чтение ответа; 0 — 10 с (L-57).
	RequestTimeout time.Duration
}

// DefaultDialOptions возвращает настройки для рабочей программы: владелец канала проверяется.
func DefaultDialOptions() DialOptions {
	return DialOptions{RequireSystemServer: true, ServiceName: ServiceName, RequestTimeout: defaultRequestTimeout}
}

// Client — соединение с службой. Запросы идут по одному (Do безопасен для вызова из
// нескольких горутин). После ошибки соединение сбрасывается, и следующий Do открывает его
// заново, снова проверив владельца канала.
type Client struct {
	name           string
	connectTimeout time.Duration
	opts           DialOptions

	stop     chan struct{} // закрывается в Close и прерывает ожидание
	stopOnce sync.Once

	mu     sync.Mutex
	conn   *pipeConn
	closed bool
}

// Dial открывает канал name, ожидая появления канала и свободного экземпляра не дольше
// timeout, и проверяет, что сервер канала работает от имени системы. Не нашёл канал —
// ошибка с видом ErrTimeout, чужой владелец — ErrNotService; текст обеих «Программа не
// отвечает», подробности в Error.Detail.
func Dial(name string, timeout time.Duration) (*Client, error) {
	return DialWith(name, timeout, DefaultDialOptions())
}

// DialWith — Dial с настройками. Нулевое значение DialOptions проверку владельца отключает:
// для рабочего кода берите DefaultDialOptions.
func DialWith(name string, timeout time.Duration, opts DialOptions) (*Client, error) {
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = defaultRequestTimeout
	}
	if opts.ServiceName == "" {
		opts.ServiceName = ServiceName
	}
	c := &Client{name: name, connectTimeout: timeout, opts: opts, stop: make(chan struct{})}
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	c.conn = conn
	return c, nil
}

func (c *Client) connect() (*pipeConn, error) {
	h, err := openPipe(c.name, time.Now().Add(c.connectTimeout), c.stop)
	if err != nil {
		return nil, err
	}
	if c.opts.RequireSystemServer {
		if err := verifySystemServer(h, c.opts.ServiceName); err != nil {
			_ = windows.CloseHandle(h)
			return nil, err
		}
	}
	conn, err := newPipeConn(h, c.stop)
	if err != nil {
		return nil, newError(ErrUnavailable, err)
	}
	conn.readTimeout, conn.writeTimeout = c.opts.RequestTimeout, c.opts.RequestTimeout
	return conn, nil
}

// openPipe открывает канал, пока не истечёт deadline: канала ещё нет (служба запускается)
// или все экземпляры заняты. Ручка открывается с перекрытием и уровнем SECURITY_IDENTIFICATION:
// служба может узнать, кто мы, но не может действовать от нашего имени.
func openPipe(name string, deadline time.Time, stop <-chan struct{}) (windows.Handle, error) {
	namep, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, newError(ErrUnavailable, err)
	}
	for {
		h, err := windows.CreateFile(namep, pipeClientAccess, 0, nil, windows.OPEN_EXISTING,
			windows.FILE_FLAG_OVERLAPPED|windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION, 0)
		if err == nil {
			return h, nil
		}
		busy := errors.Is(err, windows.ERROR_PIPE_BUSY)
		if !busy && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return 0, newError(ErrUnavailable, err)
		}
		left := time.Until(deadline)
		if left <= 0 {
			return 0, newError(ErrTimeout, err)
		}
		select {
		case <-stop:
			return 0, newError(ErrClosed, nil)
		default:
		}
		if busy {
			// Ждём освобождения экземпляра, короткими отрезками, чтобы уважать deadline и stop.
			waitNamedPipe(namep, min(left, 200*time.Millisecond))
		} else {
			select {
			case <-stop:
			case <-time.After(min(left, 50*time.Millisecond)):
			}
		}
	}
}

// waitNamedPipe ждёт, пока на канале появится слушающий экземпляр (WaitNamedPipeW).
func waitNamedPipe(namep *uint16, d time.Duration) {
	ms := uint32(d / time.Millisecond)
	if ms == 0 {
		ms = 1
	}
	start := time.Now()
	_, _, _ = procWaitNamedPipe.Call(uintptr(unsafe.Pointer(namep)), uintptr(ms))
	if time.Since(start) < time.Millisecond {
		// Вызов вернулся сразу (отказ, а не ожидание): не крутить процессор до срока.
		time.Sleep(10 * time.Millisecond)
	}
}

// Do отправляет запрос и ждёт ответ. Отказ службы приходит обычным Reply (OK = false);
// ошибка возвращается, только если связи нет: тогда это *Error с текстом «Программа не
// отвечает». Если соединение закрыто службой (например, простояло без запросов), запрос
// не доходил до неё, и Do один раз повторяет его на новом соединении. После обрыва на
// чтении запрос не повторяется: служба могла его выполнить.
func (c *Client) Do(m Msg) (Reply, error) {
	frame := Encode(m)
	if len(frame)-headerLen > MaxFrame {
		return Reply{}, ErrFrameTooLarge
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return Reply{}, newError(ErrClosed, nil)
	}
	for attempt := 0; ; attempt++ {
		if c.conn == nil {
			conn, err := c.connect()
			if err != nil {
				return Reply{}, err
			}
			c.conn = conn
		}
		if _, err := c.conn.Write(frame); err != nil {
			c.dropLocked()
			if attempt == 0 && isGone(err) {
				continue
			}
			return Reply{}, wrapIOError(err)
		}
		rep, err := ReadReply(c.conn)
		if err != nil {
			// Ответ мог опоздать или прийти битым: на этом соединении порядок кадров
			// уже не гарантирован, поэтому его закрываем.
			c.dropLocked()
			return Reply{}, wrapIOError(err)
		}
		return rep, nil
	}
}

// Close закрывает клиента; ожидающий Do прерывается.
func (c *Client) Close() error {
	c.stopOnce.Do(func() { close(c.stop) })
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.dropLocked()
	return nil
}

func (c *Client) dropLocked() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// wrapIOError превращает ошибку ввода-вывода в ошибку клиента с текстом для человека.
func wrapIOError(err error) error {
	var e *Error
	switch {
	case errors.As(err, &e):
		return err
	case errors.Is(err, errTimedOut):
		return newError(ErrTimeout, err)
	case errors.Is(err, errStopped):
		return newError(ErrClosed, err)
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.ErrClosedPipe):
		return newError(ErrUnavailable, err)
	}
	return newError(ErrUnavailable, err)
}
