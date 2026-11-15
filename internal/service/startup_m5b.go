package service

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/programs"
	"pcblock/internal/startup"
)

// Автозагрузка учёток детей (M5b, FR-39, ТЗ 8.3.11, RM5b-2…RM5b-5, уточнения плана 1–4, 16).
//
// Проверка ребёнка (checkStartup) идёт в своей горутине вне s.mu: чтение Windows (порт
// StartupSource) и штатное выключение записей — без блокировки службы, сверка с базой и журнал —
// под s.mu. Одна проверка на ребёнка: просьба во время идущей ставит повтор. Чтение и запись
// Windows одного ребёнка (проверка и SetStartup) идут по очереди под startupLock ребёнка; порядок
// блокировок всегда «startupLock, затем s.mu».
//
// Полная проверка (List) — для ребёнка в сеансе; невошедшему профиль не подгружается (вход в эту
// секунду мог бы сломаться, как T5-4) — только общие записи (ListShared). Границы «найдено при
// первой проверке» (RM5b-4) две: settingStartupBaseline — первая полная проверка (свои записи до
// неё получают решение по состоянию Windows), settingStartupBaselineShared — первая проверка общих
// (общая запись, найденная после неё, — новая, даже если ребёнок ещё ни разу не входил).

const startupCheckEvery = 10 * time.Minute // проверка детей раз в столько (RM5b-5)

const (
	settingStartupBaseline       = "startup.baseline."        // + id ребёнка: UTC мс первой полной проверки
	settingStartupBaselineShared = "startup.baseline_shared." // + id ребёнка: UTC мс первой проверки общих записей
)

// startupKey — запись автозагрузки ребёнка: Scope + Source + Name (RM5b-4).
type startupKey struct{ scope, source, name string }

func startupKeyOf(scope, source, name string) startupKey { return startupKey{scope, source, name} }

// startupHit — строка автозагрузки в наборе наблюдателя (RM5b-3): id и название для журнала.
type startupHit struct {
	id   int64
	name string
}

// plateStartup — вид закрытия «запись автозагрузки запустилась сама» (watchAct.plateKind): плашки
// нет, стражу не уходит.
const plateStartup = "startup"

// startupCreatedSlack — насколько раньше момента, когда служба увидела вход ребёнка, мог быть
// создан процесс, чтобы правило автозагрузки его ещё закрывало: служба могла увидеть сеанс позже
// входа (запуск при загрузке). Созданный раньше — шёл до перезапуска службы (Review Focus 1).
const startupCreatedSlack = 2 * time.Minute

// startupCheckSlack — сколько после окна manualLogonGrace правило ещё закрывает процесс, увиденный
// в это окно: первая полная проверка после входа (загрузка куста, задания Планировщика, папка
// автозагрузки, приложения магазина; при загрузке компьютера диск занят) должна успеть собрать
// набор. Позже процесс считается оставленным: ребёнок мог им уже пользоваться.
const startupCheckSlack = 3 * time.Minute

// filetimeEpoch — 1 января 1970 года в FILETIME (100 нс с 1601 года).
const filetimeEpoch = 116444736000000000

// procCreated — момент создания процесса по ProcInfo.Stamp (FILETIME); ok = false — не похоже
// на момент (раньше 1970 года).
func procCreated(stamp uint64) (time.Time, bool) {
	if stamp <= filetimeEpoch {
		return time.Time{}, false
	}
	return time.Unix(0, int64(stamp-filetimeEpoch)*100), true
}

// startupLaunch — процесс p сеанса t с записью e закрывается правилом автозагрузки (RM5b-3):
// запуск не ручной (launchKind), увиден в первые manualLogonGrace после входа, создан не раньше
// входа (с запасом startupCreatedSlack), проход идёт не позже manualLogonGrace +
// startupCheckSlack после входа и прошлые проходы с готовым набором процесс не оставили живым
// (startupSpared): решение родителя или смена команды позже не закрывают программу, которой
// ребёнок уже пользуется. Момент входа или создания неизвестен — нет («не знаю — не трогать»):
// после перезапуска службы первые 60 с считаются «после входа» (RF18-3), а программы, давно
// запущенные ребёнком, этим правилом не закрываются.
func startupLaunch(t *watchTarget, p ProcInfo, e *watchEntry) bool {
	f := e.facts
	if e.manual || e.startupSpared || f.LogonAt.IsZero() || f.At.Sub(f.LogonAt) >= manualLogonGrace ||
		t.at.Sub(f.LogonAt) >= manualLogonGrace+startupCheckSlack {
		return false
	}
	created, ok := procCreated(p.Stamp)
	return ok && !created.Before(f.LogonAt.Add(-startupCreatedSlack))
}

// recordStartupClosed — запись автозагрузки запустилась сама и закрыта (RM5b-3): ребёнку ничего,
// нарушения нет; родителю — строка журнала раз в день на строку автозагрузки (дочерние процессы
// того же exe — та же строка). Под s.mu.
func (s *Service) recordStartupClosed(cs *childState, a *watchAct, real time.Time) {
	if s.autoClosedOnce(cs.child.ID, a.t.d.Day, fmt.Sprint("startup|", a.startup.id), real) {
		s.journal.Write(cs.child.ID, evStartupClosed, map[string]any{"name": a.startup.name, "path": a.e.key})
	}
}

// startupWatchSet пересобирает набор наблюдателя ребёнка (RM5b-3, уточнение 1): exe записей с
// решением off или pending, общих и своих, по exe последней удачной проверки. Exe, у которого
// есть и включённая запись, в набор не входит: запуск мог быть от неё. Сбой базы — набор прежний.
// Под s.mu.
func (s *Service) startupWatchSet(cs *childState) {
	items, err := s.cfg.Store.ListStartupItems(cs.child.ID)
	if err != nil {
		s.logRate(fmt.Sprint("startupdb", cs.child.ID), "автозагрузка %s: база не прочитана, набор наблюдателя прежний: %v", cs.child.Name, err)
		return
	}
	set, on := map[string]startupHit{}, map[string]bool{}
	for _, r := range items {
		norm := programs.Normalize(cs.startupExe[startupKeyOf(r.Scope, r.Source, r.Name)])
		if norm == "" {
			continue
		}
		switch r.Decision {
		case ipc.StartupOn:
			on[norm] = true
		case ipc.StartupOff, ipc.StartupPending:
			if _, dup := set[norm]; !dup {
				set[norm] = startupHit{id: r.ID, name: startupName(r)}
			}
		}
	}
	for norm := range on {
		delete(set, norm)
	}
	cs.startupOff = set
}

// startupReady — набор наблюдателя ребёнка собран полной проверкой не раньше входа since: только
// тогда «правило не закрыло процесс» значит «оставлен» (startupSpared). Под s.mu.
func startupReady(cs *childState, since time.Time) bool {
	return !cs.startupFullAt.IsZero() && !cs.startupFullAt.Before(since)
}

// startupFlight — идущая проверка ребёнка: again — попросили ещё раз, full — следующий проход
// полный (под s.mu).
type startupFlight struct{ again, full bool }

// startupLock — блокировка чтения и записи автозагрузки Windows ребёнка id. Под s.mu.
func (s *Service) startupLock(id int64) *sync.Mutex {
	mu := s.startupLk[id]
	if mu == nil {
		mu = &sync.Mutex{}
		s.startupLk[id] = mu
	}
	return mu
}

// childInSession — ребёнок id сейчас в каком-нибудь сеансе. Под s.mu.
func (s *Service) childInSession(id int64) bool {
	for _, ss := range s.sessions {
		if ss.childID == id {
			return true
		}
	}
	return false
}

// checkStartup просит проверку автозагрузки ребёнка (full — полную, иначе только общие записи):
// не идёт — запускает горутину, идёт — ставит повтор. Под s.mu; источника нет — ничего.
func (s *Service) checkStartup(cs *childState, full bool) {
	if s.cfg.Startup == nil || s.stopping || cs == nil {
		return
	}
	id := cs.child.ID
	if fl := s.startupRun[id]; fl != nil {
		fl.again = true
		fl.full = fl.full || full
		return
	}
	fl := &startupFlight{full: full}
	s.startupRun[id] = fl
	mu := s.startupLock(id)
	s.startupWG.Add(1)
	go s.runStartup(id, cs.child.SID, fl, mu)
}

// waitStartup ждёт горутины проверок (тесты). Вызывать без s.mu.
func (s *Service) waitStartup() { s.startupWG.Wait() }

// runStartup — горутина проверки ребёнка: проходы, пока просят повтор; служба остановлена или
// ребёнка убрали — конец.
func (s *Service) runStartup(id int64, sid string, fl *startupFlight, mu *sync.Mutex) {
	defer s.startupWG.Done()
	for {
		s.mu.Lock()
		if s.stopping || s.children[id] == nil {
			delete(s.startupRun, id)
			s.mu.Unlock()
			return
		}
		full := fl.full
		fl.again, fl.full = false, false
		s.mu.Unlock()

		mu.Lock()
		s.startupPass(id, sid, full)
		mu.Unlock()

		s.mu.Lock()
		if !fl.again {
			delete(s.startupRun, id)
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
	}
}

// startupSwitch — своя запись, которую проверка выключает в Windows.
type startupSwitch struct {
	e    startup.Entry
	name string
}

// startupPass — один проход проверки (вызывается под startupLock ребёнка, без s.mu). Паника —
// сбой одного дела (internalError), служба работает дальше.
func (s *Service) startupPass(id int64, sid string, full bool) {
	defer func() {
		if p := recover(); p != nil {
			s.mu.Lock()
			s.internalError("startup", s.children[id], p)
			s.mu.Unlock()
		}
	}()
	var found []startup.Entry
	var err error
	if full {
		found, err = s.cfg.Startup.List(sid)
	} else {
		found, err = s.cfg.Startup.ListShared()
	}
	now := s.cfg.RealNow()

	s.mu.Lock()
	switches, ok := s.startupApply(id, full, found, err, now)
	name := s.names[id]
	s.mu.Unlock()
	if !ok {
		return
	}
	var failed []string
	for _, sw := range switches {
		if err := s.cfg.Startup.SetEnabled(sid, sw.e, false); err != nil && !errors.Is(err, ErrNotSwitchable) {
			failed = append(failed, fmt.Sprintf("«%s»: %v", sw.name, err))
		}
	}
	if len(failed) > 0 {
		s.mu.Lock()
		s.logRate(fmt.Sprint("startupoff", id), "автозагрузка %s: не удалось выключить %v, повтор при следующей проверке", name, failed)
		s.mu.Unlock()
	}
}

// startupApply — под s.mu: сверяет найденное с базой (RM5b-4) и записывает итог одной
// транзакцией SyncStartupItems; пишет журнал о новых; возвращает свои записи, которые надо
// выключить в Windows (решение off/pending, а Windows «включено», кроме RunOnce, уточнение 3).
// Сбой чтения Windows или базы — ничего не меняется (PL-46), ok = false.
func (s *Service) startupApply(id int64, full bool, found []startup.Entry, listErr error, now time.Time) ([]startupSwitch, bool) {
	cs := s.children[id]
	if s.stopping || cs == nil {
		return nil, false
	}
	what := "общие записи"
	if full {
		what = "все записи"
	}
	if listErr != nil {
		s.logRate(fmt.Sprint("startuplist", id), "автозагрузка %s (%s) не прочитана, строки не меняются: %v", cs.child.Name, what, listErr)
		return nil, false
	}
	stored, err := s.cfg.Store.ListStartupItems(id)
	if err != nil {
		s.logRate(fmt.Sprint("startupdb", id), "автозагрузка %s: база не прочитана: %v", cs.child.Name, err)
		return nil, false
	}
	baseFull, err1 := s.startupBaseline(settingStartupBaseline, id)
	baseShared, err2 := s.startupBaseline(settingStartupBaselineShared, id)
	if err := errors.Join(err1, err2); err != nil {
		s.logRate(fmt.Sprint("startupdb", id), "автозагрузка %s: граница первой проверки не прочитана: %v", cs.child.Name, err)
		return nil, false
	}
	baseShared = baseShared || baseFull

	// Проверка только общих: свои строки передаются как есть, чтобы Sync их не удалил.
	var storedE []startup.Entry
	var records []StartupRecord
	for _, r := range stored {
		if full || r.Scope == startup.ScopeShared {
			storedE = append(storedE, startup.Entry{Scope: r.Scope, Source: r.Source, Name: r.Name, Command: r.Command, Publisher: r.Publisher})
		} else {
			records = append(records, StartupRecord{Scope: r.Scope, Source: r.Source, Name: r.Name, Command: r.Command, Publisher: r.Publisher})
		}
	}
	if !full {
		shared := found[:0:0]
		for _, e := range found {
			if e.Scope == startup.ScopeShared {
				shared = append(shared, e)
			}
		}
		found = shared
	}
	type key = startupKey
	isNew := map[key]bool{}
	for _, e := range startup.Compare(storedE, found).New {
		isNew[key{e.Scope, e.Source, e.Name}] = true
	}
	// Включённая запись со сменившейся командой (другой exe под тем же именем) снова ждёт решения
	// (решение оркестратора по Open T4a): своя выключается в Windows, как новая.
	changed := map[key]bool{}
	for _, r := range stored {
		if r.Decision != ipc.StartupOn {
			continue
		}
		for _, e := range found {
			if e.Scope == r.Scope && e.Source == r.Source && e.Name == r.Name && e.Command != r.Command {
				changed[key{e.Scope, e.Source, e.Name}] = true
			}
		}
	}
	for _, e := range found {
		rec := StartupRecord{Scope: e.Scope, Source: e.Source, Name: e.Name, Command: e.Command, Publisher: e.Publisher}
		if changed[key{e.Scope, e.Source, e.Name}] {
			rec.Decision = ipc.StartupPending
		}
		if isNew[key{e.Scope, e.Source, e.Name}] {
			first := !baseFull
			if e.Scope == startup.ScopeShared {
				first = !baseShared
			}
			switch {
			case !first:
				rec.Decision = ipc.StartupPending
			case e.Enabled:
				rec.Decision = ipc.StartupOn
			default:
				rec.Decision = ipc.StartupOff
			}
		}
		records = append(records, rec)
	}
	added, err := s.startupSync(id, records, changed, now)
	if err != nil {
		s.logRate(fmt.Sprint("startupdb", id), "автозагрузка %s: итог проверки не записан: %v", cs.child.Name, err)
		return nil, false
	}
	exe := map[key]string{}
	if !full {
		for k, v := range cs.startupExe {
			if k.scope == startup.ScopeOwn {
				exe[k] = v
			}
		}
	}
	for _, e := range found {
		exe[key{e.Scope, e.Source, e.Name}] = e.Exe
	}
	cs.startupExe = exe
	if full {
		cs.startupFullAt = now
	}
	stamp := strconv.FormatInt(now.UnixMilli(), 10)
	if full && !baseFull {
		s.setStartupBaseline(settingStartupBaseline, id, stamp)
	}
	if !baseShared {
		s.setStartupBaseline(settingStartupBaselineShared, id, stamp)
	}
	decision := map[key]string{}
	for _, r := range stored {
		decision[key{r.Scope, r.Source, r.Name}] = r.Decision
	}
	for _, r := range added {
		decision[key{r.Scope, r.Source, r.Name}] = r.Decision
		if r.Decision == ipc.StartupPending {
			d := map[string]any{"name": startupName(r), "scope": r.Scope, "source": r.Source}
			if changed[key{r.Scope, r.Source, r.Name}] {
				d["changed"] = true
			}
			s.journal.Write(id, evStartupNew, d)
		}
	}
	s.startupWatchSet(cs)
	s.startupDone[id] = now
	var switches []startupSwitch
	if !full {
		return switches, true
	}
	for _, e := range found {
		d := decision[key{e.Scope, e.Source, e.Name}]
		if e.Scope == startup.ScopeOwn && e.Source != startup.SrcRunOnce && e.Enabled && (d == ipc.StartupOff || d == ipc.StartupPending) {
			switches = append(switches, startupSwitch{e: e, name: startup.DisplayName(e)})
		}
	}
	return switches, true
}

// startupSync записывает итог проверки (SyncStartupItems). Строки changed (включённые со сменившейся
// командой) хранилище меняет только как новые: первый вызов без них их удаляет, второй вставляет
// с решением pending. Сбой между вызовами — строк нет, следующая проверка найдёт их новыми
// (pending): решение «on» не остаётся у другой команды. Под s.mu.
func (s *Service) startupSync(id int64, records []StartupRecord, changed map[startupKey]bool, now time.Time) ([]StartupRecord, error) {
	if len(changed) == 0 {
		return s.cfg.Store.SyncStartupItems(id, records, ipc.StartupPending, now.UnixMilli())
	}
	var rest []StartupRecord
	for _, r := range records {
		if !changed[startupKeyOf(r.Scope, r.Source, r.Name)] {
			rest = append(rest, r)
		}
	}
	added, err := s.cfg.Store.SyncStartupItems(id, rest, ipc.StartupPending, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	again, err := s.cfg.Store.SyncStartupItems(id, records, ipc.StartupPending, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	return append(added, again...), nil
}

// startupBaseline — есть ли граница prefix+id. Под s.mu.
func (s *Service) startupBaseline(prefix string, id int64) (bool, error) {
	_, ok, err := s.cfg.Store.GetSetting(prefix + strconv.FormatInt(id, 10))
	return ok, err
}

// setStartupBaseline записывает границу; сбой — следующая проверка снова первая (решения по
// состоянию Windows, ничего не выключается) — в журнал работы.
func (s *Service) setStartupBaseline(prefix string, id int64, stamp string) {
	if err := s.cfg.Store.SetSetting(prefix+strconv.FormatInt(id, 10), stamp); err != nil {
		s.logRate(fmt.Sprint("startupbase", id), "автозагрузка: граница первой проверки не записана: %v", err)
	}
}

// housekeepStartup — раз в startupCheckEvery проверить всех детей: вошедших полностью, остальных —
// только общие записи (RM5b-5). Под s.mu.
func (s *Service) housekeepStartup(real time.Time) {
	if s.cfg.Startup == nil || real.Sub(s.startupAt) < startupCheckEvery {
		return
	}
	s.startupAt = real
	for _, id := range s.childOrder() {
		s.checkStartup(s.children[id], s.childInSession(id))
	}
}

// startupName — название строки: у приложения магазина — понятное имя пакета.
func startupName(r StartupRecord) string {
	return startup.DisplayName(startup.Entry{Source: r.Source, Name: r.Name})
}

func startupView(r StartupRecord) ipc.StartupItemView {
	return ipc.StartupItemView{
		ID: r.ID, Scope: r.Scope, Source: r.Source, Name: startupName(r), Publisher: r.Publisher,
		From: startup.FromText(startup.Entry{Source: r.Source}), Decision: r.Decision,
	}
}

// hListStartup — вкладка «Автозагрузка» (8.3.11): «Ждут решения», свои, общие; Refresh запускает
// проверку (полную, если ребёнок в сеансе) и не ждёт её.
func (s *Service) hListStartup(c *callCtx) ipc.Reply {
	var req ipc.ListStartupRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	if req.Refresh {
		s.checkStartup(cs, s.childInSession(cs.child.ID))
	}
	rep := ipc.ListStartupReply{Pending: []ipc.StartupItemView{}, Own: []ipc.StartupItemView{}, Shared: []ipc.StartupItemView{}}
	if s.cfg.Startup != nil {
		items, err := s.cfg.Store.ListStartupItems(cs.child.ID)
		if err != nil {
			s.logRate(fmt.Sprint("startupdb", cs.child.ID), "автозагрузка %s: база не прочитана: %v", cs.child.Name, err)
			return ipc.Fail(textRetry)
		}
		for _, r := range items {
			v := startupView(r)
			switch {
			case r.Decision == ipc.StartupPending:
				rep.Pending = append(rep.Pending, v)
			case r.Scope == startup.ScopeShared:
				rep.Shared = append(rep.Shared, v)
			default:
				rep.Own = append(rep.Own, v)
			}
		}
	}
	if t, ok := s.startupDone[cs.child.ID]; ok {
		rep.CheckedAt = t.UnixMilli()
	}
	rep.Checking = s.startupRun[cs.child.ID] != nil
	return ipc.OK(rep)
}

// hSetStartup — решение родителя по строке (Review Focus 3, уточнение 4): своя запись — сначала
// Windows (вне s.mu, по свежему List: путь задания и пакет — из найденной записи), затем база;
// сбой Windows — решение не сохраняется. Общая и RunOnce — только база (уточнения 1, 16).
func (s *Service) hSetStartup(c *callCtx) ipc.Reply {
	var req ipc.SetStartupRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.children[req.ChildID]
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	if s.cfg.Startup == nil {
		return ipc.Fail(textSaveFailed)
	}
	id := cs.child.ID
	items, err := s.cfg.Store.ListStartupItems(id)
	if err != nil {
		s.logRate(fmt.Sprint("startupdb", id), "автозагрузка %s: база не прочитана: %v", cs.child.Name, err)
		return ipc.Fail(textSaveFailed)
	}
	var rec *StartupRecord
	for i := range items {
		if items[i].ID == req.ID {
			rec = &items[i]
		}
	}
	if rec == nil {
		return ipc.Fail(textSaveFailed)
	}
	if rec.Scope == startup.ScopeOwn && rec.Source != startup.SrcRunOnce {
		mu, sid, want := s.startupLock(id), cs.child.SID, *rec
		locked := false
		defer func() {
			if locked {
				mu.Unlock()
			}
		}()
		var werr error
		s.unlocked(func() {
			mu.Lock()
			locked = true
			werr = s.startupWrite(sid, want, req.On)
		})
		if s.stopping || s.children[id] != cs {
			return ipc.Fail(textNoSuchChild)
		}
		if werr != nil {
			s.logRate(fmt.Sprint("startupset", id), "автозагрузка %s: «%s» не переключена в Windows: %v", cs.child.Name, startupName(want), werr)
			return ipc.Fail(textSaveFailed)
		}
	}
	decision := ipc.StartupOff
	if req.On {
		decision = ipc.StartupOn
	}
	saved, err := s.cfg.Store.SetStartupDecision(id, rec.ID, decision, s.cfg.RealNow().UnixMilli())
	if err != nil {
		s.logRate(fmt.Sprint("startupdb", id), "автозагрузка %s: решение не записано: %v", cs.child.Name, err)
		return ipc.Fail(textSaveFailed)
	}
	s.journal.Write(id, evStartupDecided, map[string]any{"name": startupName(saved), "on": req.On, "scope": saved.Scope, "source": saved.Source})
	s.startupWatchSet(cs)
	s.checkStartup(cs, s.childInSession(id))
	msg := "Не будет запускаться при входе " + genitiveName(cs.child.Name)
	if req.On {
		msg = "Будет запускаться при входе " + genitiveName(cs.child.Name)
	}
	return ipc.OK(ipc.SetStartupReply{Item: startupView(saved), Message: msg})
}

// errStartupGone — записи уже нет в Windows (свежий List её не нашёл).
var errStartupGone = errors.New("запись автозагрузки не найдена в Windows")

// startupWrite — вне s.mu, под startupLock: свежий List ребёнка, поиск записи по
// Scope+Source+Name и штатное включение или выключение (RM5b-2).
func (s *Service) startupWrite(sid string, rec StartupRecord, on bool) error {
	found, err := s.cfg.Startup.List(sid)
	if err != nil {
		return err
	}
	for _, e := range found {
		if e.Scope == rec.Scope && e.Source == rec.Source && e.Name == rec.Name {
			return s.cfg.Startup.SetEnabled(sid, e, on)
		}
	}
	return errStartupGone
}
