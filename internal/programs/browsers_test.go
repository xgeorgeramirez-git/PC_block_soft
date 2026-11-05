package programs

import "testing"

// braveCompany — CompanyName настоящего brave.exe.
const braveCompany = "Brave Software, Inc."

// Ловит (RM4-8, R15, D-68, PL-18, PL-20): браузер, кроме Brave, разрешён по каталогу или «ждёт
// решения» в профиле и Загрузках; browser.exe из чужой папки запрещён как Яндекс Браузер; WebView2
// или msedge_proxy стали браузером; Brave в профиле разрешён без издателя или с чужим издателем;
// служебные части Brave (обновлятор, обработчик сбоев, уведомления) — «незнакомые»; brave.exe вне
// своей папки профиля разрешён.
func TestBrowsersClassify(t *testing.T) {
	c := NewClassifier(testRoots, DefaultCatalog)
	c.Reload(nil)
	cases := []struct {
		path, pub string
		want      Result
	}{
		{`c:\program files\google\chrome\application\chrome.exe`, "",
			Result{Verdict: VerdictForbidden, Category: "blocked", Name: "chrome"}},
		{`c:\users\миша\downloads\chrome.exe`, "",
			Result{Verdict: VerdictForbidden, Category: "blocked", Name: "chrome"}},
		{`c:\users\миша\appdata\local\google\chrome\application\chrome.exe`, "",
			Result{Verdict: VerdictForbidden, Category: "blocked", Name: "chrome"}},
		{`c:\program files (x86)\microsoft\edge\application\msedge.exe`, "",
			Result{Verdict: VerdictForbidden, Category: "blocked", Name: "msedge"}},
		{`c:\users\миша\appdata\local\yandex\yandexbrowser\application\browser.exe`, "",
			Result{Verdict: VerdictForbidden, Category: "blocked", Name: "browser"}},
		{`c:\users\миша\downloads\browser.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "browser"}},
		{`c:\program files (x86)\microsoft\edgewebview\application\130.0.2849.80\msedgewebview2.exe`, "",
			Result{Verdict: VerdictSystem, Name: "msedgewebview2"}},
		{`c:\users\миша\appdata\local\teams\msedgewebview2.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "msedgewebview2"}},
		{`c:\program files\windowsapps\microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe\msedge_proxy.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "msedge_proxy"}},
		{`c:\program files\bravesoftware\brave-browser\application\brave.exe`, "",
			Result{Verdict: VerdictAllow, Category: "other", Name: "brave"}},
		{`c:\program files (x86)\bravesoftware\update\braveupdate.exe`, "",
			Result{Verdict: VerdictSystem, Name: "braveupdate"}},
		{`c:\program files\bravesoftware\update\1.3.361.151\bravecrashhandler64.exe`, "",
			Result{Verdict: VerdictSystem, Name: "bravecrashhandler64"}},
		{`c:\users\миша\appdata\local\bravesoftware\brave-browser\application\brave.exe`, braveCompany,
			Result{Verdict: VerdictAllow, Category: "other", Name: "brave"}},
		{`c:\users\миша\appdata\local\bravesoftware\brave-browser\application\brave.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "brave"}},
		{`c:\users\миша\appdata\local\bravesoftware\brave-browser\application\brave.exe`, "Evil Games",
			Result{Verdict: VerdictUnknown, Name: "brave"}},
		{`c:\users\миша\appdata\local\bravesoftware\update\braveupdate.exe`, braveCompany,
			Result{Verdict: VerdictSystem, Name: "braveupdate"}},
		{`c:\users\миша\appdata\local\bravesoftware\brave-browser\application\157.1.99.1\notification_helper.exe`, braveCompany,
			Result{Verdict: VerdictSystem, Name: "notification_helper"}},
		{`c:\users\миша\appdata\local\bravesoftware\update\braveupdate.exe`, "",
			Result{Verdict: VerdictUnknown, Name: "braveupdate"}},
		{`c:\users\миша\downloads\brave.exe`, braveCompany,
			Result{Verdict: VerdictUnknown, Name: "brave"}},
		{`c:\users\миша\appdata\local\games\brave.exe`, braveCompany,
			Result{Verdict: VerdictUnknown, Name: "brave"}},
	}
	for _, k := range cases {
		if got := c.Classify(k.path, "", k.pub); got != k.want {
			t.Errorf("Classify(%q, издатель %q) = %+v, want %+v", k.path, k.pub, got, k.want)
		}
	}
}

// Ловит (RM4-8, PL-30): каталог браузеров сильнее решения родителя по точному пути; скрытие
// Chrome не действует.
func TestBrowserParentDecisionWins(t *testing.T) {
	chrome := `c:\program files\google\chrome\application\chrome.exe`
	c := NewClassifier(testRoots, DefaultCatalog)
	c.Reload([]Entry{{Path: chrome, Category: "other"}})
	if got, want := c.Classify(chrome, "", ""), (Result{Verdict: VerdictAllow, Category: "other", Known: true, Name: "chrome"}); got != want {
		t.Errorf("разрешённый родителем chrome: %+v, want %+v", got, want)
	}
	c.Reload([]Entry{{Path: chrome, Category: "other", Hidden: true}})
	if got, want := c.Classify(chrome, "", ""), (Result{Verdict: VerdictHidden, Category: "other", Known: true, Name: "chrome"}); got != want {
		t.Errorf("скрытый родителем chrome: %+v, want %+v", got, want)
	}
	// Другая папка Chrome — снова запрет (решение по точному пути).
	if got := c.Classify(`c:\users\миша\downloads\chrome.exe`, "", "").Verdict; got != VerdictForbidden {
		t.Errorf("chrome в Загрузках при решении по другому пути: %v, want запрещена", got)
	}
}

// Ловит (RM4-8, PL-20): издатель не читается для Brave в профиле (тогда он всегда «незнакомый»)
// или читается для Brave в Program Files (лишнее чтение на каждый процесс).
func TestBraveNeedsPublisher(t *testing.T) {
	c := NewClassifier(testRoots, DefaultCatalog)
	c.Reload(nil)
	cases := []struct {
		path string
		want bool
	}{
		{`c:\users\миша\appdata\local\bravesoftware\brave-browser\application\brave.exe`, true},
		{`c:\users\миша\appdata\local\bravesoftware\update\braveupdate.exe`, true},
		{`c:\program files\bravesoftware\brave-browser\application\brave.exe`, false},
		{`c:\users\миша\downloads\brave.exe`, false},
		{`c:\users\миша\appdata\local\discord\app-1.0\discord.exe`, false},
	}
	for _, k := range cases {
		if got := c.NeedsPublisher(k.path); got != k.want {
			t.Errorf("NeedsPublisher(%q) = %v, want %v", k.path, got, k.want)
		}
	}
	// Точный путь без отпечатка: решение родителя действует и без издателя.
	brave := `c:\users\миша\appdata\local\bravesoftware\brave-browser\application\brave.exe`
	c.Reload([]Entry{{Path: brave, Category: "other"}})
	if c.NeedsPublisher(brave) {
		t.Errorf("NeedsPublisher(точный путь без отпечатка) = true, want false")
	}
}

// Ловит (RM4-8): Lookup оставил браузеры «Прочим», запретил Brave, узнаёт browser.exe по голому
// имени (общее имя: чужая игра стала бы запрещённой) или WebView2 как браузер.
func TestBrowserCatalog(t *testing.T) {
	c := DefaultCatalog
	lookups := []struct {
		name string
		cat  string
		ok   bool
	}{
		{"chrome.exe", "blocked", true},
		{"MSEDGE.EXE", "blocked", true},
		{"firefox.exe", "blocked", true},
		{"brave.exe", "other", true},
		{"browser.exe", "", false},
		{`c:\users\миша\appdata\local\yandex\yandexbrowser\application\browser.exe`, "blocked", true},
		{`c:\program files\other\browser.exe`, "", false},
		{"msedgewebview2.exe", "", false},
		{"msedge_proxy.exe", "", false},
	}
	for _, k := range lookups {
		if cat, ok := c.Lookup(k.name); cat != k.cat || ok != k.ok {
			t.Errorf("Lookup(%q) = %q, %v; want %q, %v", k.name, cat, ok, k.cat, k.ok)
		}
	}
	forbidden := []struct {
		path, name string
		ok         bool
	}{
		{`c:\program files\google\chrome\application\chrome.exe`, "Google Chrome", true},
		{`C:\Users\Миша\Downloads\Firefox.EXE`, "Mozilla Firefox", true},
		{`c:\users\миша\appdata\local\yandex\yandexbrowser\application\browser.exe`, "Яндекс Браузер", true},
		{`c:\users\миша\downloads\browser.exe`, "", false},
		{`c:\program files\bravesoftware\brave-browser\application\brave.exe`, "", false},
		{`c:\program files (x86)\microsoft\edgewebview\application\1\msedgewebview2.exe`, "", false},
		{`c:\program files\windowsapps\microsoft.microsoftedge.stable_1_neutral__8wekyb3d8bbwe\msedge_proxy.exe`, "", false},
		{"", "", false},
	}
	for _, k := range forbidden {
		if name, ok := c.ForbiddenBrowser(k.path); name != k.name || ok != k.ok {
			t.Errorf("ForbiddenBrowser(%q) = %q, %v; want %q, %v", k.path, name, ok, k.name, k.ok)
		}
	}
}
