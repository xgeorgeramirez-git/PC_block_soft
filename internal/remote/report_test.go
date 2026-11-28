package remote

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// «Статистика за сегодня» в отчёте (FR-43, Ruling FB27-2): размер тела ≤ 14 КБ (сервер связи
// принимает ≤ 16 КБ), урезание списков, игры — последними.

// bigToday — today с programs и sites строками; каждая третья программа — игра, названия длинные
// (кириллица, по 2 байта) и с < > & (json.Marshal пишет их шестью байтами).
func bigToday(programs, sites int) *TodayReport {
	t := &TodayReport{FirstMs: 1763460000000, NowAt: true, SinceMs: 1763470000000, UsedSec: 7200, LeftSec: 1800,
		GamesSec: 3600, StudySec: 900, VideoSec: 600, Violations: 3, Bypass: 1}
	for i := 0; i < programs; i++ {
		cat := ItemOther
		if i%3 == 0 {
			cat = ItemGame
		}
		t.Programs = append(t.Programs, TimeItem{Name: fmt.Sprintf("Очень длинное название программы <%d> & ещё немного слов сверху", i), Cat: cat, Sec: int64(100 + i)})
	}
	for i := 0; i < sites; i++ {
		t.Sites = append(t.Sites, TimeItem{Name: fmt.Sprintf("очень-длинный-поддомен-%d.очень-длинный-домен-сайта-для-проверки.рф", i), Cat: ItemVideo, Sec: int64(50 + i)})
	}
	return t
}

func bigReport(children, programs, sites int) Report {
	rep := Report{V: 1, T: TypeReport, PC: "pc-x", Phone: "ph1", TS: 1763490000000, Children: []ChildReport{}}
	for i := 0; i < children; i++ {
		rep.Children = append(rep.Children, ChildReport{ID: int64(i + 1), Name: fmt.Sprintf("Ребёнок %d", i+1), UsedSec: 3600, LeftSec: 600,
			Forbid: ForbidAhead, Today: bigToday(programs, sites)})
	}
	for i := 0; i < 10; i++ {
		rep.Notes = append(rep.Notes, NoteReport{ID: fmt.Sprintf("AAECAwQFBgcICQoLDA0OD%d", i), Child: 1, State: NoteRead, AtMs: 1763490000000})
	}
	return rep
}

// signedBodyLen — настоящая длина тела pc/report, как её пишет Client.SendReport.
func signedBodyLen(t *testing.T, raw []byte) int {
	t.Helper()
	s, err := Sign(mustKey(t), raw)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"body": s})
	if len(body) > reportBodySize(len(raw)) {
		t.Fatalf("оценка размера тела %d меньше настоящего %d", reportBodySize(len(raw)), len(body))
	}
	return len(body)
}

// checkFitted — отчёт после урезания: тело ≤ MaxReportBody, у всех детей одна ступень, more =
// невошедшие, по убыванию, названия ≤ 60 знаков, пустые списки — [], игры вытесняются последними.
// Возвращает ступень.
func checkFitted(t *testing.T, raw []byte, programs, sites int) int {
	t.Helper()
	if n := signedBodyLen(t, raw); n > MaxReportBody {
		t.Fatalf("тело %d байт, предел %d", n, MaxReportBody)
	}
	if strings.Contains(string(raw), "null") {
		t.Fatalf("в отчёте null: %.200s", raw)
	}
	var rep Report
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	step := -1
	for _, c := range rep.Children {
		if c.Today == nil {
			t.Fatalf("%s: today пропал", c.Name)
		}
		td := c.Today
		if step == -1 {
			for _, s := range reportSteps {
				if len(td.Programs) == min(programs, s, MaxTodayPrograms) && len(td.Sites) == min(sites, s, MaxTodaySites) {
					step = s
					break
				}
			}
			if step == -1 {
				t.Fatalf("%s: программ %d, сайтов %d — не ступень из %v", c.Name, len(td.Programs), len(td.Sites), reportSteps)
			}
		}
		if len(td.Programs) != min(programs, step, MaxTodayPrograms) || len(td.Sites) != min(sites, step, MaxTodaySites) {
			t.Fatalf("%s: урезано не одинаково: программ %d, сайтов %d, ступень %d", c.Name, len(td.Programs), len(td.Sites), step)
		}
		if td.More.Programs != programs-len(td.Programs) || td.More.Sites != sites-len(td.Sites) {
			t.Fatalf("%s: more %+v при %d/%d", c.Name, td.More, len(td.Programs), len(td.Sites))
		}
		games, keptGames := (programs+2)/3, 0
		for _, list := range [][]TimeItem{td.Programs, td.Sites} {
			for i, it := range list {
				if utf8.RuneCountInString(it.Name) > MaxItemNameRunes {
					t.Fatalf("название длиннее %d знаков: %q", MaxItemNameRunes, it.Name)
				}
				if i > 0 && list[i-1].Sec < it.Sec {
					t.Fatalf("не по убыванию: %v", list)
				}
				if it.Cat == ItemGame {
					keptGames++
				}
			}
		}
		if keptGames != min(games, len(td.Programs)) {
			t.Fatalf("%s: игр в списке %d из %d при %d строках: игры вытеснены раньше прочих", c.Name, keptGames, games, len(td.Programs))
		}
	}
	return step
}

// Ловит: 6 детей × 50 программ × 50 сайтов с длинными названиями не помещаются в 14 КБ (сервер
// связи отвергает тело больше 16 КБ, и до телефона не доходит даже остаток); урезание у детей
// разное; more не сходится; названия длиннее 60 знаков или режутся посреди буквы; урезано больше
// нужного.
func TestFitReportSixChildren(t *testing.T) {
	rep := bigReport(6, 50, 50)
	raw, err := fitReport(rep, MaxReportBody)
	if err != nil {
		t.Fatal(err)
	}
	step := checkFitted(t, raw, 50, 50)
	if step == MaxTodayPrograms {
		t.Fatal("6 × 20 × 10 длинных названий не должно поместиться в 14 КБ: проверка урезания ничего не проверяет")
	}
	prev := reportSteps[0]
	for _, s := range reportSteps {
		if s == step {
			break
		}
		prev = s
	}
	bigger, _ := json.Marshal(capToday(normalizeToday(rep), prev))
	if reportBodySize(len(bigger)) <= MaxReportBody {
		t.Fatalf("ступень %d тоже помещалась, а урезано до %d", prev, step)
	}
	if len(rep.Children[0].Today.Programs) != 50 {
		t.Fatal("fitReport изменил входной отчёт")
	}
}

// Ловит: один ребёнок с длинными списками получает больше 20 программ и 10 сайтов; игры, которые
// короче прочих программ, выпадают из списка (на телефоне пустой блок «Во что играл»).
func TestFitReportCapsKeepGames(t *testing.T) {
	td := &TodayReport{}
	for i := 0; i < 25; i++ {
		td.Programs = append(td.Programs, TimeItem{Name: fmt.Sprintf("Программа %02d", i), Cat: ItemOther, Sec: int64(1000 + i)})
	}
	for i := 0; i < 3; i++ {
		td.Programs = append(td.Programs, TimeItem{Name: fmt.Sprintf("Игра %d", i), Cat: ItemGame, Sec: int64(10 + i)})
	}
	for i := 0; i < 15; i++ {
		td.Sites = append(td.Sites, TimeItem{Name: fmt.Sprintf("site%02d.example", i), Cat: ItemOther, Sec: int64(i + 1)})
	}
	raw, err := fitReport(Report{V: 1, T: TypeReport, Children: []ChildReport{{ID: 1, Name: "Миша", Today: td}}}, MaxReportBody)
	if err != nil {
		t.Fatal(err)
	}
	var rep Report
	json.Unmarshal(raw, &rep)
	got := rep.Children[0].Today
	if len(got.Programs) != 20 || len(got.Sites) != 10 || got.More.Programs != 8 || got.More.Sites != 5 {
		t.Fatalf("списки %d/%d, more %+v; want 20/10, 8/5", len(got.Programs), len(got.Sites), got.More)
	}
	if got.Programs[0].Name != "Программа 24" || got.Sites[0].Name != "site14.example" {
		t.Fatalf("не по убыванию: %v / %v", got.Programs[0], got.Sites[0])
	}
	games := 0
	for _, it := range got.Programs {
		if it.Cat == ItemGame {
			games++
		}
	}
	if last := got.Programs[len(got.Programs)-1]; games != 3 || last.Name != "Игра 0" {
		t.Fatalf("игр %d, последняя строка %v: игры вытеснены", games, last)
	}
}

// Ловит: пустые списки уходят телефону null; ребёнок без статистики получает today; короткий
// список урезается; название с управляющими знаками; отчёт без today меняет вид.
func TestFitReportSmallAndEmpty(t *testing.T) {
	rep := Report{V: 1, T: TypeReport, PC: "pc", Phone: "ph", TS: 1, Children: []ChildReport{
		{ID: 1, Name: "Миша", Today: &TodayReport{Programs: []TimeItem{{Name: "a\tb\nc", Cat: ItemOther, Sec: 5}, {Name: "Игра", Cat: ItemGame, Sec: 9}}}},
		{ID: 2, Name: "Коля", Today: &TodayReport{}},
		{ID: 3, Name: "Даша"},
	}}
	raw, err := fitReport(rep, MaxReportBody)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "null") || !strings.Contains(s, `"programs":[],"sites":[]`) {
		t.Fatalf("пустые списки: %s", s)
	}
	var got Report
	json.Unmarshal(raw, &got)
	if got.Children[2].Today != nil {
		t.Fatalf("today у ребёнка без статистики: %s", s)
	}
	p := got.Children[0].Today.Programs
	if len(p) != 2 || p[0].Name != "Игра" || p[0].Cat != ItemGame || p[1].Name != "a b c" || got.Children[0].Today.More != (TodayMore{}) {
		t.Fatalf("короткий список: %+v", got.Children[0].Today)
	}
	plain := Report{V: 1, T: TypeReport, PC: "pc", Phone: "ph", TS: 1, Children: []ChildReport{{ID: 3, Name: "Даша"}}, Pending: []PendingReport{}}
	a, _ := fitReport(plain, MaxReportBody)
	b, _ := json.Marshal(plain)
	if string(a) != string(b) {
		t.Fatalf("отчёт без today изменился:\n%s\n%s", a, b)
	}
}

// Ловит: название обрезается по байтам (половина буквы) или длиннее 60 знаков.
func TestItemName(t *testing.T) {
	if got := ItemName(strings.Repeat("ж", 70)); got != strings.Repeat("ж", 60) {
		t.Fatalf("обрезка: %q", got)
	}
	if got := ItemName("  a\x00b  ​c "); got != "a b ​c" {
		t.Fatalf("очистка: %q", got)
	}
}

// reportOf — данные подписанного отчёта.
func reportOf(t *testing.T, s Signed) Report {
	t.Helper()
	var rep Report
	if err := json.Unmarshal(mustUnb64(t, s.Data), &rep); err != nil {
		t.Fatal(err)
	}
	return rep
}

// Ловит: связь шлёт отчёт со статистикой шести детей без урезания — тело больше 16 КБ, сервер
// связи отвечает 413, и телефон не получает даже остаток.
func TestLinkReportFits(t *testing.T) {
	phone := mustKey(t)
	g := newRig(t, func(g *rig) {
		g.h.st, _ = pairedState(t, g.r, phone, 0)
		g.h.children = bigReport(6, 50, 50).Children
	})
	g.start()
	waitFor(t, "отчёт", func() bool { _, _, _, r := g.r.snapshot(); return len(r) == 1 })
	g.r.mu.Lock()
	size := g.r.reportSizes[0]
	g.r.mu.Unlock()
	if size > MaxReportBody {
		t.Fatalf("тело отчёта %d байт, предел %d", size, MaxReportBody)
	}
	_, _, _, reps := g.r.snapshot()
	if rep := reportOf(t, reps[0]); len(rep.Children) != 6 || rep.Children[5].Today == nil || rep.Children[5].LeftSec != 600 {
		t.Fatalf("отчёт без детей или статистики: %+v", rep.Children)
	}
}

// Ловит: статистика за сегодня уезжает в ответ на команду (cmdr идёт через pc/reply с пределом
// тела 16 КБ): ответ на команду не доходит до телефона.
func TestLinkCmdrWithoutToday(t *testing.T) {
	phone := mustKey(t)
	g := newRig(t, func(g *rig) {
		g.h.st, _ = pairedState(t, g.r, phone, 0)
		g.h.children[0].Today = bigToday(20, 10)
	})
	g.start()
	waitFor(t, "отчёт", func() bool { _, _, _, r := g.r.snapshot(); return len(r) == 1 })
	_, _, _, reps := g.r.snapshot()
	if reportOf(t, reps[0]).Children[0].Today == nil {
		t.Fatal("в отчёте pc/report нет today")
	}
	raw, _ := json.Marshal(Command{V: 1, T: TypeCmd, PC: "pc-x", Phone: "ph1", Seq: 1, TS: g.clock.Ms(), Child: 3, Delta: 15})
	s, _ := Sign(phone, raw)
	m, ok := g.r.sendFrom("ph1", KindCmd, s, 2*time.Second)
	if !ok {
		t.Fatal("нет ответа")
	}
	var sr Signed
	json.Unmarshal(m.Body, &sr)
	data := mustUnb64(t, sr.Data)
	var res CmdResult
	json.Unmarshal(data, &res)
	if !res.OK || res.Report == nil || len(res.Report.Children) != 1 || res.Report.Children[0].Today != nil || strings.Contains(string(data), `"today"`) {
		t.Fatalf("ответ на команду: %s", data)
	}
}
