//go:build windows

package ipc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

var pipeSeq atomic.Int64

// testPipeName — уникальное имя канала для одного теста.
func testPipeName() string {
	return fmt.Sprintf(`\\.\pipe\PCBlock-test-%d-%d`, os.Getpid(), pipeSeq.Add(1))
}

// requireAdmin: дескриптор канала даёт право создавать экземпляры только системе и
// администраторам, поэтому тесты сервера идут от администратора (раннер CI им и является).
// В CI молчаливый пропуск недопустим: тест падает.
func requireAdmin(t *testing.T) {
	t.Helper()
	admin, err := inAdministrators(windows.GetCurrentProcessToken())
	if err == nil && admin {
		return
	}
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Fatalf("в CI тесты канала должны идти от администратора (admin=%v, err=%v)", admin, err)
	}
	t.Skip("тесты канала идут от администратора")
}

type call struct {
	peer Peer
	msg  Msg
}

type testServer struct {
	name  string
	stop  context.CancelFunc
	done  chan error
	mu    sync.Mutex
	calls []call

	downOnce sync.Once
	downIn   time.Duration
}

func (s *testServer) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *testServer) lastCall(t *testing.T) call {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.calls) == 0 {
		t.Fatal("обработчик не вызывался")
	}
	return s.calls[len(s.calls)-1]
}

// startServer запускает Serve на новом имени. Обработчик отвечает эхом команды; команда "Boom" паникует.
func startServer(t *testing.T) *testServer {
	t.Helper()
	requireAdmin(t)
	s := &testServer{name: testPipeName(), done: make(chan error, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	s.stop = cancel
	go func() {
		s.done <- Serve(ctx, s.name, func(p Peer, m Msg) Reply {
			s.mu.Lock()
			s.calls = append(s.calls, call{p, m})
			s.mu.Unlock()
			if m.Cmd == "Boom" {
				panic("тестовая паника обработчика")
			}
			if m.Cmd == "Big" {
				return Reply{OK: true, Payload: []byte(`"` + strings.Repeat("я", MaxFrame) + `"`)}
			}
			return OK(map[string]string{"echo": m.Cmd, "token": m.Token})
		})
	}()
	select {
	case err := <-s.done:
		t.Fatalf("Serve сразу вернул: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	t.Cleanup(func() { s.shutdown(t) })
	return s
}

// shutdown останавливает сервер и проверяет, что Serve вернулся быстро.
// Повторный вызов ничего не делает и возвращает то же время.
func (s *testServer) shutdown(t *testing.T) time.Duration {
	t.Helper()
	s.downOnce.Do(func() {
		start := time.Now()
		s.stop()
		select {
		case err := <-s.done:
			if err != nil {
				t.Errorf("Serve вернул ошибку при остановке: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Serve не завершился за 5 с после отмены")
		}
		s.downIn = time.Since(start)
	})
	return s.downIn
}

// dial подключается без проверки владельца: сервер теста работает не от имени системы.
func (s *testServer) dial(t *testing.T) *Client {
	t.Helper()
	c, err := DialWith(s.name, 3*time.Second, DialOptions{RequestTimeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("DialWith: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func echoOf(t *testing.T, r Reply) (cmd, token string) {
	t.Helper()
	if !r.OK {
		t.Fatalf("отказ вместо ответа: %+v", r)
	}
	var out map[string]string
	if err := r.Into(&out); err != nil {
		t.Fatalf("нагрузка ответа: %v", err)
	}
	return out["echo"], out["token"]
}

func TestPipeRoundTrip(t *testing.T) {
	s := startServer(t)
	c := s.dial(t)
	r, err := c.Do(Msg{Cmd: CmdHello, Token: "tok-ж", Payload: Payload(Empty{})})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	cmd, tok := echoOf(t, r)
	if cmd != CmdHello || tok != "tok-ж" {
		t.Fatalf("эхо: %q, %q", cmd, tok)
	}
	if got := s.lastCall(t).msg; got.Cmd != CmdHello || got.Token != "tok-ж" || string(got.Payload) != "{}" {
		t.Fatalf("обработчик получил %+v", got)
	}
	// Тот же клиент, второй запрос по тому же соединению.
	if r, err = c.Do(Msg{Cmd: CmdLogout}); err != nil {
		t.Fatalf("второй Do: %v", err)
	} else if cmd, _ := echoOf(t, r); cmd != CmdLogout {
		t.Fatalf("второй ответ: %q", cmd)
	}
}

func TestPipePeerIsFilled(t *testing.T) {
	s := startServer(t)
	c := s.dial(t)
	if _, err := c.Do(Msg{Cmd: CmdHello}); err != nil {
		t.Fatalf("Do: %v", err)
	}
	p := s.lastCall(t).peer

	me, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	var session uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &session); err != nil {
		t.Fatal(err)
	}
	if p.PID != uint32(os.Getpid()) {
		t.Errorf("PID %d, свой %d", p.PID, os.Getpid())
	}
	if p.SID != me.User.Sid.String() {
		t.Errorf("SID %q, свой %q", p.SID, me.User.Sid)
	}
	if p.SessionID != session {
		t.Errorf("сеанс %d, свой %d", p.SessionID, session)
	}
	if wantSystem := me.User.Sid.IsWellKnown(windows.WinLocalSystemSid); p.System != wantSystem {
		t.Errorf("System %v, ждали %v", p.System, wantSystem)
	}
	if !p.Admin {
		t.Errorf("тест идёт от администратора, а Peer.Admin = false: %v", p)
	}
	t.Logf("Peer: %v, повышен: %v", p, p.Elevated)
}

func TestPipeTwoClientsInARow(t *testing.T) {
	s := startServer(t)
	for i := 0; i < 3; i++ {
		c, err := DialWith(s.name, 3*time.Second, DialOptions{})
		if err != nil {
			t.Fatalf("клиент %d: Dial: %v", i, err)
		}
		r, err := c.Do(Msg{Cmd: fmt.Sprintf("Cmd%d", i)})
		if err != nil {
			t.Fatalf("клиент %d: Do: %v", i, err)
		}
		if cmd, _ := echoOf(t, r); cmd != fmt.Sprintf("Cmd%d", i) {
			t.Fatalf("клиент %d: эхо %q", i, cmd)
		}
		c.Close()
	}
}

func TestPipeConcurrentClients(t *testing.T) {
	s := startServer(t)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := DialWith(s.name, 5*time.Second, DialOptions{})
			if err != nil {
				errs <- err
				return
			}
			defer c.Close()
			for j := 0; j < 5; j++ {
				want := fmt.Sprintf("C%d-%d", i, j)
				r, err := c.Do(Msg{Cmd: want})
				if err != nil {
					errs <- err
					return
				}
				var out map[string]string
				if err := r.Into(&out); err != nil || out["echo"] != want {
					errs <- fmt.Errorf("эхо %q вместо %q (%v)", out["echo"], want, err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if got := s.callCount(); got != 40 {
		t.Errorf("обработчик вызван %d раз, ждали 40", got)
	}
}

func TestPipeStopDoesNotHang(t *testing.T) {
	s := startServer(t)
	idle := s.dial(t) // подключённый и молчащий клиент
	if _, err := idle.Do(Msg{Cmd: CmdHello}); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if d := s.shutdown(t); d > 2*time.Second {
		t.Fatalf("остановка заняла %v, нужно не больше 2 с", d)
	}
	// Клиент после остановки получает ошибку с человеческим текстом, а не висит.
	_, err := idle.Do(Msg{Cmd: CmdHello})
	if err == nil {
		t.Fatal("запрос к остановленной службе удался")
	}
	if err.Error() != "Программа не отвечает" {
		t.Fatalf("текст ошибки для человека: %q", err.Error())
	}
}

func TestPipeStopWithoutAnyClient(t *testing.T) {
	s := startServer(t)
	if d := s.shutdown(t); d > 2*time.Second {
		t.Fatalf("остановка без клиентов заняла %v", d)
	}
}

func TestPipeSecondServerSameNameIsRefused(t *testing.T) {
	s := startServer(t)
	err := Serve(context.Background(), s.name, func(Peer, Msg) Reply { return Fail("не должен вызываться") })
	if !errors.Is(err, ErrPipeTaken) {
		t.Fatalf("второй Serve при живом первом: %v", err)
	}
	// Первый сервер не пострадал.
	c := s.dial(t)
	if _, err := c.Do(Msg{Cmd: CmdHello}); err != nil {
		t.Fatalf("Do после отказа второму: %v", err)
	}
	// После остановки первого имя свободно.
	s.shutdown(t)
	s2 := &testServer{name: s.name, done: make(chan error, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { s2.done <- Serve(ctx, s2.name, func(Peer, Msg) Reply { return OK(Empty{}) }) }()
	c2, err := DialWith(s.name, 3*time.Second, DialOptions{})
	if err != nil {
		t.Fatalf("Dial ко второму серверу после остановки первого: %v", err)
	}
	c2.Close()
	cancel()
	select {
	case err := <-s2.done:
		if err != nil {
			t.Fatalf("второй Serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("второй Serve не остановился")
	}
}

func TestPipeClientChecksServerOwner(t *testing.T) {
	s := startServer(t)
	me, _ := windows.GetCurrentProcessToken().GetTokenUser()
	if me.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		t.Skip("тест идёт от имени системы: владелец канала как раз система")
	}
	// Сервер теста работает не от имени системы: Dial по умолчанию отказывает до отправки данных.
	_, err := Dial(s.name, 2*time.Second)
	if !errors.Is(err, ErrNotService) {
		t.Fatalf("Dial к несистемному серверу: %v", err)
	}
	if err.Error() != "Программа не отвечает" {
		t.Fatalf("текст для человека: %q", err.Error())
	}
	var e *Error
	if !errors.As(err, &e) || e.Detail() == "" {
		t.Fatalf("нет технической подробности для журнала: %v", err)
	}
	if n := s.callCount(); n != 0 {
		t.Fatalf("обработчик вызван %d раз: данные ушли до проверки владельца", n)
	}
	// Тот же сервер с отключённой проверкой отвечает.
	c := s.dial(t)
	if _, err := c.Do(Msg{Cmd: CmdHello}); err != nil {
		t.Fatalf("Do без проверки владельца: %v", err)
	}
}

func TestVerifySystemServerDecisions(t *testing.T) {
	origOwner, origSvc := processOwnerIsSystem, serviceProcessID
	t.Cleanup(func() { processOwnerIsSystem, serviceProcessID = origOwner, origSvc })

	s := startServer(t)
	h, err := openPipe(s.name, time.Now().Add(3*time.Second), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	own := uint32(os.Getpid())

	cases := []struct {
		name    string
		owner   func(uint32) (bool, error)
		service func(string) (uint32, error)
		svcName string
		wantOK  bool
	}{
		{"владелец система", func(uint32) (bool, error) { return true, nil }, nil, "PCBlock", true},
		{"владелец не система", func(uint32) (bool, error) { return false, nil }, nil, "PCBlock", false},
		{"токен недоступен, процесс и есть служба", func(uint32) (bool, error) { return false, windows.ERROR_ACCESS_DENIED },
			func(string) (uint32, error) { return own, nil }, "PCBlock", true},
		{"токен недоступен, служба в другом процессе", func(uint32) (bool, error) { return false, windows.ERROR_ACCESS_DENIED },
			func(string) (uint32, error) { return own + 1, nil }, "PCBlock", false},
		{"токен недоступен, служба не запущена", func(uint32) (bool, error) { return false, windows.ERROR_ACCESS_DENIED },
			func(string) (uint32, error) { return 0, nil }, "PCBlock", false},
		{"токен недоступен, службы нет", func(uint32) (bool, error) { return false, windows.ERROR_ACCESS_DENIED },
			func(string) (uint32, error) { return 0, windows.ERROR_SERVICE_DOES_NOT_EXIST }, "PCBlock", false},
		{"токен недоступен, имя службы не задано", func(uint32) (bool, error) { return false, windows.ERROR_ACCESS_DENIED }, nil, "", false},
		{"другая ошибка проверки", func(uint32) (bool, error) { return false, windows.ERROR_INVALID_HANDLE },
			func(string) (uint32, error) { return own, nil }, "PCBlock", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			processOwnerIsSystem = c.owner
			if c.service != nil {
				serviceProcessID = c.service
			}
			err := verifySystemServer(h, c.svcName)
			if c.wantOK && err != nil {
				t.Fatalf("ждали успех, получили %v", err)
			}
			if !c.wantOK && !errors.Is(err, ErrNotService) {
				t.Fatalf("ждали ErrNotService, получили %v", err)
			}
		})
	}
}

func TestProcessOwnerAndServiceLookups(t *testing.T) {
	me, _ := windows.GetCurrentProcessToken().GetTokenUser()
	system, err := processRunsAsSystem(uint32(os.Getpid()))
	if err != nil {
		t.Fatalf("processRunsAsSystem(свой PID): %v", err)
	}
	if want := me.User.Sid.IsWellKnown(windows.WinLocalSystemSid); system != want {
		t.Errorf("владелец своего процесса: system=%v, ждали %v", system, want)
	}
	if _, err := processRunsAsSystem(0xFFFFFFF0); err == nil {
		t.Errorf("несуществующий процесс принят")
	}
	if _, err := servicePID("PCBlockNoSuchService"); !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		t.Errorf("несуществующая служба: %v", err)
	}
	// Системная служба, которая есть на любой Windows 11; PID может быть 0, если её остановили.
	if _, err := servicePID("EventLog"); err != nil {
		t.Errorf("servicePID(EventLog): %v", err)
	}
	if _, err := servicePID("имя\x00с нулём"); err == nil {
		t.Errorf("имя с нулевым символом принято")
	}
}

func TestDialTimesOutWithoutServer(t *testing.T) {
	start := time.Now()
	_, err := DialWith(testPipeName(), 400*time.Millisecond, DialOptions{})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("Dial к несуществующему каналу: %v", err)
	}
	if d := time.Since(start); d < 300*time.Millisecond || d > 3*time.Second {
		t.Fatalf("Dial ждал %v при сроке 400 мс", d)
	}
	if err.Error() != "Программа не отвечает" {
		t.Fatalf("текст для человека: %q", err.Error())
	}
	if _, err := DialWith(`\\.\pipe\bad`+"\x00"+`name`, time.Second, DialOptions{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("имя с нулевым символом: %v", err)
	}
}

func TestPipeMalformedFramesAreRejected(t *testing.T) {
	s := startServer(t)
	cases := map[string][]byte{
		"огромная длина": {0xFF, 0xFF, 0xFF, 0xFF},
		"нулевая длина":  {0, 0, 0, 0},
		"не JSON": func() []byte {
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, 5)
			return append(b, "hello"...)
		}(),
		"JSON без команды": func() []byte {
			body := `{"token":"t"}`
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, uint32(len(body)))
			return append(b, body...)
		}(),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			h, err := openPipe(s.name, time.Now().Add(3*time.Second), nil)
			if err != nil {
				t.Fatal(err)
			}
			conn, err := newPipeConn(h, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.readTimeout, conn.writeTimeout = 3*time.Second, 3*time.Second
			if _, err := conn.Write(raw); err != nil {
				t.Fatalf("запись: %v", err)
			}
			rep, err := ReadReply(conn)
			if err != nil {
				t.Fatalf("ответ на мусор: %v", err)
			}
			if rep.OK || rep.Error != TextBadRequest {
				t.Fatalf("ответ на мусор: %+v", rep)
			}
			// После ответа сервер закрывает соединение.
			if _, err := ReadReply(conn); !errors.Is(err, io.EOF) {
				t.Fatalf("после мусора соединение должно закрыться: %v", err)
			}
		})
	}
	if n := s.callCount(); n != 0 {
		t.Errorf("обработчик вызван %d раз на мусор", n)
	}
	// Сервер пережил мусор.
	c := s.dial(t)
	if _, err := c.Do(Msg{Cmd: CmdHello}); err != nil {
		t.Fatalf("Do после мусора: %v", err)
	}
}

func TestPipeHandlerPanicKeepsServerAlive(t *testing.T) {
	s := startServer(t)
	c := s.dial(t)
	r, err := c.Do(Msg{Cmd: "Boom"})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if r.OK || r.Error != TextInternal {
		t.Fatalf("ответ на панику: %+v", r)
	}
	if r, err = c.Do(Msg{Cmd: CmdHello}); err != nil || !r.OK {
		t.Fatalf("то же соединение после паники: %+v, %v", r, err)
	}
}

func TestPipeLargeMessages(t *testing.T) {
	s := startServer(t)
	c := s.dial(t)
	// Почти предельный запрос: тело больше буфера канала (64 КБ) много раз.
	payload := Payload(map[string]string{"text": strings.Repeat("ж", 400_000)})
	if len(payload) < 700_000 {
		t.Fatalf("тестовая нагрузка мала: %d", len(payload))
	}
	r, err := c.Do(Msg{Cmd: "Large", Payload: payload})
	if err != nil {
		t.Fatalf("большой запрос: %v", err)
	}
	if cmd, _ := echoOf(t, r); cmd != "Large" {
		t.Fatalf("эхо: %q", cmd)
	}
	if got := s.lastCall(t).msg.Payload; len(got) != len(payload) {
		t.Fatalf("нагрузка дошла в %d байт из %d", len(got), len(payload))
	}
	// Ответ, который не влезает в кадр, заменяется человеческой ошибкой, соединение живёт.
	r, err = c.Do(Msg{Cmd: "Big"})
	if err != nil {
		t.Fatalf("Do(Big): %v", err)
	}
	if r.OK || r.Error != TextInternal {
		t.Fatalf("слишком большой ответ: ok=%v, %q", r.OK, r.Error)
	}
	if _, err = c.Do(Msg{Cmd: CmdHello}); err != nil {
		t.Fatalf("соединение после большого ответа: %v", err)
	}
	// Запрос сверх предела не уходит вообще.
	huge := Msg{Cmd: "Huge", Payload: []byte(`"` + strings.Repeat("a", MaxFrame) + `"`)}
	n := s.callCount()
	if _, err := c.Do(huge); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("запрос сверх предела: %v", err)
	}
	if s.callCount() != n {
		t.Fatalf("запрос сверх предела дошёл до обработчика")
	}
}

func TestClientReconnectsAfterIdleClose(t *testing.T) {
	old := idleTimeout
	idleTimeout = 250 * time.Millisecond
	t.Cleanup(func() { idleTimeout = old })

	s := startServer(t)
	c := s.dial(t)
	if _, err := c.Do(Msg{Cmd: CmdHello}); err != nil {
		t.Fatal(err)
	}
	first := c.conn
	time.Sleep(900 * time.Millisecond) // сервер закрыл простаивающее соединение
	r, err := c.Do(Msg{Cmd: CmdHello})
	if err != nil {
		t.Fatalf("Do после простоя: %v", err)
	}
	if cmd, _ := echoOf(t, r); cmd != CmdHello {
		t.Fatalf("эхо: %q", cmd)
	}
	if c.conn == first {
		t.Fatal("соединение не пересоздано: сервер не закрыл простаивающее")
	}
}

func TestClientReconnectsAfterServerRestart(t *testing.T) {
	s := startServer(t)
	c := s.dial(t)
	if _, err := c.Do(Msg{Cmd: CmdHello}); err != nil {
		t.Fatal(err)
	}
	s.shutdown(t)
	// Служба перезапущена на том же имени.
	s2 := &testServer{name: s.name, done: make(chan error, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		s2.done <- Serve(ctx, s2.name, func(Peer, Msg) Reply { return OK(map[string]string{"echo": "второй"}) })
	}()
	t.Cleanup(func() {
		cancel()
		<-s2.done
	})
	r, err := c.Do(Msg{Cmd: CmdHello})
	if err != nil {
		t.Fatalf("Do после перезапуска службы: %v", err)
	}
	if cmd, _ := echoOf(t, r); cmd != "второй" {
		t.Fatalf("ответил не новый сервер: %q", cmd)
	}
}

func TestClientRequestTimeout(t *testing.T) {
	requireAdmin(t)
	name := testPipeName()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	release := make(chan struct{})
	go func() {
		done <- Serve(ctx, name, func(Peer, Msg) Reply {
			<-release // «зависший» обработчик
			return OK(Empty{})
		})
	}()
	t.Cleanup(func() {
		close(release)
		cancel()
		<-done
	})
	c, err := DialWith(name, 3*time.Second, DialOptions{RequestTimeout: 400 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	start := time.Now()
	_, err = c.Do(Msg{Cmd: CmdHello})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("Do к зависшей службе: %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("Do ждал %v при сроке 400 мс (L-57)", d)
	}
	if c.conn != nil {
		t.Fatal("после таймаута соединение должно быть сброшено")
	}
}

func TestClientClose(t *testing.T) {
	s := startServer(t)
	c, err := DialWith(s.name, 3*time.Second, DialOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := c.Do(Msg{Cmd: CmdHello}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Do после Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("повторный Close: %v", err)
	}
}

func TestClientCloseInterruptsDo(t *testing.T) {
	requireAdmin(t)
	name := testPipeName()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	release := make(chan struct{})
	go func() {
		done <- Serve(ctx, name, func(Peer, Msg) Reply {
			<-release
			return OK(Empty{})
		})
	}()
	t.Cleanup(func() {
		close(release)
		cancel()
		<-done
	})
	c, err := DialWith(name, 3*time.Second, DialOptions{RequestTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	res := make(chan error, 1)
	go func() {
		_, err := c.Do(Msg{Cmd: CmdHello})
		res <- err
	}()
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	c.Close()
	select {
	case err := <-res:
		if err == nil {
			t.Fatal("Do вернул успех после Close")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close не прервал ожидающий Do")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("прерывание заняло %v", d)
	}
}

func TestPipeConnectionLimit(t *testing.T) {
	old := maxConns
	maxConns = 1
	t.Cleanup(func() { maxConns = old })

	s := startServer(t)
	a := s.dial(t)
	if _, err := a.Do(Msg{Cmd: CmdHello}); err != nil {
		t.Fatal(err)
	}
	// Единственный слот занят. Второй клиент подключается к ждущему экземпляру канала, но
	// его запрос не обслуживается, пока слот не освободится.
	b, err := DialWith(s.name, 3*time.Second, DialOptions{})
	if err != nil {
		t.Fatalf("второй клиент: %v", err)
	}
	defer b.Close()
	resB := make(chan error, 1)
	go func() {
		_, err := b.Do(Msg{Cmd: CmdHello})
		resB <- err
	}()
	// Свободных экземпляров больше нет: третий клиент ждёт в очереди и не дожидается.
	if _, err := DialWith(s.name, 400*time.Millisecond, DialOptions{}); !errors.Is(err, ErrTimeout) {
		t.Fatalf("третий клиент при maxConns=1: %v", err)
	}
	select {
	case err := <-resB:
		t.Fatalf("запрос второго клиента обслужен при занятом слоте: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	// Первый ушёл: слот освободился, второй обслужен.
	a.Close()
	select {
	case err := <-resB:
		if err != nil {
			t.Fatalf("запрос второго клиента после освобождения слота: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("второй клиент не обслужен после освобождения слота")
	}
}

func TestPipeRejectsSlowFrameBody(t *testing.T) {
	old := frameTimeout
	frameTimeout = 300 * time.Millisecond
	t.Cleanup(func() { frameTimeout = old })

	s := startServer(t)
	h, err := openPipe(s.name, time.Now().Add(3*time.Second), nil)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := newPipeConn(h, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.readTimeout = 3 * time.Second
	// Заголовок обещает 100 байт, тело не приходит: сервер обязан отпустить соединение.
	hdr := make([]byte, 4)
	binary.LittleEndian.PutUint32(hdr, 100)
	if _, err := conn.Write(hdr); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := ReadReply(conn); !errors.Is(err, io.EOF) {
		t.Fatalf("медленный клиент: %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("сервер держал медленного клиента %v при сроке 300 мс", d)
	}
}

func TestIdentifyPeerFailsOnBadHandle(t *testing.T) {
	if _, err := identifyPeer(windows.InvalidHandle); err == nil {
		t.Fatal("личность по недействительной ручке определена")
	}
}

func TestPipeConnClosedOperations(t *testing.T) {
	c := &pipeConn{}
	if _, err := c.Read(make([]byte, 1)); !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("чтение закрытого: %v", err)
	}
	if _, err := c.Write([]byte{1}); !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("запись в закрытое: %v", err)
	}
	if n, err := c.Read(nil); n != 0 || err != nil {
		t.Errorf("чтение нуля байт: %d, %v", n, err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close закрытого: %v", err)
	}
}

func TestPipeSDDLIsTheDocumentedOne(t *testing.T) {
	// Дескриптор из Ruling 17 разбирается и даёт ожидаемую строку обратно.
	sd, err := windows.SecurityDescriptorFromString(pipeSDDL)
	if err != nil {
		t.Fatalf("SDDL не разбирается: %v", err)
	}
	if pipeSDDL != "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x12019b;;;IU)" {
		t.Fatalf("SDDL изменён: %s", pipeSDDL)
	}
	// Вид обратной строки зависит от версии Windows, поэтому проверяем состав: три записи
	// для системы, администраторов и интерактивных пользователей, список защищён.
	back := sd.String()
	for _, want := range []string{"D:P", ";;;SY)", ";;;BA)", ";;;IU)"} {
		if !strings.Contains(back, want) {
			t.Errorf("в дескрипторе %q нет %q", back, want)
		}
	}
}
