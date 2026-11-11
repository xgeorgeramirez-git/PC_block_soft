package service

import (
	"sort"
	"strconv"
	"strings"
	"time"

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

// braveRules — правила Brave по последнему удачному чтению списков. Под s.mu.
func (s *Service) braveRules() BraveRules {
	return BraveRules{Blocked: append([]string(nil), s.braveBlocked...)}
}

// applyBrave пишет правила Brave в ветку ребёнка (старт, вход, новый ребёнок, изменение списков).
// Списки ни разу не прочитались — не пишет: пустой чёрный список стёр бы «Запрещённые» (T5-5).
// Ошибка — в журнал работы не чаще errLogEvery на учётку. Под s.mu.
func (s *Service) applyBrave(cs *childState) bool {
	if !s.braveReady {
		s.logRate("brave-wait", "правила Brave не пишутся: списки сайтов ещё не прочитаны")
		return false
	}
	if err := s.cfg.Policy.SetBraveRules(cs.child.SID, s.braveRules()); err != nil {
		s.logRate("brave"+cs.child.SID, "правила Brave для %s не записаны: %v", cs.child.Name, err)
		return false
	}
	return true
}

// removeBrave снимает правила Brave с учётки, с которой сняли отметку «детская» (T5-6). Под s.mu.
func (s *Service) removeBrave(cs *childState) {
	if err := s.cfg.Policy.RemoveBraveRules(cs.child.SID); err != nil {
		s.logRate("brave"+cs.child.SID, "правила Brave для %s не сняты: %v", cs.child.Name, err)
	}
}

// housekeepBrave — шаг обслуживания (T5-8): списки изменились — записать всем детям; иначе раз в
// braveCheckEvery сверить ветку каждого ребёнка и вернуть изменённую. Под s.mu.
func (s *Service) housekeepBrave(real time.Time) {
	if s.braveDue {
		s.braveDue = false
		s.braveAt = real
		for _, id := range s.childOrder() {
			s.applyBrave(s.children[id])
		}
		return
	}
	if real.Sub(s.braveAt) < braveCheckEvery {
		return
	}
	s.braveAt = real
	if !s.braveReady {
		s.logRate("brave-wait", "правила Brave не пишутся: списки сайтов ещё не прочитаны")
		return
	}
	for _, id := range s.childOrder() {
		s.checkBrave(s.children[id], real)
	}
}

// checkBrave сверяет ветку ребёнка и возвращает её, если она изменена: журнал работы — каждое
// восстановление, журнал родителя — не чаще braveLogEvery на ребёнка (T5-7). Совпадает — ничего не
// пишет. Под s.mu.
func (s *Service) checkBrave(cs *childState, real time.Time) {
	r := s.braveRules()
	ok, err := s.cfg.Policy.BraveRulesOK(cs.child.SID, r)
	if err != nil {
		s.logRate("brave"+cs.child.SID, "правила Brave для %s не проверены: %v", cs.child.Name, err)
		return
	}
	if ok || !s.applyBrave(cs) {
		return
	}
	s.cfg.Logf("правила Brave у %s восстановлены", cs.child.Name)
	if last, seen := s.braveLogAt[cs.child.ID]; seen && real.Sub(last) < braveLogEvery && !real.Before(last) {
		return
	}
	if s.braveLogAt == nil {
		s.braveLogAt = map[int64]time.Time{}
	}
	s.braveLogAt[cs.child.ID] = real
	s.journal.Write(cs.child.ID, evBraveRestored, map[string]any{"text": "правила Brave восстановлены"})
}
