package guard

import (
	"testing"
	"time"
)

// pollEveryAsync запускает pollEvery в горутине; канал закрывается, когда цикл вернулся.
func pollEveryAsync(stop <-chan struct{}, tick <-chan time.Time, kick <-chan struct{}, once func()) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		pollEvery(stop, tick, kick, once)
	}()
	return done
}

// Ловит B-20: опрос без службы длится около 4 с, за это время в канале тикера уже лежит тик.
// Если после опроса стоп и тик готовы одновременно, select выбирает случайно, и страж делает
// лишние опросы по 4 с вместо выхода — Run не укладывается в срок теста. Стоп должен выигрывать
// всегда: 200 повторов подряд с прежним select проходят с вероятностью 3^-200.
func TestPollEveryStopWinsOverReadyTick(t *testing.T) {
	for i := 0; i < 200; i++ {
		stop := make(chan struct{})
		tick := make(chan time.Time, 1)
		kick := make(chan struct{}, 1)
		n := 0
		done := pollEveryAsync(stop, tick, kick, func() {
			n++
			if n == 1 { // окно закрыли, пока шёл опрос; за опрос накопились тик и пинок
				close(stop)
				tick <- time.Time{}
				kick <- struct{}{}
			}
		})
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("повтор %d: pollEvery не вернулся за 1 с после стопа", i)
		}
		if n != 1 {
			t.Fatalf("повтор %d: опросов после стопа %d, ждали 0 (готовый тик выиграл у стопа)", i, n-1)
		}
	}
}

// Ловит: цикл перестал опрашивать по тику или по пинку (например, выходит сразу после первого
// опроса), опрашивает без повода или не выходит по стопу.
func TestPollEveryTicksAndKicks(t *testing.T) {
	stop := make(chan struct{})
	tick := make(chan time.Time)
	kick := make(chan struct{})
	calls := make(chan int, 10)
	n := 0
	done := pollEveryAsync(stop, tick, kick, func() {
		n++
		calls <- n
	})
	wait := func(want int, why string) {
		t.Helper()
		select {
		case got := <-calls:
			if got != want {
				t.Fatalf("%s: опрос номер %d, ждали %d", why, got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s: опроса номер %d нет за 1 с", why, want)
		}
	}
	wait(1, "первый опрос сразу")
	select {
	case got := <-calls:
		t.Fatalf("опрос номер %d без тика и пинка", got)
	case <-time.After(50 * time.Millisecond):
	}
	sendOrFail := func(send func(), what string) {
		t.Helper()
		ok := make(chan struct{})
		go func() { send(); close(ok) }()
		select {
		case <-ok:
		case <-time.After(time.Second):
			t.Fatalf("%s не принят за 1 с: цикл опроса не ждёт", what)
		}
	}
	sendOrFail(func() { tick <- time.Time{} }, "тик")
	wait(2, "опрос по тику")
	sendOrFail(func() { kick <- struct{}{} }, "пинок")
	wait(3, "опрос по пинку")
	close(stop)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pollEvery не вернулся за 1 с после стопа")
	}
}
