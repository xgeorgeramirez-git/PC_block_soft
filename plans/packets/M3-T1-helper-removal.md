# Пакет M3-T1: B-28 — убрать Помощник, вынести «Что нового» (FR-36) | слот: писатель | дерево: `/home/claude/pc_block_soft`, ветка `feature/m3-programs`

## Контекст продукта

PC_block_soft — домашняя программа родительского контроля для Windows 11 (Go-служба от SYSTEM, страж в сеансе ребёнка, значок, окна на Wails + React/TS, SQLite). Заказчик — родитель, не программист; весь текст для людей по-русски. Решение D-56 (29.10.2026): Помощник (контекстные подсказки, туры, центр обучения, переключатель «Помощник» в настройках) удаляется целиком; «Что нового» остаётся отдельным разделом FR-36 без подсказок. ТЗ 1.8 уже исправлено (`docs/01_ТЗ.md`: FR-36, 8.3.2, 8.3.9, S-таблица без S28, D-56 в трассировке) — **ТЗ не правь**. Ты выполняешь только код и тесты B-28.

Отвечай по-русски. Методика: TDD — сначала падающий тест, ожидания литералами, мутационная проверка; маленькие коммиты.

## 0. Среда

- Дерево `/home/claude/pc_block_soft`, ветка `feature/m3-programs` (уже запушена; другие деревья не трогай; `plans/` и `docs/01_ТЗ.md` не правь; `docs/HISTORY.md`, `CLAUDE.md`, `CHANGELOG.md` и версию не трогай — это закрытие вехи у оркестратора).
- Go: `export PATH=/home/claude/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off`. Локально собирается и тестируется только код на стандартной библиотеке: `go test -race -count=1 ./internal/core/... ./internal/ipc/... ./internal/service/...` (service использует порты и фейки). Пакеты со `storage` (SQLite) и Wails собирает CI.
- Фронт: `cd frontend && npm ci && npx tsc --noEmit && npx vitest run && npm run build`. Не ставится из-за сети — повтори; не вышло — отметь в вердикте, проверит CI.
- Коммиты — Conventional Commits на русском в повелительном наклонении (`refactor: удалить Помощник`), трейлеры в конце:
  `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01XuHYMinhbS8zjLqh6nAgCY`.
- Тексты: ребёнку «ты», родителю «вы», без длинных тире (`python3 tools/scan_ui_texts.py`).
- Skills to load: `uiux-design` (режим «рабочее приложение», ТЗ 8.0). `in-app-guidance` НЕ грузить.
- Push не делай — запушит оркестратор.

## 1. Что сделать (задачи; каждая RED → GREEN → коммит)

**T1.1. «Что нового» отдельно от Помощника (сначала создать, потом удалять).**
- Новый каталог `frontend/src/whatsnew/`: `entries.ts` — `export interface WhatsNewEntry { version: string; title: string; items: string[] }` и `export const WHATS_NEW: WhatsNewEntry[]` с теми же записями 0.1.0, 0.2.0, 0.2.2, 0.2.4, 0.2.5 (тексты один в один из `frontend/src/guidance/catalog.ts`, строки с `kind: "whatsnew"`; поле `id`/`kind` не переносить); порядок хранения «новая сверху» (первая запись — самая новая; добавь вспомогательный `newestFirst` не нужен). Сначала тест `entries.test.ts`: каждая версия — `\d+\.\d+\.\d+`, версии уникальны и идут по убыванию, у записи ≥1 пункта, тексты без длинного тире и без латиницы кроме «PC Block», «Windows», «Brave».
- Экран `frontend/src/whatsnew/WhatsNew.tsx`: заголовок «Что нового», список «Версия X.Y.Z. Название» и маркированные пункты (разметка и классы `help-news`, `subsection-title`, `help-bullets` из `HelpCenter.tsx` оставь/переименуй в `whatsnew-*`, стили в `styles.css` перенеси; не оставляй стилей без разметки — есть `styles.test.ts`). Никаких кнопок, переключателей, ссылок (8.3.9 п.2). Тест `WhatsNew.test.tsx`: список от новой к старой, нет `button`/`input`/`a`.
- В `App.tsx` пункт меню «Что нового» **внизу** меню родителя (отдельно от основной группы, ТЗ 8.3.2 п.2), экран `"whatsnew"`; пункт «Помощь» (если есть в `MENU`) удалить. Тест в `App.test.tsx`: пункт есть последним, открывает экран с версией 0.2.5 сверху (запись 0.3.0 добавит оркестратор на закрытии вехи).
- Проверка «каждая версия добавляет запись»: Go-тест `internal/version/whatsnew_test.go` — читает `frontend/src/whatsnew/entries.ts` (путь `../../frontend/src/whatsnew/entries.ts` от пакета) регуляркой `version:\s*"([^"]+)"`, и требует, чтобы версия из `version.Version` была среди записей. **Сейчас версия 0.2.5 — тест зелёный; на 0.3.0 (закрытие вехи) он заставит добавить запись.** Мутация: поменять `version.Version` в тесте на «9.9.9» → красный.

**T1.2. Удалить Помощника из фронта.**
- Удалить каталог `frontend/src/guidance/` целиком, `frontend/e2e/s28-helper.spec.ts`, снимки/ссылки на него (`grep -rn "s28\|helper" frontend tools .github docs --include=*` — оцени каждое попадание; снимки в ветке `screenshots` не трогай).
- В `App.tsx`: убрать `GuidanceProvider`, `HelpCenter`, `HintSlot`, `useQuietZone`, `NavigateOptions`, `data-help-anchor`, экран `help`; то же в `components/Dialog.tsx`, `components/PromoAdmin.tsx`, `screens/{ChildLimits,Bonus,Today,Settings}.tsx` и их тестах. `Settings.tsx`: удалить раздел «Помощник» (`settings-helper`), `useHelperSwitch`, `resetHidden`; остальное (пароль, учётки, «О программе», бонус-настройки) не менять; обнови комментарий-описание в начале файла.
- `api.ts`: удалить `getHelperState`, `saveHelperState`; `types.ts`: удалить `HelperState` и связанное; `__mocks__/backend.ts`: поле `helper`, `PARENT_COMMANDS` (строки `GetHelperState`/`SaveHelperState`), обработчики; `contract.test.ts`: оба места.
- Тесты, которые проверяли подсказки в экранах, удалить вместе с подсказками; остальные поведения экранов тестами не терять (проверь `git diff --stat` на тесты, удалённые не по теме).
- Вердикт обязан содержать: `npx tsc --noEmit` без ошибок, `vitest run` зелёный, `npm run build` зелёный (или «не вышло из-за сети»).

**T1.3. Удалить Помощника из Go.**
- Удалить `internal/guidance/` целиком (`catalog.go`, `validate.go`, `catalog_test.go`).
- `internal/ipc/commands.go`: удалить `CmdGetHelperState`, `CmdSaveHelperState`, их место в `AllCommands`, тип `HelperState` и его `MarshalJSON`; `contract_test.go`: убрать примеры, `TestHelperStateEmptyIsObject`, поправить число команд в `TestContractHasEveryCommand` (сейчас 59 → 57); `testdata/contract.json` пересоздать: `go test ./internal/ipc/ -run Contract -update`.
- `internal/service`: удалить `settingHelperParent`, `settingHelperChildPrefix`, `helperKey`, `hGetHelperState`, `hSaveHelperState`, `maxHelperState`, строки прав в `commands.go` (232–233) и тесты (`TestHelperStateProfiles`, case в `commands_test.go`, `misc_test.go`); **оставить** `settingFirstRunPrefix`, `hFirstRunSeen`, `firstRunSeen`, окно первого входа (FR-32) и их тесты — они не часть Помощника.
- Хранение: старые строки `helper.parent` и `helper.child.*` в таблице настроек остаются мусором. Добавь миграцию `internal/storage/migrations/003_drop_helper_state.sql` (первая строка `-- Назначение: удалить сохранённое состояние Помощника (D-56)`), `DELETE FROM settings WHERE key = 'helper.parent' OR key LIKE 'helper.child.%';` (проверь настоящее имя таблицы и колонки в `001_init.sql`). Обнови `store_test.go` (`TestOpenCreatesFullSchema` / номера миграций) и добавь тест: до миграции строки были, после — нет, а строка `firstrun.seen.1` и другие настройки целы. **Номер 003 занят этой миграцией; миграция M3 будет 004** (оркестратор учтёт).
- Не трогай «подсказки» значка и экрана блокировки (`systray`, `guard`, `ipc/status.go`: Tooltip, Hint) — это не Помощник.

**T1.4. CI и документы.**
- `.github/workflows/ci.yml`: удалить шаг «Каталог помощника» (`go test ./internal/guidance/...` и сверка `HELPER_COVERAGE.md`) и его упоминания (`grep -n "guidance\|HELPER" .github/workflows/*.yml tools/*`).
- Удалить `docs/HELPER_COVERAGE.md`. В `README.md` (строка про Помощника) и `SKILLS.md` (правило покрытия `in-app-guidance`) — заменить заменой, без приписок: Помощника в программе нет; скилл `in-app-guidance` не используется (D-56).
- `bash tools/codemap.sh` → обновить `docs/CODEMAP.md` (CI на PR проверяет актуальность).

## 2. Ruling оркестратора (не пересматривать без эскалации)

- Хранение состояния Помощника чистим миграцией (не оставляем мусор); FR-32 (окно первого входа) остаётся.
- «Что нового» — статический каталог во фронте, без хранения «просмотрено» (8.3.9: ни кнопок, ни напоминаний).
- Запись 0.3.0 в каталог не добавлять (закрытие вехи); Go-тест версии пройдёт, пока `version.Version` = 0.2.5.

## 3. Review Focus (классы входов/отказов)

1. Остатки: `grep -rni "helper\|guidance\|помощник\|подсказк" internal frontend/src tools .github docs README.md SKILLS.md` — каждое попадание объясни (подсказка значка/экрана блокировки — не Помощник; история в `docs/HISTORY.md` и `CHANGELOG.md` — оставить).
2. Контракт канала: фронт `contract.test.ts`, Go `contract_test.go` и `contract.json` синхронны; нет вызова удалённой команды.
3. Миграция: идемпотентность, чужие настройки не тронуты (`firstrun.seen.*`, `bonus.*`, пароль).
4. Меню родителя: «Что нового» внизу; прочие пункты и их порядок не изменились; пункт «Помощь» исчез; переходы «Ждут решения»/«Нарушения» (если есть) работают без `NavigateOptions`.
5. Тексты 0.1.0…0.2.5 перенесены без искажений (сверка построчно с `git show HEAD:frontend/src/guidance/catalog.ts`).

## 4. Вердикт (≤ 40 строк)

`done/blocked/failed`; Diff (sha + одна строка на коммит); Evidence — команды и итоги (`go test -race` затронутых пакетов, `go vet`, `gofmt -l`, `tsc`, `vitest`, `vite build`, scan, codemap; какие мутации и что упало); Rulings (свои решения с ценой ошибки); Escalations (что нужно визуальному проходу или оркестратору); что не удалось проверить локально и должен проверить CI.
