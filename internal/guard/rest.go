package guard

import (
	"strings"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/systray"
)

// Перерыв на отдых у стража (FB-14, D-62, R27): плашка «ПОРА ОТДОХНУТЬ» тем же кодом табло, что
// предупреждения 30/15/5 (D-55), и экран перерыва — вариант экрана блокировки (8.2.3 п.12):
// картинка, таймер плитками по часам, «Можно снова в …», сообщение родителя не длиннее двух
// строк. Без «Попросить ещё», копилки, промокода и автосмены пользователя. Решения здесь, окно
// Windows только рисует (PL-23).

const (
	textRestBoardTitle = "ПОРА ОТДОХНУТЬ"
	textEndBreak       = "Закончить перерыв"
	restMessageLines   = 2 // сообщение родителя на экране перерыва: не больше двух строк
	ellipsis           = "…"
)

// RestView — то, чего нет у обычного экрана блокировки: таймер плитками до конца перерыва
// («00», «14», «59»; секунды идут вживую) и сообщение родителя (перенос — WrapMessage в окне).
type RestView struct {
	Tiles   [3]string // часы, минуты, секунды двумя цифрами
	Message string
}

// isRest: показан экран перерыва (или решение о нём).
func isRest(d core.Decision) bool { return d.Lock == core.LockRest }

// restTiles — плитки таймера: тот же расчёт пар цифр, что табло остатка (systray.TimeCountdown).
func restTiles(secs int) [3]string {
	b := systray.TimeCountdown(max(secs, 0))
	return [3]string{b.Hours, b.Minutes, b.Seconds}
}

// restSecs — секунды до конца перерыва сейчас: по ответу службы и настоящим часам с ускорением
// тестового режима (как запретные часы на табло).
func (m *Model) restSecs(now time.Time) int {
	return max(secondsCeil(m.restDeadline.Sub(now)*time.Duration(m.speed)), 0)
}

// splitRest отделяет плашки отдыха от предупреждений R9: у них свои минуты (1–15), и порогом
// 30/15/5/1 они не выбираются.
func splitRest(ws []Incoming) (rest *Incoming, others []Incoming) {
	others = make([]Incoming, 0, len(ws))
	for i := range ws {
		if ws[i].Event == ipc.WarnEventRest {
			rest = &ws[i]
			continue
		}
		others = append(others, ws[i])
	}
	return rest, others
}

// restPlate ставит плашку «ПОРА ОТДОХНУТЬ», если служба прислала её впервые (повтор того же
// момента начала перерыва — та же плашка, без второго звука). Старшинство (Ruling FB14-9):
// предупреждение о конце времени, запретных часах или паузе и «Добавлено N минут» важнее —
// плашка отдыха их не заменяет, а ждёт (restWait), пока они не закроются; плашка 1 мин срока
// не имеет и уходит, только когда порог снят (добавили время) или её сменил экран. Видимая
// плашка сайта (RM4-9) тоже важнее: отдых ждёт, пока не выйдет её срок.
func (m *Model) restPlate(now time.Time, w Incoming) {
	deadline := now.Add(time.Duration(w.EventS) * time.Second / time.Duration(m.speed))
	if d := deadline.Sub(m.lastRest); !m.lastRest.IsZero() && d > -3*time.Second && d < 3*time.Second {
		return
	}
	if m.warn != nil && m.warn.kind != plateRest || m.sitePlateShown(now) {
		m.restWait = &restWait{text: w.Text, threshold: w.Threshold, deadline: deadline}
		return
	}
	m.showRest(now, w.Text, w.Threshold, deadline)
}

// restWait — плашка отдыха, которая ждёт, пока закроется более важная.
type restWait struct {
	text      string
	threshold int
	deadline  time.Time // начало перерыва по настоящим часам
}

func (m *Model) showRest(now time.Time, text string, threshold int, deadline time.Time) {
	m.restWait = nil
	m.lastRest = deadline
	m.newPlate(now, plateRest, threshold, 0)
	m.warn.text, m.warn.event, m.warn.deadline = text, ipc.WarnEventRest, deadline
}

// flushRest показывает ждущую плашку отдыха, когда более важной плашки (предупреждения или
// видимой плашки сайта) уже нет. Перерыв уже
// начался или пауза — ждущая плашка отбрасывается (её сменит экран, пауза снимает и её).
func (m *Model) flushRest(now time.Time) {
	r := m.restWait
	switch {
	case r == nil:
		return
	case !now.Before(r.deadline) || m.dec.Status == core.StatusPaused:
		m.restWait = nil
	case m.warn == nil && !m.sitePlateShown(now):
		m.showRest(now, r.text, r.threshold, r.deadline)
	}
}

// restBoard — табло плашки отдыха: плитки до начала перерыва, заголовок «ПОРА ОТДОХНУТЬ»,
// спокойный цвет (это не конец времени).
func restBoard(secs int, line string) *systray.CountdownView {
	b := systray.TimeCountdown(secs)
	b.Title, b.Status, b.Line = textRestBoardTitle, systray.StatusNormal, line
	return &b
}

// WrapMessage переносит сообщение по словам не больше чем на maxLines строк; fits говорит,
// помещается ли строка по ширине (меряет окно). Слово шире строки рвётся по буквам; не
// поместившийся хвост — многоточие в конце последней строки. Пустое сообщение — нет строк.
func WrapMessage(text string, maxLines int, fits func(string) bool) []string {
	words := strings.Fields(text)
	lines := []string{}
	for len(words) > 0 && len(lines) < maxLines {
		if len(lines) == maxLines-1 {
			// Последняя строка: всё оставшееся, если помещается, иначе с многоточием.
			if rest := strings.Join(words, " "); fits(rest) {
				return append(lines, rest)
			}
			return append(lines, withEllipsis(words, fits))
		}
		line, used := fillLine(words, fits)
		lines = append(lines, line)
		words = words[used:]
	}
	return lines
}

// fillLine набирает строку из слов words; used — сколько слов ушло целиком. Первое слово шире
// строки рвётся: в строку его начало, остаток становится первым словом (used = 0, words[0] меняется).
func fillLine(words []string, fits func(string) bool) (string, int) {
	if !fits(words[0]) {
		head := longestFit(words[0], "", fits)
		words[0] = strings.TrimPrefix(words[0], head)
		return head, 0
	}
	line, used := words[0], 1
	for used < len(words) && fits(line+" "+words[used]) {
		line += " " + words[used]
		used++
	}
	return line, used
}

// withEllipsis — последняя строка с многоточием: сколько слов помещается вместе с «…», а если
// не помещается и первое, — его начало по буквам.
func withEllipsis(words []string, fits func(string) bool) string {
	line := ""
	for _, w := range words {
		next := strings.TrimSpace(line + " " + w)
		if !fits(next + ellipsis) {
			break
		}
		line = next
	}
	if line == "" {
		line = longestFit(words[0], ellipsis, fits)
	}
	return line + ellipsis
}

// longestFit — самое длинное начало слова w, которое помещается вместе с suffix (хотя бы одна буква).
func longestFit(w, suffix string, fits func(string) bool) string {
	r := []rune(w)
	n := 1
	for n < len(r) && fits(string(r[:n+1])+suffix) {
		n++
	}
	return string(r[:n])
}
