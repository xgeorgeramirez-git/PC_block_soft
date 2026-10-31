# Пакет FB9: скрытые программы | слот: писатель | ветка `fix/feedback-0.3.1`

Прочитай: `CLAUDE.md`, план `plans/2026-10-31-feedback-0.3.1.md` (Ruling RF9-1…RF9-4, Review Focus), ТЗ 1.9 — FR-20, R3, 8.3.4 п.4, S38, D-57; `docs/LESSONS.md` (PL-9, PL-16, PL-19, PL-20, PL-22); `docs/CODEMAP.md`. Скиллы: dev-methodology (исполнитель: TDD), uiux-design (для экрана). in-app-guidance НЕ грузить.

## Продукт (F9-1)
1. Миграция `internal/storage/migrations/005_hidden_programs.sql`: `programs.hidden`. Хранилище: чтение/запись флага, список программ отдаёт флаг; «за неделю» и статистика по программам скрытые не показывают. Тест миграции на базе со схемой 004 и данными.
2. `internal/programs`: классификатор знает скрытые пути (RF9-2) — не закрывать; от скрытой программы не наследуют корень приложения и профиль; при смене отпечатка вне доверенных каталогов — как сейчас «ждёт решения» (Review Focus 2). Тест-таблица.
3. Служба: команда скрытия/показа для набора путей (по образцу `SetProgramsCategory`, контракт `internal/ipc`, права родителя), журнал «вы скрыли «X»» / «вы снова показали «X»» (для нескольких — «вы скрыли 3 программы»), отмена (как у категории), обновление классификатора без перезапуска. Учёт (RF9-3): скрытая на переднем плане — «Прочее». Тесты на фейках, включая «игры кончились: скрытая не закрыта, игра закрыта» и «только учёба: скрытая не закрыта».
4. Фронт (`frontend/src/screens/Programs.tsx`, `lib/programs.ts`, `api.ts`, моки `__mocks__/backend.ts`): «Скрыть» в строке и для отмеченных, фильтр «Скрытые», «Показать», `UndoLine` «Stalker2 скрыта». Vitest; e2e-снимок `s11-programs-hidden` (фильтр «Скрытые») и замер, что строка с кнопкой не налезает на колонки. Биндинги `frontend/wailsjs/go` перегенерирует CI — если меняются методы App, обнови вручную по образцу или сообщи в Escalations.

## Приёмка (F9-2)
`tools/accept/s38-hidden.ps1` по S38 и образцу `s12-games-limit.ps1` (обвязка `common.ps1`: `New-TestApp`, `Invoke-Setup -Programs`, `Set-ProgramCategory`, `Assert-ClosedFast`, `Assert-AppAlive`, `Get-ProgramName`, `Format-Events`): game.exe и launcher.exe — «Игры», лимит игр 2 мин; скрыть launcher.exe командой канала; запустить обе; игры кончились → game.exe закрыта, launcher.exe жива; ListPrograms без launcher.exe (кроме фильтра скрытых, если команда отдаёт флаг); журнал «вы скрыли»; «Показать» → повторный запуск launcher.exe закрыт. Включить в `run-all.ps1`, список сценариев `windows.yml`, пункт `- [ ] S38` в `docs/06_Приёмка.md`. Ожидание признаком, не временем (PL-16); образцы журнала — из ответа программы (PL-22).

## Правила
TDD (RED по ожидаемой причине → GREEN; литералы; мутационная проверка). Go 1.27: `export PATH=~/.local/go1.27/bin:$PATH`; локально `GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 ./internal/<пакет>/...` (SQLite, Wails, Windows — проверит CI; storage-тесты локально могут не собраться — тогда кросс-проверка `go vet` через временный modfile, не коммитить). npm в песочнице может быть закрыт (403) — тогда фронт проверяет CI. gofmt. Коммиты Conventional Commits по-русски, только явные пути, НЕ пушить. Трейлеры:
Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01S7we7qUEtDUsQ6Yfhud6Mc
Не трогай: версию, CHANGELOG, «Что нового», README, docs кроме `06_Приёмка.md` — это оркестратор.

Вердикт ≤30 строк: Status; Diff (sha + заголовок); Evidence; Mutation; Rulings; Escalations; Lesson-кандидат; Open.
