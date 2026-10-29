package service

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/programs"
)

// progHarness — настроенная служба с Мишей и Колей, снимком (Brave, Minecraft, Chrome, GeoGebra)
// и «Ждут решения»: SuperGame из «Загрузок» (Миша 3 попытки, Коля 1). Миша вошёл в сеанс 2.
type progHarness struct {
	*harness
	meta         *fakeMeta
	misha, kolya int64
	tok          string
}

func newProgHarness(t *testing.T) *progHarness {
	t.Helper()
	meta := newMeta()
	meta.set(pathSuperGame, "", "Super Game", "1024:1700000000")
	meta.set(pathHomework, "Школа №1", "Домашка", "77:1700000100")
	h := newHarness(t, configured(), withChild(childSID, "Миша", 120), withChild(child2SID, "Коля", 120),
		withPrograms(typicalScan()), withMeta(meta))
	h.start()
	ph := &progHarness{harness: h, meta: meta, misha: h.childID(childSID), kolya: h.childID(child2SID)}
	at := h.real.Now().Add(-2 * time.Hour).UnixMilli()
	game := PendingProgram{Path: pathSuperGame, Name: "SuperGame", Description: "Super Game"}
	for i := 0; i < 3; i++ {
		h.store.AddPendingAttempt(game, ph.misha, at+int64(i)*60_000)
	}
	h.store.AddPendingAttempt(game, ph.kolya, at+30_000)
	h.childLogsIn()
	ph.tok = h.login(adminPeer())
	return ph
}

// list возвращает «Программы».
func (h *progHarness) list() ipc.ProgramsReply {
	h.t.Helper()
	var rep ipc.ProgramsReply
	h.ok(adminPeer(), h.tok, ipc.CmdListPrograms, nil, &rep)
	return rep
}

func (h *progHarness) decide(path, cat string) ipc.DecideProgramReply {
	h.t.Helper()
	var rep ipc.DecideProgramReply
	h.ok(adminPeer(), h.tok, ipc.CmdDecideProgram, ipc.DecideProgramRequest{Path: path, Category: cat}, &rep)
	return rep
}

func (h *progHarness) classify(path, fp string) programs.Result {
	h.svc.mu.Lock()
	defer h.svc.mu.Unlock()
	return h.svc.classifier.Classify(path, fp, "")
}

// plates забирает плашки о программах из очереди стража сеанса.
func (h *progHarness) plates(session uint32) []ipc.BlockedPlate {
	h.t.Helper()
	var rep ipc.GuardPollReply
	h.ok(guardPeerOf(session), "", ipc.CmdGuardPoll, ipc.GuardPollRequest{Sample: activeSample}, &rep)
	out := []ipc.BlockedPlate{}
	for _, c := range rep.Commands {
		if c.Kind == ipc.GuardCmdBlocked && c.Plate != nil {
			out = append(out, *c.Plate)
		}
	}
	return out
}

// journal возвращает строки журнала родителя по виду.
func (h *progHarness) journal(kind string) []string {
	h.t.Helper()
	var rep ipc.ListEventsReply
	h.ok(adminPeer(), h.tok, ipc.CmdListEvents, ipc.ListEventsRequest{}, &rep)
	out := []string{}
	for _, e := range rep.Events {
		if e.Kind == kind {
			out = append(out, e.Text)
		}
	}
	return out
}

// Ловит: «Программы» показывают «системные», теряют издателя-заглушку (RM3-3), имена детей,
// время за неделю считается не за 7 местных дней, ждущие и программы не по порядку.
func TestListPrograms(t *testing.T) {
	h := newProgHarness(t)
	h.store.UpsertProgram(Program{Path: `c:\program files\vendor\driver.exe`, Name: "Драйвер", Category: "system", Source: "parent"})
	h.store.AddPendingAttempt(PendingProgram{Path: pathHomework, Name: "Homework", Publisher: "Школа №1"}, h.kolya, h.real.Now().UnixMilli())
	// Понедельник 19.10: неделя — с 13.10 (вторник) по 19.10 включительно.
	h.store.AddUsage(h.misha, "2026-10-19", "other", "program", pathBrave, 600)
	h.store.AddUsage(h.kolya, "2026-10-13", "other", "program", pathBrave, 60)
	h.store.AddUsage(h.misha, "2026-10-12", "other", "program", pathBrave, 5000)
	h.store.AddUsage(h.misha, "2026-10-19", "games", "program", pathMinecraft, 1800)
	got := h.list()
	at := h.real.Now().Add(-2 * time.Hour).UnixMilli()
	want := ipc.ProgramsReply{
		Pending: []ipc.PendingProgramDTO{
			{Path: pathHomework, Name: "Homework", Publisher: "Школа №1", Attempts: 1, LastAttemptAt: h.real.Now().UnixMilli(),
				ByChild: []ipc.PendingChildDTO{{ChildID: h.kolya, Name: "Коля", Attempts: 1}}},
			{Path: pathSuperGame, Name: "SuperGame", Publisher: "Без подписи издателя", Description: "Super Game", Attempts: 4,
				LastAttemptAt: at + 120_000,
				ByChild:       []ipc.PendingChildDTO{{ChildID: h.misha, Name: "Миша", Attempts: 3}, {ChildID: h.kolya, Name: "Коля", Attempts: 1}}},
		},
		Programs: []ipc.ProgramDTO{
			{Path: pathBrave, Name: "Brave", Publisher: "Brave Software, Inc.", Category: "other", Source: "catalog", WeekS: 660},
			{Path: pathChrome, Name: "chrome", Publisher: "Google LLC", Category: "other", Source: "catalog"},
			{Path: pathGeoGebra, Name: "GeoGebra Classic", Publisher: "Без подписи издателя", Category: "other", Source: "snapshot"},
			{Path: pathMinecraft, Name: "Minecraft Launcher", Publisher: "Mojang", Category: "games", Source: "catalog", WeekS: 1800},
		},
		Children: []ipc.ChildName{{ChildID: h.misha, Name: "Миша"}, {ChildID: h.kolya, Name: "Коля"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListPrograms:\n got %+v\nwant %+v", got, want)
	}
}

// Ловит: пустые «Программы» уходят во фронт как null (CLAUDE.md, правило 6).
func TestListProgramsEmpty(t *testing.T) {
	h := newHarness(t, configured())
	h.start()
	tok := h.login(adminPeer())
	rep := h.do(adminPeer(), tok, ipc.CmdListPrograms, nil)
	if !rep.OK || string(rep.Payload) != `{"pending":[],"programs":[],"children":[]}` {
		t.Errorf("пустые «Программы»: %v %s", rep.Error, rep.Payload)
	}
}

// Ловит: решение по ждущей программе не записывается для всех детей, не убирает её из «Ждут
// решения», не запоминает отпечаток вне доверенных каталогов (RM3-12), не доходит до
// классификатора без перезапуска, не пишется в журнал родителю на «вы», не показывает плашку
// ребёнку, который запускал, или показывает тому, кто не запускал.
func TestDecidePendingProgram(t *testing.T) {
	h := newProgHarness(t)
	h.child2LogsIn()
	h.plates(childSession)
	h.plates(child2Session)
	if r := h.classify(pathSuperGame, "1024:1700000000"); r.Verdict != programs.VerdictUnknown {
		t.Fatalf("до решения: %+v", r)
	}
	rep := h.decide(`C:\Users\Миша\Downloads\SuperGame.exe`, "games")
	if rep.Message != "SuperGame: игра" || rep.UndoID == "" || rep.UndoUntilMs != h.real.Now().Add(10*time.Second).UnixMilli() {
		t.Errorf("ответ: %q, undo %q до %d", rep.Message, rep.UndoID, rep.UndoUntilMs)
	}
	if len(rep.Programs.Pending) != 0 {
		t.Errorf("после решения «Ждут решения»: %+v", rep.Programs.Pending)
	}
	p, ok, _ := h.store.GetProgram(pathSuperGame)
	wantP := Program{ID: p.ID, Path: pathSuperGame, Name: "SuperGame", Category: "games", Source: "parent",
		Fingerprint: "1024:1700000000", DecidedAt: h.real.Now().UnixMilli()}
	if !ok || p != wantP {
		t.Errorf("запись программы:\n got %+v\nwant %+v", p, wantP)
	}
	if r := h.classify(pathSuperGame, "1024:1700000000"); r.Verdict != programs.VerdictAllow || r.Category != "games" {
		t.Errorf("классификатор после решения: %+v", r)
	}
	if got := h.journal("program_decided"); !reflect.DeepEqual(got, []string{"Вы отнесли «SuperGame» к категории «Игры»"}) {
		t.Errorf("журнал: %q", got)
	}
	want := ipc.BlockedPlate{Kind: ipc.BlockedDecided, Text: "Родитель разрешил «SuperGame»: это игра. Запусти её снова", ShowS: 10, Path: pathSuperGame}
	if got := h.plates(childSession); !reflect.DeepEqual(got, []ipc.BlockedPlate{want}) {
		t.Errorf("плашка Мише: %+v", got)
	}
	if got := h.plates(child2Session); !reflect.DeepEqual(got, []ipc.BlockedPlate{want}) {
		t.Errorf("плашка Коле (тоже запускал): %+v", got)
	}
}

// Ловит: плашка уходит ребёнку, который программу не запускал, или в отключённый сеанс.
func TestDecidePlateOnlyForTriedAndOnScreen(t *testing.T) {
	h := newProgHarness(t)
	h.store.AddPendingAttempt(PendingProgram{Path: pathHomework, Name: "Homework"}, h.kolya, h.real.Now().UnixMilli())
	h.child2LogsIn()
	h.plates(childSession)
	h.plates(child2Session)
	h.decide(pathHomework, "study")
	if got := h.plates(childSession); len(got) != 0 {
		t.Errorf("Миша не запускал Homework, а плашка есть: %+v", got)
	}
	if got := h.plates(child2Session); len(got) != 1 || got[0].Text != "Родитель разрешил «Homework»: это учёба. Запусти её снова" {
		t.Errorf("плашка Коле: %+v", got)
	}
	h.svc.OnSession(SessionDisconnect, childSession, h.real.Now())
	h.decide(pathSuperGame, "blocked")
	if got := h.plates(childSession); len(got) != 0 {
		t.Errorf("отключённый сеанс Миши получил плашку: %+v", got)
	}
	if got := h.plates(child2Session); len(got) != 1 || got[0].Text != "Родитель запретил «SuperGame»" {
		t.Errorf("плашка о запрете Коле: %+v", got)
	}
}

// Ловит: категория вне «Игры/Учёба/Прочее/Запретить» принимается (в том числе «system», которая
// вывела бы программу из-под наблюдения), неизвестный путь создаёт запись, ответ без поля.
func TestDecideProgramRefusals(t *testing.T) {
	h := newProgHarness(t)
	for _, cat := range []string{"system", "", "game", "GAMES"} {
		rep := h.fail(adminPeer(), h.tok, ipc.CmdDecideProgram, ipc.DecideProgramRequest{Path: pathSuperGame, Category: cat})
		if rep.Error != "Выберите: игра, учёба, прочее или запретить" || rep.Field != "category" {
			t.Errorf("категория %q: %q поле %q", cat, rep.Error, rep.Field)
		}
	}
	for _, path := range []string{`c:\users\миша\downloads\nope.exe`, ``, `nope.exe`} {
		rep := h.fail(adminPeer(), h.tok, ipc.CmdDecideProgram, ipc.DecideProgramRequest{Path: path, Category: "games"})
		if rep.Error != "Такой программы нет в списке" {
			t.Errorf("путь %q: %q", path, rep.Error)
		}
	}
	if _, ok, _ := h.store.GetProgram(`c:\users\миша\downloads\nope.exe`); ok {
		t.Error("неизвестный путь записан в список")
	}
	if len(h.list().Pending) != 1 {
		t.Error("отказ изменил «Ждут решения»")
	}
}

// Ловит: решение по программе, которая уже в списке (не ждёт), отказывает или не меняет
// категорию; путь в доверенном каталоге читает отпечаток (не нужен, RM3-12).
func TestDecideKnownProgramAndTrustedPath(t *testing.T) {
	h := newProgHarness(t)
	rep := h.decide(pathBrave, "study")
	if rep.Message != "Brave: учёба" {
		t.Errorf("сообщение: %q", rep.Message)
	}
	if p, _, _ := h.store.GetProgram(pathBrave); p.Category != "study" || p.Source != "parent" || p.Publisher != "Brave Software, Inc." {
		t.Errorf("Brave после решения: %+v", p)
	}
	const newApp = `c:\program files\newapp\newapp.exe`
	h.store.AddPendingAttempt(PendingProgram{Path: newApp, Name: "NewApp"}, h.misha, h.real.Now().UnixMilli())
	h.decide(newApp, "other")
	p, _, _ := h.store.GetProgram(newApp)
	if p.Fingerprint != "" || p.AppRoot != `c:\program files\newapp` || p.Category != "other" {
		t.Errorf("программа в Program Files: %+v", p)
	}
	for _, asked := range h.meta.askedPaths() {
		if asked == newApp {
			t.Error("для пути в доверенном каталоге прочитан отпечаток")
		}
	}
}

// Ловит: «Отменить» не возвращает «Ждут решения» с попытками детей, оставляет запись программы,
// срабатывает после 10 с, дважды или по чужому номеру; классификатор не откатывается.
func TestUndoDecision(t *testing.T) {
	h := newProgHarness(t)
	rep := h.decide(pathSuperGame, "games")
	if rep := h.fail(adminPeer(), h.tok, ipc.CmdUndoProgram, ipc.UndoProgramRequest{UndoID: "чужой"}); rep.Error != "Отменить уже нельзя" {
		t.Errorf("чужой номер: %q", rep.Error)
	}
	h.real.Advance(9 * time.Second)
	var undo ipc.UndoProgramReply
	h.ok(adminPeer(), h.tok, ipc.CmdUndoProgram, ipc.UndoProgramRequest{UndoID: rep.UndoID}, &undo)
	if undo.Message != "Решение отменено" {
		t.Errorf("сообщение: %q", undo.Message)
	}
	if len(undo.Programs.Pending) != 1 || undo.Programs.Pending[0].Attempts != 4 || len(undo.Programs.Pending[0].ByChild) != 2 {
		t.Errorf("«Ждут решения» после отмены: %+v", undo.Programs.Pending)
	}
	if _, ok, _ := h.store.GetProgram(pathSuperGame); ok {
		t.Error("запись программы осталась после отмены")
	}
	if r := h.classify(pathSuperGame, "1024:1700000000"); r.Verdict != programs.VerdictUnknown {
		t.Errorf("классификатор после отмены: %+v", r)
	}
	if got := h.journal("program_undone"); !reflect.DeepEqual(got, []string{"Решение по программам отменено"}) {
		t.Errorf("журнал отмены: %q", got)
	}
	if rep := h.fail(adminPeer(), h.tok, ipc.CmdUndoProgram, ipc.UndoProgramRequest{UndoID: rep.UndoID}); rep.Error != "Отменить уже нельзя" {
		t.Errorf("повторная отмена: %q", rep.Error)
	}
	rep2 := h.decide(pathSuperGame, "blocked")
	h.real.Advance(10 * time.Second)
	if rep := h.fail(adminPeer(), h.tok, ipc.CmdUndoProgram, ipc.UndoProgramRequest{UndoID: rep2.UndoID}); rep.Error != "Отменить уже нельзя" {
		t.Errorf("через 10 с: %q", rep.Error)
	}
	if p, _, _ := h.store.GetProgram(pathSuperGame); p.Category != "blocked" {
		t.Errorf("просроченная отмена изменила решение: %+v", p)
	}
}

// Ловит: новое решение не отменяет право «Отменить» прежнего (отмена старого затёрла бы новое).
func TestUndoOnlyLastOperation(t *testing.T) {
	h := newProgHarness(t)
	first := h.decide(pathSuperGame, "games")
	h.decide(pathBrave, "study")
	if rep := h.fail(adminPeer(), h.tok, ipc.CmdUndoProgram, ipc.UndoProgramRequest{UndoID: first.UndoID}); rep.Error != "Отменить уже нельзя" {
		t.Errorf("отмена не последней операции: %q", rep.Error)
	}
}

// Ловит: отмена после новой попытки ребёнка (подмена файла вернула программу в «Ждут решения»)
// теряет счётчик или прежнюю запись с отпечатком (RM3-12).
func TestUndoAfterRetryKeepsAttempts(t *testing.T) {
	h := newProgHarness(t)
	h.store.UpsertProgram(Program{Path: pathSuperGame, Name: "SuperGame", Category: "games", Source: "parent", Fingerprint: "1:1", DecidedAt: 7})
	h.store.pending[pathSuperGame].WasCategory = "games"
	rep := h.decide(pathSuperGame, "study")
	if p, _, _ := h.store.GetProgram(pathSuperGame); p.Fingerprint != "1024:1700000000" || p.Category != "study" {
		t.Errorf("после решения по подменённому файлу: %+v", p)
	}
	// Ребёнок снова запустил (наблюдатель записал попытку), потом родитель нажал «Отменить».
	h.store.AddPendingAttempt(PendingProgram{Path: pathSuperGame, Name: "SuperGame"}, h.misha, h.real.Now().UnixMilli())
	h.ok(adminPeer(), h.tok, ipc.CmdUndoProgram, ipc.UndoProgramRequest{UndoID: rep.UndoID}, nil)
	pend := h.list().Pending
	if len(pend) != 1 || pend[0].Attempts != 5 || pend[0].WasCategory != "games" || pend[0].ByChild[0].Attempts != 4 {
		t.Errorf("«Ждут решения» после отмены: %+v", pend)
	}
	if p, _, _ := h.store.GetProgram(pathSuperGame); p.Fingerprint != "1:1" || p.Category != "games" || p.DecidedAt != 7 {
		t.Errorf("прежняя запись не возвращена: %+v", p)
	}
}

// Ловит: массовая смена не одной записью журнала, отмена возвращает не всё, лишние пути
// создают записи, больше 200 путей принимается, неизвестные пути — успех.
func TestSetProgramsCategory(t *testing.T) {
	h := newProgHarness(t)
	var rep ipc.DecideProgramReply
	h.ok(adminPeer(), h.tok, ipc.CmdSetProgramsCategory, ipc.SetProgramsCategoryRequest{
		Paths: []string{strings.ToUpper(pathBrave), pathGeoGebra, pathGeoGebra, `c:\nope\x.exe`}, Category: "study"}, &rep)
	if rep.Message != "Изменено программ: 2" || rep.UndoID == "" {
		t.Errorf("ответ: %+v", rep)
	}
	for _, p := range []string{pathBrave, pathGeoGebra} {
		if pr, _, _ := h.store.GetProgram(p); pr.Category != "study" || pr.Source != "parent" {
			t.Errorf("%s: %+v", p, pr)
		}
	}
	if _, ok, _ := h.store.GetProgram(`c:\nope\x.exe`); ok {
		t.Error("неизвестный путь записан")
	}
	if got := h.journal("program_decided"); !reflect.DeepEqual(got, []string{"Вы отнесли 2 программы к категории «Учёба»"}) {
		t.Errorf("журнал: %q", got)
	}
	if r := h.classify(pathBraveUpd, ""); r.Category != "other" {
		t.Errorf("учёба унаследована вспомогательным exe: %+v", r)
	}
	h.ok(adminPeer(), h.tok, ipc.CmdUndoProgram, ipc.UndoProgramRequest{UndoID: rep.UndoID}, nil)
	if pr, _, _ := h.store.GetProgram(pathBrave); pr.Category != "other" || pr.Source != "catalog" {
		t.Errorf("Brave после отмены: %+v", pr)
	}
	if pr, _, _ := h.store.GetProgram(pathGeoGebra); pr.Category != "other" || pr.Source != "snapshot" {
		t.Errorf("GeoGebra после отмены: %+v", pr)
	}
	many := make([]string, 201)
	for i := range many {
		many[i] = fmt.Sprintf(`c:\program files\app%d\app.exe`, i)
	}
	if rep := h.fail(adminPeer(), h.tok, ipc.CmdSetProgramsCategory, ipc.SetProgramsCategoryRequest{Paths: many, Category: "games"}); rep.Error != "Можно изменить не больше 200 программ за раз" {
		t.Errorf("201 путь: %q", rep.Error)
	}
	if rep := h.fail(adminPeer(), h.tok, ipc.CmdSetProgramsCategory, ipc.SetProgramsCategoryRequest{Paths: []string{`c:\nope\x.exe`}, Category: "games"}); rep.Error != "Такой программы нет в списке" {
		t.Errorf("только неизвестные пути: %q", rep.Error)
	}
	if rep := h.fail(adminPeer(), h.tok, ipc.CmdSetProgramsCategory, ipc.SetProgramsCategoryRequest{Paths: []string{pathBrave}, Category: "system"}); rep.Field != "category" {
		t.Errorf("категория system: %q поле %q", rep.Error, rep.Field)
	}
	// Одна программа: журнал по имени.
	h.ok(adminPeer(), h.tok, ipc.CmdSetProgramsCategory, ipc.SetProgramsCategoryRequest{Paths: []string{pathChrome}, Category: "blocked"}, nil)
	if got := h.journal("program_decided"); len(got) != 2 || got[0] != "Вы запретили «chrome»" {
		t.Errorf("журнал одной программы: %q", got)
	}
}

// Ловит: смена категории родителем не доходит до Classify, который идёт одновременно (гонка
// Reload и чтения, go test -race; решение действует на следующем Classify).
func TestDecideWhileClassifying(t *testing.T) {
	h := newProgHarness(t)
	h.svc.mu.Lock()
	cl := h.svc.classifier
	h.svc.mu.Unlock()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				cl.Classify(pathBraveUpd, "", "")
			}
		}
	}()
	for _, c := range []string{"blocked", "games", "other"} {
		h.decide(pathBrave, c)
	}
	close(stop)
	wg.Wait()
	if r := cl.Classify(pathBraveUpd, "", ""); r.Category != "other" {
		t.Errorf("после решений: %+v", r)
	}
}

// Ловит: шаг мастера показывает программы вне Program Files, PC Block, повторы, предлагает
// игру как «Прочее», ставит игры не первыми, не подставляет издателя из файла или заглушку,
// отдаёт больше 300 строк; ошибка сканера выглядит как «программ нет».
func TestScanPrograms(t *testing.T) {
	meta := newMeta()
	meta.set(pathGeoGebra, "International GeoGebra Institute", "GeoGebra", "")
	h := newHarness(t, withPrograms(typicalScan()), withMeta(meta))
	h.start()
	var rep ipc.ScanProgramsReply
	h.ok(elevatedPeer(), "", ipc.CmdScanPrograms, nil, &rep)
	want := []ipc.ScanProgramDTO{
		{Path: pathMinecraft, Name: "Minecraft Launcher", Publisher: "Mojang", SuggestedCategory: "games"},
		{Path: pathBrave, Name: "Brave", Publisher: "Brave Software, Inc.", SuggestedCategory: "other"},
		{Path: pathChrome, Name: "chrome", Publisher: "Google LLC", SuggestedCategory: "other"},
		{Path: pathGeoGebra, Name: "GeoGebra Classic", Publisher: "International GeoGebra Institute", SuggestedCategory: "other"},
	}
	if !reflect.DeepEqual(rep.Programs, want) {
		t.Errorf("ScanPrograms:\n got %+v\nwant %+v", rep.Programs, want)
	}

	big := &fakeScanner{}
	for i := 0; i < 350; i++ {
		big.list = append(big.list, FoundProgram{Path: fmt.Sprintf(`c:\program files\app%03d\app.exe`, i), Name: fmt.Sprintf("App %03d", i)})
	}
	big.list = append(big.list, FoundProgram{Path: `c:\program files (x86)\steam\steam.exe`, Name: "Steam"})
	h2 := newHarness(t, withPrograms(big))
	h2.start()
	h2.ok(elevatedPeer(), "", ipc.CmdScanPrograms, nil, &rep)
	if len(rep.Programs) != 300 || rep.Programs[0].Name != "Steam" || rep.Programs[1].Publisher != "Без подписи издателя" {
		t.Errorf("350+1 программ: %d, первая %+v, вторая %+v", len(rep.Programs), rep.Programs[0], rep.Programs[1])
	}

	bad := &fakeScanner{err: fmt.Errorf("нет доступа")}
	h3 := newHarness(t, withPrograms(bad))
	h3.start()
	if rep := h3.fail(elevatedPeer(), "", ipc.CmdScanPrograms, nil); rep.Error != "Не удалось найти программы. Их можно настроить позже в разделе «Программы»" {
		t.Errorf("ошибка сканера: %q", rep.Error)
	}
	h4 := newHarness(t)
	h4.start()
	h4.ok(elevatedPeer(), "", ipc.CmdScanPrograms, nil, &rep)
	if rep.Programs == nil || len(rep.Programs) != 0 {
		t.Errorf("без сканера: %+v, want []", rep.Programs)
	}
}

// Ловит: мастер не сохраняет выбор родителя (Escalation T3a), неверная категория пишет детей и
// пароль, остальные программы не попадают в снимок, флага снимка нет.
func TestSetupPrograms(t *testing.T) {
	sc := typicalScan()
	sc.err = fmt.Errorf("занято") // при старте снимок не удался: его сделает Setup
	h := newHarness(t, withPrograms(sc))
	h.start()
	sc.mu.Lock()
	sc.err = nil
	sc.mu.Unlock()
	req := ipc.SetupRequest{Password: goodPassword, Children: []ipc.SetupChild{{SID: childSID, Name: "Миша", Limits: setupLimits(120)}}}
	req.Programs = []ipc.SetupProgram{{Path: pathMinecraft, Category: "system"}}
	if rep := h.fail(elevatedPeer(), "", ipc.CmdSetup, req); rep.Error != "Выберите: игра, учёба, прочее или запретить" || rep.Field != "programs" {
		t.Errorf("неверная категория: %q поле %q", rep.Error, rep.Field)
	}
	if h.store.auth != nil || len(h.store.children) != 0 {
		t.Fatal("отказ мастера записал пароль или детей")
	}
	req.Programs = []ipc.SetupProgram{
		{Path: strings.ToUpper(pathGeoGebra), Category: "study"},
		{Path: pathBrave, Category: "blocked"},
	}
	h.ok(elevatedPeer(), "", ipc.CmdSetup, req, nil)
	h.svc.waitSeed() // снимок после мастера — в своей горутине
	geo, _, _ := h.store.GetProgram(pathGeoGebra)
	if geo.Category != "study" || geo.Source != "parent" || geo.AppRoot != `c:\program files\geogebra` || geo.Name == "" {
		t.Errorf("GeoGebra из мастера: %+v", geo)
	}
	if br, _, _ := h.store.GetProgram(pathBrave); br.Category != "blocked" || br.Source != "parent" {
		t.Errorf("Brave из мастера: %+v", br)
	}
	if mc, ok, _ := h.store.GetProgram(pathMinecraft); !ok || mc.Category != "games" || mc.Source != "catalog" {
		t.Errorf("Minecraft из снимка: %+v %v", mc, ok)
	}
	h.svc.mu.Lock()
	ready := h.svc.programsReady()
	r := h.svc.classifier.Classify(pathBraveUpd, "", "")
	h.svc.mu.Unlock()
	if h.store.settings["programs.seeded"] != "1" || !ready || r.Verdict != programs.VerdictForbidden {
		t.Errorf("после мастера: флаг %q, готово %v, обновление Brave %+v", h.store.settings["programs.seeded"], ready, r)
	}
}

// setupLimits — неделя лимитов для мастера.
func setupLimits(min int) []ipc.WeekdayLimit {
	out := make([]ipc.WeekdayLimit, 7)
	for i := range out {
		out[i] = ipc.WeekdayLimit{Weekday: i + 1, TotalMin: min, GamesUnlimited: true}
	}
	return out
}

// Ловит: строки журнала о программах не по ТЗ (R22): родителю не на «вы», нет имени ребёнка,
// длинное имя не обрезано до 120 символов, числа из базы (float64) не читаются.
func TestProgramEventTexts(t *testing.T) {
	long := strings.Repeat("Игра", 40)
	cases := []struct {
		kind, child string
		d           map[string]any
		want        string
	}{
		{"program_decided", "", map[string]any{"name": "SuperGame", "category": "games", "count": float64(1)}, "Вы отнесли «SuperGame» к категории «Игры»"},
		{"program_decided", "", map[string]any{"name": "Брейн", "category": "study", "count": 1}, "Вы отнесли «Брейн» к категории «Учёба»"},
		{"program_decided", "", map[string]any{"name": "X", "category": "other", "count": 1}, "Вы отнесли «X» к категории «Прочее»"},
		{"program_decided", "", map[string]any{"name": "X", "category": "blocked", "count": 1}, "Вы запретили «X»"},
		{"program_decided", "", map[string]any{"category": "games", "count": float64(21)}, "Вы отнесли 21 программу к категории «Игры»"},
		{"program_decided", "", map[string]any{"category": "blocked", "count": float64(5)}, "Вы запретили 5 программ"},
		{"program_undone", "", map[string]any{}, "Решение по программам отменено"},
		{"program_pending", "Миша", map[string]any{"name": "SuperGame"}, "Миша: программа «SuperGame» ждёт решения"},
		{"program_pending", "", map[string]any{"name": "SuperGame"}, "Программа «SuperGame» ждёт решения"},
		{"program_forbidden_try", "Миша", map[string]any{"name": "SuperGame"}, "Миша: попытка открыть запрещённое: «SuperGame»"},
		{"program_closed_games", "Миша", map[string]any{"name": "Minecraft"}, "Миша: закрыта «Minecraft»: время на игры закончилось"},
		{"program_closed_study_only", "Миша", map[string]any{"name": "Brave"}, "Миша: закрыта «Brave»: время вышло, доступна только учёба"},
		{"program_forbidden_try", "Миша", map[string]any{"path": `c:\users\миша\downloads\SuperGame.exe`}, "Миша: попытка открыть запрещённое: «SuperGame»"},
		{"program_pending", "Миша", map[string]any{"name": long}, "Миша: программа «" + string([]rune(long)[:120]) + "» ждёт решения"},
	}
	for _, c := range cases {
		if got := eventText(c.kind, c.child, c.d); got != c.want {
			t.Errorf("eventText(%s, %q, %v) = %q, want %q", c.kind, c.child, c.d, got, c.want)
		}
	}
}

// Ловит (возврат T4a): ответ мастеру Setup ждёт снимок остальных программ (до минуты) под
// блокировкой службы: окно мастера и канал «висят».
func TestSetupDoesNotWaitForSnapshot(t *testing.T) {
	sc := typicalScan()
	sc.err = fmt.Errorf("занято")
	h := newHarness(t, withPrograms(sc))
	h.start()
	sc.mu.Lock()
	sc.err = nil
	sc.mu.Unlock()
	sc.hold()
	t.Cleanup(sc.release)
	req := ipc.SetupRequest{Password: goodPassword, Children: []ipc.SetupChild{{SID: childSID, Name: "Миша", Limits: setupLimits(120)}}}
	within(t, 200*time.Millisecond, "Setup во время снимка", func() { h.ok(elevatedPeer(), "", ipc.CmdSetup, req, nil) })
	if !sc.waitEntered(2 * time.Second) {
		t.Fatal("снимок после мастера не начат")
	}
	within(t, 200*time.Millisecond, "GetChildStatus во время снимка", func() {
		h.svc.Handle(childPeer(), ipc.Msg{Cmd: ipc.CmdGetChildStatus}, h.real.Now())
	})
	sc.release()
	h.svc.waitSeed()
	if h.store.settings["programs.seeded"] != "1" {
		t.Error("после мастера снимок не записан")
	}
}
