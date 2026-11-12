package service

import (
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Перерыв на отдых в службе (FB-14, D-62, R27, FR-37): настройки «Отдых» в правилах ребёнка,
// копирование FR-33, плашка и экран для стража, сохранение сразу (RF14-2), «Закончить перерыв»
// паролем, журнал, выход из перерыва — не обход (PL-13).

// s43Rules — «Отдых» вкладки 8.3.3 п.7.
func s43Rules() ipc.BreakRules {
	return ipc.BreakRules{Enabled: true, AfterMin: 60, Counts: "games", WarnMin: 5, RestMin: 15, SelfRestMin: 30, Message: "Пойди погуляй с собакой"}
}

// restHarness — ребёнок с лимитом 180 мин и перерывами 15 / всё / 1 / 5 / 5; счёт к перерыву
// 899 с сохранён до старта (плашка — на втором тике после входа).
func restHarness(t *testing.T) (*harness, int64) {
	t.Helper()
	h := newHarness(t, withChild(childSID, "Миша", 180), configured())
	id := h.childID(childSID)
	h.store.SetBreakRules(id, core.BreakRules{Enabled: true, AfterMin: 15, Counts: core.BreakAll, WarnMin: 1, RestMin: 5, SelfRestMin: 5})
	h.store.SaveBreakState(id, core.BreakState{CountedS: 899})
	h.start()
	h.childLogsIn()
	return h, id
}

func (f *fakeStore) breakOf(id int64) core.BreakState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.breakStates[id]
}

// Ловит: правила ребёнка без «Отдыха» или не с умолчаниями FR-37; сохранение не пишет «Отдых»,
// трогает лимиты, не применяется сразу или не пишется в журнал; выключенный переключатель
// теряет значения.
func TestRestSettingsReadSave(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	id := h.childID(childSID)
	tok := h.login(adminPeer())
	var r ipc.ChildRules
	h.ok(adminPeer(), tok, ipc.CmdGetChildRules, ipc.ChildRequest{ChildID: id}, &r)
	if want := (ipc.BreakRules{AfterMin: 60, Counts: "all", WarnMin: 5, RestMin: 15, SelfRestMin: 15}); r.Break != want {
		t.Fatalf("«Отдых» по умолчанию %+v, want %+v", r.Break, want)
	}
	b := s43Rules()
	var rep ipc.SaveLimitsReply
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Limits: []ipc.WeekdayLimit{}, Break: &b}, &rep)
	if rep.Rules.Break != s43Rules() || rep.Message != "Сохранено. Новые правила уже действуют" {
		t.Fatalf("ответ %+v", rep)
	}
	if got, _ := h.store.GetBreakRules(id); got != (core.BreakRules{Enabled: true, AfterMin: 60, Counts: core.BreakGames, WarnMin: 5, RestMin: 15, SelfRestMin: 30, Message: "Пойди погуляй с собакой"}) {
		t.Fatalf("в базе %+v", got)
	}
	if lim, _ := h.store.GetDayLimits(id); lim[0].TotalMin == nil || *lim[0].TotalMin != 120 {
		t.Fatalf("лимиты тронуты: %+v", lim[0])
	}
	if cs := h.svc.children[id]; !cs.rules.Break.Enabled || cs.rules.Break.SelfRestMin != 30 {
		t.Fatalf("правила ядра не обновлены: %+v", cs.rules.Break)
	}
	if evs := h.store.eventsOf("settings_changed"); len(evs) != 1 || evs[0].Details["text"] != "изменены настройки отдыха" {
		t.Fatalf("журнал %+v", evs)
	}
	b.Enabled = false
	h.ok(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Break: &b}, &rep)
	if got, _ := h.store.GetBreakRules(id); got.Enabled || got.AfterMin != 60 || got.SelfRestMin != 30 || got.Message != "Пойди погуляй с собакой" {
		t.Fatalf("выключили: %+v, want значения сохранены", got)
	}
}

// Ловит: пределы FR-37 не проверяются службой или ошибка не у своего поля (D-45).
func TestRestSettingsRejected(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	id := h.childID(childSID)
	tok := h.login(adminPeer())
	ok := s43Rules()
	for _, tc := range []struct {
		name, field, text string
		mod               func(b *ipc.BreakRules)
	}{
		{"через 14", "break.afterMin", "Можно от 15 до 240 мин", func(b *ipc.BreakRules) { b.AfterMin = 14 }},
		{"предупредить 16", "break.warnMin", "Можно от 1 до 15 мин", func(b *ipc.BreakRules) { b.WarnMin = 16 }},
		{"отдых 61", "break.restMin", "Можно от 5 до 60 мин", func(b *ipc.BreakRules) { b.RestMin = 61; b.SelfRestMin = 90 }},
		{"сам 121", "break.selfRestMin", "Можно от 5 до 120 мин", func(b *ipc.BreakRules) { b.SelfRestMin = 121 }},
		{"сам меньше отдыха", "break.selfRestMin", "Не меньше 20 мин: столько длится отдых", func(b *ipc.BreakRules) { b.RestMin = 20; b.SelfRestMin = 19 }},
		{"что считать", "break.counts", "Выберите, что считать", func(b *ipc.BreakRules) { b.Counts = "video" }},
		{"121 символ", "break.message", "Не больше 120 символов", func(b *ipc.BreakRules) { b.Message = strings.Repeat("я", 121) }},
	} {
		b := ok
		tc.mod(&b)
		rep := h.fail(adminPeer(), tok, ipc.CmdSaveLimits, ipc.SaveLimitsRequest{ChildID: id, Break: &b})
		if rep.Field != tc.field || rep.Error != tc.text {
			t.Errorf("%s: поле %q, текст %q; want %q, %q", tc.name, rep.Field, rep.Error, tc.field, tc.text)
		}
	}
	if got, _ := h.store.GetBreakRules(id); got.Enabled {
		t.Fatalf("неверные настройки записаны: %+v", got)
	}
}

// Ловит: «Скопировать правила с…» не переносит «Отдых» (FR-33).
func TestRestCopyRules(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 60), configured())
	h.start()
	from, to := h.childID(childSID), h.childID(child2SID)
	h.store.SetBreakRules(from, s43Rules().Core())
	tok := h.login(adminPeer())
	var rep ipc.CopyRulesReply
	h.ok(adminPeer(), tok, ipc.CmdCopyRules, ipc.CopyRulesRequest{FromChildID: from, ToChildID: to}, &rep)
	if rep.Break != s43Rules() {
		t.Fatalf("скопирован «Отдых» %+v, want %+v", rep.Break, s43Rules())
	}
}

// Ловит: плашка «ПОРА ОТДОХНУТЬ» не уходит стражу (или со звуком/видом не того события);
// момент начала и конец перерыва не сохраняются сразу (RF14-2); экран перерыва без конца и
// сообщения; запись журнала не та.
func TestRestPlateScreenAndSave(t *testing.T) {
	h, id := restHarness(t)
	// Тик 15:00:01 доводит счёт до 900 с, тик 15:00:02 ставит плашку (начало перерыва 15:01:02),
	// опрос стража 15:00:03 её забирает, сохранение до тика 15:00:03 пишет момент начала.
	reps := h.run(3, activeSample)
	ws := reps[2].Warnings
	if len(ws) != 1 || ws[0] != (ipc.GuardWarning{Threshold: 1, Text: "Через 1 минуту перерыв на 5 минут. Сохрани игру", Sound: true, Event: "rest", EventS: 60}) {
		t.Fatalf("плашка стражу %+v", ws)
	}
	if b := h.store.breakOf(id); b.BreakAtMs == nil || *b.BreakAtMs != time.Date(2026, 10, 19, 15, 1, 2, 0, tzMSK).UnixMilli() {
		t.Fatalf("момент начала не сохранён сразу: %+v", b)
	}
	reps = h.run(60, activeSample) // тик 15:01:02 начинает перерыв, опрос 15:01:03 его видит
	st := reps[len(reps)-1].Status
	until := time.Date(2026, 10, 19, 15, 6, 2, 0, tzMSK).UnixMilli()
	if st.Lock != ipc.LockRest || st.Reason != "Перерыв на отдых" || st.Until != "Можно снова в 15:06" || st.RestUntilMs != until || st.RestMessage != "Разомнись, попей воды, посмотри в окно" {
		t.Fatalf("экран перерыва %+v", st)
	}
	if b := h.store.breakOf(id); b.RestUntilMs == nil || *b.RestUntilMs != until {
		t.Fatalf("конец перерыва не сохранён сразу: %+v", b)
	}
	if evs := h.store.eventsOf("rest_break"); len(evs) != 1 || evs[0].Details["text"] != "Перерыв на отдых: 5 мин после 15 мин за компьютером" {
		t.Fatalf("журнал %+v", evs)
	}
	used := h.usedS()
	h.run(30, activeSample)
	if got := h.usedS(); got != used {
		t.Fatalf("время за экраном перерыва идёт: %d → %d", used, got)
	}
}

// Ловит: «Закончить перерыв» без пароля со стража, без перерыва — успех, после — экран остаётся
// или нет записи «Вы закончили перерыв».
func TestRestEndBreak(t *testing.T) {
	h, id := restHarness(t)
	tok := h.login(adminPeer())
	if rep := h.fail(adminPeer(), tok, ipc.CmdEndBreak, ipc.ChildRequest{ChildID: id}); rep.Error != "Перерыва сейчас нет" {
		t.Fatalf("без перерыва: %q", rep.Error)
	}
	h.run(65, activeSample)
	h.fail(guardPeer(), "", ipc.CmdEndBreak, ipc.ChildRequest{})
	if rep := h.guardLogin(goodPassword); !rep.OK {
		t.Fatalf("пароль стража: %+v", rep)
	}
	var cr ipc.CardReply
	h.ok(guardPeer(), "", ipc.CmdEndBreak, ipc.ChildRequest{}, &cr)
	if cr.Card.Status.Lock != ipc.LockNone || cr.Message != "Перерыв закончен" {
		t.Fatalf("после «Закончить перерыв»: %+v", cr)
	}
	if evs := h.store.eventsOf("rest_ended"); len(evs) != 1 || evs[0].Details["text"] != "Вы закончили перерыв" {
		t.Fatalf("журнал %+v", evs)
	}
	if b := h.store.breakOf(id); b.RestUntilMs != nil || b.CountedS != 0 {
		t.Fatalf("сохранено %+v, want счёт с нуля", b)
	}
}

// Ловит (PL-13): выход из учётки и вход во время перерыва пишется обходом или снимает перерыв;
// после конца перерыва по часам экран остаётся.
func TestRestLogoffIsNotBypass(t *testing.T) {
	h, _ := restHarness(t)
	h.run(65, activeSample)
	h.svc.OnSession(SessionLogoff, childSession, h.real.Now())
	h.launcher.setSessions()
	h.hk(60)
	h.childLogsIn()
	if st := h.run(1, activeSample)[0].Status; st.Lock != ipc.LockRest {
		t.Fatalf("вход во время перерыва: экран %q, want rest", st.Lock)
	}
	h.run(240, activeSample) // по часам перерыв кончился в 15:06:01
	if st := h.status(); st.Lock != ipc.LockNone {
		t.Fatalf("после конца перерыва экран %q", st.Lock)
	}
	h.hk(60)
	if n := h.bypassCount(); n != 0 {
		t.Fatalf("записей об обходе %d: %+v", n, h.store.eventsOf("bypass"))
	}
}

// Ловит: после перезапуска службы идущий перерыв не восстанавливается из break_state.
func TestRestRestoredAfterRestart(t *testing.T) {
	st := newFakeStore()
	id, _ := st.UpsertChild(childSID, "Миша")
	var week [7]core.DayLimit
	for i := range week {
		week[i] = core.LimitOf(180)
	}
	st.SetDayLimits(id, week)
	st.SetBreakRules(id, core.BreakRules{Enabled: true, AfterMin: 60, WarnMin: 5, RestMin: 15, SelfRestMin: 15})
	until := time.Date(2026, 10, 19, 15, 10, 0, 0, tzMSK).UnixMilli()
	st.SaveBreakState(id, core.BreakState{RestUntilMs: &until})
	h := newHarness(t, withStore(st))
	h.start()
	if s := h.status(); s.Lock != ipc.LockRest || s.Until != "Можно снова в 15:10" {
		t.Fatalf("после старта экран %q, %q; want перерыв до 15:10", s.Lock, s.Until)
	}
}

// Ловит: «Отдых» не прочитался, а ограничения продолжают действовать как будто всё хорошо
// (RM3-9, R18: правила целиком или безопасный режим).
func TestRestRulesReadFailureIsSafeMode(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.store.failBreak = true
	h.start()
	if s := h.status(); s.Status != ipc.StatusSafe {
		t.Fatalf("статус %q, want safe", s.Status)
	}
}
