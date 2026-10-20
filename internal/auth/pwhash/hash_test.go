package pwhash

import (
	"errors"
	"strings"
	"testing"

	"pcblock/internal/auth"
)

// lowParams — дешёвые параметры для тестов круга и разбора; настоящие 64 МБ / 3 прохода
// проверяет TestHashRealParams.
var lowParams = params{memoryKiB: 64, iterations: 1, threads: 1}

// Ловит: Hash снижает параметры NFR-9 (64 МБ, 3 прохода) или пишет строку не в формате
// PHC, и Verify не принимает собственный хеш с настоящими параметрами.
func TestHashRealParams(t *testing.T) {
	h, err := Hash("пароль-родителя")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	const prefix = "$argon2id$v=19$m=65536,t=3,p=1$"
	if !strings.HasPrefix(h, prefix) {
		t.Fatalf("Hash = %q, want начало %q", h, prefix)
	}
	ok, err := Verify(h, "пароль-родителя")
	if err != nil || !ok {
		t.Fatalf("Verify(свой хеш, верный пароль) = (%v, %v), want (true, nil)", ok, err)
	}
}

// Ловит: неверный пароль проходит проверку или даёт ошибку вместо «не подошёл»
// (окно показало бы техническую ошибку вместо «Неверный пароль»), R19.
func TestVerifyRoundTrip(t *testing.T) {
	h, err := hashWith("секрет1", lowParams, saltReader)
	if err != nil {
		t.Fatalf("hashWith: %v", err)
	}
	cases := []struct {
		pw   string
		want bool
	}{
		{"секрет1", true},
		{"секрет2", false},
		{"Секрет1", false},
		{"секрет1 ", false},
		{"", false},
	}
	for _, tc := range cases {
		ok, err := Verify(h, tc.pw)
		if err != nil {
			t.Fatalf("Verify(%q): ошибка %v, want nil", tc.pw, err)
		}
		if ok != tc.want {
			t.Errorf("Verify(%q) = %v, want %v", tc.pw, ok, tc.want)
		}
	}
}

// Ловит: соль постоянная или пустая (одинаковые пароли дают одинаковые хеши),
// соль не 16 байт.
func TestHashSaltRandom(t *testing.T) {
	h1, err1 := hashWith("одинаковый", lowParams, saltReader)
	h2, err2 := hashWith("одинаковый", lowParams, saltReader)
	if err1 != nil || err2 != nil {
		t.Fatalf("hashWith: %v, %v", err1, err2)
	}
	if h1 == h2 {
		t.Fatalf("два хеша одного пароля совпали: %q", h1)
	}
	parts := phcParts(t, h1)
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		t.Fatalf("соль %q: %d байт, ошибка %v; want 16 байт", parts[4], len(salt), err)
	}
	sum, err := b64.DecodeString(parts[5])
	if err != nil || len(sum) != 32 {
		t.Fatalf("хеш %q: %d байт, ошибка %v; want 32 байта", parts[5], len(sum), err)
	}
}

// Ловит: короткий пароль хешируется (мастер и смена пароля сохранили бы пароль
// из 5 символов), R19.
func TestHashRejectsShort(t *testing.T) {
	if h, err := Hash("12345"); !errors.Is(err, auth.ErrTooShort) || h != "" {
		t.Fatalf("Hash(5 символов) = (%q, %v), want (\"\", ErrTooShort)", h, err)
	}
}

// phcParts делит строку PHC на 6 частей по «$» или роняет тест.
func phcParts(t *testing.T, h string) []string {
	t.Helper()
	parts := strings.Split(h, "$")
	if len(parts) != 6 {
		t.Fatalf("хеш %q: %d частей, want 6", h, len(parts))
	}
	return parts
}

// failingReader — источник случайности, который не отдаёт байты.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("нет случайности") }

// Ловит: при сбое источника случайности хеш пишется с нулевой солью вместо ошибки.
func TestHashSaltFailure(t *testing.T) {
	if h, err := hashWith("секрет1", lowParams, failingReader{}); err == nil || h != "" {
		t.Fatalf("hashWith без случайности = (%q, %v), want (\"\", ошибка)", h, err)
	}
}

// Ловит: испорченная строка хеша из базы принимается (любой пароль подошёл бы)
// или роняет службу паникой; ошибка не распознаётся как ErrMalformed.
func TestVerifyMalformed(t *testing.T) {
	good, err := hashWith("секрет1", lowParams, saltReader)
	if err != nil {
		t.Fatalf("hashWith: %v", err)
	}
	parts := phcParts(t, good)
	salt, sum := parts[4], parts[5]
	cases := []struct {
		name string
		hash string
	}{
		{"пусто", ""},
		{"не PHC", "plain-text"},
		{"мало частей", "$argon2id$v=19$m=64,t=1,p=1$" + salt},
		{"много частей", good + "$x"},
		{"не с $", "x" + good},
		{"другой алгоритм", "$argon2i$v=19$m=64,t=1,p=1$" + salt + "$" + sum},
		{"другая версия", "$argon2id$v=16$m=64,t=1,p=1$" + salt + "$" + sum},
		{"версия не число", "$argon2id$v=x$m=64,t=1,p=1$" + salt + "$" + sum},
		{"параметры не по порядку", "$argon2id$v=19$t=1,m=64,p=1$" + salt + "$" + sum},
		{"параметр не число", "$argon2id$v=19$m=abc,t=1,p=1$" + salt + "$" + sum},
		{"не хватает параметра", "$argon2id$v=19$m=64,t=1$" + salt + "$" + sum},
		{"ноль проходов", "$argon2id$v=19$m=64,t=0,p=1$" + salt + "$" + sum},
		{"ноль потоков", "$argon2id$v=19$m=64,t=1,p=0$" + salt + "$" + sum},
		{"память меньше 8·p", "$argon2id$v=19$m=4,t=1,p=1$" + salt + "$" + sum},
		{"память больше 1 ГБ", "$argon2id$v=19$m=2097152,t=1,p=1$" + salt + "$" + sum},
		{"проходов больше 64", "$argon2id$v=19$m=64,t=100,p=1$" + salt + "$" + sum},
		{"соль не base64", "$argon2id$v=19$m=64,t=1,p=1$!!!$" + sum},
		{"соль короткая", "$argon2id$v=19$m=64,t=1,p=1$AAAA$" + sum},
		{"хеш не base64", "$argon2id$v=19$m=64,t=1,p=1$" + salt + "$!!!"},
		{"хеш короткий", "$argon2id$v=19$m=64,t=1,p=1$" + salt + "$AAAA"},
	}
	for _, tc := range cases {
		ok, err := Verify(tc.hash, "секрет1")
		if ok || !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: Verify(%q) = (%v, %v), want (false, ErrMalformed)", tc.name, tc.hash, ok, err)
		}
	}
}

// Ловит: подменённый в базе хеш (другие байты при тех же параметрах и соли)
// принимается с верным паролем.
func TestVerifyTamperedSum(t *testing.T) {
	good, err := hashWith("секрет1", lowParams, saltReader)
	if err != nil {
		t.Fatalf("hashWith: %v", err)
	}
	parts := phcParts(t, good)
	parts[5] = b64.EncodeToString(make([]byte, 32))
	ok, err := Verify(strings.Join(parts, "$"), "секрет1")
	if ok || err != nil {
		t.Fatalf("Verify(подменённый хеш) = (%v, %v), want (false, nil)", ok, err)
	}
}
