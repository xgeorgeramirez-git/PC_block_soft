# Пакет FB13: периоды «Итогов» статистики | слот: писатель | ветка `fix/feedback-0.3.2`

Прочитай: `CLAUDE.md`, ТЗ 1.13 — FR-12, FR-23, 8.2.6 п.2–3, 8.3.7, S42, решение D-61; план раунда `plans/2026-11-03-feedback-0.3.2.md` (RF13-1…RF13-3, Review Focus п.1–2); `docs/LESSONS.md` (классы A, C, D; PL-21, PL-22, PL-25, PL-28); `docs/CODEMAP.md`. Скиллы: dev-methodology (исполнитель: TDD), uiux-design. in-app-guidance НЕ грузить.

Заказчик: «в приложении родителя статистика только за день, а надо день, неделя, месяц, текущий год и всего».

## Как сейчас (сверить grep'ом, PL-4)
- Окно ребёнка `frontend/src/screens/MyStats.tsx`: вкладки периодов «Сегодня…Всё время» уже есть, кроме «Сегодня» отключены (`aria-disabled`, подпись `PERIODS_NOTE`); тест `MyStats.test.tsx` «MyStats: периоды». Компонент `Totals` общий с `ParentStats.tsx`.
- Служба: `internal/service/handlers.go` `hMyToday`, `hParentStats`, `myTodayOf`; ответ `ipc.MyTodayReply` (`internal/ipc/commands.go`), строки программ — `internal/service/stats.go`. Прошлые дни — `day_state` (итоги дня), `usage_daily` (по программам, RM3-6).

## Продукт
1. Запрос `period` (`today|week|month|year|all`, по умолчанию `today`) у `MyToday` и `ParentStats` (контракт `internal/ipc/testdata/contract.json`, биндинги — CI). Ответ за период: `usedS`, `gamesS`, `studyS`, `otherS`, `allS`, `programs`, `extraMin`, `bonusSpentMin`; остаток/лимиты/правила/копилка — как сейчас. Границы по местному времени (RF13-1), сегодня — из ядра, прошлые дни — из базы, без двойного счёта.
2. Фронт: включить вкладки периодов у ребёнка и у родителя (та же разметка `role="tablist"`, стрелки, `aria-selected`), убрать `PERIODS_NOTE`; выбранный период меняет «Итоги» (вкладки категорий, список программ, «Остальное время», пояснение «Всего = …»); строка «Использовано сегодня» → «Использовано за неделю/месяц/год/всё время»; пустой период — «За этот период пока нет данных». Переключение без мигания всей страницы, загрузка — короткий признак.
3. Моки `__mocks__/backend.ts` (данные S42), vitest на литералах S42, e2e-снимки `s06-my-stats-week`, `s31-parent-stats-year`, замер «окно не прокручивается» и «строки не налезают» на узких окнах.

## Приёмка
S42 в `tools/accept/s42-stats-periods.ps1` по образцу `s39-stats-tabs.ps1`: прошлые дни подкладываются тестовой командой (или прямой записью в тестовую базу до старта службы — выбери и обоснуй Ruling'ом), сегодня — game.exe на переднем плане; проверка сумм периодов по ответу программы (PL-22). Включить в `run-all.ps1`, `windows.yml`, `- [ ] S42` в `docs/06_Приёмка.md`. Если дата раннера делает период пустым (например, понедельник 1-го числа) — тест на поддельных часах в Go покрывает все пять периодов, а приёмка сверяет то, что даёт календарь.

## Правила
TDD (RED по ожидаемой причине → GREEN; литералы; мутационная проверка). Go 1.27: `export PATH=~/.local/go1.27/bin:$PATH`; локально `GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 ./internal/<пакет>/...`; SQLite/Wails/Windows — CI. Фронт: `cd frontend && npx tsc --noEmit && npm test`; e2e локально: `npm run build && npx playwright test -c pw-local.config.ts e2e/` (конфиг с `executablePath` браузера `/opt/pw-browsers/chromium_headless_shell-*/…/chrome-headless-shell` создаёт оркестратор, не коммитить). gofmt. Коммиты Conventional Commits по-русски, только явные пути, НЕ пушить. Трейлеры:
Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: <ссылка сессии оркестратора>
Не трогай: версию, CHANGELOG, «Что нового», README, docs кроме `06_Приёмка.md`.

Вердикт ≤30 строк: Status; Diff (sha + заголовок); Evidence; Mutation; Rulings; Escalations; Lesson-кандидат; Open.
