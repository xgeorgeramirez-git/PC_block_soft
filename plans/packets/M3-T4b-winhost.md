# Пакет M3-T4b: Windows-адаптер наблюдателя процессов | слот: писатель | дерево: `/home/claude/pc_block_soft`, ветка `feature/m3-programs`

## Контекст продукта

PC_block_soft — домашняя программа родительского контроля для Windows 11 (Go-служба от SYSTEM, страж в сеансе ребёнка, значок, окна Wails + React/TS, SQLite). Веха M3. План `plans/2026-10-29-M3.md` (читай: Global Constraints, RM3-1, RM3-3, RM3-8, RM3-10, RM3-11, Review Focus 3, 8, 9). Уже в ветке T1…T4a: порт `service.ProcessWatcher`/`ProcInfo`, порты `Scanner`, `FileMeta`, поля `Roots` в `service.Config`, `WatchDir` в тестовом режиме. **Изучи `git log --stat -50` и вердикт T4a; используй настоящие сигнатуры.**

Твоя задача — реальная Windows-реализация портов и тест адаптера, который в CI запускается на windows-latest. Локально Windows-код не запускается: проверяй типы кросс-сборкой (временный modfile с replace на github-зеркала x/sys, CLAUDE.md «Среда») и пиши тесты с `//go:build windows`, самостоятельно пропускающиеся без нужных условий только там, где это честно (на windows-latest пропуска быть не должно).

Отвечай по-русски. Методика: TDD, мутации, маленькие коммиты.

## 0. Среда

- Дерево `/home/claude/pc_block_soft`, ветка `feature/m3-programs`; ты единственный писатель. `plans/`, ТЗ не правь. Коммить `git add <пути>`; push не делай.
- Go: `export PATH=/home/claude/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off`. Кросс-проверка: `GOOS=windows go vet ./internal/service/winhost/...` через временный modfile (не коммитить). Файлы реализации — `*_windows.go`, общая часть без Windows тестируется на ubuntu.
- Коммиты: Conventional Commits по-русски; трейлеры `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01XuHYMinhbS8zjLqh6nAgCY`.

## 1. Что сделать

1. **`processWatcher` (`winhost/procwatch_windows.go`)**, реализует `service.ProcessWatcher`:
   - `List(sessionID)`: `CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS)` → `Process32First/Next` (переиспользуй/вынеси из `install_windows.go` общий код `imagePath`, не дублируй); для каждого pid: `ProcessIdToSessionId` (не совпал с `sessionID` — пропуск), `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)` → `QueryFullProcessImageName` (буфер растёт до 32 KiB), `GetProcessTimes` (Stamp, как в `launcher.Stamp`), путь разворачивается `GetLongPathName` (RM3-1: 8.3-имена `RUNNER~1` → длинные) и junction/symlink через `GetFinalPathNameByHandle` по возможности; ошибка → `PathOK=false` (процесс остаётся в списке, служба его не тронет). Сам процесс не читается и не изменяется. Пропускай pid 0 и 4, собственный pid.
   - `Kill(p)`: `OpenProcess(PROCESS_TERMINATE|PROCESS_QUERY_LIMITED_INFORMATION|SYNCHRONIZE)`, сверка `Stamp` (создание процесса) с `p.Stamp` по ручке, сверка пути, `TerminateProcess(h, 1)`, ожидание до 300 мс (`WaitForSingleObject`), `true` только если процесс завершён; отказ доступа (защищённый процесс) → `false` без паники; ручка всегда закрывается; вызов не берёт `s.mu` (это обязанность службы, ты ничего не блокируешь дольше 300 мс).
2. **`winScanner` (`winhost/scan_windows.go`)** — `service.Scanner`: корни приложений = первые подпапки `%ProgramFiles%` и `%ProgramFiles(x86)%` (без `Windows*`, `Common Files`, `Microsoft` внутреннего служебного, `WindowsApps`, каталога установки PC Block); в каждой — «главный» exe: имя файла совпадает с именем папки (без пробелов/регистра), иначе единственный exe в корне папки, иначе самый большой exe в корне (глубже 2 уровней не заходить); плюс известные игры/лаунчеры по каталогу (`programs.Catalog`) в этих корнях (рекурсия до 3 уровней, не больше 5000 файлов, общий таймаут 10 с, прерываемо); возвращает не больше 400 записей, `Name` — `FileDescription` или имя файла, `Publisher` — `CompanyName`. Ошибки отдельных папок пропускаются, общая ошибка — только если не удалось прочитать ни одного корня.
3. **`fileMeta(path)` (`winhost/filemeta_windows.go`)**: `GetFileVersionInfo`/`VerQueryValue` (`version.dll`; кодовая страница из `\VarFileInfo\Translation`, запасной вариант `040904b0`, `041904b0`) → `CompanyName`, `FileDescription`; нет данных → `("", "")` (интерфейс покажет «Без подписи издателя», RM3-3); отпечаток `"<size>:<mtime unix>"` через `os.Stat`. Подпись Authenticode не проверяется (README «Отклонения»).
4. **`Roots` из KnownFolders** (`SHGetKnownFolderPath`: `FOLDERID_ProgramFiles`, `FOLDERID_ProgramFilesX86`, `FOLDERID_Windows`) + `InstallDir` из `os.Executable()`; все нормализуются `programs.Normalize`. Подключи в `openHost()` (`host_windows.go`) в `service.Config` (`Watcher`, `Scanner`, `FileMeta`, `Roots`).
5. **Тестовый режим:** значение реестра `WatchDir` читается в `ParseTestMode` (T4a добавил поле); путь нормализуется.
6. **Тест адаптера на windows-latest** (`procwatch_windows_test.go`, `//go:build windows`): копирует `C:\Windows\System32\notepad.exe` (или `cmd.exe`, если notepad недоступен) во временную папку, запускает в **текущем сеансе** как дочерний процесс; `List(текущий сеанс)` находит его с `PathOK=true`, путь в нижнем регистре после `GetLongPathName` равен ожидаемому (проверь и 8.3-вариант временного пути, если системная переменная TEMP короткая); `Kill` завершает, `false` на повторном вызове (процесса нет) и на чужом `Stamp`; время от запуска до исчезновения при цикле `List`+`Kill` каждые 250 мс ≤ 1 с (допуск: измерь время на цикле, ограничь сверху 1500 мс на раннере, в сообщении при провале выведи фактическое время); процесс `System`/`csrss` — `Kill` не берётся (`false`, без паники). Пометки сборки при провале: сообщения тестов должны попадать в `tools/annotate-errors.sh`.
7. **Сухой прогон (RM3-8):** тест (или шаг в `windows.yml`, как удобнее) перечисляет процессы сеанса раннера через `List`, прогоняет каждый через `programs.Classifier` с настоящими `Roots` и **выводит** `::notice` со сводкой: сколько процессов, сколько «система», сколько «незнакомых» и их пути (первые 20). Ожидание-утверждение: ни один процесс из `C:\Windows`, `Program Files*`, каталога установки или `RUNNER_TOOL_CACHE`/`C:\hostedtoolcache`/`C:\actions-runner` не должен получить `VerdictUnknown` при включённом `WatchDir`-фильтре (вне `WatchDir` — «не трогать»); отдельно — список «что наблюдатель СЧИТАЕТ закрываемым вне WatchDir» должен быть пуст.

## 2. Ruling оркестратора

- Реализация порта не принимает решений: категория, система/не система, «закрывать ли» — только в службе (`programs`, `Watch`). Адаптер возвращает факты (процесс, сеанс, путь, штамп) и выполняет `Kill`.
- Завершение — только по `PID+Stamp+путь` (PL-9: гонка повторного использования PID).
- Время ожидания в `Kill` ≤ 300 мс; опрос не блокирует канал (служба зовёт вне `s.mu`).
- Для приёмки и сухого прогона допускается дополнительная диагностическая функция, но не в боевом пути.

## 3. Review Focus

1. PID reuse и гонка «процесс умер между List и Kill» → `false` без записи в журнал родителя.
2. Нечитаемый путь, 8.3-имена, пробелы, кириллица, длинные пути > MAX_PATH (префикс `\\?\`), сетевые пути.
3. Утечки ручек (`CloseHandle` на всех путях), буфер `QueryFullProcessImageName`.
4. Защищённые процессы и антивирус: отказ доступа — штатный, не ошибка.
5. Сканер: не заходит в `WindowsApps`, не зависает на сетевых/заблокированных папках, ограничен по числу и времени; результат детерминирован (сортировка).
6. Тест не убивает ничего, кроме собственного дочернего процесса; в сухом прогоне нет вызова `Kill`.

## 4. Вердикт (≤ 40 строк)

`done/blocked/failed`; Diff (sha + строка); Evidence (кросс-сборка `GOOS=windows go vet`, тесты ubuntu, мутации; что проверит только CI windows); Rulings; Escalations.

## 5. Дополнения оркестратора после T3b/T4a

- Сканер (`winScanner`) включает установленные пакеты Microsoft Store: подпапки `%ProgramFiles%\WindowsApps\` (доступ SYSTEM есть), для каждого пакета — exe из `AppxManifest.xml` (`<Application Executable="…">`; разбор XML стандартной библиотекой), путь `…\windowsapps\<пакет>\<exe>`; системные фреймворки (`Microsoft.VCLibs*`, `Microsoft.NET*`, `Microsoft.UI.Xaml*`, `*.Framework`, ресурсные пакеты без `Application`) пропускаются. Корень приложения — семейство пакета (см. пакет T4a, Д-3, функция в `internal/programs`).
- `FileMeta` читает свойства файла через путь; если чтение не удалось — пустой отпечаток (служба в этом случае снова спрашивает родителя, T4a Д-1). Не подменяй пустой отпечаток «запасным» значением.
- Сканирование вызывается службой вне блокировки (T4a Д-2): общий таймаут 10 с остаётся, но он больше не держит канал.
