package storage

import (
	"errors"
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
