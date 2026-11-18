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
	calls := 0
	signer := func(p string) (string, bool) {
		calls++
		if filepath.Base(p) == "signed.exe" {
			return "Roblox Corporation", true
		}
		if filepath.Base(p) == "locked.exe" {
			return "", false // файл не прочитался: ответ не окончательный
		}
		return "", true
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
	// Ловит: подпись перепроверяется для каждого нового процесса (весь файл читается заново), или
	// запоминается ответ после сбоя чтения.
	calls = 0
	signed := `C:\Users\misha\AppData\Local\Roblox\signed.exe`
	for i := 0; i < 3; i++ {
		meta(signed)
	}
	if calls != 0 {
		t.Errorf("подпись того же файла проверялась %d раз, ждали 0 (ответ запомнен)", calls)
	}
	locked := `C:\Users\misha\AppData\Local\Temp\locked.exe`
	meta(locked)
	meta(locked)
	if calls != 2 {
		t.Errorf("после сбоя чтения проверок %d, ждали 2 (не запоминается)", calls)
	}
}

// Ловит (B-57): подписанный файл не даёт имени подписанта, неподписанный даёт.
func TestSignerName(t *testing.T) {
	pwsh := filepath.Join(os.Getenv("ProgramFiles"), "PowerShell", "7", "pwsh.exe")
	raw, err := os.ReadFile(pwsh)
	if err != nil {
		t.Fatalf("нет %s: %v (на раннере windows-latest pwsh есть; без файла с подписью внутри тест ничего не ловит)", pwsh, err)
	}
	dir := t.TempDir()
	signed := filepath.Join(dir, "signed.exe")
	if err := os.WriteFile(signed, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, final := signerName(signed); got != "Microsoft Corporation" || !final {
		t.Errorf("подписант копии pwsh.exe %q (окончательно %v), ждали Microsoft Corporation", got, final)
	}
	plain := filepath.Join(dir, "plain.exe")
	if err := os.WriteFile(plain, []byte("не PE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, final := signerName(plain); got != "" || !final {
		t.Errorf("подписант файла без подписи %q (окончательно %v), ждали пусто и окончательно", got, final)
	}
	if got, _ := signerName(filepath.Join(dir, "нет.exe")); got != "" {
		t.Errorf("подписант несуществующего файла %q, ждали пусто", got)
	}
}
