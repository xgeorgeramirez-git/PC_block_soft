//go:build windows

package winhost

import (
	"errors"
	"os"
	"strconv"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"pcblock/internal/service"
)

// Адаптер правил Brave испытывается в своей тестовой ветке раздела текущего пользователя, а не в
// настоящей Software\Policies\BraveSoftware\Brave: тесты пакетов идут параллельно, а TestBraveProbe
// (internal/guard) в это время пишет и удаляет настоящий ключ HKCU (T5-10). Что Brave эти значения
// применяет, доказала проба T0 (RM4-10-итог); правила службы в окне Brave проверяет S20.

const braveTestParent = `Software\PCBlockTest`

// braveTestPolicy — адаптер с тестовой веткой и SID текущего процесса; ветка удаляется после теста.
func braveTestPolicy(t *testing.T) (*policy, string, string) {
	t.Helper()
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("SID процесса: %v", err)
	}
	sid := tu.User.Sid.String()
	base := braveTestParent + `\BraveSoftware-` + strconv.Itoa(os.Getpid())
	t.Cleanup(func() {
		_ = deleteTree(registry.USERS, sid+`\`+base)
		_ = registry.DeleteKey(registry.USERS, sid+`\`+braveTestParent) // только если пуст
	})
	return &policy{base: base}, sid, sid + `\` + base
}

func regDWord(t *testing.T, path, name string) uint64 {
	t.Helper()
	k, err := registry.OpenKey(registry.USERS, path, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("ключ %s: %v", path, err)
	}
	defer k.Close()
	n, typ, err := k.GetIntegerValue(name)
	if err != nil || typ != registry.DWORD {
		t.Fatalf("%s\\%s: %d, тип %d, %v; want REG_DWORD", path, name, n, typ, err)
	}
	return n
}

func regString(t *testing.T, path, name string) string {
	t.Helper()
	k, err := registry.OpenKey(registry.USERS, path, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("ключ %s: %v", path, err)
	}
	defer k.Close()
	s, typ, err := k.GetStringValue(name)
	if err != nil || typ != registry.SZ {
		t.Fatalf("%s\\%s: %q, тип %d, %v; want REG_SZ", path, name, s, typ, err)
	}
	return s
}

func regMissing(path, name string) bool {
	k, err := registry.OpenKey(registry.USERS, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return true
	}
	if err != nil {
		return false
	}
	defer k.Close()
	if name == "" {
		return false
	}
	_, _, err = k.GetValue(name, nil)
	return errors.Is(err, registry.ErrNotExist)
}

// Ловит (RM4-10): путь правил Brave разошёлся с тем, что Brave читает (проба T0).
func TestBravePolicyPath(t *testing.T) {
	if bravePolicyPath != `Software\Policies\BraveSoftware\Brave` {
		t.Errorf("bravePolicyPath = %q", bravePolicyPath)
	}
}

// Ловит (RM4-10, T5-3, PL-21, PL-41): значения не того типа или не на месте, сверка не замечает
// лишнего номера списка или чужого подключа, запись стирает ветку (TorDisabled пропадает),
// подключ удаляется без рекурсии, повторное снятие ветки — ошибка.
func TestBraveAdapterWriteCheckRemove(t *testing.T) {
	p, sid, path := braveTestPolicy(t)
	two := service.BraveRules{Blocked: []string{"a.example", "b.example"}}
	if ok, err := p.BraveRulesOK(sid, two); ok || err != nil {
		t.Fatalf("до записи BraveRulesOK = (%v, %v), want (false, nil)", ok, err)
	}
	if err := p.SetBraveRules(sid, two); err != nil {
		t.Fatalf("SetBraveRules: %v", err)
	}
	if n := regDWord(t, path, "TorDisabled"); n != 1 {
		t.Errorf("TorDisabled = %d", n)
	}
	if n := regDWord(t, path, "BraveVPNDisabled"); n != 1 {
		t.Errorf("BraveVPNDisabled = %d", n)
	}
	if s := regString(t, path, "DnsOverHttpsMode"); s != "off" {
		t.Errorf("DnsOverHttpsMode = %q", s)
	}
	if s := regString(t, path+`\ExtensionInstallBlocklist`, "1"); s != "*" {
		t.Errorf("ExtensionInstallBlocklist\\1 = %q", s)
	}
	if a, b := regString(t, path+`\URLBlocklist`, "1"), regString(t, path+`\URLBlocklist`, "2"); a != "a.example" || b != "b.example" {
		t.Errorf("URLBlocklist = %q, %q", a, b)
	}
	if ok, err := p.BraveRulesOK(sid, two); !ok || err != nil {
		t.Fatalf("после записи BraveRulesOK = (%v, %v), want (true, nil)", ok, err)
	}

	// Чужие руки: лишний номер списка и подключ с подключом внутри.
	k, _, err := registry.CreateKey(registry.USERS, path+`\URLBlocklist`, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	_ = k.SetStringValue("3", "c.example")
	k.Close()
	k, _, err = registry.CreateKey(registry.USERS, path+`\Proxy\Deep`, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	_ = k.SetDWordValue("x", 1)
	k.Close()
	if ok, err := p.BraveRulesOK(sid, two); ok || err != nil {
		t.Fatalf("после чужой правки BraveRulesOK = (%v, %v), want (false, nil)", ok, err)
	}

	one := service.BraveRules{Blocked: []string{"a.example"}}
	if err := p.SetBraveRules(sid, one); err != nil {
		t.Fatalf("SetBraveRules с одним доменом: %v", err)
	}
	if n := regDWord(t, path, "TorDisabled"); n != 1 {
		t.Errorf("TorDisabled после перезаписи = %d", n)
	}
	if s := regString(t, path+`\URLBlocklist`, "1"); s != "a.example" {
		t.Errorf("URLBlocklist\\1 = %q", s)
	}
	for _, name := range []string{"2", "3"} {
		if !regMissing(path+`\URLBlocklist`, name) {
			t.Errorf("URLBlocklist\\%s не удалён", name)
		}
	}
	if !regMissing(path+`\Proxy`, "") {
		t.Error("чужой подключ Proxy не удалён")
	}
	if ok, err := p.BraveRulesOK(sid, one); !ok || err != nil {
		t.Fatalf("после перезаписи BraveRulesOK = (%v, %v), want (true, nil)", ok, err)
	}

	if err := p.RemoveBraveRules(sid); err != nil {
		t.Fatalf("RemoveBraveRules: %v", err)
	}
	if !regMissing(path, "") {
		t.Error("ветка правил Brave не снята")
	}
	if !regMissing(sid+`\`+braveTestParent, "") {
		t.Error("пустая ветка над правилами Brave не снята")
	}
	if ok, err := p.BraveRulesOK(sid, one); ok || err != nil {
		t.Errorf("после снятия BraveRulesOK = (%v, %v), want (false, nil)", ok, err)
	}
	if err := p.RemoveBraveRules(sid); err != nil {
		t.Errorf("повторное снятие: %v", err)
	}
}

// Ловит (T5-4, PL-9, PL-41): сверка читает ветку невошедшего пользователя или считает её
// расхождением, запись и снятие для учётки без профиля — ошибка.
func TestBraveAdapterUnloadedHive(t *testing.T) {
	p, _, _ := braveTestPolicy(t)
	const sid = "S-1-5-21-0-0-0-4242"
	r := service.BraveRules{Blocked: []string{"a.example"}}
	if ok, err := p.BraveRulesOK(sid, r); !ok || err != nil {
		t.Errorf("BraveRulesOK без ветки = (%v, %v), want (true, nil)", ok, err)
	}
	if err := p.SetBraveRules(sid, r); err != nil {
		t.Errorf("SetBraveRules без профиля: %v", err)
	}
	if err := p.RemoveBraveRules(sid); err != nil {
		t.Errorf("RemoveBraveRules без профиля: %v", err)
	}
}
