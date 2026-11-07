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
	tab  map[uintptr]string // RuntimeId выбранной вкладки, к которой относится last
}

const maxMemoWindows = 64

// Next — домен окна hwnd по прочитанному полю адреса: text — текст поля (только в этой функции,
// дальше не идёт), focused — поле в фокусе клавиатуры, err — ошибка чтения.
func (m *domainMemo) Next(hwnd uintptr, text string, focused bool, err error) string {
	if m.last == nil {
		m.last, m.tab = map[uintptr]string{}, map[uintptr]string{}
	}
	if err != nil {
		delete(m.last, hwnd)
		delete(m.tab, hwnd)
		return ""
	}
	if focused {
		return m.last[hwnd]
	}
	d := sites.FromOmnibox(text)
	if _, ok := m.last[hwnd]; !ok && len(m.last) >= maxMemoWindows {
		m.last, m.tab = map[uintptr]string{}, map[uintptr]string{}
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

// braveRead — одно чтение окна Brave (braveReader.Read). Тексты живут только до domainMemo.Read и
// дальше не идут (R23).
type braveRead struct {
	Omnibox    string // текст поля адреса
	HasOmnibox bool   // поле адреса найдено
	Focused    bool   // поле адреса в фокусе клавиатуры (ребёнок печатает)
	Document   string // адрес документа активной вкладки (ValuePattern Document); «» — нет
	Tab        string // RuntimeId выбранной вкладки; «» — не прочитан
}

// Read — домен окна hwnd по одному чтению braveReader.Read и признак «адрес не прочитан» (RM4-1,
// RK5, ревью M4 F1-4). Ошибка UIA — («», true) и память окна забыта. Основной источник — адрес
// документа активной вкладки (http:// или https://): текст поля адреса ребёнок может набрать и не
// открыть. Нет такого адреса — поле адреса (в фокусе — прошлый домен этой вкладки, Next); смена
// выбранной вкладки (RuntimeId) забывает прошлый домен окна; у окна нет поля адреса, а адрес документа пуст — («», true). Служебная страница,
// поисковая фраза, file: дают («», false) — адрес прочитан, сайта нет. Тексты дальше этой функции
// не идут (R23).
func (m *domainMemo) Read(hwnd uintptr, rd braveRead, err error) (domain string, unread bool) {
	if err != nil {
		m.Next(hwnd, "", false, err)
		return "", true
	}
	m.switchTab(hwnd, rd.Tab)
	if isWebAddress(rd.Document) {
		return m.Next(hwnd, rd.Document, false, nil), false // адрес страницы, а не набранный текст
	}
	if rd.HasOmnibox {
		return m.Next(hwnd, rd.Omnibox, rd.Focused, nil), false
	}
	if strings.TrimSpace(rd.Document) == "" {
		return "", true
	}
	return m.Next(hwnd, rd.Document, false, nil), false
}

// switchTab забывает домен окна hwnd, если выбрана другая вкладка (RuntimeId tab; «» — не прочитан,
// смены нет): домен прошлой вкладки не должен достаться новой, пока в ней печатают адрес.
func (m *domainMemo) switchTab(hwnd uintptr, tab string) {
	if tab == "" {
		return
	}
	if m.last == nil {
		m.last, m.tab = map[uintptr]string{}, map[uintptr]string{}
	}
	if old, ok := m.tab[hwnd]; !ok || old != tab {
		if !ok && len(m.tab) >= maxMemoWindows {
			m.last, m.tab = map[uintptr]string{}, map[uintptr]string{}
		}
		delete(m.last, hwnd)
		m.tab[hwnd] = tab
	}
}

// isWebAddress — адрес начинается с http:// или https:// (регистр не важен): так выглядит адрес
// страницы, а не поисковая фраза, имя файла или служебная страница.
func isWebAddress(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://")
}
