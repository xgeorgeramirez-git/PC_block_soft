package winhost

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"pcblock/internal/service"
)

// quick выполняет f и ждёт его не дольше d: иначе тест падает (вызов ждёт).
func quick(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); f() }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s не вернулся за %v", what, d)
	}
}

// delivered — доставленные события по порядку (под mu).
type delivered struct {
	mu  sync.Mutex
	got []string
}

func (d *delivered) add(ev service.SessionEvent, id uint32) {
	d.mu.Lock()
	d.got = append(d.got, fmt.Sprintf("%s %d", ev, id))
	d.mu.Unlock()
}

func (d *delivered) list() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string{}, d.got...)
}

// Ловит (B-22, PL-8): события сеанса доставляются службе каждое в своей горутине — порядок не
// гарантирован, и «разблокировка» может прийти раньше «блокировки» (время стоит до следующего
// события); или push ждёт службу, а цикл диспетчера служб Windows ждать не должен (ответ на
// «стоп»).
func TestSessionQueueKeepsOrderAndPushNeverWaits(t *testing.T) {
	q := newSessionQueue()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gate, first := make(chan struct{}), make(chan struct{})
	var d delivered
	var once sync.Once
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		q.run(ctx, func(ev service.SessionEvent, id uint32) {
			once.Do(func() { close(first); <-gate }) // служба занята первым событием
			d.add(ev, id)
		})
	}()
	quick(t, 50*time.Millisecond, "push блокировки", func() { q.push(service.SessionLock, 2) })
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("первое событие не доставлено за 1 с")
	}
	quick(t, 50*time.Millisecond, "push разблокировки", func() { q.push(service.SessionUnlock, 2) })
	quick(t, 50*time.Millisecond, "push отключения", func() { q.push(service.SessionDisconnect, 2) })
	quick(t, 50*time.Millisecond, "push подключения", func() { q.push(service.SessionConnect, 3) })
	close(gate)
	want := []string{"блокировка 2", "разблокировка 2", "отключение 2", "подключение 3"}
	deadline := time.Now().Add(time.Second)
	for len(d.list()) < len(want) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := d.list(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("доставлено %q; ждали %q", got, want)
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("run не вернулся за 1 с после отмены")
	}
}

// Ловит: пачка событий (смена пользователя) теряет или переставляет события.
func TestSessionQueueThousandInOrder(t *testing.T) {
	q := newSessionQueue()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var d delivered
	go q.run(ctx, d.add)
	for i := range 1000 {
		q.push(service.SessionLock, uint32(i))
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(d.list()) < 1000 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	got := d.list()
	if len(got) != 1000 {
		t.Fatalf("доставлено %d из 1000", len(got))
	}
	for i, s := range got {
		if want := fmt.Sprintf("блокировка %d", i); s != want {
			t.Fatalf("событие %d: %q; ждали %q", i, s, want)
		}
	}
}

// Ловит: очередь не останавливается по ctx хоста (остановка службы ждёт её 5 с), push после
// остановки ждёт или падает, событие после остановки доставляется в остановленную службу.
func TestSessionQueueStopsWithContext(t *testing.T) {
	q := newSessionQueue()
	ctx, cancel := context.WithCancel(context.Background())
	var d delivered
	stopped := make(chan struct{})
	go func() { defer close(stopped); q.run(ctx, d.add) }()
	q.push(service.SessionLogon, 2)
	deadline := time.Now().Add(time.Second)
	for len(d.list()) < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("run не вернулся за 1 с после отмены")
	}
	quick(t, 50*time.Millisecond, "push после остановки", func() { q.push(service.SessionLogoff, 2) })
	q.mu.Lock()
	kept := len(q.items)
	q.mu.Unlock()
	if kept != 0 {
		t.Errorf("после остановки в очереди %d событий; ждали 0 (отбрасываются, память не копится)", kept)
	}
	time.Sleep(50 * time.Millisecond)
	if got, want := d.list(), []string{"вход 2"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("доставлено %q; ждали %q (после остановки — ничего)", got, want)
	}
}
