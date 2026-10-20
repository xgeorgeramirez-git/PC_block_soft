package auth

import (
	"errors"
	"testing"
)

// Ловит: короткий пароль проходит в мастер и в смену пароля, длина считается
// в байтах (кириллица из 3 букв = 6 байт прошла бы), текст ошибки технический (R19, 8.3.1).
func TestCheckLength(t *testing.T) {
	cases := []struct {
		name string
		pw   string
		ok   bool
	}{
		{"пусто", "", false},
		{"5 латинских", "abcde", false},
		{"3 кириллических (6 байт)", "абв", false},
		{"5 кириллических (10 байт)", "абвгд", false},
		{"6 латинских", "abcdef", true},
		{"6 кириллических", "абвгде", true},
		{"длинный", "очень-длинный-пароль-родителя", true},
	}
	for _, tc := range cases {
		err := CheckLength(tc.pw)
		if tc.ok && err != nil {
			t.Errorf("%s: CheckLength = %v, want nil", tc.name, err)
		}
		if !tc.ok && !errors.Is(err, ErrTooShort) {
			t.Errorf("%s: CheckLength = %v, want ErrTooShort", tc.name, err)
		}
	}
	if ErrTooShort.Error() != "Не меньше 6 символов" {
		t.Fatalf("текст ErrTooShort = %q, want %q", ErrTooShort.Error(), "Не меньше 6 символов")
	}
}

// failN вызывает Fail n раз в момент nowMs и возвращает результат последнего вызова.
func failN(a *Attempts, n int, nowMs int64) int64 {
	var until int64
	for i := 0; i < n; i++ {
		until = a.Fail(nowMs)
	}
	return until
}

// Ловит: пауза включается раньше 5-й неверной попытки или не включается на 5-й,
// длится не 60 с, серии не удваивают паузу, пауза растёт выше 15 мин (R19).
func TestAttemptsSeries(t *testing.T) {
	var a Attempts
	const t0 = int64(1_760_000_000_000)

	// Серия 1: четыре неверных — без паузы, пятая — 60 с.
	for i := 1; i <= 4; i++ {
		if until := a.Fail(t0); until != 0 {
			t.Fatalf("неверная №%d: пауза до %d, want без паузы", i, until)
		}
		if blocked, _ := a.Blocked(t0); blocked {
			t.Fatalf("неверная №%d: Blocked = true, want false", i)
		}
	}
	if until := a.Fail(t0); until != t0+60_000 {
		t.Fatalf("5-я неверная: пауза до %d, want %d (60 с)", until, t0+60_000)
	}

	// Серии 2…6 — каждая по 5 неверных после конца предыдущей паузы.
	now := t0 + 60_000
	steps := []struct {
		series int
		pauseS int64
	}{
		{2, 120},
		{3, 240},
		{4, 480},
		{5, 900}, // 960 → потолок 15 мин
		{6, 900},
	}
	for _, st := range steps {
		if until := failN(&a, 4, now); until != 0 {
			t.Fatalf("серия %d, 4 неверных: пауза до %d, want без паузы", st.series, until)
		}
		until := a.Fail(now)
		if want := now + st.pauseS*1000; until != want {
			t.Fatalf("серия %d: пауза до %d, want %d (%d с)", st.series, until, want, st.pauseS)
		}
		if a.Series != st.series || a.Failed != 0 {
			t.Fatalf("серия %d: Series=%d Failed=%d, want Series=%d Failed=0", st.series, a.Series, a.Failed, st.series)
		}
		now = until
	}
}

// Ловит: пауза не сохраняется за большое число серий (переполнение сдвига даёт
// ноль или отрицательную паузу, и ввод открывается сразу), R19 «не больше 15 мин».
func TestAttemptsCeilingManySeries(t *testing.T) {
	a := Attempts{Series: 70}
	const now = int64(1_000_000)
	until := failN(&a, 5, now)
	if until != now+900_000 {
		t.Fatalf("серия 71: пауза до %d, want %d (900 с)", until, now+900_000)
	}
}

// Ловит: во время паузы ввод не закрыт (Blocked = false), момент конца паузы
// не тот, после конца паузы ввод остаётся закрытым (R19, FR-14).
func TestAttemptsBlocked(t *testing.T) {
	var a Attempts
	const t0 = int64(5_000_000)
	failN(&a, 5, t0)

	cases := []struct {
		name      string
		nowMs     int64
		wantBlock bool
		wantUntil int64
	}{
		{"сразу после 5-й", t0, true, t0 + 60_000},
		{"за 1 мс до конца", t0 + 59_999, true, t0 + 60_000},
		{"ровно в конце", t0 + 60_000, false, 0},
		{"после конца", t0 + 120_000, false, 0},
	}
	for _, tc := range cases {
		blocked, until := a.Blocked(tc.nowMs)
		if blocked != tc.wantBlock || until != tc.wantUntil {
			t.Errorf("%s: Blocked = (%v, %d), want (%v, %d)", tc.name, blocked, until, tc.wantBlock, tc.wantUntil)
		}
	}
}

// Ловит: неверный ввод во время паузы засчитывается в следующую серию или
// продлевает паузу (ребёнок мог бы держать пароль родителя закрытым бесконечно).
func TestAttemptsFailDuringPauseIgnored(t *testing.T) {
	var a Attempts
	const t0 = int64(7_000_000)
	failN(&a, 5, t0)
	if until := a.Fail(t0 + 10_000); until != t0+60_000 {
		t.Fatalf("неверная во время паузы: пауза до %d, want %d (не продлевается)", until, t0+60_000)
	}
	if a.Failed != 0 || a.Series != 1 {
		t.Fatalf("после неверной во время паузы: Failed=%d Series=%d, want 0 и 1", a.Failed, a.Series)
	}
}

// Ловит: верный пароль не сбрасывает счётчик и серию: следующая ошибка
// сразу включает долгую паузу (R19 «5 неверных подряд»).
func TestAttemptsReset(t *testing.T) {
	var a Attempts
	const t0 = int64(9_000_000)
	failN(&a, 5, t0)
	failN(&a, 3, t0+60_000)
	a.Reset()
	if a != (Attempts{}) {
		t.Fatalf("после Reset = %+v, want нулевое состояние", a)
	}
	if blocked, _ := a.Blocked(t0 + 60_000); blocked {
		t.Fatalf("после Reset Blocked = true, want false")
	}
	if until := failN(&a, 4, t0+70_000); until != 0 {
		t.Fatalf("после Reset 4 неверных: пауза до %d, want без паузы", until)
	}
	if until := a.Fail(t0 + 70_000); until != t0+70_000+60_000 {
		t.Fatalf("после Reset 5-я неверная: пауза до %d, want %d (снова 60 с)", until, t0+130_000)
	}
}

// Ловит: счётчик «неверных подряд» для parent_auth.failed_count теряет серию или
// счётчик в серии при сохранении и чтении (после перезапуска службы пауза
// снова начиналась бы с 60 с), Ruling «серия = failed_count / 5».
func TestAttemptsInRowRoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		a      Attempts
		inRow  int
		failed int
		series int
	}{
		{"пусто", Attempts{}, 0, 0, 0},
		{"3 в первой серии", Attempts{Failed: 3}, 3, 3, 0},
		{"после первой паузы", Attempts{Series: 1, BlockedUntilMs: 42}, 5, 0, 1},
		{"2 в третьей серии", Attempts{Failed: 2, Series: 2}, 12, 2, 2},
	}
	for _, tc := range cases {
		if got := tc.a.FailedInRow(); got != tc.inRow {
			t.Errorf("%s: FailedInRow = %d, want %d", tc.name, got, tc.inRow)
		}
		back := AttemptsFromRow(tc.inRow, tc.a.BlockedUntilMs)
		if back.Failed != tc.failed || back.Series != tc.series || back.BlockedUntilMs != tc.a.BlockedUntilMs {
			t.Errorf("%s: AttemptsFromRow = %+v, want Failed=%d Series=%d BlockedUntilMs=%d", tc.name, back, tc.failed, tc.series, tc.a.BlockedUntilMs)
		}
	}
	if got := AttemptsFromRow(-3, 0); got != (Attempts{}) {
		t.Errorf("отрицательный счётчик из базы: %+v, want нулевое состояние", got)
	}
}
