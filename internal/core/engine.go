package core

import (
	"errors"
	"sync"
	"time"
)

// Границы и интервалы ядра.
const (
	MaxExtraMin   = 240              // доп. время за раз: от 1 до 240 мин (R10)
	SaveEvery     = 30 * time.Second // сохранение состояния не реже (ТЗ 2, NFR-7, G10)
	LowRemainingS = 15 * 60          // «осталось 15 мин или меньше» — янтарный статус (ТЗ 8.0 п.3)
)

// ErrExtraRange — доп. время вне 1…240 минут.
var ErrExtraRange = errors.New("доп. время: можно от 1 до 240 минут")

// Decision — решение ядра на этот момент: что показать стражу и значку. Цифры и тексты —
// единственный источник для значка, мини-панели, «Моей статистики» и карточки родителя
// (Review Focus 5).
type Decision struct {
	Day        string    // день, к которому относятся цифры, YYYY-MM-DD
	Lock       LockKind  // какой экран блокировки показать
	Reason     string    // заголовок причины блокировки
	Detail     string    // что произошло (только при обходе)
	Until      string    // «Можно снова: завтра» (только «время вышло»)
	Used       string    // «Сегодня: 2 ч 00 мин из 2 ч»
	Warning    int       // 0 или порог 15/5/1, сработавший на этом тике
	RemainingS int       // остаток общего времени, с (0 без лимита)
	LimitS     int       // лимит дня вместе с доп. временем, с (0 без лимита)
	UsedS      int       // общее время за день, с
	Unlimited  bool      // лимита нет или правила не прочитаны
	Status     Status    // статус ребёнка для значка и карточки
	ClosedDay  *DayState // закончившийся при смене суток день: службе сохранить
	Events     []Event   // события для журнала (R22); нет событий — [], не nil
}

// Engine — ядро учёта и правил одного ребёнка. Порядок тика (уточнение 1 пакета M1):
// в момент t ядро сначала решает по уже учтённому времени (предупреждение, блокировка
// в момент t), затем, если блокировки нет и секунда активна (R2), засчитывает секунду
// [t, t+1). Безопасен для вызова из нескольких горутин (тик службы и команды канала).
type Engine struct {
	mu       sync.Mutex
	clock    Clock
	loc      *time.Location
	rules    Rules
	acc      *Accumulator
	warn     warnings
	bypass   string // описание блокировки за обход; «» — блокировки нет
	pending  []Event
	started  time.Time // начало учёта — первый тик (отсчёт сохранений, G10)
	saveSlot int       // последний 30-секундный отрезок, на который ответили «сохранить»
}

// NewEngine создаёт ядро ребёнка. loc — местный пояс (сутки и дни недели; nil —
// time.Local); st — сохранённое состояние дня (может быть прошлым днём: первый тик
// закроет его); bypassDetail — сохранённая блокировка за обход («» — нет), она
// переживает перезапуск службы.
func NewEngine(clock Clock, loc *time.Location, rules Rules, st DayState, bypassDetail string) *Engine {
	if loc == nil {
		loc = time.Local
	}
	e := &Engine{clock: clock, loc: loc, rules: rules, acc: NewAccumulator(st), bypass: bypassDetail, pending: []Event{}}
	if rules.Broken {
		e.pending = append(e.pending, safeModeEvent())
	}
	e.rearm()
	return e
}

// Tick — тик раз в секунду часов ядра со снимком датчиков s.
func (e *Engine) Tick(s SessionSample) Decision {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	if e.started.IsZero() {
		e.started = now
	}
	closed := e.advance(now)
	d := e.decide(e.acc.State(), true)
	e.acc.Tick(now, s, d.Lock != LockNone)
	return e.finish(d, closed)
}

// Current — решение на сейчас без учёта секунды, без срабатывания порогов и без событий:
// для значка и окна родителя. Если сутки сменились, а тика ещё не было, показывает новый день.
func (e *Engine) Current() Decision {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.acc.State()
	if day := DayOf(e.now()); laterDay(day, st.Day) {
		st = DayState{Day: day}
	}
	return e.decide(st, false)
}

// AddExtra добавляет родительское доп. время на сегодня (R10): 1…240 мин, иначе
// ErrExtraRange и состояние не меняется. Снимает блокировку «время вышло», если остаток
// стал больше нуля; блокировку за обход не снимает. Пороги взводятся заново (R9).
func (e *Engine) AddExtra(min int) (Decision, error) {
	if min < 1 || min > MaxExtraMin {
		return e.Current(), ErrExtraRange
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
	e.acc.AddExtra(min)
	e.rearm()
	e.pending = append(e.pending, Event{Kind: EvExtraTime, Details: map[string]any{"minutes": min, "text": ExtraText(min)}})
	return e.finish(e.decide(e.acc.State(), false), closed), nil
}

// MarkBypass ставит блокировку «Попытка обойти защиту» (R17в). Её снимает только
// ClearBlock (родитель паролем): ни доп. время, ни смена суток. Пустое описание
// заменяется общим.
func (e *Engine) MarkBypass(detail string) Decision {
	if detail == "" {
		detail = defaultBypassDetail
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
	e.bypass = detail
	e.pending = append(e.pending, Event{Kind: EvBypass, Details: map[string]any{"detail": detail, "text": ReasonBypass}})
	return e.finish(e.decide(e.acc.State(), false), closed)
}

// ClearBlock снимает блокировку за обход (родитель ввёл пароль). Блокировка «время
// вышло» остаётся, пока остаток 0: её снимает доп. время или новые правила.
func (e *Engine) ClearBlock() Decision {
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
	if e.bypass != "" {
		e.bypass = ""
		e.pending = append(e.pending, Event{Kind: EvBlockCleared, Details: map[string]any{"text": "Блокировка снята родителем"}})
	}
	return e.finish(e.decide(e.acc.State(), false), closed)
}

// SetRules применяет сохранённые правила сразу (FR-19): блокировка ставится или
// снимается в возвращённом решении, пороги взводятся заново. Broken — безопасный режим
// (R18): ограничений нет, событие safe_mode — один раз при переходе в этот режим.
func (e *Engine) SetRules(r Rules) Decision {
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
	if r.Broken && !e.rules.Broken {
		e.pending = append(e.pending, safeModeEvent())
	}
	e.rules = r
	e.rearm()
	return e.finish(e.decide(e.acc.State(), false), closed)
}

// State возвращает для сохранения состояние дня и описание блокировки за обход («» — нет).
func (e *Engine) State() (DayState, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.acc.State(), e.bypass
}

// DueSave сообщает, что пора сохранить State(): не реже раза в 30 с часов ядра, отсчёт
// от первого тика (G10). На каждый 30-секундный отрезок отвечает true один раз.
// Вызывать перед Tick того же момента: тогда снимок отражает учёт ровно до now
// (сохранение в 15:40:00 содержит 40:00, как в эталоне G10).
func (e *Engine) DueSave(now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started.IsZero() {
		return false
	}
	slot := int(now.Sub(e.started) / SaveEvery)
	if slot <= e.saveSlot {
		return false
	}
	e.saveSlot = slot
	return true
}

func (e *Engine) now() time.Time { return e.clock.Now().In(e.loc) }

// advance меняет сутки (R16): пороги взводятся заново, закрытый день уходит в событие
// day_closed и возвращается для Decision.ClosedDay.
func (e *Engine) advance(now time.Time) *DayState {
	if !e.acc.Advance(now) {
		return nil
	}
	e.rearm()
	closed := e.acc.ClosedDays()
	if len(closed) == 0 {
		return nil
	}
	c := closed[len(closed)-1]
	e.pending = append(e.pending, Event{Kind: EvDayClosed, Details: map[string]any{
		"day": c.Day, "total_s": c.TotalS, "games_s": c.GamesS, "video_s": c.VideoS, "study_s": c.StudyS, "extra_min": c.ExtraMin,
	}})
	return &c
}

// limitFor возвращает лимит и день недели дня day; день не разобран — без лимита.
func (e *Engine) limitFor(day string) (DayLimit, Weekday) {
	t, err := time.ParseInLocation(time.DateOnly, day, e.loc)
	if err != nil {
		return DayLimit{}, Monday
	}
	wd := WeekdayOf(t)
	return e.rules.For(wd), wd
}

// rearm взводит пороги заново по текущему остатку.
func (e *Engine) rearm() {
	st := e.acc.State()
	lim, _ := e.limitFor(st.Day)
	rem, unl := RemainingTotalS(lim, st.ExtraMin, st.TotalS)
	e.warn.arm(rem, !unl && !e.rules.Broken)
}

// decide строит решение по состоянию дня st; fire — тик: сработавший порог гасится.
func (e *Engine) decide(st DayState, fire bool) Decision {
	d := Decision{Day: st.Day, UsedS: st.TotalS, Events: []Event{}}
	if e.rules.Broken {
		d.Status, d.Unlimited = StatusSafe, true
		d.Used = UsedText(st.TotalS, 0, true)
		return d
	}
	lim, wd := e.limitFor(st.Day)
	rem, unl := RemainingTotalS(lim, st.ExtraMin, st.TotalS)
	d.RemainingS, d.Unlimited = rem, unl
	limitMin := 0
	if !unl {
		limitMin = *lim.TotalMin + st.ExtraMin
		d.LimitS = limitMin * 60
	}
	d.Used = UsedText(st.TotalS, limitMin, unl)
	switch {
	case e.bypass != "":
		d.Lock, d.Status, d.Reason, d.Detail = LockBypass, StatusBypass, ReasonBypass, e.bypass
	case !unl && rem <= 0:
		d.Lock, d.Status, d.Reason = LockTimeUp, StatusTimeUp, ReasonTimeUp
		d.Until = UntilText(wd, e.rules)
	default:
		if fire {
			d.Warning = e.warn.fire(rem)
		}
		switch {
		case unl:
			d.Status = StatusUnlimited
		case rem <= LowRemainingS:
			d.Status = StatusLow
		default:
			d.Status = StatusOK
		}
	}
	return d
}

// finish дополняет решение закрытым днём и накопленными событиями.
func (e *Engine) finish(d Decision, closed *DayState) Decision {
	d.ClosedDay = closed
	d.Events = e.pending
	e.pending = []Event{}
	return d
}

func safeModeEvent() Event {
	return Event{Kind: EvSafeMode, Details: map[string]any{"text": "Правила не прочитаны: ограничения не действуют"}}
}
