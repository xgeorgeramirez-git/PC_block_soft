package sites

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Литералы посчитаны вне Go (FNV-1a 64).
func TestHashDomain(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"", 0xcbf29ce484222325},
		{"a", 0xaf63dc4c8601ec8c},
		{"example.com", 0x576846634e2714c6},
		{"xn--e1afmkfd.xn--p1ai", 0xf36508a22f79dd1d},
	}
	for _, c := range cases {
		if got := HashDomain(c.in); got != c.want {
			t.Errorf("HashDomain(%q) = %#x, want %#x", c.in, got, c.want)
		}
	}
	if n := testing.AllocsPerRun(100, func() { HashDomain("example.com") }); n != 0 {
		t.Errorf("HashDomain выделяет память: %v", n)
	}
}

func TestEncodeAdultEmpty(t *testing.T) {
	want := "PCBADULT\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
	got := EncodeAdult(nil)
	if string(got) != want {
		t.Fatalf("EncodeAdult(nil) = %q, want %q", got, want)
	}
	a, err := OpenAdult(string(got))
	if err != nil {
		t.Fatal(err)
	}
	if a.Count() != 0 || a.ZoneCount() != 0 || a.Contains("example.com") {
		t.Errorf("пустой список: Count = %d, Contains(example.com) = %v", a.Count(), a.Contains("example.com"))
	}
}

func TestEncodeAdultLayout(t *testing.T) {
	data := EncodeAdult([]string{"example.com", "example.com", "xn--e1afmkfd.xn--p1ai"})
	if len(data) != 20+4194308+6 {
		t.Fatalf("длина %d, want %d", len(data), 20+4194308+6)
	}
	if c := binary.LittleEndian.Uint32(data[12:]); c != 2 {
		t.Errorf("count = %d, want 2", c)
	}
	off := func(b int) uint32 { return binary.LittleEndian.Uint32(data[20+4*b:]) }
	for _, c := range []struct {
		b    int
		want uint32
	}{{0, 0}, {0x57684, 0}, {0x57685, 1}, {0xf3650, 1}, {0xf3651, 2}, {1 << 20, 2}} {
		if got := off(c.b); got != c.want {
			t.Errorf("offsets[%#x] = %d, want %d", c.b, got, c.want)
		}
	}
	entries := data[20+4194308:]
	if want := []byte{0xe2, 0x34, 0x66, 0xf7, 0x22, 0x8a}; string(entries) != string(want) {
		t.Errorf("записи % x, want % x", entries, want)
	}
}

// Домен и его родители, кроме голого домена верхнего уровня; IPv4 — только точно (RM4-3).
// «uk» в списке нарочно: голый домен верхнего уровня как родитель не проверяется.
func TestAdultContainsParents(t *testing.T) {
	a, err := OpenAdult(string(EncodeAdult([]string{"porn.example", "bad.co.uk", "10.0.0.1", "uk"})))
	if err != nil {
		t.Fatal(err)
	}
	if a.Count() != 4 {
		t.Errorf("Count = %d, want 4", a.Count())
	}
	cases := []struct {
		in   string
		want bool
	}{
		{"porn.example", true}, {"www.porn.example", true}, {"x.y.porn.example", true},
		{"example", false}, {"co.uk", false}, {"good.co.uk", false}, {"a.bad.co.uk", true},
		{"10.0.0.1", true}, {"1.10.0.0.1", false}, {"", false},
	}
	for _, c := range cases {
		if got := a.Contains(c.in); got != c.want {
			t.Errorf("Contains(%q) = %v, want %v", c.in, got, c.want)
		}
		if got := a.Contains(c.in); got != c.want {
			t.Errorf("Contains(%q) из кэша = %v, want %v", c.in, got, c.want)
		}
	}
	var zero AdultList
	if zero.Contains("porn.example") || zero.Count() != 0 {
		t.Error("нулевой AdultList не пуст")
	}
}

func TestOpenAdultRejects(t *testing.T) {
	good := EncodeAdult([]string{"example.com"})
	mod := func(f func(b []byte) []byte) string {
		b := append([]byte(nil), good...)
		return string(f(b))
	}
	setOff := func(b []byte, i int, v uint32) []byte {
		binary.LittleEndian.PutUint32(b[20+4*i:], v)
		return b
	}
	cases := []struct {
		name string
		data string
	}{
		{"короче 20", "PCBADULT\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"},
		{"пусто", ""},
		{"чужая магия", "PCBADULX\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"},
		{"версия 1", "PCBADULT\x01\x00\x00\x00\x00\x00\x00\x00"},
		{"версия 3", "PCBADULT\x03\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"},
		{"count 0 с хвостом", "PCBADULT\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"},
		{"зоны длиннее файла", "PCBADULT\x02\x00\x00\x00\x00\x00\x00\x00\x04\x00\x00\x00xxx"},
		{"зона с точкой", "PCBADULT\x02\x00\x00\x00\x00\x00\x00\x00\x05\x00\x00\x00xx.xx"},
		{"пустая зона", "PCBADULT\x02\x00\x00\x00\x00\x00\x00\x00\x04\x00\x00\x00xxx\n"},
		{"зона заглавными", "PCBADULT\x02\x00\x00\x00\x00\x00\x00\x00\x03\x00\x00\x00XXX"},
		{"зона из цифр", "PCBADULT\x02\x00\x00\x00\x00\x00\x00\x00\x02\x00\x00\x0012"},
		{"count 1, длина меньше", string(good[:len(good)-1])},
		{"count 1, длина больше", string(good) + "\x00"},
		{"count 1 без таблицы", "PCBADULT\x02\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"},
		{"первое смещение не 0", mod(func(b []byte) []byte { return setOff(b, 0, 1) })},
		{"убывающие смещения", mod(func(b []byte) []byte { return setOff(b, 1<<20-1, 0) })},
		{"смещение больше count", mod(func(b []byte) []byte { return setOff(b, 5, 2) })},
		{"последнее смещение не count", mod(func(b []byte) []byte { return setOff(b, 1<<20, 0) })},
	}
	// Проверка: «убывающие смещения» действительно убывают (запись корзины 0x57684 стоит до конца).
	if binary.LittleEndian.Uint32(good[20+4*(1<<20-1):]) != 1 {
		t.Fatal("тестовый файл: последняя корзина должна начинаться с 1")
	}
	for _, c := range cases {
		if _, err := OpenAdult(c.data); !errors.Is(err, ErrAdultFormat) {
			t.Errorf("%s: err = %v, want ErrAdultFormat", c.name, err)
		}
	}
}

// Ruling RM4-3а: зоны верхнего уровня из UT1 («xxx», «porn»…) закрывают последнюю метку домена.
func TestAdultZones(t *testing.T) {
	data := EncodeAdultZones([]string{"porn.example"}, []string{"xxx", "porn", "xxx"})
	want := "PCBADULT\x02\x00\x00\x00\x01\x00\x00\x00\x08\x00\x00\x00porn\nxxx"
	if string(data[:28]) != want {
		t.Fatalf("заголовок %q, want %q", data[:28], want)
	}
	a, err := OpenAdult(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if a.ZoneCount() != 2 || a.Count() != 1 {
		t.Errorf("ZoneCount = %d, Count = %d; want 2, 1", a.ZoneCount(), a.Count())
	}
	cases := []struct {
		in   string
		want bool
	}{
		{"a.xxx", true}, {"xxx", true}, {"www.b.porn", true}, {"porn", true},
		{"xxx.com", false}, {"porn.com", false}, {"xx", false}, {"xxxx", false}, {"a.xxxx", false},
		{"porn.example", true}, {"x.porn.example", true}, {"example", false},
	}
	for _, c := range cases {
		if got := a.Contains(c.in); got != c.want {
			t.Errorf("Contains(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	// Только зоны, без доменов: таблицы корзин нет.
	z, err := OpenAdult(string(EncodeAdultZones(nil, []string{"sex"})))
	if err != nil {
		t.Fatal(err)
	}
	if !z.Contains("a.sex") || z.Contains("sex.ru") || z.Count() != 0 {
		t.Errorf("только зоны: a.sex %v, sex.ru %v, Count %d", z.Contains("a.sex"), z.Contains("sex.ru"), z.Count())
	}
	if n := testing.AllocsPerRun(100, func() { a.containsNoCache("q.w.xxx") }); n != 0 {
		t.Errorf("проверка зоны: %v выделений", n)
	}
}

func TestNormalizeZone(t *testing.T) {
	ok := []struct{ in, want string }{{"xxx", "xxx"}, {"XXX.", "xxx"}, {" porn ", "porn"}, {"рф", "xn--p1ai"}}
	for _, c := range ok {
		if got, good := NormalizeZone(c.in); !good || got != c.want {
			t.Errorf("NormalizeZone(%q) = %q, %v; want %q", c.in, got, good, c.want)
		}
	}
	for _, in := range []string{"", ".", "a.b", "123", "a b", "a/b", strings.Repeat("a", 64), "#x"} {
		if got, good := NormalizeZone(in); good || got != "" {
			t.Errorf("NormalizeZone(%q) = %q, %v; want «», false", in, got, good)
		}
	}
}

// PL-3: список читается на месте, без копии в кучу; повторный домен — из кэша без выделений.
func TestAdultNoCopy(t *testing.T) {
	s := string(EncodeAdult([]string{"porn.example", "bad.co.uk"}))
	if n := testing.AllocsPerRun(100, func() { _, _ = OpenAdult(s) }); n > 2 {
		t.Errorf("OpenAdult: %v выделений, want ≤ 2", n)
	}
	a, err := OpenAdult(s)
	if err != nil {
		t.Fatal(err)
	}
	a.Contains("www.porn.example")
	if n := testing.AllocsPerRun(100, func() { a.Contains("www.porn.example") }); n != 0 {
		t.Errorf("Contains из кэша: %v выделений, want 0", n)
	}
}

func TestAdultCacheBound(t *testing.T) {
	a, err := OpenAdult(string(EncodeAdult([]string{"porn.example"})))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10000; i++ {
		a.Contains(fmt.Sprintf("d%d.test", i))
		if n := len(a.cache.m); n > adultCacheMax {
			t.Fatalf("кэш %d записей, граница %d", n, adultCacheMax)
		}
	}
	if adultCacheMax != 4096 {
		t.Errorf("граница кэша %d, want 4096", adultCacheMax)
	}
	if !a.Contains("porn.example") {
		t.Error("после очистки кэша домен не найден")
	}
}

// В репозитории встроенный файл пустой; полный строит windows.yml (TestAdultListFull).
func TestEmbeddedEmpty(t *testing.T) {
	if os.Getenv("PCB_ADULT_FULL") != "" {
		t.Skip("PCB_ADULT_FULL: встроен полный список")
	}
	a, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if a.Count() != 0 {
		t.Errorf("встроенный список: Count = %d, want 0", a.Count())
	}
}
