package core

import (
	"slices"
	"testing"
	"time"
)

// Тесты последней минуты (R2, D-60, G13, пакет FB-12): после предупреждения за 1 мин до
// исчерпания общего лимита остаток идёт по часам до нуля, что бы ни показывали датчики.
// Все ожидания — литералы; лимит 120 мин (7200 с), остаток 3 мин в 16:57:00.

// awaySample — ребёнок вышел из учётки или отключил сеанс: секунда по R2 не засчитывается.
func awaySample() SessionSample { return SessionSample{SessionActive: false} }

// lastMinuteEngine — пн, лимит 120 мин, к 16:57:00 учтено 117 мин; ядро дотикало активным до
// 16:59:00 включительно (предупреждение 1 мин в 16:59:00, момент окончания 17:00:00).
func lastMinuteEngine(t *testing.T) (*Engine, *fakeClock) {
	t.Helper()
	c := &fakeClock{t: at(16, 57, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 117 * 60}, "")
	log := warnEvLog(e, c, at(16, 57, 0), at(16, 59, 1), activeSample())
	if want := []string{"16:59:00 1 limit"}; !slices.Equal(log, want) {
		t.Fatalf("предупреждения %v, want %v", log, want)
	}
	return e, c
}

func endMs(st DayState) int64 {
	if st.LastMinuteEndMs == nil {
		return 0
	}
	return *st.LastMinuteEndMs
}

// Ловит: порог 1 мин общего лимита не ставит момент окончания или ставит не «сейчас + остаток».
func TestLastMinuteWarningSetsEnd(t *testing.T) {
	e, _ := lastMinuteEngine(t)
	st, _ := e.State()
	if got, want := endMs(st), at(17, 0, 0).UnixMilli(); got != want {
		t.Fatalf("LastMinuteEndMs = %d, want %d (17:00:00)", got, want)
	}
}

// Ловит: простой без ввода в последнюю минуту останавливает остаток (R2а), блокировка не в 17:00:00.
func TestLastMinuteIdleRunsByClock(t *testing.T) {
	e, c := lastMinuteEngine(t)
	log := run(e, c, at(16, 59, 1), at(17, 0, 2), idleSample())
	if log.lockAt != "17:00:00" {
		t.Fatalf("блокировка %q, want 17:00:00 (последняя минута идёт по часам)", log.lockAt)
	}
	st, _ := e.State()
	if st.TotalS != 7200 {
		t.Fatalf("общее %d, want 7200", st.TotalS)
	}
}

// Ловит: остаток в середине простоя не уменьшается по секундам (табло стоит, S34).
func TestLastMinuteRemainingDecreasesEachSecond(t *testing.T) {
	e, c := lastMinuteEngine(t)
	run(e, c, at(16, 59, 1), at(16, 59, 30), idleSample())
	c.Set(at(16, 59, 30))
	if d := e.Current(); d.RemainingS != 30 || d.Lock != LockNone {
		t.Fatalf("в 16:59:30 остаток %d, Lock %v; want 30, без блокировки", d.RemainingS, d.Lock)
	}
	if d := e.Tick(idleSample()); d.RemainingS != 30 || d.UsedS != 7170 {
		t.Fatalf("тик 16:59:30: остаток %d, общее %d; want 30 и 7170", d.RemainingS, d.UsedS)
	}
}

// Ловит: после разрыва тиков (выход, сон 3 мин) пропущенное время не вычитается: первый же
// тик должен дать остаток 0 и режим окончания; итог дня 120:00, а не 119:10.
func TestLastMinuteGapLocksOnFirstTick(t *testing.T) {
	e, c := lastMinuteEngine(t)
	run(e, c, at(16, 59, 1), at(16, 59, 10), awaySample())
	c.Set(at(17, 2, 10))
	if d := e.Current(); d.Lock != LockTimeUp || d.RemainingS != 0 {
		t.Fatalf("Current после разрыва: Lock %v, остаток %d; want LockTimeUp и 0", d.Lock, d.RemainingS)
	}
	d := e.Tick(activeSample())
	if d.Lock != LockTimeUp || d.RemainingS != 0 || d.Used != "Сегодня: 2 ч 00 мин из 2 ч" {
		t.Fatalf("первый тик после разрыва: %+v", d)
	}
	if st, _ := e.State(); st.TotalS != 7200 {
		t.Fatalf("итог дня %d, want 7200", st.TotalS)
	}
}

// Ловит: после перезапуска (новое ядро из сохранённого состояния) момент не действует: в прошлом —
// нет блокировки сразу; в будущем — остаток считается по сохранённому общему, а не по часам.
func TestLastMinuteRestoredAfterRestart(t *testing.T) {
	end := at(17, 0, 0).UnixMilli()
	saved := DayState{Day: monday, TotalS: 7141, LastMinuteEndMs: &end} // сохранено в 16:59:01

	c := &fakeClock{t: at(17, 3, 0)}
	e := NewEngine(c, msk, rulesMon(120), saved, "")
	if d := e.Current(); d.Lock != LockTimeUp || d.RemainingS != 0 {
		t.Fatalf("момент в прошлом, Current: Lock %v остаток %d; want LockTimeUp и 0", d.Lock, d.RemainingS)
	}
	if d := e.Tick(activeSample()); d.Lock != LockTimeUp || len(d.Events) != 0 {
		t.Fatalf("момент в прошлом, первый тик: %+v", d)
	}

	c2 := &fakeClock{t: at(16, 59, 40)}
	e2 := NewEngine(c2, msk, rulesMon(120), saved, "")
	if d := e2.Current(); d.RemainingS != 20 {
		t.Fatalf("момент в будущем: остаток %d, want 20 (по часам до 17:00:00)", d.RemainingS)
	}
	log := warnEvLog(e2, c2, at(16, 59, 40), at(17, 0, 1), idleSample())
	if len(log) != 0 {
		t.Fatalf("после перезапуска повторные предупреждения %v, want нет", log)
	}
	if st, _ := e2.State(); st.TotalS != 7200 {
		t.Fatalf("после перезапуска итог %d, want 7200", st.TotalS)
	}
}

// Ловит: доп. время, копилка и промокод не снимают отсчёт (время продолжает идти по часам при
// выходе) или теряют уже прошедшие секунды последней минуты. Остаток — прежний + добавленное.
func TestLastMinuteCancelledByAddedTime(t *testing.T) {
	adds := []struct {
		name string
		do   func(e *Engine) (Decision, error)
	}{
		{"доп. время +15", func(e *Engine) (Decision, error) { return e.AddExtra(15) }},
		{"копилка 15", func(e *Engine) (Decision, error) { return e.SpendBonus(15) }},
		{"промокод 15", func(e *Engine) (Decision, error) { return e.RedeemPromoMinutes(15) }},
	}
	for _, a := range adds {
		t.Run(a.name, func(t *testing.T) {
			e, c := lastMinuteEngine(t)
			run(e, c, at(16, 59, 1), at(16, 59, 30), idleSample()) // остаток по часам 30 с
			c.Set(at(16, 59, 30))
			d, err := a.do(e)
			if err != nil {
				t.Fatal(err)
			}
			if d.RemainingS != 930 {
				t.Fatalf("после добавления остаток %d, want 930 (30 с + 15 мин)", d.RemainingS)
			}
			if st, _ := e.State(); st.LastMinuteEndMs != nil {
				t.Fatalf("отсчёт не снят: %d", *st.LastMinuteEndMs)
			}
			// Ребёнок вышел на 5 мин: время снова по R2, остаток не тронут.
			run(e, c, at(16, 59, 30), at(17, 4, 30), awaySample())
			c.Set(at(17, 4, 30))
			if d := e.Tick(awaySample()); d.RemainingS != 930 || d.Lock != LockNone {
				t.Fatalf("после выхода остаток %d Lock %v; want 930 без блокировки", d.RemainingS, d.Lock)
			}
		})
	}
}

// Ловит: пауза не снимает отсчёт: после паузы остаток уже «сгорел» по часам.
func TestLastMinuteCancelledByPause(t *testing.T) {
	e, c := lastMinuteEngine(t)
	run(e, c, at(16, 59, 1), at(16, 59, 20), idleSample()) // остаток 40 с
	c.Set(at(16, 59, 20))
	if _, err := e.Pause(at(17, 9, 20)); err != nil {
		t.Fatal(err)
	}
	if st, _ := e.State(); st.LastMinuteEndMs != nil {
		t.Fatalf("пауза не сняла отсчёт: %d", *st.LastMinuteEndMs)
	}
	run(e, c, at(16, 59, 20), at(17, 9, 20), awaySample())
	c.Set(at(17, 9, 21))
	if d := e.Tick(awaySample()); d.RemainingS != 40 || d.Lock != LockNone {
		t.Fatalf("после паузы остаток %d Lock %v; want 40 без блокировки", d.RemainingS, d.Lock)
	}
}

// Ловит: увеличение лимита правилами не снимает отсчёт (добавленное «сгорает» по часам);
// сохранение тех же правил снимает отсчёт (лазейка: пересохранил — и последняя минута на паузе).
func TestLastMinuteSetRules(t *testing.T) {
	e, c := lastMinuteEngine(t)
	run(e, c, at(16, 59, 1), at(16, 59, 30), idleSample())
	c.Set(at(16, 59, 30))
	e.SetRules(rulesMon(120))
	if st, _ := e.State(); endMs(st) != at(17, 0, 0).UnixMilli() {
		t.Fatalf("те же правила сняли или сдвинули отсчёт: %+v", st)
	}
	d := e.SetRules(rulesMon(150))
	if d.RemainingS != 1830 {
		t.Fatalf("после увеличения лимита остаток %d, want 1830", d.RemainingS)
	}
	if st, _ := e.State(); st.LastMinuteEndMs != nil {
		t.Fatal("увеличение лимита не сняло отсчёт")
	}
}

// Ruling FB12-1: уменьшение доп. времени (и правил) отсчёт не снимает, а сдвигает момент раньше
// на убранное; обнуление отрицательного доп. времени увеличивает лимит и снимает отсчёт.
func TestLastMinuteReduceExtraMovesEndEarlier(t *testing.T) {
	c := &fakeClock{t: at(17, 7, 0)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 127 * 60, ExtraMin: 10}, "")
	warnEvLog(e, c, at(17, 7, 0), at(17, 9, 1), activeSample()) // 1 мин в 17:09:00, конец 17:10:00
	run(e, c, at(17, 9, 1), at(17, 9, 10), idleSample())        // остаток 50 с
	c.Set(at(17, 9, 10))
	d, removed, err := e.ReduceExtra(10)
	if err != nil || removed != 10 {
		t.Fatalf("ReduceExtra: %d, %v", removed, err)
	}
	if d.RemainingS != 0 || d.Lock != LockTimeUp {
		t.Fatalf("после −10: остаток %d Lock %v; want 0 и LockTimeUp", d.RemainingS, d.Lock)
	}

	// Момент сдвинут раньше на убранное (17:10:00 − 10 мин), а не снят: выход не возвращает время.
	if st, _ := e.State(); endMs(st) != at(17, 0, 0).UnixMilli() {
		t.Fatalf("после −10 момент %d, want 17:00:00", endMs(st))
	}
	run(e, c, at(17, 9, 10), at(17, 12, 0), awaySample())
	if d := e.Current(); d.RemainingS != 0 || d.Lock != LockTimeUp {
		t.Fatalf("после выхода остаток %d Lock %v; want 0 и LockTimeUp", d.RemainingS, d.Lock)
	}

	// Отрицательное доп. время −10 и обнуление: лимит растёт на 10 мин — отсчёт снят.
	c3 := &fakeClock{t: at(16, 47, 0)}
	e3 := NewEngine(c3, msk, rulesMon(120), DayState{Day: monday, TotalS: 107 * 60, ExtraMin: -10}, "")
	warnEvLog(e3, c3, at(16, 47, 0), at(16, 49, 1), activeSample()) // 1 мин в 16:49:00
	c3.Set(at(16, 49, 1))
	d3, _ := e3.ClearExtra()
	if st, _ := e3.State(); st.LastMinuteEndMs != nil || d3.RemainingS != 659 {
		t.Fatalf("обнуление −10: момент %v, остаток %d; want снят и 659", st.LastMinuteEndMs, d3.RemainingS)
	}
}

// Ловит: момент переходит в новые сутки (R16) или момент прошлого дня применяется сегодня.
func TestLastMinuteDayChange(t *testing.T) {
	c := &fakeClock{t: time.Date(2026, 10, 19, 23, 57, 0, 0, msk)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 117 * 60}, "")
	warnEvLog(e, c, c.t, time.Date(2026, 10, 19, 23, 59, 1, 0, msk), activeSample()) // 1 мин в 23:59:00, конец 00:00:00
	run(e, c, time.Date(2026, 10, 19, 23, 59, 1, 0, msk), time.Date(2026, 10, 19, 23, 59, 10, 0, msk), awaySample())
	c.Set(time.Date(2026, 10, 20, 0, 5, 0, 0, msk))
	var r Rules
	r.Week[0], r.Week[1] = LimitOf(120), LimitOf(60)
	dr := e.SetRules(r)
	// Вчерашняя последняя минута дошла до нуля в 00:00:00 по часам: итог закрытого дня 120:00.
	if dr.ClosedDay == nil || dr.ClosedDay.Day != monday || dr.ClosedDay.TotalS != 7200 {
		t.Fatalf("закрытый день %+v, want пн 7200 с", dr.ClosedDay)
	}
	d := e.Tick(awaySample())
	st, _ := e.State()
	if st.Day != tuesday || st.LastMinuteEndMs != nil || st.TotalS != 0 || d.RemainingS != 3600 {
		t.Fatalf("новый день: %+v, остаток %d; want вт без момента, 0 с, остаток 3600", st, d.RemainingS)
	}

	// Сохранённый вчерашний день с моментом: сегодняшний учёт его не берёт.
	end := time.Date(2026, 10, 19, 23, 59, 0, 0, msk).UnixMilli()
	c2 := &fakeClock{t: time.Date(2026, 10, 20, 10, 0, 0, 0, msk)}
	e2 := NewEngine(c2, msk, r, DayState{Day: monday, TotalS: 7170, LastMinuteEndMs: &end}, "")
	if d := e2.Current(); d.RemainingS != 3600 || d.Lock != LockNone {
		t.Fatalf("вчерашний момент применён: %+v", d)
	}
}

// Ловит (RF12-4): порог 1 мин запретного интервала ставит момент окончания.
func TestLastMinuteNotSetByForbiddenWarning(t *testing.T) {
	r := rulesMon(300)
	r.Schedule = []Interval{iv(Monday, 21*60, 7*60)}
	c := &fakeClock{t: at(20, 58, 0)}
	e := NewEngine(c, msk, r, DayState{Day: monday, TotalS: 60 * 60}, "")
	log := warnEvLog(e, c, at(20, 58, 0), at(20, 59, 1), activeSample())
	if want := []string{"20:59:00 1 forbidden"}; !slices.Equal(log, want) {
		t.Fatalf("предупреждения %v, want %v", log, want)
	}
	if st, _ := e.State(); st.LastMinuteEndMs != nil {
		t.Fatalf("порог интервала поставил момент %d", *st.LastMinuteEndMs)
	}
}

// Ловит (RF12-4 для игр): порог 1 мин лимита игр ставит момент общего окончания.
func TestLastMinuteNotSetByGamesWarning(t *testing.T) {
	c := &fakeClock{t: at(15, 58, 0)}
	e := NewEngine(c, msk, gamesMon(intp(120), intp(60), EndBlock), DayState{Day: monday, TotalS: 58 * 60, GamesS: 58 * 60}, "")
	log := warnEvLog(e, c, at(15, 58, 0), at(15, 59, 1), gameSample())
	if want := []string{"15:59:00 1 games"}; !slices.Equal(log, want) {
		t.Fatalf("предупреждения %v, want %v", log, want)
	}
	if st, _ := e.State(); st.LastMinuteEndMs != nil {
		t.Fatalf("порог игр поставил момент %d", *st.LastMinuteEndMs)
	}
}

// Ловит (RF12-3): момент не сохраняется сразу: DueSave на следующем тике после порога молчит,
// и выключение питания в последнюю минуту теряет отсчёт.
func TestLastMinuteDueSaveRightAfterSet(t *testing.T) {
	c := &fakeClock{t: at(16, 58, 50)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 7130}, "")
	for tt := at(16, 58, 50); tt.Before(at(16, 59, 1)); tt = tt.Add(time.Second) {
		c.Set(tt)
		e.DueSave(tt)
		e.Tick(activeSample())
	}
	// Отсчёт сохранений от 16:58:50: следующий плановый слот только в 16:59:20.
	if !e.DueSave(at(16, 59, 1)) {
		t.Fatal("DueSave(16:59:01) = false сразу после установки момента, want true")
	}
	if e.DueSave(at(16, 59, 2)) {
		t.Fatal("DueSave(16:59:02) = true: лишнее сохранение")
	}
	if !e.DueSave(at(16, 59, 20)) {
		t.Fatal("плановое сохранение 16:59:20 пропало")
	}
}

// Ловит: доля секунды до момента округляется вниз, и время «выходит» раньше момента окончания
// (часы службы идут не по целым секундам).
func TestLastMinuteZeroNotBeforeEnd(t *testing.T) {
	end := at(17, 0, 0).UnixMilli()
	c := &fakeClock{t: at(16, 59, 59).Add(300 * time.Millisecond)}
	e := NewEngine(c, msk, rulesMon(120), DayState{Day: monday, TotalS: 7141, LastMinuteEndMs: &end}, "")
	if d := e.Current(); d.RemainingS != 1 || d.Lock != LockNone {
		t.Fatalf("за 0,7 с до момента остаток %d Lock %v; want 1 без блокировки", d.RemainingS, d.Lock)
	}
}
