package guard

import (
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// Плашка «Сообщение от родителя» (FR-41, S57): очередь, «Понял», срок показа, поверх экрана
// блокировки, ждёт плашку 1 мин (PL-34), родительскую сессию и связь.

func noteOf(id, text string, show int) ipc.GuardNote {
	return ipc.GuardNote{ID: id, Text: text, ShowS: show, WaitS: 600}
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
	if v.Note == nil || v.Note.ID != "a" || v.Note.Text != "Иди\nужинать" || v.Note.Title != "Сообщение от родителя" || !v.Note.Sound {
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
// звук один раз на показ; «показано» и «Понял» уходят службе событиями с id; GuardState видит плашку.
func TestControllerPhoneNote(t *testing.T) {
	r := newCtRig()
	var played []bool
	r.ctl.SetSoundPlayer(func() bool { played = append(played, true); return true })
	r.svc.test = true
	r.svc.status = ctRunningStatus(3600)
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdNote, Note: &ipc.GuardNote{ID: "n1", Text: "Добавлено 30 минут? Нет, иди ужинать", WaitS: 600}}}
	cmds := ClassifyCommands(r.svc.cmds)
	if len(cmds.PhoneNotes) != 1 || len(cmds.NoticeMinutes) != 0 || len(cmds.Notices) != 0 {
		t.Fatalf("разбор: %+v", cmds)
	}
	v := r.poll()
	r.svc.cmds = nil
	if v.Note == nil || v.Note.Text != "Добавлено 30 минут? Нет, иди ужинать" || v.Warning != nil {
		t.Fatalf("вид: %+v %+v", v.Note, v.Warning)
	}
	r.poll()
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
	if got := strings.Join(kinds, ","); got != "noteShown:n1,noteDone:n1" {
		t.Fatalf("события службе: %s", got)
	}
	r.ctl.mu.Lock()
	snd := append([]ipc.ShownSound(nil), r.ctl.sounds...)
	r.ctl.mu.Unlock()
	notes := 0
	for _, s := range snd {
		if s.Note {
			notes++
		}
	}
	if notes != 1 {
		t.Fatalf("звуков сообщения %d: %+v", notes, snd)
	}
	st := StateFromView(View{Note: &NoteView{ID: "n1", Text: "т", Buttons: []ButtonView{{Button: ButtonNoteOK, Label: "Понял", Enabled: true}}}}, "", nil, nil, nil)
	if st.Note != "т" || st.NoteID != "n1" || len(st.Buttons) != 1 || st.Buttons[0] != ipc.ButtonNoteOK {
		t.Fatalf("GuardState: %+v", st)
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
