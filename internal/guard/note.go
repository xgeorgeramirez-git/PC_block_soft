package guard

import (
	"strings"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/sound"
)

// Плашка «Сообщение от родителя» (FR-41, D-97, S57): текст с телефона, кнопка «Понял», звук,
// выбранный родителем, при появлении. Видна и поверх экрана блокировки PC Block (окно своё, не мешает его кнопкам и паролю,
// NFR-10). Сообщения идут по очереди: следующее — когда закрыто прежнее. Плашку 1 мин сообщение не
// перекрывает: пока она видна без экрана блокировки, сообщение ждёт (PL-34). Ждёт и при родительской
// сессии и без связи со службой. Жизнь плашки (PL-70): showS = 0 — до «Понял»; 10, 30, 60 с —
// исчезает сама по сроку, «Понял» есть и тогда; не показанное за WaitS забывается молча (службу
// оно уже погасило). Срок показа идёт, только пока плашка на экране: за родительской сессией и без
// связи со службой он стоит.

const (
	textNoteTitle = "Сообщение от родителя"
	textNoteOK    = "Понял"
	noteSeenMax   = 64 // сколько id сообщений помнит страж, чтобы повтор службы не показался второй раз
)

// noteItem — сообщение в очереди стража.
type noteItem struct {
	id, text  string
	sticker   string        // номер стикера, как прислал телефон ("" — без стикера)
	sound     int           // номер звука, как прислал телефон (0 — без звука); подмена — в noteView
	show      time.Duration // 0 — до «Понял»
	waitUntil time.Time     // не показанное к этому моменту забывается
	shownAt   time.Time
	seq       int           // номер показа (звук один раз на показ)
	seen      time.Duration // сколько плашка была видна: срок показа идёт только на экране
	clockAt   time.Time     // до этого момента seen уже посчитано
}

// NoteView — плашка сообщения для окна.
type NoteView struct {
	// Seq растёт с каждым показом: окно играет звук, когда Seq сменился (как WarningView.ID).
	Seq     int
	ID      string // id сообщения (GuardState.NoteID)
	Title   string // «Сообщение от родителя»
	Text    string // как написал родитель, с переводами строк
	Buttons []ButtonView
	// Sound — номер встроенного звука сообщения (sound.PlayMessage): 0 — без звука. Номер, которого
	// нет среди встроенных (телефон новее компьютера), уже заменён на 1.
	Sound int
}

// OnNote принимает сообщение службы (ipc.GuardCmdNote). То же id второй раз не встаёт в очередь
// (служба повторяет, пока не узнает «показано»).
func (m *Model) OnNote(now time.Time, n ipc.GuardNote) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance(now)
	text := strings.TrimSpace(n.Text)
	if n.ID == "" || text == "" {
		return
	}
	for _, id := range m.noteSeen {
		if id == n.ID {
			return
		}
	}
	m.noteSeen = append(m.noteSeen, n.ID)
	if len(m.noteSeen) > noteSeenMax {
		m.noteSeen = m.noteSeen[len(m.noteSeen)-noteSeenMax:]
	}
	wait := time.Duration(max(n.WaitS, 1)) * time.Second
	show := time.Duration(max(n.ShowS, 0)) * time.Second
	m.noteQ = append(m.noteQ, &noteItem{id: n.ID, text: text, sticker: n.Sticker, sound: n.Sound, show: show, waitUntil: now.Add(wait)})
	m.advanceNotes(now)
}

// advanceNotes подводит сообщения к моменту now: показанное на время закрывается по сроку, следующее
// показывается, если можно. Под m.mu.
func (m *Model) advanceNotes(now time.Time) {
	m.noteClock(now)
	if n := m.note; n != nil && n.show > 0 && n.seen >= n.show {
		m.noteDone(now)
	}
	if m.note != nil {
		return
	}
	for len(m.noteQ) > 0 {
		head := m.noteQ[0]
		if now.After(head.waitUntil) {
			m.noteQ = m.noteQ[1:]
			continue
		}
		if !m.noteCanShow(now) {
			return
		}
		m.noteQ = m.noteQ[1:]
		m.noteSeq++
		head.shownAt, head.seq, head.clockAt = now, m.noteSeq, now
		m.note = head
		m.noteEvents = append(m.noteEvents, ipc.GuardEvent{Kind: ipc.GuardEvNoteShown, AtMs: now.UnixMilli(), NoteID: head.id})
		return
	}
}

// noteClock прибавляет к сроку показа время, пока плашка была на экране: с прошлого подсчёта до now,
// если тогда она была видна (есть решение службы, нет родительской сессии), и не дальше обрыва связи
// (lastOK + offlineAfter). Родительская сессия и обрыв меняются только после advance (OnWarnings),
// поэтому состояние на отрезке — то, что сейчас. Зовут advance до проверки обрыва и advanceNotes. Под m.mu.
func (m *Model) noteClock(now time.Time) {
	n := m.note
	if n == nil {
		return
	}
	if m.hasDec && !m.hidden {
		end := now
		if off := m.lastOK.Add(offlineAfter); end.After(off) {
			end = off
		}
		if end.After(n.clockAt) {
			n.seen += end.Sub(n.clockAt)
		}
	}
	if now.After(n.clockAt) {
		n.clockAt = now
	}
}

// noteDone закрывает показанное сообщение: «Понял» или вышел срок показа.
func (m *Model) noteDone(now time.Time) {
	m.noteEvents = append(m.noteEvents, ipc.GuardEvent{Kind: ipc.GuardEvNoteDone, AtMs: now.UnixMilli(), NoteID: m.note.id})
	m.note = nil
}

// noteCanShow: сообщение можно показать сейчас — служба на связи, нет родительской сессии и не видна
// плашка 1 мин без экрана блокировки (она важнее: о конце времени, с кнопками, PL-34). Экран
// блокировки показу не мешает.
func (m *Model) noteCanShow(now time.Time) bool {
	if !m.hasDec || m.hidden || m.offlineAt(now) {
		return false
	}
	return !m.oneMinuteShown(now)
}

// oneMinuteShown: на экране плашка 1 мин (без экрана блокировки).
func (m *Model) oneMinuteShown(now time.Time) bool {
	if m.dec.Lock != core.LockNone || !m.plateVisible(now) {
		return false
	}
	return m.warn.kind == plateThreshold && m.warn.threshold == 1
}

// noteVisible: плашка сообщения сейчас видна (её можно нажать).
func (m *Model) noteVisible(now time.Time) bool {
	return m.note != nil && m.hasDec && !m.hidden && !m.offlineAt(now)
}

// pressNote — «Понял»: сообщение закрыто, следующее в очереди показывается.
func (m *Model) pressNote(now time.Time) {
	if !m.noteVisible(now) {
		return
	}
	m.noteDone(now)
	m.advanceNotes(now)
}

// noteView — вид плашки сообщения; nil — не видна. Под m.mu.
func (m *Model) noteView(now time.Time) *NoteView {
	if !m.noteVisible(now) {
		return nil
	}
	n := m.note
	return &NoteView{Seq: n.seq, ID: n.id, Title: textNoteTitle, Text: n.text, Sound: noteSound(n.sound),
		Buttons: []ButtonView{{Button: ButtonNoteOK, Label: textNoteOK, Main: true, Enabled: true}}}
}

// TakeNoteEvents возвращает события «показано» и «Понял» и очищает очередь; пустой — [], не nil.
func (m *Model) TakeNoteEvents() []ipc.GuardEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.noteEvents
	m.noteEvents = []ipc.GuardEvent{}
	if out == nil {
		out = []ipc.GuardEvent{}
	}
	return out
}

// noteSound — звук, который сыграет страж: 0 («Без звука») — как есть, встроенный номер — как есть,
// остальное (номер новее этой программы) — звук 1. Служба и журнал хранят то, что прислал телефон.
func noteSound(n int) int {
	if n == 0 || sound.HasMessage(n) {
		return n
	}
	return 1
}

// NoteSoundDue: окну пора сыграть звук для плашки сообщения n, если звук уже играл для показа lastSeq.
// «Без звука» (0) — никогда.
func NoteSoundDue(lastSeq int, n *NoteView) bool {
	return n != nil && n.Sound != 0 && n.Seq != lastSeq
}
