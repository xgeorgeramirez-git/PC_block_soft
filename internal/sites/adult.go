package sites

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"slices"
	"sync"
)

// Встроенный список «18+» (RM4-3, ST2, D-67): домены базы UT1 «adult» как 44-битные хеши FNV-1a.
//
// Формат adult.bin (все целые little-endian):
//
//	0   8            магия «PCBADULT»
//	8   4            версия uint32 = 1
//	12  4            count — число записей uint32 (уникальные 44-битные хеши)
//	16  4·(2²⁰+1)    только при count > 0: offsets[b] — индекс первой записи корзины b;
//	                 offsets[2²⁰] = count; неубывающие, offsets[0] = 0
//	…   3·count      младшие 24 бита каждой записи, внутри корзины по возрастанию
//
// Хеш h = HashDomain(d), h44 = h >> 20; корзина — старшие 20 бит h44, запись — младшие 24.
// Пустой файл — 16 байт без таблицы корзин (в репозитории лежит он; полный строит windows.yml).
// Ложное совпадение хеша — около 1 на миллион проверок домена: цена — сайт закрыт как «18+»,
// родитель снимает добавлением домена в свой список (RM4-5).

// ErrAdultFormat — файл списка испорчен (PL-9: порча — ошибка, а не «пусто без слов»).
var ErrAdultFormat = errors.New("sites: файл списка 18+ повреждён")

const (
	adultMagic    = "PCBADULT"
	adultVersion  = 1
	adultHeader   = 16
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

// EncodeAdult — файл формата из нормализованных доменов (для инструмента и тестов).
func EncodeAdult(domains []string) []byte {
	hs := make([]uint64, 0, len(domains))
	for _, d := range domains {
		hs = append(hs, HashDomain(d))
	}
	return encodeHashes(hs)
}

// encodeHashes — файл формата из 64-битных хешей: 44 старших бита, сортировка, без повторов.
// Порядок hs меняется.
func encodeHashes(hs []uint64) []byte {
	for i := range hs {
		hs[i] >>= 20
	}
	slices.Sort(hs)
	hs = slices.Compact(hs)
	head := make([]byte, adultHeader)
	copy(head, adultMagic)
	binary.LittleEndian.PutUint32(head[8:], adultVersion)
	binary.LittleEndian.PutUint32(head[12:], uint32(len(hs)))
	if len(hs) == 0 {
		return head
	}
	out := make([]byte, adultHeader+adultOffsets+3*len(hs))
	copy(out, head)
	offs := out[adultHeader : adultHeader+adultOffsets]
	ents := out[adultHeader+adultOffsets:]
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
	data  string
	count int
	cache *adultCache
}

// adultCache — результат Contains по исходному домену; при adultCacheMax записях очищается целиком.
type adultCache struct {
	mu sync.Mutex
	m  map[string]bool
}

func le32(s string, i int) uint32 {
	return uint32(s[i]) | uint32(s[i+1])<<8 | uint32(s[i+2])<<16 | uint32(s[i+3])<<24
}

// OpenAdult — список из данных формата. Проверяет магию, версию, длину, монотонность смещений
// и последнее смещение; иначе ErrAdultFormat.
func OpenAdult(data string) (AdultList, error) {
	if len(data) < adultHeader || data[:8] != adultMagic || le32(data, 8) != adultVersion {
		return AdultList{}, ErrAdultFormat
	}
	count := int(le32(data, 12))
	if count == 0 {
		if len(data) != adultHeader {
			return AdultList{}, ErrAdultFormat
		}
		return AdultList{data: data, cache: &adultCache{}}, nil
	}
	if len(data) != adultHeader+adultOffsets+3*count {
		return AdultList{}, ErrAdultFormat
	}
	prev := uint32(0)
	if le32(data, adultHeader) != 0 {
		return AdultList{}, ErrAdultFormat
	}
	for b := 1; b <= adultBuckets; b++ {
		o := le32(data, adultHeader+4*b)
		if o < prev {
			return AdultList{}, ErrAdultFormat
		}
		prev = o
	}
	if int(prev) != count {
		return AdultList{}, ErrAdultFormat
	}
	return AdultList{data: data, count: count, cache: &adultCache{}}, nil
}

// Count — число записей (уникальных 44-битных хешей).
func (a AdultList) Count() int { return a.count }

// Contains — домен или его родитель в списке. Родители — без левой метки, пока в остатке есть
// точка (голый «com» не проверяется); IPv4 — только точное совпадение. Результат кэшируется.
func (a AdultList) Contains(domain string) bool {
	if a.count == 0 || domain == "" {
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
	lo := int(le32(a.data, adultHeader+4*b))
	hi := int(le32(a.data, adultHeader+4*(b+1)))
	base := adultHeader + adultOffsets
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
