package ipc

import (
	"encoding/json"
	"reflect"
)

// Payload записывает значение в JSON для Msg.Payload и Reply.Payload. nil-срезы на любой
// глубине уходят как «[]», а не «null»: во фронт null вместо массива приходить не должен
// (L-22). Значение, которое нельзя записать, даёт nil.
func Payload(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(emptySlices(reflect.ValueOf(v)).Interface())
	if err != nil {
		return nil
	}
	return b
}

// OK собирает успешный ответ с нагрузкой v. Нагрузка, которую нельзя записать, — ошибка
// службы с человеческим текстом, не пустой успех.
func OK(v any) Reply {
	if v == nil {
		return Reply{OK: true}
	}
	p := Payload(v)
	if p == nil {
		return Fail(TextInternal)
	}
	return Reply{OK: true, Payload: p}
}

// Fail собирает отказ с текстом для человека (L-112).
func Fail(text string) Reply { return Reply{Error: text} }

// FailField собирает отказ, привязанный к полю формы (L-121).
func FailField(text, field string) Reply { return Reply{Error: text, Field: field} }

// FailWith собирает отказ с подробностями в нагрузке, например LoginDenied.
func FailWith(text, field string, details any) Reply {
	return Reply{Error: text, Field: field, Payload: Payload(details)}
}

// Into разбирает нагрузку запроса в v. Пустая нагрузка оставляет v нулевым; лишние поля
// игнорируются.
func (m Msg) Into(v any) error { return intoPayload(m.Payload, v) }

// ReplyError — отказ службы как ошибка Go: Error() даёт текст для человека.
type ReplyError struct {
	Text    string
	Field   string
	Payload json.RawMessage
}

// Error возвращает текст для человека.
func (e *ReplyError) Error() string { return e.Text }

// Into разбирает нагрузку успешного ответа в v (nil — нагрузка не нужна). Отказ службы
// возвращается как *ReplyError.
func (r Reply) Into(v any) error {
	if !r.OK {
		return &ReplyError{Text: r.Error, Field: r.Field, Payload: r.Payload}
	}
	if v == nil {
		return nil
	}
	return intoPayload(r.Payload, v)
}

func intoPayload(p json.RawMessage, v any) error {
	if len(p) == 0 {
		return nil
	}
	return json.Unmarshal(p, v)
}

// Caller — всё, что умеет отправить запрос и получить ответ: *Client и поддельный клиент в тестах.
type Caller interface {
	Do(Msg) (Reply, error)
}

// Call отправляет команду с токеном и запросом req (nil — без нагрузки) и разбирает
// успешный ответ в resp (nil — нагрузка не нужна). Отказ службы возвращается как *ReplyError.
func Call(c Caller, cmd, token string, req, resp any) error {
	m := Msg{Cmd: cmd, Token: token}
	if req != nil {
		m.Payload = Payload(req)
		if m.Payload == nil {
			return newError(ErrUnavailable, ErrBadMessage)
		}
	}
	rep, err := c.Do(m)
	if err != nil {
		return err
	}
	return rep.Into(resp)
}

var rawMessageType = reflect.TypeOf(json.RawMessage(nil))

// emptySlices возвращает копию значения, в которой nil-срезы заменены пустыми. Исходное
// значение не меняется. Строки, числа, RawMessage и байтовые срезы остаются как есть.
func emptySlices(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		cp := reflect.New(v.Type().Elem())
		cp.Elem().Set(emptySlices(v.Elem()))
		return cp
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		return emptySlices(v.Elem())
	case reflect.Struct:
		cp := reflect.New(v.Type()).Elem()
		cp.Set(v)
		for i := 0; i < cp.NumField(); i++ {
			if f := cp.Field(i); f.CanSet() {
				f.Set(emptySlices(v.Field(i)))
			}
		}
		return cp
	case reflect.Slice:
		if v.Type() == rawMessageType || v.Type().Elem().Kind() == reflect.Uint8 {
			return v
		}
		if v.IsNil() {
			return reflect.MakeSlice(v.Type(), 0, 0)
		}
		cp := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			cp.Index(i).Set(emptySlices(v.Index(i)))
		}
		return cp
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		cp := reflect.MakeMapWithSize(v.Type(), v.Len())
		it := v.MapRange()
		for it.Next() {
			cp.SetMapIndex(it.Key(), emptySlices(it.Value()))
		}
		return cp
	}
	return v
}
