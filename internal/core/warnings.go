package core

// warnThresholds — пороги предупреждений R9 в минутах, по убыванию.
var warnThresholds = [3]int{15, 5, 1}

// warnings — пороги как события, а не таймер (Ruling 10, L-06, L-118): порог T взведён,
// если при взведении остаток не меньше T·60 с; срабатывает один раз, когда остаток
// стал T·60 с или меньше. Взводятся заново при сдвиге события (доп. время, новые
// правила, новые сутки); прошедшие пороги при этом не взводятся — при входе за 3 мин
// до конца покажется только порог 1 мин.
type warnings struct {
	armed [len(warnThresholds)]bool
}

// arm взводит пороги для остатка remainingS; limited = false (без лимита, безопасный
// режим) — событий нет, все пороги погашены.
func (w *warnings) arm(remainingS int, limited bool) {
	for i, t := range warnThresholds {
		w.armed[i] = limited && remainingS >= t*60
	}
}

// fire гасит сработавшие при остатке remainingS пороги и возвращает меньший из них
// (одна причина — одно уведомление); 0 — ничего не сработало.
func (w *warnings) fire(remainingS int) int {
	got := 0
	for i, t := range warnThresholds {
		if w.armed[i] && remainingS <= t*60 {
			w.armed[i] = false
			got = t
		}
	}
	return got
}
