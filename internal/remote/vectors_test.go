package remote

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "перезаписать testdata/remote/vectors.json")

var vectorsPath = filepath.Join("..", "..", "testdata", "remote", "vectors.json")

// vectors — общие векторы Go и Kotlin (T8): ключи, обязательство, число сверки, подписанные
// команда (ключом телефона), ответ и отчёт (ключом компьютера). ECDSA-подпись
// недетерминирована, поэтому подписи сверяются проверкой, а commit и sas — байт-в-байт.
type vectors struct {
	Comment   string `json:"comment"`
	PCPriv    string `json:"pcPriv"`
	PCPub     string `json:"pcPub"`
	PhonePriv string `json:"phonePriv"`
	PhonePub  string `json:"phonePub"`
	NPc       string `json:"nPc"`
	NPhone    string `json:"nPhone"`
	Commit    string `json:"commit"`
	SAS       string `json:"sas"`

	CmdJSON    string `json:"cmdJson"` // точные байты data команды
	Cmd        Signed `json:"cmd"`     // подпись ключом телефона
	CmdHash    string `json:"cmdHash"` // поле cmd ответа
	CmdrJSON   string `json:"cmdrJson"`
	Cmdr       Signed `json:"cmdr"` // подпись ключом компьютера
	ReportJSON string `json:"reportJson"`
	Report     Signed `json:"report"` // подпись ключом компьютера

	// Сообщение ребёнку (раздел 5а): data, подпись ключом телефона, хэш для noter, ответ noter и
	// отчёт с состояниями сообщений (подписи ключом компьютера).
	NoteJSON        string `json:"noteJson"`
	Note            Signed `json:"note"`
	NoteHash        string `json:"noteHash"`
	NoterJSON       string `json:"noterJson"`
	Noter           Signed `json:"noter"`
	ReportNotesJSON string `json:"reportNotesJson"`
	ReportNotes     Signed `json:"reportNotes"`
	// Сообщение со стикером и звуком (раздел 5а, FB-28): data с необязательными sticker и sound после
	// showSec, подпись ключом телефона, хэш. Вектор note выше — сообщение телефона 0.8.2 без них.
	NoteStickerJSON string `json:"noteStickerJson"`
	NoteSticker     Signed `json:"noteSticker"`
	NoteStickerHash string `json:"noteStickerHash"`

	// «Снять запрет на сегодня» (раздел 5б): data команды с op и без delta, подпись ключом телефона,
	// хэш и ответ cmdr с отчётом, где у ребёнка forbid (подпись ключом компьютера).
	CmdNoForbiddenJSON  string `json:"cmdNoForbiddenJson"`
	CmdNoForbidden      Signed `json:"cmdNoForbidden"`
	CmdNoForbiddenHash  string `json:"cmdNoForbiddenHash"`
	CmdrNoForbiddenJSON string `json:"cmdrNoForbiddenJson"`
	CmdrNoForbidden     Signed `json:"cmdrNoForbidden"`
	ReportForbidJSON    string `json:"reportForbidJson"`
	ReportForbid        Signed `json:"reportForbid"`

	// «Статистика за сегодня» (FR-43, раздел 6): отчёт pc/report с today (подпись ключом компьютера).
	ReportTodayJSON string `json:"reportTodayJson"`
	ReportToday     Signed `json:"reportToday"`
}

func vectorCommand() Command {
	return Command{V: 1, T: TypeCmd, PC: "pc-vector", Phone: "phone-vector", Seq: 17, TS: 1763490000000, Child: 3, Delta: 15}
}

func vectorReport() Report {
	return Report{V: 1, T: TypeReport, PC: "pc-vector", Phone: "phone-vector", TS: 1763490001000,
		Children: []ChildReport{{ID: 3, Name: "Миша", UsedSec: 3600, LeftSec: 1800, ExtraMin: 15}}}
}

func vectorNote() Note {
	return Note{V: 1, T: TypeNote, PC: "pc-vector", Phone: "phone-vector", Seq: 18, TS: 1763490002000, Child: 3,
		ID: "AAECAwQFBgcICQoLDA0ODw", Text: "Иди ужинать\nПотом уроки", ShowSec: 0}
}

// vectorNoteSticker — сообщение со стикером 033 и звуком 2 (FB-28).
func vectorNoteSticker() Note {
	return Note{V: 1, T: TypeNote, PC: "pc-vector", Phone: "phone-vector", Seq: 20, TS: 1763490007000, Child: 3,
		ID: "EBESExQVFhcYGRobHB0eHw", Text: "Иди ужинать", ShowSec: 30,
		StickerRaw: json.RawMessage(`"033"`), SoundRaw: json.RawMessage(`2`)}
}

func vectorReportNotes() Report {
	r := vectorReport()
	r.TS = 1763490003000
	r.Notes = []NoteReport{{ID: "AAECAwQFBgcICQoLDA0ODw", Child: 3, State: NoteShown, AtMs: 1763490002500}}
	return r
}

func vectorNoForbidden() Command {
	return Command{V: 1, T: TypeCmd, PC: "pc-vector", Phone: "phone-vector", Seq: 19, TS: 1763490004000, Child: 3, Op: OpNoForbidden}
}

// vectorReportForbid — отчёт со всеми состояниями запрета: Миша — снят, Коля — идёт до 21:00 МСК
// (forbidUntilMs), Даша — будет сегодня, Петя — сегодня нет.
func vectorReportForbid() Report {
	return Report{V: 1, T: TypeReport, PC: "pc-vector", Phone: "phone-vector", TS: 1763490005000,
		Children: []ChildReport{
			{ID: 3, Name: "Миша", UsedSec: 3600, LeftSec: 1800, ExtraMin: 15, Forbid: ForbidCleared},
			{ID: 4, Name: "Коля", UsedSec: 600, LeftSec: 3000, Blocked: true, Forbid: ForbidActive, ForbidUntilMs: 1763496000000},
			{ID: 5, Name: "Даша", LeftSec: -1, Forbid: ForbidAhead},
			{ID: 6, Name: "Петя", LeftSec: 0, Blocked: true, Forbid: ForbidNone},
		}}
}

// vectorReportToday — отчёт со статистикой за сегодня: Миша — играл и учился, сидит сейчас, урезан
// список сайтов (more); Коля — сегодня не садился (firstMs 0, пустые списки []); Даша — компьютер
// старой версии прислал бы её без today.
func vectorReportToday() Report {
	return Report{V: 1, T: TypeReport, PC: "pc-vector", Phone: "phone-vector", TS: 1763490006000,
		Children: []ChildReport{
			{ID: 3, Name: "Миша", UsedSec: 5400, LeftSec: 1800, ExtraMin: 15, Forbid: ForbidAhead, Today: &TodayReport{
				FirstMs: 1763463600000, NowAt: true, SinceMs: 1763485200000, UsedSec: 5400, LeftSec: 1800,
				GamesSec: 2700, StudySec: 1200, VideoSec: 900,
				Programs: []TimeItem{{Name: "Minecraft", Cat: ItemGame, Sec: 2700}, {Name: "GeoGebra Classic", Cat: ItemStudy, Sec: 1200}, {Name: "Блокнот", Cat: ItemOther, Sec: 300}},
				Sites:    []TimeItem{{Name: "youtube.com", Cat: ItemVideo, Sec: 900}, {Name: "uchi.ru", Cat: ItemStudy, Sec: 240}},
				More:     TodayMore{Programs: 0, Sites: 3}, Violations: 2, Bypass: 1}},
			{ID: 4, Name: "Коля", LeftSec: 3600, Forbid: ForbidNone, Today: &TodayReport{
				SinceMs: 1763480000000, LeftSec: 3600, Programs: []TimeItem{}, Sites: []TimeItem{}}},
			{ID: 5, Name: "Даша", LeftSec: -1, Forbid: ForbidNone},
		}}
}

func vectorNonces() ([]byte, []byte) {
	nPc, nPhone := make([]byte, NonceLen), make([]byte, NonceLen)
	for i := range nPc {
		nPc[i] = byte(i)
		nPhone[i] = byte(0xF0 - i)
	}
	return nPc, nPhone
}

// writeVectors пишет векторы. Ключи и подписи, которые уже есть в файле и не изменились по данным,
// остаются прежними: Android-тесты сверяют те же значения (ECDSA-подпись каждый раз другая).
func writeVectors(t *testing.T) {
	var v vectors
	if raw, err := os.ReadFile(vectorsPath); err == nil {
		_ = json.Unmarshal(raw, &v)
	}
	pc, err1 := ParsePriv(v.PCPriv)
	phone, err2 := ParsePriv(v.PhonePriv)
	if err1 != nil || err2 != nil {
		pc, phone = mustKey(t), mustKey(t)
		v = vectors{}
	}
	nPc, nPhone := vectorNonces()
	pcPriv, _ := MarshalPriv(pc)
	phonePriv, _ := MarshalPriv(phone)
	pcDER, _ := unb64(MarshalPub(&pc.PublicKey))
	phDER, _ := unb64(MarshalPub(&phone.PublicKey))
	// sign — подпись data ключом k; прежняя подпись остаётся, если данные те же.
	sign := func(k *ecdsa.PrivateKey, data []byte, oldJSON string, old Signed) Signed {
		if oldJSON == string(data) && old.Data != "" {
			return old
		}
		s, err := Sign(k, data)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	cmdJSON, _ := json.Marshal(vectorCommand())
	rep := vectorReport()
	repJSON, _ := json.Marshal(rep)
	c := vectorCommand()
	cmdr := CmdResult{V: 1, T: TypeCmdR, PC: c.PC, Phone: c.Phone, Seq: c.Seq, Cmd: HashData(cmdJSON), TS: rep.TS, OK: true, Message: "Добавлено 15 минут", Report: &rep}
	cmdrJSON, _ := json.Marshal(cmdr)
	noteJSON, _ := json.Marshal(vectorNote())
	repN := vectorReportNotes()
	repNJSON, _ := json.Marshal(repN)
	n := vectorNote()
	noter := CmdResult{V: 1, T: TypeNoteR, PC: n.PC, Phone: n.Phone, Seq: n.Seq, Cmd: HashData(noteJSON), TS: repN.TS, OK: true, Report: &repN}
	noterJSON, _ := json.Marshal(noter)
	nsJSON, _ := json.Marshal(vectorNoteSticker())
	nfJSON, _ := json.Marshal(vectorNoForbidden())
	repF := vectorReportForbid()
	repFJSON, _ := json.Marshal(repF)
	nf := vectorNoForbidden()
	cmdrNF := CmdResult{V: 1, T: TypeCmdR, PC: nf.PC, Phone: nf.Phone, Seq: nf.Seq, Cmd: HashData(nfJSON), TS: repF.TS, OK: true, Message: "Запрет на сегодня снят", Report: &repF}
	cmdrNFJSON, _ := json.Marshal(cmdrNF)
	repTJSON, err := fitReport(vectorReportToday(), MaxReportBody)
	if err != nil {
		t.Fatal(err)
	}
	out := vectors{
		Comment:   "Векторы протокола связи (docs/07_Протокол_связи.md). Генерирует go test ./internal/remote -run TestVectors -update. Все base64: base64url без '='.",
		PCPriv:    pcPriv,
		PCPub:     MarshalPub(&pc.PublicKey),
		PhonePriv: phonePriv,
		PhonePub:  MarshalPub(&phone.PublicKey),
		NPc:       b64(nPc), NPhone: b64(nPhone),
		Commit: b64(Commit(pcDER, phDER, nPc)), SAS: SAS(pcDER, phDER, nPc, nPhone),
		CmdJSON: string(cmdJSON), Cmd: sign(phone, cmdJSON, v.CmdJSON, v.Cmd), CmdHash: HashData(cmdJSON),
		CmdrJSON: string(cmdrJSON), Cmdr: sign(pc, cmdrJSON, v.CmdrJSON, v.Cmdr),
		ReportJSON: string(repJSON), Report: sign(pc, repJSON, v.ReportJSON, v.Report),
		NoteJSON: string(noteJSON), Note: sign(phone, noteJSON, v.NoteJSON, v.Note), NoteHash: HashData(noteJSON),
		NoterJSON: string(noterJSON), Noter: sign(pc, noterJSON, v.NoterJSON, v.Noter),
		ReportNotesJSON: string(repNJSON), ReportNotes: sign(pc, repNJSON, v.ReportNotesJSON, v.ReportNotes),
		NoteStickerJSON: string(nsJSON), NoteSticker: sign(phone, nsJSON, v.NoteStickerJSON, v.NoteSticker), NoteStickerHash: HashData(nsJSON),
		CmdNoForbiddenJSON: string(nfJSON), CmdNoForbidden: sign(phone, nfJSON, v.CmdNoForbiddenJSON, v.CmdNoForbidden),
		CmdNoForbiddenHash:  HashData(nfJSON),
		CmdrNoForbiddenJSON: string(cmdrNFJSON), CmdrNoForbidden: sign(pc, cmdrNFJSON, v.CmdrNoForbiddenJSON, v.CmdrNoForbidden),
		ReportForbidJSON: string(repFJSON), ReportForbid: sign(pc, repFJSON, v.ReportForbidJSON, v.ReportForbid),
		ReportTodayJSON: string(repTJSON), ReportToday: sign(pc, repTJSON, v.ReportTodayJSON, v.ReportToday),
	}
	raw, _ := json.MarshalIndent(out, "", "  ")
	if err := os.MkdirAll(filepath.Dir(vectorsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vectorsPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVectors(t *testing.T) {
	if *update {
		writeVectors(t)
	}
	raw, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatalf("нет векторов (go test -run TestVectors -update): %v", err)
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	pc, err := ParsePriv(v.PCPriv)
	if err != nil {
		t.Fatal(err)
	}
	phone, err := ParsePriv(v.PhonePriv)
	if err != nil {
		t.Fatal(err)
	}
	if MarshalPub(&pc.PublicKey) != v.PCPub || MarshalPub(&phone.PublicKey) != v.PhonePub {
		t.Fatal("открытые ключи не соответствуют закрытым")
	}
	pcDER, _ := unb64(v.PCPub)
	phDER, _ := unb64(v.PhonePub)
	nPc, nPhone := vectorNonces()
	if v.NPc != b64(nPc) || v.NPhone != b64(nPhone) {
		t.Fatal("случайные числа векторов изменились")
	}
	if got := b64(Commit(pcDER, phDER, nPc)); got != v.Commit {
		t.Fatalf("обязательство %s, в векторах %s", got, v.Commit)
	}
	if got := SAS(pcDER, phDER, nPc, nPhone); got != v.SAS {
		t.Fatalf("число сверки %s, в векторах %s", got, v.SAS)
	}
	cmdJSON, _ := json.Marshal(vectorCommand())
	if string(cmdJSON) != v.CmdJSON {
		t.Fatalf("сборка data команды изменилась:\n%s\n%s", cmdJSON, v.CmdJSON)
	}
	check := func(name string, s *Signed, key *ecdsa.PublicKey, want string) {
		t.Helper()
		data, err := Verify(key, *s)
		if err != nil || !bytes.Equal(data, []byte(want)) {
			t.Fatalf("%s: подпись или данные не сошлись: %v", name, err)
		}
	}
	check("cmd", &v.Cmd, &phone.PublicKey, v.CmdJSON)
	check("cmdr", &v.Cmdr, &pc.PublicKey, v.CmdrJSON)
	check("report", &v.Report, &pc.PublicKey, v.ReportJSON)
	if HashData([]byte(v.CmdJSON)) != v.CmdHash {
		t.Fatal("хэш команды не сошёлся")
	}
	var cr CmdResult
	if err := json.Unmarshal([]byte(v.CmdrJSON), &cr); err != nil || cr.Cmd != v.CmdHash || cr.Report == nil {
		t.Fatalf("cmdr: %+v %v", cr, err)
	}
	// Сообщение (раздел 5а): data байт в байт, подпись телефона, хэш, ответ noter и отчёт с сообщениями.
	noteJSON, _ := json.Marshal(vectorNote())
	if string(noteJSON) != v.NoteJSON {
		t.Fatalf("сборка data сообщения изменилась:\n%s\n%s", noteJSON, v.NoteJSON)
	}
	check("note", &v.Note, &phone.PublicKey, v.NoteJSON)
	check("noter", &v.Noter, &pc.PublicKey, v.NoterJSON)
	check("reportNotes", &v.ReportNotes, &pc.PublicKey, v.ReportNotesJSON)
	if HashData([]byte(v.NoteJSON)) != v.NoteHash {
		t.Fatal("хэш сообщения не сошёлся")
	}
	var nr CmdResult
	if err := json.Unmarshal([]byte(v.NoterJSON), &nr); err != nil || nr.T != TypeNoteR || nr.Cmd != v.NoteHash || nr.Report == nil || len(nr.Report.Notes) != 1 {
		t.Fatalf("noter: %+v %v", nr, err)
	}
	stN := State{PCID: "pc-vector", Phone: &Phone{ID: "phone-vector", Pub: v.PhonePub}, LastSeq: 17}
	if n, _, code := CheckNote(stN, v.Note, "phone-vector", time.UnixMilli(vectorNote().TS)); code != "" || n.Text != "Иди ужинать\nПотом уроки" {
		t.Fatalf("сообщение векторов отклонено: %q %+v", code, n)
	}
	// Сообщение со стикером и звуком (FB-28): поля после showSec, подпись телефона, хэш, разбор.
	nsJSON, _ := json.Marshal(vectorNoteSticker())
	if string(nsJSON) != v.NoteStickerJSON || v.NoteStickerJSON != `{"v":1,"t":"note","pc":"pc-vector","phone":"phone-vector","seq":20,"ts":1763490007000,"child":3,"id":"EBESExQVFhcYGRobHB0eHw","text":"Иди ужинать","showSec":30,"sticker":"033","sound":2}` {
		t.Fatalf("сборка data сообщения со стикером изменилась:\n%s\n%s", nsJSON, v.NoteStickerJSON)
	}
	check("noteSticker", &v.NoteSticker, &phone.PublicKey, v.NoteStickerJSON)
	if HashData([]byte(v.NoteStickerJSON)) != v.NoteStickerHash {
		t.Fatal("хэш сообщения со стикером не сошёлся")
	}
	stS := State{PCID: "pc-vector", Phone: &Phone{ID: "phone-vector", Pub: v.PhonePub}, LastSeq: 19}
	if n, _, code := CheckNote(stS, v.NoteSticker, "phone-vector", time.UnixMilli(vectorNoteSticker().TS)); code != "" || n.Sticker != "033" || n.Sound != 2 || n.ShowSec != 30 {
		t.Fatalf("сообщение со стикером отклонено: %q %+v", code, n)
	}
	// Сообщение 0.8.2 без полей: без стикера, звук 1.
	if n, _, _ := CheckNote(stN, v.Note, "phone-vector", time.UnixMilli(vectorNote().TS)); n.Sticker != "" || n.Sound != 1 {
		t.Fatalf("сообщение без стикера: %+v", n)
	}
	// «Снять запрет» (раздел 5б): data байт в байт, подписи, хэш, ответ с отчётом всех состояний.
	nfJSON, _ := json.Marshal(vectorNoForbidden())
	if string(nfJSON) != v.CmdNoForbiddenJSON || v.CmdNoForbiddenJSON != `{"v":1,"t":"cmd","pc":"pc-vector","phone":"phone-vector","seq":19,"ts":1763490004000,"child":3,"op":"noForbidden"}` {
		t.Fatalf("сборка data «снять запрет» изменилась:\n%s\n%s", nfJSON, v.CmdNoForbiddenJSON)
	}
	check("cmdNoForbidden", &v.CmdNoForbidden, &phone.PublicKey, v.CmdNoForbiddenJSON)
	check("cmdrNoForbidden", &v.CmdrNoForbidden, &pc.PublicKey, v.CmdrNoForbiddenJSON)
	check("reportForbid", &v.ReportForbid, &pc.PublicKey, v.ReportForbidJSON)
	if HashData([]byte(v.CmdNoForbiddenJSON)) != v.CmdNoForbiddenHash {
		t.Fatal("хэш «снять запрет» не сошёлся")
	}
	var nfr CmdResult
	if err := json.Unmarshal([]byte(v.CmdrNoForbiddenJSON), &nfr); err != nil || nfr.Cmd != v.CmdNoForbiddenHash || nfr.Report == nil || len(nfr.Report.Children) != 4 || nfr.Report.Children[1].ForbidUntilMs == 0 {
		t.Fatalf("cmdr «снять запрет»: %+v %v", nfr, err)
	}
	stF := State{PCID: "pc-vector", Phone: &Phone{ID: "phone-vector", Pub: v.PhonePub}, LastSeq: 18}
	if c, _, code := CheckCommand(stF, v.CmdNoForbidden, "phone-vector", time.UnixMilli(vectorNoForbidden().TS)); code != "" || c.Op != OpNoForbidden {
		t.Fatalf("«снять запрет» векторов отклонено: %q %+v", code, c)
	}
	// Статистика за сегодня (раздел 6): байты — как отчёт после fitReport, подпись компьютера, поля.
	repTJSON, _ := fitReport(vectorReportToday(), MaxReportBody)
	if string(repTJSON) != v.ReportTodayJSON {
		t.Fatalf("сборка отчёта со статистикой изменилась:\n%s\n%s", repTJSON, v.ReportTodayJSON)
	}
	check("reportToday", &v.ReportToday, &pc.PublicKey, v.ReportTodayJSON)
	var rt Report
	if err := json.Unmarshal([]byte(v.ReportTodayJSON), &rt); err != nil || len(rt.Children) != 3 || rt.Children[0].Today == nil ||
		rt.Children[0].Today.Programs[0].Cat != ItemGame || rt.Children[1].Today.FirstMs != 0 || rt.Children[2].Today != nil ||
		!bytes.Contains([]byte(v.ReportTodayJSON), []byte(`"programs":[],"sites":[]`)) {
		t.Fatalf("отчёт со статистикой: %s %v", v.ReportTodayJSON, err)
	}
	// Команда векторов проходит проверку компьютера.
	st := State{PCID: "pc-vector", Phone: &Phone{ID: "phone-vector", Pub: v.PhonePub}, LastSeq: 16}
	if _, _, code := CheckCommand(st, v.Cmd, "phone-vector", time.UnixMilli(vectorCommand().TS)); code != "" {
		t.Fatalf("команда векторов отклонена: %s", code)
	}
}
