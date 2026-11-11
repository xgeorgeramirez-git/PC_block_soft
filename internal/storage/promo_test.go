package storage

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Ловит: промокод создаётся и попадает в список действующих; повтор нормализованного кода
// должен отклоняться (одноразовость теряется, если родитель заведёт два «одинаковых» кода).
func TestCreatePromoAndList(t *testing.T) {
	s := openTemp(t)
	p, err := s.CreatePromo("A1234", 30)
	if err != nil {
		t.Fatalf("CreatePromo: %v", err)
	}
	if p.CodeNorm != "a1234" || p.CodeDisplay != "A1234" || p.Minutes != 30 {
		t.Errorf("создан промокод %+v; want norm=a1234 display=A1234 minutes=30", p)
	}
	list, err := s.ListPromos()
	if err != nil {
		t.Fatalf("ListPromos: %v", err)
	}
	if len(list) != 1 || list[0].CodeDisplay != "A1234" || list[0].UsedAtMs != nil {
		t.Errorf("список = %+v; want один действующий код A1234", list)
	}
}

// Ловит: два кода, совпадающие после нормализации, создаются оба — тогда одноразовость
// обходится вводом «того же» кода в другом регистре.
func TestCreatePromoDuplicateRejected(t *testing.T) {
	s := openTemp(t)
	if _, err := s.CreatePromo("A1234", 30); err != nil {
		t.Fatalf("первый CreatePromo: %v", err)
	}
	_, err := s.CreatePromo(" a1234 ", 15)
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("повтор кода: err=%v; want ErrInvalid", err)
	}
}

// Ловит: минуты вне 1..600 или пустой код проходят, хотя схема и ТЗ их запрещают.
func TestCreatePromoInvalidInput(t *testing.T) {
	s := openTemp(t)
	for _, tc := range []struct {
		name    string
		code    string
		minutes int
	}{
		{"минут 0", "code0", 0},
		{"минут 601", "code601", 601},
		{"пустой код", "   ", 30},
	} {
		if _, err := s.CreatePromo(tc.code, tc.minutes); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err=%v; want ErrInvalid", tc.name, err)
		}
	}
}

// Ловит: промокод применяется повторно (одноразовость сломана) или без учёта регистра/пробелов.
func TestRedeemPromoOnceOnly(t *testing.T) {
	s := openTemp(t)
	child := mustChild(t, s, "S-1-5-21-1", "Миша")
	if _, err := s.CreatePromo("A1234", 30); err != nil {
		t.Fatalf("CreatePromo: %v", err)
	}
	// Ребёнок вводит код в другом регистре и с пробелами — должно сработать (R26).
	min, display, ok, err := s.RedeemPromo(child, " a1234 ", 5_000)
	if err != nil || !ok || min != 30 || display != "A1234" {
		t.Fatalf("первое применение: min=%d display=%q ok=%v err=%v; want 30 A1234 true nil", min, display, ok, err)
	}
	// Повторный ввод того же кода — отказ (одноразовость, R26).
	_, _, ok2, err := s.RedeemPromo(child, "A1234", 6_000)
	if err != nil || ok2 {
		t.Fatalf("повтор: ok=%v err=%v; want false nil", ok2, err)
	}
	// В списке код помечен использованным этим ребёнком.
	list, err := s.ListPromos()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].UsedAtMs == nil || *list[0].UsedAtMs != 5_000 ||
		list[0].UsedByChild == nil || *list[0].UsedByChild != child {
		t.Errorf("после применения список = %+v; want used_at=5000 used_by=%d", list, child)
	}
}

// Ловит: несуществующий код возвращает ошибку или ok=true вместо «не подходит».
func TestRedeemPromoUnknown(t *testing.T) {
	s := openTemp(t)
	child := mustChild(t, s, "S-1-5-21-1", "Миша")
	_, _, ok, err := s.RedeemPromo(child, "нет-такого", 1_000)
	if err != nil || ok {
		t.Errorf("неизвестный код: ok=%v err=%v; want false nil", ok, err)
	}
}

// promoCodes — отображаемые коды списка по порядку.
func promoCodes(t *testing.T, s *Store) []string {
	t.Helper()
	list, err := s.ListPromos()
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, p := range list {
		out = append(out, p.CodeDisplay)
	}
	return out
}

// Ловит (RM5-12): пачка «Придумать коды» создаётся не целиком; повтор существующего кода (в том
// числе использованного) или повтор внутри пачки оставляет в базе часть пачки; минуты вне 1..600.
func TestCreatePromosAllOrNothing(t *testing.T) {
	s := openTemp(t)
	fixClock(t, 7_000)
	child := mustChild(t, s, "S-1", "Миша")
	if _, err := s.CreatePromo("USED1", 10); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := s.RedeemPromo(child, "used1", 7_500); err != nil || !ok {
		t.Fatalf("RedeemPromo: %v %v", ok, err)
	}
	codes := make([]string, 20)
	for i := range codes {
		codes[i] = fmt.Sprintf("K%04d", i)
	}
	got, err := s.CreatePromos(codes, 15)
	if err != nil {
		t.Fatalf("CreatePromos: %v", err)
	}
	if len(got) != 20 {
		t.Fatalf("создано %d, want 20", len(got))
	}
	for i, p := range got {
		if p.ID == 0 || p.CodeDisplay != codes[i] || p.CodeNorm != strings.ToLower(codes[i]) || p.Minutes != 15 || p.CreatedAtMs != 7_000 || p.UsedAtMs != nil {
			t.Errorf("код %d = %+v; want %s на 15 мин, создан 7000", i, p, codes[i])
		}
	}
	if n := len(promoCodes(t, s)); n != 21 {
		t.Errorf("в базе %d кодов, want 21", n)
	}
	for _, tc := range []struct {
		name    string
		codes   []string
		minutes int
	}{
		{"повтор действующего", []string{"NEW1", "k0003"}, 15},
		{"повтор использованного", []string{"NEW1", "used1"}, 15},
		{"повтор внутри пачки", []string{"NEW1", "NEW2", "new1"}, 15},
		{"минут 0", []string{"NEW1"}, 0},
		{"минут 601", []string{"NEW1"}, 601},
		{"пустой код", []string{"NEW1", "  "}, 15},
	} {
		if _, err := s.CreatePromos(tc.codes, tc.minutes); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", tc.name, err)
		}
		if n := len(promoCodes(t, s)); n != 21 {
			t.Errorf("%s: в базе %d кодов, want 21 (ничего не создано)", tc.name, n)
		}
	}
}

// Ловит (RM5-14): «Очистить историю» удаляет действующие коды или оставляет использованные,
// неверно считает; удалённый код нельзя создать заново.
func TestClearUsedPromos(t *testing.T) {
	s := openTemp(t)
	child := mustChild(t, s, "S-1", "Миша")
	for _, c := range []string{"AAA1", "BBB2", "CCC3"} {
		if _, err := s.CreatePromo(c, 10); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []string{"aaa1", "ccc3"} {
		if _, _, ok, err := s.RedeemPromo(child, c, 5_000); err != nil || !ok {
			t.Fatalf("RedeemPromo(%s): %v %v", c, ok, err)
		}
	}
	n, err := s.ClearUsedPromos()
	if err != nil || n != 2 {
		t.Fatalf("ClearUsedPromos = %d, %v; want 2", n, err)
	}
	if got := promoCodes(t, s); !reflect.DeepEqual(got, []string{"BBB2"}) {
		t.Errorf("после очистки = %v, want только действующий BBB2", got)
	}
	if n, err := s.ClearUsedPromos(); err != nil || n != 0 {
		t.Errorf("повтор = %d, %v; want 0", n, err)
	}
	if _, err := s.CreatePromo("AAA1", 20); err != nil {
		t.Errorf("удалённый код не создаётся заново: %v", err)
	}
}

// Ловит (RM5-11): порядок списка — действующие новые сверху, затем использованные по моменту
// использования (а не по моменту создания).
func TestListPromosOrder(t *testing.T) {
	s := openTemp(t)
	child := mustChild(t, s, "S-1", "Миша")
	for _, c := range []struct {
		code string
		at   int64
	}{{"OLD", 100}, {"MID", 200}, {"NEW", 300}, {"SAME", 300}, {"LAST", 400}} {
		fixClock(t, c.at)
		if _, err := s.CreatePromo(c.code, 10); err != nil {
			t.Fatal(err)
		}
	}
	// NEW использован раньше, чем OLD; LAST — использован последним.
	for _, r := range []struct {
		code string
		at   int64
	}{{"new", 1_000}, {"old", 2_000}, {"last", 3_000}} {
		if _, _, ok, err := s.RedeemPromo(child, r.code, r.at); err != nil || !ok {
			t.Fatalf("RedeemPromo(%s): %v %v", r.code, ok, err)
		}
	}
	want := []string{"SAME", "MID", "LAST", "OLD", "NEW"}
	if got := promoCodes(t, s); !reflect.DeepEqual(got, want) {
		t.Errorf("порядок = %v, want %v", got, want)
	}
}
