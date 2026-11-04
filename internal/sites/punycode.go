package sites

import "strings"

// Кодировщик punycode по RFC 3492 (IDN → ASCII, RM4-4). Своя реализация: golang.org/x/net/idna
// из стандартной библиотеки недоступен (ядро и sites — только стандартная библиотека).
//
// Ограничение: полной таблицы UTS 46 (сопоставление знаков, NFC, запрет отдельных символов) нет.
// Метка только переводится в нижний регистр strings.ToLower и кодируется. Для обычных доменов
// (кириллица, латиница с диакритикой, иероглифы) результат совпадает с браузером; редкие знаки,
// которые UTS 46 заменяет (например «ß», полноширинные цифры), дадут другой punycode, и такой
// домен просто не совпадёт с записью списка.

const (
	punyBase        = 36
	punyTMin        = 1
	punyTMax        = 26
	punySkew        = 38
	punyDamp        = 700
	punyInitialBias = 72
	punyInitialN    = 128
	punyMaxInt      = 1<<31 - 1
)

// encodePunycode кодирует метку без смены регистра (RFC 3492, раздел 6.3). false — переполнение.
func encodePunycode(input []rune) (string, bool) {
	var out strings.Builder
	basic := 0
	for _, r := range input {
		if r < 0x80 {
			out.WriteRune(r)
			basic++
		}
	}
	h := basic
	if basic > 0 {
		out.WriteByte('-')
	}
	n, delta, bias := punyInitialN, 0, punyInitialBias
	for h < len(input) {
		m := punyMaxInt
		for _, r := range input {
			if int(r) >= n && int(r) < m {
				m = int(r)
			}
		}
		if (m - n) > (punyMaxInt-delta)/(h+1) {
			return "", false
		}
		delta += (m - n) * (h + 1)
		n = m
		for _, r := range input {
			if int(r) < n {
				delta++
				if delta > punyMaxInt-1 {
					return "", false
				}
			}
			if int(r) != n {
				continue
			}
			q := delta
			for k := punyBase; ; k += punyBase {
				t := k - bias
				if t < punyTMin {
					t = punyTMin
				} else if t > punyTMax {
					t = punyTMax
				}
				if q < t {
					break
				}
				out.WriteByte(punyDigit(t + (q-t)%(punyBase-t)))
				q = (q - t) / (punyBase - t)
			}
			out.WriteByte(punyDigit(q))
			bias = punyAdapt(delta, h+1, h == basic)
			delta = 0
			h++
		}
		delta++
		n++
	}
	return out.String(), true
}

func punyDigit(d int) byte {
	if d < 26 {
		return byte('a' + d)
	}
	return byte('0' + d - 26)
}

func punyAdapt(delta, numPoints int, first bool) int {
	if first {
		delta /= punyDamp
	} else {
		delta /= 2
	}
	delta += delta / numPoints
	k := 0
	for delta > ((punyBase-punyTMin)*punyTMax)/2 {
		delta /= punyBase - punyTMin
		k += punyBase
	}
	return k + (punyBase-punyTMin+1)*delta/(delta+punySkew)
}

// toASCII — метка домена в ASCII: нижний регистр, не-ASCII метка → «xn--» + punycode.
func toASCII(label string) (string, bool) {
	low := strings.ToLower(label)
	for i := 0; i < len(low); i++ {
		if low[i] >= 0x80 {
			enc, ok := encodePunycode([]rune(low))
			if !ok {
				return "", false
			}
			return "xn--" + enc, true
		}
	}
	return low, true
}
