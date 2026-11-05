package service

import (
	"sort"
	"strconv"
	"strings"

	"pcblock/internal/ipc"
)

// Правила Brave для ребёнка (R15, FR-31, RM4-10): ветка HKU\<SID>\Software\Policies\BraveSoftware\Brave
// целиком принадлежит программе. Здесь — только данные: какие значения должны лежать в ветке и что
// записать или удалить, чтобы ветка стала такой (T5-2, T5-3). Реестр читает и пишет адаптер winhost.

// BraveRules — правила Brave ребёнка: домены «Запрещённых» (чёрный список Brave).
type BraveRules struct{ Blocked []string }

// BraveValue — одно значение ветки. Key — подключ относительно ветки («» — сама ветка,
// «URLBlocklist», «ExtensionInstallBlocklist»; глубже — через «\»); DWord — значение REG_DWORD Num,
// иначе строка Str. Адаптер отдаёт значение другого типа как строку с пустым Str, а подключ без
// значений — значением с пустым Name: так план видит и их.
type BraveValue struct {
	Key, Name string
	DWord     bool
	Num       uint32
	Str       string
}

// Подключи ветки, которые пишет программа.
const (
	braveKeyExtensions = "ExtensionInstallBlocklist"
	braveKeyURLs       = "URLBlocklist"
)

// Values — полный набор значений ветки: окна Tor и VPN выключены, защищённый DNS выключен, ни одно
// расширение не ставится, домены «Запрещённых» по алфавиту без повторов и пустых, не больше
// ipc.MaxBlockedSites (номера с 1).
func (r BraveRules) Values() []BraveValue {
	out := []BraveValue{
		{Name: "TorDisabled", DWord: true, Num: 1},
		{Name: "BraveVPNDisabled", DWord: true, Num: 1},
		{Name: "DnsOverHttpsMode", Str: "off"},
		{Key: braveKeyExtensions, Name: "1", Str: "*"},
	}
	domains := make([]string, 0, len(r.Blocked))
	seen := map[string]bool{}
	for _, d := range r.Blocked {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		domains = append(domains, d)
	}
	sort.Strings(domains)
	if len(domains) > ipc.MaxBlockedSites {
		domains = domains[:ipc.MaxBlockedSites]
	}
	for i, d := range domains {
		out = append(out, BraveValue{Key: braveKeyURLs, Name: strconv.Itoa(i + 1), Str: d})
	}
	return out
}

// braveID — ключ значения для сравнения: реестр не различает регистр имён.
func braveID(key, name string) string { return strings.ToLower(key) + "\x00" + strings.ToLower(name) }

// sameBraveValue — значения одного типа с одним содержимым.
func sameBraveValue(a, b BraveValue) bool {
	if a.DWord != b.DWord {
		return false
	}
	if a.DWord {
		return a.Num == b.Num
	}
	return a.Str == b.Str
}

// BravePlan — как привести ветку have к want, не стирая её (T5-3): set — недостающие и отличные
// значения want; del — лишние значения в ветке и в нужных подключах; delKeys — лишние подключи
// целиком (по алфавиту): чужой подключ ветки или подключ глубже нужного. Значения внутри
// удаляемого подключа в del не попадают. Пустой план — ветка совпадает.
func BravePlan(have, want []BraveValue) (set, del []BraveValue, delKeys []string) {
	wantKeys := map[string]bool{"": true}
	wantIDs := map[string]BraveValue{}
	for _, w := range want {
		wantKeys[strings.ToLower(w.Key)] = true
		wantIDs[braveID(w.Key, w.Name)] = w
	}
	haveIDs := map[string]BraveValue{}
	dropKeys := map[string]bool{}
	for _, h := range have {
		if key, drop := braveDropKey(h.Key, wantKeys); drop {
			dropKeys[key] = true
			continue
		}
		id := braveID(h.Key, h.Name)
		haveIDs[id] = h
		if _, ok := wantIDs[id]; !ok {
			del = append(del, h)
		}
	}
	for _, w := range want {
		if h, ok := haveIDs[braveID(w.Key, w.Name)]; !ok || !sameBraveValue(h, w) {
			set = append(set, w)
		}
	}
	for k := range dropKeys {
		delKeys = append(delKeys, k)
	}
	sort.Strings(delKeys)
	return set, del, delKeys
}

// braveDropKey — подключ key лишний: возвращает, что удалить целиком (верхний чужой подключ или
// сам подключ, если он глубже нужного).
func braveDropKey(key string, wantKeys map[string]bool) (string, bool) {
	if wantKeys[strings.ToLower(key)] {
		return "", false
	}
	if top, _, deep := strings.Cut(key, `\`); deep && !wantKeys[strings.ToLower(top)] {
		return top, true
	}
	return key, true
}
