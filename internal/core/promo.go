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

// CheckNewPromoCode — проверка нового кода (R26, D-75): после TrimSpace только ASCII A–Z, a–z, 0–9, длина 3–16.
// display — заглавными. Коды до 0.5.0 этой проверкой не меряются (ввод ребёнком — NormalizePromoCode).
// Знаки проверяются по байтам до перевода регистра: кириллица, полноширинные буквы и лигатуры
// (ToUpper превратил бы «ﬁ» в «FI») не проходят.
func CheckNewPromoCode(code string) (display string, ok bool) {
	c := strings.TrimSpace(code)
	if len(c) < 3 || len(c) > 16 || !latinDigits(c) {
		return "", false
	}
	return strings.ToUpper(c), true
}

// PromoCodeLatinOnly сообщает, что код после TrimSpace не пуст и состоит только из ASCII A–Z, a–z,
// 0–9, без проверки длины: служба по нему отличает «Код от 3 до 16 знаков» от «Только латинские
// буквы A-Z и цифры» (R26).
func PromoCodeLatinOnly(code string) bool {
	c := strings.TrimSpace(code)
	return c != "" && latinDigits(c)
}

// latinDigits — все байты c из ASCII A–Z, a–z, 0–9 (по байтам: кириллица и лигатуры не проходят).
func latinDigits(c string) bool {
	for i := 0; i < len(c); i++ {
		b := c[i]
		if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9') {
			return false
		}
	}
	return true
}
