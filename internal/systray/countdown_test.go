package systray

import (
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

func freshSnap(st ipc.ChildStatus) Snapshot {
	return Snapshot{Status: st, Has: true, Fresh: true}
}

// Ловит: плитки табло показывают не то, что осталось: часы, минуты и секунды перепутаны,
// нет ведущих нулей или секунды не берутся из остатка (D-53, ТЗ 8.2.1 п.3: «01 : 23 : 45»).
func TestCountdownDigits(t *testing.T) {
	cases := []struct {
		name       string
		remainingS int
		h, m, s    string
	}{
		{"1 ч 23 мин 45 с", 5025, "01", "23", "45"},
		{"ровно час", 3600, "01", "00", "00"},
		{"одна секунда", 1, "00", "00", "01"},
		{"59 минут 59 секунд", 3599, "00", "59", "59"},
		{"десять часов", 36000, "10", "00", "00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := CountdownFor(freshSnap(okStatus(c.remainingS, 36000)))
			if v.Kind != CountdownTime || v.Title != "ОСТАЛОСЬ ВРЕМЕНИ:" {
				t.Fatalf("вид %v, заголовок %q", v.Kind, v.Title)
			}
			if v.Hours != c.h || v.Minutes != c.m || v.Seconds != c.s {
				t.Fatalf("получили %s:%s:%s, ждали %s:%s:%s", v.Hours, v.Minutes, v.Seconds, c.h, c.m, c.s)
			}
		})
	}
}

// Ловит: больше 99 часов вылезает за две плитки (остаток в сутки и больше бывает только от
// ошибки данных, как и у значка).
func TestCountdownHoursClamped(t *testing.T) {
	v := CountdownFor(freshSnap(okStatus(100*3600+61, 400000)))
	if v.Hours != "99" || v.Minutes != "59" || v.Seconds != "59" {
		t.Fatalf("получили %s:%s:%s, ждали 99:59:59", v.Hours, v.Minutes, v.Seconds)
	}
}

// Ловит: цвет цифр расходится со значком на границе 15 минут (ТЗ 8.0 п.3): 900 с янтарный,
// 901 с обычный, время вышло красный.
func TestCountdownStatusBorder(t *testing.T) {
	cases := []struct {
		remainingS int
		want       Status
	}{
		{901, StatusNormal},
		{900, StatusLow},
		{1, StatusLow},
	}
	for _, c := range cases {
		v := CountdownFor(freshSnap(okStatus(c.remainingS, 7200)))
		if v.Status != c.want {
			t.Errorf("%d с: статус %v, ждали %v", c.remainingS, v.Status, c.want)
		}
		if got := DisplayFor(okStatus(c.remainingS, 7200)).Status; got != v.Status {
			t.Errorf("%d с: табло %v, значок %v: цвета разошлись", c.remainingS, v.Status, got)
		}
	}
}

// Ловит: при исчерпанном времени или блокировке табло показывает остаток или не красный
// «ВРЕМЯ ВЫШЛО» (ТЗ 8.2.1 п.4).
func TestCountdownTimeUp(t *testing.T) {
	timeUp := ipc.ChildStatusFrom("Миша", core.Decision{
		Status: core.StatusTimeUp, Lock: core.LockTimeUp, Reason: core.ReasonTimeUp, LimitS: 7200,
	})
	bypass := ipc.ChildStatusFrom("Миша", core.Decision{
		Status: core.StatusBypass, Lock: core.LockBypass, Reason: core.ReasonBypass, RemainingS: 3000, LimitS: 7200,
	})
	okZero := okStatus(0, 7200)
	okZero.Status = ipc.StatusOK // служба ещё не успела сменить статус, остаток уже 0
	for name, st := range map[string]ipc.ChildStatus{"время вышло": timeUp, "блокировка за обход": bypass, "остаток 0 при статусе ok": okZero} {
		v := CountdownFor(freshSnap(st))
		if v.Kind != CountdownTime || v.Title != "ВРЕМЯ ВЫШЛО" || v.Status != StatusOut {
			t.Errorf("%s: вид %v, заголовок %q, статус %v", name, v.Kind, v.Title, v.Status)
		}
		if v.Hours != "00" || v.Minutes != "00" || v.Seconds != "00" {
			t.Errorf("%s: цифры %s:%s:%s, ждали нули", name, v.Hours, v.Minutes, v.Seconds)
		}
	}
}

// Ловит: особые состояния не по ТЗ 8.2.1 п.5: вместо одной крупной строки остаются плитки
// или в строке не те слова.
func TestCountdownSpecialStates(t *testing.T) {
	unlimited := ipc.ChildStatusFrom("Миша", core.Decision{Status: core.StatusUnlimited, Unlimited: true})
	paused := ipc.ChildStatusFrom("Миша", core.Decision{Status: core.StatusPaused, RemainingS: 3000, LimitS: 7200, PauseUntil: "18:30"})
	pausedNoEnd := ipc.ChildStatusFrom("Миша", core.Decision{Status: core.StatusPaused, RemainingS: 3000, LimitS: 7200})
	forbiddenToday := ipc.ChildStatusFrom("Миша", core.Decision{
		Status: core.StatusForbidden, Lock: core.LockForbidden, Reason: core.ReasonForbidden, Until: "Можно снова: сегодня в 07:00",
	})
	forbiddenTomorrow := ipc.ChildStatusFrom("Миша", core.Decision{
		Status: core.StatusForbidden, Lock: core.LockForbidden, Reason: core.ReasonForbidden, Until: "Можно снова: завтра в 07:00",
	})
	forbiddenNoTime := ipc.ChildStatusFrom("Миша", core.Decision{
		Status: core.StatusForbidden, Lock: core.LockForbidden, Reason: core.ReasonForbidden,
	})
	safe := ipc.ChildStatusFrom("Миша", core.Decision{Status: core.StatusSafe, Unlimited: true})
	cases := []struct {
		name   string
		st     ipc.ChildStatus
		title  string
		big    string
		status Status
	}{
		{"без лимита", unlimited, "ОСТАЛОСЬ ВРЕМЕНИ:", "Сегодня без ограничений по времени", StatusNormal},
		{"пауза до 18:30", paused, "ОСТАЛОСЬ ВРЕМЕНИ:", "Пауза до 18:30. Ограничения не действуют", StatusPaused},
		{"пауза без времени конца", pausedNoEnd, "ОСТАЛОСЬ ВРЕМЕНИ:", "Пауза. Ограничения не действуют", StatusPaused},
		{"запретные часы, можно сегодня", forbiddenToday, "СЕЙЧАС НЕЛЬЗЯ", "Можно снова в 07:00", StatusOut},
		{"запретные часы, можно завтра", forbiddenTomorrow, "СЕЙЧАС НЕЛЬЗЯ", "Можно снова завтра в 07:00", StatusOut},
		{"запретные часы без времени", forbiddenNoTime, "СЕЙЧАС НЕЛЬЗЯ", "Пока нельзя пользоваться компьютером", StatusOut},
		{"правила не прочитаны", safe, "ОСТАЛОСЬ ВРЕМЕНИ:", "Ограничения сейчас не действуют", StatusPaused},
		{"администратор", ipc.ChildStatusAdmin("Миша"), "ОСТАЛОСЬ ВРЕМЕНИ:", "Администратор: ограничения не действуют", StatusPaused},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := CountdownFor(freshSnap(c.st))
			if v.Kind != CountdownMessage || v.Title != c.title || v.Big != c.big || v.Status != c.status {
				t.Fatalf("получили {%v %q %q %v}, ждали {Message %q %q %v}", v.Kind, v.Title, v.Big, v.Status, c.title, c.big, c.status)
			}
			if v.Hours != "" || v.Minutes != "" || v.Seconds != "" {
				t.Fatalf("у особого состояния плиток нет, а цифры %q %q %q", v.Hours, v.Minutes, v.Seconds)
			}
		})
	}
}

// Ловит: нет свежих данных, а табло показывает старые цифры («осталось 85» полчаса спустя
// обрыва связи было бы неправдой) или не даёт кнопку «Обновить» (ТЗ 8.2.1 п.6, урок L-114).
func TestCountdownNoData(t *testing.T) {
	good := okStatus(5025, 7200)
	cases := map[string]Snapshot{
		"ответа ещё не было": {},
		"опрос не удался":    {Status: good, Has: true, Failed: true, Fresh: true},
		"ответ устарел":      {Status: good, Has: true, Fresh: false},
	}
	for name, s := range cases {
		v := CountdownFor(s)
		if v.Kind != CountdownError || v.Big != "Не удалось получить данные. Попробуй через минуту" || v.Button != "Обновить" {
			t.Errorf("%s: получили {%v %q %q}", name, v.Kind, v.Big, v.Button)
		}
		if v.Hours != "" || v.Title != "ОСТАЛОСЬ ВРЕМЕНИ:" {
			t.Errorf("%s: цифры %q или заголовок %q", name, v.Hours, v.Title)
		}
	}
}

// Ловит: в тексте табло длинное тире (скан CI, NFR-11) или обращение на «вы» ребёнку.
func TestCountdownTextsNoLongDash(t *testing.T) {
	views := []CountdownView{
		CountdownFor(Snapshot{}),
		CountdownFor(freshSnap(okStatus(5025, 7200))),
		CountdownFor(freshSnap(ipc.ChildStatusAdmin("Миша"))),
		CountdownFor(freshSnap(ipc.ChildStatusFrom("Миша", core.Decision{Status: core.StatusForbidden, Lock: core.LockForbidden}))),
	}
	for _, v := range views {
		for _, s := range []string{v.Title, v.Big, v.Button} {
			if strings.ContainsAny(s, "—–") {
				t.Errorf("длинное тире в %q", s)
			}
		}
	}
	if strings.Contains(textCountdownError, "Попробуйте") {
		t.Error("ребёнку на «ты»: «Попробуй»")
	}
}

// Ловит: учётка не ребёнка получает табло вместо мини-панели с «Настройки для родителя»
// (ТЗ 8.2.1 п.7): решает функция по снимку.
func TestUsesCountdown(t *testing.T) {
	if UsesCountdown(freshSnap(ipc.ChildStatusNotChild())) {
		t.Error("взрослому табло не показывается")
	}
	if !UsesCountdown(freshSnap(okStatus(100, 200))) {
		t.Error("ребёнку табло показывается")
	}
	if !UsesCountdown(Snapshot{}) {
		t.Error("пока службы нет, табло с ошибкой и «Обновить»")
	}
}

// Ловит: пока карточка открыта, служба опрашивается реже раза в секунду (секунды на плитках
// «залипают»), а закрытая карточка гоняет службу каждую секунду (ТЗ 8.2.1 п.3).
func TestPollInterval(t *testing.T) {
	if got := PollInterval(true); got != time.Second {
		t.Errorf("открытая карточка: %v, ждали 1s", got)
	}
	if got := PollInterval(false); got != 5*time.Second {
		t.Errorf("закрытая: %v, ждали 5s", got)
	}
}
