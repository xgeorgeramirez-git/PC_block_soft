package systray

import (
	"strings"
	"time"

	"pcblock/internal/ipc"
)

// CardPollEvery — как часто значок спрашивает службу, пока табло открыто: секунды на плитках
// «01 : 23 : 45» уменьшаются вживую по данным службы (ТЗ 8.2.1 п.3). Закрытое табло опрашивает
// служба с периодом PollEvery.
const CardPollEvery = time.Second

// PollInterval — период опроса службы: раз в секунду, пока карточка открыта, иначе PollEvery.
func PollInterval(cardOpen bool) time.Duration {
	if cardOpen {
		return CardPollEvery
	}
	return PollEvery
}

// Тексты табло (D-53, ТЗ 8.2.1 п.3-6). Ребёнку на «ты», длинных тире нет (NFR-11).
const (
	textCountdownTitle     = "ОСТАЛОСЬ ВРЕМЕНИ:"
	textCountdownTimeUp    = "ВРЕМЯ ВЫШЛО"
	textCountdownForbidden = "СЕЙЧАС НЕЛЬЗЯ"
	textCountdownError     = "Не удалось получить данные. Попробуй через минуту"
	textCountdownPaused    = "Пауза. Ограничения не действуют"
	textCountdownSafe      = "Ограничения сейчас не действуют"
	textCountdownAdmin     = "Администратор: ограничения не действуют"
	textCountdownNoTime    = "Пока нельзя пользоваться компьютером"

	// Подписи под парами плиток.
	textLabelHours   = "часы"
	textLabelMinutes = "минуты"
	textLabelSeconds = "секунды"
)

// maxCountdownHours — предел часов на двух плитках.
const maxCountdownHours = 99

// CountdownKind — что стоит в основной части карточки.
type CountdownKind int

const (
	CountdownTime    CountdownKind = iota // шесть плиток «часы : минуты : секунды»
	CountdownMessage                      // одна крупная строка вместо плиток (ТЗ 8.2.1 п.5)
	CountdownError                        // нет свежих данных: текст и кнопка «Обновить» (п.6)
)

// CountdownView — что нарисовать на табло. Окно только рисует это; решения принимает
// CountdownFor (проверяется тестами на Linux).
type CountdownView struct {
	Kind    CountdownKind
	Title   string // заглавными: «ОСТАЛОСЬ ВРЕМЕНИ:», «ВРЕМЯ ВЫШЛО», «СЕЙЧАС НЕЛЬЗЯ»
	Hours   string // две цифры; пусто, если плиток нет
	Minutes string
	Seconds string
	Big     string // крупная строка особого состояния или текст ошибки
	Status  Status // цвет цифр: обычный, янтарный, красный (ТЗ 8.0 п.3)
	Button  string // подпись кнопки под ошибкой; пусто, если кнопки нет
	// Line — строка под плитками у плашки-табло предупреждения стража («Через 5 минут время
	// закончится. Сохрани игру», FR-9); у табло значка пусто.
	Line string
}

// UsesCountdown: табло показывается в детском сеансе и пока ответа службы нет; взрослому
// по значку открывается мини-панель с «Настройки для родителя» (ТЗ 8.2.1 п.7).
func UsesCountdown(s Snapshot) bool {
	return !(s.Has && s.Status.Status == ipc.StatusNotChild)
}

func two(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

// CountdownFor собирает табло из знания значка о ребёнке.
func CountdownFor(s Snapshot) CountdownView {
	if s.Failed || !s.Has || !s.Fresh {
		return CountdownView{Kind: CountdownError, Title: textCountdownTitle, Big: textCountdownError, Button: textButtonRefresh}
	}
	st := s.Status
	msg := func(title, big string, status Status) CountdownView {
		return CountdownView{Kind: CountdownMessage, Title: title, Big: big, Status: status}
	}
	switch st.Status {
	case ipc.StatusForbidden:
		return msg(textCountdownForbidden, canAgainText(st.Until), StatusOut)
	case ipc.StatusPaused:
		big := textCountdownPaused
		if st.PauseUntil != "" {
			big = "Пауза до " + st.PauseUntil + ". Ограничения не действуют"
		}
		return msg(textCountdownTitle, big, StatusPaused)
	case ipc.StatusSafe:
		return msg(textCountdownTitle, textCountdownSafe, StatusPaused)
	case ipc.StatusAdmin:
		return msg(textCountdownTitle, textCountdownAdmin, StatusPaused)
	}
	locked := isLocked(st)
	if (st.Unlimited || st.Status == ipc.StatusUnlimited) && !locked {
		return msg(textCountdownTitle, tooltipUnlimited, StatusNormal)
	}
	rem := st.RemainingS
	if locked {
		rem = 0
	}
	return TimeCountdown(rem)
}

// TimeCountdown — шесть плиток «часы : минуты : секунды» для rem секунд и цвет цифр по единому
// правилу значка. Один код для табло у значка и плашки-табло предупреждения стража (R-2):
// цифры и цвета считаются одинаково. 0 и меньше — «ВРЕМЯ ВЫШЛО» и красные нули.
func TimeCountdown(rem int) CountdownView {
	if rem < 0 {
		rem = 0
	}
	v := CountdownView{Kind: CountdownTime, Title: textCountdownTitle}
	// Цвет по единому правилу значка (минуты округляются вверх): граница 15 минут одна.
	v.Status = TrayStatus(ceilMin(rem), false, false, false)
	if v.Status == StatusOut {
		v.Title = textCountdownTimeUp
	}
	h := rem / 3600
	m, sec := rem%3600/60, rem%60
	if h > maxCountdownHours {
		h, m, sec = maxCountdownHours, 59, 59
	}
	v.Hours, v.Minutes, v.Seconds = two(h), two(m), two(sec)
	return v
}

// canAgainText превращает «Можно снова: сегодня в 07:00» службы в «Можно снова в 07:00»;
// для другого дня слово дня остаётся («Можно снова завтра в 07:00»). Нет времени: честная
// фраза без часа.
func canAgainText(until string) string {
	const prefix = "Можно снова: "
	rest, ok := strings.CutPrefix(until, prefix)
	if !ok || !strings.Contains(rest, " в ") {
		return textCountdownNoTime
	}
	rest = strings.TrimPrefix(rest, "сегодня ")
	return "Можно снова " + rest
}
