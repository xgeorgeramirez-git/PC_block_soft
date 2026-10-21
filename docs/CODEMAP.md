# CODEMAP — карта кода PC_block_soft (сгенерировано tools/codemap.sh)

> НЕ редактировать руками — перегенерируется скриптом. Карта сужает поиск;
> сигнатуры перед кодом сверять точечным grep'ом (урок L-52).

## Пакеты Go (роль одной строкой)

| Пакет | Файлов (тесты) | Роль |
|---|---|---|
| `.` | 1 (0) | main: точка входа, запуск окна |
| `internal/auth` | 2 (1) | auth — пароль родителя: проверка длины и паузы после неверных вводов (R19); только стандартная библиотека. |
| `internal/auth/pwhash` | 1 (1) | pwhash — хеш пароля родителя Argon2id (NFR-9): память 64 МБ, 3 прохода, строка формата PHC. |
| `internal/core` | 10 (10) | core — ядро правил и учёта времени: активные секунды, лимиты, предупреждения, блокировка; только стандартная библиотека, всё внешнее через порты. |
| `internal/guard` | 15 (6) | guard — страж сеанса: нативный экран блокировки Windows, который |
| `internal/guidance` | 2 (1) | guidance хранит каталог Помощника (FR-34, ТЗ 8.4 и 10.9) и проверяет его. |
| `internal/ipc` | 11 (6) | ipc — канал связи службы программы с её окнами, стражем сеанса и значком на том |
| `internal/launch` | 1 (1) | launch разбирает ключи запуска программы: в каком режиме работает PCBlock.exe. |
| `internal/service` | 16 (14) | service — служба PCBlock: фоновая часть, которая считает время за компьютером, |
| `internal/service/winhost` | 18 (1) | winhost — Windows-обвязка службы PCBlock: всё, что внутри internal/service |
| `internal/storage` | 4 (4) | storage — хранилище программы: SQLite в режиме WAL (ТЗ 3.1, 3.4, 4), драйвер на чистом Go. |
| `internal/systray` | 12 (6) | systray — значок у часов и мини-панель: надпись остатка, подсказка, цвет статуса, |
| `internal/ui` | 3 (1) | ui — окно программы на Wails: запуск окна и методы, доступные интерфейсу. |
| `internal/version` | 1 (1) | version хранит номер версии программы. |
| `internal/winsession` | 5 (0) | winsession запускает процессы в интерактивных сеансах Windows: |

## Миграции базы

- `001_init.sql` — Назначение: начальная схема базы по ТЗ 4 целиком (таблицы M1 и поля M2+ сразу, чтобы не мигрировать повторно).

## Интерфейс (frontend/src)

- `src/App.tsx`
- `src/__mocks__/backend.ts`
- `src/api.ts`
- `src/components/DayStrip.tsx`
- `src/components/Dialog.tsx`
- `src/components/GrantBonusForm.tsx`
- `src/components/LimitsTable.tsx`
- `src/components/Notice.tsx`
- `src/components/PausePicker.tsx`
- `src/components/SaveBar.tsx`
- `src/components/ScheduleEditor.tsx`
- `src/components/ServiceNotices.tsx`
- `src/components/TextField.tsx`
- `src/components/usePause.ts`
- `src/global.d.ts`
- `src/guard.tsx`
- `src/guidance/GuidanceProvider.tsx`
- `src/guidance/HelpCenter.tsx`
- `src/guidance/HintCard.tsx`
- `src/guidance/HintSlot.tsx`
- `src/guidance/Tour.tsx`
- `src/guidance/candidates.ts`
- `src/guidance/catalog.ts`
- `src/guidance/context.ts`
- `src/guidance/detectors.ts`
- `src/guidance/engine.ts`
- `src/guidance/icons.tsx`
- `src/guidance/io.ts`
- `src/guidance/lists.ts`
- `src/guidance/state.ts`
- `src/guidance/steps.ts`
- `src/guidance/useHelperSwitch.ts`
- `src/lib/errors.ts`
- `src/lib/format.ts`
- `src/lib/journal.ts`
- `src/lib/limits.ts`
- `src/lib/mytoday.ts`
- `src/lib/notices.ts`
- `src/lib/password.ts`
- `src/lib/schedule.ts`
- `src/lib/status.ts`
- `src/main.tsx`
- `src/screens/Bonus.tsx`
- `src/screens/ChildLimits.tsx`
- `src/screens/Children.tsx`
- `src/screens/FirstRun.tsx`
- `src/screens/Journal.tsx`
- `src/screens/LoginDialog.tsx`
- `src/screens/MyStats.tsx`
- `src/screens/ParentStats.tsx`
- `src/screens/ResetPassword.tsx`
- `src/screens/Settings.tsx`
- `src/screens/Today.tsx`
- `src/screens/UninstallGate.tsx`
- `src/screens/Wizard.tsx`
- `src/session.ts`
- `src/styles.css`
- `src/test-setup.ts`
- `src/types.ts`
- `src/useSessionWatcher.ts`
- `src/vite-env.d.ts`

## Биндинги Wails (что интерфейс может вызвать у программы)

- `wailsjs/go/ui/App.d.ts`: Call,Finish,Mode,Version

## Конвенции стека

- Ядро правил и учёта — только стандартная библиотека Go, без Wails и Windows (ТЗ 3.3); тесты ядра идут локально и на ubuntu.
- Windows-код — в файлах `*_windows.go`; проверка сборки: `GOOS=windows go vet -tags desktop,production ./...`.
- Длительности в базе — целые секунды; дата дня — `YYYY-MM-DD` по местному времени; моменты — UTC мс + смещение пояса.
- Тексты интерфейса — русские, без длинных тире (`python3 tools/scan_ui_texts.py`).
- После смены Go-методов App — биндинги `frontend/wailsjs/go` регенерирует `wails build` в CI; расхождение с закоммиченными роняет CI.

## Точки входа

- `main.go` — запуск окна (`internal/ui.Run`), встраивает `frontend/dist`.
- `internal/ui/app.go` — методы App для интерфейса.
- `frontend/src/main.tsx` → `App.tsx`; обращения к программе — только через `frontend/src/api.ts`.

## Команды

- Локально (облачная песочница, только ядро на стандартной библиотеке): `GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 ./internal/version/...`
- Фронт: `cd frontend && npm ci && npm test && npm run build`; снимки: `npm run e2e` (в CI).
- CI: `ci.yml` (ubuntu, R0–R2 + снимки), `windows.yml` (R3: установщик, окно, выпуск), `full-series.yml` (R4). go.sum обновляет CI сам в рабочей ветке (сделать git pull).
- Карта: `bash tools/codemap.sh`.
