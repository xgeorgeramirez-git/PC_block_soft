//go:build windows

package winhost

import (
	"errors"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"pcblock/internal/programs"
	"pcblock/internal/service"
)

// Издатель программ из папок пользователя (B-57, RM3-13, D-77). Ребёнок может записать в свой
// профиль и во временную папку любой exe, а CompanyName в ресурсе версии подделывается правкой
// файла: решение родителя «то же имя и тот же издатель» тогда перешло бы на чужую программу. Поэтому
// для путей внутри папки профилей издатель — имя подписанта цифровой подписи Windows (Authenticode),
// и только если подпись верна. Нет подписи или она неверна — издатель пустой (решение не переходит,
// программа снова ждёт родителя). В Program Files и Windows писать может только администратор, там
// остаётся CompanyName; прочие папки вне профилей (ProgramData, другие диски) наследования решений по
// издателю не имеют (RM3-13 — только папки приложения в профиле и временная папка).

var (
	modWintrust                        = windows.NewLazySystemDLL("wintrust.dll")
	procWTHelperProvDataFromStateData  = modWintrust.NewProc("WTHelperProvDataFromStateData")
	procWTHelperGetProvSignerFromChain = modWintrust.NewProc("WTHelperGetProvSignerFromChain")
	procWTHelperGetProvCertFromChain   = modWintrust.NewProc("WTHelperGetProvCertFromChain")
)

// providerCert — начало CRYPT_PROVIDER_CERT: размер и сертификат (раскладка C та же).
type providerCert struct {
	size uint32
	cert *windows.CertContext
}

// Коды WinVerifyTrust «подписи нет или она неверна» (окончательный ответ); любой другой отказ —
// файл не прочитался (держит антивирус, только что записан), и проверка повторяется (PL-46).
const (
	trustNoSignature  = windows.Errno(0x800B0100) // TRUST_E_NOSIGNATURE
	trustBadDigest    = windows.Errno(0x80096010) // TRUST_E_BAD_DIGEST
	trustSubjectNotOK = windows.Errno(0x800B0004) // TRUST_E_SUBJECT_NOT_TRUSTED
	trustExplicitDeny = windows.Errno(0x800B0111) // TRUST_E_EXPLICIT_DISTRUST
	trustProviderUnk  = windows.Errno(0x800B0001) // TRUST_E_PROVIDER_UNKNOWN (не PE)
	trustSubjectForm  = windows.Errno(0x800B0003) // TRUST_E_SUBJECT_FORM_UNKNOWN (не PE)
)

// signatureFinal — отказ проверки означает «подписи нет или она неверна», а не сбой чтения.
func signatureFinal(err error) bool {
	var e windows.Errno
	if !errors.As(err, &e) {
		return false
	}
	switch e {
	case trustNoSignature, trustBadDigest, trustSubjectNotOK, trustExplicitDeny, trustProviderUnk, trustSubjectForm:
		return true
	}
	return uint32(e)&0xFFFF0000 == 0x800B0000 // CERT_E_* и прочие TRUST_E_*: ответ о сертификате
}

// signerName — имя подписанта файла path («Roblox Corporation»), если подпись Authenticode верна;
// иначе «». Сбой чтения файла (не ответ о подписи) повторяется до трёх раз через 300 мс; final —
// ответ окончательный (его можно запомнить).
func signerName(path string) (name string, final bool) {
	for i := 0; ; i++ {
		name, final = signerOnce(path)
		if final || i == 2 {
			return name, final
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// signerOnce — одна проверка: имя и «ответ окончательный» (подпись верна, её нет или она неверна).
// Отзыв сертификатов не проверяется и сеть не трогается (служба работает без сети).
func signerOnce(path string) (string, bool) {
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", true
	}
	data := &windows.WinTrustData{
		Size:             uint32(unsafe.Sizeof(windows.WinTrustData{})),
		UIChoice:         windows.WTD_UI_NONE,
		RevocationChecks: windows.WTD_REVOKE_NONE,
		UnionChoice:      windows.WTD_CHOICE_FILE,
		StateAction:      windows.WTD_STATEACTION_VERIFY,
		ProvFlags:        windows.WTD_REVOCATION_CHECK_NONE | windows.WTD_CACHE_ONLY_URL_RETRIEVAL,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&windows.WinTrustFileInfo{
			Size:     uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})),
			FilePath: p16,
		}),
	}
	verr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	defer func() {
		data.StateAction = windows.WTD_STATEACTION_CLOSE
		_ = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	}()
	if verr != nil {
		return "", signatureFinal(verr)
	}
	if data.StateData == 0 {
		return "", true
	}
	if procWTHelperProvDataFromStateData.Find() != nil || procWTHelperGetProvSignerFromChain.Find() != nil || procWTHelperGetProvCertFromChain.Find() != nil {
		return "", true
	}
	prov, _, _ := procWTHelperProvDataFromStateData.Call(uintptr(data.StateData))
	if prov == 0 {
		return "", true
	}
	sgnr, _, _ := procWTHelperGetProvSignerFromChain.Call(prov, 0, 0, 0)
	if sgnr == 0 {
		return "", true
	}
	pc, _, _ := procWTHelperGetProvCertFromChain.Call(sgnr, 0)
	if pc == 0 {
		return "", true
	}
	// pc — адрес структуры в памяти wintrust (живёт до WTD_STATEACTION_CLOSE); не адрес Go.
	cert := (*(**providerCert)(unsafe.Pointer(&pc))).cert
	if cert == nil {
		return "", true
	}
	n := windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, nil, 0)
	if n <= 1 {
		return "", true
	}
	buf := make([]uint16, n)
	windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, &buf[0], n)
	return strings.TrimSpace(windows.UTF16ToString(buf)), true
}

// signerCacheMax — сколько ответов о подписи помнит profileSigned; больше — память очищается.
const signerCacheMax = 1024

// profileSigned — service.FileMeta поверх file: для путей внутри папки профилей profiles издатель —
// подписант (signer), а не CompanyName (B-57). profiles пустая — как file. Проверка подписи читает
// весь файл (Discord — около 170 МБ), поэтому ответ запоминается по пути и отпечатку «размер:время
// изменения»: новый процесс той же программы файл заново не читает. Без отпечатка или после сбоя
// чтения файла — не запоминается.
func profileSigned(file service.FileMeta, profiles string, signer func(string) (string, bool)) service.FileMeta {
	root := strings.TrimSuffix(programs.Normalize(profiles), `\`)
	var mu sync.Mutex
	cache := map[string]string{}
	return func(path string) (string, string, string) {
		pub, descr, fp := file(path)
		norm := programs.Normalize(path)
		if root == "" || !strings.HasPrefix(norm, root+`\`) {
			return pub, descr, fp
		}
		key := norm + "|" + fp
		if fp != "" {
			mu.Lock()
			name, ok := cache[key]
			mu.Unlock()
			if ok {
				return name, descr, fp
			}
		}
		name, final := signer(path)
		if fp != "" && final {
			mu.Lock()
			if len(cache) >= signerCacheMax {
				cache = map[string]string{}
			}
			cache[key] = name
			mu.Unlock()
		}
		return name, descr, fp
	}
}
