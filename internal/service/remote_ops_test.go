package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
	"pcblock/internal/remote"
)

// Команды телефона протокола 1.5 в службе (FR-44, FR-45, FR-47; S61, S62, S66 на фейках): тот же
// путь, что кнопки окна родителя (PL-83), номер seq до эффекта, сбой записи — stale (R-6).

// opPhone — тестовый телефон с часами службы (ts команды — по настоящим часам службы, ±5 мин).
type opPhone struct {
	*testPhone
	h *harness
}

func opsPhone(h *harness, p *testPhone) opPhone { return opPhone{p, h} }

// op — команда op с сырыми полями fields (`"code":"SUMMER","min":30` → CodeRaw, MinRaw …).
func (p opPhone) op(seq, child int64, op string, delta int, fields map[string]string) remote.CmdResult {
	p.t.Helper()
	c := remote.Command{V: remote.ProtoVersion, T: remote.TypeCmd, PC: p.pcID, Phone: p.id, Seq: seq, TS: p.h.real.Now().UnixMilli(), Child: child, Delta: delta, Op: op}
	for k, v := range fields {
		raw := json.RawMessage(v)
		switch k {
		case "code":
			c.CodeRaw = raw
		case "min":
			c.MinRaw = raw
		case "count":
			c.CountRaw = raw
		case "len":
			c.LenRaw = raw
		case "charset":
			c.CharsetRaw = raw
		case "key":
			c.KeyRaw = raw
		case "cat":
			c.CatRaw = raw
		default:
			p.t.Fatalf("поле %s", k)
		}
	}
	return p.sendCmd(p.key, c)
}

// countChanged — счётчик вызовов remoteChanged (крючок под s.mu).
func (h *harness) countChanged() func() int {
	n := 0
	h.svc.mu.Lock()
	h.svc.rem.onChanged = func() { n++ }
	h.svc.mu.Unlock()
	return func() int {
		h.svc.mu.Lock()
		defer h.svc.mu.Unlock()
		return n
	}
}

func (h *harness) bonusOf(id int64) int {
	h.t.Helper()
	b, err := h.store.GetBonusBalance(id)
	if err != nil {
		h.t.Fatal(err)
	}
	return b
}

// Ловит (S61): «Положить» с телефона идёт не путём «Начислить» окна (причина, журнал «с телефона»,
// предел 600 за раз без предела баланса); обнуление пустой копилки пишет журнал; повтор seq
// применяется; баланс в отчёте ответа не новый.
func TestRemoteBankAddClear(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)
	id := h.childID(childSID)
	if _, err := h.store.AppendBonus(id, 45, "Начислено родителем"); err != nil {
		t.Fatal(err)
	}
	ph := opsPhone(h, pairPhone(t, h, tok, r))

	res := ph.op(1, id, remote.OpBankAdd, 30, nil)
	if !res.OK || res.Message != "Положено 30 минут. В копилке 75 мин" {
		t.Fatalf("bankAdd 30: %+v", res)
	}
	if c := reportChild(t, res, id); c.BankMin != 75 {
		t.Fatalf("отчёт в ответе: bankMin %d", c.BankMin)
	}
	if got := (remoteHost{h.svc}).Children(false); got[0].BankMin != 75 {
		t.Fatalf("Children: bankMin %d", got[0].BankMin)
	}
	if got := h.journalTexts(evBonusGrant); !reflect.DeepEqual(got, []string{"Миша: +30 мин в копилку с телефона"}) {
		t.Fatalf("журнал: %q", got)
	}
	if got := h.bonusReasons(id); !reflect.DeepEqual(got, []string{"Начислено родителем", "С телефона"}) {
		t.Fatalf("история копилки: %q", got)
	}
	if res := ph.op(2, id, remote.OpBankAdd, 600, nil); !res.OK || res.Message != "Положено 600 минут. В копилке 675 мин" || h.bonusOf(id) != 675 {
		t.Fatalf("bankAdd 600: %+v, баланс %d", res, h.bonusOf(id))
	}
	if res := ph.op(3, id, remote.OpBankClear, 0, nil); !res.OK || res.Message != "Копилка обнулена" || reportChild(t, res, id).BankMin != 0 {
		t.Fatalf("bankClear: %+v", res)
	}
	if got := h.journalTexts(evBonusClear); !reflect.DeepEqual(got, []string{"Миша: копилка обнулена с телефона"}) {
		t.Fatalf("журнал обнуления: %q", got)
	}
	if res := ph.op(4, id, remote.OpBankClear, 0, nil); !res.OK || res.Message != "Копилка уже пуста" {
		t.Fatalf("повторное обнуление: %+v", res)
	}
	if n := len(h.store.eventsOf(evBonusClear)); n != 1 {
		t.Fatalf("записей обнуления %d, ждали 1", n)
	}
	if res := ph.op(4, id, remote.OpBankAdd, 30, nil); res.OK || res.Error != remote.ErrCodeReplay {
		t.Fatalf("повтор seq: %+v", res)
	}
	// 601 и 0 отсекает проверка до номера: range, seq не тратится.
	if res := ph.op(5, id, remote.OpBankAdd, 601, nil); res.Error != remote.ErrCodeRange || *res.LastSeq != 4 {
		t.Fatalf("601: %+v", res)
	}
	if res := ph.op(5, 999, remote.OpBankAdd, 30, nil); res.Error != remote.ErrCodeNoChild || *res.LastSeq != 5 {
		t.Fatalf("нет ребёнка: %+v", res)
	}
	if h.bonusOf(id) != 0 || h.remoteStored().LastSeq != 5 {
		t.Fatalf("баланс %d, LastSeq %d", h.bonusOf(id), h.remoteStored().LastSeq)
	}
}

// Ловит (R-6): сбой записи копилки после записи номера — ok или range вместо stale; номер не
// потрачен (повтор той же команды прошёл бы второй раз).
func TestRemoteBankStoreFailureIsStale(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)
	id := h.childID(childSID)
	ph := opsPhone(h, pairPhone(t, h, tok, r))
	h.store.mu.Lock()
	h.store.failAppendBonus = true
	h.store.mu.Unlock()
	if res := ph.op(7, id, remote.OpBankAdd, 30, nil); res.OK || res.Error != remote.ErrCodeStale || *res.LastSeq != 7 {
		t.Fatalf("сбой записи: %+v", res)
	}
	if h.remoteStored().LastSeq != 7 || len(h.store.eventsOf(evBonusGrant)) != 0 {
		t.Fatalf("LastSeq %d, журнал %d", h.remoteStored().LastSeq, len(h.store.eventsOf(evBonusGrant)))
	}
	h.store.mu.Lock()
	h.store.failAppendBonus = false
	h.store.mu.Unlock()
	if res := ph.op(7, id, remote.OpBankAdd, 30, nil); res.Error != remote.ErrCodeReplay {
		t.Fatalf("повтор после сбоя: %+v", res)
	}
}

// promoCodes — коды списка ответа по порядку.
func promoCodes(pl *remote.PromoList) (active, used []string) {
	active, used = []string{}, []string{}
	if pl == nil {
		return nil, nil
	}
	for _, p := range pl.Active {
		active = append(active, p.Code)
	}
	for _, p := range pl.Used {
		used = append(used, p.Code+"/"+p.ChildName)
	}
	return active, used
}

// Ловит (S62): промокоды с телефона не теми функциями, что окно (правила R26, журнал «с телефона»,
// повтор кода — promo_exists, удалённый можно создать снова); список без использованных или без
// имени ребёнка; коды promoGen не в ответе; ответ несёт отчёт о детях.
func TestRemotePromoOps(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)
	id := h.childID(childSID)
	if _, err := h.store.CreatePromo("AB12", 30); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := h.store.RedeemPromo(id, "ab12", h.real.Now().UnixMilli()); err != nil || !ok {
		t.Fatalf("погасить AB12: %v %v", ok, err)
	}
	ph := opsPhone(h, pairPhone(t, h, tok, r))

	res := ph.op(1, 0, remote.OpPromoList, 0, nil)
	if a, u := promoCodes(res.Promos); !res.OK || res.Message != "" || res.Report != nil || len(a) != 0 || !reflect.DeepEqual(u, []string{"AB12/Миша"}) {
		t.Fatalf("promoList: %+v %v %v", res, a, u)
	}
	res = ph.op(2, 0, remote.OpPromoCreate, 0, map[string]string{"code": `" summer "`, "min": "30"})
	if a, _ := promoCodes(res.Promos); !res.OK || res.Message != "Промокод SUMMER на 30 минут готов" || !reflect.DeepEqual(a, []string{"SUMMER"}) || res.Report != nil {
		t.Fatalf("promoCreate: %+v", res)
	}
	res = ph.op(3, 0, remote.OpPromoGen, 0, map[string]string{"count": "5", "len": "6", "charset": `"mixed"`, "min": "15"})
	if !res.OK || res.Message != "Готово: 5 кодов на 15 мин" || len(res.Codes) != 5 || len(res.Promos.Active) != 6 {
		t.Fatalf("promoGen: %+v", res)
	}
	for _, c := range res.Codes {
		if len(c) != 6 || strings.ToUpper(c) != c {
			t.Fatalf("код %q", c)
		}
	}
	res = ph.op(4, 0, remote.OpPromoDelete, 0, map[string]string{"code": `"summer"`})
	if a, _ := promoCodes(res.Promos); !res.OK || res.Message != "Промокод SUMMER удалён" || len(a) != 5 {
		t.Fatalf("promoDelete: %+v", res)
	}
	want := []string{"Промокод SUMMER на 30 мин создан с телефона", "Придумано 5 промокодов на 15 мин с телефона", "Промокод SUMMER на 30 мин удалён с телефона"}
	got := append(append(h.journalTexts(evPromoCreate), h.journalTexts(evPromoGenerate)...), h.journalTexts(evPromoDelete)...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("журнал:\n%q\nждали\n%q", got, want)
	}
	// Удалённый создаётся снова; существующий (и использованный) — promo_exists со списком.
	if res := ph.op(5, 0, remote.OpPromoCreate, 0, map[string]string{"code": `"SUMMER"`, "min": "30"}); !res.OK {
		t.Fatalf("SUMMER снова: %+v", res)
	}
	res = ph.op(6, 0, remote.OpPromoCreate, 0, map[string]string{"code": `"summer"`, "min": "60"})
	if res.OK || res.Error != remote.ErrCodePromoExists || res.Promos == nil || *res.LastSeq != 6 {
		t.Fatalf("повтор кода: %+v", res)
	}
	if res := ph.op(7, 0, remote.OpPromoCreate, 0, map[string]string{"code": `"AB12"`, "min": "60"}); res.Error != remote.ErrCodePromoExists {
		t.Fatalf("использованный код: %+v", res)
	}
	res = ph.op(8, 0, remote.OpPromoDelete, 0, map[string]string{"code": `"NOSUCH"`})
	if res.OK || res.Error != remote.ErrCodeNotFound || res.Promos == nil || *res.LastSeq != 8 {
		t.Fatalf("удалить отсутствующий: %+v", res)
	}
	if res := ph.op(9, 0, remote.OpPromoDelete, 0, map[string]string{"code": `"AB12"`}); res.Error != remote.ErrCodeNotFound {
		t.Fatalf("удалить использованный: %+v", res)
	}
	// Код не по R26 отсекает проверка до номера: range, seq не тратится.
	if res := ph.op(10, 0, remote.OpPromoCreate, 0, map[string]string{"code": `"ПРИВЕТ"`, "min": "30"}); res.Error != remote.ErrCodeRange || *res.LastSeq != 9 {
		t.Fatalf("кириллица: %+v", res)
	}
	if n := len(h.store.eventsOf(evPromoCreate)); n != 2 {
		t.Fatalf("записей создания %d, ждали 2", n)
	}
	// Ребёнок удалён — имя пустое.
	h.svc.mu.Lock()
	delete(h.svc.names, id)
	h.svc.mu.Unlock()
	if _, u := promoCodes(ph.op(10, 0, remote.OpPromoList, 0, nil).Promos); !reflect.DeepEqual(u, []string{"AB12/"}) {
		t.Fatalf("использованный кодом удалённого ребёнка: %v", u)
	}
}

// Ловит: promo_gen при нехватке кодов не отличается от сбоя (stale) или несёт список; сбой записи
// пачки — не stale (R-6).
func TestRemotePromoGenFailures(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)
	ph := opsPhone(h, pairPhone(t, h, tok, r))
	old := promoRand
	withPromoRand(t, bytes.NewReader(make([]byte, 1<<20))) // одни нули: разных кодов не хватает
	res := ph.op(1, 0, remote.OpPromoGen, 0, map[string]string{"count": "50", "len": "4", "charset": `"digits"`, "min": "15"})
	if res.OK || res.Error != remote.ErrCodePromoGen || res.Promos != nil || *res.LastSeq != 1 {
		t.Fatalf("не хватило кодов: %+v", res)
	}
	promoRand = old
	h.store.mu.Lock()
	h.store.failCreatePromos = true
	h.store.mu.Unlock()
	if res := ph.op(2, 0, remote.OpPromoGen, 0, map[string]string{"count": "2", "len": "6", "charset": `"letters"`, "min": "15"}); res.Error != remote.ErrCodeStale || *res.LastSeq != 2 {
		t.Fatalf("сбой записи: %+v", res)
	}
}

// decideHarness — служба со связью, программами и ждущей SuperGame (Миша 3 попытки), Миша на экране.
func decideHarness(t *testing.T) (*harness, string, opPhone, *testRelay) {
	t.Helper()
	r := newTestRelay(t)
	meta := newMeta()
	meta.set(pathSuperGame, "", "Super Game", "1024:1700000000")
	h, tok := remoteHarness(t, withPrograms(typicalScan()), withMeta(meta))
	id := h.childID(childSID)
	at := h.real.Now().Add(-time.Hour).UnixMilli()
	for i := 0; i < 3; i++ {
		h.store.AddPendingAttempt(PendingProgram{Path: pathSuperGame, Name: "SuperGame"}, id, at+int64(i)*60_000)
	}
	h.childLogsIn()
	h.svc.mu.Lock()
	delete(h.svc.guardCmd, childSession)
	h.svc.mu.Unlock()
	return h, tok, opsPhone(h, pairPhone(t, h, tok, r)), r
}

// decidedPlatesOf — плашки решения в очереди стража сеанса; очередь очищается.
func (h *harness) decidedPlatesOf(session uint32) []ipc.BlockedPlate {
	h.svc.mu.Lock()
	defer h.svc.mu.Unlock()
	out := []ipc.BlockedPlate{}
	for _, c := range h.svc.guardCmd[session] {
		if c.Kind == ipc.GuardCmdBlocked && c.Plate != nil && c.Plate.Kind == ipc.BlockedDecided {
			out = append(out, *c.Plate)
		}
	}
	delete(h.svc.guardCmd, session)
	return out
}

// Ловит (S66): решение с телефона не путём «Ждут решения» окна (категория, плашка ребёнку на
// экране, журнал «с телефона», отчёт сразу); ждущая остаётся; повтор того же ключа решает второй
// раз; ответ с отчётом, где программа ещё ждёт.
func TestRemoteDecide(t *testing.T) {
	h, _, ph, _ := decideHarness(t)
	changed := h.countChanged()
	key := remote.PendingKey(pathSuperGame)
	res := ph.op(1, 0, remote.OpDecide, 0, map[string]string{"key": `"` + key + `"`, "cat": `"games"`})
	if !res.OK || res.Message != "SuperGame: игра" || res.Report == nil || len(res.Report.Pending) != 0 || res.Report.PendingCount != 0 {
		t.Fatalf("решение: %+v", res)
	}
	if p, ok, _ := h.store.GetProgram(pathSuperGame); !ok || p.Category != "games" || p.Source != ProgramSourceParent {
		t.Fatalf("запись программы: %+v %v", p, ok)
	}
	if pend, _ := h.store.ListPending(); len(pend) != 0 {
		t.Fatalf("«Ждут решения»: %+v", pend)
	}
	want := []ipc.BlockedPlate{{Kind: ipc.BlockedDecided, Title: "Можно играть!", Text: "Родитель разрешил «SuperGame»: это игра. Запусти её снова", ShowS: 10, Path: pathSuperGame, Allowed: true}}
	if got := h.decidedPlatesOf(childSession); !reflect.DeepEqual(got, want) {
		t.Fatalf("плашка: %+v", got)
	}
	if got := h.journalTexts(evProgramDecided); !reflect.DeepEqual(got, []string{"Решение о «SuperGame» с телефона: игра"}) {
		t.Fatalf("журнал: %q", got)
	}
	if changed() != 1 {
		t.Fatalf("remoteChanged %d раз", changed())
	}
	if res := ph.op(2, 0, remote.OpDecide, 0, map[string]string{"key": `"` + key + `"`, "cat": `"blocked"`}); res.OK || res.Error != remote.ErrCodeNotFound || *res.LastSeq != 2 {
		t.Fatalf("второй раз: %+v", res)
	}
	if p, _, _ := h.store.GetProgram(pathSuperGame); p.Category != "games" {
		t.Fatalf("второе решение перезаписало первое: %+v", p)
	}
	h.svc.mu.Lock()
	undo := h.svc.progUndo
	h.svc.mu.Unlock()
	if undo != nil {
		t.Fatal("решение с телефона дало «Отменить» окна")
	}
}

// Ловит (PL-83): решение окна и телефона дают разные эффекты (запись, плашка, remoteChanged).
func TestDecideFromWindowSameAsPhone(t *testing.T) {
	type effects struct {
		Category, Source string
		Plates           []ipc.BlockedPlate
		Pending, Changed int
	}
	run := func(phone bool) effects {
		h, tok, ph, _ := decideHarness(t)
		changed := h.countChanged()
		if phone {
			if res := ph.op(1, 0, remote.OpDecide, 0, map[string]string{"key": `"` + remote.PendingKey(pathSuperGame) + `"`, "cat": `"study"`}); !res.OK {
				t.Fatalf("телефон: %+v", res)
			}
		} else {
			h.ok(adminPeer(), tok, ipc.CmdDecideProgram, ipc.DecideProgramRequest{Path: pathSuperGame, Category: "study"}, nil)
		}
		p, _, _ := h.store.GetProgram(pathSuperGame)
		pend, _ := h.store.ListPending()
		return effects{Category: p.Category, Source: p.Source, Plates: h.decidedPlatesOf(childSession), Pending: len(pend), Changed: changed()}
	}
	win, phone := run(false), run(true)
	if !reflect.DeepEqual(win, phone) || win.Changed != 1 || len(win.Plates) != 1 {
		t.Fatalf("окно %+v\nтелефон %+v", win, phone)
	}
}

// Ловит (раздел 6): ждущих больше 10 — не по убыванию последней попытки, total не все; издатель-
// заглушка уходит телефону текстом; длинный путь не урезан; kids без попыток детей.
func TestPendingReport(t *testing.T) {
	h, _ := remoteHarness(t, withChild(child2SID, "Коля", 60))
	misha, kolya := h.childID(childSID), h.childID(child2SID)
	base := h.real.Now().Add(-time.Hour).UnixMilli()
	for i := 0; i < 12; i++ {
		p := PendingProgram{Path: `c:\users\миша\downloads\game` + string(rune('a'+i)) + `.exe`, Name: "Game" + string(rune('A'+i)), Publisher: "Издатель"}
		h.store.AddPendingAttempt(p, misha, base+int64(i)*1000)
	}
	long := `c:\users\миша\` + strings.Repeat("папка\\", 32) + "supergame.exe"
	h.store.AddPendingAttempt(PendingProgram{Path: long, Name: "SuperGame"}, misha, base+100_000)
	h.store.AddPendingAttempt(PendingProgram{Path: long, Name: "SuperGame"}, kolya, base+200_000)
	h.store.AddPendingAttempt(PendingProgram{Path: long, Name: "SuperGame"}, kolya, base+300_000)

	rows, total := remoteHost{h.svc}.Pending()
	if total != 13 || len(rows) != 10 {
		t.Fatalf("строк %d, всего %d", len(rows), total)
	}
	first := rows[0]
	if first.ID != remote.PendingKey(long) || first.Name != "SuperGame" || first.Pub != "" || first.Tries != 3 || first.LastMs != base+300_000 ||
		first.Path != remote.ClipPath(long) || len([]rune(first.Path)) != 120 || !strings.HasSuffix(first.Path, `\supergame.exe`) {
		t.Fatalf("первая строка: %+v", first)
	}
	kids := map[int64]int{}
	for _, k := range first.Kids {
		kids[k.ID] = k.Tries
	}
	if !reflect.DeepEqual(kids, map[int64]int{misha: 1, kolya: 2}) {
		t.Fatalf("kids: %+v", first.Kids)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i-1].LastMs < rows[i].LastMs {
			t.Fatalf("не по убыванию: %d %d", rows[i-1].LastMs, rows[i].LastMs)
		}
		if rows[i].Pub != "Издатель" {
			t.Fatalf("издатель: %+v", rows[i])
		}
	}
	if rows[9].Name != "GameD" {
		t.Fatalf("последняя из 10: %+v", rows[9])
	}
}

// Ловит (протокол, раздел 6): новая ждущая программа не шлёт отчёт сразу (телефон узнает через
// минуту) или шлёт на каждую повторную попытку.
func TestNewPendingReportsNow(t *testing.T) {
	w := newWatcher()
	h := watchHarness(t, w)
	changed := h.countChanged()
	explorer := h.manualReady(w)
	before := changed()
	startManual(w, explorer, pathSuperGame)
	h.watch()
	if n := changed() - before; n != 1 {
		t.Fatalf("первая попытка: remoteChanged %d раз", n)
	}
	h.real.Advance(time.Second)
	h.poll(activeSample)
	startManual(w, explorer, pathSuperGame)
	h.watch()
	if pend, _ := h.store.ListPending(); len(pend) != 1 || pend[0].Attempts != 2 {
		t.Fatalf("вторая попытка не записана: %+v", pend)
	}
	if n := changed() - before; n != 1 {
		t.Fatalf("вторая попытка: remoteChanged ещё раз (%d)", n)
	}
}

// Ловит (B-79, одна истина): отчёт телефону считает игры, видео и их лимиты не так, как карточка
// «Сегодня» окна; «без лимита видео» уходит нулём, а не −1.
func TestChildrenGamesVideoLimits(t *testing.T) {
	h, tok := remoteHarness(t, withGames(childSID, 60), func(h *harness) {
		id := h.childID(childSID)
		h.store.days[dayKey(id, "2026-10-19")] = core.DayState{Day: "2026-10-19", TotalS: 2400, GamesS: 2400, VideoS: 600}
	})
	var today ipc.GetTodayReply
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, &today)
	card := today.Children[0]
	rep := remoteHost{h.svc}.Children(false)[0]
	if rep.GamesSec != 2400 || rep.GamesLimSec != 3600 || rep.VideoSec != 600 || rep.VideoLimSec != -1 {
		t.Fatalf("отчёт: игры %d из %d, видео %d из %d", rep.GamesSec, rep.GamesLimSec, rep.VideoSec, rep.VideoLimSec)
	}
	if card.GamesText != ipc.GamesCardText(int(rep.GamesSec), int(rep.GamesLimSec), false) || card.GamesText != "Игры 40 мин из 1 ч" || card.VideoText != "" {
		t.Fatalf("карточка: %q %q", card.GamesText, card.VideoText)
	}
	if withToday := (remoteHost{h.svc}).Children(true)[0]; withToday.GamesSec != 2400 || withToday.GamesLimSec != 3600 || withToday.VideoLimSec != -1 {
		t.Fatalf("с today: %+v", withToday)
	}
}

// Ловит (ревью 0.8.4 M2, класс A): родитель уменьшил лимит игр ниже сыгранного — окно считало
// потраченное как «лимит − остаток» («Игры 60 из 60 мин»), а телефон показывал сыгранное из отчёта
// («Игры 70 из 60 мин»). Карточка окна несёт то же число секунд, что отчёт телефону.
func TestCardUsedMatchesReportOverLimit(t *testing.T) {
	h, tok := remoteHarness(t, withGames(childSID, 60), func(h *harness) {
		id := h.childID(childSID)
		h.store.days[dayKey(id, "2026-10-19")] = core.DayState{Day: "2026-10-19", TotalS: 4200, GamesS: 4200, VideoS: 900}
	})
	var today ipc.GetTodayReply
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, &today)
	card := today.Children[0]
	rep := remoteHost{h.svc}.Children(false)[0]
	if rep.GamesSec != 4200 || rep.GamesLimSec != 3600 || rep.VideoSec != 900 {
		t.Fatalf("отчёт: игры %d из %d, видео %d", rep.GamesSec, rep.GamesLimSec, rep.VideoSec)
	}
	if card.Status.GamesRemainingS != 0 {
		t.Fatalf("остаток игр %d, ждали 0 (остаток не бывает < 0)", card.Status.GamesRemainingS)
	}
	if card.GamesUsedS != 4200 || card.VideoUsedS != 900 {
		t.Fatalf("карточка: игры %d с, видео %d с; ждали 4200 и 900, как в отчёте", card.GamesUsedS, card.VideoUsedS)
	}
}

// Ловит (ревью T1, замечание «в»): окно (cardOf) раньше отчёта меняет «с какого момента» у телефона:
// первый вызов без замера ставит «не за компьютером с этого момента», а замер на тике должен
// двигать момент только при смене состояния.
func TestPresenceWindowFirstDoesNotSkewReport(t *testing.T) {
	h, tok := remoteHarness(t)
	id := h.childID(childSID)
	h.svc.mu.Lock()
	h.svc.children[id].presence = presence{}
	h.svc.mu.Unlock()
	// Окно спросило карточку до первого замера — «не за компьютером» с этого момента.
	t0 := h.clock.Now().UnixMilli()
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, nil)
	// Ребёнок сел: тик ставит «за компьютером» с момента тика, а не с вызова окна.
	h.clock.Advance(5 * time.Second)
	h.childLogsIn()
	h.run(1, activeSample)
	tOn := h.svc.children[id].presence.sinceMs
	h.clock.Advance(30 * time.Second)
	h.ok(adminPeer(), tok, ipc.CmdGetToday, nil, nil) // окно ещё раз — момент не двигается
	rep := remoteHost{h.svc}.Children(true)[0].Today
	if !rep.NowAt || rep.SinceMs != tOn || tOn <= t0 {
		t.Fatalf("за компьютером: nowAt %v с %d (тик %d, окно %d)", rep.NowAt, rep.SinceMs, tOn, t0)
	}
}

// Ловит: пока служба читала свойства файла вне s.mu, окно решило ту же программу, а телефон затем
// перезаписал решение окна сменой категории (телефон решает только ждущую — not_found).
func TestRemoteDecideRaceWithWindow(t *testing.T) {
	r := newTestRelay(t)
	meta := newMeta()
	meta.set(pathSuperGame, "", "Super Game", "1024:1700000000")
	h, tok := remoteHarness(t, withPrograms(typicalScan()), withMeta(meta))
	h.store.AddPendingAttempt(PendingProgram{Path: pathSuperGame, Name: "SuperGame"}, h.childID(childSID), h.real.Now().UnixMilli())
	ph := opsPhone(h, pairPhone(t, h, tok, r))
	meta.hold()
	t.Cleanup(meta.release)
	got := make(chan remote.CmdResult, 1)
	go func() {
		got <- ph.op(1, 0, remote.OpDecide, 0, map[string]string{"key": `"` + remote.PendingKey(pathSuperGame) + `"`, "cat": `"games"`})
	}()
	select {
	case <-meta.in:
	case <-time.After(3 * time.Second):
		t.Fatal("служба не дошла до чтения свойств файла")
	}
	// «Окно» решило за это время: ждущей нет, программа запрещена.
	if _, _, err := h.store.ResolvePending(pathSuperGame); err != nil {
		t.Fatal(err)
	}
	if err := h.store.UpsertProgram(Program{Path: pathSuperGame, Name: "SuperGame", Category: "blocked", Source: ProgramSourceParent, DecidedAt: 1}); err != nil {
		t.Fatal(err)
	}
	meta.release()
	res := <-got
	if res.OK || res.Error != remote.ErrCodeNotFound || *res.LastSeq != 1 {
		t.Fatalf("ответ: %+v", res)
	}
	if p, _, _ := h.store.GetProgram(pathSuperGame); p.Category != "blocked" {
		t.Fatalf("телефон перезаписал решение окна: %+v", p)
	}
	if n := len(h.store.eventsOf(evProgramDecided)); n != 0 {
		t.Fatalf("журнал решения: %d", n)
	}
}

// Ловит (ревью T1, «б»): «Отменить» начисления и «Убавить» копилку в окне не шлют отчёт телефону —
// bankMin на телефоне устаревает до отчёта по расписанию (FR-44).
func TestBonusWindowChangesReportNow(t *testing.T) {
	h, tok := remoteHarness(t)
	id := h.childID(childSID)
	changed := h.countChanged()
	var g ipc.GrantBonusReply
	h.ok(adminPeer(), tok, ipc.CmdGrantBonus, ipc.GrantBonusRequest{ChildID: id, Minutes: 30, Reason: "Уборка"}, &g)
	h.ok(adminPeer(), tok, ipc.CmdUndoBonus, ipc.UndoBonusRequest{ChildID: id, BonusID: g.BonusID}, nil)
	if n := changed(); n != 2 {
		t.Fatalf("начислить и отменить: remoteChanged %d раз, ждали 2", n)
	}
	h.ok(adminPeer(), tok, ipc.CmdGrantBonus, ipc.GrantBonusRequest{ChildID: id, Minutes: 30, Reason: "Уборка"}, nil)
	h.ok(adminPeer(), tok, ipc.CmdReduceBonus, ipc.ReduceBonusRequest{ChildID: id, Minutes: 10}, nil)
	if n := changed(); n != 4 {
		t.Fatalf("начислить и убавить: remoteChanged %d раз, ждали 4", n)
	}
}

// Ловит: общая таблица правил кода (testdata/remote/promo-rules.json, её же проверяет телефон)
// разошлась с ответами «Создать код» окна (протокол 5г: телефон пишет те же тексты, что окно).
func TestCreatePromoRulesVector(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "remote", "promo-rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rules []struct {
		In    string `json:"in"`
		OK    bool   `json:"ok"`
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &rules); err != nil {
		t.Fatal(err)
	}
	h, tok := remoteHarness(t)
	for _, r := range rules {
		if r.OK {
			var rep ipc.CreatePromoReply
			h.ok(adminPeer(), tok, ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: r.In, Minutes: 30}, &rep)
			if rep.Promo.CodeDisplay != r.Code {
				t.Errorf("%q: код %q, в таблице %q", r.In, rep.Promo.CodeDisplay, r.Code)
			}
			continue
		}
		if f := h.fail(adminPeer(), tok, ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: r.In, Minutes: 30}); f.Error != r.Error {
			t.Errorf("%q: окно %q, в таблице %q", r.In, f.Error, r.Error)
		}
	}
}

// Ловит (R-6, ревью 0.8.4 M1): сбой записи кода, когда и список кодов не прочитан, принимается за
// «такой код уже есть» — телефон получает promo_exists вместо stale, окно — «Такой код уже есть»
// вместо «Не удалось сохранить».
func TestCreatePromoStoreDownIsNotExists(t *testing.T) {
	r := newTestRelay(t)
	h, tok := remoteHarness(t)
	ph := opsPhone(h, pairPhone(t, h, tok, r))
	h.store.mu.Lock()
	h.store.failCreatePromo, h.store.failListPromos = true, true
	h.store.mu.Unlock()
	res := ph.op(1, 0, remote.OpPromoCreate, 0, map[string]string{"code": `"WINTER"`, "min": "30"})
	if res.OK || res.Error != remote.ErrCodeStale || res.Promos != nil || *res.LastSeq != 1 {
		t.Fatalf("телефон при сбое базы: %+v", res)
	}
	f := h.fail(adminPeer(), tok, ipc.CmdCreatePromo, ipc.CreatePromoRequest{Code: "WINTER", Minutes: 30})
	if f.Error != "Не удалось сохранить. Попробуйте ещё раз" || f.Field != "" {
		t.Fatalf("окно при сбое базы: отказ %q у поля %q", f.Error, f.Field)
	}
	if n := len(h.store.eventsOf(evPromoCreate)); n != 0 {
		t.Fatalf("записей создания %d, ждали 0", n)
	}
	// Повтор кода при читаемом списке — по-прежнему promo_exists.
	h.store.mu.Lock()
	h.store.failListPromos = false
	h.store.mu.Unlock()
	if _, err := h.store.CreatePromo("SPRING", 30); err == nil {
		t.Fatal("фейк при failCreatePromo создал код")
	}
	h.store.mu.Lock()
	h.store.failCreatePromo = false
	h.store.mu.Unlock()
	if _, err := h.store.CreatePromo("SPRING", 30); err != nil {
		t.Fatal(err)
	}
	if res := ph.op(2, 0, remote.OpPromoCreate, 0, map[string]string{"code": `"spring"`, "min": "15"}); res.Error != remote.ErrCodePromoExists {
		t.Fatalf("повтор кода: %+v", res)
	}
}
