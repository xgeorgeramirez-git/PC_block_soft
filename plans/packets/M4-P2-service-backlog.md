# Пакет: P2 вехи M4 «Бэклог службы» (B-31, B-32, B-36, B-37, B-22, B-47, B-20; B-21, B-23, B-26) | слот: писатель | исполнитель: новый агент

Репозиторий `/home/claude/pc_block_soft`, ветка `feature/m4-sites`. Прочитай `plans/2026-11-04-M4.md`: Global Constraints, строку P2, Review Focus п. 2 и 5; `plans/BACKLOG.md` (строки B-20…B-37, B-47); ТЗ (`docs/01_ТЗ.md`): R2, R12, NFR-6, NFR-10, 3.2. Уроки — раздел «Уроки» ниже. Скиллов нет. В дереве параллельно работают другие писатели: `git add` — только свои пути, никогда `-A`.

Перед работой сверь адреса (PL-4): `git log --oneline -8`, `grep -n "a.killed = s.cfg.Watcher.Kill\|pub, _, e.fp = s.cfg.FileMeta" internal/service/watch.go`, `grep -n "scanMaxEntries\|func (st \*scanState) result" internal/service/winhost/scan.go`, `grep -n "func (h \*host) onSession" -A3 internal/service/winhost/host_windows.go`, `grep -n "func notificationBytes" internal/service/winhost/service_windows.go`, `grep -n "func (a \*app) pollLoop" -A20 internal/guard/run_windows.go`, `grep -n "func sessionLocked\|func (ss \*sessState) resetLockBySample" internal/guard/sensors_windows.go internal/service/sessions.go`.

**Предложение читателя: два захода подряд** (пакет общий, вердикты раздельные):
- **P2a — служба и сканер, всё локально** (шаги A–E): B-31 (закрыть по факту), B-37 + B-32 (закрытие параллельно, свойства после и с бюджетом), B-36 (предел снимка и запись об обрезке), B-23 (признак «рабочий стол не прочитан»), B-22 (очередь событий сеанса).
- **P2b — Windows-обвязка и CI** (шаги F–H): B-20 (опрос стража выходит по стопу), B-21 (копия 8 байт, Ruling), B-47 (ожидание окна по признаку), затем один прогон `windows.yml` (через оркестратора).

## Бриф

Десять пунктов бэклога службы. Решения: B-31 уже исправлен (закрыть с доказательством), B-32, B-36, B-37, B-22, B-23, B-20, B-47 — исправить (TDD), B-21 — Ruling «гарантии нет» + малая безопасная правка, B-26 — Ruling «закрыть без кода». Нестабильные B-20 и B-47 разобраны до причины: увеличения таймаутов нет.

## Найдено читателем (проверено по `efa46d8`)

**B-31 (повтор снимка после сбоя) — уже исправлено.** `internal/service/programs.go:99-105` `ensurePrograms`: повтор не чаще `programsRetryEvery` (10 мин, `:15`) из `Housekeep` (`loop.go:229`), снимок — в своей горутине (`startSeed` `:110-130`, `runSeed` `:134-155`), не синхронно. Тесты уже есть: `programs_test.go:135` `TestSeedProgramsScanError` (повтор через 1 мин — нет, через 10 мин — есть и удаётся), `:355` `TestHousekeepRetryDoesNotBlockTick` (повтор не держит тик), `:244` `TestScanProgramsOutsideServiceLock`. Сделано в `2c3fe55` (31.10.2026), бэклог не обновили.

**B-37 (Kill по очереди) и B-32 (свойства файла задерживают проход).** `internal/service/watch.go:195-205`: в одном цикле по `acts` — `Kill` (`:197`; Windows-адаптер ждёт исчезновения до `killWaitMs = 300` мс, `winhost/procwatch_windows.go:50, :199`), и сразу для закрытой незнакомой — `s.cfg.FileMeta(a.e.norm)` (`:199`, издатель и описание для «Ждут решения»). Итог: 5 процессов в «только учёбе» — до 1,5 с только на ожидании `Kill`; одно чтение свойств с сетевого пути (`\\сервер\папка\game.exe`) держит закрытие всех следующих процессов прохода и следующий проход (`s.watch.mu` занят всё время `Watch`). Второе чтение свойств — до решения, `watch.go:328-334` (отпечаток RM3-12 и издатель RM3-13 — только у программ с решением родителя вне доверенных каталогов и в профиле; один раз на процесс). Фейки: `fakeWatcher.holdKill/releaseKill/killIn` (`fakes_test.go:1672-1685`, `tries` — счётчик вызовов), `fakeMeta.hold/release/in` (`:1425-1470`), `within` (`programs_test.go:230`), образец `watch_test.go:138` `TestWatchKillOutsideServiceLock`.

**B-36 (снимок до 400 записей).** `internal/service/winhost/scan.go:73` `scanMaxEntries = 400`; обрезка в `result` (`:246-293`): группы по порядку «главные exe → пакеты магазина → каталог игр», лишнее молча отбрасывается, `scanStats` (`:141-149`) об обрезке не знает, а `Scan` (`:203-206`) статистику при успехе вообще выбрасывает. Последствие: программа из Program Files, не попавшая в снимок, классифицируется как «незнакомая» (RM3-1 п.5) и закрывается до решения родителя. Шаг мастера режется своим пределом `maxScanPrograms = 300` (`internal/service/handlers_m3.go:33`), то есть 400 мастеру не нужны. Ограничение сверху — кадр канала: `ipc.MaxFrame = 1 << 20` (`internal/ipc/protocol.go:17`), а `ListPrograms` отдаёт весь список без предела (`handlers_m3.go:77-146`). **Пакет `winhost` без файлов `*_windows.go` на Linux собирается и тестируется локально** (проверено: `go test ./internal/service/winhost/` — ok); `host_windows.go` — только CI.

**B-23 (станция ввода не открылась).** Страж: `internal/guard/sensors_windows.go:211-229` `sessionLocked()` — `OpenInputDesktop` не открылся или имя не прочитано → `false` («не заблокирован»); `sample.go:89` `SessionActive: !raw.Locked`. Служба: `sessions.go:181-185` `resetLockBySample` снимает признак блокировки (поставленный событием `SessionLock`) по снимку `SessionActive && LastInputAgoMs < 5000`; вызов — `handlers.go:1081`. При непрочитанном рабочем столе снимок «активен» неотличим от настоящего, и блокировка снимается: время идёт под экраном входа (игра на весь экран засчитывается по R2(б,в) и без ввода). Тесты-образцы: `screenstate_test.go:244` `TestLostUnlockIsRecoveredByGuardSample`, `:260` `TestLockStaysWhileGuardSampleAgreesWithIt`; `guard/sample_test.go:34` `TestBuildSampleFillsFieldsAndKeepsNothingElse`. `internal/guard` и `internal/ipc` на Linux — только стандартная библиотека (проверено), тесты локально.

**B-22 (порядок событий сеанса).** `winhost/host_windows.go:146-148` `onSession`: `go h.svc.OnSession(ev, id, time.Now())` — каждая горутина сама берёт `s.mu`, порядок не гарантирован. Вызов — из цикла `Execute` (`service_windows.go:49-57`), который не должен ждать (диспетчер служб ждёт ответа на «стоп»). `host` (`host_windows.go:18-25`) уже держит `wg` (`:110` `Add(2)`) и `cancel`.

**B-21 (жив ли `EventData`) — разбор исходников golang.org/x/sys v0.48.0** (клон `git clone --depth 1 --branch v0.48.0 https://github.com/golang/sys.git`, коммит `613e2570718e`, 13.07.2026; в модульном кэше песочницы x/sys нет), `windows/svc/service.go`:
- `:201-205` `ctlHandler` (это и есть `HandlerEx`, который вызывает диспетчер служб Windows): копирует `evdata` **как число** в `ctlEvent` и шлёт в небуферизованный `theService.c`, затем `return 0`.
- `:244-276` цикл `serviceMain`: `case r := <-inch` кладёт `EventData` в `outcr` и только **следующим шагом** `case outch <- outcr` отдаёт запрос в наш `Execute`.
- Значит, когда `Execute` (`winhost/service_windows.go:49-51`) читает память по `EventData`, вызов `HandlerEx` этого события **уже вернулся** в диспетчер. Срок жизни `lpEventData` после возврата `HandlerEx` Microsoft не обещает; диспетчер может принять следующее управляющее сообщение в тот же буфер. Окно гонки: два события сеанса подряд (смена пользователя даёт пачку) — `ctlHandler` следующего уже вошёл и ждёт на `theService.c`, пока `Execute` ещё не прочитал предыдущее.
- В трекере Go есть вопрос `golang.org/issue/65832` «x/sys/windows/svc: wrong handling of evdata parameter» (тело из песочницы не прочитано: github.com/golang/go закрыт для этой сессии).
- Сейчас `notificationBytes` (`service_windows.go:67-78`) читает `cbSize` и строит срез длиной `cb` поверх чужой памяти; `sessionIDOf` (`common.go:49-59`) читает из него 8 байт. Это и так первое действие при получении запроса — «скопировать 8 байт сразу» в `Execute` окно гонки **не закрывает** (копия всё равно после возврата `HandlerEx`), только убирает срез неизвестной длины.

**B-26 (неизвестный номер сеанса).** `sessions.go:134-149` `noteSessionState`: при `SessionUnknown` отключение ничего не ставит, подключение не снимает удержание (закреплено `screenstate_test.go:289` `TestUnknownSessionEventsUseFullSync`, `:386` `TestUnknownSessionNumbersAreSafe`). `SessionUnknown` появляется только при `EventData == 0` или `cbSize < 8` (`service_windows.go:51-55`) — для `SERVICE_CONTROL_SESSIONCHANGE` Windows всегда передаёт `WTSSESSION_NOTIFICATION`. Дешёвая правка «отключение с неизвестным номером — удержание всем сеансам» задевает FB-5: `away := ss.screen == ScreenOff` (`sessions.go:436`) снимает обход при выходе родителя рядом (PL-13) — ложное «отключён» у активного сеанса ребёнка открыло бы дыру «убить стража во время чужого события».

**B-20 (`TestRunStartsOneGuardAndStopsOnClose`).** Тест `internal/guard/guard_windows_test.go:267-330`: без службы запускает `Run`, шлёт `WM_CLOSE` окну стража каждые 250 мс 15 с и ждёт возврата `Run`. `WM_CLOSE` не теряется: `DefWindowProc` → `DestroyWindow` → `WM_DESTROY` → `PostQuitMessage` (`run_windows.go:445-446`), цикл `a.loop()` (`:181`) выходит. Дальше `Run` ждёт горутину опроса: `close(stop)` (`:183`), `caller.close()`, `wg.Wait()` (`:185`). **Место потери — `pollLoop` (`run_windows.go:202-221`):** после опроса `select { case <-stop: return; case <-t.C: case <-a.ctl.Kicked(): }` на `time.Ticker` раз в секунду. Без службы один опрос длится около 4 с: `ensureHello` (`controller.go:458`) и `GuardPoll` (`:351`) — каждый `ipc.Dial` ждёт канал `dialTimeout = 2 s` (`run_windows.go:29`; `ipc/client_windows.go:103-135` крутит `CreateFile` до срока при `ERROR_FILE_NOT_FOUND`). За 4 с в канале тикера уже лежит тик, поэтому после `close(stop)` готовы **оба** случая, и Go выбирает **случайно**: с вероятностью 1/2 начинается ещё один опрос на 4 с, потом снова монетка. 15 с исчерпываются при 3–4 лишних опросах: около 5–10 % прогонов — это и есть «изредка» (до B-16 срок был короче — падало чаще; B-16 удлинил срок, а не убрал причину). Подтверждение по коду рядом: у значка тот же тест (`internal/systray/systray_windows_test.go:200-247`) не падает, а его `pollLoop` (`systray/run_windows.go:201-213`) заводит **новый** `time.Timer` после опроса — после опроса готов только `stop`.

**B-47 (окно «PC Block» не появилось за 30 с).** `tools/win-window-screenshot.ps1:40-58` ждёт `Process.MainWindowHandle` с заголовком «PC Block» 30 с. `MainWindowHandle` возвращает только **видимое** окно. Wails v2.17.0 (`go.mod`) создаёт главное окно скрытым и показывает его **только после первой загрузки страницы WebView2** — `v2/internal/frontend/desktop/windows/frontend.go:952-1000` `navigationCompleted` → `win32.ShowWindow(f.mainWindow.Handle())` (прочитано читателем с raw.githubusercontent.com по тегу `v2.17.0`). На медленном раннере (в том прогоне этап 1 снимка окружения шёл 24,9 с) холодный WebView2 может не уложиться в 30 с: окно уже есть, заголовок есть, но оно скрыто → «заголовок пустой». Ждать надо по признакам этапов: окно создано (скрытое, по PID) → WebView2 запущен (дочерние `msedgewebview2.exe`) → окно видимо.

**Среда.** Go 1.27 есть: `~/.local/go1.27/bin/go` (go1.27.3); если нет — `CLAUDE.md`, «Среда облачной песочницы». Windows-vet пакета `guard` локально работает с временным modfile: `git clone --depth 1 --branch v0.48.0 https://github.com/golang/sys.git <tmp>/sys`, `cp go.mod <tmp>/vet.mod && echo 'replace golang.org/x/sys => <tmp>/sys' >> <tmp>/vet.mod`, `GOOS=windows go vet -modfile=<tmp>/vet.mod ./internal/guard/` (проверено читателем: проходит). Для `winhost` Windows-vet локально **не** идёт (тянет `modernc.org/sqlite` и `x/crypto`) — только `ci.yml`. Modfile в репозиторий не коммитить.

## Решения читателя (оркестратор подтверждает до пуска; цена ошибки — в скобках)

- **P2-R1 (B-31) Закрыть по факту без кода.** Писатель только подтверждает мутациями, что тесты ловят (шаг A). (Цена: нет.)
- **P2-R2 (B-37) Kill — параллельно, не больше `killWorkers = 8` одновременно**, порядок записей (`applyWatch`) прежний — по `acts`. Адаптер Windows не трогается (300 мс ожидания остаются: без них следующий `List` покажет умирающий процесс). Граница теста — из устройства пула (PL-33). (Цена: 8 одновременных `OpenProcess`/`TerminateProcess` — нагрузки нет.)
- **P2-R3 (B-32) Свойства незнакомой — после всех Kill, в своих горутинах, с бюджетом `metaBudget = 300 * time.Millisecond` на проход**; не успело — издатель и описание «», в «Ждут решения» — «Без подписи издателя» (решение родителя всё равно перечитывает свойства: `hDecideProgram` → `fileMetaUnlocked`). Одно чтение на путь одновременно (`metaBusy`, свой мьютекс): зависший сетевой путь, запускаемый снова и снова, не плодит горутины. Чтение **до** решения (`watch.go:328-334`, отпечаток и издатель программ с решением родителя) остаётся синхронным — **Ruling: перенос в M5**: без отпечатка решение родителя нельзя применить, а «не знаю — не трогать» дало бы запрещённой программе с медленного пути работать, пока чтение висит. (Цена: проход, в котором впервые видна разрешённая родителем программа с сетевого пути, ждёт её чтение; бывает раз на процесс.)
- **P2-R4 (B-36) Предел снимка 400 → 1000 и запись об обрезке в журнал работы** (`stats.Dropped`, строка через `logf` сканера). 1000 — запас под кадр канала: тест `ListPrograms` на 1000 программах с длинными названиями меряет ответ против `ipc.MaxFrame` (не больше 3/4 кадра); не влезло — эскалация с замером, а не другой предел наугад. (Цена 2000+: «Программы» перестали бы открываться ошибкой кадра на большом компьютере; цена 1000: на очень большом компьютере обрезка остаётся, но видна в журнале.)
- **P2-R5 (B-23) Исправить: новый признак снимка `LockUnknown`.** Страж: `sessionLocked() (locked, known bool)`; `RawSensors.LockUnknown`, `SensorSample.LockUnknown bool \`json:"lockUnknown"\`` (комментарий: «рабочий стол ввода не прочитан, о блокировке страж не знает»). `SessionActive` остаётся `!Locked` (неизвестно — секунда считается, как сейчас: R2 «лучше засчитать»), но служба при `LockUnknown` **не снимает** признак блокировки, поставленный событием. (Цена: потеряно событие разблокировки **и** рабочий стол не читается — время стоит до следующего события сеанса; оба сбоя сразу — редкость; безопасная сторона учёта — не считать.)
- **P2-R6 (B-22) Очередь событий сеанса — FIFO с одним исполнителем**, `push` никогда не ждёт (срез под мьютексом + сигнал в канал на 1), исполнитель вызывает `OnSession` по одному и по порядку с `time.Now()` **в момент доставки** (как сейчас): момент события в `OnSession` не передаётся, потому что `OnSession` ставит `s.real = real` (`sessions.go:94`) и поздняя доставка «старого» момента откатила бы часы службы назад. Остановка — по `ctx` хоста, события после остановки отбрасываются. (Цена: пачка событий доставляется не быстрее, чем служба их разбирает; разбор — микросекунды.)
- **P2-R7 (B-21) Ruling: чтение `EventData` в `Execute` не гарантированно безопасно, но полностью закрыть гонку без своего `HandlerEx` нельзя.** Малая правка — копия ровно 8 байт сразу (`notificationBytes` → `[8]byte` + признак), без среза длиной `cbSize` поверх чужой памяти. Полная правка (форк `svc` со своим `HandlerEx`, ~300 строк, или исправление в x/sys) — вне M4: строка бэклога «B-21: свой HandlerEx или x/sys после issue 65832» → M7. **Цена ошибки (неверный номер при пачке событий):** отключение приписано чужому сеансу — сеанс ребёнка считается до сверки списка (≤ 1 с, PL-8), чужой сеанс 5 с «не на экране»; блокировка приписана чужому — страж ребёнка сам видит экран входа (`SessionActive=false`), время не идёт; выход приписан чужому сеансу — снятие ожидания обхода у чужого сеанса (FB-1) может простить гибель стража в нём, если она совпала с пачкой событий (очень редко). Мусорный `cbSize` даёт `SessionUnknown` и полную сверку.
- **P2-R8 (B-26) Закрыть без кода.** Поведение закреплено двумя тестами; номер не узнать (данных нет), а «удержание всем» открывает дыру PL-13 (см. «Найдено»). `SessionUnknown` на деле не возникает; реальный риск — неверный номер — это B-21. (Цена: при неизвестном номере до 1 с учёта у чужого экрана или до 5 с неучтённого времени — как сейчас.)
- **P2-R9 (B-20) Исправить: стоп сильнее готового тика.** Цикл опроса стража выносится в общий файл `internal/guard/pollloop.go` (`pollEvery`, без тега сборки — тест локально), перед ожиданием — неблокирующая проверка `stop`. Срок теста 15 с не меняется; в сообщение падения — число опросов после закрытия окна и этап `Run` (эксперимент на случай, если гипотеза неверна, PL-15). (Цена «увеличить срок»: падения реже, причина остаётся, страж при конце сеанса держит процесс лишние секунды.)
- **P2-R10 (B-47) Ожидание окна по этапам**: окно процесса создано (`EnumWindows` по PID, скрытое тоже, заголовок «PC Block») ≤ 30 с → окно видимо ≤ 120 с, пока процесс жив; при неудаче — какой этап не наступил, сколько `msedgewebview2.exe` у процесса, сколько секунд прошло (PL-1); при успехе — `::notice::` с длительностью этапов (данные для будущих прогонов). (Цена: падение на «окно не создано» по-прежнему через 30 с — это настоящий дефект.)

## Files

P2a — Modify: `internal/service/watch.go`, `internal/service/watch_test.go` (или новый `watch_parallel_test.go`), `internal/service/fakes_test.go` (счётчики `fakeWatcher.killTries()`, `fakeMeta.entered`), `internal/service/programs_handlers_test.go` (тест кадра `ListPrograms`), `internal/service/winhost/scan.go`, `internal/service/winhost/scan_test.go`, `internal/service/winhost/host_windows.go` (`sc.logf = logf`; очередь сеансов), `internal/service/sessions.go` (`resetLockBySample`), `internal/service/screenstate_test.go`, `internal/guard/sample.go`, `internal/guard/sample_test.go`, `internal/guard/sensors_windows.go` (`sessionLocked`), `internal/ipc/commands.go` (`LockUnknown`), `internal/ipc/contract_test.go`, `internal/ipc/testdata/contract.json` (только `-update`). Create: `internal/service/winhost/sessionq.go`, `internal/service/winhost/sessionq_test.go`.
P2b — Modify: `internal/guard/run_windows.go` (`pollLoop` через `pollEvery`, этапы `Run`), `internal/guard/guard_windows_test.go` (сообщение падения), `internal/service/winhost/service_windows.go` (`notificationBytes`), `tools/win-window-screenshot.ps1`. Create: `internal/guard/pollloop.go`, `internal/guard/pollloop_test.go`.
Оба захода: `docs/CODEMAP.md` — только `bash tools/codemap.sh` (новые файлы меняют счётчики пакетов; на PR CI проверяет актуальность карты).
Не трогать: `internal/core`, `internal/sites`, `internal/storage`, `internal/programs`, `frontend/`, `docs/` (кроме CODEMAP), `plans/`, workflows (кроме вызова скрипта — не нужен), `winhost/procwatch_windows.go` (300 мс остаются).

## Interfaces (новое)

```go
// internal/service/watch.go
const (
	killWorkers = 8                      // Kill одного прохода одновременно (B-37)
	metaBudget  = 300 * time.Millisecond // ожидание свойств закрытых незнакомых за проход (B-32)
)

// internal/service/winhost/scan.go
type scanStats struct { /* … */ Dropped int } // уникальных путей сверх предела; String(): «…, найдено N, не вошло M; …»
type programScanner struct { /* … */ logf func(string, ...any) } // nil — без записи
const scanMaxEntries = 1000
// запись: «Снимок программ обрезан по пределу 1000: не вошло программ 29, они будут ждать решения родителя»

// internal/service/winhost/sessionq.go
type sessionQueue struct { /* mu, items []queuedSession, wake chan struct{} (буфер 1) */ }
type queuedSession struct { ev service.SessionEvent; id uint32 }
func newSessionQueue() *sessionQueue
func (q *sessionQueue) push(ev service.SessionEvent, id uint32)               // не ждёт никогда
func (q *sessionQueue) run(ctx context.Context, deliver func(service.SessionEvent, uint32)) // по одному, по порядку

// internal/ipc/commands.go (SensorSample, после DomainUnread)
LockUnknown bool `json:"lockUnknown"`
// internal/guard/sample.go (RawSensors)
LockUnknown bool // рабочий стол ввода не открылся или имя не прочитано: о блокировке не известно

// internal/guard/pollloop.go
var pollRounds atomic.Int64  // опросов всего (диагностика теста B-20)
var runStage atomic.Int32    // 1 — цикл окон вышел, 2 — опрос остановлен, 3 — окна убраны (диагностика)
func pollEvery(stop <-chan struct{}, tick <-chan time.Time, kick <-chan struct{}, once func())

// internal/service/winhost/service_windows.go
func notificationBytes(eventData uintptr) []byte // ровно 8 байт копией или nil
```

## Шаги P2a (каждый: RED → GREEN → мутации → коммит; всё — локально)

### A. B-31: подтвердить и закрыть

Без кода. Мутации на время прогона (вернуть!): (1) в `ensurePrograms` убрать условие `real.Sub(s.progTryAt) < programsRetryEvery` → `TestSeedProgramsScanError` «повтор раньше срока»; (2) в `startSeed` вместо `go s.runSeed(fl, leader)` вызвать `s.runSeed` синхронно → `TestHousekeepRetryDoesNotBlockTick` (взаимоблокировка или `within` 200 мс). Коммита нет; итог — строка в вердикте «B-31 закрыт по факту: `2c3fe55`, тесты …, мутации …» (бэклог правит оркестратор).

### B. B-37 + B-32: закрытие параллельно, свойства после и с бюджетом

RED `TestWatchKillsInParallel` (`watchHarness`): пять незнакомых в `c:\users\миша\downloads\` (`a.exe` … `e.exe`), `w.holdKill()`, `go h.watch()`; ждать до 2 с, пока `w.killTries() == 5` (новый помощник под `w.mu`), иначе `t.Fatalf("Kill идут по очереди: начато %d из 5", n)`; `releaseKill`, дождаться прохода → `len(w.killedPaths()) == 5`, «Ждут решения» — 5 строк. Второй подтест: 12 процессов при закрытых воротах — дождаться `killTries() == 8`, выждать ещё 200 мс → по-прежнему `8` (девятый `Kill` ждёт свободного места); открыть ворота → все 12 закрыты (PL-33: граница из устройства пула, а не «всё сразу»).
RED `TestWatchKillsBeforeReadingMeta` (`watchHarness(t, w, withMeta(meta))`): `pathSuperGame` и `pathHomework` запущены; `meta.hold()`; `go h.watch()`; `<-meta.in` (до 2 с) → в этот момент `len(w.killedPaths()) == 2` (сейчас 1: чтение свойств первой держит закрытие второй). Сначала проверь, что для этих путей `watchDecide` свойства **не** читает (`NeedsFingerprint`/`NeedsPublisher` → `false`; `meta.asked` пуст после прохода без `hold`); если читает — взять путь вне профиля и вне доверенных (`d:\games\x.exe`, `d:\games\y.exe`), иначе `meta.in` сработает до закрытия.
RED `TestWatchMetaBudget`: один незнакомый, `meta.hold()`; `within(t, time.Second, "проход при зависшем чтении свойств", h.watch)`; «Ждут решения» — строка SuperGame с `Publisher == "Без подписи издателя"`; запустить тот же exe снова (новый PID), `within(… h.watch)` → `meta.entered == 1` (второго чтения того же пути, пока первое висит, нет); `meta.release()`.
Контроль (зелёные до и после): `TestWatchKillOutsideServiceLock`, тесты `launchkind`/FB-18 (`go test -run 'Launch|Auto|FB18'`), `hidden_test.go`, `watch_profile_test.go`.
GREEN: в `Watch` (`watch.go:195-205`) — (1) все `Kill` через пул на `killWorkers` горутин и `sync.WaitGroup`, каждая пишет только свой `acts[i].killed`; (2) затем чтение свойств закрытых незнакомых (`killed && !again && VerdictUnknown`) — горутины, ожидание до `metaBudget` (`time.NewTimer`), результаты из буферизованного канала, опоздавшие отбрасываются; `metaBusy map[string]bool` под своим мьютексом снимается горутиной по завершении; (3) `e.closed = true` — после. Комментарий у констант — B-37/B-32, NFR-6.
Мутации: последовательный цикл `Kill` → `TestWatchKillsInParallel`; чтение свойств в цикле `Kill` → `TestWatchKillsBeforeReadingMeta`; без бюджета → `TestWatchMetaBudget` (`within`); без `metaBusy` → `entered == 2`; пул без предела → подтест 12 (одновременно 12 > 8).
Прогон: `go test -race -count=1 ./internal/service/` + `-run 'Watch' -count=50` (гонки пула).
Коммиты: `perf: закрывать процессы прохода параллельно`, `perf: читать свойства закрытых программ после закрытия и с бюджетом времени`.

### C. B-36: предел снимка и запись об обрезке

RED (`winhost/scan_test.go`, рядом с `:349` `TestScanLimitsEntriesMainsFirstAndSorted`): тот же набор (30 главных + 30 по каталогу), `s.maxEntries = 31`, `s.logf` собирает строки → `_, st, _ := s.scan()`: `st.Dropped == 29`, `strings.Contains(st.String(), "не вошло 29")`; `s.Scan()` → журнал ровно `["Снимок программ обрезан по пределу 31: не вошло программ 29, они будут ждать решения родителя"]`; без обрезки (`maxEntries = 100`) — `Dropped == 0`, строк журнала 0. Повторы одного пути в `Dropped` не считаются (добавь дубль главного exe — `Dropped` прежний). `scan_budget_test.go:22` сверяет `s.maxEntries == scanMaxEntries` — остаётся зелёным.
RED `TestListProgramsFitsFrame` (`programs_handlers_test.go`): 1000 программ в `h.store.programs` — путь `c:\program files\<папка 60 латинских знаков NNN>\<имя 30 знаков>.exe`, `Name` 60 кириллических знаков, `Publisher` 60 кириллических, `weekS` > 0; `ListPrograms` → `json.Marshal(rep)` (ответ целиком, как его пишет канал) `< ipc.MaxFrame*3/4`; в сообщении падения — длина в байтах (PL-32).
GREEN: `scanMaxEntries = 1000`; `Dropped` в `result` (уникальный путь сверх предела); `Scan` при `Dropped > 0 && s.logf != nil` пишет строку; `host_windows.go` после `newWinScanner(...)` — `sc.logf = logf` (подпись `newWinScanner` не менять: её зовёт `procwatch_windows_test.go:515`). Комментарий теста `:349` «400» → «предел».
Мутации: `Dropped` считает и повторы → тест дубля; запись без условия → 0 строк при `maxEntries = 100` не выполняется.
Прогон: `go test -race -count=1 ./internal/service/winhost/ ./internal/service/` — локально; `host_windows.go` — `ci.yml` (`GOOS=windows go vet/build`).
Коммит: `fix: снимок программ до 1000 записей и запись об обрезке`.

### D. B-23: рабочий стол ввода не прочитан

RED `guard/sample_test.go`: `BuildSample(now, RawSensors{IdleMs: 1000, LockUnknown: true}, 1)` → `SessionActive == true`, `LockUnknown == true`; в первом `want` теста `:34` поле по умолчанию `false` (таблица остаётся). RED `screenstate_test.go` `TestLockStaysWhileGuardSampleAgreesWithIt` — новый случай `{"страж не прочитал рабочий стол", ScreenOn, ipc.SensorSample{SessionActive: true, LastInputAgoMs: 1000, LockUnknown: true}}` → `usedS == 0` за 5 с; `TestLostUnlockIsRecoveredByGuardSample` (без признака) — по-прежнему 5. Контракт: `contract_test.go` `examples()` — в примере `GuardPoll` (любом с `SessionActive: true`) `LockUnknown: false` явно не нужен; `go test ./internal/ipc/ -run Contract -update`, `git diff --stat internal/ipc/testdata/contract.json` — новый ключ `lockUnknown` во всех снимках.
GREEN: `sessionLocked() (locked, known bool)` (`known = false` при `h == 0` и при `r == 0` у `GetUserObjectInfoW`), `readRaw` (`sensors_windows.go:64`) ставит `Locked: locked, LockUnknown: !known`; `BuildSample` переносит; `resetLockBySample`: `&& !sm.LockUnknown`.
Мутации: служба не смотрит на признак → новый случай (5 вместо 0); страж ставит `Locked = true` при неизвестном → `SessionActive` в `sample_test` (секунда перестала бы считаться без события блокировки).
Прогон: `go test -race -count=1 ./internal/guard/ ./internal/ipc/ ./internal/service/` — локально; `GOOS=windows go vet -modfile=<tmp>/vet.mod ./internal/guard/` — локально; полный — `ci.yml`.
Коммит: `fix: не снимать блокировку сеанса по снимку, если страж не прочитал рабочий стол`.

### E. B-22: очередь событий сеанса

RED `winhost/sessionq_test.go`: (1) порядок — `deliver` на первом событии ждёт ворота; `push` событий 2, 3, 4 — каждый возвращается не дольше 50 мс (`within`-подобный помощник); ворота открыть → доставлено `[(Lock,2) (Unlock,2) (Disconnect,2) (Connect,3)]` литералом; (2) 1000 событий подряд из одной горутины — доставлены по порядку номеров; (3) `cancel()` → `run` возвращается ≤ 1 с; `push` после остановки не ждёт и не паникует, `deliver` не зовётся. 
GREEN: `sessionq.go` (без тега сборки); `host_windows.go`: поле `sessq`, `h.wg.Add(3)`, горутина `h.sessq.run(ctx, func(ev, id) { h.svc.OnSession(ev, id, time.Now()) })`, `onSession` → `h.sessq.push(ev, id)`.
Мутации: `deliver` в `go` → (1) порядок; `push` в небуферизованный канал → (1) «push не ждёт»; без выхода по `ctx` → (3).
Прогон: `go test -race -count=1 ./internal/service/winhost/` — локально; `host_windows.go` — `ci.yml`; поведение событий — регресс S33, FB1, FB5, S27 в `windows.yml` (шаг H).
Коммит: `fix: события сеанса в службу по порядку через очередь`.

Коммит P2a в конце: `bash tools/codemap.sh` → `docs: обновить карту кода` (если карта изменилась).

## Шаги P2b

### F. B-20: стоп сильнее готового тика (локально + windows.yml)

Шаг 1 (рефакторинг без смены поведения): `pollloop.go` — `pollEvery(stop, tick, kick, once)` с **прежним** `select`; `pollLoop` (`run_windows.go:202`) зовёт `pollEvery(stop, t.C, a.ctl.Kicked(), func() { safely(...) })`; `pollRounds.Add(1)` на каждый `once`. Коммит `refactor: вынести цикл опроса стража в общий файл`.
RED `pollloop_test.go` `TestPollEveryStopWinsOverReadyTick`: 200 повторов; в каждом `tick := make(chan time.Time, 1)`, `kick := make(chan struct{}, 1)`; `once` при первом вызове закрывает `stop` и кладёт тик и пинок; ждать возврата ≤ 1 с; `n == 1`, иначе `t.Fatalf("повтор %d: опросов после стопа %d, ждали 0 (готовый тик выиграл у стопа)", i, n-1)`. С прежним `select` проходит с вероятностью 2⁻²⁰⁰ — красный надёжно. Второй тест `TestPollEveryTicksAndKicks`: без стопа — тик даёт второй опрос, пинок — третий; затем стоп → возврат (ловит мутацию «выйти после первого опроса»).
GREEN: в `pollEvery` после `once()` — `select { case <-stop: return; default: }`, затем прежний `select`.
Диагностика (эксперимент на случай неверной гипотезы, PL-15): `runStage` — 1 после `a.loop()`, 2 после `wg.Wait()`, 3 после `a.shutdown()`; тест `guard_windows_test.go:322-327` в сообщении падения: `"Run не завершился за 15 с после закрытия окна: этап %d, опросов службы после закрытия %d"` (`pollRounds.Load()` — `before`, снятый при первом `WM_CLOSE`), при успехе — `t.Logf("Run вернулся через %v, опросов после закрытия %d", …)`. Как читать будущий сбой: этап 0 — `WM_QUIT` не дошёл (новая гипотеза: цикл окон), этап 1 и опросов ≥ 2 — фикс не сработал, этап 1 и опросов ≤ 1 — завис сам опрос (`Dial`), этап 2 — `shutdown`.
Мутации: убрать неблокирующую проверку → `TestPollEveryStopWinsOverReadyTick`; `return` сразу после `once` → `TestPollEveryTicksAndKicks`.
Прогон: `go test -race -count=1 ./internal/guard/` — локально; `GOOS=windows go vet -modfile=<tmp>/vet.mod ./internal/guard/` — локально; сам `TestRunStartsOneGuardAndStopsOnClose` — `windows.yml` (без `-race`) и R4 (`full-series.yml`, `-race -p 2`) на закрытии вехи.
Коммит: `fix: страж выходит из опроса по стопу, не дожидаясь лишнего опроса`.

### G. B-21: копия 8 байт (ci.yml)

`notificationBytes`: `eventData == 0` → `nil`; иначе прочитать `cbSize` (4 байта) и, если `cb >= 8`, вернуть **копию** первых 8 байт (`b := make([]byte, 8); copy(b, unsafe.Slice((*byte)(p), 8))`); `cb < 8` → `nil`. `sessionIDOf` (`common.go:49`) и его тест (`common_test.go:227`) не меняются: срез длиной 8 при `cb >= 8` проходит его проверку `cb <= len(raw)` **только при `cb == 8`** — поэтому в копии записать `cbSize = 8` (байты 0–3) после копирования, комментарий: «длина структуры проверена здесь, дальше — только номер». Тест — нет (чужая память Windows); проверка — `GOOS=windows go vet`/`go build` в `ci.yml`, события сеанса — S33 и FB5 в `windows.yml`. Комментарий над функцией — Ruling P2-R7 (окно гонки, issue 65832, почему копия не закрывает его полностью).
Коммит: `fix: копировать данные события сеанса сразу и только 8 байт`.

### H. B-47: ожидание окна по этапам (windows.yml)

`tools/win-window-screenshot.ps1`: параметры `-CreateSeconds 30`, `-ShowSeconds 120` (вместо `-WaitSeconds`; вызов в `windows.yml:212` без параметров — менять не нужно). В `Add-Type` — `EnumWindows`, `GetWindowThreadProcessId`, `IsWindowVisible`, `GetWindowText` (образец — `tools/accept/common.ps1:61-90`, но файл не подключать: у него побочные действия). Функция `Find-ProcWindow($procId, $title)` — верхнее окно процесса с точным заголовком, видимое или нет. Этап 1: до `CreateSeconds` ждать окно; нет — `::error::Окно «PC Block» не создано за 30 с: процесс жив, окон у процесса N (заголовки: …)`. Этап 2: до `ShowSeconds` ждать `IsWindowVisible`; нет — `::error::Окно «PC Block» создано за X с, но не показано за 120 с: Wails показывает окно после загрузки страницы в WebView2; процессов msedgewebview2.exe у программы: N` (`Get-CimInstance Win32_Process -Filter "Name='msedgewebview2.exe'"` с `ParentProcessId = $proc.Id`). Успех — `Write-Host "::notice::Окно создано за X с, показано за Y с"`; дальше прежнее (4 с на дорисовку, снимок по найденному `hwnd`, а не `MainWindowHandle`). Сбой процесса — прежняя ветка «завершилась сама» на любом этапе. PowerShell 7 (`pwsh`), вывод кириллицы — как в прежнем скрипте (PL-2).
Проверка: только `windows.yml` (шаг «Окно открывается»), пометка `::notice::` с длительностями в итоге прогона (`gh api …/check-runs/<job>/annotations`). Локального RED нет (pwsh и окна Windows только на раннере) — в вердикте так и написать.
Коммит: `ci: ждать окно программы по этапам, а не 30 с`.

### Прогон P2b (через оркестратора, один `windows.yml`)

`gh workflow run windows.yml --ref feature/m4-sites -f scenario=accept -f only=S33,FB1,FB5,S27,S12,S14,S15` — события сеанса (B-22, B-21, B-23) и наблюдатель (B-32, B-37); в том же прогоне идут Go-тесты Windows (`TestRunStartsOneGuardAndStopsOnClose`) и шаг снимка окна (B-47). Регресс аварийного выхода S3, S7, S25, S26 — в полном `run-all` закрытия вехи.

## Где прогоняется

| Тест | Где |
|---|---|
| `TestWatchKillsInParallel`, `TestWatchKillsBeforeReadingMeta`, `TestWatchMetaBudget`, `TestListProgramsFitsFrame`, `TestLockStaysWhileGuardSampleAgreesWithIt`, мутации B-31 | **локально** (`./internal/service/`) и `ci.yml` |
| `scan_test.go` (B-36), `sessionq_test.go` (B-22) | **локально** (`./internal/service/winhost/`, файлы без `_windows`) и `ci.yml` |
| `sample_test.go`, `pollloop_test.go`, контракт `ipc` | **локально** и `ci.yml` |
| `GOOS=windows go vet ./internal/guard/` | локально (modfile с x/sys) и `ci.yml` |
| `host_windows.go`, `service_windows.go`, `sensors_windows.go` сборка | `ci.yml` (`GOOS=windows go vet/build -tags desktop,production`) |
| `TestRunStartsOneGuardAndStopsOnClose` | `windows.yml` (без `-race`), R4 `full-series.yml` (`-race`) |
| `win-window-screenshot.ps1` | только `windows.yml` |
| S33, FB1, FB5, S27, S12, S14, S15 | `windows.yml` (`scenario=accept`) |

## Среда и команды

```
export PATH=~/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off
go test -race -count=1 ./internal/service/... ./internal/guard/ ./internal/ipc/
go test -race -count=50 -run 'Watch|PollEvery|SessionQueue' ./internal/service/ ./internal/service/winhost/ ./internal/guard/
go vet ./internal/service/... ./internal/guard/ ./internal/ipc/ && gofmt -l internal
# Windows-vet стража (не коммитить modfile):
git clone --depth 1 --branch v0.48.0 https://github.com/golang/sys.git /tmp/p2/sys   # GOPROXY не нужен: обычный git
cp go.mod /tmp/p2/vet.mod && echo 'replace golang.org/x/sys => /tmp/p2/sys' >> /tmp/p2/vet.mod
GOOS=windows go vet -modfile=/tmp/p2/vet.mod ./internal/guard/
bash tools/codemap.sh
```

## Review Focus (самопроверка)

1. NFR-6: проход наблюдателя с пятью закрываемыми и одним медленным путём — не дольше `max(killWait) + metaBudget`; наблюдатель по-прежнему не трогает «не знаю» (PL-9, PL-19), параллельный `Kill` не закрывает чужое.
2. Безопасная сторона учёта (PL-7, PL-8): `LockUnknown` не снимает блокировку, но и не ставит её; очередь не меняет смысла событий, только порядок; `s.real` не откатывается.
3. PL-13: ни одна правка не ставит «отключён» сеансу ребёнка без события с его номером (B-26 не трогается).
4. PL-33: тест пула проверяет гарантию пула (≤ 8 одновременно), а не «всё одновременно».
5. PL-15, PL-1: B-20 и B-47 — причина доказана тестом или чтением кода, падение само говорит, на каком этапе.
6. Гонки: `go test -race -count=50` по новым тестам; горутины свойств пишут только в свой канал, `metaBusy` — под своим мьютексом, не под `s.mu` и не под `watch.mu`.

## Global Constraints (дословно из плана)

- Ядро `internal/core` и новый пакет `internal/sites` — только стандартная библиотека; тесты идут локально (`GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 ./internal/core/... ./internal/sites/...`, Go 1.27 в `~/.local/go1.27`). Правила, цифры и тексты считает ядро и служба; страж, значок, окна — только показывают.
- Тексты ребёнку на «ты», родителю на «вы», без длинных тире (`python3 tools/scan_ui_texts.py`), без технических ошибок. Тексты плашек — дословно ТЗ 8.2.5.
- R23: страж читает адрес вкладки только в памяти и сразу оставляет домен; ни полный адрес, ни заголовок окна не передаются, не пишутся в журнал работы и не хранятся. Хранится только домен.
- NFR-1: ни одного сетевого соединения от программы. Список «18+» встроен в программу при сборке (D-67); загрузка UT1 — только в CI при сборке, никогда в программе.
- NFR-4 установщик ≤ 40 МБ; NFR-5 служба ≤ 50 МБ, страж ≤ 40 МБ памяти; NFR-6 вкладка закрыта ≤ 2 с, программа ≤ 1 с; ST2 проверка домена ≤ 1 мс (99-й процентиль), загрузка списка ≤ 2 с.
- Репозиторий публичный; self-hosted runner заказчика к workflow НЕ привязывать; на ПК заказчика ничего не ставить и блокировки не запускать; приёмка только в CI на windows-latest в тестовом режиме. Запуски `windows.yml` — по одному.
- Аварийный выход (NFR-10) в каждой версии: S3, S7, S25, S26 в регрессе. Новый код не может заблокировать компьютер без выхода и не закрывает окна PC Block, WebView2 и системные процессы (PL-18, PL-19).
- Наблюдатель в тестовом режиме трогает только `WatchDir`; приёмка браузеров — копиями тестового exe с именем браузера в `WatchDir` (RM3-8).
- Предупреждения о видео: пороги 30/15/5/1 (D-54), табло за 30/15/5, плашка 1 мин, звук заказчика — тот же путь, что у игр; порог взводится, только если до события строго больше его минут (PL-14).
- Плашки стража имеют старшинство (PL-34, Ruling FB14-9); новые плашки сайтов — места в RM4-9.
- Миграции — следующий номер 009; ТЗ правится только заменой (D-35); документы — коммит в main, код — ветка и PR.
- Версия 0.4.0 — `internal/version/version.go` и `wails.json` вместе (тест сверяет).
- `go.sum` обновляет CI; GraphQL закрыт — PR, слияние, выпуск через `gh api` (CLAUDE.md). Журналы Actions не скачиваются — пометки сборки (`tools/annotate-errors.sh`).
- Нестабильные: B-20 (`TestRunStartsOneGuardAndStopsOnClose`), B-47 (снимок окна) — перезапускать только упавшее задание.
- Писатель коммитит пути явно (`git add <пути>`, никогда `-A`), не пушит; пушит оркестратор. Трейлеры коммитов: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze`.
- Среда этого чата: npm-реестр из песочницы закрыт, поэтому фронт (`tsc`, `vitest`, `vite build`) проверяется только в `ci.yml` после push (Ruling RM4-0).

**Трейлер этого чата (поручение оркестратора, заменяет ссылку сессии в строке выше):** `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01XuHYMfbECJBCZpbwSnf2sU`.

## Уроки (docs/LESSONS.md)

- **PL-15** (A): перед исправлением — данные, подтверждающие гипотезу: B-20 — тест `pollEvery` (2⁻²⁰⁰) и сравнение со значком; B-47 — исходник Wails; на случай ошибки — диагностика этапов в сообщении падения.
- **PL-7, PL-8** (D): служба знает о блокировке больше стража; снимок, который «не знает», не отменяет событие (B-23); свежее событие сильнее опроса — порядок событий не должен переворачиваться (B-22).
- **PL-13, PL-5** (A): штатный конец и обход — по номеру сеанса; ничего не снимать и не ставить вслепую (B-26, B-21).
- **PL-9** (E): внешний идентификатор читается и проверяется на границе, путь «неизвестно» — безопасный (`notificationBytes`, `SessionUnknown`).
- **PL-17** (E): внешний медленный ресурс (сетевой путь, сканер) — с бюджетом времени и вне критического пути (B-32).
- **PL-33** (E): пул горутин — тест проверяет гарантию пула, мутация «без предела» красная (B-37).
- **PL-16, PL-1** (E): ждать признак, который появляется после события (видимое окно после загрузки WebView2), упавший шаг сам называет этап (B-47).
- **PL-32** (E): сообщения падения — значения (длина кадра, этап, число опросов), не `%+v`.
- **PL-41** (E): новое поле снимка `LockUnknown` проходит через `hGuardPoll` хотя бы в одном тесте службы (случай в `TestLockStaysWhileGuardSampleAgreesWithIt` идёт через `h.run` → опрос).

## Вердикт (≤ 40 строк на заход)

```
TASK P2a|P2b: done | blocked | failed
Diff: sha + строка на коммит
Evidence: go test -race (пакеты и счётчики), -count=50 новых, go vet (в т.ч. GOOS=windows guard), gofmt, codemap, RED-запуски (тест и причина), мутации (какой тест ловит); P2b: прогон windows.yml (id), ::notice:: длительностей окна, итог S33/FB1/FB5/S27/S12/S14/S15
Interfaces: killWorkers, metaBudget, scanMaxEntries 1000, Dropped, LockUnknown, sessionQueue, pollEvery/pollRounds/runStage
Rulings: P2-R1…P2-R10 подтверждены или изменены, своя цена ошибки
Escalations: замер кадра TestListProgramsFitsFrame (если > 3/4), изменения контракта сверх LockUnknown, правки вне Files
Open: B-31, B-26 — закрыть в BACKLOG (оркестратор); B-21 полная правка → M7; чтение отпечатка до решения на сетевом пути (P2-R3) → M5
```

## Решения оркестратора (07.11.2026)

- P2-R1…P2-R10 утверждены. B-31 и B-26 снимаются из бэклога (B-31 — после мутационной проверки существующих тестов); полное решение B-21 (свой обработчик событий сеанса) — новая строка бэклога на M7.
- Два захода: P2a (служба и сканер, локально) → P2b (страж, B-20, B-47, очередь событий сеанса B-22) с одним прогоном `windows.yml` по списку пакета. Один писатель за раз.
- Трейлер сессии: `Claude-Session: https://claude.ai/code/session_01XuHYMfbECJBCZpbwSnf2sU`.
