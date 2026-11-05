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

// isBravePath — путь переднего плана это brave.exe (оканчивается на «\brave.exe», регистр не важен).
func isBravePath(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), `\brave.exe`)
}

// domainMemo — последний домен каждого окна Brave (только домен, R23). Пока поле адреса в фокусе
// клавиатуры (ребёнок печатает адрес или поисковую фразу), домен окна не меняется (RM4-1). Ошибка
// чтения — «» и память окна забыта (PL-9: не знаю — ничего не утверждать). Окна, которых больше
// нет, вытесняются: память держит не больше maxMemoWindows окон.
type domainMemo struct {
	last map[uintptr]string
}

const maxMemoWindows = 64

// Next — домен окна hwnd по прочитанному полю адреса: text — текст поля (только в этой функции,
// дальше не идёт), focused — поле в фокусе клавиатуры, err — ошибка чтения.
func (m *domainMemo) Next(hwnd uintptr, text string, focused bool, err error) string {
	if m.last == nil {
		m.last = map[uintptr]string{}
	}
	if err != nil {
		delete(m.last, hwnd)
		return ""
	}
	if focused {
		return m.last[hwnd]
	}
	d := omniboxDomain(text)
	if _, ok := m.last[hwnd]; !ok && len(m.last) >= maxMemoWindows {
		m.last = map[uintptr]string{}
	}
	m.last[hwnd] = d
	return d
}

// omniboxClassSuffix — окончание ClassName поля адреса Chromium: у Chrome класс «OmniboxViewViews»,
// у Brave «BraveOmniboxViewViews» (проба T0, Brave 1.98).
const omniboxClassSuffix = "OmniboxViewViews"

// isOmniboxClass — ClassName элемента UIA — поле адреса Chromium (оканчивается на
// OmniboxViewViews, регистр важен: так пишет Chromium).
func isOmniboxClass(class string) bool { return strings.HasSuffix(class, omniboxClassSuffix) }
