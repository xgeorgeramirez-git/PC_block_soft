package guard

import (
	"testing"

	"pcblock/internal/ipc"
)

func TestTargetNamesAreUniqueAndRoundTrip(t *testing.T) {
	seen := map[string]Button{}
	for b := ButtonParentPassword; b <= ButtonWarningOK; b++ {
		name := TargetName(b)
		if name == "" {
			t.Errorf("у кнопки %d нет имени для канала", b)
			continue
		}
		if other, dup := seen[name]; dup {
			t.Errorf("имя %q у кнопок %d и %d", name, other, b)
		}
		seen[name] = b
		back, ok := ButtonByTarget(name)
		if !ok || back != b {
			t.Errorf("ButtonByTarget(%q) = %d, %v; ждали %d", name, back, ok, b)
		}
	}
}

func TestTargetNamesMatchChannel(t *testing.T) {
	// Три имени заданы каналом связи, их менять нельзя: ими пользуется приёмка.
	if TargetName(ButtonParentPassword) != ipc.ButtonParentPassword ||
		TargetName(ButtonLogoff) != ipc.ButtonLogoff ||
		TargetName(ButtonWarningOK) != ipc.ButtonAck {
		t.Fatal("имена кнопок разошлись с ipc.Button*")
	}
}

func TestUnknownTargets(t *testing.T) {
	if b, ok := ButtonByTarget(""); ok || b != ButtonNone {
		t.Error("пустое имя принято")
	}
	if b, ok := ButtonByTarget("нет такой"); ok || b != ButtonNone {
		t.Error("незнакомое имя принято")
	}
	if TargetName(ButtonNone) != "" || TargetName(Button(999)) != "" {
		t.Error("у несуществующей кнопки есть имя")
	}
}
