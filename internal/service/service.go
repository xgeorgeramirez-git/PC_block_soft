package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/programs"
	"pcblock/internal/sites"
)

// Сроки службы. Всё, что про людей и процессы, считается по настоящим часам; учёт по часам ядра.
const (
	sampleMaxAge    = 5 * time.Second  // снимок стража старше этого срока не засчитывается
	adminCheckEvery = 60 * time.Second // проверка R1 для каждого ребёнка
	braveCheckEvery = 60 * time.Second // сверка правил Brave в ветке реестра ребёнка (RM4-10)
	braveLogEvery   = 10 * time.Minute // «правила Brave восстановлены» в журнал родителя не чаще, на ребёнка
	rulesRetryEvery = 60 * time.Second // повторное чтение правил, которые не удалось прочитать (R18)
	errLogEvery     = 60 * time.Second // одна и та же ошибка пишется в журнал работы не чаще
	// otherUserLogEvery — строка о процессах другого пользователя в сеансе ребёнка пишется в журнал
	// работы не чаще и только при новом числе процессов (logCountRate).
	otherUserLogEvery = time.Hour
	parentIdle        = 10 * time.Minute // родительская сессия заканчивается без действий (R19)
	guardBindLife     = 30 * time.Second // окно после верного пароля на экране блокировки
	undoLife          = 10 * time.Second // «Отменить» после добавления времени
	warnLife          = 20 * time.Second // непринятое предупреждение стареет
	bypassLogWindow   = 60 * time.Second // одинаковая попытка обхода пишется в журнал не чаще (D-41)
	bypassGrace       = 5 * time.Second  // гибель стража, значка или окна записывается обходом не раньше (FB-1)
	closingLife       = 30 * time.Second // сообщение стража «закрываюсь» действует столько, если страж жив
)

// Ключи настроек в хранилище.
const (
	settingDBNotice       = "notice.db_recovered"
	settingFirstRunPrefix = "firstrun.seen."
	settingVerboseLogs    = "diag.verbose"      // «Подробные логи для диагностики» (FR-25, D-42)
	settingBonusRate      = "bonus.rate_per_10" // автобонус N: минут за каждые 10 мин остатка (R25, D-49)
	// settingAutobonusFrom — первый день, за который служба начисляет пропущенный автобонус (D-82):
	// день первого запуска 0.5.0 по поясу службы; дни до него прожиты на прежней версии.
	settingAutobonusFrom = "bonus.autobonus_from_day"
)

// SessionEvent — событие сеанса Windows, которое диспетчер служб передаёт службе.
type SessionEvent int

// События сеанса. Любое из них заставляет службу сверить список сеансов; вход, кроме того,
// запускает проверку R1 и ставит политику Диспетчера задач.
const (
	SessionLogon SessionEvent = iota
	SessionLogoff
	SessionConnect
	SessionDisconnect
	SessionLock
	SessionUnlock
)

func (e SessionEvent) String() string {
	names := [...]string{"вход", "выход", "подключение", "отключение", "блокировка", "разблокировка"}
	if e >= 0 && int(e) < len(names) {
		return names[e]
	}
	return fmt.Sprint("событие ", int(e))
}

// SessionUnknown — номер сеанса в событии, когда служба не смогла прочитать его из уведомления
// диспетчера служб. Такое событие не относится ни к какому сеансу: служба сверяет весь список.
const SessionUnknown uint32 = 0xFFFFFFFF

// Config — всё, что нужно службе. Store, Hasher, Launcher и Clock обязательны.
type Config struct {
	Store    Store
	Hasher   Hasher
	Launcher SessionLauncher
	Admins   AdminChecker     // nil: никто не администратор
	Accounts AccountDirectory // nil: список учёток пуст
	Policy   Policy           // nil: политика не ставится
	Journal  Journal          // nil: события пишутся в Store
	Clock    core.Clock       // часы ядра
	Loc      *time.Location   // местный пояс; nil: time.Local
	Version  string
	DataDir  string // папка данных: здесь лежит память о запущенных процессах
	Test     TestMode
	// Recovered и BrokenCopy: база была испорчена и заменена новой (OpenStore), служба
	// стартует в безопасном режиме и предупреждает родителя (R18, Ruling 25).
	Recovered  bool
	BrokenCopy string
	Rand       func() string // случайная строка для токенов; nil: crypto/rand
	// RealNow даёт настоящее время (не часы ядра): возраст снимка стража, паузы, сроки сессий.
	// nil: time.Now.
	RealNow func() time.Time
	Logf    func(format string, args ...any)
	// OpenFolder открывает папку в проводнике в сеансе sessionID, то есть там, где сидит родитель
	// (служба работает в служебном сеансе и свой проводник никому бы не показала): FR-25, D-48,
	// «Открыть папку с логами». nil: ничего не делает (тесты; в тестовом режиме не вызывается).
	OpenFolder func(sessionID uint32, path string) error
	// Программы (M3, RM3-1, RM3-10): сканер Program Files для снимка и мастера, свойства файлов и
	// корни компьютера (нормализованные programs.Normalize). Scanner nil: снимка нет, список
	// программ не готов, наблюдатель ничего не закрывает. FileMeta nil: издатель и отпечаток не
	// известны.
	Scanner  Scanner
	FileMeta FileMeta
	Roots    programs.Roots
	// Watcher — процессы сеансов для наблюдателя (R12, R13, R5, R8, NFR-6): закрывает незнакомое,
	// запрещённое и то, что сейчас нельзя. nil: наблюдение выключено, ничего не закрывается.
	Watcher ProcessWatcher
	// Adult открывает список сайтов «18+» (M4, R14, D-67). nil: встроенный в программу
	// (sites.Embedded).
	Adult func() (sites.AdultList, error)
}

// pendingWarning — предупреждение, ещё не доставленное стражу.
type pendingWarning struct {
	w  ipc.GuardWarning
	at time.Time
}

// undoRec — последнее добавленное время, которое ещё можно отменить.
type undoRec struct {
	id         string
	minutes    int    // минуты в заголовке операции (для копилки и журнала)
	extraDelta int    // знаковое изменение ExtraMin, которое отменяет UndoExtra (R10, D-43)
	undoMsg    string // сообщение-подтверждение при отмене
	day        string
	until      time.Time
}

// childState — ребёнок в памяти службы: ядро, правила, снимок датчиков.
type childState struct {
	child         Child
	eng           *core.Engine
	rules         core.Rules
	admin         bool // R1: учётка стала администратором
	adminAt       time.Time
	rulesAt       time.Time
	sample        ipc.SensorSample
	sampleAt      time.Time
	sampleVerdict string // последнее решение для журнала: снимок стража учитывается или нет
	sampleSess    uint32 // сеанс, из которого пришёл снимок: время идёт, только пока он на экране (FB-3)
	haveSample    bool
	warns         []pendingWarning
	undo          *undoRec
	bonusUndo     *undoRec             // отмена последнего начисления копилки на 10 с (FR-22)
	lastBypass    string               // последняя записанная попытка обхода (дедуп журнала, D-41)
	lastBypassAt  time.Time            // когда записана; страж повторяет сообщение каждую секунду
	usage         map[progUsageKey]int // учёт по программам с прошлого сохранения (RM3-6); пишется в saveState
	unreadSince   time.Time            // с какого момента подряд адрес Brave не читается (RM4-1, S-3); нулевой — читается
	noAddrSince   time.Time            // с какого момента подряд у окна Brave нет адреса (не сбой UIA, B-55); нулевой — есть или сбой
}

// Service — служба PCBlock без привязки к Windows: логика на портах (doc.go).
type Service struct {
	cfg     Config
	journal Journal
	mu      sync.Mutex

	real     time.Time // последний переданный настоящий момент
	children map[int64]*childState
	sessions map[uint32]*sessState
	parents  map[string]*parentSession
	binds    map[uint32]*guardBind
	guardCmd map[uint32][]ipc.GuardCommand
	guardUI  map[uint32]*guardView

	// testExits — выходы с экрана блокировки, которые в тестовом режиме запрошены, но не
	// выполнены (Exit*, по порядку); читаются через GuardState.
	testExits []string

	names map[int64]string // имена всех известных детей, в том числе убранных: для журнала

	clockPaused bool
	stopping    bool
	started     bool
	loaded      bool // список детей прочитан из базы
	verbose     bool // «Подробные логи для диагностики» включены (FR-25, D-42)
	errAt       map[string]time.Time
	logCount    map[string]int // последнее записанное число по ключу logCountRate

	// Программы (M3): классификатор в памяти (RM3-11), готов ли список (снимок сделан и прочитан,
	// RM3-10), когда пробовали в последний раз, последняя операция для «Отменить».
	classifier *programs.Classifier
	progReady  bool
	progTryAt  time.Time
	progUndo   *progUndoRec
	scan       *scanFlight    // идущее сканирование программ (вне s.mu, Д-2); nil — не идёт
	seeding    bool           // горутина снимка программ работает (не больше одной)
	seedWG     sync.WaitGroup // горутина снимка (ждут только тесты)

	// Наблюдатель процессов (M3, watch.go): память о процессах (своя блокировка, не s.mu) и
	// ограничитель повторов журнала и плашек (под s.mu).
	watch   watchState
	watchAt map[string]time.Time
	// autoClosed — какие запрещённые программы уже записаны в журнал как «запускалась сама» в
	// текущий день учёта ребёнка (R13, D-65: одна запись в день на программу). Только в памяти.
	autoClosed map[int64]*autoClosedDay
	launchWait []launchPending   // запрещённые, закрытые как «сама», ждут уточнения (settleLaunches)
	fpSeen     map[string]string // отпечатки файлов, которые прочитал наблюдатель: путь → отпечаток (для категории секунды)
	pubSeen    map[string]string // издатели файлов в профиле, которые прочитал наблюдатель (RM3-13)

	bypassWait []pendingBypass // гибель стража, значка или окна, о которой ещё не решено, обход ли это (FB-1)
	logoffs    []logoffMark    // недавние выходы из учёток: гибель рядом с ними в отключённом сеансе ребёнка не обход (FB-5)

	// Сайты (M4, sites_svc.go): свои списки родителя в памяти, прочитаны ли они в последний раз,
	// когда пробовали; список «18+»; правила Brave надо переписать (braveDue, T5); в какой день
	// ребёнку уже записано «не удалось прочитать адрес» (unreadDay, раз в день, RM4-1).
	siteLists   sites.Lists
	siteListsOK bool
	sitesAt     time.Time
	adult       sites.AdultList
	braveDue    bool
	unreadDay   map[int64]string

	// Правила Brave (T5, brave.go): домены «Запрещённых» из последнего удачного чтения списков и
	// было ли оно (до него правила не пишутся, T5-5); когда сверяли; когда последний раз писали
	// родителю о восстановлении (по ребёнку, T5-7).
	braveBlocked []string
	braveReady   bool
	braveAt      time.Time
	braveLogAt   map[int64]time.Time
}

// New собирает службу. Ничего не запускает: Start читает базу и поднимает сеансные части.
func New(cfg Config) (*Service, error) {
	switch {
	case cfg.Store == nil:
		return nil, errors.New("service: не задано хранилище")
	case cfg.Hasher == nil:
		return nil, errors.New("service: не задан хеш пароля")
	case cfg.Launcher == nil:
		return nil, errors.New("service: не задан запуск процессов")
	case cfg.Clock == nil:
		return nil, errors.New("service: не заданы часы")
	}
	if cfg.Loc == nil {
		cfg.Loc = time.Local
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.Admins == nil {
		cfg.Admins = noAdmins{}
	}
	if cfg.Accounts == nil {
		cfg.Accounts = noAccounts{}
	}
	if cfg.Policy == nil {
		cfg.Policy = noPolicy{}
	}
	if cfg.Rand == nil {
		cfg.Rand = randomToken
	}
	if cfg.RealNow == nil {
		cfg.RealNow = time.Now
	}
	if cfg.OpenFolder == nil {
		cfg.OpenFolder = func(uint32, string) error { return nil }
	}
	if cfg.FileMeta == nil {
		cfg.FileMeta = func(string) (string, string, string) { return "", "", "" }
	}
	s := &Service{
		cfg: cfg, journal: cfg.Journal,
		children: map[int64]*childState{}, sessions: map[uint32]*sessState{},
		parents: map[string]*parentSession{}, binds: map[uint32]*guardBind{},
		guardCmd: map[uint32][]ipc.GuardCommand{}, guardUI: map[uint32]*guardView{},
		errAt: map[string]time.Time{}, logCount: map[string]int{}, names: map[int64]string{},
		classifier: programs.NewClassifier(cfg.Roots, programs.DefaultCatalog),
	}
	if s.journal == nil {
		s.journal = StoreJournal{Store: cfg.Store, Clock: cfg.Clock, Loc: cfg.Loc, Logf: cfg.Logf}
	}
	return s, nil
}

type noAdmins struct{}

func (noAdmins) IsAdmin(string) (bool, error) { return false, nil }

type noAccounts struct{}

func (noAccounts) List() ([]AccountInfo, error) { return []AccountInfo{}, nil }

type noPolicy struct{}

func (noPolicy) SetTaskMgrDisabled(string, bool) error         { return nil }
func (noPolicy) SetBraveRules(string, BraveRules) error        { return nil }
func (noPolicy) BraveRulesOK(string, BraveRules) (bool, error) { return true, nil }
func (noPolicy) RemoveBraveRules(string) error                 { return nil }

// randomToken возвращает 32 случайных байта в шестнадцатеричной записи.
func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Источник случайности системы недоступен: пустая строка не станет токеном,
		// parent.go отказывает во входе.
		return ""
	}
	return hex.EncodeToString(b[:])
}

// --- запуск и остановка ----------------------------------------------------------------

// Start читает базу, поднимает ядро каждого ребёнка и запускает значок и стража в сеансах.
// real: настоящий момент. Ошибки базы не мешают службе работать (R18): они идут в журнал
// работы, а чтение повторяется каждую минуту.
func (s *Service) Start(real time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.real = real
	if s.cfg.Test.Enabled {
		s.cfg.Logf("Включён тестовый режим: ускорение часов %d, папка данных %q", s.cfg.Test.ClockSpeed, s.cfg.Test.DataDir)
	}
	s.killStale()
	if s.cfg.Recovered {
		s.startRecovered()
	}
	s.autobonusFrom() // свежая база и первый запуск после обновления: граница автобонуса (D-82)
	s.loadChildren(real)
	s.ensurePrograms(real)
	s.loadAdult()
	s.reloadSites(real)
	if v, ok, err := s.cfg.Store.GetSetting(settingVerboseLogs); err == nil && ok && v == "1" {
		s.verbose = true
		s.cfg.Logf("Подробные логи для диагностики включены (FR-25, D-42)")
	}
	for _, id := range s.childOrder() {
		cs := s.children[id]
		s.checkAdmin(cs, real)
		s.applyPolicy(cs.child.SID, true)
		s.applyBrave(cs)
	}
	s.braveAt = real
	s.started = true
	s.syncSessions(real)
	return nil
}

// startRecovered пишет событие и запоминает предупреждение родителю после замены испорченной базы.
func (s *Service) startRecovered() {
	s.journal.Write(0, evDBRecovered, map[string]any{
		"copy": s.cfg.BrokenCopy,
		"text": textRecoveredNotice,
	})
	if err := s.cfg.Store.SetSetting(settingDBNotice, textRecoveredNotice); err != nil {
		s.logRate("dbnotice", "не записано предупреждение о повреждённой базе: %v", err)
	}
	s.cfg.Logf("База данных повреждена и заменена новой, копия: %s. Ограничения не применяются", s.cfg.BrokenCopy)
}

// Shutdown сохраняет состояние, завершает значки, стражей и окна во всех сеансах (Ruling 15)
// и не даёт тикам дальше менять данные. Базу закрывает тот, кто её открыл.
func (s *Service) Shutdown(real time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.real = real
	if s.stopping {
		return
	}
	s.stopping = true
	// Остановка службы это выключение или перезагрузка компьютера: то, что успело погибнуть
	// перед ней, не обход (FB-1).
	s.cancelAllBypass("остановка службы")
	for _, id := range s.childOrder() {
		s.saveDay(s.children[id])
	}
	s.stopAllProcs()
}

// --- дети ------------------------------------------------------------------------------

func (s *Service) today() string { return core.DayOf(s.cfg.Clock.Now().In(s.cfg.Loc)) }

func (s *Service) childOrder() []int64 {
	ids := make([]int64, 0, len(s.children))
	for id := range s.children {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (s *Service) childBySID(sid string) *childState {
	if sid == "" {
		return nil
	}
	for _, id := range s.childOrder() {
		if s.children[id].child.SID == sid {
			return s.children[id]
		}
	}
	return nil
}

// loadChildren читает детей из базы и заводит ядро тем, кого ещё нет в памяти.
func (s *Service) loadChildren(real time.Time) {
	list, err := s.cfg.Store.ListChildren()
	if err != nil {
		s.logRate("children", "не удалось прочитать список детей: %v", err)
		return
	}
	s.loaded = true
	for _, c := range list {
		if _, ok := s.children[c.ID]; !ok {
			s.addChild(c, real)
		}
	}
}

// addChild заводит ядро ребёнка: правила, состояние сегодняшнего дня и сохранённая блокировка.
func (s *Service) addChild(c Child, real time.Time) *childState {
	rules := s.readRules(c.ID)
	st, err := s.cfg.Store.LoadDayState(c.ID, s.today())
	if err != nil {
		s.logRate(fmt.Sprint("load", c.ID), "не удалось прочитать учёт дня для %s: %v", c.Name, err)
		st = core.DayState{Day: s.today()}
	}
	cs := &childState{
		child: c, rules: rules, adminAt: real, rulesAt: real,
		eng: core.NewEngine(s.cfg.Clock, s.cfg.Loc, rules, st, c.BypassDetail),
	}
	// Счёт к перерыву не по дням (R27): идущий перерыв продолжается по часам после перезапуска.
	if bs, err := s.cfg.Store.LoadBreakState(c.ID); err != nil {
		s.logRate(fmt.Sprint("loadbreak", c.ID), "не удалось прочитать счёт к перерыву для %s: %v", c.Name, err)
	} else {
		cs.eng.RestoreBreak(bs)
	}
	s.children[c.ID] = cs
	s.names[c.ID] = c.Name
	s.catchUpBonus(cs)
	return cs
}

// autobonusFrom — первый день, за который начисляется пропущенный автобонус (D-82): ключ
// settingAutobonusFrom; нет ключа (свежая база или первый запуск после обновления) — сегодняшний
// день по часам и поясу службы записывается и возвращается. Сбой чтения — ok = false: дни ждут
// следующего запуска (R18); сбой записи — граница сегодня (до неё начислять нечего), ключ
// запишется при следующем запуске.
func (s *Service) autobonusFrom() (day string, ok bool) {
	v, found, err := s.cfg.Store.GetSetting(settingAutobonusFrom)
	if err != nil {
		s.logRate("autobonusfrom", "автобонус: не прочитана граница пропущенных дней, повтор при следующем запуске: %v", err)
		return "", false
	}
	if found {
		if t, err := time.Parse(time.DateOnly, v); err == nil && t.Format(time.DateOnly) == v {
			return v, true
		}
	}
	today := s.today()
	if err := s.cfg.Store.SetSetting(settingAutobonusFrom, today); err != nil {
		s.logRate("autobonusfrom2", "автобонус: не записана граница пропущенных дней: %v", err)
	}
	return today, true
}

// catchUpBonus закрывает прошлые дни, которые ядро не закрыло (FB-21, D-82, R25; B-42): компьютер
// выключили до полуночи, и сутки сменились без работающей службы. Сначала итог дня дотягивается
// до полуночи по последней минуте (PastDayAtMidnight), затем начисляется автобонус. Дни — строки базы с
// bonus_granted = 0 строго до сегодня (сегодняшний и вчерашний, если ядро в нём, закрывает ядро
// через ClosedDay; пометка в строке не даёт начислить дважды). Дни раньше autobonusFrom прожиты
// на прежней версии: помечаются рассмотренными без бонуса (задним числом не начисляется, D-82).
// Правила не прочитаны (безопасный режим) — лимиты неизвестны, дни ждут следующего запуска; сбой
// чтения — то же (R18).
func (s *Service) catchUpBonus(cs *childState) {
	if cs.rules.Broken {
		return
	}
	from, ok := s.autobonusFrom()
	if !ok {
		return
	}
	days, err := s.cfg.Store.UngrantedDays(cs.child.ID, s.today())
	if err != nil {
		s.logRate(fmt.Sprint("catchup", cs.child.ID), "автобонус: не прочитаны прошлые дни у %s, повтор при следующем запуске: %v", cs.child.Name, err)
		return
	}
	for i := range days {
		d := &days[i]
		if d.Day < from {
			d.BonusGranted = true
			if err := s.cfg.Store.SaveDayState(cs.child.ID, *d); err != nil {
				s.logRate(fmt.Sprint("catchupold", cs.child.ID), "автобонус: не помечен день %s до установки у %s: %v", d.Day, cs.child.Name, err)
			}
			continue
		}
		// Сначала итог дня (B-42): последняя минута шла по часам до полуночи, пока службы не было;
		// бонус считается уже по дотянутому общему времени.
		pulled := d.LastMinuteEndMs != nil
		*d = cs.eng.PastDayAtMidnight(*d)
		if s.grantDailyBonus(cs, d) {
			continue // начислено и помечено одной транзакцией
		}
		if !d.BonusGranted && !pulled {
			continue // сбой копилки: день ждёт следующего запуска
		}
		if err := s.cfg.Store.SaveDayState(cs.child.ID, *d); err != nil {
			s.logRate(fmt.Sprint("catchupsave", cs.child.ID), "автобонус: не помечен день %s у %s: %v", d.Day, cs.child.Name, err)
		}
	}
}

// readRules читает правила ребёнка целиком: лимиты (общий и игр), расписание и режим «Когда
// время вышло». Любой путь, который применяет правила, берёт их отсюда (RM3-9: частичная сборка
// Rules теряет соседние поля). Не прочиталось хоть что-то: безопасный режим (R18).
func (s *Service) readRules(id int64) core.Rules {
	week, err := s.cfg.Store.GetDayLimits(id)
	if err != nil {
		s.logRate(fmt.Sprint("rules", id), "не удалось прочитать правила ребёнка %d, ограничения не применяются: %v", id, err)
		return core.Rules{Broken: true}
	}
	sched, err := s.cfg.Store.GetSchedule(id)
	if err != nil {
		s.logRate(fmt.Sprint("sched", id), "не удалось прочитать расписание ребёнка %d, ограничения не применяются: %v", id, err)
		return core.Rules{Broken: true}
	}
	mode, err := s.cfg.Store.GetEndMode(id)
	if err != nil {
		s.logRate(fmt.Sprint("endmode", id), "не удалось прочитать режим «Когда время вышло» ребёнка %d, ограничения не применяются: %v", id, err)
		return core.Rules{Broken: true}
	}
	brk, err := s.cfg.Store.GetBreakRules(id)
	if err != nil {
		s.logRate(fmt.Sprint("break", id), "не удалось прочитать настройки отдыха ребёнка %d, ограничения не применяются: %v", id, err)
		return core.Rules{Broken: true}
	}
	return core.Rules{Week: week, Schedule: sched, EndMode: core.EndModeFromString(mode), Break: brk}
}

// exempt сообщает, что для учётки сняты запрет A2 и правило R1 (тестовый режим, ТЗ 10.8).
func (s *Service) exempt(sid string) bool {
	return s.cfg.Test.Enabled && s.cfg.Test.TestChildSID != "" && s.cfg.Test.TestChildSID == sid
}

// checkAdmin сверяет R1: не стала ли детская учётка администратором. Изменение пишется в журнал.
func (s *Service) checkAdmin(cs *childState, real time.Time) {
	cs.adminAt = real
	if s.exempt(cs.child.SID) {
		return
	}
	admin, err := s.cfg.Admins.IsAdmin(cs.child.SID)
	if err != nil {
		s.logRate("admin"+cs.child.SID, "не удалось проверить права администратора у %s: %v", cs.child.Name, err)
		return
	}
	if admin == cs.admin {
		return
	}
	cs.admin = admin
	if admin {
		s.journal.Write(cs.child.ID, evChildAdmin, map[string]any{"text": "Учётка получила права администратора: ограничения не действуют"})
		s.cfg.Logf("Детская учётка %s стала администратором, ограничения сняты", cs.child.Name)
		return
	}
	s.journal.Write(cs.child.ID, evChildAdminOff, map[string]any{"text": "Права администратора сняты: ограничения снова действуют"})
}

func (s *Service) applyPolicy(sid string, disabled bool) {
	if err := s.cfg.Policy.SetTaskMgrDisabled(sid, disabled); err != nil {
		s.logRate("policy"+sid, "не удалось изменить запрет Диспетчера задач для %s: %v", sid, err)
	}
}

// --- сохранение и журнал ----------------------------------------------------------------

// saveDay сохраняет состояние дня ребёнка; ошибка не останавливает учёт (R18).
func (s *Service) saveDay(cs *childState) {
	st, _ := cs.eng.State()
	s.saveState(cs, st)
	// Счёт к перерыву — тем же сохранением (DueSave: раз в 30 с и сразу после плашки, начала и
	// конца перерыва, RF14-2).
	if err := s.cfg.Store.SaveBreakState(cs.child.ID, cs.eng.Break()); err != nil {
		s.logRate(fmt.Sprint("savebreak", cs.child.ID), "счёт к перерыву у %s не сохранён: %v", cs.child.Name, err)
	}
}

func (s *Service) saveState(cs *childState, st core.DayState) {
	if st.Day == "" {
		return
	}
	if len(cs.usage) == 0 {
		if err := s.cfg.Store.SaveDayState(cs.child.ID, st); err != nil {
			s.logRate(fmt.Sprint("save", cs.child.ID), "состояние дня %s у %s не сохранено: %v", st.Day, cs.child.Name, err)
		}
		return
	}
	// Учёт по программам — той же транзакцией (RM3-6). Каждая порция несёт свой день: сброс после
	// полуночи не переносит вчерашние секунды в сегодня (PL-15). Ошибка — накопитель остаётся.
	rows := make([]UsageRow, 0, len(cs.usage))
	for k, sec := range cs.usage {
		rows = append(rows, UsageRow{Day: k.day, Category: k.cat, Kind: k.kind, Subject: k.subject, Seconds: sec})
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Subject < b.Subject
	})
	if err := s.cfg.Store.SaveDayAndUsage(cs.child.ID, st, rows); err != nil {
		s.logRate(fmt.Sprint("save", cs.child.ID), "состояние дня %s и учёт программ у %s не сохранены: %v", st.Day, cs.child.Name, err)
		return
	}
	cs.usage = nil
}

// usageKindProgram — вид предмета учёта usage_daily: программа (путь exe).
const usageKindProgram = "program"

// afterDecision разбирает решение ядра: закрытый день, события, предупреждение.
func (s *Service) afterDecision(cs *childState, d core.Decision, extra map[string]any) {
	if d.ClosedDay != nil {
		s.grantDailyBonus(cs, d.ClosedDay)
		s.saveState(cs, *d.ClosedDay)
	}
	s.writeEvents(cs.child.ID, d.Events, extra)
	if d.Warning != 0 {
		// Текст предупреждения зависит от вида ближайшего события (R9, FR-9): исчерпание
		// времени, начало запретных часов или конец паузы.
		cs.warns = append(cs.warns, pendingWarning{w: ipc.NewGuardWarningFor(d.Warning, d.WarningEvent, d.WarningS), at: s.real})
	}
	if d.RestWarn {
		// «ПОРА ОТДОХНУТЬ» (R27, 8.2.2 п.7): ядро отдаёт её один раз на перерыв.
		cs.warns = append(cs.warns, pendingWarning{w: ipc.NewGuardRestWarning(d.RestWarnS, d.RestText), at: s.real})
	}
}

// writeEvents пишет события ядра в журнал; extra дополняет подробности каждого.
func (s *Service) writeEvents(childID int64, evs []core.Event, extra map[string]any) {
	for _, e := range evs {
		details := map[string]any{}
		for k, v := range e.Details {
			details[k] = v
		}
		for k, v := range extra {
			details[k] = v
		}
		//eventkind:dynamic core.Event.Kind: литералы событий ядра проверяет TestEventKindsSourceScan, путь — TestDynamicEventKindsFromCore
		s.journal.Write(childID, e.Kind, details)
	}
}

// logRate пишет строку в журнал работы не чаще раза в errLogEvery для одного ключа.
func (s *Service) logRate(key, format string, args ...any) {
	if last, ok := s.errAt[key]; ok && s.real.Sub(last) < errLogEvery {
		return
	}
	s.errAt[key] = s.real
	s.cfg.Logf(format, args...)
}

// logCountRate пишет строку о числе n в журнал работы: в первый раз, затем только при новом n и не
// чаще every по настоящим часам real. Одинаковое число не повторяется совсем.
func (s *Service) logCountRate(key string, n int, real time.Time, every time.Duration, format string, args ...any) {
	if last, ok := s.errAt[key]; ok && (s.logCount[key] == n || (real.Sub(last) < every && !real.Before(last))) {
		return
	}
	s.errAt[key], s.logCount[key] = real, n
	s.cfg.Logf(format, args...)
}

// vlogf пишет подробную диагностическую строку в журнал работы только при включённом
// переключателе «Подробные логи для диагностики» (FR-25, D-42).
func (s *Service) vlogf(format string, args ...any) {
	if s.verbose {
		s.cfg.Logf("[подробно] "+format, args...)
	}
}

// --- статусы и карточки -----------------------------------------------------------------

// statusOf возвращает статус ребёнка: единственный источник для значка, стража, карточки и окон.
// Дополняет статус ядра балансом копилки и текстом ближайшего запретного интервала (M2, FR-8).
func (s *Service) statusOf(cs *childState) ipc.ChildStatus {
	if cs.admin {
		return ipc.ChildStatusAdmin(cs.child.Name)
	}
	st := ipc.ChildStatusFrom(cs.child.Name, cs.eng.Current())
	if bal, err := s.cfg.Store.GetBonusBalance(cs.child.ID); err == nil {
		st.BonusBalance = bal
	}
	st.ForbiddenNote = s.forbiddenNote(cs)
	st.EndMode = cs.rules.EndMode.String() // правило ребёнка (R8); решение ядра его не несёт
	return st
}

// forbiddenNote — строка о ближайшем запретном интервале сегодня для мини-панели (ТЗ 8.2.1):
// «Сегодня нельзя с 21:00 до 07:00». Пусто, если интервалов сегодня впереди нет.
func (s *Service) forbiddenNote(cs *childState) string {
	now := s.cfg.Clock.Now().In(s.cfg.Loc)
	// Родитель снял запретные часы на сегодня (R24, D-51): не пугать «Сейчас нельзя» (класс A).
	if state, _ := cs.eng.State(); state.NoForbidden && state.Day == now.Format(time.DateOnly) {
		return "Сегодня запретные часы не действуют"
	}
	if in, end := core.InForbidden(cs.rules, now); in {
		return "Сейчас нельзя до " + core.FormatClock(end)
	}
	if start, ok := core.NextForbiddenStart(cs.rules, now); ok {
		// Конец интервала, начинающегося в start: момент внутри интервала даёт его конец.
		_, end := core.InForbidden(cs.rules, start.Add(time.Second))
		return "Сегодня нельзя с " + core.FormatClock(start) + " до " + core.FormatClock(end)
	}
	return ""
}

// cardOf возвращает карточку ребёнка для «Сегодня».
func (s *Service) cardOf(cs *childState) ipc.TodayCard {
	st := s.statusOf(cs)
	state, bypass := cs.eng.State()
	extra := state.ExtraMin
	if state.Day != st.Day && st.Day != "" {
		extra = 0
	}
	bonus, err := s.cfg.Store.GetBonusBalance(cs.child.ID)
	if err != nil {
		s.logRate(fmt.Sprint("bonus", cs.child.ID), "не удалось прочитать копилку ребёнка %d: %v", cs.child.ID, err)
		bonus = 0
	}
	noForbidden := state.NoForbidden && state.Day == st.Day
	gamesS, videoS := state.GamesS, state.VideoS
	if state.Day != st.Day && st.Day != "" {
		gamesS, videoS = 0, 0
	}
	return ipc.TodayCard{
		ChildID: cs.child.ID, SID: cs.child.SID, Name: cs.child.Name, Status: st,
		ExtraMin: extra, BonusBalance: bonus, CanClearBlock: bypass != "",
		NoForbiddenToday: noForbidden,
		// «Игры 40 мин из 1 ч» (8.3.2 п.3): игры за день ядра и лимит игр из того же решения.
		GamesText: ipc.GamesCardText(gamesS, st.GamesLimitS, st.GamesUnlimited),
		// «Видео 10 мин из 20 мин» (R6): так же по лимиту видео решения ядра.
		VideoText:       ipc.VideoCardText(videoS, st.VideoLimitS, st.VideoUnlimited),
		PendingPrograms: s.pendingCount(),
	}
}

// pendingCount — сколько программ ждут решения родителя (ссылка «Ждут решения: N», 8.3.2 п.5).
// Не прочиталось: 0, ошибка в журнал работы.
func (s *Service) pendingCount() int {
	list, err := s.cfg.Store.ListPending()
	if err != nil {
		s.logRate("pending", "не удалось прочитать «Ждут решения»: %v", err)
		return 0
	}
	return len(list)
}

// rulesView возвращает недельные лимиты и режим «Когда время вышло» в виде команд канала.
func rulesView(cs *childState) ipc.ChildRules {
	return ipc.ChildRules{ChildID: cs.child.ID, Name: cs.child.Name, Limits: limitsView(cs.rules), EndMode: cs.rules.EndMode.String(),
		Break: breakView(cs.rules)}
}

// limitsView — семь дней лимитов (общий и игр) в виде канала: правила ребёнка и FR-33.
func limitsView(r core.Rules) []ipc.WeekdayLimit {
	out := make([]ipc.WeekdayLimit, 0, 7)
	for i := 0; i < 7; i++ {
		l := r.Week[i]
		w := ipc.WeekdayLimit{Weekday: i + 1, Unlimited: l.Unlimited(), GamesUnlimited: l.GamesMin == nil, VideoUnlimited: l.VideoMin == nil}
		if !w.Unlimited {
			w.TotalMin = *l.TotalMin
		}
		if !w.GamesUnlimited {
			w.GamesMin = *l.GamesMin
		}
		if !w.VideoUnlimited {
			w.VideoMin = *l.VideoMin
		}
		out = append(out, w)
	}
	return out
}

// configured сообщает, что пароль родителя задан. Ошибка чтения считается «задан»:
// безопаснее не дать заново пройти настройку (R19).
func (s *Service) configured() bool {
	_, ok, err := s.cfg.Store.GetAuth()
	if err != nil {
		s.logRate("auth", "не удалось прочитать пароль родителя: %v", err)
		return true
	}
	return ok
}
