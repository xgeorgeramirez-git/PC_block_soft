# Общая часть пакетов вехи M5 (читать первым)

Ты — субагент-писатель вехи M5 проекта PC_block_soft (родительский контроль для Windows 11, Go + Wails + React).
Дерево: `/home/claude/pc_block_soft`, ветка `feature/m5-stats` (уже создана, ты в ней). Не переключай ветки.

## Что прочитать
1. `CLAUDE.md` (правила репозитория, среда песочницы).
2. План `plans/2026-11-10-M5.md`: шапка, Global Constraints, Ruling-и RM5-0…RM5-15 и **свою задачу дословно**. Чужие задачи не делай.
3. Затронутые разделы ТЗ `docs/01_ТЗ.md` по ссылкам задачи; уроки `docs/LESSONS.md` — сверка по классу дефекта (grep по ключевым словам зоны).
4. Карта кода `docs/CODEMAP.md`, дальше — точечный grep фактического кода (сигнатуры плана сверяй с кодом; расхождение плана с кодом — Ruling в вердикте).

## Правила
- TDD: сначала падающий тест (проверь, что он падает по ожидаемой причине), потом минимальный код, потом зелёный. Ожидания — литералами, не вычислены кодом под тестом.
- Ядро `internal/core` — только стандартная библиотека. Массивы ответа канала — `[]T{}`, не nil.
- Тексты: ребёнку на «ты», родителю на «вы», без длинных тире «—» в строках интерфейса (`python3 tools/scan_ui_texts.py`), без технических ошибок.
- Локальная проверка Go (сеть закрыта, Go 1.24):
  ```
  cd /home/claude/pc_block_soft && sed 's/^go 1.27.*/go 1.24/; /^toolchain/d' go.mod > /tmp/local.mod && cp go.sum /tmp/local.sum && GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 -modfile=/tmp/local.mod ./internal/core/... ./internal/service/ ./internal/ipc/... ./internal/programs/...
  ```
  `gofmt -l` по своим файлам должен быть пуст; `GOTOOLCHAIN=local go vet -modfile=/tmp/local.mod` своих пакетов. Хранилище (`internal/storage`, SQLite) и фронт (npm) локально НЕ собираются — их проверит CI после push оркестратора; поэтому код хранилища и фронта перечитай особенно внимательно (компилируемость, импорты, имена).
  Windows-код (`*_windows.go`, `winhost`): `GOOS=windows GOTOOLCHAIN=local go vet -modfile=/tmp/local.mod ./internal/service/winhost/` может не собраться из-за закрытых модулей — тогда просто внимательно сверь сигнатуры; это проверит CI.
- Коммиты: маленькие, после каждой зелёной части; `git add <пути>` явно (никогда `-A`/`.`); НЕ пушить. Conventional Commits, описание по-русски повелительно (`feat(storage): добавить ряд дней day_state`). В конце сообщения две строки трейлеров:
  ```
  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01S7we7sJBCaNL17E6bp5E61
  ```
- Другой писатель параллельно не работает; читатель может читать дерево — не мешает.
- Смена сигнатур плана, файлы чужих задач, расширение объёма — не делать: `blocked` + Escalations.

## Вердикт (последнее сообщение, не больше 30 строк)
```
Status: done | blocked | failed
Diff: <sha коммитов и одна строка о каждом>
Evidence: <команды и итоговые строки прогонов: ok/FAIL, число тестов>
Rulings: <решение — почему — цена ошибки> (если были)
Escalations: <что нужно от оркестратора> (если есть)
Lesson: <кандидат в урок, если поймал дефект/граблю>
Open: <что осталось / что проверит только CI>
```

## Тесты хранилища локально (находка T1a)
Тесты `internal/storage` можно гонять в песочнице на подменном драйвере `github.com/ncruces/go-sqlite3` (wasm; модули с github качаются при `GOPROXY=direct GOSUMDB=off`, `golang.org/x/sys` — через `github.com/golang/sys` replace), во временном modfile и временном файле-подмене драйвера, **не коммитить**. Ограничение: WAL этот драйвер не тянет — нужен `journal_mode(DELETE)` в подмене. Итог всё равно подтверждает `ci.yml` на настоящем modernc.
