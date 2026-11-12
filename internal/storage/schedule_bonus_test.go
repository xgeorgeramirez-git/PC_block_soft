package storage

import (
	"errors"
	"reflect"
	"testing"

	"pcblock/internal/core"
)

// Ловит: интервалы расписания не сохраняются, замена добавляет вместо замены, пустой
// список уходит как nil (L-22), round-trip теряет поля (R7).
func TestSetGetScheduleRoundTrip(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-1", "Миша")

	// Пустой у нового ребёнка — пустой список, не nil.
	got, err := s.GetSchedule(id)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("GetSchedule нового ребёнка = %#v, want пустой непустой-nil срез", got)
	}

	in := []core.Interval{
		{Weekday: core.Monday, StartMin: 21 * 60, EndMin: 7 * 60},
		{Weekday: core.Friday, StartMin: 22 * 60, EndMin: 8 * 60},
	}
	if err := s.SetSchedule(id, in); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetSchedule(id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("GetSchedule = %+v, want %+v", got, in)
	}

	// Замена заменяет, не добавляет.
	in2 := []core.Interval{{Weekday: core.Tuesday, StartMin: 60, EndMin: 120}}
	if err := s.SetSchedule(id, in2); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSchedule(id)
	if !reflect.DeepEqual(got, in2) {
		t.Fatalf("после замены GetSchedule = %+v, want %+v", got, in2)
	}

	// Пустая замена очищает.
	if err := s.SetSchedule(id, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSchedule(id)
	if len(got) != 0 {
		t.Fatalf("после очистки GetSchedule = %+v, want пусто", got)
	}
}

// Ловит: копилка не считает баланс, уводит ниже 0 или выше 600, трата сверх баланса
// проходит, начисление не записывается (R10, FR-22, CHECK 0..600).
func TestBonusLedgerBalance(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-2", "Катя")

	if bal, err := s.GetBonusBalance(id); err != nil || bal != 0 {
		t.Fatalf("начальный баланс = %d, err %v, want 0", bal, err)
	}

	// Начисление +30 → 30; +30 → 60.
	if bal, err := s.AppendBonus(id, 30, "за уборку"); err != nil || bal != 30 {
		t.Fatalf("AppendBonus(+30) = %d, err %v, want 30", bal, err)
	}
	if bal, err := s.AppendBonus(id, 30, "за уроки"); err != nil || bal != 60 {
		t.Fatalf("AppendBonus(+30) = %d, err %v, want 60", bal, err)
	}
	// Трата -30 → 30.
	if bal, err := s.AppendBonus(id, -30, "взято из копилки"); err != nil || bal != 30 {
		t.Fatalf("AppendBonus(-30) = %d, err %v, want 30", bal, err)
	}
	// Трата -60 при балансе 30 — ошибка, баланс не меняется.
	if _, err := s.AppendBonus(id, -60, "взято из копилки"); err == nil {
		t.Fatal("AppendBonus(-60) при балансе 30 должно вернуть ошибку")
	}
	if bal, _ := s.GetBonusBalance(id); bal != 30 {
		t.Fatalf("после неудачной траты баланс = %d, want 30 (не изменился)", bal)
	}
	// Начисление выше 600 — ошибка.
	if _, err := s.AppendBonus(id, 600, "слишком много"); err == nil {
		t.Fatal("AppendBonus(+600) при балансе 30 должно превысить 600 и вернуть ошибку")
	}
	if bal, _ := s.GetBonusBalance(id); bal != 30 {
		t.Fatalf("после отказа баланс = %d, want 30", bal)
	}
}

// Ловит: причина начисления обязательна (R22, FR-22).
func TestBonusRejectsEmptyReason(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-3", "Лена")
	if _, err := s.AppendBonus(id, 30, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("AppendBonus без причины = %v, want ErrInvalid", err)
	}
}

// Ловит: история копилки не читается или теряет записи (FR-22).
func TestListBonus(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-4", "Петя")
	if list, err := s.ListBonus(id); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("пустая история = %#v, err %v, want пустой непустой-nil срез", list, err)
	}
	_, _ = s.AppendBonus(id, 30, "за уборку")
	_, _ = s.AppendBonus(id, -15, "взято из копилки")
	list, err := s.ListBonus(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("история из %d записей, want 2", len(list))
	}
}

// Ловит: pause_until не сохраняется или не читается (R11).
func TestSaveLoadDayStatePauseUntil(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-5", "Юля")
	until := int64(1_700_000_000_000)
	st := core.DayState{Day: day1, TotalS: 600, PauseUntilMs: &until}
	if err := s.SaveDayState(id, st); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadDayState(id, day1)
	if err != nil {
		t.Fatal(err)
	}
	if got.PauseUntilMs == nil || *got.PauseUntilMs != until {
		t.Fatalf("PauseUntilMs = %v, want %d", got.PauseUntilMs, until)
	}
	// nil сохраняется как NULL и читается как nil.
	st.PauseUntilMs = nil
	if err := s.SaveDayState(id, st); err != nil {
		t.Fatal(err)
	}
	got, _ = s.LoadDayState(id, day1)
	if got.PauseUntilMs != nil {
		t.Fatalf("после снятия паузы PauseUntilMs = %v, want nil", *got.PauseUntilMs)
	}
}

// Ловит (R25 «один раз», ревью M5): автобонус и отметка bonus_granted строки дня пишутся двумя
// записями — сбой между ними оставляет начисление без отметки, и следующий запуск начисляет снова;
// отметка теряет поля строки; отказ по потолку или причине всё же пишет строку дня.
func TestGrantDayBonusAtomic(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1-5-21-7", "Миша")
	last := int64(1_792_418_460_000)
	day := core.DayState{Day: "2026-11-24", TotalS: 2400, GamesS: 600, ExtraMin: 10, NoForbidden: true, LastMinuteEndMs: &last}

	bal, err := s.GrantDayBonus(id, day, 40, "Бонус за свободное время: 40 мин")
	if err != nil || bal != 40 {
		t.Fatalf("GrantDayBonus = %d, %v; want 40", bal, err)
	}
	want := day
	want.BonusGranted = true
	if got, err := s.LoadDayState(id, "2026-11-24"); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("строка дня = %+v, %v; want %+v (помечена, поля прежние)", got, err, want)
	}
	if list, _ := s.ListBonus(id); len(list) != 1 || list[0].DeltaMin != 40 || list[0].Reason != "Бонус за свободное время: 40 мин" || list[0].BalanceAfter != 40 {
		t.Errorf("копилка = %+v; want одна запись +40", list)
	}

	// Отказы до записи: ни начисления, ни строки дня.
	other := core.DayState{Day: "2026-11-25", TotalS: 60}
	for _, tc := range []struct {
		name   string
		day    core.DayState
		delta  int
		reason string
	}{
		{"выше потолка", other, 561, "Бонус"},
		{"без причины", other, 5, ""},
		{"ноль", other, 0, "Бонус"},
		{"не дата", core.DayState{Day: "25.11.2026"}, 5, "Бонус"},
	} {
		if _, err := s.GrantDayBonus(id, tc.day, tc.delta, tc.reason); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", tc.name, err)
		}
	}
	if n := countWhere(t, s, `SELECT count(*) FROM day_state WHERE child_id = ? AND day = '2026-11-25'`, id); n != 0 {
		t.Errorf("после отказов строк среды %d, want 0", n)
	}

	// Сбой записи строки дня после вставки в копилку: откат обоих.
	if _, err := s.db.Exec(`CREATE TRIGGER fail_day BEFORE INSERT ON day_state BEGIN SELECT RAISE(ABORT, 'диск'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GrantDayBonus(id, other, 5, "Бонус за свободное время: 5 мин"); err == nil {
		t.Fatal("GrantDayBonus при сбое строки дня прошёл")
	}
	if bal, _ := s.GetBonusBalance(id); bal != 40 {
		t.Errorf("после сбоя баланс %d, want 40 (начисление откатилось)", bal)
	}
	if list, _ := s.ListBonus(id); len(list) != 1 {
		t.Errorf("после сбоя записей копилки %d, want 1", len(list))
	}
}
