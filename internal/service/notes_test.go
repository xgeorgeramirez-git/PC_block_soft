package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/remote"
)

// Сообщение ребёнку с телефона в службе (FR-41, S57, протокол связи, раздел 5а).

func (p *testPhone) note(seq, child int64, id, text string, showSec int, ts time.Time) remote.CmdResult {
	t := p.t
	t.Helper()
	raw, _ := json.Marshal(remote.Note{V: remote.ProtoVersion, T: remote.TypeNote, PC: p.pcID, Phone: p.id, Seq: seq,
		TS: ts.UnixMilli(), Child: child, ID: id, Text: text, ShowSec: showSec})
	s, err := remote.Sign(p.key, raw)
	if err != nil {
		t.Fatal(err)
	}
	rep, ok := p.r.sendFrom(p.id, remote.KindNote, s, 5*time.Second)
	if !ok || rep.Kind != remote.KindNoteR {
		t.Fatalf("note: нет ответа noter (%v, %q)", ok, rep.Kind)
	}
	var signed remote.Signed
	if err := json.Unmarshal(rep.Body, &signed); err != nil {
		t.Fatal(err)
	}
	data, err := remote.Verify(p.pcPub, signed)
	if err != nil {
		t.Fatalf("ответ без подписи компьютера: %v", err)
	}
	var res remote.CmdResult
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatal(err)
	}
	if res.T != remote.TypeNoteR || res.Cmd != remote.HashData(raw) {
		t.Fatalf("ответ не на это сообщение: %+v", res)
	}
	return res
}

// lastReport — последний отчёт компьютера, проверенный подписью.
func (r *testRelay) lastReport(t *testing.T, pc *testPhone) remote.Report {
	t.Helper()
	r.mu.Lock()
	n := len(r.reports)
	var s remote.Signed
	if n > 0 {
		s = r.reports[n-1]
	}
	r.mu.Unlock()
	if n == 0 {
		t.Fatal("отчётов нет")
	}
	data, err := remote.Verify(pc.pcPub, s)
	if err != nil {
		t.Fatal(err)
	}
	var rep remote.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatal(err)
	}
	return rep
}

func noteCmds(rep ipc.GuardPollReply) []ipc.GuardNote {
	out := []ipc.GuardNote{}
	for _, c := range rep.Commands {
		if c.Kind == ipc.GuardCmdNote && c.Note != nil {
			out = append(out, *c.Note)
		}
	}
	return out
}

func (h *harness) noteStates() map[string]string {
	h.svc.mu.Lock()
	defer h.svc.mu.Unlock()
	out := map[string]string{}
	for _, r := range h.svc.notes.list {
		out[r.ID] = r.State
	}
	return out
}

func (h *harness) storedNotes() []noteRec {
	h.t.Helper()
	v, ok, _ := h.store.GetSetting(settingNotes)
	var list []noteRec
	if ok {
		if err := json.Unmarshal([]byte(v), &list); err != nil {
			h.t.Fatal(err)
		}
	}
	return list
}

// Ловит: сообщение берётся без записи seq (после перезапуска пришло бы второй раз), чужое поколение
// или повтор ставят сообщение в очередь, отвязанный телефон пишет ребёнку, повтор того же id даёт
// второе сообщение. Журнал — «Миша: сообщение с телефона: «Иди ужинать»».
func TestNoteApplySeqFirst(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)
	pairPhone(t, h, tok, r)
	id := h.childID(childSID)
	l := h.remoteLink()
	host := remoteHost{h.svc}
	note := func(seq int64, nid string) remote.Note {
		return remote.Note{Seq: seq, Child: id, ID: nid, Text: "Иди ужинать", ShowSec: 0}
	}

	h.store.mu.Lock()
	h.store.setSettingErr = func(key string) error {
		if key == settingRemote {
			return errors.New("диск")
		}
		return nil
	}
	h.store.mu.Unlock()
	if code, last := host.ApplyNote(l.Gen(), note(1, "a")); code != remote.ErrCodeStale || last != 0 {
		t.Fatalf("номер не записан: %q %d", code, last)
	}
	if len(h.noteStates()) != 0 || len(h.journalTexts(evRemoteNote)) != 0 {
		t.Fatal("сообщение принято без записи номера")
	}
	h.store.mu.Lock()
	h.store.setSettingErr = nil
	h.store.mu.Unlock()

	if code, _ := host.ApplyNote(l.Gen()+1000, note(1, "a")); code != remote.ErrCodeStale {
		t.Fatalf("чужое поколение: %q", code)
	}
	if code, last := host.ApplyNote(l.Gen(), note(1, "a")); code != "" || last != 1 {
		t.Fatalf("сообщение: %q %d", code, last)
	}
	if st := h.remoteStored(); st.LastSeq != 1 {
		t.Fatalf("seq не записан: %d", st.LastSeq)
	}
	if code, last := host.ApplyNote(l.Gen(), note(1, "b")); code != remote.ErrCodeReplay || last != 1 {
		t.Fatalf("повтор seq: %q %d", code, last)
	}
	if code, last := host.ApplyNote(l.Gen(), remote.Note{Seq: 2, Child: 999, ID: "c", Text: "x"}); code != remote.ErrCodeNoChild || last != 2 {
		t.Fatalf("нет ребёнка: %q %d", code, last)
	}
	if code, _ := host.ApplyNote(l.Gen(), note(3, "a")); code != "" {
		t.Fatalf("тот же id новым номером: %q", code)
	}
	if got := h.noteStates(); len(got) != 1 || got["a"] != remote.NoteQueued {
		t.Fatalf("очередь: %v", got)
	}
	if got := strings.Join(h.journalTexts(evRemoteNote), "|"); got != "Миша: сообщение с телефона: «Иди ужинать»" {
		t.Fatalf("журнал: %q", got)
	}
	if st := h.storedNotes(); len(st) != 1 || st[0].Text != "Иди ужинать" || st[0].State != remote.NoteQueued {
		t.Fatalf("запись очереди: %+v", st)
	}
	// Отвязан родителем — не принимается.
	h.svc.mu.Lock()
	h.svc.rem.unpairing = h.svc.rem.st.Phone.ID
	h.svc.mu.Unlock()
	if code, _ := host.ApplyNote(l.Gen(), note(4, "d")); code != remote.ErrCodeNotPaired {
		t.Fatalf("отвязанный телефон: %q", code)
	}
}

// Ловит: сообщение уходит стражу текстом общего канала (notice) и он угадывает его по тексту (PL-81);
// показ при отключённом или заблокированном сеансе; повтор стражу каждую секунду; «показано» и
// «Понял» не доходят до отчёта телефону; текст хранится после показа.
func TestNoteDeliveryStates(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)
	h.childLogsIn()
	id := h.childID(childSID)
	ph := pairPhone(t, h, tok, r)
	h.ok(adminPeer(), tok, ipc.CmdLogout, nil, nil)

	// Сеанс заблокирован: сообщение ждёт.
	h.svc.OnSession(SessionLock, childSession, h.real.Now())
	res := ph.note(1, id, "n1", "  Иди\r\nужинать ", 0, h.real.Now())
	if !res.OK || res.Report == nil || len(res.Report.Notes) != 1 || res.Report.Notes[0].State != remote.NoteQueued {
		t.Fatalf("noter: %+v", res)
	}
	if got := noteCmds(h.step(idleSample)); len(got) != 0 {
		t.Fatalf("показ в заблокированном сеансе: %+v", got)
	}
	h.svc.OnSession(SessionUnlock, childSession, h.real.Now())

	rep := h.step(activeSample)
	got := noteCmds(rep)
	if len(got) != 1 || got[0].ID != "n1" || got[0].Text != "Иди\nужинать" || got[0].ShowS != 0 || got[0].WaitS < 590 || got[0].WaitS > 600 {
		t.Fatalf("команда стражу: %+v (%+v)", got, rep.Commands)
	}
	for _, c := range rep.Commands {
		if c.Kind == ipc.GuardCmdNotice && strings.Contains(c.Text, "ужинать") {
			t.Fatalf("сообщение ушло общим каналом notice: %+v", c)
		}
	}
	// Следующие 4 с — не повторяется; на 5-й — снова (страж мог перезапуститься).
	for i := 0; i < 4; i++ {
		if got := noteCmds(h.step(activeSample)); len(got) != 0 {
			t.Fatalf("повтор через %d с: %+v", i+1, got)
		}
	}
	if got := noteCmds(h.step(activeSample)); len(got) != 1 {
		t.Fatalf("через 5 с без «показано» не отдано снова: %+v", got)
	}
	if h.noteStates()["n1"] != remote.NoteQueued {
		t.Fatal("отдано стражу — ещё не «показано»")
	}

	before := r.reportCount()
	h.real.Advance(time.Second)
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvNoteShown, AtMs: h.real.Now().UnixMilli(), NoteID: "n1"})
	if h.noteStates()["n1"] != remote.NoteShown {
		t.Fatalf("после «показано»: %v", h.noteStates())
	}
	waitRemoteCond(t, "отчёт после показа", 5*time.Second, func() bool { return r.reportCount() > before })
	if rp := r.lastReport(t, ph); len(rp.Notes) != 1 || rp.Notes[0].State != remote.NoteShown || rp.Notes[0].AtMs != h.real.Now().UnixMilli() {
		t.Fatalf("отчёт: %+v", rp.Notes)
	}
	if st := h.storedNotes(); len(st) != 1 || st[0].Text != "" || st[0].State != remote.NoteShown {
		t.Fatalf("запись после показа: %+v", st)
	}
	if got := noteCmds(h.step(activeSample)); len(got) != 0 {
		t.Fatalf("показанное отдано снова: %+v", got)
	}
	// Событие чужого сеанса (другой ребёнок) и неизвестный id — ничего.
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvNoteDone, NoteID: "zzz"})
	before = r.reportCount()
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvNoteDone, NoteID: "n1"})
	if h.noteStates()["n1"] != remote.NoteRead {
		t.Fatalf("после «Понял»: %v", h.noteStates())
	}
	waitRemoteCond(t, "отчёт после «Понял»", 5*time.Second, func() bool { return r.reportCount() > before })
	if rp := r.lastReport(t, ph); rp.Notes[0].State != remote.NoteRead {
		t.Fatalf("отчёт: %+v", rp.Notes)
	}
}

// Ловит: экран блокировки PC Block мешает показу (FR-41: показывать поверх); отключённый сеанс
// получает сообщение; несколько сообщений подряд теряются или идут не по порядку.
func TestNoteOverLockScreenAndOrder(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t, withChild(childSID, "Миша", 1))
	h.childLogsIn()
	id := h.childID(childSID)
	ph := pairPhone(t, h, tok, r)
	h.run(75, activeSample)
	if st := h.status(); st.Lock == ipc.LockNone {
		t.Fatalf("нет блокировки: %+v", st)
	}
	h.svc.OnSession(SessionDisconnect, childSession, h.real.Now())
	ph.note(1, id, "a", "Первое", 10, h.real.Now())
	ph.note(2, id, "b", "Второе", 30, h.real.Now())
	if got := noteCmds(h.step(activeSample)); len(got) != 0 {
		t.Fatalf("показ в отключённом сеансе: %+v", got)
	}
	h.svc.OnSession(SessionConnect, childSession, h.real.Now())
	got := noteCmds(h.step(activeSample))
	if len(got) != 2 || got[0].ID != "a" || got[0].ShowS != 10 || got[1].ID != "b" || got[1].ShowS != 30 {
		t.Fatalf("при экране блокировки: %+v", got)
	}
	if st := h.status(); st.Lock == ipc.LockNone {
		t.Fatal("блокировка снялась")
	}
}

// Ловит: сообщение ждёт ребёнка дольше 10 минут или гаснет раньше; погасшее отдаётся стражу; запись
// очереди не переживает перезапуск службы.
func TestNoteExpiresAndSurvivesRestart(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)
	h.childLogsIn()
	id := h.childID(childSID)
	ph := pairPhone(t, h, tok, r)
	h.svc.OnSession(SessionLock, childSession, h.real.Now())
	ph.note(1, id, "x", "Позвони", 0, h.real.Now())
	h.real.Advance(time.Second)
	ph.note(2, id, "y", "Позвони мне", 0, h.real.Now())

	// Перезапуск службы на той же базе: очередь прочитана из записи.
	h.svc.Shutdown(h.real.Now())
	h2 := newHarness(t, withStore(h.store), withTestMode(TestMode{ClockSpeed: 1}))
	h2.real.Set(h.real.Now())
	h2.clock.Set(h.clock.Now())
	h2.start()
	t.Cleanup(func() { h2.svc.Shutdown(h2.real.Now()) })
	h2.childLogsIn()
	h2.svc.OnSession(SessionLock, childSession, h2.real.Now())
	if got := h2.noteStates(); len(got) != 0 {
		// Очередь читается лениво: первое обращение — Housekeep.
		t.Logf("до обслуживания: %v", got)
	}
	h2.real.Advance(noteWait - 3*time.Second)
	h2.svc.Housekeep(h2.real.Now())
	if got := h2.noteStates(); got["x"] != remote.NoteQueued || got["y"] != remote.NoteQueued {
		t.Fatalf("за 2 с до срока: %v", got)
	}
	h2.real.Advance(2 * time.Second)
	h2.svc.Housekeep(h2.real.Now())
	if got := h2.noteStates(); got["x"] != remote.NoteExpired || got["y"] != remote.NoteQueued {
		t.Fatalf("на сроке: %v", got)
	}
	h2.svc.OnSession(SessionUnlock, childSession, h2.real.Now())
	got := noteCmds(h2.poll(activeSample))
	if len(got) != 1 || got[0].ID != "y" || got[0].WaitS != 1 {
		t.Fatalf("после срока первого: %+v", got)
	}
	h2.real.Advance(time.Second)
	h2.svc.Housekeep(h2.real.Now())
	if got := h2.noteStates(); got["y"] != remote.NoteExpired {
		t.Fatalf("второе на сроке: %v", got)
	}
	if got := noteCmds(h2.poll(activeSample)); len(got) != 0 {
		t.Fatalf("погасшее отдано стражу: %+v", got)
	}
	// Страж всё же показал (успел до срока): правда для телефона — «показано».
	h2.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvNoteShown, NoteID: "y"})
	if got := h2.noteStates(); got["y"] != remote.NoteShown {
		t.Fatalf("показ после срока: %v", got)
	}
	if st := h2.storedNotes(); len(st) != 2 || st[0].Text != "" || st[1].Text != "" {
		t.Fatalf("текст хранится после срока и показа: %+v", st)
	}
}

// Ловит: в отчёте больше 10 сообщений или старше суток; очередь растёт без предела.
func TestNoteReportLimits(t *testing.T) {
	h, _ := remoteHarness(t)
	h.svc.mu.Lock()
	h.svc.notes.loaded = true
	base := h.real.Now().Add(-25 * time.Hour).UnixMilli()
	for i := 0; i < 40; i++ {
		st := remote.NoteRead
		if i == 39 {
			st = remote.NoteQueued
		}
		h.svc.notes.list = append(h.svc.notes.list, &noteRec{ID: string(rune('A' + i)), Child: 1, State: st,
			QueuedMs: base + int64(i)*int64(time.Hour/time.Millisecond)})
	}
	h.svc.pruneNotes(h.real.Now())
	n := len(h.svc.notes.list)
	reps := h.svc.noteReports(h.real.Now())
	h.svc.mu.Unlock()
	if n > noteStoreMax {
		t.Fatalf("хранится %d", n)
	}
	if len(reps) != noteReportMax || reps[len(reps)-1].ID != string(rune('A'+39)) || reps[len(reps)-1].State != remote.NoteQueued {
		t.Fatalf("отчёт: %+v", reps)
	}
	for _, r := range reps {
		if h.real.Now().UnixMilli()-base-int64(r.ID[0]-'A')*int64(time.Hour/time.Millisecond) > noteKeep.Milliseconds() {
			t.Fatalf("в отчёте старше суток: %+v", r)
		}
	}
}

func TestNoteJournalText(t *testing.T) {
	if got := eventText(evRemoteNote, "Миша", map[string]any{"text": "Иди\nужинать  скорее"}); got != "Миша: сообщение с телефона: «Иди ужинать скорее»" {
		t.Fatalf("%q", got)
	}
}
