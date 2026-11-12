package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Автобонус за дни, когда полночь прошла при выключенном компьютере (FB-21, D-82, R25, S51).

// tue24, wed25 — вторник 24.11.2026 и среда 25.11.2026 (строки среды в базе нет); thu1000 — четверг 10:00.
const (
	tue24 = "2026-11-24"
	wed25 = "2026-11-25"
)

var thu1000 = time.Date(2026, 11, 26, 10, 0, 0, 0, tzMSK)

// catchupStore — база S51: Миша с лимитом 120 мин, во вторник 40 мин, бонус за вторник не рассмотрен.
func catchupStore(t *testing.T) (*fakeStore, int64) {
	t.Helper()
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	id := h.childID(childSID)
	h.store.days[dayKey(id, tue24)] = core.DayState{Day: tue24, TotalS: 40 * 60, GamesS: 40 * 60}
	return h.store, id
}

// bonusOf — баланс копилки и записи автобонуса в журнале.
func bonusOf(t *testing.T, st *fakeStore, id int64) (int, []StoredEvent) {
	t.Helper()
	st.mu.Lock()
	bal := 0
	if h := st.bonus[id]; len(h) > 0 {
		bal = h[len(h)-1].Balance
	}
	st.mu.Unlock()
	return bal, st.eventsOf(core.EvBonusGranted)
}

// startAt — служба на базе st с часами ядра at (перезапуск после выключения: без Shutdown).
func startAt(t *testing.T, st *fakeStore, at time.Time) *harness {
	t.Helper()
	h := newHarness(t, withStore(st), withClockAt(at))
	h.start()
	return h
}

// Ловит (S51, FB-21): после выключения до полуночи бонус за вторник не начисляется никогда (строка
// осталась с bonus_granted = 0); начисление дважды при втором перезапуске; строка не помечена;
// другие поля строки дня затёрты пометкой.
func TestBonusCatchUpS51(t *testing.T) {
	st, id := catchupStore(t)
	startAt(t, st, thu1000)
	bal, evs := bonusOf(t, st, id)
	if bal != 40 || len(evs) != 1 {
		t.Fatalf("после запуска в четверг: копилка %d, записей %d; want 40 и 1 (5 × 8 полных десятков из 80 мин)", bal, len(evs))
	}
	if evs[0].ChildID != id || evs[0].Details["day"] != tue24 || detInt(evs[0].Details, "minutes") != 40 {
		t.Errorf("запись журнала %+v; want вторник +40", evs[0])
	}
	if txt := detStr(evs[0].Details, "text"); txt != "Бонус за свободное время 24 ноября: +40 мин" {
		t.Errorf("текст в подробностях %q; want дата словами", txt)
	}
	if txt := eventText(evs[0].Kind, "Миша", evs[0].Details); txt != "Миша: бонус за свободное время 24 ноября: +40 мин" {
		t.Errorf("текст журнала %q", txt)
	}
	row := st.days[dayKey(id, tue24)]
	if !row.BonusGranted || row.TotalS != 40*60 || row.GamesS != 40*60 {
		t.Errorf("строка вторника %+v; want помечена, учёт прежний", row)
	}
	if _, ok := st.days[dayKey(id, wed25)]; ok {
		t.Error("появилась строка среды: служба не должна выдумывать дни")
	}

	startAt(t, st, thu1000.Add(time.Hour))
	if bal, evs := bonusOf(t, st, id); bal != 40 || len(evs) != 1 {
		t.Errorf("второй перезапуск: копилка %d, записей %d; want без изменений", bal, len(evs))
	}
}

// Ловит (R25): за полностью запретный день бонус начислен; день не помечен (каждый запуск
// пересчитывает его заново); потолок копилки 600 не соблюдён.
func TestBonusCatchUpForbiddenAndCeiling(t *testing.T) {
	st, id := catchupStore(t)
	st.schedule[id] = []core.Interval{{Weekday: core.Tuesday, StartMin: 0, EndMin: 0}} // весь день
	startAt(t, st, thu1000)
	if bal, evs := bonusOf(t, st, id); bal != 0 || len(evs) != 0 {
		t.Errorf("запретный вторник: копилка %d, записей %d; want 0", bal, len(evs))
	}
	if !st.days[dayKey(id, tue24)].BonusGranted {
		t.Error("запретный вторник не помечен рассмотренным")
	}

	st, id = catchupStore(t)
	st.bonus[id] = []BonusRecord{{AtMs: 1, DeltaMin: 590, Reason: "Подарено родителем", Balance: 590}}
	startAt(t, st, thu1000)
	if bal, evs := bonusOf(t, st, id); bal != 600 || len(evs) != 1 || detInt(evs[0].Details, "minutes") != 10 {
		t.Errorf("копилка 590: стало %d, записи %+v; want 600 и +10 (потолок)", bal, evs)
	}
	if !st.days[dayKey(id, tue24)].BonusGranted {
		t.Error("вторник не помечен после начисления до потолка")
	}
}

// Ловит (R18): сбой чтения копилки или списка дней помечает день (бонус потерян навсегда) или
// роняет запуск; правила не прочитались (безопасный режим, лимиты не известны) — день помечен без
// бонуса; после восстановления бонус начисляется при следующем запуске.
func TestBonusCatchUpErrors(t *testing.T) {
	for _, fail := range []string{"balance", "ungranted", "rules"} {
		st, id := catchupStore(t)
		st.failBalance, st.failUngrant, st.failSchedule = fail == "balance", fail == "ungranted", fail == "rules"
		h := startAt(t, st, thu1000)
		if bal, evs := bonusOf(t, st, id); bal != 0 || len(evs) != 0 {
			t.Errorf("%s: копилка %d, записей %d; want 0", fail, bal, len(evs))
		}
		if st.days[dayKey(id, tue24)].BonusGranted {
			t.Errorf("%s: вторник помечен, бонус потерян", fail)
		}
		if fail == "ungranted" && !strings.Contains(h.logText(), "автобонус") {
			t.Errorf("%s: сбой не в журнале работы:\n%s", fail, h.logText())
		}
		st.failBalance, st.failUngrant, st.failSchedule = false, false, false
		startAt(t, st, thu1000.Add(time.Hour))
		if bal, evs := bonusOf(t, st, id); bal != 40 || len(evs) != 1 {
			t.Errorf("%s: после восстановления копилка %d, записей %d; want 40 и 1", fail, bal, len(evs))
		}
	}
}

// Ловит (R25, Review Focus T7c): порядок с закрытием дня ядром даёт два бонуса или ни одного:
// (а) выключение в 23:59:50 и запуск в 00:00:02 — бонус за вторник даёт догоняющее начисление,
// ядро вторник не закрывает (оно начало со среды); (б) служба работала через полночь — бонус даёт
// ядро (ClosedDay), перезапуск в среду ничего не добавляет; (в) запуск в 23:59:58, полночь при
// работающей службе.
func TestBonusCatchUpOrderWithClosedDay(t *testing.T) {
	cases := []struct {
		name     string
		start    time.Time
		run      int  // секунд работы до полуночи и после
		shutdown bool // штатная остановка, иначе выключение питания
		restart  time.Time
	}{
		{"выключение до полуночи", time.Date(2026, 11, 24, 23, 59, 0, 0, tzMSK), 50, false, time.Date(2026, 11, 25, 0, 0, 2, 0, tzMSK)},
		{"полночь при работе, затем перезапуск", time.Date(2026, 11, 24, 23, 59, 0, 0, tzMSK), 70, true, time.Date(2026, 11, 25, 0, 5, 0, 0, tzMSK)},
		{"запуск перед полуночью", time.Date(2026, 11, 24, 23, 59, 58, 0, tzMSK), 10, true, time.Date(2026, 11, 25, 9, 0, 0, 0, tzMSK)},
	}
	for _, c := range cases {
		st, id := catchupStore(t)
		h := startAt(t, st, c.start)
		h.childLogsIn()
		h.run(c.run, idleSample) // сеанс не активен: вторник не прирастает, бонус всегда 40
		if c.shutdown {
			h.svc.Shutdown(h.real.Now())
		}
		h2 := startAt(t, st, c.restart)
		h2.childLogsIn()
		h2.run(5, idleSample)
		bal, evs := bonusOf(t, st, id)
		if len(evs) != 1 || evs[0].Details["day"] != tue24 {
			t.Errorf("%s: записи автобонуса %+v; want одна за вторник", c.name, evs)
		}
		if bal != 40 {
			t.Errorf("%s: копилка %d, want 40", c.name, bal)
		}
		if !st.days[dayKey(id, tue24)].BonusGranted {
			t.Errorf("%s: вторник не помечен", c.name)
		}
	}
}

// Ловит (D-82, PL-22): засеянный TestSeedDay день без bonusPending даёт автобонус при запуске;
// с bonusPending — не даёт (приёмка S51 не проверяет то, что пишет программа).
func TestBonusCatchUpSeedDay(t *testing.T) {
	for _, pending := range []bool{false, true} {
		h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withTestMode(TestMode{}), withClockAt(thu1000))
		h.start()
		id := h.childID(childSID)
		h.ok(adminPeer(), "", ipc.CmdTestSeedDay, ipc.TestSeedDayRequest{ChildID: id, Day: tue24, TotalS: 40 * 60, BonusPending: pending}, nil)
		h.svc.Shutdown(h.real.Now())
		startAt(t, h.store, thu1000.Add(time.Minute))
		want := 0
		if pending {
			want = 40
		}
		if bal, _ := bonusOf(t, h.store, id); bal != want {
			t.Errorf("bonusPending=%v: копилка %d, want %d", pending, bal, want)
		}
	}
}

// Ловит (R25 «один раз», ревью M5): начисление в копилку и отметка дня — двумя записями; сбой
// записи между ними (диск, выключение) оставляет бонус без отметки, и следующий запуск начисляет
// его снова. Оба пути: догоняющее начисление при запуске и закрытие дня ядром в полночь.
func TestBonusGrantAtomicWithDayMark(t *testing.T) {
	// Догоняющее начисление при запуске в четверг.
	st, id := catchupStore(t)
	st.failSave = true
	startAt(t, st, thu1000)
	if bal, evs := bonusOf(t, st, id); bal != 0 || len(evs) != 0 {
		t.Errorf("запуск со сбоем записи: копилка %d, записей %d; want 0 (ни бонуса, ни отметки)", bal, len(evs))
	}
	if st.days[dayKey(id, tue24)].BonusGranted {
		t.Error("запуск со сбоем записи: вторник помечен без бонуса")
	}
	st.failSave = false
	startAt(t, st, thu1000.Add(time.Hour))
	startAt(t, st, thu1000.Add(2*time.Hour))
	if bal, evs := bonusOf(t, st, id); bal != 40 || len(evs) != 1 {
		t.Errorf("после восстановления: копилка %d, записей %d; want ровно 40 и 1", bal, len(evs))
	}
	if !st.days[dayKey(id, tue24)].BonusGranted {
		t.Error("после восстановления вторник не помечен")
	}

	// Закрытие дня ядром: полночь при работающей службе со сбоем записи, затем перезапуск.
	st, id = catchupStore(t)
	h := startAt(t, st, time.Date(2026, 11, 24, 23, 59, 0, 0, tzMSK))
	h.childLogsIn()
	st.mu.Lock()
	st.failSave = true
	st.mu.Unlock()
	h.run(70, idleSample)
	if bal, _ := bonusOf(t, st, id); bal != 0 {
		t.Errorf("полночь со сбоем записи: копилка %d, want 0", bal)
	}
	st.mu.Lock()
	st.failSave = false
	st.mu.Unlock()
	startAt(t, st, time.Date(2026, 11, 25, 9, 0, 0, 0, tzMSK))
	startAt(t, st, time.Date(2026, 11, 25, 10, 0, 0, 0, tzMSK))
	if bal, evs := bonusOf(t, st, id); bal != 40 || len(evs) != 1 || evs[0].Details["day"] != tue24 {
		t.Errorf("после перезапуска: копилка %d, записи %+v; want 40 и одна запись за вторник", bal, evs)
	}
}

// Ловит (FB-21, D-82, R18): полночь прошла при работающей службе в безопасном режиме (правила не
// прочитались, лимиты не известны) — день помечен рассмотренным без бонуса, и догоняющее начисление
// при следующем запуске с нормальными правилами его уже не видит; строка дня не сохранена.
func TestBonusSafeModeMidnightWaitsForCatchUp(t *testing.T) {
	st, id := catchupStore(t)
	st.failSchedule = true
	h := startAt(t, st, time.Date(2026, 11, 24, 23, 59, 0, 0, tzMSK))
	h.childLogsIn()
	h.run(70, idleSample) // полночь при работающей службе
	h.svc.Shutdown(h.real.Now())
	if bal, evs := bonusOf(t, st, id); bal != 0 || len(evs) != 0 {
		t.Fatalf("безопасный режим: копилка %d, записей %d; want 0", bal, len(evs))
	}
	row, ok := st.days[dayKey(id, tue24)]
	if !ok || row.BonusGranted || row.TotalS != 40*60 {
		t.Fatalf("строка вторника %+v (есть %v); want сохранена с bonus_granted = 0 и учётом 40 мин", row, ok)
	}

	st.failSchedule = false
	h2 := startAt(t, st, time.Date(2026, 11, 25, 9, 0, 0, 0, tzMSK))
	h2.childLogsIn()
	h2.run(5, idleSample)
	h2.svc.Shutdown(h2.real.Now())
	startAt(t, st, time.Date(2026, 11, 25, 10, 0, 0, 0, tzMSK))
	bal, evs := bonusOf(t, st, id)
	if bal != 40 || len(evs) != 1 || evs[0].Details["day"] != tue24 {
		t.Errorf("после запуска с правилами: копилка %d, записи %+v; want 40 и одна запись за вторник", bal, evs)
	}
	if !st.days[dayKey(id, tue24)].BonusGranted {
		t.Error("вторник не помечен после догоняющего начисления")
	}
}

// Ловит (FB-21): строка автобонуса в журнале с датой «2026-11-24» вместо «24 ноября», в том числе
// у записей прежних версий (в подробностях текст с датой цифрами, день и минуты есть).
func TestBonusGrantedJournalText(t *testing.T) {
	old := map[string]any{}
	if err := json.Unmarshal([]byte(`{"minutes":40,"balance":40,"day":"2026-11-24","text":"Бонус за свободное время 2026-11-24: +40 мин"}`), &old); err != nil {
		t.Fatal(err)
	}
	if got := eventText(core.EvBonusGranted, "Миша", old); got != "Миша: бонус за свободное время 24 ноября: +40 мин" {
		t.Errorf("запись прежней версии: %q", got)
	}
	if got := eventText(core.EvBonusGranted, "", map[string]any{"text": "Бонус за свободное время: +5 мин"}); got != "Бонус за свободное время: +5 мин" {
		t.Errorf("без дня: %q; want текст как есть", got)
	}
}
