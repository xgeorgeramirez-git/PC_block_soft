package service

import (
	"encoding/json"
	"errors"
	"testing"

	"pcblock/internal/ipc"
)

// fakeCaller — поддельный канал: запоминает запросы, отвечает по списку.
type fakeCaller struct {
	sent    []ipc.Msg
	replies []ipc.Reply
	err     error
	closed  bool
}

func (c *fakeCaller) Do(m ipc.Msg) (ipc.Reply, error) {
	c.sent = append(c.sent, m)
	if c.err != nil {
		return ipc.Reply{}, c.err
	}
	if len(c.replies) == 0 {
		return ipc.OK(ipc.Empty{}), nil
	}
	r := c.replies[0]
	c.replies = c.replies[1:]
	return r, nil
}

func (c *fakeCaller) Close() error { c.closed = true; return nil }

func dialer(c *fakeCaller) (func() (ipc.Caller, error), *int) {
	n := 0
	return func() (ipc.Caller, error) { n++; return c, nil }, &n
}

// Ловит: токен родительской сессии остаётся в JS-окне вместо Go или не подставляется в
// последующие команды; Logout не забывает токен.
func TestWindowClientTokenLifecycle(t *testing.T) {
	c := &fakeCaller{replies: []ipc.Reply{
		ipc.OK(ipc.LoginReply{Token: "tok-1", ExpiresMs: 5, IdleTimeoutS: 600}),
		ipc.OK(ipc.Empty{}),
		ipc.OK(ipc.Empty{}),
		ipc.OK(ipc.Empty{}),
	}}
	dial, _ := dialer(c)
	w := NewWindowClient(dial, "")
	for _, cmd := range []string{ipc.CmdLogin, ipc.CmdGetToday, ipc.CmdLogout, ipc.CmdGetToday} {
		if _, err := w.Call(cmd, `{}`); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
	}
	tokens := []string{c.sent[0].Token, c.sent[1].Token, c.sent[2].Token, c.sent[3].Token}
	want := []string{"", "tok-1", "tok-1", ""}
	for i := range want {
		if tokens[i] != want[i] {
			t.Errorf("запрос %d (%s): токен %q, ждали %q", i, c.sent[i].Cmd, tokens[i], want[i])
		}
	}
}

// Ловит: окно, открытое после ввода пароля в страже, не получает переданную сессию (Ruling 19, --handover).
func TestWindowClientHandoverToken(t *testing.T) {
	c := &fakeCaller{}
	dial, _ := dialer(c)
	w := NewWindowClient(dial, "from-guard")
	if _, err := w.Call(ipc.CmdGetToday, ""); err != nil {
		t.Fatal(err)
	}
	if c.sent[0].Token != "from-guard" {
		t.Fatalf("токен %q", c.sent[0].Token)
	}
}

// Ловит: недоступная служба показывается техническим текстом или ошибкой Go вместо
// «Программа не отвечает…» (L-112), а оборванный канал не открывается заново.
func TestWindowClientLinkErrors(t *testing.T) {
	c := &fakeCaller{err: errors.New("pipe: broken")}
	dial, dials := dialer(c)
	w := NewWindowClient(dial, "")
	_, err := w.Call(ipc.CmdHello, "")
	if err == nil || err.Error() != TextNoLink {
		t.Fatalf("err = %v, ждали %q", err, TextNoLink)
	}
	if !c.closed {
		t.Error("оборванный канал надо закрыть")
	}
	c.err, c.closed = nil, false
	if _, err := w.Call(ipc.CmdHello, ""); err != nil || *dials != 2 {
		t.Fatalf("после обрыва: err=%v, открытий канала %d (ждали 2)", err, *dials)
	}

	w2 := NewWindowClient(func() (ipc.Caller, error) { return nil, errors.New("нет канала") }, "")
	if _, err := w2.Call(ipc.CmdHello, ""); err == nil || err.Error() != TextNoLink {
		t.Fatalf("канал не открылся: %v", err)
	}
}

// Ловит: мусор вместо JSON уходит в службу, пустая команда не отвергается.
func TestWindowClientRejectsBadRequest(t *testing.T) {
	c := &fakeCaller{}
	dial, _ := dialer(c)
	w := NewWindowClient(dial, "")
	for _, tc := range [][2]string{{"", `{}`}, {ipc.CmdHello, `{не json`}} {
		if _, err := w.Call(tc[0], tc[1]); err == nil || err.Error() != ipc.TextBadRequest {
			t.Errorf("%q %q: err=%v", tc[0], tc[1], err)
		}
	}
	if len(c.sent) != 0 {
		t.Errorf("в службу ушло %d запросов", len(c.sent))
	}
}

// Ловит: ответ искажается по пути в окно: экранируется «<», теряется поле или нагрузка.
func TestWindowClientReplyJSONAsIs(t *testing.T) {
	c := &fakeCaller{replies: []ipc.Reply{ipc.FailWith("Осталось <3> попытки", ipc.FieldPassword, ipc.LoginDenied{AttemptsLeft: 3})}}
	dial, _ := dialer(c)
	w := NewWindowClient(dial, "")
	raw, err := w.Call(ipc.CmdLogin, `{"password":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	var got ipc.Reply
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if got.OK || got.Error != "Осталось <3> попытки" || got.Field != "password" || string(got.Payload) != `{"attemptsLeft":3,"pauseS":0}` {
		t.Fatalf("ответ %+v", got)
	}
	if string(c.sent[0].Payload) != `{"password":"x"}` {
		t.Errorf("нагрузка запроса %q", c.sent[0].Payload)
	}
}

// Ловит: после окончания сессии (SessionState: active=false) окно продолжает слать старый токен.
func TestWindowClientDropsTokenWhenSessionEnded(t *testing.T) {
	c := &fakeCaller{replies: []ipc.Reply{
		ipc.OK(ipc.SessionStateReply{Active: false}),
		ipc.OK(ipc.Empty{}),
	}}
	dial, _ := dialer(c)
	w := NewWindowClient(dial, "tok")
	w.Call(ipc.CmdSessionState, "")
	w.Call(ipc.CmdGetToday, "")
	if c.sent[1].Token != "" {
		t.Fatalf("токен %q после окончания сессии", c.sent[1].Token)
	}
}

// Ловит: Close не закрывает канал и не отпускает токен.
func TestWindowClientClose(t *testing.T) {
	c := &fakeCaller{}
	dial, _ := dialer(c)
	w := NewWindowClient(dial, "tok")
	w.Call(ipc.CmdHello, "")
	w.Close()
	if !c.closed {
		t.Error("канал не закрыт")
	}
	w.Close() // повторный вызов безопасен
}
