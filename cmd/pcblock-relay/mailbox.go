package main

import (
	"encoding/json"
	"time"
)

const (
	// queueMax — сообщений в очереди компьютера не больше (больше — 503 busy).
	queueMax = 32
	// pollBudget — сколько байтов сообщений отдаёт один опрос: клиент читает ответ не больше
	// 64 КБ (протокол, раздел 7), поэтому остаток уходит следующим опросом.
	pollBudget = 48 << 10
)

// Msg — конверт сообщения (протокол, раздел 3). body сервер не разбирает, только пересылает.
type Msg struct {
	ID   string          `json:"id"`
	From string          `json:"from"`
	Kind string          `json:"kind"`
	Body json.RawMessage `json:"body"`
}

// msgSize — оценка размера сообщения в ответе опроса.
func msgSize(m Msg) int {
	return len(m.ID) + len(m.From) + len(m.Kind) + len(m.Body) + 40
}

type queued struct {
	msg Msg
	at  time.Time
}

// queue — очередь сообщений одного компьютера. Сообщение живёт ttl (10 с) и выдаётся
// один раз. Не потокобезопасна: вызывается под мьютексом сервера.
type queue struct {
	items []queued
}

func (q *queue) purge(now time.Time, ttl time.Duration) {
	kept := q.items[:0]
	for _, it := range q.items {
		if now.Sub(it.at) < ttl {
			kept = append(kept, it)
		}
	}
	for i := len(kept); i < len(q.items); i++ {
		q.items[i] = queued{}
	}
	q.items = kept
}

// put кладёт сообщение; false — очередь полна.
func (q *queue) put(m Msg, now time.Time, ttl time.Duration) bool {
	q.purge(now, ttl)
	if len(q.items) >= queueMax {
		return false
	}
	q.items = append(q.items, queued{msg: m, at: now})
	return true
}

// take выдаёт живые сообщения по порядку, пока хватает budget байтов (хотя бы одно),
// и убирает их из очереди.
func (q *queue) take(now time.Time, ttl time.Duration, budget int) []Msg {
	q.purge(now, ttl)
	out := []Msg{}
	used := 0
	n := 0
	for i, it := range q.items {
		sz := msgSize(it.msg)
		if i > 0 && used+sz > budget {
			break
		}
		used += sz
		out = append(out, it.msg)
		n++
	}
	rest := copy(q.items, q.items[n:])
	for i := rest; i < len(q.items); i++ {
		q.items[i] = queued{}
	}
	q.items = q.items[:rest]
	return out
}

// remove убирает невыданное сообщение (телефон не дождался ответа).
func (q *queue) remove(id string) bool {
	for i, it := range q.items {
		if it.msg.ID == id {
			copy(q.items[i:], q.items[i+1:])
			q.items[len(q.items)-1] = queued{}
			q.items = q.items[:len(q.items)-1]
			return true
		}
	}
	return false
}

func (q *queue) len() int { return len(q.items) }

// pcBox — всё, что сервер держит о компьютере в памяти: очередь, сигнал для опроса,
// «на связи», последний отчёт, код привязки. После перезапуска пусто (протокол, раздел 3).
type pcBox struct {
	q        queue
	signal   chan struct{} // закрывается при новом сообщении, будит опросы
	lastSeen time.Time
	report   json.RawMessage
	code     string
	codeExp  time.Time
}

func newBox() *pcBox { return &pcBox{signal: make(chan struct{})} }

func (b *pcBox) notify() {
	close(b.signal)
	b.signal = make(chan struct{})
}

// waiter — телефон, ждущий ответа компьютера на своё сообщение.
type waiter struct {
	pcID, phoneID string
	ch            chan Msg // буфер 1
}
