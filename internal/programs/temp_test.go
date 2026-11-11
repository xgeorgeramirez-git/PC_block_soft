package programs

import "testing"

// Решения родителя для программ во временной папке профиля (FB-19, RM3-13): установщик Roblox
// перезаписывается при каждом запуске игры в «AppData\Local\Temp\Roblox\».
var tempEntries = []Entry{
	{Path: `c:\users\danila\appdata\local\temp\roblox\robloxplayerinstaller.exe`, Name: "RobloxPlayerInstaller",
		Category: "games", Publisher: "Roblox Corporation", Fingerprint: "5000:1"},
	// Решение по файлу прямо во временной папке: имена там случайные, правило не создаётся.
	{Path: `c:\users\danila\appdata\local\temp\loose.exe`, Name: "Loose", Category: "games",
		Publisher: "Loose Inc.", Fingerprint: "7:7"},
	// LocalLow не корень профиля (RM3-13) — и его «temp» не временная папка Windows.
	{Path: `c:\users\danila\appdata\locallow\temp\game\g.exe`, Name: "G", Category: "games",
		Publisher: "G Inc.", Fingerprint: "3:3"},
	// Данные Магазина (Local\Packages) — не корень профиля и не временная папка.
	{Path: `c:\users\danila\appdata\local\packages\p\x.exe`, Name: "P", Category: "games",
		Publisher: "P Inc.", Fingerprint: "3:3"},
	// «local\temp» не внутри AppData — не временная папка Windows.
	{Path: `c:\users\danila\foo\local\temp\a\x.exe`, Name: "Foo", Category: "games",
		Publisher: "Foo Inc.", Fingerprint: "3:3"},
	// Roaming\Temp — обычный корень профиля (RM3-13): его решение не переходит в Local\Temp.
	{Path: `c:\users\danila\appdata\roaming\temp\roblox\robloxplayerinstaller.exe`, Name: "RoamingInstaller", Category: "blocked",
		Publisher: "Roblox Corporation", Fingerprint: "3:3"},
}

// Ловит: решение родителя по файлу во временной папке теряется при перезаписи файла (новый
// отпечаток при том же имени и издателе, FB-19); или, наоборот, переходит на файл другого
// издателя, без издателя, с другим именем, в другую папку временных файлов или на файл прямо
// в Temp (случайные имена); решение по файлу прямо в Temp становится правилом.
func TestClassifyTempFolder(t *testing.T) {
	c := NewClassifier(testRoots, DefaultCatalog)
	c.Reload(tempEntries)
	const temp = `c:\users\danila\appdata\local\temp\`
	cases := []struct {
		name, path, fp, pub string
		want                Result
	}{
		{"FB-19: установщик перезаписан, тот же издатель", temp + `roblox\robloxplayerinstaller.exe`, "5100:2", "Roblox Corporation",
			Result{Verdict: VerdictAllow, Category: "games", Name: "RobloxPlayerInstaller"}},
		{"тот же отпечаток", temp + `roblox\robloxplayerinstaller.exe`, "5000:1", "Roblox Corporation",
			Result{Verdict: VerdictAllow, Category: "games", Known: true, Name: "RobloxPlayerInstaller"}},
		{"подмена: другой издатель", temp + `roblox\robloxplayerinstaller.exe`, "5100:2", "Evil",
			Result{Verdict: VerdictUnknown, Name: "RobloxPlayerInstaller", WasCategory: "games"}},
		{"подмена: без издателя", temp + `roblox\robloxplayerinstaller.exe`, "5100:2", "",
			Result{Verdict: VerdictUnknown, Name: "RobloxPlayerInstaller", WasCategory: "games"}},
		{"свойства не прочитаны (Д-1)", temp + `roblox\robloxplayerinstaller.exe`, "", "",
			Result{Verdict: VerdictUnknown, Name: "RobloxPlayerInstaller", WasCategory: "games"}},
		{"у другого ребёнка в той же папке Temp", `c:\users\миша\appdata\local\temp\roblox\robloxplayerinstaller.exe`, "1:1", "Roblox Corporation",
			Result{Verdict: VerdictAllow, Category: "games", Name: "robloxplayerinstaller"}},
		{"в подпапке той же папки Temp", temp + `roblox\sub\robloxplayerinstaller.exe`, "1:1", "Roblox Corporation",
			Result{Verdict: VerdictAllow, Category: "games", Name: "robloxplayerinstaller"}},
		{"другое имя в той же папке Temp", temp + `roblox\game.exe`, "1:1", "Roblox Corporation",
			Result{Verdict: VerdictUnknown, Name: "game"}},
		{"другая папка Temp", temp + `other\robloxplayerinstaller.exe`, "1:1", "Roblox Corporation",
			Result{Verdict: VerdictUnknown, Name: "robloxplayerinstaller"}},
		{"файл прямо в Temp", temp + `robloxplayerinstaller.exe`, "1:1", "Roblox Corporation",
			Result{Verdict: VerdictUnknown, Name: "robloxplayerinstaller"}},
		{"решение по файлу прямо в Temp — только точный путь", temp + `loose.exe`, "7:7", "Loose Inc.",
			Result{Verdict: VerdictAllow, Category: "games", Known: true, Name: "Loose"}},
		{"файл прямо в Temp перезаписан", temp + `loose.exe`, "8:8", "Loose Inc.",
			Result{Verdict: VerdictUnknown, Name: "Loose", WasCategory: "games"}},
		{"Packages перезаписан — подмена", `c:\users\danila\appdata\local\packages\p\x.exe`, "4:4", "P Inc.",
			Result{Verdict: VerdictUnknown, Name: "P", WasCategory: "games"}},
		{"local\\temp вне AppData перезаписан — подмена", `c:\users\danila\foo\local\temp\a\x.exe`, "4:4", "Foo Inc.",
			Result{Verdict: VerdictUnknown, Name: "Foo", WasCategory: "games"}},
		{"запрет в Roaming\\Temp не переходит в Local\\Temp", `c:\users\миша\appdata\local\temp\roblox\robloxplayerinstaller.exe`, "1:1", "Roblox Corporation",
			Result{Verdict: VerdictAllow, Category: "games", Name: "robloxplayerinstaller"}},
		{"игра из Local\\Temp не переходит в Roaming\\Temp", `c:\users\миша\appdata\roaming\temp\roblox\robloxplayerinstaller.exe`, "1:1", "Roblox Corporation",
			Result{Verdict: VerdictForbidden, Category: "blocked", Name: "robloxplayerinstaller"}},
		{"LocalLow\\Temp перезаписан — подмена", `c:\users\danila\appdata\locallow\temp\game\g.exe`, "4:4", "G Inc.",
			Result{Verdict: VerdictUnknown, Name: "G", WasCategory: "games"}},
		// B-53: установщик Brave/Chromium распаковывается в папку со случайным именем — решения
		// родителя по ней нет, правило FB-19 его не касается: по-прежнему «ждёт решения».
		{"B-53: setup.exe Brave в случайной папке Temp", temp + `cr_8a1f2.tmp\setup.exe`, "1:1", "Brave Software, Inc.",
			Result{Verdict: VerdictUnknown, Name: "setup"}},
	}
	for _, k := range cases {
		if got := c.Classify(k.path, k.fp, k.pub); got != k.want {
			t.Errorf("%s: Classify(%q, %q, %q) = %+v, want %+v", k.name, k.path, k.fp, k.pub, got, k.want)
		}
	}
}

// Ловит: издатель не читается для файла во временной папке, по которому есть решение (без
// издателя правило FB-19 не сработает), или читается для каждого файла Temp (замедление опроса).
func TestNeedsPublisherTemp(t *testing.T) {
	c := NewClassifier(testRoots, DefaultCatalog)
	c.Reload(tempEntries)
	cases := []struct {
		path string
		want bool
	}{
		{`c:\users\danila\appdata\local\temp\roblox\robloxplayerinstaller.exe`, true},
		{`c:\users\миша\appdata\local\temp\roblox\robloxplayerinstaller.exe`, true},
		{`c:\users\danila\appdata\local\temp\roblox\game.exe`, false},
		{`c:\users\danila\appdata\local\temp\other\robloxplayerinstaller.exe`, false},
		{`c:\users\danila\appdata\local\temp\robloxplayerinstaller.exe`, false},
	}
	for _, k := range cases {
		if got := c.NeedsPublisher(k.path); got != k.want {
			t.Errorf("NeedsPublisher(%q) = %v, want %v", k.path, got, k.want)
		}
	}
}

// Ловит: временная папка становится корнем приложения (AppRoot, ProfileRoot): тогда по нему
// шло бы наследование категорий и запись app_root в базе (решение FB-19 — только по имени файла
// и издателю).
func TestTempNotAppRoot(t *testing.T) {
	p := `c:\users\danila\appdata\local\temp\roblox\robloxplayerinstaller.exe`
	if got := testRoots.AppRoot(p); got != "" {
		t.Errorf("AppRoot(%q) = %q, want пусто", p, got)
	}
	if got := testRoots.ProfileRoot(p); got != "" {
		t.Errorf("ProfileRoot(%q) = %q, want пусто", p, got)
	}
}
