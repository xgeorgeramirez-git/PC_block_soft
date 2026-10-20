package service

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Сбои в поддельных портах, которые в настоящей службе значили бы панику в журнале, датчике
// или вызове Windows. Цикл учёта не должен их пропускать наружу: иначе умрёт весь процесс
// службы и канал связи вместе с учётом всех остальных детей до перезапуска диспетчером служб.

// panicJournal паникует при записи события про одного ребёнка и пишет остальные как обычно.
type panicJournal struct {
	inner    Journal
	panicFor int64
	hits     atomic.Int32
}

func (j *panicJournal) Write(childID int64, kind string, details map[string]any) {
	if childID == j.panicFor {
		j.hits.Add(1)
		panic("поддельный журнал сломался")
	}
	j.inner.Write(childID, kind, details)
}

// panicAdmins паникует при проверке прав одной учётки, когда взведён.
type panicAdmins struct {
	*fakeAdmins
	panicSID string
	armed    atomic.Bool
}

func (a *panicAdmins) IsAdmin(sid string) (bool, error) {
	if a.armed.Load() && sid == a.panicSID {
		panic("поддельная проверка прав сломалась")
	}
	return a.fakeAdmins.IsAdmin(sid)
}

// panicLauncher паникует при списке сеансов, когда взведён.
type panicLauncher struct {
	*fakeLauncher
	armed atomic.Bool
}

func (l *panicLauncher) Sessions() ([]SessionInfo, error) {
	if l.armed.Load() {
		panic("поддельный список сеансов сломался")
	}
	return l.fakeLauncher.Sessions()
}

// flakyClock паникует в первых n вызовах Now после взвода.
type flakyClock struct {
	core.Clock
	left atomic.Int32
}

func (c *flakyClock) Now() time.Time {
	if c.left.Add(-1) >= 0 {
		panic("поддельные часы сломались")
	}
	return c.Clock.Now()
}

// mustNotPanic проваливает тест, если f выпустила панику наружу.
func mustNotPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("%s: паника вышла наружу: %v", what, p)
		}
	}()
	f()
}

// safeStep — как step, но тик и обслуживание не имеют права выпускать панику.
func (h *harness) safeStep(s ipc.SensorSample) {
	h.t.Helper()
	h.clock.Advance(time.Second)
	h.real.Advance(time.Second)
	h.poll(s)
	mustNotPanic(h.t, "Tick", func() { h.svc.Tick(h.clock.Now()) })
	mustNotPanic(h.t, "Housekeep", func() { h.svc.Housekeep(h.real.Now()) })
}

// Ловит: паника при обработке одного ребёнка (здесь журнал событий на смене суток) уносит
// весь тик: остальные дети остаются без учёта и без итога дня, а служба умирает.
func TestTickPanicOnOneChildDoesNotStopOthers(t *testing.T) {
	var pj *panicJournal
	h := newHarness(t, withChild(childSID, "Миша", 600), withChild(child2SID, "Коля", 600), configured(),
		func(h *harness) {
			pj = &panicJournal{
				inner:    StoreJournal{Store: h.store, Clock: h.clock, Loc: tzMSK, Logf: h.cfg.Logf},
				panicFor: h.childID(childSID),
			}
			h.cfg.Journal = pj
		})
	h.clock.Set(time.Date(2026, 10, 19, 23, 59, 58, 0, tzMSK))
	h.start()
	h.childLogsIn()
	for i := 0; i < 4; i++ {
		h.safeStep(activeSample)
	}
	if pj.hits.Load() == 0 {
		t.Fatal("поддельный журнал не вызывался: тест ничего не проверяет")
	}

	// Второй ребёнок обработан в том же тике: итог его дня записан.
	second := h.childID(child2SID)
	var got bool
	for _, e := range h.store.eventsOf("day_closed") {
		got = got || e.ChildID == second
	}
	if !got {
		t.Fatal("после паники на первом ребёнке второй остался без итога дня")
	}
	// Первый ребёнок тоже живёт: итог дня сохранён до записи события, учёт идёт дальше.
	if _, err := h.store.LoadDayState(h.childID(childSID), "2026-10-19"); err != nil {
		t.Fatal(err)
	}
	if st := h.status(); st.Day != "2026-10-20" || st.UsedS < 2 {
		t.Fatalf("учёт первого ребёнка встал: день %s, учтено %d с", st.Day, st.UsedS)
	}
	// Сбой записан для разбора: в журнал работы со стеком и событием для родителя без техники.
	if !strings.Contains(h.logText(), "Внутренняя ошибка") || !strings.Contains(h.logText(), "поддельный журнал сломался") {
		t.Errorf("в журнале работы нет записи о сбое:\n%s", h.logText())
	}
	evs := h.store.eventsOf(evInternalError)
	if len(evs) != 1 {
		t.Fatalf("событий о внутренней ошибке %d, ждали 1", len(evs))
	}
	text := eventText(evs[0].Kind, "", evs[0].Details)
	if !strings.Contains(text, "Миша") {
		t.Errorf("в событии нет имени ребёнка: %q", text)
	}
	for _, bad := range []string{"паник", "panic", "служб", "goroutine", "баз"} {
		if strings.Contains(strings.ToLower(text), bad) {
			t.Errorf("в тексте события для родителя техническое слово %q: %q", bad, text)
		}
	}
}

// Ловит: паника в обслуживании одного ребёнка (проверка прав R1) пропускает остальных детей и
// всё, что идёт после, и выходит из Housekeep наружу.
func TestHousekeepPanicOnOneChildDoesNotStopOthers(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 120), configured())
	h.start()
	pa := &panicAdmins{fakeAdmins: h.admins, panicSID: childSID}
	h.svc.cfg.Admins = pa
	pa.armed.Store(true)

	callsBefore := func() int {
		h.admins.mu.Lock()
		defer h.admins.mu.Unlock()
		return h.admins.calls[child2SID]
	}
	before := callsBefore()
	h.real.Advance(adminCheckEvery + time.Second)
	mustNotPanic(t, "Housekeep", func() { h.svc.Housekeep(h.real.Now()) })
	if callsBefore() == before {
		t.Fatal("второго ребёнка не проверили на права: паника на первом остановила обслуживание")
	}
	if n := len(h.store.eventsOf(evInternalError)); n != 1 {
		t.Fatalf("событий о внутренней ошибке %d, ждали 1", n)
	}
	// Служба жива и отвечает.
	h.ok(childPeer(), "", ipc.CmdGetChildStatus, nil, nil)
}

// Ловит: паника при перечислении сеансов Windows (вызов системы) выходит из обслуживания или
// из события входа и роняет службу, а не записывается как сбой.
func TestSessionsPanicIsContained(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.childLogsIn()
	pl := &panicLauncher{fakeLauncher: h.launcher}
	h.svc.cfg.Launcher = pl
	pl.armed.Store(true)

	mustNotPanic(t, "OnSession", func() { h.svc.OnSession(SessionLogon, childSession, h.real.Now()) })
	h.real.Advance(time.Second)
	mustNotPanic(t, "Housekeep", func() { h.svc.Housekeep(h.real.Now()) })
	if n := len(h.store.eventsOf(evInternalError)); n == 0 {
		t.Fatal("сбой при перечислении сеансов не записан")
	}
	// Сеансы снова читаются: обслуживание продолжается как ни в чём не бывало.
	pl.armed.Store(false)
	h.real.Advance(time.Second)
	mustNotPanic(t, "Housekeep", func() { h.svc.Housekeep(h.real.Now()) })
	h.ok(childPeer(), "", ipc.CmdGetChildStatus, nil, nil)
}

// Ловит: паника вне прохода по детям (часы ядра в самом цикле) убивает горутину RunLoop и с ней
// службу; цикл должен записать сбой и тикать дальше.
func TestRunLoopSurvivesPanic(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withTestMode(TestMode{ClockSpeed: 200}))
	fc := &flakyClock{Clock: h.clock}
	h.cfg.Clock = fc
	h.cfg.RealNow = time.Now
	svc, err := New(h.cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.svc = svc
	if err := svc.Start(time.Now()); err != nil {
		t.Fatal(err)
	}
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
	svc.OnSession(SessionLogon, childSession, time.Now())
	svc.Handle(guardPeer(), ipc.Msg{Cmd: ipc.CmdGuardPoll, Payload: ipc.Payload(ipc.GuardPollRequest{Sample: activeSample})}, time.Now())

	fc.left.Store(1) // первый же вызов Now в цикле паникует
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { svc.RunLoop(ctx); close(done) }()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunLoop не остановился")
	}
	if got := h.usedS(); got < 3 {
		t.Fatalf("после паники в цикле засчитано %d с, ждали не меньше 3: цикл умер", got)
	}
	if n := len(h.store.eventsOf(evInternalError)); n != 1 {
		t.Fatalf("событий о внутренней ошибке %d, ждали 1", n)
	}
}

// Ловит: сбой, повторяющийся каждую секунду, засыпает журнал работы и журнал родителя
// (запись не чаще раза в минуту на место и ребёнка), или не записывается снова через минуту.
func TestInternalErrorIsRateLimited(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	cs := h.svc.children[h.childID(childSID)]
	record := func(where string) {
		h.svc.mu.Lock()
		defer h.svc.mu.Unlock()
		h.svc.internalError(where, cs, "сбой")
	}
	for i := 0; i < 30; i++ {
		record("tick")
		h.real.Advance(time.Second)
		h.svc.mu.Lock()
		h.svc.real = h.real.Now()
		h.svc.mu.Unlock()
	}
	if n := len(h.store.eventsOf(evInternalError)); n != 1 {
		t.Fatalf("за 30 с записано %d событий, ждали 1", n)
	}
	record("housekeep") // другое место: отдельный счёт
	if n := len(h.store.eventsOf(evInternalError)); n != 2 {
		t.Fatalf("сбой в другом месте не записан: %d событий", n)
	}
	h.real.Advance(errLogEvery)
	h.svc.mu.Lock()
	h.svc.real = h.real.Now()
	h.svc.mu.Unlock()
	record("tick")
	if n := len(h.store.eventsOf(evInternalError)); n != 3 {
		t.Fatalf("через минуту сбой не записан снова: %d событий", n)
	}
}

// Ловит: сбой самого журнала, из-за которого всё и упало, роняет разбор этого сбоя: паника в
// обработчике паники.
func TestInternalErrorSurvivesBrokenJournal(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), func(h *harness) {
		h.cfg.Journal = &panicJournal{inner: StoreJournal{Store: h.store, Clock: h.clock, Loc: tzMSK}, panicFor: 0}
	})
	h.start()
	mustNotPanic(t, "internalError", func() {
		h.svc.mu.Lock()
		defer h.svc.mu.Unlock()
		h.svc.internalError("tick", nil, "сбой")
	})
}
