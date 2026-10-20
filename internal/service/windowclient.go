package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"pcblock/internal/ipc"
)

// TextNoLink — что видит человек, когда службы нет или канал оборвался (L-112, Ruling 14).
const TextNoLink = "Программа не отвечает. Попробуйте ещё раз через минуту"

// WindowClient — клиентская сторона моста окна (internal/ui, Ruling 14): окно Wails зовёт
// Call(команда, JSON запроса) и получает JSON ответа канала связи как есть. Токен родительской
// сессии (R19) живёт здесь, в Go, а не в JavaScript: он берётся из ответа Login и подставляется
// в следующие команды, пока не придёт Logout или признак окончания сессии. Канал открывается
// при первом вызове и заново после обрыва.
type WindowClient struct {
	dial func() (ipc.Caller, error)

	mu     sync.Mutex
	caller ipc.Caller
	token  string
}

// NewWindowClient создаёт клиента. dial открывает канал (на Windows: ipc.Dial); handover —
// токен сессии, переданный окну при запуске (--handover=…, Ruling 19), или пусто.
func NewWindowClient(dial func() (ipc.Caller, error), handover string) *WindowClient {
	return &WindowClient{dial: dial, token: handover}
}

// Call отправляет команду службе. Ошибка Go возвращается только когда ответа нет вообще
// (канал не открылся, оборвался): её текст готов для человека. Отказ службы приходит в
// ответе, как в канале.
func (w *WindowClient) Call(command, payload string) (string, error) {
	if command == "" {
		return "", errors.New(ipc.TextBadRequest)
	}
	msg := ipc.Msg{Cmd: command}
	if payload = string(bytes.TrimSpace([]byte(payload))); payload != "" {
		if !json.Valid([]byte(payload)) {
			return "", errors.New(ipc.TextBadRequest)
		}
		msg.Payload = json.RawMessage(payload)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.caller == nil {
		c, err := w.dial()
		if err != nil || c == nil {
			return "", errors.New(TextNoLink)
		}
		w.caller = c
	}
	msg.Token = w.token
	rep, err := w.caller.Do(msg)
	if err != nil {
		w.dropLocked()
		return "", errors.New(TextNoLink)
	}
	w.trackToken(command, rep)

	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rep); err != nil {
		return "", errors.New(ipc.TextInternal)
	}
	return string(bytes.TrimRight(out.Bytes(), "\n")), nil
}

// trackToken запоминает токен из ответа Login и забывает его, когда сессия закончилась.
func (w *WindowClient) trackToken(command string, rep ipc.Reply) {
	if !rep.OK {
		return
	}
	switch command {
	case ipc.CmdLogin:
		var lr ipc.LoginReply
		if rep.Into(&lr) == nil && lr.Token != "" {
			w.token = lr.Token
		}
	case ipc.CmdLogout:
		w.token = ""
	case ipc.CmdSessionState, ipc.CmdExtendSession:
		var st ipc.SessionStateReply
		if rep.Into(&st) == nil && !st.Active {
			w.token = ""
		}
	}
}

func (w *WindowClient) dropLocked() {
	if c, ok := w.caller.(io.Closer); ok {
		_ = c.Close()
	}
	w.caller = nil
}

// Close закрывает канал; повторный вызов безопасен.
func (w *WindowClient) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.dropLocked()
}
