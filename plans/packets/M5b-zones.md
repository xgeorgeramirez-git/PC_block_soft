# M5b — карта зон кода (читатель, слот B, 14.11.2026)

Источник: чтение main без правок. Пути от корня репозитория. Сигнатуры скопированы из кода.

---

## A1. Команды службы от окна: регистрация, права, журнал, тексты

**Таблица прав** — `internal/service/commands.go:202-299` (`func init()` заполняет `commandRules map[string]rule`).
```go
type rule struct {
	allow   check
	handler func(*Service, *callCtx) ipc.Reply
}
type check func(c *callCtx) string          // "" — разрешено, иначе текст отказа
func parentToken(touch bool) check           // :104 вход родителя; touch=true продлевает сессию (для записи)
func childSelf() check                       // :138 ребёнок своего сеанса или страж; ставит c.self
func testMode(inner check) check             // :155
func admins() check / elevated() check / users() check / kinds(ks ...clientKind) check / or(...) / and(...)
func (s *Service) Handle(peer ipc.Peer, msg ipc.Msg, real time.Time) (rep ipc.Reply) // :313, под s.mu, recover
```
Образец строк: `ipc.CmdListPromos: {parentToken(false), (*Service).hListPromos}`, `ipc.CmdGeneratePromos: {parentToken(true), (*Service).hGeneratePromos}` (:242, :244). Чтение — `parentToken(false)`, изменение — `parentToken(true)`.

**Образец обработчика** — `internal/service/promos_m5.go:81`, `handlers_0_2_2.go:105-193`:
```go
func (s *Service) hGeneratePromos(c *callCtx) ipc.Reply
func (s *Service) hCreatePromo(c *callCtx) ipc.Reply
func (s *Service) hRedeemPromo(c *callCtx) ipc.Reply
// шаблон тела:
var req ipc.CreatePromoRequest
if c.msg.Into(&req) != nil { return bad() }
...return ipc.FailField("Можно от 1 до 600 минут", ipc.FieldMinutes)
...return ipc.Fail(textSaveFailed)
s.journal.Write(0, evPromoCreate, map[string]any{"code": ..., "text": fmt.Sprintf(...)})
return ipc.OK(ipc.CreatePromoReply{...})
```
Ребёнок по id: `cs := s.children[id]` (map `childState`), по SID — `func (s *Service) childBySID(sid string) *childState` (`service.go:387`). Нет ребёнка — `ipc.Fail(textNoSuchChild)`.

**Журнал**: порт `Journal.Write(childID int64, kind string, details map[string]any)` (`ports.go:428`, реализация `StoreJournal` :432-440). Ключ `"text"` в details — готовая строка для родителя.
- Новый вид события: константа в `internal/service/eventkinds.go` (группы const :7-60), строка в `allEventKinds` (:63-93), ровно одна группа в `journalGroups` (`journalgroups.go:11`, группы `ipc.JournalGroupPrograms`, `JournalGroupSettings`…), текст в `eventText(kind, name string, d map[string]any) string` (`journaltext.go:48`, `case ev…:`). Проверка: `TestEventKindsSourceScan` (разбор исходников), фронт — `frontend/src/lib/journal.ts` (VIOLATION_KINDS).
- Тексты ошибок — `internal/service/texts.go:11-62` (`textSaveFailed`, `textRetry`, `textNoSuchChild`, `textNeedParent`…).

Как расширять: новый файл `internal/service/startup_m5b.go` с `hListStartup`, `hSetStartup`; строки в таблице прав (`parentToken(false)` / `parentToken(true)`); вид `evStartupDecided` (+ «запустилась общая запись и закрыта»?) в группу «Программы».
Риски: тест требует строку прав для каждой `ipc.AllCommands`; вид события без группы/текста роняет `TestEventKindsSourceScan`; обработчики идут под `s.mu` — чтение реестра в обработчике держит тик (см. A2).

---

## A2. Порты службы и winhost: реестр, COM, профиль, вход ребёнка

**Порты** — `internal/service/ports.go`. Ближайший образец «реестр ребёнка»:
```go
// ports.go:413
type Policy interface {
	SetTaskMgrDisabled(sid string, disabled bool) error
	SetBraveRules(sid string, r BraveRules) error
	BraveRulesOK(sid string, r BraveRules) (bool, error)
	RemoveBraveRules(sid string) error
}
type FileMeta func(path string) (publisher, description, fingerprint string)   // :266
type Scanner interface { Scan() ([]FoundProgram, error) }                         // :260
```
`Config` (`service.go` ~:79-111): `Launcher SessionLauncher`, `Admins AdminChecker`, `Accounts AccountDirectory`, `Policy Policy` (nil — не ставится), `Test TestMode`, `FileMeta FileMeta`, `Watcher ProcessWatcher`, `Scanner`. Сборка — `internal/service/winhost/host_windows.go:78-97` (`Policy: &policy{logf: logf}`, `FileMeta: meta`, `Watcher: newProcessWatcher()`).

**Реестр уже есть** (`golang.org/x/sys/windows/registry`, x/sys v0.48.0): `internal/service/winhost/policy_windows.go`, также `install_windows.go`, `testmode_windows.go`.
```go
const profileListPath = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`  // :24
const tempHivePrefix  = "PCBlockTmp-"                                                // :26
procRegLoadKeyW = advapi32.NewProc("RegLoadKeyW"); procRegUnLoadKeyW               // :38-39
type policy struct { mu sync.Mutex; base string; logf func(string, ...any) }        // :48
func (p *policy) withHive(sid string, fn func(hive string) error) error              // :117 — ProfileImagePath → NTUSER.DAT, SeBackup/SeRestore, RegLoadKey под HKU\PCBlockTmp-<SID>, fn, выгрузка; профиля нет — nil
func loadHive(name, file string) error / func unloadHive(name string) error          // :161, :177
func hiveLoaded(hive string) (bool, error)                                           // :255 — HKU\<SID> загружен = пользователь вошёл
func readValues(k registry.Key, key string) ([]service.BraveValue, error)            // :380
func deleteTree(root registry.Key, path string) error                                // :458
func enablePrivilege(name string) error                                              // priv_windows.go:14
```
Схема «вошёл — HKU\<SID>; не вошёл — withHive» уже в `SetTaskMgrDisabled` (:68-84) и `SetBraveRules` (:194-213); `BraveRulesOK` намеренно НЕ грузит профиль ради сверки (T5-4: вход ребёнка в эту секунду сломался бы).

**COM**: в службе нет. Есть только в страже — `internal/guard/uia_windows.go:17-40` (UI Automation «только x/sys/windows: go-ole напрямую не подключается»; свой `comHeader{vtbl *[64]uintptr}`, `call(o comObj, method int, args ...uintptr)`, `release`, `queryInterface`, `bstrFrom/bstrTake`, `variantClear`, поток с `runtime.LockOSThread`, MTA). `go-ole` в go.mod только `// indirect` (от Wails). **Планировщика заданий (ITaskService), StartupApproved, AppX/PackageManager в коде нет** (grep пуст).

**Вход ребёнка** — `internal/service/sessions.go`:
```go
type sessState struct { id uint32; user, sid string; childID int64; screen ScreenState; ...; since time.Time; guard, tray *supProc; windows map[string]*supProc } // :69
func (s *Service) OnSession(ev SessionEvent, id uint32, real time.Time)   // :91 события диспетчера служб
func (s *Service) onSession(ev SessionEvent, id uint32, real time.Time)   // :99 (его же зовёт TestSession)
func (s *Service) syncSessions(real time.Time)                            // :209 — Launcher.Sessions(), раз в секунду из Housekeep и на каждое событие
```
Точка «ребёнок появился в сеансе» — `sessions.go:249-256`:
```go
if cs := s.childBySID(si.SID); cs != nil {
	ss.childID = cs.child.ID
	if prev != ss.childID {
		s.checkAdmin(cs, real)
		s.applyPolicy(cs.child.SID, true)
		s.applyBrave(cs)
	}
}
```
`ss.since` = момент, когда служба увидела сеанс (после перезапуска службы — момент перезапуска, Ruling RF18-3). Периодика: `Housekeep(real)` раз в настоящую секунду (`loop.go:209-231`, шаги через `s.guard("name", cs, func(){...})`); образец «раз в N» — `housekeepBrave` (`brave.go:156`, `braveCheckEvery = 60 * time.Second`, `service.go:22`).
Тяжёлое вне `s.mu`: образец — `beginScan()`/`ensurePrograms` (`programs.go:~107-128`, снимок в своей горутине), наблюдатель (`watch.go:17-19`).

Как расширять: новый порт, например `StartupSource interface { List(sid string) ([]StartupEntry, error); SetEnabled(sid string, e StartupEntry, on bool) error }` в ports.go, реализация `winhost/startup_windows.go` рядом с policy (переиспользовать `withHive`, `hiveLoaded`; HKLM Run/RunOnce, папки «Автозагрузка» профиля и `%ProgramData%\...\Startup`, значения `...\Explorer\StartupApproved\{Run,Run32,StartupFolder}` — штатное выключение «как в Диспетчере задач»). Вызов: на `prev != ss.childID` и раз в 10 мин из Housekeep, само чтение — в горутине вне `s.mu`.
Риски: (1) выключение через StartupApproved — запись в HKU ребёнка, при незагруженном профиле — `withHive` (у системы есть SeRestore); (2) задания Планировщика «при входе» и автозапуск приложений магазина требуют COM (ITaskService) / AppX-API — в службе этого нет, писать с нуля по образцу uia_windows.go (MTA-поток), либо урезать до реестра+папок; (3) гонка: Explorer запускает Run/RunOnce через секунды после входа, служба видит сеанс по WTS-событию или опросу 1 с — «новая запись выключена» при первом же входе возможна только если запись появилась до входа (служба проверяет раз в 10 мин); (4) RunOnce Windows удаляет после запуска.

---

## A3. Наблюдатель процессов (R13), данные о процессе, кэш свойств

Файлы: `internal/service/watch.go` (922 стр.), `launchkind.go`, `procstate.go` (это НЕ наблюдатель: файл памяти о своих страже/значке/окнах для KillStale, `procRecord{Role, ..., Stamp}`).
```go
// ports.go:276 — всё, что известно о процессе
type ProcInfo struct {
	PID, ParentPID uint32; Stamp uint64 /*момент создания*/; SessionID uint32
	Path string; PathOK bool; UserSID string; UserOK bool
}
type ProcessWatcher interface { List(sessionID uint32) ([]ProcInfo, error); Kill(p ProcInfo) bool } // :289
```
**Командной строки нет**, издатель — только через `FileMeta` и только если решение его ждёт (`needsMeta`), время старта — `Stamp` (FILETIME создания), «впервые увиден» — `launchFacts.At`. Реализация: `winhost/procwatch_windows.go` (Toolhelp32, ProcessIdToSessionId, QueryFullProcessImageName, TerminateProcess), `procinfo_windows.go`, `procpath.go`.

Проход: `func (s *Service) Watch(real time.Time)` (:163) — `watchSnapshot` под s.mu → `List` → `watchEntryOf` → `fetchMeta(need)` → `watchDecide` → `killAll` → `readClosedMeta` → под s.mu `applyWatch` → `recordClosed` / `settleLaunches`. Тикер `watchEvery = 250 * time.Millisecond` (:23).
```go
type watchTarget struct { session uint32; childID int64; sid string; d core.Decision; own map[uint32]bool;
	at, logonAt, inputAt, sampleAt time.Time; fg string; byPID map[uint32]ProcInfo; hold bool } // :91; logonAt: ss.since (:580)
type watchEntry struct { session uint32; path, norm, key, fp string; fpRead bool; pub string; pubRead, closed bool; seen uint64;
	manual bool; why, parent string; facts launchFacts; quiet, classified, forbiddenAtStart bool } // :51
type watchAct struct { t *watchTarget; p ProcInfo; e *watchEntry; res programs.Result; cat core.Category;
	plateKind string; auto bool /*без плашки и нарушения*/; killed, again bool; publisher, descr string } // :112
func (s *Service) watchDecide(t *watchTarget, p ProcInfo, e *watchEntry) *watchAct  // :630
func (s *Service) newWatchEntry(t *watchTarget, p ProcInfo, norm string, gen uint64, depth int) *watchEntry // :669
func (s *Service) recordClosed(cs *childState, a *watchAct, real time.Time)          // :804
func (s *Service) settleLaunches(real time.Time)                                      // :758 → evProgramAutoClosed раз в день
func (s *Service) autoClosedOnce(childID int64, day, path string, real time.Time) bool // :870
// launchkind.go
type launchFacts struct { At, LogonAt, InputAt time.Time; Self, Parent, Shell, Foreground string; Store bool; Broker string } // :29
func launchKind(f launchFacts) (bool, string)                         // :48; manualLogonGrace = 60 * time.Second (:24)
func launchParentOf(p ProcInfo, by map[uint32]ProcInfo, sid string) string // :90
```
Закрытие без плашки (R13): `watchDecide` → `a.auto = e.forbiddenAtStart && !e.manual` для `VerdictForbidden`; `recordClosed` при `a.auto` кладёт в `s.launchWait` (без плашки), `settleLaunches` пишет `evProgramAutoClosed` раз в день. Тестовый режим: закрывается только `Test.WatchDir` (`watchDecide` :631-633).

Как расширять (правило FR-39 «общая запись выключена для ребёнка»): в `watchTarget` добавить набор нормализованных exe выключенных общих записей ребёнка (снимок под s.mu в `watchSnapshot` :534); в `watchDecide` до `switch res.Verdict` — если `e.norm` в наборе и запуск автоматический (`!e.manual`, `t.at - t.logonAt < manualLogonGrace` или родитель — Проводник/userinit) → `&watchAct{..., auto: true, plateKind: ...}` новым видом без плашки; путь записи — отдельной веткой в `recordClosed` (своя строка журнала или без журнала). Путь exe из `command` записи автозагрузки разбирать в чистой функции (кавычки, `%ProgramFiles%`, аргументы) — тестируется на Linux.
Риски: нет командной строки — нельзя отличить «тот же exe, запущенный не записью»; программа, разрешённая родителем (VerdictAllow), сейчас никогда не закрывается — новое правило должно идти раньше вердикта, но после `VerdictSystem`/`IsSystem` (Компоненты Windows и PC Block не трогать); вне `WatchDir` в тестовом режиме ничего не закрывается — accept S50 обязан держать exe общей записи в WatchDir.

**Кэш свойств файла (B-60)** — `watchState` (`watch.go:127-145`): `metaBusy map[string]bool`, `metaDone map[string]fileMeta` (под `metaMu`), ключ — **только путь**:
```go
type fileMeta struct { publisher, description, fingerprint string }   // handlers_m3.go:218
func (w *watchState) finishMeta(path string, m fileMeta)              // :384 (сброс при maxMetaDone = 1000)
func (w *watchState) takeDone(path string) (fileMeta, bool)           // :395
func (s *Service) fetchMeta(need []*watchEntry) (late []string)       // :466, metaBudget = 300 * time.Millisecond (:40)
func (s *Service) readClosedMeta(acts []watchAct)                     // :318
```
B-60: опоздавшее чтение отдаётся следующему процессу с тем же путём; перезаписанный установщик в Temp может получить отпечаток прежнего файла. Как чинить: хранить в `fileMeta` отпечаток «размер:mtime» или `procKey` и в `takeDone` сверять (fingerprint уже содержит размер и время — дешёвое повторное `os.Stat`). Также `s.fpSeen/s.pubSeen` (по пути, :258-259) — для категории секунды учёта.

---

## A4. Хранилище: миграции, интерфейс, фейк, локальный запуск

- Миграции: `internal/storage/migrations/NNN_имя.sql`, встроены `//go:embed migrations/*.sql` (`migrate.go:19-20`), имя `^(\d+)_[a-z0-9_]+\.sql$` (:30), первая строка `-- Назначение: …`, файлы в main не переписываются (L-131). Последняя — `010_stats_m5.sql`. Следующая: `011_startup_m5b.sql`.
- **`startup_items` ещё нет ни в одной миграции** (только в ТЗ, `docs/01_ТЗ.md:221-232`):
```sql
CREATE TABLE startup_items ( id INTEGER PRIMARY KEY, child_id INTEGER NOT NULL REFERENCES children(id),
  scope TEXT NOT NULL CHECK (scope IN ('own','shared')), source TEXT NOT NULL, -- run, runonce, startup_folder, task, store_app
  name TEXT NOT NULL, command TEXT NOT NULL, publisher TEXT NOT NULL DEFAULT '',
  decision TEXT NOT NULL CHECK (decision IN ('pending','on','off')), first_seen INTEGER NOT NULL, decided_at INTEGER,
  UNIQUE (child_id, scope, source, name)
```
- Образцы методов: `internal/storage/promo.go` (`func (s *Store) CreatePromos(codesDisplay []string, minutes int) ([]Promo, error)` — одна транзакция), `rest.go` (`GetBreakRules/SetBreakRules`). Ребёнок мягко удаляется: `RemoveChild` = `UPDATE children SET removed_at` (`data.go:73`) — строки детей не удаляются.
- Интерфейс службы: `type Store interface` (`service/ports.go:156-246`); свой тип строки службы рядом (`PromoRecord`, `Program`…) — служба не видит storage-типы.
- Адаптер: `internal/service/winhost/adapters_windows.go:35` `type storeAdapter struct{ s *storage.Store }` — по методу-пробросу на каждый метод (например :371 `RedeemPromo`).
- Фейк: `internal/service/fakes_test.go:19` `type fakeStore struct` (поля `fail*` для R18), `newFakeStore()` :90; новый метод Store обязателен и в фейке, и в адаптере (иначе не собирается). Харнесс: `harness_test.go` — `newHarness(t, opts ...hopt)`, `withChild(sid, name, min)`, `configured()`, `h.login(p)`, `h.ok/h.fail(peer, token, cmd, req, resp)`, `h.childLogsIn()`, пиры `childPeer()`, `adminPeer()`, `guardPeer()`; фейки `fakePolicy` (:1467), `fakeWatcher` (:1726, `start/startFrom/startAs/exit/killedPaths`), `fakeLauncher` (:1307).
- Локально хранилище (CLAUDE.md «Состояние»): подменный драйвер `github.com/ncruces/go-sqlite3` во временном modfile (`GOPROXY=direct GOSUMDB=off`, x/sys через `github.com/golang/sys`), журнал DELETE вместо WAL; не коммитить; итог — `ci.yml`. Остальное локально: `sed 's/^go 1.27.*/go 1.24/; /^toolchain/d' go.mod > /tmp/local.mod && cp go.sum /tmp/local.sum && GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 -modfile=/tmp/local.mod ./internal/core/... ./internal/service/ ./internal/ipc/... ./internal/programs/...`.

Риски: ключ UNIQUE по `name` — у записи реестра имя значения, у ярлыка имя файла, у задания путь задания; «найденные при первой проверке остаются как были» — нужна граница (по образцу `bonus.autobonus_from_day` в settings, D-82) или признак первой проверки ребёнка; общие записи хранятся на ребёнка (scope shared) — одна общая запись = N строк.

---

## A5. Канал ipc

- Имена команд и `AllCommands` — `internal/ipc/commands.go:89-115`; поля ошибок `Field*` — :118-…; DTO по вехам в отдельных файлах (`commands_m5.go` — образец: const `Cmd…`, `Field…`, `…Request`/`…Reply` с json-тегами).
- Контракт: `internal/ipc/testdata/contract.json` — пример запроса и ответа каждой команды; обновить `go test ./internal/ipc/ -run Contract -update` (`doc.go:20-30`); vitest разбирает те же примеры (`frontend/src/api.ts`, contract.test.ts). Массивы — всегда `[]` (L-22).
- Тестовые события сеанса: `TestSessionDisconnect = "disconnect"`, `TestSessionLogoff = "logoff"` (`commands.go:837-838`) — **входа (`logon`) нет**. Тестовые действия стража `TestForeground = "foreground"` (:786) и др.

Как расширять: `internal/ipc/commands_m5b.go`: `CmdListStartup`, `CmdSetStartup`, `StartupItemView{ID, Scope, Source, Name, Command, Publisher, Decision, From string…}`, `ListStartupRequest{ChildID}`, `ListStartupReply{Pending, Own, Shared []StartupItemView}`, `SetStartupRequest{ChildID, ID int64; On bool}`, `SetStartupReply{Item, Message}`; строка в `AllCommands`, пример в contract.json, зеркало в `frontend/src/types.ts`. Возможно `TestSessionLogon` для S50.
Риски: биндинги Wails не меняются (`frontend/wailsjs/go/ui/App.d.ts` — только `Call(arg1:string,arg2:string)`), всё через `Call`.

---

## A6. Окно родителя (frontend/src)

- `screens/Children.tsx:16` `export default function Children({ onOpenSettings, onOpenPrograms }: Props)` — вкладки детей (`role="tablist"` «Дети», :86), панель `<ChildLimits childId=… />` (:111).
- `screens/ChildLimits.tsx`:
```ts
type Tab = "limits" | "schedule" | "end" | "rest";                    // :42
const TABS: { id: Tab; label: string }[] = [ {id:"limits",label:"Лимиты"}, {id:"schedule",label:"Запретные часы"},
  {id:"end",label:"Когда время вышло"}, {id:"rest",label:"Перерыв на отдых"} ];  // :44-49
export default function ChildLimits({ childId, others = [], onSaved, onOpenPrograms }: Props) // :56
// панели: {tab === "rest" ? (<div role="tabpanel" id={LIMITS_PANEL} aria-labelledby="limits-tab-rest">…)} ; одна <SaveBar dirty saving … onSave onDiscard /> (:~357)
```
  Клавиши вкладок — `onTabKey` (:80). Черновики всех вкладок сохраняются одной SaveBar.
- `api.ts:88` `export async function call<Req extends object, Resp>(command: string, req?: Req): Promise<Resp>`; образец: `export function generatePromos(req: GeneratePromosRequest): Promise<GeneratePromosReply> { return call("GeneratePromos", {...}); }` (:467). Ошибки — `lib/errors.ts` (`errorText`, `childErrorText(e)` :72; `CallError` с полем).
- Мок: `src/__mocks__/backend.ts` — `interface MockOptions` (:181; добавить `startup?: …`), `createBackend(opts)` (:956) с обработчиками по имени команды (образец `RedeemPromo: (req, now) => {...}` :1557, `fail(text, field)`, `okReply(...)`, `addEvent(...)`), `installBackend` :1927. Собирается в браузерный скрипт `e2e/mockBundle.ts` (проверка в `src/__mocks__/bundle.test.ts`).
- E2E-снимки: `frontend/playwright.config.ts` (testDir `e2e`, 960×640, `colorScheme: "light"`, `npm run preview`), `frontend/e2e/screens.spec.ts` (3171 стр.): `mockProgram(page, options)`, `noHorizontalScroll`, `wholePage`, `inFrame`; тёмная тема — `await page.emulateMedia({ colorScheme: "dark" })` (:111-116) или цикл `for (const scheme of ["light", "dark"] as const)` (:258-269); снимок `page.screenshot({ path: \`${SCREENS}/s04-journal-dark.png\`, fullPage: true })`. Запуск только в CI (браузеры в песочнице не качаются).
- Тесты экрана: `screens/ChildLimits.test.tsx`, `ChildLimits.rest.test.tsx` (образец отдельного файла на вкладку).

Как расширять: вкладка `"startup"` «Автозагрузка» в `TABS` с компонентом `components/StartupList.tsx`; выключатель действует сразу (сообщение «Будет запускаться при входе Миши»), вне SaveBar.
Риски: вкладка без черновика не должна давать `dirty` и не должна сбрасываться `discard`; `useAttemptNavigation` (guard.tsx) — переход со страницы при несохранённом в других вкладках; имя ребёнка в сообщениях — родительный/предложный падеж («при входе Миши») — нужна функция склонения или нейтральная формулировка.

---

## A7. Приёмка Windows

- `.github/workflows/windows.yml`: `workflow_dispatch.inputs.scenario` (пусто / `smoke` / `accept` / `probe`) и `only` (список через запятую), шаг :387-395:
```yaml
$only = @("${{ inputs.only }}" -split "[,\s]+" | Where-Object { $_ })
./tools/accept/run-all.ps1 -InstallerDir build/bin -LogPath "$env:RUNNER_TEMP/ci.log" -Only $only
```
- `tools/accept/run-all.ps1`: массив `$all = @( @{ Id = "S51"; File = "s51-bonus-catchup.ps1" }, … )` (S33 последним), каждый сценарий в своём процессе на свежей установке; итоговые `::notice` (список M5 :~85 — добавить S50 в notice/описание input).
- `tools/accept/common.ps1` (1713 стр.): `Invoke-Scenario "S45" "…" { … }`, `Step "…" { … }`, `Initialize-Scenario 60`, `Start-Child $LimitMin $Pinging $GamesMin $Programs`, `Wait-ProgramsReady`, `Use-Parent { … }`, `Send-Parent "Cmd" @{…}`, `Assert-Reply`, `Find-Events $Pattern`, `Get-Events` (`ListEvents limit 500`), `Wait-Until {…} 20 "что"`, `Assert-That`, `New-TestApp "tool"` (копия exe в `$script:Apps` = WatchDir), `Start-TestApp`, `Assert-ClosedFast`, `Assert-AppAlive`, `Invoke-TestGuard "foreground" "" $Path`, `Get-GuardState`, `Wait-GuardState`, `Restart-PcbService`, `Enable-TestMode` (ключи `TestChildSID`, `WatchDir` в `$script:TestKey`).
- **Детская учётка по умолчанию — учётка раннера** (`$script:MySid`, уже вошла). Второй ребёнок — `New-TestUser` (`net user pcbtest … /add`, возвращает SID; `Invoke-AsTestUser` запускает PCBlock.exe от его имени через `--test-runas`, без интерактивного входа и без Explorer). Образец двух детей — `s22-my-stats.ps1`.
- Тестовые действия службы: `TestSession` (`handlers.go:1242`, только `disconnect`/`logoff`), `TestGuard` (`TestForeground` и др., `ipc/commands.go:783-786`), `TestClock`, `TestSeedDay`, `TestSeedPrograms`. Журнал проверяется через `Find-Events` по тексту + `kind` (S45: `program_auto_closed`), лог службы — `Join-Path (Join-Path $script:Data "logs") "service.log"`.
- **Реестр ребёнка в сценариях уже читается**: `Registry::HKEY_USERS\$($script:MySid)\...` (s01:85, s07:34, s20:23, s26:32-46, smoke:89); очистка в `Reset-Machine` (common.ps1:600-601).

Как расширять: `tools/accept/s50-startup.ps1`: запись notes в `HKU\<MySid>\Software\Microsoft\Windows\CurrentVersion\Run` (exe в WatchDir) после установки и общая в `HKLM\…\Run`; вход моделировать новым `TestSession logon` (сброс `ss.since`) + запуск exe из скрипта; проверить `StartupApproved\Run` (байт 0x03 = выключено, 0x02 = включено), что значение в Run не удалено, `ListStartup` (pending/own/shared), закрытие exe общей записи у ребёнка 1 и `Assert-AppAlive` у pcbtest. Очистку новых ключей добавить в `Reset-Machine`.
Риски: настоящий запуск Run Проводником при входе на раннере не воспроизвести (ребёнок уже вошёл; pcbtest без интерактивного сеанса) — «после включения запускается» проверяется только по StartupApproved; родитель exe в сценарии — PowerShell, не Проводник; правило «60 с после входа» на раннере зависит от `ss.since`.

---

## B1. Экран блокировки стража: промокод, таймеры, картинка

**Чистая логика (Linux-тесты)**: `internal/guard/logic.go`, `layout.go`, `restimage.go`, `restscreen.go`, `controller.go`. Только Windows: `lockscreen_windows.go` (настоящий экран, `lockUI`), `warnings_windows.go` (плашка), `win32_windows.go`, `run_windows.go`, `lockwindow_windows.go` (окно M1-пробы + `NewLockWindow`, перехват клавиш).
```go
// logic.go
ButtonPromo / ButtonSubmitPromo (:66-67); PanelPromo (:85); ActRedeemPromo (:102); Action.Code (:114)
type PromoResult struct { OK bool; Minutes int; Error string }        // :122
func (m *Model) pressPromo(b Button, code string) Action              // :901 пусто → textEnterCode
func (m *Model) OnPromoResult(now time.Time, r PromoResult)           // :811
type View struct { LockShown bool; Lock LockView; Buttons []ButtonView; Panel Panel; ...; PasswordEnabled, FieldPlain bool;
	FieldLabel, PasswordError, PasswordCountdown string; PasswordClearSeq int; Warning *WarningView; Rest *RestView; SuppressKeys bool } // :188
type WarningView struct { ID int; Kind, Text string; Threshold int; Countdown string; Board *systray.CountdownView; Sound bool; Buttons []ButtonView } // :137
plateAdded (:306), addedLife = 5 * time.Second (:19); func (m *Model) plateView(now time.Time, p *plate) *WarningView (:1235)
func SplitPlate(w *WarningView) (board, text *WarningView)            // :1397
```
- Верный код: `OnPromoResult(OK)` → `resetPanels()`, `clearSeq++`, `newPlate(now, plateAdded, 0, r.Minutes)` — экран снимает следующий опрос службы (лимит вырос), плашка «Добавлено N минут» справа внизу 5 с (`ipc.PlateAdded`). Тот же `plateAdded` даёт и родительское «+30» (`OnExtraAdded` :755) — стикер только промокоду требует признака у плашки.
- Неверный: `m.promoErr = r.Error` (текст службы «Код не подходит»), панель и набранный код остаются.
- Ответ службы — `controller.go:710-735` (`ipc.Call(c.call, ipc.CmdRedeemPromo, "", ipc.RedeemPromoRequest{Code: act.Code}, &rep)`; отказ `*ipc.ReplyError` → `PromoResult{Error: re.Text}`; в запретный интервал экран остаётся, `c.setNotice(addedText(...)+". Их можно будет потратить…")`).
- Промокод из окна ребёнка: служба шлёт стражу `noticeGuards(cs, "Добавлено N минут")` (`handlers.go:598`) → `OnPlateNotice` (`ipc.PlateNotice`).
- **Таймеры**: страж — `SetTimer(host, tickTimerID=7, tickEveryMs=250)` (`run_windows.go:26-27, 180`), `tick()` → `ctl.View()` → `applyLock(v)`, `applyPlate(v)` (:291-378); опрос службы — `time.NewTicker(time.Second)` (`pollLoop` :206); изменение модели будит окно `PostMessage(wmAppRefresh)` (:166). Таймер 500 мс — «вернуть окно наверх» (`lockwindow_windows.go:226, 299`). **Анимации нет**; перерисовка частичная: `lockUI.apply` сравнивает `LayoutKey(v, notice)` и при смене только таймера перерыва зовёт `invalidateRect(u.w.hwnd, u.rest.tilesRc)` (`lockscreen_windows.go:103-115`, `restscreen.go:71-90` `LayoutKey`, `TimerChanged`).
- **Котик**: `//go:embed assets/rest-cat.png` (`restimage.go:16`, 19 КБ; исходник `assets/rest-cat.svg`, `tools/rest-cat-png.sh`); `func RestCatBGRA(size int) ([]byte, error)` (:46, усреднение по площади, предумноженная альфа, BGRA сверху вниз); `func OverBackground(bgra []byte, r, g, b byte) []byte` (`restscreen.go:46`, смешивание с фоном экрана); `func RestCatSize(dpi int, areaH, columnH int32) int32` (:35); раскладка столбцом `Flow(area, top, items []FlowItem) []Rect` (`layout.go:47`), `Scaled(px, dpi int) int32` (:15); dpi — `systemDPI()` (`win32_windows.go:136`, GetDpiForSystem); вывод — `blitBGRA(hdc, x, y, w, h int32, px []byte)` (`win32_windows.go:185`, SetDIBitsToDevice, без альфы — поэтому заранее на фон); кэш `lockUI.rest.catPx/catSize` (`prepareCat` :380).
- Плашка `plateWindow` (`warnings_windows.go:51-275`): только текст и кнопки (`NewPlateLayout(dpi, textH, n, hasClose)` в layout.go:89), картинок нет; WS_EX_NOACTIVATE.
- Тесты: `promo_test.go` (`TestLockPromoButton`, `TestPromoPanelFlow`, `TestPromoResult`, `TestControllerRedeemPromo`, `TestControllerPromoInForbiddenSaysWhenUsable`…), `restimage_test.go`, `restscreen_test.go`.

**ТЗ уже обновлено под D-83** (R26 `docs/01_ТЗ.md:284`, 8.2.3 п.3 :408, 8.2.6 п.11 :442, M5b :524, **S52** :620): на экране блокировки при верном коде стикер и надпись удачи показываются **около 3 с, затем экран исчезает** (сейчас экран снимается сразу — `OnPromoResult` + следующий опрос службы); неверный — стикер и надпись над полем; стикер и надпись выбираются отдельно для удачи и неудачи и не повторяют прошлый показ на этом экране; ответ виден, пока ребёнок не начнёт новый ввод; анимации Windows выключены → стикер без движения (SPI_GETCLIENTAREAANIMATION на стороне стража, `prefers-reduced-motion` в окне); прозрачный фон на светлой и тёмной теме.

Как расширять: стикеры как **заранее отрисованные кадры** (PNG/спрайт, встроенные `//go:embed`) — рендерера Lottie в Go нет; в модели: `PromoResult` → поле вида «стикер N, надпись k» (случайный не повторяющий прошлый — чистая функция с источником случайности, как `promoRand`); у `View`/`WarningView` — номер стикера и кадр; анимация — отдельный таймер окна (например 33–50 мс) только пока стикер виден, `invalidateRect` только его прямоугольника (по образцу плиток). Неверный код — стикер на экране блокировки над полем; верный — модель держит экран ~3 с после ответа службы (новое состояние «показ удачи», `LockShown` истинно, хотя решение ядра уже `LockNone`), затем снимает; плашка «Добавлено N минут» после этого — решить, нужна ли она (дублирует надпись).
Риски: удержание экрана после снятия блокировки ядром — новое состояние в `Model` (`screenShown`, `lockActive`, `SuppressKeys`), не сломать LE1/LE2 и S36 (accept `s36-promo.ps1` ждёт «Добавлено 30 минут» и снятие экрана — обновить);  размер бинарника (17 стикеров × ~150–180 кадров × 512² — только уменьшенные кадры/ключевые кадры); CPU/мерцание в окне поверх всех на 250-мс цикле; плашка WS_EX_NOACTIVATE без двойной буферизации; dpi фиксирован на старте; служба по-прежнему отвечает «Код не подходит» и «Добавлено N минут» — надписи D-83 выбирает страж/окно, а `TestPromoResult`, `MyStats.test.tsx:729-756`, accept S36/S49 ждут старые тексты.

---

## B2. Окно ребёнка: PromoRedeem, стили, встраивание

- `frontend/src/components/PromoRedeem.tsx` (74 стр.): `export default function PromoRedeem({ onRedeemed }: Props)`; `redeemPromo(0, code)`; успех — `setMessage(r.message)` (служба: «Добавлено N минут») в `<p className="child-feedback" role="status">`; отказ — `setError(childErrorText(e))` в `<p className="field-error" role="alert">`; пусто — «Введи код». Классы: `.promo-redeem` (`styles.css:3151`), `.child-feedback` (:761), `field-input-error`.
- Стоит в `screens/MyStats.tsx:404-417` `export function Bonuses({ bonus, onChange })` (`<PromoRedeem onRedeemed={onChange} />`). Отдельных тестов компонента нет — проверяется в `screens/MyStats.test.tsx:729-756` («Добавлено 30 минут», «Код не подходит»), e2e `s06 моя статистика ребёнка` (`screens.spec.ts:358`, требует ровно один textbox).
- Тема: `styles.css:3` `:root { --bg, --surface, --line, --text, --text-muted, --primary, --status-ok/warn/danger, --space-*, --radius-* }`, тёмная — `@media (prefers-color-scheme: dark)` (:37). `prefers-reduced-motion: reduce` уже используется (:895, :1839, :2965) — анимации надо гасить там же.
- `vite.config.ts`: `outDir: "dist"`, тесты jsdom; **папки `frontend/public` нет**; `index.html` без CSP-мета. `package.json`: зависимости только react/react-dom — **библиотеки Lottie нет**.
- Встраивание: `main.go:21-22` `//go:embed all:frontend/dist` → `fs.Sub(embedded, "frontend/dist")` → `ui.Run(assets, …)` → `assetserver.Options{Assets: assets}` (`internal/ui/app.go:199`). Любой файл из `dist` (в т.ч. из `public/` или импортированный ассет) уходит в exe.

Как расширять: компонент `components/PromoSticker.tsx` + чистая логика выбора `lib/stickers.ts` (под порог покрытия 70% src/lib); ассеты: либо `lottie-web`/`@lottiefiles/dotlottie-web` (npm работает, но новая зависимость и распаковка gzip `.tgs` — `DecompressionStream("gzip")` в WebView2 есть), либо заранее сконвертированные анимированные WebP/APNG (тот же набор, что и стражу). Различать «неверный код» по `CallError` с полем `code`, а не по тексту.
Риски: e2e-снимки анимации нестабильны — нужен стоп-кадр в моке/`reducedMotion`; `prefers-reduced-motion` — показать первый кадр (`.png` уже лежат рядом); если надписи выбирает окно, служба продолжает слать «Код не подходит», и старые тексты остаются в журнале/тестах.

---

## B3. Стикеры `assets/stickers/`

Подтверждено (149 файлов = 74 пары `.tgs`+`.png` + README.md): все 17 есть, у каждого `.tgs` и `.png` 512×512 RGBA.

| № | tgs, байт | кадров (op при fr=60) | | № | tgs, байт | кадров |
|---|---|---|---|---|---|---|
| 017 | 19857 | 161 | | 023 | 12807 | 158 |
| 033 | 50833 | 156 | | 028 | 33557 | 180 |
| 044 | 20371 | 180 | | 048 | 17023 | 180 |
| 049 | 13494 | 153 | | 069 | 9618 | 147 |
| 050 | 21351 | 117 | | 082 | 29620 | 180 |
| 077 | 20811 | 162 | | 084 | 23056 | 180 |
| 093 | 11889 | 137 | | 095 | 29000 | 180 |
| 096 | 9856 | 180 | | 110 | 11851 | 180 |
| 112 | 21449 | 180 | | | | |

Удачные: 017 033 044 049 050 077 093 096 112; неудачные: 023 028 048 069 082 084 095 110 (README и D-83). Lottie: `w=h=512`, `fr=60`, `ip=0`, `tgs=1`; assets — только прекомпозиции (`comp_*`), растровых вставок нет. Локально есть `ffmpeg`, ImageMagick `convert`, node 22; рендерера Lottie (python-lottie, rlottie) нет — перевод в кадры потребует npm-пакета или CI.

---

## C. Бэклог B-60, B-67, B-68 и проверки CI

**B-60** — см. A3 (кэш `metaDone` по пути).

**B-67** (`plans/BACKLOG.md:28`):
1. Строка журнала службы при подробных логах о первом решении по процессу вне доверенных папок: точка — `Watch` после `watchDecide` (`watch.go:221-226`) или в `watchDecide` при `!e.classified` (:637-639; там же `res`, `e.fp`, `e.pub`); доверенность — `s.classifier.Roots().IsTrusted(e.norm)` (`programs/path.go:103`); писать `s.vlogf(...)` (только при SetDiag verbose). Наблюдатель вне s.mu — `vlogf` должен быть безопасен там (проверить; иначе копить в `notes` и писать в `applyWatch`, как `watch-meta`).
2. Повторное чтение издателя ~2 с при пустом издателе в Temp: `fetchMeta` (:466) кладёт результат в `e.pub/e.pubRead=true`; признак Temp — `Classifier.profileKey` → `Roots.tempRel` (`programs/path.go:305`, не экспортирован; `NeedsPublisher(path string) bool` `classify.go:195`). Повтор: при `m.publisher == "" && tempRel != ""` не ставить `pubRead`, а помнить момент и перечитать через 2 с (решение отложено → «не знаю — не трогать», процесс живёт эти 2 с: решить, допустимо ли).
3. Юнит-тест Temp: `internal/programs/temp_test.go` — `TestClassifyTempFolder` (:31), `TestNeedsPublisherTemp` (:87), `TestTempNotAppRoot` (:110). Accept-сценария установщика в Temp нет; риск: в тестовом режиме наблюдатель закрывает только `WatchDir` (`watch.go:631`), Temp раннера вне её — нужен WatchDir/подпапка, которую `tempRel` признаёт Temp (`<UserProfiles>\<имя>\appdata\local\temp\<папка>\…`), либо особый тестовый корень.

**B-68** — `.github/workflows/windows.yml:6-13` дословно:
```yaml
on:
  pull_request:
    branches: [main]
    paths-ignore: ["docs/**", "plans/**", "**/*.md"]
  push:
    branches: [main]
    paths-ignore: ["docs/**", "plans/**", "**/*.md"]
```
Секции `paths:` нет. Добавить `"assets/stickers/**"` в оба `paths-ignore` — но в M5b стикеры встраиваются (код будет брать их из `assets/stickers/` или из сгенерированных кадров), тогда пропуск вреден для коммитов, меняющих только стикеры. Решить в плане: пропуск только если встраиваются сгенерированные файлы в другой папке.
`ci.yml` без фильтров путей (push в main/feature/**/fix/**/build/**, любой PR).

**Проверки ci.yml** (`.github/workflows/ci.yml`): «Тексты интерфейса без длинных тире» — `python3 tools/scan_ui_texts.py` (:85-88; строковые литералы `.go` кроме тестов и `.ts/.tsx` в `frontend/src` кроме тестов; ищет `—` и `–`); «Без сетевых пакетов (NFR-1)» — `bash tools/check_imports.sh` (net, net/http, net/rpc, net/smtp, crypto/tls для linux и windows); gofmt; go vet/сборка Windows; «Кодовая карта актуальна (Pull Request)» — `bash tools/codemap.sh` + `git diff --quiet -- docs/CODEMAP.md` (:120-128, ошибка «Карта устарела»). Новый пакет/миграция/файл фронта → перегенерировать `docs/CODEMAP.md`. Шаблоны NSIS — `tools/check_nsis_bom.sh`.
