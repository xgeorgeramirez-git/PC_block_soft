# CODEMAP — карта кода PC_block_soft (сгенерировано tools/codemap.sh)

> НЕ редактировать руками — перегенерируется скриптом. Карта сужает поиск;
> сигнатуры перед кодом сверять точечным grep'ом (урок L-52).

## Пакеты Go (роль одной строкой)

| Пакет | Файлов (тесты) | Роль |
|---|---|---|
| `.` | 1 (0) | main: точка входа, запуск окна |
| `internal/auth` | 2 (1) | auth — пароль родителя: проверка длины и паузы после неверных вводов (R19); только стандартная библиотека. |
| `internal/auth/pwhash` | 1 (1) | pwhash — хеш пароля родителя Argon2id (NFR-9): память 64 МБ, 3 прохода, строка формата PHC. |
| `internal/core` | 13 (19) | core — ядро правил и учёта времени: активные секунды, лимиты, предупреждения, блокировка; только стандартная библиотека, всё внешнее через порты. |
| `internal/guard` | 25 (18) | guard — страж сеанса: нативный экран блокировки Windows, который |
| `internal/ipc` | 16 (9) | ipc — канал связи службы программы с её окнами, стражем сеанса и значком на том |
| `internal/launch` | 1 (1) | launch разбирает ключи запуска программы: в каком режиме работает PCBlock.exe. |
| `internal/programs` | 3 (6) | programs — классификация программ по пути exe (веха M3: R12, R13, RM3-1, RM3-12): |
| `internal/service` | 34 (51) | service — служба PCBlock: фоновая часть, которая считает время за компьютером, |
| `internal/service/winhost` | 29 (10) | winhost — Windows-обвязка службы PCBlock: всё, что внутри internal/service |
| `internal/sites` | 5 (5) | sites — домены сайтов: нормализация ввода родителя и поля адреса Brave, punycode, свои списки родителя и встроенный список «18+». |
| `internal/sound` | 3 (1) | sound — звук предупреждения заказчика (D-55, FR-9). Файл заказчика |
| `internal/storage` | 9 (16) | storage — хранилище программы: SQLite в режиме WAL (ТЗ 3.1, 3.4, 4), драйвер на чистом Go. |
| `internal/systray` | 20 (12) | systray — значок у часов и мини-панель: надпись остатка, подсказка, цвет статуса, |
| `internal/ui` | 3 (1) | ui — окно программы на Wails: запуск окна и методы, доступные интерфейсу. |
| `internal/version` | 1 (2) | version хранит номер версии программы. |
| `internal/winsession` | 5 (0) | winsession запускает процессы в интерактивных сеансах Windows: |
| `tools/adultlist` | 1 (1) | (нет комментария Package) |

## Миграции базы

- `001_init.sql` — Назначение: начальная схема базы по ТЗ 4 целиком (таблицы M1 и поля M2+ сразу, чтобы не мигрировать повторно).
- `002_feedback_0_2_2.sql` — Назначение: фидбек-раунд 0.2.2 — автобонусы (R25), снятие запрета на день (R24), промокоды (R26).
- `003_drop_helper_state.sql` — Назначение: удалить сохранённое состояние Помощника (D-56)
- `004_programs_m3.sql` — Назначение: веха M3 «Программы и игры»: издатель, отпечаток и корень приложения у программ, время последней попытки и попытки по детям у ждущих решения, индекс учёта по программам.
- `005_hidden_programs.sql` — Назначение: фидбек-раунд 0.3.1 (FB-9, D-57): скрытые родителем программы.
- `006_last_minute.sql` — Назначение: фидбек-раунд 0.3.1 (FB-12, D-60): последняя минута идёт по часам до нуля.
- `007_custom_names.sql` — Назначение: фидбек-раунд 0.3.1 (FB-11, D-59): свои названия программ.
- `008_rest_break.sql` — Назначение: фидбек-раунд 0.3.2 (FB-14, D-62): перерыв на отдых (R27, FR-37).
- `009_sites_m4.sql` — Назначение: веха M4 «Сайты и Brave»: один домен — один список, стартовый набор «Видео и соцсети», браузеры каталога запрещены.
- `010_stats_m5.sql` — Назначение: веха M5 (FR-38, FR-24): снимок очистки статистики и индексы журнала; автобонус без начисления задним числом (D-82).

## Интерфейс (frontend/src)

- `src/App.tsx`
- `src/__mocks__/backend.ts`
- `src/__mocks__/g15.ts`
- `src/api.ts`
- `src/components/AskMoreDialog.tsx`
- `src/components/BonusHistory.tsx`
- `src/components/DayBars.tsx`
- `src/components/DayStrip.tsx`
- `src/components/Dialog.tsx`
- `src/components/FitText.tsx`
- `src/components/GrantBonusForm.tsx`
- `src/components/LimitsTable.tsx`
- `src/components/Notice.tsx`
- `src/components/PausePicker.tsx`
- `src/components/PeriodTabs.tsx`
- `src/components/PromoRedeem.tsx`
- `src/components/RestSettings.tsx`
- `src/components/SaveBar.tsx`
- `src/components/ScheduleEditor.tsx`
- `src/components/ServiceNotices.tsx`
- `src/components/SpendBonusDialog.tsx`
- `src/components/TextField.tsx`
- `src/components/UndoLine.tsx`
- `src/components/usePause.ts`
- `src/global.d.ts`
- `src/guard.tsx`
- `src/lib/bonus.ts`
- `src/lib/daybars.ts`
- `src/lib/errors.ts`
- `src/lib/format.ts`
- `src/lib/journal.ts`
- `src/lib/limits.ts`
- `src/lib/mytoday.ts`
- `src/lib/notices.ts`
- `src/lib/numinput.ts`
- `src/lib/password.ts`
- `src/lib/programs.ts`
- `src/lib/rest.ts`
- `src/lib/schedule.ts`
- `src/lib/sites.ts`
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
- `src/screens/Programs.tsx`
- `src/screens/Promos.tsx`
- `src/screens/ResetPassword.tsx`
- `src/screens/Settings.tsx`
- `src/screens/Sites.tsx`
- `src/screens/Today.tsx`
- `src/screens/UninstallGate.tsx`
- `src/screens/Wizard.tsx`
- `src/session.ts`
- `src/styles.css`
- `src/test-setup.ts`
- `src/types.ts`
- `src/useSessionWatcher.ts`
- `src/vite-env.d.ts`
- `src/whatsnew/WhatsNew.tsx`
- `src/whatsnew/entries.ts`

## Биндинги Wails (что интерфейс может вызвать у программы)

- `wailsjs/go/ui/App.d.ts`: Call,Finish,Mode,StartScreen,Version

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
