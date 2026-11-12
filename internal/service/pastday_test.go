package service

import (
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Итог прошлого дня при запуске после полуночи (B-42, G13, R2, D-60): последняя минута шла по
// часам, пока компьютер был выключен, и строка дня дотягивается до его полуночи.

// lastMinuteRow — вторник 24.11 Миши (лимит 120 мин): последняя минута с концом endAt, общее totalS,
// игры 100 мин; бонус за вторник не рассмотрен.
func lastMinuteRow(t *testing.T, totalS int, endAt time.Time) (*fakeStore, int64) {
	t.Helper()
	st, id := catchupStore(t)
	end := endAt.UnixMilli()
	st.days[dayKey(id, tue24)] = core.DayState{Day: tue24, TotalS: totalS, GamesS: 100 * 60, LastMinuteEndMs: &end}
	return st, id
}

// Ловит (B-42): компьютер выключили в 23:59:30 вторника, последняя минута кончалась бы в 00:00:20
// среды. Запуск в четверг: строка вторника осталась 1 ч 59 мин 10 с (секунды до полуночи
// потеряны) и с моментом последней минуты. Ждём: до полуночи осталось 20 с отсчёта — общее
// 2 ч 00 мин − 20 с = 7180 с, дотяжка в «Прочее» (игры прежние), момент снят, бонус 0 (остаток
// меньше 10 мин), строка помечена.
func TestPastDayLastMinuteAtMidnight(t *testing.T) {
	st, id := lastMinuteRow(t, 7150, time.Date(2026, 11, 25, 0, 0, 20, 0, tzMSK))
	startAt(t, st, thu1000)
	row := st.days[dayKey(id, tue24)]
	if row.TotalS != 7180 || row.GamesS != 100*60 || row.LastMinuteEndMs != nil || !row.BonusGranted {
		t.Errorf("строка вторника %+v; want общее 7180, игры 6000, момента нет, бонус рассмотрен", row)
	}
	if bal, evs := bonusOf(t, st, id); bal != 0 || len(evs) != 0 {
		t.Errorf("копилка %d, записей %d; want 0", bal, len(evs))
	}
	startAt(t, st, thu1000.Add(time.Hour))
	if row2 := st.days[dayKey(id, tue24)]; row2.TotalS != 7180 {
		t.Errorf("второй запуск: общее %d; want 7180 без повторной дотяжки", row2.TotalS)
	}
}

// Ловит (B-42, порядок с FB-21): бонус за прошлый день считается по строке до дотяжки. Конец
// последней минуты 23:59:30 вторника — до полуночи; в строке 40 мин (сохранение отстало). По
// часам время вышло целиком: общее 2 ч 00 мин, остатка нет — бонус 0, а не +40 за 80 «свободных»
// минут. Сбой копилки: дотянутая строка всё равно сохраняется, бонус ждёт следующего запуска.
func TestPastDayPullUpBeforeBonus(t *testing.T) {
	st, id := lastMinuteRow(t, 40*60, time.Date(2026, 11, 24, 23, 59, 30, 0, tzMSK))
	st.days[dayKey(id, tue24)] = func(r core.DayState) core.DayState { r.GamesS = 40 * 60; return r }(st.days[dayKey(id, tue24)])
	startAt(t, st, thu1000)
	if bal, evs := bonusOf(t, st, id); bal != 0 || len(evs) != 0 {
		t.Errorf("копилка %d, записей %d; want 0 (бонус после дотяжки)", bal, len(evs))
	}
	if row := st.days[dayKey(id, tue24)]; row.TotalS != 7200 || row.LastMinuteEndMs != nil || !row.BonusGranted {
		t.Errorf("строка вторника %+v; want общее 7200, момента нет, помечена", row)
	}

	// Конец в 00:20:00 среды (лимит уменьшали, момент сдвигался): до полуночи 20 мин отсчёта —
	// общее 7200 − 1200 = 6000 с, остаток 20 мин, бонус +10; копилка не читается.
	st, id = lastMinuteRow(t, 40*60, time.Date(2026, 11, 25, 0, 20, 0, 0, tzMSK))
	st.days[dayKey(id, tue24)] = func(r core.DayState) core.DayState { r.GamesS = 0; return r }(st.days[dayKey(id, tue24)])
	st.failBalance = true
	startAt(t, st, thu1000)
	if row := st.days[dayKey(id, tue24)]; row.TotalS != 6000 || row.LastMinuteEndMs != nil || row.BonusGranted {
		t.Errorf("сбой копилки: строка %+v; want общее 6000, момента нет, бонус не рассмотрен", row)
	}
	st.failBalance = false
	startAt(t, st, thu1000.Add(time.Hour))
	if bal, evs := bonusOf(t, st, id); bal != 10 || len(evs) != 1 {
		t.Errorf("копилка снова читается: %d, записей %d; want +10 по дотянутой строке", bal, len(evs))
	}
}

// Ловит (B-42, R18): правила не прочитаны (безопасный режим) — лимит дня не известен, строка не
// трогается, дотяжка ждёт запуска с правилами; прошлый день без последней минуты не меняется.
func TestPastDayPullUpNeedsRules(t *testing.T) {
	st, id := lastMinuteRow(t, 7150, time.Date(2026, 11, 25, 0, 0, 20, 0, tzMSK))
	st.failSchedule = true
	startAt(t, st, thu1000)
	if row := st.days[dayKey(id, tue24)]; row.TotalS != 7150 || row.LastMinuteEndMs == nil {
		t.Errorf("без правил: строка %+v; want прежняя", row)
	}
	st.failSchedule = false
	startAt(t, st, thu1000.Add(time.Hour))
	if row := st.days[dayKey(id, tue24)]; row.TotalS != 7180 || row.LastMinuteEndMs != nil {
		t.Errorf("с правилами: строка %+v; want общее 7180, момента нет", row)
	}
}

// Ловит (B-42, RM5-8): очистка статистики была во вторник в 23:50 (снимок 7000 с); дотяжка строки
// вторника после полуночи видна в статистике за вычетом снимка: столбик вторника недели — 180 с
// (7180 − 7000), а не 150 и не вся строка.
func TestPastDayPullUpAfterClearStats(t *testing.T) {
	st, id := lastMinuteRow(t, 7150, time.Date(2026, 11, 25, 0, 0, 20, 0, tzMSK))
	st.resets[id] = StatsReset{ClearedAtMs: time.Date(2026, 11, 24, 23, 50, 0, 0, tzMSK).UnixMilli(), Day: tue24, TotalS: 7000, GamesS: 6000}
	h := startAt(t, st, thu1000)
	tok := h.login(adminPeer())
	week := h.parentPeriod(tok, id, ipc.PeriodWeek)
	if len(week.Bars) != 4 || week.Bars[1].Key != tue24 || week.Bars[1].TotalS != 180 {
		t.Fatalf("неделя: столбики %s; want 4 с понедельника, вторник 180 с", jsonOf(t, week.Bars))
	}
	if week.UsedS != 180 {
		t.Errorf("неделя: итог %d с; want 180", week.UsedS)
	}
}
