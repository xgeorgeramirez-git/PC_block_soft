package guard

import (
	"strings"
	"unicode"
)

// Домен из поля адреса Brave (RM4-1, R23). Функция самодостаточная (только strings и unicode):
// в T1 её заменит sites.FromOmnibox, перенос — копированием.

// maxDomainLen — наибольшая длина домена (RFC 1035: 253 знака).
const maxDomainLen = 253

// omniboxNonWeb — схемы, у которых нет сайта: служебные страницы браузера, файлы, письма.
// Адрес с такой схемой — домен «» (RM4-1: «Прочее»).
var omniboxNonWeb = map[string]bool{
	"about": true, "blob": true, "brave": true, "chrome": true, "chrome-extension": true,
	"chrome-untrusted": true, "data": true, "devtools": true, "edge": true, "file": true,
	"javascript": true, "mailto": true, "tel": true,
}

// omniboxDomain берёт из текста поля адреса только хост в нижнем регистре: без схемы, пути,
// запроса, порта, логина «user:pw@», начального «www.» и точки в конце. Префикс «view-source:»
// снимается, решает остаток. Служебные страницы (brave://, chrome://, about:, file:), схемы, кроме
// http и https, поисковая фраза (пробел внутри или нет точки: «котики видео», «localhost»), IPv6
// и всё, что не похоже на имя хоста, — «» (PL-9: не знаю — ничего не утверждать). IPv4 — как есть.
func omniboxDomain(text string) string {
	s := strings.TrimSpace(text)
	for {
		low := strings.ToLower(s)
		if !strings.HasPrefix(low, "view-source:") {
			break
		}
		s = s[len("view-source:"):]
	}
	if s == "" || strings.IndexFunc(s, unicode.IsSpace) >= 0 {
		return ""
	}
	if i := strings.Index(s, "://"); i >= 0 {
		scheme := strings.ToLower(s[:i])
		if scheme != "http" && scheme != "https" {
			return ""
		}
		s = s[i+3:]
	} else if i := strings.IndexByte(s, ':'); i > 0 && omniboxNonWeb[strings.ToLower(s[:i])] {
		return ""
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		s = s[i+1:]
	}
	if strings.HasPrefix(s, "[") {
		return "" // IPv6 в скобках: не домен
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		port := s[i+1:]
		if strings.IndexFunc(port, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return ""
		}
		s = s[:i]
	}
	s = strings.TrimSuffix(strings.ToLower(s), ".")
	if rest, ok := strings.CutPrefix(s, "www."); ok && strings.Contains(rest, ".") {
		s = rest
	}
	if len(s) > maxDomainLen || !strings.Contains(s, ".") {
		return ""
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" {
			return ""
		}
	}
	for _, r := range s {
		if r != '.' && r != '-' && r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return ""
		}
	}
	return s
}
