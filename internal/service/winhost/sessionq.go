package winhost

import (
	"context"
	"sync"

	"pcblock/internal/service"
)

// Очередь событий сеанса (B-22, P2-R6). Диспетчер служб Windows отдаёт события сеанса в цикл
// Execute, который ждать не должен (диспетчер ждёт ответа на «стоп»), а служба должна получить
// их по порядку: «разблокировка», обогнавшая «блокировку», остановила бы время до следующего
// события (PL-8). Поэтому push только кладёт событие в срез и будит исполнителя, а единственный
// исполнитель run отдаёт события службе по одному. Момент события службе не передаётся:
// OnSession получает time.Now() в момент доставки, иначе поздняя доставка откатила бы часы
// службы назад (OnSession ставит s.real).

// queuedSession — событие сеанса в очереди.
type queuedSession struct {
	ev service.SessionEvent
	id uint32
}

// sessionQueue — FIFO событий сеанса с одним исполнителем.
type sessionQueue struct {
	mu      sync.Mutex
	items   []queuedSession
	stopped bool          // run вернулся: новые события отбрасываются
	wake    chan struct{} // буфер 1: сигнал «есть события» не теряется и не ждёт
}

func newSessionQueue() *sessionQueue {
	return &sessionQueue{wake: make(chan struct{}, 1)}
}

// push ставит событие в очередь и никогда не ждёт: ни исполнителя, ни службу. После остановки
// событие отбрасывается.
func (q *sessionQueue) push(ev service.SessionEvent, id uint32) {
	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return
	}
	q.items = append(q.items, queuedSession{ev: ev, id: id})
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default: // сигнал уже лежит: исполнитель заберёт и это событие
	}
}

// run отдаёт события deliver по одному и по порядку, пока не отменён ctx. Событие, взятое из
// очереди после отмены, и всё, что осталось в ней, отбрасывается.
func (q *sessionQueue) run(ctx context.Context, deliver func(service.SessionEvent, uint32)) {
	defer q.stop()
	for {
		it, ok := q.next()
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-q.wake:
			}
			continue
		}
		if ctx.Err() != nil {
			return
		}
		deliver(it.ev, it.id)
	}
}

// next забирает первое событие очереди; ok = false — очередь пуста.
func (q *sessionQueue) next() (queuedSession, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return queuedSession{}, false
	}
	it := q.items[0]
	q.items = q.items[1:]
	if len(q.items) == 0 {
		q.items = nil // память пачки не держится
	}
	return it, true
}

func (q *sessionQueue) stop() {
	q.mu.Lock()
	q.stopped, q.items = true, nil
	q.mu.Unlock()
}
