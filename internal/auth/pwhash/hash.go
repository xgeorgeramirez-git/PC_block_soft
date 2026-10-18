// Package pwhash — хеш пароля родителя Argon2id (NFR-9): память 64 МБ, 3 прохода, строка формата PHC.
//
// Строка хеша несёт параметры и соль: `$argon2id$v=19$m=65536,t=3,p=1$<соль>$<хеш>`
// (base64 без дополнения), поэтому Verify проверяет и хеши со старыми параметрами.
// Вынесен из internal/auth, потому что требует golang.org/x/crypto.
package pwhash

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"

	"pcblock/internal/auth"
)

// ErrMalformed — строка хеша из базы испорчена или не в формате PHC Argon2id.
var ErrMalformed = errors.New("pwhash: строка хеша не в формате argon2id")

// Длины соли и хеша в байтах.
const (
	saltLen = 16
	keyLen  = 32
)

// Пределы параметров при разборе: испорченная строка не должна запросить гигабайты
// памяти или минуты работы службы.
const (
	maxMemoryKiB  = 1 << 20 // 1 ГБ
	maxIterations = 64
	minSaltLen    = 8
	minKeyLen     = 16
)

var (
	saltReader io.Reader = rand.Reader
	b64                  = base64.RawStdEncoding
)

// params — параметры Argon2id.
type params struct {
	memoryKiB  uint32
	iterations uint32
	threads    uint8
}

// defaultParams — NFR-9: память 64 МБ, 3 прохода, один поток.
var defaultParams = params{memoryKiB: 64 * 1024, iterations: 3, threads: 1}

// Hash возвращает строку PHC Argon2id для пароля pw со случайной солью 16 байт.
// Пароль короче 6 символов не хешируется: ошибка auth.ErrTooShort (R19).
func Hash(pw string) (string, error) {
	if err := auth.CheckLength(pw); err != nil {
		return "", err
	}
	return hashWith(pw, defaultParams, saltReader)
}

func hashWith(pw string, p params, r io.Reader) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(r, salt); err != nil {
		return "", fmt.Errorf("pwhash: соль не получена: %w", err)
	}
	sum := argon2.IDKey([]byte(pw), salt, p.iterations, p.memoryKiB, p.threads, keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memoryKiB, p.iterations, p.threads,
		b64.EncodeToString(salt), b64.EncodeToString(sum)), nil
}

// Verify проверяет пароль pw по строке hash. Неверный пароль — (false, nil);
// испорченная строка — ошибка, обёрнутая вокруг ErrMalformed. Сравнение — за постоянное время.
func Verify(hash, pw string) (bool, error) {
	p, salt, sum, err := parse(hash)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(pw), salt, p.iterations, p.memoryKiB, p.threads, uint32(len(sum)))
	return subtle.ConstantTimeCompare(got, sum) == 1, nil
}

// parse разбирает `$argon2id$v=19$m=…,t=…,p=…$<соль>$<хеш>`.
func parse(hash string) (params, []byte, []byte, error) {
	var p params
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return p, nil, nil, fmt.Errorf("%w: не PHC argon2id", ErrMalformed)
	}
	v, ok := strings.CutPrefix(parts[2], "v=")
	if !ok || v != strconv.Itoa(argon2.Version) {
		return p, nil, nil, fmt.Errorf("%w: версия %q", ErrMalformed, parts[2])
	}
	kv := strings.Split(parts[3], ",")
	if len(kv) != 3 {
		return p, nil, nil, fmt.Errorf("%w: параметры %q", ErrMalformed, parts[3])
	}
	m, errM := paramValue(kv[0], "m=", 32)
	t, errT := paramValue(kv[1], "t=", 32)
	th, errP := paramValue(kv[2], "p=", 8)
	if err := errors.Join(errM, errT, errP); err != nil {
		return p, nil, nil, fmt.Errorf("%w: параметры %q: %v", ErrMalformed, parts[3], err)
	}
	p = params{memoryKiB: uint32(m), iterations: uint32(t), threads: uint8(th)}
	if p.threads == 0 || p.iterations == 0 || p.iterations > maxIterations ||
		p.memoryKiB < 8*uint32(p.threads) || p.memoryKiB > maxMemoryKiB {
		return p, nil, nil, fmt.Errorf("%w: параметры вне пределов %q", ErrMalformed, parts[3])
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) < minSaltLen {
		return p, nil, nil, fmt.Errorf("%w: соль", ErrMalformed)
	}
	sum, err := b64.DecodeString(parts[5])
	if err != nil || len(sum) < minKeyLen {
		return p, nil, nil, fmt.Errorf("%w: хеш", ErrMalformed)
	}
	return p, salt, sum, nil
}

// paramValue читает «ключ=число» с ожидаемым ключом.
func paramValue(s, key string, bits int) (uint64, error) {
	v, ok := strings.CutPrefix(s, key)
	if !ok {
		return 0, fmt.Errorf("ждали %s", key)
	}
	return strconv.ParseUint(v, 10, bits)
}
