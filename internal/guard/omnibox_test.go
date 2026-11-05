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
	steps := []struct {
		hwnd       uintptr
		text       string
		focused    bool
		source     string
		err        error
		want       string
		wantUnread bool
	}{
		{1, "https://www.youtube.com/watch?v=1", false, "omnibox", nil, "youtube.com", false},
		{1, "пример.рф/путь", false, "omnibox", nil, "xn--e1afmkfd.xn--p1ai", false},
		{1, "www.www.a.ru", false, "omnibox", nil, "a.ru", false},
		{1, "kotiki.ru", true, "omnibox", nil, "a.ru", false},
		{1, "brave://newtab", false, "omnibox", nil, "", false},
		{1, "котики видео", false, "omnibox", nil, "", false},
		{2, "", false, "", uiaErr, "", true},
		{3, "", false, "document", nil, "", true},
		{3, "   ", false, "document", nil, "", true},
		{3, "https://app.example/x", false, "document", nil, "app.example", false},
		{3, "", false, "omnibox", nil, "", false},
		{4, "vk.com", false, "omnibox", nil, "vk.com", false},
		{4, "", false, "", uiaErr, "", true},
		{4, "x.ru", true, "omnibox", nil, "", false},
	}
	for i, s := range steps {
		got, unread := m.Read(s.hwnd, s.text, s.focused, s.source, s.err)
		if got != s.want || unread != s.wantUnread {
			t.Errorf("шаг %d: Read(%d, %q, %v, %q, %v) = (%q, %v), want (%q, %v)",
				i, s.hwnd, s.text, s.focused, s.source, s.err, got, unread, s.want, s.wantUnread)
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
