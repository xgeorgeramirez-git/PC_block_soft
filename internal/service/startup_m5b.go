package service

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"pcblock/internal/ipc"
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
	type key struct{ scope, source, name string }
	isNew := map[key]bool{}
	for _, e := range startup.Compare(storedE, found).New {
		isNew[key{e.Scope, e.Source, e.Name}] = true
	}
	for _, e := range found {
		rec := StartupRecord{Scope: e.Scope, Source: e.Source, Name: e.Name, Command: e.Command, Publisher: e.Publisher}
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
	added, err := s.cfg.Store.SyncStartupItems(id, records, ipc.StartupPending, now.UnixMilli())
	if err != nil {
		s.logRate(fmt.Sprint("startupdb", id), "автозагрузка %s: итог проверки не записан: %v", cs.child.Name, err)
		return nil, false
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
			s.journal.Write(id, evStartupNew, map[string]any{"name": startupName(r), "scope": r.Scope, "source": r.Source})
		}
	}
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
