package guard

import (
	"errors"
	"testing"
)

// Ловит (ревью M4 F1-3, Review Focus 2): Ctrl+W уходит окну, которое уже не на переднем плане
// (ребёнок переключился между решением и нажатием), или при зажатом модификаторе.
func TestSendCtrlWOnlyToSameForeground(t *testing.T) {
	cases := []struct {
		name   string
		window uintptr
		fg     uintptr
		held   bool
		want   error
		sent   int
	}{
		{"то же окно", 7, 7, false, nil, 1},
		{"передний план сменился", 7, 9, false, errForegroundChanged, 0},
		{"переднего плана нет", 7, 0, false, errForegroundChanged, 0},
		{"окно не известно", 0, 0, false, errForegroundChanged, 0},
		{"зажат модификатор", 7, 7, true, errModifierHeld, 0},
	}
	for _, k := range cases {
		sent := 0
		port := ctrlWPort{
			foreground: func() uintptr { return k.fg },
			held:       func() bool { return k.held },
			send:       func() error { sent++; return nil },
		}
		if err := sendCtrlW(port, k.window); !errors.Is(err, k.want) || (k.want == nil && err != nil) {
			t.Errorf("%s: ошибка %v, want %v", k.name, err, k.want)
		}
		if sent != k.sent {
			t.Errorf("%s: нажатий отправлено %d, want %d", k.name, sent, k.sent)
		}
	}
}
