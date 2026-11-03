package guard

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Перерыв на отдых у стража (FB-14, R27, 8.2.2 п.7, 8.2.3 п.12): плашка «ПОРА ОТДОХНУТЬ» общим
// кодом табло, экран перерыва — вариант экрана блокировки. Ожидания — литералы.

func restDecision(leftS int) core.Decision {
	return core.Decision{
		Day: "2026-10-19", Lock: core.LockRest, Reason: "Перерыв на отдых", Until: "Можно снова в 16:20",
		Used: "Сегодня: 1 ч 05 мин из 3 ч", RemainingS: 115 * 60, LimitS: 180 * 60, Status: core.StatusOK,
		RestLeftS: leftS, RestMessage: "Пойди погуляй с собакой",
	}
}

func restWarn() Incoming {
	return Incoming{Threshold: 5, Text: "Через 5 минут перерыв на 15 минут. Сохрани игру", Event: ipc.WarnEventRest, EventS: 300}
}

func labelsOf(bs []ButtonView) []string {
	out := []string{}
	for _, b := range bs {
		out = append(out, b.Label)
	}
	return out
}

// Ловит: плашка отдыха не показывается (порог 5 не из списка 30/15/5/1 у других минут), не табло
// с заголовком «ПОРА ОТДОХНУТЬ», без звука, без крестика; плитки стоят, а не идут по часам.
func TestRestPlateBoard(t *testing.T) {
	m := NewModel()
	w := restWarn()
	w.Threshold, w.EventS = 3, 180 // «предупредить за» 3 мин — не порог R9, плашка всё равно нужна
	m.OnWarnings(t0, runningDecision(5000), false, []Incoming{w})
	v := m.View(at(2 * time.Second))
	p := v.Warning
	if p == nil || p.Board == nil || p.Kind != "rest" || !p.Sound || p.Board.Title != "ПОРА ОТДОХНУТЬ" ||
		p.Board.Line != "Через 5 минут перерыв на 15 минут. Сохрани игру" {
		t.Fatalf("плашка %+v (табло %+v)", p, p.Board)
	}
	if got := p.Board.Hours + ":" + p.Board.Minutes + ":" + p.Board.Seconds; got != "00:02:58" {
		t.Fatalf("плитки через 2 с: %s, want 00:02:58", got)
	}
	if !slices.Equal(labelsOf(p.Buttons), []string{"Закрыть"}) {
		t.Fatalf("кнопки плашки %v", labelsOf(p.Buttons))
	}
	m.Press(at(3*time.Second), ButtonWarningClose, "")
	if v := m.View(at(3 * time.Second)); v.Warning != nil {
		t.Fatal("крестик не закрыл плашку отдыха")
	}
}

// Ловит: плашка отдыха не закрывается сама через 5 с; повтор того же предупреждения даёт вторую
// плашку и звук; пауза не снимает плашку.
func TestRestPlateLifeAndDuplicate(t *testing.T) {
	m := NewModel()
	m.OnWarnings(t0, runningDecision(5000), false, []Incoming{restWarn()})
	id := m.View(t0).Warning.ID
	m.OnWarnings(at(time.Second), runningDecision(5000), false, []Incoming{restWarn()})
	if v := m.View(at(time.Second)); v.Warning == nil || v.Warning.ID != id {
		t.Fatalf("повтор дал новую плашку: %+v", v.Warning)
	}
	m.Tick(at(5 * time.Second))
	if v := m.View(at(5 * time.Second)); v.Warning != nil {
		t.Fatal("плашка отдыха не закрылась через 5 с")
	}
	m2 := NewModel()
	m2.OnWarnings(t0, runningDecision(5000), false, []Incoming{restWarn()})
	paused := runningDecision(5000)
	paused.Status = core.StatusPaused
	m2.OnWarnings(at(time.Second), paused, false, nil)
	if v := m2.View(at(time.Second)); v.Warning != nil {
		t.Fatal("пауза не сняла плашку отдыха")
	}
}

// Ловит: предупреждение о лимите в том же опросе теряется из-за плашки отдыха.
func TestRestPlateYieldsToLimit(t *testing.T) {
	m := NewModel()
	m.OnWarnings(t0, runningDecision(300), false, []Incoming{restWarn(), {Threshold: 5, Text: "Через 5 минут время закончится. Сохрани игру", Event: ipc.WarnEventLimit, EventS: 300}})
	if v := m.View(t0); v.Warning == nil || v.Warning.Kind != ipc.PlateBoard || v.Warning.Board.Title == "ПОРА ОТДОХНУТЬ" {
		t.Fatalf("плашка %+v, want табло лимита", v.Warning)
	}
}

// Ловит: на экране перерыва есть «Попросить ещё», копилка или промокод; нет «Пароль родителя»
// и выходов; подсказка «Попроси родителя…» и строка «Сегодня…» видны; нет таймера и сообщения.
func TestRestScreenView(t *testing.T) {
	d := restDecision(899)
	d.BonusBalance = 45
	m := locked(d)
	v := m.View(at(time.Second))
	if !v.LockShown || v.Lock.Reason != "Перерыв на отдых" || v.Lock.Until != "Можно снова в 16:20" || v.Lock.Used != "" || v.Lock.Hint != "" {
		t.Fatalf("тексты экрана %+v", v.Lock)
	}
	want := []string{"Пароль родителя", "Выключить компьютер", "Выйти из учётной записи", "Смена пользователя"}
	if got := labelsOf(v.Buttons); !slices.Equal(got, want) {
		t.Fatalf("кнопки %v, want %v", got, want)
	}
	if v.Rest == nil || v.Rest.Message != "Пойди погуляй с собакой" || v.Rest.Tiles != [3]string{"00", "14", "58"} {
		t.Fatalf("перерыв %+v", v.Rest)
	}
	for _, b := range []Button{ButtonAskMore, ButtonSpendBonus, ButtonPromo} {
		if a := m.Press(at(time.Second), b, ""); a.Kind != ActNone || m.View(at(time.Second)).Panel != PanelNone {
			t.Errorf("кнопка %v на экране перерыва сработала: %+v", b, a)
		}
	}
	if v := locked(timeUpDecision()).View(t0); v.Rest != nil {
		t.Fatalf("у экрана «время вышло» вид перерыва %+v", v.Rest)
	}
}

// Ловит: таймер перерыва стоит между ответами службы или идёт не в темпе часов ядра (тестовый режим).
func TestRestTimerLive(t *testing.T) {
	m := locked(restDecision(900))
	if got := m.View(at(3 * time.Second)).Rest.Tiles; got != [3]string{"00", "14", "57"} {
		t.Fatalf("через 3 с %v, want 00 14 57", got)
	}
	m2 := NewModel()
	m2.SetClockSpeed(10)
	m2.OnDecision(t0, restDecision(900), false, nil)
	if got := m2.View(at(3 * time.Second)).Rest.Tiles; got != [3]string{"00", "14", "30"} {
		t.Fatalf("ускорение 10: через 3 с %v, want 00 14 30", got)
	}
	m.OnDecision(at(901*time.Second), restDecision(0), false, nil) // служба ещё не сняла экран
	if got := m.View(at(903 * time.Second)).Rest.Tiles; got != [3]string{"00", "00", "00"} {
		t.Fatalf("после конца %v, want нули", got)
	}
}

// Ловит: после пароля на экране перерыва нет «Закончить перерыв» или есть «+15»; кнопка не
// отдаёт действие ActEndBreak; на других экранах она работает.
func TestRestEndBreakChoice(t *testing.T) {
	m := withChoices(t, restDecision(600))
	if got := labelsOf(m.View(t0).Choices); !slices.Equal(got, []string{"Закончить перерыв", "Открыть настройки", "Отмена"}) {
		t.Fatalf("выбор после пароля %v", got)
	}
	if a := m.Press(t0, ButtonAdd15, ""); a.Kind != ActNone {
		t.Fatalf("+15 на экране перерыва: %+v", a)
	}
	if a := m.Press(t0, ButtonEndBreak, ""); a.Kind != ActEndBreak {
		t.Fatalf("«Закончить перерыв»: %+v", a)
	}
	m2 := withChoices(t, timeUpDecision())
	if a := m2.Press(t0, ButtonEndBreak, ""); a.Kind != ActNone {
		t.Fatalf("«Закончить перерыв» на экране «время вышло»: %+v", a)
	}
}

// Ловит: автоматическая смена пользователя выполняется на экране перерыва (8.2.3 п.12); после
// перерыва экран «время вышло» сменяет пользователя сразу по сроку, отсчитанному от начала перерыва.
func TestRestNoAutoSwitch(t *testing.T) {
	m := NewModel()
	m.SetAutoSwitchAfter(time.Second)
	m.OnDecision(t0, restDecision(600), false, nil)
	if a := m.AutoSwitch(at(time.Second)); a.Kind != ActNone {
		t.Fatalf("автосмена на экране перерыва: %+v", a)
	}
	m2 := NewModel()
	m2.SetAutoSwitchAfter(time.Minute)
	keepAlive(m2, restDecision(600), 0, 120)
	m2.OnDecision(at(121*time.Second), timeUpDecision(), false, nil)
	if a := m2.AutoSwitch(at(121 * time.Second)); a.Kind != ActNone {
		t.Fatalf("автосмена сразу после перерыва: %+v", a)
	}
	keepAlive(m2, timeUpDecision(), 122, 181)
	if a := m2.AutoSwitch(at(181 * time.Second)); a.Kind != ActSwitchUser {
		t.Fatalf("через минуту экрана «время вышло»: %+v, want смена пользователя", a)
	}
}

// Ловит: сообщение родителя не переносится по словам, длиннее двух строк, длинное слово вылезает
// за край, обрезка без многоточия, пустое и пробелы — не одна строка.
func TestWrapMessage(t *testing.T) {
	fits := func(s string) bool { return utf8.RuneCountInString(s) <= 12 } // 12 символов в строке
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"Пойди погуляй с собакой", []string{"Пойди", "погуляй с…"}},
		{"Разомнись", []string{"Разомнись"}},
		{"Раз два три четыре", []string{"Раз два три", "четыре"}},
		{"Оченьдлинноесловобезпробелов", []string{"Оченьдлинное", "словобезпро…"}},
		{"раз два три четыре пять шесть семь", []string{"раз два три", "четыре пять…"}},
		{"   ", []string{}},
	} {
		got := WrapMessage(tc.in, 2, fits)
		if !slices.Equal(got, tc.want) {
			t.Errorf("WrapMessage(%q) = %q, want %q", tc.in, got, tc.want)
		}
		for _, l := range got {
			if !fits(l) {
				t.Errorf("строка %q не помещается", l)
			}
			if strings.TrimSpace(l) != l {
				t.Errorf("пробелы по краям в %q", l)
			}
		}
	}
}
