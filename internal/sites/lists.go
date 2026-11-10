package sites

// Kind — свой список родителя, в котором лежит домен (R14, RM4-5).
type Kind int

const (
	KindBlocked Kind = iota + 1 // "blocked" — «Запрещённые»
	KindStudy                   // "study" — «Учёба»
	KindVideo                   // "video_social" — «Видео и соцсети»
)

// String — ключ списка как в site_rules.kind (CHECK в 001_init.sql); неизвестный — «».
func (k Kind) String() string {
	switch k {
	case KindBlocked:
		return "blocked"
	case KindStudy:
		return "study"
	case KindVideo:
		return "video_social"
	}
	return ""
}

// KindFromString — список по ключу site_rules.kind; неизвестный ключ — (0, false).
func KindFromString(s string) (Kind, bool) {
	switch s {
	case "blocked":
		return KindBlocked, true
	case "study":
		return KindStudy, true
	case "video_social":
		return KindVideo, true
	}
	return 0, false
}

// Rule — запись своего списка: нормализованный домен и список.
type Rule struct {
	Domain string
	Kind   Kind
}

// Lists — свои списки родителя. Один домен — один список (RM4-5, UNIQUE(domain) в хранилище).
// Нулевое значение — пустые списки.
type Lists struct {
	byDomain map[string]Kind
}

// NewLists — списки из записей. Домены уже нормализованы (NormalizeInput); пустые пропускаются.
// При повторе домена побеждает последняя запись.
func NewLists(rules []Rule) Lists {
	m := make(map[string]Kind, len(rules))
	for _, r := range rules {
		if r.Domain != "" {
			m[r.Domain] = r.Kind
		}
	}
	return Lists{byDomain: m}
}

// Match — самая длинная совпавшая запись (RM4-5): сам домен, затем родители от длинного к
// короткому. Совпадение — сам домен и все поддомены (R14). IPv4 сравнивается только точно.
func (l Lists) Match(domain string) (Rule, bool) {
	if len(l.byDomain) == 0 || domain == "" {
		return Rule{}, false
	}
	for d := domain; ; {
		if k, ok := l.byDomain[d]; ok {
			return Rule{Domain: d, Kind: k}, true
		}
		var more bool
		if d, more = parentDomain(d, domain); !more {
			return Rule{}, false
		}
	}
}

// parentDomain — родитель d (без левой метки), пока в остатке есть точка: голый домен верхнего
// уровня («com») не проверяется. У IPv4 (исходный домен из цифр и точек) родителей нет.
func parentDomain(d, orig string) (string, bool) {
	if isNumeric(orig) {
		return "", false
	}
	for i := 0; i < len(d); i++ {
		if d[i] == '.' {
			rest := d[i+1:]
			for j := 0; j < len(rest); j++ {
				if rest[j] == '.' {
					return rest, true
				}
			}
			return "", false
		}
	}
	return "", false
}

// isNumeric — домен только из цифр и точек (IPv4).
func isNumeric(d string) bool {
	for i := 0; i < len(d); i++ {
		if (d[i] < '0' || d[i] > '9') && d[i] != '.' {
			return false
		}
	}
	return true
}
