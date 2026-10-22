package core

// Автоматические бонусы за неизрасходованное время (R25, D-49). Ядро считает только
// размер бонуса за прошедший день по его итогам; начисление в копилку с учётом потолка
// 600 и защита от повторного начисления за день — на службе (баланс копилки ядро не
// хранит, как и у SpendBonus).

// DailyBonusMinutes возвращает размер автобонуса в минутах за прошедший день (R25, D-49):
// N × (остаток_общего_времени div 10), где остаток = лимит дня + доп. время − общее время
// за день (не меньше 0), делится на 10 с округлением вниз (неполные 10 минут не считаются),
// N — ratioN («бонус за неизрасходованное время», по умолчанию 5).
//
// Бонус не начисляется (возвращается 0), если день был «не разрешён к игре»: лимит дня
// отсутствует (без лимита — «неизрасходованного» нет), равен 0, либо день целиком покрыт
// запретными интервалами (scheduleAllowsPlay = false). Потолок копилки 600 здесь не
// применяется — его учитывает служба при начислении (нужен текущий баланс).
func DailyBonusMinutes(limit DayLimit, extraMin, totalS, ratioN int, scheduleAllowsPlay bool) int {
	if ratioN <= 0 || !scheduleAllowsPlay || limit.Unlimited() || *limit.TotalMin == 0 {
		return 0
	}
	remS, unlimited := RemainingTotalS(limit, extraMin, totalS)
	if unlimited {
		return 0
	}
	remMin := remS / 60 // остаток в целых минутах (неполная минута остатка не считается)
	return ratioN * (remMin / 10)
}

// DayFullyForbidden сообщает, что все сутки дня недели wd целиком покрыты запретными
// интервалами (R24/R25): учитываются интервалы самого дня (в том числе переходящие через
// полночь — их утренняя часть) и перенос интервала предыдущего дня. В такой день игра не
// была разрешена ни минуты, поэтому автобонус (R25) не начисляется.
func DayFullyForbidden(r Rules, wd Weekday) bool {
	prev := Weekday((int(wd)+5)%7 + 1)
	type seg struct{ start, end int } // отрезки на оси суток [0, 1440)
	var segs []seg
	for _, iv := range NormalizeIntervals(r.Schedule) {
		switch {
		case iv.Weekday == wd && iv.crossesMidnight():
			segs = append(segs, seg{iv.StartMin, dayMinutes}) // часть до полуночи в этих сутках
		case iv.Weekday == wd:
			segs = append(segs, seg{iv.StartMin, iv.EndMin})
		case iv.Weekday == prev && iv.crossesMidnight():
			segs = append(segs, seg{0, iv.EndMin}) // утренний хвост интервала прошлого дня
		}
	}
	if len(segs) == 0 {
		return false
	}
	// Объединяем отрезки и проверяем сплошное покрытие [0, 1440).
	for i := 1; i < len(segs); i++ { // сортировка вставками (отрезков немного)
		for j := i; j > 0 && segs[j-1].start > segs[j].start; j-- {
			segs[j-1], segs[j] = segs[j], segs[j-1]
		}
	}
	covered := 0
	for _, s := range segs {
		if s.start > covered {
			return false // разрыв в покрытии
		}
		if s.end > covered {
			covered = s.end
		}
	}
	return covered >= dayMinutes
}
