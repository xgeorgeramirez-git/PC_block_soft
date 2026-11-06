package winhost

import (
	"strings"
	"testing"
)

// manifestPub — манифест пакета с издателем publisher.
func manifestPub(publisher string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10">
  <Properties><DisplayName>Microsoft Edge</DisplayName><PublisherDisplayName>` + publisher + `</PublisherDisplayName></Properties>
  <Applications><Application Id="App" Executable="msedge_proxy.exe"/></Applications>
</Package>`
}

// Ловит (RM4-11): издатель приложения магазина не берётся из манифеста; ссылка ms-resource
// показывается родителю как издатель; испорченный манифест роняет разбор.
func TestManifestPublisher(t *testing.T) {
	cases := []struct{ data, want string }{
		{manifestPub("Microsoft Corporation"), "Microsoft Corporation"},
		{manifestPub("  Contoso Ltd. "), "Contoso Ltd."},
		{manifestPub("ms-resource:PublisherName"), ""},
		{manifestPub("MS-Resource:x"), ""},
		{manifestApp, ""},
		{"<Package><Properties><PublisherDisplayName>", ""},
	}
	for _, k := range cases {
		if got := manifestPublisher([]byte(k.data)); got != k.want {
			t.Errorf("manifestPublisher(%.60q…) = %q, want %q", k.data, got, k.want)
		}
	}
}

// Ловит (RM4-11): для ключа магазина берётся не новейшая версия пакета, версии сравниваются
// строкой («9» новее «10»), берётся ресурсный пакет или пакет другого издателя.
func TestPickStoreDir(t *testing.T) {
	names := []string{
		"microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe",
		"Microsoft.MicrosoftEdge.Stable_131.0.2903.51_neutral__8wekyb3d8bbwe",
		"microsoft.microsoftedge.stable_131.0.2903.51_neutral_split.scale-100_8wekyb3d8bbwe",
		"microsoft.microsoftedge.stable_199.0.0.0_neutral_split.scale-100_8wekyb3d8bbwe",
		"microsoft.microsoftedge.stable_99.0.0.0_neutral__otherpub",
		"microsoft.microsoftedge.stable__8wekyb3d8bbwe",
	}
	if got := pickStoreDir(names, "microsoft.microsoftedge.stable", "8wekyb3d8bbwe"); got != "Microsoft.MicrosoftEdge.Stable_131.0.2903.51_neutral__8wekyb3d8bbwe" {
		t.Errorf("pickStoreDir(Edge) = %q", got)
	}
	if got := pickStoreDir([]string{"a.b_9.0.0.0_x64__p", "a.b_10.0.0.0_x64__p", "a.b_x.0_x64__p"}, "a.b", "p"); got != "a.b_10.0.0.0_x64__p" {
		t.Errorf("pickStoreDir(9 и 10) = %q, want 10.0.0.0", got)
	}
	if got := pickStoreDir(names, "contoso.tool", "abcdef"); got != "" {
		t.Errorf("pickStoreDir(нет пакета) = %q, want пусто", got)
	}
}

// Ловит (RM4-11, T6-10): свойства по ключу магазина ищутся по несуществующему пути; издатель не
// из манифеста новейшей версии; у пакета Microsoft без манифеста и без CompanyName издатель пустой;
// путь вне папки пакетов идёт не к свойствам файла.
func TestMetaReaderStore(t *testing.T) {
	const store = `c:\program files\windowsapps`
	fsys := newFakeFS(nil)
	v130 := store + `\microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe`
	v131 := store + `\microsoft.microsoftedge.stable_131.0.2903.51_neutral__8wekyb3d8bbwe`
	fsys.addFile(v130+`\AppxManifest.xml`, manifestPub("Old Publisher"))
	fsys.addFile(v131+`\AppxManifest.xml`, manifestPub("Microsoft Corporation"))
	fsys.addFile(store+`\contoso.tool_1.0.0.0_x64__abcdef\AppxManifest.xml`, manifestPub("ms-resource:Pub"))
	var asked []string
	file := func(p string) (string, string, string) {
		asked = append(asked, p)
		switch {
		case strings.HasSuffix(p, `\tool.exe`):
			return "Contoso Company", "Tool", "3:3"
		case strings.HasSuffix(p, `geogebra.exe`):
			return "International GeoGebra Institute", "GeoGebra", "4:4"
		}
		return "Edge Company", "Edge proxy", "1:1" // CompanyName exe слабее манифеста
	}
	m := metaReader{store: store, fs: fsys, file: file}
	cases := []struct {
		path, pub, descr, fp, asked string
	}{
		{store + `\microsoft.microsoftedge.stable__8wekyb3d8bbwe\msedge_proxy.exe`, "Microsoft Corporation", "Edge proxy", "1:1", v131 + `\msedge_proxy.exe`},
		{v130 + `\msedge_proxy.exe`, "Old Publisher", "Edge proxy", "1:1", v130 + `\msedge_proxy.exe`},
		{store + `\contoso.tool__abcdef\tool.exe`, "Contoso Company", "Tool", "3:3", store + `\contoso.tool_1.0.0.0_x64__abcdef\tool.exe`},
		{store + `\microsoft.windowscalculator__8wekyb3d8bbwe\calculatorapp.exe`, "Microsoft Corporation", "", "", ""},
		{store + `\contoso.gone__abcdef\x.exe`, "", "", "", ""},
		{`c:\program files\geogebra\geogebra.exe`, "International GeoGebra Institute", "GeoGebra", "4:4", `c:\program files\geogebra\geogebra.exe`},
	}
	for _, k := range cases {
		asked = nil
		pub, descr, fp := m.read(k.path)
		if pub != k.pub || descr != k.descr || fp != k.fp {
			t.Errorf("read(%q) = %q, %q, %q; want %q, %q, %q", k.path, pub, descr, fp, k.pub, k.descr, k.fp)
		}
		want := []string{}
		if k.asked != "" {
			want = []string{k.asked}
		}
		if strings.Join(asked, "|") != strings.Join(want, "|") {
			t.Errorf("read(%q): свойства файла спрошены по %v, want %v", k.path, asked, want)
		}
	}
	// Без папки пакетов — всё как свойства файла.
	asked = nil
	if pub, _, _ := (metaReader{fs: fsys, file: file}).read(v131 + `\msedge_proxy.exe`); pub != "Edge Company" || len(asked) != 1 {
		t.Errorf("без папки пакетов: издатель %q, спрошено %v", pub, asked)
	}
}
