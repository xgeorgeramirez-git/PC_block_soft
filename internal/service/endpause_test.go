package service

import (
	"testing"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Ловит: EndPause не снимает паузу у службы, не возвращает экран «время вышло» сразу, не
// шлёт стражу плашку «Пауза закончилась», не пишет журнал «пауза закончена раньше срока»
// или не сохраняет день (R11, D-96, S11).
func TestEndPauseLocksAndNotices(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	id := h.childID(childSID)
	var week [7]core.DayLimit
	for i := range week {
		week[i] = core.LimitOf(0) // время вышло сразу
	}
	if err := h.store.SetDayLimits(id, week); err != nil {
		t.Fatal(err)
	}
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	if st := h.status(); st.Lock != ipc.LockTimeUp {
		t.Fatalf("до паузы Lock = %q, want timeUp", st.Lock)
	}
	h.ok(adminPeer(), tok, ipc.CmdPause, ipc.PauseRequest{ChildID: id, Preset: ipc.PauseHour1}, nil)
	if st := h.status(); st.Lock != ipc.LockNone || st.Status != ipc.StatusPaused {
		t.Fatalf("в паузе Lock=%q Status=%q", st.Lock, st.Status)
	}
	h.run(3, activeSample)
	_ = h.poll(activeSample) // сбросить плашку «Пауза 1 ч»

	var rep ipc.EndPauseReply
	h.ok(adminPeer(), tok, ipc.CmdEndPause, ipc.EndPauseRequest{ChildID: id}, &rep)
	if rep.Message != "Пауза закончилась" {
		t.Fatalf("Message = %q, want «Пауза закончилась»", rep.Message)
	}
	if rep.Card.Status.Status != ipc.StatusTimeUp || rep.Card.Status.PauseUntil != "" {
		t.Fatalf("карточка после EndPause: %q, pauseUntil %q", rep.Card.Status.Status, rep.Card.Status.PauseUntil)
	}
	if st := h.status(); st.Lock != ipc.LockTimeUp {
		t.Fatalf("после EndPause Lock = %q, want timeUp сразу", st.Lock)
	}
	poll := h.poll(activeSample)
	if poll.Status.Lock != ipc.LockTimeUp {
		t.Fatalf("страж: Lock = %q, want timeUp", poll.Status.Lock)
	}
	if len(poll.Commands) != 1 || poll.Commands[0].Kind != ipc.GuardCmdNotice || poll.Commands[0].Text != "Пауза закончилась" {
		t.Fatalf("команды стражу %+v, want плашку «Пауза закончилась»", poll.Commands)
	}
	got := h.journalTexts(core.EvPauseEnd)
	if len(got) != 1 || got[0] != "Миша: пауза закончена раньше срока" {
		t.Fatalf("журнал pause_end %q", got)
	}
	if row, ok := h.dayRow(id, h.engState(id).Day); !ok || row.PauseUntilMs != nil {
		t.Fatalf("день в базе: ok=%v pause=%v, want сохранён без паузы", ok, row.PauseUntilMs)
	}
}

// Ловит: EndPause без паузы молча проходит; повторная пауза после EndPause не ставится.
func TestEndPauseNoPauseAndAgain(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	if rep := h.fail(adminPeer(), tok, ipc.CmdEndPause, ipc.EndPauseRequest{ChildID: id}); rep.Error != "Паузы сейчас нет" {
		t.Fatalf("без паузы: ошибка %q, want «Паузы сейчас нет»", rep.Error)
	}
	if rep := h.fail(adminPeer(), tok, ipc.CmdEndPause, ipc.EndPauseRequest{ChildID: 999}); rep.Error != textNoSuchChild {
		t.Fatalf("чужой ребёнок: ошибка %q", rep.Error)
	}
	h.ok(adminPeer(), tok, ipc.CmdPause, ipc.PauseRequest{ChildID: id, Preset: ipc.PauseMin15}, nil)
	h.ok(adminPeer(), tok, ipc.CmdEndPause, ipc.EndPauseRequest{ChildID: id}, nil)
	var rep ipc.PauseReply
	h.ok(adminPeer(), tok, ipc.CmdPause, ipc.PauseRequest{ChildID: id, Preset: ipc.PauseMin15}, &rep)
	if rep.Card.Status.Status != ipc.StatusPaused || rep.Card.Status.PauseUntil == "" {
		t.Fatalf("повторная пауза: %q до %q", rep.Card.Status.Status, rep.Card.Status.PauseUntil)
	}
}

// Ловит: EndPause доступна без токена родителя или ребёнку (R19).
func TestEndPauseNeedsParent(t *testing.T) {
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120))
	h.start()
	id := h.childID(childSID)
	if rep := h.do(childPeer(), "", ipc.CmdEndPause, ipc.EndPauseRequest{ChildID: id}); rep.OK {
		t.Fatal("ребёнок закончил паузу")
	}
	if rep := h.do(adminPeer(), "", ipc.CmdEndPause, ipc.EndPauseRequest{ChildID: id}); rep.OK {
		t.Fatal("пауза закончена без токена родителя")
	}
}
