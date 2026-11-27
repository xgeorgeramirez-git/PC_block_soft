package remote

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// Размер отчёта и ответа с ждущими программами и промокодами (протокол 1.5, разделы 5г, 6; PL-84):
// тело запроса к серверу связи ≤ 14 КБ по закодированной обёртке с подписью наибольшей длины.

// bigPending — n ждущих программ с путями 260 знаков (кириллица и < > &), длинными названиями и
// всеми детьми в kids, по убыванию lastMs.
func bigPending(n, kids int) []PendingReport {
	out := []PendingReport{}
	for i := 0; i < n; i++ {
		p := PendingReport{
			ID:     PendingKey(fmt.Sprint(i)),
			Name:   strings.Repeat("Игра<&>", 15) + fmt.Sprint(i),
			Pub:    strings.Repeat("Издатель", 8),
			Path:   `C:\Users\` + strings.Repeat("Пользователь<&>", 16) + fmt.Sprintf(`\game%d.exe`, i),
			Tries:  99,
			LastMs: 1763490000000 - int64(i)*1000,
		}
		for k := 0; k < kids; k++ {
			p.Kids = append(p.Kids, PendingKid{ID: int64(k + 1), Tries: 99})
		}
		out = append(out, p)
	}
	return out
}

// replyBodyLen — настоящая длина тела pc/reply, как его пишет Client.Reply; оценка replyBodySize
// не меньше настоящей.
func replyBodyLen(t *testing.T, raw []byte, kind, replyTo, to string) int {
	t.Helper()
	s, err := Sign(mustKey(t), raw)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"to": to, "replyTo": replyTo, "kind": kind, "body": s})
	if est := replyBodySize(len(raw), kind, replyTo, to); len(body) > est {
		t.Fatalf("оценка размера тела ответа %d меньше настоящего %d", est, len(body))
	}
	return len(body)
}

// reportStage — ступень урезания отчёта по его виду: 0…3 — списки today 20/10/5/0 при всех
// ждущих, 4 — pending 3, 5 — без today, 6 — pending 0.
func reportStage(t *testing.T, rep Report, pending int) int {
	t.Helper()
	td := rep.Children[0].Today
	switch {
	case td != nil && len(rep.Pending) == min(pending, MaxPending):
		for i, s := range reportSteps {
			if len(td.Programs) == min(s, MaxTodayPrograms) {
				return i
			}
		}
	case td != nil && len(rep.Pending) == 3:
		return 4
	case td == nil && len(rep.Pending) == 3:
		return 5
	case td == nil && len(rep.Pending) == 0:
		return 6
	}
	t.Fatalf("неизвестная ступень: today %v, pending %d", td != nil, len(rep.Pending))
	return -1
}

// Ловит (PL-84): отчёт с 6 детьми × 50 программ × 50 сайтов и 10 ждущими с путями 260 знаков не
// помещается в 14 КБ; ждущие урезаются раньше списков статистики или today пропадает раньше
// pending 3; pendingCount теряется при урезании; путь не урезан до 120 знаков; null в отчёте.
func TestReportWorstCaseWithPending(t *testing.T) {
	rep := bigReport(6, 50, 50)
	rep.Pending, rep.PendingCount = bigPending(10, 6), 10
	for i := range rep.Children {
		rep.Children[i].BankMin, rep.Children[i].GamesLimSec, rep.Children[i].VideoLimSec = 99999, 999999, -1
	}
	raw, err := fitReport(rep, MaxReportBody)
	if err != nil {
		t.Fatal(err)
	}
	if n := signedBodyLen(t, raw); n > MaxReportBody {
		t.Fatalf("тело %d байт, предел %d", n, MaxReportBody)
	}
	if strings.Contains(string(raw), "null") {
		t.Fatalf("в отчёте null: %.300s", raw)
	}
	var got Report
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.PendingCount != 10 {
		t.Fatalf("pendingCount %d", got.PendingCount)
	}
	for _, p := range got.Pending {
		if utf8.RuneCountInString(p.Path) != MaxPathRunes || !strings.HasSuffix(p.Path, ".exe") || utf8.RuneCountInString(p.Name) > MaxItemNameRunes || len(p.Kids) != 6 {
			t.Fatalf("строка pending: %+v", p)
		}
	}

	// Ступени по убыванию предела: только вперёд, все семь встречаются, pendingCount всегда 10.
	seen := map[int]bool{}
	prev := -1
	for limit := 64 << 10; limit >= 1<<10; limit -= 384 {
		raw, err := fitReport(rep, limit)
		if err != nil {
			t.Fatal(err)
		}
		var r Report
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		st := reportStage(t, r, 10)
		if st < prev {
			t.Fatalf("предел %d: ступень %d после %d", limit, st, prev)
		}
		if r.PendingCount != 10 {
			t.Fatalf("предел %d: pendingCount %d", limit, r.PendingCount)
		}
		if st < 6 && reportBodySize(len(raw)) > limit {
			t.Fatalf("предел %d: ступень %d не помещается (%d)", limit, st, reportBodySize(len(raw)))
		}
		prev, seen[st] = st, true
	}
	for st := 0; st <= 6; st++ {
		if !seen[st] {
			t.Fatalf("ступень %d не встретилась: %v", st, seen)
		}
	}
}

// bigPromos — 200 действующих и 50 использованных кодов по 16 знаков, у использованных — имена
// детей по 40 знаков (кириллица).
func bigPromos() PromoList {
	pl := PromoList{Active: []PromoActive{}, Used: []PromoUsed{}}
	for i := 0; i < 200; i++ {
		pl.Active = append(pl.Active, PromoActive{Code: fmt.Sprintf("ACTIVECODE%06d", i), Min: 600, CreatedMs: 1763490000000 - int64(i)})
	}
	for i := 0; i < 50; i++ {
		pl.Used = append(pl.Used, PromoUsed{Code: fmt.Sprintf("USEDCODEXXXX%04d", i), Min: 600, UsedMs: 1763480000000 - int64(i), Child: 3, ChildName: strings.Repeat("Ж", 40)})
	}
	return pl
}

// Ловит (раздел 5г): ответ на promoGen со всеми кодами больше 14 КБ; урезаются новые коды;
// more* не сходятся; порядок списков нарушен; урезание по оценке JSON, а не по телу запроса.
func TestFitReplyPromos(t *testing.T) {
	pl := bigPromos()
	codes := []string{}
	for i := 0; i < 50; i++ {
		codes = append(codes, fmt.Sprintf("NEWCODE%05d", i))
	}
	res := CmdResult{V: 1, T: TypeCmdR, PC: "pc-x", Phone: "ph-x", Seq: 99, Cmd: strings.Repeat("h", 43), TS: 1763490000000, OK: true,
		Message: "Готово: 50 кодов на 600 мин", Codes: codes, Promos: &pl}
	const kind, replyTo, to = KindCmdR, "m-0123456789abcdef", "ph-0123456789abcdef"
	raw, err := fitReply(res, kind, replyTo, to, MaxReportBody)
	if err != nil {
		t.Fatal(err)
	}
	if n := replyBodyLen(t, raw, kind, replyTo, to); n > MaxReportBody {
		t.Fatalf("тело ответа %d байт, предел %d", n, MaxReportBody)
	}
	var got CmdResult
	if err := json.Unmarshal(raw, &got); err != nil || got.Promos == nil {
		t.Fatalf("%v %.200s", err, raw)
	}
	if len(got.Codes) != 50 || got.Codes[49] != "NEWCODE00049" {
		t.Fatalf("коды урезаны: %d", len(got.Codes))
	}
	p := got.Promos
	if len(p.Active)+p.MoreActive != 200 || len(p.Used)+p.MoreUsed != 50 {
		t.Fatalf("строк %d+%d, %d+%d", len(p.Active), p.MoreActive, len(p.Used), p.MoreUsed)
	}
	if len(p.Active) == 200 && len(p.Used) == 50 {
		t.Fatal("200 + 50 кодов не должны поместиться целиком: урезание ничего не проверяет")
	}
	for i, a := range p.Active {
		if a.Code != fmt.Sprintf("ACTIVECODE%06d", i) {
			t.Fatalf("порядок действующих: %d %s", i, a.Code)
		}
	}
	for i, u := range p.Used {
		if u.Code != fmt.Sprintf("USEDCODEXXXX%04d", i) {
			t.Fatalf("порядок использованных: %d %s", i, u.Code)
		}
	}
	// Использованные урезаются раньше действующих: действующие короче 200 только при used 0.
	if len(p.Active) < 200 && len(p.Used) != 0 {
		t.Fatalf("действующие урезаны при %d использованных", len(p.Used))
	}

	// Ступени по убыванию предела: used 50 → 20 → 0, затем active вдвое; пустые списки — [].
	prevA, prevU := 201, 51
	for limit := 32 << 10; limit >= 1<<10; limit -= 128 {
		raw, _ := fitReply(res, kind, replyTo, to, limit)
		var r CmdResult
		json.Unmarshal(raw, &r)
		a, u := len(r.Promos.Active), len(r.Promos.Used)
		if a > prevA || u > prevU || u != 50 && u != 20 && u != 0 || u > 0 && a != 200 {
			t.Fatalf("предел %d: active %d used %d после %d/%d", limit, a, u, prevA, prevU)
		}
		if strings.Contains(string(raw), "null") {
			t.Fatalf("предел %d: null в ответе", limit)
		}
		prevA, prevU = a, u
	}
	if prevA != 0 || prevU != 0 {
		t.Fatalf("на 1 КБ списки не урезаны до 0: %d/%d", prevA, prevU)
	}
	// Пустые списки — [] (не null).
	empty, _ := fitReply(CmdResult{V: 1, T: TypeCmdR, OK: true, Promos: &PromoList{}}, kind, replyTo, to, MaxReportBody)
	if !strings.Contains(string(empty), `"promos":{"active":[],"used":[],"moreActive":0,"moreUsed":0}`) {
		t.Fatalf("пустой список: %s", empty)
	}
}

// Ловит: ответ на команду с отчётом 6 детей и 10 длинными ждущими больше 14 КБ; в отчёт ответа
// попал today; pendingCount потерян.
func TestFitReplyPendingInReport(t *testing.T) {
	rep := bigReport(6, 50, 50)
	for i := range rep.Children {
		rep.Children[i].Today = nil
		rep.Children[i].Name = strings.Repeat("Ж", 40)
	}
	rep.Pending, rep.PendingCount = bigPending(10, 6), 10
	res := CmdResult{V: 1, T: TypeCmdR, PC: "pc-x", Phone: "ph-x", Seq: 1, Cmd: strings.Repeat("h", 43), TS: 1, OK: true, Message: "Положено 30 минут. В копилке 75 мин", Report: &rep}
	for _, limit := range []int{MaxReportBody, 6 << 10, 4 << 10} {
		raw, err := fitReply(res, KindCmdR, "m1", "ph1", limit)
		if err != nil {
			t.Fatal(err)
		}
		var got CmdResult
		if err := json.Unmarshal(raw, &got); err != nil || got.Report == nil {
			t.Fatalf("%v", err)
		}
		n := replyBodyLen(t, raw, KindCmdR, "m1", "ph1")
		if n > limit {
			t.Fatalf("предел %d: тело %d", limit, n)
		}
		if k := len(got.Report.Pending); k != 10 && k != 3 && k != 0 || got.Report.PendingCount != 10 {
			t.Fatalf("предел %d: pending %d, всего %d", limit, k, got.Report.PendingCount)
		}
		if limit == MaxReportBody && len(got.Report.Pending) == 10 {
			continue
		}
		if limit == 4<<10 && len(got.Report.Pending) != 0 {
			t.Fatalf("на 4 КБ pending %d", len(got.Report.Pending))
		}
	}
	// Ответ без report и promos (отказ) — как есть.
	ls := int64(7)
	bad := CmdResult{V: 1, T: TypeCmdR, OK: false, Error: ErrCodeReplay, LastSeq: &ls}
	raw, _ := fitReply(bad, KindCmdR, "m1", "ph1", MaxReportBody)
	want, _ := json.Marshal(bad)
	if string(raw) != string(want) {
		t.Fatalf("отказ изменён: %s", raw)
	}
}
