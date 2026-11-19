package remote

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// ErrStale — SaveState от старого поколения связи: служба его отбрасывает.
var ErrStale = errors.New("remote: старое поколение связи")

// Этапы привязки (PairInfo.Stage; протокол, раздел 4).
const (
	StageIdle      = "idle"
	StageCode      = "code"       // код получаем (Code пуст) или показываем
	StageWaitNonce = "wait_nonce" // телефон прислал pair1, ждём pair2
	StageCompare   = "compare"    // сверка числа SAS
	StageDone      = "done"       // «Совпадает»: телефон записан
	StageFailed    = "failed"     // отменено, причина в Error
)

// Причины отмены привязки (PairInfo.Error); тексты делает окно.
const (
	PairErrNoCode   = "no_code"   // код не получен: сервер не ответил на pair-code
	PairErrNoServer = "no_server" // код был показан, ответ телефону не ушёл
	PairErrExpired  = "expired"   // 10 мин без следующего шага
	PairErrMismatch = "mismatch"  // «Не совпадает»
	PairErrAborted  = "aborted"   // нарушен порядок привязки
	PairErrBadPhone = "bad_phone" // неверное сообщение телефона
	PairErrStopped  = "stopped"   // связь выключена
	PairErrLost     = "lost"      // сервер не узнал компьютер
	PairErrSave     = "save"      // служба не записала состояние
)

// События для журнала родителя (Host.Event).
const (
	EventPaired   = "paired"   // detail — имя телефона
	EventUnpaired = "unpaired" // detail — имя телефона (отвязал родитель)
	EventLost     = "lost"     // detail — имя телефона: сервер не узнал компьютер, привязка сброшена
)

const (
	pollWait       = 25 * time.Second
	pollTimeout    = pollWait + 10*time.Second // тайм-аут клиента опроса 35 с
	requestTimeout = 15 * time.Second
	backoffMax     = 30 * time.Second
	pairTTL        = 10 * time.Minute
	panicPause     = 5 * time.Second
	logEvery       = time.Minute
	netLogRepeat   = time.Hour // та же ошибка связи — в журнал работы не чаще (PL-53)
)

// Host — порт службы. Каждый метод сам берёт блокировку службы (s.mu); Link вызывает их без
// своего мьютекса и только из своих горутин.
type Host interface {
	LoadState() State
	// SaveState пишет состояние целиком; gen не текущий (Link.Gen) — ErrStale, ничего не записано.
	SaveState(gen int64, st State) error
	// ApplyCommand под одной блокировкой: gen не текущий — отказ без записи; seq ≤ LastSeq —
	// replay; иначе записать LastSeq = seq, затем доп. время (общий путь с окном родителя).
	// code: "" | replay | no_child | range | stale; lastSeq — после вызова.
	ApplyCommand(gen int64, seq int64, childID int64, delta int) (msg string, code string, lastSeq int64)
	Children() []ChildReport
	Event(kind, detail string)
	Logf(format string, args ...any)
}

// Config — настройки связи. Нулевые поля — значения по умолчанию.
type Config struct {
	URL         *url.URL          // проверенный ValidateURL
	Host        Host              //
	Transport   http.RoundTripper // nil: свой http.Transport, Proxy: nil
	Now         func() time.Time  // настоящие часы службы (RealNow), не часы ядра
	Sleep       func(ctx context.Context, d time.Duration) bool
	Rand        io.Reader     // nil: crypto/rand
	ReportEvery time.Duration // 0: 60 с

	tick time.Duration // шаг проверки сроков (тесты)
}

// LinkStatus — состояние связи для окна.
type LinkStatus struct {
	Running  bool   // связь включена (поколение идёт)
	Online   bool   // последний опрос сервера удался
	LastOKMs int64  // время последнего удачного опроса (Config.Now)
	LastErr  string // StatusNoServer | StatusBadAnswer | StatusBadAddress | StatusTooMany | ""
}

// PairInfo — ход привязки для окна.
type PairInfo struct {
	Stage     string
	Code      string // шесть цифр на этапе code
	ExpiresMs int64  // срок кода или сверки (Config.Now)
	SAS       string // число сверки на этапе compare
	PhoneName string
	Error     string // PairErr* на этапе failed
}

var genCounter atomic.Int64 // поколения уникальны среди всех Link процесса

// Link — связь с сервером: две горутины на поколение (опрос и отправка).
type Link struct {
	cfg Config
	tr  http.RoundTripper
	hc  *http.Client

	mu     sync.Mutex
	gen    int64
	run    *run
	old    []*run // остановленные поколения, чьи горутины ещё могут работать
	status LinkStatus
	pair   PairInfo

	randMu sync.Mutex
	logMu  sync.Mutex
	logAt  map[string]time.Time
	net    netLog            // под logMu: связь с сервером для журнала работы (общая для поколений)
	quiet  map[string]netLog // под logMu: прочие повторяющиеся ошибки по ключу (logChange)
}

// netLog — что последним записано в журнал работы о связи: строка пишется при смене состояния
// (на связи ↔ нет связи, новый код ошибки), та же ошибка — не чаще netLogRepeat (PL-53).
type netLog struct {
	down bool
	code string
	at   time.Time
}

type actionKind int

const (
	actStartPairing actionKind = iota
	actCancelPairing
	actConfirm
	actUnpair
	actReport
)

type action struct {
	kind    actionKind
	match   bool
	phoneID string // actUnpair: какой телефон отвязать
}

// run — одно поколение связи.
type run struct {
	gen    int64
	ctx    context.Context
	cancel context.CancelFunc
	wake   chan struct{} // ёмкость 1
	acts   []action      // под Link.mu
	msgs   chan []Msg    // опрос → отправка
	unauth chan string   // опрос получил 401 с этим токеном
	left   atomic.Int32  // живых горутин поколения
	panics atomic.Int32  // перезапусков после паники
	done   chan struct{} // закрыт, когда обе завершились

	credMu sync.Mutex
	token  string
	credCh chan struct{} // закрывается при смене токена
}

func (r *run) creds() (string, chan struct{}) {
	r.credMu.Lock()
	defer r.credMu.Unlock()
	return r.token, r.credCh
}

func (r *run) setToken(tok string) {
	r.credMu.Lock()
	defer r.credMu.Unlock()
	if r.token != tok {
		r.token = tok
		close(r.credCh)
		r.credCh = make(chan struct{})
	}
}

// NewLink готовит связь; ничего не запускает и в сеть не ходит.
func NewLink(cfg Config) *Link {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepCtx
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Reader
	}
	if cfg.ReportEvery <= 0 {
		cfg.ReportEvery = 60 * time.Second
	}
	if cfg.tick <= 0 {
		cfg.tick = time.Second
	}
	tr := cfg.Transport
	if tr == nil {
		tr = newTransport()
	}
	return &Link{
		cfg:   cfg,
		tr:    tr,
		hc:    &http.Client{Transport: tr, CheckRedirect: noRedirect},
		pair:  PairInfo{Stage: StageIdle},
		logAt: map[string]time.Time{},
		quiet: map[string]netLog{},
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// Start запускает новое поколение и возвращает его номер (уже запущено — номер текущего;
// нет адреса — 0 и LastErr bad_address). Не ходит в сеть и не ждёт.
func (l *Link) Start() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.run != nil {
		return l.gen
	}
	if l.cfg.URL == nil || l.cfg.Host == nil {
		l.status = LinkStatus{LastErr: StatusBadAddress}
		return 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &run{
		gen: genCounter.Add(1), ctx: ctx, cancel: cancel,
		wake: make(chan struct{}, 1), msgs: make(chan []Msg, 1), unauth: make(chan string, 1),
		credCh: make(chan struct{}), done: make(chan struct{}),
	}
	r.left.Store(2)
	l.gen, l.run = r.gen, r
	l.status = LinkStatus{Running: true, LastOKMs: l.status.LastOKMs}
	l.pair = PairInfo{Stage: StageIdle}
	go l.guarded(r, "отправка", l.work)
	go l.guarded(r, "опрос", l.poll)
	return r.gen
}

// Stop отменяет поколение (идущий опрос обрывается) и закрывает простаивающие соединения.
// Не ждёт горутины: служба зовёт его под своей блокировкой. Ждать — Wait, вне блокировки.
func (l *Link) Stop() {
	l.mu.Lock()
	if l.run != nil {
		l.run.cancel()
		l.old = append(l.old, l.run)
		l.run = nil
	}
	l.gen = 0
	l.status = LinkStatus{LastOKMs: l.status.LastOKMs}
	l.pair = PairInfo{Stage: StageIdle}
	l.mu.Unlock()
	l.closeIdle()
}

// Wait ждёт завершения горутин всех остановленных поколений не дольше timeout. Звать вне
// блокировки службы: горутина может ждать её в методе Host.
func (l *Link) Wait(timeout time.Duration) bool {
	t := time.NewTimer(timeout)
	defer t.Stop()
	l.mu.Lock()
	old := append([]*run(nil), l.old...)
	l.mu.Unlock()
	for _, r := range old {
		select {
		case <-r.done:
		case <-t.C:
			return false
		}
	}
	l.mu.Lock()
	kept := l.old[:0]
	for _, r := range l.old {
		select {
		case <-r.done:
		default:
			kept = append(kept, r)
		}
	}
	l.old = kept
	l.mu.Unlock()
	return true
}

// Gen — номер идущего поколения (0 — связь остановлена). Служба сверяет с ним gen в
// SaveState и ApplyCommand.
func (l *Link) Gen() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.gen
}

// Status — состояние связи.
func (l *Link) Status() LinkStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.status
}

// PairInfo — ход привязки.
func (l *Link) PairInfo() PairInfo {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.pair
}

// StartPairing — «Подключить телефон»: этап code сразу, код — от отправки (≤ 2 с).
func (l *Link) StartPairing() {
	if !l.push(action{kind: actStartPairing}, func() { l.pair = PairInfo{Stage: StageCode} }) {
		l.mu.Lock()
		l.pair = PairInfo{Stage: StageFailed, Error: PairErrStopped}
		l.mu.Unlock()
	}
}

// CancelPairing — «Отмена» привязки.
func (l *Link) CancelPairing() { l.push(action{kind: actCancelPairing}, nil) }

// ConfirmPairing — «Совпадает» (true) или «Не совпадает» (false) на этапе compare.
func (l *Link) ConfirmPairing(match bool) { l.push(action{kind: actConfirm, match: match}, nil) }

// Unpair — «Отвязать» телефон phoneID (тот, что видел родитель). Записанный телефон другой (новый
// подключён «Совпадает» раньше в очереди) — ничего. Работает только при запущенной связи; при
// выключенной служба сама стирает Phone в своём состоянии (токен телефона на сервере тогда
// остаётся до следующей привязки).
func (l *Link) Unpair(phoneID string) { l.push(action{kind: actUnpair, phoneID: phoneID}, nil) }

// Changed — отчёт телефону вне очереди (после изменения в окне родителя).
func (l *Link) Changed() { l.push(action{kind: actReport}, nil) }

// push кладёт задание текущему поколению; не ждёт и не ходит в сеть.
func (l *Link) push(a action, under func()) bool {
	l.mu.Lock()
	r := l.run
	if r != nil {
		r.acts = append(r.acts, a)
		if under != nil {
			under()
		}
	}
	l.mu.Unlock()
	if r == nil {
		return false
	}
	select {
	case r.wake <- struct{}{}:
	default:
	}
	return true
}

func (l *Link) takeActions(r *run) []action {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := r.acts
	r.acts = nil
	return a
}

// update меняет состояние для окна, только если поколение r ещё текущее.
func (l *Link) update(r *run, f func()) {
	l.mu.Lock()
	if l.run == r {
		f()
	}
	l.mu.Unlock()
}

func (l *Link) setPair(r *run, p PairInfo) { l.update(r, func() { l.pair = p }) }

func (l *Link) closeIdle() {
	if ci, ok := l.tr.(interface{ CloseIdleConnections() }); ok {
		ci.CloseIdleConnections()
	}
}

// logRate — строка в журнал работы службы не чаще раза в минуту на ключ.
func (l *Link) logRate(key, format string, args ...any) {
	now := l.cfg.Now()
	l.logMu.Lock()
	last, seen := l.logAt[key]
	if seen && now.Sub(last) < logEvery && now.Sub(last) >= 0 {
		l.logMu.Unlock()
		return
	}
	l.logAt[key] = now
	l.logMu.Unlock()
	l.cfg.Host.Logf(format, args...)
}

// netFail — запрос к серверу не удался (опрос, регистрация): строка «нет связи» при потере связи
// или новом коде ошибки, повтор той же — не чаще раза в час.
func (l *Link) netFail(what string, err error) {
	code := errCode(err)
	now := l.cfg.Now()
	l.logMu.Lock()
	n := l.net
	var line string
	switch {
	case !n.down || n.code != code:
		line = "связь с телефоном: нет связи с сервером (%s, %s): %v"
	case now.Sub(n.at) >= netLogRepeat || now.Before(n.at):
		line = "связь с телефоном: всё ещё нет связи с сервером (%s, %s): %v"
	}
	if line != "" {
		l.net = netLog{down: true, code: code, at: now}
	}
	l.logMu.Unlock()
	if line != "" {
		l.cfg.Host.Logf(line, what, code, err)
	}
}

// netOK — запрос к серверу удался: после потери связи — строка «связь восстановлена».
func (l *Link) netOK() {
	l.logMu.Lock()
	was := l.net.down
	l.net = netLog{at: l.cfg.Now()}
	l.logMu.Unlock()
	if was {
		l.cfg.Host.Logf("связь с телефоном: связь с сервером восстановлена")
	}
}

// logChange — повторяющаяся ошибка запроса (отчёт): пока связи нет, молчит (о потере уже написал
// опрос); иначе строка при новом коде ошибки, та же — не чаще раза в час; err == nil — удался.
func (l *Link) logChange(key string, err error, format string, args ...any) {
	now := l.cfg.Now()
	l.logMu.Lock()
	if err == nil {
		delete(l.quiet, key)
		l.logMu.Unlock()
		return
	}
	code := errCode(err)
	q, seen := l.quiet[key]
	write := !l.net.down && (!seen || q.code != code || now.Sub(q.at) >= netLogRepeat || now.Before(q.at))
	if write {
		l.quiet[key] = netLog{down: true, code: code, at: now}
	}
	l.logMu.Unlock()
	if write {
		l.cfg.Host.Logf(format, args...)
	}
}

func (l *Link) randBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	l.randMu.Lock()
	defer l.randMu.Unlock()
	_, err := io.ReadFull(l.cfg.Rand, b)
	return b, err
}

// backoff — пауза после n-й подряд ошибки: 1, 2, 4 … 30 с ± 20 %, не больше 30 с.
func (l *Link) backoff(n int) time.Duration {
	base := backoffMax
	if n < 5 {
		base = time.Second << n
	}
	f := 1.0
	if b, err := l.randBytes(2); err == nil {
		f = 0.8 + 0.4*float64(binary.BigEndian.Uint16(b))/65535
	}
	d := time.Duration(float64(base) * f)
	if d > backoffMax {
		d = backoffMax
	}
	return d
}

// guarded — защита горутины: паника пишется в журнал, цикл перезапускается после паузы.
func (l *Link) guarded(r *run, name string, body func(*run)) {
	defer func() {
		if r.left.Add(-1) == 0 {
			close(r.done)
		}
		l.closeIdle()
	}()
	for r.ctx.Err() == nil {
		if l.safe(r, name, body) {
			return
		}
		if !l.cfg.Sleep(r.ctx, panicPause) {
			return
		}
	}
}

func (l *Link) safe(r *run, name string, body func(*run)) (done bool) {
	defer func() {
		if p := recover(); p != nil {
			r.panics.Add(1)
			l.cfg.Host.Logf("связь с телефоном: сбой (%s), перезапуск: %v", name, p)
			done = false
		}
	}()
	body(r)
	return true
}

func (l *Link) client(tok string) *Client {
	return &Client{Base: l.cfg.URL, HTTP: l.hc, Token: tok}
}

// ---------- горутина опроса ----------

func (l *Link) poll(r *run) {
	fails := 0
	for {
		tok, ch := r.creds()
		if tok == "" {
			select {
			case <-ch:
				continue
			case <-r.ctx.Done():
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.ctx, pollTimeout)
		msgs, err := l.client(tok).Poll(ctx, pollWait)
		cancel()
		if r.ctx.Err() != nil {
			return
		}
		if isUnauthorized(err) {
			l.update(r, func() { l.status.Online = false })
			select {
			case r.unauth <- tok:
			default:
			}
			for t2, ch2 := r.creds(); t2 == tok; t2, ch2 = r.creds() {
				select {
				case <-ch2:
				case <-r.ctx.Done():
					return
				}
			}
			continue
		}
		if err != nil {
			code := errCode(err)
			l.update(r, func() { l.status.Online, l.status.LastErr = false, code })
			l.netFail("опрос", err)
			d := l.backoff(fails)
			fails++
			if !l.cfg.Sleep(r.ctx, d) {
				return
			}
			continue
		}
		fails = 0
		l.netOK()
		now := l.cfg.Now().UnixMilli()
		l.update(r, func() { l.status.Online, l.status.LastErr, l.status.LastOKMs = true, "", now })
		if len(msgs) > 0 {
			select {
			case r.msgs <- msgs:
			case <-r.ctx.Done():
				return
			}
		}
	}
}

// ---------- горутина отправки ----------

// session — идущая привязка (только в горутине отправки).
type session struct {
	stage    string
	code     string
	deadline time.Time
	phoneID  string
	name     string
	phoneDER []byte
	nPc      []byte
	sas      string
}

type worker struct {
	l          *Link
	r          *run
	h          Host
	tok        string
	key        *ecdsa.PrivateKey
	pcDER      []byte
	sess       *session
	lastReport time.Time
	reportDue  bool
	rereg      bool // запрос получил 401: зарегистрироваться заново
	dead       bool // служба отбросила запись (старое поколение)
}

func (l *Link) work(r *run) {
	w := &worker{l: l, r: r, h: l.cfg.Host}
	if r.panics.Load() > 0 { // перезапуск: идущая привязка потеряна вместе с прежней горутиной
		l.update(r, func() {
			switch l.pair.Stage {
			case StageCode, StageWaitNonce, StageCompare:
				l.pair = PairInfo{Stage: StageFailed, Error: PairErrAborted}
			}
		})
	}
	if !w.ensureRegistered() {
		return
	}
	w.reportDue = true
	tick := time.NewTicker(l.cfg.tick)
	defer tick.Stop()
	for !w.stopped() {
		w.doActions()
		w.housekeep()
		if w.stopped() {
			return
		}
		select {
		case <-r.ctx.Done():
			return
		case <-r.wake:
		case <-tick.C:
		case msgs := <-r.msgs:
			for _, m := range msgs {
				if w.stopped() {
					return
				}
				w.handle(m)
			}
		case tok := <-r.unauth:
			if tok == w.tok {
				w.rereg = true
			}
		}
		if w.rereg && !w.stopped() {
			w.reregister()
		}
	}
}

func (w *worker) stopped() bool { return w.dead || w.r.ctx.Err() != nil }

func (w *worker) now() time.Time { return w.l.cfg.Now() }

// call — запрос отправки с тайм-аутом; 401 — пометка «зарегистрироваться заново».
func (w *worker) call(f func(ctx context.Context, c *Client) error) error {
	ctx, cancel := context.WithTimeout(w.r.ctx, requestTimeout)
	defer cancel()
	err := f(ctx, w.l.client(w.tok))
	if isUnauthorized(err) && w.r.ctx.Err() == nil {
		w.rereg = true
	}
	return err
}

func (w *worker) save(st State) bool {
	err := w.h.SaveState(w.r.gen, st)
	if errors.Is(err, ErrStale) {
		w.dead = true
		w.r.cancel()
		return false
	}
	if err != nil {
		w.l.logRate("save", "связь с телефоном: состояние не записано: %v", err)
		return false
	}
	return true
}

func (w *worker) setStatusErr(code string) {
	w.l.update(w.r, func() { w.l.status.Online, w.l.status.LastErr = false, code })
}

// ensureRegistered — ключ компьютера и учётная запись на сервере; пока сервер недоступен,
// повторяет с паузой и отвечает на задания окна.
func (w *worker) ensureRegistered() bool {
	var regID, regTok string // уже выданы сервером, но ещё не записаны службой
	fails := 0
	retry := func() bool {
		d := w.l.backoff(fails)
		fails++
		return w.l.cfg.Sleep(w.r.ctx, d)
	}
	for !w.stopped() {
		w.doActions()
		st := w.h.LoadState()
		changed := false
		key, err := ParsePriv(st.PCKey)
		if err != nil {
			if st.PCKey != "" {
				w.l.logRate("key", "связь с телефоном: ключ компьютера не прочитан, создаётся новый")
			}
			if key, err = NewKey(); err != nil {
				return false
			}
			st.PCKey, _ = MarshalPriv(key)
			st.Phone, st.LastSeq = nil, 0 // телефон знает только прежний ключ
			changed = true
		}
		if st.PCID == "" || st.PCToken == "" {
			if regID == "" {
				err := w.call(func(ctx context.Context, c *Client) error {
					c.Token = ""
					var e error
					regID, regTok, e = c.Register(ctx)
					return e
				})
				if err != nil {
					code := errCode(err)
					w.setStatusErr(code)
					w.l.netFail("регистрация", err)
					if !retry() {
						return false
					}
					continue
				}
			}
			st.PCID, st.PCToken, st.Phone, st.LastSeq = regID, regTok, nil, 0
			changed = true
		}
		if changed && !w.save(st) {
			if w.dead || !retry() {
				return false
			}
			continue
		}
		w.key = key
		w.pcDER, _ = unb64(MarshalPub(&key.PublicKey))
		w.tok = st.PCToken
		w.rereg = false
		w.r.setToken(st.PCToken)
		return true
	}
	return false
}

// reregister — сервер не узнал компьютер (401): новая регистрация, привязка сброшена.
func (w *worker) reregister() {
	w.rereg = false
	if w.sess != nil {
		w.sess = nil
		w.l.setPair(w.r, PairInfo{Stage: StageFailed, Error: PairErrLost})
	}
	st := w.h.LoadState()
	if st.PCToken != w.tok {
		return
	}
	had := st.Phone
	st.PCID, st.PCToken, st.Phone, st.LastSeq = "", "", nil, 0
	if !w.save(st) {
		return
	}
	if had != nil {
		w.h.Event(EventLost, had.Name)
	}
	w.r.setToken("")
	w.tok = ""
	if w.ensureRegistered() {
		w.reportDue = true
	}
}

func (w *worker) doActions() {
	for _, a := range w.l.takeActions(w.r) {
		if w.stopped() {
			return
		}
		switch a.kind {
		case actStartPairing:
			w.startPairing()
		case actCancelPairing:
			w.abort("", StageIdle)
		case actConfirm:
			w.confirm(a.match)
		case actUnpair:
			w.unpair(a.phoneID)
		case actReport:
			w.reportDue = true
		}
	}
}

func (w *worker) housekeep() {
	now := w.now()
	if w.sess != nil && !now.Before(w.sess.deadline) {
		w.abort(PairErrExpired, StageFailed)
	}
	if w.tok != "" && (w.reportDue || now.Sub(w.lastReport) >= w.l.cfg.ReportEvery) {
		w.sendReport()
	}
}

// ---------- привязка (протокол, раздел 4) ----------

func (w *worker) publish() {
	s := w.sess
	p := PairInfo{Stage: s.stage, PhoneName: s.name, ExpiresMs: s.deadline.UnixMilli()}
	switch s.stage {
	case StageCode:
		p.Code = s.code
	case StageCompare:
		p.SAS = s.sas
	}
	w.l.setPair(w.r, p)
}

// abort отменяет привязку: токен телефона этой привязки гасится на сервере.
func (w *worker) abort(errCode, stage string) {
	s := w.sess
	w.sess = nil
	if s == nil && stage == StageFailed {
		return
	}
	w.l.setPair(w.r, PairInfo{Stage: stage, Error: errCode})
	if s != nil && s.phoneID != "" {
		w.unpairRemote(s.phoneID)
	}
}

func (w *worker) startPairing() {
	if w.sess != nil {
		w.abort("", StageIdle)
	}
	w.l.setPair(w.r, PairInfo{Stage: StageCode})
	if w.tok == "" {
		w.l.setPair(w.r, PairInfo{Stage: StageFailed, Error: PairErrNoCode})
		return
	}
	var code string
	err := w.call(func(ctx context.Context, c *Client) error {
		var e error
		code, e = c.PairCode(ctx)
		return e
	})
	if err != nil {
		reason := PairErrNoCode
		if isUnauthorized(err) {
			reason = PairErrLost
		}
		w.l.setPair(w.r, PairInfo{Stage: StageFailed, Error: reason})
		w.l.logRate("pair-code", "связь с телефоном: код привязки не получен: %v", err)
		return
	}
	// Срок — по своим часам: окно считает отсчёт по ним же.
	w.sess = &session{stage: StageCode, code: code, deadline: w.now().Add(pairTTL)}
	w.publish()
}

func (w *worker) onPair1(m Msg, v any, perr error) {
	s := w.sess
	if s == nil {
		w.l.logRate("pair1-idle", "связь с телефоном: pair1 без привязки отброшен")
		return
	}
	if s.stage != StageCode {
		w.abort(PairErrAborted, StageFailed)
		return
	}
	if perr != nil {
		w.abort(PairErrBadPhone, StageFailed)
		return
	}
	p := v.(*Pair1)
	nPc, err := w.l.randBytes(NonceLen)
	if err != nil {
		w.abort(PairErrNoServer, StageFailed)
		return
	}
	// Код погашен первым pair1 (сервер погасил его сам).
	s.stage, s.code, s.phoneID, s.name, s.phoneDER, s.nPc = StageWaitNonce, "", m.From, p.Name, p.der, nPc
	s.deadline = w.now().Add(pairTTL)
	w.publish()
	body := Pair1R{PCPub: b64(w.pcDER), Commit: b64(Commit(w.pcDER, p.der, nPc))}
	if err := w.call(func(ctx context.Context, c *Client) error { return c.Reply(ctx, m.From, m.ID, KindPair1R, body) }); err != nil {
		w.abort(PairErrNoServer, StageFailed)
	}
}

func (w *worker) onPair2(m Msg, v any, perr error) {
	s := w.sess
	if s == nil {
		w.l.logRate("pair2-idle", "связь с телефоном: pair2 без привязки отброшен")
		return
	}
	if s.stage != StageWaitNonce || m.From != s.phoneID {
		w.abort(PairErrAborted, StageFailed)
		return
	}
	if perr != nil {
		w.abort(PairErrBadPhone, StageFailed)
		return
	}
	nPhone := v.(*Pair2).nonce
	s.stage, s.sas = StageCompare, SAS(w.pcDER, s.phoneDER, s.nPc, nPhone)
	s.deadline = w.now().Add(pairTTL)
	if err := w.call(func(ctx context.Context, c *Client) error {
		return c.Reply(ctx, m.From, m.ID, KindPair2R, Pair2R{NPc: b64(s.nPc)})
	}); err != nil {
		w.abort(PairErrNoServer, StageFailed)
		return
	}
	w.publish() // число — только когда телефон получил nPc
}

func (w *worker) confirm(match bool) {
	s := w.sess
	if s == nil || s.stage != StageCompare {
		return
	}
	if !match {
		w.abort(PairErrMismatch, StageFailed)
		return
	}
	now := w.now()
	if !now.Before(s.deadline) {
		w.abort(PairErrExpired, StageFailed)
		return
	}
	st := w.h.LoadState()
	old := st.Phone
	st.Phone = &Phone{ID: s.phoneID, Name: s.name, Pub: b64(s.phoneDER), PairedAtMs: now.UnixMilli()}
	st.LastSeq = 0 // у нового телефона счёт с 1
	if !w.save(st) {
		if !w.dead {
			w.abort(PairErrSave, StageFailed)
		}
		return
	}
	w.sess = nil
	w.l.setPair(w.r, PairInfo{Stage: StageDone, PhoneName: s.name})
	w.h.Event(EventPaired, s.name)
	if old != nil && old.ID != s.phoneID {
		w.unpairRemote(old.ID) // один телефон (D-92)
	}
	w.reportDue = true
}

func (w *worker) unpair(phoneID string) {
	st := w.h.LoadState()
	if st.Phone == nil || st.Phone.ID != phoneID {
		return
	}
	old := st.Phone
	st.Phone, st.LastSeq = nil, 0
	if !w.save(st) {
		return
	}
	w.h.Event(EventUnpaired, old.Name)
	w.unpairRemote(old.ID)
}

func (w *worker) unpairRemote(phoneID string) {
	if w.tok == "" {
		return
	}
	err := w.call(func(ctx context.Context, c *Client) error { return c.Unpair(ctx, phoneID) })
	var se *StatusError
	if err != nil && !(errors.As(err, &se) && se.Status == http.StatusNotFound) {
		w.l.logRate("unpair", "связь с телефоном: токен телефона на сервере не погашен: %v", err)
	}
}

// ---------- сообщения ----------

func (w *worker) handle(m Msg) {
	v, err := ParseMsg(m)
	switch m.Kind {
	case KindPair1:
		w.onPair1(m, v, err)
	case KindPair2:
		w.onPair2(m, v, err)
	case KindCmd:
		w.onCmd(m, v, err)
	default:
		w.l.logRate("kind", "связь с телефоном: неизвестное сообщение отброшено")
	}
}

func (w *worker) onCmd(m Msg, v any, perr error) {
	if m.ID == "" || m.From == "" {
		return
	}
	st := w.h.LoadState()
	now := w.now()
	var s Signed
	if perr == nil {
		s = *v.(*Signed)
	}
	cmd, code := Command{}, ErrCodeBadSig
	if perr == nil {
		cmd, _, code = CheckCommand(st, s, m.From, now)
	}
	lastSeq := st.LastSeq
	var msg string
	if code == "" {
		msg, code, lastSeq = w.h.ApplyCommand(w.r.gen, cmd.Seq, cmd.Child, cmd.Delta)
	}
	if w.stopped() {
		return // старое поколение: не отвечаем
	}
	var hash string
	if data, err := unb64(s.Data); err == nil && s.Data != "" {
		hash = HashData(data)
	}
	res := CmdResult{V: ProtoVersion, T: TypeCmdR, PC: st.PCID, Phone: m.From, Seq: cmd.Seq, Cmd: hash, TS: now.UnixMilli(), OK: code == ""}
	if res.OK {
		res.Message = msg
		if rep, ok := w.buildReport(); ok {
			res.Report = &rep
		}
		w.reportDue = true
	} else {
		res.Error = code
		res.LastSeq = &lastSeq
		w.l.logRate("cmd-"+code, "связь с телефоном: команда отклонена (%s)", code)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		return
	}
	signed, err := Sign(w.key, raw)
	if err != nil {
		return
	}
	if err := w.call(func(ctx context.Context, c *Client) error { return c.Reply(ctx, m.From, m.ID, KindCmdR, signed) }); err != nil {
		w.l.logRate("cmdr", "связь с телефоном: ответ на команду не отправлен: %v", err)
	}
}

// ---------- отчёт (протокол, раздел 6) ----------

func (w *worker) buildReport() (Report, bool) {
	st := w.h.LoadState()
	if st.Phone == nil {
		return Report{}, false
	}
	ch := w.h.Children()
	if ch == nil {
		ch = []ChildReport{}
	}
	return Report{V: ProtoVersion, T: TypeReport, PC: st.PCID, Phone: st.Phone.ID, TS: w.now().UnixMilli(), Children: ch}, true
}

func (w *worker) sendReport() {
	w.lastReport, w.reportDue = w.now(), false // сбой — следующая попытка по расписанию
	rep, ok := w.buildReport()
	if !ok {
		return
	}
	raw, err := json.Marshal(rep)
	if err != nil {
		return
	}
	s, err := Sign(w.key, raw)
	if err != nil {
		return
	}
	err = w.call(func(ctx context.Context, c *Client) error { return c.SendReport(ctx, s) })
	w.l.logChange("report", err, "связь с телефоном: отчёт не отправлен: %v", err)
}
