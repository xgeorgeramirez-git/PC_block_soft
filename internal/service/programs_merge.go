package service

import (
	"sort"
	"strconv"
	"strings"

	"pcblock/internal/programs"
)

// settingStoreMerged — повторы приложений магазина из 0.3.3 уже слиты на ключ (RM4-11, T6-11):
// «1» — слиты; нет или «0» — слить при старте службы.
const settingStoreMerged = "programs.storemerged"

// planStoreMerge — план слияния повторов приложений магазина (RM4-11, PL-21): группа — строки
// программ и «Ждут решения» с одним ключом roots.StableKey, если хоть один путь группы ≠ ключа.
// Строка ключа собирается по правилу на каждое поле:
//   - категория и источник — у самого позднего решения родителя (DecidedAt среди source = parent);
//     решений нет — у новейшей версии пакета (версии сравниваются по числам);
//   - скрыта, если скрыта хоть одна (запрет сильнее скрытия — в классификаторе, PL-30);
//   - своё название — непустое у самого позднего решения (ничьё — у новейшей версии);
//   - название и издатель — новейшей версии с непустым значением (название, равное имени файла,
//     слабее настоящего); DecidedAt — наибольший; отпечаток — пустой; корень — roots.AppRoot.
//
// «Ждут решения» группы при программе в группе удаляются; без программы — одна строка ключа:
// попытки по детям складываются, первая попытка — самая ранняя, последняя — самая поздняя,
// название и издатель — у последней попытки. Повтор на слитом даёт [].
func planStoreMerge(roots programs.Roots, list []Program, pend []PendingProgram) []ProgramMerge {
	type group struct {
		progs   []Program
		pending []PendingProgram
		moved   bool // хоть один путь ≠ ключа
	}
	groups := map[string]*group{}
	get := func(path string) (*group, string) {
		key := roots.StableKey(path)
		g := groups[key]
		if g == nil {
			g = &group{}
			groups[key] = g
		}
		if path != key {
			g.moved = true
		}
		return g, key
	}
	for _, p := range list {
		if g, key := get(p.Path); key != "" {
			g.progs = append(g.progs, p)
		}
	}
	for _, p := range pend {
		if g, key := get(p.Path); key != "" {
			g.pending = append(g.pending, p)
		}
	}
	keys := make([]string, 0, len(groups))
	for k, g := range groups {
		if k != "" && g.moved {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := []ProgramMerge{}
	for _, key := range keys {
		g := groups[key]
		m := ProgramMerge{Key: key, From: []string{}, PendingFrom: []string{}}
		if len(g.progs) > 0 {
			into := mergeProgramRows(roots, key, g.progs)
			m.Into = &into
			for _, p := range g.progs {
				m.From = append(m.From, p.Path)
			}
			sort.Strings(m.From)
		}
		for _, p := range g.pending {
			m.PendingFrom = append(m.PendingFrom, p.Path)
		}
		sort.Strings(m.PendingFrom)
		if m.Into == nil && len(g.pending) > 0 {
			p := mergePendingRows(key, g.pending)
			m.Pending = &p
		}
		out = append(out, m)
	}
	return out
}

// mergeProgramRows собирает строку ключа key из строк группы (правила — planStoreMerge).
func mergeProgramRows(roots programs.Roots, key string, rows []Program) Program {
	byVer := append([]Program{}, rows...)
	sort.SliceStable(byVer, func(i, j int) bool { return storeVersionLess(byVer[j].Path, byVer[i].Path) }) // новейшая первой
	byDecided := append([]Program{}, byVer...)
	sort.SliceStable(byDecided, func(i, j int) bool { return byDecided[i].DecidedAt > byDecided[j].DecidedAt })

	into := Program{Path: key, AppRoot: roots.AppRoot(key), Category: byVer[0].Category, Source: byVer[0].Source}
	for _, p := range byDecided {
		if p.Source == ProgramSourceParent && p.DecidedAt > 0 {
			into.Category, into.Source = p.Category, p.Source
			break
		}
	}
	for _, p := range byDecided {
		if p.CustomName != "" {
			into.CustomName = p.CustomName
			break
		}
	}
	for _, p := range byVer {
		into.Hidden = into.Hidden || p.Hidden
		into.DecidedAt = max(into.DecidedAt, p.DecidedAt)
		if into.Publisher == "" {
			into.Publisher = p.Publisher
		}
	}
	for _, p := range byVer { // настоящее название сильнее имени файла
		if p.Name != "" && !strings.EqualFold(p.Name, programs.NameFromPath(p.Path)) {
			into.Name = p.Name
			break
		}
	}
	if into.Name == "" {
		for _, p := range byVer {
			if p.Name != "" {
				into.Name = p.Name
				break
			}
		}
	}
	if into.Name == "" {
		into.Name = programs.NameFromPath(key)
	}
	return into
}

// mergePendingRows собирает строку «Ждут решения» ключа key из строк группы.
func mergePendingRows(key string, rows []PendingProgram) PendingProgram {
	last := rows[0]
	for _, p := range rows[1:] {
		if p.LastAttemptAt > last.LastAttemptAt {
			last = p
		}
	}
	out := PendingProgram{Path: key, Name: last.Name, Publisher: last.Publisher, Description: last.Description,
		WasCategory: last.WasCategory, FirstChildID: rows[0].FirstChildID, FirstAttemptAt: rows[0].FirstAttemptAt}
	kids := map[int64]*PendingChild{}
	for _, p := range rows {
		out.Attempts += p.Attempts
		out.LastAttemptAt = max(out.LastAttemptAt, p.LastAttemptAt)
		if p.FirstAttemptAt < out.FirstAttemptAt {
			out.FirstAttemptAt, out.FirstChildID = p.FirstAttemptAt, p.FirstChildID
		}
		for _, c := range p.Children {
			k := kids[c.ChildID]
			if k == nil {
				k = &PendingChild{ChildID: c.ChildID}
				kids[c.ChildID] = k
			}
			k.Attempts += c.Attempts
			k.LastAttemptAt = max(k.LastAttemptAt, c.LastAttemptAt)
		}
	}
	out.Children = make([]PendingChild, 0, len(kids))
	for _, k := range kids {
		out.Children = append(out.Children, *k)
	}
	sort.Slice(out.Children, func(i, j int) bool {
		if out.Children[i].LastAttemptAt != out.Children[j].LastAttemptAt {
			return out.Children[i].LastAttemptAt > out.Children[j].LastAttemptAt
		}
		return out.Children[i].ChildID < out.Children[j].ChildID
	})
	return out
}

// storeVersionLess — версия пакета в пути a старше, чем в b: числа «a.b.c.d» папки
// «<имя>_<версия>_…» слева направо; путь без версии (ключ, не магазин) старше любой версии, при
// равенстве — меньший путь.
func storeVersionLess(a, b string) bool {
	va, vb := storeVersionOf(a), storeVersionOf(b)
	for i := 0; i < len(va) && i < len(vb); i++ {
		if va[i] != vb[i] {
			return va[i] < vb[i]
		}
	}
	if len(va) != len(vb) {
		return len(va) < len(vb)
	}
	return a < b
}

// storeVersionOf — числа версии из первой папки пути вида «…\<имя>_<версия>_<арх>_<ресурс>_<издатель>\…»;
// nil — нет такой папки.
func storeVersionOf(path string) []int {
	for _, seg := range strings.Split(path, `\`) {
		f := strings.Split(seg, "_")
		if len(f) != 5 || f[0] == "" || f[4] == "" {
			continue
		}
		var out []int
		for _, x := range strings.Split(f[1], ".") {
			n, err := strconv.Atoi(x)
			if err != nil {
				return nil
			}
			out = append(out, n)
		}
		return out
	}
	return nil
}
