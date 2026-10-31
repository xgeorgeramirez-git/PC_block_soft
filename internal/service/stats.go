package service

import (
	"sort"
	"strings"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/programs"
)

// Вкладки «Итогов» (FB-10, D-58): время каждой программы за сегодня по категориям учёта.

// textOtherTime — строка вкладки для времени без программы: рабочий стол, системные и скрытые
// программы, дотяжка последней минуты (R2, D-60). Без неё сумма строк не сходилась бы с итогом
// категории (класс A).
const textOtherTime = "Другое"

// usageCatVideo — категория учёта «Видео и соцсети» (сайты, M4): программ в ней нет.
const usageCatVideo = "video_social"

// statsCatOrder — порядок категорий в ответе: как строки «Итогов».
var statsCatOrder = map[string]int{programs.CatGames: 0, usageCatVideo: 1, programs.CatStudy: 2, programs.CatOther: 3}

// programTimes собирает строки вкладок за день состояния day: сохранённый учёт по программам
// (DayUsage) и ещё не сохранённый накопитель службы (cs.usage, сброс раз в 30 с). Название —
// то же, что в списке программ и журнале (displayName, D-59); скрытая (D-57) имени не получает,
// её секунды идут в «Другое» своей категории. «Другое» — итог категории минус названные строки:
// сумма строк категории равна итогу категории. Внутри категории самые долгие сверху, «Другое»
// последней. Ошибка чтения учёта — всё время категории в «Другое» (время не пропадает).
func (s *Service) programTimes(cs *childState, day core.DayState) []ipc.ProgramTime {
	type key struct{ cat, path string }
	secs := map[key]int{}
	rows, err := s.cfg.Store.DayUsage(cs.child.ID, day.Day)
	if err != nil {
		s.logRate("dayusage", "не удалось прочитать время по программам за %s у %s: %v", day.Day, cs.child.Name, err)
		rows = nil
	}
	for _, u := range rows {
		secs[key{u.Category, u.Subject}] += u.Seconds
	}
	for k, n := range cs.usage {
		if k.day == day.Day {
			secs[key{k.cat, k.path}] += n
		}
	}
	known := map[string]Program{}
	if list, err := s.cfg.Store.ListPrograms(); err == nil {
		for _, p := range list {
			known[p.Path] = p
		}
	} else {
		s.logRate("programs", "не удалось прочитать список программ: %v", err)
	}
	pending := map[string]string{}
	if list, err := s.cfg.Store.ListPending(); err == nil {
		for _, p := range list {
			pending[p.Path] = programName(p.Name, p.Path)
		}
	}
	totals := map[string]int{
		programs.CatGames: day.GamesS,
		usageCatVideo:     day.VideoS,
		programs.CatStudy: day.StudyS,
		programs.CatOther: max(0, day.TotalS-day.GamesS-day.VideoS),
	}
	named := map[string]int{}
	out := []ipc.ProgramTime{}
	for k, n := range secs {
		if _, ok := totals[k.cat]; !ok || n <= 0 {
			continue
		}
		var name string
		if p, ok := known[k.path]; ok {
			if p.Hidden {
				continue // секунды скрытой остаются в «Другое» категории
			}
			name = displayName(p)
		} else if pn, ok := pending[k.path]; ok {
			name = pn
		} else {
			name = programs.NameFromPath(k.path)
		}
		out = append(out, ipc.ProgramTime{Path: k.path, Name: name, Category: k.cat, Seconds: n})
		named[k.cat] += n
	}
	for cat, total := range totals {
		if rest := total - named[cat]; rest > 0 {
			out = append(out, ipc.ProgramTime{Name: textOtherTime, Category: cat, Seconds: rest})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Category != b.Category {
			return statsCatOrder[a.Category] < statsCatOrder[b.Category]
		}
		if (a.Path == "") != (b.Path == "") {
			return b.Path == "" // «Другое» последней
		}
		if a.Seconds != b.Seconds {
			return a.Seconds > b.Seconds
		}
		if la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name); la != lb {
			return la < lb
		}
		return a.Path < b.Path
	})
	return out
}
