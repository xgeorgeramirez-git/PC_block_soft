package guard

import (
	"strings"

	"pcblock/internal/sites"
)

// Источники текста поля адреса (braveReader.Read): поле адреса окна или адрес документа окна без
// поля адреса (сайт-приложение, всплывающее окно; RM4-1).
const (
	braveSourceOmnibox  = "omnibox"
	braveSourceDocument = "document"
)

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
	d := sites.FromOmnibox(text)
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

// Read — домен окна hwnd по одному чтению braveReader.Read и признак «адрес не прочитан» (RM4-1,
// RK5). Ошибка UIA — («», true) и память окна забыта; адрес документа пуст (у окна нет ни поля
// адреса, ни адреса документа) — («», true). Иначе — Next: служебная страница, поисковая фраза,
// file: дают («», false) — адрес прочитан, сайта нет. text дальше этой функции не идёт (R23).
func (m *domainMemo) Read(hwnd uintptr, text string, focused bool, source string, err error) (domain string, unread bool) {
	if err != nil {
		m.Next(hwnd, "", false, err)
		return "", true
	}
	if source == braveSourceDocument && strings.TrimSpace(text) == "" {
		return "", true
	}
	return m.Next(hwnd, text, focused, nil), false
}
