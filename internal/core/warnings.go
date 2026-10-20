package core

// warnThresholds — пороги предупреждений R9 в минутах, по убыванию.
var warnThresholds = [3]int{15, 5, 1}

// warnings — пороги как события, а не таймер (Ruling 10, L-06, L-118): порог T взведён,
// если при взведении дельта до ближайшего события не меньше T·60 с; срабатывает один раз,
// когда дельта стала T·60 с или меньше. Взводятся заново при сдвиге события (доп. время,
// бонус, пауза, новые правила, новые сутки); прошедшие пороги при этом не взводятся — при
// входе за 3 мин до события покажется только порог 1 мин. Хранит вид ближайшего события
// (R9: текст предупреждения зависит от того, что именно кончится — лимит, интервал, пауза).
type warnings struct {
	armed [len(warnThresholds)]bool
	event WarnEvent
}

// arm взводит пороги для дельты deltaS секунд до ближайшего события вида ev; limited =
// false (без события — без лимита, безопасный режим) гасит все пороги.
func (w *warnings) arm(deltaS int, ev WarnEvent, limited bool) {
	w.event = ev
	for i, t := range warnThresholds {
		w.armed[i] = limited && deltaS >= t*60
	}
}

// fire гасит сработавшие при дельте deltaS пороги и возвращает меньший из них (одна
// причина — одно уведомление) и вид события; 0 — ничего не сработало.
func (w *warnings) fire(deltaS int) (int, WarnEvent) {
	got := 0
	for i, t := range warnThresholds {
		if w.armed[i] && deltaS <= t*60 {
			w.armed[i] = false
			got = t
		}
	}
	if got == 0 {
		return 0, WarnNone
	}
	return got, w.event
}
