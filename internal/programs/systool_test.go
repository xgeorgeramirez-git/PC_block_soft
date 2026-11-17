package programs

import "testing"

// Ловит: инструмент не узнаётся в своей папке (System32, SysWOW64, корень Windows, PowerShell 7,
// пакет Терминала), узнаётся копия под тем же именем в папке ребёнка или Program Files, копия под
// другим именем, или файл в тестовой папке не узнаётся в тестовом режиме.
func TestSystemTool(t *testing.T) {
	r := Roots{SystemRoot: `c:\windows`, ProgramFiles: `c:\program files`, ProgramFilesX86: `c:\program files (x86)`, UserProfiles: `c:\users`}
	cases := []struct {
		path, test, want string
	}{
		{`C:\Windows\System32\cmd.exe`, "", "Командная строка"},
		{`C:\Windows\SysWOW64\cmd.exe`, "", "Командная строка"},
		{`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, "", "Windows PowerShell"},
		{`C:\Windows\regedit.exe`, "", "Редактор реестра"},
		{`C:\Windows\System32\mmc.exe`, "", "Консоль управления"},
		{`C:\Windows\System32\taskkill.exe`, "", "taskkill"},
		{`C:\Windows\System32\sc.exe`, "", "sc"},
		{`C:\Program Files\PowerShell\7\pwsh.exe`, "", "PowerShell"},
		{`C:\Program Files\WindowsApps\Microsoft.PowerShell_7.4.6.0_x64__8wekyb3d8bbwe\pwsh.exe`, "", "PowerShell"},
		{`C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.21.2361.0_x64__8wekyb3d8bbwe\WindowsTerminal.exe`, "", "Терминал"},
		{`C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.21.2361.0_x64__8wekyb3d8bbwe\wt.exe`, "", "Терминал"},
		// Не инструменты.
		{`C:\Users\misha\Desktop\cmd.exe`, "", ""},
		{`C:\Program Files\Game\cmd.exe`, "", ""},
		{`C:\Users\misha\pwsh.exe`, "", ""},
		{`C:\Windows\System32\notepad.exe`, "", ""},
		{`C:\Windows\System32\cmd2.exe`, "", ""},
		{`C:\Program Files\WindowsApps\Some.App_1.0.0.0_x64__abc\WindowsTerminal.exe`, "", ""},
		{`C:\Windows\System32\conhost.exe`, "", ""},
		{`C:\Windows\explorer.exe`, "", ""},
		// Тестовый режим: прямо в тестовой папке.
		{`C:\pcb-test\apps\cmd.exe`, `C:\pcb-test\apps\`, "Командная строка"},
		{`C:\pcb-test\apps\sub\cmd.exe`, `C:\pcb-test\apps`, ""},
		{`C:\pcb-test\apps\game.exe`, `C:\pcb-test\apps`, ""},
	}
	for _, c := range cases {
		got, ok := r.SystemTool(c.path, c.test)
		if ok != (c.want != "") || got != c.want {
			t.Errorf("SystemTool(%q, %q) = %q, %v; ждали %q", c.path, c.test, got, ok, c.want)
		}
	}
	if !IsTerminal(`C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1_x64__8w\WindowsTerminal.exe`) || IsTerminal(`c:\windows\system32\cmd.exe`) {
		t.Error("IsTerminal ошибается")
	}
}
