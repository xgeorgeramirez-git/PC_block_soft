package service

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// week собирает семь дней одного лимита.
func week(min int) []ipc.WeekdayLimit {
	out := make([]ipc.WeekdayLimit, 0, 7)
	for d := 1; d <= 7; d++ {
		out = append(out, ipc.WeekdayLimit{Weekday: d, TotalMin: min})
	}
	return out
}

func setupReq(pw string, sids ...string) ipc.SetupRequest {
	req := ipc.SetupRequest{Password: pw}
	for _, sid := range sids {
		req.Children = append(req.Children, ipc.SetupChild{SID: sid, Name: "", Limits: week(120)})
	}
	return req
}

// --- первоначальная настройка (FR-2) ------------------------------------------------------

// Ловит: мастер не задаёт пароль или не создаёт детей с лимитами, не ставит запрет Диспетчера
// задач; настроенная программа после этого не принимает вход паролем.
func TestSetupCreatesPasswordChildrenAndPolicy(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.ok(elevatedPeer(), "", ipc.CmdSetup, setupReq("секрет-123", childSID), nil)
	var hello ipc.HelloReply
	h.ok(adminPeer(), "", ipc.CmdHello, nil, &hello)
	if !hello.Configured {
		t.Fatal("после настройки Hello говорит «не настроена»")
	}
	id := h.childID(childSID)
	if lim, _ := h.store.GetDayLimits(id); lim[0].TotalMin == nil || *lim[0].TotalMin != 120 {
		t.Fatalf("лимиты не записаны: %+v", lim)
	}
	if !h.policy.disabled(childSID) {
		t.Fatal("запрет Диспетчера задач не поставлен")
	}
	if n := len(h.store.eventsOf("setup_done")); n != 1 {
		t.Fatalf("событий setup_done %d", n)
	}
	h.login(adminPeer())
	// имя по умолчанию берётся из учётки
	cs, _ := h.store.ListChildren()
	if cs[0].Name != "Миша" {
		t.Fatalf("имя ребёнка %q", cs[0].Name)
	}
}

// Ловит: ребёнок, добавленный мастером, не получает ядро: учёт и ограничения не действуют до
// перезапуска службы.
func TestSetupChildIsAccountedWithoutRestart(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.ok(elevatedPeer(), "", ipc.CmdSetup, setupReq("секрет-123", childSID), nil)
	h.childLogsIn()
	h.run(5, activeSample)
	if got := h.usedS(); got != 5 {
		t.Fatalf("учтено %d с", got)
	}
}

// Ловит: мастер принимает короткий пароль, пустой список детей, администратора и чужую
// учётку, а ошибка не привязана к нужному полю (L-121).
func TestSetupValidationErrorsAreBoundToFields(t *testing.T) {
	cases := []struct {
		name  string
		req   ipc.SetupRequest
		field string
	}{
		{"короткий пароль", setupReq("123", childSID), ipc.FieldPassword},
		{"нет детей", setupReq("секрет-123"), ipc.FieldChildren},
		{"администратор", setupReq("секрет-123", parentSID), ipc.FieldChildren},
		{"нет такой учётки", setupReq("секрет-123", "S-1-5-21-9-9-9-9"), ipc.FieldChildren},
		{"лимит больше суток", func() ipc.SetupRequest {
			r := setupReq("секрет-123", childSID)
			r.Children[0].Limits[2].TotalMin = 2000
			return r
		}(), ipc.LimitField(3)},
		{"не все дни", func() ipc.SetupRequest {
			r := setupReq("секрет-123", childSID)
			r.Children[0].Limits = r.Children[0].Limits[:6]
			return r
		}(), "limits"},
	}
	for _, c := range cases {
		h := newHarness(t)
		h.start()
		rep := h.fail(elevatedPeer(), "", ipc.CmdSetup, c.req)
		if rep.Field != c.field || rep.Error == "" {
			t.Errorf("%s: поле %q (ждали %q), текст %q", c.name, rep.Field, c.field, rep.Error)
		}
		if h.svc.configured() {
			t.Errorf("%s: неудачная настройка оставила программу настроенной", c.name)
		}
	}
}

// Ловит: A2 нарушен в тестовом режиме для чужой учётки или не снят для учётки раннера
// (ТЗ 10.8: запрет снят только для TestChildSID).
func TestSetupAdminAllowedOnlyForTestChild(t *testing.T) {
	h := newHarness(t, withTestMode(TestMode{ClockSpeed: 1, TestChildSID: parentSID}))
	h.start()
	h.ok(elevatedPeer(), "", ipc.CmdSetup, setupReq("секрет-123", parentSID), nil)

	h2 := newHarness(t, withTestMode(TestMode{ClockSpeed: 1, TestChildSID: childSID}))
	h2.start()
	rep := h2.fail(elevatedPeer(), "", ipc.CmdSetup, setupReq("секрет-123", parentSID))
	if rep.Field != ipc.FieldChildren {
		t.Fatalf("администратор не раннера принят: %+v", rep)
	}
}

// Ловит: повторная настройка перезаписывает пароль родителя из процесса администратора
// (угон пароля): после первой настройки Setup недоступен.
func TestSetupOnlyOnce(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.ok(elevatedPeer(), "", ipc.CmdSetup, setupReq("секрет-123", childSID), nil)
	if rep := h.fail(elevatedPeer(), "", ipc.CmdSetup, setupReq("другой-пароль", childSID)); rep.Error != textAlreadySetup {
		t.Fatalf("ответ %q", rep.Error)
	}
	h.login(adminPeer())
}

// Ловит: оборвавшаяся настройка (ошибка записи детей) оставляет программу «настроенной» без
// детей: после сбоя мастер должен пройти заново.
func TestSetupFailureLeavesProgramUnconfigured(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.store.failLimits = false
	h.cfg.Accounts = h.accounts
	badStore := &failingSetStore{fakeStore: h.store}
	h2 := newHarness(t, func(x *harness) { x.cfg.Store = badStore; x.cfg.Journal = nil })
	h2.start()
	rep := h2.fail(elevatedPeer(), "", ipc.CmdSetup, setupReq("секрет-123", childSID))
	if rep.Error != textSaveFailed {
		t.Fatalf("ответ %q", rep.Error)
	}
	if h2.svc.configured() {
		t.Fatal("пароль записан, хотя дети не записались")
	}
}

type failingSetStore struct{ *fakeStore }

func (f *failingSetStore) SetDayLimits(int64, [7]core.DayLimit) error { return errFake }

// --- сброс пароля, смена пароля, удаление (R19, R20, FR-4) ---------------------------------

// Ловит: сброс не меняет пароль, оставляет старый рабочим или оставляет открытые сессии
// родителя (после сброса прежние токены должны умереть).
func TestResetPasswordReplacesPasswordAndEndsSessions(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	tok := h.login(adminPeer())
	h.ok(elevatedPeer(), "", ipc.CmdResetPassword, ipc.ResetPasswordRequest{NewPassword: "новый-секрет"}, nil)
	h.fail(adminPeer(), tok, ipc.CmdGetToday, nil)
	h.fail(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: goodPassword})
	h.ok(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: "новый-секрет"}, nil)
	if n := len(h.store.eventsOf("password_reset")); n != 1 {
		t.Fatalf("событий сброса %d", n)
	}
}

// Ловит: сброс принимает короткий пароль; сброс обнуляет счётчик неверных вводов и паузу
// (сброс снимает блокировку ввода: пароль новый).
func TestResetPasswordValidatesAndClearsPause(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	if rep := h.fail(elevatedPeer(), "", ipc.CmdResetPassword, ipc.ResetPasswordRequest{NewPassword: "123"}); rep.Field != ipc.FieldNewPassword {
		t.Fatalf("ответ %+v", rep)
	}
	for i := 0; i < 5; i++ {
		h.wrongLogin(adminPeer())
	}
	h.ok(elevatedPeer(), "", ipc.CmdResetPassword, ipc.ResetPasswordRequest{NewPassword: "новый-секрет"}, nil)
	h.ok(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: "новый-секрет"}, nil)
}

// Ловит: сброс до первой настройки создаёт пароль без детей, обходя мастер.
func TestResetPasswordBeforeSetupRefused(t *testing.T) {
	h := newHarness(t)
	h.start()
	rep := h.fail(elevatedPeer(), "", ipc.CmdResetPassword, ipc.ResetPasswordRequest{NewPassword: "новый-секрет"})
	if rep.Error != textNotConfigured {
		t.Fatalf("ответ %q", rep.Error)
	}
}

// Ловит: смена пароля не проверяет старый (или проверяет без подсчёта попыток), ошибки не
// привязаны к полям «старый» и «новый» (L-121).
func TestChangePassword(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	tok := h.login(adminPeer())
	rep := h.fail(adminPeer(), tok, ipc.CmdChangePassword, ipc.ChangePasswordRequest{OldPassword: "не тот", NewPassword: "новый-секрет"})
	if rep.Field != ipc.FieldOldPassword || deniedOf(t, rep).AttemptsLeft != 4 {
		t.Fatalf("неверный старый: %+v", rep)
	}
	rep = h.fail(adminPeer(), tok, ipc.CmdChangePassword, ipc.ChangePasswordRequest{OldPassword: goodPassword, NewPassword: "123"})
	if rep.Field != ipc.FieldNewPassword {
		t.Fatalf("короткий новый: %+v", rep)
	}
	h.ok(adminPeer(), tok, ipc.CmdChangePassword, ipc.ChangePasswordRequest{OldPassword: goodPassword, NewPassword: "новый-секрет"}, nil)
	h.fail(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: goodPassword})
	h.ok(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: "новый-секрет"}, nil)
	if n := len(h.store.eventsOf("password_changed")); n != 1 {
		t.Fatalf("событий смены пароля %d", n)
	}
}

// Ловит: удаление до настройки требует пароль (FR-4: программа не настроена, пароль не
// спрашивается), или после настройки пускает без пароля и без «забыл»; неверный пароль не
// считается попыткой (R19).
func TestUninstallCheck(t *testing.T) {
	fresh := newHarness(t)
	fresh.start()
	var r ipc.UninstallCheckReply
	fresh.ok(elevatedPeer(), "", ipc.CmdUninstallCheck, ipc.UninstallCheckRequest{}, &r)
	if !r.Allowed {
		t.Fatal("до настройки удаление запрещено")
	}

	h := newHarness(t, configured())
	h.start()
	if rep := h.fail(elevatedPeer(), "", ipc.CmdUninstallCheck, ipc.UninstallCheckRequest{Password: "не тот"}); deniedOf(t, rep).AttemptsLeft != 4 {
		t.Fatalf("неверный пароль: %+v", rep)
	}
	h.fail(elevatedPeer(), "", ipc.CmdUninstallCheck, ipc.UninstallCheckRequest{})
	h.ok(elevatedPeer(), "", ipc.CmdUninstallCheck, ipc.UninstallCheckRequest{Password: goodPassword}, &r)
	if !r.Allowed {
		t.Fatal("верный пароль не разрешил удаление")
	}
	h.ok(elevatedPeer(), "", ipc.CmdUninstallCheck, ipc.UninstallCheckRequest{Forgot: true}, &r)
	if !r.Allowed || len(h.store.eventsOf("uninstall_forgot")) != 1 {
		t.Fatalf("«забыл пароль»: %+v", r)
	}
}

// --- лимиты (FR-16, FR-19) -----------------------------------------------------------------

// Ловит: FR-19: сохранённые лимиты видны стражу не в ближайшем опросе, а позже: ребёнок
// остаётся под экраном блокировки, хотя родитель уже добавил время.
func TestSaveLimitsAppliesInNextPoll(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 1), configured())
	h.start()
	h.childLogsIn()
	h.run(70, activeSample)
	if rep := h.poll(activeSample); rep.Status.Lock != ipc.LockTimeUp {
		t.Fatalf("блокировки нет до сохранения: %q", rep.Status.Lock)
	}
	tok := h.login(adminPeer())
	var sr ipc.SaveLimitsReply
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: h.childID(childSID), Limits: week(120)}, &sr)
	if sr.Message != textSaved || sr.Card.Status.Lock != ipc.LockNone {
		t.Fatalf("ответ SaveLimits: %+v", sr)
	}
	if rep := h.poll(activeSample); rep.Status.Lock != ipc.LockNone || rep.Status.Status == ipc.StatusTimeUp {
		t.Fatalf("страж в следующем опросе: %+v", rep.Status)
	}
	if n := len(h.store.eventsOf("settings_changed")); n != 1 {
		t.Fatalf("событий изменения настроек %d", n)
	}
}

// Ловит: сохранение уменьшенного лимита не ставит блокировку сразу.
func TestSaveLimitsLowerLimitLocksImmediately(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	h.run(120, activeSample)
	tok := h.login(adminPeer())
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: h.childID(childSID), Limits: week(1)}, nil)
	if rep := h.poll(activeSample); rep.Status.Lock != ipc.LockTimeUp {
		t.Fatalf("блокировки нет: %+v", rep.Status)
	}
}

// Ловит: ошибки лимитов не привязаны к полю дня (L-121: limits.3), неполная неделя принимается;
// неверные данные меняют уже сохранённое.
func TestSaveLimitsValidation(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	bad := week(120)
	bad[2].TotalMin = 2000
	rep := h.fail(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: bad})
	if rep.Field != "limits.3" || rep.Error != "Не больше 24 ч" {
		t.Fatalf("ответ %+v", rep)
	}
	bad[2].TotalMin = -5
	if rep := h.fail(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: bad}); rep.Field != "limits.3" {
		t.Fatalf("отрицательный лимит: %+v", rep)
	}
	if rep := h.fail(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: week(120)[:3]}); rep.Field != "limits" {
		t.Fatalf("неполная неделя: %+v", rep)
	}
	dup := week(120)
	dup[1].Weekday = 1
	h.fail(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: dup})
	if rep := h.fail(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: 777, Limits: week(120)}); rep.Error != textNoSuchChild {
		t.Fatalf("чужой ребёнок: %q", rep.Error)
	}
	var cr ipc.ChildRules
	h.ok(adminPeer(), tok, ipc.CmdGetChildRules, ipc.ChildRequest{ChildID: id}, &cr)
	if cr.Limits[2].TotalMin != 120 {
		t.Fatalf("после неудачных сохранений лимит изменился: %+v", cr.Limits[2])
	}
}

// Ловит: «Без лимита» и 0 минут путаются (R4: 0 это «нельзя в этот день»), массив лимитов
// теряет порядок дней.
func TestSaveLimitsUnlimitedAndZero(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	l := week(120)
	l[0] = ipc.WeekdayLimit{Weekday: 1, TotalMin: 0}                   // понедельник: нельзя
	l[1] = ipc.WeekdayLimit{Weekday: 2, TotalMin: 77, Unlimited: true} // вторник: без лимита
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: h.childID(childSID), Limits: l}, nil)
	var cr ipc.ChildRules
	h.ok(adminPeer(), tok, ipc.CmdGetChildRules, ipc.ChildRequest{ChildID: h.childID(childSID)}, &cr)
	if cr.Limits[0].Unlimited || cr.Limits[0].TotalMin != 0 || !cr.Limits[1].Unlimited || cr.Limits[1].TotalMin != 0 || len(cr.Limits) != 7 {
		t.Fatalf("недельные лимиты: %+v", cr.Limits)
	}
	if st := h.status(); st.Status != ipc.StatusTimeUp {
		t.Fatalf("понедельник с лимитом 0: статус %q", st.Status)
	}
}

// Ловит: ошибка записи лимитов всё равно применяет их в памяти (родитель увидит одно, после
// перезапуска будет другое).
func TestSaveLimitsStoreFailureKeepsOldRules(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	bad := &limitsFailStore{fakeStore: h.store}
	h.svc.cfg.Store = bad
	rep := h.fail(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: h.childID(childSID), Limits: week(1)})
	if rep.Error != textSaveFailed {
		t.Fatalf("ответ %q", rep.Error)
	}
	h.run(100, activeSample)
	if st := h.status(); st.Lock != ipc.LockNone {
		t.Fatalf("правила применились без записи: %+v", st)
	}
}

type limitsFailStore struct{ *fakeStore }

func (f *limitsFailStore) SetDayLimits(int64, [7]core.DayLimit) error { return errFake }

// --- доп. время и блокировки (R10, R17) --------------------------------------------------------

// Ловит: доп. время не увеличивает лимит и не снимает блокировку «время вышло» сразу;
// границы 1…240 не проверяются; ошибка не привязана к полю minutes.
func TestAddExtra(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 1), configured())
	h.start()
	h.childLogsIn()
	h.run(70, activeSample)
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	for _, m := range []int{0, 241, -5} {
		rep := h.fail(adminPeer(), tok, ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: id, Minutes: m})
		if rep.Field != ipc.FieldMinutes || rep.Error != "Можно добавить от 1 до 240 минут" {
			t.Fatalf("%d минут: %+v", m, rep)
		}
	}
	var ar ipc.AddExtraReply
	h.ok(adminPeer(), tok, ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: id, Minutes: 30}, &ar)
	if ar.Message != "Добавлено 30 минут" || ar.Card.ExtraMin != 30 || ar.Card.Status.LimitS != 31*60 || ar.Card.Status.Lock != ipc.LockNone {
		t.Fatalf("ответ: %+v", ar)
	}
	if want := h.real.Now().Add(10 * time.Second).UnixMilli(); ar.UndoUntilMs != want || ar.ExtraID == "" {
		t.Fatalf("отмена: %d (ждали %d), id %q", ar.UndoUntilMs, want, ar.ExtraID)
	}
	if evs := h.store.eventsOf("extra_time"); len(evs) != 1 || detInt(evs[0].Details, "minutes") != 30 {
		t.Fatalf("события: %+v", evs)
	}
	if got, _ := h.store.lastSavedTotal(id); got == 0 {
		t.Fatal("доп. время не сохранено в базе сразу")
	}
	if st, ok := h.store.days[dayKey(id, "2026-10-19")]; !ok || st.ExtraMin != 30 {
		t.Fatalf("сохранённый день: %+v", st)
	}
}

// Ловит: плашка «Добавлено N минут» не доходит до стража, когда родитель добавил время из окна;
// когда время добавил сам страж по паролю, плашку страж показывает сам и дублей нет.
func TestAddExtraNoticeGoesToGuard(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	h.ok(adminPeer(), tok, ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: h.childID(childSID), Minutes: 1}, nil)
	rep := h.poll(activeSample)
	if len(rep.Commands) != 1 || rep.Commands[0].Kind != ipc.GuardCmdNotice || rep.Commands[0].Text != "Добавлено 1 минуту" {
		t.Fatalf("команды: %+v", rep.Commands)
	}
	if rep := h.poll(activeSample); len(rep.Commands) != 0 {
		t.Fatalf("команда повторена: %+v", rep.Commands)
	}
}

// Ловит: отмена работает после 10 с, чужим id, дважды, после смены суток; не возвращает
// блокировку и порог (R9 взводится заново).
func TestUndoExtra(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 1), configured())
	h.start()
	h.childLogsIn()
	h.run(70, activeSample)
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	var ar ipc.AddExtraReply
	h.ok(adminPeer(), tok, ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: id, Minutes: 30}, &ar)

	if rep := h.fail(adminPeer(), tok, ipc.CmdUndoExtra, ipc.UndoExtraRequest{ChildID: id, ExtraID: "чужой"}); rep.Error != textUndoGone {
		t.Fatalf("чужой id: %q", rep.Error)
	}
	var cr ipc.CardReply
	h.real.Advance(9 * time.Second)
	h.ok(adminPeer(), tok, ipc.CmdUndoExtra, ipc.UndoExtraRequest{ChildID: id, ExtraID: ar.ExtraID}, &cr)
	if cr.Card.ExtraMin != 0 || cr.Card.Status.Lock != ipc.LockTimeUp {
		t.Fatalf("после отмены: %+v", cr.Card)
	}
	if n := len(h.store.eventsOf("extra_undone")); n != 1 {
		t.Fatalf("событий отмены %d", n)
	}
	h.fail(adminPeer(), tok, ipc.CmdUndoExtra, ipc.UndoExtraRequest{ChildID: id, ExtraID: ar.ExtraID}) // второй раз

	h.ok(adminPeer(), tok, ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: id, Minutes: 5}, &ar)
	h.real.Advance(10 * time.Second)
	if rep := h.fail(adminPeer(), tok, ipc.CmdUndoExtra, ipc.UndoExtraRequest{ChildID: id, ExtraID: ar.ExtraID}); rep.Error != textUndoGone {
		t.Fatalf("через 10 с: %q", rep.Error)
	}
	if st, _ := h.store.LoadDayState(id, "2026-10-19"); st.ExtraMin != 5 {
		t.Fatalf("доп. время в базе %d, ждали 5", st.ExtraMin)
	}
}

// Ловит: отмена после смены суток снимает время не того дня.
func TestUndoExtraRefusedAfterMidnight(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.clock.Set(time.Date(2026, 10, 19, 23, 59, 58, 0, tzMSK))
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	var ar ipc.AddExtraReply
	h.ok(adminPeer(), tok, ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: id, Minutes: 15}, &ar)
	h.run(3, activeSample)
	h.fail(adminPeer(), tok, ipc.CmdUndoExtra, ipc.UndoExtraRequest{ChildID: id, ExtraID: ar.ExtraID})
}

// Ловит: родитель не может убавить доп. время, обнулить его, отменить убавление; убавление
// ниже суммарного лимита 0 не зажато (R10, D-43).
func TestReduceAndClearExtra(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	id := h.childID(childSID)

	h.ok(adminPeer(), tok, ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: id, Minutes: 60}, nil)
	var rr ipc.AddExtraReply
	h.ok(adminPeer(), tok, ipc.CmdReduceExtra, ipc.ReduceExtraRequest{ChildID: id, Minutes: 30}, &rr)
	if rr.Message != "Убрано 30 минут" || rr.Card.ExtraMin != 30 || rr.ExtraID == "" {
		t.Fatalf("после -30: %+v", rr)
	}

	// Отмена убавления возвращает к 60.
	var cr ipc.CardReply
	h.ok(adminPeer(), tok, ipc.CmdUndoExtra, ipc.UndoExtraRequest{ChildID: id, ExtraID: rr.ExtraID}, &cr)
	if cr.Card.ExtraMin != 60 {
		t.Fatalf("после отмены убавления доп. %d, want 60", cr.Card.ExtraMin)
	}

	// Обнулить доп. время.
	var clr ipc.AddExtraReply
	h.ok(adminPeer(), tok, ipc.CmdClearExtra, ipc.ClearExtraRequest{ChildID: id}, &clr)
	if clr.Card.ExtraMin != 0 || clr.Message != "Доп. время убрано" {
		t.Fatalf("после обнуления: %+v", clr)
	}

	// Убавление ниже 0 зажато: лимит 120, убрать 240 → доп. −120 (суммарный лимит 0).
	h.ok(adminPeer(), tok, ipc.CmdReduceExtra, ipc.ReduceExtraRequest{ChildID: id, Minutes: 240}, &rr)
	if rr.Card.ExtraMin != -120 {
		t.Fatalf("упор снизу: доп. %d, want -120", rr.Card.ExtraMin)
	}

	// Границы 1…240.
	if rep := h.fail(adminPeer(), tok, ipc.CmdReduceExtra, ipc.ReduceExtraRequest{ChildID: id, Minutes: 500}); rep.Field != ipc.FieldMinutes {
		t.Fatalf("убавление 500: поле %q, ошибка %q", rep.Field, rep.Error)
	}
}

// Ловит: переключатель «Подробные логи для диагностики» не пишет подробных строк при
// включении или продолжает писать после выключения (FR-25, D-42).
func TestVerboseLogsToggle(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())

	// По умолчанию выключено: команда не оставляет подробной строки.
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, nil)
	if strings.Contains(h.logText(), "[подробно]") {
		t.Fatalf("подробные логи при выключенном переключателе:\n%s", h.logText())
	}

	// Включаем — следующая команда пишет подробную строку.
	var d ipc.DiagReply
	h.ok(adminPeer(), tok, ipc.CmdSetDiag, ipc.SetDiagRequest{Verbose: true}, &d)
	if !d.Verbose {
		t.Fatalf("переключатель не включился: %+v", d)
	}
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, nil)
	if !strings.Contains(h.logText(), "[подробно] команда GetToday") {
		t.Fatalf("нет подробной строки при включённом переключателе:\n%s", h.logText())
	}

	// Выключаем — снова тихо; GetDiag читает состояние.
	h.ok(adminPeer(), tok, ipc.CmdSetDiag, ipc.SetDiagRequest{Verbose: false}, &d)
	before := h.logText()
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, nil)
	if h.logText() != before {
		t.Fatalf("после выключения всё ещё пишутся подробные строки")
	}
	h.ok(adminPeer(), tok, ipc.CmdGetDiag, nil, &d)
	if d.Verbose {
		t.Fatalf("GetDiag после выключения: %+v", d)
	}
	// D-48: путь к папке логов виден в настройках сразу, без нажатия «Открыть».
	if want := filepath.Join(h.dataDir, "logs"); d.LogsPath != want {
		t.Fatalf("GetDiag.LogsPath = %q, want %q", d.LogsPath, want)
	}
}

// Ловит: статистика родителя за сегодня не отдаёт данные выбранного ребёнка, не переключается
// между детьми или доступна не родителю (FR-23, D-44).
func TestParentStats(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 60), configured())
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	id, id2 := h.childID(childSID), h.childID(child2SID)

	var ms ipc.MyTodayReply
	h.ok(adminPeer(), tok, ipc.CmdParentStats, ipc.ParentStatsRequest{ChildID: id}, &ms)
	if ms.Status.Name != "Миша" || len(ms.Rules) != 7 || len(ms.Today) < 2 {
		t.Fatalf("статистика Миши: %+v", ms)
	}
	h.ok(adminPeer(), tok, ipc.CmdParentStats, ipc.ParentStatsRequest{ChildID: id2}, &ms)
	if ms.Status.Name != "Коля" {
		t.Fatalf("переключение на Колю: %+v", ms)
	}
	// Ребёнок не зовёт статистику родителя (у него своя «Моя статистика»).
	if rep := h.do(childPeer(), "", ipc.CmdParentStats, ipc.ParentStatsRequest{ChildID: id}); !isRightsDenial(rep) {
		t.Fatalf("ребёнок получил статистику родителя: %+v", rep)
	}
}

// Ловит: попытка обхода блокирует экран или предлагает «Снять блокировку» (D-41 — не должна):
// пишется подробное событие, экран не блокируется; повтор той же подробности в пределах
// минуты не плодит записи в журнале (страж шлёт сообщение каждую секунду).
func TestBypassJournaledNoLock(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvBypass, Detail: "Экран закрыли насильно"})
	tok := h.login(adminPeer())
	var today ipc.GetTodayReply
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, &today)
	c := today.Children[0]
	if c.CanClearBlock || c.Status.Status == ipc.StatusBypass || c.Status.Lock == ipc.LockBypass {
		t.Fatalf("карточка показывает блокировку за обход: %+v (D-41)", c)
	}
	if n := len(h.store.eventsOf("bypass")); n != 1 {
		t.Fatalf("событий обхода %d; want 1 подробное событие", n)
	}
	// Повтор той же подробности в пределах окна — без новой записи.
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvBypass, Detail: "Экран закрыли насильно"})
	if n := len(h.store.eventsOf("bypass")); n != 1 {
		t.Fatalf("повтор подробности записан снова: %d; want 1 (дедуп, D-41)", n)
	}
}

// --- карточки и журнал ------------------------------------------------------------------------

// Ловит: «Сегодня» теряет детей, показывает не те цифры, что значок (Review Focus 5: один
// источник статуса), или отдаёт null вместо пустого списка.
func TestGetTodayMatchesChildStatus(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 60), configured())
	h.start()
	h.childLogsIn()
	h.run(30, activeSample)
	tok := h.login(adminPeer())
	rep := h.do(adminPeer(), tok, ipc.CmdGetToday, nil)
	if strings.Contains(string(rep.Payload), "null") {
		t.Fatalf("в ответе null: %s", rep.Payload)
	}
	var today ipc.GetTodayReply
	_ = rep.Into(&today)
	if len(today.Children) != 2 || today.Children[0].Name != "Миша" || today.Children[1].Name != "Коля" {
		t.Fatalf("дети: %+v", today.Children)
	}
	if got, want := today.Children[0].Status, h.status(); got != want {
		t.Fatalf("карточка %+v, значок %+v", got, want)
	}
}

// Ловит: пустой «Сегодня» отдаёт null вместо пустых списков.
func TestGetTodayEmptyHasNoNulls(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	tok := h.login(adminPeer())
	rep := h.do(adminPeer(), tok, ipc.CmdGetToday, nil)
	if rep.OK != true || strings.Contains(string(rep.Payload), "null") {
		t.Fatalf("ответ %+v", rep)
	}
}

// Ловит: предупреждение о повреждённой базе не показывается родителю при входе, показывается
// вечно (после конца сессии, в которой его показали, оно гаснет) или написано техническими
// словами (NFR-11).
func TestDatabaseNoticeShownOnceThenCleared(t *testing.T) {
	h := newHarness(t, configured(), withRecovered("pcblock.db.broken-20261019-120000"))
	h.start()
	tok := h.login(adminPeer())
	var today ipc.GetTodayReply
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, &today)
	if len(today.Notices) != 1 || today.Notices[0] != textRecoveredNotice {
		t.Fatalf("сообщения: %v", today.Notices)
	}
	// NFR-11, L-112: родителю ни «базы данных», ни «службы», ни пути к файлу копии.
	for _, bad := range []string{"баз", "служб", "broken", "pcblock", `\`} {
		if strings.Contains(strings.ToLower(today.Notices[0]), bad) {
			t.Fatalf("в сообщении техническое слово %q: %q", bad, today.Notices[0])
		}
	}
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, &today)
	if len(today.Notices) != 1 {
		t.Fatalf("в той же сессии сообщение пропало: %v", today.Notices)
	}
	h.ok(adminPeer(), tok, ipc.CmdLogout, nil, nil)
	tok = h.login(adminPeer())
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, &today)
	if len(today.Notices) != 0 {
		t.Fatalf("сообщение осталось после сессии: %v", today.Notices)
	}
}

// Ловит: журнал отдаёт события не простыми словами, без имени ребёнка, не новыми сверху,
// без границ времени и лимита, или с null.
func TestListEvents(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 120), configured())
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	h.ok(adminPeer(), tok, ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: id, Minutes: 30}, nil)
	h.real.Advance(time.Minute)
	h.clock.Advance(time.Minute)
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvBypass, Detail: "Закрыт значок программы"})
	h.clock.Advance(time.Second)
	h.wrongLogin(adminPeer())

	var all ipc.ListEventsReply
	h.ok(adminPeer(), tok, ipc.CmdListEvents, ipc.ListEventsRequest{}, &all)
	var kinds []string
	for _, e := range all.Events {
		kinds = append(kinds, e.Kind)
	}
	if strings.Join(kinds, ",") != "password_failed,bypass,extra_time" {
		t.Fatalf("виды %v, ждали новые сверху", kinds)
	}
	if got := all.Events[1].Text; got != "Миша: попытка обойти защиту. Закрыт значок программы" {
		t.Fatalf("текст обхода %q", got)
	}
	if got := all.Events[2].Text; got != "Миша: доп. время +30 мин" {
		t.Fatalf("текст доп. времени %q", got)
	}
	if got := all.Events[0].Text; got != "Неверный пароль родителя" || all.Events[0].ChildName != "" {
		t.Fatalf("событие не про ребёнка: %+v", all.Events[0])
	}
	var koly ipc.ListEventsReply
	h.ok(adminPeer(), tok, ipc.CmdListEvents, ipc.ListEventsRequest{ChildID: h.childID(child2SID)}, &koly)
	if len(koly.Events) != 0 {
		t.Fatalf("у Коли события: %+v", koly.Events)
	}
	rep := h.do(adminPeer(), tok, ipc.CmdListEvents, ipc.ListEventsRequest{ChildID: h.childID(child2SID)})
	if strings.Contains(string(rep.Payload), "null") {
		t.Fatalf("null в пустом журнале: %s", rep.Payload)
	}
	var one ipc.ListEventsReply
	h.ok(adminPeer(), tok, ipc.CmdListEvents, ipc.ListEventsRequest{Limit: 1}, &one)
	if len(one.Events) != 1 || one.Events[0].Kind != "password_failed" {
		t.Fatalf("лимит 1: %+v", one.Events)
	}
	from := all.Events[1].AtMs
	var window ipc.ListEventsReply
	h.ok(adminPeer(), tok, ipc.CmdListEvents, ipc.ListEventsRequest{FromMs: from, ToMs: from + 1}, &window)
	if len(window.Events) != 1 || window.Events[0].Kind != "bypass" {
		t.Fatalf("окно времени: %+v", window.Events)
	}
}

// Ловит: экран журнала (вкладки «За месяц» и «Всё», D-47) просит до 1000 событий, а служба
// молча режет ответ до 500 и старые события пропадают без предупреждения.
func TestListEventsAllowsThousand(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	for i := 0; i < 700; i++ {
		if err := h.store.AppendEvent(StoredEvent{ChildID: id, AtMs: int64(1000 + i), Kind: "extra_time", Details: map[string]any{"minutes": 5}}); err != nil {
			t.Fatal(err)
		}
	}
	var rep ipc.ListEventsReply
	h.ok(adminPeer(), tok, ipc.CmdListEvents, ipc.ListEventsRequest{Limit: 1000}, &rep)
	if len(rep.Events) != 700 {
		t.Fatalf("событий %d, ждали все 700 (лимит 1000)", len(rep.Events))
	}
	if b, err := json.Marshal(rep); err != nil || len(b) > ipc.MaxFrame/2 {
		t.Fatalf("700 записей журнала занимают %d байт: тысяча не поместится в кадр с запасом (%v)", len(b), err)
	}
	var over ipc.ListEventsReply
	h.ok(adminPeer(), tok, ipc.CmdListEvents, ipc.ListEventsRequest{Limit: 5000}, &over)
	if len(over.Events) != 700 {
		t.Fatalf("лимит выше потолка: %d событий", len(over.Events))
	}
}

// Ловит: итог дня в журнале выводится числами из базы (float64) с ошибкой формата.
func TestEventTextDayClosedFromStoredNumbers(t *testing.T) {
	d := map[string]any{}
	if err := json.Unmarshal([]byte(`{"day":"2026-10-19","total_s":3900}`), &d); err != nil {
		t.Fatal(err)
	}
	if got := eventText("day_closed", "Миша", d); got != "Миша: итог дня 19 октября: 1 ч 05 мин" {
		t.Fatalf("текст %q", got)
	}
	if got := eventText("неизвестное", "", map[string]any{"text": "сделано"}); got != "Сделано" {
		t.Fatalf("неизвестный вид %q", got)
	}
	if got := eventText("неизвестное", "Миша", nil); got != "Миша: неизвестное" {
		t.Fatalf("без текста %q", got)
	}
}

// --- состав детей (FR-25) ------------------------------------------------------------------------

// Ловит: SetChildren не добавляет, не убирает и не переименовывает; запрет Диспетчера задач
// не снимается у убранного и не ставится новому; администратора можно отметить детским (A2).
func TestSetChildren(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	var sc ipc.SetChildrenReply
	h.ok(adminPeer(), tok, ipc.CmdSetChildren, ipc.SetChildrenRequest{
		Add:    []ipc.ChildAdd{{SID: otherSID, Name: "Даша"}},
		Rename: []ipc.ChildRename{{ChildID: id, Name: "Михаил"}},
	}, &sc)
	if len(sc.Children) != 2 || sc.Children[0].Name != "Михаил" || sc.Children[1].Name != "Даша" {
		t.Fatalf("дети: %+v", sc.Children)
	}
	if !h.policy.disabled(otherSID) {
		t.Fatal("новому ребёнку не поставлен запрет Диспетчера задач")
	}
	dasha := sc.Children[1].ChildID
	h.ok(adminPeer(), tok, ipc.CmdSetChildren, ipc.SetChildrenRequest{Remove: []int64{dasha}}, &sc)
	if len(sc.Children) != 1 || h.policy.disabled(otherSID) {
		t.Fatalf("после удаления: %+v, запрет %v", sc.Children, h.policy.disabled(otherSID))
	}
	rep := h.fail(adminPeer(), tok, ipc.CmdSetChildren, ipc.SetChildrenRequest{Add: []ipc.ChildAdd{{SID: parentSID, Name: "Анна"}}})
	if rep.Field != ipc.FieldChildren || rep.Error != textAdminAccount {
		t.Fatalf("администратор: %+v", rep)
	}
	rep = h.fail(adminPeer(), tok, ipc.CmdSetChildren, ipc.SetChildrenRequest{Remove: []int64{999}})
	if rep.Field != ipc.FieldChildren {
		t.Fatalf("несуществующий: %+v", rep)
	}
	rep = h.fail(adminPeer(), tok, ipc.CmdSetChildren, ipc.SetChildrenRequest{Rename: []ipc.ChildRename{{ChildID: id, Name: "  "}}})
	if rep.Field != ipc.FieldChildren {
		t.Fatalf("пустое имя: %+v", rep)
	}
}

// Ловит: эскалация T7: убранный ребёнок уносит блокировку за обход в базе, и при повторной
// отметке учётки детской она сразу заблокирована.
func TestRemoveChildClearsBypassBeforeReAdd(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvBypass, Detail: "обход"})
	id := h.childID(childSID)
	tok := h.login(adminPeer())
	h.ok(adminPeer(), tok, ipc.CmdSetChildren, ipc.SetChildrenRequest{Remove: []int64{id}}, nil)
	if c := h.store.children[id]; c.BypassDetail != "" {
		t.Fatalf("блокировка осталась у убранного ребёнка: %q", c.BypassDetail)
	}
	var sc ipc.SetChildrenReply
	h.ok(adminPeer(), tok, ipc.CmdSetChildren, ipc.SetChildrenRequest{Add: []ipc.ChildAdd{{SID: childSID, Name: "Миша"}}}, &sc)
	if st := h.status(); st.Lock != ipc.LockNone {
		t.Fatalf("после повторной отметки ребёнок заблокирован: %+v", st)
	}
}

// Ловит: после снятия отметки страж в сеансе остаётся и продолжает ограничивать; новый
// ребёнок, уже вошедший в сеанс, остаётся без стража.
func TestSetChildrenStartsAndStopsGuard(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
	if len(h.launcher.aliveOf(ProcGuard, childSession)) != 0 {
		t.Fatal("страж запущен в сеансе не ребёнка")
	}
	tok := h.login(adminPeer())
	var sc ipc.SetChildrenReply
	h.ok(adminPeer(), tok, ipc.CmdSetChildren, ipc.SetChildrenRequest{Add: []ipc.ChildAdd{{SID: childSID, Name: "Миша"}}}, &sc)
	if len(h.launcher.aliveOf(ProcGuard, childSession)) != 1 {
		t.Fatal("страж не запущен новому ребёнку")
	}
	h.ok(adminPeer(), tok, ipc.CmdSetChildren, ipc.SetChildrenRequest{Remove: []int64{sc.Children[0].ChildID}}, nil)
	if len(h.launcher.aliveOf(ProcGuard, childSession)) != 0 {
		t.Fatal("страж остался после снятия отметки")
	}
	if len(h.launcher.aliveOf(ProcTray, childSession)) != 1 {
		t.Fatal("значок пропал после снятия отметки (он нужен в каждом сеансе)")
	}
}

// Ловит: список учёток путает администратора и ребёнка, не отмечает текущую учётку и не даёт
// выбрать администратора (D-38).
func TestListAccounts(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	var la ipc.ListAccountsReply
	h.ok(elevatedPeer(), "", ipc.CmdListAccounts, nil, &la)
	byName := map[string]ipc.Account{}
	for _, a := range la.Accounts {
		byName[a.Name] = a
	}
	if a := byName["Анна"]; !a.Admin || a.CanBeChild || !a.Current || a.Child {
		t.Errorf("Анна: %+v", a)
	}
	if a := byName["Миша"]; a.Admin || !a.CanBeChild || a.Current || !a.Child {
		t.Errorf("Миша: %+v", a)
	}
	if a := byName["Даша"]; !a.CanBeChild || a.Child {
		t.Errorf("Даша: %+v", a)
	}
}

// --- ребёнок о себе ------------------------------------------------------------------------------

// Ловит: «Моя статистика» показывает не те строки и правила, чужому пользователю
// отдаёт данные ребёнка, или null в пустых списках.
func TestMyToday(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.run(65, activeSample)
	var my ipc.MyTodayReply
	h.ok(childPeer(), "", ipc.CmdMyToday, nil, &my)
	if my.Status.Name != "Миша" || len(my.Rules) != 7 || my.Rules[0].Day != "пн" || my.Rules[0].Text != "2 ч" {
		t.Fatalf("ответ: %+v", my)
	}
	if len(my.Today) != 2 || my.Today[1] != "Использовано 1 мин" {
		t.Fatalf("сегодня: %v", my.Today)
	}
	rep := h.do(otherPeer(), "", ipc.CmdMyToday, nil)
	var other ipc.MyTodayReply
	_ = rep.Into(&other)
	if other.Status.Status != ipc.StatusNotChild || strings.Contains(string(rep.Payload), "null") || other.Status.Name != "" {
		t.Fatalf("чужой пользователь: %s", rep.Payload)
	}
}

// Ловит: слова «Нельзя» и «Без лимита» в «Моих правилах» путаются.
func TestMyRulesWords(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	l := week(90)
	l[5].TotalMin = 0
	l[6].Unlimited = true
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: h.childID(childSID), Limits: l}, nil)
	var my ipc.MyTodayReply
	h.ok(childPeer(), "", ipc.CmdMyToday, nil, &my)
	if my.Rules[0].Text != "1 ч 30 мин" || my.Rules[5].Text != "Нельзя" || my.Rules[6].Text != "Без лимита" {
		t.Fatalf("правила: %+v", my.Rules)
	}
}

// Ловит: учёт показывается ребёнку-администратору как обычный (в «Моей статистике» нужен
// статус «Администратор: ограничения не действуют»).
func TestMyTodayForAdminChild(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.admins.set(childSID, true)
	h.start()
	var my ipc.MyTodayReply
	h.ok(childPeer(), "", ipc.CmdMyToday, nil, &my)
	if my.Status.Status != ipc.StatusAdmin || my.Status.StatusText != "Администратор: ограничения не действуют" {
		t.Fatalf("статус: %+v", my.Status)
	}
}

// Ловит: окно первого входа открывается при каждом запуске значка (после «Понятно» оно
// не должно появляться); чужая учётка получает окно ребёнка.
func TestFirstRunWindowOpensUntilSeen(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	tray := h.trayPeer(childSession, childSID)
	var ow ipc.OpenWindowReply
	h.ok(tray, "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowFirstRun}, &ow)
	if !ow.Opened || len(h.launcher.aliveOf(ProcWindow, childSession)) != 1 {
		t.Fatalf("окно не открыто: %+v", ow)
	}
	h.ok(tray, "", ipc.CmdFirstRunSeen, nil, nil)
	h.ok(tray, "", ipc.CmdWindowClosing, ipc.WindowClosingRequest{Kind: ipc.WindowFirstRun}, nil)
	h.ok(tray, "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowFirstRun}, &ow)
	if ow.Opened {
		t.Fatal("окно первого входа открыто повторно")
	}
	rep := h.fail(otherPeer(), "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: ipc.WindowMyStats})
	if rep.Error != textNotAChild {
		t.Fatalf("чужой пользователь: %q", rep.Error)
	}
	h.fail(tray, "", ipc.CmdOpenWindow, ipc.OpenWindowRequest{Kind: "нет такого"})
}

// --- Помощник ---------------------------------------------------------------------------------------

// Ловит: профили Помощника родителя и ребёнка смешиваются; непонятный JSON и огромное
// состояние принимаются; чужому пользователю без сессии отдают профиль.
func TestHelperStateProfiles(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	h.ok(adminPeer(), tok, ipc.CmdSaveHelperState, ipc.HelperState{State: json.RawMessage(`{"enabled":true,"seen":["H-M1-01"]}`)}, nil)
	h.ok(childPeer(), "", ipc.CmdSaveHelperState, ipc.HelperState{State: json.RawMessage(`{"enabled":false}`)}, nil)

	var p, c ipc.HelperState
	h.ok(adminPeer(), tok, ipc.CmdGetHelperState, nil, &p)
	h.ok(childPeer(), "", ipc.CmdGetHelperState, nil, &c)
	if !strings.Contains(string(p.State), "H-M1-01") || strings.Contains(string(c.State), "H-M1-01") || !strings.Contains(string(c.State), "false") {
		t.Fatalf("родитель %s, ребёнок %s", p.State, c.State)
	}
	if rep := h.fail(otherPeer(), "", ipc.CmdGetHelperState, nil); rep.Error != textNeedParent {
		t.Fatalf("чужой без сессии: %q", rep.Error)
	}
	h.fail(adminPeer(), tok, ipc.CmdSaveHelperState, ipc.HelperState{State: json.RawMessage(`[1,2]`)})
	huge := `{"x":"` + strings.Repeat("я", maxHelperState) + `"}`
	h.fail(adminPeer(), tok, ipc.CmdSaveHelperState, ipc.HelperState{State: json.RawMessage(huge)})
	rep := h.do(otherPeer(), "", ipc.CmdGetHelperState, nil)
	_ = rep
	var empty ipc.HelperState
	h2 := newHarness(t, configured())
	h2.start()
	tok2 := h2.login(adminPeer())
	rep = h2.do(adminPeer(), tok2, ipc.CmdGetHelperState, nil)
	_ = rep.Into(&empty)
	if string(rep.Payload) != `{"state":{}}` {
		t.Fatalf("пустое состояние: %s", rep.Payload)
	}
}
