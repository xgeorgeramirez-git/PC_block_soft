package systray

import (
	"errors"
	"strconv"

	"pcblock/internal/ipc"
)

// Решения значка без вызовов Windows (run_windows.go только выполняет их): что делает кнопка
// мини-панели и табло, сколько взять из копилки, какой текст показать после ответа службы,
// какой сеанс указан в ключе запуска.

// panelAct — что значок делает по нажатию кнопки мини-панели.
type panelAct int

const (
	actNone    panelAct = iota // кнопка ничего не открывает
	actExplain                 // недоступная кнопка: показать, почему (L-17)
	actRefresh                 // повторить запрос к службе
	actSpend                   // взять из копилки
	actOpen                    // попросить службу открыть окно
)

// panelAction решает, что делает кнопка b; у actExplain второе значение — пояснение, у actOpen —
// вид окна для команды OpenWindow. «Попросить ещё» открывает «Мою статистику»: там ребёнок
// зовёт родителя вводом пароля, значок сам пароль не принимает.
func panelAction(b PanelButton) (panelAct, string) {
	switch {
	case b.Disabled:
		return actExplain, b.DisabledNote
	case b.Kind == PanelRefresh:
		return actRefresh, ""
	case b.Kind == PanelSpendBonus:
		return actSpend, ""
	}
	k := b.Kind
	if k == PanelAskMore {
		k = PanelMyStats
	}
	if kind, ok := k.OpenKind(); ok {
		return actOpen, kind
	}
	return actNone, ""
}

// cardPressButton — какая кнопка мини-панели стоит за кнопкой табло: стрелка открывает «Мою
// статистику», «Обновить» повторяет запрос.
func cardPressButton(t CardTarget) (PanelButton, bool) {
	switch t {
	case TargetArrow:
		return PanelButton{Kind: PanelMyStats}, true
	case TargetRefresh:
		return PanelButton{Kind: PanelRefresh}, true
	}
	return PanelButton{}, false
}

// spendBonusMax — сколько минут значок берёт из копилки за одно нажатие без пароля (FR-11).
const spendBonusMax = 30

// spendMinutes — сколько взять из копилки: 30 минут или весь остаток, если его меньше.
func spendMinutes(balance int) int {
	if balance > 0 && balance < spendBonusMax {
		return balance
	}
	return spendBonusMax
}

// serviceText — человеческий текст отказа службы; пусто, если ошибка не от службы (обрыв
// канала: технический текст ребёнку не показывается).
func serviceText(err error) string {
	var re *ipc.ReplyError
	if errors.As(err, &re) {
		return re.Text
	}
	return ""
}

// spendResultText — что показать после траты из копилки: итог службы, её отказ или общий текст
// ошибки.
func spendResultText(err error, message string) string {
	if err == nil {
		return message
	}
	if text := serviceText(err); text != "" {
		return text
	}
	return textPanelError
}

// cardViewWith — что показать на табло: по данным службы; если не вышло открыть окно, текст
// отказа с кнопкой «Обновить» (она снимает его). Ошибка службы важнее отказа.
func cardViewWith(s Snapshot, note string) CountdownView {
	v := CountdownFor(s)
	if note != "" && v.Kind != CountdownError {
		return CountdownView{Kind: CountdownError, Title: v.Title, Big: note, Button: textButtonRefresh}
	}
	return v
}

// parseSession читает номер сеанса из ключа --session; ok=false: ключа нет или он не число.
func parseSession(s string) (uint32, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}
