package core

import "strings"

// Промокоды на время (R26, D-50). Ядро задаёт только правило сравнения кода; хранение
// (таблица promo_codes), одноразовость и запись в журнал — на storage/service. Применение
// минут к сегодняшнему дню идёт через то же доп. время (Engine.AddExtra): промокод
// расширяет все три лимита, как доп. время (R10), и в копилку не кладётся.

// NormalizePromoCode приводит код к виду для сравнения (R26): убирает пробелы по краям и
// переводит в нижний регистр. Хранится и ищется именно нормализованный код, поэтому
// «A1234», « a1234 » и «a1234» — один и тот же промокод.
func NormalizePromoCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

// PromoMinutesValid — допустимое число минут промокода (R26, FR-35): 1…600.
func PromoMinutesValid(minutes int) bool {
	return minutes >= 1 && minutes <= 600
}
