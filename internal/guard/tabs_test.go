package guard

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// Ловит (RM4-2, PL-41): лестница закрытия вкладки не доходит до кнопки вкладки и WM_CLOSE, второй
// Ctrl+W раньше 2 с закрыл бы соседнюю вкладку, новое окно продолжало бы чужую лестницу. Ожидания —
// литералы, шаги одной лестницы по порядку.
func TestNextClose(t *testing.T) {
	look := TabLook{Window: 7, Brave: true, Domain: "example.com"}
	other := look
	other.Window = 9
	steps := []struct {
		look     TabLook
		at       time.Time
		wantWay  string
		wantStep int
	}{
		{look, t0, "ctrlW", 1},
		{look, at(time.Second), "", 1},
		{look, at(2 * time.Second), "invoke", 2},
		{look, at(3 * time.Second), "", 2},
		{look, at(4 * time.Second), "wmClose", 3},
		{look, at(6 * time.Second), "wmClose", 3},
		{other, at(6500 * time.Millisecond), "ctrlW", 1},
	}
	var l tabLadder
	for i, s := range steps {
		way, next := nextClose(l, "example.com", false, s.look, s.at)
		if way != s.wantWay || next.step != s.wantStep {
			t.Fatalf("шаг %d: way %q, step %d; ждали %q, %d", i+1, way, next.step, s.wantWay, s.wantStep)
		}
		l = next
	}
	if l.window != 9 || l.domain != "example.com" || !l.at.Equal(at(6500*time.Millisecond)) {
		t.Fatalf("лестница после смены окна: %+v", l)
	}
}

// Ловит (RM4-2, PL-9, Review Focus 2): команда исполнилась бы в окне, которое уже не Brave, во
// вкладке с другим доменом или при непрочитанном адресе.
func TestNextCloseNeedsSameBraveTab(t *testing.T) {
	base := TabLook{Window: 7, Brave: true, Domain: "example.com"}
	cases := []struct {
		name   string
		look   TabLook
		domain string
	}{
		{"другая вкладка", TabLook{Window: 7, Brave: true, Domain: "school.example"}, "example.com"},
		{"не Brave", TabLook{Window: 7, Brave: false, Domain: "example.com"}, "example.com"},
		{"адрес не прочитан", TabLook{Window: 7, Brave: true, Domain: "example.com", Unread: true}, "example.com"},
		{"нет окна", TabLook{Window: 0, Brave: true, Domain: "example.com"}, "example.com"},
		{"команда без домена", base, ""},
	}
	for _, c := range cases {
		if way, next := nextClose(tabLadder{}, c.domain, false, c.look, t0); way != "" || next.step != 0 {
			t.Errorf("%s: way %q, step %d; ждали ничего", c.name, way, next.step)
		}
	}
}

// Уточнение оркестратора к T7-3: если после действия вкладок стало меньше, действие удалось —
// следующая вкладка того же сайта закрывается снова Ctrl+W, окно Brave целиком не закрывается.
// Вкладки не посчитаны (0) — считается, что не уменьшились.
func TestNextCloseResetsWhenTabsDecreased(t *testing.T) {
	tabs := func(n int) TabLook { return TabLook{Window: 7, Brave: true, Domain: "example.com", Tabs: n} }
	steps := []struct {
		look     TabLook
		at       time.Time
		wantWay  string
		wantStep int
	}{
		{tabs(3), t0, "ctrlW", 1},
		{tabs(2), at(time.Second), "ctrlW", 1},
		{tabs(2), at(2 * time.Second), "", 1},
		{tabs(2), at(3 * time.Second), "invoke", 2},
		{tabs(1), at(4 * time.Second), "ctrlW", 1},
		{tabs(1), at(6 * time.Second), "invoke", 2},
		{tabs(0), at(7 * time.Second), "", 2},
		{tabs(0), at(8 * time.Second), "wmClose", 3},
		{tabs(1), at(10 * time.Second), "wmClose", 3},
		{tabs(3), at(12 * time.Second), "wmClose", 3},
		{tabs(2), at(12500 * time.Millisecond), "ctrlW", 1},
	}
	var l tabLadder
	for i, s := range steps {
		way, next := nextClose(l, "example.com", false, s.look, s.at)
		if way != s.wantWay || next.step != s.wantStep {
			t.Fatalf("шаг %d (вкладок %d): way %q, step %d; ждали %q, %d", i+1, s.look.Tabs, way, next.step, s.wantWay, s.wantStep)
		}
		l = next
	}
}

// Ловит (RM4-1, T7-6): окно Brave без адреса закрывалось бы, когда адрес уже прочитан, чаще раза в
// 2 с или не в Brave.
func TestNextCloseBraveWindow(t *testing.T) {
	look := TabLook{Window: 7, Brave: true, Unread: true, NoAddress: true}
	steps := []struct {
		look    TabLook
		at      time.Time
		wantWay string
	}{
		{look, t0, "wmClose"},
		{look, at(time.Second), ""},
		{look, at(2 * time.Second), "wmClose"},
		{TabLook{Window: 7, Brave: true, Unread: false}, at(5 * time.Second), ""},
		{TabLook{Window: 7, Brave: false, Unread: true}, at(5 * time.Second), ""},
		{TabLook{Window: 0, Brave: true, Unread: true}, at(5 * time.Second), ""},
		{TabLook{Window: 9, Brave: true, Unread: true, NoAddress: true}, at(2500 * time.Millisecond), "wmClose"},
	}
	var l tabLadder
	for i, s := range steps {
		way, next := nextClose(l, "", true, s.look, s.at)
		if way != s.wantWay {
			t.Fatalf("шаг %d: way %q; ждали %q", i+1, way, s.wantWay)
		}
		l = next
	}
}

// Плашки сайтов (RM4-9) уступают плашке 1 мин; плашки о программах — нет (M3 без изменений).
func TestIsSitePlate(t *testing.T) {
	for kind, want := range map[string]bool{
		ipc.BlockedSite: true, ipc.BlockedAdult: true, ipc.BlockedVideoOver: true, ipc.BlockedBrowser: true,
		ipc.BlockedForbidden: false, ipc.BlockedUnknown: false, ipc.BlockedStudyOnly: false,
		ipc.BlockedGamesOver: false, ipc.BlockedDecided: false, "": false,
	} {
		if got := isSitePlate(kind); got != want {
			t.Errorf("isSitePlate(%q) = %v, ждали %v", kind, got, want)
		}
	}
}

// --- исполнение в контроллере (T7-4, T7-7, T7-9) ----------------------------------------------

const testBrave = `C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`

// fakeTabs — подделка TabActor: передний план look, ошибки Close по очереди, журнал вызовов
// «<способ> <окно>».
type fakeTabs struct {
	mu    sync.Mutex
	look  TabLook
	errs  []error
	calls []string
}

func (f *fakeTabs) Look() TabLook {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.look
}

func (f *fakeTabs) Close(way string, window uintptr) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf("%s %d", way, window))
	if len(f.errs) == 0 {
		return nil
	}
	err := f.errs[0]
	f.errs = f.errs[1:]
	return err
}

func (f *fakeTabs) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.calls...)
}

func tabRig(test bool, look TabLook) (*ctRig, *fakeTabs) {
	r := newCtRig()
	r.svc.test = test
	r.svc.status = ctRunningStatus(3600)
	f := &fakeTabs{look: look}
	r.ctl.SetTabActor(f)
	return r, f
}

func closeTabCmd(domain string) []ipc.GuardCommand {
	return []ipc.GuardCommand{{Kind: ipc.GuardCmdCloseTab, Domain: domain}}
}

func domainCmd(text string) ipc.GuardCommand {
	return ipc.GuardCommand{Kind: ipc.GuardCmdTest, Test: &ipc.TestGuardAction{Action: ipc.TestDomain, Text: text}}
}

func sameCalls(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Ловит (PL-41, RM4-2): команда closeTab из ответа службы не доходит до исполнителя, лестница не
// поднимается до кнопки вкладки и WM_CLOSE, ClosedTab не попадает в GuardState того же кадра,
// пропуск команды не сбрасывает лестницу.
func TestCloseTabRunsLadderThroughController(t *testing.T) {
	r, f := tabRig(true, TabLook{Window: 7, Brave: true, Domain: "example.com"})
	for i := 0; i < 5; i++ {
		if i > 0 {
			r.clk.Advance(time.Second)
		}
		r.svc.cmds = closeTabCmd("example.com")
		r.ctl.PollOnce()
		r.ctl.PushState()
		if i == 0 {
			if st := lastPushed(t, r); st.ClosedTab != "example.com" || st.ClosedTabWay != "ctrlW" {
				t.Fatalf("после первого опроса ClosedTab %q, ClosedTabWay %q", st.ClosedTab, st.ClosedTabWay)
			}
		}
	}
	if got := f.got(); !sameCalls(got, "ctrlW 7", "invoke 7", "wmClose 7") {
		t.Fatalf("вызовы %q", got)
	}
	if st := lastPushed(t, r); st.ClosedTab != "example.com" || st.ClosedTabWay != "wmClose" {
		t.Fatalf("ClosedTab %q, ClosedTabWay %q; ждали example.com, wmClose", st.ClosedTab, st.ClosedTabWay)
	}
	r.clk.Advance(time.Second)
	r.ctl.PollOnce() // ответ без команды: лестница сброшена
	r.clk.Advance(time.Second)
	r.svc.cmds = closeTabCmd("example.com")
	r.ctl.PollOnce()
	if got := f.got(); len(got) != 4 || got[3] != "ctrlW 7" {
		t.Fatalf("после ответа без команды вызовы %q; ждали снова ctrlW 7", got)
	}
}

// Ловит (RM4-2, T7-5): зажатый Shift засчитан как шаг — следующей была бы кнопка вкладки, а не
// новый Ctrl+W; ClosedTab записан, хотя ничего не нажато.
func TestCloseTabWaitsForModifiers(t *testing.T) {
	r, f := tabRig(true, TabLook{Window: 7, Brave: true, Domain: "example.com"})
	f.errs = []error{errModifierHeld}
	r.svc.cmds = closeTabCmd("example.com")
	r.ctl.PollOnce()
	r.ctl.PushState()
	if st := lastPushed(t, r); st.ClosedTab != "" || st.ClosedTabWay != "" {
		t.Fatalf("при зажатом модификаторе ClosedTab %q, ClosedTabWay %q", st.ClosedTab, st.ClosedTabWay)
	}
	r.clk.Advance(time.Second)
	r.svc.cmds = closeTabCmd("example.com")
	r.ctl.PollOnce()
	if got := f.got(); !sameCalls(got, "ctrlW 7", "ctrlW 7") {
		t.Fatalf("вызовы %q; ждали два ctrlW 7", got)
	}
	// Ошибка кнопки вкладки — шаг засчитан (дальше WM_CLOSE), но ClosedTab о ней не пишется.
	f.mu.Lock()
	f.errs = []error{errors.New("UIA: у выбранной вкладки нет кнопки закрытия")}
	f.mu.Unlock()
	r.clk.Advance(2 * time.Second)
	r.svc.cmds = closeTabCmd("example.com")
	r.ctl.PollOnce()
	r.ctl.PushState()
	if st := lastPushed(t, r); st.ClosedTabWay != "ctrlW" {
		t.Fatalf("после ошибки кнопки ClosedTabWay %q; ждали прежний ctrlW", st.ClosedTabWay)
	}
	r.clk.Advance(2 * time.Second)
	r.svc.cmds = closeTabCmd("example.com")
	r.ctl.PollOnce()
	if got := f.got(); !sameCalls(got, "ctrlW 7", "ctrlW 7", "invoke 7", "wmClose 7") {
		t.Fatalf("вызовы %q; ждали после ошибки кнопки wmClose 7", got)
	}
}

// Ловит (Review Focus 2, PL-9, PL-18): Ctrl+W ушёл бы окну, которое уже не Brave, вкладке с другим
// доменом или окну с непрочитанным адресом; команда без домена что-то закрыла бы.
func TestCloseTabRechecksForeground(t *testing.T) {
	cases := []struct {
		name string
		look TabLook
		cmd  string
	}{
		{"передний план не Brave", TabLook{Window: 9, Brave: false}, "example.com"},
		{"вкладка другая", TabLook{Window: 7, Brave: true, Domain: "school.example"}, "example.com"},
		{"команда без домена", TabLook{Window: 7, Brave: true, Domain: "example.com"}, ""},
		{"адрес не прочитан", TabLook{Window: 7, Brave: true, Domain: "example.com", Unread: true}, "example.com"},
	}
	for _, c := range cases {
		r, f := tabRig(true, c.look)
		r.svc.cmds = closeTabCmd(c.cmd)
		r.ctl.PollOnce()
		r.ctl.PushState()
		if got := f.got(); len(got) != 0 {
			t.Errorf("%s: вызовы %q; ждали ничего", c.name, got)
		}
		if st := lastPushed(t, r); st.ClosedTab != "" || st.ClosedTabWay != "" {
			t.Errorf("%s: ClosedTab %q, ClosedTabWay %q", c.name, st.ClosedTab, st.ClosedTabWay)
		}
	}
}

// Ловит (RM4-1, T7-6): окно Brave без адреса не закрывается по closeBraveWindow или закрывается,
// когда адрес уже прочитан.
func TestCloseBraveWindowCommand(t *testing.T) {
	r, f := tabRig(true, TabLook{Window: 7, Brave: true, Unread: true, NoAddress: true})
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdCloseBraveWindow}}
	r.ctl.PollOnce()
	r.ctl.PushState()
	if got := f.got(); !sameCalls(got, "wmClose 7") {
		t.Fatalf("вызовы %q; ждали wmClose 7", got)
	}
	if st := lastPushed(t, r); st.ClosedTab != "" || st.ClosedTabWay != "wmClose" {
		t.Fatalf("ClosedTab %q, ClosedTabWay %q; ждали «», wmClose", st.ClosedTab, st.ClosedTabWay)
	}
	r, f = tabRig(true, TabLook{Window: 7, Brave: true, Unread: false})
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdCloseBraveWindow}}
	r.ctl.PollOnce()
	if got := f.got(); len(got) != 0 {
		t.Fatalf("адрес прочитан, а окно закрыто: %q", got)
	}
}

// Ловит: закрытие работает только в тестовом режиме (в бою вкладка осталась бы открытой) или в бою
// шлёт GuardState.
func TestCloseTabOutsideTestMode(t *testing.T) {
	r, f := tabRig(false, TabLook{Window: 7, Brave: true, Domain: "example.com"})
	r.svc.cmds = closeTabCmd("example.com")
	r.ctl.PollOnce()
	r.ctl.PushState()
	if got := f.got(); !sameCalls(got, "ctrlW 7") {
		t.Fatalf("вне тестового режима вызовы %q; ждали ctrlW 7", got)
	}
	if n := len(r.svc.called(ipc.CmdGuardState)); n != 0 {
		t.Fatalf("вне тестового режима GuardState отправлен %d раз", n)
	}
}

// Ловит: без исполнителя (Linux, страж без окон) команда closeTab роняет опрос.
func TestCloseWithoutActor(t *testing.T) {
	r := newCtRig()
	r.svc.status = ctRunningStatus(3600)
	r.svc.cmds = append(closeTabCmd("example.com"), ipc.GuardCommand{Kind: ipc.GuardCmdCloseBraveWindow})
	if res := r.ctl.PollOnce(); res.Err != nil {
		t.Fatalf("опрос: %v", res.Err)
	}
}

// Ловит (RM4-12, PL-41): подмена домена для приёмки не доходит до снимка, действует без Brave на
// переднем плане или вне тестового режима, уходит окну как действие; исполнение не видит подмены
// или исполняет Ctrl+W, когда настоящий передний план не Brave.
func TestDomainOverrideInTestMode(t *testing.T) {
	path := testBrave
	r, f := tabRig(true, TabLook{Window: 7, Brave: true, Domain: "real.example"})
	r.ctl.read = func(now time.Time) ipc.SensorSample {
		return BuildSample(now, RawSensors{IdleMs: 1500, ForegroundPath: path, ForegroundDomain: "real.example", DomainUnread: true, DomainNoAddress: true}, 1)
	}
	r.poll()
	if s := r.lastSample(t); s.ForegroundDomain != "real.example" || !s.DomainUnread || !s.DomainNoAddress {
		t.Fatalf("настоящий снимок: %q, %v", s.ForegroundDomain, s.DomainUnread)
	}
	r.svc.cmds = []ipc.GuardCommand{domainCmd("youtube.com")}
	r.poll()
	if q := r.ctl.QueuedTests(); len(q) != 0 {
		t.Fatalf("подмена ушла окну: %+v", q)
	}
	r.poll()
	if s := r.lastSample(t); s.ForegroundDomain != "youtube.com" || s.DomainUnread || s.DomainNoAddress {
		t.Fatalf("после подмены снимок: %q, %v, %v; ждали youtube.com, false, false", s.ForegroundDomain, s.DomainUnread, s.DomainNoAddress)
	}
	if st := r.lastState(t); st.ForegroundDomain != "youtube.com" {
		t.Fatalf("GuardState.foregroundDomain = %q", st.ForegroundDomain)
	}

	// Исполнение видит подмену, только если настоящий передний план — Brave.
	r.svc.cmds = closeTabCmd("youtube.com")
	r.poll()
	if got := f.got(); !sameCalls(got, "ctrlW 7") {
		t.Fatalf("closeTab с подменой: вызовы %q; ждали ctrlW 7", got)
	}
	f.mu.Lock()
	f.look = TabLook{Window: 9, Brave: false, Domain: ""}
	f.mu.Unlock()
	r.clk.Advance(3 * time.Second)
	r.svc.cmds = closeTabCmd("youtube.com")
	r.poll()
	if got := f.got(); len(got) != 1 {
		t.Fatalf("настоящий передний план не Brave, а вызовы %q", got)
	}

	// Подмена foreground на не-Brave: домен не подменяется.
	r.svc.cmds = []ipc.GuardCommand{foregroundCmd(`c:\x\chrome.exe`)}
	r.poll()
	r.poll()
	if s := r.lastSample(t); s.ForegroundDomain != "real.example" {
		t.Fatalf("путь не Brave, а домен %q", s.ForegroundDomain)
	}
	// Подмена foreground на Brave при настоящем chrome.exe: подмена домена действует (S18).
	path = `C:\Program Files\Google\Chrome\Application\chrome.exe`
	r.svc.cmds = []ipc.GuardCommand{foregroundCmd(testBrave)}
	r.poll()
	r.poll()
	if s := r.lastSample(t); s.ForegroundDomain != "youtube.com" {
		t.Fatalf("подменённый путь Brave, а домен %q", s.ForegroundDomain)
	}
	r.svc.cmds = []ipc.GuardCommand{foregroundCmd(""), domainCmd("")}
	r.poll()
	path = testBrave
	r.poll()
	if s := r.lastSample(t); s.ForegroundDomain != "real.example" || !s.DomainUnread {
		t.Fatalf("после пустой подмены снимок: %q, %v", s.ForegroundDomain, s.DomainUnread)
	}

	r, _ = tabRig(false, TabLook{})
	r.ctl.read = func(now time.Time) ipc.SensorSample {
		return BuildSample(now, RawSensors{ForegroundPath: testBrave, ForegroundDomain: "real.example"}, 1)
	}
	r.svc.cmds = []ipc.GuardCommand{domainCmd("youtube.com")}
	r.poll()
	r.poll()
	if s := r.lastSample(t); s.ForegroundDomain != "real.example" {
		t.Fatalf("вне тестового режима подмена сработала: %q", s.ForegroundDomain)
	}
}

// Ловит (ревью M4 F1-3): Ctrl+W не отправлен, потому что сменился передний план, а шаг лестницы
// засчитан и ClosedTab записан: следующий опрос пошёл бы сразу на кнопку вкладки.
func TestCloseTabForegroundChangedNotCounted(t *testing.T) {
	r, f := tabRig(true, TabLook{Window: 7, Brave: true, Domain: "example.com"})
	f.errs = []error{errForegroundChanged}
	r.svc.cmds = closeTabCmd("example.com")
	r.ctl.PollOnce()
	r.ctl.PushState()
	if st := lastPushed(t, r); st.ClosedTab != "" || st.ClosedTabWay != "" {
		t.Fatalf("при сменившемся окне ClosedTab %q, ClosedTabWay %q", st.ClosedTab, st.ClosedTabWay)
	}
	r.clk.Advance(2 * time.Second)
	r.svc.cmds = closeTabCmd("example.com")
	r.ctl.PollOnce()
	if got := f.got(); !sameCalls(got, "ctrlW 7", "ctrlW 7") {
		t.Fatalf("вызовы %q; ждали два ctrlW 7 (первый шаг не засчитан)", got)
	}
}

// Ловит (ревью M4 F1-7, PL-9): сбой UIA (ошибка, нет ответа, занят) неотличим от окна без адреса —
// при исчерпанном видео окно Brave со страницами закрывается WM_CLOSE целиком, хотя адрес просто
// не прочитан. Окно, у которого адреса нет, закрывается как раньше.
func TestCloseBraveWindowOnlyWithoutAddress(t *testing.T) {
	r, f := tabRig(true, TabLook{Window: 7, Brave: true, Unread: true})
	for i := 0; i < 5; i++ {
		r.clk.Advance(time.Second)
		r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdCloseBraveWindow}}
		r.ctl.PollOnce()
	}
	if got := f.got(); len(got) != 0 {
		t.Fatalf("5 с сбоев UIA: вызовы %q, want нет", got)
	}
	f.mu.Lock()
	f.look = TabLook{Window: 7, Brave: true, Unread: true, NoAddress: true}
	f.mu.Unlock()
	r.clk.Advance(time.Second)
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdCloseBraveWindow}}
	r.ctl.PollOnce()
	if got := f.got(); !sameCalls(got, "wmClose 7") {
		t.Fatalf("окно без адреса: вызовы %q, want wmClose 7", got)
	}
}
