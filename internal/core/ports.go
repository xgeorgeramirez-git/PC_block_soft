package core

// Порты ядра (ТЗ 3.3): всё, что связано с Windows и базой, реализуют адаптеры службы
// и стража; ядро видит только эти интерфейсы. Правила служба передаёт в Engine значением
// (NewEngine, SetRules), события журнала ядро возвращает в Decision.Events.

// Sensors — датчики детского сеанса (страж): снимок раз в секунду.
type Sensors interface {
	Sample() SessionSample
}

// ProcessControl — управление процессами в детском сеансе (R8: закрыть программы категории).
type ProcessControl interface {
	CloseCategory(Category) error
}

// Journal — журнал событий (R22): адаптер службы пишет Decision.Events в таблицу events,
// добавляя момент и смещение пояса.
type Journal interface {
	Write(kind string, details map[string]any)
}
