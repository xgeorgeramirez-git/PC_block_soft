package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Эталон G15 (S21, docs/04_Эталоны_учёта.md): эталонная неделя статистики. Данные и ожидания —
// из одного файла testdata/golden/G15.json (план M5, T0), тесты своих литералов недели не держат.

type g15Day struct {
	Day      string `json:"day"`
	Today    bool   `json:"today"`
	ExtraMin int    `json:"extra_min"`
	UsedMin  int    `json:"used_min"`
	GamesMin int    `json:"games_min"`
	VideoMin int    `json:"video_min"`
	StudyMin int    `json:"study_min"`
	OtherMin int    `json:"other_min"`
}

type g15Bar struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	TotalMin int    `json:"total_min"`
	StudyMin int    `json:"study_min"`
	Title    string `json:"title"`
	Today    bool   `json:"today"`
}

type g15Row struct {
	Name string `json:"name"`
	Min  int    `json:"min"`
	Pct  int    `json:"pct"`
}

type g15File struct {
	Name        string            `json:"name"`
	Source      string            `json:"source"`
	TzOffsetMin int               `json:"tz_offset_min"`
	Now         string            `json:"now"`
	LimitsMin   map[string]int    `json:"limits_min"`
	Subjects    map[string]string `json:"subjects"`
	Days        []g15Day          `json:"days"`
	Expect      struct {
		Week struct {
			UsedMin  int `json:"used_min"`
			GamesMin int `json:"games_min"`
			VideoMin int `json:"video_min"`
			StudyMin int `json:"study_min"`
			OtherMin int `json:"other_min"`
			AllMin   int `json:"all_min"`
			ExtraMin int `json:"extra_min"`
		} `json:"week"`
		BarsWeek []g15Bar `json:"bars_week"`
		Programs []g15Row `json:"programs"`
		Sites    []g15Row `json:"sites"`
		Year     struct {
			Bars int    `json:"bars"`
			Nov  g15Bar `json:"nov"`
		} `json:"year"`
	} `json:"expect"`
}

// loadG15 читает эталон строго: неизвестный ключ — ошибка (опечатка в файле не пропадает молча).
func loadG15(t *testing.T) g15File {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "G15.json"))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var g g15File
	if err := dec.Decode(&g); err != nil {
		t.Fatalf("G15.json: %v", err)
	}
	return g
}

// Ловит: эталон G15 сам с собой не сходится (опечатка в таблице дня или в строке «Неделя»), и
// тесты статистики проверяли бы службу по неверной истине; столбиков не 7 или не те дни.
func TestG15SelfCheck(t *testing.T) {
	g := loadG15(t)
	if g.Name != "G15" || len(g.Days) != 7 || len(g.Expect.BarsWeek) != 7 {
		t.Fatalf("G15: имя %q, дней %d, столбиков %d; want G15, 7, 7", g.Name, len(g.Days), len(g.Expect.BarsWeek))
	}
	var used, games, video, study, other, extra, todays int
	for i, d := range g.Days {
		if d.UsedMin != d.GamesMin+d.VideoMin+d.OtherMin {
			t.Errorf("%s: в счёт лимита %d ≠ игры %d + видео %d + прочее %d", d.Day, d.UsedMin, d.GamesMin, d.VideoMin, d.OtherMin)
		}
		b := g.Expect.BarsWeek[i]
		if b.Key != d.Day || b.TotalMin != d.UsedMin || b.StudyMin != d.StudyMin || b.Today != d.Today {
			t.Errorf("столбик %+v не совпадает с днём %+v", b, d)
		}
		if d.Today {
			todays++
		}
		used += d.UsedMin
		games += d.GamesMin
		video += d.VideoMin
		study += d.StudyMin
		other += d.OtherMin
		extra += d.ExtraMin
	}
	w := g.Expect.Week
	if used != w.UsedMin || games != w.GamesMin || video != w.VideoMin || study != w.StudyMin || other != w.OtherMin ||
		extra != w.ExtraMin || used+study != w.AllMin || todays != 1 || !g.Days[6].Today {
		t.Errorf("суммы дней: %d, %d, %d, %d, %d, доп. %d, всего %d, сегодня %d раз; строка «Неделя» %+v",
			used, games, video, study, other, extra, used+study, todays, w)
	}
	byName := map[string]int{g.Subjects["games"]: games, g.Subjects["other"]: other, g.Subjects["study"]: study}
	for _, r := range g.Expect.Programs {
		if byName[r.Name] != r.Min {
			t.Errorf("программа %s: %d мин в таблице, по дням %d", r.Name, r.Min, byName[r.Name])
		}
	}
	if len(g.Expect.Sites) != 1 || g.Expect.Sites[0].Name != g.Subjects["video_social"] || g.Expect.Sites[0].Min != video {
		t.Errorf("сайты %+v; want %s %d мин", g.Expect.Sites, g.Subjects["video_social"], video)
	}
	if n := g.Expect.Year.Nov; n.TotalMin != used || n.StudyMin != study || g.Expect.Year.Bars != 11 {
		t.Errorf("год: %+v, столбиков %d; want ноябрь %d/%d, 11 столбиков", n, g.Expect.Year.Bars, used, study)
	}
}

// g15Paths — предметы эталона на путях службы в тестах: «game.exe» — игра из каталога (сегодняшние
// 10 мин идут через неё), «study.exe» — программа с решением «Учёба», «other.exe» — незнакомая из
// прошлых дней, сайт — домен как есть.
var g15Paths = map[string]string{"games": pathMinecraft, "study": pathGeoGebra, "other": `c:\tools\other.exe`}

// g15Harness — служба на часах эталона: прошлые дни G15 в базе, сегодня 10 мин игры через ядро.
func g15Harness(t *testing.T, opts ...hopt) (*harness, g15File, int64) {
	t.Helper()
	g := loadG15(t)
	now, err := time.Parse(time.RFC3339, g.Now)
	if err != nil {
		t.Fatal(err)
	}
	all := append([]hopt{configured(), withChild(childSID, "Миша", 240), withPrograms(typicalScan()), withClockAt(now.In(tzMSK))}, opts...)
	h := newHarness(t, all...)
	h.start()
	h.childLogsIn()
	h.setCategory(pathGeoGebra, "study")
	id := h.childID(childSID)
	for _, d := range g.Days {
		if d.Today {
			continue
		}
		rows := []UsageRow{}
		add := func(cat, kind, subject string, min int) {
			if min > 0 {
				rows = append(rows, UsageRow{Day: d.Day, Category: cat, Kind: kind, Subject: subject, Seconds: min * 60})
			}
		}
		add("games", usageKindProgram, g15Paths["games"], d.GamesMin)
		add("study", usageKindProgram, g15Paths["study"], d.StudyMin)
		add("other", usageKindProgram, g15Paths["other"], d.OtherMin)
		add(usageCatVideo, usageKindSite, g.Subjects["video_social"], d.VideoMin)
		st := core.DayState{Day: d.Day, TotalS: d.UsedMin * 60, GamesS: d.GamesMin * 60, VideoS: d.VideoMin * 60, StudyS: d.StudyMin * 60,
			ExtraMin: d.ExtraMin, BonusGranted: true}
		if err := h.store.SaveDayAndUsage(id, st, rows); err != nil {
			t.Fatal(err)
		}
	}
	today := g.Days[6]
	h.run(today.GamesMin*60, fgSample(pathMinecraft, ipc.CatNameOther))
	return h, g, id
}

// barsSum — сумма столбиков: в счёт лимита и учёба.
func barsSum(bars []ipc.DayBar) (total, study int) {
	for _, b := range bars {
		total += b.TotalS
		study += b.StudyS
	}
	return total, study
}

// seriesCalls — сколько раз служба прочитала ряд дней.
func (h *harness) seriesCalls() int {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	return h.store.seriesCalls
}

// Ловит (S21, G15, RM5-2…RM5-4, класс A): столбики не те (ключ, подпись, секунды, подсказка), среда
// без данных пропала из календаря; сумма столбиков не равна «Итогам» (столбики и итоги из разных
// чтений: второй запрос ряда дней); сегодня взято из сохранённой копии базы, а не из ядра; таблицы
// «Программы» и «Сайты» за неделю не сходятся с эталоном; родитель видит иное, чем ребёнок.
func TestMyTodayG15Week(t *testing.T) {
	h, g, id := g15Harness(t)
	// Сохранённая копия сегодняшнего дня отстаёт от ядра (до 30 с) или испорчена: сегодня — только ядро.
	h.store.mu.Lock()
	h.store.days[dayKey(id, "2026-11-22")] = core.DayState{Day: "2026-11-22", TotalS: 9999, StudyS: 9999, GamesS: 9999}
	h.store.mu.Unlock()

	before := h.seriesCalls()
	my := h.periodReply(ipc.PeriodWeek)
	if n := h.seriesCalls() - before; n != 1 {
		t.Errorf("ряд дней прочитан %d раз за ответ, want 1 (итоги и столбики из одного чтения)", n)
	}
	if my.BarsBy != ipc.BarsByDay || len(my.Bars) != len(g.Expect.BarsWeek) {
		t.Fatalf("неделя: разбивка %q, столбиков %d; want day, %d: %s", my.BarsBy, len(my.Bars), len(g.Expect.BarsWeek), jsonOf(t, my.Bars))
	}
	for i, w := range g.Expect.BarsWeek {
		want := ipc.DayBar{Key: w.Key, Label: w.Label, Title: w.Title, TotalS: w.TotalMin * 60, StudyS: w.StudyMin * 60, Today: w.Today}
		if my.Bars[i] != want {
			t.Errorf("столбик %d: %+v; want %+v", i, my.Bars[i], want)
		}
	}
	w := g.Expect.Week
	if my.UsedS != w.UsedMin*60 || my.GamesS != w.GamesMin*60 || my.VideoS != w.VideoMin*60 || my.StudyS != w.StudyMin*60 ||
		my.OtherS != w.OtherMin*60 || my.AllS != w.AllMin*60 || my.ExtraMin != w.ExtraMin {
		t.Errorf("итоги недели: %d/%d/%d/%d/%d/%d с, доп. %d; want %+v (мин)", my.UsedS, my.GamesS, my.VideoS, my.StudyS, my.OtherS, my.AllS, my.ExtraMin, w)
	}
	if total, study := barsSum(my.Bars); total != my.UsedS || study != my.StudyS {
		t.Errorf("сумма столбиков %d/%d ≠ итоги %d/%d", total, study, my.UsedS, my.StudyS)
	}
	// Таблицы «Программы» и «Сайты» окно строит из Programs (RM5-4): строка — сумма по пути из
	// всех категорий, «Остальное время» не входит; доля — от суммы своей таблицы.
	table := func(kind string) map[string]int {
		out := map[string]int{}
		for _, p := range my.Programs {
			if p.Path != "" && p.Kind == kind {
				out[p.Path] += p.Seconds
			}
		}
		return out
	}
	check := func(what string, got map[string]int, want []g15Row, pathOf func(string) string) {
		sum := 0
		for _, n := range got {
			sum += n
		}
		if len(got) != len(want) {
			t.Errorf("%s: %v; want %+v", what, got, want)
		}
		for _, r := range want {
			n := got[pathOf(r.Name)]
			if pct := int(math.Round(float64(n) * 100 / float64(max(sum, 1)))); n != r.Min*60 || pct != r.Pct {
				t.Errorf("%s %s: %d с, %d %%; want %d мин, %d %%", what, r.Name, n, pct, r.Min, r.Pct)
			}
		}
	}
	byName := map[string]string{}
	for cat, name := range g.Subjects {
		byName[name] = g15Paths[cat]
	}
	check("программы", table(usageKindProgram), g.Expect.Programs, func(n string) string { return byName[n] })
	check("сайты", table(usageKindSite), g.Expect.Sites, func(n string) string { return n })

	tok := h.login(adminPeer())
	var ps ipc.MyTodayReply
	h.ok(adminPeer(), tok, ipc.CmdParentStats, ipc.ParentStatsRequest{ChildID: id, Period: ipc.PeriodWeek}, &ps)
	if jsonOf(t, ps.Bars) != jsonOf(t, my.Bars) || ps.StatsSinceText != my.StatsSinceText {
		t.Errorf("родитель видит иные столбики:\n%s\nребёнок:\n%s", jsonOf(t, ps.Bars), jsonOf(t, my.Bars))
	}

	// «Сегодня» — один столбик из ядра.
	today := h.periodReply(ipc.PeriodToday)
	if len(today.Bars) != 1 || today.Bars[0] != (ipc.DayBar{Key: "2026-11-22", Label: "вс 22", Title: "вс, 22 ноября: 10 мин, учёба 0 мин", TotalS: 600, Today: true}) ||
		today.BarsBy != ipc.BarsByDay || today.UsedS != 600 {
		t.Errorf("сегодня: %s, итог %d", jsonOf(t, today.Bars), today.UsedS)
	}
}

// Ловит (RM5-2): «Год» не по месяцам, без пустых месяцев между январём и ноябрём, без сегодняшнего
// дня в ноябре; подписи месяцев не «янв»…«ноя»; сумма столбиков ≠ «Итоги» года.
func TestMyTodayG15Year(t *testing.T) {
	h, g, id := g15Harness(t)
	if err := h.store.SaveDayAndUsage(id, core.DayState{Day: "2026-01-05", TotalS: 3600, StudyS: 600, BonusGranted: true}, nil); err != nil {
		t.Fatal(err)
	}
	my := h.periodReply(ipc.PeriodYear)
	if my.BarsBy != ipc.BarsByMonth || len(my.Bars) != g.Expect.Year.Bars {
		t.Fatalf("год: разбивка %q, столбиков %d; want month, %d: %s", my.BarsBy, len(my.Bars), g.Expect.Year.Bars, jsonOf(t, my.Bars))
	}
	labels := []string{}
	for i, b := range my.Bars {
		labels = append(labels, b.Label)
		if key := fmt.Sprintf("2026-%02d", i+1); b.Key != key {
			t.Errorf("месяц %d: ключ %q, want %q", i, b.Key, key)
		}
		if i > 0 && i < 10 && (b.TotalS != 0 || b.StudyS != 0 || b.Today) {
			t.Errorf("пустой месяц %s: %+v", b.Key, b)
		}
	}
	if got := strings.Join(labels, " "); got != "янв фев мар апр май июн июл авг сен окт ноя" {
		t.Errorf("подписи месяцев %q", got)
	}
	jan := my.Bars[0]
	if jan.TotalS != 3600 || jan.StudyS != 600 || jan.Title != "январь 2026: 1 ч 00 мин, учёба 10 мин" || jan.Today {
		t.Errorf("январь: %+v", jan)
	}
	n := g.Expect.Year.Nov
	want := ipc.DayBar{Key: n.Key, Label: n.Label, Title: n.Title, TotalS: n.TotalMin * 60, StudyS: n.StudyMin * 60, Today: true}
	if my.Bars[10] != want {
		t.Errorf("ноябрь: %+v; want %+v", my.Bars[10], want)
	}
	if total, study := barsSum(my.Bars); total != my.UsedS || study != my.StudyS || my.UsedS != (n.TotalMin+60)*60 {
		t.Errorf("сумма столбиков %d/%d, итоги %d/%d; want %d", total, study, my.UsedS, my.StudyS, (n.TotalMin+60)*60)
	}
}

// Ловит (RM5-2): «Всё время» начинается не с первого месяца данных, с дырами между годами, подпись
// без года, когда период захватывает несколько лет; без данных — не один текущий месяц.
func TestMyTodayAllTimeBars(t *testing.T) {
	h, _, id := g15Harness(t)
	empty := newHarness(t, configured(), withChild(childSID, "Миша", 240), withClockAt(time.Date(2026, 11, 22, 15, 0, 0, 0, tzMSK)))
	empty.start()
	if my := empty.periodReply(ipc.PeriodAll); len(my.Bars) != 1 || my.Bars[0].Key != "2026-11" || my.Bars[0].Label != "ноя" || my.BarsBy != ipc.BarsByMonth {
		t.Errorf("всё время без данных: %s", jsonOf(t, my.Bars))
	}
	if err := h.store.SaveDayAndUsage(id, core.DayState{Day: "2025-10-03", TotalS: 1200, BonusGranted: true}, nil); err != nil {
		t.Fatal(err)
	}
	my := h.periodReply(ipc.PeriodAll)
	if len(my.Bars) != 14 {
		t.Fatalf("всё время: %d столбиков, want 14 (окт 2025…ноя 2026): %s", len(my.Bars), jsonOf(t, my.Bars))
	}
	first, last := my.Bars[0], my.Bars[13]
	if first.Key != "2025-10" || first.Label != "окт 2025" || first.TotalS != 1200 || first.Title != "октябрь 2025: 20 мин, учёба 0 мин" {
		t.Errorf("первый месяц: %+v", first)
	}
	if my.Bars[3].Key != "2026-01" || my.Bars[3].Label != "янв 2026" || my.Bars[3].TotalS != 0 {
		t.Errorf("январь 2026: %+v", my.Bars[3])
	}
	if last.Key != "2026-11" || last.Label != "ноя 2026" || !last.Today {
		t.Errorf("последний месяц: %+v", last)
	}
	if total, study := barsSum(my.Bars); total != my.UsedS || study != my.StudyS {
		t.Errorf("сумма столбиков %d/%d ≠ итоги %d/%d", total, study, my.UsedS, my.StudyS)
	}
}

// Ловит (PL-31, Review Focus 1): после полуночи до тика ядра вчерашние счётчики ядра выдаются за
// сегодняшний столбик; вчерашний день пропадает (его строка в базе — прошлый день).
func TestMyTodayBarsCoreStillYesterday(t *testing.T) {
	h, _, id := g15Harness(t)
	saved, _ := h.store.LoadDayState(id, "2026-11-22") // сохранённая копия: отстаёт от ядра до 30 с
	if saved.TotalS == 0 {
		t.Fatal("сегодняшний день не сохранён в базу: проверка ничего не проверяет")
	}
	h.clock.Advance(9*time.Hour + 10*time.Minute) // пн 23.11 00:10, тика ещё не было
	my := h.periodReply(ipc.PeriodMonth)
	if len(my.Bars) != 23 {
		t.Fatalf("месяц: %d столбиков, want 23", len(my.Bars))
	}
	if b := my.Bars[22]; b.Key != "2026-11-23" || b.TotalS != 0 || !b.Today || b.Label != "пн 23" {
		t.Errorf("сегодня до тика ядра: %+v; want пустой столбик", b)
	}
	if b := my.Bars[21]; b.Key != "2026-11-22" || b.TotalS != saved.TotalS || b.Today {
		t.Errorf("вчера: %+v; want %d с из строки базы", b, saved.TotalS)
	}
	if total, _ := barsSum(my.Bars); total != my.UsedS {
		t.Errorf("сумма столбиков %d ≠ итог %d", total, my.UsedS)
	}
}

// Ловит (RM5-4, класс A, PL-28): ошибка чтения ряда дней или снимка очистки роняет ответ или даёт
// итоги за период при столбиках только за сегодня (или наоборот).
func TestMyTodayBarsStoreErrors(t *testing.T) {
	for _, fail := range []string{"series", "reset"} {
		h, _, _ := g15Harness(t)
		h.store.mu.Lock()
		h.store.failStats, h.store.failReset = fail == "series", fail == "reset"
		h.store.mu.Unlock()
		my := h.periodReply(ipc.PeriodWeek)
		if my.UsedS != 600 || len(my.Bars) != 7 {
			t.Errorf("%s: итог %d, столбиков %d; want 600 и 7", fail, my.UsedS, len(my.Bars))
		}
		for _, b := range my.Bars {
			if !b.Today && b.TotalS != 0 {
				t.Errorf("%s: прошлый столбик с данными %+v", fail, b)
			}
		}
		if total, _ := barsSum(my.Bars); total != my.UsedS || sumOf(my.Programs, "") != my.AllS {
			t.Errorf("%s: сумма столбиков %d, строк %d, итоги %d/%d", fail, total, sumOf(my.Programs, ""), my.UsedS, my.AllS)
		}
		if !strings.Contains(h.logText(), "не удалось прочитать") {
			t.Errorf("%s: ошибка не в журнале работы", fail)
		}
	}
}

// Ловит (RM5-8, чтение): снимок очистки не вычитается из дня очистки или из сегодняшнего дня; дни
// до очистки остаются в «Итогах» и столбиках; «Взято из копилки» считает траты до очистки;
// «Статистика ведётся с» не дата очистки; поле ушло ниже нуля (доп. время убавили после очистки).
func TestMyTodayAfterStatsReset(t *testing.T) {
	h, _, id := g15Harness(t)
	clearedAt := time.Date(2026, 11, 19, 12, 0, 0, 0, tzMSK).UnixMilli()
	h.store.mu.Lock()
	h.store.resets[id] = StatsReset{ClearedAtMs: clearedAt, Day: "2026-11-19", TotalS: 100 * 60, GamesS: 45 * 60, VideoS: 40 * 60, StudyS: 10 * 60, ExtraMin: 20}
	h.store.bonus[id] = []BonusRecord{ // старые первыми, как пишет fake
		{AtMs: clearedAt - 1000, DeltaMin: -15, Reason: reasonBonusSpent},
		{AtMs: clearedAt + 1000, DeltaMin: -5, Reason: reasonBonusSpent},
	}
	h.store.mu.Unlock()
	my := h.periodReply(ipc.PeriodWeek)
	// чт: 128−100 = 28 мин, учёба 45−10 = 35; игры 45−45 = 0, видео 40−40 = 0, доп. 15−20 → 0.
	want := []int{0, 0, 0, 28 * 60, 180 * 60, 200 * 60, 600}
	if len(my.Bars) != len(want) {
		t.Fatalf("неделя после очистки: %d столбиков, want 7 с понедельника", len(my.Bars))
	}
	for i, b := range my.Bars {
		if b.TotalS != want[i] {
			t.Errorf("столбик %s: %d с, want %d", b.Key, b.TotalS, want[i])
		}
	}
	if my.Bars[3].StudyS != 35*60 || my.Bars[0].StudyS != 0 {
		t.Errorf("учёба: чт %d, пн %d; want 2100, 0", my.Bars[3].StudyS, my.Bars[0].StudyS)
	}
	if my.UsedS != (28+180+200)*60+600 || my.ExtraMin != 0 || my.StudyS != (35+20)*60 {
		t.Errorf("итоги после очистки: %d с, доп. %d, учёба %d", my.UsedS, my.ExtraMin, my.StudyS)
	}
	if total, study := barsSum(my.Bars); total != my.UsedS || study != my.StudyS {
		t.Errorf("сумма столбиков %d/%d ≠ итоги %d/%d", total, study, my.UsedS, my.StudyS)
	}
	if my.BonusSpentMin != 5 {
		t.Errorf("взято из копилки %d, want 5 (только после очистки)", my.BonusSpentMin)
	}
	if my.StatsSinceDay != "2026-11-19" || my.StatsSinceText != "Статистика ведётся с 19 ноября 2026" {
		t.Errorf("ведётся с: %q, %q", my.StatsSinceDay, my.StatsSinceText)
	}
	// Копилка за период от очистки не зависит (RM5-8).
	if len(my.Bonus) != 2 {
		t.Errorf("история копилки за неделю: %d записей, want 2", len(my.Bonus))
	}

	// Очистка сегодня: сегодня — счётчики ядра минус снимок.
	h.store.mu.Lock()
	h.store.resets[id] = StatsReset{ClearedAtMs: time.Date(2026, 11, 22, 14, 0, 0, 0, tzMSK).UnixMilli(), Day: "2026-11-22", TotalS: 240, GamesS: 240}
	h.store.mu.Unlock()
	my = h.periodReply(ipc.PeriodAll)
	if my.UsedS != 360 || my.GamesS != 360 || len(my.Bars) != 1 || my.Bars[0].TotalS != 360 || my.StatsSinceDay != "2026-11-22" {
		t.Errorf("очистка сегодня: итог %d, игры %d, столбики %s, с %q; want 360", my.UsedS, my.GamesS, jsonOf(t, my.Bars), my.StatsSinceDay)
	}
}

// Ловит (R21, S22, RM5-4): ребёнок видит нарушения и копилку другого ребёнка; в нарушения попали
// обычные события или события до начала периода; порядок не «новые сверху»; предел 200 не соблюдён;
// у копилки нет пояса записи (время строки в окне сдвинется).
func TestMyTodayViolationsAndBonus(t *testing.T) {
	h, _, id := g15Harness(t, withChild(child2SID, "Коля", 240))
	kolya := h.childID(child2SID)
	mon := time.Date(2026, 11, 16, 0, 0, 0, 0, tzMSK).UnixMilli()
	add := func(child int64, at int64, kind string) {
		if err := h.store.AppendEvent(StoredEvent{ChildID: child, AtMs: at, TzOffsetMin: 180, Kind: kind, Details: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
	}
	add(id, mon-1, core.EvBypass)            // до недели
	add(id, mon+1000, core.EvBypass)         // пн
	add(id, mon+2000, core.EvExtraTime)      // не нарушение
	add(id, mon+3000, evProgramForbiddenTry) // пн
	add(id, mon+4000, evSiteForbiddenTry)    // пн
	add(kolya, mon+5000, core.EvBypass)      // чужое
	add(0, mon+6000, "password_changed")     // не про ребёнка
	h.store.mu.Lock()
	h.store.bonus[id] = []BonusRecord{
		{AtMs: mon - 1, DeltaMin: 10, Reason: "старое", Balance: 10},
		{AtMs: mon + 1000, DeltaMin: 30, Reason: "Бонус за свободное время: 30 мин", Balance: 40},
		{AtMs: mon + 2000, DeltaMin: -10, Reason: reasonBonusSpent, Balance: 30},
	}
	h.store.bonus[kolya] = []BonusRecord{{AtMs: mon + 1500, DeltaMin: 60, Reason: "Коле", Balance: 60}}
	h.store.mu.Unlock()

	my := h.periodReply(ipc.PeriodWeek)
	kinds := []string{}
	for _, v := range my.Violations {
		kinds = append(kinds, v.Kind)
		if v.ChildID != id || v.ChildName != "Миша" || v.Text == "" || v.TzOffsetMin != 180 {
			t.Errorf("нарушение %+v", v)
		}
	}
	if got := strings.Join(kinds, " "); got != "site_forbidden_try program_forbidden_try bypass" {
		t.Errorf("нарушения за неделю: %q", got)
	}
	if len(my.Bonus) != 2 || my.Bonus[0].DeltaMin != -10 || my.Bonus[1].DeltaMin != 30 || my.Bonus[0].TzOffset != 180 || my.Bonus[1].Balance != 40 {
		t.Errorf("копилка за неделю: %+v", my.Bonus)
	}
	if all := h.periodReply(ipc.PeriodAll); len(all.Violations) != 4 || len(all.Bonus) != 3 {
		t.Errorf("всё время: нарушений %d, копилки %d; want 4, 3", len(all.Violations), len(all.Bonus))
	}

	for i := 0; i < 250; i++ {
		add(id, mon+10_000+int64(i), core.EvBypass)
	}
	h.store.mu.Lock()
	for i := 0; i < 250; i++ {
		h.store.bonus[id] = append(h.store.bonus[id], BonusRecord{AtMs: mon + 10_000 + int64(i), DeltaMin: 1, Reason: "r", Balance: 31 + i})
	}
	h.store.mu.Unlock()
	my = h.periodReply(ipc.PeriodWeek)
	if len(my.Violations) != ipc.StatsListLimit || my.Violations[0].AtMs != mon+10_249 || len(my.Bonus) != ipc.StatsListLimit || my.Bonus[0].AtMs != mon+10_249 {
		t.Errorf("предел: нарушений %d (первое %d), копилки %d (первая %d)", len(my.Violations), my.Violations[0].AtMs, len(my.Bonus), my.Bonus[0].AtMs)
	}
}

// Ловит (RM5-8): «Статистика ведётся с» без очистки — не первый день базы; без строк — не сегодня;
// не ребёнок получает null вместо пустых списков.
func TestMyTodayStatsSince(t *testing.T) {
	h, _, _ := g15Harness(t)
	if my := h.periodReply(ipc.PeriodToday); my.StatsSinceDay != "2026-11-16" || my.StatsSinceText != "Статистика ведётся с 16 ноября 2026" {
		t.Errorf("с первого дня: %q, %q", my.StatsSinceDay, my.StatsSinceText)
	}
	fresh := newHarness(t, configured(), withChild(childSID, "Миша", 240), withClockAt(wed1500))
	fresh.start()
	fresh.childLogsIn()
	if my := fresh.periodReply(ipc.PeriodWeek); my.StatsSinceDay != "2026-10-21" || my.StatsSinceText != "Статистика ведётся с 21 октября 2026" {
		t.Errorf("без строк: %q, %q", my.StatsSinceDay, my.StatsSinceText)
	}
	rep := fresh.do(ipc.Peer{PID: 9, SessionID: 1, SID: parentSID, Admin: true}, "", ipc.CmdMyToday, ipc.MyTodayRequest{Period: ipc.PeriodWeek})
	for _, want := range []string{`"bars":[]`, `"violations":[]`, `"bonus":[]`, `"programs":[]`} {
		if !strings.Contains(string(rep.Payload), want) {
			t.Errorf("не ребёнок: нет %s в %s", want, rep.Payload)
		}
	}
}

// Ловит (RM5-8): снимок не вычитается из строки дня очистки, вычитается из других дней, поле уходит
// ниже нуля; без снимка строки меняются; живой день другого дня уменьшен.
func TestApplyResetAndLive(t *testing.T) {
	r := StatsReset{Day: "2026-11-19", TotalS: 100, GamesS: 50, VideoS: 10, StudyS: 5, ExtraMin: 20}
	days := []DayRow{
		{Day: "2026-11-18", TotalS: 500},
		{Day: "2026-11-19", TotalS: 300, GamesS: 40, VideoS: 10, StudyS: 15, ExtraMin: 10},
		{Day: "2026-11-20", TotalS: 700, GamesS: 70},
	}
	got := applyReset(days, r, true)
	want := []DayRow{{Day: "2026-11-19", TotalS: 200, GamesS: 0, VideoS: 0, StudyS: 10, ExtraMin: 0}, {Day: "2026-11-20", TotalS: 700, GamesS: 70}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("applyReset = %v, want %v", got, want)
	}
	if got := applyReset(days, r, false); fmt.Sprint(got) != fmt.Sprint(days) {
		t.Errorf("без снимка: %v", got)
	}
	if got := applyReset([]DayRow{}, r, true); got == nil {
		t.Errorf("пусто — nil")
	}
	live := core.DayState{Day: "2026-11-19", TotalS: 400, GamesS: 30, VideoS: 20, StudyS: 1, ExtraMin: 25, BonusGranted: true}
	if got := liveMinusReset(live, r, true); got.TotalS != 300 || got.GamesS != 0 || got.VideoS != 10 || got.StudyS != 0 || got.ExtraMin != 5 || got.Day != live.Day {
		t.Errorf("liveMinusReset = %+v", got)
	}
	other := core.DayState{Day: "2026-11-20", TotalS: 400}
	if got := liveMinusReset(other, r, true); got != other {
		t.Errorf("другой день уменьшен: %+v", got)
	}
	if got := liveMinusReset(live, r, false); got != live {
		t.Errorf("без снимка: %+v", got)
	}
}
