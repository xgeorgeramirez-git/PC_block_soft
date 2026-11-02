package service

import (
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Последняя минута в службе (FB-12, D-60, RF12-3): момент окончания сохраняется сразу после
// предупреждения за 1 мин, перезапуск службы его не теряет, вход ребёнка после момента — обычный
// экран «Время вышло» без записи об обходе.

// savedEnd — момент последней минуты из последнего сохранения ребёнка; 0 — не сохранён.
func (f *fakeStore) savedEnd(childID int64) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.saves) - 1; i >= 0; i-- {
		if f.saves[i].childID == childID {
			if e := f.saves[i].st.LastMinuteEndMs; e != nil {
				return *e
			}
			return 0
		}
	}
	return 0
}

// Ловит (RF12-3): момент последней минуты сохраняется только плановым сохранением раз в 30 с —
// выключение питания сразу после предупреждения теряет отсчёт. Лимит 2 мин, первый тик 15:00:01:
// порог 1 мин на тике 15:01:01 (учтено 60 с), плановые сохранения — 15:00:31 и 15:01:01 (до
// тика), следующее плановое только в 15:01:31.
func TestLastMinuteSavedRightAfterWarning(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 2))
	h.start()
	h.childLogsIn()
	id := h.childID(childSID)
	h.run(61, activeSample) // 15:00:01 … 15:01:01: на тике 15:01:01 сработал порог 1 мин
	if got := h.store.savedEnd(id); got != 0 {
		t.Fatalf("момент сохранён до порога: %d", got)
	}
	h.run(1, activeSample) // 15:01:02: ближайший тик после установки
	if got, want := h.store.savedEnd(id), time.Date(2026, 10, 19, 15, 2, 1, 0, tzMSK).UnixMilli(); got != want {
		t.Fatalf("сохранённый момент %d, want %d (15:02:01) сразу на следующем тике", got, want)
	}
}

// Ловит: служба, запущенная после выключения в последнюю минуту, не применяет сохранённый
// прошедший момент (остаток не 0, блокировки нет), а вход ребёнка пишется «попыткой обхода».
func TestLastMinuteRestartWithPassedEnd(t *testing.T) {
	st := newFakeStore()
	id, err := st.UpsertChild(childSID, "Миша")
	if err != nil {
		t.Fatal(err)
	}
	var week [7]core.DayLimit
	for i := range week {
		week[i] = core.LimitOf(120)
	}
	st.SetDayLimits(id, week)
	end := time.Date(2026, 10, 19, 17, 0, 0, 0, tzMSK).UnixMilli()
	st.days[dayKey(id, "2026-10-19")] = core.DayState{Day: "2026-10-19", TotalS: 7150, LastMinuteEndMs: &end}

	h := newHarness(t, withStore(st))
	h.clock.Set(time.Date(2026, 10, 19, 17, 1, 0, 0, tzMSK))
	h.start()
	if s := h.status(); s.RemainingS != 0 || s.Status != ipc.StatusTimeUp || s.Lock != ipc.LockTimeUp {
		t.Fatalf("сразу после старта: остаток %d, статус %q, экран %q; want 0, timeUp, timeUp", s.RemainingS, s.Status, s.Lock)
	}
	h.childLogsIn()
	reps := h.run(5, activeSample)
	if r := reps[0]; r.Status.Lock != ipc.LockTimeUp || r.Status.Used != "Сегодня: 2 ч 00 мин из 2 ч" {
		t.Fatalf("первый опрос стража: %+v; want экран «Время вышло» и 2 ч 00 мин", r.Status)
	}
	h.hk(60)
	if n := h.bypassCount(); n != 0 {
		t.Fatalf("записей об обходе %d; want 0: %+v", n, h.store.eventsOf("bypass"))
	}
	if s := h.status(); s.UsedS != 7200 {
		t.Fatalf("общее %d, want 7200", s.UsedS)
	}
}

// Ловит: служба после перезапуска с будущим моментом считает остаток по R2 (простой
// останавливает время), а не по часам до сохранённого момента.
func TestLastMinuteRestartWithFutureEnd(t *testing.T) {
	st := newFakeStore()
	id, _ := st.UpsertChild(childSID, "Миша")
	var week [7]core.DayLimit
	for i := range week {
		week[i] = core.LimitOf(120)
	}
	st.SetDayLimits(id, week)
	end := time.Date(2026, 10, 19, 17, 0, 0, 0, tzMSK).UnixMilli()
	st.days[dayKey(id, "2026-10-19")] = core.DayState{Day: "2026-10-19", TotalS: 7141, LastMinuteEndMs: &end}

	h := newHarness(t, withStore(st))
	h.clock.Set(time.Date(2026, 10, 19, 16, 59, 30, 0, tzMSK))
	h.start()
	if s := h.status(); s.RemainingS != 30 {
		t.Fatalf("после старта остаток %d, want 30", s.RemainingS)
	}
	h.childLogsIn()
	h.run(31, idleSample) // 16:59:31 … 17:00:01 без ввода
	if s := h.status(); s.RemainingS != 0 || s.Lock != ipc.LockTimeUp {
		t.Fatalf("к 17:00:01 остаток %d, экран %q; want 0 и timeUp", s.RemainingS, s.Lock)
	}
}
