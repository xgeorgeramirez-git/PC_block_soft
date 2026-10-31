package service

import (
	"fmt"
	"time"

	"pcblock/internal/programs"
)

// Программы (веха M3: R12, RM3-1, RM3-10, RM3-11): снимок Program Files, список программ в
// памяти (классификатор) и признак готовности для наблюдателя.

const (
	settingProgramsSeeded = "programs.seeded" // снимок программ сделан (RM3-10)
	programsRetryEvery    = 10 * time.Minute  // повтор неудавшегося снимка или чтения списка
	textNoPublisher       = "Без подписи издателя"
)

// progUndoRec — последняя операция с программами, которую ещё можно отменить (FR-20, 10 с):
// прежние записи программ, пути, которых до операции в списке не было, и снятые «Ждут решения».
type progUndoRec struct {
	id       string
	until    time.Time
	programs []Program        // вернуть как было (RestoreProgram)
	added    []string         // удалить (DeleteProgram): программы не было в списке
	pending  []PendingProgram // вернуть в «Ждут решения» (RestorePending)
}

// scanFlight — идущее сканирование программ: второй вызов во время первого ждёт его итога, а не
// запускает новое (Д-2).
type scanFlight struct {
	done  chan struct{}
	found []FoundProgram
	err   error
}

// scanUnlocked сканирует программы, отпустив s.mu на время Scan (Д-2): сканер на большом
// компьютере идёт до минуты, а канал связи, значок и страж не должны его ждать. Если сканирование
// уже идёт (в том числе снимок в своей горутине), ждёт его итог. Вызывать под s.mu; возвращается
// под s.mu, но состояние службы за это время могло измениться.
func (s *Service) scanUnlocked() ([]FoundProgram, error) {
	fl, leader := s.beginScan()
	s.unlocked(func() { s.awaitScan(fl, leader) })
	if leader {
		s.endScan(fl)
	}
	return fl.found, fl.err
}

// beginScan — под s.mu: идущее сканирование (leader = false) или новое (leader = true, его надо
// выполнить awaitScan и закрыть endScan). Одновременно идёт не больше одного сканирования.
func (s *Service) beginScan() (*scanFlight, bool) {
	if s.scan != nil {
		return s.scan, false
	}
	s.scan = &scanFlight{done: make(chan struct{})}
	return s.scan, true
}

// awaitScan — вне s.mu: ведущий сканирует (паника сканера — ошибка сканирования), остальные ждут.
func (s *Service) awaitScan(fl *scanFlight, leader bool) {
	if !leader {
		<-fl.done
		return
	}
	defer close(fl.done)
	defer func() {
		if p := recover(); p != nil {
			fl.found, fl.err = nil, fmt.Errorf("сбой сканера программ: %v", p)
		}
	}()
	fl.found, fl.err = s.cfg.Scanner.Scan()
}

// endScan — под s.mu: сканирование закончено, следующий вызов начнёт новое.
func (s *Service) endScan(fl *scanFlight) {
	if s.scan == fl {
		s.scan = nil
	}
}

// unlocked выполняет f, отпустив s.mu, и всегда берёт её снова (и при панике в f): обработчик
// канала и guard дальше работают под блокировкой, а отложенный Unlock в Handle не падает.
func (s *Service) unlocked(f func()) {
	s.mu.Unlock()
	defer s.mu.Lock()
	f()
}

// programsReady сообщает, что список программ готов: снимок сделан и прочитан в классификатор.
// Пока не готов, наблюдатель ничего не закрывает (RM3-10, R18). Вызывать под s.mu.
func (s *Service) programsReady() bool { return s.progReady }

// ensurePrograms готовит список программ, если это ещё не удалось; повтор не чаще
// programsRetryEvery. Вызывается под s.mu из Start и Housekeep и никогда не ждёт сканер: снимок
// уже есть — список читается сразу; нет — снимок запускается в своей горутине (startSeed), а
// Start и цикл учёта идут дальше (диспетчер служб ждёт «запущена» не дольше WaitHint, сканер на
// большом компьютере идёт до минуты). Пока снимка нет, programsReady() = false (RM3-10).
func (s *Service) ensurePrograms(real time.Time) {
	if s.progReady || s.seeding || (!s.progTryAt.IsZero() && real.Sub(s.progTryAt) < programsRetryEvery) {
		return
	}
	s.progTryAt = real
	s.startSeed()
}

// startSeed — под s.mu: снимок сделан раньше — список читается сразу; иначе снимок запускается в
// своей горутине (не больше одного сразу). Ошибка — в журнал работы, повтор через programsRetryEvery.
func (s *Service) startSeed() {
	if s.seeding || s.stopping {
		return
	}
	v, ok, err := s.cfg.Store.GetSetting(settingProgramsSeeded)
	if err != nil {
		s.logRate("seed", "не удалось прочитать, сделан ли снимок программ: %v", err)
		return
	}
	if ok && v == "1" {
		s.progReady = s.reloadPrograms()
		return
	}
	if s.cfg.Scanner == nil {
		s.logRate("seed", "снимок программ не сделан: сканер не подключён, программы не закрываются")
		return
	}
	s.seeding = true
	fl, leader := s.beginScan()
	s.seedWG.Add(1)
	go s.runSeed(fl, leader)
}

// runSeed — горутина снимка: сканирование вне s.mu, запись итога под s.mu. После остановки
// службы ничего не пишет (база может быть уже закрыта) и не держит остановку: Shutdown её не ждёт.
func (s *Service) runSeed(fl *scanFlight, leader bool) {
	defer s.seedWG.Done()
	s.awaitScan(fl, leader)
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if p := recover(); p != nil {
			s.internalError("programs", nil, p)
		}
	}()
	s.seeding = false
	if leader {
		s.endScan(fl)
	}
	if s.stopping {
		s.cfg.Logf("Снимок программ не записан: служба остановлена")
		return
	}
	if s.writeSeed(fl.found, fl.err) {
		s.progReady = s.reloadPrograms()
	}
}

// waitSeed ждёт горутину снимка (тесты; служба её не ждёт). Вызывать без s.mu.
func (s *Service) waitSeed() { s.seedWG.Wait() }

// writeSeed записывает снимок (RM3-10) под s.mu: находки сканера → записи «Прочее» или по
// каталогу → SeedPrograms (решения родителя не трогаются) → флаг. Ошибка сканирования или записи —
// в журнал работы, флага нет.
func (s *Service) writeSeed(found []FoundProgram, scanErr error) bool {
	if scanErr != nil {
		s.logRate("seed", "снимок программ не сделан, программы не закрываются до повтора: %v", scanErr)
		return false
	}
	list := s.snapshotPrograms(found)
	added, err := s.cfg.Store.SeedPrograms(list)
	if err != nil {
		s.logRate("seed", "снимок программ не записан: %v", err)
		return false
	}
	if err := s.cfg.Store.SetSetting(settingProgramsSeeded, "1"); err != nil {
		s.logRate("seed", "отметка о снимке программ не записана: %v", err)
		return false
	}
	s.cfg.Logf("Снимок программ: найдено %d, в списке %d, добавлено %d", len(found), len(list), added)
	return true
}

// snapshotPrograms превращает находки сканера в записи списка: только доверенные каталоги вне
// класса «система» (что лежит в папке ребёнка, снимком не разрешается), без повторов; категория
// по каталогу (источник catalog) или «Прочее» (snapshot).
func (s *Service) snapshotPrograms(found []FoundProgram) []Program {
	roots := s.classifier.Roots()
	out := make([]Program, 0, len(found))
	seen := map[string]bool{}
	for _, f := range found {
		p := programs.Normalize(f.Path)
		if p == "" || seen[p] || !roots.IsTrusted(p) || roots.IsSystem(p) {
			continue
		}
		seen[p] = true
		pr := Program{
			Path: p, Name: programName(f.Name, f.Path), Publisher: programs.ClipName(f.Publisher),
			Category: programs.CatOther, Source: ProgramSourceSnapshot, AppRoot: roots.AppRoot(p),
		}
		if c, ok := s.classifier.CatalogCategory(p); ok { // по имени exe и по пакету Store (Д-3)
			pr.Category, pr.Source = c, ProgramSourceCatalog
		}
		out = append(out, pr)
	}
	return out
}

// Источники категории программы (как storage.Source* и ipc.ProgramSource*).
const (
	ProgramSourceCatalog  = "catalog"
	ProgramSourceSnapshot = "snapshot"
	ProgramSourceParent   = "parent"
)

// displayName — как программа называется во всех показах (D-59): одно место, programs.DisplayName.
func displayName(p Program) string { return programs.DisplayName(p.CustomName, p.Name, p.Path) }

// programName — название из файла для списка: из свойств файла, иначе из пути (без своего названия).
func programName(name, path string) string {
	if n := programs.ClipName(name); n != "" {
		return n
	}
	return programs.NameFromPath(path)
}

// reloadPrograms перечитывает список программ в классификатор (после снимка и решений
// родителя). Ошибка чтения — false, классификатор остаётся прежним.
func (s *Service) reloadPrograms() bool {
	list, err := s.cfg.Store.ListPrograms()
	if err != nil {
		s.logRate("programs", "не удалось прочитать список программ: %v", err)
		return false
	}
	entries := make([]programs.Entry, 0, len(list))
	for _, p := range list {
		entries = append(entries, programs.Entry{Path: p.Path, Name: p.Name, CustomName: p.CustomName, Category: p.Category, Fingerprint: p.Fingerprint, Publisher: p.Publisher, Hidden: p.Hidden})
	}
	s.classifier.Reload(entries)
	return true
}
