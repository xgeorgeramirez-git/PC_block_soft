package guard

import (
	"errors"
	"strings"
	"testing"
	"unicode"
)

// Домен из текста поля адреса Brave (RM4-1, R23): только хост, без пути, порта, логина и www.
// Ожидания — литералы: так поле адреса показывает адрес ребёнку.
func TestOmniboxDomain(t *testing.T) {
	longPath := "a.ru/" + strings.Repeat("x", 1995)
	longHost := strings.Repeat("a", 1996) + ".com"
	cases := []struct{ in, want string }{
		{"youtube.com/watch?v=1", "youtube.com"},
		{"https://WWW.Example.COM./a", "example.com"},
		{"http://probe-site.test/a?b=c", "probe-site.test"},
		{"  https://m.vk.com#x  ", "m.vk.com"},
		{"brave://newtab", ""},
		{"brave://settings/", ""},
		{"chrome://version", ""},
		{"about:blank", ""},
		{"file:///C:/Users/miша/a.html", ""},
		{"view-source:https://www.example.com/", "example.com"},
		{"view-source:brave://newtab", ""},
		{"котики видео", ""},
		{"котики", ""},
		{"localhost:3000", ""},
		{"192.168.0.1:8080/x", "192.168.0.1"},
		{"user:pw@host.ru/", "host.ru"},
		{"https://user@Host.RU:443/p", "host.ru"},
		{"", ""},
		{"   ", ""},
		{"www.ru", "www.ru"},
		{"https://[::1]:8080/", ""},
		{"https://пример.рф/путь", "пример.рф"},
		{"xn--e1afmkfd.xn--p1ai", "xn--e1afmkfd.xn--p1ai"},
		{"https://a..b/", ""},
		{"https://.a.b/", ""},
		{"https://a.b\\c/", ""},
		{"mailto:me@a.ru", ""},
		{longPath, "a.ru"},
		{longHost, ""},
	}
	for _, c := range cases {
		if got := omniboxDomain(c.in); got != c.want {
			in := c.in
			if len(in) > 60 {
				in = in[:60] + "…"
			}
			t.Errorf("omniboxDomain(%q) = %q, want %q", in, got, c.want)
		}
	}
}

func FuzzOmniboxDomain(f *testing.F) {
	for _, s := range []string{"youtube.com/watch?v=1", "https://WWW.Example.COM./a", "brave://newtab",
		"котики видео", "localhost:3000", "192.168.0.1:8080/x", "user:pw@host.ru/", "", "view-source:x.ru"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := omniboxDomain(s)
		if strings.ContainsAny(got, "/:@\\") || strings.IndexFunc(got, unicode.IsSpace) >= 0 {
			t.Fatalf("omniboxDomain(%q) = %q: в домене недопустимые знаки", s, got)
		}
		if got != "" && (!strings.Contains(got, ".") || strings.ToLower(got) != got || len(got) > maxDomainLen) {
			t.Fatalf("omniboxDomain(%q) = %q: не домен", s, got)
		}
	})
}

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
