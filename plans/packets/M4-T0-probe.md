# Пакет: T0 вехи M4 «Проба Brave на windows-latest» | слот: писатель | исполнитель: новый агент

Репозиторий `/home/claude/pc_block_soft`, ветка `feature/m4-sites`. Прочитай `plans/2026-11-04-M4.md`: Global Constraints, Rulings RM4-0, RM4-1, RM4-2, RM4-10, RM4-12, строку T0. Это проба: её цель — **факты для Ruling-ов**, а код адаптера остаётся боевым (T7 подключит его к решениям службы).

## Что сделать

### Часть A. Адаптер и Go-проба (тест на раннере от имени пользователя раннера)

1. `internal/guard/omnibox.go` (без тега Windows) — `func omniboxDomain(text string) string`: из текста поля адреса только хост в нижнем регистре, без `www.`, без порта, без логина `user@`, без точки в конце; `brave://…`, `chrome://…`, `about:`, `file:`, `view-source:`-префикс снимается и решается по остатку, текст с пробелом или без точки (поисковая фраза) — «»; IP — как есть. В T1 эту функцию заменит `sites.FromOmnibox` — пиши её так, чтобы перенос был копированием. Тесты `internal/guard/omnibox_test.go` — таблица литералами (не меньше 15 строк: `youtube.com/watch?v=1`, `https://WWW.Example.COM./a`, `brave://newtab`, `котики видео`, `localhost:3000` → «», `192.168.0.1:8080/x` → `192.168.0.1`, `user:pw@host.ru/` → `host.ru`, пусто, 2000 символов), плюс `FuzzOmniboxDomain` (не паникует, результат без `/`, `:` и пробелов).
2. `internal/guard/uia_windows.go` — COM UI Automation на `golang.org/x/sys/windows` (в проекте COM ещё нет; `go-ole` прямо не подключать). Нужно: `CoInitializeEx` (поток с `runtime.LockOSThread`), `CoCreateInstance(CLSID_CUIAutomation {ff48dba4-60ef-4201-aa87-54103eef594e}, IID_IUIAutomation {30cbe57d-d9d0-452a-ab13-7ac5ac4825ee})`, `ElementFromHandle`, `CreatePropertyCondition(UIA_ClassNamePropertyId=30012, VT_BSTR "OmniboxViewViews")`, `FindFirst(TreeScope_Descendants)`, `GetCurrentPropertyValue(UIA_HasKeyboardFocusPropertyId=30008)`, `GetCurrentPattern(UIA_ValuePatternId=10002)` → `get_CurrentValue`; для окна без поля адреса — `CreatePropertyCondition(UIA_ControlTypePropertyId=30003, UIA_DocumentControlTypeId=50030)` → `ValuePattern`. Индексы методов vtable сверить с `UIAutomationClient.h` (Windows SDK) и записать в комментарий с номерами; ошибку индекса проба покажет сразу. Освобождать всё (`Release`, `SysFreeString`, `VariantClear`).
   API: `type braveReader struct{…}` с кэшем элемента поля адреса по hwnd; `func (r *braveReader) Read(hwnd windows.HWND) (text string, focused bool, source string, err error)` (`source`: `omnibox` / `document` / «»). Поиск — только в рамке окна; кэш сбрасывается при смене hwnd или ошибке.
3. `internal/guard/sendkeys_windows.go` — `func closeActiveTab() error`: `GetAsyncKeyState` для Shift, Alt, Win — зажат хоть один → `errModifierHeld` без нажатий; иначе `SendInput` Ctrl down, W down, W up, Ctrl up. Отдельно `func closeTabButton(hwnd) error` — UIA: выбранный элемент `TabItem` → его кнопка закрытия → `InvokePattern.Invoke` (запасной путь для закреплённой вкладки).
4. `internal/guard/brave_probe_windows_test.go` — `TestBraveProbe`, только при `PCB_BRAVE_PROBE=1` (иначе `t.Skip`). Каждый факт — строкой `::notice title=Проба Brave::<факт>` в файл `$RUNNER_TEMP/pcb-brave-probe.txt` (образец `writeNotice` в `internal/service/winhost/procwatch_windows_test.go:505`), потому что журналы Actions не скачиваются. Шаги, у каждого отдельный факт «работает / не работает / что видно»:
   - (а) найти `brave.exe` (Program Files, Program Files (x86), `%LOCALAPPDATA%\BraveSoftware\Brave-Browser\Application`); версия файла;
   - (б) запуск `brave.exe --no-first-run --no-default-browser-check --user-data-dir=<temp> --new-window http://probe-site.test/a?b=c`; окно верхнего уровня процесса; `Read` ≤ 10 с → ожидание `omniboxDomain == "probe-site.test"`, источник `omnibox`; вывести ClassName/ControlType найденного элемента;
   - (в) 60 раз `Read` раз в секунду: процессорное время теста и всех процессов Brave до и после (`GetProcessTimes`) → факт «процессор за 60 с»;
   - (г) вторая вкладка `http://probe-two.test/` (Ctrl+T не нажимать: запуск `brave.exe … http://probe-two.test/` в том же профиле открывает вкладку), `closeActiveTab` → через ≤ 2 с `Read` даёт `probe-site.test`; закрепить вкладку нельзя без меню — проверить `closeTabButton` на обычной вкладке и записать, есть ли у `TabItem` кнопка закрытия;
   - (д) окно приложения `brave.exe --app=http://probe-app.test/ …` → `Read` даёт `document` и домен `probe-app.test`? (факт);
   - (е) политики: записать в `HKCU\Software\Policies\BraveSoftware\Brave` `TorDisabled=1`, `BraveVPNDisabled=1`, `DnsOverHttpsMode="off"`, `ExtensionInstallBlocklist\1="*"`, `URLBlocklist\1="blocked-probe.test"`; новый профиль, открыть `http://blocked-probe.test/` → текст документа (UIA `Document` → `Name`/текстовые потомки) содержит `ERR_BLOCKED_BY_ADMINISTRATOR`? открыть `brave://policy` → вывести строки с именами политик и статусом; поиск в окне кнопки с Name, содержащим `VPN`; в `t.Cleanup` удалить ключ и все процессы Brave, запущенные тестом.
   Тест падает только на (б) — остальное факты.
5. `.github/workflows/windows.yml`:
   - шаг «Brave для пробы и приёмки» перед «Тесты Go на Windows», если `inputs.scenario == 'accept' || inputs.scenario == 'probe'`: установка полного офлайн-установщика (сначала `choco install brave -y --no-progress`, затем проверка, что `brave.exe` есть; нет — скачать `BraveBrowserStandaloneSetup.exe` последнего стабильного релиза `github.com/brave/brave-browser/releases` и `/silent /install`), путь и версия — `::notice`; строки `127.0.0.1 probe-site.test probe-two.test probe-app.test blocked-probe.test` в `C:\Windows\System32\drivers\etc\hosts`;
   - вход `scenario` получает значение `probe`; в «Тесты Go на Windows» `PCB_BRAVE_PROBE=1`, если Brave поставлен;
   - шаг «Проба Brave (пометки)» `if: always()` выводит `$RUNNER_TEMP/pcb-brave-probe.txt` (образец «Сухой прогон наблюдателя (пометка)»).

### Часть B. Чтение от SYSTEM в сеансе ребёнка

6. `internal/ipc/commands.go` — `GuardStateReply.ForegroundDomain string \`json:"foregroundDomain"\`` (комментарий: домен активной вкладки Brave, прочитанный стражем последним; «» — не Brave или не прочитан). Обновить пример `internal/ipc/testdata/contract.json` через `contract_test.go` (как там принято) и, если фронт сверяет контракт, `frontend/src/types.ts`.
7. Страж: в `readRaw` (`internal/guard/sensors_windows.go:62`), если путь переднего плана оканчивается на `\brave.exe` (регистр не важен), — `braveReader.Read(fg)` → `omniboxDomain`, поле фокуса учитывается: в фокусе — прежний домен этого hwnd. Домен — новое поле `RawSensors.ForegroundDomain` (только в памяти стража); в службу пока не отправлять (SensorSample — T3); в `GuardStateReply.ForegroundDomain` — в тестовом режиме (см. сборку ответа около `internal/guard/controller.go:814`). Никакого полного адреса нигде, кроме локальной переменной (R23).
8. `tools/accept/probe-brave.ps1` (Id `PROBE`, добавить в `run-all.ps1`): по образцу `s45-forbidden-autostart.ps1` — `Initialize-Scenario`, `Start-Child` (ребёнок — пользователь раннера), запуск Brave от его имени на `http://probe-site.test/` (флаги как в 4б), `Wait-GuardState` ≤ 15 с, пока `foregroundDomain -eq "probe-site.test"` (если Brave не на переднем плане — вывести, что на переднем плане); факт — `::notice title=Проба Brave от SYSTEM::…`. Закрыть Brave в конце.

## Порядок и прогоны

- R0/R1 локально: `export PATH=~/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off`; `go test -race -count=1 ./internal/guard/ ./internal/ipc/`; `gofmt -l internal/`.
- Windows-код: `GOOS=windows go vet -tags desktop,production ./internal/guard/ ./internal/ipc/` с временным modfile, где `replace golang.org/x/sys => /tmp/claude-0/-home-claude/25f23a37-6a15-5cbe-89db-2e036117eed9/scratchpad/xsys` (копия v0.48.0 уже там; modfile не коммитить).
- RED прежде кода для `omniboxDomain` (таблица падает → код → зелёный → мутации: убрать снятие `www.`, порта, логина — каждая ломает тест). COM-адаптер проверяется только пробой на раннере — это разрешённое исключение.
- Коммиты маленькие, пути явно (`git add <пути>`), Conventional Commits по-русски; трейлеры:
  `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze`. НЕ пушить — пушит оркестратор и запускает `windows.yml` (`scenario=probe`, затем `scenario=accept only=PROBE`).

## Уроки (docs/LESSONS.md)

- PL-1: упавший шаг CI сам рассказывает, почему упал — факты пробы пометками `::notice`, ошибки — `tools/annotate-errors.sh`.
- PL-9: «не прочитано» = ничего не утверждать: ошибка UIA → домен «», не догадка.
- PL-15: путь и домен — из одного прохода `readRaw` по одному hwnd.
- PL-16: приёмка ждёт содержимое (домен в GuardState), а не окно Brave.
- PL-22: образец ожидания строится из того, что пишет программа.
- PL-41: запасную ветку, которую не исполняли, считать сломанной — `closeTabButton` и `document` проверяются пробой явно.

## Вердикт (≤ 30 строк)

```
TASK T0: done | blocked | failed
Diff: файлы ±строк; sha коммитов
Evidence: локальные тесты (имена, счётчики)
Probe facts: (оставь пустым — факты с раннера соберёт оркестратор по пометкам)
Rulings / Escalations / Lesson / Open
```
