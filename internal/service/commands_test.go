package service

import (
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// rightsDenials — тексты, которыми служба отказывает по правам (L-112): по ним матрица
// отличает «нельзя» от «можно, но запрос пустой».
var rightsDenials = []string{
	textDenyKind, textNeedParent, textNeedAdmin, textTestOnly, textSessionEnded, textAlreadySetup,
}

func isRightsDenial(rep ipc.Reply) bool {
	if rep.OK {
		return false
	}
	for _, d := range rightsDenials {
		if rep.Error == d {
			return true
		}
	}
	return false
}

// Ловит: новая команда канала без строки в таблице прав (команду принимали бы или отклоняли
// как «неразобранную» без решения о правах), или строка для команды, которой нет.
func TestEveryChannelCommandHasRightsRule(t *testing.T) {
	for _, cmd := range ipc.AllCommands {
		r, ok := commandRules[cmd]
		if !ok || r.allow == nil || r.handler == nil {
			t.Errorf("у команды %s нет строки в таблице прав", cmd)
		}
	}
	if len(commandRules) != len(ipc.AllCommands) {
		t.Errorf("в таблице прав %d строк, команд канала %d", len(commandRules), len(ipc.AllCommands))
	}
}

// Ловит: неизвестная команда выполняется или роняет службу; ответ без человеческого текста.
func TestUnknownCommandIsRefusedPolitely(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	rep := h.fail(adminPeer(), "", "НеТакаяКоманда", nil)
	if rep.Error != ipc.TextBadRequest {
		t.Fatalf("ответ %q", rep.Error)
	}
}

// Ловит: паника в обработчике команды убивает службу (R18: ошибка службы не должна
// останавливать защиту) или ответ человеку содержит технический текст.
func TestHandlerPanicBecomesFriendlyError(t *testing.T) {
	const cmd = "ПаникаТест"
	commandRules[cmd] = rule{anyone(), func(*Service, *callCtx) ipc.Reply { panic("сломалось") }}
	defer delete(commandRules, cmd)
	h := newHarness(t, configured())
	h.start()
	rep := h.fail(adminPeer(), "", cmd, nil)
	if rep.Error != ipc.TextInternal || strings.Contains(rep.Error, "сломалось") {
		t.Fatalf("ответ %q", rep.Error)
	}
	h.ok(adminPeer(), "", ipc.CmdHello, nil, nil) // служба жива и отвечает
	if !strings.Contains(h.logText(), "сломалось") {
		t.Errorf("причина паники не попала в журнал работы:\n%s", h.logText())
	}
}

// wantAllowed — таблица прав из ТЗ (3.2, R19, R20, NFR-9, Ruling 5, Review Focus 7),
// записанная отдельно от кода службы: что разрешено какому клиенту. tok: "none", "bogus",
// "own" (токен самого клиента), "foreign" (токен чужого процесса).
func wantAllowed(cmd, kind, tok string, test, configured bool) bool {
	user := kind == "child" || kind == "other" || kind == "admin" || kind == "elev"
	own := tok == "own"
	switch cmd {
	case ipc.CmdHello:
		return true
	case ipc.CmdOpenWindow:
		// Окна пользователя; страж своего сеанса — окно родителя (пароль спросит окно, RM3-4) и
		// «Моя статистика» (плашка 1 мин, RM3-5).
		return user || kind == "guard"
	case ipc.CmdGetChildStatus, ipc.CmdWindowClosing, ipc.CmdLogout, ipc.CmdSessionState,
		ipc.CmdExtendSession, ipc.CmdMyToday, ipc.CmdFirstRunSeen, ipc.CmdTrayClosing:
		return user
	case ipc.CmdLogin:
		return user || kind == "guard"
	case ipc.CmdListAccounts:
		return (kind == "elev" && !configured) || (user && own)
	case ipc.CmdSetup:
		return kind == "elev" && !configured
	case ipc.CmdResetPassword, ipc.CmdUninstallCheck:
		return kind == "elev"
	case ipc.CmdGetToday, ipc.CmdGetChildRules, ipc.CmdSaveLimits, ipc.CmdAddExtra, ipc.CmdReduceExtra, ipc.CmdClearExtra, ipc.CmdUndoExtra,
		ipc.CmdClearBlock, ipc.CmdListEvents, ipc.CmdChangePassword, ipc.CmdSetChildren, ipc.CmdParentStats, ipc.CmdGetDiag, ipc.CmdSetDiag,
		ipc.CmdOpenLogs, ipc.CmdSetNoForbidden, ipc.CmdClearBonus, ipc.CmdReduceBonus, ipc.CmdCreatePromo, ipc.CmdListPromos,
		ipc.CmdGetBonusRate, ipc.CmdSetBonusRate:
		// Родительские (parentToken): нужен свой токен (R19).
		return user && own
	case ipc.CmdRedeemPromo:
		// Ребёнок своего сеанса вводит промокод (childSelf): экран блокировки и «Бонусы» (R26, D-50).
		return kind == "child" || kind == "guard"
	case ipc.CmdGuardPoll, ipc.CmdGuardLogoff, ipc.CmdGuardShutdown, ipc.CmdGuardSwitchUser, ipc.CmdGuardClosing:
		return kind == "guard"
	case ipc.CmdTestGuard, ipc.CmdTestClock, ipc.CmdTestSession:
		return test && (kind == "admin" || kind == "elev")
	case ipc.CmdGuardState:
		return test && (kind == "guard" || kind == "admin" || kind == "elev")
	case ipc.CmdSaveSchedule, ipc.CmdPause, ipc.CmdPauseAll, ipc.CmdGrantBonus, ipc.CmdUndoBonus, ipc.CmdCopyRules,
		ipc.CmdGetSchedule:
		// Родительские (parentToken): нужен свой токен (R19).
		return user && own
	case ipc.CmdGetBonus:
		// Родитель по токену или ребёнок своего сеанса о себе (childSelf).
		return (user && own) || kind == "child" || kind == "guard"
	case ipc.CmdAskMore:
		// «Попросить ещё»: любой пользователь (пароль проверяется внутри, FR-10).
		return user
	case ipc.CmdSpendBonus:
		// «Взять из копилки»: ребёнок своего сеанса без пароля (childSelf, FR-11).
		return kind == "child" || kind == "guard"
	case ipc.CmdListPrograms, ipc.CmdDecideProgram, ipc.CmdSetProgramsCategory, ipc.CmdUndoProgram, ipc.CmdSetProgramsHidden, ipc.CmdRenameProgram:
		// «Программы» (FR-20): только родитель своим токеном; ребёнок не читает и не меняет.
		return user && own
	case ipc.CmdScanPrograms:
		// Шаг мастера «Программы» (FR-2): как ListAccounts до настройки, после настройки — никому.
		return kind == "elev" && !configured
	}
	return false
}

// Ловит: любая ячейка матрицы «команда × вид клиента × токен × тестовый режим × настроена»
// разрешает лишнее или запрещает нужное (Review Focus 7, NFR-9).
func TestRightsMatrix(t *testing.T) {
	kinds := []struct {
		name string
		peer ipc.Peer
	}{
		{"child", childPeer()}, {"other", otherPeer()}, {"admin", adminPeer()}, {"elev", elevatedPeer()},
		{"guard", guardPeer()}, {"systemOther", systemOtherPeer()},
	}
	for _, cmd := range ipc.AllCommands {
		for _, test := range []bool{false, true} {
			for _, conf := range []bool{false, true} {
				for _, k := range kinds {
					for _, tok := range []string{"none", "bogus", "own", "foreign"} {
						if !conf && (tok == "own" || tok == "foreign") {
							continue // без пароля токен получить нечем
						}
						opts := []hopt{withChild(childSID, "Миша", 120)}
						if conf {
							opts = append(opts, configured())
						}
						if test {
							opts = append(opts, withTestMode(TestMode{ClockSpeed: 1}))
						}
						h := newHarness(t, opts...)
						h.start()
						h.childLogsIn()
						token := ""
						switch tok {
						case "bogus":
							token = "не-токен"
						case "own":
							if k.peer.System {
								continue
							}
							token = h.login(k.peer)
						case "foreign":
							other := adminPeer()
							if k.name == "admin" {
								other = elevatedPeer()
							}
							token = h.login(other)
						}
						rep := h.do(k.peer, token, cmd, nil)
						want := wantAllowed(cmd, k.name, tok, test, conf)
						if got := !isRightsDenial(rep); got != want {
							t.Errorf("%s от «%s», токен %s, тест=%v, настроена=%v: разрешено=%v, ждали %v (ответ %q)",
								cmd, k.name, tok, test, conf, got, want, rep.Error)
						}
						if !want && !rep.OK && (rep.Error == "" || strings.Contains(rep.Error, "token") || strings.Contains(rep.Error, "PID")) {
							t.Errorf("%s: отказ с нечеловеческим текстом %q", cmd, rep.Error)
						}
					}
				}
			}
		}
	}
}

// --- вход родителя и паузы (R19) ---------------------------------------------------------

func (h *harness) wrongLogin(p ipc.Peer) ipc.Reply {
	h.t.Helper()
	return h.fail(p, "", ipc.CmdLogin, ipc.LoginRequest{Password: "не тот пароль"})
}

func deniedOf(t *testing.T, rep ipc.Reply) ipc.LoginDenied {
	t.Helper()
	var d ipc.LoginDenied
	if err := rep.Into(&d); err == nil {
		t.Fatal("отказ не должен разбираться как успех")
	}
	if len(rep.Payload) == 0 {
		t.Fatalf("у отказа нет подробностей: %+v", rep)
	}
	if err := jsonUnmarshal(rep.Payload, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// Ловит: верный пароль не выдаёт токен или токен не привязан к сроку 10 мин (R19).
func TestLoginIssuesTokenWithTenMinutes(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	var lr ipc.LoginReply
	h.ok(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: goodPassword}, &lr)
	if lr.Token == "" || lr.IdleTimeoutS != 600 {
		t.Fatalf("ответ %+v", lr)
	}
	if want := h.real.Now().Add(10 * time.Minute).UnixMilli(); lr.ExpiresMs != want {
		t.Fatalf("expiresMs %d, ждали %d", lr.ExpiresMs, want)
	}
}

// Ловит: счётчик попыток считает не так (R19): после неверных вводов должно оставаться
// 4, 3, 2, 1 попыток, а ошибка привязана к полю пароля.
func TestWrongPasswordCountsAttempts(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	for _, left := range []int{4, 3, 2, 1} {
		rep := h.wrongLogin(adminPeer())
		d := deniedOf(t, rep)
		if d.AttemptsLeft != left || d.PauseS != 0 || rep.Field != ipc.FieldPassword {
			t.Fatalf("осталось %d (ждали %d), пауза %d, поле %q", d.AttemptsLeft, left, d.PauseS, rep.Field)
		}
		if want := "Неверный пароль. Осталось попыток: " + itoa(left); rep.Error != want {
			t.Fatalf("текст %q, ждали %q", rep.Error, want)
		}
	}
}

// Ловит: пятая неверная попытка не включает паузу в 60 с, верный пароль в паузу принимается,
// проверка пароля во время паузы всё равно идёт (перебор), пауза не считается по реальным часам.
func TestFiveWrongPasswordsPauseSixtySeconds(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	for i := 0; i < 4; i++ {
		h.wrongLogin(adminPeer())
	}
	rep := h.wrongLogin(adminPeer())
	d := deniedOf(t, rep)
	if d.PauseS != 60 || rep.Error != "Слишком много попыток. Повторить можно через 1:00" {
		t.Fatalf("пауза %d, текст %q", d.PauseS, rep.Error)
	}
	checks := h.hasher.verifys
	h.real.Advance(30 * time.Second)
	rep = h.fail(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: goodPassword})
	if d := deniedOf(t, rep); d.PauseS != 30 {
		t.Fatalf("через 30 с осталось %d с паузы", d.PauseS)
	}
	if h.hasher.verifys != checks {
		t.Fatal("во время паузы пароль всё равно проверялся")
	}
	h.real.Advance(30 * time.Second)
	h.login(adminPeer())
}

// Ловит: вторая серия не удваивает паузу (120 с), верный пароль не сбрасывает счётчик.
func TestSecondSeriesDoublesPauseAndSuccessResets(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	for i := 0; i < 5; i++ {
		h.wrongLogin(adminPeer())
	}
	h.real.Advance(60 * time.Second)
	for i := 0; i < 4; i++ {
		h.wrongLogin(adminPeer())
	}
	if d := deniedOf(t, h.wrongLogin(adminPeer())); d.PauseS != 120 {
		t.Fatalf("вторая пауза %d с, ждали 120", d.PauseS)
	}
	h.real.Advance(120 * time.Second)
	h.login(adminPeer())
	if d := deniedOf(t, h.wrongLogin(adminPeer())); d.AttemptsLeft != 4 {
		t.Fatalf("после верного пароля осталось %d попыток, ждали 4", d.AttemptsLeft)
	}
}

// Ловит: счётчик неверных вводов живёт только в памяти (перезапуск службы сбрасывал бы паузу).
func TestAttemptsSurviveServiceRestart(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	for i := 0; i < 5; i++ {
		h.wrongLogin(adminPeer())
	}
	h2 := newHarness(t, withStore(h.store))
	h2.real.Set(h.real.Now())
	h2.start()
	if d := deniedOf(t, h2.fail(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: goodPassword})); d.PauseS == 0 {
		t.Fatal("после перезапуска службы пауза пропала")
	}
}

// Ловит: неверный ввод не попадает в журнал (R22) или не привязан к ребёнку, из чьего
// сеанса вводили.
func TestWrongPasswordIsJournaledForTheChild(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.wrongLogin(childPeer())
	evs := h.store.eventsOf("password_failed")
	if len(evs) != 1 || evs[0].ChildID != h.childID(childSID) {
		t.Fatalf("события: %+v", evs)
	}
}

// Ловит: пустой пароль считается неверной попыткой (родитель случайно нажал Enter и приблизил паузу).
func TestEmptyPasswordIsNotAnAttempt(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	for i := 0; i < 10; i++ {
		rep := h.fail(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: ""})
		if rep.Error != textEnterPassword || rep.Field != ipc.FieldPassword {
			t.Fatalf("ответ %+v", rep)
		}
	}
	h.login(adminPeer())
}

// Ловит: испорченный хеш пароля превращается в «неверный пароль» и копит попытки, вместо
// понятного сообщения; технический текст попадает человеку.
func TestBrokenHashGivesHumanMessage(t *testing.T) {
	h := newHarness(t, configured())
	h.hasher.broken = true
	h.start()
	rep := h.fail(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: goodPassword})
	if rep.Error != textHashBroken || strings.Contains(rep.Error, "испорчен") {
		t.Fatalf("ответ %q", rep.Error)
	}
	if a, _, _ := h.store.GetAuth(); a.Attempts.FailedInRow() != 0 {
		t.Fatalf("попытки засчитаны: %+v", a.Attempts)
	}
}

// Ловит: вход до настройки пароля пускает кого угодно или падает с техническим текстом.
func TestLoginBeforeSetupAsksToSetUp(t *testing.T) {
	h := newHarness(t)
	h.start()
	rep := h.fail(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: "что-угодно"})
	if rep.Error != textNotConfigured {
		t.Fatalf("ответ %q", rep.Error)
	}
}

// --- сессия родителя (R19) ----------------------------------------------------------------

func (h *harness) state(p ipc.Peer, token string) ipc.SessionStateReply {
	h.t.Helper()
	var st ipc.SessionStateReply
	h.ok(p, token, ipc.CmdSessionState, nil, &st)
	return st
}

// Ловит: сессия не кончается через 10 мин бездействия (R19) или кончается раньше срока.
func TestParentSessionExpiresAfterTenIdleMinutes(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	h.real.Advance(9*time.Minute + 59*time.Second)
	if st := h.state(adminPeer(), tok); !st.Active || st.RemainingS != 1 {
		t.Fatalf("за секунду до конца: %+v", st)
	}
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, nil)
	h.real.Advance(time.Second)
	if st := h.state(adminPeer(), tok); st.Active {
		t.Fatalf("через 10 минут сессия жива: %+v", st)
	}
	rep := h.fail(adminPeer(), tok, ipc.CmdGetToday, nil)
	if rep.Error != textNeedParent && rep.Error != textSessionEnded {
		t.Fatalf("текст %q", rep.Error)
	}
}

// Ловит: чтение (GetToday, SessionState) продлевает сессию: окно, которое обновляет карточки
// раз в несколько секунд, никогда не закончило бы её (R19: срок считается от действий).
func TestReadingDoesNotExtendSession(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	for i := 0; i < 11; i++ {
		h.real.Advance(time.Minute)
		if i < 9 {
			h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, nil)
			h.state(adminPeer(), tok)
		}
	}
	if st := h.state(adminPeer(), tok); st.Active {
		t.Fatalf("после 11 минут чтения сессия жива: %+v", st)
	}
}

// Ловит: ExtendSession и правки не продлевают сессию на 10 мин от действия.
func TestExtendAndEditsExtendSession(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	h.real.Advance(9 * time.Minute)
	var st ipc.SessionStateReply
	h.ok(adminPeer(), tok, ipc.CmdExtendSession, nil, &st)
	if !st.Active || st.RemainingS != 600 {
		t.Fatalf("после продления: %+v", st)
	}
	h.real.Advance(9 * time.Minute)
	h.ok(adminPeer(), tok, ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: h.childID(childSID), Minutes: 15}, nil)
	h.real.Advance(9 * time.Minute)
	if !h.state(adminPeer(), tok).Active {
		t.Fatal("правка не продлила сессию")
	}
}

// Ловит: токен можно использовать из другого процесса (R19: токен привязан к процессу окна);
// ребёнок, подсмотревший токен, получил бы настройки.
func TestTokenWorksOnlyFromItsProcess(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	other := adminPeer()
	other.PID++
	if rep := h.fail(other, tok, ipc.CmdGetToday, nil); rep.Error != textSessionEnded {
		t.Fatalf("другой процесс: %q", rep.Error)
	}
	otherSession := adminPeer()
	otherSession.SessionID = 9
	h.fail(otherSession, tok, ipc.CmdGetToday, nil)
	if st := h.state(other, tok); st.Active {
		t.Fatal("чужой процесс видит сессию живой")
	}
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, nil) // настоящий владелец не пострадал
}

// Ловит: токен переживает смену процесса с тем же номером (PID повторился, момент создания
// процесса другой): новый процесс под тем же PID получил бы чужую сессию.
func TestTokenRejectedWhenPIDIsReusedByAnotherProcess(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	h.launcher.stamps[520] = 111
	tok := h.login(adminPeer())
	h.launcher.stamps[520] = 222
	h.fail(adminPeer(), tok, ipc.CmdGetToday, nil)
}

// Ловит: Logout не заканчивает сессию; повторный Login оставляет прежний токен рабочим.
func TestLogoutAndRelogin(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	h.ok(adminPeer(), tok, ipc.CmdLogout, nil, nil)
	h.fail(adminPeer(), tok, ipc.CmdGetToday, nil)
	a := h.login(adminPeer())
	b := h.login(adminPeer())
	if a == b {
		t.Fatal("токены одинаковые")
	}
	h.fail(adminPeer(), a, ipc.CmdGetToday, nil)
	h.ok(adminPeer(), b, ipc.CmdGetToday, nil, nil)
}

// Ловит: токены угадываются (одинаковые у разных входов) или пустой источник случайных чисел
// выдаёт пустой токен вместо отказа.
func TestEmptyRandomSourceRefusesLogin(t *testing.T) {
	h := newHarness(t, configured(), func(h *harness) { h.cfg.Rand = func() string { return "" } })
	h.start()
	rep := h.fail(adminPeer(), "", ipc.CmdLogin, ipc.LoginRequest{Password: goodPassword})
	if rep.Error != textRetry {
		t.Fatalf("ответ %q", rep.Error)
	}
}

// Ловит: при остановке службы остаются родительские сессии и привязки, а команды после
// Shutdown всё ещё выполняются.
func TestShutdownRefusesFurtherCommands(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	tok := h.login(adminPeer())
	h.svc.Shutdown(h.real.Now())
	rep := h.fail(adminPeer(), tok, ipc.CmdSessionState, nil)
	if rep.Error != textRetry {
		t.Fatalf("ответ %q", rep.Error)
	}
}
