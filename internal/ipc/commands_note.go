package ipc

// Сообщение с телефона ребёнку (FR-41, D-97, протокол связи, раздел 5а). Служба отдаёт его стражу
// сеанса ребёнка командой явного вида GuardCmdNote (PL-81: страж не угадывает смысл по тексту),
// страж сообщает показ и «Понял» событиями GuardEvNoteShown и GuardEvNoteDone.

// GuardCmdNote — команда стражу показать плашку «Сообщение от родителя» (GuardCommand.Note).
const GuardCmdNote = "note"

// События стража о сообщении (GuardEvent.NoteID).
const (
	GuardEvNoteShown = "noteShown" // плашка с сообщением появилась на экране ребёнка
	GuardEvNoteDone  = "noteDone"  // ребёнок нажал «Понял» или вышло время показа
)

// ButtonNoteOK — кнопка «Понял» на плашке сообщения (GuardState.Buttons, TestGuard click).
const ButtonNoteOK = "noteOk"

// GuardNote — сообщение для стража. ShowS — сколько показывать (0 — пока не нажмёт «Понял»; 10, 30,
// 60). WaitS — сколько сообщение ещё может ждать показа (10 минут от приёма минус прошедшее): не
// показанное за этот срок страж молча забывает, служба гасит его сама (expired).
type GuardNote struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	ShowS int    `json:"showS"`
	WaitS int    `json:"waitS"`
}
