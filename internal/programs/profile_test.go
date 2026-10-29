package programs

import "testing"

// Ловит: компоненты Защитника Windows из ProgramData «незнакомые» (наблюдатель закрыл бы их,
// сухой прогон 40954826900) или класс «система» ловит соседние папки без границы разделителя.
func TestIsSystemDefender(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{`c:\programdata\microsoft\windows defender\platform\4.18.26090.9-0\defendersessionhelper.exe`, true},
		{`C:\ProgramData\Microsoft\Windows Defender\Platform\4.18.26090.9-0\MsMpEng.exe`, true},
		{`c:\programdata\microsoft\windows defender advanced threat protection\platform\1.0\mssense.exe`, true},
		{`c:\programdata\microsoft\windows defender games\x.exe`, false},
		{`c:\programdata\microsoft\windows defenderx\x.exe`, false},
		{`c:\programdata\microsoft\windows defender`, false},
		{`c:\programdata\microsoft\x.exe`, false},
		{`c:\programdata\roblox\x.exe`, false},
		{`c:\users\миша\microsoft\windows defender\x.exe`, false},
	}
	for _, c := range cases {
		if got := testRoots.IsSystem(c.path); got != c.want {
			t.Errorf("IsSystem(%q) = %v, want %v", c.path, got, c.want)
		}
	}
	if (Roots{SystemRoot: `c:\windows`}).IsSystem(`c:\programdata\microsoft\windows defender\x.exe`) {
		t.Error("пустой ProgramData: префикс Защитника от корня диска")
	}
}

// Ловит: корень приложения в профиле берётся не по форме пути (RM3-13): не первая папка под
// AppData\Local и Roaming, не вторая под Local\Programs и Local\Microsoft; корнем становятся
// Packages (данные Store), Temp, сама папка, «Загрузки» или похожие на Users папки.
func TestProfileRoot(t *testing.T) {
	cases := []struct{ path, want string }{
		{`C:\Users\Миша\AppData\Local\Roblox\Versions\version-111\RobloxPlayerBeta.exe`, `c:\users\миша\appdata\local\roblox`},
		{`c:\users\миша\appdata\local\discord\app-1.0.9001\discord.exe`, `c:\users\миша\appdata\local\discord`},
		{`c:\users\миша\appdata\local\programs\cursor\cursor.exe`, `c:\users\миша\appdata\local\programs\cursor`},
		{`c:\users\миша\appdata\local\microsoft\onedrive\24.1\onedrive.exe`, `c:\users\миша\appdata\local\microsoft\onedrive`},
		{`c:\users\миша\appdata\local\microsoft\teams\current\teams.exe`, `c:\users\миша\appdata\local\microsoft\teams`},
		{`c:\users\миша\appdata\roaming\spotify\spotify.exe`, `c:\users\миша\appdata\roaming\spotify`},
		{`c:\users\миша\appdata\roaming\microsoft\x\x.exe`, `c:\users\миша\appdata\roaming\microsoft\x`},
		{`c:\users\миша\appdata\local\packages\microsoft.minecraftuwp_8wekyb3d8bbwe\x.exe`, ``},
		{`c:\users\миша\appdata\local\temp\setup\x.exe`, ``},
		{`c:\users\миша\appdata\local\x.exe`, ``},
		{`c:\users\миша\appdata\local\programs\x.exe`, ``},
		{`c:\users\миша\appdata\local\microsoft\x.exe`, ``},
		{`c:\users\миша\appdata\locallow\game\game.exe`, ``},
		{`c:\users\миша\downloads\game\game.exe`, ``},
		{`c:\users\миша\desktop\game.exe`, ``},
		{`c:\usersx\миша\appdata\local\roblox\r.exe`, ``},
		{`d:\users\миша\appdata\local\roblox\r.exe`, ``},
		{`c:\program files\brave software\brave.exe`, ``},
	}
	for _, c := range cases {
		if got := testRoots.ProfileRoot(c.path); got != c.want {
			t.Errorf("ProfileRoot(%q) = %q, want %q", c.path, got, c.want)
		}
	}
	// Запись решения хранит корень профиля как корень приложения.
	if got := testRoots.AppRoot(`c:\users\миша\appdata\local\discord\app-1.0.9001\discord.exe`); got != `c:\users\миша\appdata\local\discord` {
		t.Errorf("AppRoot в профиле = %q", got)
	}
	if got := (Roots{}).ProfileRoot(`c:\users\миша\appdata\local\roblox\r.exe`); got != "" {
		t.Errorf("без UserProfiles: %q", got)
	}
}

// Решения родителя в профилях детей (RM3-13).
var profileEntries = []Entry{
	{Path: `c:\users\миша\appdata\local\roblox\versions\version-111\robloxplayerbeta.exe`, Name: "Roblox", Category: "games",
		Publisher: "Roblox Corporation", Fingerprint: "10:1"},
	{Path: `c:\users\миша\appdata\local\discord\app-1.0.9001\discord.exe`, Name: "Discord", Category: "other",
		Publisher: "Discord Inc.", Fingerprint: "20:2"},
	{Path: `c:\users\миша\appdata\local\programs\cursor\cursor.exe`, Name: "Cursor", Category: "study", Publisher: "Anysphere"},
	{Path: `c:\users\миша\appdata\local\microsoft\onedrive\24.1\onedrive.exe`, Name: "OneDrive", Category: "other",
		Publisher: "Microsoft Corporation"},
	{Path: `c:\users\миша\appdata\roaming\badgame\v1\badgame.exe`, Name: "BadGame", Category: "blocked", Publisher: "Bad Inc."},
	{Path: `c:\users\миша\appdata\local\nopub\v1\nopub.exe`, Name: "NoPub", Category: "games"},
	{Path: `c:\users\миша\downloads\supergame.exe`, Name: "SuperGame", Category: "games", Publisher: "Super", Fingerprint: "1:1"},
}

// Ловит: решение родителя в профиле теряется при обновлении в новую папку версии, переходит на
// exe с другим именем или другим/пустым издателем (переименованная игра в папке Discord),
// действует без издателя у решения, выходит за пределы корня приложения, распространяется на
// «Загрузки»; Защитник «незнакомый».
func TestClassifyProfileApps(t *testing.T) {
	c := NewClassifier(testRoots, DefaultCatalog)
	c.Reload(profileEntries)
	const local = `c:\users\миша\appdata\local\`
	cases := []struct {
		name, path, fp, pub string
		want                Result
	}{
		{"новая версия Roblox того же издателя", local + `roblox\versions\version-222\robloxplayerbeta.exe`, "", "Roblox Corporation",
			Result{Verdict: VerdictAllow, Category: "games", Name: "robloxplayerbeta"}},
		{"издатель без учёта регистра и пробелов", local + `roblox\versions\version-222\robloxplayerbeta.exe`, "", " roblox corporation ",
			Result{Verdict: VerdictAllow, Category: "games", Name: "robloxplayerbeta"}},
		{"Roblox другого издателя", local + `roblox\versions\version-222\robloxplayerbeta.exe`, "", "Evil",
			Result{Verdict: VerdictUnknown, Name: "robloxplayerbeta"}},
		{"Roblox без издателя", local + `roblox\versions\version-222\robloxplayerbeta.exe`, "", "",
			Result{Verdict: VerdictUnknown, Name: "robloxplayerbeta"}},
		{"game.exe рядом с Roblox того же издателя", local + `roblox\versions\version-222\game.exe`, "", "Roblox Corporation",
			Result{Verdict: VerdictUnknown, Name: "game"}},
		{"Roblox у Коли (решение для всех детей)", `c:\users\коля\appdata\local\roblox\versions\version-222\robloxplayerbeta.exe`, "", "Roblox Corporation",
			Result{Verdict: VerdictAllow, Category: "games", Name: "robloxplayerbeta"}},
		{"Roblox в Roaming (другой корень)", `c:\users\миша\appdata\roaming\roblox\versions\v\robloxplayerbeta.exe`, "", "Roblox Corporation",
			Result{Verdict: VerdictUnknown, Name: "robloxplayerbeta"}},
		{"точный путь Roblox с отпечатком", local + `roblox\versions\version-111\robloxplayerbeta.exe`, "10:1", "Roblox Corporation",
			Result{Verdict: VerdictAllow, Category: "games", Known: true, Name: "Roblox"}},
		{"точный путь Roblox подменён", local + `roblox\versions\version-111\robloxplayerbeta.exe`, "99:9", "Roblox Corporation",
			Result{Verdict: VerdictUnknown, Name: "Roblox", WasCategory: "games"}},
		{"новая версия Discord", local + `discord\app-1.0.9002\discord.exe`, "", "Discord Inc.",
			Result{Verdict: VerdictAllow, Category: "other", Name: "discord"}},
		{"игра, переименованная в discord.exe, без издателя", local + `discord\app-1.0.9002\discord.exe`, "", "",
			Result{Verdict: VerdictUnknown, Name: "discord"}},
		{"игра, переименованная в discord.exe, чужой издатель", local + `discord\game\discord.exe`, "", "Epic Games",
			Result{Verdict: VerdictUnknown, Name: "discord"}},
		{"Cursor: учёба наследуется в профиле", local + `programs\cursor\resources\app\cursor.exe`, "", "Anysphere",
			Result{Verdict: VerdictAllow, Category: "study", Name: "cursor"}},
		{"cursor.exe в другой папке Programs", local + `programs\other\cursor.exe`, "", "Anysphere",
			Result{Verdict: VerdictUnknown, Name: "cursor"}},
		{"новая версия OneDrive", local + `microsoft\onedrive\25.0\onedrive.exe`, "", "Microsoft Corporation",
			Result{Verdict: VerdictAllow, Category: "other", Name: "onedrive"}},
		{"onedrive.exe в папке Teams", local + `microsoft\teams\onedrive.exe`, "", "Microsoft Corporation",
			Result{Verdict: VerdictUnknown, Name: "onedrive"}},
		{"новая версия запрещённой игры", `c:\users\миша\appdata\roaming\badgame\v2\badgame.exe`, "", "Bad Inc.",
			Result{Verdict: VerdictForbidden, Category: "blocked", Name: "badgame"}},
		{"решение без издателя не наследуется", local + `nopub\v2\nopub.exe`, "", "",
			Result{Verdict: VerdictUnknown, Name: "nopub"}},
		{"решение без издателя, exe с издателем", local + `nopub\v2\nopub.exe`, "", "Any",
			Result{Verdict: VerdictUnknown, Name: "nopub"}},
		{"Загрузки: только точный путь", `c:\users\миша\downloads\new\supergame.exe`, "", "Super",
			Result{Verdict: VerdictUnknown, Name: "supergame"}},
		{"Packages: не корень", local + `packages\roblox\versions\v\robloxplayerbeta.exe`, "", "Roblox Corporation",
			Result{Verdict: VerdictUnknown, Name: "robloxplayerbeta"}},
		{"помощник Защитника", `c:\programdata\microsoft\windows defender\platform\4.18.26090.9-0\defendersessionhelper.exe`, "", "",
			Result{Verdict: VerdictSystem, Name: "defendersessionhelper"}},
		{"Windows Defender Games не система", `c:\programdata\microsoft\windows defender games\x.exe`, "", "Microsoft Corporation",
			Result{Verdict: VerdictUnknown, Name: "x"}},
	}
	for _, k := range cases {
		if got := c.Classify(k.path, k.fp, k.pub); got != k.want {
			t.Errorf("%s: Classify(%q, %q, %q) = %+v, want %+v", k.name, k.path, k.fp, k.pub, got, k.want)
		}
	}
}

// Ловит: издатель читается для каждого процесса в профиле (замедление опроса, RM3-11) или не
// читается там, где решение по имени файла есть.
func TestNeedsPublisher(t *testing.T) {
	c := NewClassifier(testRoots, DefaultCatalog)
	c.Reload(profileEntries)
	cases := []struct {
		path string
		want bool
	}{
		{`c:\users\коля\appdata\local\roblox\versions\version-222\robloxplayerbeta.exe`, true},
		{`c:\users\миша\appdata\local\discord\app-1.0.9002\discord.exe`, true},
		{`c:\users\миша\appdata\local\roblox\versions\version-111\robloxplayerbeta.exe`, false}, // точный путь
		{`c:\users\миша\appdata\local\roblox\versions\version-222\game.exe`, false},             // имени нет в решениях
		{`c:\users\миша\appdata\local\nopub\v2\nopub.exe`, false},                               // решение без издателя
		{`c:\users\миша\appdata\local\spotify\spotify.exe`, false},                              // в корне решений нет
		{`c:\users\миша\downloads\supergame.exe`, false},
		{`c:\program files\brave software\brave.exe`, false},
	}
	for _, k := range cases {
		if got := c.NeedsPublisher(k.path); got != k.want {
			t.Errorf("NeedsPublisher(%q) = %v, want %v", k.path, got, k.want)
		}
	}
}

// Ловит: старое решение в профиле без издателя (до RM3-13) перестаёт работать по точному пути.
func TestProfileOldDecisionExactPath(t *testing.T) {
	c := NewClassifier(testRoots, DefaultCatalog)
	c.Reload([]Entry{{Path: `c:\users\миша\appdata\local\nopub\v1\nopub.exe`, Name: "NoPub", Category: "games"}})
	if got := c.Classify(`c:\users\миша\appdata\local\nopub\v1\nopub.exe`, "", ""); got.Verdict != VerdictAllow || !got.Known {
		t.Errorf("старое решение по точному пути: %+v", got)
	}
}
