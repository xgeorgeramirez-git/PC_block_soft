package guard

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// Плашка «Сообщение от родителя» (FR-41, S57): очередь, «Понял», срок показа, поверх экрана
// блокировки, ждёт плашку 1 мин (PL-34), родительскую сессию и связь.

func noteOf(id, text string, show int) ipc.GuardNote {
	return ipc.GuardNote{ID: id, Text: text, ShowS: show, WaitS: 600, Sound: 1}
}

func noteKinds(evs []ipc.GuardEvent) string {
	var b []string
	for _, e := range evs {
		b = append(b, e.Kind+":"+e.NoteID)
	}
	return strings.Join(b, ",")
}

func TestNoteShownPressedNext(t *testing.T) {
	m := NewModel()
	m.OnDecision(t0, runningDecision(3600), false, nil)
	m.OnNote(t0, noteOf("a", "  Иди\nужинать ", 0))
	m.OnNote(t0, noteOf("b", "Потом уроки", 0))
	m.OnNote(t0, noteOf("a", "Иди\nужинать", 0)) // повтор службы — не второй раз
	v := m.View(t0)
	if v.Note == nil || v.Note.ID != "a" || v.Note.Text != "Иди\nужинать" || v.Note.Title != "Сообщение от родителя" || v.Note.Sound != 1 {
		t.Fatalf("плашка: %+v", v.Note)
	}
	if len(v.Note.Buttons) != 1 || v.Note.Buttons[0].Button != ButtonNoteOK || v.Note.Buttons[0].Label != "Понял" || !v.Note.Buttons[0].Main {
		t.Fatalf("кнопки: %+v", v.Note.Buttons)
	}
	if got := noteKinds(m.TakeNoteEvents()); got != "noteShown:a" {
		t.Fatalf("события: %s", got)
	}
	// Без «Понял» держится: через 5 минут на месте.
	keepAlive(m, runningDecision(3600), 1, 300)
	if v := m.View(at(300 * time.Second)); v.Note == nil || v.Note.ID != "a" {
		t.Fatalf("через 5 мин: %+v", v.Note)
	}
	seq := v.Note.Seq
	if act := m.Press(at(300*time.Second), ButtonNoteOK, ""); act.Kind != ActNone {
		t.Fatalf("действие %v", act.Kind)
	}
	v = m.View(at(300 * time.Second))
	if v.Note == nil || v.Note.ID != "b" || v.Note.Seq == seq {
		t.Fatalf("следующее: %+v", v.Note)
	}
	if got := noteKinds(m.TakeNoteEvents()); got != "noteDone:a,noteShown:b" {
		t.Fatalf("события: %s", got)
	}
	m.Press(at(300*time.Second), ButtonNoteOK, "")
	if v := m.View(at(300 * time.Second)); v.Note != nil {
		t.Fatalf("очередь пуста, а плашка %+v", v.Note)
	}
	if got := noteKinds(m.TakeNoteEvents()); got != "noteDone:b" {
		t.Fatalf("события: %s", got)
	}
	if got := m.TakeNoteEvents(); got == nil || len(got) != 0 {
		t.Fatalf("пустые события: %#v", got)
	}
}

func TestNoteTimedCloses(t *testing.T) {
	m := NewModel()
	m.OnDecision(t0, runningDecision(3600), false, nil)
	m.OnNote(t0, noteOf("a", "Скоро ужин", 10))
	m.TakeNoteEvents()
	m.Tick(at(10*time.Second - time.Millisecond))
	if v := m.View(at(10*time.Second - time.Millisecond)); v.Note == nil {
		t.Fatal("закрылась раньше 10 с")
	}
	m.Tick(at(10 * time.Second))
	if v := m.View(at(10 * time.Second)); v.Note != nil {
		t.Fatal("не закрылась через 10 с")
	}
	if got := noteKinds(m.TakeNoteEvents()); got != "noteDone:a" {
		t.Fatalf("события: %s", got)
	}
}

// Поверх экрана блокировки: экран, его кнопки и аварийный выход на месте; «Понял» панели не трогает.
func TestNoteOverLockScreen(t *testing.T) {
	m := withPasswordPanel(t, timeUpDecision())
	m.OnNote(t0, noteOf("a", "Иди ужинать", 10))
	v := m.View(t0)
	if !v.LockShown || v.Panel != PanelPassword || v.Note == nil || !v.SuppressKeys {
		t.Fatalf("экран и плашка: %+v", v)
	}
	m.Press(t0, ButtonNoteOK, "")
	v = m.View(t0)
	if !v.LockShown || v.Panel != PanelPassword || v.Note != nil || len(v.Choices) == 0 {
		t.Fatalf("после «Понял»: %+v", v)
	}
	// Нажатия экрана плашку не закрывают.
	m.OnNote(t0, noteOf("b", "Ещё", 0))
	m.Press(t0, ButtonCancel, "")
	if v := m.View(t0); v.Note == nil || v.Panel != PanelNone {
		t.Fatalf("«Отмена» экрана: %+v", v)
	}
}

// Плашка 1 мин важнее (PL-34): сообщение ждёт, пока она видна; экран блокировки — показывается поверх.
func TestNoteWaitsForOneMinutePlate(t *testing.T) {
	m := NewModel()
	m.OnDecision(t0, runningDecision(59), false, []int{1})
	if v := m.View(t0); v.Warning == nil || v.Warning.Kind != ipc.PlateOneMinute {
		t.Fatalf("нет плашки 1 мин: %+v", v.Warning)
	}
	m.OnNote(t0, noteOf("a", "Иди ужинать", 0))
	keepAlive(m, runningDecision(29), 1, 30)
	if v := m.View(at(30 * time.Second)); v.Note != nil || v.Warning == nil || v.Warning.Kind != ipc.PlateOneMinute {
		t.Fatalf("сообщение перекрыло плашку 1 мин: %+v %+v", v.Note, v.Warning)
	}
	if got := m.TakeNoteEvents(); len(got) != 0 {
		t.Fatalf("события до показа: %v", got)
	}
	m.OnDecision(at(60*time.Second), timeUpDecision(), false, nil)
	if v := m.View(at(60 * time.Second)); !v.LockShown || v.Note == nil {
		t.Fatalf("при экране блокировки: %+v", v.Note)
	}
}

// Родительская сессия и обрыв связи: сообщение ждёт; не показанное за WaitS забывается молча.
func TestNoteWaitsHiddenOfflineAndForgets(t *testing.T) {
	m := NewModel()
	m.OnDecision(t0, runningDecision(3600), true, nil)
	m.OnNote(t0, ipc.GuardNote{ID: "a", Text: "Привет", WaitS: 20})
	if v := m.View(t0); v.Note != nil {
		t.Fatal("показ при родительской сессии")
	}
	if got := m.TakeNoteEvents(); len(got) != 0 {
		t.Fatalf("«показано» при родительской сессии: %v", got)
	}
	m.OnDecision(at(5*time.Second), runningDecision(3600), false, nil)
	if v := m.View(at(5 * time.Second)); v.Note == nil {
		t.Fatal("после родительской сессии не показано")
	}
	m.Press(at(5*time.Second), ButtonNoteOK, "")
	m.TakeNoteEvents()

	m.OnNote(at(5*time.Second), ipc.GuardNote{ID: "b", Text: "Второе", WaitS: 20})
	m.OnDecision(at(6*time.Second), runningDecision(3600), true, nil) // плашка видна, затем родитель
	m.Press(at(6*time.Second), ButtonNoteOK, "")                      // не видна — нажать нельзя
	if got := noteKinds(m.TakeNoteEvents()); got != "noteShown:b" {
		t.Fatalf("события: %s", got)
	}
	m.OnDecision(at(7*time.Second), runningDecision(3600), false, nil)
	if v := m.View(at(7 * time.Second)); v.Note == nil || v.Note.ID != "b" {
		t.Fatal("показанное потерялось за родительской сессией")
	}
	m.Press(at(7*time.Second), ButtonNoteOK, "")
	m.TakeNoteEvents()

	m.OnNote(at(7*time.Second), ipc.GuardNote{ID: "c", Text: "Третье", WaitS: 3})
	m.mu.Lock()
	m.note = nil
	m.mu.Unlock()
	// Обрыв связи дольше срока ожидания: не показано и забыто.
	m2 := NewModel()
	m2.OnDecision(t0, runningDecision(59), false, []int{1})
	m2.OnNote(t0, ipc.GuardNote{ID: "z", Text: "Ждёт", WaitS: 3})
	m2.OnDecision(at(4*time.Second), timeUpDecision(), false, nil)
	if v := m2.View(at(4 * time.Second)); v.Note != nil {
		t.Fatalf("показано после срока ожидания: %+v", v.Note)
	}
	if got := m2.TakeNoteEvents(); len(got) != 0 {
		t.Fatalf("события забытого: %v", got)
	}
	if v := m.View(at(12 * time.Hour)); v.Note != nil {
		t.Fatal("без связи плашка видна")
	}
}

// Сообщение — своя команда (PL-81): не превращается в «Добавлено N минут» и не идёт плашкой notice;
// звук один раз на показ, номер из сообщения (FR-41): 2 — звук 2, 0 — ни одного вызова плеера, 9 (нет
// среди встроенных, телефон новее компьютера) — звук 1; повтор того же показа — без звука; «показано»
// и «Понял» уходят службе событиями с id; GuardState видит плашку.
func TestControllerPhoneNote(t *testing.T) {
	r := newCtRig()
	var mu sync.Mutex
	var played []int
	var warned atomic.Int32
	r.ctl.SetSoundPlayers(func() bool { warned.Add(1); return true }, func(n int) bool {
		mu.Lock()
		played = append(played, n)
		mu.Unlock()
		return true
	})
	playedNow := func(want int) string {
		for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
			mu.Lock()
			n := len(played)
			mu.Unlock()
			if n >= want {
				break
			}
		}
		time.Sleep(30 * time.Millisecond) // лишний вызов успел бы прийти
		mu.Lock()
		defer mu.Unlock()
		return fmt.Sprint(played)
	}
	r.svc.test = true
	r.svc.status = ctRunningStatus(3600)
	r.svc.cmds = []ipc.GuardCommand{
		{Kind: ipc.GuardCmdNote, Note: &ipc.GuardNote{ID: "n1", Text: "Добавлено 30 минут? Нет, иди ужинать", WaitS: 600, Sticker: "033", Sound: 2}},
		{Kind: ipc.GuardCmdNote, Note: &ipc.GuardNote{ID: "n2", Text: "Тихо", WaitS: 600, Sound: 0}},
		{Kind: ipc.GuardCmdNote, Note: &ipc.GuardNote{ID: "n3", Text: "Новый звук", WaitS: 600, Sound: 9}},
	}
	cmds := ClassifyCommands(r.svc.cmds)
	if len(cmds.PhoneNotes) != 3 || len(cmds.NoticeMinutes) != 0 || len(cmds.Notices) != 0 {
		t.Fatalf("разбор: %+v", cmds)
	}
	v := r.poll()
	r.svc.cmds = nil
	if v.Note == nil || v.Note.Text != "Добавлено 30 минут? Нет, иди ужинать" || v.Note.Sound != 2 || v.Warning != nil {
		t.Fatalf("вид: %+v %+v", v.Note, v.Warning)
	}
	r.poll() // тот же показ: звук не повторяется
	r.ctl.View()
	if got := playedNow(1); got != "[2]" {
		t.Fatalf("после первого показа сыграно %s, ждали [2]", got)
	}
	r.ctl.Press(ButtonNoteOK, "")
	if v := r.ctl.View(); v.Note == nil || v.Note.ID != "n2" || v.Note.Sound != 0 {
		t.Fatalf("второе: %+v", v.Note)
	}
	r.poll()
	if got := playedNow(2); got != "[2]" {
		t.Fatalf("«Без звука» сыграло: %s", got)
	}
	r.ctl.Press(ButtonNoteOK, "")
	if v := r.ctl.View(); v.Note == nil || v.Note.ID != "n3" || v.Note.Sound != 1 {
		t.Fatalf("третье: %+v", v.Note)
	}
	if got := playedNow(2); got != "[2 1]" {
		t.Fatalf("неизвестный звук 9: сыграно %s, ждали [2 1]", got)
	}
	r.ctl.Press(ButtonNoteOK, "")
	r.ctl.View()
	r.ctl.PollOnce()
	var kinds []string
	for _, msg := range r.svc.called(ipc.CmdGuardPoll) {
		var req ipc.GuardPollRequest
		if err := msg.Into(&req); err != nil {
			t.Fatal(err)
		}
		for _, e := range req.Events {
			if e.NoteID != "" {
				kinds = append(kinds, e.Kind+":"+e.NoteID)
			}
		}
	}
	if got := strings.Join(kinds, ","); got != "noteShown:n1,noteDone:n1,noteShown:n2,noteDone:n2,noteShown:n3,noteDone:n3" {
		t.Fatalf("события службе: %s", got)
	}
	if n := warned.Load(); n != 0 {
		t.Fatalf("сообщение сыграло звук предупреждения %d раз", n)
	}
	r.ctl.mu.Lock()
	snd := append([]ipc.ShownSound(nil), r.ctl.sounds...)
	r.ctl.mu.Unlock()
	var notes []string
	for _, s := range snd {
		if s.Note {
			notes = append(notes, fmt.Sprintf("%d:%v", s.NoteSound, s.OK))
		}
	}
	if got := strings.Join(notes, ","); got != "2:true,1:true" {
		t.Fatalf("журнал звуков сообщения %s: %+v", got, snd)
	}
	st := StateFromView(View{Note: &NoteView{ID: "n1", Text: "т", Buttons: []ButtonView{{Button: ButtonNoteOK, Label: "Понял", Enabled: true}}}}, "", nil, nil, nil)
	if st.Note != "т" || st.NoteID != "n1" || len(st.Buttons) != 1 || st.Buttons[0] != ipc.ButtonNoteOK {
		t.Fatalf("GuardState: %+v", st)
	}
}

// Ловит: номер звука и стикер теряются по пути от команды службы до вида; подмена неизвестного звука
// не та (0 — без звука, встроенный — как есть, не встроенный — 1); стикер не сохранён в очереди
// (он нужен окну, T4a).
func TestNoteSoundNumber(t *testing.T) {
	cases := []struct{ in, want int }{{1, 1}, {2, 2}, {5, 5}, {0, 0}, {9, 1}, {99, 1}, {-1, 1}}
	for _, c := range cases {
		m := NewModel()
		m.OnDecision(t0, runningDecision(3600), false, nil)
		m.OnNote(t0, ipc.GuardNote{ID: "a", Text: "т", WaitS: 600, Sticker: "033", Sound: c.in})
		v := m.View(t0)
		if v.Note == nil || v.Note.Sound != c.want {
			t.Errorf("звук %d: вид %+v, ждали звук %d", c.in, v.Note, c.want)
			continue
		}
		m.mu.Lock()
		raw, sticker := m.note.sound, m.note.sticker
		m.mu.Unlock()
		if raw != c.in || sticker != "033" {
			t.Errorf("звук %d: в очереди звук %d, стикер %q", c.in, raw, sticker)
		}
		if NoteSoundDue(0, v.Note) != (c.want != 0) {
			t.Errorf("звук %d: NoteSoundDue = %v", c.in, NoteSoundDue(0, v.Note))
		}
	}
}

// Ловит (ревью I-5): предупреждение и сообщение в одном кадре играют двумя исполнителями сразу
// (PlaySoundW обрывает или путает звуки) или один из них теряется; проверка «плеер задан» берёт
// чужой вид (нет плеера предупреждений — не играет и сообщение).
func TestSoundWarningAndNoteOneLoop(t *testing.T) {
	var active, maxActive atomic.Int32
	var mu sync.Mutex
	var order []string
	busy := func(name string) bool {
		n := active.Add(1)
		for {
			m := maxActive.Load()
			if n <= m || maxActive.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		mu.Lock()
		order = append(order, name)
		mu.Unlock()
		active.Add(-1)
		return true
	}
	got := func(want int) string {
		for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
			mu.Lock()
			n := len(order)
			mu.Unlock()
			if n >= want {
				break
			}
		}
		time.Sleep(60 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(order, ",")
	}
	start := func(warn func() bool, msg func(int) bool) *ctRig {
		r := newCtRig()
		r.svc.test = true
		r.ctl.SetSoundPlayers(warn, msg)
		r.svc.status = ctRunningStatus(300)
		r.svc.warns = []ipc.GuardWarning{ipc.NewGuardWarning(5)}
		r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdNote, Note: &ipc.GuardNote{ID: "n1", Text: "Ужин", WaitS: 600, Sound: 3}}}
		v := r.poll()
		if v.Warning == nil || v.Note == nil {
			t.Fatalf("в одном кадре нет табло или сообщения: %+v %+v", v.Warning, v.Note)
		}
		return r
	}
	r := start(func() bool { return busy("w") }, func(n int) bool { return busy(fmt.Sprint("n", n)) })
	if o := got(2); o != "w,n3" || maxActive.Load() != 1 {
		t.Fatalf("сыграно %q, одновременно %d; ждали w,n3 по очереди", o, maxActive.Load())
	}
	r.ctl.mu.Lock()
	snd := append([]ipc.ShownSound(nil), r.ctl.sounds...)
	r.ctl.mu.Unlock()
	if len(snd) != 2 || snd[0].Note || snd[0].Threshold != 5 || !snd[1].Note || snd[1].NoteSound != 3 || !snd[0].OK || !snd[1].OK {
		t.Fatalf("журнал звуков %+v", snd)
	}

	// Нет плеера предупреждений: сообщение всё равно звучит, предупреждение в журнал не пишется.
	mu.Lock()
	order = nil
	mu.Unlock()
	r = start(nil, func(n int) bool { return busy(fmt.Sprint("n", n)) })
	if o := got(1); o != "n3" {
		t.Fatalf("без плеера предупреждений сыграно %q", o)
	}
	r.ctl.mu.Lock()
	snd = append([]ipc.ShownSound(nil), r.ctl.sounds...)
	r.ctl.mu.Unlock()
	if len(snd) != 1 || !snd[0].Note {
		t.Fatalf("журнал без плеера предупреждений %+v", snd)
	}

	// Нет плеера сообщений: предупреждение звучит, сообщение в журнал не пишется.
	mu.Lock()
	order = nil
	mu.Unlock()
	r = start(func() bool { return busy("w") }, nil)
	if o := got(1); o != "w" {
		t.Fatalf("без плеера сообщений сыграно %q", o)
	}
	r.ctl.mu.Lock()
	snd = append([]ipc.ShownSound(nil), r.ctl.sounds...)
	r.ctl.mu.Unlock()
	if len(snd) != 1 || snd[0].Note {
		t.Fatalf("журнал без плеера сообщений %+v", snd)
	}
}

// Срок показа 10 с идёт, только пока плашка на экране: за родительской сессией и без связи со службой
// он стоит, после возврата плашка видна оставшееся время.
func TestNoteTimedCountsOnlyVisible(t *testing.T) {
	m := NewModel()
	m.OnDecision(t0, runningDecision(3600), false, nil)
	m.OnNote(t0, noteOf("a", "Скоро ужин", 10))
	m.TakeNoteEvents()
	m.OnDecision(at(4*time.Second), runningDecision(3600), true, nil) // родитель: 4 с показано
	m.Tick(at(30 * time.Second))
	m.OnDecision(at(30*time.Second), runningDecision(3600), false, nil)
	if v := m.View(at(30 * time.Second)); v.Note == nil {
		t.Fatal("срок показа шёл за родительской сессией")
	}
	keepAlive(m, runningDecision(3600), 31, 35)
	if v := m.View(at(35 * time.Second)); v.Note == nil {
		t.Fatal("закрылась раньше 10 с на экране (4 + 5)")
	}
	m.OnDecision(at(36*time.Second), runningDecision(3600), false, nil)
	if v := m.View(at(36 * time.Second)); v.Note != nil {
		t.Fatal("не закрылась через 10 с на экране")
	}
	if got := noteKinds(m.TakeNoteEvents()); got != "noteDone:a" {
		t.Fatalf("события: %s", got)
	}

	// Обрыв связи: последний ответ в 2 с, плашка видна до 12 с (offlineAfter), потом без связи до 60 с.
	m2 := NewModel()
	m2.OnDecision(t0, runningDecision(3600), false, nil)
	m2.OnNote(t0, noteOf("b", "Скоро ужин", 30))
	m2.OnDecision(at(2*time.Second), runningDecision(3600), false, nil)
	m2.Tick(at(40 * time.Second))
	m2.Tick(at(60 * time.Second))
	m2.OnDecision(at(60*time.Second), runningDecision(3600), false, nil)
	if v := m2.View(at(60 * time.Second)); v.Note == nil {
		t.Fatal("срок показа шёл без связи со службой")
	}
	keepAlive(m2, runningDecision(3600), 61, 77) // 12 + 17 = 29 с на экране
	if v := m2.View(at(77 * time.Second)); v.Note == nil {
		t.Fatal("закрылась раньше 30 с на экране")
	}
	m2.OnDecision(at(78*time.Second), runningDecision(3600), false, nil)
	if v := m2.View(at(78 * time.Second)); v.Note != nil {
		t.Fatal("не закрылась через 30 с на экране")
	}
}
