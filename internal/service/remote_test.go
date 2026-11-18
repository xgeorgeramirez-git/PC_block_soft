package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/remote"
)

// Связь с телефоном в службе (M7, T4): состояние, команды окна, применение команд с телефона.

func waitRemoteCond(t *testing.T, what string, d time.Duration, cond func() bool) {
	t.Helper()
	end := time.Now().Add(d)
	for !cond() {
		if time.Now().After(end) {
			t.Fatalf("не дождались: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// remoteHarness — настроенная служба в тестовом режиме (http://127.0.0.1 разрешён) с ребёнком
// Миша (лимит 60 мин) и токеном родителя.
func remoteHarness(t *testing.T, opts ...hopt) (*harness, string) {
	t.Helper()
	all := append([]hopt{withChild(childSID, "Миша", 60), configured(), withTestMode(TestMode{ClockSpeed: 1})}, opts...)
	h := newHarness(t, all...)
	h.start()
	t.Cleanup(func() { h.svc.Shutdown(h.real.Now()) })
	return h, h.login(adminPeer())
}

func (h *harness) remoteView(tok string) ipc.RemoteView {
	h.t.Helper()
	var v ipc.RemoteView
	h.ok(adminPeer(), tok, ipc.CmdGetRemote, nil, &v)
	return v
}

func (h *harness) setRemote(tok string, on bool, url string) ipc.RemoteView {
	h.t.Helper()
	var v ipc.RemoteView
	h.ok(adminPeer(), tok, ipc.CmdSetRemote, ipc.SetRemoteRequest{Enabled: on, URL: url}, &v)
	return v
}

func (h *harness) remoteStored() remoteStored {
	h.t.Helper()
	v, ok, _ := h.store.GetSetting(settingRemote)
	var st remoteStored
	if ok {
		if err := json.Unmarshal([]byte(v), &st); err != nil {
			h.t.Fatal(err)
		}
	}
	return st
}

func (h *harness) remoteLink() *remote.Link {
	h.svc.mu.Lock()
	defer h.svc.mu.Unlock()
	return h.svc.rem.link
}

func (h *harness) remoteTexts() []string { return h.journalTexts(evRemote) }

// pairPhone включает связь с relay и привязывает телефон «Pixel 7» полным кругом протокола.
func pairPhone(t *testing.T, h *harness, tok string, r *testRelay) *testPhone {
	t.Helper()
	h.setRemote(tok, true, r.url())
	waitRemoteCond(t, "регистрация компьютера", 5*time.Second, func() bool { return h.remoteView(tok).State == ipc.RemoteStateOnline })
	h.ok(adminPeer(), tok, ipc.CmdRemotePairStart, nil, nil)
	waitRemoteCond(t, "код привязки", 5*time.Second, func() bool { return h.remoteView(tok).Pair.Stage == ipc.PairCode })
	if v := h.remoteView(tok); len(v.Pair.Code) != 6 || v.Pair.ExpiresMs == 0 {
		t.Fatalf("код в окне: %+v", v.Pair)
	}
	ph := newTestPhone(t, r, "phone-1", "Pixel 7")
	sas := ph.pair()
	waitRemoteCond(t, "сверка числа", 5*time.Second, func() bool { return h.remoteView(tok).Pair.Stage == ipc.PairCompare })
	if v := h.remoteView(tok); v.Pair.SAS != sas || v.Pair.PhoneName != "Pixel 7" {
		t.Fatalf("сверка в окне %+v, у телефона %s", v.Pair, sas)
	}
	h.ok(adminPeer(), tok, ipc.CmdRemotePairConfirm, ipc.RemotePairConfirmRequest{Match: true}, nil)
	waitRemoteCond(t, "телефон подключён", 5*time.Second, func() bool { return h.remoteView(tok).Phone != nil })
	st := h.remoteStored()
	if st.Phone == nil || st.Phone.ID != ph.id || st.LastSeq != 0 || st.PCID == "" || st.PCKey == "" || !st.Enabled || st.URL != r.url() {
		t.Fatalf("запись связи после привязки: %+v", st)
	}
	ph.pcID = st.PCID
	return ph
}

// Ловит: связь запускается без включения родителем (S56, NFR-1).
func TestRemoteOffByDefault(t *testing.T) {
	r := newTestRelay(t)
	for _, preset := range []string{"", `{"enabled":false,"url":"` + r.url() + `"}`} {
		h, tok := remoteHarness(t, func(h *harness) {
			if preset != "" {
				h.store.SetSetting(settingRemote, preset)
			}
		})
		for i := 0; i < 3; i++ {
			h.real.Advance(time.Second)
			h.svc.Housekeep(h.real.Now())
		}
		if h.remoteLink() != nil {
			t.Fatalf("%q: связь создана без включения", preset)
		}
		v := h.remoteView(tok)
		if v.Enabled || v.State != ipc.RemoteStateOff || v.Phone != nil || v.Pair.Stage != ipc.PairIdle {
			t.Fatalf("%q: вид %+v", preset, v)
		}
		if preset == "" && v.URL != ipc.RemoteDefaultURL {
			t.Fatalf("адрес по умолчанию %q", v.URL)
		}
		rep := h.fail(adminPeer(), tok, ipc.CmdRemotePairStart, nil)
		if rep.Error != textRemoteOff {
			t.Fatalf("привязка при выключенной связи: %q", rep.Error)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if n := r.count(); n != 0 {
		t.Fatalf("запросов к серверу при выключенной связи: %d", n)
	}
}

// Ловит: выключатель не запускает и не останавливает связь, нет записей журнала, адрес не проверен.
func TestRemoteSetOnOff(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)

	for _, bad := range []string{"http://example.com", "ftp://x", "pcblock.atlas404.ru", "https://"} {
		rep := h.fail(adminPeer(), tok, ipc.CmdSetRemote, ipc.SetRemoteRequest{Enabled: true, URL: bad})
		if rep.Error != "Адрес должен начинаться с https://" || rep.Field != ipc.FieldURL {
			t.Fatalf("%q: отказ %q поле %q", bad, rep.Error, rep.Field)
		}
	}
	if h.remoteLink() != nil || h.remoteStored().Enabled {
		t.Fatal("неверный адрес включил связь")
	}
	// Пустой адрес — адрес по умолчанию (выключено: в сеть не идём).
	if v := h.setRemote(tok, false, "  "); v.URL != ipc.RemoteDefaultURL || v.State != ipc.RemoteStateOff {
		t.Fatalf("пустой адрес: %+v", v)
	}
	if len(h.remoteTexts()) != 0 {
		t.Fatalf("выключение выключенной связи в журнале: %v", h.remoteTexts())
	}

	v := h.setRemote(tok, true, r.url())
	if !v.Enabled || v.URL != r.url() || (v.State != ipc.RemoteStateConnecting && v.State != ipc.RemoteStateOnline) {
		t.Fatalf("включение: %+v", v)
	}
	first := h.remoteLink()
	if first == nil {
		t.Fatal("связь не запущена")
	}
	waitRemoteCond(t, "на связи", 5*time.Second, func() bool { return h.remoteView(tok).State == ipc.RemoteStateOnline })
	if v := h.remoteView(tok); v.LastOKMs == 0 {
		t.Fatalf("нет времени последнего обмена: %+v", v)
	}
	if st := h.remoteStored(); st.PCID == "" || st.PCToken == "" || st.PCKey == "" {
		t.Fatalf("регистрация не записана: %+v", st)
	}
	// То же самое ещё раз — связь не перезапускается.
	h.setRemote(tok, true, r.url())
	if h.remoteLink() != first {
		t.Fatal("связь перезапущена без изменений")
	}
	// Смена адреса — новая связь.
	r2 := newTestRelay(t)
	h.setRemote(tok, true, r2.url())
	if l := h.remoteLink(); l == nil || l == first {
		t.Fatal("смена адреса не перезапустила связь")
	}
	waitRemoteCond(t, "второй сервер", 5*time.Second, func() bool { return r2.count() > 0 })

	v = h.setRemote(tok, false, r2.url())
	if v.Enabled || v.State != ipc.RemoteStateOff || h.remoteLink() != nil {
		t.Fatalf("выключение: %+v", v)
	}
	if !h.svc.waitRemote(3 * time.Second) {
		t.Fatal("горутины связи не закончились")
	}
	n1, n2 := r.count(), r2.count()
	time.Sleep(100 * time.Millisecond)
	if r.count() != n1 || r2.count() != n2 {
		t.Fatal("запросы после выключения связи")
	}
	got := strings.Join(h.remoteTexts(), "|")
	if got != "Связь с телефоном включена|Связь с телефоном выключена" {
		t.Fatalf("журнал: %q", got)
	}
}

// Ловит: доп. время с телефона идёт не общим путём окна (журнал, плашка, предел), «Отменить»
// создаётся или стирается, seq не защищает от повтора, часы связи — часы ядра.
func TestRemoteFullCircle(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)
	h.childLogsIn()
	// Часы ядра ускорены и ушли вперёд: команда подписана настоящим «сейчас».
	h.clock.Advance(3 * time.Hour)
	id := h.childID(childSID)
	ph := pairPhone(t, h, tok, r)
	if got := strings.Join(h.remoteTexts(), "|"); got != "Связь с телефоном включена|Телефон подключён: Pixel 7" {
		t.Fatalf("журнал привязки: %q", got)
	}
	waitRemoteCond(t, "отчёт после привязки", 5*time.Second, func() bool { return r.reportCount() > 0 })

	// Окно родителя: +10, «Отменить» живо.
	var add ipc.AddExtraReply
	h.ok(adminPeer(), tok, ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: id, Minutes: 10}, &add)
	h.svc.mu.Lock()
	undo := h.svc.children[id].undo
	delete(h.svc.guardCmd, childSession)
	h.svc.mu.Unlock()
	if undo == nil || add.ExtraID == "" {
		t.Fatal("окно не создало «Отменить»")
	}

	res := ph.command(1, id, 15, h.real.Now())
	if !res.OK || res.Message != "Добавлено 15 минут" || res.Report == nil {
		t.Fatalf("+15: %+v", res)
	}
	if st := h.engState(id); st.ExtraMin != 25 {
		t.Fatalf("доп. время %d, ждали 25", st.ExtraMin)
	}
	h.svc.mu.Lock()
	notes := h.svc.guardCmd[childSession]
	sameUndo := h.svc.children[id].undo == undo
	h.svc.mu.Unlock()
	if len(notes) != 1 || notes[0].Kind != ipc.GuardCmdNotice || notes[0].Text != "Добавлено 15 минут" {
		t.Fatalf("плашка ребёнку: %+v", notes)
	}
	if !sameUndo {
		t.Fatal("команда с телефона тронула «Отменить» окна")
	}
	if st := h.remoteStored(); st.LastSeq != 1 {
		t.Fatalf("LastSeq %d", st.LastSeq)
	}
	// «Отменить» окна отменяет только своё +10.
	h.ok(adminPeer(), tok, ipc.CmdUndoExtra, ipc.UndoExtraRequest{ChildID: id, ExtraID: add.ExtraID}, nil)
	if st := h.engState(id); st.ExtraMin != 15 {
		t.Fatalf("после «Отменить» окна %d, ждали 15", st.ExtraMin)
	}

	// Повтор того же seq — replay, ничего не применено.
	if res := ph.command(1, id, 15, h.real.Now()); res.OK || res.Error != remote.ErrCodeReplay || res.LastSeq == nil || *res.LastSeq != 1 {
		t.Fatalf("повтор: %+v", res)
	}
	// −240 при 15 доп. и лимите 60: убраны фактические 75 (лимит дня не ниже 0).
	h.svc.mu.Lock()
	delete(h.svc.guardCmd, childSession)
	h.svc.mu.Unlock()
	if res := ph.command(2, id, -240, h.real.Now()); !res.OK || res.Message != "Убрано 75 минут" {
		t.Fatalf("−240: %+v", res)
	}
	h.svc.mu.Lock()
	notes = h.svc.guardCmd[childSession]
	h.svc.mu.Unlock()
	if len(notes) != 1 || notes[0].Text != "Убрано 75 минут" {
		t.Fatalf("плашка при «−»: %+v", notes)
	}
	// Убирать нечего — ok с сообщением.
	if res := ph.command(3, id, -5, h.real.Now()); !res.OK || res.Message != "Убирать нечего" {
		t.Fatalf("−5 без доп. времени: %+v", res)
	}
	before := len(h.store.eventsOf(core.EvExtraTime))
	if res := ph.command(4, id, 241, h.real.Now()); res.OK || res.Error != remote.ErrCodeRange {
		t.Fatalf("+241: %+v", res)
	}
	if res := ph.command(5, 999, 15, h.real.Now()); res.OK || res.Error != remote.ErrCodeNoChild {
		t.Fatalf("нет ребёнка: %+v", res)
	}
	if st := h.engState(id); st.ExtraMin != -60 || len(h.store.eventsOf(core.EvExtraTime)) != before {
		t.Fatalf("отказ что-то записал: доп. %d", st.ExtraMin)
	}
	want := []string{"Миша: доп. время +10 мин", "Миша: вы добавили 15 мин с телефона", "Миша: вы убавили 75 мин с телефона"}
	if got := h.journalTexts(core.EvExtraTime); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("журнал доп. времени: %q", got)
	}

	// Перезапуск службы: телефон, ключ и LastSeq на месте, связь поднимается сама.
	stored := h.remoteStored()
	h.svc.Shutdown(h.real.Now())
	h2 := newHarness(t, withStore(h.store), withTestMode(TestMode{ClockSpeed: 1}))
	h2.start()
	t.Cleanup(func() { h2.svc.Shutdown(h2.real.Now()) })
	tok2 := h2.login(adminPeer())
	if st := h2.remoteStored(); st.LastSeq != 5 || st.Phone == nil || st.Phone.ID != ph.id || st.PCKey != stored.PCKey {
		t.Fatalf("после перезапуска: %+v", st)
	}
	if h2.remoteLink() == nil {
		t.Fatal("включённая связь не поднялась после перезапуска")
	}
	waitRemoteCond(t, "на связи после перезапуска", 5*time.Second, func() bool { return h2.remoteView(tok2).State == ipc.RemoteStateOnline })
	if res := ph.command(5, id, 15, h2.real.Now()); res.OK || res.Error != remote.ErrCodeReplay {
		t.Fatalf("повтор после перезапуска: %+v", res)
	}
	if res := ph.command(6, id, 15, h2.real.Now()); !res.OK {
		t.Fatalf("новая команда после перезапуска: %+v", res)
	}

	// «Отвязать»: окно сразу без телефона, связь гасит токен на сервере и пишет журнал.
	v := ipc.RemoteView{}
	h2.ok(adminPeer(), tok2, ipc.CmdRemoteUnpair, nil, &v)
	if v.Phone != nil {
		t.Fatalf("окно показывает отвязанный телефон: %+v", v.Phone)
	}
	waitRemoteCond(t, "гашение токена телефона", 5*time.Second, func() bool { return len(r.unpaired()) == 1 && r.unpaired()[0] == ph.id })
	waitRemoteCond(t, "отвязка записана", 5*time.Second, func() bool { return h2.remoteStored().Phone == nil })
	if st := h2.remoteStored(); st.LastSeq != 0 {
		t.Fatalf("LastSeq после отвязки %d", st.LastSeq)
	}
	texts := h2.remoteTexts()
	if texts[len(texts)-1] != "Телефон отвязан" {
		t.Fatalf("журнал отвязки: %q", texts)
	}
	if res := ph.command(7, id, 15, h2.real.Now()); res.OK || res.Error != remote.ErrCodeNotPaired {
		t.Fatalf("команда отвязанного телефона: %+v", res)
	}
}

// Ловит: связь берёт часы ядра — в тестовом режиме они ускорены и стоят в другом времени,
// команда с настоящим ts отклоняется как устаревшая.
func TestRemoteRealClock(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)
	h.clock.Advance(48 * time.Hour)
	ph := pairPhone(t, h, tok, r)
	if res := ph.command(1, h.childID(childSID), 5, h.real.Now()); !res.OK {
		t.Fatalf("команда по настоящим часам: %+v", res)
	}
	if res := ph.command(2, h.childID(childSID), 5, h.clock.Now()); res.OK || res.Error != remote.ErrCodeStale {
		t.Fatalf("команда по часам ядра: %+v", res)
	}
}

// Ловит: номер команды пишется после доп. времени или сбой записи номера не останавливает команду.
func TestRemoteApplySeqFirst(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)
	ph := pairPhone(t, h, tok, r)
	id := h.childID(childSID)
	l := h.remoteLink()
	host := remoteHost{h.svc}

	failSeq := errors.New("диск")
	h.store.mu.Lock()
	h.store.setSettingErr = func(key string) error {
		if key == settingRemote {
			return failSeq
		}
		return nil
	}
	h.store.mu.Unlock()
	if msg, code, last := host.ApplyCommand(l.Gen(), 1, id, 15); code != remote.ErrCodeStale || msg != "" || last != 0 {
		t.Fatalf("номер не записан: %q %q %d", msg, code, last)
	}
	if st := h.engState(id); st.ExtraMin != 0 {
		t.Fatalf("команда применена без записи номера: %d", st.ExtraMin)
	}
	h.store.mu.Lock()
	h.store.setSettingErr = nil
	h.store.mu.Unlock()

	// Старое поколение — stale без записи.
	if _, code, _ := host.ApplyCommand(l.Gen()+1000, 1, id, 15); code != remote.ErrCodeStale {
		t.Fatalf("чужое поколение: %q", code)
	}
	if err := host.SaveState(l.Gen()+1000, remote.State{}); !errors.Is(err, remote.ErrStale) {
		t.Fatalf("SaveState чужого поколения: %v", err)
	}
	if st := h.remoteStored(); st.LastSeq != 0 || st.Phone == nil {
		t.Fatalf("чужое поколение записало: %+v", st)
	}
	if msg, code, last := host.ApplyCommand(l.Gen(), 1, id, 15); code != "" || msg != "Добавлено 15 минут" || last != 1 {
		t.Fatalf("+15: %q %q %d", msg, code, last)
	}
	// +241 и −241 — range, номер израсходован, доп. время не тронуто.
	for i, d := range []int{241, -241, 0} {
		seq := int64(2 + i)
		if _, code, last := host.ApplyCommand(l.Gen(), seq, id, d); code != remote.ErrCodeRange || last != seq {
			t.Fatalf("%+d: %q %d", d, code, last)
		}
	}
	if st := h.engState(id); st.ExtraMin != 15 {
		t.Fatalf("доп. время %d", st.ExtraMin)
	}
	_ = ph
	// Связь выключена — поколение старое.
	gen := l.Gen()
	h.setRemote(tok, false, r.url())
	if _, code, _ := host.ApplyCommand(gen, 10, id, 15); code != remote.ErrCodeStale {
		t.Fatalf("после выключения: %q", code)
	}
}

// Ловит: отчёт телефону считает остаток не так, как карточка окна (одна истина), окно меняет
// доп. время, а телефон об этом не узнаёт.
func TestRemoteChildrenAndChanged(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t, withChild(child2SID, "Коля", 30))
	h.childLogsIn()
	h.run(90, activeSample)
	id := h.childID(childSID)
	var today ipc.GetTodayReply
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, &today)
	got := remoteHost{h.svc}.Children()
	if len(got) != 2 || len(today.Children) != 2 {
		t.Fatalf("детей %d/%d", len(got), len(today.Children))
	}
	for i, c := range today.Children {
		g := got[i]
		if g.ID != c.ChildID || g.Name != c.Name || g.LeftSec != int64(c.Status.RemainingS) || g.UsedSec != int64(c.Status.UsedS) || g.ExtraMin != c.ExtraMin || g.Blocked {
			t.Fatalf("ребёнок %d: отчёт %+v, карточка %+v", i, g, c.Status)
		}
	}
	if got[0].UsedSec != 90 || got[0].LeftSec != 3600-90 {
		t.Fatalf("Миша: %+v", got[0])
	}

	pairPhone(t, h, tok, r)
	waitRemoteCond(t, "отчёт после привязки", 5*time.Second, func() bool { return r.reportCount() > 0 })
	time.Sleep(1500 * time.Millisecond) // следующий отчёт по расписанию — через 60 с
	n := r.reportCount()
	h.ok(adminPeer(), tok, ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: id, Minutes: 5}, nil)
	waitRemoteCond(t, "отчёт после изменения в окне", 5*time.Second, func() bool { return r.reportCount() > n })
}

// Ловит взаимоблокировку: обработчик выключает связь, пока горутина связи ждёт s.mu в
// ApplyCommand (Wait под s.mu завис бы навсегда).
func TestRemoteStopWhileApplyWaits(t *testing.T) {
	r := newTestRelay(t)
	entered, release := make(chan struct{}), make(chan struct{})
	h, tok := remoteHarness(t)
	// Крючки — до запуска связи: горутины связи читают их без s.mu.
	h.svc.mu.Lock()
	h.svc.rem.beforeApplyLock = func() { close(entered); <-release }
	h.svc.rem.beforeStop = func() {
		close(release)
		time.Sleep(50 * time.Millisecond) // горутина связи дошла до s.mu и ждёт
	}
	h.svc.mu.Unlock()
	ph := pairPhone(t, h, tok, r)
	id := h.childID(childSID)

	go ph.r.sendFrom(ph.id, remote.KindCmd, mustSignedCmd(t, ph, 1, id, 15, h.real.Now()), 3*time.Second)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("команда не дошла до ApplyCommand")
	}
	done := make(chan struct{})
	go func() {
		h.setRemote(tok, false, r.url())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("выключение связи зависло")
	}
	if !h.svc.waitRemote(3 * time.Second) {
		t.Fatal("горутины связи не закончились")
	}
	if st := h.engState(id); st.ExtraMin != 0 {
		t.Fatalf("команда старого поколения применена: %d", st.ExtraMin)
	}
	if st := h.remoteStored(); st.LastSeq != 0 {
		t.Fatalf("старое поколение записало номер: %d", st.LastSeq)
	}
}

func mustSignedCmd(t *testing.T, p *testPhone, seq, child int64, delta int, ts time.Time) remote.Signed {
	raw, _ := json.Marshal(remote.Command{V: remote.ProtoVersion, T: remote.TypeCmd, PC: p.pcID, Phone: p.id, Seq: seq, TS: ts.UnixMilli(), Child: child, Delta: delta})
	s, err := remote.Sign(p.key, raw)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Ловит: «Отвязать» при выключенной связи не стирает телефон или не пишет журнал; «Не совпадает»
// не отменяет привязку.
func TestRemoteUnpairOffAndMismatch(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)
	pairPhone(t, h, tok, r)
	h.setRemote(tok, false, r.url())
	var v ipc.RemoteView
	h.ok(adminPeer(), tok, ipc.CmdRemoteUnpair, nil, &v)
	if v.Phone != nil || h.remoteStored().Phone != nil {
		t.Fatal("телефон не стёрт при выключенной связи")
	}
	texts := h.remoteTexts()
	if texts[len(texts)-1] != "Телефон отвязан" {
		t.Fatalf("журнал: %q", texts)
	}

	h.setRemote(tok, true, r.url())
	waitRemoteCond(t, "на связи", 5*time.Second, func() bool { return h.remoteView(tok).State == ipc.RemoteStateOnline })
	h.ok(adminPeer(), tok, ipc.CmdRemotePairStart, nil, nil)
	waitRemoteCond(t, "код", 5*time.Second, func() bool { return h.remoteView(tok).Pair.Stage == ipc.PairCode })
	ph := newTestPhone(t, r, "phone-2", "Galaxy")
	ph.pair()
	waitRemoteCond(t, "сверка", 5*time.Second, func() bool { return h.remoteView(tok).Pair.Stage == ipc.PairCompare })
	h.ok(adminPeer(), tok, ipc.CmdRemotePairConfirm, ipc.RemotePairConfirmRequest{Match: false}, nil)
	waitRemoteCond(t, "отмена", 5*time.Second, func() bool {
		p := h.remoteView(tok).Pair
		return p.Stage == ipc.PairFailed && p.Error == ipc.PairErrMismatch
	})
	if h.remoteStored().Phone != nil {
		t.Fatal("«Не совпадает» записало телефон")
	}
}

// Ловит: неверный текст журнала доп. времени с телефона и событий связи.
func TestRemoteEventTexts(t *testing.T) {
	cases := []struct {
		kind, name string
		d          map[string]any
		want       string
	}{
		{core.EvExtraTime, "Миша", map[string]any{"minutes": 15, "source": "phone"}, "Миша: вы добавили 15 мин с телефона"},
		{core.EvExtraTime, "Миша", map[string]any{"minutes": float64(-15), "source": "phone"}, "Миша: вы убавили 15 мин с телефона"},
		{core.EvExtraTime, "Миша", map[string]any{"minutes": 15}, "Миша: доп. время +15 мин"},
		{evRemote, "", map[string]any{"action": "on"}, "Связь с телефоном включена"},
		{evRemote, "", map[string]any{"action": "off"}, "Связь с телефоном выключена"},
		{evRemote, "", map[string]any{"action": "paired", "phone": "Pixel 7"}, "Телефон подключён: Pixel 7"},
		{evRemote, "", map[string]any{"action": "unpaired", "phone": "Pixel 7"}, "Телефон отвязан"},
		{evRemote, "", map[string]any{"action": "lost", "phone": "Pixel 7"}, "Телефон отвязан: сервер связи не узнал компьютер"},
	}
	for _, c := range cases {
		if got := eventText(c.kind, c.name, c.d); got != c.want {
			t.Errorf("%s %v: %q, ждали %q", c.kind, c.d, got, c.want)
		}
	}
}

// Ловит: этапы привязки internal/remote не переведены в этапы окна.
func TestRemotePairView(t *testing.T) {
	s := &Service{}
	cases := []struct {
		in   remote.PairInfo
		want ipc.RemotePair
	}{
		{remote.PairInfo{Stage: remote.StageIdle}, ipc.RemotePair{Stage: ipc.PairIdle}},
		{remote.PairInfo{Stage: remote.StageCode}, ipc.RemotePair{Stage: ipc.PairStarting}},
		{remote.PairInfo{Stage: remote.StageCode, Code: "123456", ExpiresMs: 9}, ipc.RemotePair{Stage: ipc.PairCode, Code: "123456", ExpiresMs: 9}},
		{remote.PairInfo{Stage: remote.StageWaitNonce, ExpiresMs: 20}, ipc.RemotePair{Stage: ipc.PairCode, Code: "123456", ExpiresMs: 9}},
		{remote.PairInfo{Stage: remote.StageCompare, SAS: "482913", PhoneName: "P", ExpiresMs: 30}, ipc.RemotePair{Stage: ipc.PairCompare, SAS: "482913", PhoneName: "P", ExpiresMs: 30}},
		{remote.PairInfo{Stage: remote.StageWaitNonce}, ipc.RemotePair{Stage: ipc.PairStarting}},
		{remote.PairInfo{Stage: remote.StageDone, PhoneName: "P"}, ipc.RemotePair{Stage: ipc.PairIdle}},
		{remote.PairInfo{Stage: remote.StageFailed, Error: remote.PairErrExpired}, ipc.RemotePair{Stage: ipc.PairFailed, Error: ipc.PairErrExpired}},
		{remote.PairInfo{Stage: remote.StageFailed, Error: remote.PairErrMismatch}, ipc.RemotePair{Stage: ipc.PairFailed, Error: ipc.PairErrMismatch}},
		{remote.PairInfo{Stage: remote.StageFailed, Error: remote.PairErrNoServer}, ipc.RemotePair{Stage: ipc.PairFailed, Error: ipc.PairErrNoServer}},
		{remote.PairInfo{Stage: remote.StageFailed, Error: remote.PairErrBadPhone}, ipc.RemotePair{Stage: ipc.PairFailed, Error: ipc.PairErrAborted}},
		{remote.PairInfo{Stage: remote.StageFailed, Error: remote.PairErrStopped}, ipc.RemotePair{Stage: ipc.PairIdle}},
	}
	for i, c := range cases {
		if got := s.pairView(c.in); got != c.want {
			t.Errorf("%d %+v: %+v, ждали %+v", i, c.in, got, c.want)
		}
	}
	for code, want := range map[string]string{
		remote.StatusNoServer: ipc.RemoteErrNoServer, remote.StatusTooMany: ipc.RemoteErrNoServer,
		remote.StatusBadAddress: ipc.RemoteErrBadAddress, remote.StatusBadAnswer: ipc.RemoteErrBadAddress,
		remote.StatusUnauthorized: ipc.RemoteErrLost,
	} {
		if got := remoteErrOf(code); got != want {
			t.Errorf("%s: %s, ждали %s", code, got, want)
		}
	}
}
