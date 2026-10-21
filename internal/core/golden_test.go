package core

// Прогон эталонов docs/04_Эталоны_учёта.md (testdata/golden/G*.json) на поддельных часах.
//
// Формат golden-файла (неизвестный ключ — ошибка разбора):
//   name, source — имя эталона (= имя файла) и откуда цифры;
//   tz_offset_min — смещение местного пояса, моменты "YYYY-MM-DDTHH:MM:SS" — местные;
//   limits_min — лимиты по дням (mon…sun), нет дня или null — без лимита;
//   sessions — сеансы ребёнка [from, to): тик каждую секунду, вне сеансов тиков нет;
//     end: "logoff" (ядро живёт дальше) или "power_off" (состояние теряется, новое ядро
//     восстанавливается из последнего сохранения; сохранение — по DueSave перед тиком);
//   segments — что на экране в [from, to): foreground (other, games, video_social, study),
//     fullscreen; ввод в моменты from, from+N, …, не позже to (N = input_every_s, 0 — ввода
//     нет); вне отрезков — «Прочее», обычное окно, без ввода;
//   expect — warnings (момент и порог), locks (момент появления экрана блокировки и
//     тексты), checkpoints (общее время к моменту, до тика), power_off (последнее
//     сохранение перед выключением), days (итоги дней в секундах, остаток).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

const goldenLayout = "2006-01-02T15:04:05"

type goldenFile struct {
	Name        string           `json:"name"`
	Source      string           `json:"source"`
	TzOffsetMin *int             `json:"tz_offset_min"`
	LimitsMin   *goldenLimits    `json:"limits_min"`
	Schedule    []goldenInterval `json:"schedule"`
	Sessions    []goldenSession  `json:"sessions"`
	Segments    []goldenSegment  `json:"segments"`
	Actions     []goldenAction   `json:"actions"`
	Expect      *goldenExpect    `json:"expect"`

	loc *time.Location
}

// goldenInterval — запретный интервал расписания (R7): день недели 1=пн..7=вс, минуты.
type goldenInterval struct {
	Weekday  int `json:"weekday"`
	StartMin int `json:"start_min"`
	EndMin   int `json:"end_min"`
}

// goldenAction — родительская или детская команда в момент At (местное время): доп. время
// (add_extra), трата из копилки (spend_bonus) или пауза (pause). Применяется перед тиком
// момента At. minutes — для add_extra/spend_bonus; until — момент конца паузы (pause).
type goldenAction struct {
	At      string `json:"at"`
	Kind    string `json:"kind"`
	Minutes int    `json:"minutes"`
	Until   string `json:"until"`
}

type goldenLimits struct {
	Mon *int `json:"mon"`
	Tue *int `json:"tue"`
	Wed *int `json:"wed"`
	Thu *int `json:"thu"`
	Fri *int `json:"fri"`
	Sat *int `json:"sat"`
	Sun *int `json:"sun"`
}

type goldenSession struct {
	From string `json:"from"`
	To   string `json:"to"`
	End  string `json:"end"`
}

type goldenSegment struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Foreground  string `json:"foreground"`
	Fullscreen  bool   `json:"fullscreen"`
	InputEveryS int    `json:"input_every_s"`
}

type goldenExpect struct {
	Warnings    []goldenWarning    `json:"warnings"`
	Locks       []goldenLock       `json:"locks"`
	Checkpoints []goldenCheckpoint `json:"checkpoints"`
	PowerOff    []goldenPowerOff   `json:"power_off"`
	Days        []goldenDay        `json:"days"`
}

type goldenWarning struct {
	At  string `json:"at"`
	Min int    `json:"min"`
}

type goldenLock struct {
	At     string `json:"at"`
	Reason string `json:"reason"`
	Until  string `json:"until"`
	Used   string `json:"used"`
}

type goldenCheckpoint struct {
	At     string `json:"at"`
	TotalS int    `json:"total_s"`
}

type goldenPowerOff struct {
	LastSaveAt     string `json:"last_save_at"`
	RestoredTotalS int    `json:"restored_total_s"`
}

type goldenDay struct {
	Day        string `json:"day"`
	TotalS     *int   `json:"total_s"`
	GamesS     *int   `json:"games_s"`
	VideoS     *int   `json:"video_s"`
	StudyS     *int   `json:"study_s"`
	ExtraMin   *int   `json:"extra_min"`
	RemainingS *int   `json:"remaining_s"`
}

var goldenForeground = map[string]Category{
	"other": CatOther, "games": CatGames, "video_social": CatVideoSocial, "study": CatStudy,
}

// parseGolden разбирает и проверяет golden-файл.
func parseGolden(data []byte) (*goldenFile, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var g goldenFile
	if err := dec.Decode(&g); err != nil {
		return nil, fmt.Errorf("разбор: %w", err)
	}
	if dec.More() {
		return nil, errors.New("лишние данные после объекта")
	}
	switch {
	case g.Name == "" || g.Source == "":
		return nil, errors.New("нет name или source")
	case g.TzOffsetMin == nil:
		return nil, errors.New("нет tz_offset_min")
	case g.LimitsMin == nil:
		return nil, errors.New("нет limits_min")
	case len(g.Sessions) == 0:
		return nil, errors.New("нет sessions")
	case g.Expect == nil:
		return nil, errors.New("нет expect")
	case g.Expect.Warnings == nil || g.Expect.Locks == nil || len(g.Expect.Days) == 0:
		return nil, errors.New("в expect нужны warnings, locks и days (пустые — [])")
	}
	g.loc = time.FixedZone("golden", *g.TzOffsetMin*60)

	var prevEnd time.Time
	for i, s := range g.Sessions {
		from, to, err := g.span(s.From, s.To)
		if err != nil {
			return nil, fmt.Errorf("сеанс %d: %w", i, err)
		}
		if s.End != "logoff" && s.End != "power_off" {
			return nil, fmt.Errorf("сеанс %d: end %q, want logoff или power_off", i, s.End)
		}
		if from.Before(prevEnd) {
			return nil, fmt.Errorf("сеанс %d пересекается с предыдущим", i)
		}
		prevEnd = to
	}
	prevEnd = time.Time{}
	for i, s := range g.Segments {
		from, to, err := g.span(s.From, s.To)
		if err != nil {
			return nil, fmt.Errorf("отрезок %d: %w", i, err)
		}
		if _, ok := goldenForeground[s.Foreground]; !ok {
			return nil, fmt.Errorf("отрезок %d: foreground %q", i, s.Foreground)
		}
		if s.InputEveryS < 0 || from.Before(prevEnd) {
			return nil, fmt.Errorf("отрезок %d: input_every_s < 0 или пересечение", i)
		}
		prevEnd = to
	}
	for _, w := range g.Expect.Warnings {
		if _, err := g.at(w.At); err != nil {
			return nil, err
		}
	}
	for _, l := range g.Expect.Locks {
		if _, err := g.at(l.At); err != nil {
			return nil, err
		}
	}
	for _, c := range g.Expect.Checkpoints {
		if _, err := g.at(c.At); err != nil {
			return nil, err
		}
	}
	for _, d := range g.Expect.Days {
		if _, err := time.Parse(time.DateOnly, d.Day); err != nil || d.TotalS == nil {
			return nil, fmt.Errorf("день %q: нужна дата YYYY-MM-DD и total_s", d.Day)
		}
	}
	for i, a := range g.Actions {
		if _, err := g.at(a.At); err != nil {
			return nil, fmt.Errorf("action %d: %w", i, err)
		}
		switch a.Kind {
		case "add_extra", "spend_bonus":
			if a.Minutes < 1 {
				return nil, fmt.Errorf("action %d (%s): нужно minutes >= 1", i, a.Kind)
			}
		case "pause":
			if _, err := g.at(a.Until); err != nil {
				return nil, fmt.Errorf("action %d (pause): until %w", i, err)
			}
		default:
			return nil, fmt.Errorf("action %d: неизвестный kind %q", i, a.Kind)
		}
	}
	return &g, nil
}

func (g *goldenFile) at(s string) (time.Time, error) {
	t, err := time.ParseInLocation(goldenLayout, s, g.loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("момент %q: %w", s, err)
	}
	return t, nil
}

func (g *goldenFile) span(from, to string) (time.Time, time.Time, error) {
	f, err := g.at(from)
	if err != nil {
		return f, f, err
	}
	t, err := g.at(to)
	if err != nil {
		return f, t, err
	}
	if !f.Before(t) {
		return f, t, fmt.Errorf("from %s не раньше to %s", from, to)
	}
	return f, t, nil
}

func (g *goldenFile) rules() Rules {
	var r Rules
	for i, m := range []*int{g.LimitsMin.Mon, g.LimitsMin.Tue, g.LimitsMin.Wed, g.LimitsMin.Thu, g.LimitsMin.Fri, g.LimitsMin.Sat, g.LimitsMin.Sun} {
		if m != nil {
			r.Week[i] = LimitOf(*m)
		}
	}
	for _, iv := range g.Schedule {
		r.Schedule = append(r.Schedule, Interval{Weekday: Weekday(iv.Weekday), StartMin: iv.StartMin, EndMin: iv.EndMin})
	}
	return r
}

// goldenResult — что получилось на ядре.
type goldenResult struct {
	warnings    []goldenWarning
	locks       []goldenLock
	checkpoints map[string]int
	powerOff    []goldenPowerOff
	days        map[string]DayState
}

// runGolden воспроизводит сценарий на поддельных часах тем же порядком, что служба:
// в момент t сначала DueSave и снимок для сохранения, затем Tick.
func runGolden(g *goldenFile) goldenResult {
	res := goldenResult{
		warnings: []goldenWarning{}, locks: []goldenLock{}, powerOff: []goldenPowerOff{},
		checkpoints: map[string]int{}, days: map[string]DayState{},
	}
	wantCP := map[string]bool{}
	for _, c := range g.Expect.Checkpoints {
		wantCP[c.At] = true
	}
	type segment struct {
		from, to time.Time
		fg       Category
		full     bool
	}
	segs := []segment{}
	inputs := []time.Time{}
	for _, s := range g.Segments {
		from, to, _ := g.span(s.From, s.To)
		segs = append(segs, segment{from, to, goldenForeground[s.Foreground], s.Fullscreen})
		if s.InputEveryS > 0 {
			for m := from; !m.After(to); m = m.Add(time.Duration(s.InputEveryS) * time.Second) {
				inputs = append(inputs, m)
			}
		}
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Before(inputs[j]) })

	rules := g.rules()
	clock := &fakeClock{}
	e := NewEngine(clock, g.loc, rules, DayState{}, "")
	var saved DayState
	var savedDetail string
	var savedAt time.Time
	next := 0 // следующий ввод в inputs
	var lastInput time.Time
	for _, sess := range g.Sessions {
		from, to, _ := g.span(sess.From, sess.To)
		prevLock := LockNone
		for t := from; t.Before(to); t = t.Add(time.Second) {
			clock.Set(t)
			for next < len(inputs) && !inputs[next].After(t) {
				lastInput = inputs[next]
				next++
			}
			s := SessionSample{AtMs: t.UnixMilli(), TzOffsetMin: *g.TzOffsetMin, SessionActive: true, LastInputAgoMs: 24 * 3600 * 1000}
			if !lastInput.IsZero() {
				s.LastInputAgoMs = t.Sub(lastInput).Milliseconds()
			}
			for _, sg := range segs {
				if !t.Before(sg.from) && t.Before(sg.to) {
					s.Foreground, s.Fullscreen = sg.fg, sg.full
				}
			}
			stamp := t.Format(goldenLayout)
			if wantCP[stamp] {
				st, _ := e.State()
				res.checkpoints[stamp] = st.TotalS
			}
			if e.DueSave(t) {
				saved, savedDetail = e.State()
				savedAt = t
			}
			// Команды момента t применяются перед тиком (как в службе: доп. время, трата
			// из копилки, пауза сдвигают событие до учёта секунды t).
			for _, a := range g.Actions {
				am, _ := g.at(a.At)
				if !am.Equal(t) {
					continue
				}
				switch a.Kind {
				case "add_extra":
					if _, err := e.AddExtra(a.Minutes); err != nil {
						panic(err)
					}
				case "spend_bonus":
					if _, err := e.SpendBonus(a.Minutes); err != nil {
						panic(err)
					}
				case "pause":
					until, _ := g.at(a.Until)
					if _, err := e.Pause(until); err != nil {
						panic(err)
					}
				}
			}
			d := e.Tick(s)
			if d.Warning != 0 {
				res.warnings = append(res.warnings, goldenWarning{At: stamp, Min: d.Warning})
			}
			if d.Lock != LockNone && prevLock == LockNone {
				res.locks = append(res.locks, goldenLock{At: stamp, Reason: d.Reason, Until: d.Until, Used: d.Used})
			}
			prevLock = d.Lock
			if d.ClosedDay != nil {
				res.days[d.ClosedDay.Day] = *d.ClosedDay
			}
		}
		if sess.End == "power_off" {
			last := ""
			if !savedAt.IsZero() {
				last = savedAt.Format(goldenLayout)
			}
			res.powerOff = append(res.powerOff, goldenPowerOff{LastSaveAt: last, RestoredTotalS: saved.TotalS})
			e = NewEngine(clock, g.loc, rules, saved, savedDetail)
		}
	}
	st, _ := e.State()
	res.days[st.Day] = st
	return res
}

// Ловит: расхождение ядра с эталонами G1, G2, G3, G9, G10 (правило 2: эталон — истина,
// расхождение — ошибка кода): моменты предупреждений и блокировки, тексты экрана,
// итоги дней, остаток, сохранение и восстановление после выключения.
func TestGolden(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "golden", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, p := range paths {
		names = append(names, strings.TrimSuffix(filepath.Base(p), ".json"))
	}
	for _, need := range []string{"G1", "G2", "G3", "G6", "G7", "G8", "G9", "G10", "G11"} {
		if !slices.Contains(names, need) {
			t.Errorf("нет эталона testdata/golden/%s.json (M1: G1,G2,G3,G9,G10; M2: G6,G7,G8,G11)", need)
		}
	}
	for i, p := range paths {
		t.Run(names[i], func(t *testing.T) {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			g, err := parseGolden(data)
			if err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			if g.Name != names[i] {
				t.Fatalf("name %q не совпадает с именем файла %q", g.Name, names[i])
			}
			checkGolden(t, g, runGolden(g))
		})
	}
}

func checkGolden(t *testing.T, g *goldenFile, res goldenResult) {
	t.Helper()
	ex := g.Expect
	if !reflect.DeepEqual(res.warnings, ex.Warnings) {
		t.Errorf("предупреждения:\n got  %+v\n want %+v", res.warnings, ex.Warnings)
	}
	if !reflect.DeepEqual(res.locks, ex.Locks) {
		t.Errorf("блокировки:\n got  %+v\n want %+v", res.locks, ex.Locks)
	}
	for _, c := range ex.Checkpoints {
		got, ok := res.checkpoints[c.At]
		if !ok {
			t.Errorf("контрольная точка %s вне сеансов", c.At)
		} else if got != c.TotalS {
			t.Errorf("к %s общее %d с, want %d", c.At, got, c.TotalS)
		}
	}
	wantPO := ex.PowerOff
	if wantPO == nil {
		wantPO = []goldenPowerOff{}
	}
	if !reflect.DeepEqual(res.powerOff, wantPO) {
		t.Errorf("выключения:\n got  %+v\n want %+v", res.powerOff, wantPO)
	}
	if len(res.days) != len(ex.Days) {
		t.Errorf("дней %d (%v), want %d", len(res.days), res.days, len(ex.Days))
	}
	rules := g.rules()
	for _, want := range ex.Days {
		got, ok := res.days[want.Day]
		if !ok {
			t.Errorf("нет итога дня %s", want.Day)
			continue
		}
		day, _ := time.ParseInLocation(time.DateOnly, want.Day, g.loc)
		rem, _ := RemainingTotalS(rules.For(WeekdayOf(day)), got.ExtraMin, got.TotalS)
		for _, f := range []struct {
			name string
			want *int
			got  int
		}{
			{"total_s", want.TotalS, got.TotalS},
			{"games_s", want.GamesS, got.GamesS},
			{"video_s", want.VideoS, got.VideoS},
			{"study_s", want.StudyS, got.StudyS},
			{"extra_min", want.ExtraMin, got.ExtraMin},
			{"remaining_s", want.RemainingS, rem},
		} {
			if f.want != nil && *f.want != f.got {
				t.Errorf("%s %s = %d, want %d", want.Day, f.name, f.got, *f.want)
			}
		}
	}
}

// goldenMinimal — минимальный правильный golden-файл для проверок загрузчика.
const goldenMinimal = `{
  "name": "GX", "source": "тест загрузчика", "tz_offset_min": 180,
  "limits_min": {"mon": 120},
  "sessions": [{"from": "2026-10-19T15:00:00", "to": "2026-10-19T15:00:10", "end": "logoff"}],
  "segments": [{"from": "2026-10-19T15:00:00", "to": "2026-10-19T15:00:10", "foreground": "other", "fullscreen": false, "input_every_s": 60}],
  "expect": {"warnings": [], "locks": [], "days": [{"day": "2026-10-19", "total_s": 10}]}
}`

// Ловит: загрузчик эталонов молча принимает опечатку в ключе, пропущенные ожидания
// или битые моменты, и эталон перестаёт что-либо проверять.
func TestGoldenLoaderRejectsBadFiles(t *testing.T) {
	g, err := parseGolden([]byte(goldenMinimal))
	if err != nil {
		t.Fatalf("минимальный файл не разобран: %v", err)
	}
	res := runGolden(g)
	if res.days["2026-10-19"].TotalS != 10 {
		t.Fatalf("минимальный файл: итог %+v, want 10 с", res.days)
	}
	bad := []struct{ name, from, to string }{
		{"неизвестный ключ", `"source"`, `"sorce"`},
		{"неизвестный ключ лимита", `"mon": 120`, `"monday": 120`},
		{"нет warnings", `"warnings": [], `, ``},
		{"нет tz", `"tz_offset_min": 180,`, ``},
		{"нет лимитов", `"limits_min": {"mon": 120},`, ``},
		{"плохой конец сеанса", `"logoff"`, `"sleep"`},
		{"плохой момент", `"to": "2026-10-19T15:00:10", "end"`, `"to": "19.10.2026 15:00:10", "end"`},
		{"сеанс задом наперёд", `"from": "2026-10-19T15:00:00", "to": "2026-10-19T15:00:10", "end"`, `"from": "2026-10-19T15:00:10", "to": "2026-10-19T15:00:00", "end"`},
		{"плохая категория", `"other"`, `"games_and_more"`},
		{"нет total_s", `"total_s": 10`, `"games_s": 10`},
		{"лишние данные", "", `{}`},
	}
	for _, tc := range bad {
		src := goldenMinimal + tc.to
		if tc.from != "" {
			src = strings.Replace(goldenMinimal, tc.from, tc.to, 1)
		}
		if src == goldenMinimal {
			t.Fatalf("%s: замена не применилась", tc.name)
		}
		if _, err := parseGolden([]byte(src)); err == nil {
			t.Errorf("%s: файл принят, want ошибка", tc.name)
		}
	}
}
