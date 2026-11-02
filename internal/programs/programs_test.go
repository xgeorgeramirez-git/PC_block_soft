package programs

import (
	"strings"
	"sync"
	"testing"
)

// Корни, как их отдаёт адаптер Windows (KnownFolders, уже нормализованные).
var testRoots = Roots{
	SystemRoot:      `c:\windows`,
	ProgramFiles:    `c:\program files`,
	ProgramFilesX86: `c:\program files (x86)`,
	InstallDir:      `c:\program files\pcblock`,
	ProgramData:     `c:\programdata`,
	UserProfiles:    `c:\users`,
}

// Ловит: нормализатор оставляет регистр, прямые слеши, «..», двойные разделители, хвостовой «\»
// или префикс «\\?\»; принимает относительный путь (его нельзя сравнивать с корнями).
func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{`C:\Program Files\Brave Software\Brave-Browser\Application\brave.exe`, `c:\program files\brave software\brave-browser\application\brave.exe`},
		{`c:/users/kid/Downloads/game.exe`, `c:\users\kid\downloads\game.exe`},
		{`c:\program files\..\users\kid\game.exe`, `c:\users\kid\game.exe`},
		{`c:\program files\.\x\\y.exe`, `c:\program files\x\y.exe`},
		{`c:\program files\x\`, `c:\program files\x`},
		{`\\?\C:\Program Files\X\x.exe`, `c:\program files\x\x.exe`},
		{`\\?\UNC\Server\Share\x.exe`, `\\server\share\x.exe`},
		{`\\server\share\dir\..\x.exe`, `\\server\share\x.exe`},
		{`\\server\share\..\..\x.exe`, `\\server\share\x.exe`},
		{`c:\..\..\x.exe`, `c:\x.exe`},
		{`  c:\x.exe`, ``}, // пробел в начале: путь не абсолютный; пробелы и точки внутри не срезаются (\\?\-папка «program files.» не Program Files)
		{`c:\program files.\x.exe`, `c:\program files.\x.exe`},
		{`c:\program files \x.exe`, `c:\program files \x.exe`},
		{`C:\`, `c:\`},
		{`c:\users\RUNNER~1\AppData\Local\Temp\x.exe`, `c:\users\runner~1\appdata\local\temp\x.exe`},
		{`c:\Users\Миша\Загрузки\Игра.EXE`, `c:\users\миша\загрузки\игра.exe`},
		{``, ``},
		{`game.exe`, ``},
		{`downloads\game.exe`, ``},
		{`\windows\x.exe`, ``},
		{`c:game.exe`, ``},
		{`c:`, ``},
		{`\\server`, ``},
		{`\\server\`, ``},
		{"c:\\x\x00.exe", ``},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Ловит: доверенный каталог проверяется простым HasPrefix без разделителя (C:\Program Files Games\
// и C:\Program Files.evil\ становятся доверенными), сам корень считается файлом внутри него,
// сетевой путь или путь с «..» проходит проверку.
func TestRootsIsTrusted(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{`c:\program files\brave software\brave.exe`, true},
		{`C:\PROGRAM FILES\Brave Software\brave.exe`, true},
		{`c:\program files (x86)\steam\steam.exe`, true},
		{`c:\windows\system32\notepad.exe`, true},
		{`c:\program files games\x.exe`, false},
		{`c:\program files.evil\x.exe`, false},
		{`c:\program filesx\x.exe`, false},
		{`c:\program files`, false},
		{`c:\program files\`, false},
		{`c:\program files\..\users\kid\x.exe`, false},
		{`\\server\share\x.exe`, false},
		{`\\?\c:\program files\x\x.exe`, true},
		{`c:\users\kid\downloads\x.exe`, false},
		{`d:\program files\x\x.exe`, false},
		{``, false},
		{`program files\x.exe`, false},
	}
	for _, c := range cases {
		if got := testRoots.IsTrusted(c.path); got != c.want {
			t.Errorf("IsTrusted(%q) = %v, want %v", c.path, got, c.want)
		}
	}
	if (Roots{}).IsTrusted(`c:\program files\x\x.exe`) {
		t.Error("пустые корни: путь доверенный, want нет (пустой корень не префикс всего)")
	}
}

// Ловит: системными становятся папки с похожим именем или WindowsApps в папке ребёнка; каталог
// установки PC Block, WebView2 или WindowsApps не считаются системой (наблюдатель убьёт свои окна).
func TestRootsIsSystem(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{`c:\windows\system32\notepad.exe`, true},
		{`c:\windows\explorer.exe`, true},
		{`c:\windowsold\x.exe`, false},
		{`c:\program files\pcblock\pcblock.exe`, true},
		{`c:\program files\pcblock\guard\pcblock-guard.exe`, true},
		{`c:\program files\pcblockx\x.exe`, false},
		// Д-3: приложения Microsoft Store ребёнок ставит сам, поэтому WindowsApps не «система»;
		// «система» в нём только пакеты компонентов Windows (издатель cw5n1h2txyewy).
		{`c:\program files\windowsapps\microsoft.windowscalculator_11.2405.2.0_x64__8wekyb3d8bbwe\calculatorapp.exe`, false},
		{`c:\program files\windowsapps\microsoftwindows.client.webexperience_524.33200.0.0_x64__cw5n1h2txyewy\widgets.exe`, true},
		{`c:\program files\windowsapps\fake_1.0.0.0_x64__cw5n1h2txyewyx\x.exe`, false},
		{`c:\program files (x86)\windowsapps\x\x.exe`, false},
		{`c:\windows\systemapps\microsoft.windows.startmenuexperiencehost_cw5n1h2txyewy\startmenuexperiencehost.exe`, true},
		{`c:\users\kid\windowsapps\game.exe`, false},
		{`c:\users\kid\appdata\local\microsoft\windowsapps\game.exe`, false},
		{`c:\program files (x86)\microsoft\edgewebview\application\1.2.3\msedgewebview2.exe`, true},
		{`c:\program files\microsoft\edgewebview\application\1.2.3\msedgewebview2.exe`, true},
		{`c:\program files (x86)\microsoft\edge\application\msedge.exe`, false},
		{`c:\program files (x86)\microsoft\edgewebviewx\x.exe`, false},
		{`c:\users\kid\microsoft\edgewebview\x.exe`, false},
		{`c:\program files\brave software\brave.exe`, false},
		{`c:\users\kid\downloads\x.exe`, false},
		{``, false},
	}
	for _, c := range cases {
		if got := testRoots.IsSystem(c.path); got != c.want {
			t.Errorf("IsSystem(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// Ловит: корень приложения берётся не по первой папке внутри Program Files, находится у файла
// прямо в Program Files или у пути вне доверенных; общие папки Microsoft и Common Files склеивают
// разные приложения в одно.
func TestRootsAppRoot(t *testing.T) {
	cases := []struct{ path, want string }{
		{`c:\program files\brave software\brave-browser\application\123.0\updater.exe`, `c:\program files\brave software`},
		{`C:\Program Files (x86)\Steam\steamapps\common\Game\game.exe`, `c:\program files (x86)\steam`},
		{`c:\program files\geogebra\geogebra.exe`, `c:\program files\geogebra`},
		{`c:\program files\x.exe`, ``},
		{`c:\program files\geogebra`, ``},
		{`c:\program files (x86)\microsoft\edge\application\msedge.exe`, `c:\program files (x86)\microsoft\edge`},
		{`c:\program files\common files\vendor\tool.exe`, `c:\program files\common files\vendor`},
		{`c:\program files\microsoft\x.exe`, ``},
		{`c:\program files games\x\x.exe`, ``},
		// Д-3: корень приложения Store — семейство пакета без версии (обновление не «незнакомое»).
		{`c:\program files\windowsapps\microsoft.windowscalculator_11.2405.2.0_x64__8wekyb3d8bbwe\calculatorapp.exe`,
			`c:\program files\windowsapps\microsoft.windowscalculator__8wekyb3d8bbwe`},
		{`c:\program files\windowsapps\microsoft.minecraftuwp_1.21.0.0_x64__8wekyb3d8bbwe\data\minecraft.windows.exe`,
			`c:\program files\windowsapps\microsoft.minecraftuwp__8wekyb3d8bbwe`},
		{`c:\program files\windowsapps\microsoft.bundle_1.0.0.0_neutral_~_8wekyb3d8bbwe\x.exe`,
			`c:\program files\windowsapps\microsoft.bundle__8wekyb3d8bbwe`},
		{`c:\program files\windowsapps\strangefolder\x.exe`, `c:\program files\windowsapps\strangefolder`},
		{`c:\program files\windowsapps\x.exe`, ``},
		{`c:\users\kid\windowsapps\pkg_1_x64__p\x.exe`, ``},
		{`c:\windows\system32\notepad.exe`, ``},
		{`c:\users\kid\downloads\x\x.exe`, ``},
		{``, ``},
	}
	for _, c := range cases {
		if got := testRoots.AppRoot(c.path); got != c.want {
			t.Errorf("AppRoot(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// Ловит: каталог беден (меньше 40 игр), в него попал javaw.exe (Java-программы — не только
// Minecraft), браузер стал «Игрой», регистр или путь мешают поиску по имени файла.
func TestCatalog(t *testing.T) {
	c := DefaultCatalog
	if n := len(c.Games); n < 40 {
		t.Errorf("игр в каталоге %d, want не меньше 40", n)
	}
	for name := range c.Games {
		if name != strings.ToLower(name) || strings.ContainsAny(name, `\/`) || !strings.HasSuffix(name, ".exe") {
			t.Errorf("игра %q: имя файла должно быть в нижнем регистре, без папок, с .exe", name)
		}
		if _, dup := c.Browsers[name]; dup {
			t.Errorf("%q и игра, и браузер", name)
		}
	}
	cases := []struct {
		name   string
		cat    string
		ok     bool
		browse bool
	}{
		{"minecraftlauncher.exe", "games", true, false},
		{"RobloxPlayerBeta.exe", "games", true, false},
		{`c:\program files (x86)\steam\steam.exe`, "games", true, false},
		{"cs2.exe", "games", true, false},
		{"brave.exe", "other", true, true},
		{"chrome.exe", "other", true, true},
		{"MSEDGE.EXE", "other", true, true},
		{"firefox.exe", "other", true, true},
		{"javaw.exe", "", false, false},
		{"notepad.exe", "", false, false},
		{"", "", false, false},
	}
	for _, k := range cases {
		cat, ok := c.Lookup(k.name)
		if cat != k.cat || ok != k.ok {
			t.Errorf("Lookup(%q) = %q, %v; want %q, %v", k.name, cat, ok, k.cat, k.ok)
		}
		if got := c.IsBrowser(k.name); got != k.browse {
			t.Errorf("IsBrowser(%q) = %v, want %v", k.name, got, k.browse)
		}
	}
}

// testEntries — список программ, как его хранит служба.
var testEntries = []Entry{
	{Path: `c:\program files\brave software\brave-browser\application\brave.exe`, Name: "Brave", Category: "other"},
	{Path: `c:\program files (x86)\steam\steam.exe`, Name: "Steam", Category: "games"},
	{Path: `c:\program files (x86)\steam\uninstall.exe`, Name: "Steam uninstall", Category: "other"},
	{Path: `c:\program files\geogebra\geogebra.exe`, Name: "GeoGebra", Category: "study"},
	{Path: `c:\program files\roblox\versions\version-1\robloxplayerbeta.exe`, Name: "Roblox", Category: "blocked"},
	{Path: `c:\program files\roblox\versions\version-1\robloxstudio.exe`, Name: "Roblox Studio", Category: "study"},
	{Path: `c:\users\миша\downloads\supergame.exe`, Name: "SuperGame", Category: "games", Fingerprint: "1024:1700000000"},
	{Path: `c:\users\миша\downloads\homework.exe`, Name: "Homework", Category: "study"},
	{Path: `c:\program files\vendor\driverhelper.exe`, Name: "Driver helper", Category: "system"},
	{Path: `c:\windows\system32\notepad.exe`, Name: "Блокнот", Category: "blocked"},
	// Родитель по ошибке запретил WebView2: окна PC Block всё равно не закрываются (система раньше списка).
	{Path: `c:\program files (x86)\microsoft\edgewebview\application\1.2.3\msedgewebview2.exe`, Name: "WebView2", Category: "blocked"},
}

func newTestClassifier() *Classifier {
	c := NewClassifier(testRoots, DefaultCatalog)
	c.Reload(testEntries)
	return c
}

// Ловит: порядок RM3-1 нарушен (точный путь раньше системы, каталог для недоверенных путей,
// корень приложения раньше точного пути), неизвестный путь вне Windows не «незнакомый»,
// непригодный путь закрывается (PL-9: «не знаю» = «не трогать»).
func TestClassify(t *testing.T) {
	c := newTestClassifier()
	cases := []struct {
		name, path, fp string
		want           Result
	}{
		{"переименованный chrome в Загрузках", `C:\Users\kid\Downloads\chrome.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "chrome"}},
		{"chrome в Program Files по каталогу", `C:\Program Files\Google\Chrome\Application\chrome.exe`, "",
			Result{Verdict: VerdictAllow, Category: "other", Name: "chrome"}},
		{"игра из каталога в Program Files", `c:\program files\minecraft launcher\MinecraftLauncher.exe`, "",
			Result{Verdict: VerdictAllow, Category: "games", Name: "MinecraftLauncher"}},
		{"игра из каталога вне доверенных", `c:\users\kid\games\minecraftlauncher.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "minecraftlauncher"}},
		{"Program Files Games не доверенный", `c:\program files games\x.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "x"}},
		{"Program Files Games с именем из каталога", `c:\program files games\steam.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "steam"}},
		{"обновление Brave через корень", `c:\program files\brave software\brave-browser\application\123.0\updater.exe`, "",
			Result{Verdict: VerdictAllow, Category: "other", Name: "updater"}},
		{"точный путь Brave", `c:\program files\brave software\brave-browser\application\brave.exe`, "",
			Result{Verdict: VerdictAllow, Category: "other", Known: true, Name: "Brave"}},
		{"игра Steam наследует «Игры» корня (games > other)", `c:\program files (x86)\steam\steamapps\common\game\game.exe`, "",
			Result{Verdict: VerdictAllow, Category: "games", Name: "game"}},
		{"учёба не наследуется: помощник GeoGebra — «Прочее»", `c:\program files\geogebra\helper.exe`, "",
			Result{Verdict: VerdictAllow, Category: "other", Name: "helper"}},
		{"точный путь учёбы", `c:\program files\geogebra\geogebra.exe`, "",
			Result{Verdict: VerdictAllow, Category: "study", Known: true, Name: "GeoGebra"}},
		{"новая версия запрещённого Roblox наследует запрет", `c:\program files\roblox\versions\version-2\robloxplayerbeta.exe`, "",
			Result{Verdict: VerdictForbidden, Category: "blocked", Name: "robloxplayerbeta"}},
		{"запрещённая программа по точному пути", `c:\program files\roblox\versions\version-1\robloxplayerbeta.exe`, "",
			Result{Verdict: VerdictForbidden, Category: "blocked", Known: true, Name: "Roblox"}},
		{"блокнот: система раньше точного пути", `c:\windows\system32\notepad.exe`, "",
			Result{Verdict: VerdictSystem, Name: "notepad"}},
		{"WebView2: система раньше каталога и корня", `c:\program files (x86)\microsoft\edgewebview\application\1.2.3\msedgewebview2.exe`, "",
			Result{Verdict: VerdictSystem, Name: "msedgewebview2"}},
		{"окна PC Block — система", `c:\program files\pcblock\pcblock.exe`, "",
			Result{Verdict: VerdictSystem, Name: "pcblock"}},
		{"категория «system» по точному пути", `c:\program files\vendor\driverhelper.exe`, "",
			Result{Verdict: VerdictSystem, Category: "system", Known: true, Name: "Driver helper"}},
		{"решение родителя вне доверенных, отпечаток совпал", `c:\users\миша\downloads\supergame.exe`, "1024:1700000000",
			Result{Verdict: VerdictAllow, Category: "games", Known: true, Name: "SuperGame"}},
		{"подмена файла: отпечаток другой", `c:\users\миша\downloads\supergame.exe`, "2048:1700000500",
			Result{Verdict: VerdictUnknown, Name: "SuperGame", WasCategory: "games"}},
		// Д-1: ребёнок закрыл SYSTEM доступ к свойствам своего файла и подменил exe; нечитаемый
		// отпечаток у решения с отпечатком — снова «ждёт решения», прежняя категория сохраняется.
		{"отпечаток не прочитан: снова ждёт решения", `c:\users\миша\downloads\supergame.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "SuperGame", WasCategory: "games"}},
		{"решение без отпечатка", `c:\users\миша\downloads\homework.exe`, "5:6",
			Result{Verdict: VerdictAllow, Category: "study", Known: true, Name: "Homework"}},
		{"путь с .. ведёт в Загрузки", `c:\program files\brave software\..\..\users\kid\downloads\chrome.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "chrome"}},
		{"путь с .. ведёт в Windows", `c:\users\kid\..\..\windows\system32\cmd.exe`, "",
			Result{Verdict: VerdictSystem, Name: "cmd"}},
		{"верхний регистр", `C:\PROGRAM FILES\BRAVE SOFTWARE\BRAVE-BROWSER\APPLICATION\BRAVE.EXE`, "",
			Result{Verdict: VerdictAllow, Category: "other", Known: true, Name: "Brave"}},
		{"префикс \\\\?\\", `\\?\C:\Users\kid\Downloads\SuperGame.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "SuperGame"}},
		// 8.3-имя нормализатор не разворачивает (это делает адаптер Windows, GetLongPathName):
		// короткий путь не совпадает с доверенными корнями, поэтому он «незнакомый», а не
		// «разрешённый», — обойти проверку коротким именем нельзя.
		{"8.3-имя Program Files", `C:\PROGRA~1\BRAVES~1\BRAVE-~1\APPLIC~1\brave.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "brave"}},
		{"8.3-имя временной папки раннера", `C:\Users\RUNNER~1\AppData\Local\Temp\x.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "x"}},
		{"сетевой путь", `\\server\share\game.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "game"}},
		{"пустой путь: не трогать", ``, "",
			Result{Verdict: VerdictSystem}},
		{"относительный путь: не трогать", `game.exe`, "",
			Result{Verdict: VerdictSystem}},
		{"незнакомая программа в Program Files", `c:\program files\newapp\newapp.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "newapp"}},
	}
	for _, k := range cases {
		if got := c.Classify(k.path, k.fp, ""); got != k.want {
			t.Errorf("%s: Classify(%q, %q) = %+v, want %+v", k.name, k.path, k.fp, got, k.want)
		}
	}
}

// Ловит (Д-3): приложение Microsoft Store с новой версией становится «незнакомым» (корень по
// папке с версией), игра из Store не узнаётся каталогом (ни по имени exe, ни по семейству
// пакета) и никогда не закрывается, новый пакет после снимка не «ждёт решения», папка
// «windowsapps» у ребёнка считается доверенной.
func TestClassifyStoreApps(t *testing.T) {
	const wa = `c:\program files\windowsapps\`
	c := NewClassifier(testRoots, DefaultCatalog)
	c.Reload([]Entry{
		{Path: wa + `microsoft.windowscalculator_11.2405.2.0_x64__8wekyb3d8bbwe\calculatorapp.exe`, Name: "Калькулятор", Category: "other"},
	})
	cases := []struct {
		name, path string
		want       Result
	}{
		{"калькулятор из снимка", wa + `microsoft.windowscalculator_11.2405.2.0_x64__8wekyb3d8bbwe\calculatorapp.exe`,
			Result{Verdict: VerdictAllow, Category: "other", Known: true, Name: "Калькулятор"}},
		{"калькулятор новой версии: тот же корень", wa + `microsoft.windowscalculator_11.2502.0.0_x64__8wekyb3d8bbwe\calculatorapp.exe`,
			Result{Verdict: VerdictAllow, Category: "other", Name: "calculatorapp"}},
		{"Minecraft из Store: игра по имени exe", wa + `microsoft.minecraftuwp_1.21.0.0_x64__8wekyb3d8bbwe\minecraft.windows.exe`,
			Result{Verdict: VerdictAllow, Category: "games", Name: "minecraft.windows"}},
		{"Minecraft Java из Store: игра по семейству", wa + `microsoft.minecraftjavaedition_1.0.0.0_x64__8wekyb3d8bbwe\launcher\gamelauncher.exe`,
			Result{Verdict: VerdictAllow, Category: "games", Name: "gamelauncher"}},
		{"Roblox из Store: игра по семейству", wa + `robloxcorporation.roblox_2.600.0.0_x86__55nm5eh3cm0pr\windows10universal.exe`,
			Result{Verdict: VerdictAllow, Category: "games", Name: "windows10universal"}},
		{"неизвестный пакет после снимка", wa + `vendor.newapp_1.0.0.0_x64__abcdefghijklm\newapp.exe`,
			Result{Verdict: VerdictUnknown, Name: "newapp"}},
		{"компонент Windows в WindowsApps", wa + `microsoftwindows.client.webexperience_524.33200.0.0_x64__cw5n1h2txyewy\widgets.exe`,
			Result{Verdict: VerdictSystem, Name: "widgets"}},
		{"windowsapps в папке ребёнка", `c:\users\kid\windowsapps\x.exe`,
			Result{Verdict: VerdictUnknown, Name: "x"}},
		{"игра из каталога в windowsapps ребёнка", `c:\users\kid\windowsapps\microsoft.minecraftuwp_1_x64__8wekyb3d8bbwe\minecraft.windows.exe`,
			Result{Verdict: VerdictUnknown, Name: "minecraft.windows"}},
	}
	for _, k := range cases {
		if got := c.Classify(k.path, "", ""); got != k.want {
			t.Errorf("%s: Classify(%q) = %+v, want %+v", k.name, k.path, got, k.want)
		}
	}
	if cat, ok := c.CatalogCategory(wa + `microsoft.minecraftjavaedition_1.0.0.0_x64__8wekyb3d8bbwe\x.exe`); cat != "games" || !ok {
		t.Errorf("CatalogCategory(Minecraft Java из Store) = %q, %v; want games, true", cat, ok)
	}
	if _, ok := c.CatalogCategory(`c:\users\kid\downloads\steam.exe`); ok {
		t.Error("CatalogCategory вне доверенных каталогов: ok = true")
	}
}

// Ловит: имя для плашки и журнала берётся без обрезки (журнал ≤ 120 символов) или теряет
// расширение не только .exe.
func TestNameFromPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{`C:\Users\Миша\Downloads\SuperGame.exe`, "SuperGame"},
		{`c:/x/Game.EXE`, "Game"},
		{`c:\x\tool.com`, "tool.com"},
		{`c:\x\.exe`, ".exe"},
		{``, ""},
		{`c:\x\` + strings.Repeat("я", 200) + `.exe`, strings.Repeat("я", MaxNameLen)},
	}
	for _, c := range cases {
		if got := NameFromPath(c.in); got != c.want {
			t.Errorf("NameFromPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := ClipName("  " + strings.Repeat("a", 130) + " "); got != strings.Repeat("a", MaxNameLen) {
		t.Errorf("ClipName: %d символов, want %d", len(got), MaxNameLen)
	}
	if got := ClipName("Игра\nвторая строка"); got != "Игра вторая строка" {
		t.Errorf("ClipName с переводом строки: %q", got)
	}
}

// Ловит: Reload не меняет решения (смена категории родителем не доходит до следующего Classify),
// старый корень приложения остаётся после удаления программы.
func TestClassifierReload(t *testing.T) {
	c := newTestClassifier()
	game := `c:\program files (x86)\steam\steamapps\common\game\game.exe`
	if got := c.Classify(game, "", ""); got.Category != "games" {
		t.Fatalf("до Reload: %+v", got)
	}
	c.Reload([]Entry{{Path: `c:\program files (x86)\steam\steam.exe`, Name: "Steam", Category: "blocked"}})
	if got := c.Classify(game, "", ""); got.Verdict != VerdictForbidden {
		t.Errorf("после запрета Steam: %+v, want Forbidden", got)
	}
	c.Reload(nil)
	// steam.exe ушёл из списка; game.exe не в каталоге и не в корне известной программы.
	if got := c.Classify(game, "", ""); got.Verdict != VerdictUnknown {
		t.Errorf("после пустого Reload: %+v, want Unknown", got)
	}
	if c.NeedsFingerprint(`c:\users\миша\downloads\supergame.exe`) {
		t.Error("пустой список: NeedsFingerprint = true, want false")
	}
	c.Reload(testEntries)
	if !c.NeedsFingerprint(`C:\Users\Миша\Downloads\SuperGame.exe`) {
		t.Error("NeedsFingerprint: решение с отпечатком, want true")
	}
	if c.NeedsFingerprint(`c:\users\миша\downloads\homework.exe`) || c.NeedsFingerprint(`c:\users\kid\x.exe`) {
		t.Error("NeedsFingerprint: без отпечатка или нет в списке, want false")
	}
}

// Ловит гонку: Classify во время Reload читает полуобновлённые карты (go test -race).
func TestClassifierConcurrent(t *testing.T) {
	c := newTestClassifier()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				c.Classify(`c:\program files (x86)\steam\steamapps\common\game\game.exe`, "", "")
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				c.Reload(testEntries)
			}
		}()
	}
	wg.Wait()
}

// Ловит: строковое имя вердикта для журнала работы пустое.
func TestVerdictString(t *testing.T) {
	for v, want := range map[Verdict]string{VerdictSystem: "система", VerdictAllow: "разрешена", VerdictForbidden: "запрещена", VerdictUnknown: "незнакомая", VerdictHidden: "скрыта"} {
		if got := v.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", v, got, want)
		}
	}
}
