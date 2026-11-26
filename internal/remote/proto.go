package remote

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
)

// Виды сообщений конверта (протокол, разделы 3–5).
const (
	KindPair1  = "pair1"
	KindPair1R = "pair1r"
	KindPair2  = "pair2"
	KindPair2R = "pair2r"
	KindCmd    = "cmd"
	KindCmdR   = "cmdr"
	KindNote   = "note"  // сообщение ребёнку (раздел 5а)
	KindNoteR  = "noter" // ответ на сообщение
)

// Типы подписанных данных (поле "t", протокол, раздел 2).
const (
	TypeCmd    = "cmd"
	TypeCmdR   = "cmdr"
	TypeReport = "report"
	TypeNote   = "note"
	TypeNoteR  = "noter"
)

// Состояния сообщения в отчёте (NoteReport.State, раздел 5а).
const (
	NoteQueued  = "queued"  // ждёт ребёнка на экране
	NoteShown   = "shown"   // плашка на экране ребёнка («Доставлено»)
	NoteRead    = "read"    // нажал «Понял» или вышло время показа
	NoteExpired = "expired" // 10 минут ждало ребёнка и погасло
)

// Действия команды (поле "op", раздел 5б). Пустое — доп. время (телефон 0.8.0/0.8.1).
const (
	OpExtra       = "extra"       // доп. время на delta минут
	OpNoForbidden = "noForbidden" // снять запретные часы на сегодня (FR-42), delta нет
)

// Запретные часы ребёнка сегодня в отчёте (ChildReport.Forbid, раздел 6).
const (
	ForbidNone    = "none"    // сегодня запретных часов нет
	ForbidAhead   = "ahead"   // будут сегодня
	ForbidActive  = "active"  // идут сейчас (ForbidUntilMs — конец интервала)
	ForbidCleared = "cleared" // сняты на сегодня
)

// Пределы сообщения (раздел 5а).
const (
	MaxNoteRunes = 300 // знаков Unicode после обрезки пробелов по краям
	MaxNoteLines = 5   // переводов строк
	maxNoteIDLen = 64
)

// Коды отказа команды (поле error ответа cmdr, протокол, раздел 5).
const (
	ErrCodeBadSig    = "bad_sig"
	ErrCodeReplay    = "replay"
	ErrCodeStale     = "stale"
	ErrCodeNoChild   = "no_child"
	ErrCodeRange     = "range"
	ErrCodeNotPaired = "not_paired"
)

const (
	// ProtoVersion — версия данных команды, ответа и отчёта (поле "v").
	ProtoVersion = 1
	// MaxSkew — допустимое расхождение ts команды с часами компьютера.
	MaxSkew = 5 * time.Minute
	// NonceLen — длина nPc и nPhone.
	NonceLen     = 16
	maxNameRunes = 40
)

// ErrBadMsg — сообщение не разобрано (неизвестный вид, неверное тело).
var ErrBadMsg = errors.New("remote: неверное сообщение")

// Msg — конверт сервера: {"id","from","kind","body"}. Тело сервер не понимает.
type Msg struct {
	ID   string          `json:"id"`
	From string          `json:"from"`
	Kind string          `json:"kind"`
	Body json.RawMessage `json:"body"`
}

// Signed — подписанный блок {"data","sig"}: base64url байтов JSON и base64url подписи DER.
type Signed struct {
	Data string `json:"data"`
	Sig  string `json:"sig"`
}

// Pair1 — первое сообщение телефона при привязке.
type Pair1 struct {
	PhonePub string `json:"phonePub"`
	Name     string `json:"name"`

	key *ecdsa.PublicKey
	der []byte
}

// Pair1R — ответ компьютера: его ключ и обязательство.
type Pair1R struct {
	PCPub  string `json:"pcPub"`
	Commit string `json:"commit"`
}

// Pair2 — случайное число телефона.
type Pair2 struct {
	NPhone string `json:"nPhone"`

	nonce []byte
}

// Pair2R — раскрытие числа компьютера.
type Pair2R struct {
	NPc string `json:"nPc"`
}

// Command — данные команды телефона (data в Signed). Op пусто или OpExtra — доп. время на Delta
// минут (раздел 5); OpNoForbidden — снять запрет на сегодня, Delta нет (раздел 5б). Пустые поля в
// JSON не пишутся: байты команды без op — те же, что у телефона 0.8.0.
type Command struct {
	V     int    `json:"v"`
	T     string `json:"t"`
	PC    string `json:"pc"`
	Phone string `json:"phone"`
	Seq   int64  `json:"seq"`
	TS    int64  `json:"ts"`
	Child int64  `json:"child"`
	Delta int    `json:"delta,omitempty"`
	Op    string `json:"op,omitempty"`
}

// CmdResult — данные ответа cmdr. LastSeq — только в отказе.
type CmdResult struct {
	V       int     `json:"v"`
	T       string  `json:"t"`
	PC      string  `json:"pc"`
	Phone   string  `json:"phone"`
	Seq     int64   `json:"seq"`
	Cmd     string  `json:"cmd"`
	TS      int64   `json:"ts"`
	OK      bool    `json:"ok"`
	Message string  `json:"message,omitempty"`
	Error   string  `json:"error,omitempty"`
	LastSeq *int64  `json:"lastSeq,omitempty"`
	Report  *Report `json:"report,omitempty"`
}

// Report — отчёт компьютера телефону (протокол, раздел 6).
type Report struct {
	V        int           `json:"v"`
	T        string        `json:"t"`
	PC       string        `json:"pc"`
	Phone    string        `json:"phone"`
	TS       int64         `json:"ts"`
	Children []ChildReport `json:"children"`
	// Notes — последние сообщения не старше суток (раздел 5а); пусто — поля нет.
	Notes []NoteReport `json:"notes,omitempty"`
}

// ChildReport — строка ребёнка в отчёте; LeftSec — то же «осталось», что в окне родителя; -1 — сегодня без лимита.
type ChildReport struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	UsedSec  int64  `json:"usedSec"`
	LeftSec  int64  `json:"leftSec"`
	ExtraMin int    `json:"extraMin"`
	Blocked  bool   `json:"blocked"`
	// Forbid — запретные часы сегодня (Forbid*, раздел 6); пусто — компьютер старой версии.
	Forbid string `json:"forbid,omitempty"`
	// ForbidUntilMs — конец идущего запретного интервала при ForbidActive (часы компьютера).
	ForbidUntilMs int64 `json:"forbidUntilMs,omitempty"`
	// Today — «Статистика за сегодня» (FR-43, раздел 6): только в отчёте pc/report, в отчёте внутри
	// cmdr/noter поля нет; нет и у компьютера старой версии.
	Today *TodayReport `json:"today,omitempty"`
}

// TodayReport — «Итоги» за сегодня окна родителя для телефона (FR-43, раздел 6). Моменты — UTC мс
// по часам компьютера; FirstMs 0 — сегодня ребёнок за компьютер не садился. Списки — по убыванию
// времени, пустые — [] (не null); More — сколько строк не вошло в списки (урезание, раздел 6).
type TodayReport struct {
	FirstMs    int64      `json:"firstMs"`
	NowAt      bool       `json:"nowAt"`
	SinceMs    int64      `json:"sinceMs"`
	UsedSec    int64      `json:"usedSec"`
	LeftSec    int64      `json:"leftSec"`
	GamesSec   int64      `json:"gamesSec"`
	StudySec   int64      `json:"studySec"`
	VideoSec   int64      `json:"videoSec"`
	Programs   []TimeItem `json:"programs"`
	Sites      []TimeItem `json:"sites"`
	More       TodayMore  `json:"more"`
	Violations int        `json:"violations"`
	Bypass     int        `json:"bypass"`
}

// TimeItem — строка списка программ или сайтов за сегодня: название (≤ MaxItemNameRunes знаков),
// категория вкладки «Итогов» (Item*: game, study, video, other) и секунды.
type TimeItem struct {
	Name string `json:"name"`
	Cat  string `json:"cat"`
	Sec  int64  `json:"sec"`
}

// TodayMore — сколько программ и сайтов не вошло в списки отчёта.
type TodayMore struct {
	Programs int `json:"programs"`
	Sites    int `json:"sites"`
}

// Note — данные сообщения телефона ребёнку (data в Signed, раздел 5а).
type Note struct {
	V       int    `json:"v"`
	T       string `json:"t"`
	PC      string `json:"pc"`
	Phone   string `json:"phone"`
	Seq     int64  `json:"seq"`
	TS      int64  `json:"ts"`
	Child   int64  `json:"child"`
	ID      string `json:"id"`
	Text    string `json:"text"`
	ShowSec int    `json:"showSec"`
	// StickerRaw и SoundRaw — необязательные поля как пришли (раздел 5а). Сырыми, чтобы неверный
	// JSON-тип не валил разбор подписанных данных в bad_sig: вид проверяет CheckNote (range).
	StickerRaw json.RawMessage `json:"sticker,omitempty"`
	SoundRaw   json.RawMessage `json:"sound,omitempty"`
	// Sticker — номер стикера из трёх цифр ("" — без стикера), Sound — 0 (без звука) или 1…99; нет
	// поля sound — 1. Заполняет CheckNote; неизвестный номер не ошибка (подменяет страж).
	Sticker string `json:"-"`
	Sound   int    `json:"-"`
}

// NoteReport — строка сообщения в отчёте: состояние и момент последней смены (часы компьютера).
type NoteReport struct {
	ID    string `json:"id"`
	Child int64  `json:"child"`
	State string `json:"state"` // Note*
	AtMs  int64  `json:"atMs"`
}

// State — то, что служба хранит в настройках (запись целиком, PL-55).
type State struct {
	PCID    string `json:"pcId"`
	PCToken string `json:"pcToken"`
	PCKey   string `json:"pcKey"` // PKCS#8 base64url
	Phone   *Phone `json:"phone"`
	LastSeq int64  `json:"lastSeq"`
}

// Phone — привязанный телефон.
type Phone struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Pub        string `json:"pub"`
	PairedAtMs int64  `json:"pairedAtMs"`
}

// ParsePoll разбирает ответ опроса {"messages":[…],"serverNowMs"}.
func ParsePoll(raw []byte) ([]Msg, int64, error) {
	var out struct {
		Messages    []Msg `json:"messages"`
		ServerNowMs int64 `json:"serverNowMs"`
	}
	if !isObject(raw) {
		return nil, 0, ErrBadMsg
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, 0, ErrBadMsg
	}
	return out.Messages, out.ServerNowMs, nil
}

func isObject(raw []byte) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && raw[0] == '{'
}

// ParseMsg разбирает тело по виду: *Pair1, *Pair2 или *Signed (cmd, note). Ключ pair1 проверяется
// (P-256), nPhone — 16 байт. Подпись команды здесь не проверяется — это CheckCommand.
func ParseMsg(m Msg) (any, error) {
	if m.ID == "" || m.From == "" || !isObject(m.Body) {
		return nil, ErrBadMsg
	}
	switch m.Kind {
	case KindPair1:
		var p Pair1
		if json.Unmarshal(m.Body, &p) != nil {
			return nil, ErrBadMsg
		}
		der, err := unb64(p.PhonePub)
		if err != nil {
			return nil, ErrBadMsg
		}
		k, err := parsePubDER(der)
		if err != nil {
			return nil, ErrBadMsg
		}
		p.key, p.der, p.Name = k, der, cleanName(p.Name)
		return &p, nil
	case KindPair2:
		var p Pair2
		if json.Unmarshal(m.Body, &p) != nil {
			return nil, ErrBadMsg
		}
		n, err := unb64(p.NPhone)
		if err != nil || len(n) != NonceLen {
			return nil, ErrBadMsg
		}
		p.nonce = n
		return &p, nil
	case KindCmd, KindNote:
		var s Signed
		if json.Unmarshal(m.Body, &s) != nil || s.Data == "" || s.Sig == "" {
			return nil, ErrBadMsg
		}
		return &s, nil
	}
	return nil, ErrBadMsg
}

// cleanName — имя телефона без управляющих символов, пробелы схлопнуты, не длиннее 40 знаков.
func cleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxNameRunes {
		s = strings.TrimSpace(string(r[:maxNameRunes]))
	}
	return s
}

// header — общие поля подписанных данных телефона (команда и сообщение).
type header struct {
	V     int    `json:"v"`
	T     string `json:"t"`
	PC    string `json:"pc"`
	Phone string `json:"phone"`
	Seq   int64  `json:"seq"`
	TS    int64  `json:"ts"`
}

// checkSigned — общая часть проверки команды и сообщения (разделы 5, 5а): подпись ключом
// привязанного телефона → тип typ и версия → pc/phone/отправитель → ts ±5 мин. Подпись — первой:
// JSON разбирается только из проверенных байтов (раздел 2). into получает данные целиком.
func checkSigned(st State, s Signed, from string, now time.Time, typ string, into any) (header, []byte, string) {
	var h header
	if st.Phone == nil {
		return h, nil, ErrCodeNotPaired
	}
	pub, err := ParsePub(st.Phone.Pub)
	if err != nil {
		return h, nil, ErrCodeNotPaired
	}
	data, err := Verify(pub, s)
	if err != nil {
		return h, nil, ErrCodeBadSig
	}
	if !isObject(data) || json.Unmarshal(data, &h) != nil || h.T != typ || h.V != ProtoVersion || json.Unmarshal(data, into) != nil {
		return header{}, data, ErrCodeBadSig
	}
	if h.PC != st.PCID || h.Phone != st.Phone.ID || from != st.Phone.ID {
		return h, data, ErrCodeNotPaired
	}
	if d := now.UnixMilli() - h.TS; d > MaxSkew.Milliseconds() || d < -MaxSkew.Milliseconds() {
		return h, data, ErrCodeStale
	}
	return h, data, ""
}

// CheckCommand — проверка команды (протокол, разделы 5, 5б) до применения: подпись ключом
// привязанного телефона → тип и версия → pc/phone/отправитель → ts ±5 мин → op (неизвестный или
// delta у «снять запрет» — range, номер не тратится) → seq больше последнего. Подпись — первой:
// JSON разбирается только из проверенных байтов (раздел 2).
// Пустой код — команду можно отдать Host.ApplyCommand (он проверит seq ещё раз под своей
// блокировкой). data — подписанные байты (их хэш идёт в ответ cmdr).
func CheckCommand(st State, s Signed, from string, now time.Time) (Command, []byte, string) {
	var c Command
	h, data, code := checkSigned(st, s, from, now, TypeCmd, &c)
	if code == ErrCodeBadSig {
		return Command{}, data, code
	}
	if code != "" {
		return c, data, code
	}
	switch c.Op {
	case "", OpExtra: // предел delta проверяет Host.ApplyCommand (общий путь с окном, R10)
	case OpNoForbidden:
		if c.Delta != 0 {
			return c, data, ErrCodeRange
		}
	default:
		return c, data, ErrCodeRange
	}
	if h.Seq <= st.LastSeq {
		return c, data, ErrCodeReplay
	}
	return c, data, ""
}

// CheckNote — проверка сообщения (раздел 5а) до показа: как команда (подпись, тип, pc/phone, ts),
// затем текст (1…300 знаков после обрезки, не больше 5 переводов строк), showSec ∈ {0, 10, 30, 60},
// id, вид sticker и sound (ошибка range) и только потом seq — общий с командами. Текст в ответе уже
// очищен (NoteText), Sticker и Sound разобраны (нет поля sound — 1).
func CheckNote(st State, s Signed, from string, now time.Time) (Note, []byte, string) {
	var n Note
	h, data, code := checkSigned(st, s, from, now, TypeNote, &n)
	if code == ErrCodeBadSig {
		return Note{}, data, code
	}
	if code != "" {
		return n, data, code
	}
	text, ok := NoteText(n.Text)
	sticker, okSticker := noteSticker(n.StickerRaw)
	sound, okSound := noteSound(n.SoundRaw)
	if !ok || !validShowSec(n.ShowSec) || !validNoteID(n.ID) || !okSticker || !okSound {
		return n, data, ErrCodeRange
	}
	n.Text, n.Sticker, n.Sound = text, sticker, sound
	if h.Seq <= st.LastSeq {
		return n, data, ErrCodeReplay
	}
	return n, data, ""
}

// NoteText очищает текст сообщения: переводы строк \r\n и \r — \n, табуляция — пробел, прочие
// управляющие и неверные символы убраны, пробелы по краям обрезаны. false — пусто, длиннее
// MaxNoteRunes знаков или больше MaxNoteLines переводов строк.
func NoteText(raw string) (string, bool) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r == '\n':
			b.WriteRune(r)
		case r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r) || r == unicode.ReplacementChar || unicode.Is(unicode.Cf, r) && r != '\u200d':
		default:
			b.WriteRune(r)
		}
	}
	text := strings.TrimSpace(b.String())
	n := len([]rune(text))
	return text, n >= 1 && n <= MaxNoteRunes && strings.Count(text, "\n") <= MaxNoteLines
}

// noteSticker — номер стикера: нет поля — "" (без стикера), иначе JSON-строка ровно из 3 цифр
// ASCII. Есть ли такой стикер в этой версии, решает страж.
func noteSticker(raw json.RawMessage) (string, bool) {
	if raw == nil {
		return "", true
	}
	var s string
	if t := bytes.TrimSpace(raw); len(t) == 0 || t[0] != '"' || json.Unmarshal(t, &s) != nil || len(s) != 3 {
		return "", false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return "", false
		}
	}
	return s, true
}

// noteSound — номер звука: нет поля — 1, иначе JSON-число записью целого 0…99 без знака, дробной
// части и степени (2.0, 1e0, -0, "2", null — неверный вид). Есть ли такой звук, решает страж.
func noteSound(raw json.RawMessage) (int, bool) {
	if raw == nil {
		return 1, true
	}
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || len(t) > 2 || len(t) == 2 && t[0] == '0' {
		return 0, false
	}
	n := 0
	for _, c := range t {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

func validShowSec(s int) bool { return s == 0 || s == 10 || s == 30 || s == 60 }

// validNoteID — id сообщения: 1…64 знака base64url (телефон шлёт 16 случайных байтов).
func validNoteID(id string) bool {
	if id == "" || len(id) > maxNoteIDLen {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
