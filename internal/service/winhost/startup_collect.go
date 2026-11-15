package winhost

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"pcblock/internal/startup"
)

// Сборка записей автозагрузки без вызовов Windows (испытывается на любой системе): значения
// Run/RunOnce и файлы заданий Планировщика. Чтение реестра и папок — в startup_windows.go.

// runEntry — запись автозагрузки из строкового значения Run/RunOnce. Значение по умолчанию ключа
// (пустое имя) — ok = false: ключ записи scope+source+name требует имени (хранилище отвечает
// ErrInvalid, и проверка ребёнка не прошла бы никогда), а Проводник такое значение не запускает.
func runEntry(scope, source, name, cmd string, env func(string) string, state map[string][]byte) (startup.Entry, bool) {
	if name == "" {
		return startup.Entry{}, false
	}
	return startup.Entry{
		Scope: scope, Source: source, Name: name, Command: cmd,
		Exe: startup.ExeOf(cmd, env), Enabled: approvedEnabled(state, name),
	}, true
}

// approvedEnabled — запись включена: значения нет или оно не разобрано — включено (так считает
// Windows), иначе по первому байту.
func approvedEnabled(state map[string][]byte, name string) bool {
	b, ok := state[strings.ToLower(name)]
	if !ok {
		return true
	}
	on, known := startup.DecodeApproved(b)
	return on || !known
}

// collectTasks — задания «при входе» ребёнка sid (учётка account) и общие (startup.TaskScope) из
// дерева файлов заданий tasks (%WINDIR%\System32\Tasks). Папка Microsoft в корне пропускается
// целиком: её задания скрываются (RM5b-7). Корня нет — заданий нет. Файл не читается или не
// разбирается — ошибка всего списка (PL-46): иначе задание пропало бы на одну проверку, его строка
// удалилась бы, а на следующей проверке оно стало бы «новым» и выключилось бы вопреки решению
// родителя. Пустой файл (0 байт) пропускается: Планировщик такое задание не запускает, и пропуск
// одинаков на каждой проверке. Для exe своих заданий — переменные ребёнка ownEnv, общих — env.
func collectTasks(tasks fs.FS, sid, account string, ownEnv, env func(string) string) ([]startup.Entry, error) {
	out := []startup.Entry{}
	err := fs.WalkDir(tasks, ".", func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			if p == "." && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		if de.IsDir() {
			if strings.EqualFold(p, "Microsoft") {
				return fs.SkipDir
			}
			return nil
		}
		if !de.Type().IsRegular() {
			return nil
		}
		data, err := fs.ReadFile(tasks, p)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return nil
		}
		t, err := startup.ParseTaskXML(data)
		if err != nil {
			return fmt.Errorf("задание %s: %w", p, err)
		}
		scope, ok := startup.TaskScope(t, sid, account)
		if !ok {
			return nil
		}
		e := env
		if scope == startup.ScopeOwn {
			e = ownEnv
		}
		tp := `\` + strings.ReplaceAll(p, "/", `\`) // путь задания — по месту файла: его понимает schtasks /TN
		out = append(out, startup.Entry{
			Scope: scope, Source: startup.SrcTask, Name: tp, Task: tp, Command: t.Command,
			Exe: startup.ExeOf(t.Command, e), Enabled: t.Enabled,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
