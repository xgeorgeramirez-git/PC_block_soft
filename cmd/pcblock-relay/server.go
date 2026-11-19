package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Пределы сервера (протокол, раздел 3).
const (
	maxBody        = 16 << 10
	codeTTL        = 10 * time.Minute
	pairFailMax    = 5
	pairWindow     = 10 * time.Minute
	registerMax    = 10
	registerWindow = time.Hour
	maxPCs         = 20
	// maxPhonesPerPC — учётных записей телефонов на компьютер. Привязка, прерванная на сверке
	// числа, оставляет запись на сервере; при переполнении выбрасывается самая старая.
	maxPhonesPerPC = 5
	// staleAfter — компьютер без единого запроса дольше этого удаляется с телефонами;
	// проверка при регистрации, при старте и не чаще раза в час на любом запросе.
	staleAfter = 30 * 24 * time.Hour
	// unseenAfter — компьютер, зарегистрированный и ни разу не вышедший на связь (ни опроса, ни
	// отчёта), удаляется через столько: регистрации не занимают 20 мест на 30 дней. Знает об этом
	// только память сервера: записи из файла после перезапуска живут по правилу staleAfter.
	unseenAfter = 24 * time.Hour
	// seenWriteEvery — «на связи» пишется в relay.json не чаще этого (не на каждый опрос).
	seenWriteEvery = time.Hour
	purgeEvery     = time.Hour
)

var (
	kindRe = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
	codeRe = regexp.MustCompile(`^[0-9]{6}$`)
)

// Config — настройки сервера. Нулевые поля — значения по умолчанию.
type Config struct {
	DataDir    string
	TrustProxy bool
	Now        func() time.Time
	Rand       io.Reader
	PollMax    time.Duration // по умолчанию 25 с
	ReplyWait  time.Duration // по умолчанию 10 с
	MsgTTL     time.Duration // срок сообщения в очереди, по умолчанию 10 с (как ожидание ответа)
	Log        *log.Logger
}

// Server — почтовый ящик между компьютерами и телефонами. Содержимое сообщений не понимает.
type Server struct {
	cfg Config

	randMu sync.Mutex

	mu       sync.Mutex
	store    *Store
	boxes    map[string]*pcBox
	waiters  map[string]*waiter
	pairIP   *limiter
	pairPC   *limiter
	register *limiter
	purgedAt time.Time
	unseen   map[string]bool // зарегистрированы в этом запуске и ещё ни разу не вышли на связь
}

// New открывает хранилище в cfg.DataDir и готовит сервер.
func New(cfg Config) (*Server, error) {
	if cfg.DataDir == "" {
		return nil, errors.New("не задана папка данных")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Reader
	}
	if cfg.PollMax <= 0 {
		cfg.PollMax = 25 * time.Second
	}
	if cfg.ReplyWait <= 0 {
		cfg.ReplyWait = 10 * time.Second
	}
	if cfg.MsgTTL <= 0 {
		cfg.MsgTTL = 10 * time.Second
	}
	if cfg.Log == nil {
		cfg.Log = log.New(os.Stderr, "", log.LstdFlags|log.LUTC)
	}
	st, err := openStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:      cfg,
		store:    st,
		boxes:    map[string]*pcBox{},
		waiters:  map[string]*waiter{},
		unseen:   map[string]bool{},
		pairIP:   newLimiter(pairFailMax, pairWindow),
		pairPC:   newLimiter(pairFailMax, pairWindow),
		register: newLimiter(registerMax, registerWindow),
	}
	s.mu.Lock()
	err = s.purge(cfg.Now())
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return s, nil
}

// purge удаляет брошенные компьютеры (под s.mu). Очереди и ожидатели удалённых — тоже.
func (s *Server) purge(now time.Time) error {
	s.purgedAt = now
	gone := s.store.dropStale(now.UnixMilli(), staleAfter.Milliseconds(), s.unseen, unseenAfter.Milliseconds())
	if len(gone) == 0 {
		return nil
	}
	for _, id := range gone {
		delete(s.boxes, id)
		delete(s.unseen, id)
		for mid, wt := range s.waiters {
			if wt.pcID == id {
				delete(s.waiters, mid)
			}
		}
	}
	s.cfg.Log.Printf("удалено компьютеров без связи (30 дней или сутки после регистрации): %d", len(gone))
	return s.store.save()
}

// maybePurge — чистка не чаще раза в час (под s.mu). Сбой записи — только журнал.
func (s *Server) maybePurge(now time.Time) {
	if now.Sub(s.purgedAt) < purgeEvery {
		return
	}
	if err := s.purge(now); err != nil {
		s.cfg.Log.Printf("запись %s: %v", storeFile, err)
	}
}

// Handler — все пути протокола (шаблоны http.ServeMux Go 1.22) с журналом запросов.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.hHealth)
	mux.HandleFunc("POST /v1/pc/register", s.hRegister)
	mux.HandleFunc("POST /v1/pc/pair-code", s.hPairCode)
	mux.HandleFunc("GET /v1/pc/poll", s.hPoll)
	mux.HandleFunc("POST /v1/pc/reply", s.hReply)
	mux.HandleFunc("POST /v1/pc/report", s.hReport)
	mux.HandleFunc("POST /v1/pc/unpair", s.hUnpair)
	mux.HandleFunc("POST /v1/phone/pair", s.hPhonePair)
	mux.HandleFunc("POST /v1/phone/send", s.hPhoneSend)
	mux.HandleFunc("GET /v1/phone/status", s.hPhoneStatus)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { s.fail(w, http.StatusNotFound, "not_found") })
	return s.logged(mux)
}

// ---------- общие помощники ----------

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// logged пишет журнал: метод, путь, код, длительность. Без токенов, кодов, тел и строки запроса.
func (s *Server) logged(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		h.ServeHTTP(sw, r)
		path := r.URL.Path
		if len(path) > 64 {
			path = path[:64]
		}
		q := strconv.QuoteToASCII(path) // управляющие символы из чужого пути не портят журнал
		s.cfg.Log.Printf("%s %s %d %dms", r.Method, q[1:len(q)-1], sw.code, time.Since(start).Milliseconds())
	})
}

func (s *Server) now() time.Time { return s.cfg.Now() }

func (s *Server) send(w http.ResponseWriter, code int, v map[string]any) {
	if v == nil {
		v = map[string]any{}
	}
	v["serverNowMs"] = s.now().UnixMilli()
	raw, err := encodeJSON(v)
	if err != nil {
		code, raw = http.StatusInternalServerError, []byte(`{"error":"internal"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	w.Write(raw)
}

func (s *Server) fail(w http.ResponseWriter, code int, errCode string) {
	s.send(w, code, map[string]any{"error": errCode})
}

func (s *Server) ok(w http.ResponseWriter, v map[string]any) { s.send(w, http.StatusOK, v) }

// randBytes — n случайных байтов из инжектируемого источника.
func (s *Server) randBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	s.randMu.Lock()
	defer s.randMu.Unlock()
	if _, err := io.ReadFull(s.cfg.Rand, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Server) newID() (string, error) {
	b, err := s.randBytes(16)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Server) newToken() (string, error) {
	b, err := s.randBytes(32)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// newCode — шесть цифр, равномерно (отбрасываем хвост 2^32, не кратный миллиону).
func (s *Server) newCode() (string, error) {
	const limit = 4294000000 // наибольшее кратное 1 000 000 ниже 2^32
	for {
		b, err := s.randBytes(4)
		if err != nil {
			return "", err
		}
		if v := binary.BigEndian.Uint32(b); v < limit {
			return fmt.Sprintf("%06d", v%1000000), nil
		}
	}
}

func (s *Server) ip(r *http.Request) string {
	return clientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), s.cfg.TrustProxy)
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// decodeJSON разбирает одно JSON-тело запроса в v. Неизвестные ключи не ошибка; хвост после
// объекта — ошибка; пустое тело допустимо только при allowEmpty.
func decodeJSON(rd io.Reader, v any, allowEmpty bool) error {
	dec := json.NewDecoder(rd)
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) && allowEmpty {
			return nil
		}
		return err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("лишние данные после объекта")
		}
		return err
	}
	return nil
}

// body читает тело запроса (≤ 16 КБ). false — ответ об ошибке уже отправлен.
func (s *Server) body(w http.ResponseWriter, r *http.Request, v any, allowEmpty bool) bool {
	if r.ContentLength > maxBody {
		s.fail(w, http.StatusRequestEntityTooLarge, "too_large")
		return false
	}
	err := decodeJSON(http.MaxBytesReader(w, r.Body, maxBody), v, allowEmpty)
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		s.fail(w, http.StatusRequestEntityTooLarge, "too_large")
		return false
	case err != nil:
		s.fail(w, http.StatusBadRequest, "bad_request")
		return false
	}
	return true
}

func (s *Server) box(pcID string) *pcBox {
	b := s.boxes[pcID]
	if b == nil {
		b = newBox()
		s.boxes[pcID] = b
	}
	return b
}

// authPC — компьютер по токену (под s.mu). Отмечает «на связи»: в памяти всегда,
// в relay.json — не чаще раза в час.
func (s *Server) authPC(w http.ResponseWriter, r *http.Request) (pcRec, bool) {
	now := s.now()
	s.maybePurge(now)
	tok := bearer(r)
	if tok != "" {
		if i := s.store.pcByToken(tok); i >= 0 {
			pc := &s.store.data.PCs[i]
			s.box(pc.ID).lastSeen = now
			delete(s.unseen, pc.ID)
			if now.UnixMilli()-pc.LastSeenMs >= seenWriteEvery.Milliseconds() {
				pc.LastSeenMs = now.UnixMilli()
				if err := s.store.save(); err != nil {
					s.cfg.Log.Printf("запись %s: %v", storeFile, err)
				}
			}
			return *pc, true
		}
	}
	s.fail(w, http.StatusUnauthorized, "unauthorized")
	return pcRec{}, false
}

func (s *Server) authPhone(w http.ResponseWriter, r *http.Request) (phoneRec, bool) {
	s.maybePurge(s.now())
	tok := bearer(r)
	if tok != "" {
		if ph := s.store.phoneByToken(tok); ph != nil {
			return *ph, true
		}
	}
	s.fail(w, http.StatusUnauthorized, "unauthorized")
	return phoneRec{}, false
}

// ---------- обработчики ----------

func (s *Server) hHealth(w http.ResponseWriter, r *http.Request) {
	// Порядок ключей фиксирован: ответ начинается с {"ok":true (проверка в инструкции).
	raw := fmt.Sprintf(`{"ok":true,"serverNowMs":%d}`, s.now().UnixMilli())
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, raw)
}

func (s *Server) hRegister(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if !s.body(w, r, &in, true) {
		return
	}
	ip := s.ip(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.register.blocked(ip, now) {
		s.fail(w, http.StatusTooManyRequests, "too_many")
		return
	}
	s.register.hit(ip, now)
	if err := s.purge(now); err != nil {
		s.cfg.Log.Printf("запись %s: %v", storeFile, err)
	}
	if len(s.store.data.PCs) >= maxPCs {
		s.fail(w, http.StatusTooManyRequests, "too_many")
		return
	}
	id, err1 := s.newID()
	tok, err2 := s.newToken()
	if err1 != nil || err2 != nil {
		s.fail(w, http.StatusInternalServerError, "internal")
		return
	}
	s.store.data.PCs = append(s.store.data.PCs, pcRec{ID: id, TokenHash: hashToken(tok), CreatedMs: now.UnixMilli(), LastSeenMs: now.UnixMilli()})
	if err := s.store.save(); err != nil {
		s.store.data.PCs = s.store.data.PCs[:len(s.store.data.PCs)-1]
		s.cfg.Log.Printf("запись %s: %v", storeFile, err)
		s.fail(w, http.StatusInternalServerError, "internal")
		return
	}
	s.unseen[id] = true
	s.ok(w, map[string]any{"pcId": id, "pcToken": tok})
}

func (s *Server) hPairCode(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if !s.body(w, r, &in, true) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pc, ok := s.authPC(w, r)
	if !ok {
		return
	}
	now := s.now()
	var code string
	for {
		c, err := s.newCode()
		if err != nil {
			s.fail(w, http.StatusInternalServerError, "internal")
			return
		}
		if !s.codeTaken(c, now) {
			code = c
			break
		}
	}
	b := s.box(pc.ID)
	b.code, b.codeExp = code, now.Add(codeTTL) // прежний код этого компьютера погашен
	s.ok(w, map[string]any{"code": code, "expiresMs": b.codeExp.UnixMilli()})
}

func (s *Server) codeTaken(code string, now time.Time) bool {
	for _, b := range s.boxes {
		if b.code == code && now.Before(b.codeExp) {
			return true
		}
	}
	return false
}

func (s *Server) hPoll(w http.ResponseWriter, r *http.Request) {
	wait := s.cfg.PollMax
	if q := r.URL.Query().Get("wait"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 0 {
			s.fail(w, http.StatusBadRequest, "bad_request")
			return
		}
		if d := time.Duration(n) * time.Second; d < wait {
			wait = d
		}
	}
	s.mu.Lock()
	pc, ok := s.authPC(w, r)
	if !ok {
		s.mu.Unlock()
		return
	}
	b := s.box(pc.ID)
	msgs := b.q.take(s.now(), s.cfg.MsgTTL, pollBudget)
	sig := b.signal
	s.mu.Unlock()

	if len(msgs) == 0 && wait > 0 {
		t := time.NewTimer(wait)
		select {
		case <-sig:
		case <-t.C:
		case <-r.Context().Done():
		}
		t.Stop()
		s.mu.Lock()
		b.lastSeen = s.now()
		msgs = b.q.take(s.now(), s.cfg.MsgTTL, pollBudget)
		s.mu.Unlock()
	}
	s.ok(w, map[string]any{"messages": msgs})
}

type replyReq struct {
	To      string          `json:"to"`
	ReplyTo string          `json:"replyTo"`
	Kind    string          `json:"kind"`
	Body    json.RawMessage `json:"body"`
}

func (s *Server) hReply(w http.ResponseWriter, r *http.Request) {
	var in replyReq
	if !s.body(w, r, &in, false) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pc, ok := s.authPC(w, r)
	if !ok {
		return
	}
	if in.To == "" || in.ReplyTo == "" || !kindRe.MatchString(in.Kind) {
		s.fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	wt := s.waiters[in.ReplyTo]
	if wt == nil || wt.pcID != pc.ID || wt.phoneID != in.To {
		s.fail(w, http.StatusNotFound, "not_found")
		return
	}
	id, err := s.newID()
	if err != nil {
		s.fail(w, http.StatusInternalServerError, "internal")
		return
	}
	delete(s.waiters, in.ReplyTo)
	wt.ch <- Msg{ID: id, From: pc.ID, Kind: in.Kind, Body: in.Body}
	s.ok(w, nil)
}

type reportReq struct {
	Body json.RawMessage `json:"body"`
}

func (s *Server) hReport(w http.ResponseWriter, r *http.Request) {
	var in reportReq
	if !s.body(w, r, &in, false) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pc, ok := s.authPC(w, r)
	if !ok {
		return
	}
	if len(in.Body) == 0 {
		s.fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	s.box(pc.ID).report = append(json.RawMessage(nil), in.Body...)
	s.ok(w, nil)
}

type unpairReq struct {
	PhoneID string `json:"phoneId"`
}

func (s *Server) hUnpair(w http.ResponseWriter, r *http.Request) {
	var in unpairReq
	if !s.body(w, r, &in, false) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pc, ok := s.authPC(w, r)
	if !ok {
		return
	}
	if in.PhoneID == "" {
		s.fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	phones := s.store.data.Phones
	for i, ph := range phones {
		if ph.ID == in.PhoneID && ph.PCID == pc.ID {
			kept := append(append([]phoneRec{}, phones[:i]...), phones[i+1:]...)
			s.store.data.Phones = kept
			if err := s.store.save(); err != nil {
				s.store.data.Phones = phones
				s.cfg.Log.Printf("запись %s: %v", storeFile, err)
				s.fail(w, http.StatusInternalServerError, "internal")
				return
			}
			s.ok(w, nil)
			return
		}
	}
	s.fail(w, http.StatusNotFound, "not_found")
}

type pairReq struct {
	Code string          `json:"code"`
	Kind string          `json:"kind"`
	Body json.RawMessage `json:"body"`
}

func (s *Server) hPhonePair(w http.ResponseWriter, r *http.Request) {
	var in pairReq
	if !s.body(w, r, &in, false) {
		return
	}
	if in.Code == "" || in.Kind != "pair1" {
		s.fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	ip := s.ip(r)
	s.mu.Lock()
	now := s.now()
	if s.pairIP.blocked(ip, now) {
		s.mu.Unlock()
		s.fail(w, http.StatusTooManyRequests, "too_many")
		return
	}
	// Ищем компьютер с этим кодом (и действующим, и истёкшим — истёкший тоже ошибка этого компьютера).
	var pcID string
	var b *pcBox
	if codeRe.MatchString(in.Code) {
		for id, bx := range s.boxes {
			if bx.code == in.Code {
				pcID, b = id, bx
				break
			}
		}
	}
	if b != nil && s.pairPC.blocked(pcID, now) {
		s.mu.Unlock()
		s.fail(w, http.StatusTooManyRequests, "too_many")
		return
	}
	if b == nil || !now.Before(b.codeExp) {
		s.pairIP.hit(ip, now)
		if b != nil {
			s.pairPC.hit(pcID, now)
		} else {
			// Неверный код не указывает на компьютер: засчитываем всем, у кого код сейчас
			// действует, — так перебор с разных адресов тоже упирается в предел.
			for id, bx := range s.boxes {
				if bx.code != "" && now.Before(bx.codeExp) {
					s.pairPC.hit(id, now)
				}
			}
		}
		s.mu.Unlock()
		s.fail(w, http.StatusForbidden, "bad_code")
		return
	}
	phoneID, err1 := s.newID()
	msgID, err2 := s.newID()
	if err1 != nil || err2 != nil {
		s.mu.Unlock()
		s.fail(w, http.StatusInternalServerError, "internal")
		return
	}
	msg := Msg{ID: msgID, From: phoneID, Kind: in.Kind, Body: in.Body}
	if !b.q.put(msg, now, s.cfg.MsgTTL) {
		s.mu.Unlock()
		s.fail(w, http.StatusServiceUnavailable, "busy")
		return
	}
	code, codeExp := b.code, b.codeExp
	b.code = "" // код одноразовый; вернём, если компьютер не ответит
	wt := &waiter{pcID: pcID, phoneID: phoneID, ch: make(chan Msg, 1)}
	s.waiters[msgID] = wt
	b.notify()
	s.mu.Unlock()

	reply, ok, delivered := s.awaitReply(r.Context(), pcID, msgID, wt)
	if !ok {
		// Код возвращается, только если компьютер pair1 не получил: выданный pair1 уже перевёл
		// его привязку дальше, и код у компьютера погашен (протокол, раздел 4).
		s.mu.Lock()
		if !delivered && b.code == "" && s.now().Before(codeExp) {
			b.code, b.codeExp = code, codeExp
		}
		s.mu.Unlock()
		s.fail(w, http.StatusGatewayTimeout, "pc_offline")
		return
	}

	tok, err := s.newToken()
	if err != nil {
		s.fail(w, http.StatusInternalServerError, "internal")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.pcExists(pcID) { // компьютер удалён чисткой, пока шла привязка
		s.fail(w, http.StatusGatewayTimeout, "pc_offline")
		return
	}
	prev := s.store.data.Phones
	s.store.data.Phones = addPhone(prev, phoneRec{ID: phoneID, PCID: pcID, TokenHash: hashToken(tok), CreatedMs: s.now().UnixMilli()})
	if err := s.store.save(); err != nil {
		s.store.data.Phones = prev
		s.cfg.Log.Printf("запись %s: %v", storeFile, err)
		s.fail(w, http.StatusInternalServerError, "internal")
		return
	}
	s.ok(w, map[string]any{"pcId": pcID, "phoneId": phoneID, "phoneToken": tok, "reply": reply})
}

func (s *Server) pcExists(id string) bool {
	for _, pc := range s.store.data.PCs {
		if pc.ID == id {
			return true
		}
	}
	return false
}

// addPhone добавляет телефон; у компьютера остаётся не больше maxPhonesPerPC самых новых.
func addPhone(list []phoneRec, ph phoneRec) []phoneRec {
	out := append(append([]phoneRec{}, list...), ph)
	for {
		n, oldest := 0, -1
		for i, p := range out {
			if p.PCID != ph.PCID {
				continue
			}
			n++
			if oldest < 0 || p.CreatedMs < out[oldest].CreatedMs {
				oldest = i
			}
		}
		if n <= maxPhonesPerPC {
			return out
		}
		out = append(out[:oldest], out[oldest+1:]...)
	}
}

type sendReq struct {
	Kind string          `json:"kind"`
	Body json.RawMessage `json:"body"`
}

func (s *Server) hPhoneSend(w http.ResponseWriter, r *http.Request) {
	var in sendReq
	if !s.body(w, r, &in, false) {
		return
	}
	s.mu.Lock()
	ph, ok := s.authPhone(w, r)
	if !ok {
		s.mu.Unlock()
		return
	}
	// pair1 — только через phone/pair (код и его пределы); иначе привязанный телефон начинал бы
	// новую привязку в обход кода.
	if !kindRe.MatchString(in.Kind) || in.Kind == "pair1" {
		s.mu.Unlock()
		s.fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	msgID, err := s.newID()
	if err != nil {
		s.mu.Unlock()
		s.fail(w, http.StatusInternalServerError, "internal")
		return
	}
	b := s.box(ph.PCID)
	if !b.q.put(Msg{ID: msgID, From: ph.ID, Kind: in.Kind, Body: in.Body}, s.now(), s.cfg.MsgTTL) {
		s.mu.Unlock()
		s.fail(w, http.StatusServiceUnavailable, "busy")
		return
	}
	wt := &waiter{pcID: ph.PCID, phoneID: ph.ID, ch: make(chan Msg, 1)}
	s.waiters[msgID] = wt
	b.notify()
	s.mu.Unlock()

	reply, ok, _ := s.awaitReply(r.Context(), ph.PCID, msgID, wt)
	if !ok {
		s.fail(w, http.StatusGatewayTimeout, "pc_offline")
		return
	}
	s.ok(w, map[string]any{"reply": reply})
}

// awaitReply ждёт ответа компьютера до ReplyWait. Не дождались — сообщение убирается из очереди
// (если ещё не выдано), чтобы компьютер не выполнил команду, о которой телефон думает «не отправлена».
// delivered — сообщение уже выдано опросом компьютеру (из очереди не убрано).
func (s *Server) awaitReply(ctx context.Context, pcID, msgID string, wt *waiter) (m Msg, ok, delivered bool) {
	t := time.NewTimer(s.cfg.ReplyWait)
	defer t.Stop()
	select {
	case m := <-wt.ch:
		return m, true, true
	case <-t.C:
	case <-ctx.Done():
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.waiters, msgID)
	delivered = true
	if b := s.boxes[pcID]; b != nil && b.q.remove(msgID) {
		delivered = false
	}
	select {
	case m := <-wt.ch: // ответ успел прийти в последний момент
		return m, true, true
	default:
		return Msg{}, false, delivered
	}
}

func (s *Server) hPhoneStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ph, ok := s.authPhone(w, r)
	if !ok {
		return
	}
	var seen int64
	var report any // null, пока отчёта нет
	if b := s.boxes[ph.PCID]; b != nil {
		if !b.lastSeen.IsZero() {
			seen = b.lastSeen.UnixMilli()
		}
		if len(b.report) > 0 {
			report = b.report
		}
	}
	s.ok(w, map[string]any{"pcLastSeenMs": seen, "report": report})
}

// ---------- для тестов ----------

func (s *Server) queueLen(pcID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b := s.boxes[pcID]; b != nil {
		return b.q.len()
	}
	return 0
}

func (s *Server) pcCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.store.data.PCs)
}

func (s *Server) phoneCount(pcID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, p := range s.store.data.Phones {
		if p.PCID == pcID {
			n++
		}
	}
	return n
}
