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

	// Протокол 1.5 (разделы 5в, 5г, 5д, 6): команды копилки, промокодов и решения (data байт в байт,
	// подпись ключом телефона, хэш для cmdr), ответ на promoGen со списком и кодами и отчёт с
	// bankMin, играми и видео и pending (подписи ключом компьютера). PendingKey — id строки pending
	// для PendingPath (первые 16 байт SHA-256 от UTF-8 пути, base64url).
	CmdBankAddJSON     string `json:"cmdBankAddJson"`
	CmdBankAdd         Signed `json:"cmdBankAdd"`
	CmdBankAddHash     string `json:"cmdBankAddHash"`
	CmdBankClearJSON   string `json:"cmdBankClearJson"`
	CmdBankClear       Signed `json:"cmdBankClear"`
	CmdBankClearHash   string `json:"cmdBankClearHash"`
	CmdPromoListJSON   string `json:"cmdPromoListJson"`
	CmdPromoList       Signed `json:"cmdPromoList"`
	CmdPromoListHash   string `json:"cmdPromoListHash"`
	CmdPromoCreateJSON string `json:"cmdPromoCreateJson"`
	CmdPromoCreate     Signed `json:"cmdPromoCreate"`
	CmdPromoCreateHash string `json:"cmdPromoCreateHash"`
	CmdPromoGenJSON    string `json:"cmdPromoGenJson"`
	CmdPromoGen        Signed `json:"cmdPromoGen"`
	CmdPromoGenHash    string `json:"cmdPromoGenHash"`
	CmdPromoDeleteJSON string `json:"cmdPromoDeleteJson"`
	CmdPromoDelete     Signed `json:"cmdPromoDelete"`
	CmdPromoDeleteHash string `json:"cmdPromoDeleteHash"`
	PendingPath        string `json:"pendingPath"`
	PendingKey         string `json:"pendingKey"`
	CmdDecideJSON      string `json:"cmdDecideJson"`
	CmdDecide          Signed `json:"cmdDecide"`
	CmdDecideHash      string `json:"cmdDecideHash"`
	CmdrPromosJSON     string `json:"cmdrPromosJson"`
	CmdrPromos         Signed `json:"cmdrPromos"`
	ReportPendingJSON  string `json:"reportPendingJson"`
	ReportPending      Signed `json:"reportPending"`
}

// vectorPendingPath — путь ждущей программы векторов (ключ решения службы).
const vectorPendingPath = `C:\Users\Misha\Downloads\SuperGame\game.exe`

// vectorOps — команды протокола 1.5 векторов по порядку: копилка, промокоды, решение (seq 20…26).
// Поля разделов 5г, 5д — сырые, как их пишет телефон.
func vectorOps() map[string]Command {
	c := func(seq, ts, child int64, op string) Command {
		return Command{V: 1, T: TypeCmd, PC: "pc-vector", Phone: "phone-vector", Seq: seq, TS: ts, Child: child, Op: op}
	}
	raw := func(s string) json.RawMessage { return json.RawMessage(s) }
	add := c(20, 1763490005000, 3, OpBankAdd)
	add.Delta = 30
	create := c(23, 1763490008000, 0, OpPromoCreate)
	create.CodeRaw, create.MinRaw = raw(`"SUMMER"`), raw(`30`)
	gen := c(24, 1763490009000, 0, OpPromoGen)
	gen.MinRaw, gen.CountRaw, gen.LenRaw, gen.CharsetRaw = raw(`15`), raw(`5`), raw(`6`), raw(`"mixed"`)
	del := c(25, 1763490010000, 0, OpPromoDelete)
	del.CodeRaw = raw(`"SUMMER"`)
	dec := c(26, 1763490011000, 0, OpDecide)
	dec.KeyRaw, dec.CatRaw = raw(`"`+PendingKey(vectorPendingPath)+`"`), raw(`"games"`)
	return map[string]Command{
		"bankAdd": add, "bankClear": c(21, 1763490006000, 3, OpBankClear), "promoList": c(22, 1763490007000, 0, OpPromoList),
		"promoCreate": create, "promoGen": gen, "promoDelete": del, "decide": dec,
	}
}

// vectorOpsJSON — ожидаемые байты data команд 1.5 (литералы плана T2.5: порядок полей — порядок
// структуры Command; Kotlin собирает так же).
func vectorOpsJSON() map[string]string {
	return map[string]string{
		"bankAdd":     `{"v":1,"t":"cmd","pc":"pc-vector","phone":"phone-vector","seq":20,"ts":1763490005000,"child":3,"delta":30,"op":"bankAdd"}`,
		"bankClear":   `{"v":1,"t":"cmd","pc":"pc-vector","phone":"phone-vector","seq":21,"ts":1763490006000,"child":3,"op":"bankClear"}`,
		"promoList":   `{"v":1,"t":"cmd","pc":"pc-vector","phone":"phone-vector","seq":22,"ts":1763490007000,"child":0,"op":"promoList"}`,
		"promoCreate": `{"v":1,"t":"cmd","pc":"pc-vector","phone":"phone-vector","seq":23,"ts":1763490008000,"child":0,"op":"promoCreate","code":"SUMMER","min":30}`,
		"promoGen":    `{"v":1,"t":"cmd","pc":"pc-vector","phone":"phone-vector","seq":24,"ts":1763490009000,"child":0,"op":"promoGen","min":15,"count":5,"len":6,"charset":"mixed"}`,
		"promoDelete": `{"v":1,"t":"cmd","pc":"pc-vector","phone":"phone-vector","seq":25,"ts":1763490010000,"child":0,"op":"promoDelete","code":"SUMMER"}`,
		"decide":      `{"v":1,"t":"cmd","pc":"pc-vector","phone":"phone-vector","seq":26,"ts":1763490011000,"child":0,"op":"decide","key":"TgOw4J51Seo0S6yK66bHgw","cat":"games"}`,
	}
}

// vectorCmdrPromos — ответ на promoGen векторов: новые коды и список (SUMMER действует, AB12
// использовал Миша).
func vectorCmdrPromos(cmdHash string) CmdResult {
	return CmdResult{V: 1, T: TypeCmdR, PC: "pc-vector", Phone: "phone-vector", Seq: 24, Cmd: cmdHash, TS: 1763490009500, OK: true,
		Message: "Готово: 2 кода на 15 мин", Codes: []string{"K7PX", "M3QA"},
		Promos: &PromoList{
			Active: []PromoActive{{Code: "SUMMER", Min: 30, CreatedMs: 1763490000000}},
			Used:   []PromoUsed{{Code: "AB12", Min: 30, UsedMs: 1763480000000, Child: 3, ChildName: "Миша"}},
		}}
}

// vectorReportPending — отчёт 1.5: Миша — копилка 45, игры 40 из 60 мин, видео 10 из 30 мин;
// Коля — без лимита видео (−1); одна ждущая без издателя, Миша запускал 3 раза.
func vectorReportPending() Report {
	return Report{V: 1, T: TypeReport, PC: "pc-vector", Phone: "phone-vector", TS: 1763490012000,
		Children: []ChildReport{
			{ID: 3, Name: "Миша", UsedSec: 3600, LeftSec: 1800, ExtraMin: 15, Forbid: ForbidNone,
				BankMin: 45, GamesSec: 2400, GamesLimSec: 3600, VideoSec: 600, VideoLimSec: 1800},
			{ID: 4, Name: "Коля", UsedSec: 600, LeftSec: 3000, Forbid: ForbidNone,
				BankMin: 0, GamesSec: 300, GamesLimSec: -1, VideoSec: 0, VideoLimSec: -1},
		},
		Pending: []PendingReport{{ID: PendingKey(vectorPendingPath), Name: "SuperGame", Pub: "", Path: vectorPendingPath,
			Kids: []PendingKid{{ID: 3, Tries: 3}}, Tries: 3, LastMs: 1763488000000}},
		PendingCount: 1}
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
	// Отчёты и ответы до протокола 1.5 — байты компьютера 0.8.3 (без bankMin, pending): Android
	// проверяет по ним «компьютер старше 0.8.4». Уже записанные не пересобираются.
	legacy := func(old string, built []byte) []byte {
		if old != "" {
			return []byte(old)
		}
		return built
	}
	cmdrJSON = legacy(v.CmdrJSON, cmdrJSON)
	repJSON = legacy(v.ReportJSON, repJSON)
	noterJSON = legacy(v.NoterJSON, noterJSON)
	repNJSON = legacy(v.ReportNotesJSON, repNJSON)
	cmdrNFJSON = legacy(v.CmdrNoForbiddenJSON, cmdrNFJSON)
	repFJSON = legacy(v.ReportForbidJSON, repFJSON)
	repTJSON = legacy(v.ReportTodayJSON, repTJSON)
	ops := vectorOps()
	opJSON := map[string][]byte{}
	for name, c := range ops {
		opJSON[name], _ = json.Marshal(c)
	}
	cmdrP, err := fitReply(vectorCmdrPromos(HashData(opJSON["promoGen"])), KindCmdR, "msg-vector", "phone-vector", MaxReportBody)
	if err != nil {
		t.Fatal(err)
	}
	repP, err := fitReport(vectorReportPending(), MaxReportBody)
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

		CmdBankAddJSON: string(opJSON["bankAdd"]), CmdBankAdd: sign(phone, opJSON["bankAdd"], v.CmdBankAddJSON, v.CmdBankAdd), CmdBankAddHash: HashData(opJSON["bankAdd"]),
		CmdBankClearJSON: string(opJSON["bankClear"]), CmdBankClear: sign(phone, opJSON["bankClear"], v.CmdBankClearJSON, v.CmdBankClear), CmdBankClearHash: HashData(opJSON["bankClear"]),
		CmdPromoListJSON: string(opJSON["promoList"]), CmdPromoList: sign(phone, opJSON["promoList"], v.CmdPromoListJSON, v.CmdPromoList), CmdPromoListHash: HashData(opJSON["promoList"]),
		CmdPromoCreateJSON: string(opJSON["promoCreate"]), CmdPromoCreate: sign(phone, opJSON["promoCreate"], v.CmdPromoCreateJSON, v.CmdPromoCreate), CmdPromoCreateHash: HashData(opJSON["promoCreate"]),
		CmdPromoGenJSON: string(opJSON["promoGen"]), CmdPromoGen: sign(phone, opJSON["promoGen"], v.CmdPromoGenJSON, v.CmdPromoGen), CmdPromoGenHash: HashData(opJSON["promoGen"]),
		CmdPromoDeleteJSON: string(opJSON["promoDelete"]), CmdPromoDelete: sign(phone, opJSON["promoDelete"], v.CmdPromoDeleteJSON, v.CmdPromoDelete), CmdPromoDeleteHash: HashData(opJSON["promoDelete"]),
		PendingPath: vectorPendingPath, PendingKey: PendingKey(vectorPendingPath),
		CmdDecideJSON: string(opJSON["decide"]), CmdDecide: sign(phone, opJSON["decide"], v.CmdDecideJSON, v.CmdDecide), CmdDecideHash: HashData(opJSON["decide"]),
		CmdrPromosJSON: string(cmdrP), CmdrPromos: sign(pc, cmdrP, v.CmdrPromosJSON, v.CmdrPromos),
		ReportPendingJSON: string(repP), ReportPending: sign(pc, repP, v.ReportPendingJSON, v.ReportPending),
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
	// Статистика за сегодня (раздел 6): байты компьютера 0.8.3 (без bankMin и pending — признак
	// старого компьютера для телефона 0.8.4), подпись компьютера, поля. Сборку fitReport с полями 1.5
	// проверяет вектор reportPending.
	check("reportToday", &v.ReportToday, &pc.PublicKey, v.ReportTodayJSON)
	for _, old := range []string{v.ReportTodayJSON, v.ReportJSON, v.ReportForbidJSON, v.ReportNotesJSON, v.CmdrJSON, v.NoterJSON, v.CmdrNoForbiddenJSON} {
		if bytes.Contains([]byte(old), []byte(`"bankMin"`)) || bytes.Contains([]byte(old), []byte(`"pending"`)) {
			t.Fatalf("вектор компьютера 0.8.3 с полями 1.5: %.120s", old)
		}
	}
	var rt Report
	if err := json.Unmarshal([]byte(v.ReportTodayJSON), &rt); err != nil || len(rt.Children) != 3 || rt.Children[0].Today == nil ||
		rt.Children[0].Today.Programs[0].Cat != ItemGame || rt.Children[1].Today.FirstMs != 0 || rt.Children[2].Today != nil ||
		!bytes.Contains([]byte(v.ReportTodayJSON), []byte(`"programs":[],"sites":[]`)) {
		t.Fatalf("отчёт со статистикой: %s %v", v.ReportTodayJSON, err)
	}
	// Протокол 1.5: data команд байт в байт (литералы), подписи телефона, хэши, разбор компьютером.
	if v.PendingPath != vectorPendingPath || v.PendingKey != PendingKey(vectorPendingPath) || v.PendingKey != "TgOw4J51Seo0S6yK66bHgw" {
		t.Fatalf("ключ pending: %q для %q", v.PendingKey, v.PendingPath)
	}
	wantOps := vectorOpsJSON()
	stored := map[string][3]string{
		"bankAdd":     {v.CmdBankAddJSON, v.CmdBankAddHash},
		"bankClear":   {v.CmdBankClearJSON, v.CmdBankClearHash},
		"promoList":   {v.CmdPromoListJSON, v.CmdPromoListHash},
		"promoCreate": {v.CmdPromoCreateJSON, v.CmdPromoCreateHash},
		"promoGen":    {v.CmdPromoGenJSON, v.CmdPromoGenHash},
		"promoDelete": {v.CmdPromoDeleteJSON, v.CmdPromoDeleteHash},
		"decide":      {v.CmdDecideJSON, v.CmdDecideHash},
	}
	signedOps := map[string]*Signed{
		"bankAdd": &v.CmdBankAdd, "bankClear": &v.CmdBankClear, "promoList": &v.CmdPromoList, "promoCreate": &v.CmdPromoCreate,
		"promoGen": &v.CmdPromoGen, "promoDelete": &v.CmdPromoDelete, "decide": &v.CmdDecide,
	}
	parsed := map[string]Command{}
	for name, c := range vectorOps() {
		built, _ := json.Marshal(c)
		if string(built) != wantOps[name] || stored[name][0] != wantOps[name] {
			t.Fatalf("%s: сборка data изменилась:\n%s\n%s\n%s", name, built, stored[name][0], wantOps[name])
		}
		check(name, signedOps[name], &phone.PublicKey, wantOps[name])
		if HashData(built) != stored[name][1] {
			t.Fatalf("%s: хэш не сошёлся", name)
		}
		stOp := State{PCID: "pc-vector", Phone: &Phone{ID: "phone-vector", Pub: v.PhonePub}, LastSeq: 19}
		got, _, code := CheckCommand(stOp, *signedOps[name], "phone-vector", time.UnixMilli(c.TS))
		if code != "" {
			t.Fatalf("%s: команда векторов отклонена: %s", name, code)
		}
		parsed[name] = got
	}
	if p := parsed["bankAdd"]; p.Op != OpBankAdd || p.Delta != 30 || p.Child != 3 {
		t.Fatalf("bankAdd: %+v", p)
	}
	if p := parsed["promoCreate"]; p.Code != "SUMMER" || p.Min != 30 {
		t.Fatalf("promoCreate: %+v", p)
	}
	if p := parsed["promoGen"]; p.Count != 5 || p.Len != 6 || p.Charset != CharsetMixed || p.Min != 15 {
		t.Fatalf("promoGen: %+v", p)
	}
	if p := parsed["decide"]; p.Key != v.PendingKey || p.Cat != CatGames {
		t.Fatalf("decide: %+v", p)
	}
	// Ответ на promoGen: байты — как ответ после fitReply, подпись компьютера, поля.
	cmdrP, _ := fitReply(vectorCmdrPromos(v.CmdPromoGenHash), KindCmdR, "msg-vector", "phone-vector", MaxReportBody)
	if string(cmdrP) != v.CmdrPromosJSON {
		t.Fatalf("сборка ответа promoGen изменилась:\n%s\n%s", cmdrP, v.CmdrPromosJSON)
	}
	check("cmdrPromos", &v.CmdrPromos, &pc.PublicKey, v.CmdrPromosJSON)
	if !bytes.Contains([]byte(v.CmdrPromosJSON), []byte(`"codes":["K7PX","M3QA"],"promos":{"active":[{"code":"SUMMER","min":30,"createdMs":1763490000000}],"used":[{"code":"AB12","min":30,"usedMs":1763480000000,"child":3,"childName":"Миша"}],"moreActive":0,"moreUsed":0}}`)) ||
		bytes.Contains([]byte(v.CmdrPromosJSON), []byte(`"report"`)) {
		t.Fatalf("ответ promoGen: %s", v.CmdrPromosJSON)
	}
	// Отчёт 1.5: байты — как после fitReport, подпись компьютера, поля.
	repP, _ := fitReport(vectorReportPending(), MaxReportBody)
	if string(repP) != v.ReportPendingJSON {
		t.Fatalf("сборка отчёта 1.5 изменилась:\n%s\n%s", repP, v.ReportPendingJSON)
	}
	check("reportPending", &v.ReportPending, &pc.PublicKey, v.ReportPendingJSON)
	for _, part := range []string{
		`"bankMin":45,"gamesSec":2400,"gamesLimSec":3600,"videoSec":600,"videoLimSec":1800}`,
		`"bankMin":0,"gamesSec":300,"gamesLimSec":-1,"videoSec":0,"videoLimSec":-1}`,
		`"pending":[{"id":"TgOw4J51Seo0S6yK66bHgw","name":"SuperGame","pub":"","path":"C:\\Users\\Misha\\Downloads\\SuperGame\\game.exe","kids":[{"id":3,"tries":3}],"tries":3,"lastMs":1763488000000}],"pendingCount":1}`,
	} {
		if !bytes.Contains([]byte(v.ReportPendingJSON), []byte(part)) {
			t.Fatalf("отчёт 1.5 без %s:\n%s", part, v.ReportPendingJSON)
		}
	}
	// Команда векторов проходит проверку компьютера.
	st := State{PCID: "pc-vector", Phone: &Phone{ID: "phone-vector", Pub: v.PhonePub}, LastSeq: 16}
	if _, _, code := CheckCommand(st, v.Cmd, "phone-vector", time.UnixMilli(vectorCommand().TS)); code != "" {
		t.Fatalf("команда векторов отклонена: %s", code)
	}
}
