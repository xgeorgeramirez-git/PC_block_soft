# Пакет M3-T3a: хранилище и контракт канала M3 | слот: писатель | дерево: `/home/claude/pc_block_soft`, ветка `feature/m3-programs`

## Контекст продукта

PC_block_soft — домашняя программа родительского контроля для Windows 11 (Go-служба от SYSTEM, страж в сеансе ребёнка, значок, окна Wails + React/TS, SQLite). Веха M3 «Программы и игры»: лимит игр, учёба не считается, незнакомые программы ждут решения родителя, режим «Оставить только учёбу». План: `plans/2026-10-29-M3.md` (прочитай целиком: Global Constraints, Ruling RM3-1…RM3-12, Review Focus). К этому моменту в ветке уже сделаны T1 (Помощник удалён, миграция 003 чистит `helper.*`) и T2 (ядро: `core.DayLimit.GamesMin`, `core.Rules.EndMode`, `Decision.CloseGames/StudyOnly/Allows/Counted/CountedCat`, `GamesRemainingS/GamesLimitS/GamesUnlimited`, `WarnGames`, тексты `StudyOnlyPlateText`, `GamesOverPlateText`, `StudyOnlyDeniedText`, `GamesDeniedText`) — **перед работой прочитай их вердикты/диф (`git log --stat -15`) и используй их настоящие имена**.

Твоя задача — фундамент для остальных: хранилище, миграция 004 и ВЕСЬ контракт канала M3 (все типы и команды, чтобы T3b/T4a/T5/T6 не зависели друг от друга в обратную сторону). Логику программ (классификатор, каталог) делает T3b, наблюдение — T4a/T4b, окна стража — T5, экраны — T6.

Отвечай по-русски. Методика: TDD — сначала падающий тест, ожидания литералами, мутационная проверка; маленькие коммиты (по одной подзадаче).

## 0. Среда

- Дерево `/home/claude/pc_block_soft`, ветка `feature/m3-programs`; ты единственный писатель. `plans/`, `docs/01_ТЗ.md` не правь. Коммить `git add <пути>` явно (не `-A`); push не делай.
- Go: `export PATH=/home/claude/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off`. Локально собираются и тестируются пакеты на стандартной библиотеке: `go test -race -count=1 ./internal/core/... ./internal/ipc/... ./internal/service/...`. `internal/storage` (SQLite modernc) локально недоступен: пиши тесты, проверяй `gofmt`/`go vet` настолько, насколько получается (`go vet ./internal/storage/...` может не собраться без модулей — тогда отметь «проверит CI»). Для Windows-кода: временный modfile с replace x/sys (см. CLAUDE.md «Среда»).
- Фронт-типы: ты правишь `frontend/src/types.ts` (TS-зеркало DTO), `api.ts`, `contract.test.ts`, `__mocks__/backend.ts` только в части контракта (интерфейсы, обёртки, заглушки ответов) — экраны не трогай. `cd frontend && npx tsc --noEmit && npx vitest run`.
- Коммиты — Conventional Commits по-русски в повелительном наклонении; трейлеры: `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01XuHYMinhbS8zjLqh6nAgCY`.
- Тексты ребёнку «ты», родителю «вы», без длинных тире (`python3 tools/scan_ui_texts.py`). JSON-ключи только lowerCamel; массивы в ответах `[]`, не `null`.

## 1. Что сделать (подзадачи: RED → GREEN → коммит)

**A. Лимиты игр и режим окончания в хранилище и правилах службы.**
- `internal/storage/data.go`: `SetDayLimits`/`GetDayLimits` читают и пишут `games_min` вместе с `total_min` (`nil` ↔ NULL; убери из комментария «games_min не трогаются», перепиши тест `data_test.go:251-291` на новое поведение: записал игры 60 → прочитал 60; `nil` остался `nil`; 0 остался 0). Видео (`video_min`) по-прежнему не трогать (M4).
- `children.end_mode` (`'lock'|'study_only'`, колонка уже в 001): `storage.Child` получает `EndMode string`, читается `ListChildren`; `SetEndMode(childID int64, mode string) error` (иное значение — ошибка).
- `service.Store` (`ports.go`), `winhost/adapters_windows.go` (`storeAdapter`), `fakes_test.go` (`fakeStore`) — все три места получают новые методы (§B, §C).
- `service.readRules(id)` (`service.go`): собирает `core.Rules{Week (с GamesMin), Schedule, EndMode}`. Остаётся «ошибка чтения → `Broken`».
- **RM3-9 (найденная ошибка).** `hSaveLimits` пересобирает `core.Rules{Week: week}` без расписания; замени на сохранение лимитов и `s.applyRules(cs, s.readRules(id))` (как `hSaveSchedule`). **Первым — регресс-тест**: у ребёнка есть запретный интервал в базе; `SaveLimits` только с лимитами → в следующем `Current()`/`Tick` запретные часы действуют (до исправления тест красный).
- `hSaveLimits`/`SaveLimitsRequest`: принимает `EndMode` (необязательно; пусто — не менять) и `GamesMin/GamesUnlimited` по дням; валидация как для общего (часы 0–23, минуты 0–59, поле игр ≤ общего лимита? — НЕТ, не ограничивай; игры могут быть больше общего, фактически общий кончится раньше), ошибки с полем. Журнал `settings_changed`: «изменены лимиты времени» / «изменён режим, когда время вышло» (тексты родителю на «вы»).
- `hCopyRules` (`handlers_m2.go`, FR-33): копирует `TotalMin`, `GamesMin` и `EndMode`.
- `Setup` (`SetupChild.Limits`): принимает `GamesMin`/`GamesUnlimited` (по умолчанию игры без отдельного лимита).

**B. Миграция 004 `internal/storage/migrations/004_programs_m3.sql`** (первая строка `-- Назначение: …`; 001–003 не переписывать; обнови карту `want` в `store_test.go:TestOpenCreatesFullSchema`, `TestSchemaConstraints`, проверку номеров подряд). Содержит ровно:
  - `ALTER TABLE programs ADD COLUMN publisher TEXT NOT NULL DEFAULT ''`, `... ADD COLUMN fingerprint TEXT NOT NULL DEFAULT ''` (отпечаток «размер:время изменения», RM3-12; пусто — не требуется), `... ADD COLUMN app_root TEXT NOT NULL DEFAULT ''` (корень приложения, RM3-1 п.3);
  - `ALTER TABLE pending_programs ADD COLUMN publisher TEXT NOT NULL DEFAULT ''`, `... ADD COLUMN description TEXT NOT NULL DEFAULT ''`, `... ADD COLUMN last_attempt_at INTEGER NOT NULL DEFAULT 0`, `... ADD COLUMN was_category TEXT NOT NULL DEFAULT ''` (прежняя категория при смене отпечатка, «раньше: Игра»);
  - `CREATE TABLE pending_attempts (exe_path TEXT NOT NULL REFERENCES pending_programs(exe_path) ON DELETE CASCADE, child_id INTEGER NOT NULL REFERENCES children(id) ON DELETE CASCADE, attempts INTEGER NOT NULL, last_attempt_at INTEGER NOT NULL, PRIMARY KEY (exe_path, child_id))`;
  - `CREATE INDEX usage_by_subject ON usage_daily(subject_kind, subject, day)`.
  Тест миграции на **заполненной базе 0.2.5** (создай базу версией до 003, добавь детей, `day_limits`, `settings`, пароль; применить 003 и 004; ничего не потеряно, новые колонки получили значения по умолчанию).

**C. Методы хранилища (в `data.go` или `programs.go`), интерфейс `service.Store`, адаптер, фейк:**
  - Программы: `type Program struct{ID int64; Path, Name, Publisher, Category, Source, Fingerprint, AppRoot string; DecidedAt int64}`; `UpsertProgram(Program) error` (по `exe_path`), `SeedPrograms([]Program) (added int, err)` — вставка только отсутствующих (решения родителя не перезаписываются), одной транзакцией; `GetProgram(path) (Program, bool, error)`; `ListPrograms() ([]Program, error)`; `SetProgramsCategory(paths []string, category string, at int64) (changed []Program, err error)` — возвращает прежние значения для «Отменить»; `RestoreProgram(Program)` и `DeleteProgram(path)` для отмены. Категории — как `CHECK` в схеме (`games|study|other|blocked|system`), источник `parent` при решении родителя.
  - Ждущие решения: `AddPendingAttempt(p PendingProgram, childID, at int64) (attempts int, err)` — создаёт строку или увеличивает `attempts` и `last_attempt_at`, ведёт `pending_attempts` по ребёнку; `ListPending() ([]PendingProgram, error)` — со списком детей (`[]PendingChild{ChildID, Attempts, LastAttemptAt}`), новые сверху по `last_attempt_at`; `ResolvePending(path) (PendingProgram, bool, error)` — удаляет и возвращает (для «Отменить»); `RestorePending(PendingProgram)`.
  - Использование: `AddUsage(childID int64, day, category, kind, subject string, seconds int) error` (upsert со сложением); `SaveDayAndUsage(childID int64, st core.DayState, usage []UsageRow) error` — **одной транзакцией** (RM3-6/G10: накопление в памяти службы, запись вместе с `day_state`); `ProgramSeconds(fromDay string) (map[string]int, error)` — секунды по `subject` (путь программы) для `subject_kind='program'` с `day >= fromDay` (время за неделю в «Программах»).
  - Тесты: каждый метод, граничные случаи (повторный `AddPendingAttempt` растёт, два ребёнка, `ResolvePending` каскадом чистит `pending_attempts`, `SeedPrograms` не перезаписывает `source='parent'`, откат транзакции `SaveDayAndUsage` при ошибке — ни `day_state`, ни `usage_daily` не изменились).

**D. Контракт канала `internal/ipc` (новый файл `commands_m3.go`; константы команд в `commands.go` и `AllCommands`; пример каждого типа в `contract_test.go:examples()`, ключи `"Cmd#empty"` для пустых массивов; `go test ./internal/ipc/ -run Contract -update` пересоздаёт `testdata/contract.json`; число команд в `TestContractHasEveryCommand` увеличь). Минимально, имена можешь уточнить, но состав обязателен:
  - `SensorSample.ForegroundPath string` (`json:"foregroundPath"`, пусто — не определён; длина ≤ 520 символов, иначе служба отбросит); `BuildSample` стража пока оставляет пусто (T5 заполнит).
  - `GuardCommand` получает `Plate *BlockedPlate`; `GuardCmdBlocked = "blocked"`; `BlockedPlate{Kind string; Text string; ShowS int; Path string; CanAsk bool}` с видами `BlockedUnknown`, `BlockedForbidden`, `BlockedGamesOver`, `BlockedStudyOnly`, `BlockedDecided` (константы). `ClassifyCommands` стража (`internal/guard/controller.go`) пока пропускает неизвестное — не меняй (T5).
  - `GuardWarning.Event` получает значение `"games"` (`NewGuardWarningFor` для `core.WarnGames`); тексты игровых порогов в `status.go` рядом с остальными: 30/15 — «Через N минут время на игры закончится», 5 — «Сохрани игру: через 5 минут время на игры закончится», 1 — «Осталась 1 минута на игры: …» по образцу 1-минутной плашки (используй функции форматирования, что уже есть); тест на все четыре порога и «без длинных тире».
  - `ChildStatus` (`status.go`, `ChildStatusFrom`): `GamesRemainingS int`, `GamesLimitS int`, `GamesUnlimited bool`, `EndMode string` (`lock|study_only`), `CloseGames bool`, `StudyOnly bool`; значения берутся из `core.Decision` (ядро — единственный источник цифр). Текст подсказки значка (`Tooltip`): при наличии лимита игр — вторая строка «Игры: 20 мин» (формат `core.FormatDuration`/`RemainingMinutes`); тест. Экран блокировки определяется по `Decision.Lock`, **не** по статусу: в режиме «только учёба» при `StatusTimeUp` экрана нет (проверь `ChildStatusFrom` и места, где `Status` превращается в `Lock`, и поправь, если там завязка на статус).
  - Карточка родителя («Сегодня»): строка игр «Игры 40 мин из 1 ч» (`Card.GamesText`, пусто, если лимита игр нет) и `Card.PendingPrograms int` (число ждущих решения, для ссылки «Ждут решения: N», 8.3.2 п.5).
  - `MyTodayReply.Rules` («Мои правила», R21): строки лимита игр и режима окончания простыми словами («Игры: до 1 ч в день», «Когда время вышло: экран блокировки» / «…: остаётся только учёба»).
  - `ChildRules`/`WeekdayLimit`: `GamesMin int`, `GamesUnlimited bool` по дням; `ChildRules.EndMode`; `SaveLimitsRequest.EndMode`; `SetupChild.Limits` — те же поля.
  - `OpenWindowRequest.Screen string` (необязательно; допустимо `"programs"`; иное → отказ `textBadRequest`); `launch.params` и `App.StartScreen()` делает T5 — не трогай.
  - Команды программ (только родительская сессия, `parentToken`): `CmdListPrograms` → `ProgramsReply{Pending []PendingProgramDTO; Programs []ProgramDTO; Children []ChildName}` (`PendingProgramDTO{Path, Name, Publisher, Description, Attempts, LastAttemptAt (мс UTC), WasCategory, ByChild []PendingChildDTO{ChildID, Name, Attempts}}`, `ProgramDTO{Path, Name, Publisher, Category, Source, WeekS int}`); `CmdDecideProgram` `{Path, Category}` → `DecideProgramReply{Message, UndoID, Programs…}`; `CmdSetProgramsCategory` `{Paths []string, Category}` → то же; `CmdUndoProgram` `{UndoID}`. Категории в DTO: `games|study|other|blocked|system` (в интерфейсе родителю `system` не показывается как выбор).
  - Настройка до первого входа: `CmdScanPrograms` → `ScanProgramsReply{Programs []ScanProgramDTO{Path, Name, Publisher, SuggestedCategory}}` — права как у `CmdListAccounts` (повышенный администратор, пока программа не настроена); `SetupRequest.Programs []SetupProgram{Path, Category}`.
  - Права: строка в `commandRules` (`service/commands.go`) на каждую команду + независимая таблица `wantAllowed` в `commands_test.go`; `TestEveryChannelCommandHasRightsRule` зелёный.
  - Заглушки обработчиков в службе: возвращают `ipc.Fail(textNotReady)`?? — НЕТ: обработчики реализует T3b. Здесь для новых команд регистрируй обработчики-«не реализовано» (`ipc.Fail("Пока не готово")` с TODO-тестом, что они помечены), чтобы `TestEveryChannelCommandHasHandler` (если есть) оставался зелёным; T3b их заменит.
  - TS-зеркало: интерфейсы в `types.ts`, обёртки в `api.ts` (`null` → `[]`), строки в `CASES`/`payloadOf` в `contract.test.ts`, заглушки в `__mocks__/backend.ts` (родительские команды в `PARENT_COMMANDS`); мок хранит программы в памяти и умеет решать/отменять — этого достаточно T6.

**E. Статус по играм в службе.** `statusOf`/`cardOf`/`myTodayOf` заполняют новые поля из `Decision` (Card.PendingPrograms — из `Store.ListPending()` длина). Тесты на фейках службы: игры 60 мин, 20 использовано → «Игры 40 мин из 1 ч»; без лимита игр пусто; режим `study_only` в «Моих правилах».

## 2. Ruling оркестратора

- Миграция 004, не 003 (003 занята T1). Не объединяй с другими.
- Отпечаток — строка `"<size>:<mtime unix>"`, формируется службой (T3b/T4b); хранилище только хранит.
- `usage_daily` пишется службой накопленными порциями; ключ — категория + путь программы в нижнем регистре; для «Системного» писать не нужно (`CHECK` запрещает).
- Новые команды канала — только в `commands_m3.go` и в таблице прав; ничего не удаляй из старых.

## 3. Review Focus

1. Частичные пути сохранения правил теряют соседние поля (RM3-9): любой путь, собирающий `core.Rules`, берёт его целиком из `readRules` (лимиты+игры+расписание+режим); найди все вызовы `applyRules` (`grep -n applyRules internal/service`) и объясни каждый в вердикте.
2. Контракт: TS ↔ Go ↔ `contract.json` ↔ мок синхронны; нет `null` в массивах; ключи lowerCamel; числа времён — мс UTC.
3. Миграция на заполненной базе 0.2.5; идемпотентность `schema_migrations`; внешние ключи (`PRAGMA foreign_keys` включён?) — каскад `pending_attempts`.
4. Права: ребёнок не может читать/менять программы; `ScanPrograms` невозможен после настройки.
5. Игры в карточках: нет расхождения между значком, карточкой, «Моей статистикой» (все из одного `Decision`).

## 4. Вердикт (≤ 40 строк)

`done/blocked/failed`; Diff (sha + строка на коммит); Evidence — команды и итоги (`go test -race` пакетов, `go vet`, `gofmt -l`, `tsc`, `vitest`, scan, мутации и что упало; что не проверено локально и должен проверить CI); Rulings; Escalations; список публичных имён хранилища и ipc, которые понадобятся T3b/T4a/T5/T6.
