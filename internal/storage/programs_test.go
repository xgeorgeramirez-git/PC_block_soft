package storage

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"pcblock/internal/core"
)

const (
	pathGame  = `c:\users\миша\downloads\supergame.exe`
	pathBrave = `c:\program files\bravesoftware\brave-browser\application\brave.exe`
	pathWord  = `c:\program files\microsoft office\root\office16\winword.exe`
)

func braveSnapshot() Program {
	return Program{Path: pathBrave, Name: "Brave", Publisher: "Brave Software, Inc.", Category: ProgramOther, Source: SourceSnapshot,
		AppRoot: `c:\program files\bravesoftware`}
}

// Ловит: программа записывается без издателя, отпечатка или корня приложения (RM3-1, RM3-3,
// RM3-12), повторная запись по тому же пути создаёт вторую строку, «нет программы» читается
// как ошибка, пустой список уходит во фронт как null, в базу попадает мусорная категория.
func TestUpsertGetListPrograms(t *testing.T) {
	s := openTemp(t)
	if got, err := s.ListPrograms(); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("ListPrograms пустой базы = %#v, %v; want []Program{}", got, err)
	}
	if b, _ := json.Marshal([]Program{}); string(b) != "[]" {
		t.Fatal("пустой список не []")
	}
	if p, ok, err := s.GetProgram(pathGame); err != nil || ok || p != (Program{}) {
		t.Fatalf("GetProgram без записи = %+v, %v, %v; want пусто, false, nil", p, ok, err)
	}
	game := Program{Path: pathGame, Name: "SuperGame", Publisher: "Без подписи издателя", Category: ProgramGames, Source: SourceParent,
		Fingerprint: "1048576:1792423845", DecidedAt: 1_792_423_845_000}
	if err := s.UpsertProgram(game); err != nil {
		t.Fatalf("UpsertProgram: %v", err)
	}
	if err := s.UpsertProgram(braveSnapshot()); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetProgram(pathGame)
	if err != nil || !ok {
		t.Fatalf("GetProgram = %v, %v", ok, err)
	}
	game.ID = got.ID
	if got != game || got.ID == 0 {
		t.Fatalf("GetProgram = %+v, want %+v", got, game)
	}
	if v := rawNull(t, s, `SELECT decided_at FROM programs WHERE exe_path = ?`, pathBrave); v.Valid {
		t.Errorf("decided_at снимка = %d, want NULL (DecidedAt 0)", v.Int64)
	}

	game.Category, game.Fingerprint = ProgramStudy, "2:3"
	if err := s.UpsertProgram(game); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListPrograms()
	if err != nil {
		t.Fatal(err)
	}
	brave := braveSnapshot()
	brave.ID = list[0].ID
	if want := []Program{brave, game}; !reflect.DeepEqual(list, want) {
		t.Fatalf("ListPrograms = %+v, want %+v (по имени, без второй строки)", list, want)
	}

	for name, bad := range map[string]Program{
		"пустой путь":     {Name: "x", Category: ProgramGames, Source: SourceParent},
		"пустое имя":      {Path: `c:\x.exe`, Category: ProgramGames, Source: SourceParent},
		"категория":       {Path: `c:\x.exe`, Name: "x", Category: "video", Source: SourceParent},
		"источник":        {Path: `c:\x.exe`, Name: "x", Category: ProgramGames, Source: "child"},
		"без категории":   {Path: `c:\x.exe`, Name: "x", Source: SourceParent},
		"без источника":   {Path: `c:\x.exe`, Name: "x", Category: ProgramOther},
		"момент меньше 0": {Path: `c:\x.exe`, Name: "x", Category: ProgramOther, Source: SourceParent, DecidedAt: -1},
	} {
		if err := s.UpsertProgram(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("UpsertProgram(%s) = %v, want ErrInvalid", name, err)
		}
	}
	if n := rawNull(t, s, `SELECT count(*) FROM programs`); n.Int64 != 2 {
		t.Errorf("строк programs = %d, want 2", n.Int64)
	}
}

// Ловит: снимок при старте (RM3-10) перезаписывает решение родителя или прежний снимок, считает
// добавленными уже известные программы, пишет половину набора при ошибке в середине.
func TestSeedProgramsKeepsExisting(t *testing.T) {
	s := openTemp(t)
	parent := Program{Path: pathGame, Name: "SuperGame", Category: ProgramBlocked, Source: SourceParent, DecidedAt: 5}
	if err := s.UpsertProgram(parent); err != nil {
		t.Fatal(err)
	}
	seed := []Program{
		{Path: pathGame, Name: "SuperGame", Category: ProgramGames, Source: SourceCatalog},
		braveSnapshot(),
		{Path: pathWord, Name: "Word", Category: ProgramOther, Source: SourceSnapshot},
		{Path: pathWord, Name: "Word второй раз", Category: ProgramStudy, Source: SourceSnapshot},
	}
	added, err := s.SeedPrograms(seed)
	if err != nil || added != 2 {
		t.Fatalf("SeedPrograms = %d, %v; want 2, nil", added, err)
	}
	if p, _, _ := s.GetProgram(pathGame); p.Category != ProgramBlocked || p.Source != SourceParent || p.DecidedAt != 5 {
		t.Errorf("решение родителя после снимка = %+v, want blocked/parent/5", p)
	}
	if p, _, _ := s.GetProgram(pathWord); p.Name != "Word" || p.Category != ProgramOther {
		t.Errorf("Word = %+v, want первая запись набора", p)
	}
	if added, err := s.SeedPrograms(seed); err != nil || added != 0 {
		t.Errorf("повторный снимок = %d, %v; want 0, nil", added, err)
	}
	if added, err := s.SeedPrograms(nil); err != nil || added != 0 {
		t.Errorf("пустой снимок = %d, %v; want 0, nil", added, err)
	}
	bad := []Program{{Path: `c:\program files\new\new.exe`, Name: "New", Category: ProgramOther, Source: SourceSnapshot}, {Path: `c:\y.exe`, Name: "y", Category: "мусор", Source: SourceSnapshot}}
	if _, err := s.SeedPrograms(bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("снимок с мусорной категорией = %v, want ErrInvalid", err)
	}
	if n := rawNull(t, s, `SELECT count(*) FROM programs`); n.Int64 != 3 {
		t.Errorf("строк programs = %d, want 3 (испорченный набор не записан целиком)", n.Int64)
	}
}

// Ловит: массовая смена категории (FR-20) не отдаёт прежние значения для «Отменить», трогает
// программы, которых нет, или уже такие; не ставит источник «родитель» и момент решения;
// «Отменить» (RestoreProgram) не возвращает строку как была; DeleteProgram не удаляет.
func TestSetProgramsCategoryAndUndo(t *testing.T) {
	s := openTemp(t)
	brave := braveSnapshot()
	word := Program{Path: pathWord, Name: "Word", Category: ProgramStudy, Source: SourceParent, DecidedAt: 7}
	for _, p := range []Program{brave, word} {
		if err := s.UpsertProgram(p); err != nil {
			t.Fatal(err)
		}
	}
	b0, _, _ := s.GetProgram(pathBrave)
	changed, err := s.SetProgramsCategory([]string{pathBrave, pathWord, `c:\нет.exe`, pathBrave}, ProgramStudy, 1_000)
	if err != nil {
		t.Fatalf("SetProgramsCategory: %v", err)
	}
	if want := []Program{b0}; !reflect.DeepEqual(changed, want) {
		t.Fatalf("changed = %+v, want %+v (только Brave: Word уже «Учёба» от родителя, третьей нет)", changed, want)
	}
	b1, _, _ := s.GetProgram(pathBrave)
	if b1.Category != ProgramStudy || b1.Source != SourceParent || b1.DecidedAt != 1_000 || b1.Publisher != brave.Publisher || b1.AppRoot != brave.AppRoot {
		t.Errorf("Brave после смены = %+v, want study/parent/1000 с прежним издателем и корнем", b1)
	}
	if w, _, _ := s.GetProgram(pathWord); w.DecidedAt != 7 {
		t.Errorf("Word тронут: %+v", w)
	}
	if got, err := s.SetProgramsCategory([]string{`c:\нет.exe`}, ProgramGames, 2); err != nil || got == nil || len(got) != 0 {
		t.Errorf("смена несуществующей = %#v, %v; want [], nil", got, err)
	}
	if _, err := s.SetProgramsCategory([]string{pathBrave}, "video", 3); !errors.Is(err, ErrInvalid) {
		t.Errorf("мусорная категория = %v, want ErrInvalid", err)
	}

	if err := s.RestoreProgram(changed[0]); err != nil {
		t.Fatalf("RestoreProgram: %v", err)
	}
	if b, _, _ := s.GetProgram(pathBrave); b != b0 {
		t.Errorf("после «Отменить» = %+v, want %+v", b, b0)
	}
	if err := s.DeleteProgram(pathWord); err != nil {
		t.Fatalf("DeleteProgram: %v", err)
	}
	if _, ok, _ := s.GetProgram(pathWord); ok {
		t.Error("Word не удалён")
	}
	if err := s.DeleteProgram(pathWord); !errors.Is(err, ErrNotFound) {
		t.Errorf("повторное удаление = %v, want ErrNotFound", err)
	}
}

// Ловит: повторная попытка запуска не увеличивает счётчик (R12), попытки двух детей
// смешиваются или теряются (FR-20 «Миша, 3 попытки»), время последней попытки не растёт или
// уходит назад, список «Ждут решения» не новые сверху, пустые списки — null.
func TestPendingAttemptsByChild(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	anya := mustChild(t, s, "S-2", "Аня")
	if got, err := s.ListPending(); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("ListPending пустой = %#v, %v; want []", got, err)
	}
	game := PendingProgram{Path: pathGame, Name: "SuperGame", Publisher: "Без подписи издателя", Description: "Super Game"}
	steps := []struct {
		child int64
		at    int64
		want  int
	}{{misha, 100, 1}, {misha, 300, 2}, {anya, 200, 3}, {misha, 250, 4}}
	for _, st := range steps {
		n, err := s.AddPendingAttempt(game, st.child, st.at)
		if err != nil || n != st.want {
			t.Fatalf("AddPendingAttempt(%d, %d) = %d, %v; want %d", st.child, st.at, n, err, st.want)
		}
	}
	other := PendingProgram{Path: `c:\temp\tool.exe`, Name: "Tool", WasCategory: ProgramGames}
	if _, err := s.AddPendingAttempt(other, anya, 400); err != nil {
		t.Fatal(err)
	}
	noChild := PendingProgram{Path: `c:\temp\old.exe`, Name: "Old"}
	if _, err := s.AddPendingAttempt(noChild, 0, 50); err != nil {
		t.Fatalf("попытка без ребёнка: %v", err)
	}
	got, err := s.ListPending()
	if err != nil {
		t.Fatal(err)
	}
	want := []PendingProgram{
		{Path: `c:\temp\tool.exe`, Name: "Tool", WasCategory: ProgramGames, FirstChildID: anya, FirstAttemptAt: 400, LastAttemptAt: 400, Attempts: 1,
			Children: []PendingChild{{ChildID: anya, Attempts: 1, LastAttemptAt: 400}}},
		{Path: pathGame, Name: "SuperGame", Publisher: "Без подписи издателя", Description: "Super Game", FirstChildID: misha, FirstAttemptAt: 100,
			LastAttemptAt: 300, Attempts: 4,
			Children: []PendingChild{{ChildID: misha, Attempts: 3, LastAttemptAt: 300}, {ChildID: anya, Attempts: 1, LastAttemptAt: 200}}},
		{Path: `c:\temp\old.exe`, Name: "Old", FirstAttemptAt: 50, LastAttemptAt: 50, Attempts: 1, Children: []PendingChild{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListPending =\n%+v\nwant\n%+v", got, want)
	}
	if b, _ := json.Marshal(got[2].Children); string(b) != "[]" {
		t.Errorf("дети без попыток = %s, want []", b)
	}
	for name, bad := range map[string]PendingProgram{"пустой путь": {Name: "x"}, "пустое имя": {Path: `c:\x.exe`}} {
		if _, err := s.AddPendingAttempt(bad, misha, 1); !errors.Is(err, ErrInvalid) {
			t.Errorf("AddPendingAttempt(%s) = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := s.AddPendingAttempt(game, 999, 500); err == nil {
		t.Error("попытка несуществующего ребёнка прошла, want ошибку внешнего ключа")
	}
	if n := rawNull(t, s, `SELECT attempts FROM pending_programs WHERE exe_path = ?`, pathGame); n.Int64 != 4 {
		t.Errorf("попытка с ошибкой изменила счётчик: %d, want 4 (одна транзакция)", n.Int64)
	}
}

// Ловит: решение родителя (ResolvePending) не возвращает строку для «Отменить», оставляет
// попытки по детям (каскад, внешние ключи), «Отменить» (RestorePending) теряет попытки, детей
// или прибавленные за 10 с новые попытки.
func TestResolveAndRestorePending(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	anya := mustChild(t, s, "S-2", "Аня")
	game := PendingProgram{Path: pathGame, Name: "SuperGame", Publisher: "P"}
	for _, a := range []struct{ child, at int64 }{{misha, 100}, {anya, 200}, {misha, 300}} {
		if _, err := s.AddPendingAttempt(game, a.child, a.at); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := s.ListPending()
	if p, ok, err := s.ResolvePending(`c:\нет.exe`); err != nil || ok || p.Path != "" {
		t.Fatalf("ResolvePending несуществующей = %+v, %v, %v; want пусто, false, nil", p, ok, err)
	}
	gone, ok, err := s.ResolvePending(pathGame)
	if err != nil || !ok {
		t.Fatalf("ResolvePending = %v, %v", ok, err)
	}
	if !reflect.DeepEqual(gone, before[0]) {
		t.Fatalf("ResolvePending вернул %+v, want %+v", gone, before[0])
	}
	if n := rawNull(t, s, `SELECT count(*) FROM pending_attempts`); n.Int64 != 0 {
		t.Errorf("попыток после решения %d, want 0 (каскад)", n.Int64)
	}
	if got, _ := s.ListPending(); len(got) != 0 {
		t.Errorf("ListPending после решения = %+v", got)
	}

	if err := s.RestorePending(gone); err != nil {
		t.Fatalf("RestorePending: %v", err)
	}
	if got, _ := s.ListPending(); !reflect.DeepEqual(got, before) {
		t.Fatalf("после «Отменить» = %+v, want %+v", got, before)
	}

	// Ребёнок успел попробовать снова, пока шли 10 с «Отменить»: попытки складываются.
	gone, _, _ = s.ResolvePending(pathGame)
	if _, err := s.AddPendingAttempt(game, anya, 400); err != nil {
		t.Fatal(err)
	}
	if err := s.RestorePending(gone); err != nil {
		t.Fatal(err)
	}
	got, _ := s.ListPending()
	want := PendingProgram{Path: pathGame, Name: "SuperGame", Publisher: "P", FirstChildID: misha, FirstAttemptAt: 100, LastAttemptAt: 400, Attempts: 4,
		Children: []PendingChild{{ChildID: anya, Attempts: 2, LastAttemptAt: 400}, {ChildID: misha, Attempts: 2, LastAttemptAt: 300}}}
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("слияние = %+v, want %+v", got, want)
	}
	if err := s.RestorePending(PendingProgram{Name: "x"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("RestorePending без пути = %v, want ErrInvalid", err)
	}
}

// Ловит: учёт по программам (RM3-6) не складывается при повторной записи, пишет «Системное»
// (CHECK запрещает), отрицательные секунды или мусорный день; время за неделю (FR-20) берёт
// дни раньше начала недели, сайты или не суммирует детей.
func TestUsageAndProgramSeconds(t *testing.T) {
	s := openTemp(t)
	misha := mustChild(t, s, "S-1", "Миша")
	anya := mustChild(t, s, "S-2", "Аня")
	for _, u := range []struct {
		child                        int64
		day, category, kind, subject string
		seconds                      int
	}{
		{misha, "2026-10-26", UsageGames, SubjectProgram, pathGame, 600},
		{misha, "2026-10-26", UsageGames, SubjectProgram, pathGame, 60},
		{anya, "2026-10-27", UsageGames, SubjectProgram, pathGame, 40},
		{misha, "2026-10-25", UsageGames, SubjectProgram, pathGame, 1000},
		{misha, "2026-10-26", UsageOther, SubjectProgram, pathBrave, 30},
		{misha, "2026-10-26", UsageVideoSocial, SubjectSite, "youtube.com", 500},
		{misha, "2026-10-26", UsageStudy, SubjectProgram, pathWord, 0},
	} {
		if err := s.AddUsage(u.child, u.day, u.category, u.kind, u.subject, u.seconds); err != nil {
			t.Fatalf("AddUsage(%+v): %v", u, err)
		}
	}
	if v := rawNull(t, s, `SELECT seconds FROM usage_daily WHERE child_id = ? AND day = '2026-10-26' AND subject = ?`, misha, pathGame); v.Int64 != 660 {
		t.Errorf("сложение = %d, want 660", v.Int64)
	}
	if n := rawNull(t, s, `SELECT count(*) FROM usage_daily WHERE subject = ?`, pathWord); n.Int64 != 0 {
		t.Errorf("ноль секунд записан строкой: %d", n.Int64)
	}
	got, err := s.ProgramSeconds("2026-10-26")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]int{pathGame: 700, pathBrave: 30}; !reflect.DeepEqual(got, want) {
		t.Errorf("ProgramSeconds = %v, want %v", got, want)
	}
	if got, err := s.ProgramSeconds("2026-11-01"); err != nil || got == nil || len(got) != 0 {
		t.Errorf("ProgramSeconds без данных = %#v, %v; want пустая карта", got, err)
	}
	for name, call := range map[string]func() error{
		"Системное":      func() error { return s.AddUsage(misha, "2026-10-26", "system", SubjectProgram, pathGame, 1) },
		"вид":            func() error { return s.AddUsage(misha, "2026-10-26", UsageGames, "window", pathGame, 1) },
		"пустой предмет": func() error { return s.AddUsage(misha, "2026-10-26", UsageGames, SubjectProgram, "", 1) },
		"минус секунды":  func() error { return s.AddUsage(misha, "2026-10-26", UsageGames, SubjectProgram, pathGame, -5) },
		"день":           func() error { return s.AddUsage(misha, "26.10.2026", UsageGames, SubjectProgram, pathGame, 1) },
		"неделя":         func() error { _, err := s.ProgramSeconds("вчера"); return err },
	} {
		if err := call(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
}

// Ловит: состояние дня и учёт по программам пишутся порознь (RM3-6, G10): после сбоя посреди
// записи день сохранён, а программы нет (или наоборот), и «Программы» расходятся с «Сегодня».
func TestSaveDayAndUsageIsOneTransaction(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1", "Миша")
	st := core.DayState{Day: day1, TotalS: 700, GamesS: 600, StudyS: 50}
	usage := []UsageRow{
		{Category: UsageGames, Kind: SubjectProgram, Subject: pathGame, Seconds: 600},
		{Day: "2026-10-18", Category: UsageOther, Kind: SubjectProgram, Subject: pathBrave, Seconds: 100},
		{Category: UsageStudy, Kind: SubjectProgram, Subject: pathWord, Seconds: 0},
	}
	if err := s.SaveDayAndUsage(id, st, usage); err != nil {
		t.Fatalf("SaveDayAndUsage: %v", err)
	}
	if got, _ := s.LoadDayState(id, day1); got != st {
		t.Errorf("день = %+v, want %+v", got, st)
	}
	if v := rawNull(t, s, `SELECT seconds FROM usage_daily WHERE day = ? AND subject = ?`, day1, pathGame); v.Int64 != 600 {
		t.Errorf("игра за день = %d, want 600 (пустой день строки — день состояния)", v.Int64)
	}
	if v := rawNull(t, s, `SELECT seconds FROM usage_daily WHERE day = '2026-10-18' AND subject = ?`, pathBrave); v.Int64 != 100 {
		t.Errorf("Brave за прошлый день = %d, want 100", v.Int64)
	}
	if err := s.SaveDayAndUsage(id, core.DayState{Day: day1, TotalS: 701, GamesS: 601}, []UsageRow{{Category: UsageGames, Kind: SubjectProgram, Subject: pathGame, Seconds: 1}}); err != nil {
		t.Fatal(err)
	}
	if v := rawNull(t, s, `SELECT seconds FROM usage_daily WHERE day = ? AND subject = ?`, day1, pathGame); v.Int64 != 601 {
		t.Errorf("после второй порции = %d, want 601 (сложение)", v.Int64)
	}

	// Сбой базы на второй строке: откатывается всё.
	if _, err := s.db.Exec(`CREATE TRIGGER boom BEFORE INSERT ON usage_daily WHEN NEW.subject = 'boom' BEGIN SELECT RAISE(ABORT, 'сбой'); END`); err != nil {
		t.Fatal(err)
	}
	beforeDay, _ := s.LoadDayState(id, day1)
	err := s.SaveDayAndUsage(id, core.DayState{Day: day1, TotalS: 9999, GamesS: 9999}, []UsageRow{
		{Category: UsageGames, Kind: SubjectProgram, Subject: pathGame, Seconds: 500},
		{Category: UsageGames, Kind: SubjectProgram, Subject: "boom", Seconds: 1},
	})
	if err == nil {
		t.Fatal("SaveDayAndUsage со сбоем прошёл")
	}
	if got, _ := s.LoadDayState(id, day1); got != beforeDay {
		t.Errorf("день после отката = %+v, want %+v", got, beforeDay)
	}
	if v := rawNull(t, s, `SELECT seconds FROM usage_daily WHERE day = ? AND subject = ?`, day1, pathGame); v.Int64 != 601 {
		t.Errorf("учёт после отката = %d, want 601", v.Int64)
	}

	// Проверка до записи: мусор не пишет ни дня, ни учёта.
	for name, bad := range map[string][]UsageRow{
		"Системное": {{Category: "system", Kind: SubjectProgram, Subject: pathGame, Seconds: 1}},
		"день":      {{Day: "вчера", Category: UsageGames, Kind: SubjectProgram, Subject: pathGame, Seconds: 1}},
	} {
		if err := s.SaveDayAndUsage(id, core.DayState{Day: day1, TotalS: 1}, bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
	if err := s.SaveDayAndUsage(id, core.DayState{Day: "плохой"}, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("плохой день состояния: %v, want ErrInvalid", err)
	}
	if got, _ := s.LoadDayState(id, day1); got != beforeDay {
		t.Errorf("день после отказов = %+v, want %+v", got, beforeDay)
	}
}

// Ловит: новые методы M3 глотают ошибку закрытой базы и отвечают «пусто» (L-129).
func TestProgramMethodsReturnErrorsWhenStoreClosed(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1", "Миша")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	p := braveSnapshot()
	calls := map[string]func() error{
		"UpsertProgram":       func() error { return s.UpsertProgram(p) },
		"SeedPrograms":        func() error { _, err := s.SeedPrograms([]Program{p}); return err },
		"GetProgram":          func() error { _, _, err := s.GetProgram(pathBrave); return err },
		"ListPrograms":        func() error { _, err := s.ListPrograms(); return err },
		"SetProgramsCategory": func() error { _, err := s.SetProgramsCategory([]string{pathBrave}, ProgramGames, 1); return err },
		"RestoreProgram":      func() error { return s.RestoreProgram(p) },
		"DeleteProgram":       func() error { return s.DeleteProgram(pathBrave) },
		"AddPendingAttempt": func() error {
			_, err := s.AddPendingAttempt(PendingProgram{Path: pathGame, Name: "g"}, id, 1)
			return err
		},
		"ListPending":     func() error { _, err := s.ListPending(); return err },
		"ResolvePending":  func() error { _, _, err := s.ResolvePending(pathGame); return err },
		"RestorePending":  func() error { return s.RestorePending(PendingProgram{Path: pathGame, Name: "g", Attempts: 1}) },
		"AddUsage":        func() error { return s.AddUsage(id, day1, UsageGames, SubjectProgram, pathGame, 1) },
		"SaveDayAndUsage": func() error { return s.SaveDayAndUsage(id, core.DayState{Day: day1}, nil) },
		"ProgramSeconds":  func() error { _, err := s.ProgramSeconds(day1); return err },
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s на закрытом хранилище вернул nil, want ошибку", name)
		}
	}
}
