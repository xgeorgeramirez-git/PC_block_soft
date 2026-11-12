package sites

import (
	"errors"
	"strings"
	"testing"
	"unicode"
)

// fromOmniboxCases — домен из текста поля адреса Brave (RM4-1, RM4-4): литералы. Строки
// TestOmniboxDomain стража (T0), но IDN — в punycode.
func fromOmniboxCases() []struct{ in, want string } {
	longPath := "a.ru/" + strings.Repeat("x", 1995)
	longHost := strings.Repeat("a", 1996) + ".com"
	return []struct{ in, want string }{
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
		{`https://evil.ru\@youtube.com`, "evil.ru"}, // ревью M4 F1-8: «\» — разделитель пути, как у Chromium
		{`evil.ru\path`, "evil.ru"},
		{"", ""},
		{"   ", ""},
		{"www.ru", "www.ru"},
		{"https://[::1]:8080/", ""},
		{"https://пример.рф/путь", "xn--e1afmkfd.xn--p1ai"},
		{"xn--e1afmkfd.xn--p1ai", "xn--e1afmkfd.xn--p1ai"},
		{"https://a..b/", ""},
		{"https://.a.b/", ""},
		{"https://a.b\\c/", "a.b"}, // «\» — начало пути (ревью M4 F1-8)
		{"mailto:me@a.ru", ""},
		{longPath, "a.ru"},
		{longHost, ""},
		{"ПРИМЕР.РФ", "xn--e1afmkfd.xn--p1ai"},
		{"https://www.youtube.com/watch?v=1&t=2", "youtube.com"},
		{"m.YouTube.com.", "m.youtube.com"},
		{"example.com:8080", "example.com"},
	}
}

func TestFromOmnibox(t *testing.T) {
	for _, c := range fromOmniboxCases() {
		if got := FromOmnibox(c.in); got != c.want {
			in := c.in
			if len(in) > 60 {
				in = in[:60] + "…"
			}
			t.Errorf("FromOmnibox(%q) = %q, want %q", in, got, c.want)
		}
	}
}

func normalizeErrInputs() []string {
	return []string{
		"", "котики", "котики видео", "localhost", "brave://settings", "a..b", "*.example.com",
		"https://[::1]/", strings.Repeat("a", 64) + ".com",
		strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 60) + ".e",
	}
}

func TestNormalizeInput(t *testing.T) {
	ok := []struct{ in, want string }{
		{"example.com", "example.com"},
		{"https://WWW.Example.COM./a?b", "example.com"},
		{"пример.рф", "xn--e1afmkfd.xn--p1ai"},
		{"192.168.0.1", "192.168.0.1"},
		// 253 знака — ещё домен.
		{strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61), strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61)},
	}
	for _, c := range ok {
		got, err := NormalizeInput(c.in)
		if err != nil || got != c.want {
			t.Errorf("NormalizeInput(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, in := range normalizeErrInputs() {
		got, err := NormalizeInput(in)
		if !errors.Is(err, ErrNotDomain) || got != "" {
			t.Errorf("NormalizeInput(%q) = %q, %v; want ErrNotDomain", in, got, err)
		}
	}
	if len(normalizeErrInputs()[9]) != 254 {
		t.Fatalf("длина домена в тесте %d, нужно 254", len(normalizeErrInputs()[9]))
	}
}

// B-51: полноширинные знаки U+FF01…U+FF5E — как обычные ASCII U+0021…U+007E, точки U+3002,
// U+FF0E и U+FF61 — «.» (часть UTS46, достижимая на стандартной библиотеке; README).
func TestFullwidthDomain(t *testing.T) {
	cases := []struct{ in, want string }{
		{"\uff59\uff4f\uff55\uff54\uff55\uff42\uff45\uff0e\uff43\uff4f\uff4d", "youtube.com"}, // ｙｏｕｔｕｂｅ．ｃｏｍ
		{"\uff39\uff2f\uff35\uff34\uff35\uff22\uff25.com", "youtube.com"},                     // ＹＯＵＴＵＢＥ.com
		{"youtube\u3002com", "youtube.com"},                                                   // идеографическая точка
		{"youtube\uff61com", "youtube.com"},                                                   // полуширинная идеографическая точка
		{"\uff48\uff54\uff54\uff50\uff53\uff1a\uff0f\uff0fvk.com\uff0fid1", "vk.com"},         // ｈｔｔｐｓ：／／vk.com／id1
		{"\uff57\uff57\uff57\uff0eexample\uff0ecom", "example.com"},                           // ｗｗｗ．example．com
		{"\uff11\uff19\uff12.168.0.1", "192.168.0.1"},                                         // полноширинные цифры
		{"\u043f\u0440\u0438\u043c\u0435\u0440\u3002\u0440\u0444", "xn--e1afmkfd.xn--p1ai"},   // пример。рф
		{"\uff41\u3000\uff42.ru", ""},                                                         // идеографический пробел — поисковая фраза
	}
	for _, c := range cases {
		if got := FromOmnibox(c.in); got != c.want {
			t.Errorf("FromOmnibox(%q) = %q, want %q", c.in, got, c.want)
		}
		if c.want != "" {
			if got, err := NormalizeInput(c.in); err != nil || got != c.want {
				t.Errorf("NormalizeInput(%q) = %q, %v; want %q", c.in, got, err, c.want)
			}
		}
	}
}

// Метка, которая после кодирования длиннее 63 знаков: 60 кириллических букв.
func TestNormalizeInputEncodedLength(t *testing.T) {
	if _, err := NormalizeInput(strings.Repeat("я", 60) + ".рф"); !errors.Is(err, ErrNotDomain) {
		t.Errorf("длинная метка после кодирования: err = %v, want ErrNotDomain", err)
	}
}

func checkDomainShape(t *testing.T, in, got string) {
	t.Helper()
	if got == "" {
		return
	}
	if strings.ContainsAny(got, "/:@\\") || strings.IndexFunc(got, unicode.IsSpace) >= 0 || len(got) > MaxDomainLen {
		t.Fatalf("(%q) = %q: недопустимые знаки или длина", in, got)
	}
	for i := 0; i < len(got); i++ {
		if got[i] >= 0x80 || (got[i] >= 'A' && got[i] <= 'Z') {
			t.Fatalf("(%q) = %q: не ASCII нижнего регистра", in, got)
		}
	}
	if n, err := NormalizeInput(got); err != nil || n != got {
		t.Fatalf("NormalizeInput(%q) = %q, %v: не согласен с FromOmnibox", got, n, err)
	}
	if again := FromOmnibox(got); again != got {
		t.Fatalf("FromOmnibox(%q) = %q: не идемпотентен", got, again)
	}
}

func fuzzSeeds(f *testing.F) {
	for _, c := range fromOmniboxCases() {
		f.Add(c.in)
	}
	for _, s := range normalizeErrInputs() {
		f.Add(s)
	}
	f.Add(strings.Repeat("я.", 1000))
}

func FuzzFromOmnibox(f *testing.F) {
	fuzzSeeds(f)
	f.Fuzz(func(t *testing.T, s string) {
		checkDomainShape(t, s, FromOmnibox(s))
	})
}

func FuzzNormalizeInput(f *testing.F) {
	fuzzSeeds(f)
	f.Fuzz(func(t *testing.T, s string) {
		got, err := NormalizeInput(s)
		if err != nil {
			if !errors.Is(err, ErrNotDomain) || got != "" {
				t.Fatalf("NormalizeInput(%q) = %q, %v", s, got, err)
			}
			return
		}
		if got == "" {
			t.Fatalf("NormalizeInput(%q): пустой домен без ошибки", s)
		}
		checkDomainShape(t, s, got)
	})
}
