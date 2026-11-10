package programs

import "testing"

const wa = `c:\program files\windowsapps`

// Ловит (RM4-11, D-66, S46): ключ приложения магазина зависит от версии или архитектуры пакета;
// регистр пути даёт другой ключ; повторный вызов меняет ключ; папка не по образцу или путь вне
// папки пакетов меняется; тестовая папка пакетов (StoreRoot) не действует или действует вместе с
// настоящей WindowsApps.
func TestStableKey(t *testing.T) {
	edgeKey := wa + `\microsoft.microsoftedge.stable__8wekyb3d8bbwe\msedge_proxy.exe`
	cases := []struct{ in, want string }{
		{wa + `\microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe\msedge_proxy.exe`, edgeKey},
		{`C:\Program Files\WindowsApps\Microsoft.MicrosoftEdge.Stable_131.0.2903.51_neutral__8wekyb3d8bbwe\msedge_proxy.exe`, edgeKey},
		{wa + `\microsoft.minecraftuwp_1.21.4401.0_x64__8wekyb3d8bbwe\minecraft.windows.exe`, wa + `\microsoft.minecraftuwp__8wekyb3d8bbwe\minecraft.windows.exe`},
		{wa + `\contoso.tool_2.0.0.0_x64__abcdef\bin\tool.exe`, wa + `\contoso.tool__abcdef\bin\tool.exe`},
		{edgeKey, edgeKey},
		{wa + `\strange\x.exe`, wa + `\strange\x.exe`},
		{wa + `\x.exe`, wa + `\x.exe`},
		{wa + `\a_b_c_d\x.exe`, wa + `\a_b_c_d\x.exe`},
		{`c:\program files\geogebra\geogebra.exe`, `c:\program files\geogebra\geogebra.exe`},
		{`C:\Users\Миша\Downloads\Game.exe`, `c:\users\миша\downloads\game.exe`},
		{``, ``},
	}
	for _, k := range cases {
		if got := testRoots.StableKey(k.in); got != k.want {
			t.Errorf("StableKey(%q) = %q, want %q", k.in, got, k.want)
		}
		if got := testRoots.StableKey(testRoots.StableKey(k.in)); got != k.want {
			t.Errorf("StableKey дважды (%q) = %q, want %q", k.in, got, k.want)
		}
	}
	if got := testRoots.StoreDir(); got != wa {
		t.Errorf("StoreDir() = %q, want %q", got, wa)
	}
	if got := (Roots{}).StoreDir(); got != "" {
		t.Errorf("StoreDir() без Program Files = %q, want пусто", got)
	}

	tr := testRoots
	tr.Store = `c:\temp\pcb-programs\store`
	if got := tr.StoreDir(); got != `c:\temp\pcb-programs\store` {
		t.Errorf("StoreDir() тестовой папки = %q", got)
	}
	test := []struct{ in, want string }{
		{`c:\temp\pcb-programs\store\contoso.tool_2.0.0.0_x64__abcdef\tool.exe`, `c:\temp\pcb-programs\store\contoso.tool__abcdef\tool.exe`},
		{wa + `\microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe\msedge_proxy.exe`,
			wa + `\microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe\msedge_proxy.exe`},
	}
	for _, k := range test {
		if got := tr.StableKey(k.in); got != k.want {
			t.Errorf("StableKey с тестовой папкой (%q) = %q, want %q", k.in, got, k.want)
		}
	}
	if got := tr.AppRoot(`c:\temp\pcb-programs\store\contoso.tool_2.0.0.0_x64__abcdef\tool.exe`); got != `c:\temp\pcb-programs\store\contoso.tool__abcdef` {
		t.Errorf("AppRoot в тестовой папке = %q", got)
	}
	if !tr.IsSystem(`c:\temp\pcb-programs\store\microsoftwindows.client.cbs_1000.1_x64__cw5n1h2txyewy\x.exe`) {
		t.Error("пакет компонента Windows в тестовой папке не система")
	}
}

// Ловит (RM4-11, RF9-2, PL-30): скрытие или решение родителя по пути одной версии пакета не
// действует на другую версию; ключ сам по себе не узнаётся классификатором; пакет компонента
// Windows по ключу перестаёт быть системой; игра магазина по ключу не узнаётся каталогом.
func TestClassifyStoreKey(t *testing.T) {
	p130 := wa + `\microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe\msedge_proxy.exe`
	p131 := wa + `\microsoft.microsoftedge.stable_131.0.2903.51_neutral__8wekyb3d8bbwe\msedge_proxy.exe`
	key := wa + `\microsoft.microsoftedge.stable__8wekyb3d8bbwe\msedge_proxy.exe`
	c := NewClassifier(testRoots, DefaultCatalog)
	c.Reload([]Entry{{Path: p130, Name: "msedge_proxy", Category: "other", Hidden: true}})
	if got, want := c.Classify(p131, "", ""), (Result{Verdict: VerdictHidden, Category: "other", Known: true, Name: "msedge_proxy"}); got != want {
		t.Errorf("скрытая версия 130, запуск 131: %+v, want %+v", got, want)
	}
	if got := c.Classify(key, "", "").Verdict; got != VerdictHidden {
		t.Errorf("ключ после скрытия версии 130: %v, want скрыта", got)
	}
	c.Reload([]Entry{{Path: key, Category: "blocked"}})
	if got := c.Classify(p130, "", "").Verdict; got != VerdictForbidden {
		t.Errorf("запрет по ключу, запуск 130: %v, want запрещена", got)
	}
	c.Reload([]Entry{{Path: p131, Category: "games", Fingerprint: "1:1"}})
	if !c.NeedsFingerprint(p130) {
		t.Error("NeedsFingerprint(130) при отпечатке у 131 = false, want true")
	}
	cbs := wa + `\microsoftwindows.client.cbs_1000.26100.1.0_x64__cw5n1h2txyewy\searchhost.exe`
	if got := c.Classify(cbs, "", "").Verdict; got != VerdictSystem {
		t.Errorf("пакет компонента Windows: %v, want система", got)
	}
	c.Reload(nil)
	mc := wa + `\microsoft.minecraftuwp_1.21.4401.0_x64__8wekyb3d8bbwe\launcher.exe`
	if got := c.Classify(mc, "", ""); got.Verdict != VerdictAllow || got.Category != "games" {
		t.Errorf("игра магазина по имени пакета: %+v, want игры", got)
	}
}
