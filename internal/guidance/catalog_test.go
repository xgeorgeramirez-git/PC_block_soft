package guidance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// Каталог Помощника (FR-34, ТЗ 8.4 п.8 и 10.9): уникальные номера, длина текстов, нет длинных
// тире, все якоря существуют. Тесты ниже проверяют и сам каталог, и сам валидатор: плохая запись
// обязана быть найдена, иначе проверка ничего не стоит.

func problemsText(ps []Problem) string {
	var b strings.Builder
	for _, p := range ps {
		b.WriteString(p.String())
		b.WriteString("\n")
	}
	return b.String()
}

// Ловит: в каталоге M1 появилась запись, не прошедшая правила.
func TestCatalogM1IsValid(t *testing.T) {
	if ps := Validate(Catalog()); len(ps) > 0 {
		t.Fatalf("каталог не прошёл проверку:\n%s", problemsText(ps))
	}
}

// Ловит: состав M1 разошёлся с разделом «Покрытие Помощника» плана вехи.
func TestCatalogM1Contents(t *testing.T) {
	want := map[string]Kind{
		"H-M1-01": KindHint, "H-M1-02": KindHint, "H-M1-03": KindHint,
		"D-M1-01": KindDetector, "D-M1-02": KindDetector,
		"T-M1-01": KindTour,
		"N-0.1.0": KindWhatsNew,
		// M2: расписание, копилка, пауза (раздел «Покрытие Помощника» плана M2).
		"H-M2-01": KindHint, "H-M2-02": KindHint, "H-M2-03": KindHint,
		"D-M2-01": KindDetector,
		"T-M2-01": KindTour,
		"N-0.2.0": KindWhatsNew,
		// Фидбек-раунд 0.2.2: снять запрет на день, убавить копилку, вкладка «Бонусы», промокоды.
		"H-M2-04": KindHint, "H-M2-05": KindHint, "H-M2-06": KindHint, "H-M2-07": KindHint,
		"N-0.2.2": KindWhatsNew,
		// 0.2.4: табло остатка у значка и окно ребёнка с кнопками (D-53).
		"N-0.2.4": KindWhatsNew,
	}
	got := map[string]Kind{}
	for _, e := range Catalog() {
		got[e.ID] = e.Kind
	}
	if len(got) != len(want) {
		t.Fatalf("записей %d, ждали %d: %v", len(got), len(want), got)
	}
	for id, kind := range want {
		if got[id] != kind {
			t.Errorf("запись %s: вид %q, ждали %q", id, got[id], kind)
		}
	}
}

func TestEveryAnchorOfCatalogIsKnown(t *testing.T) {
	known := map[string]bool{}
	for _, a := range Anchors {
		known[a.ID] = true
	}
	for _, e := range Catalog() {
		if e.Anchor != "" && !known[e.Anchor] {
			t.Errorf("%s: неизвестный якорь %q", e.ID, e.Anchor)
		}
		for i, s := range e.Steps {
			if !known[s.Anchor] {
				t.Errorf("%s, шаг %d: неизвестный якорь %q", e.ID, i+1, s.Anchor)
			}
		}
	}
}

func TestAnchorListHasNoDuplicatesAndKnownScreens(t *testing.T) {
	seen := map[string]bool{}
	screens := map[string]bool{ScreenAny: true}
	for _, s := range Screens {
		screens[s] = true
	}
	for _, a := range Anchors {
		if seen[a.ID] {
			t.Errorf("якорь %q записан дважды", a.ID)
		}
		seen[a.ID] = true
		if !screens[a.Screen] {
			t.Errorf("якорь %q: неизвестный экран %q", a.ID, a.Screen)
		}
	}
}

// bad берёт запись из каталога, портит её и ждёт, что валидатор скажет про это слово.
func bad(t *testing.T, id string, mutate func(e *Entry), want string) {
	t.Helper()
	var entries []Entry
	for _, e := range Catalog() {
		// Копия, чтобы порча не попала в каталог: срезы тоже копируем.
		e.Actions = append([]Action(nil), e.Actions...)
		e.Steps = append([]Step(nil), e.Steps...)
		e.Items = append([]string(nil), e.Items...)
		e.HowTo = append([]string(nil), e.HowTo...)
		e.Covers = append([]string(nil), e.Covers...)
		if e.ID == id {
			mutate(&e)
		}
		entries = append(entries, e)
	}
	ps := Validate(entries)
	if len(ps) == 0 {
		t.Fatalf("%s: порча (%s) не найдена", id, want)
	}
	if !strings.Contains(problemsText(ps), want) {
		t.Fatalf("%s: ждали проблему со словом %q, получили:\n%s", id, want, problemsText(ps))
	}
}

func TestValidatorCatchesBrokenEntries(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		mutate func(e *Entry)
		want   string
	}{
		{"заголовок длиннее предела", "H-M1-02", func(e *Entry) { e.Title = strings.Repeat("а", MaxTitle+1) }, "заголовок длиннее"},
		{"текст длиннее предела", "H-M1-02", func(e *Entry) { e.Body = strings.Repeat("б", MaxBody+1) }, "текст длиннее"},
		{"пустой заголовок", "H-M1-02", func(e *Entry) { e.Title = "" }, "пуст"},
		{"пустой текст", "H-M1-03", func(e *Entry) { e.Body = "  " }, "пуст"},
		{"длинное тире в заголовке", "H-M1-02", func(e *Entry) { e.Title = "Лимиты — по дням" }, "тире"},
		{"среднее тире в тексте", "H-M1-03", func(e *Entry) { e.Body = "Только на сегодня – не навсегда" }, "тире"},
		{"тире в шаге тура", "T-M1-01", func(e *Entry) { e.Steps[0].Body = "Здесь — сегодня" }, "тире"},
		{"тире в пункте «Что нового»", "N-0.1.0", func(e *Entry) { e.Items[0] = "Лимиты — по дням" }, "тире"},
		{"тире в шаге «Как исправить»", "D-M1-02", func(e *Entry) { e.HowTo[0] = "Войдите — в Windows" }, "тире"},
		{"эмодзи в тексте", "H-M1-03", func(e *Entry) { e.Body = "Добавляет время \U0001F600" }, "эмодзи"},
		{"неизвестный якорь у подсказки", "H-M1-02", func(e *Entry) { e.Anchor = "нет-такого" }, "якорь"},
		{"неизвестный якорь в шаге тура", "T-M1-01", func(e *Entry) { e.Steps[2].Anchor = "кнопка-которой-нет" }, "якорь"},
		{"у подсказки нет якоря", "H-M1-02", func(e *Entry) { e.Anchor = "" }, "якорь"},
		{"неизвестный экран в шаге", "T-M1-01", func(e *Entry) { e.Steps[0].Screen = "ничего" }, "экран"},
		{"в туре больше шести шагов", "T-M1-01", func(e *Entry) {
			e.Steps = append(e.Steps, Step{Anchor: "topbar", Screen: ScreenToday, Title: "Лишний", Body: "Седьмой шаг"})
		}, "шагов"},
		{"в туре нет шагов", "T-M1-01", func(e *Entry) { e.Steps = nil }, "шагов"},
		{"тур ссылается на несуществующую подсказку", "T-M1-01", func(e *Entry) { e.Covers = []string{"H-M9-99"} }, "H-M9-99"},
		{"больше двух кнопок-действий", "H-M1-01", func(e *Entry) {
			e.Actions = []Action{{ID: ActionAck, Label: "Раз"}, {ID: ActionStartTour, Label: "Два"}, {ID: ActionOpenLimits, Label: "Три"}}
		}, "действ"},
		{"подпись кнопки длиннее предела", "H-M1-01", func(e *Entry) { e.Actions[0].Label = strings.Repeat("в", MaxLabel+1) }, "подпись"},
		{"неизвестное действие", "H-M1-01", func(e *Entry) { e.Actions[0].ID = "взорвать" }, "действие"},
		{"у напоминания о риске нет кнопки-действия", "D-M1-01", func(e *Entry) { e.Actions = nil }, "действ"},
		{"напоминание не повторяющееся: нет «Больше не напоминать»", "D-M1-01", func(e *Entry) { e.Repeat = false }, "повторя"},
		{"неизвестная подстановка", "D-M1-01", func(e *Entry) { e.Title = "Похоже, у {кого} нет лимитов" }, "подстановк"},
		{"оборванная скобка подстановки", "D-M1-01", func(e *Entry) { e.Title = "Похоже, у {nameGen нет лимитов" }, "подстановк"},
		{"имя ребёнка в заголовке разваливает длину", "D-M1-02", func(e *Entry) {
			e.Title = strings.Repeat("я", MaxTitle-len("{nameGen}")) + "{nameGen}"
		}, "заголовок длиннее"},
		{"номер не по образцу", "H-M1-02", func(e *Entry) { e.ID = "подсказка-2" }, "номер"},
		{"номер не того вида", "H-M1-02", func(e *Entry) { e.ID = "D-M1-77" }, "номер"},
		{"номер дублируется", "H-M1-03", func(e *Entry) { e.ID = "H-M1-02" }, "повторяется"},
		{"версия «Что нового» не совпала с номером", "N-0.1.0", func(e *Entry) { e.Version = "9.9.9" }, "версия"},
		{"в «Что нового» нет пунктов", "N-0.1.0", func(e *Entry) { e.Items = nil }, "пункт"},
		{"технический текст", "H-M1-03", func(e *Entry) { e.Body = "Ошибка: null reference в ipc" }, "техническ"},
		{"лишние пробелы", "H-M1-03", func(e *Entry) { e.Body = " Время  на сегодня " }, "пробел"},
		{"приглашение с «Больше не напоминать»", "H-M1-01", func(e *Entry) { e.Repeat = true }, "приглаш"},
		{"«Как исправить» без шагов", "D-M1-02", func(e *Entry) { e.HowTo = nil }, "шаг"},
		{"действие «Как исправить» без шагов в записи", "D-M1-02", func(e *Entry) { e.HowTo = nil; e.HowToTitle = "" }, "шаг"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { bad(t, c.id, c.mutate, c.want) })
	}
}

func TestValidatorFindsEmptyCatalogAndUnknownKind(t *testing.T) {
	if len(Validate(nil)) == 0 {
		t.Error("пустой каталог не должен проходить")
	}
	ps := Validate([]Entry{{ID: "H-M1-01", Kind: "что-то"}})
	if !strings.Contains(problemsText(ps), "вид") {
		t.Errorf("неизвестный вид не найден:\n%s", problemsText(ps))
	}
}

func TestLabelsAreChecked(t *testing.T) {
	if ps := ValidateLabels(Labels); len(ps) > 0 {
		t.Fatalf("подписи не прошли проверку:\n%s", problemsText(ps))
	}
	broken := map[string]string{"later": "Позже — потом", "mute": strings.Repeat("н", MaxLabel+1)}
	ps := ValidateLabels(broken)
	if len(ps) < 2 {
		t.Fatalf("ждали две проблемы, получили:\n%s", problemsText(ps))
	}
}

// Ловит: записи есть, а в матрице покрытия (экран × записи) о них ни слова.
func TestCoverageMatrixMentionsEveryEntryAndAnchor(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "HELPER_COVERAGE.md"))
	if err != nil {
		t.Fatalf("нет матрицы покрытия Помощника: %v", err)
	}
	doc := string(raw)
	for _, e := range Catalog() {
		if !strings.Contains(doc, e.ID) {
			t.Errorf("в docs/HELPER_COVERAGE.md нет записи %s", e.ID)
		}
	}
	for _, a := range Anchors {
		if !strings.Contains(doc, a.ID) {
			t.Errorf("в docs/HELPER_COVERAGE.md нет якоря %s", a.ID)
		}
	}
}

// Ловит: подставили в шаблон длинное имя, и заголовок не влезает в карточку.
func TestExpandedTemplatesStayShort(t *testing.T) {
	for _, e := range Catalog() {
		if e.Kind != KindHint && e.Kind != KindDetector {
			continue
		}
		if n := utf8.RuneCountInString(Expand(e.Title, LongName)); n > MaxTitleExpanded {
			t.Errorf("%s: заголовок с длинным именем %d знаков, предел %d", e.ID, n, MaxTitleExpanded)
		}
		if n := utf8.RuneCountInString(Expand(e.Body, LongName)); n > MaxBodyExpanded {
			t.Errorf("%s: текст с длинным именем %d знаков, предел %d", e.ID, n, MaxBodyExpanded)
		}
	}
}
