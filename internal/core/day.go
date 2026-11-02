package core

import "time"

// DayState — счётчики одного дня ребёнка (таблица day_state, ТЗ 4). Длительности —
// целые секунды; TotalS — общее время R4 (игры + видео + прочее), учёба отдельно (D-7);
// ExtraMin — доп. время за этот день (R10).
type DayState struct {
	Day          string // YYYY-MM-DD по местному времени
	TotalS       int
	GamesS       int
	VideoS       int
	StudyS       int
	ExtraMin     int
	PauseUntilMs *int64 // момент конца паузы, UTC мс; nil — паузы нет (R11)
	NoForbidden  bool   // запретные часы сняты родителем на этот день (R24, D-51); новый день — false
	BonusGranted bool   // автобонус за этот день уже начислен (R25, D-49); защита от повторного начисления
	// LastMinuteEndMs — момент «конец по часам» последней минуты (R2, D-60), UTC мс: ставится
	// предупреждением за 1 мин до исчерпания общего лимита; пока задан, остаток общего времени
	// идёт по часам до этого момента независимо от R2. nil — отсчёта нет.
	LastMinuteEndMs *int64
}

// DayOf возвращает дату дня момента t по его поясу в формате YYYY-MM-DD (ТЗ 4).
func DayOf(t time.Time) string { return t.Format(time.DateOnly) }

// RemainingTotalS — остаток общего времени (R4): лимит + доп. время − общее время.
// Без лимита — unlimited = true. Остаток не бывает меньше нуля (лимит могли уменьшить
// ниже уже использованного).
func RemainingTotalS(limit DayLimit, extraMin, totalS int) (remaining int, unlimited bool) {
	if limit.Unlimited() {
		return 0, true
	}
	remaining = (*limit.TotalMin+extraMin)*60 - totalS
	if remaining < 0 {
		remaining = 0
	}
	return remaining, false
}

// RemainingGamesS — остаток игр (R5): лимит игр дня + доп. время − игры за день. Лимита
// игр нет (GamesMin nil) — unlimited = true: игры ограничены только общим временем.
// Остаток не бывает меньше нуля (лимит уменьшили или доп. время убавили).
func RemainingGamesS(limit DayLimit, extraMin, gamesS int) (remaining int, unlimited bool) {
	if limit.GamesMin == nil {
		return 0, true
	}
	remaining = (*limit.GamesMin+extraMin)*60 - gamesS
	if remaining < 0 {
		remaining = 0
	}
	return remaining, false
}
