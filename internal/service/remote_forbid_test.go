package service

import (
	"crypto/ecdsa"
	"errors"
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/remote"
)

// «Снять запрет на сегодня» с телефона (FR-42, D-98, протокол 5б) и статус запретных часов в отчёте.

// withForbidden задаёт ребёнку sid запретные интервалы (до Start).
func withForbidden(sid string, ivs ...core.Interval) hopt {
	return func(h *harness) {
		if err := h.store.SetSchedule(h.childID(sid), ivs); err != nil {
			h.t.Fatal(err)
		}
	}
}

// guardNotices — тексты плашек-уведомлений стражу сеанса ребёнка; очередь очищается.
func (h *harness) guardNotices() []string {
	h.svc.mu.Lock()
	defer h.svc.mu.Unlock()
	out := []string{}
	for _, c := range h.svc.guardCmd[childSession] {
		if c.Kind == ipc.GuardCmdNotice {
			out = append(out, c.Text)
		}
	}
	delete(h.svc.guardCmd, childSession)
	return out
}

func reportChild(t *testing.T, res remote.CmdResult, id int64) remote.ChildReport {
	t.Helper()
	if res.Report == nil {
		t.Fatalf("ответ без отчёта: %+v", res)
	}
	for _, c := range res.Report.Children {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("в отчёте нет ребёнка %d: %+v", id, res.Report.Children)
	return remote.ChildReport{}
}

// Ловит: команда «снять запрет» идёт не путём кнопки окна (флаг дня, экран блокировки, журнал
// «с телефона», плашка ребёнку, сохранение дня), повтор seq или чужая подпись действуют, повторное
// снятие пишет журнал второй раз, отчёт в ответе не говорит «снят», seq не общий с «+»/«−».
func TestRemoteNoForbidden(t *testing.T) {
	r := newTestRelay(t)
	// Часы ядра — пн 15:00; интервал пн 14:00–16:00 идёт.
	h, tok := remoteHarness(t, withForbidden(childSID, core.Interval{Weekday: core.Monday, StartMin: 14 * 60, EndMin: 16 * 60}))
	h.childLogsIn()
	id := h.childID(childSID)
	ph := pairPhone(t, h, tok, r)
	if st := h.svc.statusOf(h.svc.children[id]); st.Lock != ipc.LockForbidden {
		t.Fatalf("до команды нет экрана запрета: %+v", st)
	}
	h.guardNotices()

	// «+30» и «−60» с телефона — та же нумерация seq.
	if res := ph.command(1, id, 30, h.real.Now()); !res.OK {
		t.Fatalf("+30: %+v", res)
	}
	if res := ph.command(2, id, -60, h.real.Now()); !res.OK || res.Message != "Убрано 60 минут" {
		t.Fatalf("−60: %+v", res)
	}
	h.guardNotices()

	// Чужая подпись — отказ, ничего не изменилось.
	if res := ph.noForbidden(mustNewKey(t), 3, id, h.real.Now()); res.OK || res.Error != remote.ErrCodeBadSig {
		t.Fatalf("чужая подпись: %+v", res)
	}
	if h.engState(id).NoForbidden || len(h.store.eventsOf(core.EvNoForbidden)) != 0 || h.remoteStored().LastSeq != 2 {
		t.Fatal("команда с чужой подписью подействовала")
	}

	res := ph.noForbidden(nil, 3, id, h.real.Now())
	if !res.OK || res.Message != "Запрет на сегодня снят" {
		t.Fatalf("снять запрет: %+v", res)
	}
	if c := reportChild(t, res, id); c.Forbid != remote.ForbidCleared || c.ForbidUntilMs != 0 || c.Blocked {
		t.Fatalf("отчёт в ответе: %+v", c)
	}
	if !h.engState(id).NoForbidden {
		t.Fatal("флаг дня не поставлен")
	}
	if st := h.svc.statusOf(h.svc.children[id]); st.Lock == ipc.LockForbidden {
		t.Fatalf("экран запрета не снят: %+v", st)
	}
	if got := h.guardNotices(); strings.Join(got, "|") != textNoForbiddenNotice {
		t.Fatalf("плашка ребёнку: %q", got)
	}
	if row, ok := h.dayRow(id, "2026-10-19"); !ok || !row.NoForbidden {
		t.Fatalf("день не сохранён: %+v %v", row, ok)
	}
	if st := h.remoteStored(); st.LastSeq != 3 {
		t.Fatalf("LastSeq %d", st.LastSeq)
	}
	want := []string{"Миша: запрет на сегодня снят с телефона"}
	if got := h.journalTexts(core.EvNoForbidden); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("журнал: %q", got)
	}

	// Повтор seq — replay; уже снят — ok без второй записи журнала и без плашки.
	if res := ph.noForbidden(nil, 3, id, h.real.Now()); res.OK || res.Error != remote.ErrCodeReplay {
		t.Fatalf("повтор: %+v", res)
	}
	if res := ph.noForbidden(nil, 4, id, h.real.Now()); !res.OK || res.Message != "Запрет на сегодня уже снят" {
		t.Fatalf("уже снят: %+v", res)
	}
	if n := len(h.store.eventsOf(core.EvNoForbidden)); n != 1 {
		t.Fatalf("записей журнала %d, ждали одну", n)
	}
	if got := h.guardNotices(); len(got) != 0 {
		t.Fatalf("плашка при повторном снятии: %q", got)
	}
	if res := ph.noForbidden(nil, 5, 999, h.real.Now()); res.OK || res.Error != remote.ErrCodeNoChild {
		t.Fatalf("нет ребёнка: %+v", res)
	}
	// Журнал доп. времени — «с телефона», один на команду.
	wantExtra := []string{"Миша: вы добавили 30 мин с телефона", "Миша: вы убавили 60 мин с телефона"}
	if got := h.journalTexts(core.EvExtraTime); strings.Join(got, "|") != strings.Join(wantExtra, "|") {
		t.Fatalf("журнал доп. времени: %q", got)
	}
}

// Ловит: при отсутствии запретных часов сегодня команда ставит флаг дня и пишет журнал (ложная
// запись «снят с телефона»); старый телефон без op (только delta) перестал работать.
func TestRemoteNoForbiddenNone(t *testing.T) {
	r := newTestRelay(t)
	// Интервал во вторник: сегодня (пн) запретных часов нет.
	h, tok := remoteHarness(t, withForbidden(childSID, core.Interval{Weekday: core.Tuesday, StartMin: 14 * 60, EndMin: 16 * 60}))
	id := h.childID(childSID)
	ph := pairPhone(t, h, tok, r)
	res := ph.noForbidden(nil, 1, id, h.real.Now())
	if !res.OK || res.Message != "Сегодня запретных часов нет" {
		t.Fatalf("нет запретных часов: %+v", res)
	}
	if c := reportChild(t, res, id); c.Forbid != remote.ForbidNone {
		t.Fatalf("отчёт: %+v", c)
	}
	if h.engState(id).NoForbidden || len(h.store.eventsOf(core.EvNoForbidden)) != 0 {
		t.Fatal("без запретных часов поставлен флаг или запись журнала")
	}
	if st := h.remoteStored(); st.LastSeq != 1 {
		t.Fatalf("LastSeq %d", st.LastSeq)
	}
	// Старый телефон: команда без op с delta — доп. время, как раньше.
	if res := ph.command(2, id, 15, h.real.Now()); !res.OK || res.Message != "Добавлено 15 минут" {
		t.Fatalf("старая команда: %+v", res)
	}
}

// Ловит: номер «снять запрет» пишется после эффекта, сбой записи номера не останавливает команду,
// чужое поколение связи применяет команду.
func TestRemoteNoForbiddenSeqFirst(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t, withForbidden(childSID, core.Interval{Weekday: core.Monday, StartMin: 14 * 60, EndMin: 16 * 60}))
	pairPhone(t, h, tok, r)
	id := h.childID(childSID)
	l := h.remoteLink()
	host := remoteHost{h.svc}
	h.store.mu.Lock()
	h.store.setSettingErr = func(key string) error {
		if key == settingRemote {
			return errors.New("диск")
		}
		return nil
	}
	h.store.mu.Unlock()
	if msg, code, last := host.ApplyNoForbidden(l.Gen(), 1, id); code != remote.ErrCodeStale || msg != "" || last != 0 {
		t.Fatalf("номер не записан: %q %q %d", msg, code, last)
	}
	if h.engState(id).NoForbidden {
		t.Fatal("запрет снят без записи номера")
	}
	h.store.mu.Lock()
	h.store.setSettingErr = nil
	h.store.mu.Unlock()
	if _, code, _ := host.ApplyNoForbidden(l.Gen()+1000, 1, id); code != remote.ErrCodeStale || h.engState(id).NoForbidden {
		t.Fatalf("чужое поколение: %q", code)
	}
	if msg, code, last := host.ApplyNoForbidden(l.Gen(), 1, id); code != "" || msg != "Запрет на сегодня снят" || last != 1 {
		t.Fatalf("снять запрет: %q %q %d", msg, code, last)
	}
	if _, code, last := host.ApplyNoForbidden(l.Gen(), 1, id); code != remote.ErrCodeReplay || last != 1 {
		t.Fatalf("повтор: %q %d", code, last)
	}
}

// Ловит: статус запретных часов в отчёте телефону расходится с карточкой окна (одна истина):
// «идут» без конца интервала, «будут» на завтрашний интервал, «снят» не виден, без лимита ломается.
func TestRemoteChildrenForbid(t *testing.T) {
	h, tok := remoteHarness(t,
		withChild(child2SID, "Коля", 60),
		withForbidden(childSID, core.Interval{Weekday: core.Monday, StartMin: 14 * 60, EndMin: 16 * 60}),
		// Коля: пн 22:00–07:00 — будет сегодня.
		withForbidden(child2SID, core.Interval{Weekday: core.Monday, StartMin: 22 * 60, EndMin: 7 * 60}),
	)
	h.childLogsIn()
	misha, kolya := h.childID(childSID), h.childID(child2SID)
	byID := func() map[int64]remote.ChildReport {
		out := map[int64]remote.ChildReport{}
		for _, c := range (remoteHost{h.svc}).Children(false) {
			out[c.ID] = c
		}
		return out
	}
	got := byID()
	end := time.Date(2026, 10, 19, 16, 0, 0, 0, tzMSK).UnixMilli()
	if c := got[misha]; c.Forbid != remote.ForbidActive || c.ForbidUntilMs != end {
		t.Fatalf("Миша (идут до 16:00): %+v", c)
	}
	if c := got[kolya]; c.Forbid != remote.ForbidAhead || c.ForbidUntilMs != 0 {
		t.Fatalf("Коля (будут в 22:00): %+v", c)
	}
	// Окно снимает запрет у Миши: отчёт «снят»; плашка ребёнку та же, что с телефона.
	h.guardNotices()
	h.ok(adminPeer(), tok, ipc.CmdSetNoForbidden, ipc.SetNoForbiddenRequest{ChildID: misha, On: true}, nil)
	if c := byID()[misha]; c.Forbid != remote.ForbidCleared || c.ForbidUntilMs != 0 {
		t.Fatalf("Миша после снятия в окне: %+v", c)
	}
	if got := h.guardNotices(); strings.Join(got, "|") != textNoForbiddenNotice {
		t.Fatalf("плашка ребёнку от окна: %q", got)
	}
	if got := h.journalTexts(core.EvNoForbidden); strings.Join(got, "|") != "Миша: Запретные часы сняты на сегодня" {
		t.Fatalf("журнал окна: %q", got)
	}
	// Коля: интервал на завтра (вт) — сегодня запретных часов нет.
	h.ok(adminPeer(), tok, ipc.CmdSaveSchedule, ipc.SaveScheduleRequest{ChildID: kolya, Intervals: []ipc.Interval{{Weekday: 2, StartMin: 1 * 60, EndMin: 2 * 60}}}, nil)
	if c := byID()[kolya]; c.Forbid != remote.ForbidNone {
		t.Fatalf("Коля без запретных часов сегодня: %+v", c)
	}
	// Вернуть запрет в окне — снова «идут».
	h.ok(adminPeer(), tok, ipc.CmdSetNoForbidden, ipc.SetNoForbiddenRequest{ChildID: misha, On: false}, nil)
	if c := byID()[misha]; c.Forbid != remote.ForbidActive || c.ForbidUntilMs != end {
		t.Fatalf("Миша после возврата: %+v", c)
	}
}

func mustNewKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := remote.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}
