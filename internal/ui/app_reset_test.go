package ui

import (
	"testing"

	"pcblock/internal/ipc"
	"pcblock/internal/launch"
	"pcblock/internal/service"
)

// fakeCaller отвечает на любую команду одним и тем же ответом: службы в тесте нет.
type fakeCaller struct{ reply ipc.Reply }

func (f fakeCaller) Do(ipc.Msg) (ipc.Reply, error) { return f.reply, nil }

func newTestApp(mode launch.Mode, reply ipc.Reply) *App {
	c := fakeCaller{reply: reply}
	return &App{
		mode:     mode,
		client:   service.NewWindowClient(func() (ipc.Caller, error) { return c, nil }, ""),
		exitCode: defaultExitCode(mode),
	}
}

// Ловит (FR-26): после удавшегося сброса пароля окно закрывают крестиком, а установщик и ярлык
// должны увидеть код 0, а не 1 «отмена»; отказ службы и чужие команды код не меняют.
func TestResetPasswordExitCode(t *testing.T) {
	cases := []struct {
		name    string
		mode    launch.Mode
		command string
		reply   ipc.Reply
		want    int
	}{
		{"сброс удался", launch.ModeResetPassword, ipc.CmdResetPassword, ipc.Reply{OK: true}, 0},
		{"служба отказала", launch.ModeResetPassword, ipc.CmdResetPassword, ipc.Reply{Error: "Нужны права администратора"}, 1},
		{"другая команда окна", launch.ModeResetPassword, ipc.CmdHello, ipc.Reply{OK: true}, 1},
		{"сброс из окна удаления не считается", launch.ModeUninstallGate, ipc.CmdResetPassword, ipc.Reply{OK: true}, 1},
	}
	for _, tc := range cases {
		a := newTestApp(tc.mode, tc.reply)
		if got := a.code(); got != 1 {
			t.Fatalf("%s: до команды код выхода %d, ждали 1 (отмена)", tc.name, got)
		}
		if _, err := a.Call(tc.command, `{"newPassword":"secret1"}`); err != nil {
			t.Fatalf("%s: Call: %v", tc.name, err)
		}
		if got := a.code(); got != tc.want {
			t.Errorf("%s: код выхода %d, ждали %d", tc.name, got, tc.want)
		}
	}
}

// Ловит (FR-4): окно удаления отдаёт установщику 0, 2 или 1; число вне 0..255 превращается в отмену.
func TestFinishSetsExitCode(t *testing.T) {
	for _, tc := range []struct{ code, want int }{{0, 0}, {2, 2}, {1, 1}, {-1, 1}, {256, 1}} {
		a := newTestApp(launch.ModeUninstallGate, ipc.Reply{})
		a.Finish(tc.code)
		if got := a.code(); got != tc.want {
			t.Errorf("Finish(%d): код выхода %d, ждали %d", tc.code, got, tc.want)
		}
	}
}

// Ловит (RM3-4): окно родителя, открытое «Попросить сейчас» (--screen=programs), не знает своего
// экрана; другие окна или окно без ключа отдают экран.
func TestStartScreen(t *testing.T) {
	for _, args := range []struct {
		argv []string
		want string
	}{
		{[]string{"--parent", "--screen=programs"}, "programs"},
		{[]string{"--parent"}, ""},
		{[]string{"--mystats"}, ""},
		{nil, ""},
	} {
		mode, opts, err := launch.Parse(args.argv)
		if err != nil {
			t.Fatal(err)
		}
		if got := NewApp(mode, opts).StartScreen(); got != args.want {
			t.Errorf("%q: StartScreen() = %q, ждали %q", args.argv, got, args.want)
		}
	}
	a := newTestApp(launch.ModeMyStats, ipc.OK(ipc.Empty{}))
	a.opts = launch.Options{"screen": "programs"}
	if got := a.StartScreen(); got != "" {
		t.Errorf("«Моя статистика» с экраном: %q", got)
	}
}
