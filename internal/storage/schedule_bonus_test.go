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
