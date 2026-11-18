//go:build windows

package winhost

import (
	"os"
	"path/filepath"
	"testing"
)

// Ловит (B-57): в папке профилей издатель берётся из CompanyName (подделывается правкой файла), а не
// из подписи; вне профилей CompanyName подменён подписью.
func TestProfileSignedPublisher(t *testing.T) {
	file := func(string) (string, string, string) { return "Roblox Corporation", "Roblox", "1:2" }
	signer := func(p string) string {
		if filepath.Base(p) == "signed.exe" {
			return "Roblox Corporation"
		}
		return ""
	}
	meta := profileSigned(file, `C:\Users`, signer)
	cases := []struct{ path, want string }{
		{`C:\Users\misha\AppData\Local\Temp\RobloxPlayerInstaller.exe`, ""}, // CompanyName подделан, подписи нет
		{`C:\Users\misha\AppData\Local\Roblox\signed.exe`, "Roblox Corporation"},
		{`C:\Program Files\Roblox\RobloxPlayerInstaller.exe`, "Roblox Corporation"}, // вне профилей — CompanyName
		{`C:\UsersX\a.exe`, "Roblox Corporation"},
	}
	for _, c := range cases {
		if pub, descr, fp := meta(c.path); pub != c.want || descr != "Roblox" || fp != "1:2" {
			t.Errorf("%s: %q, %q, %q; ждали издателя %q, описание и отпечаток как есть", c.path, pub, descr, fp, c.want)
		}
	}
}

// Ловит (B-57): подписанный файл не даёт имени подписанта, неподписанный даёт.
func TestSignerName(t *testing.T) {
	pwsh := filepath.Join(os.Getenv("ProgramFiles"), "PowerShell", "7", "pwsh.exe")
	raw, err := os.ReadFile(pwsh)
	if err != nil {
		t.Skipf("нет %s: %v", pwsh, err)
	}
	dir := t.TempDir()
	signed := filepath.Join(dir, "signed.exe")
	if err := os.WriteFile(signed, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := signerName(signed); got != "Microsoft Corporation" {
		t.Errorf("подписант копии pwsh.exe %q, ждали Microsoft Corporation", got)
	}
	plain := filepath.Join(dir, "plain.exe")
	if err := os.WriteFile(plain, []byte("не PE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := signerName(plain); got != "" {
		t.Errorf("подписант файла без подписи %q, ждали пусто", got)
	}
	if got := signerName(filepath.Join(dir, "нет.exe")); got != "" {
		t.Errorf("подписант несуществующего файла %q, ждали пусто", got)
	}
}
