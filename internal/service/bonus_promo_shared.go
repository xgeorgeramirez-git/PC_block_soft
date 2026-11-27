package service

import (
	"fmt"
	"strings"

	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// Копилка и промокоды — один путь окна родителя и телефона (FR-22, FR-44, FR-45, R26, PL-83):
// обработчики окна и команды телефона вызывают эти функции под s.mu, поэтому проверки, запись,
// журнал и отметка «телефон узнает сам» у них одни. Источник — extraSource, как у changeExtra:
// с телефона подробность журнала source: "phone" (тексты «… с телефона» — journaltext.go).

// bonusGrantMax — одно начисление в копилку, мин: 1…600 за раз; предела баланса нет (D-113).
const bonusGrantMax = 600

// textPromoExists — код с таким нормализованным видом уже есть (UNIQUE базы).
const textPromoExists = "Такой код уже есть, придумайте другой"

// bonusMinutesValid — одно начисление родителя или телефона в пределах 1…600 (FR-22, FR-44).
func bonusMinutesValid(minutes int) bool { return minutes >= 1 && minutes <= bonusGrantMax }

// sourceDetails — подробности журнала с отметкой телефона; окно — без source.
func sourceDetails(src extraSource, d map[string]any) map[string]any {
	if src == extraFromPhone {
		d["source"] = extraSourcePhone
	}
	return d
}

// grantBonus — «Начислить» окна и «Положить» телефона (FR-22, FR-44): minutes 1…600 за раз, без предела
// баланса (D-113); причина — reason окна или «С телефона»; журнал evBonusGrant (с телефона — подробность
// source). failText != "" — отказ словами окна (textBonusRange или textSaveFailed), ничего не записано.
func (s *Service) grantBonus(cs *childState, minutes int, reason string, src extraSource) (balance int, failText string) {
	if !bonusMinutesValid(minutes) {
		return 0, textBonusRange
	}
	text := fmt.Sprintf("Начислено в копилку +%d мин: %s", minutes, reason)
	if src == extraFromPhone {
		reason = "С телефона"
		text = fmt.Sprintf("Начислено в копилку +%d мин с телефона", minutes)
	}
	bal, err := s.cfg.Store.AppendBonus(cs.child.ID, minutes, reason)
	if err != nil {
		s.logRate(fmt.Sprint("grantbonus", cs.child.ID), "копилка: не начислено %d мин ребёнку %d: %v", minutes, cs.child.ID, err)
		return 0, textSaveFailed
	}
	s.journal.Write(cs.child.ID, evBonusGrant, sourceDetails(src, map[string]any{
		"minutes": minutes, "reason": reason, "balance": bal, "text": text,
	}))
	if src != extraFromPhone {
		s.remoteChanged() // баланс копилки в отчёте телефону (FR-44); команда телефона шлёт отчёт сама
	}
	return bal, ""
}

// clearBonus — «Обнулить копилку» окна и телефона (FR-22, FR-44, D-46): пусто — was = 0, без записи и журнала;
// история — «Обнулено родителем» / «Обнулено с телефона»; журнал evBonusClear (с телефона — source).
func (s *Service) clearBonus(cs *childState, src extraSource) (was, balance int, failText string) {
	bal, err := s.cfg.Store.GetBonusBalance(cs.child.ID)
	if err != nil {
		return 0, 0, textRetry // чтение, а не запись: «не удалось сохранить» сбивает с толку (B-62)
	}
	if bal <= 0 {
		return 0, 0, ""
	}
	reason, text := "Обнулено родителем", fmt.Sprintf("Копилка обнулена родителем (было %d мин)", bal)
	if src == extraFromPhone {
		reason, text = "Обнулено с телефона", fmt.Sprintf("Копилка обнулена с телефона (было %d мин)", bal)
	}
	after, err := s.cfg.Store.AppendBonus(cs.child.ID, -bal, reason)
	if err != nil {
		return 0, 0, textSaveFailed
	}
	s.journal.Write(cs.child.ID, evBonusClear, sourceDetails(src, map[string]any{"was": bal, "text": text}))
	if src != extraFromPhone {
		s.remoteChanged()
	}
	return bal, after, ""
}

// createPromo — «Создать код» (R26): проверки и тексты окна; exists — такой код уже есть (поле кода);
// журнал evPromoCreate (с телефона — source). field — ipc.FieldCode / ipc.FieldMinutes для окна.
func (s *Service) createPromo(raw string, minutes int, src extraSource) (p PromoRecord, field, failText string, exists bool) {
	if strings.TrimSpace(raw) == "" {
		return PromoRecord{}, ipc.FieldCode, "Введите код", false
	}
	// RM5-11: новый код — только A–Z и цифры, 3–16 знаков; хранится заглавными.
	display, valid := core.CheckNewPromoCode(raw)
	if !valid {
		if core.PromoCodeLatinOnly(raw) {
			return PromoRecord{}, ipc.FieldCode, "Код от 3 до 16 знаков", false
		}
		return PromoRecord{}, ipc.FieldCode, "Только латинские буквы A-Z и цифры", false
	}
	if !core.PromoMinutesValid(minutes) {
		return PromoRecord{}, ipc.FieldMinutes, "Можно от 1 до 600 минут", false
	}
	p, err := s.cfg.Store.CreatePromo(display, minutes)
	if err != nil {
		// Самая частая причина — такой код уже есть (UNIQUE); показываем у поля кода. Повтор отличаем
		// от сбоя записи по списку кодов: телефону нужен отказ promo_exists, а не «устарела» (R-6).
		// Список не прочитан — это сбой хранилища, как и прочий сбой записи (окну textSaveFailed,
		// телефону stale), а не повтор кода.
		if taken, ok := s.promoTaken(display); ok && taken {
			return PromoRecord{}, ipc.FieldCode, textPromoExists, true
		}
		s.logRate("createpromo", "не удалось записать промокод: %v", err)
		return PromoRecord{}, "", textSaveFailed, false
	}
	text := fmt.Sprintf("Создан промокод «%s» на %d мин", p.CodeDisplay, p.Minutes)
	if src == extraFromPhone {
		text = fmt.Sprintf("Промокод %s на %d мин создан с телефона", p.CodeDisplay, p.Minutes)
	}
	s.journal.Write(0, evPromoCreate, sourceDetails(src, map[string]any{"code": p.CodeDisplay, "minutes": p.Minutes, "text": text}))
	return p, "", "", false
}

// promoTaken сообщает, что код с тем же нормализованным видом уже есть в базе (действующий или
// использованный); ok == false — список не прочитан, ответа нет (сбой хранилища).
func (s *Service) promoTaken(code string) (taken, ok bool) {
	list, err := s.cfg.Store.ListPromos()
	if err != nil {
		return false, false
	}
	norm := core.NormalizePromoCode(code)
	for _, p := range list {
		if core.NormalizePromoCode(p.CodeDisplay) == norm {
			return true, true
		}
	}
	return false, true
}

// generatePromos — «Придумать коды» (RM5-12): пределы окна; noCodes — не хватило разных кодов
// («Не получилось придумать столько разных кодов. Увеличьте длину»); журнал evPromoGenerate (source).
// Коды не совпадают ни с одним кодом базы (действующим и использованным, сравнение нормализованных) и
// между собой; запись — одной транзакцией хранилища, сбой — ничего не создано. Неизвестный состав —
// ipc.TextBadRequest без поля (окно его не шлёт).
func (s *Service) generatePromos(count, length int, charset string, minutes int, src extraSource) (created []PromoRecord, field, failText string, noCodes bool) {
	if count < 1 || count > 50 {
		return nil, ipc.FieldCount, "Можно от 1 до 50 кодов", false
	}
	if length < 4 || length > 12 {
		return nil, ipc.FieldLength, "Длина от 4 до 12 знаков", false
	}
	if _, known := promoAlphabets[charset]; !known {
		return nil, "", ipc.TextBadRequest, false
	}
	if !core.PromoMinutesValid(minutes) {
		return nil, ipc.FieldMinutes, "Можно от 1 до 600 минут", false
	}
	list, err := s.cfg.Store.ListPromos()
	if err != nil {
		// Ещё ничего не сохранялось: «Не удалось выполнить», не «сохранить».
		return nil, "", textRetry, false
	}
	taken := make(map[string]bool, len(list))
	for _, p := range list {
		taken[core.NormalizePromoCode(p.CodeDisplay)] = true
	}
	codes, ok := genPromoCodes(promoRand, count, length, charset, taken)
	if !ok {
		return nil, "", "Не получилось придумать столько разных кодов. Увеличьте длину", true
	}
	created, err = s.cfg.Store.CreatePromos(codes, minutes)
	if err != nil {
		// База могла измениться извне (повтор UNIQUE) — пачка откатилась целиком.
		s.logRate("genpromos", "не удалось записать придуманные промокоды: %v", err)
		return nil, "", textRetry, false
	}
	text := promoGenerateText(len(created), minutes)
	if src == extraFromPhone {
		text += " с телефона"
	}
	s.journal.Write(0, evPromoGenerate, sourceDetails(src, map[string]any{
		"count": len(created), "minutes": minutes, "codes": codes, "text": text,
	}))
	return created, "", "", false
}

// deletePromo — удаление действующего кода (FR-45): только телефон (кнопки в окне нет, ТЗ не просит);
// notFound — кода нет среди действующих; журнал evPromoDelete «Промокод SUMMER на 30 мин удалён с телефона».
func (s *Service) deletePromo(code string, src extraSource) (p PromoRecord, notFound bool, failText string) {
	p, ok, err := s.cfg.Store.DeletePromo(code)
	if err != nil {
		s.logRate("deletepromo", "не удалось удалить промокод: %v", err)
		return PromoRecord{}, false, textSaveFailed
	}
	if !ok {
		return PromoRecord{}, true, ""
	}
	display := strings.ToUpper(p.CodeDisplay) // коды до 0.5.0 могли быть строчными (RM5-11)
	text := fmt.Sprintf("Промокод %s на %d мин удалён", display, p.Minutes)
	if src == extraFromPhone {
		text += " с телефона"
	}
	s.journal.Write(0, evPromoDelete, sourceDetails(src, map[string]any{"code": display, "minutes": p.Minutes, "text": text}))
	return p, false, ""
}

// promoLists — действующие и использованные для окна и телефона (порядок хранилища, RM5-11); имена детей — s.names.
// Пусто — [], не nil.
func (s *Service) promoLists() (active, used []ipc.PromoView, err error) {
	list, err := s.cfg.Store.ListPromos()
	if err != nil {
		return nil, nil, err
	}
	active, used = []ipc.PromoView{}, []ipc.PromoView{}
	for _, p := range list {
		if p.UsedAtMs == 0 {
			active = append(active, promoViewOf(p, ""))
		} else {
			used = append(used, promoViewOf(p, s.names[p.UsedByChild]))
		}
	}
	return active, used, nil
}
