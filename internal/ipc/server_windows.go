//go:build windows

package ipc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// pipeSDDL — дескриптор канала (Ruling 17, NFR-9): полный доступ у системы и администраторов,
// у интерактивных пользователей только чтение и запись без права создавать свой экземпляр
// канала (0x12019b = READ_CONTROL | SYNCHRONIZE | чтение и запись данных, атрибутов и
// расширенных атрибутов, без FILE_CREATE_PIPE_INSTANCE).
const pipeSDDL = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x12019b;;;IU)"

// ErrPipeTaken — имя канала уже занято: другой экземпляр службы или чужой процесс (тогда
// имя заняли раньше службы, это тревога для журнала).
var ErrPipeTaken = errors.New("ipc: имя канала уже занято")

// Настройки сервера. Переменные, чтобы тесты могли их ужать; меняются только до Serve.
var (
	idleTimeout  = 5 * time.Minute  // сколько соединение может молчать между запросами
	frameTimeout = 10 * time.Second // сколько даётся на тело запроса и на запись ответа
	maxConns     = 64               // одновременных соединений; остальные ждут в очереди (ERROR_PIPE_BUSY)
	stopWait     = 1500 * time.Millisecond
)

const pipeBufferSize = 64 * 1024

// Serve держит канал name (например, PipeName) и вызывает handler для каждого запроса,
// пока не отменён ctx. Каждое подключение обслуживается в своей горутине; handler может
// вызываться из нескольких горутин сразу и не должен паниковать (транспорт перехватывает
// панику только затем, чтобы служба осталась жива, и отвечает человеческой ошибкой).
// Права на команды handler проверяет сам по Peer.
//
// Serve возвращает nil при остановке по ctx (быстро: ожидание подключений и ввод-вывод
// прерываются CancelIoEx), ErrPipeTaken, если имя занято, иначе ошибку создания канала.
func Serve(ctx context.Context, name string, handler func(Peer, Msg) Reply) error {
	namep, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("ipc: имя канала: %w", err)
	}
	sd, err := windows.SecurityDescriptorFromString(pipeSDDL)
	if err != nil {
		return fmt.Errorf("ipc: дескриптор канала: %w", err)
	}
	sa := &windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(*sa))

	// Первый экземпляр с FILE_FLAG_FIRST_PIPE_INSTANCE: если имя уже занято, создание
	// не удастся (ERROR_ACCESS_DENIED), и чужой процесс не подсунет свой канал под нашим именем.
	cur, err := createInstance(namep, sa, true)
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_PIPE_BUSY) {
			return fmt.Errorf("%w: %v", ErrPipeTaken, err)
		}
		return fmt.Errorf("ipc: создать канал: %w", err)
	}
	defer func() {
		if cur != 0 {
			_ = windows.CloseHandle(cur)
		}
	}()

	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer waitBounded(&wg) // выполняется после cancel
	defer cancel()
	stop := ctx.Done()
	slots := make(chan struct{}, maxConns)

	for {
		select {
		case slots <- struct{}{}:
		case <-stop:
			return nil
		}
		if err := connectPipe(cur, stop); err != nil {
			<-slots
			if errors.Is(err, errStopped) {
				return nil
			}
			return fmt.Errorf("ipc: ожидание подключения: %w", err)
		}
		conn := cur
		cur = 0
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			serveConn(conn, stop, handler)
		}()
		// Новый слушающий экземпляр, пока этот занят клиентом. Короткий промежуток между
		// подключением и созданием клиенты переживают через ERROR_PIPE_BUSY и повтор.
		if cur, err = createInstance(namep, sa, false); err != nil {
			cur = 0
			return fmt.Errorf("ipc: создать экземпляр канала: %w", err)
		}
	}
}

func createInstance(namep *uint16, sa *windows.SecurityAttributes, first bool) (windows.Handle, error) {
	open := uint32(windows.PIPE_ACCESS_DUPLEX | windows.FILE_FLAG_OVERLAPPED)
	if first {
		open |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	mode := uint32(windows.PIPE_TYPE_BYTE | windows.PIPE_READMODE_BYTE | windows.PIPE_WAIT | windows.PIPE_REJECT_REMOTE_CLIENTS)
	return windows.CreateNamedPipe(namep, open, mode, windows.PIPE_UNLIMITED_INSTANCES, pipeBufferSize, pipeBufferSize, 0, sa)
}

// connectPipe ждёт клиента на экземпляре h; stop прерывает ожидание через CancelIoEx.
func connectPipe(h windows.Handle, stop <-chan struct{}) error {
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ev)
	ov := new(windows.Overlapped)
	ov.HEvent = ev
	err = windows.ConnectNamedPipe(h, ov)
	switch {
	case err == nil, errors.Is(err, windows.ERROR_PIPE_CONNECTED):
		return nil // клиент успел подключиться до вызова
	case errors.Is(err, windows.ERROR_IO_PENDING):
		_, err = waitOverlapped(h, ov, stop, 0)
		return err
	}
	return err
}

// waitBounded ждёт завершения обслуживания соединений не дольше stopWait: зависший
// handler не должен держать остановку службы.
func waitBounded(wg *sync.WaitGroup) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(stopWait):
	}
}

// serveConn обслуживает одно соединение: запрос, ответ, запрос, ответ, пока клиент не
// закроет канал, не замолчит на idleTimeout или не пришлёт мусор.
func serveConn(h windows.Handle, stop <-chan struct{}, handler func(Peer, Msg) Reply) {
	c, err := newPipeConn(h, stop)
	if err != nil {
		return
	}
	defer c.Close()

	var peer Peer
	identified := false
	for {
		c.readTimeout = idleTimeout
		n, err := ReadHeader(c)
		if err != nil {
			if errors.Is(err, ErrEmptyFrame) || errors.Is(err, ErrFrameTooLarge) {
				c.replyAndClose(Fail(TextBadRequest))
			}
			return
		}
		c.readTimeout = frameTimeout
		body, err := ReadBody(c, n)
		if err != nil {
			return
		}
		msg, err := decodeMsg(body)
		if err != nil {
			c.replyAndClose(Fail(TextBadRequest))
			return
		}
		// Личность проверяется после первого прочитанного кадра (требование
		// ImpersonateNamedPipeClient) и действует на всё соединение.
		if !identified {
			p, err := identifyPeer(h)
			if err != nil {
				c.replyAndClose(Fail(TextNoIdentity))
				return
			}
			peer, identified = p, true
		}
		rep := callHandler(handler, peer, msg)
		c.writeTimeout = frameTimeout
		if err := WriteReply(c, rep); err != nil {
			if !errors.Is(err, ErrFrameTooLarge) {
				return
			}
			// Ответ не влезает в кадр: клиент получает человеческую ошибку, соединение живёт.
			if WriteReply(c, Fail(TextInternal)) != nil {
				return
			}
		}
	}
}

// replyAndClose отправляет последний ответ перед закрытием соединения (ошибка записи не важна).
func (c *pipeConn) replyAndClose(r Reply) {
	c.writeTimeout = time.Second
	_ = WriteReply(c, r)
}

func callHandler(handler func(Peer, Msg) Reply, p Peer, m Msg) (rep Reply) {
	defer func() {
		if recover() != nil {
			rep = Fail(TextInternal)
		}
	}()
	return handler(p, m)
}
