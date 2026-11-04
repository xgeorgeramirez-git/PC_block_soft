package guard

import (
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
