package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Проба ST1 (ТЗ NFR-13, RM5-7, D-71): база за три года на трёх детей, «Всё время» на ребёнка — не
// дольше секунды. Размер базы и таблиц только печатается для сведения: объём не ограничен (D-71).
// Тест тяжёлый (сотни тысяч строк), поэтому идёт только при PCB_ST1=1 отдельным шагом без -race и
// без покрытия (tools/test-series.sh, функция st1_step): с ними чистый Go SQLite медленнее в разы.
// В общем прогоне тест пропускается.

const (
	st1Children    = 3
	st1Days        = 1095 // три года
	st1Programs    = 50   // программ в день
	st1Sites       = 50   // сайтов в день
	st1Events      = 16000
	st1BonusPerKid = 550 // запись копилки примерно раз в два дня
	st1Limit       = time.Second
)

// st1ViolationKinds — виды «Нарушений» (journalGroups службы, RM5-5): как читает их статистика.
var st1ViolationKinds = []string{"bypass", "program_forbidden_try", "site_forbidden_try"}

// st1ProgramPaths — 50 путей exe реальной длины (нижний регистр, как хранит служба).
func st1ProgramPaths() []string {
	roots := []string{
		`c:\program files (x86)\steam\steamapps\common\%s\bin\win64\%s.exe`,
		`c:\users\ребёнок\appdata\local\programs\%s\%s.exe`,
		`c:\program files\windowsapps\%s_1.24.1105.0_x64__8wekyb3d8bbwe\%s.exe`,
		`c:\program files\%s\application\%s.exe`,
		`c:\users\ребёнок\appdata\local\temp\%s-installer\%s.exe`,
	}
	names := []string{"minecraft", "roblox", "geometrydash", "terraria", "stardewvalley",
		"brave", "notepad", "calculator", "paint", "skysmart"}
	out := make([]string, 0, st1Programs)
	for i := 0; i < st1Programs; i++ {
		name := fmt.Sprintf("%s%02d", names[i%len(names)], i)
		out = append(out, fmt.Sprintf(roots[i%len(roots)], name, name))
	}
	return out
}

// st1SiteDomains — 50 доменов реальной длины.
func st1SiteDomains() []string {
	bases := []string{"youtube.com", "vk.com", "rutube.ru", "tiktok.com", "twitch.tv",
		"uchi.ru", "resh.edu.ru", "foxford.ru", "wikipedia.org", "news-portal-example.ru"}
	out := make([]string, 0, st1Sites)
	for i := 0; i < st1Sites; i++ {
		out = append(out, fmt.Sprintf("sub%02d.%s", i, bases[i%len(bases)]))
	}
	return out
}

func TestStatsST1(t *testing.T) {
	if os.Getenv("PCB_ST1") != "1" {
		t.Skip("проба ST1 только при PCB_ST1=1 (tools/test-series.sh --st1-only)")
	}
	path := filepath.Join(t.TempDir(), "pcblock.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ids := make([]int64, 0, st1Children)
	for i := 0; i < st1Children; i++ {
		id, err := s.UpsertChild(Child{SID: fmt.Sprintf("S-1-5-21-1004336348-1177238915-682003330-%d", 1001+i), Name: fmt.Sprintf("Ребёнок %d", i+1)})
		if err != nil {
			t.Fatalf("UpsertChild: %v", err)
		}
		ids = append(ids, id)
	}
	today := time.Now().Format(time.DateOnly)
	genStart := time.Now()
	st1Generate(t, s, ids, today)
	t.Logf("генерация: %v", time.Since(genStart).Round(time.Millisecond))
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Холодный кэш (RM5-7): база открывается заново, замер — первыми запросами нового соединения.
	s, err = Open(path)
	if err != nil {
		t.Fatalf("повторный Open: %v", err)
	}
	defer s.Close()

	// Запросы «Всё время» на ребёнка — те же, что myTodayOf службы (T11): снимок очистки, ряд дней
	// (с дня очистки, если она была), учёт по программам и сайтам, копилка, нарушения за период
	// (журнал с фильтром по виду, не больше 200) и «Статистика ведётся с» (первый день без очистки).
	var worst time.Duration
	for i, id := range ids {
		t0 := time.Now()
		from := "0001-01-01"
		reset, resetOK, err := s.GetStatsReset(id)
		if err != nil {
			t.Fatalf("GetStatsReset: %v", err)
		}
		if resetOK && reset.Day > from {
			from = reset.Day
		}
		days, err := s.DaysSeries(id, from, today)
		if err != nil {
			t.Fatalf("DaysSeries: %v", err)
		}
		totalS := 0
		for _, d := range days {
			totalS += d.TotalS
		}
		usage, err := s.PeriodUsage(id, from, today)
		if err != nil {
			t.Fatalf("PeriodUsage: %v", err)
		}
		bonus, err := s.ListBonus(id)
		if err != nil {
			t.Fatalf("ListBonus: %v", err)
		}
		child := id
		violations, err := s.ListEvents(EventFilter{ChildID: &child, Kinds: st1ViolationKinds, Limit: 200})
		if err != nil {
			t.Fatalf("ListEvents (нарушения): %v", err)
		}
		firstDay := ""
		if !resetOK {
			if firstDay, _, err = s.FirstStatsDay(id); err != nil {
				t.Fatalf("FirstStatsDay: %v", err)
			}
		}
		d := time.Since(t0)
		worst = max(worst, d)
		t.Logf("ребёнок %d: %v (очистка %v с %s, дней %d, итог %d с, строк учёта %d, копилка %d, нарушений %d, первый день %q)",
			id, d.Round(time.Millisecond), resetOK, reset.Day, len(days), totalS, len(usage), len(bonus), len(violations), firstDay)
		// Сверка, что запросы видят всё сгенерированное (литералы по RM5-7): снимок очистки только у
		// первого ребёнка, в день первой строки — ряд дней от него целиком.
		if resetOK != (i == 0) || len(days) != st1Days || len(usage) != 100 || len(bonus) != 550 || len(violations) != 200 || totalS == 0 {
			t.Errorf("ребёнок %d: очистка %v (ждём %v), дней %d (ждём %d), строк учёта %d (ждём 100), копилка %d (ждём 550), нарушений %d (ждём 200), итог %d с",
				id, resetOK, i == 0, len(days), st1Days, len(usage), len(bonus), len(violations), totalS)
		}
		if !resetOK && firstDay == "" {
			t.Errorf("ребёнок %d: нет первого дня статистики", id)
		}
	}

	// Размер — после контрольной точки WAL, для сведения (D-71).
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Logf("контрольная точка WAL: %v", err)
	}
	var size int64
	for _, p := range []string{path, path + "-wal"} {
		if fi, err := os.Stat(p); err == nil {
			size += fi.Size()
		}
	}
	mb := float64(size) / (1 << 20)
	st1TableSizes(t, s)

	summary := fmt.Sprintf("ST1: %d мс на ребёнка; база %.1f МБ (для сведения, D-71)", worst.Milliseconds(), mb)
	fmt.Println(summary)
	fmt.Printf("::notice title=ST1::%s, порог %d мс, %s\n", summary, st1Limit.Milliseconds(), st1Verdict(worst))
	if worst > st1Limit {
		t.Fatalf("«Всё время» на ребёнка %v дольше порога %v (NFR-13)", worst.Round(time.Millisecond), st1Limit)
	}
}

func st1Verdict(d time.Duration) string {
	if d > st1Limit {
		return "ДОЛЬШЕ ПОРОГА"
	}
	return "в пределах порога"
}

// st1Generate пишет данные одной транзакцией подготовленными выражениями: учёт по программам и
// сайтам, строки дней, события журнала, копилку и строку stats_reset у первого ребёнка (RM5-7):
// очистка в день первой строки, снимок — счётчики того дня, поэтому ряд дней читается целиком.
func st1Generate(t *testing.T, s *Store, ids []int64, today string) {
	t.Helper()
	end, err := time.ParseInLocation(time.DateOnly, today, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	// Полдень: переход на летнее время не сдвигает дату.
	end = end.Add(12 * time.Hour)
	days := make([]string, st1Days)
	for i := 0; i < st1Days; i++ {
		days[i] = end.AddDate(0, 0, -(st1Days - i)).Format(time.DateOnly) // строго до сегодня
	}
	progs, sites := st1ProgramPaths(), st1SiteDomains()
	progCat := []string{"games", "study", "other", "games", "other"}
	siteCat := []string{"video_social", "study", "other", "video_social", "other"}

	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	usageStmt, err := tx.Prepare(`INSERT INTO usage_daily (child_id, day, category, subject_kind, subject, seconds) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	dayStmt, err := tx.Prepare(`INSERT INTO day_state (child_id, day, total_s, games_s, video_s, study_s, extra_min,
		pause_until, bonus_granted, no_forbidden, last_minute_end) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	eventStmt, err := tx.Prepare(`INSERT INTO events (child_id, at, tz_offset_min, kind, details) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	bonusStmt, err := tx.Prepare(`INSERT INTO bonus_ledger (child_id, at, delta_min, reason, balance_after) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	resetStmt, err := tx.Prepare(`INSERT INTO stats_reset (child_id, cleared_at, day, total_s, games_s, video_s, study_s, extra_min)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}

	for ci, id := range ids {
		for di, day := range days {
			var total, games, video, study int
			for pi, p := range progs {
				sec := 30 + (ci*7+di*13+pi*29)%600
				cat := progCat[pi%len(progCat)]
				if _, err := usageStmt.Exec(id, day, cat, SubjectProgram, p, sec); err != nil {
					t.Fatalf("usage_daily: %v", err)
				}
				switch cat {
				case "games":
					games += sec
					total += sec
				case "study":
					study += sec
				default:
					total += sec
				}
			}
			for si, d := range sites {
				sec := 20 + (ci*11+di*17+si*31)%400
				cat := siteCat[si%len(siteCat)]
				if _, err := usageStmt.Exec(id, day, cat, SubjectSite, d, sec); err != nil {
					t.Fatalf("usage_daily: %v", err)
				}
				switch cat {
				case "video_social":
					video += sec
					total += sec
				case "study":
					study += sec
				default:
					total += sec
				}
			}
			dayAt, _ := time.ParseInLocation(time.DateOnly, day, time.Local)
			lastMin := any(nil)
			if di%5 == 0 {
				lastMin = dayAt.Add(20 * time.Hour).UnixMilli()
			}
			if _, err := dayStmt.Exec(id, day, total, games, video, study, (di*3)%60, nil, boolToInt(di%7 == 0), boolToInt(di%11 == 0), lastMin); err != nil {
				t.Fatalf("day_state: %v", err)
			}
			if ci == 0 && di == 0 {
				// Очистка в 18:00 дня первой строки: снимок — половина счётчиков того дня.
				if _, err := resetStmt.Exec(id, dayAt.Add(18*time.Hour).UnixMilli(), day, total/2, games/2, video/2, study/2, 0); err != nil {
					t.Fatalf("stats_reset: %v", err)
				}
			}
		}
		// Копилка: начисления и траты, баланс в пределах 0…600 (CHECK).
		balance := 0
		for bi := 0; bi < st1BonusPerKid; bi++ {
			at := end.AddDate(0, 0, -st1Days+bi*2).UnixMilli()
			delta, reason := 15, "автобонус: лимит не превышен"
			if bi%3 == 2 && balance >= 20 {
				delta, reason = -20, "взято из копилки"
			}
			if balance+delta > 600 {
				delta, reason = -20, "взято из копилки"
			}
			balance += delta
			if _, err := bonusStmt.Exec(id, at, delta, reason, balance); err != nil {
				t.Fatalf("bonus_ledger: %v", err)
			}
		}
	}

	// События: поровну на детей, по три года, с подробностями реальной длины.
	kinds := []string{"program_forbidden_try", "program_closed_games", "program_pending", "extra_time",
		"site_blocked", "limit_reached", "program_auto_closed", "bypass"}
	startMs := end.AddDate(0, 0, -st1Days).UnixMilli()
	stepMs := int64(st1Days) * 24 * 3600 * 1000 / st1Events
	for ei := 0; ei < st1Events; ei++ {
		id := ids[ei%len(ids)]
		details := fmt.Sprintf(`{"path":%q,"name":"%s","text":"Попытка открыть запрещённую программу %s"}`,
			progs[ei%len(progs)], strings.ToUpper(progs[ei%len(progs)][:1]), sites[ei%len(sites)])
		if _, err := eventStmt.Exec(id, startMs+int64(ei)*stepMs, 180, kinds[ei%len(kinds)], details); err != nil {
			t.Fatalf("events: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// st1TableSizes печатает размеры таблиц и индексов (виртуальная таблица dbstat). Нет её в сборке
// драйвера — пропускается без ошибки.
func st1TableSizes(t *testing.T, s *Store) {
	t.Helper()
	rows, err := s.db.Query(`SELECT name, sum(pgsize) FROM dbstat GROUP BY name ORDER BY sum(pgsize) DESC`)
	if err != nil {
		t.Logf("размеры таблиц недоступны (dbstat): %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var size int64
		if err := rows.Scan(&name, &size); err != nil {
			t.Logf("dbstat: %v", err)
			return
		}
		t.Logf("  %-32s %8.2f МБ", name, float64(size)/(1<<20))
	}
	if err := rows.Err(); err != nil {
		t.Logf("dbstat: %v", err)
	}
}
