package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Перерыв на отдых (фидбек-раунд 0.3.2: FB-14, D-62, R27, FR-37).

// Тексты вкладки «Отдых» и команды «Закончить перерыв».
const (
	textBreakAfterRange    = "Можно от 15 до 240 мин"
	textBreakWarnRange     = "Можно от 1 до 15 мин"
	textBreakRestRange     = "Можно от 5 до 60 мин"
	textBreakSelfRestRange = "Можно от 5 до 120 мин"
	textBreakSelfRestLess  = "Не меньше %d мин: столько длится отдых" // %d — «Отдых» (B-44)
	textBreakCountsBad     = "Выберите, что считать"
	textBreakMessageLong   = "Не больше 120 символов"
	textBreakEnded         = "Перерыв закончен"
	textNoBreak            = "Перерыва сейчас нет"
)

// breakView — «Отдых» правил ребёнка в виде канала.
func breakView(r core.Rules) ipc.BreakRules { return ipc.BreakRulesFrom(r.Break) }

// breakFromIPC проверяет «Отдых» из запроса по пределам FR-37 (и при выключенном переключателе:
// значения сохраняются) и возвращает настройки ядра; ошибка — имя поля и текст у поля (D-45).
func breakFromIPC(b ipc.BreakRules) (core.BreakRules, string, string) {
	in := func(v, lo, hi int) bool { return v >= lo && v <= hi }
	b.Message = strings.TrimSpace(b.Message)
	switch {
	case !in(b.AfterMin, core.BreakAfterMinMin, core.BreakAfterMinMax):
		return core.BreakRules{}, ipc.FieldBreakAfterMin, textBreakAfterRange
	case b.Counts != ipc.BreakCountsAll && b.Counts != ipc.BreakCountsGames:
		return core.BreakRules{}, ipc.FieldBreakCounts, textBreakCountsBad
	case !in(b.WarnMin, core.BreakWarnMinMin, core.BreakWarnMinMax):
		return core.BreakRules{}, ipc.FieldBreakWarnMin, textBreakWarnRange
	case !in(b.RestMin, core.BreakRestMinMin, core.BreakRestMinMax):
		return core.BreakRules{}, ipc.FieldBreakRestMin, textBreakRestRange
	case !in(b.SelfRestMin, core.BreakSelfRestMinMin, core.BreakSelfRestMinMax):
		return core.BreakRules{}, ipc.FieldBreakSelfRestMin, textBreakSelfRestRange
	case b.SelfRestMin < b.RestMin:
		return core.BreakRules{}, ipc.FieldBreakSelfRestMin, fmt.Sprintf(textBreakSelfRestLess, b.RestMin)
	case utf8.RuneCountInString(b.Message) > core.BreakMessageMaxRunes:
		return core.BreakRules{}, ipc.FieldBreakMessage, textBreakMessageLong
	}
	return b.Core(), "", ""
}

// hEndBreak — «Закончить перерыв» (R27): родитель из окна или страж после пароля (одно действие).
// Перерыва нет — отказ без изменений. Счёт к перерыву с нуля сохраняется сразу.
func (s *Service) hEndBreak(c *callCtx) ipc.Reply {
	var req ipc.ChildRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	cs := s.childArg(c, req.ChildID)
	if cs == nil {
		return ipc.Fail(textNoSuchChild)
	}
	d, ok := cs.eng.EndBreak()
	if !ok {
		return ipc.Fail(textNoBreak)
	}
	if c.viaBind {
		s.spendBind(c.peer.SessionID)
	}
	s.afterDecision(cs, d, nil)
	s.saveDay(cs)
	return ipc.OK(ipc.CardReply{Card: s.cardOf(cs), Message: textBreakEnded})
}
