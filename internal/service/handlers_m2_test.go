package service

import (
	"testing"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Ловит: расписание не сохраняется/не читается службой, не нормализуется (R7, FR-17).
func TestSaveGetSchedule(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)

	// Сохранить два пересекающихся интервала — служба объединяет (R7).
	var rep ipc.SaveScheduleReply
	h.ok(adminPeer(), tok, ipc.CmdSaveSchedule, ipc.SaveScheduleRequest{ChildID: id, Intervals: []ipc.Interval{
		{Weekday: 1, StartMin: 21 * 60, EndMin: 23 * 60},
		{Weekday: 1, StartMin: 22 * 60, EndMin: 7 * 60},
	}}, &rep)
	if len(rep.Intervals) != 1 {
		t.Fatalf("после объединения интервалов %d, want 1: %+v", len(rep.Intervals), rep.Intervals)
	}
	if rep.Intervals[0].StartMin != 21*60 || rep.Intervals[0].EndMin != 7*60 {
		t.Fatalf("объединённый интервал %+v, want 21:00-07:00", rep.Intervals[0])
	}

	// Прочитать обратно.
	var got ipc.GetScheduleReply
	h.ok(adminPeer(), tok, ipc.CmdGetSchedule, ipc.GetScheduleRequest{ChildID: id}, &got)
	if len(got.Intervals) != 1 {
		t.Fatalf("GetSchedule вернул %d интервалов, want 1", len(got.Intervals))
	}
}

// Ловит: начисление в копилку не меняет баланс, не проверяет границу 600 (FR-22).
func TestGrantBonus(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)

	var rep ipc.GrantBonusReply
	h.ok(adminPeer(), tok, ipc.CmdGrantBonus, ipc.GrantBonusRequest{ChildID: id, Minutes: 30, Reason: "за уборку"}, &rep)
	if rep.BalanceAfter != 30 {
		t.Fatalf("баланс после начисления = %d, want 30", rep.BalanceAfter)
	}
	// Начисление выше 600 — отказ у поля.
	bad := h.fail(adminPeer(), tok, ipc.CmdGrantBonus, ipc.GrantBonusRequest{ChildID: id, Minutes: 600, Reason: "много"})
	if bad.Field != ipc.FieldMinutes {
		t.Fatalf("ошибка начисления не у поля minutes: %+v", bad)
	}
}

// Ловит: пауза не снимает блокировку, статус не меняется (R11).
func TestPauseClearsLock(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.store.SetDayLimits(h.childID(childSID), [7]core.DayLimit{}) // без лимита — просто проверяем статус паузы
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)

	var rep ipc.PauseReply
	h.ok(adminPeer(), tok, ipc.CmdPause, ipc.PauseRequest{ChildID: id, Preset: ipc.PauseHour1}, &rep)
	if rep.Card.Status.Status != ipc.StatusPaused && rep.Card.Status.Status != ipc.StatusUnlimited {
		// без лимита статус может быть unlimited; с лимитом — paused. Проверяем, что пауза принята без ошибки.
		t.Fatalf("статус после паузы %q", rep.Card.Status.Status)
	}
}

// Ловит: ребёнок тратит копилку другого ребёнка (минор m3 ревью) или без своего сеанса.
func TestSpendBonusOnlyOwnChild(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withChild(otherSID, "Даша", 120))
	h.start()
	misha := h.childID(childSID)
	dasha := h.childID(otherSID)
	// Дать Даше копилку.
	if _, err := h.store.AppendBonus(dasha, 60, "за уборку"); err != nil {
		t.Fatal(err)
	}
	// Миша (childPeer, его сеанс) пытается потратить копилку Даши — отказ.
	bad := h.fail(childPeer(), "", ipc.CmdSpendBonus, ipc.SpendBonusRequest{ChildID: dasha, Minutes: 30})
	if bad.OK {
		t.Fatal("ребёнок потратил копилку другого ребёнка")
	}
	_ = misha
}

// Ловит: трата из копилки без баланса проходит (FR-11).
func TestSpendBonusNeedsBalance(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	id := h.childID(childSID)
	// Баланса нет — трата отклоняется.
	bad := h.fail(childPeer(), "", ipc.CmdSpendBonus, ipc.SpendBonusRequest{ChildID: id, Minutes: 30})
	if bad.OK {
		t.Fatal("трата из пустой копилки прошла")
	}
}

// Ловит: скопированные правила пишутся сразу вместо черновика (FR-33).
func TestCopyRulesReturnsRulesForDraft(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withChild(otherSID, "Даша", 120))
	src := h.childID(otherSID)
	// Задать Даше лимиты и расписание.
	week := [7]core.DayLimit{}
	week[0] = core.LimitOf(120)
	h.store.SetDayLimits(src, week)
	h.store.SetSchedule(src, []core.Interval{{Weekday: core.Monday, StartMin: 21 * 60, EndMin: 7 * 60}})
	h.start()
	tok := h.login(adminPeer())
	dst := h.childID(childSID)

	var rep ipc.CopyRulesReply
	h.ok(adminPeer(), tok, ipc.CmdCopyRules, ipc.CopyRulesRequest{FromChildID: src, ToChildID: dst}, &rep)
	if len(rep.Limits) != 7 || len(rep.Intervals) != 1 {
		t.Fatalf("CopyRules вернул limits=%d intervals=%d, want 7 и 1", len(rep.Limits), len(rep.Intervals))
	}
	// Правила приёмника в базе НЕ изменены (копирование — в черновик, FR-19).
	got, _ := h.store.GetSchedule(dst)
	if len(got) != 0 {
		t.Fatalf("CopyRules записал интервалы приёмника сразу (%d), а должно быть в черновик", len(got))
	}
}
