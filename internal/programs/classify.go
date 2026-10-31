package programs

import (
	"strings"
	"sync"
)

// Verdict — что делать с программой (RM3-1). Нулевое значение — «система»: не трогать.
type Verdict int

const (
	VerdictSystem    Verdict = iota // не трогать: система, PC Block, непригодный путь
	VerdictAllow                    // разрешена, категория в Result.Category
	VerdictForbidden                // запрещена родителем (R13)
	VerdictUnknown                  // незнакомая: ждёт решения родителя (R12)
	VerdictHidden                   // скрыта родителем (D-57, RF9-2): не закрывать, секунда — «Прочее»
)

// String — вердикт для журнала работы.
func (v Verdict) String() string {
	switch v {
	case VerdictSystem:
		return "система"
	case VerdictAllow:
		return "разрешена"
	case VerdictForbidden:
		return "запрещена"
	case VerdictUnknown:
		return "незнакомая"
	case VerdictHidden:
		return "скрыта"
	}
	return "?"
}

// Entry — программа из списка (таблица programs) для классификатора.
type Entry struct {
	Path        string // путь exe (нормализуется при Reload)
	Name        string // название из файла
	CustomName  string // своё название родителя (D-59); пусто — название из файла
	Category    string // Cat*
	Fingerprint string // «размер:время изменения» (RM3-12); пусто — сверять не нужно
	Publisher   string // издатель (CompanyName) решения в профиле: по нему решение переходит на новые версии (RM3-13)
	Hidden      bool   // скрыта родителем (D-57): не закрывается и ничего не передаёт корню и профилю (RF9-2)
}

// Result — итог классификации.
type Result struct {
	Verdict     Verdict
	Category    string // категория для VerdictAllow и VerdictForbidden («blocked»), «system» по списку, «other» у VerdictHidden
	Known       bool   // путь есть в списке программ (точное совпадение, отпечаток не разошёлся)
	Name        string // название для людей (DisplayName: своё, из списка или из пути; не длиннее MaxNameLen)
	WasCategory string // VerdictUnknown после подмены файла: прежняя категория («раньше: Игра», RM3-12)
}

// Classifier — классификация пути exe по списку программ, корням приложений и каталогу (RM3-1).
// Безопасен для одновременного использования: Reload меняет список целиком под блокировкой,
// Classify видит либо старый, либо новый список.
type Classifier struct {
	roots Roots
	cat   Catalog

	mu      sync.RWMutex
	exact   map[string]Entry  // по нормализованному пути
	rootCat map[string]string // корень приложения → наследуемая категория
	// profile — решения в профилях (RM3-13): «корень без имени профиля|имя файла» → издатели и
	// категории. Корень без имени профиля: решение действует для всех детей (R12).
	profile map[string][]profileRule
}

// profileRule — решение родителя в профиле: exe с тем же именем файла и тем же издателем внутри
// того же корня получает категорию решения.
type profileRule struct {
	publisher string // нормализованный издатель (publisherKey)
	category  string
}

// publisherKey — издатель для сравнения: без пробелов по краям, без учёта регистра.
func publisherKey(p string) string { return strings.ToLower(strings.TrimSpace(p)) }

// profileRank — какая категория побеждает, если у одного имени и издателя несколько решений:
// запрет, затем игры, прочее, учёба (учёба даёт больше всего прав).
var profileRank = map[string]int{CatStudy: 1, CatOther: 2, CatGames: 3, CatBlocked: 4}

// bestProfileRule — категория решения в профиле для exe издателя publisher (пустой — нет).
func bestProfileRule(rules []profileRule, publisher string) string {
	best, pub := "", publisherKey(publisher)
	if pub == "" {
		return ""
	}
	for _, r := range rules {
		if r.publisher == pub && profileRank[r.category] > profileRank[best] {
			best = r.category
		}
	}
	return best
}

// profileKey — ключ решений в профиле для нормализованного пути: корень без имени профиля и имя
// файла; ok = false — путь не в корне приложения профиля.
func (c *Classifier) profileKey(p string) (string, bool) {
	rel := c.roots.profileRel(p)
	if rel == "" {
		return "", false
	}
	rel = rel[strings.Index(rel, `\`)+1:] // без имени профиля
	return rel + "|" + p[strings.LastIndex(p, `\`)+1:], true
}

// NewClassifier создаёт классификатор с пустым списком программ.
func NewClassifier(roots Roots, cat Catalog) *Classifier {
	return &Classifier{roots: roots, cat: cat, exact: map[string]Entry{}, rootCat: map[string]string{}, profile: map[string][]profileRule{}}
}

// Roots возвращает корни классификатора.
func (c *Classifier) Roots() Roots { return c.roots }

// Catalog возвращает каталог классификатора.
func (c *Classifier) Catalog() Catalog { return c.cat }

// inheritRank — сила категории при наследовании корнем приложения: запрет сильнее игр, игры
// сильнее прочего (вспомогательные exe и новые версии игры — тоже игра, новая версия
// запрещённой программы — тоже запрещена); учёба не наследуется и даёт «Прочее».
var inheritRank = map[string]int{CatOther: 1, CatStudy: 1, CatGames: 2, CatBlocked: 3}

// Reload заменяет список программ (после снимка и решений родителя). Пустой и непригодный путь
// пропускается. Корень приложения считается по корням классификатора, а не берётся из базы:
// наследование возможно только внутри Program Files. Скрытая программа (D-57) известна по
// точному пути, но не передаёт категорию корню приложения и профилю (RF9-2): иначе скрытие
// запускалки решало бы судьбу соседних exe, в том числе самой игры.
func (c *Classifier) Reload(entries []Entry) {
	exact := make(map[string]Entry, len(entries))
	rank := map[string]int{}
	profile := map[string][]profileRule{}
	for _, e := range entries {
		p := Normalize(e.Path)
		if p == "" {
			continue
		}
		e.Path = p
		exact[p] = e
		if e.Hidden {
			continue
		}
		if key, ok := c.profileKey(p); ok {
			// В профиле решение наследуется только по имени файла и издателю (RM3-13).
			if pub := publisherKey(e.Publisher); pub != "" && profileRank[e.Category] > 0 {
				profile[key] = append(profile[key], profileRule{publisher: pub, category: e.Category})
			}
			continue
		}
		root := c.roots.AppRoot(p)
		if r := inheritRank[e.Category]; root != "" && r > rank[root] {
			rank[root] = r
		}
	}
	rootCat := make(map[string]string, len(rank))
	for root, r := range rank {
		switch r {
		case 3:
			rootCat[root] = CatBlocked
		case 2:
			rootCat[root] = CatGames
		default:
			rootCat[root] = CatOther
		}
	}
	c.mu.Lock()
	c.exact, c.rootCat, c.profile = exact, rootCat, profile
	c.mu.Unlock()
}

// NeedsPublisher сообщает, что для пути нужно прочитать издателя exe и передать его в Classify:
// путь в корне приложения профиля, его нет в списке (или есть с отпечатком: обновление на
// месте), а в этом корне есть решение с тем же именем файла и издателем (RM3-13). Остальным путям издатель не нужен: опрос не замедляется.
func (c *Classifier) NeedsPublisher(path string) bool {
	p := Normalize(path)
	key, ok := c.profileKey(p)
	if !ok {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if e, known := c.exact[p]; known && e.Fingerprint == "" {
		return false // точный путь без отпечатка: решение действует и без издателя
	}
	return len(c.profile[key]) > 0
}

// NeedsFingerprint сообщает, что для пути в списке сохранён отпечаток файла: наблюдателю нужно
// прочитать отпечаток и передать его в Classify (RM3-12).
func (c *Classifier) NeedsFingerprint(path string) bool {
	p := Normalize(path)
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.exact[p].Fingerprint != ""
}

// Classify определяет вердикт для пути exe строго в порядке RM3-1:
//  1. пустой или непригодный путь — «система» (PL-9: «не знаю» = «не трогать»);
//  2. класс «система» (папка Windows, PC Block, WebView2, пакеты компонентов Windows);
//  3. точный путь в списке: категория программы (скрытая родителем — «скрыта», D-57); для записи с отпечатком при другом или
//     непрочитанном отпечатке (fingerprint "") — «незнакомая» с прежней категорией, кроме корня
//     профиля, где файл обновился на месте с тем же издателем (RM3-13) (RM3-12, Д-1:
//     иначе ребёнок закроет системе доступ к свойствам своего файла и подменит exe; подмена
//     важнее скрытия: скрытая тоже снова «ждёт решения»). Это закрытие
//     одной программы, а не блокировка компьютера (R18 не нарушается);
//  4. корень приложения в профиле пользователя (RM3-13): категория решения с тем же именем файла
//     и тем же издателем (publisher; пустой — не наследуется), иначе «незнакомая»;
//     корень приложения в Program Files: наследуемая категория корня;
//  5. каталог по имени файла (в WindowsApps и по имени пакета) — только в доверенных каталогах;
//  6. иначе «незнакомая».
func (c *Classifier) Classify(path, fingerprint, publisher string) Result {
	p := Normalize(path)
	if p == "" {
		return Result{Verdict: VerdictSystem}
	}
	name := NameFromPath(path)
	if c.roots.IsSystem(p) {
		return Result{Verdict: VerdictSystem, Name: name}
	}
	pkey, inProfile := c.profileKey(p)
	c.mu.RLock()
	e, known := c.exact[p]
	rootCat, inRoot := c.rootCat[c.roots.AppRoot(p)]
	rules := c.profile[pkey]
	c.mu.RUnlock()
	if known {
		name = DisplayName(e.CustomName, e.Name, path)
		if e.Fingerprint != "" && fingerprint != e.Fingerprint {
			// В корне профиля файл мог обновиться на месте: решение действует, если совпали имя
			// файла и непустой издатель (сам этот путь — тоже кандидат). Вне профиля — подмена.
			if best := bestProfileRule(rules, publisher); inProfile && best != "" {
				return verdictOf(best, false, name)
			}
			return Result{Verdict: VerdictUnknown, Name: name, WasCategory: e.Category}
		}
		if e.Hidden {
			return Result{Verdict: VerdictHidden, Category: CatOther, Known: true, Name: name}
		}
		return verdictOf(e.Category, true, name)
	}
	if inProfile {
		if best := bestProfileRule(rules, publisher); best != "" {
			return verdictOf(best, false, name)
		}
		return Result{Verdict: VerdictUnknown, Name: name}
	}
	if inRoot {
		return verdictOf(rootCat, false, name)
	}
	if cat, ok := c.CatalogCategory(p); ok {
		return Result{Verdict: VerdictAllow, Category: cat, Name: name}
	}
	return Result{Verdict: VerdictUnknown, Name: name}
}

// CatalogCategory — категория по встроенному каталогу (RM3-1 п.4): только для путей в доверенных
// каталогах; по имени файла, а в WindowsApps ещё и по имени пакета Store (Д-3). Снимок и
// классификатор узнают каталог одинаково.
func (c *Classifier) CatalogCategory(path string) (string, bool) {
	p := Normalize(path)
	if !c.roots.IsTrusted(p) {
		return "", false
	}
	if cat, ok := c.cat.Lookup(p); ok {
		return cat, true
	}
	if pkg, ok := c.roots.storePackage(p); ok {
		return c.cat.LookupFamily(pkg.name)
	}
	return "", false
}

// verdictOf превращает категорию в вердикт. Непонятная категория — «система»: не трогать.
func verdictOf(cat string, known bool, name string) Result {
	switch cat {
	case CatGames, CatStudy, CatOther:
		return Result{Verdict: VerdictAllow, Category: cat, Known: known, Name: name}
	case CatBlocked:
		return Result{Verdict: VerdictForbidden, Category: cat, Known: known, Name: name}
	case CatSystem:
		return Result{Verdict: VerdictSystem, Category: cat, Known: known, Name: name}
	}
	return Result{Verdict: VerdictSystem, Known: known, Name: name}
}
