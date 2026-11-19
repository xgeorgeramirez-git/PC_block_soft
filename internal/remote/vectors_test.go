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
}

func vectorCommand() Command {
	return Command{V: 1, T: TypeCmd, PC: "pc-vector", Phone: "phone-vector", Seq: 17, TS: 1763490000000, Child: 3, Delta: 15}
}

func vectorReport() Report {
	return Report{V: 1, T: TypeReport, PC: "pc-vector", Phone: "phone-vector", TS: 1763490001000,
		Children: []ChildReport{{ID: 3, Name: "Миша", UsedSec: 3600, LeftSec: 1800, ExtraMin: 15}}}
}

func vectorNonces() ([]byte, []byte) {
	nPc, nPhone := make([]byte, NonceLen), make([]byte, NonceLen)
	for i := range nPc {
		nPc[i] = byte(i)
		nPhone[i] = byte(0xF0 - i)
	}
	return nPc, nPhone
}

func writeVectors(t *testing.T) {
	pc, phone := mustKey(t), mustKey(t)
	nPc, nPhone := vectorNonces()
	pcPriv, _ := MarshalPriv(pc)
	phonePriv, _ := MarshalPriv(phone)
	pcDER, _ := unb64(MarshalPub(&pc.PublicKey))
	phDER, _ := unb64(MarshalPub(&phone.PublicKey))
	cmdJSON, _ := json.Marshal(vectorCommand())
	cmdSig, err := Sign(phone, cmdJSON)
	if err != nil {
		t.Fatal(err)
	}
	rep := vectorReport()
	repJSON, _ := json.Marshal(rep)
	repSig, _ := Sign(pc, repJSON)
	c := vectorCommand()
	cmdr := CmdResult{V: 1, T: TypeCmdR, PC: c.PC, Phone: c.Phone, Seq: c.Seq, Cmd: HashData(cmdJSON), TS: rep.TS, OK: true, Message: "Добавлено 15 минут", Report: &rep}
	cmdrJSON, _ := json.Marshal(cmdr)
	cmdrSig, _ := Sign(pc, cmdrJSON)
	v := vectors{
		Comment:   "Векторы протокола связи (docs/07_Протокол_связи.md). Генерирует go test ./internal/remote -run TestVectors -update. Все base64: base64url без '='.",
		PCPriv:    pcPriv,
		PCPub:     MarshalPub(&pc.PublicKey),
		PhonePriv: phonePriv,
		PhonePub:  MarshalPub(&phone.PublicKey),
		NPc:       b64(nPc), NPhone: b64(nPhone),
		Commit: b64(Commit(pcDER, phDER, nPc)), SAS: SAS(pcDER, phDER, nPc, nPhone),
		CmdJSON: string(cmdJSON), Cmd: cmdSig, CmdHash: HashData(cmdJSON),
		CmdrJSON: string(cmdrJSON), Cmdr: cmdrSig,
		ReportJSON: string(repJSON), Report: repSig,
	}
	raw, _ := json.MarshalIndent(v, "", "  ")
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
	// Команда векторов проходит проверку компьютера.
	st := State{PCID: "pc-vector", Phone: &Phone{ID: "phone-vector", Pub: v.PhonePub}, LastSeq: 16}
	if _, _, code := CheckCommand(st, v.Cmd, "phone-vector", time.UnixMilli(vectorCommand().TS)); code != "" {
		t.Fatalf("команда векторов отклонена: %s", code)
	}
}
