package guard

import (
	"sync/atomic"
	"time"
)

// pollRounds — сколько опросов службы сделал страж за жизнь процесса (диагностика теста B-20:
// сколько опросов прошло после закрытия окна).
var pollRounds atomic.Int64

// runStage — докуда дошёл Run после цикла окон (диагностика B-20): 0 — цикл окон ещё идёт,
// 1 — цикл окон вышел, 2 — опрос остановлен, 3 — окна убраны.
var runStage atomic.Int32

// pollEvery вызывает once сразу, затем по каждому тику и пинку, пока не закрыт stop.
//
// Стоп сильнее готового тика (B-20): опрос без службы длится секунды, и к его концу в канале
// тикера уже лежит тик. Если стоп и тик готовы вместе, select выбирает случайно, и страж делал
// лишние опросы вместо выхода. Поэтому после опроса стоп проверяется отдельно и первым.
func pollEvery(stop <-chan struct{}, tick <-chan time.Time, kick <-chan struct{}, once func()) {
	for {
		pollRounds.Add(1)
		once()
		select {
		case <-stop:
			return
		default:
		}
		select {
		case <-stop:
			return
		case <-tick:
		case <-kick:
		}
	}
}
