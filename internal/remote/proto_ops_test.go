package remote

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// Команды протокола 1.5 (разделы 5в, 5г, 5д): копилка, промокоды и решения о программах.

// opsFixture — привязанный телефон и подпись данных с дописанными сырыми полями (вид полей
// проверяется на байтах, а не на структуре: строка вместо числа и т. п.).
type opsFixture struct {
	t   *testing.T
	st  State
	now time.Time
	key func([]byte) Signed
}

func newOpsFixture(t *testing.T) opsFixture {
	phone := mustKey(t)
	f := opsFixture{
		t:   t,
		st:  State{PCID: "pc1", Phone: &Phone{ID: "ph1", Pub: MarshalPub(&phone.PublicKey)}, LastSeq: 16},
		now: time.UnixMilli(1763490000000),
	}
	f.key = func(raw []byte) Signed {
		s, err := Sign(phone, raw)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	return f
}

// data — данные команды: заголовок с seq и child, затем поля fields как есть (`"op":"bankAdd","delta":30`).
func (f opsFixture) data(seq, child int64, fields string) []byte {
	head := `{"v":1,"t":"cmd","pc":"pc1","phone":"ph1","seq":` + itoa(seq) + `,"ts":1763490000000,"child":` + itoa(child)
	if fields != "" {
		head += "," + fields
	}
	return []byte(head + "}")
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func (f opsFixture) check(seq, child int64, fields string) (Command, string) {
	c, _, code := CheckCommand(f.st, f.key(f.data(seq, child, fields)), "ph1", f.now)
	return c, code
}

// Ключ строки «Ждут решения» для векторов и тестов (независимо от PendingKey).
func manualKey(path string) string {
	sum := sha256.Sum256([]byte(path))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

// Ловит: верная команда нового op отклонена; поля не разобраны (код не обрезан и не заглавными,
// числа не прочитаны); delta у bankAdd не дошла до Host.
func TestCheckCommandOpsValid(t *testing.T) {
	f := newOpsFixture(t)
	key := manualKey(`C:\Users\Misha\Downloads\SuperGame\game.exe`)
	cases := []struct {
		name   string
		child  int64
		fields string
		want   Command
	}{
		{"bankAdd 30", 3, `"delta":30,"op":"bankAdd"`, Command{Op: "bankAdd", Delta: 30}},
		{"bankAdd 1", 3, `"op":"bankAdd","delta":1`, Command{Op: "bankAdd", Delta: 1}},
		{"bankAdd 600", 3, `"op":"bankAdd","delta":600`, Command{Op: "bankAdd", Delta: 600}},
		{"bankClear", 3, `"op":"bankClear"`, Command{Op: "bankClear"}},
		{"bankClear delta 0", 3, `"delta":0,"op":"bankClear"`, Command{Op: "bankClear"}},
		{"promoList", 0, `"op":"promoList"`, Command{Op: "promoList"}},
		{"promoCreate", 0, `"op":"promoCreate","code":" ab12 ","min":30`, Command{Op: "promoCreate", Code: "AB12", Min: 30}},
		{"promoCreate 16 знаков 600", 0, `"op":"promoCreate","code":"ABCDEFGHIJKLMNOP","min":600`, Command{Op: "promoCreate", Code: "ABCDEFGHIJKLMNOP", Min: 600}},
		{"promoGen", 0, `"op":"promoGen","min":15,"count":5,"len":6,"charset":"mixed"`, Command{Op: "promoGen", Min: 15, Count: 5, Len: 6, Charset: "mixed"}},
		{"promoGen края", 0, `"op":"promoGen","min":1,"count":50,"len":12,"charset":"digits"`, Command{Op: "promoGen", Min: 1, Count: 50, Len: 12, Charset: "digits"}},
		{"promoGen буквы", 0, `"op":"promoGen","min":600,"count":1,"len":4,"charset":"letters"`, Command{Op: "promoGen", Min: 600, Count: 1, Len: 4, Charset: "letters"}},
		{"promoDelete", 0, `"op":"promoDelete","code":"SUMMER"`, Command{Op: "promoDelete", Code: "SUMMER"}},
		{"promoDelete старый код", 0, `"op":"promoDelete","code":"  лето2025 "`, Command{Op: "promoDelete", Code: "лето2025"}},
		{"promoDelete 64 знака", 0, `"op":"promoDelete","code":"` + strings.Repeat("я", 64) + `"`, Command{Op: "promoDelete", Code: strings.Repeat("я", 64)}},
		{"decide games", 0, `"op":"decide","key":"` + key + `","cat":"games"`, Command{Op: "decide", Key: key, Cat: "games"}},
		{"decide study", 0, `"op":"decide","key":"` + key + `","cat":"study"`, Command{Op: "decide", Key: key, Cat: "study"}},
		{"decide other", 0, `"op":"decide","key":"` + key + `","cat":"other"`, Command{Op: "decide", Key: key, Cat: "other"}},
		{"decide blocked", 0, `"op":"decide","key":"` + key + `","cat":"blocked"`, Command{Op: "decide", Key: key, Cat: "blocked"}},
	}
	for _, c := range cases {
		cmd, code := f.check(17, c.child, c.fields)
		if code != "" {
			t.Errorf("%s: код %q", c.name, code)
			continue
		}
		got := Command{Op: cmd.Op, Delta: cmd.Delta, Code: cmd.Code, Min: cmd.Min, Count: cmd.Count, Len: cmd.Len, Charset: cmd.Charset, Key: cmd.Key, Cat: cmd.Cat}
		if got.Op != c.want.Op || got.Delta != c.want.Delta || got.Code != c.want.Code || got.Min != c.want.Min ||
			got.Count != c.want.Count || got.Len != c.want.Len || got.Charset != c.want.Charset || got.Key != c.want.Key || got.Cat != c.want.Cat {
			t.Errorf("%s: разобрано %+v, нужно %+v", c.name, got, c.want)
		}
		if cmd.Seq != 17 || cmd.Child != c.child {
			t.Errorf("%s: seq %d child %d", c.name, cmd.Seq, cmd.Child)
		}
	}
}

// Ловит (PL-85): неверный вид нового поля валит разбор подписанных данных (bad_sig вместо range);
// отказ по виду проверяется после seq (старый номер — replay вместо range, номер «тратится» на
// неверную команду); пределы 5в–5д не проверены до Host.
func TestCheckCommandOpsRangeKeepsSeq(t *testing.T) {
	f := newOpsFixture(t)
	key := manualKey(`C:\x.exe`)
	key21 := base64.RawURLEncoding.EncodeToString(make([]byte, 21))
	bad := []struct {
		name   string
		child  int64
		fields string
	}{
		{"min строкой", 0, `"op":"promoCreate","code":"SUMMER","min":"30"`},
		{"min дробное", 0, `"op":"promoCreate","code":"SUMMER","min":30.5`},
		{"min 30.0", 0, `"op":"promoCreate","code":"SUMMER","min":30.0`},
		{"min степенью", 0, `"op":"promoCreate","code":"SUMMER","min":3e1`},
		{"min null", 0, `"op":"promoCreate","code":"SUMMER","min":null`},
		{"min массив", 0, `"op":"promoCreate","code":"SUMMER","min":[]`},
		{"min отрицательное", 0, `"op":"promoCreate","code":"SUMMER","min":-30`},
		{"min нет", 0, `"op":"promoCreate","code":"SUMMER"`},
		{"min 0", 0, `"op":"promoCreate","code":"SUMMER","min":0`},
		{"min 601", 0, `"op":"promoCreate","code":"SUMMER","min":601`},
		{"code нет", 0, `"op":"promoCreate","min":30`},
		{"code число", 0, `"op":"promoCreate","code":1234,"min":30`},
		{"code null", 0, `"op":"promoCreate","code":null,"min":30`},
		{"code кириллица", 0, `"op":"promoCreate","code":"ПРИВЕТ","min":30`},
		{"code 2 знака", 0, `"op":"promoCreate","code":"AB","min":30`},
		{"code 17 знаков", 0, `"op":"promoCreate","code":"ABCDEFGHIJKLMNOPQ","min":30`},
		{"code с пробелом", 0, `"op":"promoCreate","code":"AB 12","min":30`},
		{"promoCreate child 3", 3, `"op":"promoCreate","code":"SUMMER","min":30`},
		{"promoCreate delta", 0, `"delta":5,"op":"promoCreate","code":"SUMMER","min":30`},
		{"bankAdd 0", 3, `"op":"bankAdd"`},
		{"bankAdd 601", 3, `"op":"bankAdd","delta":601`},
		{"bankAdd -1", 3, `"op":"bankAdd","delta":-1`},
		{"bankClear delta 5", 3, `"op":"bankClear","delta":5`},
		{"promoList child 3", 3, `"op":"promoList"`},
		{"promoList delta", 0, `"op":"promoList","delta":1`},
		{"count 0", 0, `"op":"promoGen","min":15,"count":0,"len":6,"charset":"mixed"`},
		{"count 51", 0, `"op":"promoGen","min":15,"count":51,"len":6,"charset":"mixed"`},
		{"count строкой", 0, `"op":"promoGen","min":15,"count":"5","len":6,"charset":"mixed"`},
		{"count нет", 0, `"op":"promoGen","min":15,"len":6,"charset":"mixed"`},
		{"len 3", 0, `"op":"promoGen","min":15,"count":5,"len":3,"charset":"mixed"`},
		{"len 13", 0, `"op":"promoGen","min":15,"count":5,"len":13,"charset":"mixed"`},
		{"len дробное", 0, `"op":"promoGen","min":15,"count":5,"len":6.5,"charset":"mixed"`},
		{"charset hex", 0, `"op":"promoGen","min":15,"count":5,"len":6,"charset":"hex"`},
		{"charset нет", 0, `"op":"promoGen","min":15,"count":5,"len":6`},
		{"charset число", 0, `"op":"promoGen","min":15,"count":5,"len":6,"charset":1`},
		{"promoGen min 0", 0, `"op":"promoGen","min":0,"count":5,"len":6,"charset":"mixed"`},
		{"promoGen child 3", 3, `"op":"promoGen","min":15,"count":5,"len":6,"charset":"mixed"`},
		{"promoDelete пусто", 0, `"op":"promoDelete","code":""`},
		{"promoDelete пробелы", 0, `"op":"promoDelete","code":"   "`},
		{"promoDelete нет кода", 0, `"op":"promoDelete"`},
		{"promoDelete 65 знаков", 0, `"op":"promoDelete","code":"` + strings.Repeat("я", 65) + `"`},
		{"promoDelete число", 0, `"op":"promoDelete","code":12`},
		{"promoDelete child 3", 3, `"op":"promoDelete","code":"SUMMER"`},
		{"key abc", 0, `"op":"decide","key":"abc","cat":"games"`},
		{"key 22 знака не base64url", 0, `"op":"decide","key":"` + strings.Repeat("+", 22) + `","cat":"games"`},
		{"key с =", 0, `"op":"decide","key":"` + base64.URLEncoding.EncodeToString(make([]byte, 16)) + `","cat":"games"`},
		{"key 21 байт", 0, `"op":"decide","key":"` + key21 + `","cat":"games"`},
		{"key пусто", 0, `"op":"decide","key":"","cat":"games"`},
		{"key нет", 0, `"op":"decide","cat":"games"`},
		{"key число", 0, `"op":"decide","key":5,"cat":"games"`},
		{"cat game", 0, `"op":"decide","key":"` + key + `","cat":"game"`},
		{"cat system", 0, `"op":"decide","key":"` + key + `","cat":"system"`},
		{"cat нет", 0, `"op":"decide","key":"` + key + `"`},
		{"cat null", 0, `"op":"decide","key":"` + key + `","cat":null`},
		{"decide child 3", 3, `"op":"decide","key":"` + key + `","cat":"games"`},
		{"неизвестный op", 0, `"op":"promoEdit","code":"SUMMER"`},
	}
	for _, c := range bad {
		if _, code := f.check(17, c.child, c.fields); code != ErrCodeRange {
			t.Errorf("%s: код %q, нужно range", c.name, code)
		}
		// Номер не тратится: та же неверная команда со старым seq — тоже range, а не replay
		// (проверка вида раньше seq), а CheckCommand состояние не меняет.
		if _, code := f.check(16, c.child, c.fields); code != ErrCodeRange {
			t.Errorf("%s со старым seq: код %q, нужно range", c.name, code)
		}
	}
	if f.st.LastSeq != 16 {
		t.Fatalf("LastSeq %d", f.st.LastSeq)
	}
	// Верная команда со старым seq — replay (вид проверен, номер — потом).
	if _, code := f.check(16, 3, `"op":"bankAdd","delta":30`); code != ErrCodeReplay {
		t.Fatalf("верная со старым seq: %q", code)
	}
	// delta строкой — прежнее поле, по-прежнему bad_sig (R-5: delta целое с 0.8.0).
	if _, code := f.check(17, 3, `"op":"bankAdd","delta":"30"`); code != ErrCodeBadSig {
		t.Fatalf("delta строкой: %q", code)
	}
}

// Ловит: ключ строки pending не совпадает с ручным расчётом (Kotlin посчитал бы иначе); урезание
// пути режет короткий путь, теряет хвост с именем файла или режет кириллицу посередине знака.
func TestPendingKeyAndClipPath(t *testing.T) {
	for _, p := range []string{`C:\Users\Misha\Downloads\SuperGame\game.exe`, `c:\users\миша\игра.exe`, ""} {
		k := PendingKey(p)
		if len(k) != 22 || k != manualKey(p) {
			t.Errorf("PendingKey(%q) = %q, нужно %q", p, k, manualKey(p))
		}
	}
	if PendingKey("a") == PendingKey("A") {
		t.Fatal("разные пути — один ключ")
	}

	p120 := strings.Repeat("a", 100) + strings.Repeat("я", 20)
	if got := ClipPath(p120); got != p120 {
		t.Fatalf("путь 120 знаков изменён: %q", got)
	}
	head := `C:\Users\` + strings.Repeat("П", 31) // 40 знаков
	mid := "XY"                                   // выпадает
	tail := strings.Repeat("ж", 70) + `\game.exe` // 79 знаков
	p121 := head + mid + tail
	if n := utf8.RuneCountInString(p121); n != 121 {
		t.Fatalf("подготовка: %d знаков", n)
	}
	got := ClipPath(p121)
	r := []rune(got)
	if len(r) != 120 || r[40] != '…' || string(r[:40]) != head || string(r[41:]) != tail || !utf8.ValidString(got) {
		t.Fatalf("путь 121 знак: %q (%d знаков)", got, len(r))
	}
	long := strings.Repeat("д", 200) + `\SuperGame.exe`
	got = ClipPath(long)
	if r := []rune(got); len(r) != 120 || !strings.HasSuffix(got, `\SuperGame.exe`) || !utf8.ValidString(got) {
		t.Fatalf("длинный путь: %q", got)
	}
}
