package ipc

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strconv"
)

// Кадр: 4 байта длины тела (little-endian, без знака) и тело в JSON. Длина 0 или больше
// MaxFrame — ошибка. Запрос — Msg, ответ — Reply.
const (
	headerLen = 4
	// MaxFrame — наибольшая длина тела кадра: 1 МБ.
	MaxFrame = 1 << 20
)

// Имена, под которыми программа работает в Windows.
const (
	// PipeName — полное имя канала службы.
	PipeName = `\\.\pipe\PCBlock`
	// ServiceName — имя службы Windows.
	ServiceName = "PCBlock"
)

// Msg — запрос к службе.
type Msg struct {
	Cmd     string          `json:"cmd"`
	Token   string          `json:"token,omitempty"` // токен родительской сессии (R19)
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Reply — ответ службы. Error — готовый текст для человека (L-112); Field — поле формы,
// к которому он относится (L-121). У отказа Payload может нести подробности (LoginDenied).
type Reply struct {
	OK      bool            `json:"ok"`
	Error   string          `json:"error,omitempty"`
	Field   string          `json:"field,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Ошибки протокола. Это ошибки для журнала: человеку их не показывают (L-112).
var (
	ErrEmptyFrame    = errors.New("ipc: кадр нулевой длины")
	ErrFrameTooLarge = errors.New("ipc: кадр больше 1 МБ")
	ErrTruncated     = errors.New("ipc: кадр оборван")
	ErrTrailing      = errors.New("ipc: после тела кадра лишние байты")
	ErrBadMessage    = errors.New("ipc: сообщение не разобрано")
)

// Ошибки клиента. Текст — для человека; технические подробности лежат в Error.Detail.
var (
	// ErrNotService — по ту сторону канала не служба программы (клиент проверил владельца
	// канала до отправки данных).
	ErrNotService = errors.New("Программа не отвечает")
	// ErrUnavailable — канал не открылся или оборвался.
	ErrUnavailable = errors.New("Программа не отвечает")
	// ErrTimeout — служба не ответила вовремя.
	ErrTimeout = errors.New("Программа не отвечает")
	// ErrClosed — клиент уже закрыт.
	ErrClosed = errors.New("Программа не отвечает")
)

// Error — ошибка клиента канала. Error() даёт только текст для человека без технических
// деталей (L-112); причина для журнала — Detail() и Unwrap().
type Error struct {
	Kind  error // ErrNotService, ErrUnavailable, ErrTimeout или ErrClosed
	Cause error // что произошло на самом деле, может быть nil
}

func newError(kind, cause error) *Error { return &Error{Kind: kind, Cause: cause} }

// Error возвращает текст для человека.
func (e *Error) Error() string { return e.Kind.Error() }

// Unwrap позволяет errors.Is найти и вид, и причину.
func (e *Error) Unwrap() []error {
	if e.Cause == nil {
		return []error{e.Kind}
	}
	return []error{e.Kind, e.Cause}
}

// Detail возвращает техническую подробность для журнала работы.
func (e *Error) Detail() string {
	if e.Cause == nil {
		return e.Kind.Error()
	}
	return e.Cause.Error()
}

// Тексты, которые транспорт отправляет сам, когда служба до обработчика не дошла.
const (
	TextBadRequest = "Не удалось разобрать запрос. Попробуйте ещё раз"
	TextNoIdentity = "Не удалось проверить, кто обращается к программе"
	TextInternal   = "Что-то пошло не так. Попробуйте ещё раз"
)

// Encode возвращает кадр запроса. Испорченная полезная нагрузка (не JSON) отбрасывается:
// кадр всегда разбирается. Размер здесь не проверяется: WriteMsg вернёт ErrFrameTooLarge.
func Encode(m Msg) []byte {
	if len(m.Payload) > 0 && !json.Valid(m.Payload) {
		m.Payload = nil
	}
	return frameOf(m)
}

// EncodeReply возвращает кадр ответа; нагрузка проверяется так же, как в Encode.
func EncodeReply(r Reply) []byte {
	if len(r.Payload) > 0 && !json.Valid(r.Payload) {
		r.Payload = nil
	}
	return frameOf(r)
}

// frameOf пишет значение в JSON без экранирования HTML и ставит перед ним длину.
// Msg и Reply состоят из строк, булева и проверенного RawMessage: Marshal не падает.
func frameOf(v any) []byte {
	var buf bytes.Buffer
	buf.Write(make([]byte, headerLen))
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil
	}
	buf.Truncate(buf.Len() - 1) // Encoder дописывает перевод строки
	out := buf.Bytes()
	binary.LittleEndian.PutUint32(out, uint32(len(out)-headerLen))
	return out
}

// Decode разбирает кадр запроса целиком (длина и тело). Никогда не паникует.
func Decode(frame []byte) (Msg, error) {
	body, err := splitFrame(frame)
	if err != nil {
		return Msg{}, err
	}
	return decodeMsg(body)
}

// DecodeReply разбирает кадр ответа целиком.
func DecodeReply(frame []byte) (Reply, error) {
	body, err := splitFrame(frame)
	if err != nil {
		return Reply{}, err
	}
	return decodeReply(body)
}

func decodeMsg(body []byte) (Msg, error) {
	var m Msg
	if err := json.Unmarshal(body, &m); err != nil {
		return Msg{}, errors.Join(ErrBadMessage, err)
	}
	if m.Cmd == "" {
		return Msg{}, errors.Join(ErrBadMessage, errors.New("нет команды"))
	}
	m.Payload = trimNull(m.Payload)
	return m, nil
}

func decodeReply(body []byte) (Reply, error) {
	var r Reply
	if err := json.Unmarshal(body, &r); err != nil {
		return Reply{}, errors.Join(ErrBadMessage, err)
	}
	r.Payload = trimNull(r.Payload)
	return r, nil
}

// trimNull превращает JSON null в пустую нагрузку.
func trimNull(p json.RawMessage) json.RawMessage {
	if string(p) == "null" {
		return nil
	}
	return p
}

// splitFrame проверяет заголовок и возвращает тело кадра, лежащего в памяти целиком.
func splitFrame(frame []byte) ([]byte, error) {
	if len(frame) < headerLen {
		return nil, ErrTruncated
	}
	n, err := checkLength(binary.LittleEndian.Uint32(frame))
	if err != nil {
		return nil, err
	}
	body := frame[headerLen:]
	switch {
	case len(body) < n:
		return nil, ErrTruncated
	case len(body) > n:
		return nil, ErrTrailing
	}
	return body, nil
}

func checkLength(n uint32) (int, error) {
	switch {
	case n == 0:
		return 0, ErrEmptyFrame
	case n > MaxFrame:
		return 0, ErrFrameTooLarge
	}
	return int(n), nil
}

// ReadHeader читает 4 байта длины и проверяет её. Нужна, когда на заголовок и на тело
// действуют разные сроки ожидания; обычно хватает ReadFrame. Чистый конец потока до
// первого байта — io.EOF, обрыв внутри заголовка — io.ErrUnexpectedEOF.
func ReadHeader(r io.Reader) (int, error) {
	var h [headerLen]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return 0, err
	}
	return checkLength(binary.LittleEndian.Uint32(h[:]))
}

// ReadBody читает тело кадра длиной n, полученной из ReadHeader. Обрыв — io.ErrUnexpectedEOF.
func ReadBody(r io.Reader, n int) ([]byte, error) {
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return body, nil
}

// ReadFrame читает один кадр из потока и возвращает его тело. Память под тело выделяется
// только после проверки длины, поэтому огромная длина в заголовке ничего не стоит.
func ReadFrame(r io.Reader) ([]byte, error) {
	n, err := ReadHeader(r)
	if err != nil {
		return nil, err
	}
	return ReadBody(r, n)
}

// WriteFrame пишет тело кадра с заголовком одной записью.
func WriteFrame(w io.Writer, body []byte) error {
	switch {
	case len(body) == 0:
		return ErrEmptyFrame
	case len(body) > MaxFrame:
		return ErrFrameTooLarge
	}
	frame := make([]byte, headerLen+len(body))
	binary.LittleEndian.PutUint32(frame, uint32(len(body)))
	copy(frame[headerLen:], body)
	_, err := w.Write(frame)
	return err
}

// ReadMsg читает и разбирает один запрос.
func ReadMsg(r io.Reader) (Msg, error) {
	body, err := ReadFrame(r)
	if err != nil {
		return Msg{}, err
	}
	return decodeMsg(body)
}

// ReadReply читает и разбирает один ответ.
func ReadReply(r io.Reader) (Reply, error) {
	body, err := ReadFrame(r)
	if err != nil {
		return Reply{}, err
	}
	return decodeReply(body)
}

// WriteMsg пишет запрос; больше MaxFrame — ErrFrameTooLarge, ничего не отправлено.
func WriteMsg(w io.Writer, m Msg) error { return writeEncoded(w, Encode(m)) }

// WriteReply пишет ответ; больше MaxFrame — ErrFrameTooLarge, ничего не отправлено.
func WriteReply(w io.Writer, r Reply) error { return writeEncoded(w, EncodeReply(r)) }

func writeEncoded(w io.Writer, frame []byte) error {
	if len(frame) < headerLen {
		return ErrBadMessage
	}
	if len(frame)-headerLen > MaxFrame {
		return ErrFrameTooLarge
	}
	_, err := w.Write(frame)
	return err
}

func itoa(n int) string { return strconv.Itoa(n) }
