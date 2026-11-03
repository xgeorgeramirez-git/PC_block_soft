package core

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Перерыв на отдых (R27, D-62, FR-37, эталон G14). Ядро считает активные секунды выбранного
// вида к перерыву; когда набралось «через сколько», один раз отдаёт плашку «ПОРА ОТДОХНУТЬ» и
// ставит момент начала перерыва по часам («предупредить за» позже); перерыв идёт по часам до
// «отдыха» минут при любом выходе, выключении и перезапуске службы. «Сам отошёл» — тишина без
// засчитанных секунд не короче порога «засчитать как отдых» (по часам: разрыв тиков тоже тишина).
// Пауза снимает перерыв и предупреждение и замораживает счёт и тишину. Экран окончания и
// запретные часы сильнее перерыва (decide). Смена суток счёт не сбрасывает.

// BreakCounts — что считать к перерыву на отдых (R27): всё, кроме учёбы, или только игры.
type BreakCounts int

// Виды счёта к перерыву.
const (
	BreakAll   BreakCounts = iota // «Всё время, кроме учёбы» (игры, видео, прочее)
	BreakGames                    // «Только игры»
)

// String — ключ вида счёта для хранилища и канала: "all" или "games".
func (c BreakCounts) String() string {
	if c == BreakGames {
		return "games"
	}
	return "all"
}

// BreakCountsFromString разбирает ключ вида счёта; неизвестное — «всё, кроме учёбы».
func BreakCountsFromString(s string) BreakCounts {
	if s == "games" {
		return BreakGames
	}
	return BreakAll
}

// Пределы настроек «Отдых» (FR-37, таблица break_rules) и стандартный текст экрана перерыва.
const (
	BreakAfterMinMin, BreakAfterMinMax       = 15, 240
	BreakWarnMinMin, BreakWarnMinMax         = 1, 15
	BreakRestMinMin, BreakRestMinMax         = 5, 60
	BreakSelfRestMinMin, BreakSelfRestMinMax = 5, 120
	BreakMessageMaxRunes                     = 120
	DefaultRestMessage                       = "Разомнись, попей воды, посмотри в окно"
)

// ErrBreakRules — настройки «Отдых» вне пределов FR-37.
var ErrBreakRules = errors.New("перерыв на отдых: настройки вне пределов")

// BreakRules — настройки «Отдых» ребёнка (FR-37, таблица break_rules). Нулевое значение —
// перерывы выключены. Message «» — стандартный текст (DefaultRestMessage).
type BreakRules struct {
	Enabled     bool
	AfterMin    int
	Counts      BreakCounts
	WarnMin     int
	RestMin     int
	SelfRestMin int
	Message     string
}

// DefaultBreakRules — настройки по умолчанию (FR-37): выключено, 60 / всё, кроме учёбы / 5 / 15 / 15.
func DefaultBreakRules() BreakRules {
	return BreakRules{AfterMin: 60, Counts: BreakAll, WarnMin: 5, RestMin: 15, SelfRestMin: 15}
}

// Validate проверяет пределы FR-37 (и при выключенном переключателе: значения сохраняются).
// «Засчитать как отдых» не меньше «Отдыха»; сообщение — до 120 символов, пустое — стандартное.
func (b BreakRules) Validate() error {
	in := func(v, lo, hi int) bool { return v >= lo && v <= hi }
	switch {
	case !in(b.AfterMin, BreakAfterMinMin, BreakAfterMinMax),
		!in(b.WarnMin, BreakWarnMinMin, BreakWarnMinMax),
		!in(b.RestMin, BreakRestMinMin, BreakRestMinMax),
		!in(b.SelfRestMin, BreakSelfRestMinMin, BreakSelfRestMinMax),
		b.SelfRestMin < b.RestMin,
		b.Counts != BreakAll && b.Counts != BreakGames,
		utf8.RuneCountInString(b.Message) > BreakMessageMaxRunes:
		return ErrBreakRules
	}
	return nil
}

// MessageText — сообщение на экране перерыва: свой текст родителя или стандартный.
func (b BreakRules) MessageText() string {
	if m := strings.TrimSpace(b.Message); m != "" {
		return m
	}
	return DefaultRestMessage
}

// BreakState — счёт к перерыву (таблица break_state; не по дням: идёт и через полночь).
// Моменты — UTC мс. Указатели не меняются на месте: ядро ставит новые значения.
type BreakState struct {
	CountedS     int    // засчитано к перерыву с прошлого отдыха, с
	QuietS       int    // тишина, накопленная до паузы (заморожена), с
	QuietSinceMs *int64 // с какого момента нет засчитанных секунд; nil — тишина не идёт
	BreakAtMs    *int64 // плашка показана: начало перерыва по часам; nil — предупреждения нет
	RestUntilMs  *int64 // конец идущего перерыва по часам; nil — перерыва нет
}

// Warned сообщает, что плашка «ПОРА ОТДОХНУТЬ» уже показана к ближайшему перерыву.
func (b BreakState) Warned() bool { return b.BreakAtMs != nil }

// Виды событий журнала перерыва.
const (
	EvRestBreak = "rest_break" // начался перерыв на отдых (R27)
	EvRestEnded = "rest_ended" // родитель закончил перерыв паролем
)

// Тексты перерыва.
const (
	ReasonRest       = "Перерыв на отдых"
	RestEndedText    = "Вы закончили перерыв"
	restAgainPrefix  = "Можно снова в "
	restPlateSuffix  = ". Сохрани игру"
	restBreakTextFmt = "Перерыв на отдых: %d мин после %d мин %s"
)

// RestUntilText — строка экрана перерыва «Можно снова в 16:20».
func RestUntilText(until time.Time) string { return restAgainPrefix + FormatClock(until) }

// RestPlateText — строка под плитками плашки «ПОРА ОТДОХНУТЬ» (8.2.2 п.7):
// «Через 5 минут перерыв на 15 минут. Сохрани игру».
func RestPlateText(warnMin, restMin int) string {
	return fmt.Sprintf("Через %d %s перерыв на %d %s", warnMin, minutesAcc(warnMin), restMin, minutesAcc(restMin)) + restPlateSuffix
}

// RestBreakText — запись журнала о начале перерыва: «Перерыв на отдых: 15 мин после 60 мин за
// компьютером» («… игр» в режиме «Только игры»).
func RestBreakText(b BreakRules) string {
	what := "за компьютером"
	if b.Counts == BreakGames {
		what = "игр"
	}
	return fmt.Sprintf(restBreakTextFmt, b.RestMin, b.AfterMin, what)
}

// minutesAcc — «минуту», «минуты», «минут» для числа n («через 1 минуту», «на 15 минут»).
func minutesAcc(n int) string {
	if n%100 >= 11 && n%100 <= 14 {
		return "минут"
	}
	switch n % 10 {
	case 1:
		return "минуту"
	case 2, 3, 4:
		return "минуты"
	}
	return "минут"
}

// Break возвращает состояние счёта к перерыву для сохранения (break_state).
func (e *Engine) Break() BreakState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.rest
}

// RestoreBreak ставит сохранённый счёт к перерыву (служба — сразу после NewEngine). Перерыв,
// который шёл по часам, продолжается до своего конца; закончившийся снимется на первом тике.
func (e *Engine) RestoreBreak(bs BreakState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rest = bs
}

// EndBreak — родитель паролем закончил перерыв (R27): экран исчезает, счёт к перерыву с нуля,
// событие EvRestEnded в журнал. ok = false — перерыва сейчас нет, ничего не меняется.
func (e *Engine) EndBreak() (Decision, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	closed := e.advance(now)
	e.restStep(now)
	if _, on := e.restActive(now); !on {
		return e.finish(e.decide(e.acc.State(), false), closed), false
	}
	e.rest = BreakState{}
	e.saveNow = true
	e.pending = append(e.pending, Event{Kind: EvRestEnded, Details: map[string]any{"text": RestEndedText}})
	return e.finish(e.decide(e.acc.State(), false), closed), true
}

// restOn — перерывы включены и правила прочитаны (R18: в безопасном режиме ограничений нет).
func (e *Engine) restOn() bool { return e.rules.Break.Enabled && !e.rules.Broken }

// pausedAt — пауза активна в момент now (R11).
func (e *Engine) pausedAt(now time.Time) bool {
	until, ok := e.pauseUntil(e.acc.State())
	return ok && now.Before(until)
}

// restActive возвращает конец перерыва, если в момент now он идёт по часам. Учитывает и
// наступивший момент начала, который тик ещё не перевёл в перерыв (Current между тиками).
func (e *Engine) restActive(now time.Time) (time.Time, bool) {
	if !e.restOn() || e.pausedAt(now) {
		return time.Time{}, false
	}
	var until time.Time
	switch {
	case e.rest.RestUntilMs != nil:
		until = time.UnixMilli(*e.rest.RestUntilMs)
	case e.rest.BreakAtMs != nil && !now.Before(time.UnixMilli(*e.rest.BreakAtMs)):
		until = time.UnixMilli(*e.rest.BreakAtMs).Add(time.Duration(e.rules.Break.RestMin) * time.Minute)
	default:
		return time.Time{}, false
	}
	return until.In(e.loc), now.Before(until)
}

// quietAt — сколько секунд подряд к моменту now не было засчитанных к перерыву секунд.
func (e *Engine) quietAt(now time.Time) int {
	q := e.rest.QuietS
	if e.rest.QuietSinceMs != nil {
		if d := now.Sub(time.UnixMilli(*e.rest.QuietSinceMs)); d > 0 {
			q += int(d / time.Second)
		}
	}
	return q
}

func msPtr(t time.Time) *int64 {
	ms := t.UnixMilli()
	return &ms
}

// restStep переводит счёт к перерыву по часам на момент now (перед решением тика и командами):
// выключено — счёт пуст; пауза — снимает перерыв и предупреждение, тишина замораживается;
// наступил момент начала — перерыв до «начало + отдых» (сохранить сразу, RF14-2; если он уже
// весь прошёл по часам, пока никого не было, — просто счёт с нуля); перерыв кончился — счёт с
// нуля; тишина не короче порога «засчитать как отдых» — сам отошёл: счёт с нуля, плашка снята.
func (e *Engine) restStep(now time.Time) {
	if !e.restOn() {
		e.rest = BreakState{}
		return
	}
	r := &e.rest
	if e.pausedAt(now) {
		if r.BreakAtMs != nil || r.RestUntilMs != nil {
			e.saveNow = true
		}
		r.QuietS = e.quietAt(now)
		r.QuietSinceMs, r.BreakAtMs, r.RestUntilMs = nil, nil, nil
		return
	}
	b := e.rules.Break
	if r.BreakAtMs != nil && !now.Before(time.UnixMilli(*r.BreakAtMs)) {
		until := time.UnixMilli(*r.BreakAtMs).Add(time.Duration(b.RestMin) * time.Minute)
		*r = BreakState{}
		e.saveNow = true
		if now.Before(until) {
			r.RestUntilMs = msPtr(until)
			e.pending = append(e.pending, Event{Kind: EvRestBreak, Details: map[string]any{
				"rest_min": b.RestMin, "after_min": b.AfterMin, "until_ms": until.UnixMilli(), "text": RestBreakText(b),
			}})
		}
	}
	if r.RestUntilMs != nil {
		if now.Before(time.UnixMilli(*r.RestUntilMs)) {
			return
		}
		*r = BreakState{}
		e.saveNow = true
	}
	if (r.CountedS > 0 || r.BreakAtMs != nil) && e.quietAt(now) >= b.SelfRestMin*60 {
		if r.BreakAtMs != nil {
			e.saveNow = true
		}
		r.CountedS, r.BreakAtMs = 0, nil
	}
}

// restWarn на тике без другого ограничения ставит предупреждение «ПОРА ОТДОХНУТЬ», когда к
// перерыву набралось «через сколько» минут (один раз на перерыв: момент начала сохраняется
// сразу и не ставится, пока он есть). Под экраном блокировки, в «только учёбе» и в паузе не ставит.
func (e *Engine) restWarn(d *Decision, now time.Time) {
	r := &e.rest
	if !e.restOn() || d.Lock != LockNone || d.StudyOnly || e.pausedAt(now) || r.BreakAtMs != nil || r.RestUntilMs != nil {
		return
	}
	b := e.rules.Break
	if r.CountedS < b.AfterMin*60 {
		return
	}
	r.BreakAtMs = msPtr(now.Add(time.Duration(b.WarnMin) * time.Minute))
	e.saveNow = true
	d.RestWarn, d.RestWarnS, d.RestText = true, b.WarnMin*60, RestPlateText(b.WarnMin, b.RestMin)
}

// restCount засчитывает секунду тика к перерыву (R27): секунда засчитана учётом (R2), не учёба,
// в режиме «Только игры» — игра. Незасчитанная секунда начинает тишину (если она ещё не идёт).
// В паузе и в перерыве счёт и тишина стоят.
func (e *Engine) restCount(now time.Time, counted bool, cat Category) {
	if !e.restOn() || e.pausedAt(now) {
		return
	}
	if _, on := e.restActive(now); on {
		return
	}
	r := &e.rest
	if counted && cat != CatStudy && (e.rules.Break.Counts == BreakAll || cat == CatGames) {
		r.CountedS++
		r.QuietS = 0
		r.QuietSinceMs = msPtr(now.Add(time.Second)) // тишина начнётся после этой секунды
		return
	}
	if r.QuietSinceMs == nil {
		r.QuietSinceMs = msPtr(now)
	}
}

// restDecide заполняет экран перерыва, если он идёт и нет более сильного экрана (R27).
func (e *Engine) restDecide(d *Decision, now time.Time) {
	until, on := e.restActive(now)
	if !on || d.Lock != LockNone || d.StudyOnly {
		return
	}
	d.Lock, d.Reason, d.Until = LockRest, ReasonRest, RestUntilText(until)
	d.RestUntilMs = until.UnixMilli()
	d.RestMessage = e.rules.Break.MessageText()
}
