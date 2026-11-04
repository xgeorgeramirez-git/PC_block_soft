# Пакет: T2 вехи M4 «Ядро: лимит видео» | слот: писатель | исполнитель: новый агент

Репозиторий `/home/claude/pc_block_soft`, ветка `feature/m4-sites`. Прочитай `plans/2026-11-04-M4.md`: Global Constraints, RM4-6, Review Focus п. 4 и 9, строку T2. ТЗ (`docs/01_ТЗ.md`): R5, R6, R8, R9, R10, R11, R26, FR-9, 8.2.2, 8.2.5 п. 3.

## Бриф

Ядро `internal/core` — единственный источник цифр и решений. Ты добавляешь лимит «Видео и соцсети» (R6) **точно по образцу лимита игр (R5) из M3**: тот же расчёт остатка, доп. время его расширяет, свой набор порогов 30/15/5/1, которые срабатывают только при видео на переднем плане, закрытие видео по исчерпании и незасчитанная секунда видео, когда видео нельзя. Ядро знает только категорию `CatVideoSocial`: какой домен видео, решает служба (T4). Хранилище, канал (`internal/ipc`), служба, страж, фронт — следующие задачи, их не трогай (кроме починки компиляции, см. шаг 5).

Перед работой сверь с кодом (PL-4), что образец устроен так, как описано ниже: `grep -n "Games\|gwarn\|e.fg" internal/core/{types,day,engine,warnings,texts,account}.go`.

## Образец в коде (проверено читателем, адреса для сверки)

- `internal/core/types.go:61` — `DayLimit{TotalMin, GamesMin *int}`; `:143` — `WarnEvent` с `WarnGames` последним, `String()` → `"games"`.
- `internal/core/day.go:44` — `RemainingGamesS(limit DayLimit, extraMin, gamesS int) (remaining int, unlimited bool)`.
- `internal/core/engine.go`:
  - `Decision` (стр. 26–66): `GamesRemainingS`, `GamesLimitS`, `GamesUnlimited`, `CloseGames`, `StudyOnly`, `CloseReason`, `Counted`, `CountedCat`;
  - `Allows` (стр. 72) и `DeniedText` (стр. 86);
  - `Engine.gwarn warnings`, `Engine.fg Category` (выставляет `Tick` до `decide`, стр. 149);
  - `Tick`: `blocked := d.Lock != LockNone || !d.Allows(cat)` (RM3-7) — менять не нужно, хватит `Allows`;
  - `rearm` (стр. 464) взводит `warn` и `gwarn`; его зовут `AddExtra`, `RedeemPromoMinutes`, `ReduceExtra`, `ClearExtra`, `Pause`, `SpendBonus`, `SetNoForbiddenToday`, `SetRules`, `advance` (сутки), `checkPauseEnd`, `NewEngine` — новый набор порогов достаточно добавить в `rearm`;
  - `nextGames` (стр. 475): нет события при `Broken` и активной паузе;
  - `decide` (стр. 526): `grem, gunl := RemainingGamesS(...)`, `GamesLimitS = max(0, (*lim.GamesMin+st.ExtraMin)*60)`; ветки: пауза → запретный интервал (`endMode` + `quenchGames`) → время вышло (то же) → иначе `CloseGames` по лимиту игр + `fireWarnings`;
  - `endMode` (стр. 606), `quenchGames` (стр. 617), `fireWarnings` (стр. 628): общий порог, затем игровой — только при `e.fg == CatGames` или `CloseGames`; не показывается, если `grem > deltaS` общего события; при одновременном срабатывании показывается меньший остаток, при равенстве общий.
- `internal/core/warnings.go` — `warnings.arm/fire` (строгое «больше», PL-14) — без изменений.
- `internal/core/account.go:64` — `Accumulator.Tick` уже пишет `VideoS++` и `TotalS++` для `CatVideoSocial` — без изменений.
- `internal/core/texts.go:22` — `GamesOverPlateText`, `GamesDeniedText`, `StudyOnly…`; `RemainingMinutes` (вверх, B-5) и `FormatDuration` («40 мин», «1 ч 05 мин»).

## Ruling читателя (оркестратор подтверждает до пуска; цена ошибки — в скобках)

- **V-1 Типы.** `DayLimit.VideoMin *int` (nil — без лимита, 0 — сегодня нельзя). `WarnVideo` — после `WarnGames`, `String()` → `"video"`. Хранилище уже имеет столбец `video_min` (`internal/storage/migrations/001_init.sql:19`), его чтение — T3.
- **V-2 Остаток.** `day.go`: `RemainingVideoS(limit DayLimit, extraMin, videoS int) (remaining int, unlimited bool)` — копия `RemainingGamesS` по `VideoMin`.
- **V-3 Решение.** Поля `Decision`: `VideoRemainingS int`, `VideoLimitS int` (лимит с доп. временем, `max(0, …)`; 0 без лимита), `VideoUnlimited bool`, `CloseVideo bool`, `VideoCloseReason string`. Отдельный текст, а не `CloseReason`: игры и видео могут быть закрыты одновременно, у каждой плашки свой текст (цена: служба T4 берёт не то поле — видно в её тесте).
- **V-4 Когда закрывать.** `CloseVideo` ставится **только в ветке без блокировки** (`default` в `decide`), если лимит видео есть и остаток ≤ 0 — как `CloseGames` по лимиту игр. Под экраном блокировки (EndBlock) видео не закрывается: R8 закрывает только игры, остальное остаётся под экраном. В режиме «только учёба» видео закрывает `StudyOnly` (`Allows(CatVideoSocial) == false`, `DeniedText` = `StudyOnlyDeniedText`) — в ядре для R8/Brave ничего нового: домен «Учёбы» служба передаёт как `CatStudy`, остальные домены — как видео или «Прочее», и они закрываются; служебные страницы Brave (домен «») служба не закрывает сама (как «Системное», C-8 M3). Пауза и `Broken` — ничего не закрыто, `VideoUnlimited = true` при `Broken`.
- **V-5 Allows/DeniedText.** `Allows`: новая ветка `case d.CloseVideo && cat == CatVideoSocial: return false` после игр. `DeniedText`: после игр `case d.CloseVideo && cat == CatVideoSocial: return VideoDeniedText`. Флаг `blocked` в `Tick` подхватывает это сам: секунда видео при закрытом видео не засчитывается (RM3-7).
- **V-6 Тексты** (`texts.go`, ребёнку на «ты», без длинных тире):
  - `VideoDeniedText = "Закрыто: время на видео и соцсети на сегодня закончилось"` — журнал; дословно в ТЗ нет, собран по образцу `GamesDeniedText` (цена: заказчик попросит другую формулировку).
  - `func VideoOverPlateText(totalRemainingS int, totalUnlimited bool) string` — плашка 8.2.5 п. 3 дословно: `"Время на видео и соцсети на сегодня закончилось. Общего времени осталось " + FormatDuration(RemainingMinutes(totalRemainingS)*60)`; без общего лимита — только `"Время на видео и соцсети на сегодня закончилось"` (вторую фразу при «без лимита» сказать нечем; цена — мелкая). Минуты — единым правилом B-5 (вверх), как значок. `decide` кладёт результат в `VideoCloseReason`, когда `CloseVideo`.
  - Тексты **предупреждений** о видео живут не в ядре, а в `internal/ipc/status.go` (`warnSubject`, `warnOneMinute`, `NewGuardWarningFor`, стр. 238–300) и `internal/guard/logic.go:255` (табло игр) — это T3/T7. Для них (дословно из задания оркестратора и по образцу игр): 30/15 — «Через 30 минут время на видео и соцсети закончится»; 5 — «Через 5 минут время на видео и соцсети закончится» (без «Сохрани игру»); 1 — «Осталась 1 минута на видео и соцсети». До T3 `warnEventName(WarnVideo)` падает в ветку `default` → `"limit"` с текстом общего лимита: это не ошибка T2, а работа T3 (отметь в вердикте).
- **V-7 Пороги видео.** `Engine.vwarn warnings`; `rearm()` взводит его `e.vwarn.arm(videoS, WarnVideo, videoLimited)` из `nextVideo(st, now)` (копия `nextGames`). `decide`: в ветках запретного интервала и «время вышло» — `quenchVideo` рядом с `quenchGames` (обобщи в одну функцию с параметром набора, если читается проще). В `fireWarnings` после игрового порога — видео-порог по тем же правилам: срабатывает, только если `e.fg == CatVideoSocial` или `CloseVideo` (тогда гасится без показа); не показывается, если `vrem > deltaS` общего события (общее кончится раньше), но гасится; показывается, если ещё ничего не выбрано или его остаток строго меньше выбранного. Игра и видео не бывают на переднем плане в одну секунду, поэтому игровой и видео-порог в один тик показаться не могут; общий + видео — одно уведомление (меньший остаток, при равенстве общий). Внимание: сейчас `fireWarnings` выходит **ранним `return`** при `gunl || (e.fg != CatGames && !d.CloseGames)` и при непоказанном игровом пороге — видео-ветка за ними не выполнится; раздели на две независимые ветки (игры, видео), каждая со своим «гасить/показать». Порог 1 мин видео **не** ставит `LastMinuteEndMs` (это только общий лимит, D-60).
- **V-8 Комментарии.** Обнови комментарии `Engine.fg`, `rearm`, `fireWarnings`, `WarnEvent` (текст «закончится время на видео и соцсети»), `Decision.Allows`.

## Files

Modify: `internal/core/types.go`, `internal/core/day.go`, `internal/core/engine.go`, `internal/core/texts.go`, `internal/core/engine_games_test.go` (только новые строки в таблице `TestDecisionAllows`, старые не менять).
Create: `internal/core/engine_video_test.go`.
Не трогать: `internal/core/warnings.go`, `account.go`, `golden_test.go`, `testdata/golden/*`, `docs/`, `plans/` (кроме своего вердикта в ответе), версию.

## Interfaces

Produces (для T3 ipc/хранилище, T4 служба, T7 страж): `DayLimit.VideoMin *int`; `RemainingVideoS`; `WarnVideo` (`"video"`); `Decision.VideoRemainingS`, `VideoLimitS`, `VideoUnlimited`, `CloseVideo`, `VideoCloseReason`; `VideoDeniedText`; `VideoOverPlateText(totalRemainingS int, totalUnlimited bool) string`.
Consumes: ничего нового; существующие `RemainingMinutes`, `FormatDuration`, `warnings`.

## Шаги (каждый: RED → GREEN → мутации → коммит)

Среда: `export PATH=~/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off`. Прогон: `go test -race -count=1 ./internal/core/...`; затем `go vet ./internal/core/...`, `gofmt -l internal`.

Образцы тестов (те же помощники: `intp`, `gamesMon`, `gameSample`, `studySample`, `tue` — `engine_games_test.go:12–33`; `at`, `run`, `monday`, `tuesday` — `engine_test.go:16–40`; `fakeClock`, `msk` — `clock_test.go`; `activeSample` — `account_test.go:18`; `warnEvLog` — `schedule_engine_test.go:11`, формат `"15:30:00 30 games"`; `iv` — `schedule_test.go:16`):
`TestRemainingGamesS`, `TestEngineGamesRemaining` (подтесты «60 + 30, 20 мин игры», «лимит игр 0», «без лимита игр», «правила не прочитаны»), `TestEngineGamesAndStudyTotals`, `TestDecisionAllows`, `TestEngineStudyOnly`, `TestEngineEndBlockClosesGames`, `TestEngineGamesWarnings` (подтесты «+30 при исчерпанных играх», «SetRules на границе той же секунды», «равные события в один тик», «два порога в один тик, меньший остаток», «общий лимит кончится раньше», «только при игре на переднем плане», «пауза гасит, конец паузы взводит», «смена суток обнуляет игры»), `TestEngineSetRulesEndModeAndGames`, `TestEndModeStrings` — все в `engine_games_test.go`; `TestRedeemPromoMinutesExtendsGames` — `promo_engine_test.go:48`; `TestWarn30BonusMovesEventPast30` — `warnings30_test.go:59`.

В новом файле помощники: `videoMon(total, games, video *int, mode EndMode) Rules` (пн; как `gamesMon` + `VideoMin`) и `videoSample()` (`SessionActive: true, LastInputAgoMs: 1000, Foreground: CatVideoSocial`).

1. **Остаток и типы.** RED `TestRemainingVideoS` (таблица, `(rem, unl)`): нет лимита видео (`TotalMin` 120, extra 30, видео 100 с) → `(0, true)`; 20 мин, 5 мин видео → `(900, false)`; 20 + 30, 5 мин → `(2700, false)`; 0 → `(0, false)`; 0 + 30 → `(1800, false)`; 10 − 5, видео 900 → `(0, false)`. `TestWarnVideoString`: `WarnVideo.String() == "video"`, `WarnGames.String() == "games"`.
2. **Решение.** RED `TestEngineVideoRemaining`:
   - «20 + 30, 5 мин видео»: `videoMon(120, nil, 20, EndBlock)`, 15:00, `AddExtra(30)` → `VideoLimitS == 3000`, `LimitS == 9000`, `!VideoUnlimited`; `run` 15:00:00–15:05:00 `videoSample()` → `Current()`: `VideoRemainingS == 2700`, `RemainingS == 8700`, `!CloseVideo`; `ReduceExtra(60)` (доп. −30) → `VideoLimitS == 0`, `VideoRemainingS == 0`, `CloseVideo`.
   - «лимит видео 0»: `videoMon(120, nil, 0, EndBlock)`, первый `Tick(videoSample())` → `CloseVideo`, `Lock == LockNone`, `RemainingS == 7200`, `!Counted`, `Status == StatusOK`, `VideoCloseReason == "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 2 ч 00 мин"`, `CloseReason == ""`, `!CloseGames`; следующий тик `activeSample()` → `Counted`, `CountedCat == CatOther`; затем `gameSample()` → `Counted`, `CatGames`; итог `TotalS == 2`, `VideoS == 0`; `AddExtra(15)` → `VideoLimitS == 900`, `!CloseVideo`, `VideoCloseReason == ""`.
   - «без лимита видео»: `videoMon(120, nil, nil, EndBlock)`, `VideoS: 7000, TotalS: 7000` → `VideoUnlimited`, `VideoLimitS == 0`, `!CloseVideo`, `Counted`.
   - «правила не прочитаны»: `videoMon(0, 0, 0, EndStudyOnly)`, `Broken` → `VideoUnlimited`, `!CloseVideo`, `Allows(CatVideoSocial)`, `Counted`.
3. **Секунда после исчерпания.** RED `TestEngineVideoClosedSecondNotCounted`: `videoMon(120, nil, 20, EndBlock)`, `DayState{Day: monday, VideoS: 1199, TotalS: 1199}`; 15:00:00 видео → `Counted`, `CountedCat == CatVideoSocial`; 15:00:01 видео → `CloseVideo`, `!Counted`; 15:00:02 `activeSample()` → `Counted`; `State()` → `VideoS == 1200`, `TotalS == 1201`; `Allows(CatVideoSocial) == false`, `Allows(CatOther) == true`, `DeniedText(CatVideoSocial) == "Закрыто: время на видео и соцсети на сегодня закончилось"`.
4. **Текст плашки.** RED `TestVideoOverPlateText` (таблица): `(2400, false)` → `"Время на видео и соцсети на сегодня закончилось. Общего времени осталось 40 мин"`; `(2399, false)` → `"… осталось 40 мин"` (вверх); `(3899, false)` → `"… осталось 1 ч 05 мин"`; `(0, true)` → `"Время на видео и соцсети на сегодня закончилось"`. `TestEngineVideoCloseText`: `videoMon(120, nil, 20, EndBlock)`, `VideoS: 1200, TotalS: 4800` → `Tick(videoSample()).VideoCloseReason == "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 40 мин"`; без общего лимита `videoMon(nil, nil, 20, …)` → без второй фразы.
5. **Allows.** В `TestDecisionAllows` (`engine_games_test.go:162`) добавь строки (порядок `CatOther, CatGames, CatVideoSocial, CatStudy`): «видео закрыто» `Decision{CloseVideo: true}` → `[true, true, false, true]`, тексты `["", "", "Закрыто: время на видео и соцсети на сегодня закончилось", ""]`; «игры и видео закрыты» `Decision{CloseGames: true, CloseVideo: true}` → `[true, false, false, true]`, `["", "Закрыто: время на игры на сегодня закончилось", "Закрыто: время на видео и соцсети на сегодня закончилось", ""]`; «только учёба и видео закрыто» `Decision{StudyOnly: true, CloseVideo: true}` → `[false, false, false, true]`, видео — `"Закрыто: время вышло, доступна только учёба"`.
6. **Режимы окончания.** RED `TestEngineVideoEndModes`: (а) `EndBlock`, `videoMon(60, nil, 20, EndBlock)`, `TotalS: 3600, VideoS: 1200` → `Lock == LockTimeUp`, `CloseGames`, `!CloseVideo`, `VideoCloseReason == ""` (V-4); (б) `EndStudyOnly`, `videoMon(60, nil, nil, EndStudyOnly)`, `TotalS: 3600` → `StudyOnly`, `Allows(CatVideoSocial) == false`, `Tick(videoSample()).Counted == false`, `Tick(studySample()).Counted == true`; (в) пауза при исчерпанном видео: `Pause(at(17,0,0))` → `!CloseVideo`, видео `Counted`.
7. **Пороги.** RED `TestEngineVideoWarnings` (литералы по `warnEvLog`):
   - «пороги при видео на переднем плане»: `videoMon(300, nil, 60, EndBlock)`, 15:00:00–16:00:05 видео → `["15:30:00 30 video", "15:45:00 15 video", "15:55:00 5 video", "15:59:00 1 video"]`.
   - «только при видео на переднем плане»: `videoMon(120, nil, 60, EndBlock)`, `TotalS: 1790, VideoS: 1790`; 15:00:00–15:00:10 видео → `[]`; 15:00:10–15:05:00 `gameSample()` → `[]` (и игровых нет: лимита игр нет); `e.Current()`; 15:05:00–15:05:02 видео → `["15:05:00 30 video"]`.
   - «игры и видео с равным остатком»: `videoMon(300, 60, 60, EndBlock)`, `GamesS: 1799, VideoS: 1799, TotalS: 3598`; 15:00:00–15:00:05 видео → `["15:00:01 30 video"]`; 15:00:05–15:00:08 игра → `["15:00:06 30 games"]` (каждое событие своим порогом, ни одного двойного).
   - «+30 при исчерпанном видео»: `videoMon(300, nil, 0, EndBlock)`, `AddExtra(30)`, 15:00:00–15:30:05 видео → `["15:15:00 15 video", "15:25:00 5 video", "15:29:00 1 video"]` (без «за 30», PL-14).
   - «SetRules на границе той же секунды»: копия игрового подтеста (`engine_games_test.go:290`) с `videoMon(120, nil, 60, …)`, `VideoS`, `Foreground: CatVideoSocial` → `["15:30:00 30 video"]`, после `SetRules(r)` — `[]`.
   - «равные события в один тик»: `videoMon(60, nil, 60, EndBlock)` → `["15:30:00 30 limit", "15:45:00 15 limit", "15:55:00 5 limit", "15:59:00 1 limit"]`.
   - «два порога в один тик, меньший остаток»: `videoMon(70, nil, 60, EndBlock)` → `["15:30:00 30 video", "15:40:00 30 limit", "15:45:00 15 video", "15:55:00 5 video", "15:59:00 1 video"]`.
   - «общий лимит кончится раньше»: `videoMon(120, nil, 60, EndBlock)`, `TotalS: 6600, VideoS: 2640`, 15:00:00–15:10:05 видео → `["15:05:00 5 limit", "15:09:00 1 limit"]`.
   - «пауза гасит, конец паузы взводит»: `videoMon(300, nil, 60, EndBlock)`, `Pause(at(15,40,0))`, 15:00:00–16:00:05 видео → `["15:10:00 30 pause_end", "15:25:00 15 pause_end", "15:35:00 5 pause_end", "15:39:00 1 pause_end", "15:45:00 15 video", "15:55:00 5 video", "15:59:00 1 video"]`.
   - «копилка взводит 30 заново»: `videoMon(300, nil, 60, EndBlock)`, `VideoS: 1920, TotalS: 1920` (остаток видео 28 мин), `SpendBonus(5)` → 33 мин; 15:00:00–15:03:02 видео → `["15:03:00 30 video"]`.
   - «смена суток обнуляет видео»: `r := videoMon(120, nil, 60, EndBlock)`, `r.Week[1] = DayLimit{TotalMin: intp(120), VideoMin: intp(60)}`; пн 23:59:59, `VideoS: 3600, TotalS: 3600` → `Tick(videoSample()).CloseVideo`; `tue(0,0,0)` → `Current()`: `Day == tuesday`, `!CloseVideo`, `VideoRemainingS == 3600`, `VideoCloseReason == ""`; 00:00:00–00:30:01 видео → `["00:30:00 30 video"]`.
   - «1 мин видео не ставит последнюю минуту»: после подтеста «пороги при видео…» `State()` → `LastMinuteEndMs == nil`.
8. **Промокод и правила.** RED `TestRedeemPromoMinutesExtendsVideo` (в `engine_video_test.go`): `videoMon(120, nil, 30, EndBlock)`, `run` 15:00–15:30 видео → `CloseVideo`, `VideoRemainingS == 0`; `RedeemPromoMinutes(20)` → `!CloseVideo`, `VideoRemainingS == 1200`, `VideoLimitS == 3000`, `LimitS == 8400`. `TestEngineSetRulesVideo`: `videoMon(120, nil, nil, EndBlock)`, `VideoS: 1800, TotalS: 1800`; `SetRules(videoMon(120, nil, 30, …))` → `CloseVideo`, `VideoLimitS == 1800`; `SetRules(videoMon(120, nil, 60, …))` → `!CloseVideo`, `VideoRemainingS == 1800`.
9. **Мутации** (каждая — какой тест упал, в Evidence): (а) убрать ветку `CloseVideo` из `Allows` → `TestEngineVideoClosedSecondNotCounted` (`VideoS 1201`); (б) взвод `vwarn` по `>=` вместо строгого (в `warnings.arm` не трогать — замени `limited` на всегда `true` или подай `videoS+1`) → «+30 при исчерпанном видео»; (в) убрать условие `e.fg == CatVideoSocial` → «только при видео на переднем плане»; (г) не звать `vwarn.arm` в `rearm` → «пауза гасит…», «копилка…»; (д) `CloseVideo` в ветке «время вышло» → `TestEngineVideoEndModes` (а); (е) `extraMin` не прибавлять в `RemainingVideoS` → `TestRemainingVideoS`, промокод; (ж) `FormatDuration(totalRemainingS)` без округления вверх → `TestVideoOverPlateText` (2399).
10. **Соседние пакеты.** `go vet ./internal/ipc/... ./internal/service/... ./internal/guard/...` (без SQLite и Wails) — новые поля не ломают компиляцию; если сломалось — минимальная починка без логики. `go test -race -count=1 ./internal/ipc/...` должен остаться зелёным (`warnEventName` для `WarnVideo` — ветка `default`; поведение для видео добавит T3).

## Review Focus (классы для самопроверки)

1. Дубли уведомлений (PL-13, PL-14): общий + видео в один тик; игра ↔ видео по очереди; перевзвод при всех вызовах `rearm`; граница + `SetRules` в ту же секунду.
2. Доп. время, копилка, промокод, `ReduceExtra`, `ClearExtra` расширяют и сужают видео; `VideoLimitS` не отрицательный.
3. Пауза, `Broken`, экран блокировки — видео не закрыто; «только учёба» — видео закрыто через `StudyOnly`.
4. Сутки: `Current()` нового дня без тика — видео открыто, остаток полный.
5. Совместимость: нулевые новые поля = «ничего не закрывать»; все старые тесты и эталоны G1…G14 зелёные **без правки ожиданий** (пришлось править — стоп, Escalations).

## Global Constraints (дословно из плана)

- Ядро `internal/core` и новый пакет `internal/sites` — только стандартная библиотека; тесты идут локально (`GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 ./internal/core/... ./internal/sites/...`, Go 1.27 в `~/.local/go1.27`). Правила, цифры и тексты считает ядро и служба; страж, значок, окна — только показывают.
- Тексты ребёнку на «ты», родителю на «вы», без длинных тире (`python3 tools/scan_ui_texts.py`), без технических ошибок. Тексты плашек — дословно ТЗ 8.2.5.
- Предупреждения о видео: пороги 30/15/5/1 (D-54), табло за 30/15/5, плашка 1 мин, звук заказчика — тот же путь, что у игр; порог взводится, только если до события строго больше его минут (PL-14).
- Плашки стража имеют старшинство (PL-34, Ruling FB14-9); новые плашки сайтов — места в RM4-9.
- Писатель коммитит пути явно (`git add <пути>`, никогда `-A`), не пушит; пушит оркестратор. Трейлеры коммитов: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze`.

## Уроки (docs/LESSONS.md)

- PL-14 (класс A, уведомления): взвод строго «больше», пересчёт без сдвига события не повторяет порог — подтесты «+30», «SetRules на границе».
- PL-34 (класс B): у плашек старшинство; видео-плашка не должна вытеснять «Осталась 1 минута» — в ядре это одно предупреждение на тик (меньший остаток, при равенстве общее).
- PL-37 (класс A): «засчитывать ли секунду» — одна точка (`blocked` в `Tick` через `Allows`), без особой категории; все потребители (остаток, `VideoS`, `Counted`, перерыв R27) получают одно решение.
- PL-35 (класс A): подписи следуют состоянию ядра — текст плашки видео строится из того же остатка, что значок (`RemainingMinutes`, вверх).
- PL-15 (класс D): решение из одного момента — `Current()` при смене суток показывает новый день целиком (видео открыто), а не смесь.

## Коммиты

Маленькие, Conventional Commits по-русски, повелительное наклонение (`feat: добавить в ядро лимит видео и соцсетей`, `test: проверить пороги предупреждений о видео`). Пути явно: `git add internal/core/types.go internal/core/day.go internal/core/engine.go internal/core/texts.go internal/core/engine_video_test.go internal/core/engine_games_test.go` (и только починенные соседние файлы, если были). В дереве может работать другой писатель — чужие файлы не добавлять. Трейлеры:

```
Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze
```

НЕ пушить.

## Вердикт (≤ 40 строк)

```
TASK T2: done | blocked | failed
Diff: sha + строка на коммит
Evidence: go test -race -count=1 ./internal/core/... (счётчик), go vet, gofmt -l, соседние пакеты (vet, ipc test), RED-запуски (тест и причина), мутации а…ж (какой тест упал)
Interfaces: новые публичные имена ядра (для T3/T4/T7), если отличаются от V-1…V-6
Rulings: свои решения с ценой ошибки
Escalations: изменённые ожидания старых тестов, неясности ТЗ
Open: что ждёт T3 (warnEventName/тексты предупреждений для WarnVideo)
```
