package service

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/remote"
)

// Связь с телефоном (M7, FR-40, docs/07_Протокол_связи.md). Сеть — только в internal/remote;
// здесь состояние в settings, порт remote.Host и команды блока «Телефон» окна родителя.
//
// Блокировки (план M7, «Без взаимоблокировок»): обработчики держат s.mu и зовут только
// неблокирующие методы Link (Start, Stop, StartPairing…); методы remoteHost сами берут s.mu;
// ждать горутины связи (Link.Wait) — только вне s.mu (reapLink, Shutdown).

const (
	settingRemote    = "remote.state" // одна запись JSON: remote.State + enabled, url (PL-55)
	extraSourcePhone = "phone"        // подробность source события EvExtraTime с телефона
	remoteReportEach = 60 * time.Second
	remoteStopWait   = 5 * time.Second // остановка службы ждёт горутины связи не дольше
	remoteReapWait   = time.Minute     // остановленная связь: горутины должны закончиться за это время
)

// Действия события evRemote (подробность action).
const (
	remoteActOn       = "on"
	remoteActOff      = "off"
	remoteActPaired   = remote.EventPaired
	remoteActUnpaired = remote.EventUnpaired
	remoteActLost     = remote.EventLost
)

// Тексты блока «Телефон» (родителю на «вы»).
const (
	textRemoteURL     = "Адрес должен начинаться с https://" // его же ждёт мок окна (REMOTE_URL_ERROR)
	textRemoteOff     = "Чтобы подключить телефон, включите связь с телефоном"
	textRemoteNoState = "Не удалось прочитать настройки связи. Попробуйте ещё раз"
)

// remoteStored — запись settingRemote: состояние связи целиком.
type remoteStored struct {
	remote.State
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

// remoteState — связь с телефоном в памяти службы (под s.mu).
type remoteState struct {
	loaded bool         // запись прочитана (или её нет): st — истина
	st     remoteStored // копия записи в базе; пишется только целиком (writeRemote)
	link   *remote.Link // nil — связь выключена: горутин нет, соединений нет (S56)
	// unpairing — телефон, который родитель отвязал при включённой связи: горутина связи ещё не
	// записала это (Link.Unpair), окно его уже не показывает, команды с него не применяются.
	unpairing string
	lost      bool   // сервер не узнал компьютер, телефон отвязан: причина в окне до новой привязки
	code      string // последний показанный код привязки: окно держит его, пока телефон не прислал pair2
	codeExp   int64
	wg        sync.WaitGroup // reapLink: ожидание горутин остановленных связей

	// Крючки тестов: задаются до запуска связи и дальше не меняются.
	beforeApplyLock func() // ApplyCommand до s.mu
	beforeStop      func() // stopLink под s.mu перед Link.Stop
}

// remoteHost — порт remote.Host службы. Каждый метод сам берёт s.mu.
type remoteHost struct{ s *Service }

var _ remote.Host = remoteHost{}

// loadRemote читает запись связи, если ещё не прочитана. false — база не ответила (R18): связь не
// запускается, окно получает отказ, следующая попытка — при следующей команде.
func (s *Service) loadRemote() bool {
	if s.rem.loaded {
		return true
	}
	v, ok, err := s.cfg.Store.GetSetting(settingRemote)
	if err != nil {
		s.logRate("remote-load", "связь с телефоном: настройки не прочитаны: %v", err)
		return false
	}
	var st remoteStored
	if ok && v != "" {
		if err := json.Unmarshal([]byte(v), &st); err != nil {
			// Запись испорчена: связь выключена, ключ и привязка забыты (телефон подключают заново).
			s.cfg.Logf("связь с телефоном: запись настроек не разобрана, связь выключена: %v", err)
			st = remoteStored{}
		}
	}
	s.rem.st, s.rem.loaded = st, true
	return true
}

// writeRemote пишет запись связи целиком и, если удалось, делает её текущей.
func (s *Service) writeRemote(st remoteStored) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if err := s.cfg.Store.SetSetting(settingRemote, string(raw)); err != nil {
		return err
	}
	s.rem.st = st
	return nil
}

func (s *Service) remoteURL() string {
	if u := strings.TrimSpace(s.rem.st.URL); u != "" {
		return u
	}
	return ipc.RemoteDefaultURL
}

// startRemote — при запуске службы: связь включена в настройках — запустить.
func (s *Service) startRemote() {
	if !s.loadRemote() || !s.rem.st.Enabled {
		return
	}
	s.startLink()
}

// startLink запускает новую связь по адресу из настроек (под s.mu, в сеть не ходит). Часы связи —
// настоящие (RealNow): часы ядра в тестовом режиме ускорены и стоят в октябре 2026.
func (s *Service) startLink() {
	u, err := remote.ValidateURL(s.remoteURL(), s.cfg.Test.Enabled)
	if err != nil {
		s.logRate("remote-url", "связь с телефоном: адрес сервера не подходит, связь не запущена")
		return
	}
	l := remote.NewLink(remote.Config{URL: u, Host: remoteHost{s}, Now: s.cfg.RealNow, ReportEvery: remoteReportEach})
	s.rem.link = l
	s.rem.code, s.rem.codeExp = "", 0
	l.Start()
}

// stopLink останавливает связь (под s.mu): Stop не ждёт горутины, их ждёт reapLink вне s.mu.
// Отвязка, которую связь не успела записать, записывается здесь.
func (s *Service) stopLink() {
	l := s.rem.link
	if l == nil {
		return
	}
	if s.rem.beforeStop != nil {
		s.rem.beforeStop()
	}
	l.Stop()
	s.rem.link = nil
	s.rem.code, s.rem.codeExp = "", 0
	s.rem.wg.Add(1)
	go s.reapLink(l)
	if id := s.rem.unpairing; id != "" {
		s.rem.unpairing = ""
		if p := s.rem.st.Phone; p != nil && p.ID == id {
			s.unpairLocal()
		}
	}
}

// reapLink ждёт горутины остановленной связи вне s.mu.
func (s *Service) reapLink(l *remote.Link) {
	defer s.rem.wg.Done()
	if !l.Wait(remoteReapWait) {
		s.cfg.Logf("связь с телефоном: горутины остановленной связи не закончились за %s", remoteReapWait)
	}
}

// waitRemote ждёт горутины всех остановленных связей не дольше d (вне s.mu).
func (s *Service) waitRemote(d time.Duration) bool {
	done := make(chan struct{})
	go func() { s.rem.wg.Wait(); close(done) }()
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-done:
		return true
	case <-t.C:
		return false
	}
}

// unpairLocal стирает телефон в записи и пишет журнал (связь выключена или остановлена).
func (s *Service) unpairLocal() {
	st := s.rem.st
	if st.Phone == nil {
		return
	}
	name := st.Phone.Name
	st.Phone, st.LastSeq = nil, 0
	if err := s.writeRemote(st); err != nil {
		s.logRate("remote-unpair", "связь с телефоном: отвязка не записана: %v", err)
		return
	}
	s.rem.lost = false
	s.journal.Write(0, evRemote, map[string]any{"action": remoteActUnpaired, "phone": name})
}

// remoteChanged — отчёт телефону вне очереди после изменения в окне (только если связь включена).
func (s *Service) remoteChanged() {
	if s.rem.link != nil {
		s.rem.link.Changed()
	}
}

// remoteEventText — строка журнала события evRemote (eventText делает первую букву заглавной).
func remoteEventText(action, phone string) string {
	switch action {
	case remoteActOn:
		return "связь с телефоном включена"
	case remoteActOff:
		return "связь с телефоном выключена"
	case remoteActPaired:
		if phone == "" {
			return "телефон подключён"
		}
		return "телефон подключён: " + phone
	case remoteActUnpaired:
		return "телефон отвязан"
	case remoteActLost:
		return "телефон отвязан: сервер связи не узнал компьютер"
	}
	return "связь с телефоном"
}

// --- remote.Host ----------------------------------------------------------------------------

func (h remoteHost) LoadState() remote.State {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rem.st.State
}

// SaveState пишет состояние связи целиком; поколение не текущее — ErrStale, ничего не записано.
func (h remoteHost) SaveState(gen int64, st remote.State) error {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.remoteGen(gen) {
		return remote.ErrStale
	}
	rec := s.rem.st
	rec.State = st
	return s.writeRemote(rec)
}

// remoteGen — gen это идущее поколение связи (под s.mu).
func (s *Service) remoteGen(gen int64) bool {
	return !s.stopping && s.rem.link != nil && gen != 0 && gen == s.rem.link.Gen()
}

// ApplyCommand — команда телефона под одной s.mu: поколение → seq → запись seq → доп. время тем же
// путём, что кнопки окна (changeExtra, источник «телефон»). Команда применяется не больше одного
// раза: seq записан раньше доп. времени, после перезапуска повтор отсекается.
func (h remoteHost) ApplyCommand(gen, seq, childID int64, delta int) (string, string, int64) {
	s := h.s
	if s.rem.beforeApplyLock != nil {
		s.rem.beforeApplyLock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.rem.st
	if !s.remoteGen(gen) {
		return "", remote.ErrCodeStale, st.LastSeq
	}
	if st.Phone == nil || st.Phone.ID == s.rem.unpairing {
		return "", remote.ErrCodeNotPaired, st.LastSeq
	}
	if seq <= st.LastSeq {
		return "", remote.ErrCodeReplay, st.LastSeq
	}
	next := st
	next.LastSeq = seq
	if err := s.writeRemote(next); err != nil {
		// Номер не записан — команду не применяем: после перезапуска она прошла бы второй раз.
		s.logRate("remote-seq", "связь с телефоном: номер команды не записан, команда не применена: %v", err)
		return "", remote.ErrCodeStale, st.LastSeq
	}
	cs := s.children[childID]
	if cs == nil {
		return "", remote.ErrCodeNoChild, seq
	}
	if delta == 0 || delta > core.MaxExtraMin || delta < -core.MaxExtraMin {
		return "", remote.ErrCodeRange, seq
	}
	s.real = s.cfg.RealNow()
	rep, ok := s.changeExtra(cs, delta, extraFromPhone, s.real)
	if !ok {
		return "", remote.ErrCodeRange, seq
	}
	return rep.Message, "", seq
}

// Children — отчёт телефону: «осталось» и «за компьютером» той же карточкой, что окно родителя.
func (h remoteHost) Children() []remote.ChildReport {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []remote.ChildReport{}
	for _, id := range s.childOrder() {
		c := s.cardOf(s.children[id])
		left := int64(c.Status.RemainingS)
		if c.Status.Unlimited {
			left = -1 // без лимита сегодня (протокол, раздел 6)
		}
		out = append(out, remote.ChildReport{
			ID: c.ChildID, Name: c.Name, UsedSec: int64(c.Status.UsedS), LeftSec: left,
			ExtraMin: c.ExtraMin, Blocked: c.Status.Lock != "" && c.Status.Lock != ipc.LockNone,
		})
	}
	return out
}

// Event — запись журнала родителя без ребёнка: телефон подключён, отвязан, потерян.
func (h remoteHost) Event(kind, detail string) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	switch kind {
	case remote.EventPaired:
		s.rem.lost, s.rem.unpairing = false, ""
	case remote.EventUnpaired:
		s.rem.unpairing = ""
	case remote.EventLost:
		s.rem.lost, s.rem.unpairing = true, ""
	default:
		return
	}
	s.journal.Write(0, evRemote, map[string]any{"action": kind, "phone": detail})
}

// Logf — журнал работы службы без второго ограничителя: повторы связь ограничивает сама (строка о
// связи — при смене состояния, та же ошибка — не чаще раза в час, PL-53).
func (h remoteHost) Logf(format string, args ...any) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Logf(format, args...)
}

// --- команды окна ---------------------------------------------------------------------------

func (s *Service) hGetRemote(c *callCtx) ipc.Reply {
	if !s.loadRemote() {
		return ipc.Fail(textRemoteNoState)
	}
	return ipc.OK(s.remoteView())
}

// hSetRemote — выключатель «Связь с телефоном» и «Адрес сервера»: пустой адрес — адрес по
// умолчанию; включение, выключение и смена адреса — старая связь остановлена, новая запущена.
func (s *Service) hSetRemote(c *callCtx) ipc.Reply {
	var req ipc.SetRemoteRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	raw := strings.TrimSpace(req.URL)
	if raw == "" {
		raw = ipc.RemoteDefaultURL
	}
	u, err := remote.ValidateURL(raw, s.cfg.Test.Enabled)
	if err != nil {
		return ipc.FailField(textRemoteURL, ipc.FieldURL)
	}
	if !s.loadRemote() {
		return ipc.Fail(textRemoteNoState)
	}
	url := u.String()
	was, oldURL := s.rem.st.Enabled, s.remoteURL()
	st := s.rem.st
	st.Enabled, st.URL = req.Enabled, url
	if err := s.writeRemote(st); err != nil {
		s.logRate("remote-save", "связь с телефоном: настройки не записаны: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	switch {
	case !req.Enabled:
		s.stopLink()
		s.rem.lost = false
	case s.rem.link == nil || url != oldURL:
		s.stopLink()
		s.startLink()
	}
	if was != req.Enabled {
		act := remoteActOff
		if req.Enabled {
			act = remoteActOn
		}
		s.journal.Write(0, evRemote, map[string]any{"action": act})
	}
	return ipc.OK(s.remoteView())
}

func (s *Service) hRemotePairStart(c *callCtx) ipc.Reply {
	if !s.loadRemote() {
		return ipc.Fail(textRemoteNoState)
	}
	if s.rem.link == nil {
		return ipc.Fail(textRemoteOff)
	}
	s.rem.code, s.rem.codeExp = "", 0
	s.rem.link.StartPairing()
	return ipc.OK(s.remoteView())
}

func (s *Service) hRemotePairCancel(c *callCtx) ipc.Reply {
	if !s.loadRemote() {
		return ipc.Fail(textRemoteNoState)
	}
	if s.rem.link != nil {
		s.rem.link.CancelPairing()
	}
	return ipc.OK(s.remoteView())
}

func (s *Service) hRemotePairConfirm(c *callCtx) ipc.Reply {
	var req ipc.RemotePairConfirmRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if !s.loadRemote() {
		return ipc.Fail(textRemoteNoState)
	}
	if s.rem.link != nil {
		s.rem.link.ConfirmPairing(req.Match)
	}
	return ipc.OK(s.remoteView())
}

// hRemoteUnpair — «Отвязать»: связь включена — Link.Unpair (запись, журнал и гашение токена
// телефона на сервере делает горутина связи; окно телефон уже не показывает); выключена — служба
// стирает телефон сама.
func (s *Service) hRemoteUnpair(c *callCtx) ipc.Reply {
	if !s.loadRemote() {
		return ipc.Fail(textRemoteNoState)
	}
	p := s.rem.st.Phone
	switch {
	case p == nil:
	case s.rem.link != nil:
		s.rem.unpairing = p.ID
		s.rem.link.Unpair()
	default:
		s.unpairLocal()
	}
	return ipc.OK(s.remoteView())
}

// remoteView — состояние блока «Телефон» для окна (под s.mu).
func (s *Service) remoteView() ipc.RemoteView {
	st := s.rem.st
	v := ipc.RemoteView{Enabled: st.Enabled, URL: s.remoteURL(), State: ipc.RemoteStateOff, Pair: ipc.RemotePair{Stage: ipc.PairIdle}}
	if p := st.Phone; p != nil && p.ID != s.rem.unpairing {
		v.Phone = &ipc.RemotePhone{Name: p.Name, PairedAtMs: p.PairedAtMs}
	}
	if !st.Enabled {
		return v
	}
	l := s.rem.link
	if l == nil { // включена, но адрес не подошёл (запись старой версии или испорчена)
		v.State, v.Error = ipc.RemoteStateOffline, ipc.RemoteErrBadAddress
		return v
	}
	ls := l.Status()
	v.LastOKMs = ls.LastOKMs
	switch {
	case ls.Online:
		v.State = ipc.RemoteStateOnline
	case ls.LastErr == "":
		v.State = ipc.RemoteStateConnecting
	default:
		v.State = ipc.RemoteStateOffline
		v.Error = remoteErrOf(ls.LastErr)
	}
	if v.Error == "" && s.rem.lost {
		v.Error = ipc.RemoteErrLost
	}
	v.Pair = s.pairView(l.PairInfo())
	return v
}

// remoteErrOf — код ошибки связи для окна.
func remoteErrOf(code string) string {
	switch code {
	case remote.StatusBadAddress, remote.StatusBadAnswer:
		return ipc.RemoteErrBadAddress
	case remote.StatusUnauthorized:
		return ipc.RemoteErrLost
	}
	return ipc.RemoteErrNoServer
}

// pairView переводит этапы привязки internal/remote в этапы окна: код запрашивается — starting;
// код показан и телефон прислал pair1 — code (окно держит код, пока не пришёл pair2); сверка —
// compare; «Совпадает» записан — idle (телефон в Phone); отменено — failed с причиной окна.
func (s *Service) pairView(p remote.PairInfo) ipc.RemotePair {
	out := ipc.RemotePair{Stage: ipc.PairIdle}
	switch p.Stage {
	case remote.StageCode:
		if p.Code == "" {
			out.Stage = ipc.PairStarting
			break
		}
		s.rem.code, s.rem.codeExp = p.Code, p.ExpiresMs
		out.Stage, out.Code, out.ExpiresMs = ipc.PairCode, p.Code, p.ExpiresMs
	case remote.StageWaitNonce:
		if s.rem.code == "" {
			out.Stage = ipc.PairStarting
			break
		}
		out.Stage, out.Code, out.ExpiresMs = ipc.PairCode, s.rem.code, s.rem.codeExp
	case remote.StageCompare:
		out.Stage, out.SAS, out.PhoneName, out.ExpiresMs = ipc.PairCompare, p.SAS, p.PhoneName, p.ExpiresMs
	case remote.StageFailed:
		out.Stage, out.Error = ipc.PairFailed, pairErrOf(p.Error)
		if p.Error == remote.PairErrStopped {
			out = ipc.RemotePair{Stage: ipc.PairIdle}
		}
	}
	if out.Stage != ipc.PairCode {
		s.rem.code, s.rem.codeExp = "", 0
	}
	return out
}

func pairErrOf(code string) string {
	switch code {
	case remote.PairErrExpired:
		return ipc.PairErrExpired
	case remote.PairErrMismatch:
		return ipc.PairErrMismatch
	case remote.PairErrNoServer:
		return ipc.PairErrNoServer
	}
	return ipc.PairErrAborted
}
