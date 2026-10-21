package service

import (
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Ловит: активные секунды не попадают в учёт ребёнка (тик службы не доходит до ядра или
// снимок страже не привязывается к ребёнку сеанса).
func TestTickCountsActiveSeconds(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.run(10, activeSample)
	if got := h.usedS(); got != 10 {
		t.Fatalf("за 10 активных секунд учтено %d с", got)
	}
}

// Ловит: бездействующий сеанс (сеанс заблокирован, нет ввода) засчитывается как активный (R2).
func TestInactiveSessionNotCounted(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.run(10, idleSample)
	if got := h.usedS(); got != 0 {
		t.Fatalf("неактивный сеанс: учтено %d с, ждали 0", got)
	}
}

// Ловит: служба тянет последний снимок бесконечно: страж завис или убит, а секунды всё
// идут. Снимок старше 5 с по настоящим часам не засчитывается.
func TestStaleSampleNotCounted(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.poll(activeSample)
	for i := 0; i < 10; i++ { // опросов нет, часы идут
		h.clock.Advance(time.Second)
		h.real.Advance(time.Second)
		h.svc.Tick(h.clock.Now())
		h.svc.Housekeep(h.real.Now())
	}
	if got := h.usedS(); got != 5 {
		t.Fatalf("учтено %d с, ждали ровно 5 (снимок свежий 5 с)", got)
	}
}

// Ловит: время считается без вошедшего ребёнка (сеанса нет, снимков нет).
func TestNoSessionNoAccounting(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	for i := 0; i < 10; i++ {
		h.clock.Advance(time.Second)
		h.real.Advance(time.Second)
		h.svc.Tick(h.clock.Now())
		h.svc.Housekeep(h.real.Now())
	}
	if got := h.usedS(); got != 0 {
		t.Fatalf("без сеанса учтено %d с", got)
	}
}

// Ловит: сохранение делается после тика, а не до него (эталон G10): в 30-й секунде в базе
// оказалось бы 31 с или сохранений не реже 30 с не было. Сохранённые итоги: 30, 60, 90.
func TestDueSaveBeforeTickSavesExactTotals(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 600))
	h.start()
	h.childLogsIn()
	id := h.childID(childSID)
	h.run(95, activeSample)
	var got []int
	for _, s := range h.store.saves {
		if s.childID == id {
			got = append(got, s.st.TotalS)
		}
	}
	want := []int{30, 60, 90}
	if len(got) != len(want) {
		t.Fatalf("сохранения %v, ждали %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("сохранения %v, ждали %v", got, want)
		}
	}
}

// Ловит: итог закончившегося дня теряется при смене суток (G9, R16): день 19.10 должен
// быть сохранён с его секундами, а секунды после 00:00 идти в новый день.
func TestClosedDaySavedAtMidnight(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 600))
	h.clock.Set(time.Date(2026, 10, 19, 23, 59, 58, 0, tzMSK))
	h.start()
	h.childLogsIn()
	h.run(5, activeSample)
	id := h.childID(childSID)
	old, err := h.store.LoadDayState(id, "2026-10-19")
	if err != nil || old.TotalS != 1 {
		t.Fatalf("закрытый день 19.10: %+v, ждали 1 с (секунда 23:59:59)", old)
	}
	if st := h.status(); st.Day != "2026-10-20" || st.UsedS != 4 {
		t.Fatalf("новый день: %s, учтено %d с, ждали 2026-10-20 и 4", st.Day, st.UsedS)
	}
	if n := len(h.store.eventsOf("day_closed")); n != 1 {
		t.Fatalf("событий day_closed %d, ждали 1", n)
	}
}

// Ловит: предупреждения R9 доходят до стража не по одному разу (повтор при каждом опросе)
// или не в порядке 15, 5, 1; текст берётся не из общего места.
func TestWarningsDeliveredOncePerThreshold(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 20))
	h.start()
	h.childLogsIn()
	var got []int
	collect := func(rs []ipc.GuardPollReply) {
		for _, r := range rs {
			for _, w := range r.Warnings {
				got = append(got, w.Threshold)
				if want := ipc.NewGuardWarning(w.Threshold); w.Text != want.Text || w.Sound != want.Sound {
					t.Errorf("предупреждение %d: %+v, ждали %+v", w.Threshold, w, want)
				}
			}
		}
	}
	collect(h.run(20*60+5, activeSample))
	if len(got) != 3 || got[0] != 15 || got[1] != 5 || got[2] != 1 {
		t.Fatalf("предупреждения %v, ждали [15 5 1] по одному разу", got)
	}
}

// Ловит: предупреждения перед запретным интервалом (R9, G6) не доходят до стража тем же
// путём, что и предупреждения о лимите. Большой лимит, интервал пн 15:16 наступает раньше.
func TestForbiddenWarningsDeliveredBeforeInterval(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 600))
	id := h.childID(childSID)
	if err := h.store.SetSchedule(id, []core.Interval{{Weekday: core.Monday, StartMin: 916, EndMin: 420}}); err != nil {
		t.Fatal(err)
	}
	h.start()
	h.childLogsIn()
	var got []int
	locked := false
	for _, r := range h.run(20*60, activeSample) {
		for _, w := range r.Warnings {
			got = append(got, w.Threshold)
		}
		if r.Status.Lock == ipc.LockForbidden {
			locked = true
		}
	}
	if len(got) != 3 || got[0] != 15 || got[1] != 5 || got[2] != 1 {
		t.Fatalf("предупреждения перед интервалом %v, ждали [15 5 1]", got)
	}
	if !locked {
		t.Fatal("блокировка по запретному интервалу не наступила")
	}
}

// Ловит: блокировка «время вышло» не ставится или секунды продолжают считаться под
// экраном блокировки (R4).
func TestTimeUpLocksAndStopsCounting(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 1))
	h.start()
	h.childLogsIn()
	rs := h.run(100, activeSample)
	st := h.status()
	if st.Status != ipc.StatusTimeUp || st.Lock != ipc.LockTimeUp {
		t.Fatalf("статус %q блокировка %q, ждали timeUp", st.Status, st.Lock)
	}
	if st.UsedS != 60 {
		t.Fatalf("под блокировкой учтено %d с, ждали ровно 60", st.UsedS)
	}
	if last := rs[len(rs)-1].Status; last.Lock != ipc.LockTimeUp {
		t.Fatalf("страж получил блокировку %q", last.Lock)
	}
}

// Ловит: два ребёнка делят один счётчик или снимок стража одного сеанса попадает
// другому ребёнку.
func TestTwoChildrenAccountedSeparately(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 120))
	h.start()
	h.childLogsIn()
	h.child2LogsIn()
	for i := 0; i < 10; i++ {
		h.clock.Advance(time.Second)
		h.real.Advance(time.Second)
		h.ok(guardPeerOf(childSession), "", ipc.CmdGuardPoll, ipc.GuardPollRequest{Sample: activeSample}, nil)
		h.ok(guardPeerOf(child2Session), "", ipc.CmdGuardPoll, ipc.GuardPollRequest{Sample: idleSample}, nil)
		h.svc.Tick(h.clock.Now())
		h.svc.Housekeep(h.real.Now())
	}
	var s1, s2 ipc.ChildStatus
	h.ok(ipc.Peer{PID: 500, SessionID: childSession, SID: childSID}, "", ipc.CmdGetChildStatus, nil, &s1)
	h.ok(ipc.Peer{PID: 501, SessionID: child2Session, SID: child2SID}, "", ipc.CmdGetChildStatus, nil, &s2)
	if s1.UsedS != 10 || s2.UsedS != 0 || s1.Name != "Миша" || s2.Name != "Коля" {
		t.Fatalf("Миша %d с (%s), Коля %d с (%s), ждали 10 и 0", s1.UsedS, s1.Name, s2.UsedS, s2.Name)
	}
}

// Ловит: R1 не действует: детская учётка с правами администратора продолжает ограничиваться
// и учитываться, а родитель не видит статус «Администратор: ограничения не действуют».
func TestAdminChildNotRestricted(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 1))
	h.admins.set(childSID, true)
	h.start()
	h.childLogsIn()
	h.run(120, activeSample)
	st := h.status()
	if st.Status != ipc.StatusAdmin || st.Lock != ipc.LockNone || st.UsedS != 0 {
		t.Fatalf("администратор: статус %q, блокировка %q, учтено %d с", st.Status, st.Lock, st.UsedS)
	}
	if n := len(h.store.eventsOf("child_admin")); n != 1 {
		t.Fatalf("событий child_admin %d, ждали 1 (а не по одному на проверку)", n)
	}
}

// Ловит: после снятия прав администратора ограничения не возвращаются (проверка R1 раз в 60 с).
func TestAdminChildRestrictedAgainAfterRightsRemoved(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.admins.set(childSID, true)
	h.start()
	h.childLogsIn()
	h.run(5, activeSample)
	h.admins.set(childSID, false)
	h.run(61, activeSample)
	st := h.status()
	// Проверка R1 срабатывает в 60-й секунде после тика: засчитаны только тики 61…66.
	if st.Status == ipc.StatusAdmin || st.UsedS != 6 {
		t.Fatalf("после снятия прав: статус %q, учтено %d с, ждали 6 (под правами администратора секунды не идут)", st.Status, st.UsedS)
	}
	if n := len(h.store.eventsOf("child_admin_off")); n != 1 {
		t.Fatalf("событий child_admin_off %d, ждали 1", n)
	}
}

// Ловит: проверка администратора раз в секунду вместо раза в 60 с (лишняя нагрузка на
// системный вызов) или совсем не повторяется.
func TestAdminCheckEverySixtySeconds(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	base := h.admins.calls[childSID]
	h.run(59, activeSample)
	if got := h.admins.calls[childSID] - base; got != 0 {
		t.Fatalf("за 59 с проверок %d, ждали 0", got)
	}
	h.run(2, activeSample)
	if got := h.admins.calls[childSID] - base; got != 1 {
		t.Fatalf("за 61 с проверок %d, ждали 1", got)
	}
}

// Ловит: в тестовом режиме учётка раннера (TestChildSID) всё равно проверяется по R1 и
// не ограничивается (ТЗ 10.8: для неё проверка R1 снята).
func TestTestChildSIDExemptFromAdminCheck(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withTestMode(TestMode{ClockSpeed: 1, TestChildSID: childSID}))
	h.admins.set(childSID, true)
	h.start()
	h.childLogsIn()
	h.run(70, activeSample)
	st := h.status()
	if st.Status == ipc.StatusAdmin || st.UsedS != 70 {
		t.Fatalf("тестовый ребёнок: статус %q, учтено %d с, ждали учёт 70 с", st.Status, st.UsedS)
	}
	if h.admins.calls[childSID] != 0 {
		t.Fatalf("проверок R1 для TestChildSID %d, ждали 0", h.admins.calls[childSID])
	}
}

// Ловит: ошибка проверки прав превращает ребёнка в администратора (или обратно): при
// сбое состояние остаётся прежним, служба не падает.
func TestAdminCheckErrorKeepsState(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.admins.errs[childSID] = errFake
	h.run(125, activeSample)
	if st := h.status(); st.Status == ipc.StatusAdmin || st.UsedS != 125 {
		t.Fatalf("при ошибке проверки: статус %q, учтено %d", st.Status, st.UsedS)
	}
	if !strings.Contains(h.logText(), "права администратора") {
		t.Errorf("ошибка проверки не попала в журнал работы:\n%s", h.logText())
	}
}

// Ловит: правила не прочитаны (ошибка базы), а ограничения применяются; событие safe_mode
// пишется на каждой проверке; после починки базы правила не подхватываются (R18).
func TestRulesUnreadableSafeModeThenRecovers(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 1))
	h.store.failLimits = true
	h.start()
	h.childLogsIn()
	h.run(200, activeSample)
	st := h.status()
	if st.Status != ipc.StatusSafe || st.Lock != ipc.LockNone {
		t.Fatalf("правила не прочитаны: статус %q, блокировка %q, ждали safe без блокировки", st.Status, st.Lock)
	}
	if n := len(h.store.eventsOf("safe_mode")); n != 1 {
		t.Fatalf("событий safe_mode %d, ждали 1", n)
	}
	h.store.failLimits = false
	h.run(61, activeSample)
	if st := h.status(); st.Status != ipc.StatusTimeUp {
		t.Fatalf("после починки базы статус %q, ждали timeUp (лимит 1 мин исчерпан)", st.Status)
	}
}

// Ловит: после порчи базы служба молчит: нет события (с путём копии для разбора), нет
// предупреждения родителю человеческими словами.
func TestRecoveredDatabaseWritesEventAndNotice(t *testing.T) {
	h := newHarness(t, withRecovered("pcblock.db.broken-20261019-120000"))
	h.start()
	evs := h.store.eventsOf("db_recovered")
	if len(evs) != 1 {
		t.Fatalf("событий db_recovered %d, ждали 1", len(evs))
	}
	if cp, _ := evs[0].Details["copy"].(string); !strings.Contains(cp, "broken-20261019-120000") {
		t.Fatalf("в событии нет пути копии испорченной базы: %v", evs[0].Details)
	}
	if v, ok, _ := h.store.GetSetting(settingDBNotice); !ok || v != textRecoveredNotice {
		t.Fatalf("флаг предупреждения родителю не записан: %q %v", v, ok)
	}
}

// Ловит: сбой записи в базу останавливает учёт (R18) или засыпает журнал работы строкой
// на каждый тик: ошибка пишется не чаще раза в 60 с.
func TestSaveErrorsDoNotStopAccountingAndAreRateLimited(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 600))
	h.start()
	h.childLogsIn()
	h.store.failSave = true
	h.run(130, activeSample)
	if got := h.usedS(); got != 130 {
		t.Fatalf("при ошибках записи учтено %d с, ждали 130", got)
	}
	n := strings.Count(h.logText(), "не сохранено")
	if n < 1 || n > 3 {
		t.Fatalf("строк об ошибке записи %d (за 130 с сбоев было 4), ждали от 1 до 3", n)
	}
	h.store.failSave = false
	h.run(31, activeSample)
	if _, ok := h.store.lastSavedTotal(h.childID(childSID)); !ok {
		t.Fatal("после починки базы состояние не сохраняется")
	}
}

// Ловит: при остановке службы несохранённые секунды теряются (NFR-7), после перезапуска
// счёт идёт заново.
func TestShutdownSavesStateAndRestartContinues(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.run(10, activeSample)
	h.svc.Shutdown(h.real.Now())

	h2 := newHarness(t, withStore(h.store))
	h2.clock.Set(h.clock.Now())
	h2.start()
	if got := h2.usedS(); got != 10 {
		t.Fatalf("после перезапуска учтено %d с, ждали 10", got)
	}
}

// Ловит: после остановки службы тики и опросы продолжают менять данные (тик после Shutdown).
func TestTickAfterShutdownDoesNothing(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.run(5, activeSample)
	h.svc.Shutdown(h.real.Now())
	n := len(h.store.saves)
	h.clock.Advance(time.Minute)
	h.svc.Tick(h.clock.Now())
	if len(h.store.saves) != n {
		t.Fatal("после Shutdown появились новые сохранения")
	}
}

// Ловит: блокировка за обход не переживает перезапуск службы (Ruling 18): ребёнок
// выходом из учётки или перезапуском службы снимал бы её.
func TestBypassSurvivesRestart(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvBypass, Detail: "Окно проверки пароля закрыто насильно"})
	if st := h.status(); st.Lock != ipc.LockBypass || st.Detail != "Окно проверки пароля закрыто насильно" {
		t.Fatalf("после обхода: блокировка %q, подробности %q", st.Lock, st.Detail)
	}
	h.svc.Shutdown(h.real.Now())

	h2 := newHarness(t, withStore(h.store))
	h2.start()
	if st := h2.status(); st.Lock != ipc.LockBypass {
		t.Fatalf("после перезапуска блокировка за обход пропала: %q", st.Lock)
	}
}

// Ловит: повторное сообщение об обходе стирает первую подробность или плодит события
// (страж шлёт его каждую секунду, пока экран показан).
func TestRepeatedBypassKeepsFirstDetailAndOneEvent(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120))
	h.start()
	h.childLogsIn()
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvBypass, Detail: "первое"})
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvBypass, Detail: "второе"})
	if st := h.status(); st.Detail != "первое" {
		t.Fatalf("подробность %q, ждали первую", st.Detail)
	}
	if n := len(h.store.eventsOf("bypass")); n != 1 {
		t.Fatalf("событий bypass %d, ждали 1", n)
	}
}

// Ловит: учёт идёт при остановленных часах тестового режима: тик с тем же моментом
// засчитывал бы секунду каждый раз.
func TestPausedClockSkipsTicks(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), withTestMode(TestMode{ClockSpeed: 1}))
	h.start()
	h.childLogsIn()
	h.run(5, activeSample)
	var cr ipc.TestClockReply
	h.ok(adminPeer(), "", ipc.CmdTestClock, ipc.TestClockRequest{Paused: true}, &cr)
	if !cr.Paused || !h.clock.isPaused() {
		t.Fatalf("часы не остановлены: %+v", cr)
	}
	for i := 0; i < 10; i++ {
		h.svc.Tick(h.clock.Now())
	}
	if got := h.usedS(); got != 5 {
		t.Fatalf("на паузе учтено %d с, ждали 5", got)
	}
	h.ok(adminPeer(), "", ipc.CmdTestClock, ipc.TestClockRequest{Paused: false}, &cr)
	h.run(3, activeSample)
	if got := h.usedS(); got != 8 {
		t.Fatalf("после продолжения учтено %d с, ждали 8", got)
	}
}

// Ловит: событие ядра (доп. время, итог дня) записывается в журнал без ребёнка или с
// чужим ребёнком.
func TestEventsAreWrittenForTheirChild(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 600), withChild(child2SID, "Коля", 600), configured())
	h.clock.Set(time.Date(2026, 10, 19, 23, 59, 58, 0, tzMSK))
	h.start()
	h.childLogsIn()
	h.run(4, activeSample)
	evs := h.store.eventsOf("day_closed")
	// закрылся день и у второго ребёнка: у него нет сеанса, но сутки меняются у всех
	if len(evs) != 2 {
		t.Fatalf("событий day_closed %d, ждали 2 (по одному на ребёнка)", len(evs))
	}
	seen := map[int64]bool{}
	for _, e := range evs {
		seen[e.ChildID] = true
	}
	if !seen[h.childID(childSID)] || !seen[h.childID(child2SID)] || seen[0] {
		t.Fatalf("события привязаны к детям неверно: %v", seen)
	}
}
