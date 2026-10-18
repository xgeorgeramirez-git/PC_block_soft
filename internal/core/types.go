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
// В M1 только общий лимит.
type DayLimit struct {
	TotalMin *int
}

// LimitOf возвращает лимит в min минут.
func LimitOf(min int) DayLimit { return DayLimit{TotalMin: &min} }

// Unlimited сообщает, что лимита нет.
func (l DayLimit) Unlimited() bool { return l.TotalMin == nil }

// Rules — правила ребёнка, которые читает ядро: лимиты по дням недели (индекс — Weekday-1).
type Rules struct {
	Days [7]DayLimit
}

// For возвращает лимит дня недели; день вне 1…7 — без лимита (безопасно, R18).
func (r Rules) For(wd Weekday) DayLimit {
	if wd < Monday || wd > Sunday {
		return DayLimit{}
	}
	return r.Days[wd-1]
}

// Stage — состояние ребёнка сейчас; один источник статуса для значка, мини-панели,
// «Моей статистики» и карточки родителя (Review Focus 5). Цвет по ТЗ 8.0 п.3.
type Stage int

// Состояния.
const (
	StageUnlimited Stage = iota // лимита нет или правила не прочитаны (R18)
	StageOK                     // можно, осталось больше 15 мин (зелёный)
	StageLow                    // осталось 15 мин или меньше (янтарный)
	StageTimeUp                 // время вышло, экран блокировки (красный)
	StageBypass                 // попытка обойти защиту, экран блокировки (красный)
)
