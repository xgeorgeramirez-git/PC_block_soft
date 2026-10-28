package ipc

import (
	"strings"
	"testing"

	"pcblock/internal/core"
)

func TestStatusNameCoversAllCoreStatuses(t *testing.T) {
	want := map[core.Status]string{
		core.StatusOK:        StatusOK,
		core.StatusLow:       StatusLow,
		core.StatusTimeUp:    StatusTimeUp,
		core.StatusBypass:    StatusBypass,
		core.StatusUnlimited: StatusUnlimited,
		core.StatusSafe:      StatusSafe,
	}
	for cs, name := range want {
		if got := StatusName(cs); got != name {
			t.Errorf("статус ядра %d: %q, ждали %q", cs, got, name)
		}
	}
	// Неизвестный статус безопасен: ограничений нет (R18).
	if got := StatusName(core.Status(99)); got != StatusSafe {
		t.Errorf("неизвестный статус: %q", got)
	}
}

func TestChildStatusFromDecision(t *testing.T) {
	cases := []struct {
		name                      string
		d                         core.Decision
		status, lock              string
		label, tooltip, summaryIs string
	}{
		{
			name:   "осталось много",
			d:      core.Decision{Status: core.StatusOK, RemainingS: 85 * 60, LimitS: 120 * 60, UsedS: 35 * 60, Used: "Сегодня: 35 мин из 2 ч", Day: "2026-10-19"},
			status: StatusOK, lock: LockNone,
			label: "85", tooltip: "Осталось 1 ч 25 мин", summaryIs: "Сегодня осталось 1 ч 25 мин из 2 ч",
		},
		{
			name:   "осталось мало, неполная минута округляется вверх",
			d:      core.Decision{Status: core.StatusLow, RemainingS: 61, LimitS: 3600, UsedS: 3539},
			status: StatusLow, lock: LockNone,
			label: "2", tooltip: "Осталось 2 мин", summaryIs: "Сегодня осталось 2 мин из 1 ч",
		},
		{
			name:   "осталось меньше минуты",
			d:      core.Decision{Status: core.StatusLow, RemainingS: 5, LimitS: 3600, UsedS: 3595},
			status: StatusLow, lock: LockNone,
			label: "1", tooltip: "Осталось 1 мин", summaryIs: "Сегодня осталось 1 мин из 1 ч",
		},
		{
			name: "время вышло",
			d: core.Decision{Status: core.StatusTimeUp, Lock: core.LockTimeUp, Reason: core.ReasonTimeUp, Until: "Можно снова: завтра",
				Used: "Сегодня: 2 ч 00 мин из 2 ч", LimitS: 7200, UsedS: 7200},
			status: StatusTimeUp, lock: LockTimeUp,
			label: "0", tooltip: "Время на сегодня закончилось", summaryIs: "Время на сегодня закончилось",
		},
		{
			name: "обход защиты",
			d: core.Decision{Status: core.StatusBypass, Lock: core.LockBypass, Reason: core.ReasonBypass, Detail: "Изменено системное время",
				Until: core.UntilBypass},
			status: StatusBypass, lock: LockBypass,
			label: "!", tooltip: "Попытка обойти защиту", summaryIs: "Попытка обойти защиту",
		},
		{
			name:   "без лимита",
			d:      core.Decision{Status: core.StatusUnlimited, Unlimited: true, UsedS: 600, Used: "Сегодня: 10 мин"},
			status: StatusUnlimited, lock: LockNone,
			label: "∞", tooltip: "Сегодня без ограничений по времени", summaryIs: "Сегодня без ограничений по времени",
		},
		{
			name:   "правила не прочитаны",
			d:      core.Decision{Status: core.StatusSafe, Unlimited: true},
			status: StatusSafe, lock: LockNone,
			label: "?", tooltip: "Ограничения сейчас не действуют", summaryIs: "Ограничения сейчас не действуют",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := ChildStatusFrom("Миша", c.d)
			if s.Name != "Миша" || s.Status != c.status || s.Lock != c.lock {
				t.Fatalf("имя/статус/блокировка: %+v", s)
			}
			if s.Label != c.label || s.Tooltip != c.tooltip || s.Summary != c.summaryIs {
				t.Fatalf("надпись %q, подсказка %q, строка %q", s.Label, s.Tooltip, s.Summary)
			}
			if s.RemainingS != c.d.RemainingS || s.LimitS != c.d.LimitS || s.UsedS != c.d.UsedS || s.Unlimited != c.d.Unlimited ||
				s.Reason != c.d.Reason || s.Detail != c.d.Detail || s.Until != c.d.Until || s.Used != c.d.Used || s.Day != c.d.Day {
				t.Fatalf("поля решения ядра должны копироваться без пересчёта: %+v", s)
			}
			if s.StatusText == "" {
				t.Fatalf("нет слова статуса для карточки родителя")
			}
		})
	}
}

func TestChildStatusAdminAndNotChild(t *testing.T) {
	a := ChildStatusAdmin("Оля")
	if a.Status != StatusAdmin || a.Lock != LockNone || a.Name != "Оля" || a.Tooltip != "Администратор: ограничения не действуют" || a.Label != "" {
		t.Fatalf("admin: %+v", a)
	}
	n := ChildStatusNotChild()
	if n.Status != StatusNotChild || n.Lock != LockNone || n.Label != "" || n.Tooltip != "" {
		t.Fatalf("notChild: %+v", n)
	}
}

func TestStatusTextsHaveNoLongDashes(t *testing.T) {
	var all []ChildStatus
	for _, st := range []core.Status{core.StatusOK, core.StatusLow, core.StatusTimeUp, core.StatusBypass, core.StatusUnlimited, core.StatusSafe} {
		all = append(all, ChildStatusFrom("A", core.Decision{Status: st, RemainingS: 600, LimitS: 3600}))
	}
	all = append(all, ChildStatusAdmin("A"), ChildStatusNotChild())
	for _, s := range all {
		for _, txt := range []string{s.StatusText, s.Label, s.Tooltip, s.Summary} {
			if strings.ContainsAny(txt, "—–") {
				t.Errorf("длинное тире в тексте %q", txt)
			}
		}
	}
}

func TestCategoryNames(t *testing.T) {
	for _, c := range []core.Category{core.CatOther, core.CatGames, core.CatVideoSocial, core.CatStudy} {
		if got := CategoryFromName(CategoryName(c)); got != c {
			t.Errorf("категория %d: имя %q вернулось как %d", c, CategoryName(c), got)
		}
	}
	// Мусор от стража — «Прочее», не паника и не «Игры».
	for _, name := range []string{"", "unknown", "GAMES", "<script>"} {
		if got := CategoryFromName(name); got != core.CatOther {
			t.Errorf("имя %q дало категорию %d", name, got)
		}
	}
	if got := CategoryName(core.Category(77)); got != CatNameOther {
		t.Errorf("неизвестная категория: %q", got)
	}
}

func TestSensorSampleRoundTrip(t *testing.T) {
	in := core.SessionSample{AtMs: 1_760_000_000_123, TzOffsetMin: 180, LastInputAgoMs: 2500, Fullscreen: true, Foreground: core.CatGames, SessionActive: true}
	dto := SampleFromCore(in)
	if dto.AtMs != in.AtMs || dto.Foreground != CatNameGames || !dto.SessionActive || !dto.Fullscreen {
		t.Fatalf("dto: %+v", dto)
	}
	if got := dto.ToCore(); got != in {
		t.Fatalf("после круга: %+v, было %+v", got, in)
	}
}

func TestSensorSampleJSONKeys(t *testing.T) {
	s := string(OK(SampleFromCore(core.SessionSample{AtMs: 1, SessionActive: true})).Payload)
	for _, key := range []string{`"atMs":1`, `"sessionActive":true`, `"lastInputAgoMs":0`, `"fullscreen":false`, `"foreground":"other"`, `"tzOffsetMin":0`} {
		if !strings.Contains(s, key) {
			t.Errorf("в %s нет %s", s, key)
		}
	}
}

// Ловит: нет текста за 30 минут, звук не на всех четырёх порогах (D-55), вид события и секунды до
// него не доходят до стража (R-3: плитки табло считают до события).
func TestGuardWarningTexts(t *testing.T) {
	cases := []struct {
		th   int
		ev   core.WarnEvent
		text string
		name string
	}{
		{30, core.WarnLimit, "Через 30 минут время на сегодня закончится", WarnEventLimit},
		{15, core.WarnLimit, "Через 15 минут время на сегодня закончится", WarnEventLimit},
		{5, core.WarnLimit, "Через 5 минут время закончится. Сохрани игру", WarnEventLimit},
		{1, core.WarnLimit, "Осталась 1 минута", WarnEventLimit},
		{30, core.WarnForbidden, "Через 30 минут начнутся запретные часы", WarnEventForbidden},
		{15, core.WarnForbidden, "Через 15 минут начнутся запретные часы", WarnEventForbidden},
		{5, core.WarnForbidden, "Через 5 минут начнутся запретные часы", WarnEventForbidden},
		{1, core.WarnForbidden, "Через минуту начнутся запретные часы", WarnEventForbidden},
		{30, core.WarnPauseEnd, "Через 30 минут закончится пауза", WarnEventPause},
		{15, core.WarnPauseEnd, "Через 15 минут закончится пауза", WarnEventPause},
		{5, core.WarnPauseEnd, "Через 5 минут закончится пауза", WarnEventPause},
		{1, core.WarnPauseEnd, "Через минуту закончится пауза", WarnEventPause},
	}
	for _, c := range cases {
		w := NewGuardWarningFor(c.th, c.ev, c.th*60)
		if w.Threshold != c.th || w.Text != c.text || !w.Sound || w.Event != c.name || w.EventS != c.th*60 {
			t.Errorf("порог %d %v: %+v", c.th, c.ev, w)
		}
	}
	if w := NewGuardWarning(3); w.Threshold != 3 || w.Text != "Через 3 мин время закончится" || w.Event != WarnEventLimit || w.EventS != 180 {
		t.Errorf("нестандартный порог: %+v", w)
	}
}
