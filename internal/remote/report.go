package remote

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"unicode"
)

// Размер отчёта pc/report (Ruling FB27-2, протокол, раздел 6): сервер связи принимает тело ≤ 16 КБ.
const (
	// MaxReportBody — JSON тела запроса pc/report целиком ({"body":{"data","sig"}}), байт.
	MaxReportBody = 14 << 10
	// MaxTodayPrograms, MaxTodaySites — строк списков на ребёнка в отчёте не больше.
	MaxTodayPrograms = 20
	MaxTodaySites    = 10
	// MaxItemNameRunes — название программы или сайта в отчёте не длиннее, знаков Unicode.
	MaxItemNameRunes = 60
	// maxSigB64 — base64url подписи ECDSA P-256 в DER наибольшей длины (72 байта).
	maxSigB64 = 96
)

// Категории строк today (TimeItem.Cat) — как вкладки «Итогов» окна.
const (
	ItemGame  = "game"
	ItemStudy = "study"
	ItemVideo = "video"
	ItemOther = "other"
)

// reportSteps — ступени урезания списков у всех детей сразу, если отчёт больше MaxReportBody:
// не больше n строк в каждом списке (поверх MaxTodayPrograms и MaxTodaySites).
var reportSteps = []int{MaxTodayPrograms, 10, 5, 0}

// reportBodySize — байтов тела pc/report для data длиной n: base64url без «=», подпись — наибольшей
// длины (настоящая короче или равна), обёртка — как её пишет Client.SendReport.
func reportBodySize(n int) int {
	return len(`{"body":{"data":"","sig":""}}`) + base64.RawURLEncoding.EncodedLen(n) + maxSigB64
}

// reportPendingCut — строк pending на ступени урезания (раздел 6), когда не помещаются и списки 0.
const reportPendingCut = 3

// fitReport — байты data отчёта, тело которого не больше limit: списки программ и сайтов
// по убыванию времени, названия ≤ MaxItemNameRunes знаков, пустые списки — []; программ не больше
// MaxTodayPrograms, сайтов — MaxTodaySites; не поместилось — списки у всех детей урезаются по
// ступеням (10 → 5 → 0), игры отбрасываются последними (на телефоне «Во что играл»); в more —
// сколько строк не вошло. Не поместилось и без списков — pending до 3 строк, затем отчёт без today
// (остаток и запрет важнее статистики), затем pending 0 (pendingCount остаётся). Не помещается и
// так — последняя ступень.
func fitReport(rep Report, limit int) ([]byte, error) {
	var raw []byte
	for _, cand := range reportCandidates(rep) {
		var err error
		if raw, err = json.Marshal(cand); err != nil {
			return nil, err
		}
		if reportBodySize(len(raw)) <= limit {
			break
		}
	}
	return raw, nil
}

// reportCandidates — ступени урезания отчёта по порядку (раздел 6): списки today 20 → 10 → 5 → 0,
// pending 3, без today, pending 0. Без today ступени списков совпадают — идёт одна. Каждая — копия.
func reportCandidates(rep Report) []Report {
	base := normalizeReport(rep)
	out := []Report{}
	if hasToday(base) {
		for _, n := range reportSteps {
			out = append(out, capToday(base, n))
		}
	} else {
		out = append(out, base)
	}
	last := out[len(out)-1]
	if len(last.Pending) > reportPendingCut {
		last = capPending(last, reportPendingCut)
		out = append(out, last)
	}
	if hasToday(last) {
		last = withoutToday(last)
		out = append(out, last)
	}
	if len(last.Pending) > 0 {
		out = append(out, capPending(last, 0))
	}
	return out
}

// normalizeReport — копия отчёта: today как normalizeToday; pending не больше MaxPending строк,
// названия ItemName, пути ClipPath, nil — [] (Go nil-слайс ушёл бы телефону как null).
func normalizeReport(rep Report) Report {
	out := normalizeToday(rep)
	pend := make([]PendingReport, 0, min(len(rep.Pending), MaxPending))
	for _, p := range rep.Pending {
		if len(pend) == MaxPending {
			break
		}
		p.Name, p.Path = ItemName(p.Name), ClipPath(p.Path)
		if p.Kids == nil {
			p.Kids = []PendingKid{}
		}
		pend = append(pend, p)
	}
	out.Pending = pend
	if out.PendingCount < len(rep.Pending) {
		out.PendingCount = len(rep.Pending)
	}
	return out
}

// capPending — копия отчёта, где pending не больше n строк (первые — самые свежие); pendingCount
// не меняется.
func capPending(rep Report, n int) Report {
	out := rep
	out.Pending = rep.Pending[:min(n, len(rep.Pending)):min(n, len(rep.Pending))]
	return out
}

// replyBodySize — байтов тела pc/reply для data длиной n: обёртка как у Client.Reply (map → ключи по
// алфавиту: {"body":{"data":"","sig":""},"kind":"","replyTo":"","to":""}, строки — как их пишет
// json.Marshal) + base64url(n) + подпись наибольшей длины.
func replyBodySize(n int, kind, replyTo, to string) int {
	env, _ := json.Marshal(map[string]any{"to": to, "replyTo": replyTo, "kind": kind, "body": Signed{}})
	return len(env) + base64.RawURLEncoding.EncodedLen(n) + maxSigB64
}

// fitReply — байты data ответа cmdr/noter, тело pc/reply которого ≤ limit. С report — ступени
// reportCandidates (в ответе today нет: pending до 3, затем 0, pendingCount остаётся); с promos —
// used ≤ 50 и active ≤ 200, затем used 20, затем 0, затем active вдвое меньше до 0 (раздел 5г);
// more* — сколько не вошло; codes не урезаются. Не помещается и так — последняя ступень.
func fitReply(res CmdResult, kind, replyTo, to string, limit int) ([]byte, error) {
	cands := []CmdResult{res}
	switch {
	case res.Report != nil:
		cands = cands[:0]
		for _, rep := range reportCandidates(*res.Report) {
			c := res
			c.Report = &rep
			cands = append(cands, c)
		}
	case res.Promos != nil:
		cands = cands[:0]
		for _, pl := range promoCandidates(*res.Promos) {
			c := res
			c.Promos = &pl
			cands = append(cands, c)
		}
	}
	var raw []byte
	for _, c := range cands {
		var err error
		if raw, err = json.Marshal(c); err != nil {
			return nil, err
		}
		if replyBodySize(len(raw), kind, replyTo, to) <= limit {
			break
		}
	}
	return raw, nil
}

// promoCandidates — ступени урезания списка промокодов (раздел 5г), каждая — копия с More*.
func promoCandidates(pl PromoList) []PromoList {
	active, used := pl.Active, pl.Used
	if active == nil {
		active = []PromoActive{}
	}
	if used == nil {
		used = []PromoUsed{}
	}
	totalA, totalU := len(active)+pl.MoreActive, len(used)+pl.MoreUsed
	mk := func(a, u int) PromoList {
		a, u = min(a, len(active)), min(u, len(used))
		return PromoList{Active: active[:a:a], Used: used[:u:u], MoreActive: totalA - a, MoreUsed: totalU - u}
	}
	a := min(len(active), MaxActivePromos)
	out := []PromoList{mk(a, MaxUsedPromos)}
	for _, u := range []int{20, 0} {
		if u < len(out[len(out)-1].Used) {
			out = append(out, mk(a, u))
		}
	}
	for a > 0 {
		a /= 2
		out = append(out, mk(a, 0))
	}
	return out
}

func hasToday(rep Report) bool {
	for _, c := range rep.Children {
		if c.Today != nil {
			return true
		}
	}
	return false
}

// normalizeToday — копия отчёта: списки today отсортированы (дольше — выше, при равенстве — по
// названию), названия очищены и обрезаны, nil — [] (Go nil-слайс ушёл бы телефону как null).
func normalizeToday(rep Report) Report {
	out := rep
	out.Children = make([]ChildReport, len(rep.Children))
	for i, c := range rep.Children {
		if c.Today != nil {
			t := *c.Today
			t.Programs = cleanItems(t.Programs)
			t.Sites = cleanItems(t.Sites)
			c.Today = &t
		}
		out.Children[i] = c
	}
	if out.Children == nil {
		out.Children = []ChildReport{}
	}
	return out
}

func cleanItems(in []TimeItem) []TimeItem {
	out := make([]TimeItem, 0, len(in))
	for _, it := range in {
		out = append(out, TimeItem{Name: ItemName(it.Name), Cat: it.Cat, Sec: it.Sec})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sec != out[j].Sec {
			return out[i].Sec > out[j].Sec
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ItemName — название для отчёта: управляющие и неверные символы — пробел, пробелы схлопнуты,
// не длиннее MaxItemNameRunes знаков (по знакам, не по байтам: кириллица не режется посередине).
func ItemName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > MaxItemNameRunes {
		s = strings.TrimSpace(string(r[:MaxItemNameRunes]))
	}
	return s
}

// capToday — копия отчёта, где у каждого ребёнка не больше min(n, MaxTodayPrograms) программ и
// min(n, MaxTodaySites) сайтов; more прибавляет невошедшие.
func capToday(rep Report, n int) Report {
	out := rep
	out.Children = make([]ChildReport, len(rep.Children))
	for i, c := range rep.Children {
		if c.Today != nil {
			t := *c.Today
			t.Programs, t.More.Programs = capItems(t.Programs, min(n, MaxTodayPrograms), t.More.Programs)
			t.Sites, t.More.Sites = capItems(t.Sites, min(n, MaxTodaySites), t.More.Sites)
			c.Today = &t
		}
		out.Children[i] = c
	}
	return out
}

// capItems — не больше n строк упорядоченного списка in: сначала игры, затем остальные (самые
// долгие из каждой группы), порядок по убыванию сохраняется.
func capItems(in []TimeItem, n, more int) ([]TimeItem, int) {
	if len(in) <= n {
		return in, more
	}
	keep := make([]bool, len(in))
	left := n
	for _, games := range []bool{true, false} {
		for i, it := range in {
			if left > 0 && !keep[i] && (it.Cat == ItemGame) == games {
				keep[i] = true
				left--
			}
		}
	}
	out := make([]TimeItem, 0, n)
	for i, it := range in {
		if keep[i] {
			out = append(out, it)
		}
	}
	return out, more + len(in) - n
}

func withoutToday(rep Report) Report {
	out := rep
	out.Children = make([]ChildReport, len(rep.Children))
	for i, c := range rep.Children {
		c.Today = nil
		out.Children[i] = c
	}
	return out
}
