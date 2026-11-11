package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
