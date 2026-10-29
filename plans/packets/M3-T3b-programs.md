# Пакет M3-T3b: программы — классификатор, каталог, снимок, команды родителя | слот: писатель | дерево: `/home/claude/pc_block_soft`, ветка `feature/m3-programs`

## Контекст продукта

PC_block_soft — домашняя программа родительского контроля для Windows 11 (Go-служба от SYSTEM, страж, значок, окна Wails + React/TS, SQLite). Веха M3 «Программы и игры». План `plans/2026-10-29-M3.md` — прочитай целиком (Global Constraints, Ruling RM3-1…RM3-12, Review Focus). Уже в ветке: T1 (Помощник удалён), T2 (ядро игр/режима), T3a (хранилище `programs`/`pending_programs`/`pending_attempts`/`usage_daily`, миграция 004, весь контракт канала M3 и заглушки обработчиков). **Перед работой изучи `git log --stat -25` и вердикты T2/T3a; используй настоящие имена.**

Твоя задача — логика программ (R12, R13, FR-20, FR-2): чистый пакет `internal/programs`, снимок и каталог, обработчики команд родителя. Наблюдение за процессами (завершение) — T4a/T4b; окна стража — T5; экраны — T6.

Отвечай по-русски. Методика: TDD — сначала падающий тест, ожидания литералами, мутационная проверка; маленькие коммиты.

## 0. Среда

- Дерево `/home/claude/pc_block_soft`, ветка `feature/m3-programs`; ты единственный писатель. `plans/`, `docs/01_ТЗ.md` не правь. Коммить `git add <пути>` (не `-A`); push не делай.
- Go: `export PATH=/home/claude/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off`; локально: `go test -race -count=1 ./internal/programs/... ./internal/core/... ./internal/ipc/... ./internal/service/...`, `go vet`, `gofmt -l internal`. Windows-код — временный modfile (CLAUDE.md «Среда»), SQLite/Wails проверит CI.
- Коммиты — Conventional Commits по-русски в повелительном наклонении; трейлеры `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01XuHYMinhbS8zjLqh6nAgCY`.
- Тексты ребёнку «ты», родителю «вы», без длинных тире (`python3 tools/scan_ui_texts.py`).

## 1. Требования (дословно ТЗ 1.8: прочитай R3, R8, R12, R13, FR-2, FR-13, FR-20, 8.2.4, 8.3.4, R22, R23)

- R12: запуск в детском сеансе программы, которой нет в списке и которая лежит вне `C:\Windows`, отменяется; она попадает в «Ждут решения» (повторные попытки увеличивают счётчик); ребёнку — сообщение. Родитель относит программу к «Играм», «Учёбе», «Прочему» или «Запрещено»; решение действует для всех детей. При установке программа запоминает программы из `Program Files` как «Прочее», известные игры и браузеры — по встроенному каталогу.
- R13: «Запрещено» — завершение ≤ 1 с, сообщение, запись в журнал «попытка открыть запрещённое»; экран не блокируется.
- FR-20 (список/решения/массовая смена/«Отменить» 10 с/время за неделю), FR-2 (шаг «Программы» мастера: сканирование ДО настройки).

## 2. Ruling оркестратора (см. план: RM3-1, RM3-3, RM3-10, RM3-12 — действуют дословно)

- **P-1 Чистый пакет `internal/programs`** (только стандартная библиотека, без файловой системы и Windows): 
  - `Normalize(path string) string` — нижний регистр (`strings.ToLower`), слеши `/` → `\`, `filepath`-независимая очистка (`..`, двойные разделители, хвостовой `\`), префикс `\\?\` снимается; пустой/относительный путь → `""` (непригоден).
  - `type Roots struct{ SystemRoot, ProgramFiles, ProgramFilesX86, InstallDir string }` (уже нормализованные; пустые игнорируются); методы: `IsTrusted(path) bool` (путь начинается с `<корень>\` ТОЧНО по границе разделителя: `c:\program files games\x.exe` НЕ доверенный), `IsSystem(path) bool` (RM3-1 п.1: `SystemRoot\`, `InstallDir\`, `...\windowsapps\`, `<ProgramFiles*>\microsoft\edgewebview\`), `AppRoot(path) string` (первая папка внутри `ProgramFiles`/`ProgramFilesX86`, т.е. `c:\program files\brave software`; вне них — `""`).
  - `Catalog` (встроенная таблица по имени файла в нижнем регистре): `Games` (не менее 40 известных: `minecraft.exe`, `minecraftlauncher.exe`, `javaw.exe` НЕ включать, `robloxplayerbeta.exe`, `steam.exe`, `epicgameslauncher.exe`, `fortniteclient-win64-shipping.exe`, `valorant.exe`, `leagueclient.exe`, `genshinimpact.exe`, `gta5.exe`, `csgo.exe`, `cs2.exe`, `dota2.exe`, `terraria.exe`, `among us.exe`, `osu!.exe`, `hearthstone.exe`, `wow.exe`, `overwatch.exe`, `brawlhalla.exe`, `rocketleague.exe`, `fallguys_client_game.exe`, `battle.net.exe`, `origin.exe`, `eadesktop.exe`, `ubisoftconnect.exe`/`upc.exe`, `gog galaxy`-ы и т.п.; подбери разумно, каждый — с комментарием-названием), `Browsers` (`brave.exe`, `chrome.exe`, `msedge.exe`, `firefox.exe`, `opera.exe`, `vivaldi.exe`, `iexplore.exe`; в M3 категория всех — `other`, RM3-2; флаг `IsBrowser(name)` для M4), `func (c Catalog) Lookup(name string) (category string, ok bool)`. Действует **только в доверенных каталогах** (RM3-1 п.4).
  - `type Classifier struct` (потокобезопасный, `Reload(programs []Entry)`): `Classify(path, fingerprint string) Result`, где `Result{Verdict Verdict; Category string; Known bool; Name string; WasCategory string}`, `Verdict ∈ {VerdictSystem, VerdictAllow(Category), VerdictForbidden, VerdictUnknown}`. Порядок строго по RM3-1: система → точный путь (для `Entry.Fingerprint != ""` и несовпадения отпечатка → `VerdictUnknown` с `WasCategory`) → корень приложения (категория корня = категория любой программы с этим `AppRoot`; если у корня разные категории — наименее строгая? НЕТ: `games` > `other`; `study` для вспомогательных не наследуется, берётся `other`) → каталог (только если `IsTrusted`) → `VerdictUnknown`, если путь вне `SystemRoot` и непустой; пустой/непригодный путь → `VerdictSystem` (не трогать, PL-9). Категория `blocked` → `VerdictForbidden`; `system` → `VerdictSystem`.
  - Таблично-тестовые случаи (литералы): переименованный `chrome.exe` в `c:\users\kid\downloads\` → Unknown; тот же в `c:\program files\google\chrome\application\` → Allow(other); `c:\program files games\x.exe` → Unknown (не доверенный); `c:\program files\brave software\brave-browser\application\123.0\updater.exe` → Allow(other) через корень; `c:\windows\system32\notepad.exe` → System; `c:\program files (x86)\microsoft\edgewebview\application\1.2.3\msedgewebview2.exe` → System; путь с `..`; путь в верхнем регистре; 8.3 `RUNNER~1` (нормализатор НЕ разворачивает — это делает адаптер Windows; тест фиксирует, что `~1`-путь вне доверенных → Unknown, т.е. защита от обхода: адаптер обязан развернуть).
- **P-2 Порты (в `service.Config`, фейки в `fakes_test.go`):** `Scanner interface{ Scan() ([]FoundProgram, error) }` (`FoundProgram{Path, Name, Publisher string}`; Windows-реализация — T4b), `FileMeta func(path string) (publisher, description, fingerprint string)`. `Roots` приходят в `Config` (в T4b — из KnownFolders).
- **P-3 Снимок (RM3-10):** `Service.seedPrograms()` при старте: если в `settings` нет `programs.seeded`: `Scanner.Scan()` → записи (`Category`: каталог → `games`/`other`, остальное `other`; `source`: `catalog` для каталожных, иначе `snapshot`; `AppRoot` по `Roots`), `Store.SeedPrograms`, затем `settings["programs.seeded"]="1"`. Ошибка сканирования → журнал работы, флага нет, **наблюдатель не закрывает ничего** (экспорт `Service.programsReady() bool` для T4a). Тесты: чистая база → снимок → флаг; повтор → второй скан не вызывается; ошибка скана → `programsReady() == false`; решения родителя не затираются повторным сидированием.
- **P-4 Обработчики** (заменяют заглушки T3a; права `parentToken`; ошибки на русском через `ipc.Fail`):
  - `hListPrograms`: `Pending` (из `ListPending`, имена детей из `s.children`), `Programs` = все записи, кроме `system` и `catalog/snapshot` в категории `other`, у которых нет секунд за неделю? — НЕТ: показывай ВСЕ, кроме категории `system` (фильтр — на фронте); `WeekS` из `ProgramSeconds(последние 7 дней по местному дню)`. Отсортируй: ждущие — по последней попытке, программы — по имени.
  - `hDecideProgram {Path, Category}`: категории `games|study|other|blocked` (иначе отказ); путь нормализуется; если путь в ждущих — `ResolvePending` + `UpsertProgram` (`source=parent`, `DecidedAt`, `Fingerprint` через `FileMeta` **только если путь вне доверенных**, `AppRoot`); если программа уже есть — меняется категория; иначе отказ «Такой программы нет в списке». После — `classifier.Reload`; журнал R22 (`program_decided`: «Вы отнесли «Name» к категории «Игры»» — родителю на «вы»); ребёнку, который пытался (`ByChild`) и чей сеанс сейчас есть, — плашка `BlockedDecided` через очередь команд стража (`guardCmd`, по образцу `noticeGuards`): «Родитель разрешил «Name»: это игра. Запусти её снова» / «…: это учёба.» / «…: это обычная программа.» / «Родитель запретил «Name»»; ответ: `Message` («SuperGame: игра»), `UndoID`.
  - `hSetProgramsCategory {Paths, Category}` — массово, те же правила, один `UndoID`, до 200 путей; журнал одной записью.
  - `hUndoProgram {UndoID}`: возвращает прежнее состояние (`RestoreProgram`/`RestorePending`/`DeleteProgram`) в течение `undoLife` (10 с, как у доп. времени; используй тот же механизм сроков и часы `c.real`); просроченное или чужое — «Отменить уже нельзя».
  - `hScanPrograms` (до настройки, повышенный администратор): `Scanner.Scan()` + `FileMeta` → `SuggestedCategory` (каталог / `other`); не больше 300 записей (приложения с каталогом и играми вперёд).
  - `hSetup` применяет `SetupRequest.Programs`: `UpsertProgram` (`source=parent`) для отмеченных (категории `games|study|other|blocked`), затем `seedPrograms()` для остальных (RM3-10 — тот же снимок, флаг).
- **P-5 Журнал (R22):** виды событий `program_decided`, `program_pending` («Программа «X» ждёт решения»), `program_forbidden_try` («Миша: попытка открыть запрещённое: «X»»), `program_closed_games` («Миша: закрыта «X»: время на игры закончилось»), `program_closed_study_only` («Миша: закрыто «X»: время вышло, доступна только учёба»; **не нарушение**) — тексты в `service/journaltext.go:eventText` (числа из базы — `detInt`/`detStr`); запись самих событий делает T4a, тексты и тесты `eventText` — ты.
- **P-6 Отпечаток:** строка `"<size>:<mtime unix>"`; формирует `FileMeta` (в T4b), ты — только хранение/сравнение.

## 3. Задачи (RED → GREEN → мутации → коммит)

T3b.1 `internal/programs`: `Normalize`, `Roots`, `Catalog`, `Classifier` + таблица случаев P-1. Мутации: убрать проверку границы разделителя в `IsTrusted` → падает случай `program files games`; пустить каталог для недоверенных → падает переименованный `chrome.exe`; убрать сверку отпечатка → падает тест подмены; поменять порядок «система» и «точный путь» → падает `msedgewebview2`.
T3b.2 Порты и снимок (P-2, P-3).
T3b.3 Обработчики (P-4), журнал (P-5), `hSetup`.
T3b.4 Права/контракт: `commands_test.go` (`wantAllowed`), фронтовый мок (`__mocks__/backend.ts`) соответствует новым ответам (только заглушки, если ты их менял).

## 4. Review Focus

1. Граница доверенных каталогов (RM3-1): `c:\program files games\`, `c:\program files.evil\`, регистр, `..`, `\\?\`, UNC-пути (`\\server\share\x.exe` — не доверенный, Unknown).
2. Решение действует для всех детей; смена категории запущенной сейчас программы — на следующем `Classify` (Reload под блокировкой, без гонок: `go test -race`).
3. «Отменить» не восстанавливает чужое/просроченное; отмена массовой операции восстанавливает всё; отмена после повторной попытки ребёнка не теряет счётчик.
4. Нет доверия к имени из ребёнка: имя берётся из файла/пути, не из снимка стража; длинные строки обрезаются (в журнале ≤ 120 символов).
5. Снимок: ошибка скана ≠ «пусто, значит всё незнакомое» (RM3-10, R18); повторное сидирование не затирает решения.
6. Тексты: ребёнку «ты», родителю «вы»; в журнале нарушением помечается только `program_forbidden_try` (R13), закрытие по лимиту — нет.

## 5. Вердикт (≤ 40 строк)

`done/blocked/failed`; Diff (sha + строка); Evidence (команды и итоги; мутации и какой тест упал; что проверит CI); Rulings; Escalations; публичные имена `internal/programs` и `Service` (`programsReady`, `classifier`, порты), нужные T4a/T4b.
