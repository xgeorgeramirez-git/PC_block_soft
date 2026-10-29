package core

import "time"

// Category — куда относится активная секунда (R3, ТЗ 2). Нулевое значение — «Прочее».
type Category int

// Категории секунд. «Системное» программы и рабочий стол датчики относят к CatOther (R3).
const (
	CatOther Category = iota
	CatGames
	CatVideoSocial
	CatStudy
)

// String возвращает название категории для интерфейса; неизвестное значение — «Прочее».
func (c Category) String() string {
	switch c {
	case CatGames:
		return "Игры"
	case CatVideoSocial:
		return "Видео и соцсети"
	case CatStudy:
		return "Учёба"
	default:
		return "Прочее"
	}
}

// Weekday — день недели по ТЗ 4: 1 = пн … 7 = вс.
type Weekday int

// Дни недели.
const (
	Monday Weekday = iota + 1
	Tuesday
	Wednesday
	Thursday
	Friday
	Saturday
	Sunday
)

// WeekdayOf возвращает день недели момента t по его поясу.
func WeekdayOf(t time.Time) Weekday {
	return Weekday((int(t.Weekday())+6)%7 + 1)
}

// SessionSample — один снимок датчиков детского сеанса (R2, R3); страж присылает его
// раз в секунду. Заголовки окон и нажатия клавиш сюда не попадают (R23).
type SessionSample struct {
	AtMs           int64    // момент снимка, UTC в миллисекундах (ТЗ 4)
	TzOffsetMin    int      // смещение пояса на момент снимка, минуты
	LastInputAgoMs int64    // сколько прошло с последнего ввода с клавиатуры или мыши
	Fullscreen     bool     // окно на переднем плане занимает весь экран
	Foreground     Category // категория программы (для Brave — сайта) на переднем плане
	SessionActive  bool     // сеанс не заблокирован и не спит
}

// DayLimit — лимит дня недели. TotalMin nil — без лимита; 0 — в этот день нельзя (R4).
// GamesMin — лимит игр (R5): nil — игры ограничены только общим лимитом; 0 — игры сегодня нельзя.
type DayLimit struct {
	TotalMin *int
	GamesMin *int
}

// LimitOf возвращает лимит в min минут.
func LimitOf(min int) DayLimit { return DayLimit{TotalMin: &min} }

// Unlimited сообщает, что лимита нет.
func (l DayLimit) Unlimited() bool { return l.TotalMin == nil }

// Rules — правила ребёнка, которые служба передаёт ядру: лимиты по дням недели
// (индекс 0 = пн), запретные интервалы расписания (R7, плоский список — проще
// копировать для FR-33) и режим окончания (R8). Broken — правила не прочитаны
// (повреждение базы и т.п.): ограничения не применяются, событие уходит в журнал (R18).
type Rules struct {
	Week     [7]DayLimit
	Schedule []Interval
	EndMode  EndMode
	Broken   bool
}

// EndMode — режим окончания (R8, D-9): что происходит, когда общее время вышло или идёт
// запретный интервал. Нулевое значение — «Блокировать экран» (по умолчанию).
type EndMode int

// Режимы окончания.
const (
	EndBlock     EndMode = iota // экран блокировки, игры закрываются, остальное под экраном
	EndStudyOnly                // экрана нет, закрывается всё, кроме «Учёбы» и «Системного»
)

// String — ключ режима для хранилища и канала: "lock" или "study_only".
func (m EndMode) String() string {
	if m == EndStudyOnly {
		return "study_only"
	}
	return "lock"
}

// EndModeFromString разбирает ключ режима; неизвестное значение — «Блокировать экран»
// (безопасный вариант по умолчанию, R8).
func EndModeFromString(s string) EndMode {
	if s == "study_only" {
		return EndStudyOnly
	}
	return EndBlock
}

// For возвращает лимит дня недели; день вне 1…7 — без лимита (безопасно, R18).
func (r Rules) For(wd Weekday) DayLimit {
	if wd < Monday || wd > Sunday {
		return DayLimit{}
	}
	return r.Week[wd-1]
}

// LockKind — какой экран блокировки показать.
type LockKind int

// Виды блокировки.
const (
	LockNone      LockKind = iota // экрана блокировки нет
	LockTimeUp                    // «Время на сегодня закончилось» (R4)
	LockForbidden                 // «Сейчас нельзя пользоваться компьютером» — запретный интервал (R7)
	LockBypass                    // «Попытка обойти защиту» (R17)
)

// WarnEvent — вид ближайшего события, к которому относится предупреждение R9: от него
// зависит текст у стража («время закончится» / «начнутся запретные часы» / «закончится
// пауза» / «закончится время на игры»). Нулевое значение — нет события.
type WarnEvent int

// Виды событий предупреждений.
const (
	WarnNone      WarnEvent = iota
	WarnLimit               // исчерпание общего времени (R4)
	WarnForbidden           // начало запретного интервала (R7)
	WarnPauseEnd            // окончание паузы (R11, G11)
	WarnGames               // исчерпание лимита игр, когда на переднем плане игра (R5, R9)
)

// String — короткий ключ вида события (для тестов и выбора текста).
func (w WarnEvent) String() string {
	switch w {
	case WarnLimit:
		return "limit"
	case WarnForbidden:
		return "forbidden"
	case WarnPauseEnd:
		return "pause_end"
	case WarnGames:
		return "games"
	default:
		return "none"
	}
}

// Status — состояние ребёнка сейчас; один источник статуса для значка, мини-панели,
// «Моей статистики» и карточки родителя (Review Focus 5). Цвет по ТЗ 8.0 п.3.
type Status int

// Состояния.
const (
	StatusOK        Status = iota // можно, осталось больше 15 мин (зелёный)
	StatusLow                     // осталось 15 мин или меньше (янтарный)
	StatusTimeUp                  // время вышло, экран блокировки (красный)
	StatusForbidden               // запретные часы, экран блокировки (красный) — R7
	StatusBypass                  // попытка обойти защиту, экран блокировки (красный)
	StatusUnlimited               // на сегодня лимита нет
	StatusPaused                  // пауза: ограничения не действуют, учёт идёт (R11)
	StatusSafe                    // правила не прочитаны: ограничения не действуют (R18)
)

// Виды событий журнала (R22), которые возвращает ядро.
const (
	EvExtraTime    = "extra_time"    // родитель добавил доп. время (R10)
	EvSafeMode     = "safe_mode"     // правила не прочитаны, ограничения сняты (R18)
	EvDayClosed    = "day_closed"    // сутки сменились, итог дня — в ClosedDay (R16)
	EvBypass       = "bypass"        // попытка обойти защиту (R17)
	EvBlockCleared = "block_cleared" // родитель снял блокировку за обход
	EvPause        = "pause"         // родитель поставил паузу (R11)
	EvPauseEnd     = "pause_end"     // пауза закончилась, правила снова действуют (R11)
	EvBonusSpent   = "bonus_spent"   // ребёнок взял из копилки (R10, FR-11)
	EvNoForbidden  = "no_forbidden"  // родитель снял/вернул запретные часы на сегодня (R24, D-51)
	EvBonusGranted = "bonus_granted" // автоначисление бонуса за неизрасходованное время (R25, D-49)
	EvPromoRedeem  = "promo_redeem"  // ребёнок ввёл промокод, добавлено время (R26, D-50)
)

// Event — событие для журнала (R22): вид и данные; момент добавляет служба.
type Event struct {
	Kind    string
	Details map[string]any
}
