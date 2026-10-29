package programs

import "sync"

// Verdict — что делать с программой (RM3-1). Нулевое значение — «система»: не трогать.
type Verdict int

const (
	VerdictSystem    Verdict = iota // не трогать: система, PC Block, непригодный путь
	VerdictAllow                    // разрешена, категория в Result.Category
	VerdictForbidden                // запрещена родителем (R13)
	VerdictUnknown                  // незнакомая: ждёт решения родителя (R12)
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
	}
	return "?"
}

// Entry — программа из списка (таблица programs) для классификатора.
type Entry struct {
	Path        string // путь exe (нормализуется при Reload)
	Name        string // имя для людей
	Category    string // Cat*
	Fingerprint string // «размер:время изменения» (RM3-12); пусто — сверять не нужно
}

// Result — итог классификации.
type Result struct {
	Verdict     Verdict
	Category    string // категория для VerdictAllow и VerdictForbidden («blocked»), «system» по списку
	Known       bool   // путь есть в списке программ (точное совпадение, отпечаток не разошёлся)
	Name        string // имя из списка или из пути (не длиннее MaxNameLen)
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
}

// NewClassifier создаёт классификатор с пустым списком программ.
func NewClassifier(roots Roots, cat Catalog) *Classifier {
	return &Classifier{roots: roots, cat: cat, exact: map[string]Entry{}, rootCat: map[string]string{}}
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
// наследование возможно только внутри Program Files.
func (c *Classifier) Reload(entries []Entry) {
	exact := make(map[string]Entry, len(entries))
	rank := map[string]int{}
	for _, e := range entries {
		p := Normalize(e.Path)
		if p == "" {
			continue
		}
		e.Path = p
		exact[p] = e
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
	c.exact, c.rootCat = exact, rootCat
	c.mu.Unlock()
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
//  2. класс «система» (папка Windows, PC Block, WindowsApps, WebView2);
//  3. точный путь в списке: категория программы; для записи с отпечатком при другом отпечатке —
//     «незнакомая» с прежней категорией (RM3-12). fingerprint "" — отпечаток не прочитан: решение
//     остаётся (ошибка чтения не закрывает программу);
//  4. корень приложения известной программы: наследуемая категория корня;
//  5. каталог по имени файла — только в доверенных каталогах;
//  6. иначе «незнакомая».
func (c *Classifier) Classify(path, fingerprint string) Result {
	p := Normalize(path)
	if p == "" {
		return Result{Verdict: VerdictSystem}
	}
	name := NameFromPath(path)
	if c.roots.IsSystem(p) {
		return Result{Verdict: VerdictSystem, Name: name}
	}
	c.mu.RLock()
	e, known := c.exact[p]
	rootCat, inRoot := c.rootCat[c.roots.AppRoot(p)]
	c.mu.RUnlock()
	if known {
		if e.Name != "" {
			name = ClipName(e.Name)
		}
		if e.Fingerprint != "" && fingerprint != "" && fingerprint != e.Fingerprint {
			return Result{Verdict: VerdictUnknown, Name: name, WasCategory: e.Category}
		}
		return verdictOf(e.Category, true, name)
	}
	if inRoot {
		return verdictOf(rootCat, false, name)
	}
	if c.roots.IsTrusted(p) {
		if cat, ok := c.cat.Lookup(p); ok {
			return Result{Verdict: VerdictAllow, Category: cat, Name: name}
		}
	}
	return Result{Verdict: VerdictUnknown, Name: name}
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
