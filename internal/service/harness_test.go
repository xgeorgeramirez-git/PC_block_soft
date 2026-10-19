package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Константы теста: пн 19.10.2026 (грабля «тесты с датами»: даты у границы суток только константами).
const (
	parentSID = "S-1-5-21-1-1-1-1000"
	childSID  = "S-1-5-21-1-1-1-1001"
	otherSID  = "S-1-5-21-1-1-1-1002"
	child2SID = "S-1-5-21-1-1-1-1003"
	systemSID = "S-1-5-18"

	goodPassword = "секрет-123"
)

var (
	mon1500 = time.Date(2026, 10, 19, 15, 0, 0, 0, tzMSK) // понедельник 15:00 (часы ядра)
	realT0  = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
)

// harness — служба на поддельных портах.
type harness struct {
	t        *testing.T
	svc      *Service
	store    *fakeStore
	clock    *fakeClock // часы ядра
	real     *fakeClock // настоящие часы
	launcher *fakeLauncher
	admins   *fakeAdmins
	policy   *fakePolicy
	hasher   *fakeHasher
	accounts *fakeAccounts
	cfg      Config
	dataDir  string

	logMu  sync.Mutex
	logs   []string
	tokenN int
}

type hopt func(*harness)

// newHarness собирает службу; настройка до Start. Службу ещё не запускает.
func newHarness(t *testing.T, opts ...hopt) *harness {
	t.Helper()
	h := &harness{
		t: t, store: newFakeStore(), clock: newClock(mon1500), real: newClock(realT0),
		launcher: newLauncher(), admins: newAdmins(), policy: newPolicy(), hasher: &fakeHasher{},
		dataDir: t.TempDir(),
		accounts: &fakeAccounts{list: []AccountInfo{
			{SID: parentSID, Name: "Анна", Admin: true},
			{SID: childSID, Name: "Миша"},
			{SID: otherSID, Name: "Даша"},
			{SID: child2SID, Name: "Коля"},
		}},
	}
	h.cfg = Config{
		Store: h.store, Hasher: h.hasher, Launcher: h.launcher, Admins: h.admins, Accounts: h.accounts,
		Policy: h.policy, Clock: h.clock, RealNow: h.real.Now, Loc: tzMSK, Version: "0.1.0-test", DataDir: h.dataDir,
		Rand: func() string { h.tokenN++; return fmt.Sprintf("tok-%d", h.tokenN) },
		Logf: func(f string, a ...any) {
			h.logMu.Lock()
			h.logs = append(h.logs, fmt.Sprintf(f, a...))
			h.logMu.Unlock()
		},
	}
	for _, o := range opts {
		o(h)
	}
	svc, err := New(h.cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.svc = svc
	return h
}

func (h *harness) logText() string {
	h.logMu.Lock()
	defer h.logMu.Unlock()
	return strings.Join(h.logs, "\n")
}

// withTestMode включает тестовый режим.
func withTestMode(tm TestMode) hopt {
	return func(h *harness) { tm.Enabled = true; h.cfg.Test = tm }
}

// withChild заранее записывает ребёнка с общим лимитом min минут на каждый день (до Start).
func withChild(sid, name string, min int) hopt {
	return func(h *harness) {
		id, err := h.store.UpsertChild(sid, name)
		if err != nil {
			h.t.Fatal(err)
		}
		var week [7]core.DayLimit
		for i := range week {
			week[i] = core.LimitOf(min)
		}
		h.store.SetDayLimits(id, week)
	}
}

// configured записывает пароль родителя (до Start): программа настроена.
func configured() hopt {
	return func(h *harness) {
		h.store.auth = &Auth{Hash: "H:" + goodPassword, ChangedAtMs: 1}
	}
}

// start запускает службу.
func (h *harness) start() {
	h.t.Helper()
	if err := h.svc.Start(h.real.Now()); err != nil {
		h.t.Fatalf("Start: %v", err)
	}
}

// childID возвращает id ребёнка по SID.
func (h *harness) childID(sid string) int64 {
	h.t.Helper()
	cs, _ := h.store.ListChildren()
	for _, c := range cs {
		if c.SID == sid {
			return c.ID
		}
	}
	h.t.Fatalf("нет ребёнка %s", sid)
	return 0
}

// --- кто звонит --------------------------------------------------------------------------

const childSession = 2

func childPeer() ipc.Peer { return ipc.Peer{PID: 500, SessionID: childSession, SID: childSID} }
func otherPeer() ipc.Peer { return ipc.Peer{PID: 510, SessionID: 3, SID: otherSID} }
func adminPeer() ipc.Peer { return ipc.Peer{PID: 520, SessionID: 1, SID: parentSID, Admin: true} }
func elevatedPeer() ipc.Peer {
	return ipc.Peer{PID: 530, SessionID: 1, SID: parentSID, Admin: true, Elevated: true}
}
func guardPeer() ipc.Peer {
	return ipc.Peer{PID: 600, SessionID: childSession, SID: systemSID, System: true}
}
func systemOtherPeer() ipc.Peer {
	return ipc.Peer{PID: 610, SessionID: 7, SID: systemSID, System: true}
}

// peerKinds — все виды клиента из пакета поручения.
var peerKinds = []struct {
	name string
	peer ipc.Peer
}{
	{"ребёнок своего сеанса", childPeer()},
	{"другой пользователь", otherPeer()},
	{"администратор без повышения", adminPeer()},
	{"повышенный администратор", elevatedPeer()},
	{"системный страж того же сеанса", guardPeer()},
	{"системный процесс другого сеанса", systemOtherPeer()},
}

// --- вызовы -------------------------------------------------------------------------------

// do отправляет команду; req (или nil) кладётся в нагрузку.
func (h *harness) do(p ipc.Peer, token, cmd string, req any) ipc.Reply {
	h.t.Helper()
	m := ipc.Msg{Cmd: cmd, Token: token}
	if req != nil {
		m.Payload = ipc.Payload(req)
	}
	return h.svc.Handle(p, m, h.real.Now())
}

// ok проверяет успешный ответ и разбирает нагрузку в resp (nil: не нужна).
func (h *harness) ok(p ipc.Peer, token, cmd string, req, resp any) {
	h.t.Helper()
	rep := h.do(p, token, cmd, req)
	if !rep.OK {
		h.t.Fatalf("%s: отказ %q (поле %q)", cmd, rep.Error, rep.Field)
	}
	if resp != nil {
		if err := rep.Into(resp); err != nil {
			h.t.Fatalf("%s: нагрузка: %v", cmd, err)
		}
	}
}

// fail проверяет отказ и возвращает ответ.
func (h *harness) fail(p ipc.Peer, token, cmd string, req any) ipc.Reply {
	h.t.Helper()
	rep := h.do(p, token, cmd, req)
	if rep.OK {
		h.t.Fatalf("%s: ждали отказ, получили успех %s", cmd, rep.Payload)
	}
	return rep
}

// login входит родителем из процесса p и возвращает токен.
func (h *harness) login(p ipc.Peer) string {
	h.t.Helper()
	var lr ipc.LoginReply
	h.ok(p, "", ipc.CmdLogin, ipc.LoginRequest{Password: goodPassword}, &lr)
	if lr.Token == "" {
		h.t.Fatal("пустой токен")
	}
	return lr.Token
}

// --- сеансы и тики -----------------------------------------------------------------------

// childLogsIn: ребёнок вошёл в сеанс 2, служба запускает страж и значок.
func (h *harness) childLogsIn() {
	h.t.Helper()
	h.launcher.setSessions(SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID})
	h.svc.OnSession(SessionLogon, childSession, h.real.Now())
}

var (
	activeSample = ipc.SensorSample{SessionActive: true, LastInputAgoMs: 0, Foreground: ipc.CatNameOther}
	idleSample   = ipc.SensorSample{SessionActive: false}
)

// poll отправляет GuardPoll от стража сеанса 2.
func (h *harness) poll(s ipc.SensorSample, events ...ipc.GuardEvent) ipc.GuardPollReply {
	h.t.Helper()
	req := ipc.GuardPollRequest{Sample: s, Events: events}
	var rep ipc.GuardPollReply
	h.ok(guardPeer(), "", ipc.CmdGuardPoll, req, &rep)
	return rep
}

// step: проходит одна секунда: часы идут, страж присылает снимок, служба тикает и обслуживает.
func (h *harness) step(s ipc.SensorSample) ipc.GuardPollReply {
	h.t.Helper()
	h.clock.Advance(time.Second)
	h.real.Advance(time.Second)
	rep := h.poll(s)
	h.svc.Tick(h.clock.Now())
	h.svc.Housekeep(h.real.Now())
	return rep
}

// run выполняет n секунд со снимком s и возвращает ответы стража по порядку.
func (h *harness) run(n int, s ipc.SensorSample) []ipc.GuardPollReply {
	h.t.Helper()
	out := make([]ipc.GuardPollReply, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, h.step(s))
	}
	return out
}

// status возвращает статус ребёнка для значка (GetChildStatus от процесса ребёнка).
func (h *harness) status() ipc.ChildStatus {
	h.t.Helper()
	var st ipc.ChildStatus
	h.ok(childPeer(), "", ipc.CmdGetChildStatus, nil, &st)
	return st
}

// dayState возвращает состояние дня ребёнка из ядра через слой команд родителя (без токена
// нельзя), поэтому читаем сохранённое в хранилище при Shutdown; для тестов учёта есть usedS.
func (h *harness) usedS() int { return h.status().UsedS }

// jsonOf возвращает нагрузку ответа как текст.
func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// procsFile возвращает путь файла памяти о процессах.
func (h *harness) procsFile() string { return filepath.Join(h.dataDir, procStateFile) }

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// withStore подставляет готовое хранилище (перезапуск службы на тех же данных).
func withStore(st *fakeStore) hopt {
	return func(h *harness) { h.store = st; h.cfg.Store = st }
}

// withRecovered имитирует запуск после порчи базы (R18, Ruling 25).
func withRecovered(copyName string) hopt {
	return func(h *harness) { h.cfg.Recovered, h.cfg.BrokenCopy = true, copyName }
}

// guardPeerOf — страж сеанса session.
func guardPeerOf(session uint32) ipc.Peer {
	return ipc.Peer{PID: 600 + session, SessionID: session, SID: systemSID, System: true}
}

// child2Session — сеанс второго ребёнка.
const child2Session = 3

func (h *harness) child2LogsIn() {
	h.t.Helper()
	h.launcher.setSessions(
		SessionInfo{ID: childSession, User: `PC\Миша`, SID: childSID},
		SessionInfo{ID: child2Session, User: `PC\Коля`, SID: child2SID},
	)
	h.svc.OnSession(SessionLogon, child2Session, h.real.Now())
}

// trayPeer возвращает клиента: настоящий значок сеанса (PID запущенного службой процесса).
func (h *harness) trayPeer(session uint32, sid string) ipc.Peer {
	h.t.Helper()
	l := h.launcher.aliveOf(ProcTray, session)
	if len(l) == 0 {
		h.t.Fatalf("в сеансе %d нет живого значка", session)
	}
	return ipc.Peer{PID: l[0].pid, SessionID: session, SID: sid}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

// systemClockForTest — часы, которые нельзя остановить.
type systemClockForTest struct{}

func (systemClockForTest) Now() time.Time { return time.Now() }
