package remote

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
)

// Метки обязательства и числа сверки (протокол, раздел 4) — байты UTF-8.
const (
	commitLabel = "pcblock-commit-v1"
	sasLabel    = "pcblock-sas-v1"
)

var (
	// ErrBadKey — ключ не разобран или не P-256.
	ErrBadKey = errors.New("remote: неверный ключ")
	// ErrBadSig — подпись не сошлась или блок не разобран.
	ErrBadSig = errors.New("remote: неверная подпись")
)

// b64 — base64url без дополнения (протокол, раздел 2).
func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// unb64 — строгий разбор base64url без дополнения: «=», «+», «/» и лишние биты — ошибка.
func unb64(s string) ([]byte, error) { return base64.RawURLEncoding.Strict().DecodeString(s) }

// NewKey — новая пара ключей P-256 (ключ компьютера).
func NewKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// MarshalPub — открытый ключ как SubjectPublicKeyInfo DER в base64url.
func MarshalPub(k *ecdsa.PublicKey) string {
	der, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		return ""
	}
	return b64(der)
}

// ParsePub — открытый ключ из base64url SubjectPublicKeyInfo; только P-256.
func ParsePub(s string) (*ecdsa.PublicKey, error) {
	der, err := unb64(s)
	if err != nil || len(der) == 0 {
		return nil, ErrBadKey
	}
	return parsePubDER(der)
}

func parsePubDER(der []byte) (*ecdsa.PublicKey, error) {
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, ErrBadKey
	}
	ek, ok := k.(*ecdsa.PublicKey)
	if !ok || ek.Curve != elliptic.P256() {
		return nil, ErrBadKey
	}
	return ek, nil
}

// MarshalPriv — закрытый ключ PKCS#8 DER в base64url (хранится в настройках службы).
func MarshalPriv(k *ecdsa.PrivateKey) (string, error) {
	if k == nil || k.Curve != elliptic.P256() {
		return "", ErrBadKey
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		return "", fmt.Errorf("remote: ключ не записан: %w", err)
	}
	return b64(der), nil
}

// ParsePriv — закрытый ключ из base64url PKCS#8; только P-256.
func ParsePriv(s string) (*ecdsa.PrivateKey, error) {
	der, err := unb64(s)
	if err != nil {
		return nil, ErrBadKey
	}
	k, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, ErrBadKey
	}
	ek, ok := k.(*ecdsa.PrivateKey)
	if !ok || ek.Curve != elliptic.P256() {
		return nil, ErrBadKey
	}
	return ek, nil
}

// Sign подписывает байты data (SHA-256, ASN.1 DER). Подпись недетерминирована.
func Sign(k *ecdsa.PrivateKey, data []byte) (Signed, error) {
	if k == nil {
		return Signed{}, ErrBadKey
	}
	h := sha256.Sum256(data)
	sig, err := ecdsa.SignASN1(rand.Reader, k, h[:])
	if err != nil {
		return Signed{}, fmt.Errorf("remote: подпись не создана: %w", err)
	}
	return Signed{Data: b64(data), Sig: b64(sig)}, nil
}

// Verify проверяет подпись и возвращает подписанные байты. JSON разбирается только после.
func Verify(pub *ecdsa.PublicKey, s Signed) ([]byte, error) {
	if pub == nil {
		return nil, ErrBadKey
	}
	data, err1 := unb64(s.Data)
	sig, err2 := unb64(s.Sig)
	if err1 != nil || err2 != nil || len(sig) == 0 {
		return nil, ErrBadSig
	}
	h := sha256.Sum256(data)
	if !ecdsa.VerifyASN1(pub, h[:], sig) {
		return nil, ErrBadSig
	}
	return data, nil
}

func hashJoin(label string, parts ...[]byte) [32]byte {
	h := sha256.New()
	h.Write([]byte(label))
	for _, p := range parts {
		h.Write(p)
	}
	var out [32]byte
	h.Sum(out[:0])
	return out
}

// Commit — обязательство компьютера: SHA-256("pcblock-commit-v1" ‖ pcPubDER ‖ phonePubDER ‖ nPc).
func Commit(pcPub, phonePub, nPc []byte) []byte {
	h := hashJoin(commitLabel, pcPub, phonePub, nPc)
	return h[:]
}

// SAS — число сверки: первые 4 байта SHA-256("pcblock-sas-v1" ‖ pcPub ‖ phonePub ‖ nPc ‖ nPhone)
// big-endian по модулю 1 000 000, шесть цифр с ведущими нулями ("482913").
func SAS(pcPub, phonePub, nPc, nPhone []byte) string {
	h := hashJoin(sasLabel, pcPub, phonePub, nPc, nPhone)
	return fmt.Sprintf("%06d", binary.BigEndian.Uint32(h[:4])%1000000)
}

// HashData — base64url SHA-256 байтов data команды (поле cmd ответа cmdr).
func HashData(data []byte) string {
	h := sha256.Sum256(data)
	return b64(h[:])
}
