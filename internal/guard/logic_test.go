package guard

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Граница связи со службой (Ruling 15): ровно 10 с экран ещё показан, 10 с + 1 мс скрыт.
// Сроки плашек: предупреждение закрывается через 15 с, «Добавлено» через 5 с.
// Константы заданы в тестах отдельно от кода: сдвиг границы в коде ломает тест.
const (
	testOffline   = 10 * time.Second
	testWarnLife  = 15 * time.Second
	testAddedLife = 5 * time.Second

	// Автоматическая смена пользователя: 15 минут от появления экрана (ТЗ 8.2.3 п.10).
	testAutoSwitch = 15 * time.Minute
	// Кнопок на главном экране блокировки: пароль, выключение, выход, смена пользователя.
	// Экран блокировки «время вышло» без копилки (M2): «Попросить ещё», «Пароль родителя»
	// и три выхода (ТЗ 8.2.3 п.3).
	testScreenButtons = 5
	// Экран блокировки за обход: «Пароль родителя» и три выхода (копилка/попросить скрыты).
	testBypassButtons = 4
)

// Начало отсчёта всех тестов (реальные часы стража, Ruling 4).
var t0 = time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return t0.Add(d) }

func timeUpDecision() core.Decision {
	return core.Decision{
		Day: "2026-10-19", Lock: core.LockTimeUp, Reason: "Время на сегодня закончилось",
		Until: "Можно снова: завтра", Used: "Сегодня: 2 ч 00 мин из 2 ч",
		LimitS: 7200, UsedS: 7200, Status: core.StatusTimeUp,
	}
}

func bypassDecision() core.Decision {
	return core.Decision{
		Day: "2026-10-19", Lock: core.LockBypass, Reason: "Попытка обойти защиту",
		Detail: "Изменено системное время", Until: "Можно снова: когда родитель снимет блокировку",
		Used: "Сегодня: 40 мин", UsedS: 2400, Status: core.StatusBypass,
	}
}

func runningDecision(remainingS int) core.Decision {
	return core.Decision{Day: "2026-10-19", RemainingS: remainingS, LimitS: 7200, UsedS: 7200 - remainingS, Status: core.StatusOK}
}

func locked(d core.Decision) *Model {
	m := NewModel()
	m.OnDecision(t0, d, false, nil)
	return m
}

// withPasswordPanel открывает панель пароля на экране блокировки.
func withPasswordPanel(t *testing.T, d core.Decision) *Model {
	t.Helper()
	m := locked(d)
	m.Press(t0, ButtonParentPassword, "")
	if v := m.View(t0); v.Panel != PanelPassword {
		t.Fatalf("панель пароля не открылась: Panel = %v", v.Panel)
	}
	return m
}

// withChoices проводит родителя через верный пароль до выбора действий.
func withChoices(t *testing.T, d core.Decision) *Model {
	t.Helper()
	m := withPasswordPanel(t, d)
	if a := m.Press(t0, ButtonSubmitPassword, "верный-пароль"); a.Kind != ActCheckPassword {
		t.Fatalf("пароль не ушёл на проверку: %+v", a)
	}
	m.OnPasswordResult(t0, PasswordResult{OK: true})
	if v := m.View(t0); v.Panel != PanelParentChoices {
		t.Fatalf("выбор действий не открылся: Panel = %v", v.Panel)
	}
	return m
}

// feed присылает по решению в секунду (связь жива) с остатком, убывающим от rem0.
func feed(m *Model, from, to int, rem0 int) {
	for s := from; s <= to; s++ {
		m.OnDecision(at(time.Duration(s)*time.Second), runningDecision(rem0-s), false, nil)
	}
}

// keepAlive присылает решение d каждую секунду на отрезке [from, to] секунд: связь со службой жива.
func keepAlive(m *Model, d core.Decision, from, to int) {
	for s := from; s <= to; s++ {
		m.OnDecision(at(time.Duration(s)*time.Second), d, false, nil)
	}
}

func TestLockShownByDecision(t *testing.T) {
	// Ловит: экран блокировки показан не по решению службы (без блокировки, в родительской
	// сессии, без решения) или поля LockView не переданы из решения.
	cases := []struct {
		name string
		d    core.Decision
		hide bool
		want bool
		lock LockView
	}{
		{"время вышло", timeUpDecision(), false, true, LockView{
			Reason: "Время на сегодня закончилось", Until: "Можно снова: завтра", Used: "Сегодня: 2 ч 00 мин из 2 ч",
			Hint: "Попроси родителя, если нужно больше времени"}},
		{"обход защиты", bypassDecision(), false, true, LockView{
			Reason: "Попытка обойти защиту", Detail: "Изменено системное время",
			Until: "Можно снова: когда родитель снимет блокировку", Used: "Сегодня: 40 мин"}},
		{"блокировки нет", runningDecision(3000), false, false, LockView{}},
		{"родитель в сессии", timeUpDecision(), true, false, LockView{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewModel()
			m.OnDecision(t0, c.d, c.hide, nil)
			v := m.View(t0)
			if v.LockShown != c.want || v.Lock != c.lock {
				t.Fatalf("LockShown = %v, Lock = %+v; ждали %v, %+v", v.LockShown, v.Lock, c.want, c.lock)
			}
		})
	}
	v := NewModel().View(t0)
	if v.LockShown || v.SuppressKeys || v.Warning != nil {
		t.Fatalf("без решения службы страж ничего не показывает: %+v", v)
	}
}

func TestReasonFallback(t *testing.T) {
	// Ловит: экран блокировки без заголовка, если служба прислала пустую причину:
	// на аварийном экране всегда есть текст.
	d := timeUpDecision()
	d.Reason = ""
	if got := locked(d).View(t0).Lock.Reason; got != "Время на сегодня закончилось" {
		t.Fatalf("заголовок времени = %q", got)
	}
	b := bypassDecision()
	b.Reason = ""
	if got := locked(b).View(t0).Lock.Reason; got != "Попытка обойти защиту" {
		t.Fatalf("заголовок обхода = %q", got)
	}
}

func TestLockButtonsM2(t *testing.T) {
	// ТЗ 8.2.3 п.3: при блокировке «время вышло»/интервал главная «Попросить ещё», ниже
	// «Пароль родителя» и три выхода (без копилки, если баланс 0). При блокировке за обход
	// — только «Пароль родителя» (главная) и три выхода; «Попросить ещё» и «Взять из копилки»
	// скрыты. Выходы доступны всегда (не считаются обходом).
	wantTimeUp := []ButtonView{
		{Button: ButtonAskMore, Label: "Попросить ещё", Main: true, Enabled: true},
		{Button: ButtonParentPassword, Label: "Пароль родителя", Enabled: true},
		{Button: ButtonShutdown, Label: "Выключить компьютер", Enabled: true},
		{Button: ButtonLogoff, Label: "Выйти из учётной записи", Enabled: true},
		{Button: ButtonSwitchUser, Label: "Смена пользователя", Enabled: true},
	}
	wantBypass := []ButtonView{
		{Button: ButtonParentPassword, Label: "Пароль родителя", Main: true, Enabled: true},
		{Button: ButtonShutdown, Label: "Выключить компьютер", Enabled: true},
		{Button: ButtonLogoff, Label: "Выйти из учётной записи", Enabled: true},
		{Button: ButtonSwitchUser, Label: "Смена пользователя", Enabled: true},
	}
	cases := map[string]struct {
		d    core.Decision
		want []ButtonView
	}{"время": {timeUpDecision(), wantTimeUp}, "обход": {bypassDecision(), wantBypass}}
	for name, c := range cases {
		v := locked(c.d).View(t0)
		if !reflect.DeepEqual(v.Buttons, c.want) {
			t.Errorf("%s: кнопки %+v, ждали %+v", name, v.Buttons, c.want)
		}
		if len(v.Choices) != 0 || v.Panel != PanelNone || v.PanelText != "" {
			t.Errorf("%s: панель не должна быть открыта: %+v", name, v)
		}
	}
}

func TestLockSpendBonusButton(t *testing.T) {
	// FR-11, ТЗ 8.2.3 п.3: «Взять из копилки (N мин)» при балансе; недоступна в запретный
	// интервал с пояснением (L-17).
	d := timeUpDecision()
	d.BonusBalance = 45
	v := locked(d).View(t0)
	var bonus *ButtonView
	for i := range v.Buttons {
		if v.Buttons[i].Button == ButtonSpendBonus {
			bonus = &v.Buttons[i]
		}
	}
	if bonus == nil || !bonus.Enabled {
		t.Fatalf("при балансе «Взять из копилки» должна быть доступна: %+v", v.Buttons)
	}
	// Запретный интервал: кнопка недоступна с пояснением.
	df := timeUpDecision()
	df.Lock = core.LockForbidden
	df.BonusBalance = 45
	df.Until = "Можно снова: завтра в 07:00"
	vf := locked(df).View(t0)
	for i := range vf.Buttons {
		if vf.Buttons[i].Button == ButtonSpendBonus {
			if vf.Buttons[i].Enabled || vf.Buttons[i].Hint == "" {
				t.Fatalf("в интервал «Взять из копилки» недоступна с пояснением: %+v", vf.Buttons[i])
			}
		}
	}
}

func TestSuppressKeysOnlyWithLockScreen(t *testing.T) {
	// Ловит: перехват системных сочетаний без экрана блокировки (R23, ТЗ 8.2.3 п.8),
	// в родительской сессии, без связи и при открытой панели пароля.
	cases := []struct {
		name  string
		build func() (*Model, time.Time)
		want  bool
	}{
		{"нет решения", func() (*Model, time.Time) { return NewModel(), t0 }, false},
		{"блокировки нет", func() (*Model, time.Time) { return locked(runningDecision(900)), t0 }, false},
		{"время вышло", func() (*Model, time.Time) { return locked(timeUpDecision()), t0 }, true},
		{"обход", func() (*Model, time.Time) { return locked(bypassDecision()), t0 }, true},
		{"родительская сессия", func() (*Model, time.Time) {
			m := NewModel()
			m.OnDecision(t0, timeUpDecision(), true, nil)
			return m, t0
		}, false},
		{"нет связи 10 с + 1 мс", func() (*Model, time.Time) { return locked(timeUpDecision()), at(testOffline + time.Millisecond) }, false},
		{"панель пароля поверх экрана", func() (*Model, time.Time) { return withPasswordPanel(t, timeUpDecision()), t0 }, true},
		{"только плашка", func() (*Model, time.Time) {
			m := NewModel()
			m.OnDecision(t0, runningDecision(300), false, []int{5})
			return m, t0
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, now := c.build()
			if got := m.View(now).SuppressKeys; got != c.want {
				t.Fatalf("SuppressKeys = %v, ждали %v", got, c.want)
			}
		})
	}
}

func TestPasswordPanelOpens(t *testing.T) {
	// Ловит: «Пароль родителя» не открывает панель; у панели нет «Продолжить» (главная,
	// Enter) и «Отмена»; поле недоступно или с чужой ошибкой; под панелью остаются кнопки экрана.
	v := withPasswordPanel(t, timeUpDecision()).View(t0)
	wantChoices := []ButtonView{
		{Button: ButtonSubmitPassword, Label: "Продолжить", Main: true, Enabled: true},
		{Button: ButtonCancel, Label: "Отмена", Enabled: true},
	}
	if !reflect.DeepEqual(v.Choices, wantChoices) {
		t.Fatalf("кнопки панели %+v, ждали %+v", v.Choices, wantChoices)
	}
	if !v.PasswordEnabled || v.PasswordError != "" || v.PasswordCountdown != "" {
		t.Fatalf("поле: Enabled=%v Error=%q Countdown=%q", v.PasswordEnabled, v.PasswordError, v.PasswordCountdown)
	}
	if len(v.Buttons) != 0 || !v.LockShown {
		t.Fatalf("при панели пароля кнопок экрана нет, экран остаётся: Buttons=%+v LockShown=%v", v.Buttons, v.LockShown)
	}
}

func TestEmptyPasswordIsFieldError(t *testing.T) {
	// Ловит: пустой пароль уходит в службу и тратит попытку; ошибка не у поля (L-121, L-124).
	m := withPasswordPanel(t, timeUpDecision())
	if a := m.Press(t0, ButtonSubmitPassword, ""); a.Kind != ActNone {
		t.Fatalf("пустой пароль породил действие %+v", a)
	}
	v := m.View(t0)
	if v.PasswordError != "Введите пароль" || v.Panel != PanelPassword || !v.PasswordEnabled {
		t.Fatalf("Error=%q Panel=%v Enabled=%v", v.PasswordError, v.Panel, v.PasswordEnabled)
	}
	// Непустой пароль снимает ошибку «Введите пароль» и уходит на проверку.
	a := m.Press(t0, ButtonSubmitPassword, "abcdef")
	if a != (Action{Kind: ActCheckPassword, Password: "abcdef"}) {
		t.Fatalf("действие %+v", a)
	}
	if got := m.View(t0).PasswordError; got != "" {
		t.Fatalf("ошибка осталась: %q", got)
	}
}

func TestWrongPasswordShowsAttemptsLeft(t *testing.T) {
	// Ловит: неверный пароль без счётчика попыток; счётчик не из ответа службы; отрицательное число.
	cases := []struct {
		left int
		want string
	}{
		{4, "Неверный пароль. Осталось попыток: 4"},
		{3, "Неверный пароль. Осталось попыток: 3"},
		{1, "Неверный пароль. Осталось попыток: 1"},
		{0, "Неверный пароль. Осталось попыток: 0"},
		{-2, "Неверный пароль. Осталось попыток: 0"},
	}
	for _, c := range cases {
		m := withPasswordPanel(t, timeUpDecision())
		m.Press(t0, ButtonSubmitPassword, "неверный")
		m.OnPasswordResult(t0, PasswordResult{AttemptsLeft: c.left})
		v := m.View(t0)
		if v.PasswordError != c.want || v.Panel != PanelPassword || !v.PasswordEnabled || v.PasswordCountdown != "" {
			t.Errorf("left=%d: Error=%q Panel=%v Enabled=%v", c.left, v.PasswordError, v.Panel, v.PasswordEnabled)
		}
	}
}

func TestPauseCountdownInRealTime(t *testing.T) {
	// Ловит: пауза без отсчёта или с отсчётом не по реальным часам; поле остаётся доступным;
	// округление вниз (в первый момент «0:59» вместо «1:00»); текст ошибки не снят по окончании.
	m := withPasswordPanel(t, timeUpDecision())
	m.Press(t0, ButtonSubmitPassword, "неверный")
	m.OnPasswordResult(t0, PasswordResult{BlockedUntil: at(60 * time.Second)})
	cases := []struct {
		after   time.Duration
		enabled bool
		text    string
		cd      string
	}{
		{0, false, "Слишком много попыток. Повторить можно через 1 мин", "1:00"},
		{time.Second, false, "Слишком много попыток. Повторить можно через 59 сек", "0:59"},
		{30 * time.Second, false, "Слишком много попыток. Повторить можно через 30 сек", "0:30"},
		{59*time.Second + 500*time.Millisecond, false, "Слишком много попыток. Повторить можно через 1 сек", "0:01"},
		{60*time.Second - time.Millisecond, false, "Слишком много попыток. Повторить можно через 1 сек", "0:01"},
		{60 * time.Second, true, "", ""},
		{61 * time.Second, true, "", ""},
	}
	sent := 0
	for _, c := range cases {
		if sec := int(c.after / time.Second); sec > sent {
			keepAlive(m, timeUpDecision(), sent+1, sec)
			sent = sec
		}
		v := m.View(at(c.after))
		if v.PasswordEnabled != c.enabled || v.PasswordError != c.text || v.PasswordCountdown != c.cd {
			t.Errorf("через %v: Enabled=%v Error=%q Countdown=%q; ждали %v, %q, %q",
				c.after, v.PasswordEnabled, v.PasswordError, v.PasswordCountdown, c.enabled, c.text, c.cd)
		}
	}
}

func TestPauseLengths(t *testing.T) {
	// Ловит: отсчёт паузы длиннее минуты (каждая следующая серия удваивает паузу, R19)
	// и пауза с долей секунды после момента ответа службы.
	cases := []struct {
		until time.Duration
		want  string
	}{
		{2 * time.Minute, "2:00"},
		{15 * time.Minute, "15:00"},
		{59*time.Second + 900*time.Millisecond, "1:00"},
		{125 * time.Second, "2:05"},
	}
	for _, c := range cases {
		m := withPasswordPanel(t, timeUpDecision())
		m.OnPasswordResult(t0, PasswordResult{BlockedUntil: at(c.until)})
		if got := m.View(t0).PasswordCountdown; got != c.want {
			t.Errorf("пауза %v: отсчёт %q, ждали %q", c.until, got, c.want)
		}
	}
}

func TestPausedContinueExplainsWhy(t *testing.T) {
	// Ловит: недоступная «Продолжить» без объяснения (L-17, ТЗ 8.0 п.5) или доступная в паузе.
	m := withPasswordPanel(t, timeUpDecision())
	m.OnPasswordResult(t0, PasswordResult{BlockedUntil: at(60 * time.Second)})
	v := m.View(at(time.Second))
	want := ButtonView{Button: ButtonSubmitPassword, Label: "Продолжить", Main: true, Enabled: false, Hint: "Повторить можно через 59 сек"}
	if len(v.Choices) != 2 || v.Choices[0] != want {
		t.Fatalf("кнопка в паузе %+v, ждали %+v", v.Choices, want)
	}
	if !v.Choices[1].Enabled {
		t.Fatal("«Отмена» должна работать в паузе: из панели всегда есть выход (L-119)")
	}
	if a := m.Press(at(time.Second), ButtonSubmitPassword, "пароль"); a.Kind != ActNone {
		t.Fatalf("в паузе пароль ушёл на проверку: %+v", a)
	}
	// После паузы кнопка снова рабочая и проверка уходит в службу.
	keepAlive(m, timeUpDecision(), 2, 60)
	if a := m.Press(at(60*time.Second), ButtonSubmitPassword, "пароль"); a.Kind != ActCheckPassword {
		t.Fatalf("после паузы действие %+v", a)
	}
}

func TestPauseSurvivesCancelAndReopen(t *testing.T) {
	// Ловит: «Отмена» и повторное открытие панели обходят паузу; в паузе нет пути назад.
	m := withPasswordPanel(t, timeUpDecision())
	m.OnPasswordResult(t0, PasswordResult{BlockedUntil: at(60 * time.Second)})
	keepAlive(m, timeUpDecision(), 1, 5)
	m.Press(at(5*time.Second), ButtonCancel, "")
	if v := m.View(at(5 * time.Second)); v.Panel != PanelNone || len(v.Buttons) != testScreenButtons {
		t.Fatalf("после «Отмена» нет экрана: %+v", v)
	}
	keepAlive(m, timeUpDecision(), 6, 10)
	m.Press(at(10*time.Second), ButtonParentPassword, "")
	v := m.View(at(10 * time.Second))
	if v.Panel != PanelPassword || v.PasswordEnabled || v.PasswordError != "Слишком много попыток. Повторить можно через 50 сек" {
		t.Fatalf("пауза потеряна: Panel=%v Enabled=%v Error=%q", v.Panel, v.PasswordEnabled, v.PasswordError)
	}
}

func TestCancelPasswordPanel(t *testing.T) {
	// Ловит: «Отмена» оставляет панель, старую ошибку или не просит очистить поле
	// (ClearSeq не растёт ни при открытии, ни при ошибке, ни при отмене).
	m := withPasswordPanel(t, timeUpDecision())
	seqOpen := m.View(t0).PasswordClearSeq
	m.Press(t0, ButtonSubmitPassword, "неверный")
	m.OnPasswordResult(t0, PasswordResult{AttemptsLeft: 3})
	seqWrong := m.View(t0).PasswordClearSeq
	if seqWrong <= seqOpen {
		t.Fatalf("после неверного пароля поле не очищается: %d -> %d", seqOpen, seqWrong)
	}
	if got := m.View(t0).PasswordClearSeq; got != seqWrong {
		t.Fatalf("просмотр меняет счётчик очистки: %d -> %d", seqWrong, got)
	}
	m.Press(t0, ButtonCancel, "")
	v := m.View(t0)
	if v.Panel != PanelNone || v.PasswordError != "" || v.PasswordClearSeq <= seqWrong || len(v.Buttons) != testScreenButtons {
		t.Fatalf("после «Отмена»: Panel=%v Error=%q Seq=%d Buttons=%d", v.Panel, v.PasswordError, v.PasswordClearSeq, len(v.Buttons))
	}
	m.Press(t0, ButtonParentPassword, "")
	v = m.View(t0)
	if v.Panel != PanelPassword || v.PasswordError != "" || !v.PasswordEnabled {
		t.Fatalf("повторное открытие: Panel=%v Error=%q Enabled=%v", v.Panel, v.PasswordError, v.PasswordEnabled)
	}
}

// holds ищет строку s в любом строковом поле значения v (включая неэкспортируемые).
func holds(v reflect.Value, s string, depth int) bool {
	if depth > 8 {
		return false
	}
	switch v.Kind() {
	case reflect.String:
		return strings.Contains(v.String(), s)
	case reflect.Pointer, reflect.Interface:
		return !v.IsNil() && holds(v.Elem(), s, depth+1)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if holds(v.Field(i), s, depth+1) {
				return true
			}
		}
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return strings.Contains(string(v.Bytes()), s)
		}
		fallthrough
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if holds(v.Index(i), s, depth+1) {
				return true
			}
		}
	case reflect.Map:
		it := v.MapRange()
		for it.Next() {
			if holds(it.Key(), s, depth+1) || holds(it.Value(), s, depth+1) {
				return true
			}
		}
	}
	return false
}

func TestPasswordIsNotKept(t *testing.T) {
	// Ловит: пароль родителя остаётся в модели после отправки на проверку, после верного
	// и неверного ответа, после «Отмена» (ни в полях модели, ни в вариантах View, ни в событиях).
	const secret = "тайный-пароль-4821"
	steps := []struct {
		name string
		run  func(m *Model)
	}{
		{"после отправки", func(m *Model) { m.Press(t0, ButtonSubmitPassword, secret) }},
		{"после неверного ответа", func(m *Model) {
			m.Press(t0, ButtonSubmitPassword, secret)
			m.OnPasswordResult(t0, PasswordResult{AttemptsLeft: 2})
		}},
		{"после паузы", func(m *Model) {
			m.Press(t0, ButtonSubmitPassword, secret)
			m.OnPasswordResult(t0, PasswordResult{BlockedUntil: at(time.Minute)})
		}},
		{"после верного ответа", func(m *Model) {
			m.Press(t0, ButtonSubmitPassword, secret)
			m.OnPasswordResult(t0, PasswordResult{OK: true})
		}},
		{"после выбора времени", func(m *Model) {
			m.Press(t0, ButtonSubmitPassword, secret)
			m.OnPasswordResult(t0, PasswordResult{OK: true})
			m.Press(t0, ButtonAdd30, "")
		}},
		{"после отмены", func(m *Model) {
			m.Press(t0, ButtonSubmitPassword, secret)
			m.Press(t0, ButtonCancel, secret)
		}},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			m := withPasswordPanel(t, timeUpDecision())
			s.run(m)
			if holds(reflect.ValueOf(m).Elem(), secret, 0) {
				t.Fatal("пароль найден в состоянии модели")
			}
			if strings.Contains(fmt.Sprintf("%+v", m.View(t0)), secret) {
				t.Fatal("пароль найден в View")
			}
			if strings.Contains(fmt.Sprintf("%+v", m.TakeEvents()), secret) {
				t.Fatal("пароль найден в событиях")
			}
		})
	}
}

func TestCorrectPasswordOpensChoices(t *testing.T) {
	// Ловит: набор кнопок после верного пароля не зависит от вида блокировки (ТЗ 8.2.3 п.6, Ruling 24):
	// при «время вышло» нужны +15/+30/+60, при обходе только «Снять блокировку» (главная); «Открыть настройки»
	// и «Отмена» в обоих.
	cases := []struct {
		name string
		d    core.Decision
		want []ButtonView
	}{
		{"время вышло", timeUpDecision(), []ButtonView{
			{Button: ButtonAdd15, Label: "+15", Enabled: true},
			{Button: ButtonAdd30, Label: "+30", Enabled: true},
			{Button: ButtonAdd60, Label: "+60", Enabled: true},
			{Button: ButtonOpenSettings, Label: "Открыть настройки", Enabled: true},
			{Button: ButtonCancel, Label: "Отмена", Enabled: true},
		}},
		{"обход защиты", bypassDecision(), []ButtonView{
			{Button: ButtonClearBlock, Label: "Снять блокировку", Main: true, Enabled: true},
			{Button: ButtonOpenSettings, Label: "Открыть настройки", Enabled: true},
			{Button: ButtonCancel, Label: "Отмена", Enabled: true},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := withChoices(t, c.d).View(t0)
			if !reflect.DeepEqual(v.Choices, c.want) {
				t.Fatalf("кнопки %+v, ждали %+v", v.Choices, c.want)
			}
			if v.PasswordEnabled || v.PasswordError != "" || len(v.Buttons) != 0 || !v.LockShown {
				t.Fatalf("после пароля поле закрыто, экран остаётся: %+v", v)
			}
		})
	}
}

func TestChoiceActions(t *testing.T) {
	// Ловит: нажатие выбора даёт не то действие или не те минуты; кнопка, которой нет в наборе
	// для этой блокировки (+15 при обходе, «Снять блокировку» при времени), всё же работает.
	cases := []struct {
		name string
		d    core.Decision
		b    Button
		want Action
	}{
		{"+15", timeUpDecision(), ButtonAdd15, Action{Kind: ActAddExtra, Minutes: 15}},
		{"+30", timeUpDecision(), ButtonAdd30, Action{Kind: ActAddExtra, Minutes: 30}},
		{"+60", timeUpDecision(), ButtonAdd60, Action{Kind: ActAddExtra, Minutes: 60}},
		{"настройки при времени", timeUpDecision(), ButtonOpenSettings, Action{Kind: ActOpenSettings}},
		{"настройки при обходе", bypassDecision(), ButtonOpenSettings, Action{Kind: ActOpenSettings}},
		{"снять блокировку", bypassDecision(), ButtonClearBlock, Action{Kind: ActClearBlock}},
		{"+15 при обходе", bypassDecision(), ButtonAdd15, Action{}},
		{"+60 при обходе", bypassDecision(), ButtonAdd60, Action{}},
		{"снять блокировку при времени", timeUpDecision(), ButtonClearBlock, Action{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := withChoices(t, c.d).Press(t0, c.b, ""); got != c.want {
				t.Fatalf("действие %+v, ждали %+v", got, c.want)
			}
		})
	}
}

func TestExtraTimeIsNotRepeated(t *testing.T) {
	// Ловит: двойной щелчок по «+30» добавляет 60 минут (доп. время не повторяется до ответа службы).
	m := withChoices(t, timeUpDecision())
	if a := m.Press(t0, ButtonAdd30, ""); a != (Action{Kind: ActAddExtra, Minutes: 30}) {
		t.Fatalf("первое нажатие: %+v", a)
	}
	if a := m.Press(t0, ButtonAdd30, ""); a.Kind != ActNone {
		t.Fatalf("повторное нажатие дало действие %+v", a)
	}
	if v := m.View(t0); v.Panel != PanelNone || len(v.Buttons) != testScreenButtons {
		t.Fatalf("после выбора вернулся не экран: %+v", v)
	}
}

func TestCancelEndsParentChoice(t *testing.T) {
	// Ловит: «Отмена» оставляет родителя вошедшим: следующее нажатие «+60» срабатывает без пароля.
	m := withChoices(t, timeUpDecision())
	m.Press(t0, ButtonCancel, "")
	if v := m.View(t0); v.Panel != PanelNone || len(v.Buttons) != testScreenButtons {
		t.Fatalf("после «Отмена» нет экрана: %+v", v)
	}
	if a := m.Press(t0, ButtonAdd60, ""); a.Kind != ActNone {
		t.Fatalf("действие без пароля: %+v", a)
	}
}

func TestLateResultsAreIgnored(t *testing.T) {
	// Ловит: ответ службы на пароль, который родитель уже отменил, открывает выбор действий
	// (верный ответ) или перекрашивает экран ошибкой (неверный); пауза от службы теряется.
	m := withPasswordPanel(t, timeUpDecision())
	m.Press(t0, ButtonSubmitPassword, "пароль")
	m.Press(t0, ButtonCancel, "")
	m.OnPasswordResult(t0, PasswordResult{OK: true})
	if v := m.View(t0); v.Panel != PanelNone || len(v.Buttons) != testScreenButtons {
		t.Fatalf("верный ответ после «Отмена» открыл панель: %+v", v)
	}
	m.OnPasswordResult(t0, PasswordResult{AttemptsLeft: 2})
	m.Press(t0, ButtonParentPassword, "")
	if v := m.View(t0); v.PasswordError != "" || !v.PasswordEnabled {
		t.Fatalf("чужой ответ остался на панели: Error=%q Enabled=%v", v.PasswordError, v.PasswordEnabled)
	}
	// Пауза, о которой служба сообщила после «Отмена», всё равно действует.
	m.Press(t0, ButtonCancel, "")
	m.OnPasswordResult(t0, PasswordResult{BlockedUntil: at(time.Minute)})
	m.Press(t0, ButtonParentPassword, "")
	if v := m.View(t0); v.PasswordEnabled || v.PasswordCountdown != "1:00" {
		t.Fatalf("пауза потеряна: Enabled=%v Countdown=%q", v.PasswordEnabled, v.PasswordCountdown)
	}
}

func TestResultWithoutScreenIsIgnored(t *testing.T) {
	// Ловит: ответ службы на пароль при скрытом экране (обрыв связи, родительская сессия)
	// открывает выбор действий или ставит паузу, которая потом всплывает на новом экране.
	m := withPasswordPanel(t, timeUpDecision())
	m.OnPasswordResult(at(testOffline+time.Millisecond), PasswordResult{OK: true})
	m.OnPasswordResult(at(testOffline+time.Millisecond), PasswordResult{BlockedUntil: at(time.Hour)})
	m.OnDecision(at(20*time.Second), timeUpDecision(), false, nil)
	m.Press(at(20*time.Second), ButtonParentPassword, "")
	if v := m.View(at(20 * time.Second)); v.Panel != PanelPassword || !v.PasswordEnabled || v.PasswordError != "" {
		t.Fatalf("чужой ответ остался: %+v", v)
	}

	p := withPasswordPanel(t, timeUpDecision())
	p.OnDecision(at(time.Second), timeUpDecision(), true, nil)
	p.OnPasswordResult(at(time.Second), PasswordResult{OK: true})
	p.OnDecision(at(2*time.Second), timeUpDecision(), false, nil)
	if v := p.View(at(2 * time.Second)); v.Panel != PanelNone || len(v.Buttons) != testScreenButtons {
		t.Fatalf("после сессии родителя: %+v", v)
	}
}

func TestLogoffNeedsConfirmation(t *testing.T) {
	// Ловит: выход из учётной записи без подтверждения (ТЗ 8.2.3 п.7); неверный текст вопроса;
	// «Отмена» не возвращает на экран.
	m := locked(timeUpDecision())
	if a := m.Press(t0, ButtonLogoff, ""); a.Kind != ActNone {
		t.Fatalf("нажатие «Выйти из учётной записи» сразу выходит: %+v", a)
	}
	v := m.View(t0)
	want := []ButtonView{
		{Button: ButtonCancel, Label: "Отмена", Main: true, Enabled: true},
		{Button: ButtonConfirmLogoff, Label: "Выйти", Enabled: true},
	}
	if v.Panel != PanelConfirmLogoff || v.PanelText != "Несохранённые документы закроются. Выйти?" || !reflect.DeepEqual(v.Choices, want) {
		t.Fatalf("подтверждение: Panel=%v Text=%q Choices=%+v", v.Panel, v.PanelText, v.Choices)
	}
	if a := m.Press(t0, ButtonConfirmLogoff, ""); a != (Action{Kind: ActLogoff}) {
		t.Fatalf("«Выйти»: %+v", a)
	}

	m2 := locked(timeUpDecision())
	m2.Press(t0, ButtonLogoff, "")
	m2.Press(t0, ButtonCancel, "")
	if v := m2.View(t0); v.Panel != PanelNone || v.PanelText != "" || len(v.Buttons) != testScreenButtons {
		t.Fatalf("после «Отмена»: %+v", v)
	}
	if a := m2.Press(t0, ButtonConfirmLogoff, ""); a.Kind != ActNone {
		t.Fatalf("«Выйти» сработала без вопроса: %+v", a)
	}
}

func TestButtonMatrix(t *testing.T) {
	// Ловит: кнопка срабатывает на чужой панели (например «Выйти» из окна пароля,
	// «+30» с главного экрана, «Продолжить» из подтверждения выхода).
	all := []Button{ButtonNone, ButtonParentPassword, ButtonLogoff, ButtonSubmitPassword, ButtonCancel, ButtonAdd15,
		ButtonAdd30, ButtonAdd60, ButtonOpenSettings, ButtonClearBlock, ButtonConfirmLogoff, ButtonWarningOK,
		ButtonShutdown, ButtonConfirmShutdown, ButtonSwitchUser}
	states := []struct {
		name  string
		build func(t *testing.T) *Model
		ok    map[Button]Action
	}{
		{"главный экран", func(t *testing.T) *Model { return locked(timeUpDecision()) },
			map[Button]Action{ButtonSwitchUser: {Kind: ActSwitchUser}}},
		{"главный экран при обходе", func(t *testing.T) *Model { return locked(bypassDecision()) },
			map[Button]Action{ButtonSwitchUser: {Kind: ActSwitchUser}}},
		{"панель пароля", func(t *testing.T) *Model { return withPasswordPanel(t, timeUpDecision()) },
			map[Button]Action{ButtonSubmitPassword: {Kind: ActCheckPassword, Password: "pw-123456"}}},
		{"выбор при времени", func(t *testing.T) *Model { return withChoices(t, timeUpDecision()) },
			map[Button]Action{
				ButtonAdd15: {Kind: ActAddExtra, Minutes: 15}, ButtonAdd30: {Kind: ActAddExtra, Minutes: 30},
				ButtonAdd60: {Kind: ActAddExtra, Minutes: 60}, ButtonOpenSettings: {Kind: ActOpenSettings}}},
		{"выбор при обходе", func(t *testing.T) *Model { return withChoices(t, bypassDecision()) },
			map[Button]Action{ButtonClearBlock: {Kind: ActClearBlock}, ButtonOpenSettings: {Kind: ActOpenSettings}}},
		{"подтверждение выхода", func(t *testing.T) *Model {
			m := locked(timeUpDecision())
			m.Press(t0, ButtonLogoff, "")
			return m
		}, map[Button]Action{ButtonConfirmLogoff: {Kind: ActLogoff}}},
		{"подтверждение выключения", func(t *testing.T) *Model {
			m := locked(timeUpDecision())
			m.Press(t0, ButtonShutdown, "")
			return m
		}, map[Button]Action{ButtonConfirmShutdown: {Kind: ActShutdown}}},
	}
	for _, s := range states {
		for _, b := range all {
			m := s.build(t)
			want := s.ok[b]
			if got := m.Press(t0, b, "pw-123456"); got != want {
				t.Errorf("%s, кнопка %d: действие %+v, ждали %+v", s.name, b, got, want)
			}
		}
	}
}

func TestNoScreenNoActions(t *testing.T) {
	// Ловит: нажатия действуют, когда экрана нет (нет решения, блокировки нет, родительская
	// сессия, нет связи): иначе невидимая кнопка может выйти из учётки или снять блокировку.
	all := []Button{ButtonParentPassword, ButtonLogoff, ButtonSubmitPassword, ButtonCancel, ButtonAdd15,
		ButtonAdd30, ButtonAdd60, ButtonOpenSettings, ButtonClearBlock, ButtonConfirmLogoff, ButtonWarningOK,
		ButtonShutdown, ButtonConfirmShutdown, ButtonSwitchUser}
	cases := []struct {
		name  string
		build func() (*Model, time.Time)
	}{
		{"нет решения", func() (*Model, time.Time) { return NewModel(), t0 }},
		{"блокировки нет", func() (*Model, time.Time) { return locked(runningDecision(900)), t0 }},
		{"родительская сессия", func() (*Model, time.Time) {
			m := NewModel()
			m.OnDecision(t0, timeUpDecision(), true, nil)
			return m, t0
		}},
		{"нет связи", func() (*Model, time.Time) {
			return withChoices(t, timeUpDecision()), at(testOffline + time.Millisecond)
		}},
	}
	for _, c := range cases {
		for _, b := range all {
			m, now := c.build()
			if got := m.Press(now, b, "pw-123456"); got != (Action{}) {
				t.Errorf("%s, кнопка %d: действие %+v", c.name, b, got)
			}
		}
	}
}

func TestOfflineBoundary(t *testing.T) {
	// Ловит: граница 10 с (ровно 10 с экран ещё показан, 10 с + 1 мс скрыт, Ruling 15);
	// при потере связи остаются экран, панели, плашки или перехват клавиш (NFR-10, R18).
	cases := []struct {
		after time.Duration
		shown bool
	}{
		{0, true},
		{testOffline - time.Millisecond, true},
		{testOffline, true},
		{testOffline + time.Millisecond, false},
		{time.Minute, false},
	}
	for _, c := range cases {
		m := withPasswordPanel(t, timeUpDecision())
		v := m.View(at(c.after))
		if v.LockShown != c.shown || v.SuppressKeys != c.shown {
			t.Errorf("через %v: LockShown=%v SuppressKeys=%v; ждали %v", c.after, v.LockShown, v.SuppressKeys, c.shown)
		}
		if !c.shown && (v.Panel != PanelNone || v.Warning != nil || len(v.Buttons) != 0 || len(v.Choices) != 0 || v.PasswordEnabled) {
			t.Errorf("через %v при потере связи остались элементы: %+v", c.after, v)
		}
	}
}

func TestFailedPollsDoNotExtendLife(t *testing.T) {
	// Ловит: неудачный опрос службы (OnServiceUnreachable) или Tick сбрасывают отсчёт 10 с:
	// экран с недоступной службой не снимался бы никогда.
	m := locked(timeUpDecision())
	for s := 1; s <= 9; s++ {
		m.OnServiceUnreachable(at(time.Duration(s) * time.Second))
		m.Tick(at(time.Duration(s) * time.Second))
	}
	m.OnServiceUnreachable(at(testOffline))
	if v := m.View(at(testOffline)); !v.LockShown {
		t.Fatal("ровно через 10 с экран должен быть ещё показан")
	}
	m.OnServiceUnreachable(at(testOffline + time.Millisecond))
	m.Tick(at(testOffline + time.Millisecond))
	if v := m.View(at(testOffline + time.Millisecond)); v.LockShown || v.SuppressKeys {
		t.Fatalf("через 10 с + 1 мс экран не снят: %+v", v)
	}
}

func TestOfflineEventAfterRecovery(t *testing.T) {
	// Ловит: нет события после восстановления связи, событие записано дважды или с неверной
	// длительностью; короткий обрыв (до 10 с включительно) попал в журнал; TakeEvents вернул nil.
	m := locked(timeUpDecision())
	if ev := m.TakeEvents(); ev == nil || len(ev) != 0 {
		t.Fatalf("пустой журнал должен быть [], а не nil: %#v", ev)
	}
	m.Tick(at(25 * time.Second))
	if ev := m.TakeEvents(); len(ev) != 0 {
		t.Fatalf("событие записано до восстановления связи: %+v", ev)
	}
	m.OnDecision(at(25*time.Second), timeUpDecision(), false, nil)
	ev := m.TakeEvents()
	if len(ev) != 1 || ev[0].Kind != "guard_offline" || ev[0].Details["seconds"] != 25 {
		t.Fatalf("события %+v", ev)
	}
	if text, _ := ev[0].Details["text"].(string); text != "Страж был без связи со службой 25 с" {
		t.Fatalf("текст события %q", text)
	}
	m.OnDecision(at(26*time.Second), timeUpDecision(), false, nil)
	m.OnDecision(at(27*time.Second), timeUpDecision(), false, nil)
	if ev := m.TakeEvents(); ev == nil || len(ev) != 0 {
		t.Fatalf("повторная запись события: %#v", ev)
	}

	short := locked(timeUpDecision())
	short.OnDecision(at(testOffline), timeUpDecision(), false, nil)
	if ev := short.TakeEvents(); len(ev) != 0 {
		t.Fatalf("обрыв ровно в 10 с попал в журнал: %+v", ev)
	}
	edge := locked(timeUpDecision())
	edge.OnDecision(at(testOffline+time.Millisecond), timeUpDecision(), false, nil)
	if ev := edge.TakeEvents(); len(ev) != 1 || ev[0].Details["seconds"] != 10 {
		t.Fatalf("обрыв в 10 с + 1 мс: %+v", ev)
	}
}

func TestOfflineEventWithoutTick(t *testing.T) {
	// Ловит: событие «без связи» пишется только если между обрывом и ответом был Tick:
	// при зависшем опросе (нет ни Tick, ни OnServiceUnreachable) обрыв терялся бы.
	m := locked(timeUpDecision())
	m.OnDecision(at(90*time.Second), runningDecision(3000), false, nil)
	ev := m.TakeEvents()
	if len(ev) != 1 || ev[0].Kind != "guard_offline" || ev[0].Details["seconds"] != 90 {
		t.Fatalf("события %+v", ev)
	}
}

func TestOfflineResetsAndScreenReturnsByFreshDecision(t *testing.T) {
	// Ловит: после обрыва возвращается старая панель пароля, старая ошибка или старый вид
	// блокировки вместо свежего решения службы; нет связи не снимает ограничение навсегда.
	m := withPasswordPanel(t, timeUpDecision())
	m.Press(t0, ButtonSubmitPassword, "неверный")
	m.OnPasswordResult(t0, PasswordResult{AttemptsLeft: 3})
	m.Tick(at(testOffline + time.Millisecond))

	m.OnDecision(at(30*time.Second), bypassDecision(), false, nil)
	v := m.View(at(30 * time.Second))
	if !v.LockShown || v.Lock.Reason != "Попытка обойти защиту" || v.Panel != PanelNone || v.PasswordError != "" || len(v.Buttons) != testBypassButtons {
		t.Fatalf("после обрыва: %+v", v)
	}
	m.OnDecision(at(31*time.Second), runningDecision(900), false, nil)
	if v := m.View(at(31 * time.Second)); v.LockShown || v.SuppressKeys {
		t.Fatalf("свежее решение без блокировки, а экран остался: %+v", v)
	}
}

func TestOfflineDropsPauseDisplay(t *testing.T) {
	// Ловит: после обрыва на экране остаётся старая пауза пароля: состояние службы неизвестно,
	// родитель не должен упираться в устаревший отсчёт на аварийном пути.
	m := withPasswordPanel(t, timeUpDecision())
	m.OnPasswordResult(t0, PasswordResult{BlockedUntil: at(15 * time.Minute)})
	m.Tick(at(testOffline + time.Millisecond))
	m.OnDecision(at(20*time.Second), timeUpDecision(), false, nil)
	m.Press(at(20*time.Second), ButtonParentPassword, "")
	if v := m.View(at(20 * time.Second)); !v.PasswordEnabled || v.PasswordError != "" {
		t.Fatalf("старая пауза осталась: Enabled=%v Error=%q", v.PasswordEnabled, v.PasswordError)
	}
}

func TestParentSessionHidesScreen(t *testing.T) {
	// Ловит: экран и перехват клавиш поверх окна родителя (Ruling 19); плашки в родительской
	// сессии; после сессии возвращается старая панель, а не экран по решению.
	m := withChoices(t, timeUpDecision())
	if a := m.Press(t0, ButtonOpenSettings, ""); a.Kind != ActOpenSettings {
		t.Fatalf("действие %+v", a)
	}
	m.OnDecision(at(time.Second), timeUpDecision(), true, []int{5})
	v := m.View(at(time.Second))
	if v.LockShown || v.SuppressKeys || v.Warning != nil || v.Panel != PanelNone || len(v.Buttons) != 0 || len(v.Choices) != 0 {
		t.Fatalf("в сессии родителя: %+v", v)
	}
	// Конец сессии: экран снова по решению, чистый, и плашка из сессии не всплыла.
	m.OnDecision(at(2*time.Second), timeUpDecision(), false, nil)
	v = m.View(at(2 * time.Second))
	if !v.LockShown || !v.SuppressKeys || v.Panel != PanelNone || len(v.Buttons) != testScreenButtons || v.Warning != nil {
		t.Fatalf("после сессии: %+v", v)
	}
	// Если ограничения уже нет, экран не возвращается.
	m.OnDecision(at(3*time.Second), timeUpDecision(), true, nil)
	m.OnDecision(at(4*time.Second), runningDecision(1800), false, nil)
	if v := m.View(at(4 * time.Second)); v.LockShown || v.SuppressKeys || v.Warning != nil {
		t.Fatalf("экран вернулся без ограничения: %+v", v)
	}
}

func TestParentSessionWithoutLockHidesPlates(t *testing.T) {
	// Ловит: плашка предупреждения показывается в родительской сессии, даже когда блокировки нет;
	// пороги, сообщённые в сессии, откладываются и показываются после неё.
	m := NewModel()
	m.OnDecision(t0, runningDecision(300), true, []int{5})
	if v := m.View(t0); v.Warning != nil {
		t.Fatalf("плашка в сессии родителя: %+v", v.Warning)
	}
	m.OnDecision(at(time.Second), runningDecision(299), false, nil)
	if v := m.View(at(time.Second)); v.Warning != nil {
		t.Fatalf("плашка из сессии всплыла после неё: %+v", v.Warning)
	}
	m.OnDecision(at(3*time.Second), runningDecision(300), true, nil)
	m.OnExtraAdded(at(3*time.Second), 30)
	if v := m.View(at(3 * time.Second)); v.Warning != nil {
		t.Fatalf("«Добавлено» в сессии родителя: %+v", v.Warning)
	}
}

func TestWarningTexts(t *testing.T) {
	// Ловит: тексты плашек 15/5/1 (ТЗ 8.2.2) не дословные; звук не только у плашки за 1 минуту;
	// отсчёт не по остатку из решения; нет кнопки «Понятно».
	cases := []struct {
		name      string
		thr       int
		remaining int
		text      string
		countdown string
		sound     bool
	}{
		{"за 15 минут", 15, 900, "Через 15 минут время на сегодня закончится", "", false},
		{"за 5 минут", 5, 300, "Через 5 минут время закончится. Сохрани игру", "", false},
		{"за 1 минуту, 59 с", 1, 59, "Осталась 1 минута: 0:59", "0:59", true},
		{"за 1 минуту, 60 с", 1, 60, "Осталась 1 минута: 1:00", "1:00", true},
		{"за 1 минуту, 5 с", 1, 5, "Осталась 1 минута: 0:05", "0:05", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewModel()
			m.OnDecision(t0, runningDecision(c.remaining), false, []int{c.thr})
			v := m.View(t0)
			w := v.Warning
			if w == nil {
				t.Fatal("плашки нет")
			}
			wantButtons := []ButtonView{{Button: ButtonWarningOK, Label: "Понятно", Enabled: true}}
			if w.Text != c.text || w.Countdown != c.countdown || w.Sound != c.sound || !reflect.DeepEqual(w.Buttons, wantButtons) {
				t.Fatalf("плашка %+v", *w)
			}
			if v.LockShown || v.SuppressKeys || v.Panel != PanelNone {
				t.Fatalf("плашка не должна включать экран и перехват: %+v", v)
			}
		})
	}
}

func TestWarningCountdownFollowsDecisions(t *testing.T) {
	// Ловит: отсчёт «Осталась 1 минута» замирает на первом значении или считается не по
	// остатку решения; новый номер плашки (для звука) выдаётся при каждом обновлении отсчёта.
	m := NewModel()
	m.OnDecision(t0, runningDecision(59), false, []int{1})
	first := m.View(t0).Warning
	m.OnDecision(at(time.Second), runningDecision(58), false, nil)
	m.OnDecision(at(2*time.Second), runningDecision(57), false, nil)
	w := m.View(at(2 * time.Second)).Warning
	if w == nil || w.Text != "Осталась 1 минута: 0:57" || w.Countdown != "0:57" {
		t.Fatalf("отсчёт %+v", w)
	}
	if w.ID != first.ID {
		t.Fatalf("номер плашки изменился при обновлении отсчёта: %d -> %d (звук сыграл бы снова)", first.ID, w.ID)
	}
	m.OnDecision(at(3*time.Second), runningDecision(-4), false, nil)
	if w := m.View(at(3 * time.Second)).Warning; w == nil || w.Countdown != "0:00" {
		t.Fatalf("отрицательный остаток: %+v", w)
	}
}

func TestWarningAutoClose(t *testing.T) {
	// Ловит: граница 15 с (до 15 с плашка видна, на 15-й секунде закрыта); плашка висит дольше срока;
	// «Понятно» не закрывает; повторное «Понятно» что-то делает.
	for _, c := range []struct {
		after   time.Duration
		visible bool
	}{
		{testWarnLife - time.Millisecond, true},
		{testWarnLife, false},
		{testWarnLife + time.Second, false},
	} {
		// Последнее решение приходит раньше момента проверки: срок плашки должен
		// считаться по часам в View, а не только по ходу решений; Tick даёт тот же итог.
		for _, tick := range []bool{false, true} {
			m := NewModel()
			m.OnDecision(t0, runningDecision(900), false, []int{15})
			feed(m, 1, int((c.after-time.Millisecond)/time.Second), 900)
			if tick {
				m.Tick(at(c.after))
			}
			if got := m.View(at(c.after)).Warning != nil; got != c.visible {
				t.Errorf("через %v (Tick=%v): плашка видна = %v, ждали %v", c.after, tick, got, c.visible)
			}
		}
	}

	m := NewModel()
	m.OnDecision(t0, runningDecision(900), false, []int{15})
	if a := m.Press(at(2*time.Second), ButtonWarningOK, ""); a != (Action{}) {
		t.Fatalf("«Понятно» вернуло действие %+v", a)
	}
	if m.View(at(2*time.Second)).Warning != nil {
		t.Fatal("«Понятно» не закрыло плашку")
	}
	if a := m.Press(at(2*time.Second), ButtonWarningOK, ""); a != (Action{}) {
		t.Fatalf("повторное «Понятно» вернуло действие %+v", a)
	}
}

func TestWarningReplaced(t *testing.T) {
	// Ловит: новая плашка не заменяет старую (две сразу, L-118) или номер не меняется (звук не сыграет).
	m := NewModel()
	m.OnDecision(t0, runningDecision(900), false, []int{15})
	a := m.View(t0).Warning
	m.OnDecision(at(3*time.Second), runningDecision(300), false, []int{5})
	b := m.View(at(3 * time.Second)).Warning
	if b == nil || b.Text != "Через 5 минут время закончится. Сохрани игру" || b.ID == a.ID {
		t.Fatalf("плашка за 5 минут: %+v (прежняя %+v)", b, a)
	}
	m.OnDecision(at(4*time.Second), runningDecision(59), false, []int{1})
	c := m.View(at(4 * time.Second)).Warning
	if c == nil || c.Text != "Осталась 1 минута: 0:59" || c.ID == b.ID || !c.Sound {
		t.Fatalf("плашка за 1 минуту: %+v", c)
	}
}

func TestWarningPicksMostUrgent(t *testing.T) {
	// Ловит: несколько порогов в одном ответе дают несколько плашек или самый дальний порог;
	// неизвестные пороги (7, 0, -5, 100) рисуют плашку.
	cases := []struct {
		name string
		rem  int
		in   []int
		want string
	}{
		{"15 и 5", 300, []int{15, 5}, "Через 5 минут время закончится. Сохрани игру"},
		{"1, 15, 5", 59, []int{1, 15, 5}, "Осталась 1 минута: 0:59"},
		{"15 и мусор", 900, []int{7, 15, 100}, "Через 15 минут время на сегодня закончится"},
		{"только мусор", 900, []int{7, 0, -5, 100}, ""},
		{"nil", 900, nil, ""},
		{"пусто", 900, []int{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewModel()
			m.OnDecision(t0, runningDecision(c.rem), false, c.in)
			w := m.View(t0).Warning
			if c.want == "" {
				if w != nil {
					t.Fatalf("плашка %+v", *w)
				}
				return
			}
			if w == nil || w.Text != c.want {
				t.Fatalf("плашка %+v, ждали %q", w, c.want)
			}
		})
	}
}

func TestNoWarningsOverLockScreen(t *testing.T) {
	// Ловит: плашка показана при экране блокировки (ТЗ: во время экрана плашек нет); плашка,
	// объявленная под экраном, всплывает после его снятия; старая плашка переживает появление экрана.
	// Остаток после снятия экрана 900 с (+15 минут): плашка «за 15» не устарела бы и всплыла,
	// если бы модель её держала.
	m := NewModel()
	m.OnDecision(t0, timeUpDecision(), false, []int{15})
	if v := m.View(t0); v.Warning != nil || !v.LockShown {
		t.Fatalf("при экране блокировки: %+v", v)
	}
	m.OnDecision(at(time.Second), runningDecision(900), false, nil)
	if v := m.View(at(time.Second)); v.Warning != nil || v.LockShown {
		t.Fatalf("плашка из-под экрана всплыла: %+v", v)
	}

	m2 := NewModel()
	m2.OnDecision(t0, runningDecision(900), false, []int{15})
	if m2.View(t0).Warning == nil {
		t.Fatal("плашка не показана")
	}
	m2.OnDecision(at(time.Second), timeUpDecision(), false, nil)
	if v := m2.View(at(time.Second)); v.Warning != nil || !v.LockShown {
		t.Fatalf("плашка поверх экрана: %+v", v)
	}
	m2.OnDecision(at(2*time.Second), runningDecision(900), false, nil)
	if v := m2.View(at(2 * time.Second)); v.Warning != nil {
		t.Fatalf("старая плашка вернулась после экрана: %+v", *v.Warning)
	}
}

func TestStaleWarningIsDropped(t *testing.T) {
	// Ловит: плашка «через 5 минут» висит после того, как событие отодвинулось
	// (время добавили из окна родителя, R9) или лимит снят.
	m := NewModel()
	m.OnDecision(t0, runningDecision(300), false, []int{5})
	m.OnDecision(at(time.Second), runningDecision(299), false, nil)
	if m.View(at(time.Second)).Warning == nil {
		t.Fatal("плашка исчезла без причины")
	}
	m.OnDecision(at(2*time.Second), runningDecision(2099), false, nil)
	if w := m.View(at(2 * time.Second)).Warning; w != nil {
		t.Fatalf("плашка осталась после доп. времени: %+v", *w)
	}

	m2 := NewModel()
	m2.OnDecision(t0, runningDecision(59), false, []int{1})
	unlimited := runningDecision(0)
	unlimited.Unlimited = true
	m2.OnDecision(at(time.Second), unlimited, false, nil)
	if w := m2.View(at(time.Second)).Warning; w != nil {
		t.Fatalf("плашка осталась после снятия лимита: %+v", *w)
	}
}

func TestOfflineHidesPlate(t *testing.T) {
	// Ловит: плашка остаётся, когда связи со службой нет 10 с (плашка живёт 15 с, поэтому
	// её скрывает именно потеря связи, а не срок).
	m := NewModel()
	m.OnDecision(t0, runningDecision(300), false, []int{5})
	if m.View(at(testOffline)).Warning == nil {
		t.Fatal("ровно через 10 с плашка должна быть видна")
	}
	if w := m.View(at(testOffline + time.Millisecond)).Warning; w != nil {
		t.Fatalf("плашка при потере связи: %+v", *w)
	}
}

func TestAddedPluralForms(t *testing.T) {
	// Ловит: склонение «минута/минуты/минут» и согласование «Добавлена 1 минута» / «Добавлено 2 минуты»
	// (L-36: генерируемые тексты проверяются таблицей, включая 11–14, 21, 111, 121).
	cases := []struct {
		n    int
		want string
	}{
		{1, "Добавлена 1 минута"},
		{2, "Добавлено 2 минуты"},
		{5, "Добавлено 5 минут"},
		{11, "Добавлено 11 минут"},
		{15, "Добавлено 15 минут"},
		{21, "Добавлена 21 минута"},
		{22, "Добавлено 22 минуты"},
		{25, "Добавлено 25 минут"},
		{30, "Добавлено 30 минут"},
		{60, "Добавлено 60 минут"},
		{111, "Добавлено 111 минут"},
		{121, "Добавлена 121 минута"},
	}
	for _, c := range cases {
		m := NewModel()
		m.OnDecision(t0, runningDecision(900), false, nil)
		m.OnExtraAdded(t0, c.n)
		w := m.View(t0).Warning
		if w == nil || w.Text != c.want {
			t.Errorf("%d: плашка %+v, ждали %q", c.n, w, c.want)
		}
	}
}

func TestAddedPlate(t *testing.T) {
	// Ловит: доп. время не заменяет видимую плашку предупреждения (ТЗ 8.2.2 п.4); «Добавлено»
	// висит дольше 5 с или закрывается раньше; у неё есть звук/отсчёт/кнопки; ноль и минус минут рисуют плашку.
	m := NewModel()
	m.OnDecision(t0, runningDecision(300), false, []int{5})
	old := m.View(t0).Warning
	m.OnDecision(at(time.Second), runningDecision(2099), false, nil)
	m.OnExtraAdded(at(time.Second), 30)
	w := m.View(at(time.Second)).Warning
	if w == nil || w.Text != "Добавлено 30 минут" || w.Sound || w.Countdown != "" || len(w.Buttons) != 0 || w.Buttons == nil || w.ID == old.ID {
		t.Fatalf("плашка %+v", w)
	}
	for _, c := range []struct {
		after   time.Duration
		visible bool
	}{
		{testAddedLife - time.Millisecond, true},
		{testAddedLife, false},
	} {
		for _, tick := range []bool{false, true} {
			m2 := NewModel()
			m2.OnDecision(t0, runningDecision(900), false, nil)
			m2.OnExtraAdded(t0, 30)
			feed(m2, 1, int((c.after-time.Millisecond)/time.Second), 900)
			if tick {
				m2.Tick(at(c.after))
			}
			if got := m2.View(at(c.after)).Warning != nil; got != c.visible {
				t.Errorf("через %v (Tick=%v): видна = %v, ждали %v", c.after, tick, got, c.visible)
			}
		}
	}
	m3 := NewModel()
	m3.OnDecision(t0, runningDecision(900), false, nil)
	m3.OnExtraAdded(t0, 0)
	m3.OnExtraAdded(t0, -15)
	if w := m3.View(t0).Warning; w != nil {
		t.Fatalf("плашка для 0 и -15 минут: %+v", *w)
	}
}

func TestAddedAfterLockScreen(t *testing.T) {
	// Ловит: после доп. времени с экрана блокировки нет плашки «Добавлено N минут» (ни при
	// порядке «служба подтвердила, потом решение», ни при обратном); плашка показана под экраном.
	for _, order := range []string{"сначала подтверждение", "сначала решение"} {
		t.Run(order, func(t *testing.T) {
			m := withChoices(t, timeUpDecision())
			if a := m.Press(t0, ButtonAdd30, ""); a != (Action{Kind: ActAddExtra, Minutes: 30}) {
				t.Fatalf("действие %+v", a)
			}
			if order == "сначала подтверждение" {
				m.OnExtraAdded(at(time.Second), 30)
				if v := m.View(at(time.Second)); v.Warning != nil || !v.LockShown {
					t.Fatalf("плашка под экраном блокировки: %+v", v)
				}
				m.OnDecision(at(time.Second), runningDecision(1800), false, nil)
			} else {
				m.OnDecision(at(time.Second), runningDecision(1800), false, nil)
				m.OnExtraAdded(at(time.Second), 30)
			}
			v := m.View(at(time.Second))
			if v.LockShown || v.SuppressKeys || v.Warning == nil || v.Warning.Text != "Добавлено 30 минут" {
				t.Fatalf("после доп. времени: %+v", v)
			}
			if got := m.View(at(time.Second + testAddedLife - time.Millisecond)); got.Warning == nil {
				t.Fatal("плашка закрылась раньше 5 с")
			}
		})
	}
}

func TestExtraAddedIgnoredWithoutLink(t *testing.T) {
	// Ловит: «Добавлено» рисуется без решения службы и при потере связи.
	m := NewModel()
	m.OnExtraAdded(t0, 30)
	if v := m.View(t0); v.Warning != nil {
		t.Fatalf("плашка без решения: %+v", *v.Warning)
	}
	m.OnDecision(t0, runningDecision(900), false, nil)
	m.OnExtraAdded(at(testOffline+time.Millisecond), 30)
	m.OnDecision(at(testOffline+2*time.Millisecond), runningDecision(900), false, nil)
	if v := m.View(at(testOffline + 2*time.Millisecond)); v.Warning != nil {
		t.Fatalf("плашка из периода без связи: %+v", *v.Warning)
	}
}

func TestLockKindChangeResetsPanel(t *testing.T) {
	// Ловит: при смене вида блокировки (время -> обход) остаётся панель выбора от прежнего вида:
	// родитель увидел бы чужой набор кнопок.
	m := withChoices(t, timeUpDecision())
	m.OnDecision(at(time.Second), bypassDecision(), false, nil)
	v := m.View(at(time.Second))
	if v.Panel != PanelNone || v.Lock.Reason != "Попытка обойти защиту" || len(v.Buttons) != testBypassButtons {
		t.Fatalf("после смены вида: %+v", v)
	}
	// Тот же вид блокировки: обновления решения панель не сбрасывают.
	m2 := withPasswordPanel(t, timeUpDecision())
	m2.OnDecision(at(time.Second), timeUpDecision(), false, nil)
	m2.OnDecision(at(2*time.Second), timeUpDecision(), false, nil)
	if v := m2.View(at(2 * time.Second)); v.Panel != PanelPassword {
		t.Fatalf("секундный опрос закрыл панель пароля: %+v", v)
	}
}

func TestLockGoneResetsPanel(t *testing.T) {
	// Ловит: после снятия блокировки (доп. время из окна родителя) панель остаётся
	// «открытой» и всплывает вместе со следующим экраном.
	m := withChoices(t, timeUpDecision())
	m.OnDecision(at(time.Second), runningDecision(900), false, nil)
	m.OnDecision(at(2*time.Second), timeUpDecision(), false, nil)
	v := m.View(at(2 * time.Second))
	if v.Panel != PanelNone || len(v.Buttons) != testScreenButtons {
		t.Fatalf("новый экран с прежней панелью: %+v", v)
	}
}

func TestDisabledButtonsExplainWhy(t *testing.T) {
	// Ловит: недоступная кнопка без пояснения (L-17, ТЗ 8.0 п.5) и пояснение у доступной;
	// каждая панель содержит «Отмена» либо кнопки экрана: тупика нет (L-119).
	builders := map[string]func() (*Model, time.Time){
		"экран":         func() (*Model, time.Time) { return locked(timeUpDecision()), t0 },
		"панель пароля": func() (*Model, time.Time) { return withPasswordPanel(t, timeUpDecision()), t0 },
		"пауза": func() (*Model, time.Time) {
			m := withPasswordPanel(t, timeUpDecision())
			m.OnPasswordResult(t0, PasswordResult{BlockedUntil: at(time.Minute)})
			return m, at(7 * time.Second)
		},
		"выбор времени": func() (*Model, time.Time) { return withChoices(t, timeUpDecision()), t0 },
		"выбор обхода":  func() (*Model, time.Time) { return withChoices(t, bypassDecision()), t0 },
		"выход": func() (*Model, time.Time) {
			m := locked(timeUpDecision())
			m.Press(t0, ButtonLogoff, "")
			return m, t0
		},
		"выключение": func() (*Model, time.Time) {
			m := locked(timeUpDecision())
			m.Press(t0, ButtonShutdown, "")
			return m, t0
		},
	}
	for name, build := range builders {
		m, now := build()
		v := m.View(now)
		cancel := false
		for _, b := range append(append([]ButtonView{}, v.Buttons...), v.Choices...) {
			if !b.Enabled && b.Hint == "" {
				t.Errorf("%s: кнопка %q недоступна без пояснения", name, b.Label)
			}
			if b.Enabled && b.Hint != "" {
				t.Errorf("%s: у доступной кнопки %q лишнее пояснение %q", name, b.Label, b.Hint)
			}
			if b.Enabled && (b.Button == ButtonCancel || b.Button == ButtonParentPassword) {
				cancel = true
			}
		}
		if !cancel {
			t.Errorf("%s: нет доступного выхода (Отмена или Пароль родителя)", name)
		}
	}
}

func TestTextsAreChildFriendly(t *testing.T) {
	// Ловит: длинное или среднее тире в текстах, технические слова (ошибка, служба, pipe) и «вы»-форму
	// ребёнку в плашках (NFR-11, L-112).
	m := NewModel()
	var all []string
	for _, rem := range [][2]int{{900, 15}, {300, 5}, {59, 1}} {
		m.OnDecision(t0, runningDecision(rem[0]), false, []int{rem[1]})
		all = append(all, m.View(t0).Warning.Text)
	}
	m.OnExtraAdded(t0, 30)
	all = append(all, m.View(t0).Warning.Text)
	m2 := withPasswordPanel(t, timeUpDecision())
	m2.Press(t0, ButtonSubmitPassword, "")
	all = append(all, m2.View(t0).PasswordError)
	m2.OnPasswordResult(t0, PasswordResult{AttemptsLeft: 2})
	all = append(all, m2.View(t0).PasswordError)
	m2.OnPasswordResult(t0, PasswordResult{BlockedUntil: at(time.Minute)})
	all = append(all, m2.View(t0).PasswordError)
	m3 := locked(timeUpDecision())
	m3.Press(t0, ButtonLogoff, "")
	all = append(all, m3.View(t0).PanelText)
	m3.Press(t0, ButtonCancel, "")
	m3.Press(t0, ButtonShutdown, "")
	all = append(all, m3.View(t0).PanelText)
	for _, b := range m3.View(t0).Choices {
		all = append(all, b.Label)
	}
	m3.Press(t0, ButtonCancel, "")
	for _, b := range m3.View(t0).Buttons {
		all = append(all, b.Label)
	}
	m3.OnDecision(at(40*time.Second), timeUpDecision(), false, nil)
	for _, e := range m3.TakeEvents() {
		all = append(all, fmt.Sprint(e.Details["text"]))
	}
	for _, s := range all {
		if s == "" || strings.ContainsAny(s, "—–") {
			t.Errorf("текст %q пуст или с длинным тире", s)
		}
		low := strings.ToLower(s)
		for _, bad := range []string{"ошибка", "error", "pipe", "ipc", "таймаут", "exception"} {
			if strings.Contains(low, bad) {
				t.Errorf("технический текст %q (%s)", s, bad)
			}
		}
	}
}

func TestViewArraysAreNeverNull(t *testing.T) {
	// Ловит: nil-срезы в View уходят в JSON как null (CLAUDE.md, правило 6): страж отдаёт
	// состояние окна в тестовых командах и в снимках.
	models := []*Model{
		NewModel(),
		locked(timeUpDecision()),
		withPasswordPanel(t, timeUpDecision()),
		withChoices(t, bypassDecision()),
		func() *Model {
			m := locked(timeUpDecision())
			m.Press(t0, ButtonShutdown, "")
			return m
		}(),
	}
	w := NewModel()
	w.OnDecision(t0, runningDecision(300), false, []int{5})
	models = append(models, w)
	a := NewModel()
	a.OnDecision(t0, runningDecision(300), false, nil)
	a.OnExtraAdded(t0, 15)
	models = append(models, a)
	for i, m := range models {
		raw, err := json.Marshal(m.View(t0))
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{`"Buttons":null`, `"Choices":null`} {
			if strings.Contains(string(raw), bad) {
				t.Errorf("модель %d: %s в %s", i, bad, raw)
			}
		}
	}
}

func TestModelIsSafeForConcurrentUse(t *testing.T) {
	// Ловит: гонку между опросом службы (горутина) и окном (поток интерфейса); проверяется под -race.
	m := NewModel()
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				now := at(time.Duration(i) * 10 * time.Millisecond)
				switch g {
				case 0:
					m.OnDecision(now, timeUpDecision(), false, []int{5})
				case 1:
					m.Press(now, ButtonParentPassword, "pw")
					m.OnPasswordResult(now, PasswordResult{AttemptsLeft: 3})
				case 2:
					_ = m.View(now)
					m.Tick(now)
				default:
					m.OnExtraAdded(now, 15)
					m.OnServiceUnreachable(now)
					_ = m.TakeEvents()
				}
			}
		}(g)
	}
	wg.Wait()
}

// --- выходы с экрана блокировки (ТЗ 8.2.3 п.7-10, D-40) ---------------------------------------------

func TestShutdownNeedsConfirmation(t *testing.T) {
	// Ловит: выключение без подтверждения (ТЗ 8.2.3 п.8); неверный вопрос или подписи кнопок;
	// «Отмена» не возвращает на экран; «Выключить» срабатывает без вопроса; на панели
	// выключения «Выйти» выходит из учётной записи.
	for name, d := range map[string]core.Decision{"время": timeUpDecision(), "обход": bypassDecision()} {
		m := locked(d)
		if a := m.Press(t0, ButtonShutdown, ""); a.Kind != ActNone {
			t.Fatalf("%s: нажатие «Выключить компьютер» сразу выключает: %+v", name, a)
		}
		v := m.View(t0)
		want := []ButtonView{
			{Button: ButtonCancel, Label: "Отмена", Main: true, Enabled: true},
			{Button: ButtonConfirmShutdown, Label: "Выключить", Enabled: true},
		}
		if v.Panel != PanelConfirmShutdown || v.PanelText != "Несохранённые документы закроются. Выключить компьютер?" ||
			!reflect.DeepEqual(v.Choices, want) || len(v.Buttons) != 0 {
			t.Fatalf("%s: подтверждение: Panel=%v Text=%q Choices=%+v Buttons=%+v", name, v.Panel, v.PanelText, v.Choices, v.Buttons)
		}
		if a := m.Press(t0, ButtonConfirmLogoff, ""); a.Kind != ActNone {
			t.Fatalf("%s: «Выйти» сработала на панели выключения: %+v", name, a)
		}
		if a := m.Press(t0, ButtonConfirmShutdown, ""); a != (Action{Kind: ActShutdown}) {
			t.Fatalf("%s: «Выключить»: %+v", name, a)
		}
	}

	m2 := locked(timeUpDecision())
	m2.Press(t0, ButtonShutdown, "")
	m2.Press(t0, ButtonCancel, "")
	if v := m2.View(t0); v.Panel != PanelNone || v.PanelText != "" || len(v.Buttons) != testScreenButtons {
		t.Fatalf("после «Отмена»: %+v", v)
	}
	if a := m2.Press(t0, ButtonConfirmShutdown, ""); a.Kind != ActNone {
		t.Fatalf("«Выключить» сработала без вопроса: %+v", a)
	}
}

func TestLogoffPanelIgnoresShutdownConfirm(t *testing.T) {
	// Ловит: на панели выхода «Выключить» выключает компьютер (панели перепутаны).
	m := locked(timeUpDecision())
	m.Press(t0, ButtonLogoff, "")
	if a := m.Press(t0, ButtonConfirmShutdown, ""); a.Kind != ActNone {
		t.Fatalf("на панели выхода сработало выключение: %+v", a)
	}
	if a := m.Press(t0, ButtonConfirmLogoff, ""); a != (Action{Kind: ActLogoff}) {
		t.Fatalf("выход перестал работать: %+v", a)
	}
}

func TestSwitchUserHasNoConfirmation(t *testing.T) {
	// Ловит: у «Смены пользователя» появилось подтверждение (ТЗ 8.2.3 п.9, D-40: данные
	// сохраняются, спрашивать нечего); панель открылась; кнопки экрана пропали после нажатия
	// (сеанс остаётся, ребёнок вернётся к тому же экрану).
	cases := map[string]struct {
		d    core.Decision
		want int
	}{"время": {timeUpDecision(), testScreenButtons}, "обход": {bypassDecision(), testBypassButtons}}
	for name, c := range cases {
		m := locked(c.d)
		if a := m.Press(t0, ButtonSwitchUser, ""); a != (Action{Kind: ActSwitchUser}) {
			t.Fatalf("%s: нажатие «Смена пользователя»: %+v", name, a)
		}
		v := m.View(t0)
		if v.Panel != PanelNone || v.PanelText != "" || len(v.Buttons) != c.want || !v.LockShown {
			t.Fatalf("%s: после смены пользователя вид изменился: %+v", name, v)
		}
	}
}

func TestExitsAreAlwaysEnabledAndNeverNeedPassword(t *testing.T) {
	// Ловит: выход закрыт паузой пароля или недоступен с пояснением (они не дают доступа ко
	// времени, ТЗ 8.2.3 п.3); выход пропадает, когда пароль в паузе после серии ошибок.
	m := withPasswordPanel(t, timeUpDecision())
	m.OnPasswordResult(t0, PasswordResult{BlockedUntil: at(time.Minute)})
	m.Press(at(time.Second), ButtonCancel, "")
	v := m.View(at(time.Second))
	got := map[Button]ButtonView{}
	for _, b := range v.Buttons {
		got[b.Button] = b
	}
	for _, b := range []Button{ButtonShutdown, ButtonLogoff, ButtonSwitchUser} {
		if bv, ok := got[b]; !ok || !bv.Enabled || bv.Hint != "" {
			t.Errorf("кнопка %d во время паузы пароля: %+v, есть=%v", b, bv, ok)
		}
	}
	if a := m.Press(at(time.Second), ButtonSwitchUser, ""); a.Kind != ActSwitchUser {
		t.Fatalf("смена пользователя во время паузы пароля: %+v", a)
	}
}

func TestTargetsOfNewButtons(t *testing.T) {
	// Ловит: у новых кнопок нет имён для канала связи (приёмка не сможет их нажать) или имена
	// разошлись с ipc.Button*.
	if TargetName(ButtonShutdown) != ipc.ButtonShutdown || TargetName(ButtonSwitchUser) != ipc.ButtonSwitchUser {
		t.Fatalf("имена: %q, %q", TargetName(ButtonShutdown), TargetName(ButtonSwitchUser))
	}
	if ipc.ButtonShutdown != "shutdown" || ipc.ButtonSwitchUser != "switchUser" {
		t.Fatalf("имена в канале связи изменились: %q, %q", ipc.ButtonShutdown, ipc.ButtonSwitchUser)
	}
	if b, ok := ButtonByTarget("confirmShutdown"); !ok || b != ButtonConfirmShutdown {
		t.Fatalf("confirmShutdown -> %d, %v", b, ok)
	}
}

// --- автоматическая смена пользователя через 15 минут -------------------------------------------------

// aliveLocked показывает экран блокировки с t0 и держит связь живой решениями каждые 5 с
// до момента to (включительно, если он кратен 5 с).
func aliveLocked(d core.Decision, to time.Duration) *Model {
	m := locked(d)
	for s := 5 * time.Second; s <= to; s += 5 * time.Second {
		m.OnDecision(at(s), d, false, nil)
	}
	return m
}

func TestAutoSwitchAfterFifteenMinutes(t *testing.T) {
	// Ловит: порог не 15 минут (раньше или позже, ровно 15 минут считается); отсчёт не от
	// появления экрана; второй вызов в тот же момент даёт второе действие.
	m := aliveLocked(timeUpDecision(), testAutoSwitch-5*time.Second)
	if a := m.AutoSwitch(at(testAutoSwitch - 5*time.Second)); a.Kind != ActNone {
		t.Fatalf("за 5 с до срока: %+v", a)
	}
	if a := m.AutoSwitch(at(testAutoSwitch - time.Millisecond)); a.Kind != ActNone {
		t.Fatalf("за 1 мс до срока: %+v", a)
	}
	m.OnDecision(at(testAutoSwitch), timeUpDecision(), false, nil)
	if a := m.AutoSwitch(at(testAutoSwitch)); a != (Action{Kind: ActSwitchUser}) {
		t.Fatalf("ровно через 15 минут: %+v", a)
	}
	if a := m.AutoSwitch(at(testAutoSwitch)); a.Kind != ActNone {
		t.Fatalf("второе действие в тот же момент: %+v", a)
	}
	if a := m.AutoSwitch(at(testAutoSwitch + time.Second)); a.Kind != ActNone {
		t.Fatalf("второе действие через секунду: %+v", a)
	}
}

func TestAutoSwitchCountsFromScreenAppearance(t *testing.T) {
	// Ловит: отсчёт идёт от запуска стража или от первого решения вместо появления экрана:
	// сначала 40 минут экрана нет, потом экран появился.
	m := NewModel()
	shownAt := 40 * time.Minute
	for s := time.Duration(0); s < shownAt; s += 5 * time.Second {
		m.OnDecision(at(s), runningDecision(3000), false, nil)
		if a := m.AutoSwitch(at(s)); a.Kind != ActNone {
			t.Fatalf("смена без экрана блокировки, через %v", s)
		}
	}
	var firedAt time.Duration = -1
	for s := shownAt; s <= shownAt+testAutoSwitch; s += 5 * time.Second {
		m.OnDecision(at(s), timeUpDecision(), false, nil)
		if a := m.AutoSwitch(at(s)); a.Kind == ActSwitchUser && firedAt < 0 {
			firedAt = s - shownAt
		}
	}
	if firedAt != testAutoSwitch {
		t.Fatalf("смена через %v после появления экрана, ждали %v", firedAt, testAutoSwitch)
	}
}

func TestAutoSwitchFiresOncePerPeriod(t *testing.T) {
	// Ловит: смена повторяется каждую секунду после срока (сеанс отключается снова и снова);
	// следующая смена приходит раньше чем через 15 минут после предыдущей. Если ребёнок
	// вернулся к тому же экрану, через 15 минут будет следующая смена.
	d := timeUpDecision()
	m := aliveLocked(d, testAutoSwitch)
	if a := m.AutoSwitch(at(testAutoSwitch)); a.Kind != ActSwitchUser {
		t.Fatalf("первая смена: %+v", a)
	}
	n := 0
	for s := testAutoSwitch + 5*time.Second; s < 2*testAutoSwitch; s += 5 * time.Second {
		m.OnDecision(at(s), d, false, nil)
		if a := m.AutoSwitch(at(s)); a.Kind != ActNone {
			n++
		}
	}
	if n != 0 {
		t.Fatalf("за следующие 15 минут лишних смен: %d", n)
	}
	m.OnDecision(at(2*testAutoSwitch), d, false, nil)
	if a := m.AutoSwitch(at(2 * testAutoSwitch)); a.Kind != ActSwitchUser {
		t.Fatalf("вторая смена через 15 минут после первой: %+v", a)
	}
}

func TestAutoSwitchSkippedOnLateCall(t *testing.T) {
	// Ловит: если вызов опоздал (стража не опрашивали), копится очередь смен: одно действие
	// на вызов, следующее не раньше чем через 15 минут после этого.
	m := aliveLocked(timeUpDecision(), testAutoSwitch+time.Minute)
	late := testAutoSwitch + time.Minute
	if a := m.AutoSwitch(at(late)); a.Kind != ActSwitchUser {
		t.Fatalf("опоздавший вызов: %+v", a)
	}
	if a := m.AutoSwitch(at(late + testAutoSwitch - 5*time.Second)); a.Kind != ActNone {
		t.Fatalf("следующая смена раньше срока: %+v", a)
	}
}

func TestAutoSwitchNotWhenPasswordEnteredBeforeScreenGone(t *testing.T) {
	// Ловит: смена пользователя после того, как родитель снял блокировку (доп. время): экрана
	// уже нет, а таймер идёт.
	d := timeUpDecision()
	m := aliveLocked(d, 10*time.Minute)
	for s := 10*time.Minute + 5*time.Second; s <= testAutoSwitch+time.Minute; s += 5 * time.Second {
		m.OnDecision(at(s), runningDecision(900), false, nil)
		if a := m.AutoSwitch(at(s)); a.Kind != ActNone {
			t.Fatalf("смена без экрана блокировки, через %v: %+v", s, a)
		}
	}
}

func TestAutoSwitchRestartsWhenScreenReturns(t *testing.T) {
	// Ловит: старый отсчёт переживает снятие экрана: экран вернулся через минуту, а смена
	// приходит по прежнему таймеру. Отсчёт заново от нового появления.
	d := timeUpDecision()
	m := aliveLocked(d, 10*time.Minute)
	gone := 10*time.Minute + 5*time.Second
	m.OnDecision(at(gone), runningDecision(900), false, nil)
	back := gone + time.Minute
	m.OnDecision(at(back), d, false, nil)
	for s := back + 5*time.Second; s < back+testAutoSwitch; s += 5 * time.Second {
		m.OnDecision(at(s), d, false, nil)
		if a := m.AutoSwitch(at(s)); a.Kind != ActNone {
			t.Fatalf("смена через %v после нового появления экрана", s-back)
		}
	}
	m.OnDecision(at(back+testAutoSwitch), d, false, nil)
	if a := m.AutoSwitch(at(back + testAutoSwitch)); a.Kind != ActSwitchUser {
		t.Fatalf("смена через 15 минут после нового появления: %+v", a)
	}
}

func TestAutoSwitchRestartsAfterParentSession(t *testing.T) {
	// Ловит: пока родитель в окне настроек (экран спрятан, Ruling 19), таймер идёт и ребёнка
	// выбрасывает из сеанса посреди работы родителя.
	d := timeUpDecision()
	m := aliveLocked(d, 10*time.Minute)
	m.OnDecision(at(10*time.Minute+5*time.Second), d, true, nil)
	for s := 10*time.Minute + 10*time.Second; s <= testAutoSwitch+2*time.Minute; s += 5 * time.Second {
		m.OnDecision(at(s), d, true, nil)
		if a := m.AutoSwitch(at(s)); a.Kind != ActNone {
			t.Fatalf("смена в родительской сессии, через %v: %+v", s, a)
		}
	}
}

func TestAutoSwitchNotWithoutLink(t *testing.T) {
	// Ловит: смена по таймеру, когда службы нет дольше 10 с и экран снят (Ruling 15): страж
	// без связи ничего не делает; обрыв связи сбрасывает отсчёт, по возвращении связи он идёт заново.
	d := timeUpDecision()
	m := aliveLocked(d, 5*time.Second)
	if a := m.AutoSwitch(at(testAutoSwitch + time.Minute)); a.Kind != ActNone {
		t.Fatalf("смена без связи со службой: %+v", a)
	}

	// Обрыв на 11 с в середине срока: экран снимается, отсчёт начинается заново.
	m2 := aliveLocked(d, 7*time.Minute)
	resume := 7*time.Minute + 16*time.Second
	for s := resume; s <= resume+testAutoSwitch; s += 5 * time.Second {
		m2.OnDecision(at(s), d, false, nil)
		a := m2.AutoSwitch(at(s))
		if s < resume+testAutoSwitch && a.Kind != ActNone {
			t.Fatalf("отсчёт пережил обрыв связи: смена через %v после возвращения связи", s-resume)
		}
		if s == resume+testAutoSwitch && a.Kind != ActSwitchUser {
			t.Fatalf("нет смены через 15 минут после возвращения связи: %+v", a)
		}
	}
}

func TestAutoSwitchNoScreenNoAction(t *testing.T) {
	// Ловит: смена, когда нет решения или блокировки.
	if a := NewModel().AutoSwitch(at(time.Hour)); a.Kind != ActNone {
		t.Fatalf("без решения: %+v", a)
	}
	m := NewModel()
	for s := time.Duration(0); s <= time.Hour; s += 5 * time.Second {
		m.OnDecision(at(s), runningDecision(1800), false, nil)
		if a := m.AutoSwitch(at(s)); a.Kind != ActNone {
			t.Fatalf("без блокировки через %v: %+v", s, a)
		}
	}
}

func TestAutoSwitchIsNotExtendedByChild(t *testing.T) {
	// Ловит: действия ребёнка на экране продлевают отсчёт (ТЗ 8.2.3 п.10): нажатия кнопок,
	// панель пароля, неверные пароли, смена вида блокировки.
	d := timeUpDecision()
	m := locked(d)
	step := 5 * time.Second
	for s := step; s < testAutoSwitch; s += step {
		m.OnDecision(at(s), d, false, nil)
		switch s {
		case 2 * time.Minute:
			m.Press(at(s), ButtonParentPassword, "")
		case 4 * time.Minute:
			m.Press(at(s), ButtonSubmitPassword, "не тот")
			m.OnPasswordResult(at(s), PasswordResult{AttemptsLeft: 4})
		case 6 * time.Minute:
			m.Press(at(s), ButtonCancel, "")
			m.Press(at(s), ButtonLogoff, "")
		case 8 * time.Minute:
			m.Press(at(s), ButtonCancel, "")
			m.Press(at(s), ButtonShutdown, "")
		case 10 * time.Minute:
			m.Press(at(s), ButtonCancel, "")
		case 12 * time.Minute:
			m.OnDecision(at(s), bypassDecision(), false, nil)
		}
		if a := m.AutoSwitch(at(s)); a.Kind != ActNone {
			t.Fatalf("смена раньше срока, через %v", s)
		}
	}
	m.OnDecision(at(testAutoSwitch), bypassDecision(), false, nil)
	if a := m.AutoSwitch(at(testAutoSwitch)); a.Kind != ActSwitchUser {
		t.Fatalf("смена через 15 минут после появления экрана: %+v", a)
	}
}

func TestAutoSwitchWaitsAfterCorrectPassword(t *testing.T) {
	// Ловит: родитель ввёл верный пароль, а ребёнка отключают посреди выбора действий
	// (ТЗ 8.2.3 п.10: «пароль не введён»); после верного пароля у родителя ещё 15 минут.
	d := timeUpDecision()
	m := aliveLocked(d, 14*time.Minute)
	m.Press(at(14*time.Minute), ButtonParentPassword, "")
	m.Press(at(14*time.Minute), ButtonSubmitPassword, "верный")
	m.OnPasswordResult(at(14*time.Minute), PasswordResult{OK: true})
	for s := 14*time.Minute + 5*time.Second; s < 14*time.Minute+testAutoSwitch; s += 5 * time.Second {
		m.OnDecision(at(s), d, false, nil)
		if a := m.AutoSwitch(at(s)); a.Kind != ActNone {
			t.Fatalf("смена через %v после верного пароля", s-14*time.Minute)
		}
	}
	end := 14*time.Minute + testAutoSwitch
	m.OnDecision(at(end), d, false, nil)
	if a := m.AutoSwitch(at(end)); a.Kind != ActSwitchUser {
		t.Fatalf("смена через 15 минут после верного пароля: %+v", a)
	}
}

func TestAutoSwitchClosesPanelsAndClearsPassword(t *testing.T) {
	// Ловит: после автосмены в оставленном сеансе висит открытая панель с набранным паролем
	// родителя; по возвращении ребёнок увидел бы его в поле.
	d := timeUpDecision()
	m := aliveLocked(d, testAutoSwitch)
	m.Press(at(testAutoSwitch), ButtonParentPassword, "")
	before := m.View(at(testAutoSwitch)).PasswordClearSeq
	if a := m.AutoSwitch(at(testAutoSwitch)); a.Kind != ActSwitchUser {
		t.Fatalf("смена: %+v", a)
	}
	v := m.View(at(testAutoSwitch))
	if v.Panel != PanelNone || len(v.Buttons) != testScreenButtons || v.PasswordClearSeq <= before {
		t.Fatalf("после автосмены: Panel=%v Buttons=%d Seq=%d (было %d)", v.Panel, len(v.Buttons), v.PasswordClearSeq, before)
	}
}

func TestAutoSwitchAfterIsConfigurable(t *testing.T) {
	// Ловит: тестовый режим не может сократить срок (приёмка ждала бы 15 минут настоящих часов);
	// ноль, отрицательное значение и срок длиннее 15 минут меняют настоящий срок.
	d := timeUpDecision()
	m := NewModel()
	m.SetAutoSwitchAfter(20 * time.Second)
	m.OnDecision(t0, d, false, nil)
	for s := 5 * time.Second; s < 20*time.Second; s += 5 * time.Second {
		m.OnDecision(at(s), d, false, nil)
		if a := m.AutoSwitch(at(s)); a.Kind != ActNone {
			t.Fatalf("смена через %v при сроке 20 с", s)
		}
	}
	m.OnDecision(at(20*time.Second), d, false, nil)
	if a := m.AutoSwitch(at(20 * time.Second)); a.Kind != ActSwitchUser {
		t.Fatalf("смена через 20 с: %+v", a)
	}
	for _, bad := range []time.Duration{0, -time.Second, testAutoSwitch + time.Second, 24 * time.Hour} {
		m3 := NewModel()
		m3.SetAutoSwitchAfter(bad)
		m3.OnDecision(t0, d, false, nil)
		for s := 5 * time.Second; s < testAutoSwitch; s += 5 * time.Second {
			m3.OnDecision(at(s), d, false, nil)
			if a := m3.AutoSwitch(at(s)); a.Kind != ActNone {
				t.Fatalf("срок %v: смена через %v", bad, s)
			}
		}
		m3.OnDecision(at(testAutoSwitch), d, false, nil)
		if a := m3.AutoSwitch(at(testAutoSwitch)); a.Kind != ActSwitchUser {
			t.Fatalf("срок %v: нет смены через 15 минут: %+v", bad, a)
		}
	}
}
