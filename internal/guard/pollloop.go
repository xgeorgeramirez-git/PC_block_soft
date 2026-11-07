package guard

import (
	"sync/atomic"
	"time"
)

// pollRounds — сколько опросов службы сделал страж за жизнь процесса (диагностика теста B-20:
// сколько опросов прошло после закрытия окна).
var pollRounds atomic.Int64

// pollEvery вызывает once сразу, затем по каждому тику и пинку, пока не закрыт stop.
func pollEvery(stop <-chan struct{}, tick <-chan time.Time, kick <-chan struct{}, once func()) {
	for {
		pollRounds.Add(1)
		once()
		select {
		case <-stop:
			return
		case <-tick:
		case <-kick:
		}
	}
}
