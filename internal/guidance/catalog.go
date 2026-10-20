// Package guidance хранит каталог Помощника (FR-34, ТЗ 8.4 и 10.9) и проверяет его.
//
// Тексты подсказок, туров и «Что нового» живут в данных, а не в коде экранов. Источник для
// окна — frontend/src/guidance/catalog.ts; этот файл — его Go-зеркало (номера записей, тексты,
// якоря), чтобы проверка каталога не зависела от фронта и шла в обычном go test. Рассинхрон
// зеркала и каталога окна ловит тест фронта (catalog.test.ts): он читает этот файл.
//
// Записи неизменяемы по номеру: смысл поменялся — новый номер (состояние профиля ссылается на номера).
package guidance

// Kind — вид записи каталога.
type Kind string

const (
	KindHint     Kind = "hint"     // карточка-подсказка у элемента
	KindDetector Kind = "detector" // напоминание о рискованной ситуации
	KindTour     Kind = "tour"     // тур по окну
	KindWhatsNew Kind = "whatsnew" // «Что нового» по версии
)

// Идентификаторы действий кнопок карточки. Что делает каждое, решает окно, но список закрыт:
// действие, которого нет здесь, не пройдёт проверку.
const (
	ActionAck          = "ack"           // «Понятно»: принято, больше не показывать
	ActionStartTour    = "start-tour"    // запустить тур
	ActionOpenLimits   = "open-limits"   // открыть «Дети → Лимиты» этого ребёнка
	ActionHowToFix     = "how-to-fix"    // показать шаги исправления
	ActionOpenSchedule = "open-schedule" // открыть «Дети → Запретные часы» (M2, I1 ревью)
)

// Action — кнопка-действие карточки.
type Action struct {
	ID    string
	Label string
}

// Step — шаг тура: к чему привязан и на каком экране виден.
type Step struct {
	Anchor string
	Screen string
	Title  string
	Body   string
}

// Entry — запись каталога. Нужные поля зависят от вида: у подсказок и напоминаний — якорь,
// заголовок, текст и действия; у тура — шаги; у «Что нового» — версия и пункты.
type Entry struct {
	ID   string
	Kind Kind

	// Подсказка и напоминание.
	Anchor  string
	Title   string
	Body    string
	Actions []Action
	// Once — приглашение: показывается один раз. Repeat — повторяющееся: у карточки есть
	// «Больше не напоминать». Напоминания о риске всегда повторяющиеся.
	Once   bool
	Repeat bool
	// HowToTitle и HowTo — шаги окна «Как исправить» у напоминания.
	HowToTitle string
	HowTo      []string

	// Тур.
	Steps []Step
	// Covers — подсказки, которые тур уже объяснил: после прохождения тура они не показываются.
	Covers []string

	// «Что нового».
	Version string
	Items   []string
}

// Anchor — элемент окна, к которому привязаны подсказки и шаги тура.
type Anchor struct {
	ID     string
	Screen string // экран, где элемент виден; ScreenAny — на любом экране окна родителя
}

// Экраны окна родителя (id совпадают с меню окна).
const (
	ScreenAny      = "any"
	ScreenToday    = "today"
	ScreenChildren = "children"
	ScreenBonus    = "bonus" // экран «Копилка» (M2, I1 ревью)
	ScreenJournal  = "journal"
	ScreenSettings = "settings"
	ScreenHelp     = "help"
)

// Screens — экраны, на которые может ссылаться шаг тура.
var Screens = []string{ScreenToday, ScreenChildren, ScreenBonus, ScreenJournal, ScreenSettings, ScreenHelp}

// Anchors — известные якоря окна родителя. Новая подсказка привязывается только к якорю из
// этого списка; сам элемент в экране помечается data-help-anchor с тем же номером.
var Anchors = []Anchor{
	{ID: "topbar", Screen: ScreenAny},
	{ID: "menu-today", Screen: ScreenAny},
	{ID: "menu-children", Screen: ScreenAny},
	{ID: "menu-bonus", Screen: ScreenAny},
	{ID: "savebar", Screen: ScreenAny},
	{ID: "child-card", Screen: ScreenToday},
	{ID: "extra-buttons", Screen: ScreenToday},
	{ID: "child-card-pause", Screen: ScreenToday},
	{ID: "child-card-bonus", Screen: ScreenToday},
	{ID: "limits-table", Screen: ScreenChildren},
	{ID: "schedule-tab", Screen: ScreenChildren},
	{ID: "bonus-grant", Screen: ScreenBonus},
}

// Подписи окна Помощника, общие для всех записей: кнопки карточки и тура.
var Labels = map[string]string{
	"later":           "Позже",
	"close":           "Закрыть подсказку",
	"mute":            "Больше не напоминать",
	"tourSkip":        "Пропустить тур",
	"tourNext":        "Далее",
	"tourBack":        "Назад",
	"tourDone":        "Готово",
	"tourMissingNote": "Этот элемент сейчас на экране не виден. Ничего страшного, продолжим.",
}

// Catalog возвращает записи M1 (раздел «Покрытие Помощника» плана вехи).
func Catalog() []Entry {
	return []Entry{
		{
			ID:     "H-M1-01",
			Kind:   KindHint,
			Anchor: "topbar",
			Title:  "Короткий тур по окну родителя: 1 минута",
			Body:   "Покажем, где смотреть время детей и как менять лимиты. Это необязательно.",
			Actions: []Action{
				{ID: ActionStartTour, Label: "Начать"},
			},
			Once: true,
		},
		{
			ID:     "H-M1-02",
			Kind:   KindHint,
			Anchor: "limits-table",
			Title:  "Лимиты по дням недели",
			Body:   "Задайте время на каждый день. «Без лимита» значит, что ограничений по времени нет.",
		},
		{
			ID:     "H-M1-03",
			Kind:   KindHint,
			Anchor: "extra-buttons",
			Title:  "Кнопки добавляют время только на сегодня",
			Body:   "+15, +30 и +60 прибавляют минуты к сегодняшнему лимиту. Завтра лимит снова обычный.",
		},
		{
			ID:     "D-M1-01",
			Kind:   KindDetector,
			Anchor: "child-card",
			Title:  "Похоже, у {nameGen} нет ограничений ни в один день",
			Body:   "Время за компьютером не ограничено ни в один день недели. Задайте лимиты, если это не так.",
			Actions: []Action{
				{ID: ActionOpenLimits, Label: "Задать лимиты"},
			},
			Repeat: true,
		},
		{
			ID:     "D-M1-02",
			Kind:   KindDetector,
			Anchor: "child-card",
			Title:  "У {nameGen} теперь права администратора: ограничения не работают",
			Body:   "Windows считает эту учётную запись взрослой. Верните обычные права, и ограничения заработают.",
			Actions: []Action{
				{ID: ActionHowToFix, Label: "Как исправить"},
			},
			Repeat:     true,
			HowToTitle: "Как вернуть обычные права",
			HowTo: []string{
				"Войдите в Windows в своей учётной записи администратора.",
				"Откройте «Параметры», затем «Учётные записи» и «Другие пользователи».",
				"Выберите учётную запись ребёнка и нажмите «Изменить тип учётной записи».",
				"Выберите «Стандартный пользователь» и сохраните. Программа сама заметит перемену.",
			},
		},
		{
			ID:    "T-M1-01",
			Kind:  KindTour,
			Title: "Короткий тур по окну родителя",
			Steps: []Step{
				{
					Anchor: "menu-today",
					Screen: ScreenToday,
					Title:  "Сегодня",
					Body:   "Здесь у каждого ребёнка видно, сколько времени осталось и не заблокирован ли компьютер.",
				},
				{
					Anchor: "child-card",
					Screen: ScreenToday,
					Title:  "Карточка ребёнка",
					Body:   "Статус словом и цветом, сколько времени использовано из лимита.",
				},
				{
					Anchor: "extra-buttons",
					Screen: ScreenToday,
					Title:  "Добавить время",
					Body:   "+15, +30 и +60 прибавляют минуты на сегодня. Ошибку можно отменить в течение 10 секунд.",
				},
				{
					Anchor: "menu-children",
					Screen: ScreenChildren,
					Title:  "Дети",
					Body:   "Здесь правила каждого ребёнка.",
				},
				{
					Anchor: "limits-table",
					Screen: ScreenChildren,
					Title:  "Лимиты по дням",
					Body:   "Задайте время на каждый день недели. «Без лимита» значит, что ограничений нет.",
				},
				{
					Anchor: "savebar",
					Screen: ScreenChildren,
					Title:  "Сохранить",
					Body:   "Правки начинают действовать после «Сохранить» внизу окна. Лишнее можно отменить.",
				},
			},
			Covers: []string{"H-M1-02", "H-M1-03"},
		},
		{
			ID:      "N-0.1.0",
			Kind:    KindWhatsNew,
			Title:   "Первая рабочая версия",
			Version: "0.1.0",
			Items: []string{
				"Лимит времени на каждый день недели для каждого ребёнка",
				"Значок у часов с остатком времени",
				"Предупреждения за 15, 5 и 1 минуту",
				"Экран блокировки, когда время вышло, и снятие паролем родителя",
			},
		},
	}
}
