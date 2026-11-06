package storage

import (
	"reflect"
	"testing"
)

const (
	mergeWA   = `c:\program files\windowsapps`
	merge130  = mergeWA + `\microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe\msedge_proxy.exe`
	merge131  = mergeWA + `\microsoft.microsoftedge.stable_131.0.2903.51_neutral__8wekyb3d8bbwe\msedge_proxy.exe`
	mergeKey  = mergeWA + `\microsoft.microsoftedge.stable__8wekyb3d8bbwe\msedge_proxy.exe`
	mergeTool = mergeWA + `\contoso.tool_1.0.0.0_x64__abcdef\tool.exe`
	mergeTKey = mergeWA + `\contoso.tool__abcdef\tool.exe`
)

// mergeBase — база «0.3.3»: две строки msedge_proxy, учёт по обеим версиям в один день, попытки
// по версии 131 и незнакомое приложение двух версий только в «Ждут решения».
func mergeBase(t *testing.T) (*Store, int64, int64) {
	t.Helper()
	s := openTemp(t)
	misha, err := s.UpsertChild(Child{SID: "S-1-5-21-1", Name: "Миша"})
	if err != nil {
		t.Fatal(err)
	}
	dasha, err := s.UpsertChild(Child{SID: "S-1-5-21-2", Name: "Даша"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []Program{
		{Path: merge130, Name: "msedge_proxy", Category: ProgramOther, Source: SourceSnapshot},
		{Path: merge131, Name: "Microsoft Edge", Publisher: "Microsoft Corporation", Category: ProgramOther, Source: SourceParent, DecidedAt: 100, Hidden: true},
		{Path: pathBrave, Name: "Brave", Category: ProgramOther, Source: SourceCatalog},
	} {
		if err := s.UpsertProgram(p); err != nil {
			t.Fatal(err)
		}
	}
	for _, u := range []struct {
		day, path string
		sec       int
	}{{"2026-10-12", merge130, 600}, {"2026-10-12", merge131, 300}, {"2026-10-13", merge131, 60}, {"2026-10-12", pathBrave, 7}} {
		if err := s.AddUsage(misha, u.day, UsageOther, SubjectProgram, u.path, u.sec); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []PendingProgram{
		{Path: merge131, Name: "msedge_proxy", Attempts: 2, FirstChildID: misha, FirstAttemptAt: 10, LastAttemptAt: 20,
			Children: []PendingChild{{ChildID: misha, Attempts: 2, LastAttemptAt: 20}}},
		{Path: mergeTool, Name: "tool", Attempts: 1, FirstChildID: dasha, FirstAttemptAt: 5, LastAttemptAt: 5,
			Children: []PendingChild{{ChildID: dasha, Attempts: 1, LastAttemptAt: 5}}},
	} {
		if err := s.RestorePending(p); err != nil {
			t.Fatal(err)
		}
	}
	return s, misha, dasha
}

// mergePlan — план, как его строит служба для mergeBase.
func mergePlan(misha, dasha int64) []ProgramMerge {
	return []ProgramMerge{
		{Key: mergeKey, From: []string{merge130, merge131}, PendingFrom: []string{merge131},
			Into: &Program{Path: mergeKey, Name: "Microsoft Edge", Publisher: "Microsoft Corporation", Category: ProgramOther, Source: SourceParent,
				AppRoot: mergeWA + `\microsoft.microsoftedge.stable__8wekyb3d8bbwe`, DecidedAt: 100, Hidden: true}},
		{Key: mergeTKey, From: []string{}, PendingFrom: []string{mergeTool},
			Pending: &PendingProgram{Path: mergeTKey, Name: "tool", Attempts: 1, FirstChildID: dasha, FirstAttemptAt: 5, LastAttemptAt: 5,
				Children: []PendingChild{{ChildID: dasha, Attempts: 1, LastAttemptAt: 5}}}},
	}
}

// Ловит (RM4-11, T6-11): слияние оставляет строки версий; учёт по версиям не суммируется на
// ключ (затирается последним) или задевает чужие программы; «Ждут решения» по версии остаются;
// попытки без строки программы теряются; сбой посреди набора оставляет базу частично слитой
// (без транзакции); повтор плана на слитой базе что-то меняет.
func TestMergePrograms(t *testing.T) {
	s, misha, dasha := mergeBase(t)
	if err := s.MergePrograms(mergePlan(misha, dasha)); err != nil {
		t.Fatalf("MergePrograms: %v", err)
	}
	list, err := s.ListPrograms()
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{}
	for _, p := range list {
		paths = append(paths, p.Path)
	}
	if !reflect.DeepEqual(paths, []string{pathBrave, mergeKey}) {
		t.Fatalf("программы после слияния: %v", paths)
	}
	if p, _, _ := s.GetProgram(mergeKey); !p.Hidden || p.Source != SourceParent || p.DecidedAt != 100 || p.Publisher != "Microsoft Corporation" {
		t.Errorf("строка ключа: %+v", p)
	}
	day, err := s.DayUsage(misha, "2026-10-12")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{mergeKey: 900, pathBrave: 7}
	got := map[string]int{}
	for _, u := range day {
		got[u.Subject] = u.Seconds
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("учёт 2026-10-12: %v, want %v", got, want)
	}
	if day, _ := s.DayUsage(misha, "2026-10-13"); len(day) != 1 || day[0].Subject != mergeKey || day[0].Seconds != 60 {
		t.Errorf("учёт 2026-10-13: %+v, want ключ 60", day)
	}
	pend, err := s.ListPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pend) != 1 || pend[0].Path != mergeTKey || pend[0].Attempts != 1 || len(pend[0].Children) != 1 || pend[0].Children[0].ChildID != dasha {
		t.Errorf("«Ждут решения» после слияния: %+v, want одна строка ключа tool", pend)
	}

	// Повтор того же плана на слитой базе — без изменений.
	before, _ := s.ListPrograms()
	if err := s.MergePrograms(mergePlan(misha, dasha)); err != nil {
		t.Fatalf("повтор MergePrograms: %v", err)
	}
	after, _ := s.ListPrograms()
	if !reflect.DeepEqual(before, after) {
		t.Errorf("повтор изменил программы:\n%v\n%v", before, after)
	}
	if day, _ := s.DayUsage(misha, "2026-10-12"); len(day) != 2 {
		t.Errorf("повтор изменил учёт: %+v", day)
	}
	if pend, _ := s.ListPending(); len(pend) != 1 || pend[0].Attempts != 1 {
		t.Errorf("повтор изменил «Ждут решения»: %+v", pend)
	}
}

// Ловит (T6-11): ошибка во втором слиянии набора оставляет первое записанным (запись не одной
// транзакцией): проверка каждого слияния идёт уже внутри транзакции, после записи первого.
func TestMergeProgramsAtomic(t *testing.T) {
	s, misha, dasha := mergeBase(t)
	before, _ := s.ListPrograms()
	usage, _ := s.DayUsage(misha, "2026-10-12")
	plan := mergePlan(misha, dasha)
	bad := *plan[0].Into
	bad.Path, bad.Category = mergeTKey, "x"
	plan[1].Into, plan[1].Pending = &bad, nil
	if err := s.MergePrograms(plan); err == nil {
		t.Fatal("MergePrograms с недопустимой категорией: нет ошибки")
	}
	after, _ := s.ListPrograms()
	if !reflect.DeepEqual(before, after) {
		t.Errorf("после ошибки программы изменились:\n%v\n%v", before, after)
	}
	if got, _ := s.DayUsage(misha, "2026-10-12"); !reflect.DeepEqual(got, usage) {
		t.Errorf("после ошибки учёт изменился: %+v, want %+v", got, usage)
	}
	if pend, _ := s.ListPending(); len(pend) != 2 {
		t.Errorf("после ошибки «Ждут решения»: %+v, want 2 прежние строки", pend)
	}
}
