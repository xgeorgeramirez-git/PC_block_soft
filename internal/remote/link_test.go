package remote

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------- поддельные часы, сон и Host ----------

type fakeClock struct{ ms atomic.Int64 }

func newClock() *fakeClock {
	c := &fakeClock{}
	c.ms.Store(1763490000000)
	return c
}
func (c *fakeClock) Now() time.Time           { return time.UnixMilli(c.ms.Load()) }
func (c *fakeClock) Add(d time.Duration)      { c.ms.Add(d.Milliseconds()) }
func (c *fakeClock) Ms() int64                { return c.ms.Load() }
func (c *fakeClock) At(d time.Duration) int64 { return c.ms.Load() + d.Milliseconds() }

type sleeper struct {
	mu   sync.Mutex
	got  []time.Duration
	hook func(time.Duration)
}

func (s *sleeper) Sleep(ctx context.Context, d time.Duration) bool {
	s.mu.Lock()
	s.got = append(s.got, d)
	hook := s.hook
	s.mu.Unlock()
	if hook != nil {
		hook(d)
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(time.Millisecond):
		return true
	}
}

func (s *sleeper) list() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.got...)
}

// fakeHost — как служба: своя блокировка mu (s.mu), каждый метод берёт её сам.
type fakeHost struct {
	mu         sync.Mutex
	link       *Link
	st         State
	saves      int
	staleSaves int
	applied    []string
	events     []string
	logs       []string
	children   []ChildReport
	onApply    func()
	beforeLock func() // ApplyCommand до блокировки
	forceStale atomic.Bool
	panicOnce  atomic.Bool
	applyEnter atomic.Int32
	loadEnter  atomic.Int32
}

func newFakeHost() *fakeHost {
	return &fakeHost{children: []ChildReport{{ID: 3, Name: "Миша", UsedSec: 60, LeftSec: 600}}}
}

func cloneState(st State) State {
	if st.Phone != nil {
		p := *st.Phone
		st.Phone = &p
	}
	return st
}

func (h *fakeHost) LoadState() State {
	h.loadEnter.Add(1)
	h.mu.Lock()
	defer h.mu.Unlock()
	return cloneState(h.st)
}

func (h *fakeHost) SaveState(gen int64, st State) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if gen != h.link.Gen() || h.forceStale.Load() {
		h.staleSaves++
		return ErrStale
	}
	h.st = cloneState(st)
	h.saves++
	return nil
}

func (h *fakeHost) ApplyCommand(gen, seq, childID int64, delta int) (string, string, int64) {
	h.applyEnter.Add(1)
	if h.beforeLock != nil {
		h.beforeLock()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.onApply != nil {
		h.onApply()
	}
	if gen != h.link.Gen() {
		return "", ErrCodeStale, h.st.LastSeq
	}
	if seq <= h.st.LastSeq {
		return "", ErrCodeReplay, h.st.LastSeq
	}
	h.st.LastSeq = seq
	if childID != 3 {
		return "", ErrCodeNoChild, seq
	}
	if delta == 0 || delta > 240 || delta < -240 {
		return "", ErrCodeRange, seq
	}
	h.applied = append(h.applied, fmt.Sprintf("%d:%+d", seq, delta))
	return fmt.Sprintf("Добавлено %d минут", delta), "", seq
}

func (h *fakeHost) Children() []ChildReport {
	if h.panicOnce.CompareAndSwap(true, false) {
		panic("сбой в Children")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]ChildReport(nil), h.children...)
}

func (h *fakeHost) Event(kind, detail string) {
	h.mu.Lock()
	h.events = append(h.events, kind+":"+detail)
	h.mu.Unlock()
}

func (h *fakeHost) Logf(format string, args ...any) {
	h.mu.Lock()
	h.logs = append(h.logs, fmt.Sprintf(format, args...))
	h.mu.Unlock()
}

func (h *fakeHost) get() (st State, applied, events []string, stale int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return cloneState(h.st), append([]string(nil), h.applied...), append([]string(nil), h.events...), h.staleSaves
}

// ---------- запуск ----------

type rig struct {
	t     *testing.T
	r     *miniRelay
	h     *fakeHost
	l     *Link
	clock *fakeClock
	sl    *sleeper
	tick  time.Duration
}

func newRig(t *testing.T, prep func(*rig)) *rig {
	t.Helper()
	g := &rig{t: t, r: newMiniRelay(t), h: newFakeHost(), clock: newClock(), sl: &sleeper{}, tick: 5 * time.Millisecond}
	if prep != nil {
		prep(g)
	}
	u, err := ValidateURL(g.r.srv.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	g.l = NewLink(Config{URL: u, Host: g.h, Now: g.clock.Now, Sleep: g.sl.Sleep, ReportEvery: time.Hour, tick: g.tick})
	g.h.link = g.l
	t.Cleanup(func() {
		g.l.Stop()
		if !g.l.Wait(2 * time.Second) {
			t.Error("горутины связи не завершились")
		}
	})
	return g
}

func (g *rig) start() int64 {
	gen := g.l.Start()
	waitFor(g.t, "регистрация и опрос", func() bool { return g.l.Status().Online })
	return gen
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("не дождались: %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// pairedState — компьютер уже зарегистрирован на мини-ящике и привязан к телефону ph1.
func pairedState(t *testing.T, r *miniRelay, phone *ecdsa.PrivateKey, lastSeq int64) (State, *ecdsa.PrivateKey) {
	pcKey := mustKey(t)
	priv, _ := MarshalPriv(pcKey)
	r.mu.Lock()
	r.pcs["tok-x"] = "pc-x"
	r.mu.Unlock()
	return State{PCID: "pc-x", PCToken: "tok-x", PCKey: priv, LastSeq: lastSeq,
		Phone: &Phone{ID: "ph1", Name: "Pixel", Pub: MarshalPub(&phone.PublicKey), PairedAtMs: 1}}, pcKey
}

// ---------- тестовый телефон ----------

type testPhone struct {
	t      *testing.T
	r      *miniRelay
	id     string
	key    *ecdsa.PrivateKey
	pcPub  []byte
	commit []byte
	nPhone []byte
	sas    string
}

func newPhone(t *testing.T, r *miniRelay, id string) *testPhone {
	return &testPhone{t: t, r: r, id: id, key: mustKey(t)}
}

func (p *testPhone) pubDER() []byte {
	der, _ := unb64(MarshalPub(&p.key.PublicKey))
	return der
}

const noReply = 150 * time.Millisecond

func (p *testPhone) pair1(wantReply bool) {
	p.t.Helper()
	wait := 2 * time.Second
	if !wantReply {
		wait = noReply
	}
	rep, ok := p.r.sendFrom(p.id, KindPair1, Pair1{PhonePub: MarshalPub(&p.key.PublicKey), Name: "Pixel 7"}, wait)
	if ok != wantReply {
		p.t.Fatalf("pair1 от %s: ответ %v, ожидали %v", p.id, ok, wantReply)
	}
	if !ok {
		return
	}
	var b Pair1R
	if rep.Kind != KindPair1R || json.Unmarshal(rep.Body, &b) != nil {
		p.t.Fatalf("ответ на pair1: %+v", rep)
	}
	p.pcPub, _ = unb64(b.PCPub)
	p.commit, _ = unb64(b.Commit)
}

func (p *testPhone) pair2(wantReply bool) {
	p.t.Helper()
	wait := 2 * time.Second
	if !wantReply {
		wait = noReply
	}
	p.nPhone = make([]byte, NonceLen)
	rand.Read(p.nPhone)
	rep, ok := p.r.sendFrom(p.id, KindPair2, Pair2{NPhone: b64(p.nPhone)}, wait)
	if ok != wantReply {
		p.t.Fatalf("pair2 от %s: ответ %v, ожидали %v", p.id, ok, wantReply)
	}
	if !ok {
		return
	}
	var b Pair2R
	if rep.Kind != KindPair2R || json.Unmarshal(rep.Body, &b) != nil {
		p.t.Fatalf("ответ на pair2: %+v", rep)
	}
	nPc, _ := unb64(b.NPc)
	if !bytes.Equal(Commit(p.pcPub, p.pubDER(), nPc), p.commit) {
		p.sas = "обязательство не сошлось"
		return
	}
	p.sas = SAS(p.pcPub, p.pubDER(), nPc, p.nPhone)
}

func (g *rig) startPairing() string {
	g.t.Helper()
	g.l.StartPairing()
	waitFor(g.t, "код привязки", func() bool { return g.l.PairInfo().Code != "" })
	pi := g.l.PairInfo()
	if pi.Stage != StageCode || pi.Code != g.r.currentCode() || pi.ExpiresMs != g.clock.At(10*time.Minute) {
		g.t.Fatalf("код: %+v (на сервере %s)", pi, g.r.currentCode())
	}
	return pi.Code
}

func (g *rig) waitStage(stage string) PairInfo {
	g.t.Helper()
	waitFor(g.t, "этап "+stage, func() bool { return g.l.PairInfo().Stage == stage })
	return g.l.PairInfo()
}

// toCompare — привязка дошла до сверки числа.
func (g *rig) toCompare(p *testPhone) {
	g.t.Helper()
	g.startPairing()
	p.pair1(true)
	if pi := g.l.PairInfo(); pi.Stage != StageWaitNonce || pi.Code != "" {
		g.t.Fatalf("после pair1: %+v", pi)
	}
	p.pair2(true)
	pi := g.waitStage(StageCompare)
	if pi.SAS != p.sas || len(pi.SAS) != 6 || pi.PhoneName != "Pixel 7" {
		g.t.Fatalf("числа сверки: компьютер %q, телефон %q (%+v)", pi.SAS, p.sas, pi)
	}
}

// ---------- привязка ----------

func TestLinkPairingHappy(t *testing.T) {
	oldPhone := mustKey(t)
	var pcKey *ecdsa.PrivateKey
	g := newRig(t, func(g *rig) { g.h.st, pcKey = pairedState(t, g.r, oldPhone, 9) })
	g.start()
	p := newPhone(t, g.r, "ph-new")
	g.toCompare(p)
	pcDER, _ := unb64(MarshalPub(&pcKey.PublicKey))
	if !bytes.Equal(p.pcPub, pcDER) {
		t.Fatal("в pair1r не ключ компьютера")
	}
	_, _, unpairs, _ := g.r.snapshot()
	if len(unpairs) != 0 {
		t.Fatalf("прежний телефон отвязан до «Совпадает»: %v", unpairs)
	}
	g.l.ConfirmPairing(true)
	g.waitStage(StageDone)
	st, _, events, _ := g.h.get()
	if st.Phone == nil || st.Phone.ID != "ph-new" || st.Phone.Name != "Pixel 7" || st.LastSeq != 0 || st.Phone.PairedAtMs != g.clock.Ms() {
		t.Fatalf("состояние после привязки: %+v %+v", st, st.Phone)
	}
	if !bytes.Equal(mustUnb64(t, st.Phone.Pub), p.pubDER()) {
		t.Fatal("записан не тот ключ телефона")
	}
	if len(events) != 1 || events[0] != "paired:Pixel 7" {
		t.Fatalf("события: %v", events)
	}
	waitFor(t, "отвязка прежнего и отчёт", func() bool {
		_, _, u, reps := g.r.snapshot()
		return len(u) == 1 && u[0] == "ph1" && len(reps) >= 2
	})
	_, _, _, reps := g.r.snapshot()
	data, err := Verify(&pcKey.PublicKey, reps[len(reps)-1])
	if err != nil {
		t.Fatal(err)
	}
	var rep Report
	json.Unmarshal(data, &rep)
	if rep.T != TypeReport || rep.Phone != "ph-new" || rep.PC != "pc-x" || len(rep.Children) != 1 {
		t.Fatalf("отчёт после привязки: %+v", rep)
	}
	// Подтверждение после «done» ничего не меняет.
	g.l.ConfirmPairing(true)
	g.l.ConfirmPairing(false)
	time.Sleep(20 * time.Millisecond)
	if st2, _, ev, _ := g.h.get(); st2.Phone.ID != "ph-new" || len(ev) != 1 {
		t.Fatal("повторное подтверждение что-то изменило")
	}
}

func mustUnb64(t *testing.T, s string) []byte {
	b, err := unb64(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Конечный автомат привязки (протокол, раздел 4): любое нарушение порядка отменяет привязку
// и гасит токен телефона текущей привязки.
func TestLinkPairingViolations(t *testing.T) {
	cases := []struct {
		name    string
		run     func(g *rig, p *testPhone)
		err     string
		unpairs []string
	}{
		{"второй pair1", func(g *rig, p *testPhone) {
			g.startPairing()
			p.pair1(true)
			newPhone(g.t, g.r, "ph2").pair1(false)
		}, PairErrAborted, []string{"ph1"}},
		{"второй pair1 на сверке", func(g *rig, p *testPhone) {
			g.toCompare(p)
			newPhone(g.t, g.r, "ph2").pair1(false)
		}, PairErrAborted, []string{"ph1"}},
		{"второй pair2", func(g *rig, p *testPhone) {
			g.toCompare(p)
			p.pair2(false)
		}, PairErrAborted, []string{"ph1"}},
		{"pair2 от чужого phoneId", func(g *rig, p *testPhone) {
			g.startPairing()
			p.pair1(true)
			newPhone(g.t, g.r, "ph2").pair2(false)
		}, PairErrAborted, []string{"ph1"}},
		{"pair2 до pair1r", func(g *rig, p *testPhone) {
			g.startPairing()
			p.pair2(false)
		}, PairErrAborted, nil},
		{"неверный pair1", func(g *rig, p *testPhone) {
			g.startPairing()
			if _, ok := g.r.sendFrom("ph1", KindPair1, map[string]string{"phonePub": "AAAA"}, noReply); ok {
				g.t.Fatal("ответ на неверный pair1")
			}
		}, PairErrBadPhone, nil},
		{"не совпадает", func(g *rig, p *testPhone) {
			g.toCompare(p)
			g.l.ConfirmPairing(false)
		}, PairErrMismatch, []string{"ph1"}},
		{"сверка 10 мин", func(g *rig, p *testPhone) {
			g.toCompare(p)
			g.clock.Add(10*time.Minute + time.Second)
		}, PairErrExpired, []string{"ph1"}},
		{"ожидание pair2 10 мин", func(g *rig, p *testPhone) {
			g.startPairing()
			p.pair1(true)
			g.clock.Add(10*time.Minute + time.Second)
		}, PairErrExpired, []string{"ph1"}},
		{"код 10 мин", func(g *rig, p *testPhone) {
			g.startPairing()
			g.clock.Add(10*time.Minute + time.Second)
			g.waitStage(StageFailed)
			p.pair1(false) // истёкший код не принимается
		}, PairErrExpired, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			g := newRig(t, nil)
			g.start()
			p := newPhone(t, g.r, "ph1")
			c.run(g, p)
			pi := g.waitStage(StageFailed)
			if pi.Error != c.err || pi.Code != "" || pi.SAS != "" {
				t.Fatalf("итог привязки: %+v, нужна ошибка %s", pi, c.err)
			}
			if c.unpairs != nil {
				waitFor(t, "отвязка брошенного телефона", func() bool { _, _, u, _ := g.r.snapshot(); return len(u) >= len(c.unpairs) })
			}
			time.Sleep(20 * time.Millisecond)
			_, _, unpairs, _ := g.r.snapshot()
			if strings.Join(unpairs, ",") != strings.Join(c.unpairs, ",") {
				t.Fatalf("отвязаны %v, нужно %v", unpairs, c.unpairs)
			}
			// «Совпадает» после отмены не привязывает.
			g.l.ConfirmPairing(true)
			p.pair2(false)
			time.Sleep(20 * time.Millisecond)
			if st, _, ev, _ := g.h.get(); st.Phone != nil || len(ev) != 0 {
				t.Fatalf("после отмены привязан телефон: %+v %v", st.Phone, ev)
			}
		})
	}
}

func TestLinkCancelPairing(t *testing.T) {
	g := newRig(t, nil)
	g.start()
	p := newPhone(t, g.r, "ph1")
	g.startPairing()
	p.pair1(true)
	g.l.CancelPairing()
	if pi := g.waitStage(StageIdle); pi.Error != "" {
		t.Fatalf("%+v", pi)
	}
	waitFor(t, "отвязка", func() bool { _, _, u, _ := g.r.snapshot(); return len(u) == 1 && u[0] == "ph1" })
	// Новая привязка после отмены — новый код.
	g.startPairing()
	if _, codes, _, _ := g.r.snapshot(); codes != 2 {
		t.Fatalf("кодов %d", codes)
	}
}

// Код привязки виден ≤ 2 с, даже если опрос висит (отдельная горутина отправки).
func TestPairCodeWhilePollHangs(t *testing.T) {
	g := newRig(t, nil)
	g.start()
	g.r.mu.Lock()
	g.r.hangPoll = true
	n := g.r.polls
	g.r.mu.Unlock()
	waitFor(t, "висящий опрос", func() bool { g.r.mu.Lock(); defer g.r.mu.Unlock(); return g.r.polls > n })
	start := time.Now()
	g.l.StartPairing()
	if pi := g.l.PairInfo(); pi.Stage != StageCode {
		t.Fatalf("StartPairing сразу не показал этап кода: %+v", pi)
	}
	for g.l.PairInfo().Code == "" {
		if time.Since(start) > 2*time.Second {
			t.Fatal("код не появился за 2 с при висящем опросе")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// Посредник подменяет ключи в обе стороны: числа сверки на компьютере и телефоне различаются.
func TestLinkMITM(t *testing.T) {
	g := newRig(t, nil)
	g.start()
	mitmPhone, mitmPC := mustKey(t), mustKey(t)
	mitmPCDER := mustUnb64(t, MarshalPub(&mitmPC.PublicKey))
	nFake := bytes.Repeat([]byte{7}, NonceLen)
	var realPhoneDER []byte
	g.r.mu.Lock()
	g.r.toPC = func(m *Msg) {
		if m.Kind == KindPair1 {
			var b Pair1
			json.Unmarshal(m.Body, &b)
			realPhoneDER = mustUnb64(t, b.PhonePub)
			b.PhonePub = MarshalPub(&mitmPhone.PublicKey)
			m.Body, _ = json.Marshal(b)
		}
	}
	g.r.toPhone = func(m *Msg) {
		switch m.Kind {
		case KindPair1R: // своё обязательство под свой ключ: nPc компьютера посредник ещё не знает
			m.Body, _ = json.Marshal(Pair1R{PCPub: b64(mitmPCDER), Commit: b64(Commit(mitmPCDER, realPhoneDER, nFake))})
		case KindPair2R:
			m.Body, _ = json.Marshal(Pair2R{NPc: b64(nFake)})
		}
	}
	g.r.mu.Unlock()
	p := newPhone(t, g.r, "ph1")
	g.startPairing()
	p.pair1(true)
	p.pair2(true)
	pi := g.waitStage(StageCompare)
	if p.sas == "" || len(p.sas) != 6 || pi.SAS == p.sas {
		t.Fatalf("посредник не обнаружен: компьютер %q, телефон %q", pi.SAS, p.sas)
	}
}

// ---------- команды ----------

func TestLinkCommandTable(t *testing.T) {
	phone, stranger := mustKey(t), mustKey(t)
	var pcKey *ecdsa.PrivateKey
	g := newRig(t, func(g *rig) { g.h.st, pcKey = pairedState(t, g.r, phone, 4) })
	g.start()
	now := g.clock.Ms()
	base := Command{V: 1, T: TypeCmd, PC: "pc-x", Phone: "ph1", Seq: 5, TS: now, Child: 3, Delta: 15}
	sign := func(k *ecdsa.PrivateKey, c Command) (Signed, []byte) {
		raw, _ := json.Marshal(c)
		s, _ := Sign(k, raw)
		return s, raw
	}
	with := func(f func(*Command)) Command { c := base; f(&c); return c }
	type tc struct {
		name    string
		from    string
		key     *ecdsa.PrivateKey
		cmd     Command
		mangle  func(*Signed)
		code    string
		lastSeq int64
		applied int
		unpair  bool
	}
	cases := []tc{
		{"верная", "ph1", phone, base, nil, "", 5, 1, false},
		{"чужой ключ", "ph1", stranger, with(func(c *Command) { c.Seq = 6 }), nil, ErrCodeBadSig, 5, 1, false},
		{"испорченная подпись", "ph1", phone, with(func(c *Command) { c.Seq = 6 }), func(s *Signed) {
			sig := mustUnb64(t, s.Sig)
			sig[len(sig)-1] ^= 0x55
			s.Sig = b64(sig)
		}, ErrCodeBadSig, 5, 1, false},
		{"старый seq", "ph1", phone, with(func(c *Command) { c.Seq = 3 }), nil, ErrCodeReplay, 5, 1, false},
		{"равный seq", "ph1", phone, base, nil, ErrCodeReplay, 5, 1, false},
		{"ts −6 мин", "ph1", phone, with(func(c *Command) { c.Seq = 6; c.TS = now - 6*60000 }), nil, ErrCodeStale, 5, 1, false},
		{"ts +6 мин", "ph1", phone, with(func(c *Command) { c.Seq = 6; c.TS = now + 6*60000 }), nil, ErrCodeStale, 5, 1, false},
		{"чужой pc", "ph1", phone, with(func(c *Command) { c.Seq = 6; c.PC = "pc-y" }), nil, ErrCodeNotPaired, 5, 1, false},
		{"чужой phone", "ph1", phone, with(func(c *Command) { c.Seq = 6; c.Phone = "ph2" }), nil, ErrCodeNotPaired, 5, 1, false},
		{"отчёт вместо команды", "ph1", phone, with(func(c *Command) { c.Seq = 6; c.T = TypeReport }), nil, ErrCodeBadSig, 5, 1, false},
		{"минус", "ph1", phone, with(func(c *Command) { c.Seq = 9; c.Delta = -15 }), nil, "", 9, 2, false},
		{"нет ребёнка", "ph1", phone, with(func(c *Command) { c.Seq = 10; c.Child = 99 }), nil, ErrCodeNoChild, 10, 2, false},
		{"предел", "ph1", phone, with(func(c *Command) { c.Seq = 11; c.Delta = 241 }), nil, ErrCodeRange, 11, 2, false},
		{"нет привязки", "ph1", phone, with(func(c *Command) { c.Seq = 12 }), nil, ErrCodeNotPaired, 0, 2, true},
	}
	for _, c := range cases {
		if c.unpair {
			g.h.mu.Lock()
			g.h.st.Phone, g.h.st.LastSeq = nil, 0
			g.h.mu.Unlock()
		}
		s, raw := sign(c.key, c.cmd)
		if c.mangle != nil {
			c.mangle(&s)
		}
		rep, ok := g.r.sendFrom(c.from, KindCmd, s, 2*time.Second)
		if !ok || rep.Kind != KindCmdR {
			t.Fatalf("%s: нет ответа cmdr (%+v)", c.name, rep)
		}
		var sr Signed
		json.Unmarshal(rep.Body, &sr)
		data, err := Verify(&pcKey.PublicKey, sr)
		if err != nil {
			t.Fatalf("%s: ответ без подписи компьютера", c.name)
		}
		var res CmdResult
		json.Unmarshal(data, &res)
		wantSeq := c.cmd.Seq
		if c.code == ErrCodeBadSig || c.unpair { // данные не проверены: seq из них не повторяется
			wantSeq = 0
		}
		if res.T != TypeCmdR || res.V != 1 || res.Cmd != HashData(raw) || res.PC != "pc-x" || res.Phone != c.from || res.Seq != wantSeq || res.TS != g.clock.Ms() {
			t.Fatalf("%s: поля ответа %+v", c.name, res)
		}
		if res.OK != (c.code == "") || res.Error != c.code {
			t.Fatalf("%s: ответ ok=%v error=%q, нужно %q", c.name, res.OK, res.Error, c.code)
		}
		if res.OK {
			if res.LastSeq != nil || res.Report == nil || res.Report.Phone != "ph1" || !strings.HasPrefix(res.Message, "Добавлено") {
				t.Fatalf("%s: удачный ответ %+v", c.name, res)
			}
		} else if res.LastSeq == nil || *res.LastSeq != c.lastSeq {
			t.Fatalf("%s: lastSeq %v, нужно %d", c.name, res.LastSeq, c.lastSeq)
		}
		_, applied, _, _ := g.h.get()
		if len(applied) != c.applied {
			t.Fatalf("%s: применено %v", c.name, applied)
		}
	}
	if _, applied, _, _ := g.h.get(); strings.Join(applied, " ") != "5:+15 9:-15" {
		t.Fatalf("применены %v", applied)
	}
	// Отклонённые проверкой (до Host) не доходят до ApplyCommand: зашли только 4 (2 верных, нет ребёнка, предел).
	if n := g.h.applyEnter.Load(); n != 4 {
		t.Fatalf("ApplyCommand вызван %d раз", n)
	}
}

// Host вызывается без мьютекса Link: ApplyCommand зовёт методы Link — не зависает.
func TestLinkHostCallsWithoutLinkLock(t *testing.T) {
	phone := mustKey(t)
	g := newRig(t, func(g *rig) { g.h.st, _ = pairedState(t, g.r, phone, 0) })
	g.h.onApply = func() {
		_ = g.l.Status()
		_ = g.l.PairInfo()
		g.l.Changed()
		g.l.StartPairing()
		g.l.CancelPairing()
	}
	g.start()
	raw, _ := json.Marshal(Command{V: 1, T: TypeCmd, PC: "pc-x", Phone: "ph1", Seq: 1, TS: g.clock.Ms(), Child: 3, Delta: 5})
	s, _ := Sign(phone, raw)
	if _, ok := g.r.sendFrom("ph1", KindCmd, s, 2*time.Second); !ok {
		t.Fatal("нет ответа: взаимоблокировка Host ↔ Link")
	}
}

// Stop под блокировкой службы, пока ApplyCommand ждёт её: Stop не зависает, старое поколение
// отброшено (команда не применена), новый Start работает.
func TestStopWhileApplyWaits(t *testing.T) {
	phone := mustKey(t)
	g := newRig(t, func(g *rig) { g.h.st, _ = pairedState(t, g.r, phone, 0) })
	g.start()
	cmd := func(seq int64) Signed {
		raw, _ := json.Marshal(Command{V: 1, T: TypeCmd, PC: "pc-x", Phone: "ph1", Seq: seq, TS: g.clock.Ms(), Child: 3, Delta: 5})
		s, _ := Sign(phone, raw)
		return s
	}
	entered, locked := make(chan struct{}), make(chan struct{})
	var once sync.Once
	g.h.beforeLock = func() { once.Do(func() { close(entered); <-locked }) }
	done := make(chan bool, 1)
	go func() { _, ok := g.r.sendFrom("ph1", KindCmd, cmd(1), noReply); done <- ok }()
	<-entered
	g.h.mu.Lock() // обработчик службы держит s.mu, ApplyCommand ждёт её
	close(locked)
	stopped := make(chan struct{})
	go func() { g.l.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		g.h.mu.Unlock()
		t.Fatal("Stop завис под блокировкой службы")
	}
	if g.l.Gen() != 0 || g.l.Status().Running {
		t.Fatal("после Stop связь числится запущенной")
	}
	g.h.mu.Unlock()
	if !g.l.Wait(time.Second) {
		t.Fatal("горутины старого поколения не завершились")
	}
	if ok := <-done; ok {
		t.Fatal("старое поколение ответило на команду")
	}
	if st, applied, _, _ := g.h.get(); len(applied) != 0 || st.LastSeq != 0 {
		t.Fatalf("старое поколение применило команду: %v seq=%d", applied, st.LastSeq)
	}
	gen2 := g.start()
	if gen2 == 0 {
		t.Fatal("нет поколения")
	}
	if _, ok := g.r.sendFrom("ph1", KindCmd, cmd(1), 2*time.Second); !ok {
		t.Fatal("новое поколение не отвечает")
	}
	if _, applied, _, _ := g.h.get(); len(applied) != 1 {
		t.Fatalf("новое поколение: %v", applied)
	}
}

// SaveState старого поколения отброшен: «Совпадает» успел дойти до службы после Stop.
func TestStaleSaveAfterStop(t *testing.T) {
	g := newRig(t, nil)
	g.start()
	p := newPhone(t, g.r, "ph1")
	g.toCompare(p)
	g.h.mu.Lock()
	n := g.h.loadEnter.Load()
	g.l.ConfirmPairing(true)
	waitFor(t, "отправка ждёт блокировку в LoadState", func() bool { return g.h.loadEnter.Load() > n })
	g.l.Stop()
	g.h.mu.Unlock()
	if !g.l.Wait(time.Second) {
		t.Fatal("не завершились")
	}
	st, _, events, stale := g.h.get()
	if st.Phone != nil || len(events) != 0 {
		t.Fatalf("старое поколение записало привязку: %+v %v", st.Phone, events)
	}
	if stale != 1 {
		t.Fatalf("отброшенных записей %d", stale)
	}
}

func TestStopDuringPoll(t *testing.T) {
	g := newRig(t, nil)
	g.start()
	g.r.mu.Lock()
	g.r.hangPoll = true
	n := g.r.polls
	g.r.mu.Unlock()
	waitFor(t, "висящий опрос", func() bool { g.r.mu.Lock(); defer g.r.mu.Unlock(); return g.r.polls > n })
	start := time.Now()
	g.l.Stop()
	if !g.l.Wait(time.Second) || time.Since(start) > time.Second {
		t.Fatalf("Stop во время опроса: %v", time.Since(start))
	}
	if st := g.l.Status(); st.Running || st.Online {
		t.Fatalf("после Stop: %+v", st)
	}
	g.r.mu.Lock()
	polls := g.r.polls
	g.r.mu.Unlock()
	time.Sleep(30 * time.Millisecond)
	g.r.mu.Lock()
	defer g.r.mu.Unlock()
	if g.r.polls != polls {
		t.Fatal("после Stop идут запросы")
	}
}

// ---------- обрывы ----------

func TestLinkBackoff(t *testing.T) {
	g := newRig(t, nil)
	g.r.pollHook = func(n int) (int, []byte, bool) {
		if n <= 10 || n == 12 {
			return 503, []byte(`{"error":"busy"}`), true
		}
		return 0, nil, false
	}
	g.l.Start()
	waitFor(t, "12-й опрос", func() bool { return len(g.sl.list()) >= 11 })
	got := g.sl.list()
	bases := []time.Duration{1, 2, 4, 8, 16, 30, 30, 30, 30, 30}
	for i, b := range bases {
		b *= time.Second
		lo, hi := b*8/10, b*12/10
		if hi > 30*time.Second {
			hi = 30 * time.Second
		}
		if got[i] < lo || got[i] > hi {
			t.Fatalf("пауза %d: %v, нужно %v…%v (все: %v)", i+1, got[i], lo, hi, got)
		}
	}
	if got[10] > 1200*time.Millisecond {
		t.Fatalf("после успешного опроса пауза не сброшена: %v", got[10])
	}
	waitFor(t, "на связи", func() bool { return g.l.Status().Online })
}

func TestBackoffNeverAbove30s(t *testing.T) {
	l := NewLink(Config{Host: newFakeHost()})
	for n := 0; n < 50; n++ {
		for i := 0; i < 20; i++ {
			if d := l.backoff(n); d > 30*time.Second || d < 800*time.Millisecond {
				t.Fatalf("пауза %d: %v", n, d)
			}
		}
	}
}

func TestLinkBadAnswersKeepLoop(t *testing.T) {
	g := newRig(t, nil)
	var seen []string
	var mu sync.Mutex
	g.sl.hook = func(time.Duration) {
		mu.Lock()
		seen = append(seen, g.l.Status().LastErr)
		mu.Unlock()
	}
	g.r.pollHook = func(n int) (int, []byte, bool) {
		switch n {
		case 1:
			return 200, append([]byte(`{"messages":[],"x":"`), bytes.Repeat([]byte("я"), 1<<19)...), true
		case 2:
			return 200, []byte(`{"messages":[{"id":`), true
		case 3:
			return 200, []byte(`{"messages":"x"}`), true
		}
		return 0, nil, false
	}
	g.start()
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(seen[:3], ",") != "bad_answer,bad_answer,bad_answer" {
		t.Fatalf("коды ошибок: %v", seen)
	}
	// Цикл жив: привязка работает.
	g.startPairing()
}

func TestLinkUnauthorizedReregisters(t *testing.T) {
	phone := mustKey(t)
	g := newRig(t, func(g *rig) { g.h.st, _ = pairedState(t, g.r, phone, 7) })
	g.start()
	g.r.revokeAll()
	waitFor(t, "регистрация заново", func() bool { n, _, _, _ := g.r.snapshot(); return n == 1 })
	waitFor(t, "на связи", func() bool { st, _, _, _ := g.h.get(); return st.PCID != "pc-x" && g.l.Status().Online })
	st, _, events, _ := g.h.get()
	if st.Phone != nil || st.LastSeq != 0 || st.PCID == "" || st.PCToken == "tok-x" || st.PCKey == "" {
		t.Fatalf("состояние после 401: %+v", st)
	}
	if len(events) != 1 || events[0] != "lost:Pixel" {
		t.Fatalf("события: %v", events)
	}
}

func TestLinkRegistersAndReports(t *testing.T) {
	g := newRig(t, nil)
	g.start()
	st, _, _, _ := g.h.get()
	if st.PCID == "" || st.PCToken == "" {
		t.Fatalf("не зарегистрирован: %+v", st)
	}
	key, err := ParsePriv(st.PCKey)
	if err != nil {
		t.Fatal(err)
	}
	// Без телефона отчёт не шлётся.
	g.l.Changed()
	time.Sleep(30 * time.Millisecond)
	if _, _, _, reps := g.r.snapshot(); len(reps) != 0 {
		t.Fatal("отчёт без привязанного телефона")
	}
	// Перезапуск со старым состоянием — без новой регистрации.
	g.l.Stop()
	g.l.Wait(time.Second)
	phone := mustKey(t)
	g.h.mu.Lock()
	g.h.st.Phone = &Phone{ID: "ph1", Pub: MarshalPub(&phone.PublicKey)}
	g.h.mu.Unlock()
	g.start()
	waitFor(t, "отчёт при включении", func() bool { _, _, _, r := g.r.snapshot(); return len(r) == 1 })
	g.l.Changed()
	waitFor(t, "отчёт после Changed", func() bool { _, _, _, r := g.r.snapshot(); return len(r) == 2 })
	n, _, _, reps := g.r.snapshot()
	if n != 1 {
		t.Fatalf("регистраций %d", n)
	}
	data, err := Verify(&key.PublicKey, reps[1])
	if err != nil {
		t.Fatal(err)
	}
	var rep Report
	json.Unmarshal(data, &rep)
	if rep.V != 1 || rep.T != TypeReport || rep.PC != st.PCID || rep.Phone != "ph1" || rep.TS != g.clock.Ms() || len(rep.Children) != 1 || rep.Children[0].LeftSec != 600 {
		t.Fatalf("отчёт: %+v", rep)
	}
}

func TestLinkPanicRestarts(t *testing.T) {
	phone := mustKey(t)
	g := newRig(t, func(g *rig) { g.h.st, _ = pairedState(t, g.r, phone, 0) })
	g.h.panicOnce.Store(true)
	g.start()
	waitFor(t, "отчёт после перезапуска", func() bool { _, _, _, r := g.r.snapshot(); return len(r) >= 1 })
	g.h.mu.Lock()
	logs := strings.Join(g.h.logs, "\n")
	g.h.mu.Unlock()
	if !strings.Contains(logs, "сбой в Children") {
		t.Fatalf("паника не в журнале: %s", logs)
	}
}

func TestLinkUnpair(t *testing.T) {
	phone := mustKey(t)
	g := newRig(t, func(g *rig) { g.h.st, _ = pairedState(t, g.r, phone, 3) })
	g.start()
	g.l.Unpair()
	waitFor(t, "отвязка", func() bool { _, _, u, _ := g.r.snapshot(); return len(u) == 1 && u[0] == "ph1" })
	st, _, events, _ := g.h.get()
	if st.Phone != nil || len(events) != 1 || events[0] != "unpaired:Pixel" {
		t.Fatalf("%+v %v", st.Phone, events)
	}
}

func TestLinkNotRunning(t *testing.T) {
	l := NewLink(Config{Host: newFakeHost()})
	l.StartPairing()
	if pi := l.PairInfo(); pi.Stage != StageFailed || pi.Error != PairErrStopped {
		t.Fatalf("%+v", pi)
	}
	l.ConfirmPairing(true)
	l.Unpair()
	l.Changed()
	l.CancelPairing()
	l.Stop()
	if !l.Wait(10 * time.Millisecond) {
		t.Fatal("Wait без запуска")
	}
	h := newFakeHost()
	h.link = l
	if gen := l.Start(); gen != 0 || l.Status().LastErr != StatusBadAddress {
		t.Fatalf("запуск без адреса: %d %+v", gen, l.Status())
	}
}

func TestGenUniqueAcrossLinks(t *testing.T) {
	a := newRig(t, nil)
	b := newRig(t, nil)
	ga, gb := a.start(), b.start()
	if ga == 0 || gb == 0 || ga == gb || a.l.Start() != ga {
		t.Fatalf("поколения %d %d", ga, gb)
	}
}

// «Совпадает» до сверки числа ничего не записывает.
func TestConfirmBeforeCompareIgnored(t *testing.T) {
	g := newRig(t, nil)
	g.start()
	p := newPhone(t, g.r, "ph1")
	g.startPairing()
	g.l.ConfirmPairing(true)
	p.pair1(true)
	g.l.ConfirmPairing(true)
	time.Sleep(20 * time.Millisecond)
	if st, _, ev, _ := g.h.get(); st.Phone != nil || len(ev) != 0 || g.l.PairInfo().Stage != StageWaitNonce {
		t.Fatalf("подтверждение до сверки: %+v %v %+v", st.Phone, ev, g.l.PairInfo())
	}
}

// Срок сверки проверяется и в самом «Совпадает» (проверка сроков могла ещё не пройти).
func TestConfirmAfterDeadline(t *testing.T) {
	g := newRig(t, func(g *rig) { g.tick = time.Hour })
	g.start()
	p := newPhone(t, g.r, "ph1")
	g.toCompare(p)
	g.clock.Add(10*time.Minute + time.Second)
	g.l.ConfirmPairing(true)
	if pi := g.waitStage(StageFailed); pi.Error != PairErrExpired {
		t.Fatalf("%+v", pi)
	}
	if st, _, _, _ := g.h.get(); st.Phone != nil {
		t.Fatal("привязан после срока")
	}
}

// Служба отбросила запись (ErrStale) — поколение прекращает работу и не ходит в сеть.
func TestStaleSaveStopsGeneration(t *testing.T) {
	g := newRig(t, nil)
	g.start()
	p := newPhone(t, g.r, "ph1")
	g.toCompare(p)
	g.h.forceStale.Store(true)
	g.l.ConfirmPairing(true)
	waitFor(t, "отброшенная запись", func() bool { _, _, _, n := g.h.get(); return n == 1 })
	g.l.mu.Lock()
	r := g.l.run
	g.l.mu.Unlock()
	select {
	case <-r.done:
	case <-time.After(time.Second):
		t.Fatal("горутины поколения работают после ErrStale")
	}
	if _, _, ev, _ := g.h.get(); len(ev) != 0 {
		t.Fatalf("события: %v", ev)
	}
}

// Паника в отправке во время привязки: после перезапуска окно видит отмену, а не «вечную» сверку.
func TestPanicDuringPairingShowsAborted(t *testing.T) {
	g := newRig(t, nil)
	g.start()
	p := newPhone(t, g.r, "ph1")
	g.toCompare(p)
	g.h.mu.Lock()
	g.h.st.Phone = &Phone{ID: "x", Pub: "x"} // отчёт пойдёт и вызовет Children
	g.h.mu.Unlock()
	g.h.panicOnce.Store(true)
	g.l.Changed()
	if pi := g.waitStage(StageFailed); pi.Error != PairErrAborted {
		t.Fatalf("%+v", pi)
	}
}

// ---------- журнал работы при обрыве (R1, PL-53) ----------

func (h *fakeHost) logsWith(sub string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, s := range h.logs {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

// Полтора часа обрыва: одна строка о потере (и не больше одной часовой), возврат — «восстановлена».
func TestLinkOutageLogsByChange(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	g := newRig(t, nil)
	start := g.clock.Ms()
	g.sl.hook = func(d time.Duration) {
		g.clock.Add(d)
		if g.clock.Ms()-start >= (90 * time.Minute).Milliseconds() {
			failing.Store(false)
		}
	}
	g.r.pollHook = func(int) (int, []byte, bool) {
		if failing.Load() {
			return 503, []byte(`{"error":"busy"}`), true
		}
		return 0, nil, false
	}
	g.l.Start()
	waitFor(t, "конец обрыва", func() bool { return !failing.Load() && g.l.Status().Online })
	if n := len(g.sl.list()); n < 150 {
		t.Fatalf("опросов в обрыве %d, ожидали ~180", n)
	}
	lost, again, back := g.h.logsWith(": нет связи с сервером"), g.h.logsWith("всё ещё нет связи"), g.h.logsWith("связь с сервером восстановлена")
	if lost != 1 || again > 1 || back != 1 {
		g.h.mu.Lock()
		t.Fatalf("потеря %d, повтор %d, возврат %d; журнал:\n%s", lost, again, back, strings.Join(g.h.logs, "\n"))
	}
	g.h.mu.Lock()
	total := len(g.h.logs)
	g.h.mu.Unlock()
	if total > 3 {
		t.Fatalf("строк в журнале работы %d", total)
	}
}

// Новый код ошибки — новая строка; та же — молчит.
func TestLinkOutageNewCodeLogged(t *testing.T) {
	g := newRig(t, nil)
	g.r.pollHook = func(n int) (int, []byte, bool) {
		switch {
		case n <= 3:
			return 503, []byte(`{"error":"busy"}`), true
		case n <= 6:
			return 200, []byte(`{"messages":"x"}`), true
		}
		return 0, nil, false
	}
	g.start()
	if lost, back := g.h.logsWith(": нет связи с сервером"), g.h.logsWith("восстановлена"); lost != 2 || back != 1 {
		g.h.mu.Lock()
		t.Fatalf("потеря %d, возврат %d:\n%s", lost, back, strings.Join(g.h.logs, "\n"))
	}
}
