package storage

import (
	"reflect"
	"testing"
)

// Сайты и браузеры вехи M4 (R14, R15, FR-21, RM4-5, RM4-8): миграция 009 и методы сайтов.

// Ловит: миграция 009 на базе 0.3.3 (001–008 с данными) оставляет домену две записи или берёт не
// строгую (S-1: запрещённые > учёба > видео, id сохраняется); стартовый набор «Видео и соцсети»
// (R14) затирает решение родителя (youtube.com в «Учёбе») или не вставлен; браузеры каталога и
// снимка не стали «Запрещено» либо задеты решение родителя, Brave, WebView2, msedge_proxy, чужое
// имя с похожим окончанием или чужой browser.exe не из папки Яндекса (RM4-8, PL-18); скрытие и
// своё название браузера потеряны (PL-30); другие таблицы задеты; повторное применение что-то меняет.
func TestMigration009OnFilled008Base(t *testing.T) {
	all, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var v008 []migration
	for _, m := range all {
		if m.version < 9 {
			v008 = append(v008, m)
		}
	}
	db := rawDB(t)
	if err := applyMigrations(db, v008, 100); err != nil {
		t.Fatalf("миграции 001–008: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO children (id, windows_sid, display_name, created_at) VALUES (1, 'S-1', 'Миша', 10)`,
		`INSERT INTO children (id, windows_sid, display_name, created_at, adult_filter) VALUES (2, 'S-2', 'Даша', 10, 0)`,
		`INSERT INTO day_limits (child_id, weekday, total_min, games_min, video_min) VALUES (1, 1, 120, 60, NULL), (1, 2, NULL, NULL, NULL)`,
		`INSERT INTO site_rules (id, domain, kind) VALUES
			(1, 'example.com', 'blocked'), (2, 'example.com', 'study'), (3, 'kids.example', 'video_social'),
			(4, 'kids.example', 'study'), (5, 'youtube.com', 'study'), (6, 'school.example', 'study')`,
		`INSERT INTO programs (exe_path, display_name, category, source, hidden, custom_name, decided_at) VALUES
			('c:\program files\microsoft\edge\application\msedge.exe', 'Microsoft Edge', 'other', 'catalog', 0, '', NULL),
			('c:\program files\google\chrome\application\chrome.exe', 'Google Chrome', 'other', 'snapshot', 1, '', NULL),
			('c:\program files\mozilla firefox\firefox.exe', 'Mozilla Firefox', 'study', 'parent', 0, '', 500),
			('c:\program files\bravesoftware\brave-browser\application\brave.exe', 'Brave', 'other', 'snapshot', 0, '', NULL),
			('c:\program files (x86)\microsoft\edgewebview\application\130.0.2849.80\msedgewebview2.exe', 'WebView2', 'system', 'catalog', 0, '', NULL),
			('c:\program files (x86)\microsoft\edge\application\msedge_proxy.exe', 'msedge_proxy', 'other', 'snapshot', 0, '', NULL),
			('c:\users\миша\appdata\local\yandex\yandexbrowser\application\browser.exe', 'Яндекс Браузер', 'games', 'snapshot', 0, 'Яндекс', NULL),
			('c:\tools\xmsedge.exe', 'xmsedge', 'other', 'snapshot', 0, '', NULL),
			('c:\program files\internet explorer\iexplore.exe', 'Internet Explorer', 'system', 'catalog', 0, '', NULL),
			('c:\games\launcher\browser.exe', 'browser', 'games', 'snapshot', 0, '', NULL)`,
		`INSERT INTO day_state (child_id, day, total_s) VALUES (1, '2026-11-04', 3600)`,
		`INSERT INTO usage_daily (child_id, day, category, subject_kind, subject, seconds) VALUES (1, '2026-11-04', 'other', 'program', 'c:\tools\xmsedge.exe', 60)`,
		`INSERT INTO settings (key, value) VALUES ('bonus.rate', '30')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("данные 008: %s: %v", q, err)
		}
	}
	const dayQ = `SELECT * FROM day_state`
	const usageQ = `SELECT * FROM usage_daily`
	const settingsQ = `SELECT key, value FROM settings ORDER BY key`
	before := [][][]string{rowStrings(t, db, dayQ), rowStrings(t, db, usageQ), rowStrings(t, db, settingsQ)}

	if err := applyMigrations(db, all, 200); err != nil {
		t.Fatalf("миграция 009: %v", err)
	}

	const sitesQ = `SELECT domain, kind FROM site_rules ORDER BY domain`
	wantSites := [][]string{
		{"discord.com", "video_social"}, {"example.com", "blocked"}, {"kids.example", "study"}, {"ok.ru", "video_social"},
		{"rutube.ru", "video_social"}, {"school.example", "study"}, {"telegram.org", "video_social"}, {"tiktok.com", "video_social"},
		{"twitch.tv", "video_social"}, {"vk.com", "video_social"}, {"web.telegram.org", "video_social"}, {"youtube.com", "study"},
	}
	if got := rowStrings(t, db, sitesQ); !reflect.DeepEqual(got, wantSites) {
		t.Errorf("site_rules после миграции = %v, want %v", got, wantSites)
	}
	if got, want := rowStrings(t, db, `SELECT id, domain FROM site_rules WHERE id <= 6 ORDER BY id`),
		[][]string{{"1", "example.com"}, {"4", "kids.example"}, {"5", "youtube.com"}, {"6", "school.example"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("id прежних записей = %v, want %v (у домена остаётся строгая запись со своим id)", got, want)
	}
	const progQ = `SELECT category, hidden, custom_name, source, ifnull(decided_at, 'NULL') FROM programs ORDER BY id`
	wantProg := [][]string{
		{"blocked", "0", "", "catalog", "NULL"},        // Edge из каталога
		{"blocked", "1", "", "snapshot", "NULL"},       // Chrome скрыт и всё равно запрещён (PL-30)
		{"study", "0", "", "parent", "500"},            // Firefox: решение родителя сильнее (RM4-8)
		{"other", "0", "", "snapshot", "NULL"},         // Brave — сайты решают
		{"system", "0", "", "catalog", "NULL"},         // WebView2 не браузер (PL-18)
		{"other", "0", "", "snapshot", "NULL"},         // msedge_proxy не браузер
		{"blocked", "0", "Яндекс", "snapshot", "NULL"}, // Яндекс Браузер в своей папке
		{"other", "0", "", "snapshot", "NULL"},         // xmsedge.exe — другое имя
		{"blocked", "0", "", "catalog", "NULL"},        // Internet Explorer, хоть и «Системное»
		{"games", "0", "", "snapshot", "NULL"},         // чужой browser.exe не из папки Яндекса
	}
	if got := rowStrings(t, db, progQ); !reflect.DeepEqual(got, wantProg) {
		t.Errorf("programs после миграции = %v, want %v", got, wantProg)
	}
	if got, want := rowStrings(t, db, `SELECT id, adult_filter FROM children ORDER BY id`), [][]string{{"1", "1"}, {"2", "0"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("children (id, adult_filter) = %v, want %v", got, want)
	}
	if got, want := rowStrings(t, db, `SELECT child_id, weekday, ifnull(total_min, 'NULL'), ifnull(games_min, 'NULL'), ifnull(video_min, 'NULL') FROM day_limits ORDER BY child_id, weekday`),
		[][]string{{"1", "1", "120", "60", "NULL"}, {"1", "2", "NULL", "NULL", "NULL"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("day_limits = %v, want %v (video_min NULL — без лимита)", got, want)
	}
	if got := [][][]string{rowStrings(t, db, dayQ), rowStrings(t, db, usageQ), rowStrings(t, db, settingsQ)}; !reflect.DeepEqual(got, before) {
		t.Errorf("day_state, usage_daily, settings после миграции = %v, want %v", got, before)
	}
	if _, err := db.Exec(`INSERT INTO site_rules (domain, kind) VALUES ('youtube.com', 'blocked')`); err == nil {
		t.Error("youtube.com записан вторым списком, want отказ UNIQUE (один домен — один список, RM4-5)")
	}

	if err := applyMigrations(db, all, 300); err != nil {
		t.Fatalf("повторное применение: %v", err)
	}
	if got := rowStrings(t, db, sitesQ); !reflect.DeepEqual(got, wantSites) {
		t.Errorf("site_rules после повтора = %v, want %v", got, wantSites)
	}
	if got := rowStrings(t, db, progQ); !reflect.DeepEqual(got, wantProg) {
		t.Errorf("programs после повтора = %v, want %v", got, wantProg)
	}
	if got := versions(t, db); !reflect.DeepEqual(got, [][2]int64{{1, 100}, {2, 100}, {3, 100}, {4, 100}, {5, 100}, {6, 100}, {7, 100}, {8, 100}, {9, 200}}) {
		t.Errorf("schema_migrations = %v, want 1–8 в 100 и 9 в 200", got)
	}
}

// Ловит: при новой установке стартовый набор «Видео и соцсети» (R14) не вставлен, неполон или
// попал в другой список; в «Запрещённых» и «Учёбе» что-то есть.
func TestFreshBaseHasVideoStarterSet(t *testing.T) {
	s := openTemp(t)
	want := [][]string{{"discord.com"}, {"ok.ru"}, {"rutube.ru"}, {"telegram.org"}, {"tiktok.com"}, {"twitch.tv"}, {"vk.com"}, {"web.telegram.org"}, {"youtube.com"}}
	if got := rowStrings(t, s.db, `SELECT domain FROM site_rules WHERE kind = 'video_social' ORDER BY domain`); !reflect.DeepEqual(got, want) {
		t.Errorf("«Видео и соцсети» новой базы = %v, want %v", got, want)
	}
	if got := rowStrings(t, s.db, `SELECT count(*) FROM site_rules WHERE kind <> 'video_social'`); !reflect.DeepEqual(got, [][]string{{"0"}}) {
		t.Errorf("записей других списков = %v, want 0", got)
	}
}
