package sites

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"slices"
	"strings"
	"sync"
)

// Встроенный список «18+» (RM4-3, ST2, D-67): домены базы UT1 «adult» как 44-битные хеши FNV-1a.
//
// Формат adult.bin (все целые little-endian):
//
//	0   8            магия «PCBADULT»
//	8   4            версия uint32 = 2
//	12  4            count — число записей uint32 (уникальные 44-битные хеши)
//	16  4            zlen — длина списка зон в байтах uint32
//	20  zlen         зоны верхнего уровня (Ruling RM4-3а): метки ASCII через «\n», без повторов
//	…   4·(2²⁰+1)    только при count > 0: offsets[b] — индекс первой записи корзины b;
//	                 offsets[2²⁰] = count; неубывающие, offsets[0] = 0
//	…   3·count      младшие 24 бита каждой записи, внутри корзины по возрастанию
//
// Хеш h = HashDomain(d), h44 = h >> 20; корзина — старшие 20 бит h44, запись — младшие 24.
// Зоны — голые домены верхнего уровня из UT1 («xxx», «porn», «sex», «adult»): закрыт любой домен,
// у которого последняя метка — такая зона. Старый формат не читается: файл всегда строится заново.
// Пустой файл — 20 байт без зон и таблицы корзин (в репозитории лежит он; полный строит windows.yml).
// Ложное совпадение хеша — около 1 на миллион проверок домена: цена — сайт закрыт как «18+»,
// родитель снимает добавлением домена в свой список (RM4-5).

// ErrAdultFormat — файл списка испорчен (PL-9: порча — ошибка, а не «пусто без слов»).
var ErrAdultFormat = errors.New("sites: файл списка 18+ повреждён")

const (
	adultMagic    = "PCBADULT"
	adultVersion  = 2
	adultHeader   = 20
	adultZonesMax = 4096
	adultBuckets  = 1 << 20
	adultOffsets  = 4 * (adultBuckets + 1)
	adultCacheMax = 4096
)

//go:embed adult/adult.bin
var adultBin string

// HashDomain — FNV-1a 64 бита по байтам домена. Свой цикл: hash/fnv выделяет память.
func HashDomain(domain string) uint64 {
	h := uint64(0xcbf29ce484222325)
	for i := 0; i < len(domain); i++ {
		h ^= uint64(domain[i])
		h *= 0x100000001b3
	}
	return h
}

// EncodeAdult — файл формата из нормализованных доменов без зон (для инструмента и тестов).
func EncodeAdult(domains []string) []byte { return EncodeAdultZones(domains, nil) }

// EncodeAdultZones — файл формата из нормализованных доменов и зон (NormalizeZone).
func EncodeAdultZones(domains, zones []string) []byte {
	hs := make([]uint64, 0, len(domains))
	for _, d := range domains {
		hs = append(hs, HashDomain(d))
	}
	return encodeHashesZones(hs, zones)
}

// encodeHashes — файл формата из 64-битных хешей без зон.
func encodeHashes(hs []uint64) []byte { return encodeHashesZones(hs, nil) }

// encodeHashesZones — файл формата из 64-битных хешей (44 старших бита, сортировка, без повторов)
// и зон (сортировка, без повторов). Порядок hs меняется.
func encodeHashesZones(hs []uint64, zones []string) []byte {
	zs := slices.Clone(zones)
	slices.Sort(zs)
	zs = slices.Compact(zs)
	ztext := strings.Join(zs, "\n")
	for i := range hs {
		hs[i] >>= 20
	}
	slices.Sort(hs)
	hs = slices.Compact(hs)
	head := make([]byte, adultHeader, adultHeader+len(ztext))
	copy(head, adultMagic)
	binary.LittleEndian.PutUint32(head[8:], adultVersion)
	binary.LittleEndian.PutUint32(head[12:], uint32(len(hs)))
	binary.LittleEndian.PutUint32(head[16:], uint32(len(ztext)))
	head = append(head, ztext...)
	if len(hs) == 0 {
		return head
	}
	base := len(head)
	out := make([]byte, base+adultOffsets+3*len(hs))
	copy(out, head)
	offs := out[base : base+adultOffsets]
	ents := out[base+adultOffsets:]
	i := 0
	for b := 0; b <= adultBuckets; b++ {
		for i < len(hs) && int(hs[i]>>24) < b {
			i++
		}
		binary.LittleEndian.PutUint32(offs[4*b:], uint32(i))
	}
	for j, h := range hs {
		ents[3*j] = byte(h)
		ents[3*j+1] = byte(h >> 8)
		ents[3*j+2] = byte(h >> 16)
	}
	return out
}

// AdultList — открытый список «18+». Данные не копируются: строка читается индексом на месте
// (PL-3, NFR-5). Нулевое значение — пустой список.
type AdultList struct {
	data   string
	count  int
	base   int    // начало таблицы корзин: adultHeader + zlen
	zones  string // зоны через «\n» (подстрока data)
	nzones int
	cache  *adultCache
}

// adultCache — результат Contains по исходному домену; при adultCacheMax записях очищается целиком.
type adultCache struct {
	mu sync.Mutex
	m  map[string]bool
}

func le32(s string, i int) uint32 {
	return uint32(s[i]) | uint32(s[i+1])<<8 | uint32(s[i+2])<<16 | uint32(s[i+3])<<24
}

// OpenAdult — список из данных формата. Проверяет магию, версию, зоны, длину, монотонность
// смещений и последнее смещение; иначе ErrAdultFormat.
func OpenAdult(data string) (AdultList, error) {
	if len(data) < adultHeader || data[:8] != adultMagic || le32(data, 8) != adultVersion {
		return AdultList{}, ErrAdultFormat
	}
	count := int(le32(data, 12))
	zlen := int(le32(data, 16))
	if zlen > adultZonesMax || len(data) < adultHeader+zlen {
		return AdultList{}, ErrAdultFormat
	}
	base := adultHeader + zlen
	zones := data[adultHeader:base]
	nzones := 0
	if zlen > 0 {
		for z := range strings.SplitSeq(zones, "\n") {
			if !validZone(z) {
				return AdultList{}, ErrAdultFormat
			}
			nzones++
		}
	}
	a := AdultList{data: data, count: count, base: base, zones: zones, nzones: nzones, cache: &adultCache{}}
	if count == 0 {
		if len(data) != base {
			return AdultList{}, ErrAdultFormat
		}
		return a, nil
	}
	if len(data) != base+adultOffsets+3*count {
		return AdultList{}, ErrAdultFormat
	}
	prev := uint32(0)
	if le32(data, base) != 0 {
		return AdultList{}, ErrAdultFormat
	}
	for b := 1; b <= adultBuckets; b++ {
		o := le32(data, base+4*b)
		if o < prev {
			return AdultList{}, ErrAdultFormat
		}
		prev = o
	}
	if int(prev) != count {
		return AdultList{}, ErrAdultFormat
	}
	return a, nil
}

// Count — число записей (уникальных 44-битных хешей).
func (a AdultList) Count() int { return a.count }

// ZoneCount — число зон верхнего уровня (Ruling RM4-3а).
func (a AdultList) ZoneCount() int { return a.nzones }

// validZone — зона в файле: 1…63 знака a-z, 0-9, «-», «_», не только цифры.
func validZone(z string) bool {
	if z == "" || len(z) > maxLabelLen || isNumeric(z) {
		return false
	}
	for i := 0; i < len(z); i++ {
		c := z[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

// NormalizeZone — голый домен верхнего уровня из строки UT1 («XXX.» → «xxx», «рф» → «xn--p1ai»)
// или («», false), если это не одна метка (Ruling RM4-3а).
func NormalizeZone(s string) (string, bool) {
	z := strings.TrimSuffix(strings.TrimSpace(s), ".")
	if z == "" || strings.Contains(z, ".") {
		return "", false
	}
	enc, ok := toASCII(z)
	if !ok || !validZone(enc) {
		return "", false
	}
	return enc, true
}

// inZone — последняя метка домена есть в списке зон. Без выделений памяти.
func (a AdultList) inZone(domain string) bool {
	if a.nzones == 0 {
		return false
	}
	label := domain[strings.LastIndexByte(domain, '.')+1:]
	for z := a.zones; ; {
		i := strings.IndexByte(z, '\n')
		if i < 0 {
			return z == label
		}
		if z[:i] == label {
			return true
		}
		z = z[i+1:]
	}
}

// Contains — домен или его родитель в списке. Родители — без левой метки, пока в остатке есть
// точка (голый «com» не проверяется); IPv4 — только точное совпадение. Результат кэшируется.
func (a AdultList) Contains(domain string) bool {
	if (a.count == 0 && a.nzones == 0) || domain == "" {
		return false
	}
	c := a.cache
	c.mu.Lock()
	if v, ok := c.m[domain]; ok {
		c.mu.Unlock()
		return v
	}
	c.mu.Unlock()
	v := a.containsNoCache(domain)
	c.mu.Lock()
	if c.m == nil || len(c.m) >= adultCacheMax {
		c.m = make(map[string]bool, 64)
	}
	c.m[domain] = v
	c.mu.Unlock()
	return v
}

// containsNoCache — Contains без кэша (для замера ST2).
func (a AdultList) containsNoCache(domain string) bool {
	if a.inZone(domain) {
		return true
	}
	if a.count == 0 {
		return false
	}
	for d := domain; ; {
		if a.hasHash(HashDomain(d)) {
			return true
		}
		var more bool
		if d, more = parentDomain(d, domain); !more {
			return false
		}
	}
}

// hasHash — двоичный поиск записи в корзине.
func (a AdultList) hasHash(h uint64) bool {
	h44 := h >> 20
	b := int(h44 >> 24)
	want := uint32(h44 & 0xFFFFFF)
	lo := int(le32(a.data, a.base+4*b))
	hi := int(le32(a.data, a.base+4*(b+1)))
	base := a.base + adultOffsets
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		p := base + 3*mid
		v := uint32(a.data[p]) | uint32(a.data[p+1])<<8 | uint32(a.data[p+2])<<16
		switch {
		case v == want:
			return true
		case v < want:
			lo = mid + 1
		default:
			hi = mid
		}
	}
	return false
}

var (
	embeddedOnce sync.Once
	embeddedList AdultList
	embeddedErr  error
)

// Embedded — встроенный список (OpenAdult встроенного файла, один раз).
func Embedded() (AdultList, error) {
	embeddedOnce.Do(func() { embeddedList, embeddedErr = OpenAdult(adultBin) })
	return embeddedList, embeddedErr
}
