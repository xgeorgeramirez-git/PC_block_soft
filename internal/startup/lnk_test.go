package startup

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

// lnkSpec — синтетический ярлык [MS-SHLLINK] для тестов.
type lnkSpec struct {
	idList       bool
	local        string // LocalBasePath (ANSI); "" — LinkInfo без пути
	suffix       string // CommonPathSuffix (ANSI)
	localUnicode string // LocalBasePathUnicode; "" — заголовок LinkInfo 0x1C
	noLinkInfo   bool
	rel, args    string // StringData (Юникод)
	name         string
	ansiStrings  bool // StringData в ANSI (без IsUnicode)
}

func le32(v int) []byte { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, uint32(v)); return b }
func le16(v int) []byte { b := make([]byte, 2); binary.LittleEndian.PutUint16(b, uint16(v)); return b }

func u16z(s string) []byte {
	var b []byte
	for _, c := range utf16.Encode([]rune(s)) {
		b = append(b, le16(int(c))...)
	}
	return append(b, 0, 0)
}

func buildLnk(s lnkSpec) []byte {
	flags := 0
	if s.idList {
		flags |= lnkHasIDList
	}
	if !s.noLinkInfo {
		flags |= lnkHasLinkInfo
	}
	if s.name != "" {
		flags |= lnkHasName
	}
	if s.rel != "" {
		flags |= lnkHasRelPath
	}
	if s.args != "" {
		flags |= lnkHasArgs
	}
	if !s.ansiStrings {
		flags |= lnkIsUnicode
	}
	b := append(le32(lnkHeaderSize), lnkCLSID...)
	b = append(b, le32(flags)...)
	b = append(b, make([]byte, lnkHeaderSize-len(b))...)
	if s.idList {
		b = append(b, le16(4)...)
		b = append(b, 2, 0, 0, 0) // одна пустая запись и конец списка
	}
	if !s.noLinkInfo {
		hdr := 0x1c
		if s.localUnicode != "" {
			hdr = 0x24
		}
		vol := []byte{0x10, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0x10, 0, 0, 0} // VolumeID без метки
		body := vol
		localOff := hdr + len(vol)
		body = append(body, append([]byte(s.local), 0)...)
		sufOff := hdr + len(body)
		body = append(body, append([]byte(s.suffix), 0)...)
		var uLocalOff, uSufOff int
		if s.localUnicode != "" {
			uLocalOff = hdr + len(body)
			body = append(body, u16z(s.localUnicode)...)
			uSufOff = hdr + len(body)
			body = append(body, u16z("")...)
		}
		lflags := 0
		if s.local != "" || s.localUnicode != "" {
			lflags = lnkVolumeAndLocalPath
		}
		li := le32(hdr + len(body))
		li = append(li, le32(hdr)...)
		li = append(li, le32(lflags)...)
		li = append(li, le32(hdr)...) // VolumeIDOffset
		li = append(li, le32(localOff)...)
		li = append(li, le32(0)...) // CommonNetworkRelativeLinkOffset
		li = append(li, le32(sufOff)...)
		if hdr == 0x24 {
			li = append(li, le32(uLocalOff)...)
			li = append(li, le32(uSufOff)...)
		}
		b = append(b, append(li, body...)...)
	}
	str := func(v string) {
		if s.ansiStrings {
			b = append(b, le16(len(v))...)
			b = append(b, v...)
			return
		}
		u := utf16.Encode([]rune(v))
		b = append(b, le16(len(u))...)
		for _, c := range u {
			b = append(b, le16(int(c))...)
		}
	}
	for _, v := range []string{s.name, s.rel, s.args} {
		if v != "" {
			str(v)
		}
	}
	return append(b, 0, 0, 0, 0) // TerminalBlock ExtraData
}

// Ловит: цель не из LinkInfo, потерян CommonPathSuffix, Юникод-путь не предпочтён ANSI,
// ANSI не из ASCII принят как есть (кракозябры кириллицы), относительный путь не от папки
// ярлыка, пропуск IDList и строк StringData со сдвигом, аргументы потеряны.
func TestLnkTarget(t *testing.T) {
	const dir = `C:\Users\Миша\AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup`
	for _, tc := range []struct {
		name       string
		spec       lnkSpec
		want, args string
	}{
		{"ANSI-путь", lnkSpec{local: `C:\Program Files\App\app.exe`}, `C:\Program Files\App\app.exe`, ""},
		{"с IDList и аргументами", lnkSpec{idList: true, local: `C:\A\a.exe`, name: "Описание", args: " --tray "}, `C:\A\a.exe`, "--tray"},
		{"суффикс", lnkSpec{local: `C:\Tools\`, suffix: `x.exe`}, `C:\Tools\x.exe`, ""},
		{"Юникод сильнее ANSI", lnkSpec{local: `C:\Users\????\a.exe`, localUnicode: `C:\Users\Миша\a.exe`}, `C:\Users\Миша\a.exe`, ""},
		{"ANSI не ASCII — относительный путь", lnkSpec{local: "C:\\Users\\\xcc\xe8\xf8\xe0\\a.exe", rel: `..\..\..\..\..\..\Local\App\a.exe`}, `C:\Users\Миша\AppData\Local\App\a.exe`, ""},
		{"нет LinkInfo — относительный путь", lnkSpec{noLinkInfo: true, rel: `.\b.exe`, args: "-x"}, dir + `\b.exe`, "-x"},
		{"относительный путь полный", lnkSpec{noLinkInfo: true, rel: `D:\c.exe`}, `D:\c.exe`, ""},
		{"строки ANSI", lnkSpec{local: `C:\A\a.exe`, rel: `a.exe`, args: "/s", ansiStrings: true}, `C:\A\a.exe`, "/s"},
	} {
		got, args, ok := LnkTarget(buildLnk(tc.spec), dir)
		if !ok || got != tc.want || args != tc.args {
			t.Errorf("%s: LnkTarget = %q, %q, %v; want %q, %q, true", tc.name, got, args, ok, tc.want, tc.args)
		}
	}
}

// Ловит: принятие не ярлыка, выход за границы обрезанного файла (паника), цель из пустоты.
func TestLnkTargetRejects(t *testing.T) {
	good := buildLnk(lnkSpec{idList: true, local: `C:\A\a.exe`, args: "-x"})
	badCLSID := append([]byte{}, good...)
	badCLSID[5] ^= 0xff
	cases := map[string][]byte{
		"пусто":          nil,
		"не ярлык":       []byte("MZ\x90\x00 not a shortcut at all, just bytes........................................"),
		"чужой CLSID":    badCLSID,
		"нет цели":       buildLnk(lnkSpec{noLinkInfo: true, args: "-x"}),
		"LinkInfo пуст":  buildLnk(lnkSpec{}),
		"ANSI не ASCII":  buildLnk(lnkSpec{local: "C:\\\xcc\\a.exe"}),
		"только заголов": good[:lnkHeaderSize],
	}
	for n := lnkHeaderSize; n < len(good)-4; n++ { // обрезки на каждом байте после заголовка
		if _, _, ok := LnkTarget(good[:n], `C:\D`); ok {
			t.Errorf("обрезка до %d байт: ok = true", n)
		}
	}
	for name, data := range cases {
		if got, _, ok := LnkTarget(data, `C:\D`); ok {
			t.Errorf("%s: LnkTarget = %q, true; want false", name, got)
		}
	}
	if _, _, ok := LnkTarget(buildLnk(lnkSpec{noLinkInfo: true, rel: `a.exe`}), ""); ok {
		t.Error("относительный путь без папки ярлыка: ok = true")
	}
}

func FuzzLnkTarget(f *testing.F) {
	f.Add(buildLnk(lnkSpec{idList: true, local: `C:\A\a.exe`, args: "-x", rel: `a.exe`}))
	f.Add(buildLnk(lnkSpec{localUnicode: `C:\Б\b.exe`, local: `C:\?\b.exe`}))
	f.Fuzz(func(t *testing.T, b []byte) { LnkTarget(b, `C:\D`) })
}
