# Пакет: T7 вехи M4 «Страж» | слот: писатель | исполнитель: новый агент

Репозиторий `/home/claude/pc_block_soft`, ветка `feature/m4-sites`. Прочитай `plans/2026-11-04-M4.md`: Global Constraints, RM4-1 (+итог), RM4-2 (+итог), RM4-9, RM4-12, Review Focus п. 1, 2, 4, 5, 8; строку T7. ТЗ (`docs/01_ТЗ.md`): R6, R9, R23, FR-9, 8.2.1 п. 2, 8.2.2, 8.2.5. Уроки — раздел «Уроки» ниже. В `internal/service` в это время может работать писатель T5: **`internal/service` не трогать**, `git add` — только свои пути.

Перед работой сверь адреса (PL-4): `git log --oneline -10`, `grep -n "func (c \*Controller) PollOnce\|func ClassifyCommands\|func DecisionFromStatus\|domainOf\|func (c \*Controller) PushState" internal/guard/controller.go`, `grep -n "func (m \*Model) moved\|func mostUrgent\|func (m \*Model) stale\|func (m \*Model) eventSecs\|func (m \*Model) shownPlate\|textGamesBoardTitle\|func fallbackWarnText" internal/guard/logic.go`.

## Бриф

Страж шлёт службе домен вкладки Brave и признак «адрес не прочитан» из одного прохода датчиков, исполняет `closeTab` (Ctrl+W → кнопка вкладки UIA → `WM_CLOSE`) и `closeBraveWindow` только после перечитывания переднего плана, показывает плашки сайтов по старшинству и табло/плашку 1 мин о видео со звуком; тестовое действие `domain`; подсказка значка — строки «Игры:» и «Видео:». Решения считает служба (T4a готова), страж только исполняет и показывает.

## Найдено читателем (проверено по `8dfa1f9`)

- **Домен сейчас не уходит в службу вовсе.** `readRaw` (`sensors_windows.go:62`) кладёт домен в `RawSensors.ForegroundDomain`, но `BuildSample` (`sample.go:80`) его не копирует; `run_windows.go:193-202` `readSensors` складывает его в `a.domain` (`:53`), контроллер берёт через `SetDomainReader` (`controller.go:128`, вызов `run_windows.go:172`) и пишет только в `c.lastDomain` (`:327`) для `GuardState`. `req.Sample.ForegroundDomain` всегда «». `DomainUnread` не ставится нигде.
- `braveDomain(fg) string` (`sensors_windows.go:80`) глотает ошибку `braveRd.Read` (`uia_windows.go:652`, возвращает `text, focused, source, err`; `source` — `braveSourceOmnibox`|`braveSourceDocument`, `:551-552`). Ошибки: `errUIANotFound` (нет ни поля адреса, ни `Document`), `errUIABusy`, `errUIATimeout`, `errUIAClosed`, `errUIANoPattern` (`uia_windows.go:41-45`).
- `omniboxDomain` (`omnibox.go:27`) — временная копия; `sites.FromOmnibox(text string) string` (`internal/sites/domain.go:33`) отличается: снимает «www.» повторно (`www.www.a.ru` → `a.ru`) и кодирует IDN в punycode (`encodeHost`). Пакет `sites` — только стандартная библиотека; бинарник один (`main.go:49`), встроенный список «18+» уже в нём. Вызовы `omniboxDomain`: `omnibox.go:112` (`domainMemo.Next`), `brave_probe_windows_test.go:319`, `omnibox_test.go:47-68`.
- `closeActiveTab() error` (`sendkeys_windows.go:53`; `errModifierHeld` `:20`, проверка Shift/Alt/LWin/RWin `:54-58`), `closeTabButton(r *braveReader, hwnd windows.HWND) error` (`uia_windows.go:715`, `errNoCloseButton` `:711`). Глобальные `braveMu`, `braveRd`, `braveDomains` — `sensors_windows.go:71-76`. `wmClose = 0x0010` (`lockwindow_windows.go:61`), `procPostMessage` (`win32_windows.go:23`), `procGetForegroundWin` (`lockwindow_windows.go:33`), `foregroundPath(fg uintptr) string` (`sensors_windows.go:139`, окна PC Block и стража — «»), `isBravePath` (`omnibox.go:85`).
- Контракт готов (T3b, T4a), **ipc не меняется**: `GuardCmdCloseTab`, `GuardCommand.Domain`, `GuardCmdCloseBraveWindow`, `TestDomain`, `CloseTabCtrlW/Invoke/WMClose` (`ipc/commands_m4.go`), `SensorSample.ForegroundDomain/DomainUnread` (`ipc/commands.go:629,633`), `GuardStateReply.ForegroundDomain/ClosedTab/ClosedTabWay` (`ipc/commands.go:851-856`), виды плашек `BlockedSite/BlockedAdult/BlockedVideoOver/BlockedBrowser` (`commands_m4.go`) и `BlockedStudyOnly` (`commands_m3.go:53`), `WarnEventVideo`. Тексты видео-предупреждений строит служба (`ipc/status.go:260` `NewGuardWarningFor`: 30/15 «Через N минут время на видео и соцсети закончится», 5 «Через 5 минут время на видео и соцсети закончится», 1 «Осталась 1 минута на видео и соцсети»).
- Служба (T4a, `internal/service/sites_svc.go:188` `siteGuard`): `closeTab{Domain: <домен вкладки>}` — **в каждом ответе**, пока снимок показывает домен под запретом; плашка `GuardCmdBlocked` — не чаще 5 с на домен (`ShowS: 10`, `Path: ""`); `closeBraveWindow` — в каждом ответе после ≥ 5 с подряд `DomainUnread` при «только учёбе» или исчерпанном видео, без плашки. Плашки сайтов приходят обычным `GuardCmdBlocked` → `ClassifyCommands` (`controller.go:184`) → `Model.OnBlocked` (`logic.go:746`): показ уже общий (текст службы, `ShowS`, крестик `plateClose`, без звука, дубль по `kind+path+text` не продлевает).
- Видео в модели стража **не поддержано**: `DecisionFromStatus` (`controller.go:214-222`) не переносит `VideoRemainingS/VideoLimitS/VideoUnlimited/CloseVideo` (поля есть в `ipc.ChildStatus`, `commands.go:247-250`, и в `core.Decision`, `engine.go:56-59`); `moved` (`logic.go:593`), `stale` (`:637`), `eventSecs` (`:659`) знают только игры, иначе считают по общему остатку — табло видео показало бы общий остаток, а плашка 1 мин не снималась бы при `CloseVideo`. Заголовок игр — `textGamesBoardTitle = "ОСТАЛОСЬ НА ИГРЫ:"` (`logic.go:255`, применяется `:1230`), запасные тексты — `fallbackWarnText` (`:1266`). Звук: `plateView` ставит `Sound = true` любой пороговой плашке (`:1216`), `Controller.playDue` (`controller.go:478`) играет по `SoundDue` — для видео ничего добавлять не надо.
- Старшинство сейчас: `shownPlate` (`logic.go:1037`) показывает из `m.warn` и `m.blocked` **более новую**; тест `TestBlockedPlateAndWarningShareCorner` (`m3_test.go:271`) закрепляет: плашка о программе ложится поверх плашки 1 мин, та возвращается. Отдых: `restPlate` (`rest.go:66`) ждёт только при `m.warn != nil`, `flushRest` (`rest.go:93`) показывает при `m.warn == nil` — плашка программы/сайта её не держит.
- **Значок «Видео: N мин» сам не появится.** Служба кладёт строки в `ChildStatus.Tooltip` (`ipc/status.go:100-107`, `"\nИгры: …"`, `"\nВидео: …"`), но значок свою подсказку строит сам: `systray.DisplayFor` (`view.go:48`) → `TrayTooltip` (`format.go:125`) — только общий остаток; `st.Tooltip` берётся лишь при обходе и безопасном режиме (`view.go:72`). Строки «Игры:» (ТЗ 8.2.1 п. 2) там тоже нет с M3.
- Обвязка тестов: `newCtRig` (`controller_test.go:107`; `read` фиксирован — `RawSensors{IdleMs: 1500}`, подменяй `r.ctl.read`), `ctService` (`:35`; `cmds`, `warns`, `status`, `test`), `r.svc.called(ipc.CmdGuardPoll)` + `.Into(&req)` (`:155-161`), `lastPushed` (`board_test.go:320`), `foregroundCmd(path)` (`m3_test.go:58`) — образец команды `GuardCmdTest`, `TestForegroundOverrideInTestMode` (`m3_test.go:64`), `gamesDecision`/`gamesWarn` (`m3_test.go:512,518`), `runningDecision` (`logic_test.go:56`), `t0`, `at` (`logic_test.go:36,38`), `hms` (`board_test.go:16`). Базовая линия: `go test ./internal/guard/ ./internal/systray/` — ok, 177 тестов в guard.

## Решения читателя (оркестратор подтверждает до пуска; цена ошибки — в скобках)

- **T7-1 Домен и признак — в снимок из одного прохода (PL-15).** `RawSensors` получает `DomainUnread bool`; `BuildSample` копирует `ForegroundDomain` и `DomainUnread` в `SensorSample`. `SetDomainReader`, `c.domainOf`, `a.domain` и чтение в `PollOnce` (`controller.go:319-327`) удаляются; `c.lastDomain = req.Sample.ForegroundDomain` (после подмены `domain`, T7-9). (Цена без этого: служба не видит ни одного сайта — вся M4 молчит; с двумя путями чтения домен и путь могут быть из разных окон.)
- **T7-2 Различие «» и «не прочитан»** — чистая функция в `omnibox.go`: `func (m *domainMemo) Read(hwnd uintptr, text string, focused bool, source string, err error) (domain string, unread bool)`: `err != nil` → `("", true)` и память окна забыта; `source == braveSourceDocument && strings.TrimSpace(text) == ""` → `("", true)` (окно без поля адреса, у документа нет адреса); иначе `(m.Next(...), false)` — служебная страница, поисковая фраза, `file:` дают `("", false)`. Константы `braveSourceOmnibox/Document` переносятся из `uia_windows.go:551-552` в `omnibox.go` (без тега сборки), чтобы функция тестировалась на Linux. `domainMemo.Next` зовёт `sites.FromOmnibox`; `omniboxDomain`, `omniboxNonWeb`, `maxDomainLen` удаляются (`isBravePath`, `isOmniboxClass` остаются). (Цена: ошибка UIA, принятая за «служебную страницу», не даёт ни RK5, ни закрытия окна без адреса в «только учёбе»; пустой документ, принятый за служебную, — то же.)
- **T7-3 Лестница — чистая, в контроллере.** Новый `internal/guard/tabs.go` (без тега): `tabLadder{window uintptr; domain string; step int; at time.Time}`, `tabRetryAfter = 2 * time.Second`, `nextClose(...)` (Interfaces). Шаги по командам подряд для того же окна и домена: 1-я → `ctrlW`; следующая раньше 2 с от прошлого шага — ничего (Ctrl+W ещё мог не дойти; иначе закрылась бы вторая вкладка); ≥ 2 с → `invoke`; ещё ≥ 2 с → `wmClose`, дальше `wmClose` раз в 2 с. Ответ службы без `closeTab` и без `closeBraveWindow` — лестница сбрасывается. Хранится в `Controller.tabs` под `c.mu`; Windows-вызовы — без `c.mu`. (Цена: несколько вкладок одного запрещённого домена подряд неотличимы от «вкладка не закрылась» — третья закроется вместе с окном через `WM_CLOSE`, Brave при следующем запуске восстановит вкладки, и они снова закроются; принимаем, в README не нужно.)
- **T7-4 Перечитывание прямо перед действием (RM4-2, Review Focus 2, 5).** Интерфейс `TabActor.Look()` читает **настоящий** передний план: `GetForegroundWindow`, `foregroundPath`, `isBravePath`, домен и признак через тот же `braveDomain` (общие `braveRd`/`braveDomains` под `braveMu`). Подмена `foreground` (`TestForeground`) к исполнению **не применяется** — Ctrl+W никогда не уходит в окно, которое не настоящий `brave.exe`; подмена `domain` применяется к `Look().Domain`, только если настоящий передний план — Brave (иначе S16…S18 с подменой не исполнили бы ни одной команды). Исполнять `closeTab`, только если `look.Window != 0`, `look.Brave`, `!look.Unread` и `look.Domain == cmd.Domain`; иначе команда отбрасывается молча (окно для лестницы — `look.Window`). (Цена: Ctrl+W в чужом окне закрыл бы вкладку/документ другой программы; WM_CLOSE — окно PC Block или WebView2, PL-18.)
- **T7-5 Модификаторы.** `errModifierHeld` переносится из `sendkeys_windows.go:20` в `tabs.go` (контроллер без Windows должен его узнавать через `errors.Is`). Если `Close(ctrlW)` вернул его — шаг **не засчитан** (лестница прежняя, `ClosedTab` не меняется), следующая команда службы (≤ 1 с) пробует снова. Ошибка `invoke` (`errNoCloseButton`, тайм-аут) и `SendInput` — шаг засчитан (следующий — выше по лестнице), `ClosedTab` не пишется. (Цена: Ctrl+Shift+W закрыл бы всё окно; без засчёта ошибок `invoke` лестница застряла бы на сломанном шаге.)
- **T7-6 `closeBraveWindow`.** Исполняется, только если `look.Brave && look.Unread && look.Window != 0`, `PostMessage(WM_CLOSE)` (не `SendMessage`: зависший Brave не держит опрос), не чаще раза в `tabRetryAfter` для того же окна; запись `ClosedTab = ""`, `ClosedTabWay = "wmClose"`. (Цена: окно, у которого адрес уже прочитался, закрылось бы зря.)
- **T7-7 Место исполнения — конец `PollOnce`**, после модели и `c.wake()` (плашка видна сразу), до `return`: горутина опроса потом зовёт `PushState`, и `ClosedTab` попадает в тот же кадр (PL-15). Без `TabActor` (Linux, тесты без подделки) — ничего. Вызовы `Look`/`Close` обёрнуты в `safely`. Исполняется и вне тестового режима; в тестовом — дополнительно `c.closedTab/closedWay` для `GuardState`. (Цена: исполнение в потоке окон подвесило бы кадры на UIA до 2 с.)
- **T7-8 Старшинство плашек сайтов (RM4-9, PL-34, Review Focus 4).** `isSitePlate(kind) bool` — `BlockedSite`, `BlockedAdult`, `BlockedVideoOver`, `BlockedBrowser`. (а) В `shownPlate`: плашка сайта не ложится поверх видимой плашки 1 мин (`m.warn.kind == plateThreshold && threshold == 1` и `plateVisible`) — показывается та; плашка сайта видна потом, если не вышел её срок (10 с от прихода). (б) Отдых ниже: `restPlate` ждёт (`restWait`) и при видимой плашке сайта; `flushRest` показывает ждущую, только когда нет ни `m.warn`, ни видимой плашки сайта. Плашки о программах (`BlockedForbidden`, `BlockedUnknown`, `BlockedStudyOnly`, …) — **без изменений** (`TestBlockedPlateAndWarningShareCorner` зелёный без правки). (Цена: последнюю минуту ребёнок не видит текста «Сайт закрыт», если плашка 1 мин держится дольше 10 с — вкладка всё равно закрыта, журнал записан; без правила повтор плашки сайта раз в 5 с мог бы прятать кнопки «Попросить ещё» почти всю последнюю минуту.)
- **T7-9 Тестовое действие `domain` (RM4-12, PL-41)** — в контроллере по образцу `foreground` (`controller.go:383`): `c.domSet, c.domText = t.Text != "", t.Text`; в снимок: если `c.domSet && c.testMode && isBravePath(req.Sample.ForegroundPath)` (путь — после подмены `foreground`) → `ForegroundDomain = c.domText`, `DomainUnread = false`. В очередь окна (`c.tests`) не идёт. Текст не нормализуется (служба отвергнет неканонический — так приёмка видит то же, что в бою). (Цена: подмена без Brave на переднем плане дала бы службе домен у не-Brave — служба его игнорирует, но приёмка S18 с подменой `foreground` на Brave должна работать — поэтому проверка по подменённому пути.)
- **T7-10 Видео в модели — по образцу игр.** `DecisionFromStatus` переносит четыре поля видео; `moved`: `WarnEventVideo` → `VideoUnlimited || VideoRemainingS > thr*60`; `stale`: видео → `moved || CloseVideo || StudyOnly`; `eventSecs`: видео → `VideoRemainingS`; `mostUrgent` при равных порогах и секундах: общий (и запретные часы, пауза) → игры → видео (ранг `eventRank`). Заголовок табло — **`textVideoBoardTitle = "ОСТАЛОСЬ НА ВИДЕО:"`** (плитки считают остаток видео, не общий; «ВРЕМЯ ВЫШЛО» — как у игр, при `StatusOut`). Запасные тексты: `textVideo30/15` «Через 30/15 минут время на видео и соцсети закончится», `textVideo5` «Через 5 минут время на видео и соцсети закончится», `textVideo1` «Осталась 1 минута на видео и соцсети». **Эскалация:** в плане UX-сценарий пишет «табло «ОСТАЛОСЬ ВРЕМЕНИ:» с «Через 5 минут время на видео…»» — оркестратор выбирает; при «ОСТАЛОСЬ ВРЕМЕНИ:» убрать только константу заголовка. (Цена «ОСТАЛОСЬ ВРЕМЕНИ:»: ребёнок примет остаток видео за общий.)
- **T7-11 Подсказка значка.** `DisplayFor` (`view.go:48`) в состояниях `StatusOK`/`StatusLow` без блокировки, без «без лимита» и без паузы дописывает к `TrayTooltip` строки `"\nИгры: " + core.FormatDuration(ceilMin(st.GamesRemainingS)*60)` (если `!st.GamesUnlimited && (st.GamesLimitS > 0 || st.CloseGames)`) и `"\nВидео: " + …VideoRemainingS` (то же по видео) — помощники `hasGames(st)`, `hasVideo(st)` в `view.go` (правило `ipc.hasGamesLimit/hasVideoLimit`, `status.go:153`, там неэкспортировано; не экспортировать — ipc не трогаем). Строку игр закрывает пропуск M3 (ТЗ 8.2.1 п. 2) — сообщить в вердикте. (Цена: без правки приёмка «Значок … показывает «Видео: 20 мин»» (UX-сценарий плана) провалится.)

## Files

Create: `internal/guard/tabs.go` (TabLook, TabActor, tabLadder, nextClose, errModifierHeld, tabRetryAfter, isSitePlate), `internal/guard/tabs_test.go`, `internal/guard/tabs_windows.go` (winTabs), `internal/guard/video_test.go` (видео и старшинство плашек сайтов).
Modify: `internal/guard/omnibox.go`, `omnibox_test.go`, `sample.go`, `sample_test.go`, `sensors_windows.go`, `sendkeys_windows.go` (убрать `errModifierHeld`), `uia_windows.go` (убрать две константы источника), `controller.go`, `logic.go`, `rest.go`, `run_windows.go`, `board_test.go` (`TestPushStateCarriesForegroundDomain`), `controller_test.go` (`TestDecisionFromStatusKeepsEverythingTheScreenNeeds`), `brave_probe_windows_test.go`, `internal/systray/view.go`, `internal/systray/view_test.go`, `docs/CODEMAP.md` (только `bash tools/codemap.sh`).
Не трогать: `internal/service` (там T5), `internal/ipc` (контракт полон; нужна правка — эскалация), `internal/core`, `internal/sites`, `internal/storage`, `frontend/`, `docs/` (кроме CODEMAP), `plans/`, workflows.

## Interfaces (новое; сигнатуры — писателю как цель)

```go
// tabs.go
type TabLook struct {
	Window uintptr // окно переднего плана; 0 — нет
	Brave  bool    // настоящий путь окна — brave.exe (isBravePath), не подмена
	Domain string  // домен активной вкладки (после подмены domain в тестовом режиме)
	Unread bool    // адрес не прочитан (T7-2)
}
type TabActor interface {
	Look() TabLook
	Close(way string, window uintptr) error // way — ipc.CloseTabCtrlW | CloseTabInvoke | CloseTabWMClose
}
var errModifierHeld = errors.New("зажата клавиша-модификатор")
const tabRetryAfter = 2 * time.Second
type tabLadder struct { window uintptr; domain string; step int; at time.Time }
// nextClose — что сделать с командой: domain — closeTab.Domain («» и closeWin=false — ничего),
// closeWin — команда closeBraveWindow. way «» — ничего; next — лестница после шага (контроллер
// сохраняет её, если Close не вернул errModifierHeld).
func nextClose(l tabLadder, domain string, closeWin bool, look TabLook, now time.Time) (way string, next tabLadder)
func isSitePlate(kind string) bool

// controller.go
type Commands struct { /* как было */ CloseTab string /* домен последней closeTab; «» — нет */; CloseWindow bool }
func (c *Controller) SetTabActor(a TabActor) // вызвать до первого PollOnce; run_windows: a.ctl.SetTabActor(winTabs{})
// поля Controller: tabActor TabActor; tabs tabLadder; domSet bool; domText string; closedTab, closedWay string
// PushState: st.ClosedTab, st.ClosedTabWay = c.closedTab, c.closedWay

// omnibox.go
func (m *domainMemo) Read(hwnd uintptr, text string, focused bool, source string, err error) (domain string, unread bool)

// sensors_windows.go
func braveDomain(fg windows.HWND) (domain string, unread bool) // braveDomains.Read(...)
// readRaw: raw.ForegroundDomain, raw.DomainUnread = braveDomain(...)

// tabs_windows.go
type winTabs struct{} // Look: настоящий передний план; Close: ctrlW → closeActiveTab(); invoke → closeTabButton(braveRd, hwnd) под braveMu (braveRd nil → newBraveReader()); wmClose → procPostMessage(window, wmClose, 0, 0), ошибка — если вернул 0
```

`domSet/domText` нужны `winTabs.Look` → контроллер подставляет подмену сам: `look := actor.Look(); if c.domSet && c.testMode && look.Brave { look.Domain, look.Unread = c.domText, false }` (чтение полей под `c.mu`).

## Шаги (каждый: RED → GREEN → мутации → коммит)

### A. Домен и признак в снимке (T7-1, T7-2)

`omnibox.go`: `domainMemo.Read`, `Next` на `sites.FromOmnibox`, удалить `omniboxDomain`/`omniboxNonWeb`/`maxDomainLen`, константы источника. `sample.go`: поле `DomainUnread`, комментарий `ForegroundDomain` без «не идёт до T3»; `BuildSample` копирует оба. `sensors_windows.go`: `braveDomain` → `(string, bool)`, `readRaw`. `run_windows.go`: убрать `a.domain` (`:52-53`), `SetDomainReader` (`:172`), `a.domain.Store` (`:201`, комментарий `:193-194` — «домен в снимке»). `controller.go`: убрать `domainOf`/`SetDomainReader`/чтение; `c.lastDomain = req.Sample.ForegroundDomain`. `brave_probe_windows_test.go:319`: `sites.FromOmnibox(text)`.

RED:
- `TestDomainMemoRead` (`omnibox_test.go`), шаги одного `domainMemo`: `(1, "https://www.youtube.com/watch?v=1", false, "omnibox", nil)` → `("youtube.com", false)`; `(1, "пример.рф/путь", false, "omnibox", nil)` → `("xn--e1afmkfd.xn--p1ai", false)`; `(1, "www.www.a.ru", false, "omnibox", nil)` → `("a.ru", false)`; `(1, "brave://newtab", false, "omnibox", nil)` → `("", false)`; `(1, "котики видео", false, "omnibox", nil)` → `("", false)`; `(2, "", false, "", errUIANotFoundStub)` → `("", true)` (в тесте `errors.New("UIA: элемент не найден")`); `(3, "", false, "document", nil)` → `("", true)`; `(3, "https://app.example/x", false, "document", nil)` → `("app.example", false)`; `(1, "kotiki.ru", true, "omnibox", nil)` → `("a.ru", false)` (фокус — прежний). `TestDomainMemo` — прежняя таблица зелёная. Удалить тесты `omniboxDomain` (`omnibox_test.go:47-68`; их классы покрывает `internal/sites`).
- `TestBuildSampleFillsFieldsAndKeepsNothingElse` (`sample_test.go:34`): `RawSensors{…, ForegroundPath: brave, ForegroundDomain: "youtube.com", DomainUnread: true}` → `want` с `ForegroundDomain: "youtube.com", DomainUnread: true` (литерал `brave = \`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe\``).
- `TestPushStateCarriesForegroundDomain` (`board_test.go:417`) переписать: `r.ctl.read` отдаёт по очереди снимки с `ForegroundDomain` `"probe-site.test"` и `""` → в **запросе** `GuardPoll` (`r.svc.called(ipc.CmdGuardPoll)[i].Into(&req)`) `req.Sample.ForegroundDomain` = `"probe-site.test"`, затем `""`, и `lastPushed(t, r).ForegroundDomain` — то же.

Мутации A: `BuildSample` без `ForegroundDomain` → новый RED запроса; `Read` без ветки `document` → шаг 3; `Next` на старом `omniboxDomain` (временно вернуть) → punycode-шаг; `unread` при `err` = false → шаг 2.

Коммит: `feat: домен вкладки Brave и признак непрочитанного адреса в снимке стража`.

### B. Лестница закрытия (T7-3, T7-5, T7-6)

`tabs.go` (Interfaces). RED `TestNextClose` (`tabs_test.go`), таблица по шагам одной лестницы, `look := TabLook{Window: 7, Brave: true, Domain: "example.com"}`:
| шаг | вход | ждём `way`, `next.step` |
|---|---|---|
| 1 | `"example.com"`, `t0` | `"ctrlW"`, 1 |
| 2 | то же, `at(time.Second)` | `""`, 1 |
| 3 | `at(2*time.Second)` | `"invoke"`, 2 |
| 4 | `at(3*time.Second)` | `""`, 2 |
| 5 | `at(4*time.Second)` | `"wmClose"`, 3 |
| 6 | `at(6*time.Second)` | `"wmClose"`, 3 |
| 7 | `look.Window = 9`, `at(6500*time.Millisecond)` | `"ctrlW"`, 1 |
Отдельные случаи с пустой лестницей: `look.Domain = "school.example"` → `""`; `look.Brave = false` → `""`; `look.Unread = true` → `""`; `look.Window = 0` → `""`; домен команды `""` → `""`. Окно: `TabLook{7, true, "", true}`, `closeWin` → `"wmClose"`; через 1 с → `""`; через 2 с → `"wmClose"`; `Unread: false` → `""`; `Brave: false` → `""`.

Мутации B: `<` вместо `>=` у `tabRetryAfter` (шаг 3); без сравнения окна (шаг 7 дал бы `wmClose`); без проверки `look.Brave` для `closeWin`.

Коммит: `feat: лестница закрытия вкладки Brave в страже`.

### C. Исполнение в контроллере и `domain` (T7-4, T7-7, T7-9)

`ClassifyCommands`: `GuardCmdCloseTab` с непустым `Domain` → `CloseTab`; `GuardCmdCloseBraveWindow` → `CloseWindow`. `PollOnce`: подмена `domain` в снимке; в конце — `c.runCloses(cmds, now)`. `tabs_windows.go` — `winTabs`; `run_windows.go` — `SetTabActor(winTabs{})`. Подделка в тесте: `fakeTabs{look TabLook; errs []error; calls []string}` (`calls` — `"<way> <window>"`), `Close` берёт ошибку из `errs` по очереди.

RED (`tabs_test.go`, `newCtRig`, `r.svc.test = true`, `r.svc.status = ctRunningStatus(3600)`):
- `TestCloseTabRunsLadderThroughController`: `fake.look = {7, true, "example.com", false}`; 5 опросов с шагом 1 с, перед каждым `r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdCloseTab, Domain: "example.com"}}`, после каждого `r.ctl.PushState()` → `fake.calls == ["ctrlW 7", "invoke 7", "wmClose 7"]`; `lastPushed`: `ClosedTab "example.com"`, `ClosedTabWay "wmClose"`; после 1-го опроса — `"ctrlW"`. Опрос без команды, затем снова команда → следующий вызов `"ctrlW 7"` (сброс).
- `TestCloseTabWaitsForModifiers`: `fake.errs = [errModifierHeld]`; два опроса с командой через 1 с → `calls == ["ctrlW 7", "ctrlW 7"]`; после первого `ClosedTab == ""`.
- `TestCloseTabRechecksForeground` (Review Focus 2): `look = {9, false, "", false}` (передний план сменился на не-Brave) и `{7, true, "school.example", false}` (вкладка другая) → `calls` пусто; команда `closeTab` без домена → пусто; `look = {7, true, "example.com", true}` → пусто.
- `TestCloseBraveWindowCommand`: `look = {7, true, "", true}`, команда `{Kind: ipc.GuardCmdCloseBraveWindow}` → `["wmClose 7"]`, `ClosedTab ""`, `ClosedTabWay "wmClose"`; `look.Unread = false` → без вызова.
- `TestCloseTabOutsideTestMode`: `r.svc.test = false` → `["ctrlW 7"]` (исполняется в бою), `GuardState` не отправлен.
- `TestCloseWithoutActor`: без `SetTabActor` команда не роняет опрос (`res.Err == nil`).
- `TestDomainOverrideInTestMode` (по образцу `m3_test.go:64`): `r.ctl.read` отдаёт `ForegroundPath: brave`, `ForegroundDomain: "real.example", DomainUnread: true`; команда `{Kind: GuardCmdTest, Test: &{Action: ipc.TestDomain, Text: "youtube.com"}}` → следующий запрос `Sample.ForegroundDomain == "youtube.com"`, `DomainUnread == false`, `lastPushed().ForegroundDomain == "youtube.com"`, `r.ctl.QueuedTests()` пусто; с `foregroundCmd(\`c:\x\chrome.exe\`)` и настоящим путём `chrome.exe` → `"real.example"` не подменяется (путь не Brave); `{Action: TestDomain}` → снова `"real.example"`; вне тестового режима — без подмены. Исполнение с подменой: `fake.look = {7, true, "real.example", false}`, подмена `youtube.com`, команда `closeTab{youtube.com}` → `["ctrlW 7"]`; `fake.look.Brave = false` → пусто.

Мутации C: исполнение с подменой `foreground` (брать путь снимка) → `TestDomainOverrideInTestMode` последний случай; без сброса лестницы → повтор после паузы даёт `invoke`; засчитывать `errModifierHeld` → второй вызов `invoke`.

Коммит: `feat: исполнять закрытие вкладки и окна Brave после перечитывания`, `feat: подмена домена вкладки в тестовом режиме стража`.

### D. Видео-предупреждения (T7-10)

`controller.go` `DecisionFromStatus`, `logic.go` (`moved`, `stale`, `eventSecs`, `mostUrgent`, `plateView` заголовок, `fallbackWarnText`, константы). Помощники теста (`video_test.go`): `videoDecision(remainingS, videoS int) core.Decision` (`runningDecision`, `VideoRemainingS = videoS`, `VideoLimitS = 1200`), `videoWarn(thr, eventS int) Incoming` (из `ipc.NewGuardWarningFor(thr, core.WarnVideo, eventS)`).

RED `TestVideoBoardCountsVideo`: `OnWarnings(t0, videoDecision(5000, 300), false, {videoWarn(5, 300)})` → `Kind "board"`, `hms "00 : 05 : 00"`, `Sound`, `Board.Title "ОСТАЛОСЬ НА ВИДЕО:"`, `Text "Через 5 минут время на видео и соцсети закончится"`; `at(2s)`, `videoDecision(4998, 298)` → `"00 : 04 : 58"`. Плашка 1 мин: `videoWarn(1, 60)`, через 1 с `videoDecision(4999, 59)` → `Text "Осталась 1 минута на видео и соцсети: 0:59"`, `Kind "oneMinute"`, кнопки `[plateAskMore]`; `CloseVideo = true` при `VideoRemainingS 0` → `View().Warning == nil`; затем `OnBlocked(at(62s), {Kind: "videoOver", Text: "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 40 мин", ShowS: 10})` → видна, `Kind "videoOver"`, кнопки `[plateClose]`, `Sound == false`. Запасной текст: `{Threshold: 15, Event: "video", EventS: 900}` → `"Через 15 минут время на видео и соцсети закончится"`.
RED `TestVideoWarningRearmsAndSounds` (контроллер, как конец `TestGamesAndLimitInOnePoll`, `m3_test.go:596-621`): статус `ipc.ChildStatusFrom("Миша", videoDecision(5000, 300))` + `NewGuardWarningFor(5, core.WarnVideo, 300)` дважды → `Warnings` 1, `Sounds` 1; доп. время `videoDecision(6800, 2100)`, затем снова 300 и порог → `Warnings` 2.
RED `TestDecisionFromStatusKeepsEverythingTheScreenNeeds`: случай `"видео"` — `videoDecision(900, 300)` с `CloseVideo: false`, и `"видео закрыто"` — `VideoRemainingS 0, CloseVideo true`; сравнить четыре поля видео.
RED `TestMostUrgentRank`: поровну 5 и 300 с `{gamesWarn(5,300), videoWarn(5,300)}` → игры; `{videoWarn(5,300), limit5}` → общий; `{videoWarn(5,290), gamesWarn(5,300)}` → видео (меньше секунд).

Мутации D: `eventSecs` без ветки видео → `"00 : 05 : 00"` станет `"01 : 23 : 20"`; `stale` без `CloseVideo` → плашка 1 мин висит; `moved` без видео → `Warnings` останется 1; заголовок игр для видео → заголовок.

Коммит: `feat: табло и плашка 1 мин о видео в страже`.

### E. Старшинство плашек сайтов (T7-8)

`tabs.go` `isSitePlate`, `logic.go` `shownPlate`, `rest.go` `restPlate`/`flushRest` (помощник `func (m *Model) sitePlateShown(now time.Time) bool` — `m.blocked != nil && isSitePlate(m.blocked.blockedKind) && !m.blocked.expired(now)`).

RED `TestSitePlateWaitsBehindOneMinute` (`video_test.go`): `OnDecision(t0, runningDecision(60), false, []int{1})`, `one := View(t0).Warning`; `OnBlocked(at(1s), {Kind: "site", Text: "Сайт example.com закрыт: он в списке запрещённых", ShowS: 10})` → `View(at(1s)).Warning.ID == one.ID`, `Kind "oneMinute"`, кнопки содержат `plateAskMore`; `OnExtraAdded(at(3s), 30)` и `OnDecision(at(3s), runningDecision(1860), false, nil)` → `Kind "added"`; `at(9s)` (после 5 с «Добавлено») → `Kind "site"`, текст тот же; `at(12s)` → `nil`. Контроль: `{Kind: "forbidden", Text: "Программа «X» запрещена родителем", ShowS: 3, Path: \`c:\x.exe\`}` поверх 1 мин по-прежнему сверху (`TestBlockedPlateAndWarningShareCorner` без правки).
RED `TestRestPlateWaitsBehindSitePlate` (`rest_test.go` или `video_test.go`, по образцу `rest_test.go:220`): плашка сайта (`ShowS 10`) в `t0`, в `at(1s)` `OnWarnings` с плашкой отдыха (`Event: ipc.WarnEventRest`, `Threshold 5`, `EventS 300`) → видна плашка сайта; `at(11s)` следующий `OnWarnings(nil)` → `Kind "rest"`. Обратный порядок (отдых, потом сайт) — сайт сверху (новее), отдых вернётся, если не вышел его срок.

Мутации E: без условия в `shownPlate` → 1-й RED; без условия в `flushRest` → отдых сразу в `at(1s)`; `isSitePlate` с `BlockedForbidden` → тест-контроль M3 краснеет.

Коммит: `feat: плашки сайтов уступают плашке 1 мин, отдых ждёт плашку сайта`.

### F. Подсказка значка (T7-11)

RED `TestDisplayForLimitLines` (`internal/systray/view_test.go`): `d := core.Decision{Status: core.StatusOK, RemainingS: 85*60, LimitS: 7200, GamesRemainingS: 1200, GamesLimitS: 3600, VideoRemainingS: 600, VideoLimitS: 1200}` → `Tooltip == "Осталось 1 ч 25 мин из 2 ч\nИгры: 20 мин\nВидео: 10 мин"`; только видео (`GamesUnlimited: true`) → `"Осталось 1 ч 25 мин из 2 ч\nВидео: 10 мин"`; видео исчерпано (`VideoRemainingS 0, CloseVideo true`) → `"…\nВидео: 0 мин"`; без лимитов игр и видео (`GamesUnlimited, VideoUnlimited`) → без строк; «время вышло» с лимитом видео → `core.ReasonTimeUp` без строк; `StatusUnlimited` → `"Сегодня без ограничений по времени"`; длина каждой ≤ 127 UTF-16 (как `format_test.go:237`). `TestDisplayFor` и `TestDisplayForAgreesWithServiceEverySecond` — без правки.

Мутация F: строка видео по `GamesRemainingS` → литерал; без проверки «без лимита» → лишняя строка.

Коммит: `feat: строки игр и видео в подсказке значка`.

### G. Проба на Windows (R3) — расширить `TestBraveProbe`

`brave_probe_windows_test.go`, после шага (г): шаг **(ж) «closeTab через контроллер»**: вторая вкладка `probeTwo` (как в (г)), `forceForeground(hwnd)`; `Controller` с настоящим `winTabs{}`, `read = ReadSensors` и подделкой службы (`ipc.Caller`), которая отвечает `closeTab{probeTwo}`, **только если** в пришедшем снимке `ForegroundDomain == probeTwo` (как служба); `PollOnce` раз в 1 с до 6 раз. Факт пометкой: домен в снимке до/после, `ClosedTabWay` (через `PushState` подделки) и время до снимка с `probeSite` (NFR-6 ≤ 2 с); `DomainUnread` первого снимка свежего окна (холодное дерево, RM4-1-итог 3,5 с). Тест не падает по фактам (проба), падает только на панике. Прогон — оркестратор: `gh workflow run windows.yml --ref feature/m4-sites -f scenario=probe` (один запуск).

Коммит: `test: проба закрытия вкладки через контроллер стража`.

## Среда и команды прогона

```
export PATH=~/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off
go test -race -count=1 ./internal/guard/ ./internal/systray/
go test -race -count=1 ./internal/ipc/... ./internal/sites/...
go vet ./internal/guard/ ./internal/systray/
SP=/tmp/claude-0/-home-claude/25f23a37-6a15-5cbe-89db-2e036117eed9/scratchpad
GOOS=windows go vet -modfile=$SP/vet5.mod -tags desktop,production ./internal/guard/ ./internal/systray/
gofmt -l internal
python3 tools/scan_ui_texts.py
bash tools/codemap.sh   # docs/CODEMAP.md — вместе с последним коммитом кода
```
`vet5.mod` — временный modfile в песочнице с `replace golang.org/x/sys => $SP/xsys` (копия v0.48.0), `x/crypto` и заглушкой sqlite; проверен читателем (ok). Не коммитить, в дерево не копировать. Windows-тесты (`*_windows_test.go`) идут только в `windows.yml`.

## Review Focus (самопроверка)

1. R23: текст поля адреса не выходит из `braveDomain`/`domainMemo.Read`; в `GuardState` и снимке — только домен; в журнал работы стража адрес не пишется.
2. PL-9, PL-18: «не прочитано» → ничего не закрывать вкладкой; Ctrl+W и `WM_CLOSE` — только окну с настоящим путём `brave.exe` на переднем плане в момент действия; окна PC Block дают путь «» (`foregroundPath`).
3. PL-15: путь, домен и признак — один `readRaw`; `ClosedTab` и вид плашки — в одном кадре `PushState`.
4. PL-14, PL-34: видео-порог один раз на событие (`duplicate`, `moved`), звук один раз на плашку; плашка 1 мин сильнее плашки сайта, плашка сайта сильнее отдыха.
5. PL-41: `TestDomain`, `closeTab`, `closeBraveWindow` проходят через `PollOnce` в тестах; подмена `domain` проверена и в снимке, и в исполнении.
6. Гонки: `go test -race` с `PollOnce` и `View` из разных горутин (`TestControllerIsSafeForConcurrentUse`, `controller_test.go:944`) зелёный; поля лестницы и подмены — под `c.mu`.

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

## Уроки (docs/LESSONS.md)

- **PL-9** (внешний идентификатор в бою нулевой, тесты подставляют настоящий): `TabLook` собирается из настоящего переднего плана, тесты не подставляют то, чего `winTabs.Look` не получает; путь «не прочитано» (`Unread`, `Window 0`, путь «») — ничего не трогать. Сейчас домен в бою вообще не уходил в службу, а тест `TestPushStateCarriesForegroundDomain` был зелёным — тот же класс: проверять запрос `GuardPoll`, а не только `GuardState`.
- **PL-13** (штатный конец, подтверждение для всех сеансов): закрытие вкладки и окна Brave — не гибель частей PC Block и не обход; `WM_CLOSE` уходит только окну Brave, страж и значок не затрагиваются.
- **PL-14** (порог «строго больше», взведённый не сбрасывается): видео — те же `duplicate`/`moved`, что игры; сдвиг остатка видео (доп. время, копилка, промокод) снимает `m.last` только через `moved`.
- **PL-15** (снимок из одного момента): путь, домен и `DomainUnread` — из одного `readRaw`; решение о закрытии — по снимку службы, исполнение — после `Look()`; `ClosedTab` пишется до `PushState` в той же горутине.
- **PL-34** (старшинство плашек): плашка 1 мин > плашка сайта > отдых; менее важная ждёт (срок идёт), а не заменяет; таблица — литералами в тесте E.
- **PL-41** (непроверенная запасная ветка сломана): подмена `domain` исполняется в тесте контроллера и в снимке, и в `closeTab`; лестница доходит до `invoke` и `wmClose` хотя бы в одном тесте; на Windows — шаг (ж) пробы.

## Open (передать дальше, не делать в T7)

- `SendInput` от имени SYSTEM в сеансе ребёнка проба T0 не проверяла (Ctrl+W мерился от имени раннера; от SYSTEM — только чтение домена). Проверяет T10 (S16: настоящий страж, `ClosedTabWay == "ctrlW"`); если не сработает — лестница сама дойдёт до `invoke` (UIA от SYSTEM работает) — отметить в вердикте как риск.
- План, UX-сценарий: заголовок табло видео (T7-10) — после решения оркестратора поправить строку плана.
- README «Отклонения от ТЗ» — ничего от T7, если оркестратор не выберет иное по T7-3.

## Вердикт (≤ 40 строк)

```
TASK T7: done | blocked | failed
Diff: sha + строка на коммит
Evidence: go test -race -count=1 ./internal/guard/ ./internal/systray/ (счётчики), ipc/sites, go vet (Linux и GOOS=windows через vet5.mod), gofmt -l, scan_ui_texts, RED-запуски (тест и причина), мутации (какой тест упал)
Interfaces: для T10 (GuardState.ClosedTab/ClosedTabWay, подмена domain только при Brave на переднем плане — по пути после подмены foreground, исполнение — по настоящему; заголовок табло видео; подсказка значка «\nИгры:», «\nВидео:»)
Rulings: T7-1…T7-11 подтверждены или изменены, своя цена ошибки
Escalations: изменённые ожидания старых тестов (TestPushStateCarriesForegroundDomain, TestDomainMemo, тесты omniboxDomain), любые правки вне Files, заголовок табло видео
Open: SendInput от SYSTEM (T10), прогон windows.yml scenario=probe (оркестратор), строка «Игры:» в значке — закрыт пропуск M3
```

## Решения оркестратора по вопросам пакета

- **T7-10 утверждено:** заголовок табло видео — «ОСТАЛОСЬ НА ВИДЕО:» (по образцу игр), чтобы ребёнок не принял остаток видео за общий; UX-сценарий плана поправлен.
- **T7-3 уточнено:** шаг лестницы повышается, только если после действия **число вкладок окна не уменьшилось** (подсчёт `TabItem` через UIA до и после) и домен тот же; если вкладок стало меньше — действие удалось, лестница сбрасывается на Ctrl+W (следующая вкладка того же запрещённого сайта закрывается так же, окно Brave целиком не закрывается). Не удалось посчитать вкладки — считать, что не уменьшилось. Тест чистой функции `nextClose` с литералами на оба случая.
