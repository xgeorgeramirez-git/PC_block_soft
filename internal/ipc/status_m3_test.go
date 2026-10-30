package ipc

import (
	"strings"
	"testing"

	"pcblock/internal/core"
)

// Ловит: игровые предупреждения (R5, R9) идут текстами общего лимита или без вида события
// «games»: страж показал бы «время на сегодня закончится», хотя кончаются только игры; текст за
// 1 минуту не по образцу плашки «Осталась 1 минута» (страж дописывает «: 0:59»).
func TestGuardWarningGamesTexts(t *testing.T) {
	want := map[int]string{
		30: "Через 30 минут время на игры закончится",
		15: "Через 15 минут время на игры закончится",
		5:  "Сохрани игру: через 5 минут время на игры закончится",
		1:  "Осталась 1 минута на игры",
	}
	for th, text := range want {
		w := NewGuardWarningFor(th, core.WarnGames, th*60-7)
		if w.Text != text || w.Event != WarnEventGames || !w.Sound || w.Threshold != th || w.EventS != th*60-7 {
			t.Errorf("порог %d: %+v, want текст %q, событие games", th, w, text)
		}
		if strings.ContainsAny(w.Text, "—–") {
			t.Errorf("длинное тире: %q", w.Text)
		}
	}
	if WarnEventGames != "games" {
		t.Errorf("WarnEventGames = %q", WarnEventGames)
	}
}

// Ловит: поля игр и режима окончания не доходят из решения ядра до значка, карточки и «Моей
// статистики» (Review Focus 5: одно решение — одни цифры); подсказка значка без второй строки
// «Игры: 20 мин» (ТЗ 8.2.1 п.2) или с ней, когда отдельного лимита игр нет.
func TestChildStatusGamesFields(t *testing.T) {
	d := core.Decision{
		Day: "2026-10-19", Status: core.StatusOK, RemainingS: 85 * 60, LimitS: 120 * 60, UsedS: 35 * 60,
		GamesRemainingS: 19*60 + 5, GamesLimitS: 3600, CloseGames: false,
	}
	s := ChildStatusFrom("Миша", d)
	if s.GamesRemainingS != 19*60+5 || s.GamesLimitS != 3600 || s.GamesUnlimited || s.CloseGames || s.StudyOnly {
		t.Fatalf("поля игр: %+v", s)
	}
	if s.Tooltip != "Осталось 1 ч 25 мин\nИгры: 20 мин" {
		t.Errorf("подсказка %q, want вторую строку «Игры: 20 мин» (минуты вверх, как у остатка)", s.Tooltip)
	}
	if s.EndMode != EndModeLock {
		t.Errorf("режим по умолчанию %q, want lock", s.EndMode)
	}

	d.GamesUnlimited, d.GamesRemainingS, d.GamesLimitS = true, 0, 0
	if s := ChildStatusFrom("Миша", d); s.Tooltip != "Осталось 1 ч 25 мин" || !s.GamesUnlimited {
		t.Errorf("без лимита игр: подсказка %q, GamesUnlimited %v", s.Tooltip, s.GamesUnlimited)
	}

	// Игры сегодня нельзя (лимит 0): ядро отдаёт лимит игр 0 и закрытие игр — строка «Игры: 0 мин».
	zero := core.Decision{Status: core.StatusOK, RemainingS: 600, LimitS: 3600, CloseGames: true, CloseReason: core.GamesOverPlateText}
	if s := ChildStatusFrom("Миша", zero); s.Tooltip != "Осталось 10 мин\nИгры: 0 мин" {
		t.Errorf("игры нельзя: подсказка %q", s.Tooltip)
	}

	// Игры кончились, общее время осталось: экрана нет, игры закрыты.
	over := core.Decision{Status: core.StatusOK, RemainingS: 600 * 60, LimitS: 3600 * 10, GamesLimitS: 3600, CloseGames: true,
		CloseReason: core.GamesOverPlateText}
	s = ChildStatusFrom("Миша", over)
	if !s.CloseGames || s.Lock != LockNone || s.Tooltip != "Осталось 10 ч 00 мин\nИгры: 0 мин" {
		t.Errorf("игры кончились: %+v", s)
	}
}

// Ловит: в режиме «Оставить только учёбу» (R8) статус «время вышло» превращается в экран
// блокировки: экран определяется по Decision.Lock, а не по слову статуса; подсказка и строка
// мини-панели не говорят, что доступна только учёба.
func TestChildStatusStudyOnlyHasNoLock(t *testing.T) {
	d := core.Decision{
		Status: core.StatusTimeUp, StudyOnly: true, CloseReason: core.StudyOnlyPlateText, Until: "Можно снова: завтра",
		LimitS: 7200, UsedS: 7200, GamesUnlimited: true,
	}
	s := ChildStatusFrom("Миша", d)
	if s.Lock != LockNone || !s.StudyOnly || s.Status != StatusTimeUp || s.EndMode != EndModeStudyOnly {
		t.Fatalf("только учёба: %+v", s)
	}
	if s.Tooltip != core.StudyOnlyPlateText || s.Summary != core.StudyOnlyPlateText || s.Label != "0" {
		t.Errorf("подсказка %q, строка %q, надпись %q", s.Tooltip, s.Summary, s.Label)
	}
	f := ChildStatusFrom("Миша", core.Decision{Status: core.StatusForbidden, StudyOnly: true, CloseReason: core.StudyOnlyForbiddenPlateText})
	if f.Lock != LockNone || f.Tooltip != core.StudyOnlyForbiddenPlateText {
		t.Errorf("запретные часы, только учёба: %+v", f)
	}
	if a := ChildStatusAdmin("Оля"); !a.GamesUnlimited || a.EndMode != EndModeLock {
		t.Errorf("admin: %+v", a)
	}
	if n := ChildStatusNotChild(); !n.GamesUnlimited || n.EndMode != EndModeLock {
		t.Errorf("notChild: %+v", n)
	}
}

// Ловит: строка игр карточки «Сегодня» (8.3.2 п.3) не в формате ТЗ или есть без лимита игр.
func TestGamesCardText(t *testing.T) {
	cases := []struct {
		used, limit int
		unl         bool
		want        string
	}{
		{20 * 60, 3600, false, "Игры 20 мин из 1 ч"},
		{40*60 + 59, 3600, false, "Игры 40 мин из 1 ч"},
		{65 * 60, 90 * 60, false, "Игры 1 ч 05 мин из 1 ч 30 мин"},
		{0, 0, false, "Игры 0 мин из 0 мин"},
		{600, 0, true, ""},
	}
	for _, c := range cases {
		if got := GamesCardText(c.used, c.limit, c.unl); got != c.want {
			t.Errorf("GamesCardText(%d, %d, %v) = %q, want %q", c.used, c.limit, c.unl, got, c.want)
		}
	}
}
