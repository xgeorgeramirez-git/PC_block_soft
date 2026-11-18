package remote

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	mrand "math/rand"
	"testing"
)

func mustKey(t testing.TB) *ecdsa.PrivateKey {
	t.Helper()
	k, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestKeyRoundTrip(t *testing.T) {
	k := mustKey(t)
	pub := MarshalPub(&k.PublicKey)
	p, err := ParsePub(pub)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Equal(&k.PublicKey) {
		t.Fatal("открытый ключ не совпал после разбора")
	}
	priv, err := MarshalPriv(k)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := ParsePriv(priv)
	if err != nil {
		t.Fatal(err)
	}
	if !k2.Equal(k) {
		t.Fatal("закрытый ключ не совпал после разбора")
	}
	if bytes.ContainsAny([]byte(pub+priv), "=+/") {
		t.Fatal("ключи должны быть в base64url без дополнения")
	}
}

func TestParsePubRejects(t *testing.T) {
	k384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]string{
		"пусто":     "",
		"мусор":     "!!!",
		"padding":   MarshalPub(&mustKey(t).PublicKey) + "=",
		"не ключ":   b64(bytes.Repeat([]byte{1}, 91)),
		"P-384":     MarshalPub(&k384.PublicKey),
		"закрытый":  mustPriv(t),
		"стандарт+": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE+/",
	} {
		if _, err := ParsePub(s); err == nil {
			t.Errorf("%s: ключ принят", name)
		}
	}
	if _, err := ParsePriv(MarshalPub(&mustKey(t).PublicKey)); err == nil {
		t.Error("открытый ключ принят как закрытый")
	}
	if _, err := MarshalPriv(k384); err == nil {
		t.Error("P-384 принят")
	}
}

func mustPriv(t testing.TB) string {
	s, err := MarshalPriv(mustKey(t))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Свойство: Verify(Sign(data)) == data для случайных данных; любая порча отклоняется.
func TestSignVerifyProperty(t *testing.T) {
	r := mrand.New(mrand.NewSource(7))
	k := mustKey(t)
	other := mustKey(t)
	for i := 0; i < 200; i++ {
		data := make([]byte, r.Intn(2048))
		r.Read(data)
		s, err := Sign(k, data)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Verify(&k.PublicKey, s)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("прогон %d: подпись не проверилась: %v", i, err)
		}
		if _, err := Verify(&other.PublicKey, s); err == nil {
			t.Fatalf("прогон %d: чужой ключ принял подпись", i)
		}
		if len(data) > 0 {
			bad := append([]byte(nil), data...)
			bad[r.Intn(len(bad))] ^= byte(1 + r.Intn(255))
			if _, err := Verify(&k.PublicKey, Signed{Data: b64(bad), Sig: s.Sig}); err == nil {
				t.Fatalf("прогон %d: испорченные данные приняты", i)
			}
		}
		sig, _ := unb64(s.Sig)
		sig[r.Intn(len(sig))] ^= byte(1 + r.Intn(255))
		if _, err := Verify(&k.PublicKey, Signed{Data: s.Data, Sig: b64(sig)}); err == nil {
			t.Fatalf("прогон %d: испорченная подпись принята", i)
		}
	}
}

func TestVerifyRejectsEncoding(t *testing.T) {
	k := mustKey(t)
	s, _ := Sign(k, []byte(`{"t":"cmd"}`))
	for name, bad := range map[string]Signed{
		"data с =":     {Data: s.Data + "=", Sig: s.Sig},
		"sig стандарт": {Data: s.Data, Sig: base64.StdEncoding.EncodeToString([]byte("x"))},
		"пустая sig":   {Data: s.Data},
		"nil ключ":     s,
	} {
		pub := &k.PublicKey
		if name == "nil ключ" {
			pub = nil
		}
		if _, err := Verify(pub, bad); err == nil {
			t.Errorf("%s: принято", name)
		}
	}
}

// Commit и SAS — по формулам протокола (раздел 4), считаем независимо.
func TestCommitSASFormula(t *testing.T) {
	pc, ph := []byte("pc-pub-der"), []byte("phone-pub-der")
	nPc, nPhone := bytes.Repeat([]byte{0xA1}, 16), bytes.Repeat([]byte{0x5C}, 16)
	h := sha256.Sum256(append(append(append([]byte("pcblock-commit-v1"), pc...), ph...), nPc...))
	if !bytes.Equal(Commit(pc, ph, nPc), h[:]) {
		t.Fatal("обязательство не по формуле")
	}
	s := sha256.Sum256(append(append(append(append([]byte("pcblock-sas-v1"), pc...), ph...), nPc...), nPhone...))
	want := fmt.Sprintf("%06d", binary.BigEndian.Uint32(s[:4])%1000000)
	if got := SAS(pc, ph, nPc, nPhone); got != want || len(got) != 6 {
		t.Fatalf("число сверки %q, нужно %q", got, want)
	}
	// Любое изменение входа меняет число (на этих данных).
	if SAS(pc, ph, nPc, bytes.Repeat([]byte{0x5D}, 16)) == want {
		t.Fatal("число не зависит от nPhone")
	}
	if SAS(ph, pc, nPc, nPhone) == want {
		t.Fatal("число не зависит от порядка ключей")
	}
}

func TestSASLeadingZeros(t *testing.T) {
	// Ищем вход, дающий число < 100000, и проверяем ведущие нули.
	pc, ph := []byte("a"), []byte("b")
	for i := 0; i < 100000; i++ {
		n := []byte(fmt.Sprint(i))
		s := SAS(pc, ph, n, n)
		if len(s) != 6 {
			t.Fatalf("длина %d", len(s))
		}
		if s[0] == '0' {
			return
		}
	}
	t.Fatal("не нашли число с ведущим нулём")
}

func TestHashData(t *testing.T) {
	h := sha256.Sum256([]byte("abc"))
	if HashData([]byte("abc")) != base64.RawURLEncoding.EncodeToString(h[:]) {
		t.Fatal("хэш команды не base64url SHA-256")
	}
}
