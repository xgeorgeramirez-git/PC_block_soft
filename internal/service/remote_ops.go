package service

import (
	"fmt"
	"sort"

	"pcblock/internal/ipc"
	"pcblock/internal/remote"
)

// Команды телефона протокола 1.5 (разделы 5в, 5г, 5д): копилка, промокоды и решения о программах.
// Каждая — под одной s.mu: поколение → номер seq записан до эффекта (takeRemoteSeq) → та же функция
// службы, что кнопка окна родителя (PL-83), с источником «телефон». Сбой записи после номера —
// stale (R-6): номер потрачен, повтор пойдёт со следующим.

// lockRemote — общий вход методов remoteHost с командами: крючок тестов до блокировки, затем s.mu.
func (h remoteHost) lockRemote() func() {
	s := h.s
	if s.rem.beforeApplyLock != nil {
		s.rem.beforeApplyLock()
	}
	s.mu.Lock()
	return s.mu.Unlock
}

// ApplyBank — «Положить в копилку» и «Обнулить копилку» с телефона (FR-44, протокол 5в): grantBonus и
// clearBonus окна с источником «телефон» (причина «С телефона», журнал «… с телефона»).
func (h remoteHost) ApplyBank(gen, seq, childID int64, op string, delta int) (string, string, int64) {
	s := h.s
	defer h.lockRemote()()
	if code, last := s.takeRemoteSeq(gen, seq); code != "" {
		return "", code, last
	}
	cs := s.children[childID]
	if cs == nil {
		return "", remote.ErrCodeNoChild, seq
	}
	s.real = s.cfg.RealNow()
	switch op {
	case remote.OpBankAdd:
		if !bonusMinutesValid(delta) {
			return "", remote.ErrCodeRange, seq
		}
		bal, fail := s.grantBonus(cs, delta, "", extraFromPhone)
		if fail != "" {
			return "", remote.ErrCodeStale, seq
		}
		return fmt.Sprintf("Положено %s. В копилке %d мин", minutesText(delta), bal), "", seq
	case remote.OpBankClear:
		was, _, fail := s.clearBonus(cs, extraFromPhone)
		if fail != "" {
			return "", remote.ErrCodeStale, seq
		}
		if was == 0 {
			return "Копилка уже пуста", "", seq
		}
		return "Копилка обнулена", "", seq
	}
	return "", remote.ErrCodeRange, seq
}

// ApplyPromo — промокоды с телефона (FR-45, протокол 5г): createPromo, generatePromos, deletePromo и
// promoLists окна с источником «телефон». Список — в ответе при ok и при promo_exists, not_found.
func (h remoteHost) ApplyPromo(gen int64, c remote.Command) remote.PromoOutcome {
	s := h.s
	defer h.lockRemote()()
	if code, last := s.takeRemoteSeq(gen, c.Seq); code != "" {
		return remote.PromoOutcome{Code: code, LastSeq: last}
	}
	s.real = s.cfg.RealNow()
	out := remote.PromoOutcome{LastSeq: c.Seq}
	switch c.Op {
	case remote.OpPromoList:
	case remote.OpPromoCreate:
		p, field, fail, exists := s.createPromo(c.Code, c.Min, extraFromPhone)
		switch {
		case exists:
			out.Code = remote.ErrCodePromoExists
		case fail != "" && field != "":
			out.Code = remote.ErrCodeRange // CheckCommand проверил те же правила; сюда не доходит
		case fail != "":
			out.Code = remote.ErrCodeStale
		default:
			out.Msg = fmt.Sprintf("Промокод %s на %s готов", p.CodeDisplay, minutesText(p.Minutes))
		}
	case remote.OpPromoGen:
		created, field, fail, noCodes := s.generatePromos(c.Count, c.Len, c.Charset, c.Min, extraFromPhone)
		switch {
		case noCodes:
			return remote.PromoOutcome{Code: remote.ErrCodePromoGen, LastSeq: c.Seq}
		case fail == ipc.TextBadRequest || fail != "" && field != "":
			return remote.PromoOutcome{Code: remote.ErrCodeRange, LastSeq: c.Seq}
		case fail != "":
			return remote.PromoOutcome{Code: remote.ErrCodeStale, LastSeq: c.Seq}
		}
		out.Codes = make([]string, 0, len(created))
		for _, p := range created {
			out.Codes = append(out.Codes, p.CodeDisplay)
		}
		out.Msg = fmt.Sprintf("Готово: %s на %d мин", codesText(len(created)), c.Min)
	case remote.OpPromoDelete:
		p, notFound, fail := s.deletePromo(c.Code, extraFromPhone)
		switch {
		case notFound:
			out.Code = remote.ErrCodeNotFound
		case fail != "":
			return remote.PromoOutcome{Code: remote.ErrCodeStale, LastSeq: c.Seq}
		default:
			out.Msg = fmt.Sprintf("Промокод %s удалён", promoViewOf(p, "").CodeDisplay)
		}
	default:
		return remote.PromoOutcome{Code: remote.ErrCodeRange, LastSeq: c.Seq}
	}
	if out.Code != "" && out.Code != remote.ErrCodePromoExists && out.Code != remote.ErrCodeNotFound {
		return out
	}
	active, used, err := s.promoLists()
	if err != nil {
		s.logRate("remote-promos", "связь с телефоном: список промокодов не прочитан: %v", err)
		if c.Op == remote.OpPromoList {
			return remote.PromoOutcome{Code: remote.ErrCodeStale, LastSeq: c.Seq}
		}
		return out // действие выполнено; телефон оставит прежний список до следующего ответа
	}
	out.List = promoListOf(active, used)
	return out
}

// promoListOf — список промокодов для телефона (протокол 5г): коды заглавными, active по убыванию
// createdMs, used — usedMs (порядок хранилища; сортировка — на случай другого порядка), имя
// ребёнка на момент ответа («» — ребёнок удалён). Пустые списки — [].
func promoListOf(active, used []ipc.PromoView) *remote.PromoList {
	pl := &remote.PromoList{Active: []remote.PromoActive{}, Used: []remote.PromoUsed{}}
	for _, p := range active {
		pl.Active = append(pl.Active, remote.PromoActive{Code: p.CodeDisplay, Min: p.Minutes, CreatedMs: p.CreatedAtMs})
	}
	for _, p := range used {
		pl.Used = append(pl.Used, remote.PromoUsed{Code: p.CodeDisplay, Min: p.Minutes, UsedMs: p.UsedAtMs, Child: p.UsedByChild, ChildName: p.UsedByName})
	}
	sort.SliceStable(pl.Active, func(i, j int) bool { return pl.Active[i].CreatedMs > pl.Active[j].CreatedMs })
	sort.SliceStable(pl.Used, func(i, j int) bool { return pl.Used[i].UsedMs > pl.Used[j].UsedMs })
	return pl
}

// ApplyDecide — решение о незнакомой программе с телефона (FR-47, протокол 5д): ждущая с тем же
// ключом PendingKey(путь) решается decideProgram окна с источником «телефон» (запись, журнал,
// плашки детям, отчёт). Нет такой ждущей — not_found («Решение уже принято»).
func (h remoteHost) ApplyDecide(gen, seq int64, key, cat string) (string, string, int64) {
	s := h.s
	defer h.lockRemote()()
	if code, last := s.takeRemoteSeq(gen, seq); code != "" {
		return "", code, last
	}
	if !parentCategories[cat] {
		return "", remote.ErrCodeRange, seq
	}
	list, err := s.cfg.Store.ListPending()
	if err != nil {
		s.logRate("pending", "не удалось прочитать «Ждут решения»: %v", err)
		return "", remote.ErrCodeStale, seq
	}
	path := ""
	for _, p := range list {
		if remote.PendingKey(p.Path) == key {
			path = p.Path
			break
		}
	}
	if path == "" {
		return "", remote.ErrCodeNotFound, seq
	}
	s.real = s.cfg.RealNow()
	name, _, _, fail := s.decideProgram(path, cat, extraFromPhone, 0, s.real)
	switch {
	case fail == textNoProgram:
		return "", remote.ErrCodeNotFound, seq
	case fail != "":
		return "", remote.ErrCodeStale, seq
	}
	return name + ": " + categoryWord[cat], "", seq
}

// Pending — «Ждут решения» для отчёта телефону (протокол, раздел 6): по убыванию последней попытки,
// не больше remote.MaxPending строк, и сколько ждёт всего. Не прочиталось — пусто, строка в журнал.
func (h remoteHost) Pending() ([]remote.PendingReport, int) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.cfg.Store.ListPending()
	if err != nil {
		s.logRate("pending", "не удалось прочитать «Ждут решения»: %v", err)
		return []remote.PendingReport{}, 0
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].LastAttemptAt > list[j].LastAttemptAt })
	out := make([]remote.PendingReport, 0, min(len(list), remote.MaxPending))
	for _, p := range list[:min(len(list), remote.MaxPending)] {
		kids := []remote.PendingKid{}
		for _, ch := range p.Children {
			kids = append(kids, remote.PendingKid{ID: ch.ChildID, Tries: ch.Attempts})
		}
		out = append(out, remote.PendingReport{
			ID: remote.PendingKey(p.Path), Name: remote.ItemName(programName(p.Name, p.Path)), Pub: p.Publisher,
			Path: remote.ClipPath(p.Path), Kids: kids, Tries: p.Attempts, LastMs: p.LastAttemptAt,
		})
	}
	return out, len(list)
}
