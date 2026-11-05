package service

import (
	"fmt"
	"reflect"
	"testing"

	"pcblock/internal/ipc"
)

// sameValues — два набора значений равны; nil и пустой срез не различаются.
func sameValues(a, b []BraveValue) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func sameKeys(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// Ловит (RM4-10, R15): нет одного из запретов ветки, домены не по порядку или с повторами и
// пустыми, номер URLBlocklist не с 1, больше 1000 доменов в чёрном списке Brave.
func TestBraveValues(t *testing.T) {
	base := []BraveValue{
		{Name: "TorDisabled", DWord: true, Num: 1},
		{Name: "BraveVPNDisabled", DWord: true, Num: 1},
		{Name: "DnsOverHttpsMode", Str: "off"},
		{Key: "ExtensionInstallBlocklist", Name: "1", Str: "*"},
	}
	got := BraveRules{Blocked: []string{"b.example", "a.example", "", "a.example"}}.Values()
	want := append(append([]BraveValue{}, base...),
		BraveValue{Key: "URLBlocklist", Name: "1", Str: "a.example"},
		BraveValue{Key: "URLBlocklist", Name: "2", Str: "b.example"})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Values\n got %+v\nwant %+v", got, want)
	}
	if got := (BraveRules{}).Values(); !reflect.DeepEqual(got, base) {
		t.Errorf("Values без запрещённых\n got %+v\nwant %+v", got, base)
	}
	many := make([]string, 0, ipc.MaxBlockedSites+1)
	for i := ipc.MaxBlockedSites; i >= 0; i-- {
		many = append(many, fmt.Sprintf("s%04d.example", i))
	}
	got = BraveRules{Blocked: many}.Values()
	if n := len(got) - len(base); n != 1000 {
		t.Fatalf("URLBlocklist: %d значений, want 1000", n)
	}
	if last := got[len(got)-1]; last != (BraveValue{Key: "URLBlocklist", Name: "1000", Str: "s0999.example"}) {
		t.Errorf("последнее значение %+v, want URLBlocklist\\1000 = s0999.example", last)
	}
}

// Ловит (RM4-10, T5-3, PL-21): план стирает и переписывает всю ветку, не замечает пропавшего
// запрета, значения другого типа, лишнего номера списка, чужого значения или подключа;
// сравнивает имена с учётом регистра (в реестре регистр не важен).
func TestBravePlan(t *testing.T) {
	want := BraveRules{Blocked: []string{"a.example"}}.Values()
	without := func(name string) []BraveValue {
		out := []BraveValue{}
		for _, v := range want {
			if v.Name != name {
				out = append(out, v)
			}
		}
		return out
	}
	with := func(extra ...BraveValue) []BraveValue {
		return append(append([]BraveValue{}, want...), extra...)
	}
	replaced := func(i int, v BraveValue) []BraveValue {
		out := append([]BraveValue{}, want...)
		out[i] = v
		return out
	}
	tor := BraveValue{Name: "TorDisabled", DWord: true, Num: 1}
	cases := []struct {
		name    string
		have    []BraveValue
		set     []BraveValue
		del     []BraveValue
		delKeys []string
	}{
		{"ветки нет", nil, want, nil, nil},
		{"совпадает", with(), nil, nil, nil},
		{"нет TorDisabled", without("TorDisabled"), []BraveValue{tor}, nil, nil},
		{"TorDisabled = 0", replaced(0, BraveValue{Name: "TorDisabled", DWord: true, Num: 0}), []BraveValue{tor}, nil, nil},
		{"TorDisabled строкой", replaced(0, BraveValue{Name: "TorDisabled", Str: "1"}), []BraveValue{tor}, nil, nil},
		{"TorDisabled другого типа", replaced(0, BraveValue{Name: "TorDisabled"}), []BraveValue{tor}, nil, nil},
		{"лишний номер списка", with(BraveValue{Key: "URLBlocklist", Name: "2", Str: "old.example"}),
			nil, []BraveValue{{Key: "URLBlocklist", Name: "2", Str: "old.example"}}, nil},
		{"чужое значение ветки", with(BraveValue{Name: "BraveRewardsDisabled", DWord: true, Num: 1}),
			nil, []BraveValue{{Name: "BraveRewardsDisabled", DWord: true, Num: 1}}, nil},
		{"чужой подключ", with(BraveValue{Key: "Proxy", Name: "Mode", Str: "direct"}, BraveValue{Key: "Proxy", Name: "x", DWord: true, Num: 1}),
			nil, nil, []string{"Proxy"}},
		{"подключ глубже нужного", with(BraveValue{Key: `URLBlocklist\Deep`}, BraveValue{Key: `Proxy\Deep`}),
			nil, nil, []string{"Proxy", `URLBlocklist\Deep`}},
		{"другой домен под номером", replaced(4, BraveValue{Key: "URLBlocklist", Name: "1", Str: "b.example"}),
			[]BraveValue{{Key: "URLBlocklist", Name: "1", Str: "a.example"}}, nil, nil},
		{"имена в другом регистре", replaced(0, BraveValue{Name: "torDisabled", DWord: true, Num: 1}), nil, nil, nil},
		{"подключ в другом регистре", replaced(4, BraveValue{Key: "urlblocklist", Name: "1", Str: "a.example"}), nil, nil, nil},
	}
	for _, c := range cases {
		set, del, delKeys := BravePlan(c.have, want)
		if !sameValues(set, c.set) || !sameValues(del, c.del) || !sameKeys(delKeys, c.delKeys) {
			t.Errorf("%s:\n set %+v\n del %+v\n delKeys %q\nwant set %+v\n del %+v\n delKeys %q", c.name, set, del, delKeys, c.set, c.del, c.delKeys)
		}
	}
}
