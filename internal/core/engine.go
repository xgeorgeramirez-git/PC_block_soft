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

// ErrPromoRange — минуты промокода вне 1…600 (R26, D-50).
var ErrPromoRange = errors.New("промокод: можно от 1 до 600 минут")

// ErrPausePast — конец паузы не позже текущего момента.
var ErrPausePast = errors.New("пауза: конец должен быть в будущем")

// Decision — решение ядра на этот момент: что показать стражу и значку. Цифры и тексты —
// единственный источник для значка, мини-панели, «Моей статистики» и карточки родителя
// (Review Focus 5).
type Decision struct {
	Day          string    // день, к которому относятся цифры, YYYY-MM-DD
	Lock         LockKind  // какой экран блокировки показать
	Reason       string    // заголовок причины блокировки
	Detail       string    // что произошло (только при обходе)
	Until        string    // «Можно снова: завтра» (время вышло, запретные часы, обход)
	PauseUntil   string    // «18:30» — конец активной паузы (R11); «» — нет паузы
	Used         string    // «Сегодня: 2 ч 00 мин из 2 ч»
	Warning      int       // 0 или порог 30/15/5/1, сработавший на этом тике
	WarningEvent WarnEvent // вид события сработавшего предупреждения (лимит/интервал/пауза)
	WarningS     int       // секунды до события сработавшего предупреждения

	RemainingS   int       // остаток общего времени, с (0 без лимита)
	LimitS       int       // лимит дня вместе с доп. временем, с (0 без лимита)
	UsedS        int       // общее время за день, с
	BonusBalance int       // баланс копилки, мин (заполняет служба для стража; ядро не считает)
	Unlimited    bool      // лимита нет или правила не прочитаны
	Status       Status    // статус ребёнка для значка и карточки
	ClosedDay    *DayState // закончившийся при смене суток день: службе сохранить
	Events       []Event   // события для журнала (R22); нет событий — [], не nil

	GamesRemainingS int    // остаток игр, с (R5); 0 без лимита игр
	GamesLimitS     int    // лимит игр дня вместе с доп. временем, с; 0 без лимита игр
	GamesUnlimited  bool   // отдельного лимита игр нет (или правила не прочитаны)
	CloseGames      bool   // закрыть и не запускать программы «Игры» (R5; R8 «Блокировать экран»)
	StudyOnly       bool   // режим «Оставить только учёбу» действует: экрана нет, открыта только учёба (R8)
	CloseReason     string // текст плашки ребёнку о закрытии; «» — ничего не закрыто

	Counted    bool     // секунда этого тика засчитана (только Tick; в Current — false)
	CountedCat Category // категория засчитанной секунды (для usage_daily службы)
}

// Allows сообщает, можно ли сейчас программе категории cat работать на переднем плане и
// запускаться (R5, R8). Экран блокировки — нельзя ничего; «только учёба» — только CatStudy;
// игры закрыты — всё, кроме игр. Нулевое решение (старый код, пауза, Broken) — можно всё.
// Программы класса «Системное» служба передаёт как CatOther и не закрывает сама (C-8).
func (d Decision) Allows(cat Category) bool {
	switch {
	case d.Lock != LockNone:
		return false
	case d.StudyOnly:
		return cat == CatStudy
	case d.CloseGames && cat == CatGames:
		return false
	}
	return true
}

// DeniedText — запись журнала и плашки о закрытой попытке запуска программы категории cat
// (R5, R8); «» — запуск разрешён или закрытие не связано с играми и режимом «только учёба».
func (d Decision) DeniedText(cat Category) string {
	switch {
	case d.StudyOnly && cat != CatStudy:
		return StudyOnlyDeniedText
	case d.CloseGames && cat == CatGames:
		return GamesDeniedText
	}
	return ""
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
	gwarn    warnings // пороги остатка игр (R5, R9): отдельный набор, событие WarnGames
	fg       Category // категория переднего плана текущего тика (игровые пороги — только при игре, R9)
	bypass   string   // описание блокировки за обход; «» — блокировки нет
	pending  []Event
	started  time.Time // начало учёта — первый тик (отсчёт сохранений, G10)
	saveSlot int       // последний 30-секундный отрезок, на который ответили «сохранить»

	pauseWasActive bool // пауза была активна на прошлом тике (для события EvPauseEnd)
	saveNow        bool // момент последней минуты только что поставлен: сохранить на ближайшем DueSave (RF12-3)
}

// NewEngine создаёт ядро ребёнка. loc — местный пояс (сутки и дни недели; nil —
// time.Local); st — сохранённое состояние дня (может быть прошлым днём: первый тик
// закроет его). Параметр bypassDetail (сохранённая подробность обхода) больше не
// восстанавливает блокировку: по D-41 попытка обхода только пишется в журнал.
func NewEngine(clock Clock, loc *time.Location, rules Rules, st DayState, bypassDetail string) *Engine {
	if loc == nil {
		loc = time.Local
	}
	_ = bypassDetail // D-41: обход не блокирует, сохранённая подробность не поднимает блокировку
	e := &Engine{clock: clock, loc: loc, rules: rules, acc: NewAccumulator(st), pending: []Event{}}
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
	e.checkPauseEnd(now)
	e.pullUp(now)
	cat := CategoryOf(s)
	e.fg = cat
	d := e.decide(e.acc.State(), true)
	// Пауза снимает все ограничения, но учёт идёт (R11): секунда засчитывается, даже если
	// без паузы здесь была бы блокировка. Не засчитывается секунда под экраном блокировки и
	// секунда программы, которой сейчас нельзя (игра при закрытых играх, всё, кроме учёбы,
	// в режиме «только учёба»; RM3-7): иначе G5/G12 разъезжаются на секунды закрытия.
	blocked := d.Lock != LockNone || !d.Allows(cat)
	if e.acc.Tick(now, s, blocked) {
		d.Counted, d.CountedCat = true, cat
	}
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
// стал больше нуля. Пороги взводятся заново (R9).
func (e *Engine) AddExtra(min int) (Decision, error) {
	if min < 1 || min > MaxExtraMin {
		return e.Current(), ErrExtraRange
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
	e.cancelLastMinute()
	e.acc.AddExtra(min)
	e.rearm()
	e.pending = append(e.pending, Event{Kind: EvExtraTime, Details: map[string]any{"minutes": min, "text": ExtraText(min)}})
	return e.finish(e.decide(e.acc.State(), false), closed), nil
}

// RedeemPromoMinutes добавляет на сегодня время по промокоду (R26, D-50): 1…600 мин. Как и
// доп. время (R10), расширяет все три лимита и снимает блокировку «время вышло», но не
// выводит из запретного интервала. Диапазон шире, чем у AddExtra (240): промокод создаёт
// родитель, а не ребёнок кнопкой «Попросить ещё». Неверный диапазон — ErrPromoRange.
// Записью/одноразовостью самого кода ведает служба; ядро только применяет минуты и пишет
// событие с текстом для журнала.
func (e *Engine) RedeemPromoMinutes(min int) (Decision, error) {
	if !PromoMinutesValid(min) {
		return e.Current(), ErrPromoRange
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
	e.cancelLastMinute()
	e.acc.AddExtra(min)
	e.rearm()
	e.pending = append(e.pending, Event{Kind: EvPromoRedeem, Details: map[string]any{"minutes": min, "text": PromoRedeemText(min)}})
	return e.finish(e.decide(e.acc.State(), false), closed), nil
}

// ReduceExtra убавляет родительское доп. время на сегодня (R10, D-43): 1…240 мин, иначе
// ErrExtraRange. Суммарный дневной лимит (лимит дня + доп. время) не опускается ниже 0,
// поэтому убирается не больше, чем есть. Возвращает решение и фактически убранные минуты;
// 0 — убирать было нечего. Пороги взводятся заново (R9).
func (e *Engine) ReduceExtra(min int) (Decision, int, error) {
	if min < 1 || min > MaxExtraMin {
		return e.Current(), 0, ErrExtraRange
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
	e.pullUp(e.now())
	st := e.acc.State()
	lim, _ := e.limitFor(st.Day)
	newExtra := st.ExtraMin - min
	if floor := extraFloor(lim); newExtra < floor {
		newExtra = floor
	}
	removed := st.ExtraMin - newExtra
	if removed != 0 {
		oldS, oldUnl := e.totalLimitS(e.acc.State())
		e.acc.SetExtra(newExtra)
		e.limitChanged(oldS, oldUnl)
		e.rearm()
		e.pending = append(e.pending, Event{Kind: EvExtraTime, Details: map[string]any{"minutes": -removed, "text": ExtraRemovedText(removed)}})
	}
	return e.finish(e.decide(e.acc.State(), false), closed), removed, nil
}

// ClearExtra обнуляет сегодняшнее доп. время (R10, D-43, кнопка «Убрать доп. время»):
// ExtraMin → 0. Возвращает решение и сколько было убрано (отрицательное — доп. время было
// в минусе, обнуление вернуло время к лимиту дня). Пороги взводятся заново (R9).
func (e *Engine) ClearExtra() (Decision, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
	e.pullUp(e.now())
	st := e.acc.State()
	removed := st.ExtraMin
	if removed != 0 {
		oldS, oldUnl := e.totalLimitS(st)
		e.acc.SetExtra(0)
		e.limitChanged(oldS, oldUnl)
		e.rearm()
		e.pending = append(e.pending, Event{Kind: EvExtraTime, Details: map[string]any{"minutes": -removed, "text": ExtraClearedText}})
	}
	return e.finish(e.decide(e.acc.State(), false), closed), removed
}

// extraFloor — наименьшее доп. время дня, при котором суммарный лимит (лимит дня + доп.
// время) не отрицательный (R10, D-43): −(лимит дня) при лимите, 0 без лимита.
func extraFloor(lim DayLimit) int {
	if lim.Unlimited() {
		return 0
	}
	return -*lim.TotalMin
}

// Pause ставит паузу до момента until (R11): пока активна, блокировки не показываются,
// статус «пауза», учёт идёт. Пороги предупреждений взводятся на конец паузы (G11).
// until в прошлом паузу не ставит. Событие EvPause в журнал.
func (e *Engine) Pause(until time.Time) (Decision, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
	if !until.After(e.now()) {
		return e.finish(e.decide(e.acc.State(), false), closed), ErrPausePast
	}
	ms := until.UnixMilli()
	e.cancelLastMinute()
	e.acc.SetPause(&ms)
	e.pauseWasActive = true
	e.rearm()
	e.pending = append(e.pending, Event{Kind: EvPause, Details: map[string]any{"until_ms": ms, "text": PauseText(e.now(), until)}})
	return e.finish(e.decide(e.acc.State(), false), closed), nil
}

// SpendBonus тратит min минут из копилки (R10, FR-11): увеличивает доп. время на сегодня
// (как AddExtra), снимает блокировку «время вышло», если остаток стал больше нуля; пороги
// взводятся заново. Баланс копилки ядро не хранит — проверку и запись в журнал бонусов
// делает служба; ядро получает уже одобренные минуты. Событие EvBonusSpent.
func (e *Engine) SpendBonus(min int) (Decision, error) {
	if min < 1 || min > MaxExtraMin {
		return e.Current(), ErrExtraRange
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
	e.cancelLastMinute()
	e.acc.AddExtra(min)
	e.rearm()
	e.pending = append(e.pending, Event{Kind: EvBonusSpent, Details: map[string]any{"minutes": min, "text": BonusSpentText(min)}})
	return e.finish(e.decide(e.acc.State(), false), closed), nil
}

// SetNoForbiddenToday снимает (on=true) или возвращает (on=false) запретные часы на
// сегодня (R24, D-51). Общий лимит времени при снятии продолжает действовать. Флаг
// привязан к дню и сам сбрасывается в местную полночь (новый день — NoForbidden=false).
// Пороги взводятся заново (R9: при снятии исчезает ближайший запретный интервал).
func (e *Engine) SetNoForbiddenToday(on bool) Decision {
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
	if e.acc.State().NoForbidden != on {
		e.acc.SetNoForbidden(on)
		e.rearm()
		text := NoForbiddenOnText
		if !on {
			text = NoForbiddenOffText
		}
		e.pending = append(e.pending, Event{Kind: EvNoForbidden, Details: map[string]any{"on": on, "text": text}})
	}
	return e.finish(e.decide(e.acc.State(), false), closed)
}

// MarkBypass записывает попытку обхода в журнал (R17в, D-41): подробное событие с
// описанием, что произошло. Экран НЕ блокируется и время не отключается. Пустое
// описание заменяется общим.
func (e *Engine) MarkBypass(detail string) Decision {
	if detail == "" {
		detail = defaultBypassDetail
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
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

// SetRules применяет сохранённые правила сразу (FR-19): r — правила целиком (лимиты, лимиты
// игр, расписание, режим окончания; по частям ядро их не собирает). Блокировка и закрытия
// ставятся или снимаются в возвращённом решении, пороги (общие и игровые) взводятся заново. Broken — безопасный режим
// (R18): ограничений нет, событие safe_mode — один раз при переходе в этот режим.
func (e *Engine) SetRules(r Rules) Decision {
	e.mu.Lock()
	defer e.mu.Unlock()
	closed := e.advance(e.now())
	if r.Broken && !e.rules.Broken {
		e.pending = append(e.pending, safeModeEvent())
	}
	e.pullUp(e.now())
	oldS, oldUnl := e.totalLimitS(e.acc.State())
	e.rules = r
	e.limitChanged(oldS, oldUnl)
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
// от первого тика (G10). На каждый 30-секундный отрезок отвечает true один раз. Кроме того —
// один раз сразу после того, как тик поставил момент последней минуты (R2, D-60, RF12-3):
// выключение питания в последнюю минуту не должно его терять.
// Вызывать перед Tick того же момента: тогда снимок отражает учёт ровно до now
// (сохранение в 15:40:00 содержит 40:00, как в эталоне G10).
func (e *Engine) DueSave(now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started.IsZero() {
		return false
	}
	due := e.saveNow // момент последней минуты сохраняется сразу (RF12-3)
	e.saveNow = false
	if slot := int(now.Sub(e.started) / SaveEvery); slot > e.saveSlot {
		e.saveSlot = slot
		due = true
	}
	return due
}

func (e *Engine) now() time.Time { return e.clock.Now().In(e.loc) }

// advance меняет сутки (R16): пороги взводятся заново, закрытый день уходит в событие
// day_closed и возвращается для Decision.ClosedDay.
func (e *Engine) advance(now time.Time) *DayState {
	if st := e.acc.State(); st.LastMinuteEndMs != nil && laterDay(DayOf(now), st.Day) {
		// Последняя минута закрываемого дня идёт по часам до его полуночи (R2, D-60): секунды
		// после полуночи — уже новый день, а он начинается с нуля и без отсчёта (R16).
		if day, err := time.ParseInLocation(time.DateOnly, st.Day, e.loc); err == nil {
			e.pullUp(day.AddDate(0, 0, 1)) // now уже позже этой полуночи
		}
	}
	if !e.acc.Advance(now) {
		return nil
	}
	// Новый день начинается без паузы (R16): «до конца дня» и любая пауза закончились.
	e.pauseWasActive = false
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

// checkPauseEnd ловит окончание паузы: если пауза была активна и момент now уже не
// раньше её конца, пороги взводятся заново на новое ближайшее событие (R9, G11), а в
// журнал уходит EvPauseEnd. Истёкший момент паузы из состояния не стирается — он
// безвреден (decide и nextEvent сверяют с now) и уйдёт сам при смене суток.
func (e *Engine) checkPauseEnd(now time.Time) {
	if !e.pauseWasActive {
		return
	}
	st := e.acc.State()
	until, ok := e.pauseUntil(st)
	if !ok || !now.Before(until) {
		e.pauseWasActive = false
		e.rearm()
		e.pending = append(e.pending, Event{Kind: EvPauseEnd, Details: map[string]any{"text": PauseEndText}})
	}
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

// rearm взводит пороги заново по дельте до ближайшего события (R9): минимум из остатка
// общего лимита, секунд до начала ближайшего запретного интервала и секунд до конца
// паузы (если активна). Вид этого события кладётся в warnings для текста предупреждения.
// Второй набор порогов — по остатку игр (R5): взводится тем же строгим правилом.
func (e *Engine) rearm() {
	now := e.now()
	st := e.effective(e.acc.State(), now)
	deltaS, ev, limited := e.nextEvent(st, now)
	e.warn.arm(deltaS, ev, limited)
	gamesS, gamesLimited := e.nextGames(st, now)
	e.gwarn.arm(gamesS, WarnGames, gamesLimited)
}

// nextGames возвращает секунды до исчерпания лимита игр и признак, что такое событие есть:
// лимит игр задан, паузы нет (R11 снимает все ограничения) и правила прочитаны (R18).
func (e *Engine) nextGames(st DayState, now time.Time) (deltaS int, limited bool) {
	if e.rules.Broken {
		return 0, false
	}
	if until, ok := e.pauseUntil(st); ok && now.Before(until) {
		return 0, false
	}
	lim, _ := e.limitFor(st.Day)
	rem, unl := RemainingGamesS(lim, st.ExtraMin, st.GamesS)
	return rem, !unl
}

// nextEvent возвращает секунды до ближайшего события ограничения, его вид и признак, что
// событие вообще есть (limited). Паузой события отодвигаются: пока пауза активна,
// ближайшее событие — её конец. Без лимита и без интервалов событий нет.
func (e *Engine) nextEvent(st DayState, now time.Time) (deltaS int, ev WarnEvent, limited bool) {
	if e.rules.Broken {
		return 0, WarnNone, false
	}
	// Пауза активна: единственное ближайшее событие — её конец (G11).
	if until, ok := e.pauseUntil(st); ok && now.Before(until) {
		return int(until.Sub(now).Seconds()), WarnPauseEnd, true
	}
	best, bestEv, has := 0, WarnNone, false
	// Исчерпание общего лимита.
	lim, _ := e.limitFor(st.Day)
	if rem, unl := RemainingTotalS(lim, st.ExtraMin, st.TotalS); !unl {
		best, bestEv, has = rem, WarnLimit, true
	}
	// Начало ближайшего запретного интервала (кроме дня, где запрет снят родителем, R24).
	if !st.NoForbidden {
		if start, ok := NextForbiddenStart(e.rules, now); ok {
			d := int(start.Sub(now).Seconds())
			if d >= 0 && (!has || d < best) {
				best, bestEv, has = d, WarnForbidden, true
			}
		}
	}
	return best, bestEv, has
}

// pauseUntil возвращает момент конца активной паузы (R11); в Task 2 паузы нет — Task 3
// добавит реальное значение из состояния дня.
func (e *Engine) pauseUntil(st DayState) (time.Time, bool) {
	if st.PauseUntilMs == nil {
		return time.Time{}, false
	}
	return time.UnixMilli(*st.PauseUntilMs).In(e.loc), true
}

// decide строит решение по состоянию дня st; fire — тик: сработавший порог гасится.
func (e *Engine) decide(st DayState, fire bool) Decision {
	st = e.effective(st, e.now()) // последняя минута идёт по часам и без тиков (R2, D-60)
	d := Decision{Day: st.Day, UsedS: st.TotalS, Events: []Event{}}
	if e.rules.Broken {
		d.Status, d.Unlimited, d.GamesUnlimited = StatusSafe, true, true
		d.Used = UsedText(st.TotalS, 0, true)
		return d
	}
	now := e.now()
	lim, wd := e.limitFor(st.Day)
	rem, unl := RemainingTotalS(lim, st.ExtraMin, st.TotalS)
	d.RemainingS, d.Unlimited = rem, unl
	limitMin := 0
	if !unl {
		limitMin = *lim.TotalMin + st.ExtraMin
		d.LimitS = limitMin * 60
	}
	d.Used = UsedText(st.TotalS, limitMin, unl)
	grem, gunl := RemainingGamesS(lim, st.ExtraMin, st.GamesS)
	d.GamesRemainingS, d.GamesUnlimited = grem, gunl
	if !gunl {
		d.GamesLimitS = max(0, (*lim.GamesMin+st.ExtraMin)*60)
	}

	paused := false
	if until, ok := e.pauseUntil(st); ok && now.Before(until) {
		paused = true
		d.PauseUntil = FormatClock(until)
	}
	inForbidden, forbiddenEnd := InForbidden(e.rules, now)
	if st.NoForbidden {
		// Родитель снял запретные часы на этот день (R24, D-51): интервалы не применяются,
		// но общий лимит времени (R4) действует как обычно.
		inForbidden = false
	}

	switch {
	case paused:
		// Пауза снимает все ограничения (R11); учёт идёт (Tick засчитывает секунды).
		// Предупреждения о конце паузы срабатывают во время паузы (G11).
		d.Status = StatusPaused
		if fire {
			deltaS, _, _ := e.nextEvent(st, now)
			d.Warning, d.WarningEvent = e.warn.fire(deltaS)
			if d.Warning != 0 {
				d.WarningS = deltaS
			}
		}
	case inForbidden:
		d.Status, d.Until = StatusForbidden, UntilForbidden(now, forbiddenEnd)
		e.endMode(&d, LockForbidden, ReasonForbidden, StudyOnlyForbiddenPlateText)
		e.quenchGames(grem, gunl, fire)
	case !unl && rem <= 0:
		d.Status, d.Until = StatusTimeUp, UntilText(wd, e.rules, now)
		e.endMode(&d, LockTimeUp, ReasonTimeUp, StudyOnlyPlateText)
		e.quenchGames(grem, gunl, fire)
	default:
		if !gunl && grem <= 0 {
			// Лимит игр исчерпан (R5): закрываются только игры, экрана блокировки нет.
			d.CloseGames, d.CloseReason = true, GamesOverPlateText
		}
		if fire {
			e.fireWarnings(&d, st, now, grem, gunl)
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

// endMode заполняет решение по режиму окончания (R8), когда общее время вышло или идёт
// запретный интервал: «Блокировать экран» — экран lock с причиной reason и закрытие игр;
// «Оставить только учёбу» — экрана нет, открыта только учёба, плашка studyPlate.
func (e *Engine) endMode(d *Decision, lock LockKind, reason, studyPlate string) {
	if e.rules.EndMode == EndStudyOnly {
		d.StudyOnly, d.CloseReason = true, studyPlate
		return
	}
	d.Lock, d.Reason = lock, reason
	d.CloseGames, d.CloseReason = true, reason
}

// quenchGames на тике при закрытии (экран, «только учёба») гасит игровые пороги, до которых
// уже дошло (без уведомления): после снятия закрытия они не всплывут задним числом.
func (e *Engine) quenchGames(grem int, gunl, fire bool) {
	if fire && !gunl {
		e.gwarn.fire(grem)
	}
}

// fireWarnings — срабатывание порогов на тике без блокировки (R9, PL-14). Общий порог — по
// ближайшему событию ограничения. Игровой порог — по остатку игр и только когда на переднем
// плане игра (иначе он ждёт); при закрытых играх он гасится без уведомления. Игровой порог не
// показывается, если общее событие ближе (общий лимит кончится раньше), но гасится. Если
// сработали оба — одно уведомление: у кого меньше секунд до события, при равенстве общее.
func (e *Engine) fireWarnings(d *Decision, st DayState, now time.Time, grem int, gunl bool) {
	deltaS, _, has := e.nextEvent(st, now)
	d.Warning, d.WarningEvent = e.warn.fire(deltaS)
	if d.Warning != 0 {
		d.WarningS = deltaS
	}
	if d.Warning == 1 && d.WarningEvent == WarnLimit && st.LastMinuteEndMs == nil {
		// Последняя минута общего лимита (R2, D-60): дальше остаток идёт по часам до нуля.
		ms := now.Add(time.Duration(deltaS) * time.Second).UnixMilli()
		e.acc.SetLastMinute(&ms)
		e.saveNow = true
	}
	if gunl || (e.fg != CatGames && !d.CloseGames) {
		return
	}
	gw, _ := e.gwarn.fire(grem)
	if gw == 0 || d.CloseGames || (has && grem > deltaS) {
		return
	}
	if d.Warning == 0 || grem < deltaS {
		d.Warning, d.WarningEvent, d.WarningS = gw, WarnGames, grem
	}
}

// totalLimitS — суммарный лимит дня st (лимит + доп. время) в секундах; unlimited — лимита нет
// или правила не прочитаны (R18).
func (e *Engine) totalLimitS(st DayState) (limitS int, unlimited bool) {
	if e.rules.Broken {
		return 0, true
	}
	lim, _ := e.limitFor(st.Day)
	if lim.Unlimited() {
		return 0, true
	}
	return (*lim.TotalMin + st.ExtraMin) * 60, false
}

// effective — состояние дня с учётом последней минуты (R2, D-60): пока момент «конец по часам»
// задан, общее время не меньше «лимит + доп. − секунды до момента» (после момента — весь лимит).
// Дотяжка идёт в общее время («Прочее»), игры, видео и учёба по R2 как обычно. Без лимита, в
// безопасном режиме и в паузе момент не действует. Общее время только растёт, не убавляется.
func (e *Engine) effective(st DayState, now time.Time) DayState {
	if st.LastMinuteEndMs == nil {
		return st
	}
	if until, ok := e.pauseUntil(st); ok && now.Before(until) {
		return st
	}
	limS, unl := e.totalLimitS(st)
	if unl {
		return st
	}
	left := time.UnixMilli(*st.LastMinuteEndMs).Sub(now)
	leftS := 0
	if left > 0 {
		leftS = int((left + time.Second - 1) / time.Second) // вверх: ноль не раньше момента
	}
	if target := limS - leftS; st.TotalS < target {
		st.TotalS = target
	}
	return st
}

// pullUp дотягивает накопленное общее время до последней минуты на момент now (R2, D-60).
// Момент, который уже наступил, после дотяжки снимается: время вышло и учтено целиком, а
// дальше (доп. время, новые правила) время снова идёт по R2 до следующего порога 1 мин.
func (e *Engine) pullUp(now time.Time) {
	st := e.acc.State()
	if st.LastMinuteEndMs == nil {
		return
	}
	eff := e.effective(st, now)
	if eff.TotalS != st.TotalS {
		e.acc.SetTotal(eff.TotalS)
	}
	if limS, unl := e.totalLimitS(eff); !unl && eff.TotalS >= limS && !now.Before(time.UnixMilli(*st.LastMinuteEndMs)) {
		e.acc.SetLastMinute(nil)
	}
}

// cancelLastMinute снимает отсчёт последней минуты (RF12-2: добавилось время или пауза):
// прошедшие по часам секунды остаются учтёнными, дальше время снова по R2.
func (e *Engine) cancelLastMinute() {
	e.pullUp(e.now())
	e.acc.SetLastMinute(nil)
}

// limitChanged после смены суммарного лимита (правила, убавление и обнуление доп. времени)
// поправляет момент последней минуты: лимит вырос или снят — отсчёт отменяется (RF12-2); лимит
// уменьшился — момент сдвигается раньше на разницу (Ruling FB12-1: выход не возвращает время);
// не изменился — момент прежний. Перед вызовом общее время дотянуто по старому лимиту.
func (e *Engine) limitChanged(oldS int, oldUnl bool) {
	st := e.acc.State()
	if st.LastMinuteEndMs == nil {
		return
	}
	newS, newUnl := e.totalLimitS(st)
	switch {
	case oldUnl || newUnl || newS > oldS:
		e.acc.SetLastMinute(nil)
	case newS < oldS:
		ms := *st.LastMinuteEndMs - int64(oldS-newS)*1000
		e.acc.SetLastMinute(&ms)
	}
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
