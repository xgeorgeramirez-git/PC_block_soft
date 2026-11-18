//go:build windows

package winhost

import (
	"strings"
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
// программа снова ждёт родителя). Вне профилей (Program Files, Windows) писать может только
// администратор, там остаётся CompanyName.

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

// signerName — имя подписанта файла path («Roblox Corporation»), если подпись Authenticode верна;
// иначе «». Отзыв сертификатов не проверяется и сеть не трогается (служба работает без сети).
func signerName(path string) string {
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return ""
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
	if verr != nil || data.StateData == 0 {
		return ""
	}
	if procWTHelperProvDataFromStateData.Find() != nil || procWTHelperGetProvSignerFromChain.Find() != nil || procWTHelperGetProvCertFromChain.Find() != nil {
		return ""
	}
	prov, _, _ := procWTHelperProvDataFromStateData.Call(uintptr(data.StateData))
	if prov == 0 {
		return ""
	}
	sgnr, _, _ := procWTHelperGetProvSignerFromChain.Call(prov, 0, 0, 0)
	if sgnr == 0 {
		return ""
	}
	pc, _, _ := procWTHelperGetProvCertFromChain.Call(sgnr, 0)
	if pc == 0 {
		return ""
	}
	// pc — адрес структуры в памяти wintrust (живёт до WTD_STATEACTION_CLOSE); не адрес Go.
	cert := (*(**providerCert)(unsafe.Pointer(&pc))).cert
	if cert == nil {
		return ""
	}
	n := windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, nil, 0)
	if n <= 1 {
		return ""
	}
	buf := make([]uint16, n)
	windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, &buf[0], n)
	return strings.TrimSpace(windows.UTF16ToString(buf))
}

// profileSigned — service.FileMeta поверх file: для путей внутри папки профилей profiles издатель —
// подписант (signer), а не CompanyName (B-57). profiles пустая — как file.
func profileSigned(file service.FileMeta, profiles string, signer func(string) string) service.FileMeta {
	root := strings.TrimSuffix(programs.Normalize(profiles), `\`)
	return func(path string) (string, string, string) {
		pub, descr, fp := file(path)
		if root != "" && strings.HasPrefix(programs.Normalize(path), root+`\`) {
			pub = signer(path)
		}
		return pub, descr, fp
	}
}
