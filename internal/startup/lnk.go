package startup

import (
	"encoding/binary"
	"errors"
	"path"
	"strings"
	"unicode/utf16"
)

// Ярлык .lnk в папке «Автозагрузка» (формат Shell Link, [MS-SHLLINK]): цель берётся из
// LinkInfo (LocalBasePath + CommonPathSuffix), иначе — из относительного пути StringData.

const (
	lnkHeaderSize = 0x4c

	lnkHasIDList   = 0x01
	lnkHasLinkInfo = 0x02
	lnkHasName     = 0x04
	lnkHasRelPath  = 0x08
	lnkHasWorkDir  = 0x10
	lnkHasArgs     = 0x20
	lnkIsUnicode   = 0x80

	lnkVolumeAndLocalPath = 0x01
)

// lnkCLSID — 00021401-0000-0000-C000-000000000046 в порядке байтов файла.
var lnkCLSID = []byte{0x01, 0x14, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}

var errLnk = errors.New("startup: ярлык не разобран")

// LnkTarget — цель ярлыка .lnk и её аргументы. dir — папка, где лежит ярлык (для
// относительного пути). Цель: Юникод-путь LinkInfo; ANSI-путь LinkInfo, если он только из
// ASCII (кодовая страница системы неизвестна); иначе относительный путь от dir. Не разобрать
// (не ярлык, обрезан, цели нет) — ok = false.
func LnkTarget(data []byte, dir string) (target, args string, ok bool) {
	l, err := parseLnk(data)
	if err != nil {
		return "", "", false
	}
	target = l.local
	if target == "" && l.rel != "" && dir != "" {
		target = joinWin(dir, l.rel)
	}
	if target == "" {
		return "", "", false
	}
	return target, l.args, true
}

type lnkData struct{ local, rel, args string }

func parseLnk(b []byte) (lnkData, error) {
	var l lnkData
	if len(b) < lnkHeaderSize || binary.LittleEndian.Uint32(b) != lnkHeaderSize || string(b[4:20]) != string(lnkCLSID) {
		return l, errLnk
	}
	flags := binary.LittleEndian.Uint32(b[0x14:])
	off := lnkHeaderSize
	if flags&lnkHasIDList != 0 {
		if off+2 > len(b) {
			return l, errLnk
		}
		off += 2 + int(binary.LittleEndian.Uint16(b[off:]))
	}
	if flags&lnkHasLinkInfo != 0 {
		if off+4 > len(b) {
			return l, errLnk
		}
		size := int(binary.LittleEndian.Uint32(b[off:]))
		if size < 0x1c || off+size > len(b) {
			return l, errLnk
		}
		l.local = linkInfoPath(b[off : off+size])
		off += size
	}
	unicode := flags&lnkIsUnicode != 0
	for _, f := range []uint32{lnkHasName, lnkHasRelPath, lnkHasWorkDir, lnkHasArgs} {
		if flags&f == 0 {
			continue
		}
		s, n, err := lnkString(b, off, unicode)
		if err != nil {
			return l, err
		}
		off = n
		switch f {
		case lnkHasRelPath:
			l.rel = s
		case lnkHasArgs:
			l.args = strings.TrimSpace(s)
		}
	}
	return l, nil
}

// linkInfoPath — LocalBasePath + CommonPathSuffix из структуры LinkInfo li; "" — нет пути или
// ANSI-путь не из ASCII.
func linkInfoPath(li []byte) string {
	u32 := func(at int) int {
		if at+4 > len(li) {
			return 0
		}
		return int(binary.LittleEndian.Uint32(li[at:]))
	}
	hdr, lflags := u32(4), u32(8)
	if lflags&lnkVolumeAndLocalPath == 0 {
		return ""
	}
	if hdr >= 0x24 {
		base, okB := utf16z(li, u32(0x1c))
		suf, okS := utf16z(li, u32(0x20))
		if okB && okS && base != "" {
			return base + suf
		}
	}
	base, okB := ansiz(li, u32(0x10))
	suf, okS := ansiz(li, u32(0x18))
	if !okB || !okS || base == "" {
		return ""
	}
	return base + suf
}

// ansiz — строка с нулём в конце от at; не только ASCII или нет нуля — ok = false.
func ansiz(b []byte, at int) (string, bool) {
	if at <= 0 || at >= len(b) {
		return "", false
	}
	for i := at; i < len(b); i++ {
		if b[i] == 0 {
			return string(b[at:i]), true
		}
		if b[i] >= 0x80 {
			return "", false
		}
	}
	return "", false
}

// utf16z — строка UTF-16 LE с нулём в конце от at.
func utf16z(b []byte, at int) (string, bool) {
	if at <= 0 || at >= len(b) {
		return "", false
	}
	var u []uint16
	for i := at; i+1 < len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c == 0 {
			return string(utf16.Decode(u)), true
		}
		u = append(u, c)
	}
	return "", false
}

// lnkString — строка StringData от off: число символов (2 байта) и символы; возвращает конец.
func lnkString(b []byte, off int, unicode bool) (string, int, error) {
	if off+2 > len(b) {
		return "", 0, errLnk
	}
	n := int(binary.LittleEndian.Uint16(b[off:]))
	off += 2
	if !unicode {
		if off+n > len(b) {
			return "", 0, errLnk
		}
		return string(b[off : off+n]), off + n, nil
	}
	if off+2*n > len(b) {
		return "", 0, errLnk
	}
	u := make([]uint16, n)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[off+2*i:])
	}
	return string(utf16.Decode(u)), off + 2*n, nil
}

// joinWin соединяет папку Windows и относительный путь, сворачивая «.» и «..».
func joinWin(dir, rel string) string {
	if len(rel) >= 2 && (rel[1] == ':' || strings.HasPrefix(rel, `\\`)) {
		return rel // путь уже полный
	}
	d := strings.ReplaceAll(dir, `\`, "/")
	vol := ""
	if len(d) >= 2 && d[1] == ':' {
		vol, d = d[:2], d[2:]
	}
	p := path.Clean("/" + d + "/" + strings.ReplaceAll(rel, `\`, "/"))
	return vol + strings.ReplaceAll(p, "/", `\`)
}
