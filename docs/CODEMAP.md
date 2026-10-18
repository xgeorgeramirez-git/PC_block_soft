# CODEMAP — карта кода PC_block_soft (сгенерировано tools/codemap.sh)

> НЕ редактировать руками — перегенерируется скриптом. Карта сужает поиск;
> сигнатуры перед кодом сверять точечным grep'ом (урок L-52).

## Пакеты Go (роль одной строкой)

| Пакет | Файлов (тесты) | Роль |
|---|---|---|
| `.` | 1 (0) | main: точка входа, запуск окна |
| `internal/ui` | 1 (0) | ui — окно программы на Wails: запуск окна и методы, доступные интерфейсу. |
| `internal/version` | 1 (1) | version хранит номер версии программы. |

## Миграции базы

Пока нет (появятся в M1: internal/storage/migrations/NNN_имя.sql, первая строка — комментарий о назначении).

## Интерфейс (frontend/src)

- `src/App.tsx`
- `src/api.ts`
- `src/main.tsx`
- `src/styles.css`
- `src/test-setup.ts`
- `src/vite-env.d.ts`

## Биндинги Wails (что интерфейс может вызвать у программы)

- `wailsjs/go/ui/App.d.ts`: Version

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
