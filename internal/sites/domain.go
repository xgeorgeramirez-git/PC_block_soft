package sites

import (
	"errors"
	"strings"
	"unicode"
)

// ErrNotDomain — текст не похож на адрес сайта (FR-21). Текст человеку даёт служба.
var ErrNotDomain = errors.New("sites: это не похоже на адрес сайта")

// MaxDomainLen — наибольшая длина домена после кодирования (RFC 1035: 253 знака).
const MaxDomainLen = 253

// maxLabelLen — наибольшая длина метки после кодирования (RFC 1035: 63 знака).
const maxLabelLen = 63

// nonWebSchemes — схемы, у которых нет сайта: служебные страницы браузера, файлы, письма.
// Адрес с такой схемой — домен «» (RM4-1: «Прочее»).
var nonWebSchemes = map[string]bool{
	"about": true, "blob": true, "brave": true, "chrome": true, "chrome-extension": true,
	"chrome-untrusted": true, "data": true, "devtools": true, "edge": true, "file": true,
	"javascript": true, "mailto": true, "tel": true,
}

// FromOmnibox — домен из текста поля адреса Brave (RM4-1, RM4-4) или «». Только хост в нижнем
// регистре (полноширинные буквы и точки — как обычные, foldWidth): без схемы, пути, запроса, «#», порта, логина «user:pw@», начального «www.» (если после
// него остаётся точка) и точки в конце; IDN — в punycode по меткам. Префикс «view-source:»
// снимается, решает остаток. Служебные страницы (brave://, chrome://, about:, file:), схемы, кроме
// http и https, поисковая фраза (пробел внутри или нет точки: «котики видео», «localhost»), IPv6
// в скобках, метка длиннее 63 или домен длиннее 253 знаков после кодирования и всё, что не похоже
// на имя хоста, — «» (PL-9: не знаю — ничего не утверждать). IPv4 — как есть.
func FromOmnibox(text string) string {
	s := strings.TrimSpace(foldWidth(text))
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
	} else if i := strings.IndexByte(s, ':'); i > 0 && nonWebSchemes[strings.ToLower(s[:i])] {
		return ""
	}
	if i := strings.IndexAny(s, "/?#\\"); i >= 0 { // «\» Chromium читает как «/» в адресах http(s)
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
	// «www.» снимается, пока после него остаётся точка: так результат идемпотентен
	// (www.www.a.ru → a.ru, а www.ru остаётся www.ru).
	for {
		rest, ok := strings.CutPrefix(s, "www.")
		if !ok || !strings.Contains(rest, ".") {
			break
		}
		s = rest
	}
	if !strings.Contains(s, ".") {
		return ""
	}
	return encodeHost(s)
}

// foldWidth — полноширинные знаки U+FF01…U+FF5E → ASCII U+0021…U+007E, точки U+3002, U+FF0E
// и U+FF61 → «.» (B-51; RFC 3490 3.1 и часть сопоставлений UTS46). Полный UTS46 на стандартной
// библиотеке недостижим (README, «Отклонения от ТЗ»): прочие совместимые знаки остаются как есть.
func foldWidth(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 0xFF01 && r <= 0xFF5E:
			return r - 0xFF01 + 0x21
		case r == 0x3002 || r == 0xFF61:
			return '.'
		}
		return r
	}, s)
}

// encodeHost — хост в нижнем регистре → ASCII по меткам или «», если это не имя хоста.
func encodeHost(s string) string {
	var b strings.Builder
	for i, label := range strings.Split(s, ".") {
		if label == "" || len([]rune(label)) > maxLabelLen {
			return "" // пустая метка; длинная — длиннее и после кодирования
		}
		for _, r := range label {
			if r != '-' && r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r) {
				return ""
			}
		}
		enc, ok := toASCII(label)
		if !ok || len(enc) > maxLabelLen {
			return ""
		}
		for j := 0; j < len(enc); j++ {
			c := enc[j]
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
				return ""
			}
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(enc)
		if b.Len() > MaxDomainLen {
			return ""
		}
	}
	return b.String()
}

// NormalizeInput — домен из ввода родителя (FR-21): домен или вставленный адрес. Тот же разбор,
// что у FromOmnibox (PL-39: один разборщик на оба входа); не домен — ErrNotDomain.
func NormalizeInput(s string) (domain string, err error) {
	d := FromOmnibox(s)
	if d == "" {
		return "", ErrNotDomain
	}
	return d, nil
}
