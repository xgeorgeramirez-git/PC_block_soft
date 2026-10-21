package systray

import (
	"sync"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Сроки опроса службы значком.
const (
	// PollEvery — как часто значок спрашивает службу о состоянии ребёнка (FR-7: значок
	// появляется не позже 10 с, остаток обновляется заметно чаще минуты).
	PollEvery = 5 * time.Second
	// StaleAfter — через сколько без ответа службы цифры на значке считаются устаревшими:
	// «осталось 85» полчаса спустя после обрыва связи было бы неправдой.
	StaleAfter = 30 * time.Second
)

// Display — что показывает значок у часов.
type Display struct {
	Label   string // надпись на значке; пустая у учётки не ребёнка и пока нет данных
	Status  Status // определяет цвет значка (ТЗ 8.0 п.3)
	Tooltip string // подсказка при наведении
}

// ceilMin переводит остаток в минуты единым правилом ядра (B-5): «осталось 5 с» показывает
// «1 мин», а не «0». То же правило применяет служба в ipc, поэтому значок и окно родителя
// не расходятся на минуту.
func ceilMin(seconds int) int {
	return core.RemainingMinutes(seconds)
}

// isPaused: ограничения не действуют без блокировки и без лимита (безопасный режим и учётка,
// ставшая администратором, R1).
func isPaused(st ipc.ChildStatus) bool {
	return st.Status == ipc.StatusSafe || st.Status == ipc.StatusAdmin
}

// isLocked: служба просит показать экран блокировки.
func isLocked(st ipc.ChildStatus) bool {
	return st.Lock != "" && st.Lock != ipc.LockNone
}

// DisplayFor собирает вид значка из состояния ребёнка. Надпись, статус и подсказка считаются
// функциями format.go по числам службы; сами строки службы (Label, Tooltip) здесь не
// берутся, чтобы «1ч» вместо «85» при остатке от 100 минут решал только значок.
func DisplayFor(st ipc.ChildStatus) Display {
	switch st.Status {
	case ipc.StatusNotChild:
		return Display{Status: StatusNotChild, Tooltip: TrayTooltip(0, 0, false, false, true)}
	case ipc.StatusOK, ipc.StatusLow, ipc.StatusTimeUp, ipc.StatusBypass, ipc.StatusUnlimited,
		ipc.StatusSafe, ipc.StatusAdmin:
	default:
		return waitingDisplay(tooltipWaiting) // пустой или незнакомый ответ: цифр не выдумываем
	}
	locked := isLocked(st)
	remMin := ceilMin(st.RemainingS)
	if locked {
		remMin = 0
	}
	paused := isPaused(st)
	unlimited := st.Unlimited || paused
	d := Display{
		Label:   TrayLabel(remMin, unlimited),
		Status:  TrayStatus(remMin, unlimited, locked, paused),
		Tooltip: TrayTooltip(remMin*60, st.LimitS, unlimited, locked, false),
	}
	// Обход и «ограничения не действуют» служба объясняет точнее, чем общий текст значка:
	// «время закончилось» при блокировке за обход было бы неправдой.
	if (st.Status == ipc.StatusBypass || paused) && st.Tooltip != "" {
		d.Tooltip = st.Tooltip
	}
	return d
}

// waitingDisplay — значок, пока службе нечего сказать: графитовый квадрат без надписи.
func waitingDisplay(tooltip string) Display {
	return Display{Status: StatusNormal, Tooltip: tooltip}
}

// Snapshot — что значок знает о ребёнке на данный момент.
type Snapshot struct {
	Status ipc.ChildStatus
	Has    bool // служба хотя бы раз ответила
	Failed bool // последний опрос не удался
	Fresh  bool // последний удачный ответ не старше StaleAfter
}

// Tracker помнит последний ответ службы и итог последнего опроса. Безопасен для вызова
// из нескольких горутин: опрашивает одна, читает поток окон.
type Tracker struct {
	mu     sync.Mutex
	st     ipc.ChildStatus
	has    bool
	lastOK time.Time
	failed bool
}

// OnStatus принимает успешный ответ службы.
func (t *Tracker) OnStatus(now time.Time, st ipc.ChildStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.st, t.has, t.lastOK, t.failed = st, true, now, false
}

// OnError принимает неудачный опрос; последний удачный ответ остаётся до StaleAfter.
func (t *Tracker) OnError() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
}

// Snapshot возвращает текущее знание.
func (t *Tracker) Snapshot(now time.Time) Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return Snapshot{
		Status: t.st,
		Has:    t.has,
		Failed: t.failed,
		Fresh:  t.has && now.Sub(t.lastOK) <= StaleAfter,
	}
}

// Display возвращает вид значка: по свежему ответу, иначе без цифр.
func (t *Tracker) Display(now time.Time) Display {
	s := t.Snapshot(now)
	switch {
	case !s.Has && !s.Failed:
		return waitingDisplay(tooltipWaiting)
	case !s.Fresh:
		return waitingDisplay(tooltipNoData)
	}
	return DisplayFor(s.Status)
}
