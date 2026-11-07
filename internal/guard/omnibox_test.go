package guard

import (
	"errors"
	"testing"
)

// Путь переднего плана — Brave (RM4-1): только brave.exe, регистр не важен.
func TestIsBravePath(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`, true},
		{`c:\users\миша\appdata\local\bravesoftware\brave-browser\application\BRAVE.EXE`, true},
		{`C:\Program Files\Google\Chrome\Application\chrome.exe`, false},
		{`C:\games\notbrave.exe`, false},
		{`brave.exe`, false},
		{``, false},
	}
	for _, c := range cases {
		if got := isBravePath(c.in); got != c.want {
			t.Errorf("isBravePath(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// Поле адреса в фокусе (ребёнок печатает) — домен этого окна прежний; ошибка чтения — «»
// и память окна забыта (PL-9); у каждого окна своя память.
func TestDomainMemo(t *testing.T) {
	var m domainMemo
	steps := []struct {
		hwnd    uintptr
		text    string
		focused bool
		err     error
		want    string
	}{
		{1, "котики видео", true, nil, ""},
		{1, "https://www.youtube.com/watch?v=1", false, nil, "youtube.com"},
		{1, "котики видео", true, nil, "youtube.com"},
		{2, "vk.com/feed", false, nil, "vk.com"},
		{1, "kotiki.ru", true, nil, "youtube.com"},
		{1, "kotiki.ru/a", false, nil, "kotiki.ru"},
		{1, "", false, errors.New("UIA: элемент недоступен"), ""},
		{1, "kotiki.ru", true, nil, ""},
		{2, "", true, nil, "vk.com"},
		{2, "brave://newtab", false, nil, ""},
		{2, "vk.com", true, nil, ""},
	}
	for i, s := range steps {
		if got := m.Next(s.hwnd, s.text, s.focused, s.err); got != s.want {
			t.Errorf("шаг %d: Next(%d, %q, %v, %v) = %q, want %q", i, s.hwnd, s.text, s.focused, s.err, got, s.want)
		}
	}
}

// Ловит (RM4-1, PL-9, RK5): ошибка UIA и окно, где у Document нет адреса, неотличимы от служебной
// страницы — служба не узнала бы, что адрес не прочитан. Домен — по sites.FromOmnibox (punycode,
// повторное «www.»). Ожидания — литералы.
func TestDomainMemoRead(t *testing.T) {
	var m domainMemo
	uiaErr := errors.New("UIA: элемент не найден")
	omni := func(text string, focused bool) braveRead {
		return braveRead{Omnibox: text, HasOmnibox: true, Focused: focused}
	}
	doc := func(text string) braveRead { return braveRead{Document: text} }
	steps := []struct {
		hwnd       uintptr
		rd         braveRead
		err        error
		want       string
		wantUnread bool
	}{
		{1, omni("https://www.youtube.com/watch?v=1", false), nil, "youtube.com", false},
		{1, omni("пример.рф/путь", false), nil, "xn--e1afmkfd.xn--p1ai", false},
		{1, omni("www.www.a.ru", false), nil, "a.ru", false},
		{1, omni("kotiki.ru", true), nil, "a.ru", false},
		{1, omni("brave://newtab", false), nil, "", false},
		{1, omni("котики видео", false), nil, "", false},
		{2, braveRead{}, uiaErr, "", true},
		{3, doc(""), nil, "", true},
		{3, doc("   "), nil, "", true},
		{3, doc("https://app.example/x"), nil, "app.example", false},
		{3, omni("", false), nil, "", false},
		{4, omni("vk.com", false), nil, "vk.com", false},
		{4, braveRead{}, uiaErr, "", true},
		{4, omni("x.ru", true), nil, "", false},
	}
	for i, s := range steps {
		got, unread := m.Read(s.hwnd, s.rd, s.err)
		if got != s.want || unread != s.wantUnread {
			t.Errorf("шаг %d: Read(%d, %+v, %v) = (%q, %v), want (%q, %v)",
				i, s.hwnd, s.rd, s.err, got, unread, s.want, s.wantUnread)
		}
	}
}

// Ловит (ревью M4 F1-4): домен берётся из текста поля адреса, который ребёнок набрал и не открыл
// (поле не в фокусе, а набранное осталось), а не из адреса страницы; поле в фокусе без адреса
// страницы даёт набранный домен; после смены вкладки (RuntimeId, а не заголовок) поле в фокусе
// даёт домен прошлой вкладки.
func TestDomainMemoPrefersDocument(t *testing.T) {
	var m domainMemo
	steps := []struct {
		rd   braveRead
		want string
	}{
		{braveRead{Document: "https://a.ru/x", Omnibox: "evil.ru", HasOmnibox: true, Tab: "42.1"}, "a.ru"},
		{braveRead{Document: "HTTPS://b.ru/", Omnibox: "evil.ru", HasOmnibox: true, Focused: true, Tab: "42.1"}, "b.ru"},
		{braveRead{Omnibox: "evil.ru", HasOmnibox: true, Focused: true, Tab: "42.1"}, "b.ru"},
		{braveRead{Document: "brave://newtab/", Omnibox: "evil.ru", HasOmnibox: true, Focused: true, Tab: "42.1"}, "b.ru"},
		{braveRead{Omnibox: "evil.ru", HasOmnibox: true, Focused: true, Tab: "42.7"}, ""},
		{braveRead{Document: "http://c.ru/", Omnibox: "c.ru", HasOmnibox: true, Tab: "42.7"}, "c.ru"},
		{braveRead{Omnibox: "evil.ru", HasOmnibox: true, Focused: true, Tab: "42.7"}, "c.ru"},
		{braveRead{Omnibox: "evil.ru", HasOmnibox: true, Focused: true}, "c.ru"}, // вкладка не прочитана — не смена
	}
	for i, s := range steps {
		if got, unread := m.Read(1, s.rd, nil); got != s.want || unread {
			t.Errorf("шаг %d: Read(%+v) = (%q, %v), want (%q, false)", i, s.rd, got, unread, s.want)
		}
	}
}

// Поле адреса Chromium по ClassName (проба T0: у Brave «BraveOmniboxViewViews», у Chrome
// «OmniboxViewViews»): класс оканчивается на OmniboxViewViews.
func TestIsOmniboxClass(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"BraveOmniboxViewViews", true},
		{"OmniboxViewViews", true},
		{"OmniboxViewViewsX", false},
		{"OmniboxView", false},
		{"omniboxviewviews", false},
		{"LocationBarView", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isOmniboxClass(c.in); got != c.want {
			t.Errorf("isOmniboxClass(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// Ловит (ревью M4 F1-6): запасные источники (первый Edit с ValuePattern, Document окна без поля
// адреса) дают «домен» из поля диалога Brave («Сохранить как»: report.pdf) или служебной страницы.
func TestFallbackDomain(t *testing.T) {
	cases := []struct{ in, want string }{
		{"report.pdf", ""},
		{"https://a.ru/x", "a.ru"},
		{"HTTP://WWW.B.RU", "b.ru"},
		{"  https://c.ru  ", "c.ru"},
		{"a.ru", ""},
		{"file:///c:/x.html", ""},
		{"brave://settings", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := fallbackDomain(c.in); got != c.want {
			t.Errorf("fallbackDomain(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	var m domainMemo
	steps := []struct {
		rd         braveRead
		want       string
		wantUnread bool
	}{
		{braveRead{Omnibox: "report.pdf", HasOmnibox: true, OmniboxByEdit: true}, "", false},
		{braveRead{Omnibox: "https://a.ru/x", HasOmnibox: true, OmniboxByEdit: true}, "a.ru", false},
		{braveRead{Omnibox: "b.ru", HasOmnibox: true}, "b.ru", false},
		{braveRead{Document: "report.pdf"}, "", false},
		{braveRead{Document: "https://app.example/"}, "app.example", false},
	}
	for i, s := range steps {
		if got, unread := m.Read(5, s.rd, nil); got != s.want || unread != s.wantUnread {
			t.Errorf("шаг %d: Read(%+v) = (%q, %v), want (%q, %v)", i, s.rd, got, unread, s.want, s.wantUnread)
		}
	}
}
