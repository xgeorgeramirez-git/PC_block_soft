package guard

import (
	"testing"
	"time"

	"pcblock/internal/ipc"
)

// Тесты стража вехи M3: путь переднего плана, плашки о закрытых программах, кнопки плашки 1 мин,
// игровые предупреждения (R3, R5, R9, R12, FR-13, RM3-4, RM3-5).

const realGame = `C:\Users\Миша\Downloads\SuperGame.exe`

// m3Rig — контроллер на поддельной службе, у которого датчики видят на переднем плане realGame.
func m3Rig(test bool) *ctRig {
	r := newCtRig()
	r.svc.test = test
	read := func(now time.Time) ipc.SensorSample {
		return BuildSample(now, RawSensors{IdleMs: 1500, ForegroundPath: realGame}, 1)
	}
	r.ctl = NewController(NewModel(), r.svc, r.clk.Now, read, nil)
	return r
}

// lastSample — снимок датчиков из последнего GuardPoll.
func (r *ctRig) lastSample(t *testing.T) ipc.SensorSample {
	t.Helper()
	calls := r.svc.called(ipc.CmdGuardPoll)
	if len(calls) == 0 {
		t.Fatal("опросов не было")
	}
	var req ipc.GuardPollRequest
	if err := calls[len(calls)-1].Into(&req); err != nil {
		t.Fatal(err)
	}
	return req.Sample
}

// lastState — последний отправленный GuardState.
func (r *ctRig) lastState(t *testing.T) ipc.GuardStateReply {
	t.Helper()
	r.ctl.PushState()
	calls := r.svc.called(ipc.CmdGuardState)
	if len(calls) == 0 {
		t.Fatal("GuardState не отправлен")
	}
	var st ipc.GuardStateReply
	if err := calls[len(calls)-1].Into(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

func foregroundCmd(path string) ipc.GuardCommand {
	return ipc.GuardCommand{Kind: ipc.GuardCmdTest, Test: &ipc.TestGuardAction{Action: ipc.TestForeground, Text: path}}
}

// Ловит (RM3-8): подмена переднего плана для приёмки не доходит до снимка, держится один опрос,
// не возвращается к настоящему чтению по пустому тексту, уходит окну как действие click или
// работает вне тестового режима; GuardState не показывает, что ушло службе.
func TestForegroundOverrideInTestMode(t *testing.T) {
	const fake = `C:\Users\runneradmin\AppData\Local\Temp\pcb\game.exe`
	r := m3Rig(true)
	r.svc.status = ctRunningStatus(3600)
	r.poll()
	if got := r.lastSample(t).ForegroundPath; got != realGame {
		t.Fatalf("настоящий путь: %q", got)
	}
	r.svc.cmds = []ipc.GuardCommand{foregroundCmd(fake)}
	r.poll()
	if q := r.ctl.QueuedTests(); len(q) != 0 {
		t.Fatalf("подмена ушла окну: %+v", q)
	}
	for i := 0; i < 3; i++ {
		r.poll()
		if got := r.lastSample(t).ForegroundPath; got != fake {
			t.Fatalf("опрос %d после подмены: %q", i, got)
		}
	}
	if st := r.lastState(t); st.ForegroundPath != fake {
		t.Fatalf("GuardState.foregroundPath = %q", st.ForegroundPath)
	}
	r.svc.cmds = []ipc.GuardCommand{foregroundCmd("")}
	r.poll()
	r.poll()
	if got := r.lastSample(t).ForegroundPath; got != realGame {
		t.Fatalf("после пустой подмены: %q", got)
	}
	if st := r.lastState(t); st.ForegroundPath != realGame {
		t.Fatalf("GuardState.foregroundPath после пустой подмены = %q", st.ForegroundPath)
	}

	r = m3Rig(false)
	r.svc.status = ctRunningStatus(3600)
	r.svc.cmds = []ipc.GuardCommand{foregroundCmd(fake)}
	r.poll()
	r.poll()
	if got := r.lastSample(t).ForegroundPath; got != realGame {
		t.Fatalf("вне тестового режима подмена сработала: %q", got)
	}
}
