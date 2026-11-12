package storage

import (
	"errors"
	"reflect"
	"strings"
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

	// Без 010: она по своему замыслу помечает прошлые дни автобонусом (D-82), её проверяет
	// TestMigration010OnFilled009Base.
	if err := applyMigrations(db, migrationsBefore(all, 10), 200); err != nil {
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
	if got := versions(t, db); !reflect.DeepEqual(got, [][2]int64{{1, 100}, {2, 100}, {3, 100}, {4, 100}, {5, 100}, {6, 100}, {7, 100}, {8, 100}, {9, 200}, {10, 300}}) {
		t.Errorf("schema_migrations = %v, want 1–8 в 100, 9 в 200 и 10 в 300", got)
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

// siteDomains — домены списка kind по алфавиту (прямым запросом, мимо методов хранилища).
func siteDomains(t *testing.T, s *Store, kind string) []string {
	t.Helper()
	out := []string{}
	for _, r := range rowStrings(t, s.db, `SELECT domain FROM site_rules WHERE kind = '`+kind+`' ORDER BY domain`) {
		out = append(out, r[0])
	}
	return out
}

// Ловит (R14, FR-21, RM4-5, S-1, S-6): добавление в другой список даёт вторую запись вместо
// переноса или не сообщает прежний список; повтор в том же списке что-то меняет; перенос меняет id;
// счётчик и список читают не то; удаление не отдаёт прежний список для «Отменить» или удаляет
// несуществующее с успехом; пустой, длиннее 253 символов домен или чужой список доходят до базы.
func TestSiteRules(t *testing.T) {
	s := openTemp(t)
	if moved, prev, err := s.AddSiteRule("example.com", SiteBlocked); err != nil || moved || prev != "" {
		t.Errorf("новый домен = %v, %q, %v; want false, \"\", nil", moved, prev, err)
	}
	id := rawNull(t, s, `SELECT id FROM site_rules WHERE domain = 'example.com'`).Int64
	if moved, prev, err := s.AddSiteRule("example.com", SiteBlocked); err != nil || moved || prev != SiteBlocked {
		t.Errorf("повтор в том же списке = %v, %q, %v; want false, \"blocked\", nil", moved, prev, err)
	}
	if moved, prev, err := s.AddSiteRule("example.com", SiteStudy); err != nil || !moved || prev != SiteBlocked {
		t.Errorf("перенос в «Учёбу» = %v, %q, %v; want true, \"blocked\", nil", moved, prev, err)
	}
	if got := rawNull(t, s, `SELECT id FROM site_rules WHERE domain = 'example.com'`).Int64; got != id {
		t.Errorf("id после переноса = %d, want %d (запись та же)", got, id)
	}
	if moved, prev, err := s.AddSiteRule("youtube.com", SiteStudy); err != nil || !moved || prev != SiteVideoSocial {
		t.Errorf("youtube.com в «Учёбу» = %v, %q, %v; want true, \"video_social\", nil", moved, prev, err)
	}
	for kind, want := range map[string]int{SiteVideoSocial: 8, SiteBlocked: 0, SiteStudy: 2} {
		if n, err := s.CountSiteRules(kind); err != nil || n != want {
			t.Errorf("CountSiteRules(%s) = %d, %v; want %d", kind, n, err, want)
		}
	}
	list, err := s.ListSiteRules()
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(list))
	for _, r := range list {
		got = append(got, r.Domain+" "+r.Kind)
	}
	want := []string{"example.com study", "youtube.com study", "discord.com video_social", "ok.ru video_social",
		"rutube.ru video_social", "telegram.org video_social", "tiktok.com video_social", "twitch.tv video_social",
		"vk.com video_social", "web.telegram.org video_social"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListSiteRules = %v, want %v (по списку, затем по домену)", got, want)
	}
	if list[0].ID != id {
		t.Errorf("ListSiteRules[0].ID = %d, want %d", list[0].ID, id)
	}
	if prev, ok, err := s.RemoveSiteRule("example.com"); err != nil || !ok || prev != SiteStudy {
		t.Errorf("удаление = %q, %v, %v; want \"study\", true, nil", prev, ok, err)
	}
	if prev, ok, err := s.RemoveSiteRule("example.com"); err != nil || ok || prev != "" {
		t.Errorf("повторное удаление = %q, %v, %v; want \"\", false, nil", prev, ok, err)
	}
	before := rowStrings(t, s.db, `SELECT * FROM site_rules ORDER BY id`)
	for name, call := range map[string]func() error{
		"пустой домен":     func() error { _, _, err := s.AddSiteRule("", SiteStudy); return err },
		"254 символа":      func() error { _, _, err := s.AddSiteRule(strings.Repeat("a", 250)+".com", SiteStudy); return err },
		"чужой список":     func() error { _, _, err := s.AddSiteRule("a.com", "video"); return err },
		"счётчик чужого":   func() error { _, err := s.CountSiteRules("x"); return err },
		"замена чужого":    func() error { return s.ReplaceSiteRules("x", []string{"a.com"}) },
		"удаление пустого": func() error { _, _, err := s.RemoveSiteRule(""); return err },
	} {
		if err := call(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
	if _, _, err := s.AddSiteRule(strings.Repeat("a", 249)+".com", SiteStudy); err != nil {
		t.Errorf("домен в 253 символа: %v, want записан", err)
	} else if _, _, err := s.RemoveSiteRule(strings.Repeat("a", 249) + ".com"); err != nil {
		t.Fatal(err)
	}
	if got := rowStrings(t, s.db, `SELECT * FROM site_rules ORDER BY id`); !reflect.DeepEqual(got, before) {
		t.Errorf("после отказов site_rules = %v, want %v", got, before)
	}
}

// Ловит (мастер FR-2, S-4, PL-21): замена списка трогает чужие списки сверх переноса доменов из них,
// оставляет старые домены своего списка или пишет дубль входа второй строкой; ошибка одного домена
// оставляет список наполовину заменённым (нет одной транзакции).
func TestReplaceSiteRules(t *testing.T) {
	s := openTemp(t)
	if _, _, err := s.AddSiteRule("example.com", SiteBlocked); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceSiteRules(SiteVideoSocial, []string{"youtube.com", "kids.example", "kids.example"}); err != nil {
		t.Fatal(err)
	}
	if got, want := siteDomains(t, s, SiteVideoSocial), []string{"kids.example", "youtube.com"}; !reflect.DeepEqual(got, want) {
		t.Errorf("видео после замены = %v, want %v", got, want)
	}
	if got, want := siteDomains(t, s, SiteBlocked), []string{"example.com"}; !reflect.DeepEqual(got, want) {
		t.Errorf("запрещённые после замены видео = %v, want %v (чужой список не трогается)", got, want)
	}
	if _, _, err := s.AddSiteRule("school.example", SiteStudy); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceSiteRules(SiteVideoSocial, []string{"school.example"}); err != nil {
		t.Fatal(err)
	}
	if got, want := siteDomains(t, s, SiteVideoSocial), []string{"school.example"}; !reflect.DeepEqual(got, want) {
		t.Errorf("видео = %v, want %v", got, want)
	}
	if got, want := siteDomains(t, s, SiteStudy), []string{}; !reflect.DeepEqual(got, want) {
		t.Errorf("учёба = %v, want %v (домен перенесён в видео)", got, want)
	}
	if err := s.ReplaceSiteRules(SiteVideoSocial, []string{"ok.ru", ""}); !errors.Is(err, ErrInvalid) {
		t.Errorf("замена с пустым доменом: %v, want ErrInvalid", err)
	}
	if got, want := siteDomains(t, s, SiteVideoSocial), []string{"school.example"}; !reflect.DeepEqual(got, want) {
		t.Errorf("видео после отказа = %v, want %v (ничего не изменилось)", got, want)
	}
	if err := s.ReplaceSiteRules(SiteVideoSocial, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := siteDomains(t, s, SiteVideoSocial), []string{}; !reflect.DeepEqual(got, want) {
		t.Errorf("видео после пустой замены = %v, want %v", got, want)
	}
	if list, err := s.ListSiteRules(); err != nil || list == nil || len(list) != 1 {
		t.Errorf("ListSiteRules = %#v, %v; want одна запись example.com", list, err)
	}
}

// Ловит (S-7, RM4-7): «Итоги» за день и период теряют строки сайтов или их вид; учёт сайта
// попадает во «время за неделю» программ; повторная порция сайта не складывается.
func TestDayUsageWithSites(t *testing.T) {
	s := openTemp(t)
	id := mustChild(t, s, "S-1", "Миша")
	for _, u := range []struct {
		category, kind, subject string
		seconds                 int
	}{
		{UsageVideoSocial, SubjectSite, "youtube.com", 600},
		{UsageGames, SubjectProgram, `c:\games\game.exe`, 300},
		{UsageVideoSocial, SubjectSite, "youtube.com", 60},
	} {
		if err := s.AddUsage(id, "2026-11-05", u.category, u.kind, u.subject, u.seconds); err != nil {
			t.Fatal(err)
		}
	}
	day, err := s.DayUsage(id, "2026-11-05")
	if err != nil {
		t.Fatal(err)
	}
	want := []UsageRow{
		{Day: "2026-11-05", Category: UsageGames, Kind: SubjectProgram, Subject: `c:\games\game.exe`, Seconds: 300},
		{Day: "2026-11-05", Category: UsageVideoSocial, Kind: SubjectSite, Subject: "youtube.com", Seconds: 660},
	}
	if !reflect.DeepEqual(day, want) {
		t.Errorf("DayUsage = %+v, want %+v", day, want)
	}
	period, err := s.PeriodUsage(id, "2026-11-01", "2026-11-06")
	if err != nil {
		t.Fatal(err)
	}
	for i := range want {
		want[i].Day = ""
	}
	if !reflect.DeepEqual(period, want) {
		t.Errorf("PeriodUsage = %+v, want %+v", period, want)
	}
	if got, err := s.ProgramSeconds("2026-11-01"); err != nil || !reflect.DeepEqual(got, map[string]int{`c:\games\game.exe`: 300}) {
		t.Errorf("ProgramSeconds = %v, %v; want только программа (сайта нет)", got, err)
	}
}
