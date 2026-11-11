package service

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Перечень видов событий и группы журнала (FR-24, RM5-5).

// Ловит: вид из перечня не попал ни в одну группу или попал в две; в группе вид, которого нет в
// перечне; повтор в перечне; набор «Нарушений» разошёлся с VIOLATION_KINDS окна;
// child_admin считается нарушением.
func TestJournalGroupsCoverEventKinds(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range allEventKinds {
		if k == "" {
			t.Error("пустой вид в перечне")
		}
		if seen[k] {
			t.Errorf("вид %q в перечне дважды", k)
		}
		seen[k] = true
	}
	in := map[string][]string{}
	for g, kinds := range journalGroups {
		for _, k := range kinds {
			if !seen[k] {
				t.Errorf("в группе %q вид %q, которого нет в перечне allEventKinds", g, k)
			}
			in[k] = append(in[k], g)
		}
	}
	for _, k := range allEventKinds {
		if n := len(in[k]); n != 1 {
			t.Errorf("вид %q в группах %v; want ровно в одной", k, in[k])
		}
	}
	groups := []string{}
	for g := range journalGroups {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	if got := strings.Join(groups, ","); got != "programs,settings,system,time,violations" {
		t.Errorf("группы %s", got)
	}
	if got := strings.Join(journalGroups[ipc.JournalGroupViolations], ","); got != "bypass,program_forbidden_try,site_forbidden_try" {
		t.Errorf("«Нарушения» = %s; want тот же набор, что VIOLATION_KINDS окна", got)
	}
	want := map[string]string{
		"child_admin": "settings", "child_admin_off": "settings", "stats_cleared": "settings", "journal_cleared": "settings",
		"promo_generate": "time", "promo_history_cleared": "time", "promo_redeem": "time", "extra_time": "time",
		"pause": "time", "rest_break": "time", "bonus_granted": "time",
		"program_pending": "programs", "site_added": "programs", "brave_restored": "programs",
		"internal_error": "system", "db_recovered": "system", "safe_mode": "system",
		"password_failed": "settings", "settings_changed": "settings",
	}
	for k, g := range want {
		if len(in[k]) != 1 || in[k][0] != g {
			t.Errorf("вид %q в группах %v; want %q", k, in[k], g)
		}
	}
}

// Ловит (RM5-5): новое место записи в журнал с литералом вида или константой не из перечня;
// константа вида объявлена вне eventkinds.go; вид ядра (core.Ev*) не внесён в перечень; вид из
// переменной без пометки //eventkind:dynamic; новое неразрешимое место без отдельного теста.
func TestEventKindsSourceScan(t *testing.T) {
	svc := parsePkg(t, ".")
	corePkg := parsePkg(t, filepath.Join("..", "core"))

	// Перечень: элементы allEventKinds в исходнике.
	listed := map[string]bool{}
	for _, f := range svc.files {
		ast.Inspect(f, func(n ast.Node) bool {
			vs, ok := n.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "allEventKinds" || len(vs.Values) != 1 {
				return true
			}
			for _, el := range vs.Values[0].(*ast.CompositeLit).Elts {
				listed[types.ExprString(el)] = true
			}
			return false
		})
	}
	if len(listed) < 40 {
		t.Fatalf("в перечне allEventKinds найдено %d видов: разбор исходников не нашёл перечень", len(listed))
	}
	// Константы видов службы — только в eventkinds.go и все в перечне; виды ядра Ev* — все в перечне.
	for name, f := range svc.byName {
		for _, c := range stringConsts(f) {
			if !strings.HasPrefix(c, "ev") {
				continue
			}
			if filepath.Base(name) != "eventkinds.go" {
				t.Errorf("%s: константа вида %s вне eventkinds.go (один перечень, RM5-5)", name, c)
			}
			if !listed[c] {
				t.Errorf("константа %s не внесена в allEventKinds", c)
			}
		}
	}
	coreKinds := map[string]bool{}
	for _, f := range corePkg.files {
		for _, c := range stringConsts(f) {
			if strings.HasPrefix(c, "Ev") {
				coreKinds[c] = true
				if !listed["core."+c] {
					t.Errorf("вид ядра core.%s не внесён в allEventKinds", c)
				}
			}
		}
	}
	for k := range listed {
		if name, ok := strings.CutPrefix(k, "core."); ok && !coreKinds[name] {
			t.Errorf("в перечне %s, такой константы вида в ядре нет", k)
		}
	}

	sites, dynamic := 0, map[string]bool{}
	scan := func(p *pkgSrc, allowed func(ast.Expr) bool, isWrite func(ast.Node) (ast.Expr, bool)) {
		for _, f := range p.files {
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					kind, ok := isWrite(n)
					if !ok {
						return true
					}
					sites++
					pos := p.fset.Position(n.Pos())
					switch res := resolveKind(fn, kind, allowed); res {
					case "":
					case "dynamic":
						if !p.marked(pos) {
							t.Errorf("%s: вид события %s не константа из перечня и не прослеживается; нужна константа или пометка //eventkind:dynamic", pos, types.ExprString(kind))
						} else {
							dynamic[funcName(fn)] = true
						}
					default:
						t.Errorf("%s: вид события %s", pos, res)
					}
					return true
				})
			}
		}
	}
	listedExpr := func(e ast.Expr) bool { return listed[types.ExprString(e)] }
	scan(svc, listedExpr, func(n ast.Node) (ast.Expr, bool) {
		switch x := n.(type) {
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Write" || len(x.Args) != 3 || !isJournal(sel.X) {
				return nil, false
			}
			return x.Args[1], true
		case *ast.CompositeLit:
			if s := types.ExprString(x.Type); s == "StoredEvent" || s == "core.Event" {
				return kindField(x)
			}
		}
		return nil, false
	})
	scan(corePkg, func(e ast.Expr) bool { return listed["core."+types.ExprString(e)] }, func(n ast.Node) (ast.Expr, bool) {
		if x, ok := n.(*ast.CompositeLit); ok && types.ExprString(x.Type) == "Event" {
			return kindField(x)
		}
		return nil, false
	})
	if sites < 50 {
		t.Errorf("мест записи событий найдено %d: разбор исходников их не видит", sites)
	}
	var dyn []string
	for k := range dynamic {
		dyn = append(dyn, k)
	}
	sort.Strings(dyn)
	// Каждое неразрешимое место — с отдельным тестом: writeEvents — TestDynamicEventKindsFromCore,
	// StoreJournal.Write — сам приёмник журнала (вид проверяют места вызова Write выше).
	if got := strings.Join(dyn, ","); got != "Service.writeEvents,StoreJournal.Write" {
		t.Errorf("места с //eventkind:dynamic: %s; want Service.writeEvents,StoreJournal.Write (новое место — отдельный тест)", got)
	}
}

// Ловит (RM5-5, пометка //eventkind:dynamic у writeEvents): событие ядра пишется в журнал видом
// не из перечня или вид подменён по дороге.
func TestDynamicEventKindsFromCore(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured(), withClockAt(time.Date(2026, 11, 24, 23, 50, 0, 0, tzMSK)))
	h.start()
	h.childLogsIn()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	h.ok(adminPeer(), tok, ipc.CmdAddExtra, ipc.AddExtraRequest{ChildID: id, Minutes: 30}, nil)
	h.poll(activeSample, ipc.GuardEvent{Kind: ipc.GuardEvBypass, Detail: "Закрыт значок программы"})
	h.ok(adminPeer(), tok, ipc.CmdPause, ipc.PauseRequest{ChildID: id, Preset: ipc.PauseMin15}, nil)
	h.run(11*60, activeSample) // полночь: итог дня
	inList := map[string]bool{}
	for _, k := range allEventKinds {
		inList[k] = true
	}
	got := map[string]bool{}
	h.store.mu.Lock()
	for _, e := range h.store.events {
		got[e.Kind] = true
		if !inList[e.Kind] {
			t.Errorf("в журнале вид %q не из перечня", e.Kind)
		}
	}
	h.store.mu.Unlock()
	for _, k := range []string{core.EvExtraTime, core.EvPause, core.EvBypass, core.EvDayClosed} {
		if !got[k] {
			t.Errorf("вида %q нет в журнале: сценарий не дошёл до него (есть %v)", k, got)
		}
	}
}

// pkgSrc — разобранные исходники пакета без тестовых файлов.
type pkgSrc struct {
	fset   *token.FileSet
	files  []*ast.File
	byName map[string]*ast.File
	marks  map[string]map[int]bool // файл → строки с //eventkind:dynamic
}

func parsePkg(t *testing.T, dir string) *pkgSrc {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := &pkgSrc{fset: token.NewFileSet(), byName: map[string]*ast.File{}, marks: map[string]map[int]bool{}}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		f, err := parser.ParseFile(p.fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		p.files = append(p.files, f)
		p.byName[path] = f
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if strings.HasPrefix(c.Text, "//eventkind:dynamic ") {
					pos := p.fset.Position(c.Pos())
					if p.marks[pos.Filename] == nil {
						p.marks[pos.Filename] = map[int]bool{}
					}
					p.marks[pos.Filename][pos.Line] = true
				}
			}
		}
	}
	if len(p.files) == 0 {
		t.Fatalf("в %s нет исходников", dir)
	}
	return p
}

// marked — пометка //eventkind:dynamic <откуда вид> на строке места или строкой выше.
func (p *pkgSrc) marked(pos token.Position) bool {
	m := p.marks[pos.Filename]
	return m[pos.Line] || m[pos.Line-1]
}

// stringConsts — имена констант верхнего уровня файла со строковым значением.
func stringConsts(f *ast.File) []string {
	var out []string
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, sp := range gd.Specs {
			vs := sp.(*ast.ValueSpec)
			for i, n := range vs.Names {
				if i < len(vs.Values) {
					if bl, ok := vs.Values[i].(*ast.BasicLit); ok && bl.Kind == token.STRING {
						out = append(out, n.Name)
					}
				}
			}
		}
	}
	return out
}

// isJournal — приёмник s.journal (или journal).
func isJournal(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "journal"
	case *ast.Ident:
		return x.Name == "journal"
	}
	return false
}

// kindField — значение поля Kind литерала события; литерал без поля Kind (позиционный) — его тип,
// чтобы проверка назвала место.
func kindField(c *ast.CompositeLit) (ast.Expr, bool) {
	if len(c.Elts) == 0 {
		return nil, false
	}
	for _, el := range c.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Kind" {
				return kv.Value, true
			}
		}
	}
	return c, true
}

// resolveKind: "" — вид константа из перечня (сам или через все присваивания переменной в той же
// функции); "dynamic" — не прослеживается; иначе — что не так.
func resolveKind(fn *ast.FuncDecl, kind ast.Expr, allowed func(ast.Expr) bool) string {
	if allowed(kind) {
		return ""
	}
	switch x := kind.(type) {
	case *ast.BasicLit:
		return "литерал " + x.Value + ": заведите константу в eventkinds.go и внесите в перечень и группу"
	case *ast.CompositeLit:
		return "литерал события без поля Kind"
	case *ast.Ident:
		var rhs []ast.Expr
		unresolved := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch s := n.(type) {
			case *ast.AssignStmt:
				for i, l := range s.Lhs {
					if id, ok := l.(*ast.Ident); ok && id.Name == x.Name {
						if len(s.Rhs) == len(s.Lhs) {
							rhs = append(rhs, s.Rhs[i])
						} else {
							unresolved = true
						}
					}
				}
			case *ast.ValueSpec:
				for i, id := range s.Names {
					if id.Name == x.Name && i < len(s.Values) {
						rhs = append(rhs, s.Values[i])
					}
				}
			}
			return true
		})
		if unresolved || len(rhs) == 0 {
			return "dynamic"
		}
		for _, r := range rhs {
			if !allowed(r) {
				return "переменная " + x.Name + " = " + types.ExprString(r) + ": не константа из перечня"
			}
		}
		return ""
	case *ast.SelectorExpr:
		if _, ok := x.X.(*ast.Ident); ok && strings.HasPrefix(x.Sel.Name, "Ev") {
			return types.ExprString(x) + " нет в перечне allEventKinds"
		}
	}
	return "dynamic"
}

// funcName — «Тип.метод» или имя функции.
func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	t := fn.Recv.List[0].Type
	if st, ok := t.(*ast.StarExpr); ok {
		t = st.X
	}
	return types.ExprString(t) + "." + fn.Name.Name
}

// Ловит (FR-24, RM5-5): группа «Что» отдаёт чужие виды; вид старой версии (нет в перечне) попадает
// в группу; «Все события» отбирает по виду; неизвестная группа не отказ.
func TestListEventsGroups(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	h.store.mu.Lock()
	h.store.events = nil
	h.store.mu.Unlock()
	kinds := append(append([]string{}, allEventKinds...), "old_kind")
	for i, k := range kinds {
		if err := h.store.AppendEvent(StoredEvent{ChildID: id, AtMs: int64(1000 + i), Kind: k, Details: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
	}
	list := func(group string) []string {
		t.Helper()
		var rep ipc.ListEventsReply
		h.ok(adminPeer(), tok, ipc.CmdListEvents, ipc.ListEventsRequest{Group: group}, &rep)
		var out []string
		for _, e := range rep.Events {
			out = append(out, e.Kind)
		}
		sort.Strings(out)
		return out
	}
	total := 0
	for g, want := range journalGroups {
		got := list(g)
		w := append([]string{}, want...)
		sort.Strings(w)
		if strings.Join(got, ",") != strings.Join(w, ",") {
			t.Errorf("группа %q: %v; want %v", g, got, w)
		}
		total += len(got)
	}
	all := list(ipc.JournalGroupAll)
	if len(all) != len(kinds) || total != len(allEventKinds) {
		t.Errorf("«Все события» %d записей, в группах %d; want %d и %d (old_kind только во «Всех»)", len(all), total, len(kinds), len(allEventKinds))
	}
	h.store.mu.Lock()
	q := h.store.evQueries[len(h.store.evQueries)-1]
	h.store.mu.Unlock()
	if q.Kinds != nil {
		t.Errorf("«Все события» отбирают по виду: %v", q.Kinds)
	}
	rep := h.do(adminPeer(), tok, ipc.CmdListEvents, ipc.ListEventsRequest{Group: "nope"})
	if rep.OK || rep.Error != ipc.TextBadRequest {
		t.Errorf("неизвестная группа: ok=%v, ошибка %q; want отказ «%s»", rep.OK, rep.Error, ipc.TextBadRequest)
	}
}

// Ловит (RM5-5): ребёнок, период, группа и курсор отбираются службой после чтения, а не уходят в
// запрос хранилища; More считается без запроса Limit + 1.
func TestListEventsQueryToStore(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	h.ok(adminPeer(), tok, ipc.CmdListEvents, ipc.ListEventsRequest{
		ChildID: id, FromMs: 1_000, ToMs: 9_000, Group: ipc.JournalGroupViolations, BeforeAtMs: 5_000, BeforeID: 77, Limit: 50,
	}, nil)
	h.store.mu.Lock()
	q := h.store.evQueries[len(h.store.evQueries)-1]
	h.store.mu.Unlock()
	want := EventQuery{ChildID: id, FromMs: 1_000, ToMs: 9_000, Kinds: []string{"bypass", "program_forbidden_try", "site_forbidden_try"},
		BeforeAtMs: 5_000, BeforeID: 77, Limit: 51}
	if fmt.Sprintf("%+v", q) != fmt.Sprintf("%+v", want) {
		t.Errorf("запрос хранилища %+v; want %+v", q, want)
	}
}

// Ловит (RM5-5, «Показать ещё»): страница повторяет или теряет записи; More врёт на последней
// странице; ID записи не отдаётся; на потолке 1000 More не видит 1001-ю запись (хранилище больше
// 1000 строк за раз не отдаёт).
func TestListEventsCursorMore(t *testing.T) {
	h := newHarness(t, withChild(childSID, "Миша", 120), configured())
	h.start()
	tok := h.login(adminPeer())
	id := h.childID(childSID)
	h.store.mu.Lock()
	h.store.events = nil
	h.store.mu.Unlock()
	for i := 0; i < 450; i++ {
		_ = h.store.AppendEvent(StoredEvent{ChildID: id, AtMs: int64(1000 + i/3), Kind: core.EvExtraTime, Details: map[string]any{"minutes": 5}})
	}
	req := ipc.ListEventsRequest{ChildID: id, Limit: 200}
	seen := map[int64]bool{}
	var pages []string
	for page := 0; page < 5; page++ {
		var rep ipc.ListEventsReply
		h.ok(adminPeer(), tok, ipc.CmdListEvents, req, &rep)
		pages = append(pages, fmt.Sprintf("%d/%v", len(rep.Events), rep.More))
		for _, e := range rep.Events {
			if e.ID == 0 || seen[e.ID] {
				t.Fatalf("запись %+v: нет номера или повтор", e)
			}
			seen[e.ID] = true
		}
		if !rep.More {
			break
		}
		last := rep.Events[len(rep.Events)-1]
		req.BeforeAtMs, req.BeforeID = last.AtMs, last.ID
	}
	if got := strings.Join(pages, " "); got != "200/true 200/true 50/false" || len(seen) != 450 {
		t.Errorf("страницы %s, всего %d; want 200/true 200/true 50/false и 450", got, len(seen))
	}

	for _, n := range []int{1000, 1001} {
		h.store.mu.Lock()
		h.store.events = nil
		h.store.mu.Unlock()
		for i := 0; i < n; i++ {
			_ = h.store.AppendEvent(StoredEvent{ChildID: id, AtMs: int64(1000 + i), Kind: core.EvExtraTime, Details: map[string]any{"minutes": 5}})
		}
		var rep ipc.ListEventsReply
		h.ok(adminPeer(), tok, ipc.CmdListEvents, ipc.ListEventsRequest{Limit: 1000}, &rep)
		if len(rep.Events) != 1000 || rep.More != (n == 1001) {
			t.Errorf("%d записей, страница 1000: получено %d, more %v; want 1000 и %v", n, len(rep.Events), rep.More, n == 1001)
		}
	}
}

// Ловит (RM5-5): «Нарушения» журнала разошлись с VIOLATION_KINDS окна (frontend/src/lib/journal.ts):
// «Нарушения сегодня: N» на главной и фильтр журнала считают разное.
func TestViolationGroupMatchesWindow(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "lib", "journal.ts"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(src), "VIOLATION_KINDS = [")
	list, _, ok2 := strings.Cut(rest, "]")
	if !ok || !ok2 {
		t.Fatal("в journal.ts не найден VIOLATION_KINDS")
	}
	var win []string
	for _, k := range strings.Split(list, ",") {
		win = append(win, strings.Trim(strings.TrimSpace(k), `"`))
	}
	if got, want := strings.Join(win, ","), strings.Join(journalGroups[ipc.JournalGroupViolations], ","); got != want {
		t.Errorf("VIOLATION_KINDS окна %s; группа «Нарушения» службы %s", got, want)
	}
}
