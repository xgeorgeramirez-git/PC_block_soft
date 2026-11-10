# Пакет: T3 вехи M4 «Хранилище и контракт канала» | слот: писатель | исполнитель: новый агент

Репозиторий `/home/claude/pc_block_soft`, ветка `feature/m4-sites`. Прочитай `plans/2026-11-04-M4.md`: Global Constraints, RM4-4, RM4-5, RM4-6, RM4-7, RM4-8, RM4-9, RM4-11, RM4-12, Review Focus п. 3, 7, 9, строку T3. ТЗ (`docs/01_ТЗ.md`): R6, R14, R15, FR-2, FR-16, FR-21, 8.2.1, 8.2.2, 8.2.5, 8.3.5.

**Предложение читателя: разбить T3 на два захода одного или двух писателей подряд** (пакет общий, вердикты раздельные):
- **T3a — хранилище и миграция** (разделы A–C): `internal/storage`, порт `service.Store`, адаптер `winhost`, фейк службы. Локально проверяется только служба; `internal/storage` и `winhost` — только `ci.yml` (см. «Среда»).
- **T3b — контракт канала и фронт-типы** (разделы D–F): `internal/ipc`, строки прав и заглушки в `internal/service`, `frontend/src/{types,api}.ts`, `contract.test.ts`, `__mocks__/backend.ts`. Фронт проверяет только `ci.yml` (RM4-0).
T3a и T3b не зависят друг от друга; T4 нужны оба. Если оркестратор не делит — порядок A → F, коммит на каждый раздел.

## Бриф

Фундамент для T4…T9: в базе — один домен в одном списке, стартовый набор «Видео и соцсети», браузеры каталога запрещены, лимит видео по дням, переключатель «18+» по детям, учёт секунд по сайтам; в канале — весь контракт M4, чтобы служба (T4), страж (T7) и экраны (T8, T9) не меняли его в обратную сторону. Логику (решение по домену, закрытие вкладки, обработчики сайтов, правила Brave, слияние магазина) делают T4…T6 — здесь только хранение, типы, тексты предупреждений и подсказки значка (они живут в `ipc/status.go`).

Перед работой сверь адреса ниже (PL-4): `git log --oneline -20`, `grep -n "games_min\|GamesMin" internal/storage/data.go internal/ipc/*.go internal/service/{handlers,service}.go`.

## Образец в коде (проверено читателем, адреса для сверки)

- Схема: `internal/storage/migrations/001_init.sql:19` (`day_limits.video_min` уже есть, CHECK 0…1440), `:9` (`children.adult_filter INTEGER NOT NULL DEFAULT 1`), `:45-50` (`site_rules` с `UNIQUE (domain, kind)`), `:51-59` (`usage_daily`: PK уже `(child_id, day, category, subject_kind, subject)`, `subject_kind IN ('program','site')`). Последняя миграция — `008_rest_break.sql`; формат шапки — две строки `-- Назначение: …` / `-- Правки схемы идут новой миграцией, 001–008 не переписываются (L-131).`
- Применение: `internal/storage/migrate.go:124` `applyOne` — файл целиком в одной транзакции вместе с записью в `schema_migrations`. Счётчика «последней версии» в коде нет, но он есть в тестах (см. A.4).
- Лимиты: `internal/storage/data.go:141` `SetDayLimits` (комментарий «Поле M4 video_min не трогается» — переписать), `:180` `GetDayLimits`; `nullableMin` `:165`; `maxLimitMin` `:137`. Тест `internal/storage/data_test.go:305` `TestSetGetDayLimitsRoundTrip` проверяет, что `video_min` **не** трогается (стр. 316, 358) — переписать на новое поведение.
- Служба собирает `core.Rules` в `internal/service/service.go:399` `readRules` из `GetDayLimits` целиком — `VideoMin` дойдёт до ядра сам, менять не нужно. Обратный путь: `handlers.go:160-182` (разбор `WeekdayLimit` → `core.DayLimit`, игры по `GamesField`) и `service.go:637` `limitsView` (ядро → `WeekdayLimit`) — см. D.2.
- Дети: `storage.Child` `data.go:14`, `ListChildren` `:47` (SELECT без `adult_filter`); `service.Child` `internal/service/ports.go:20`; адаптер `internal/service/winhost/adapters_windows.go:41`; фейк `internal/service/fakes_test.go:486` (`UpsertChild`), `:490` (`ListChildren`).
- Учёт: `internal/storage/programs.go:35-48` (`UsageVideoSocial = "video_social"`, `SubjectSite = "site"`, `usageKinds` уже с `site`), `addUsage` `:554` (ON CONFLICT по всем пяти полям — **ключ уже включает `subject_kind`, менять не нужно**), `SaveDayAndUsage` `:581` (пишет любые `UsageRow` с `Kind` — сайты пройдут без изменений), `ProgramSeconds` `:617`, `DayUsage` `:646` и `PeriodUsage` `:694` фильтруют `subject_kind = 'program'`.
- Порт: `internal/service/ports.go:117` `type Store interface`; три места на каждый новый метод — `ports.go`, `winhost/adapters_windows.go` (`storeAdapter`), `fakes_test.go` (`fakeStore`, ключ учёта уже `"id|день|категория|вид|предмет"`).
- Канал: `internal/ipc/commands.go:90` `AllCommands`, `:126-129` `LimitField`/`GamesField`, `:211` `ChildStatus`, `:268` `WeekdayLimit`, `:277` `SetupChild`, `:284` `SetupRequest`, `:339` `TodayCard`, `:365` `ChildRules`, `:545` `MyTodayReply`, `:572` `ProgramTime`, `:591` `SensorSample`, `:648-653` `WarnEvent*`, `:657` `GuardCmd*`, `:664-678` `Test*`, `:688` `GuardCommand`, `:790` `GuardStateReply` (T0 уже добавил `ForegroundDomain`, стр. 813); `internal/ipc/commands_m3.go` — образец файла и `BlockedPlate`/`Blocked*` (стр. 46-66), `MaxForegroundPathLen` (стр. 41).
- Тексты предупреждений: `internal/ipc/status.go:238` `NewGuardWarningFor`, `:262` `warnEventName` (для `core.WarnVideo` сейчас падает в `default` → `"limit"`), `:276` `warnSubject`, `:290` `warnOneMinute`; подсказка значка `:96-99` и `hasGamesLimit` `:141`; `GamesCardText` `:148`; `ChildStatusAdmin`/`ChildStatusNotChild` `:156-167`. Тексты **табло** игр в страже (`internal/guard/logic.go:256-258`, ветки `WarnEventGames` стр. 594, 614, 651, 662, 1230, 1267) — это **T7**, здесь не трогать.
- Контракт-тест: `internal/ipc/contract_test.go:59` `weekLimits`, `:67` `okStatus`, `:96` `card`, `:156` `examples()`, `:475` `len(AllCommands) != 66`; `-update` пересоздаёт `testdata/contract.json`. Права: `internal/service/commands.go:186-290` `commandRules`, тесты `commands_test.go:31` `TestEveryChannelCommandHasRightsRule` (число строк = число команд) и `:86` `wantAllowed`.
- Фронт: `frontend/src/api.ts:162` `withGames`, `:166` `setup`, `:297` `myTodayOf` (собирает объект **явно** по полям: новое поле ответа без строки здесь — красный `contract.test.ts`), `frontend/src/types.ts:95` `WeekdayLimit`, `:44` `ChildStatus`, `:154` `TodayCard`, `:369` `MyTodayReply`, `:402` `ProgramTime` (категория `video_social` уже есть), `:639` `SetupProgram`; `contract.test.ts:37` `CASES`, `:129` `payloadOf`, `:150` отказы, `:192` «полнота»; `__mocks__/backend.ts:589` `PARENT_COMMANDS`, `:1373` неизвестная команда отдаёт пример из контракта.
- Каталог браузеров: `internal/programs/catalog.go:67` `catalogBrowsers` — `brave.exe`, `chrome.exe`, `msedge.exe`, `firefox.exe`, `opera.exe`, `vivaldi.exe`, `iexplore.exe`, `browser.exe` (Яндекс). Ключ `programs.exe_path` — полный путь нижним регистром с `\` (`internal/programs/path.go:20` `Normalize`).
- Пакет сайтов: `internal/sites/domain.go:13` `MaxDomainLen = 253`; `lists.go:13` `Kind.String()` → `"blocked"`, `"study"`, `"video_social"` (те же ключи, что `site_rules.kind`).

## Ruling читателя (оркестратор подтверждает до пуска; цена ошибки — в скобках)

- **S-1 Дубли `site_rules`.** При пересоздании таблицы у домена остаётся одна запись с наименьшим рангом `blocked` (1) < `study` (2) < `video_social` (3), при равном — меньший `id`; `id` сохраняется. Обоснование: запрет строже (PL-30: из двух действует строгий), учёба сильнее видео, потому что учёба не считается, а видео ограничено — «перенос в строгую сторону» не открывает ребёнку ничего нового. На практике в базах 0.3.3 таблица пуста (экрана «Сайты» не было), правило нужно для честности миграции (цена: ноль у заказчика).
- **S-2 Браузеры в миграции — по имени файла** (`exe_path LIKE '%\<имя>'`, обратная косая в SQLite в `LIKE` без `ESCAPE` — обычный символ; `_` в именах нет), только у `source IN ('catalog','snapshot')`, любая категория (в том числе `system`) → `blocked`; `hidden`, `custom_name`, `source`, `decided_at` не меняются (запрет сильнее скрытия, PL-30). `brave.exe` не трогается; `msedge_proxy.exe` и `msedgewebview2.exe` не совпадают с `%\msedge.exe` (PL-18). Риск: `browser.exe` — общее имя; чужая программа с таким именем из снимка станет «Запрещено» (родитель вернёт в «Программах»; то же правило даст каталог T6 по RM4-8) — цена мелкая, отметить в вердикте.
- **S-3 Отсутствующее поле видео = без лимита.** У `WeekdayLimit` поля `VideoMin int` / `VideoUnlimited bool` по образцу игр, **но** `UnmarshalJSON` ставит `VideoUnlimited = true`, если ключа `videoUnlimited` в JSON нет. Иначе старый фронт (экраны получат видео только в T9), PowerShell-приёмка и мастер присылали бы «видео нельзя» (ловушка, которую в M3 закрывал `withGames` на фронте). Цена ошибки без этого: после T4 любое «Сохранить» в «Лимитах» запрещает видео. `withGames` на фронте **не** менять: точные `toEqual` отправленных лимитов в `ChildLimits.test.tsx:196…673`, `Wizard.test.tsx:327-333`, `api.test.ts:168` сломались бы без возможности проверить локально.
- **S-4 Мастер.** `SetupRequest.Sites *SetupSites` (`omitempty`): нет — 18+ включён всем, «Видео и соцсети» — стартовый набор миграции. Есть — `AdultOff []string` (SID детей с выключенным 18+; отрицание, чтобы ноль был безопасным) и `Video []string` — **полный** список «Видео и соцсетей» после правок родителя (служба заменяет им список, T4). Стартовый набор в Go — `ipc.DefaultVideoSites` (для фронта T9 и примера контракта).
- **S-5 Удаление и «Отменить».** Отдельной команды отмены нет: `RemoveSiteReply` возвращает `Domain` и `Kind`, «Отменить» на фронте — `AddSite(domain, kind)` (меньше состояния в службе; цена: при 1000 запрещённых отмена удаления невозможна не бывает — удаление освобождает место).
- **S-6 Дубль и предел — отказы у поля.** «Этот сайт уже в списке», «Это не похоже на адрес сайта», «В списке запрещённых уже 1000 сайтов. Удалите ненужные» — `Fail(text, FieldDomain)`; перенос — успех с `Moved: true` и «Сайт перенесён в «Учёбу»». Предел 1000 проверяет служба (T4) через `CountSiteRules`, хранилище предела не знает (служба — единственный писатель).
- **S-7 Учёт сайтов в «Итогах».** `DayUsage`/`PeriodUsage` начинают отдавать и `site`-строки (`Kind` заполнен), порядок `category, subject_kind, subject`; `ProgramSeconds` остаётся только про программы. До T4 сайтовых строк в базе нет, поэтому поведение службы не меняется.

## Files

T3a — Create: `internal/storage/migrations/009_sites_m4.sql`, `internal/storage/sites.go`, `internal/storage/sites_test.go` (тест миграции 009 и методов сайтов). Modify: `internal/storage/data.go`, `data_test.go`, `programs.go` (`DayUsage`, `PeriodUsage`), `programs_test.go` (если там тесты `DayUsage`), `store_test.go`, `migrate_m3_test.go:135`, `hidden_test.go:70`, `lastminute_test.go:60`, `customname_test.go:68`, `rest_test.go:58`, `internal/service/ports.go`, `internal/service/winhost/adapters_windows.go`, `internal/service/fakes_test.go`.
T3b — Create: `internal/ipc/commands_m4.go`, `internal/ipc/status_m4_test.go`, `internal/service/handlers_m4.go` (заглушки). Modify: `internal/ipc/commands.go` (`AllCommands`, поля в существующих типах), `status.go`, `contract_test.go`, `testdata/contract.json` (только через `-update`), `internal/service/commands.go`, `commands_test.go`, `handlers.go` и `service.go` (только D.2), `frontend/src/types.ts`, `api.ts`, `contract.test.ts`, `__mocks__/backend.ts`.
Не трогать: `internal/core`, `internal/sites`, `internal/guard`, `internal/systray`, экраны `frontend/src/screens/*`, `lib/*`, `docs/`, `plans/`, версию.

## Шаги (каждый: RED → GREEN → мутации → коммит)

### A. Миграция 009 (T3a)

A.1. Файл `internal/storage/migrations/009_sites_m4.sql` ровно такого содержания по смыслу (комментарии — свои, по-русски):

```sql
-- Назначение: веха M4 «Сайты и Brave»: один домен — один список, стартовый набор «Видео и соцсети», браузеры каталога запрещены.
-- Правки схемы идут новой миграцией, 001–008 не переписываются (L-131).

CREATE TABLE site_rules_new (
  id INTEGER PRIMARY KEY,
  domain TEXT NOT NULL UNIQUE CHECK (length(domain) BETWEEN 1 AND 253),
  kind TEXT NOT NULL CHECK (kind IN ('blocked','study','video_social'))
);
INSERT INTO site_rules_new (id, domain, kind)
  SELECT r.id, r.domain, r.kind FROM site_rules r
  WHERE r.id = (SELECT r2.id FROM site_rules r2 WHERE r2.domain = r.domain
                ORDER BY CASE r2.kind WHEN 'blocked' THEN 1 WHEN 'study' THEN 2 ELSE 3 END, r2.id LIMIT 1);
DROP TABLE site_rules;
ALTER TABLE site_rules_new RENAME TO site_rules;

INSERT OR IGNORE INTO site_rules (domain, kind) VALUES
  ('youtube.com','video_social'), ('vk.com','video_social'), ('tiktok.com','video_social'),
  ('rutube.ru','video_social'), ('ok.ru','video_social'), ('telegram.org','video_social'),
  ('web.telegram.org','video_social'), ('discord.com','video_social'), ('twitch.tv','video_social');

UPDATE programs SET category = 'blocked'
  WHERE source IN ('catalog','snapshot') AND (
    exe_path LIKE '%\chrome.exe' OR exe_path LIKE '%\msedge.exe' OR exe_path LIKE '%\firefox.exe' OR
    exe_path LIKE '%\opera.exe' OR exe_path LIKE '%\vivaldi.exe' OR exe_path LIKE '%\iexplore.exe' OR
    exe_path LIKE '%\browser.exe');
```
Колонки `site_rules` остаются `id, domain, kind` в том же порядке (`TestOpenCreatesFullSchema` не меняется). `video_min` не трогается: `NULL` = без лимита. Флаг в `settings` не нужен.

A.2. RED `TestMigration009OnFilled008Base` (в `sites_test.go`, по образцу `customname_test.go:15` `TestMigration007OnFilled006Base`: `loadMigrations` → миграции `version < 9` в `rawDB(t)` через `applyMigrations(db, v008, 100)` → данные → `applyMigrations(db, all, 200)`; помощники `rowStrings` — `migrate_m3_test.go:10`, `versions` — `migrate_test.go:30`). Данные 0.3.3:
- `children`: `(1,'S-1','Миша',10)` (adult_filter по умолчанию), `(2,'S-2','Даша',10)` с `adult_filter = 0`;
- `day_limits`: `(1,1,120,60,NULL)`, `(1,2,NULL,NULL,NULL)`;
- `site_rules` по порядку id 1…6: `example.com/blocked`, `example.com/study`, `kids.example/video_social`, `kids.example/study`, `youtube.com/study`, `school.example/study`;
- `programs` (`exe_path, display_name, category, source, hidden, custom_name, decided_at`), по порядку:
  1. `c:\program files\microsoft\edge\application\msedge.exe`, `Microsoft Edge`, `other`, `catalog`, 0, '', NULL
  2. `c:\program files\google\chrome\application\chrome.exe`, `Google Chrome`, `other`, `snapshot`, 1, '', NULL
  3. `c:\program files\mozilla firefox\firefox.exe`, `Mozilla Firefox`, `study`, `parent`, 0, '', 500
  4. `c:\program files\bravesoftware\brave-browser\application\brave.exe`, `Brave`, `other`, `snapshot`, 0, '', NULL
  5. `c:\program files (x86)\microsoft\edgewebview\application\130.0.2849.80\msedgewebview2.exe`, `WebView2`, `system`, `catalog`, 0, '', NULL
  6. `c:\program files (x86)\microsoft\edge\application\msedge_proxy.exe`, `msedge_proxy`, `other`, `snapshot`, 0, '', NULL
  7. `c:\users\миша\appdata\local\yandex\yandexbrowser\application\browser.exe`, `Яндекс Браузер`, `games`, `snapshot`, 0, `Яндекс`, NULL
  8. `c:\tools\xmsedge.exe`, `xmsedge`, `other`, `snapshot`, 0, '', NULL
  9. `c:\program files\internet explorer\iexplore.exe`, `Internet Explorer`, `system`, `catalog`, 0, '', NULL
- `day_state (1,'2026-11-04',3600,…)`, `usage_daily (1,'2026-11-04','other','program','c:\tools\xmsedge.exe',60)`, `settings ('bonus.rate','30')`.

Expected:
- `SELECT domain, kind FROM site_rules ORDER BY domain` = `[[discord.com video_social] [example.com blocked] [kids.example study] [ok.ru video_social] [rutube.ru video_social] [school.example study] [telegram.org video_social] [tiktok.com video_social] [twitch.tv video_social] [vk.com video_social] [web.telegram.org video_social] [youtube.com study]]` (12 строк; youtube.com остался «Учёбой» — `INSERT OR IGNORE`);
- `SELECT id, domain FROM site_rules WHERE id <= 6 ORDER BY id` = `[[1 example.com] [4 kids.example] [5 youtube.com] [6 school.example]]`;
- `SELECT category, hidden, custom_name, source, ifnull(decided_at,'NULL') FROM programs ORDER BY id` = `[[blocked 0  catalog NULL] [blocked 1  snapshot NULL] [study 0  parent 500] [other 0  snapshot NULL] [system 0  catalog NULL] [other 0  snapshot NULL] [blocked 0 Яндекс snapshot NULL] [other 0  snapshot NULL] [blocked 0  catalog NULL]]` (пустой `custom_name` — пустая строка в срезе);
- `children` (`id, adult_filter`) = `[[1 1] [2 0]]`; `day_limits` (`child_id, weekday, ifnull(total_min,'NULL'), ifnull(games_min,'NULL'), ifnull(video_min,'NULL')`) = `[[1 1 120 60 NULL] [1 2 NULL NULL NULL]]`; `day_state`, `usage_daily`, `settings` — равны снятым до миграции;
- вставка `('youtube.com','blocked')` после миграции — ошибка UNIQUE;
- повторное `applyMigrations(db, all, 300)` — строк `site_rules` по-прежнему 12, программы те же; `versions` = `{1,100}…{8,100},{9,200}`.

A.3. RED `TestFreshBaseHasVideoStarterSet`: `openTemp(t)` → `SELECT domain FROM site_rules WHERE kind = 'video_social' ORDER BY domain` = `[discord.com ok.ru rutube.ru telegram.org tiktok.com twitch.tv vk.com web.telegram.org youtube.com]`, других списков нет. В `TestSchemaConstraints` (`store_test.go:125`) добавь отказы: «повтор домена в другом списке» `INSERT INTO site_rules (domain, kind) VALUES ('youtube.com','blocked')`; «пустой домен» `('', 'study')`; «домен длиннее 253» — `strings.Repeat("a", 250) + ".com"` (254); «список вне перечня» `('a.com','video')`.

A.4. Счётчики миграций в старых тестах (иначе CI красный): `store_test.go:216-224` `TestOpenTwiceKeepsDataAndMigrationOnce` — `8` → `9` (два места) и цикл `{2,…,8}` → `{2,…,9}`; списки `versions` в `migrate_m3_test.go:135`, `hidden_test.go:70`, `lastminute_test.go:60`, `customname_test.go:68`, `rest_test.go:58` — дописать `{9, 200}` (у `rest_test.go` тоже `{9, 200}`: 009 применяется во втором вызове). Старые сравнения «до и после» данных не затронуты: в них нет браузеров, кроме `brave.exe`, и нет `site_rules` (проверено читателем; перепроверь `grep -n "site_rules\|msedge\|chrome" internal/storage/*_test.go`).

Мутации A: (а) `LIKE '%msedge.exe'` без `\` → строка 8 стала `blocked`; (б) `source IN ('catalog','snapshot','parent')` → Firefox `blocked`; (в) ранг `video_social` раньше `study` → `kids.example video_social`; (г) `INSERT OR REPLACE` вместо `OR IGNORE` → youtube.com `video_social`; (д) `UNIQUE` убран → вставка дубля прошла.

### B. Лимит видео, дети, сайты, учёт в хранилище (T3a)

B.1. `SetDayLimits`/`GetDayLimits` (`data.go:141`, `:180`): `video_min` пишется и читается вместе с `total_min`, `games_min` (`nil` ↔ NULL, 0 = нельзя); проверка 0…1440 — `"%w: лимит видео дня %d равен %d мин, допустимо от 0 до %d"`. Комментарии обеих функций — про три лимита. RED: перепиши `TestSetGetDayLimitsRoundTrip` (`data_test.go:305`): в `week` у пн `VideoMin: ptr(20)`, ср `VideoMin: ptr(0)`, у остальных `nil`; удали ручной `UPDATE … video_min = 20` и ожидание «поле M4 не трогаем»; Expected: `GetDayLimits == week`; `video_min` пн = 20, ср = 0 (не NULL), вт NULL; после `week2` (без видео) — пн NULL. Новый подтест: `VideoMin: ptr(1441)` в чт → `errors.Is(err, ErrInvalid)`, в базе ничего не изменилось.
B.2. Дети: `Child.AdultFilter bool` (комментарий «список 18+ включён, R14»), `ListChildren` читает `adult_filter <> 0`; `SetAdultFilter(childID int64, on bool) error` — нет ребёнка (`affectedOne`) → `ErrNotFound`. В `want` тестов `data_test.go:67-70, 94, 181` и `store_test.go:227` добавь `AdultFilter: true`. RED `TestSetAdultFilter`: новый ребёнок → `AdultFilter == true`; `SetAdultFilter(id, false)` → `false`, у второго ребёнка `true`; `SetAdultFilter(999, true)` → `ErrNotFound`; `RemoveChild` и `UpsertChild` того же SID значение не сбрасывают. Добавь `SetAdultFilter` в `TestMethodsReturnErrorsWhenStoreClosed` (`store_test.go:375`).
B.3. Сайты — `internal/storage/sites.go`:
```go
const (SiteBlocked = "blocked"; SiteStudy = "study"; SiteVideoSocial = "video_social")
type SiteRule struct { ID int64; Domain, Kind string }
func (s *Store) ListSiteRules() ([]SiteRule, error)            // ORDER BY kind, domain; нет — []SiteRule{}
func (s *Store) AddSiteRule(domain, kind string) (moved bool, prevKind string, err error)
func (s *Store) RemoveSiteRule(domain string) (prevKind string, ok bool, err error)
func (s *Store) CountSiteRules(kind string) (int, error)
func (s *Store) ReplaceSiteRules(kind string, domains []string) error // мастер: список kind целиком, одной транзакцией
```
Хранилище **не нормализует** домен (это `sites.NormalizeInput` в службе), только проверяет: пусто, длиннее 253, неизвестный список → `ErrInvalid` до записи. `AddSiteRule`: новой записи нет → вставка, `(false, "", nil)`; тот же список → ничего, `(false, kind, nil)`; другой → `UPDATE kind`, `(true, прежний, nil)` (S-1: один домен — одна строка, `id` сохраняется). `ReplaceSiteRules`: удалить все записи `kind`, затем каждый домен `INSERT … ON CONFLICT(domain) DO UPDATE SET kind = excluded.kind` (домен из другого списка переносится); дубли во входе — одна запись; ошибка любого домена — `ErrInvalid`, ничего не изменилось.
RED `TestSiteRules` (на `openTemp`, 9 доменов видео уже есть): `AddSiteRule("example.com","blocked")` → `(false,"",nil)`; повтор → `(false,"blocked",nil)`; `AddSiteRule("example.com","study")` → `(true,"blocked",nil)`; `AddSiteRule("youtube.com","study")` → `(true,"video_social",nil)`; `CountSiteRules("video_social") == 8`, `("blocked") == 0`, `("study") == 2`; `ListSiteRules()` первые две строки `{example.com study}`, `{youtube.com study}`, затем 8 видео по алфавиту; `RemoveSiteRule("example.com")` → `("study", true, nil)`, повтор → `("", false, nil)`; отказы `ErrInvalid`: `AddSiteRule("", "study")`, `AddSiteRule(strings.Repeat("a",250)+".com","study")`, `AddSiteRule("a.com","video")`, `CountSiteRules("x")`. RED `TestReplaceSiteRules`: `ReplaceSiteRules("video_social", []string{"youtube.com","kids.example","kids.example"})` → видео `[kids.example youtube.com]`; затем `AddSiteRule("school.example","study")` и `ReplaceSiteRules("video_social", []string{"school.example"})` → видео `[school.example]`, учёба `[]`; `ReplaceSiteRules("video_social", []string{"ok.ru",""})` → `ErrInvalid`, видео по-прежнему `[school.example]`. Все пять методов — в `TestMethodsReturnErrorsWhenStoreClosed`.
B.4. Учёт (S-7): `DayUsage`/`PeriodUsage` без фильтра `subject_kind`, `u.Kind` читается из базы, порядок `ORDER BY category, subject_kind, subject`; комментарии «Сайтов нет (M4)» заменить. RED `TestDayUsageWithSites`: `AddUsage(id,"2026-11-05","video_social","site","youtube.com",600)`, `AddUsage(id,"2026-11-05","games","program",`c:\games\game.exe`,300)`, тот же сайт ещё 60 → `DayUsage` = `[{2026-11-05 games program c:\games\game.exe 300} {2026-11-05 video_social site youtube.com 660}]`; `PeriodUsage(id,"2026-11-01","2026-11-06")` — те же строки с `Day: ""`; `ProgramSeconds("2026-11-01")` = `{c:\games\game.exe: 300}` (сайта нет). Проверь, что прежние тесты `DayUsage`/`PeriodUsage` (`grep -n "DayUsage\|PeriodUsage" internal/storage/*_test.go`) остаются зелёными без правки ожиданий; пришлось править — Escalations. `SaveDayAndUsage` с `UsageRow{Kind: SubjectSite}` — одна строка в тесте отката (что сайт откатывается вместе с днём).

Мутации B: `video_min` не в `UPDATE SET` → пн NULL после записи; `adult_filter` не читается → `TestSetAdultFilter`; `AddSiteRule` вставляет вторую строку вместо `UPDATE` → UNIQUE-ошибка/`Count`; `ReplaceSiteRules` без транзакции → видео пусто после отказа; `DayUsage` с прежним фильтром → нет строки сайта.

### C. Порт службы (T3a)

`internal/service/ports.go`: `Child.AdultFilter bool`; `type SiteRule struct{ Domain, Kind string }`; в `Store` после программ блок «Сайты (M4: R14, FR-21, RM4-5)»: `ListSiteRules() ([]SiteRule, error)`, `AddSiteRule(domain, kind string) (moved bool, prevKind string, err error)`, `RemoveSiteRule(domain string) (prevKind string, ok bool, err error)`, `CountSiteRules(kind string) (int, error)`, `ReplaceSiteRules(kind string, domains []string) error`, `SetAdultFilter(childID int64, on bool) error`. Комментарии `SaveDayAndUsage`/`DayUsage`/`PeriodUsage` — «программы и сайты». `storeAdapter` — сквозные вызовы + `AdultFilter` в `ListChildren` (стр. 48). `fakeStore`: `sites map[string]string` (домен → список; конструктор кладёт 9 доменов стартового набора, как миграция), `adultOff map[int64]bool` (нет записи — включён), `ListChildren` заполняет `AdultFilter`; методы с той же семантикой, что B.3 (без проверок длины — их держит хранилище). Тест фейка не нужен; `go test -race ./internal/service/...` зелёный.

### D. Контракт канала M4 (T3b)

D.1. Новый файл `internal/ipc/commands_m4.go` (шапка по образцу `commands_m3.go:1-5`):
```go
const (
	CmdListSites         = "ListSites"         // «Сайты»: списки и переключатели 18+ (родитель, FR-21)
	CmdAddSite           = "AddSite"           // добавить или перенести домен (родитель)
	CmdRemoveSite        = "RemoveSite"        // удалить домен; «Отменить» — AddSite (родитель)
	CmdSetAdultFilter    = "SetAdultFilter"    // «Блокировать сайты 18+» ребёнку (родитель)
	CmdTestSeedPrograms  = "TestSeedPrograms"  // тестовый режим: программы «до обновления» (RM4-11, S46)
)
const FieldDomain = "domain"
const (SiteKindBlocked = "blocked"; SiteKindStudy = "study"; SiteKindVideo = "video_social")
const MaxBlockedSites = 1000          // «N из 1000» и URLBlocklist Brave (R14, RM4-10)
const MaxDomainLen = 253              // SensorSample.ForegroundDomain длиннее — служба отбрасывает (RM4-4)
var DefaultVideoSites = []string{"youtube.com","vk.com","tiktok.com","rutube.ru","ok.ru","telegram.org","web.telegram.org","discord.com","twitch.tv"} // R14, как миграция 009
const GuardCmdCloseTab = "closeTab"   // закрыть активную вкладку Brave с доменом Domain (RM4-2)
const (BlockedSite = "site"; BlockedAdult = "adult"; BlockedVideoOver = "videoOver"; BlockedBrowser = "browser")
const WarnEventVideo = "video"        // исчерпание лимита видео (R6): остаток службы, как у игр
const TestDomain = "domain"           // TestGuard: подмена домена вкладки Text; «» снимает (RM4-12)
const (CloseTabCtrlW = "ctrlW"; CloseTabInvoke = "invoke"; CloseTabWMClose = "wmClose") // GuardStateReply.ClosedTabWay
type SiteDTO struct { Domain string `json:"domain"`; Kind string `json:"kind"` }
type AdultFilterDTO struct { ChildID int64 `json:"childId"`; Name string `json:"name"`; On bool `json:"on"` }
type ListSitesReply struct {
	Sites       []SiteDTO        `json:"sites"`       // по списку и домену
	Children    []AdultFilterDTO `json:"children"`    // переключатели «18+»
	BlockedMax  int              `json:"blockedMax"`  // MaxBlockedSites
	AdultSource string           `json:"adultSource"` // 8.3.5 п.3 дословно
}
type AddSiteRequest struct { Domain string `json:"domain"`; Kind string `json:"kind"` }   // Domain — ввод родителя как есть
type AddSiteReply struct { Message string `json:"message"`; Domain string `json:"domain"`; Moved bool `json:"moved"`; Sites ListSitesReply `json:"sites"` }
type RemoveSiteRequest struct { Domain string `json:"domain"` }
type RemoveSiteReply struct { Message string `json:"message"`; Domain string `json:"domain"`; Kind string `json:"kind"`; UndoUntilMs int64 `json:"undoUntilMs"`; Sites ListSitesReply `json:"sites"` }
type SetAdultFilterRequest struct { ChildID int64 `json:"childId"`; On bool `json:"on"` }
type SetAdultFilterReply struct { Message string `json:"message"`; Sites ListSitesReply `json:"sites"` }
type SetupSites struct { AdultOff []string `json:"adultOff"`; Video []string `json:"video"` } // S-4
type TestSeedProgramsRequest struct {
	Programs []TestSeedProgram      `json:"programs"`
	Usage    []TestSeedProgramUsage `json:"usage"`
	Pending  []TestSeedPending      `json:"pending"`
}
type TestSeedProgram struct { Path, Name, Publisher, Category, Source string; Hidden bool; CustomName string; DecidedAt int64 } // теги: path, name, publisher, category, source, hidden, customName, decidedAt
type TestSeedProgramUsage struct { ChildID int64; Day, Path, Category string; Seconds int }                                    // childId, day, path, category, seconds
type TestSeedPending struct { Path, Name, Publisher string; ChildID int64; Attempts int; LastAttemptAt int64 }                // path, name, publisher, childId, attempts, lastAttemptAt
```
Поля в существующих типах (`commands.go`, с комментариями по-русски):
- `SensorSample.ForegroundDomain string \`json:"foregroundDomain"\`` после `ForegroundPath` (только домен, R23; длиннее `MaxDomainLen` служба отбрасывает). `SampleFromCore`/`ToCore` не меняются.
- `GuardCommand.Domain string \`json:"domain,omitempty"\`` (для `GuardCmdCloseTab`).
- `GuardStateReply.ClosedTab string \`json:"closedTab"\`` (домен последней исполненной `closeTab`, «» — не было) и `ClosedTabWay string \`json:"closedTabWay"\`` (`CloseTab*`) — для приёмки S16…S18 (RM4-12); заполняет страж (T7).
- `WeekdayLimit.VideoMin int \`json:"videoMin"\``, `VideoUnlimited bool \`json:"videoUnlimited"\`` + `func (w *WeekdayLimit) UnmarshalJSON(b []byte) error` (S-3: `type plain WeekdayLimit; p := plain{VideoUnlimited: true}; json.Unmarshal(b, &p)`). `func VideoField(weekday int) string { return "video." + itoa(weekday) }` рядом с `GamesField`.
- `SetupRequest.Sites *SetupSites \`json:"sites,omitempty"\``.
- `ChildStatus`: `VideoRemainingS int` (`videoRemainingS`), `VideoLimitS int` (`videoLimitS`), `VideoUnlimited bool` (`videoUnlimited`), `CloseVideo bool` (`closeVideo`) — блок «Видео и соцсети (M4: R6)» после игр.
- `TodayCard.VideoText string \`json:"videoText"\`` («Видео 10 мин из 20 мин»; «» — отдельного лимита видео нет).
- `MyTodayReply.VideoS int \`json:"videoS"\`` (видео за период, с) после `GamesS`; комментарий `OtherS` уже говорит «без игр и видео».
- `ProgramTime.Kind string \`json:"kind"\`` (`"program"` | `"site"`; у сайта `Path` — домен, R23).
- `AllCommands` — отдельной строкой `CmdListSites, CmdAddSite, CmdRemoveSite, CmdSetAdultFilter, CmdTestSeedPrograms, // M4 (commands_m4.go)`.

D.2. Минимальная связка в службе, чтобы S-3 не врал в ответах (без логики видео): `service.go:637` `limitsView` — `VideoUnlimited: l.VideoMin == nil`, иначе `VideoMin = *l.VideoMin`; `handlers.go:172` — после игр тот же блок для видео с `ipc.VideoField(l.Weekday)` и теми же текстами `textLimitNegative`/`textLimitTooBig`, `week[i].VideoMin = &video`. RED в службе (`internal/service/limits_video_test.go`): `SaveLimits` с пн `{TotalMin:120, GamesUnlimited:true, VideoMin:30}` → `GetChildRules` пн `VideoMin 30, VideoUnlimited false`, у вт (без поля) `VideoUnlimited true`; запрос JSON-строкой без ключей видео (`h.do` с `json.RawMessage`) → все дни `VideoUnlimited true`; `VideoMin: 1441` → отказ с полем `video.1`. Остальное (статус, карточка, «Мои правила», копирование) — T4.

D.3. Тексты и статус — `internal/ipc/status.go`:
- `warnEventName`: `core.WarnVideo` → `WarnEventVideo`; `warnSubject`: `"время на видео и соцсети закончится"` (обе строки); `NewGuardWarningFor` порог 5: `core.WarnVideo` — в ветку `WarnForbidden, WarnPauseEnd` («Через 5 минут …», без «Сохрани игру»); `warnOneMinute`: `"Осталась 1 минута на видео и соцсети"`.
- `ChildStatusFrom`: четыре поля видео из `Decision` (`VideoRemainingS`, `VideoLimitS`, `VideoUnlimited`, `CloseVideo`); в подсказке после строки игр — `if hasVideoLimit(d) { s.Tooltip += "\nВидео: " + core.FormatDuration(core.RemainingMinutes(d.VideoRemainingS)*60) }`, `hasVideoLimit` — копия `hasGamesLimit` по видео. `ChildStatusAdmin`/`ChildStatusNotChild` — `VideoUnlimited: true`.
- `func VideoCardText(usedS, limitS int, unlimited bool) string` — копия `GamesCardText` со словом «Видео».
- RED `internal/ipc/status_m4_test.go`: `TestGuardWarningVideoTexts` — `NewGuardWarningFor(th, core.WarnVideo, th*60-7)`: 30 → `"Через 30 минут время на видео и соцсети закончится"`, 15 → `"Через 15 минут время на видео и соцсети закончится"`, 5 → `"Через 5 минут время на видео и соцсети закончится"`, 1 → `"Осталась 1 минута на видео и соцсети"`; `Event == "video"`, `Sound`, `EventS == th*60-7`, без длинных тире. `TestChildStatusVideoFields` — `Decision{Status: StatusOK, RemainingS: 85*60, LimitS: 7200, GamesRemainingS: 19*60+5, GamesLimitS: 3600, VideoRemainingS: 19*60+5, VideoLimitS: 1200}` → подсказка `"Осталось 1 ч 25 мин\nИгры: 20 мин\nВидео: 20 мин"`, поля скопированы; без игр (`GamesUnlimited: true`) → `"Осталось 1 ч 25 мин\nВидео: 20 мин"`; видео нельзя `{RemainingS: 600, LimitS: 3600, GamesUnlimited: true, CloseVideo: true}` → `"Осталось 10 мин\nВидео: 0 мин"`; `VideoUnlimited: true` → без строки видео; `ChildStatusAdmin("Оля").VideoUnlimited == true`, `ChildStatusNotChild().VideoUnlimited == true`. `TestVideoCardText`: `(600, 1200, false)` → `"Видео 10 мин из 20 мин"`, `(3900, 7200, false)` → `"Видео 1 ч 05 мин из 2 ч"`, `(0, 0, true)` → `""`. `TestWeekdayLimitVideoDefault`: `{"weekday":1,"totalMin":60,"unlimited":false}` → `VideoUnlimited true, VideoMin 0`; `{"weekday":1,"totalMin":60,"unlimited":false,"videoMin":30,"videoUnlimited":false}` → `30, false`; `{"weekday":1,"videoMin":30}` → `VideoUnlimited true` (нет ключа — без лимита); круг `Payload(w)` → `Unmarshal` → равенство. `TestMaxDomainLenMatchesSites`: `ipc.MaxDomainLen == sites.MaxDomainLen` (импорт `internal/sites` только в тесте) и `strings.Join(DefaultVideoSites, " ")` равен литералу из 9 доменов в порядке R14. Старые `TestChildStatusGamesFields`, `TestGuardWarningGamesTexts`, `TestChildStatusFromDecision` — зелёные без правки.

D.4. Примеры `contract_test.go:examples()` (карта; порядок в файле наводит `marshalContract`):
- `weekLimits()`: у всех дней `VideoUnlimited: true`, кроме пн `VideoMin: 60` и ср `VideoMin: 0` (оба `VideoUnlimited: false`).
- `CmdListSites` — `sitesReply()`: `Sites: [{example.com blocked} {school.example study} {youtube.com video_social}]`, `Children: [{1 Миша true} {2 Даша false}]`, `BlockedMax: 1000`, `AdultSource: "Список сайтов для взрослых: UT1, Университет Тулуза Капитоль, лицензия CC BY-SA 4.0"`; `"ListSites#empty"` — `ListSitesReply{Sites: []SiteDTO{}, Children: []AdultFilterDTO{}, BlockedMax: 1000, AdultSource: тот же}`.
- `CmdAddSite`: `AddSiteRequest{Domain: "https://www.Example.com/page?x=1", Kind: SiteKindBlocked}` → `AddSiteReply{Message: "Добавлено: example.com", Domain: "example.com", Sites: sitesReply()}`; `"AddSite#moved"`: `{Domain: "youtube.com", Kind: SiteKindStudy}` → `{Message: "Сайт перенесён в «Учёбу»", Domain: "youtube.com", Moved: true, Sites: sitesReply()}`; отказы `"AddSite#invalid"` (`{Domain: "просто текст", Kind: SiteKindBlocked}`, `"Это не похоже на адрес сайта"`, `FieldDomain`), `"AddSite#dup"` (`"Этот сайт уже в списке"`), `"AddSite#full"` (`"В списке запрещённых уже 1000 сайтов. Удалите ненужные"`).
- `CmdRemoveSite`: `{Domain: "example.com"}` → `{Message: "Удалён example.com", Domain: "example.com", Kind: SiteKindBlocked, UndoUntilMs: t0 + 10_000, Sites: sitesReply()}`.
- `CmdSetAdultFilter`: `{ChildID: 2, On: false}` → `{Message: "Даша: сайты 18+ не блокируются", Sites: sitesReply()}`.
- `"Setup#sites"`: как `CmdSetup` + `Sites: &SetupSites{AdultOff: []string{"S-1-5-21-111-222-333-1002"}, Video: []string{"youtube.com", "vk.com", "kids.example"}}`; `CmdSetup` остаётся без `Sites`.
- `"GuardPoll#closeTab"`: запрос `SensorSample{AtMs: t0, SessionActive: true, Foreground: CatNameVideoSocial, ForegroundPath: \`c:\program files\bravesoftware\brave-browser\application\brave.exe\`, ForegroundDomain: "youtube.com"}`; ответ `Status: okStatus()`, `Warnings: []GuardWarning{NewGuardWarningFor(5, core.WarnVideo, 300)}`, `Commands: [{Kind: GuardCmdCloseTab, Domain: "youtube.com"}, {Kind: GuardCmdBlocked, Plate: &BlockedPlate{Kind: BlockedVideoOver, Text: core.VideoOverPlateText(2400, false)}}]`.
- `"GuardState#closeTab"`: `ForegroundPath` Brave, `ForegroundDomain: "youtube.com"`, `ClosedTab: "youtube.com"`, `ClosedTabWay: CloseTabCtrlW`, `Plate: "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 40 мин"`, `PlateKind: BlockedVideoOver`, `Buttons: []string{"plateClose"}`.
- `"TestGuard#domain"`: `{Action: TestGuardAction{Action: TestDomain, Text: "youtube.com"}}` → `Empty{}`.
- `CmdTestSeedPrograms`: две версии Edge из магазина — `Programs: [{Path: \`c:\program files\windowsapps\microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe\msedge_proxy.exe\`, Name: "msedge_proxy", Publisher: "Microsoft Corporation", Category: ProgramCatOther, Source: ProgramSourceParent, Hidden: true, DecidedAt: t0 - 86_400_000}, {Path: …_131.0.2903.51_…, Name: "msedge_proxy", Publisher: "Microsoft Corporation", Category: ProgramCatBlocked, Source: ProgramSourceParent, CustomName: "Edge", DecidedAt: t0}]`, `Usage: [{ChildID: 1, Day: "2026-11-03", Path: <первый>, Category: ProgramCatOther, Seconds: 120}]`, `Pending: [{Path: …_132…, Name: "msedge_proxy", Publisher: "Microsoft Corporation", ChildID: 1, Attempts: 2, LastAttemptAt: t0}]` → `Empty{}`.
- `MyToday`/`ParentStats` (все четыре): `VideoS: 0`, в `s39Programs()` и прочих `ProgramTime` — `Kind: "program"`; в `CmdMyToday` добавь строку сайта `{Path: "youtube.com", Name: "youtube.com", Category: "video_social", Kind: "site", Seconds: 600}` и `VideoS: 600` (проверь, что тест «сумма строк = итог» — PL-28 — не в Go-контракте; если есть, подгони `OtherS`/`AllS` литералами и опиши в вердикте).
- `TestContractHasEveryCommand` (`:475`): `66` → `71`, текст: «… и 5 команд M4: ListSites, AddSite, RemoveSite, SetAdultFilter, тестовая TestSeedPrograms».
Затем `go test ./internal/ipc/ -run Contract -update` и `git diff --stat internal/ipc/testdata/contract.json` (ожидаются: новые ключи, `videoMin`/`videoUnlimited` в лимитах, `foregroundDomain` в снимках, `video*` в статусах, `videoText`, `videoS`, `kind` в строках «Итогов»). `TestContractHasNoNull` — зелёный (все новые массивы непустые или `[]T{}`).

D.5. Права и заглушки службы: `internal/service/commands.go` после блока M3 — `ipc.CmdListSites: {parentToken(false), (*Service).hListSites}`, `CmdAddSite`, `CmdRemoveSite`, `CmdSetAdultFilter` — `parentToken(true)`; `CmdTestSeedPrograms: {testMode(admins()), (*Service).hTestSeedPrograms}`. Новый `internal/service/handlers_m4.go`: пять обработчиков `return ipc.Fail(textNotReadyM4)` с `textNotReadyM4 = "Пока не готово"` и комментарием «заглушка T3; обработчик — T4 (сайты), T6 (TestSeedPrograms)». `commands_test.go:86` `wantAllowed`: `case ipc.CmdListSites, ipc.CmdAddSite, ipc.CmdRemoveSite, ipc.CmdSetAdultFilter: return user && own` («Сайты» — только родитель своим токеном); `ipc.CmdTestSeedPrograms` — в строку тестовых команд (`:106`). `TestEveryChannelCommandHasRightsRule`, `TestRightsMatrix` — зелёные. `testActions` (`handlers.go:1047`) **не** менять — `TestDomain` пропускает T4 с тестом `hTestGuard` (PL-41).

### E. Фронт: только контракт (T3b, проверяет `ci.yml`, RM4-0)

Новые поля ответа — **необязательные** в TS (`?:`), иначе подменная программа и литералы тестов (`lib/mytoday.test.ts:58` `reply()`, `status()`; `backend.ts:340-380` статусы) не соберутся `tsc`. Точный список:
- `types.ts`: `WeekdayLimit` + `videoMin?: number; videoUnlimited?: boolean;`; `ChildStatus` + `videoRemainingS?`, `videoLimitS?`, `videoUnlimited?: boolean`, `closeVideo?: boolean`; `TodayCard` + `videoText?: string`; `MyTodayReply` + `videoS?: number`; `ProgramTime` + `kind?: "program" | "site" | ""`; новые `SiteKind = "blocked" | "study" | "video_social"`, `SiteDTO`, `AdultFilterDTO`, `ListSitesReply`, `AddSiteReply`, `RemoveSiteReply`, `SetAdultFilterReply`, `SetupSites { adultOff: string[]; video: string[] }`.
- `api.ts`: `myTodayOf` — строка `videoS: r.videoS ?? 0,` после `gamesS` (**обязательно**: без неё `ParentStats` в `contract.test.ts` красный); `setup(password, children, programs?, sites?: SetupSites)` — `...(sites ? { sites } : {})`; `withGames` **не менять** (S-3); обёртки `listSites()`, `addSite(domain, kind)`, `removeSite(domain)`, `setAdultFilter(childId, on)` с `sitesOf(r) = { ...r, sites: r.sites ?? [], children: r.children ?? [] }` для `ListSitesReply` и вложенного `sites`.
- `contract.test.ts`: в `CASES` — `["Setup", "Setup#sites", (r) => api.setup(r.password, r.children, r.programs, r.sites)]`, `["ListSites", "ListSites", () => api.listSites()]`, `["ListSites", "ListSites#empty", …]`, `["AddSite", "AddSite", (r) => api.addSite(r.domain, r.kind)]`, `["AddSite", "AddSite#moved", …]`, `["RemoveSite", "RemoveSite", (r) => api.removeSite(r.domain)]`, `["SetAdultFilter", "SetAdultFilter", (r) => api.setAdultFilter(r.childId, r.on)]`; в отказы `extra` — `AddSite#invalid`, `AddSite#dup`, `AddSite#full`; в «полноту» (`:194`) — `"ListSites", "AddSite", "RemoveSite", "SetAdultFilter"`. `payloadOf` не меняется (все четыре — `default`).
- `__mocks__/backend.ts`: в `PARENT_COMMANDS` четыре команды сайтов; ответы — примеры контракта по умолчанию (`:1373`). Состояние сайтов в подмене — T8.
- Сверка руками до коммита (локально `npm` закрыт): `grep -n "videoS\|videoMin\|listSites\|SetupSites" frontend/src/{types,api}.ts frontend/src/contract.test.ts` — каждое имя на месте; `grep -rn "toEqual(" frontend/src --include=*.test.* | grep -n "statusOf\|myToday"` — нет точных сравнений, которые получат лишнее `videoS` (читатель нашёл только `toEqual` по частям: `backend.test.ts:263-313`).

### F. Прогон и сдача (T3a и T3b)

Карта кода: `bash tools/codemap.sh` и коммит `docs/CODEMAP.md` вместе с последним коммитом кода (на PR `ci.yml` сверяет). Скан текстов: `python3 tools/scan_ui_texts.py`.

## Среда и команды прогона

```
export PATH=$HOME/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off
go test -race -count=1 ./internal/ipc/... ./internal/service/... ./internal/core/... ./internal/sites/...
go vet ./internal/ipc/... ./internal/service/...
gofmt -l internal
python3 tools/scan_ui_texts.py
```
**`internal/storage` локально не собирается** (проверено читателем: `go vet ./internal/storage/` → `module lookup disabled by GOPROXY=off` для `modernc.org/sqlite v1.60.1`, в `~/go/pkg/mod` только lock-файл). Значит раздел A–B проверяет **только `ci.yml`** после push оркестратора: пиши тесты до кода (RED виден в CI), локально — `gofmt -l internal/storage` и вычитка SQL; в вердикте — «storage: проверит CI». `internal/service/winhost/adapters_windows.go` (x/sys в кэше нет) — тоже только CI (`GOOS=windows go vet` в `ci.yml`). Фронт — только `ci.yml` (RM4-0). Поэтому оркестратору: push после T3a и отдельно после T3b, чтобы красное в CI указывало на один заход.

## Review Focus (классы для самопроверки)

1. Обновление 0.3.3 → 0.4.0 (Review Focus плана п. 7): миграция на заполненной базе, повтор миграции, стартовый набор и при новой установке, и при обновлении; решение родителя (`source='parent'`) сильнее каталога браузеров; WebView2 и `msedge_proxy` не браузеры (PL-18).
2. Строгий флаг (PL-30): дубль домена — в строгий список; запрещённый браузер остаётся скрытым и всё равно закрывается.
3. Безопасный ноль (S-3, S-4): нет поля видео — без лимита; нет `sites` в мастере — 18+ включён всем; `adult_filter` по умолчанию 1.
4. Частичная запись (PL-21): `SetDayLimits` пишет три лимита разом; `ReplaceSiteRules` трогает только свой список и откатывается целиком.
5. Контракт Go ↔ `contract.json` ↔ TS ↔ подмена: нет `null`, ключи lowerCamel, новые поля TS необязательны, `myTodayOf` знает `videoS`.
6. R23: в контракте и примерах — только домены, ни одного пути страницы (пример `AddSite` с адресом — это **ввод родителя**, ответ — домен).

## Global Constraints (дословно из плана)

- Ядро `internal/core` и новый пакет `internal/sites` — только стандартная библиотека; тесты идут локально (`GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 ./internal/core/... ./internal/sites/...`, Go 1.27 в `~/.local/go1.27`). Правила, цифры и тексты считает ядро и служба; страж, значок, окна — только показывают.
- Тексты ребёнку на «ты», родителю на «вы», без длинных тире (`python3 tools/scan_ui_texts.py`), без технических ошибок. Тексты плашек — дословно ТЗ 8.2.5.
- R23: страж читает адрес вкладки только в памяти и сразу оставляет домен; ни полный адрес, ни заголовок окна не передаются, не пишутся в журнал работы и не хранятся. Хранится только домен.
- Предупреждения о видео: пороги 30/15/5/1 (D-54), табло за 30/15/5, плашка 1 мин, звук заказчика — тот же путь, что у игр; порог взводится, только если до события строго больше его минут (PL-14).
- Плашки стража имеют старшинство (PL-34, Ruling FB14-9); новые плашки сайтов — места в RM4-9.
- Миграции — следующий номер 009; ТЗ правится только заменой (D-35); документы — коммит в main, код — ветка и PR.
- `go.sum` обновляет CI; GraphQL закрыт — PR, слияние, выпуск через `gh api` (CLAUDE.md). Журналы Actions не скачиваются — пометки сборки (`tools/annotate-errors.sh`).
- Писатель коммитит пути явно (`git add <пути>`, никогда `-A`), не пушит; пушит оркестратор. Трейлеры коммитов: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze`.
- Среда этого чата: npm-реестр из песочницы закрыт, поэтому фронт (`tsc`, `vitest`, `vite build`) проверяется только в `ci.yml` после push (Ruling RM4-0).

## Уроки (docs/LESSONS.md)

- PL-21 (частичная пересборка теряет соседние поля): `SetDayLimits` пишет общий, игры и видео одной строкой; служба собирает правила только через `readRules` — не плодить второй путь.
- PL-28 (разбивка рядом с итогом): строки сайтов в «Итогах» входят в ту же сумму, что строки программ, остаток — «Остальное время»; пример `MyToday` с сайтом согласуй с итогами.
- PL-30 (из двух флагов действует строгий): дубль домена — в строгий список; браузер, скрытый родителем, всё равно «Запрещено»; `custom_name`, `hidden` миграция не трогает.
- PL-38 (показ по текущему решению): учёт хранит категорию на момент секунды, а показ решает служба по текущему списку (T4); хранилище отдаёт строки как есть, с `Kind`.
- PL-41 (непроверенная запасная ветка сломана): `TestDomain` и `CmdTestSeedPrograms` появляются здесь только в контракте; пропуск через `hTestGuard`/обработчик и тест службы — T4/T6, отметь это в вердикте как Open.

## Коммиты

Conventional Commits по-русски, повелительное наклонение, по одному на раздел: `feat: добавить миграцию 009 для сайтов и браузеров`, `feat: хранить лимит видео, переключатель 18+ и списки сайтов`, `feat: отдать учёт сайтов в итогах из хранилища`, `feat: добавить контракт канала вехи M4`, `feat: тексты предупреждений и подсказка значка о видео`, `feat: зеркалить контракт M4 во фронте`. Пути явно, например: `git add internal/storage/migrations/009_sites_m4.sql internal/storage/sites.go internal/storage/sites_test.go internal/storage/data.go …` — в дереве может работать другой писатель, чужие файлы не добавлять. Трейлеры:

```
Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze
```

НЕ пушить.

## Вердикт (≤ 40 строк на заход)

```
TASK T3a|T3b: done | blocked | failed
Diff: sha + строка на коммит
Evidence: go test -race -count=1 (ipc, service, core, sites — счётчики), go vet, gofmt -l, scan_ui_texts, RED-запуски (тест и причина), мутации (какой тест упал); «storage, winhost, фронт: проверит ci.yml»
Interfaces: публичные имена хранилища, порта и ipc для T4/T6/T7/T8/T9, если отличаются от пакета
Rulings: свои решения с ценой ошибки (S-1…S-7 подтверждены или изменены)
Escalations: изменённые ожидания старых тестов (кроме перечисленных в A.4, B.2), неясности ТЗ
Open: TestDomain в testActions и тест hTestGuard (T4); обработчики сайтов вместо заглушек (T4); TestSeedPrograms (T6); видео в статусе/карточке/«Моих правилах»/CopyRules (T4); табло видео в страже (T7)
```
