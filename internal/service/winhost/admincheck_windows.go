//go:build windows

package winhost

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"pcblock/internal/service"
)

var (
	netapi32                  = windows.NewLazySystemDLL("netapi32.dll")
	procNetUserGetLocalGroups = netapi32.NewProc("NetUserGetLocalGroups")
)

const (
	lgIncludeIndirect  = 0x1        // LG_INCLUDE_INDIRECT: и группы, в которые учётка входит через другие группы
	maxPreferredLength = 0xFFFFFFFF // MAX_PREFERRED_LENGTH: весь список за один вызов
	filterNormal       = 0x2        // FILTER_NORMAL_ACCOUNT: обычные учётки, без компьютеров и доменных доверий
	ufAccountDisable   = 0x2        // UF_ACCOUNTDISABLE
)

// localGroupsOf возвращает имена локальных групп учётки, в том числе косвенные (R1).
func localGroupsOf(account string) ([]string, error) {
	user, err := windows.UTF16PtrFromString(account)
	if err != nil {
		return nil, err
	}
	var (
		buf         *byte
		read, total uint32
	)
	r, _, _ := procNetUserGetLocalGroups.Call(
		0, // этот компьютер
		uintptr(unsafe.Pointer(user)),
		0, // уровень 0: только имена групп
		lgIncludeIndirect,
		uintptr(unsafe.Pointer(&buf)),
		maxPreferredLength,
		uintptr(unsafe.Pointer(&read)),
		uintptr(unsafe.Pointer(&total)),
	)
	if r != 0 {
		return nil, fmt.Errorf("NetUserGetLocalGroups(%s): код %d", account, r)
	}
	if buf == nil {
		return nil, nil
	}
	defer windows.NetApiBufferFree(buf)
	names := make([]string, 0, read)
	for _, p := range unsafe.Slice((**uint16)(unsafe.Pointer(buf)), read) {
		names = append(names, windows.UTF16PtrToString(p))
	}
	return names, nil
}

// adminGroupName — как называется группа Администраторов на этом компьютере («Administrators»,
// «Администраторы»): имя берётся по известному SID, а не по английскому слову.
func adminGroupName() (string, error) {
	sid, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return "", err
	}
	name, _, _, err := sid.LookupAccount("")
	return name, err
}

// adminChecker отвечает, входит ли учётка в Администраторы (service.AdminChecker, R1).
type adminChecker struct{}

// IsAdmin проверяет учётку по SID. Любой сбой возвращается ошибкой: служба не знает ответа и
// оставляет прежнее состояние учётки.
func (adminChecker) IsAdmin(sidString string) (bool, error) {
	sid, err := windows.StringToSid(sidString)
	if err != nil {
		return false, fmt.Errorf("SID %q: %w", sidString, err)
	}
	account, domain, _, err := sid.LookupAccount("")
	if err != nil {
		return false, fmt.Errorf("имя учётки для %s: %w", sidString, err)
	}
	admins, err := adminGroupName()
	if err != nil {
		return false, fmt.Errorf("имя группы Администраторов: %w", err)
	}
	candidates := []string{account}
	if domain != "" {
		candidates = []string{domain + `\` + account, account}
	}
	var lastErr error
	for _, name := range candidates {
		groups, err := localGroupsOf(name)
		if err != nil {
			lastErr = err
			continue
		}
		for _, g := range groups {
			if strings.EqualFold(g, admins) {
				return true, nil
			}
		}
		return false, nil
	}
	return false, lastErr
}

// userInfo20 — USER_INFO_20 из NetUserEnum (уровень 20 даёт имя и признаки учётки).
type userInfo20 struct {
	Name     *uint16
	FullName *uint16
	Comment  *uint16
	Flags    uint32
	UserID   uint32
}

// accountDir перечисляет локальные учётки (service.AccountDirectory): отключённые, такие как
// встроенные «Гость» и «Администратор», в список не попадают.
type accountDir struct {
	admins adminChecker
}

func (d accountDir) List() ([]service.AccountInfo, error) {
	var (
		buf         *byte
		read, total uint32
		resume      uint32
	)
	if err := windows.NetUserEnum(nil, 20, filterNormal, &buf, maxPreferredLength, &read, &total, &resume); err != nil {
		return nil, fmt.Errorf("NetUserEnum: %w", err)
	}
	if buf == nil {
		return []service.AccountInfo{}, nil
	}
	defer windows.NetApiBufferFree(buf)
	out := make([]service.AccountInfo, 0, read)
	for _, u := range unsafe.Slice((*userInfo20)(unsafe.Pointer(buf)), read) {
		if u.Flags&ufAccountDisable != 0 {
			continue
		}
		name := windows.UTF16PtrToString(u.Name)
		sid, _, _, err := windows.LookupSID("", name)
		if err != nil {
			continue
		}
		admin, _ := d.admins.IsAdmin(sid.String())
		out = append(out, service.AccountInfo{SID: sid.String(), Name: name, Admin: admin})
	}
	return out, nil
}
