package service

import (
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

// programsReady сообщает, что список программ готов: снимок сделан и прочитан в классификатор.
// Пока не готов, наблюдатель ничего не закрывает (RM3-10, R18). Вызывать под s.mu.
func (s *Service) programsReady() bool { return s.progReady }

// ensurePrograms делает снимок и загружает список программ, если это ещё не удалось; повтор не
// чаще programsRetryEvery. Вызывается под s.mu из Start и Housekeep.
func (s *Service) ensurePrograms(real time.Time) {
	if s.progReady || (!s.progTryAt.IsZero() && real.Sub(s.progTryAt) < programsRetryEvery) {
		return
	}
	s.progTryAt = real
	if !s.seedPrograms() {
		return
	}
	s.progReady = s.reloadPrograms()
}

// seedPrograms делает снимок программ, если флага programs.seeded ещё нет (RM3-10): сканер →
// записи «Прочее» или по каталогу → SeedPrograms (решения родителя не трогаются) → флаг.
// Возвращает true, если снимок есть (сделан сейчас или раньше). Ошибка — в журнал работы, флага нет.
func (s *Service) seedPrograms() bool {
	v, ok, err := s.cfg.Store.GetSetting(settingProgramsSeeded)
	if err != nil {
		s.logRate("seed", "не удалось прочитать, сделан ли снимок программ: %v", err)
		return false
	}
	if ok && v == "1" {
		return true
	}
	if s.cfg.Scanner == nil {
		s.logRate("seed", "снимок программ не сделан: сканер не подключён, программы не закрываются")
		return false
	}
	found, err := s.cfg.Scanner.Scan()
	if err != nil {
		s.logRate("seed", "снимок программ не сделан, программы не закрываются до повтора: %v", err)
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
	cat := s.classifier.Catalog()
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
		if c, ok := cat.Lookup(p); ok {
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

// programName — имя программы для списка: из свойств файла, иначе из пути.
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
		entries = append(entries, programs.Entry{Path: p.Path, Name: p.Name, Category: p.Category, Fingerprint: p.Fingerprint})
	}
	s.classifier.Reload(entries)
	return true
}
