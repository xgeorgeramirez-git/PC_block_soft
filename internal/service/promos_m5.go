package service

import (
	"crypto/rand"
	"fmt"
	"io"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Промокоды M5: «Придумать коды» и «Очистить историю» (FR-35, D-75a, RM5-12, RM5-14).

// promoRand — источник случайности «Придумать коды»; тесты подменяют его детерминированным.
var promoRand io.Reader = rand.Reader

// promoAlphabets — состав знаков придуманных кодов (RM5-12): наличие и буквы, и цифры в каждом
// коде mixed не гарантируется. В составах с буквами нет похожих знаков I, O, 0 и 1, чтобы ребёнок
// не ошибся при вводе (ТЗ 8.3.10, FR-35); «Только цифры» — все десять цифр.
var promoAlphabets = map[string]string{
	ipc.CharsetDigits:  "0123456789",
	ipc.CharsetLetters: "ABCDEFGHJKLMNPQRSTUVWXYZ",
	ipc.CharsetMixed:   "ABCDEFGHJKLMNPQRSTUVWXYZ23456789",
}

// promoMaxRejects — сколько байт подряд вне честного диапазона терпит выбор знака: у исправного
// источника вероятность такого ряда ничтожна, у сломанного — отказ вместо зависания под s.mu.
const promoMaxRejects = 1000

// genPromoCodes — count разных кодов длины length из алфавита charset, не из taken (нормализованные коды базы);
// rnd — источник случайности (crypto/rand.Reader в работе). Не хватило попыток (count*100) — ok = false.
// Знак выбирается без смещения по модулю: байт вне 0…(256 − 256 mod n − 1) отбрасывается. Источник
// кончился или сломан — ok = false.
func genPromoCodes(rnd io.Reader, count, length int, charset string, taken map[string]bool) (codes []string, ok bool) {
	alpha, known := promoAlphabets[charset]
	if !known || count < 1 || length < 1 {
		return nil, false
	}
	n := len(alpha)
	limit := 256 - 256%n
	var buf [1]byte
	pick := func() (byte, bool) {
		for rejects := 0; rejects < promoMaxRejects; rejects++ {
			if _, err := io.ReadFull(rnd, buf[:]); err != nil {
				return 0, false
			}
			if int(buf[0]) < limit {
				return alpha[int(buf[0])%n], true
			}
		}
		return 0, false
	}
	codes = make([]string, 0, count)
	batch := map[string]bool{}
	code := make([]byte, length)
	for attempt := 0; attempt < count*100 && len(codes) < count; attempt++ {
		for i := range code {
			c, good := pick()
			if !good {
				return nil, false
			}
			code[i] = c
		}
		s := string(code)
		norm := core.NormalizePromoCode(s)
		if taken[norm] || batch[norm] {
			continue
		}
		batch[norm] = true
		codes = append(codes, s)
	}
	if len(codes) < count {
		return nil, false
	}
	return codes, true
}

// hGeneratePromos — «Придумать коды» (RM5-12): пределы — у своего поля; коды не совпадают ни с
// одним кодом базы (действующим и использованным, сравнение нормализованных) и между собой;
// запись — одной транзакцией хранилища, сбой — ничего не создано; в журнал одна строка.
func (s *Service) hGeneratePromos(c *callCtx) ipc.Reply {
	var req ipc.GeneratePromosRequest
	if c.msg.Into(&req) != nil {
		return bad()
	}
	if req.Count < 1 || req.Count > 50 {
		return ipc.FailField("Можно от 1 до 50 кодов", ipc.FieldCount)
	}
	if req.Length < 4 || req.Length > 12 {
		return ipc.FailField("Длина от 4 до 12 знаков", ipc.FieldLength)
	}
	if _, known := promoAlphabets[req.Charset]; !known {
		return bad()
	}
	if !core.PromoMinutesValid(req.Minutes) {
		return ipc.FailField("Можно от 1 до 600 минут", ipc.FieldMinutes)
	}
	list, err := s.cfg.Store.ListPromos()
	if err != nil {
		// Ещё ничего не сохранялось: «Не удалось выполнить», не «сохранить».
		return ipc.Fail(textRetry)
	}
	taken := make(map[string]bool, len(list))
	for _, p := range list {
		taken[core.NormalizePromoCode(p.CodeDisplay)] = true
	}
	codes, ok := genPromoCodes(promoRand, req.Count, req.Length, req.Charset, taken)
	if !ok {
		return ipc.Fail("Не получилось придумать столько разных кодов. Увеличьте длину")
	}
	created, err := s.cfg.Store.CreatePromos(codes, req.Minutes)
	if err != nil {
		// База могла измениться извне (повтор UNIQUE) — пачка откатилась целиком.
		s.logRate("genpromos", "не удалось записать придуманные промокоды: %v", err)
		return ipc.Fail(textRetry)
	}
	views := make([]ipc.PromoView, 0, len(created))
	for _, p := range created {
		views = append(views, promoViewOf(p, ""))
	}
	s.journal.Write(0, evPromoGenerate, map[string]any{
		"count": len(created), "minutes": req.Minutes, "codes": codes,
		"text": promoGenerateText(len(created), req.Minutes),
	})
	return ipc.OK(ipc.GeneratePromosReply{
		Promos:  views,
		Message: fmt.Sprintf("Готово: %s на %d мин", codesText(len(created)), req.Minutes),
	})
}

// hClearPromoHistory — «Очистить историю» промокодов (RM5-14): использованные коды удаляются,
// действующие остаются; удалённый код можно создать заново.
func (s *Service) hClearPromoHistory(c *callCtx) ipc.Reply {
	n, err := s.cfg.Store.ClearUsedPromos()
	if err != nil {
		s.logRate("clearpromos", "не удалось очистить историю промокодов: %v", err)
		return ipc.Fail(textSaveFailed)
	}
	s.journal.Write(0, evPromoHistoryCleared, map[string]any{
		"count": n, "text": fmt.Sprintf("История промокодов очищена (%d)", n),
	})
	return ipc.OK(ipc.ClearPromoHistoryReply{Message: "История промокодов очищена"})
}

// codesText — «1 код», «2 кода», «5 кодов», «11 кодов», «21 код» (по последним двум цифрам).
func codesText(n int) string {
	return fmt.Sprintf("%d %s", n, pluralRu(n, "код", "кода", "кодов"))
}

// promoGenerateText — строка журнала «Придумать коды»: «Придуман 1 промокод на 15 мин»,
// «Придумано 20 промокодов на 15 мин».
func promoGenerateText(n, minutes int) string {
	verb := "Придумано"
	if m := abs(n); m%10 == 1 && m%100 != 11 {
		verb = "Придуман"
	}
	return fmt.Sprintf("%s %d %s на %d мин", verb, n, pluralRu(n, "промокод", "промокода", "промокодов"), minutes)
}

// pluralRu выбирает форму слова по числу: one — 1, 21; few — 2–4, 22–24; many — 0, 5–20, 25.
func pluralRu(n int, one, few, many string) string {
	n = abs(n)
	switch {
	case n%100 >= 11 && n%100 <= 14:
		return many
	case n%10 == 1:
		return one
	case n%10 >= 2 && n%10 <= 4:
		return few
	}
	return many
}
