package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// Ловит (ТЗ 8.2.4, 8.2.5, D-111, D-114): у плашки «Игрового лобби» не тот заголовок, текст не
// дословно ТЗ или какой-то вид остался без заголовка (страж нарисует его прежней плашкой).
func TestPlateTitles(t *testing.T) {
	titles := []struct {
		kind    string
		allowed bool
		want    string
	}{
		{ipc.BlockedUnknown, false, "Ждёт разрешения"},
		{ipc.BlockedForbidden, false, "Эта программа запрещена"},
		{ipc.BlockedSite, false, "Сайт закрыт"},
		{ipc.BlockedAdult, false, "Сайт закрыт"},
		{ipc.BlockedVideoOver, false, "Видео на сегодня всё"},
		{ipc.BlockedBrowser, false, "Этот браузер запрещён"},
		{ipc.BlockedGamesOver, false, "Игры на сегодня всё"},
		{ipc.BlockedStudyOnly, false, "Сейчас только учёба"},
		{ipc.BlockedDecided, true, "Можно играть!"},
		{ipc.BlockedDecided, false, "Родитель запретил"},
	}
	for _, c := range titles {
		if got := plateTitle(c.kind, c.allowed); got != c.want {
			t.Errorf("%s (разрешил %v): заголовок %q, ждали %q", c.kind, c.allowed, got, c.want)
		}
	}
	if got := plateTitle("нет такого", false); got != "" {
		t.Errorf("неизвестный вид: заголовок %q, ждали пусто (прежняя плашка)", got)
	}
	texts := []struct{ got, want string }{
		{fmt.Sprintf(textPlateUnknown, "SuperGame"), "Программа «SuperGame» новая. Родитель должен решить, можно ли её запускать"},
		{fmt.Sprintf(textPlateForbidden, "Блокнот-копия"), "Родитель запретил «Блокнот-копия». Выбери другую игру"},
		{fmt.Sprintf(textPlateSite, "example.com"), "example.com в списке запрещённых. Если он нужен, попроси родителей"},
		{textPlateAdult, "Этот сайт для взрослых"},
		{textPlateBrowser, "Пользуйся Brave"},
	}
	for _, c := range texts {
		if c.got != c.want {
			t.Errorf("текст %q, ждали %q", c.got, c.want)
		}
	}
}

// Ловит (8.2.4 п. 4): плашка решения без признака «разрешил» (страж покажет не тот стикер и звук),
// текст не дословно ТЗ, кавычки не «ёлочки».
func TestDecidedPlateAllowedFlag(t *testing.T) {
	for _, c := range []struct {
		cat, text string
		allowed   bool
	}{
		{"games", "Родитель разрешил «SuperGame»: это игра. Запусти её снова", true},
		{"study", "Родитель разрешил «SuperGame»: это учёба. Запусти её снова", true},
		{"other", "Родитель разрешил «SuperGame»: это обычная программа. Запусти её снова", true},
		{"blocked", "«SuperGame» запускать нельзя", false},
	} {
		h := newProgHarness(t)
		h.plates(childSession)
		h.decide(pathSuperGame, c.cat)
		got := h.plates(childSession)
		title := "Можно играть!"
		if !c.allowed {
			title = "Родитель запретил"
		}
		want := ipc.BlockedPlate{Kind: ipc.BlockedDecided, Title: title, Text: c.text, ShowS: 10, Path: pathSuperGame, Allowed: c.allowed}
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s: плашки %+v, ждали %+v", c.cat, got, want)
		}
		if len(got) == 1 && strings.ContainsAny(got[0].Text, "“”\"") {
			t.Errorf("%s: кавычки не «»: %q", c.cat, got[0].Text)
		}
	}
}

// askWindowPeer — окно родителя, открытое «Попросить сейчас» в сеансе ребёнка session: работает от
// имени ребёнка, пароль родителя вводится в нём.
func askWindowPeer(session uint32) ipc.Peer {
	return ipc.Peer{PID: 700 + session, SessionID: session, SID: childSID}
}

// decidedOf — плашки решения из очереди стража сеанса.
func decidedOf(plates []ipc.BlockedPlate) []ipc.BlockedPlate {
	out := []ipc.BlockedPlate{}
	for _, p := range plates {
		if p.Kind == ipc.BlockedDecided {
			out = append(out, p)
		}
	}
	return out
}

// Ловит (8.2.4 п. 2, S65, D-114): после «Попросить сейчас» родитель решил при ребёнке в окне его
// сеанса, а ребёнку всё равно плашка решения; или плашки нет, когда решили с телефона, из окна
// другого сеанса или в том же сеансе, но через 11 мин (просьба забыта через 10 мин).
func TestAskNowSkipsPlateInThatSession(t *testing.T) {
	ask := func(h *progHarness) {
		h.t.Helper()
		var ow ipc.OpenWindowReply
		h.ok(guardPeerOf(childSession), "", ipc.CmdOpenWindow,
			ipc.OpenWindowRequest{Kind: ipc.WindowParent, Screen: ipc.ScreenPrograms, Path: `C:\Users\Миша\Downloads\SuperGame.exe`}, &ow)
	}
	decideIn := func(h *progHarness, p ipc.Peer) {
		h.t.Helper()
		tok := h.login(p)
		h.ok(p, tok, ipc.CmdDecideProgram, ipc.DecideProgramRequest{Path: pathSuperGame, Category: "games"}, nil)
	}

	// Решение окном того же сеанса: плашки нет.
	h := newProgHarness(t)
	h.plates(childSession)
	ask(h)
	decideIn(h, askWindowPeer(childSession))
	if got := decidedOf(h.plates(childSession)); len(got) != 0 {
		t.Fatalf("решение при ребёнке: плашка %+v", got)
	}

	// Решение с телефона после просьбы: плашка есть.
	h = newProgHarness(t)
	h.plates(childSession)
	ask(h)
	h.svc.mu.Lock()
	_, _, _, fail := h.svc.decideProgram(pathSuperGame, "games", extraFromPhone, 0, h.real.Now())
	h.svc.mu.Unlock()
	if fail != "" {
		t.Fatalf("решение с телефона: %q", fail)
	}
	if got := decidedOf(h.plates(childSession)); len(got) != 1 || !got[0].Allowed {
		t.Fatalf("решение с телефона: плашки %+v", got)
	}

	// Решение из окна родителя в его учётке (сеанс 1): плашка есть.
	h = newProgHarness(t)
	h.plates(childSession)
	ask(h)
	decideIn(h, adminPeer())
	if got := decidedOf(h.plates(childSession)); len(got) != 1 {
		t.Fatalf("решение из окна другого сеанса: плашки %+v", got)
	}

	// Решение окном того же сеанса через 11 мин: просьба забыта, плашка есть.
	h = newProgHarness(t)
	h.plates(childSession)
	ask(h)
	h.real.Advance(11 * time.Minute)
	decideIn(h, askWindowPeer(childSession))
	if got := decidedOf(h.plates(childSession)); len(got) != 1 {
		t.Fatalf("решение через 11 мин: плашки %+v", got)
	}

	// Просьба о другой программе не прячет плашку этой.
	h = newProgHarness(t)
	h.plates(childSession)
	var ow ipc.OpenWindowReply
	h.ok(guardPeerOf(childSession), "", ipc.CmdOpenWindow,
		ipc.OpenWindowRequest{Kind: ipc.WindowParent, Screen: ipc.ScreenPrograms, Path: pathHomework}, &ow)
	decideIn(h, askWindowPeer(childSession))
	if got := decidedOf(h.plates(childSession)); len(got) != 1 {
		t.Fatalf("просьба о другой программе: плашки %+v", got)
	}
}
