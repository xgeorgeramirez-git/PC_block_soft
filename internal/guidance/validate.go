package guidance

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Пределы текстов (ТЗ 8.4 п.1: заголовок и 1-2 строки). Меряются в знаках, не в байтах.
const (
	MaxTitle     = 80  // заголовок карточки, тура, «Что нового»
	MaxBody      = 140 // текст карточки
	MaxLabel     = 24  // подпись кнопки
	MaxStepTitle = 60  // заголовок шага тура
	MaxStepBody  = 160 // текст шага тура
	MaxItem      = 100 // пункт «Что нового» и шаг «Как исправить»
	MaxSteps     = 6   // шагов в туре (ТЗ 8.4 п.5)
	MaxActions   = 2   // кнопок-действий в карточке (ТЗ 8.4 п.1)
	MaxItems     = 8   // пунктов в «Что нового»
	MaxHowTo     = 6   // шагов в «Как исправить»

	// Пределы с подставленным именем: длинное имя ребёнка не должно ломать карточку.
	MaxTitleExpanded = 100
	MaxBodyExpanded  = 180
)

// LongName — самое длинное имя, которое проверка подставляет в шаблоны (в окне имена усекаются
// до 40 знаков).
var LongName = strings.Repeat("Ы", 40)

// Problem — найденная в каталоге проблема: номер записи и что не так, простыми словами.
type Problem struct {
	ID  string
	Msg string
}

func (p Problem) String() string { return p.ID + ": " + p.Msg }

var (
	idPattern = map[Kind]*regexp.Regexp{
		KindHint:     regexp.MustCompile(`^H-M[0-9]-[0-9]{2}$`),
		KindDetector: regexp.MustCompile(`^D-M[0-9]-[0-9]{2}$`),
		KindTour:     regexp.MustCompile(`^T-M[0-9]-[0-9]{2}$`),
		KindWhatsNew: regexp.MustCompile(`^N-([0-9]+\.[0-9]+\.[0-9]+)$`),
	}
	placeholder = regexp.MustCompile(`\{[^{}]*\}`)
	techText    = regexp.MustCompile(`(?i)\b(ipc|rpc|null|nil|undefined|panic|exception|stacktrace|errno|hresult|sqlite|json|http|api)\b|0x[0-9a-f]{4,}|\.(go|ts|tsx):[0-9]+|баз[аыуе] данных|служб`)
)

// Допустимые подстановки в шаблонах: имя ребёнка в именительном и родительном падеже (L-36).
var placeholders = map[string]bool{"{name}": true, "{nameGen}": true}

// Expand подставляет имя в шаблон так же, как окно.
func Expand(template, name string) string {
	return strings.NewReplacer("{name}", name, "{nameGen}", name).Replace(template)
}

func knownAnchor(id string) bool {
	for _, a := range Anchors {
		if a.ID == id {
			return true
		}
	}
	return false
}

func knownScreen(id string) bool {
	for _, s := range Screens {
		if s == id {
			return true
		}
	}
	return false
}

func knownAction(id string) bool {
	switch id {
	case ActionAck, ActionStartTour, ActionOpenLimits, ActionHowToFix, ActionOpenSchedule:
		return true
	}
	return false
}

// isDash — длинные и средние тире, минус и им подобные знаки (NFR-11, урок L-117).
func isDash(r rune) bool {
	switch r {
	case '‒', '–', '—', '―', '−':
		return true
	}
	return false
}

// isEmoji — значки эмодзи: иконки в окне только из единого векторного набора (урок L-45).
func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF:
		return true
	case r >= 0x2600 && r <= 0x27BF:
		return true
	case r >= 0x2B00 && r <= 0x2BFF:
		return true
	case r == 0xFE0F || r == 0x200D:
		return true
	}
	return false
}

type checker struct {
	problems []Problem
}

func (c *checker) add(id, format string, args ...any) {
	c.problems = append(c.problems, Problem{ID: id, Msg: fmt.Sprintf(format, args...)})
}

// text проверяет одну строку: не пустая, влезает, без тире, без эмодзи, без лишних пробелов,
// без технических слов, подстановки только из списка. what — как назвать строку в сообщении.
func (c *checker) text(id, what, s string, limit int) {
	if strings.TrimSpace(s) == "" {
		c.add(id, "%s пуст", what)
		return
	}
	if n := utf8.RuneCountInString(s); n > limit {
		c.add(id, "%s длиннее %d знаков (%d)", what, limit, n)
	}
	if s != strings.TrimSpace(s) || strings.Contains(s, "  ") || strings.ContainsAny(s, "\n\r\t") {
		c.add(id, "%s: лишние пробелы или переносы", what)
	}
	for _, r := range s {
		if isDash(r) {
			c.add(id, "%s: длинное тире %q, замените на дефис, запятую или двоеточие", what, string(r))
			break
		}
	}
	for _, r := range s {
		if isEmoji(r) {
			c.add(id, "%s: эмодзи %q, нужны значки из единого набора", what, string(r))
			break
		}
	}
	if techText.MatchString(s) {
		c.add(id, "%s: технический текст, пишите словами для человека", what)
	}
	c.placeholders(id, what, s)
}

func (c *checker) placeholders(id, what, s string) {
	for _, m := range placeholder.FindAllString(s, -1) {
		if !placeholders[m] {
			c.add(id, "%s: неизвестная подстановка %s", what, m)
		}
	}
	if rest := placeholder.ReplaceAllString(s, ""); strings.ContainsAny(rest, "{}") {
		c.add(id, "%s: оборванная подстановка", what)
	}
}

// expanded проверяет шаблон с подставленным длинным именем.
func (c *checker) expanded(id, what, s string, limit int) {
	if !strings.Contains(s, "{") {
		return
	}
	if n := utf8.RuneCountInString(Expand(s, LongName)); n > limit {
		c.add(id, "%s длиннее %d знаков, если подставить длинное имя (%d)", what, limit, n)
	}
}

// Validate проверяет каталог целиком (ТЗ 10.9, п.9): уникальные номера, длина текстов, нет длинных
// тире, к каждому якорю есть элемент в списке Anchors. Пустой список — тоже проблема.
func Validate(entries []Entry) []Problem {
	c := &checker{}
	if len(entries) == 0 {
		c.add("-", "каталог пуст")
		return c.problems
	}
	ids := map[string]Kind{}
	for _, e := range entries {
		if _, dup := ids[e.ID]; dup {
			c.add(e.ID, "номер повторяется")
		}
		ids[e.ID] = e.Kind
	}
	for _, e := range entries {
		c.entry(e, ids)
	}
	return c.problems
}

func (c *checker) entry(e Entry, ids map[string]Kind) {
	re, ok := idPattern[e.Kind]
	if !ok {
		c.add(e.ID, "неизвестный вид записи %q", string(e.Kind))
		return
	}
	if !re.MatchString(e.ID) {
		c.add(e.ID, "номер не подходит к виду %q (ждали вид H-M1-01, D-M1-01, T-M1-01 или N-0.1.0)", string(e.Kind))
	}
	switch e.Kind {
	case KindHint, KindDetector:
		c.card(e)
	case KindTour:
		c.tour(e, ids)
	case KindWhatsNew:
		c.whatsNew(e)
	}
}

func (c *checker) card(e Entry) {
	c.text(e.ID, "заголовок", e.Title, MaxTitle)
	c.text(e.ID, "текст", e.Body, MaxBody)
	c.expanded(e.ID, "заголовок", e.Title, MaxTitleExpanded)
	c.expanded(e.ID, "текст", e.Body, MaxBodyExpanded)
	if e.Anchor == "" {
		c.add(e.ID, "не указан якорь")
	} else if !knownAnchor(e.Anchor) {
		c.add(e.ID, "неизвестный якорь %q: его нет в списке якорей экранов", e.Anchor)
	}
	if len(e.Actions) > MaxActions {
		c.add(e.ID, "кнопок-действий %d, можно не больше %d", len(e.Actions), MaxActions)
	}
	howTo := false
	for _, a := range e.Actions {
		if !knownAction(a.ID) {
			c.add(e.ID, "неизвестное действие %q", a.ID)
		}
		c.text(e.ID, "подпись кнопки", a.Label, MaxLabel)
		if a.ID == ActionHowToFix {
			howTo = true
		}
	}
	if e.Kind == KindDetector {
		if len(e.Actions) == 0 {
			c.add(e.ID, "у напоминания о риске нет кнопки-действия: оно должно вести к исправлению")
		}
		if !e.Repeat {
			c.add(e.ID, "напоминание должно быть повторяющимся: у него есть «Больше не напоминать»")
		}
	}
	if e.Once && e.Repeat {
		c.add(e.ID, "приглашение, которое показывают один раз, не бывает повторяющимся")
	}
	if howTo {
		c.text(e.ID, "заголовок окна «Как исправить»", e.HowToTitle, MaxTitle)
		if len(e.HowTo) == 0 || len(e.HowTo) > MaxHowTo {
			c.add(e.ID, "у действия «Как исправить» шагов %d, нужно от 1 до %d", len(e.HowTo), MaxHowTo)
		}
		for i, line := range e.HowTo {
			c.text(e.ID, fmt.Sprintf("шаг %d окна «Как исправить»", i+1), line, MaxItem)
		}
	}
}

func (c *checker) tour(e Entry, ids map[string]Kind) {
	c.text(e.ID, "заголовок", e.Title, MaxTitle)
	if len(e.Steps) == 0 || len(e.Steps) > MaxSteps {
		c.add(e.ID, "в туре шагов %d, нужно от 1 до %d", len(e.Steps), MaxSteps)
	}
	for i, s := range e.Steps {
		what := fmt.Sprintf("шаг %d", i+1)
		c.text(e.ID, what+", заголовок", s.Title, MaxStepTitle)
		c.text(e.ID, what+", текст", s.Body, MaxStepBody)
		if !knownAnchor(s.Anchor) {
			c.add(e.ID, "%s: неизвестный якорь %q", what, s.Anchor)
		}
		if !knownScreen(s.Screen) {
			c.add(e.ID, "%s: неизвестный экран %q", what, s.Screen)
		}
	}
	for _, cover := range e.Covers {
		if kind, ok := ids[cover]; !ok || kind != KindHint {
			c.add(e.ID, "тур ссылается на несуществующую подсказку %s", cover)
		}
	}
}

func (c *checker) whatsNew(e Entry) {
	c.text(e.ID, "заголовок", e.Title, MaxTitle)
	if want := strings.TrimPrefix(e.ID, "N-"); e.Version != want {
		c.add(e.ID, "версия %q не совпадает с номером записи (ждали %q)", e.Version, want)
	}
	if len(e.Items) == 0 || len(e.Items) > MaxItems {
		c.add(e.ID, "пунктов %d, нужно от 1 до %d", len(e.Items), MaxItems)
	}
	for i, item := range e.Items {
		c.text(e.ID, fmt.Sprintf("пункт %d", i+1), item, MaxItem)
	}
}

// ValidateLabels проверяет общие подписи окна Помощника по тем же правилам. Ключ на «Note»
// означает пояснение: оно длиннее подписи кнопки.
func ValidateLabels(labels map[string]string) []Problem {
	c := &checker{}
	for key, s := range labels {
		limit := MaxLabel
		if strings.HasSuffix(key, "Note") {
			limit = MaxBody
		}
		c.text("подпись "+key, "текст", s, limit)
	}
	return c.problems
}
