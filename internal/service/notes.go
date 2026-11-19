package service

import (
	"encoding/json"
	"strings"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/remote"
)

// Сообщение ребёнку с телефона (FR-41, D-97, протокол связи, раздел 5а). Связь (internal/remote)
// проверяет подпись и отдаёт сообщение в ApplyNote; дальше служба держит очередь по ребёнку: показ —
// стражу сеанса ребёнка на экране командой явного вида ipc.GuardCmdNote, «показано» и «Понял» —
// событиями стража. Очередь — в памяти и одной записью settingNotes (PL-55): переживает перезапуск.

const (
	settingNotes = "remote.notes" // одна запись JSON: []noteRec
	// noteWait — сколько сообщение ждёт ребёнка на экране; потом гаснет (expired).
	noteWait = 10 * time.Minute
	// noteKeep — сколько сообщения в последнем состоянии остаются в отчёте телефону.
	noteKeep = 24 * time.Hour
	// noteReportMax — сколько последних сообщений уходит в отчёт.
	noteReportMax = 10
	// noteStoreMax — сколько сообщений хранится всего (очередь и последние).
	noteStoreMax = 30
	// noteResend — ждущее сообщение отдаётся стражу снова, если он не сказал «показано» (страж мог
	// перезапуститься; одинаковое он не показывает дважды по id).
	noteResend = 5 * time.Second
)

// noteRec — сообщение в очереди службы. Текст хранится, только пока сообщение ждёт показа.
type noteRec struct {
	ID       string `json:"id"`
	Child    int64  `json:"child"`
	Text     string `json:"text,omitempty"`
	ShowSec  int    `json:"showSec"`
	State    string `json:"state"`    // remote.Note*
	QueuedMs int64  `json:"queuedMs"` // приём (настоящие часы службы)
	AtMs     int64  `json:"atMs"`     // последняя смена состояния

	sentTo uint32    // сеанс, стражу которого сообщение отдано последним (в памяти)
	sentAt time.Time // когда
}

// notesState — очередь сообщений (под s.mu).
type notesState struct {
	loaded bool
	list   []*noteRec // по порядку приёма
}

// loadNotes читает очередь, если ещё не прочитана; база не ответила — false, попытка позже.
// Испорченная запись — очередь пустая (сообщения забыты, телефон увидит их без отметок).
func (s *Service) loadNotes() bool {
	if s.notes.loaded {
		return true
	}
	v, ok, err := s.cfg.Store.GetSetting(settingNotes)
	if err != nil {
		s.logRate("notes-load", "сообщения с телефона: очередь не прочитана: %v", err)
		return false
	}
	var list []*noteRec
	if ok && v != "" {
		if err := json.Unmarshal([]byte(v), &list); err != nil {
			s.cfg.Logf("сообщения с телефона: запись очереди не разобрана, очередь пустая: %v", err)
			list = nil
		}
	}
	s.notes.list, s.notes.loaded = list, true
	return true
}

// saveNotes пишет очередь целиком. Сбой — строка в журнал работы: очередь в памяти остаётся верной,
// после перезапуска службы теряется только последнее изменение.
func (s *Service) saveNotes() {
	raw, err := json.Marshal(s.notes.list)
	if err == nil {
		err = s.cfg.Store.SetSetting(settingNotes, string(raw))
	}
	if err != nil {
		s.logRate("notes-save", "сообщения с телефона: очередь не записана: %v", err)
	}
}

// ApplyNote — сообщение телефона под одной s.mu (протокол, раздел 5а): поколение → привязка → seq →
// запись seq → ребёнок → очередь и журнал. Сообщение принимается не больше одного раза: seq записан
// раньше очереди, после перезапуска повтор отсекается; тот же id второй раз в очередь не встаёт.
func (h remoteHost) ApplyNote(gen int64, n remote.Note) (string, int64) {
	s := h.s
	if s.rem.beforeApplyLock != nil {
		s.rem.beforeApplyLock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.rem.st
	if !s.remoteGen(gen) {
		return remote.ErrCodeStale, st.LastSeq
	}
	if st.Phone == nil || st.Phone.ID == s.rem.unpairing {
		return remote.ErrCodeNotPaired, st.LastSeq
	}
	if n.Seq <= st.LastSeq {
		return remote.ErrCodeReplay, st.LastSeq
	}
	next := st
	next.LastSeq = n.Seq
	if err := s.writeRemote(next); err != nil {
		// Номер не записан — сообщение не берём: после перезапуска оно пришло бы второй раз.
		s.logRate("remote-seq", "связь с телефоном: номер сообщения не записан, сообщение не принято: %v", err)
		return remote.ErrCodeStale, st.LastSeq
	}
	cs := s.children[n.Child]
	if cs == nil {
		return remote.ErrCodeNoChild, n.Seq
	}
	if !s.loadNotes() {
		return remote.ErrCodeStale, n.Seq
	}
	for _, r := range s.notes.list {
		if r.ID == n.ID {
			return "", n.Seq // телефон повторил то же сообщение новым номером: одно сообщение
		}
	}
	real := s.cfg.RealNow()
	s.real = real
	ms := real.UnixMilli()
	s.notes.list = append(s.notes.list, &noteRec{ID: n.ID, Child: n.Child, Text: n.Text, ShowSec: n.ShowSec,
		State: remote.NoteQueued, QueuedMs: ms, AtMs: ms})
	s.pruneNotes(real)
	s.saveNotes()
	s.journal.Write(cs.child.ID, evRemoteNote, map[string]any{"text": n.Text})
	return "", n.Seq
}

// Notes — отчёт телефону: последние noteReportMax сообщений не старше суток, по порядку приёма.
func (h remoteHost) Notes() []remote.NoteReport {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loadNotes() {
		return nil
	}
	return s.noteReports(s.cfg.RealNow())
}

func (s *Service) noteReports(real time.Time) []remote.NoteReport {
	out := []remote.NoteReport{}
	for _, r := range s.notes.list {
		if real.UnixMilli()-r.QueuedMs <= noteKeep.Milliseconds() {
			out = append(out, remote.NoteReport{ID: r.ID, Child: r.Child, State: r.State, AtMs: r.AtMs})
		}
	}
	if len(out) > noteReportMax {
		out = out[len(out)-noteReportMax:]
	}
	return out
}

// pruneNotes убирает сообщения старше суток в последнем состоянии и лишние сверх noteStoreMax
// (самые старые из законченных).
func (s *Service) pruneNotes(real time.Time) {
	kept := s.notes.list[:0]
	for _, r := range s.notes.list {
		if r.State != remote.NoteQueued && real.UnixMilli()-r.QueuedMs > noteKeep.Milliseconds() {
			continue
		}
		kept = append(kept, r)
	}
	for len(kept) > noteStoreMax {
		i := 0
		for i < len(kept) && kept[i].State == remote.NoteQueued {
			i++
		}
		if i == len(kept) {
			i = 0
		}
		kept = append(kept[:i], kept[i+1:]...)
	}
	s.notes.list = kept
}

// noteChanged — состояние сообщения сменилось: запись очереди и отчёт телефону сразу (раздел 5а).
func (s *Service) noteChanged() {
	s.saveNotes()
	s.remoteChanged()
}

// expireNotes гасит сообщения, которые ждали ребёнка дольше noteWait (Housekeep).
func (s *Service) expireNotes(real time.Time) {
	if !s.loadNotes() {
		return
	}
	changed := false
	for _, r := range s.notes.list {
		if r.State == remote.NoteQueued && real.UnixMilli()-r.QueuedMs >= noteWait.Milliseconds() {
			r.State, r.AtMs, r.Text = remote.NoteExpired, real.UnixMilli(), ""
			changed = true
			s.vlogf("сообщение с телефона %s: 10 минут не показано, погашено", r.ID)
		}
	}
	if changed {
		s.pruneNotes(real)
		s.noteChanged()
	}
}

// deliverNotes — команды стражу сеанса session о ждущих сообщениях его ребёнка (под s.mu, в ответе
// GuardPoll). Только когда ребёнок на экране: сеанс не отключён и не заблокирован. Экран блокировки
// PC Block — не препятствие: плашка встаёт поверх (FR-41). Родительская сессия в сеансе — сообщение
// ждёт у стража (он прячет плашки, Ruling 19).
func (s *Service) deliverNotes(cs *childState, ss *sessState, session uint32, real time.Time) []ipc.GuardCommand {
	if ss == nil || ss.offScreen() || !s.loadNotes() {
		return nil
	}
	var out []ipc.GuardCommand
	for _, r := range s.notes.list {
		if r.Child != cs.child.ID || r.State != remote.NoteQueued {
			continue
		}
		left := noteWait - real.Sub(time.UnixMilli(r.QueuedMs))
		if left <= 0 {
			continue // погасит expireNotes
		}
		if r.sentTo == session && real.Sub(r.sentAt) < noteResend && !real.Before(r.sentAt) {
			continue
		}
		r.sentTo, r.sentAt = session, real
		out = append(out, ipc.GuardCommand{Kind: ipc.GuardCmdNote, Note: &ipc.GuardNote{
			ID: r.ID, Text: r.Text, ShowS: r.ShowSec, WaitS: int(left / time.Second),
		}})
	}
	return out
}

// noteEvent — событие стража о сообщении: показано (shown) или «Понял»/время вышло (read). Чужое
// сообщение (другой ребёнок) и неизвестный id пропускаются. Показ после срока ожидания засчитывается:
// плашка на экране, это правда для телефона.
func (s *Service) noteEvent(cs *childState, ev ipc.GuardEvent, real time.Time) {
	if ev.NoteID == "" || !s.loadNotes() {
		return
	}
	for _, r := range s.notes.list {
		if r.ID != ev.NoteID || r.Child != cs.child.ID {
			continue
		}
		switch {
		case ev.Kind == ipc.GuardEvNoteShown && (r.State == remote.NoteQueued || r.State == remote.NoteExpired):
			r.State = remote.NoteShown
		case ev.Kind == ipc.GuardEvNoteDone && r.State != remote.NoteRead:
			r.State = remote.NoteRead
		default:
			return
		}
		r.AtMs, r.Text = real.UnixMilli(), ""
		s.noteChanged()
		return
	}
}

// noteJournalText — строка журнала «сообщение с телефона: «Иди ужинать»» (eventText добавит имя).
// Переводы строк — пробелами: журнал однострочный.
func noteJournalText(text string) string {
	return "сообщение с телефона: «" + strings.Join(strings.Fields(text), " ") + "»"
}
