package service

import (
	"fmt"
	"sort"
	"strings"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/programs"
)

// Команды «Программ» (веха M3: R12, R13, FR-2, FR-20, RM3-1, RM3-10, RM3-12). Права проверены в
// Handle (parentToken; ScanPrograms — повышенный администратор до настройки).

// Виды событий журнала о программах (R22). program_decided и program_undone пишут обработчики
// родителя, остальные — наблюдатель (T4a). Нарушение из них только program_forbidden_try (R13).
const (
	evProgramDecided         = "program_decided"
	evProgramUndone          = "program_undone"
	evProgramHidden          = "program_hidden"  // вы скрыли программу (D-57)
	evProgramShown           = "program_shown"   // вы снова показали программу (D-57)
	evProgramRenamed         = "program_renamed" // вы переименовали программу (D-59)
	evProgramPending         = "program_pending"
	evProgramForbiddenTry    = "program_forbidden_try"
	evProgramClosedGames     = "program_closed_games"
	evProgramClosedStudyOnly = "program_closed_study_only"
)

const (
	maxProgramsPerChange = 200 // путей в одной массовой смене категории (FR-20)
	maxScanPrograms      = 300 // строк в шаге мастера «Программы» (FR-2)
	maxSetupPrograms     = 500 // программ в SetupRequest.Programs
	decidedPlateShowS    = 10  // секунд показа плашки о решении родителя
	fieldCategory        = "category"
	fieldPrograms        = "programs"
)

// parentCategories — категории, которые родитель может выбрать (FR-20): «system» не выбирается.
var parentCategories = map[string]bool{
	programs.CatGames: true, programs.CatStudy: true, programs.CatOther: true, programs.CatBlocked: true,
}

// categoryWord — категория в сообщении «SuperGame: игра» (8.3.4).
var categoryWord = map[string]string{
	programs.CatGames: "игра", programs.CatStudy: "учёба", programs.CatOther: "прочее", programs.CatBlocked: "запрещена",
}

// categoryTitle — название категории в журнале: «к категории «Игры»».
var categoryTitle = map[string]string{
	programs.CatGames: "Игры", programs.CatStudy: "Учёба", programs.CatOther: "Прочее", programs.CatBlocked: "Запрещено",
}

// decidedPlateText — плашка ребёнку о решении родителя (8.2.4 п.4, RM3-4): на «ты».
func decidedPlateText(cat, name string) string {
	switch cat {
	case programs.CatGames:
		return "Родитель разрешил «" + name + "»: это игра. Запусти её снова"
	case programs.CatStudy:
		return "Родитель разрешил «" + name + "»: это учёба. Запусти её снова"
	case programs.CatOther:
		return "Родитель разрешил «" + name + "»: это обычная программа. Запусти её снова"
	}
	return "Родитель запретил «" + name + "»"
}

func publisherOrStub(p string) string {
	if p == "" {
		return textNoPublisher
	}
	return p
}

// --- список -------------------------------------------------------------------------------

func (s *Service) hListPrograms(c *callCtx) ipc.Reply {
	view, ok := s.programsView()
	if !ok {
		return ipc.Fail(textRetry)
	}
	return ipc.OK(view)
}

// programsView собирает «Программы» (FR-20): ждущие решения (новые сверху), все программы,
// кроме «системных», по имени с временем всех детей за 7 местных дней, дети для подписей.
// ok = false: список не прочитан (в ответе пустые списки).
func (s *Service) programsView() (ipc.ProgramsReply, bool) {
	out := ipc.ProgramsReply{Pending: []ipc.PendingProgramDTO{}, Programs: []ipc.ProgramDTO{}, Children: []ipc.ChildName{}}
	for _, id := range s.childOrder() {
		out.Children = append(out.Children, ipc.ChildName{ChildID: id, Name: s.children[id].child.Name})
	}
	pend, err := s.cfg.Store.ListPending()
	if err != nil {
		s.logRate("pending", "не удалось прочитать «Ждут решения»: %v", err)
		return out, false
	}
	list, err := s.cfg.Store.ListPrograms()
	if err != nil {
		s.logRate("programs", "не удалось прочитать список программ: %v", err)
		return out, false
	}
	from := core.DayOf(s.cfg.Clock.Now().In(s.cfg.Loc).AddDate(0, 0, -6))
	secs, err := s.cfg.Store.ProgramSeconds(from)
	if err != nil {
		s.logRate("usage", "не удалось прочитать время по программам: %v", err)
		secs = map[string]int{}
	}
	sort.SliceStable(pend, func(i, j int) bool {
		if pend[i].LastAttemptAt != pend[j].LastAttemptAt {
			return pend[i].LastAttemptAt > pend[j].LastAttemptAt
		}
		return pend[i].Path < pend[j].Path
	})
	for _, p := range pend {
		dto := ipc.PendingProgramDTO{
			Path: p.Path, Name: programName(p.Name, p.Path), Publisher: publisherOrStub(p.Publisher), Description: programs.ClipName(p.Description),
			Attempts: p.Attempts, LastAttemptAt: p.LastAttemptAt, WasCategory: p.WasCategory, ByChild: []ipc.PendingChildDTO{},
		}
		for _, ch := range p.Children {
			dto.ByChild = append(dto.ByChild, ipc.PendingChildDTO{ChildID: ch.ChildID, Name: s.names[ch.ChildID], Attempts: ch.Attempts})
		}
		out.Pending = append(out.Pending, dto)
	}
	for _, p := range list {
		if p.Category == programs.CatSystem {
			continue
		}
		dto := ipc.ProgramDTO{
			Path: p.Path, Name: displayName(p), Publisher: publisherOrStub(p.Publisher),
			Category: p.Category, Source: p.Source, WeekS: secs[p.Path], Hidden: p.Hidden,
		}
		if file := programName(p.Name, p.Path); dto.Name != file {
			dto.FileName = file // «из файла: …» под своим названием (D-59)
		}
		out.Programs = append(out.Programs, dto)
	}
	sort.SliceStable(out.Programs, func(i, j int) bool {
		a, b := strings.ToLower(out.Programs[i].Name), strings.ToLower(out.Programs[j].Name)
		if a != b {
			return a < b
		}
		return out.Programs[i].Path < out.Programs[j].Path
	})
	return out, true
}

// --- решения ------------------------------------------------------------------------------

func (s *Service) hDecideProgram(c *callCtx) ipc.Reply {
	var req ipc.DecideProgramRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if !parentCategories[req.Category] {
		return ipc.FailField(textCategoryBad, fieldCategory)
	}
	path := programs.Normalize(req.Path)
	if path == "" {
		return ipc.Fail(textNoProgram)
	}
	at := c.real.UnixMilli()
	// Свойства файла (отпечаток, издатель) — вне блокировки службы: сетевой путь читается долго,
	// а тик, канал и снятие блокировки паролем ждать не должны (NFR-10).
	metas := s.fileMetaUnlocked([]string{path}, false)
	if s.stopping {
		return ipc.Fail(textRetry)
	}
	pend, waiting, err := s.cfg.Store.ResolvePending(path)
	if err != nil {
		s.logRate("decide", "решение по программе не записано: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	prev, existed, err := s.cfg.Store.GetProgram(path)
	if err != nil {
		s.restorePendingAfterFailure(waiting, pend)
		s.logRate("decide", "решение по программе не записано: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	rec := &progUndoRec{}
	var name string
	switch {
	case waiting:
		p := s.decidedProgram(path, pend.Name, pend.Publisher, req.Category, at, metas[path])
		if existed {
			if p.Publisher == "" {
				p.Publisher = prev.Publisher
			}
			p.CustomName, p.Hidden = prev.CustomName, prev.Hidden // своё название переживает подмену файла (D-59)
		}
		if err := s.cfg.Store.UpsertProgram(p); err != nil {
			s.restorePendingAfterFailure(true, pend)
			s.logRate("decide", "решение по программе не записано: %v", err)
			return ipc.Fail(textSaveFailed)
		}
		rec.pending = []PendingProgram{pend}
		if existed {
			rec.programs = []Program{prev}
		} else {
			rec.added = []string{path}
		}
		name = displayName(p)
	case existed:
		changed, err := s.cfg.Store.SetProgramsCategory([]string{path}, req.Category, at)
		if err != nil {
			s.logRate("decide", "решение по программе не записано: %v", err)
			return ipc.Fail(textSaveFailed)
		}
		rec.programs = changed
		name = displayName(prev)
	default:
		return ipc.Fail(textNoProgram)
	}
	s.reloadPrograms()
	s.journal.Write(0, evProgramDecided, map[string]any{"name": name, "path": path, "category": req.Category, "count": 1})
	if waiting {
		s.decidedPlates(pend, req.Category, name)
	}
	s.rememberProgramsUndo(rec, c)
	view, _ := s.programsView()
	return ipc.OK(ipc.DecideProgramReply{
		Message: name + ": " + categoryWord[req.Category], UndoID: rec.id, UndoUntilMs: rec.until.UnixMilli(), Programs: view,
	})
}

// fileMeta — свойства файла, прочитанные заранее вне блокировки службы.
type fileMeta struct {
	publisher, description, fingerprint string
}

// fileMetaUnlocked читает свойства файлов paths вне s.mu (NFR-10: сетевой путь не держит тик и
// канал) и возвращает их по пути. all = false — только пути вне доверенных каталогов (им нужен
// отпечаток, RM3-12). Вызывать под s.mu; возвращается под s.mu, но состояние службы за это время
// могло измениться: вызывающий заново проверяет то, от чего зависит запись (s.stopping и т.п.).
func (s *Service) fileMetaUnlocked(paths []string, all bool) map[string]fileMeta {
	roots := s.classifier.Roots()
	need := make([]string, 0, len(paths))
	for _, p := range paths {
		if all || !roots.IsTrusted(p) {
			need = append(need, p)
		}
	}
	out := make(map[string]fileMeta, len(need))
	if len(need) == 0 {
		return out
	}
	read := s.cfg.FileMeta
	s.unlocked(func() {
		for _, p := range need {
			var m fileMeta
			m.publisher, m.description, m.fingerprint = read(p)
			out[p] = m
		}
	})
	return out
}

// decidedProgram — запись решения родителя по пути: источник «родитель», корень приложения;
// вне доверенных каталогов — отпечаток файла (RM3-12), издатель из файла, если не известен.
// Имя берётся из «Ждут решения» (служба записала его из пути и файла), иначе из пути. Свойства
// файла meta прочитаны заранее вне блокировки службы (fileMetaUnlocked).
func (s *Service) decidedProgram(path, name, publisher, category string, at int64, meta fileMeta) Program {
	roots := s.classifier.Roots()
	p := Program{
		Path: path, Name: programName(name, path), Publisher: programs.ClipName(publisher),
		Category: category, Source: ProgramSourceParent, AppRoot: roots.AppRoot(path), DecidedAt: at,
	}
	if !roots.IsTrusted(path) {
		fp := meta.fingerprint
		p.Fingerprint = fp
		if p.Publisher == "" {
			p.Publisher = programs.ClipName(meta.publisher)
		}
		if fp == "" {
			s.logRate("fingerprint", "отпечаток файла %s не прочитан: подмена файла не будет замечена", path)
		}
	}
	return p
}

// restorePendingAfterFailure возвращает снятую «Ждут решения», если решение не записалось.
func (s *Service) restorePendingAfterFailure(waiting bool, p PendingProgram) {
	if !waiting {
		return
	}
	if err := s.cfg.Store.RestorePending(p); err != nil {
		s.logRate("decide", "«Ждут решения» не возвращена после сбоя: %v", err)
	}
}

// decidedPlates показывает плашку о решении детям, которые запускали программу и чей сеанс
// сейчас на экране (8.2.4 п.4): через очередь команд стража, как noticeGuards.
func (s *Service) decidedPlates(p PendingProgram, category, name string) {
	tried := map[int64]bool{p.FirstChildID: p.FirstChildID != 0}
	for _, ch := range p.Children {
		tried[ch.ChildID] = true
	}
	plate := ipc.BlockedPlate{Kind: ipc.BlockedDecided, Text: decidedPlateText(category, name), ShowS: decidedPlateShowS, Path: p.Path}
	for _, id := range s.sessionOrder() {
		ss := s.sessions[id]
		if ss.childID == 0 || !tried[ss.childID] || ss.offScreen() {
			continue
		}
		pl := plate
		s.guardCmd[id] = append(s.guardCmd[id], ipc.GuardCommand{Kind: ipc.GuardCmdBlocked, Plate: &pl})
	}
}

// rememberProgramsUndo делает операцию последней отменяемой: номер и срок 10 с по настоящим часам.
func (s *Service) rememberProgramsUndo(rec *progUndoRec, c *callCtx) {
	rec.id = s.cfg.Rand()
	rec.until = c.real.Add(undoLife)
	s.progUndo = rec
}

func (s *Service) hSetProgramsCategory(c *callCtx) ipc.Reply {
	var req ipc.SetProgramsCategoryRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if !parentCategories[req.Category] {
		return ipc.FailField(textCategoryBad, fieldCategory)
	}
	if len(req.Paths) > maxProgramsPerChange {
		return ipc.Fail(textTooManyPrograms)
	}
	paths := make([]string, 0, len(req.Paths))
	seen := map[string]bool{}
	known := 0
	for _, raw := range req.Paths {
		p := programs.Normalize(raw)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
		_, ok, err := s.cfg.Store.GetProgram(p)
		if err != nil {
			s.logRate("decide", "список программ не прочитан: %v", err)
			return ipc.Fail(textRetry)
		}
		if ok {
			known++
		}
	}
	if known == 0 {
		return ipc.Fail(textNoProgram)
	}
	changed, err := s.cfg.Store.SetProgramsCategory(paths, req.Category, c.real.UnixMilli())
	if err != nil {
		s.logRate("decide", "смена категории программ не записана: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	s.reloadPrograms()
	if len(changed) > 0 {
		d := map[string]any{"category": req.Category, "count": len(changed)}
		if len(changed) == 1 {
			d["name"], d["path"] = displayName(changed[0]), changed[0].Path
		}
		s.journal.Write(0, evProgramDecided, d)
	}
	rec := &progUndoRec{programs: changed}
	s.rememberProgramsUndo(rec, c)
	view, _ := s.programsView()
	return ipc.OK(ipc.DecideProgramReply{
		Message: fmt.Sprintf(textProgramsChanged, len(changed)), UndoID: rec.id, UndoUntilMs: rec.until.UnixMilli(), Programs: view,
	})
}

// hSetProgramsHidden — «Скрыть» и «Показать» (FR-20, D-57, RF9-1): меняется только флаг, категория
// остаётся; одна запись журнала на операцию, «Отменить» 10 с, классификатор перечитывается сразу.
// Скрывать можно только программы списка, кроме «системных» (RF9-1). Плашек ребёнку нет.
func (s *Service) hSetProgramsHidden(c *callCtx) ipc.Reply {
	var req ipc.SetProgramsHiddenRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if len(req.Paths) > maxProgramsPerChange {
		return ipc.Fail(textTooManyPrograms)
	}
	paths := make([]string, 0, len(req.Paths))
	seen := map[string]bool{}
	known := 0
	for _, raw := range req.Paths {
		p := programs.Normalize(raw)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
		prog, ok, err := s.cfg.Store.GetProgram(p)
		if err != nil {
			s.logRate("hide", "список программ не прочитан: %v", err)
			return ipc.Fail(textRetry)
		}
		if ok && prog.Category != programs.CatSystem {
			known++
		}
	}
	if known == 0 {
		return ipc.Fail(textNoProgram)
	}
	changed, err := s.cfg.Store.SetProgramsHidden(paths, req.Hidden)
	if err != nil {
		s.logRate("hide", "скрытие программ не записано: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	s.reloadPrograms()
	if len(changed) > 0 {
		kind := evProgramShown
		if req.Hidden {
			kind = evProgramHidden
		}
		d := map[string]any{"count": len(changed)}
		if len(changed) == 1 {
			d["name"], d["path"] = displayName(changed[0]), changed[0].Path
		}
		s.journal.Write(0, kind, d)
	}
	rec := &progUndoRec{programs: changed}
	s.rememberProgramsUndo(rec, c)
	view, _ := s.programsView()
	return ipc.OK(ipc.DecideProgramReply{
		Message: hiddenMessage(changed, req.Hidden), UndoID: rec.id, UndoUntilMs: rec.until.UnixMilli(), Programs: view,
	})
}

// hiddenMessage — «Stalker2 скрыта», «Скрыто программ: 3», «Stalker2 снова в списке»,
// «Снова в списке программ: 2» (8.3.4 п.4).
func hiddenMessage(changed []Program, hidden bool) string {
	if len(changed) == 1 {
		name := displayName(changed[0])
		if hidden {
			return fmt.Sprintf(textProgramHidden, name)
		}
		return fmt.Sprintf(textProgramShown, name)
	}
	if hidden {
		return fmt.Sprintf(textProgramsHidden, len(changed))
	}
	return fmt.Sprintf(textProgramsShown, len(changed))
}

// hRenameProgram — «Переименовать» (FR-20, D-59, 8.3.4 п.5): своё название 1–60 символов после
// обрезки пробелов, пустое — снова название из файла. Меняется только своё название; классификатор
// перечитывается сразу (плашки и журнал наблюдателя берут его оттуда); одна запись журнала, если
// название изменилось; «Отменить» 10 с. Плашек ребёнку нет.
func (s *Service) hRenameProgram(c *callCtx) ipc.Reply {
	var req ipc.RenameProgramRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	name, ok := programs.CustomName(req.Name)
	if !ok {
		return ipc.FailField(textNameTooLong, ipc.FieldName)
	}
	path := programs.Normalize(req.Path)
	if path == "" {
		return ipc.Fail(textNoProgram)
	}
	cur, known, err := s.cfg.Store.GetProgram(path)
	if err != nil {
		s.logRate("rename", "список программ не прочитан: %v", err)
		return ipc.Fail(textRetry)
	}
	if !known || cur.Category == programs.CatSystem {
		return ipc.Fail(textNoProgram)
	}
	prev, ok, err := s.cfg.Store.SetProgramName(path, name)
	if err != nil {
		s.logRate("rename", "название программы не записано: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	if !ok {
		return ipc.Fail(textNoProgram)
	}
	s.reloadPrograms()
	next := prev
	next.CustomName = name
	if from, to := displayName(prev), displayName(next); from != to {
		d := map[string]any{"from": from, "to": to, "path": path}
		if name == "" {
			d["reset"] = true
		}
		s.journal.Write(0, evProgramRenamed, d)
	}
	rec := &progUndoRec{programs: []Program{prev}}
	s.rememberProgramsUndo(rec, c)
	view, _ := s.programsView()
	return ipc.OK(ipc.DecideProgramReply{
		Message: textProgramRenamed, UndoID: rec.id, UndoUntilMs: rec.until.UnixMilli(), Programs: view,
	})
}

func (s *Service) hUndoProgram(c *callCtx) ipc.Reply {
	var req ipc.UndoProgramRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	rec := s.progUndo
	if rec == nil || rec.id == "" || rec.id != req.UndoID || !c.real.Before(rec.until) {
		return ipc.Fail(textUndoGone)
	}
	s.progUndo = nil
	failed := false
	for _, p := range rec.programs {
		if err := s.cfg.Store.RestoreProgram(p); err != nil {
			s.logRate("undo", "программа %s не возвращена: %v", p.Path, err)
			failed = true
		}
	}
	for _, path := range rec.added {
		if err := s.cfg.Store.DeleteProgram(path); err != nil {
			s.logRate("undo", "программа %s не убрана из списка: %v", path, err)
			failed = true
		}
	}
	for _, p := range rec.pending {
		if err := s.cfg.Store.RestorePending(p); err != nil {
			s.logRate("undo", "программа %s не возвращена в «Ждут решения»: %v", p.Path, err)
			failed = true
		}
	}
	s.reloadPrograms()
	if failed {
		return ipc.Fail(textSaveFailed)
	}
	s.journal.Write(0, evProgramUndone, map[string]any{})
	view, _ := s.programsView()
	return ipc.OK(ipc.UndoProgramReply{Message: textProgramUndone, Programs: view})
}

// --- мастер ---------------------------------------------------------------------------------

// hScanPrograms — программы компьютера для шага мастера «Программы» (FR-2) с предложенной
// категорией: игры вперёд, затем браузеры из каталога, затем остальные; не больше 300.
func (s *Service) hScanPrograms(c *callCtx) ipc.Reply {
	out := ipc.ScanProgramsReply{Programs: []ipc.ScanProgramDTO{}}
	if s.cfg.Scanner == nil {
		return ipc.OK(out)
	}
	found, err := s.scanUnlocked() // сканер и свойства файлов — вне блокировки службы (Д-2)
	if err != nil {
		s.logRate("scan", "программы компьютера не найдены: %v", err)
		return ipc.Fail(textScanFailed)
	}
	s.unlocked(func() { out.Programs = s.scanDTOs(found) })
	return ipc.OK(out)
}

// scanDTOs — строки шага мастера: игры вперёд, затем браузеры из каталога, затем остальные; не
// больше maxScanPrograms. Не трогает состояние службы (классификатор безопасен сам), поэтому
// зовётся вне s.mu: свойства файлов читаются с диска.
func (s *Service) scanDTOs(found []FoundProgram) []ipc.ScanProgramDTO {
	out := ipc.ScanProgramsReply{Programs: []ipc.ScanProgramDTO{}}
	rank := map[string]int{}
	for _, p := range s.snapshotPrograms(found) {
		dto := ipc.ScanProgramDTO{Path: p.Path, Name: p.Name, Publisher: p.Publisher, SuggestedCategory: p.Category}
		if dto.Publisher == "" {
			pub, _, _ := s.cfg.FileMeta(p.Path)
			dto.Publisher = programs.ClipName(pub)
		}
		dto.Publisher = publisherOrStub(dto.Publisher)
		switch {
		case p.Category == programs.CatGames:
			rank[p.Path] = 0
		case p.Source == ProgramSourceCatalog:
			rank[p.Path] = 1
		default:
			rank[p.Path] = 2
		}
		out.Programs = append(out.Programs, dto)
	}
	sort.SliceStable(out.Programs, func(i, j int) bool {
		a, b := out.Programs[i], out.Programs[j]
		if rank[a.Path] != rank[b.Path] {
			return rank[a.Path] < rank[b.Path]
		}
		if x, y := strings.ToLower(a.Name), strings.ToLower(b.Name); x != y {
			return x < y
		}
		return a.Path < b.Path
	})
	if len(out.Programs) > maxScanPrograms {
		out.Programs = out.Programs[:maxScanPrograms]
	}
	return out.Programs
}

// setupPrograms проверяет выбор мастера (до записи чего-либо): категории родителя, не больше
// maxSetupPrograms; непригодные пути и повторы пропускаются. Свойства файлов (до 500) читаются
// вне блокировки службы (NFR-10), поэтому после чтения вызывающий заново проверяет, что служба
// не останавливается и не настроена другим мастером. Возвращает записи и текст отказа.
func (s *Service) setupPrograms(list []ipc.SetupProgram, at int64) ([]Program, string) {
	if len(list) > maxSetupPrograms {
		return nil, ipc.TextBadRequest
	}
	type pick struct{ path, category string }
	picks := make([]pick, 0, len(list))
	paths := make([]string, 0, len(list))
	seen := map[string]bool{}
	roots := s.classifier.Roots()
	for _, sp := range list {
		if !parentCategories[sp.Category] {
			return nil, textCategoryBad
		}
		p := programs.Normalize(sp.Path)
		if p == "" || seen[p] || roots.IsSystem(p) {
			continue
		}
		seen[p] = true
		picks = append(picks, pick{p, sp.Category})
		paths = append(paths, p)
	}
	metas := s.fileMetaUnlocked(paths, true)
	out := make([]Program, 0, len(picks))
	for _, pk := range picks {
		m := metas[pk.path]
		pub, name := m.publisher, m.description
		if cur, ok, err := s.cfg.Store.GetProgram(pk.path); err == nil && ok {
			// Снимок при старте уже записал программу: имя и издатель из него.
			name, pub = cur.Name, cur.Publisher
		}
		out = append(out, s.decidedProgram(pk.path, name, pub, pk.category, at, m))
	}
	return out, ""
}

// writeSetupPrograms записывает выбор мастера и делает снимок остальных программ (RM3-10: тот
// же снимок и флаг). Сбой снимка мастер не останавливает: повтор в Housekeep.
func (s *Service) writeSetupPrograms(list []Program) error {
	for _, p := range list {
		if err := s.cfg.Store.UpsertProgram(p); err != nil {
			return err
		}
	}
	return nil
}

// afterSetupPrograms — снимок остальных программ и загрузка списка после мастера: как при старте,
// в своей горутине (ответ мастеру не ждёт сканер и не держит s.mu). Снимок уже идёт — его итог
// запишет та горутина (SeedPrograms не затирает выбор мастера).
func (s *Service) afterSetupPrograms(c *callCtx) {
	s.progTryAt = c.real
	s.startSeed()
}
