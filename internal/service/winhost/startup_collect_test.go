package winhost

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"pcblock/internal/startup"
)

const testChildSID = "S-1-5-21-1-2-3-1001"

// logonTaskXML — задание «при входе» пользователя user (пусто — любого) с запуском от
// интерактивного входящего.
func logonTaskXML(user, cmd string, enabled bool) []byte {
	u := ""
	if user != "" {
		u = "<UserId>" + user + "</UserId>"
	}
	en := "true"
	if !enabled {
		en = "false"
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers><LogonTrigger><Enabled>true</Enabled>` + u + `</LogonTrigger></Triggers>
  <Principals><Principal id="Author"><LogonType>InteractiveToken</LogonType></Principal></Principals>
  <Settings><Enabled>` + en + `</Enabled></Settings>
  <Actions Context="Author"><Exec><Command>` + cmd + `</Command><Arguments>--x</Arguments></Exec></Actions>
</Task>`)
}

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[strings.ToUpper(k)] }
}

// Ловит (RM5b-1, уточнение 7, RM5b-7): путь задания не по месту файла (вложенная папка), свои и
// общие перепутаны, задание чужого пользователя показано, папка Microsoft читается, exe своего
// задания раскрыт переменными службы, а не ребёнка, выключенное задание показано включённым.
func TestCollectTasks(t *testing.T) {
	fsys := fstest.MapFS{
		"Own":                 {Data: logonTaskXML(testChildSID, `%LOCALAPPDATA%\App\own.exe`, true)},
		`Vendor/Shared Task`:  {Data: logonTaskXML("", `%LOCALAPPDATA%\App\shared.exe`, false)},
		"Other":               {Data: logonTaskXML("S-1-5-21-9-9-9-1002", `C:\o.exe`, true)},
		"Microsoft/Windows/X": {Data: []byte("не XML: папка Microsoft не читается")},
		"Empty":               {Data: []byte{}},
	}
	own := envOf(map[string]string{"LOCALAPPDATA": `C:\Users\Миша\AppData\Local`})
	sys := envOf(map[string]string{"LOCALAPPDATA": `C:\Windows\system32\config\systemprofile\AppData\Local`})
	got, err := collectTasks(fsys, testChildSID, `PC\Миша`, own, sys)
	if err != nil {
		t.Fatalf("collectTasks: %v", err)
	}
	want := []startup.Entry{
		{Scope: startup.ScopeOwn, Source: startup.SrcTask, Name: `\Own`, Task: `\Own`,
			Command: `%LOCALAPPDATA%\App\own.exe --x`, Exe: `C:\Users\Миша\AppData\Local\App\own.exe`, Enabled: true},
		{Scope: startup.ScopeShared, Source: startup.SrcTask, Name: `\Vendor\Shared Task`, Task: `\Vendor\Shared Task`,
			Command: `%LOCALAPPDATA%\App\shared.exe --x`, Exe: `C:\Windows\system32\config\systemprofile\AppData\Local\App\shared.exe`},
	}
	if len(got) != len(want) {
		t.Fatalf("collectTasks = %+v\nwant %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("запись %d = %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

// Ловит (PL-46, возврат 1 по T3): неразобранный файл задания пропускается молча — строка задания
// удалилась бы, а на следующей проверке оно стало бы «новым» и выключилось вопреки решению.
func TestCollectTasksBrokenFileFailsList(t *testing.T) {
	fsys := fstest.MapFS{
		"Good":   {Data: logonTaskXML(testChildSID, `C:\a.exe`, true)},
		"Broken": {Data: []byte("<Task><Triggers>обрыв")},
	}
	got, err := collectTasks(fsys, testChildSID, "", envOf(nil), envOf(nil))
	if err == nil || !strings.Contains(err.Error(), "Broken") {
		t.Fatalf("collectTasks = %+v, %v; want ошибку про Broken", got, err)
	}
}

// Ловит: нет папки заданий — ошибка вместо «заданий нет».
func TestCollectTasksNoRoot(t *testing.T) {
	for name, fsys := range map[string]fs.FS{
		"пустое дерево": fstest.MapFS{},
		"нет папки":     os.DirFS(filepath.Join(t.TempDir(), "нет")),
	} {
		got, err := collectTasks(fsys, testChildSID, "", envOf(nil), envOf(nil))
		if err != nil || len(got) != 0 || got == nil {
			t.Errorf("%s: %v, %v; want [], nil", name, got, err)
		}
	}
}

// Ловит (возврат 2 по T3): значение по умолчанию ключа Run (пустое имя) попадает в список —
// хранилище отвечает ErrInvalid, и проверка ребёнка не проходит никогда; состояние
// StartupApproved читается без учёта регистра имени.
func TestRunEntry(t *testing.T) {
	if e, ok := runEntry(startup.ScopeOwn, startup.SrcRun, "", `C:\a.exe`, envOf(nil), nil); ok {
		t.Errorf("пустое имя: %+v, true; want false", e)
	}
	state := map[string][]byte{"app": startup.EncodeApproved(false, time.Unix(1_800_000_000, 0))}
	e, ok := runEntry(startup.ScopeOwn, startup.SrcRun, "App", `"C:\P F\a.exe" -m`, envOf(nil), state)
	want := startup.Entry{Scope: startup.ScopeOwn, Source: startup.SrcRun, Name: "App", Command: `"C:\P F\a.exe" -m`, Exe: `C:\P F\a.exe`}
	if !ok || e != want {
		t.Errorf("runEntry = %+v, %v; want %+v, true", e, ok, want)
	}
	if e, _ := runEntry(startup.ScopeOwn, startup.SrcRun, "Other", `C:\a.exe`, envOf(nil), state); !e.Enabled {
		t.Error("нет значения StartupApproved — должно быть включено")
	}
}
