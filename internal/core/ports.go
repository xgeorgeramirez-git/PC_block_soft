package core

// Порты ядра (ТЗ 3.3): всё, что связано с Windows и базой, реализуют адаптеры службы
// и стража; ядро видит только эти интерфейсы.

// Sensors — датчики детского сеанса (страж): снимок раз в секунду.
type Sensors interface {
	Sample() SessionSample
}

// ProcessControl — управление процессами в детском сеансе (R8: закрыть программы категории).
type ProcessControl interface {
	CloseCategory(Category) error
}

// Journal — журнал событий (R22). kind — вид события (константы Ev*), details — данные
// события; адаптер добавляет момент и пишет в таблицу events.
type Journal interface {
	Write(kind string, details map[string]any)
}

// RulesSource — откуда ядро читает правила ребёнка. Ошибка чтения включает безопасный
// режим: ограничения не применяются, событие уходит в журнал (R18).
type RulesSource interface {
	Rules() (Rules, error)
}
