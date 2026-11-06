# Пакет: T8 вехи M4 «Экран «Сайты»» | слот: писатель | исполнитель: новый агент

Репозиторий `/home/claude/pc_block_soft`, ветка `feature/m4-sites`. Прочитай `plans/2026-11-04-M4.md`: Global Constraints, RM4-0, RM4-4, RM4-5, UX-сценарий (абзац «Родитель»), Review Focus п. 3, 9; строку T8. ТЗ (`docs/01_ТЗ.md`): FR-21, R14, 8.0 (все пункты), 8.3.2 п.2, 8.3.5. Уроки — раздел «Уроки» ниже. В рабочем дереве могут лежать чужие правки Go (писатель T6: `internal/programs`, `internal/service`): **их не трогать, `git add` — только свои пути**.

Перед работой сверь адреса (PL-4): `git log --oneline -5`, `grep -n "listSites\|addSite\|removeSite\|setAdultFilter\|function sitesOf" frontend/src/api.ts`, `grep -n "SiteKind\|SiteDTO\|ListSitesReply\|AddSiteReply\|RemoveSiteReply\|SetAdultFilterReply" frontend/src/types.ts`, `grep -n "\"ListSites\"\|\"AddSite\"\|\"RemoveSite\"\|\"SetAdultFilter\"" frontend/src/__mocks__/backend.ts`, `grep -n "const MENU\|type Screen" frontend/src/App.tsx`.

## Бриф

Новый раздел окна родителя «Сайты» (FR-21, 8.3.5) — пункт меню сразу после «Программ»: вверху переключатели «Блокировать сайты 18+» по детям и под ними мелко источник списка; ниже вкладки «Запрещённые», «Учёба», «Видео и соцсети» со счётчиками; поле «Добавить сайт» (домен или вставленный адрес); удаление с «Отменить» на 10 с; поиск по списку. Все решения и тексты ответов — от службы (обработчики T4 готовы), экран только показывает. Подменная программа (`__mocks__/backend.ts`) получает обработчики четырёх команд по образцу службы; vitest и снимки Playwright. **npm в песочнице закрыт (RM4-0)**: `tsc`, `vitest`, `vite build` и снимки прогонит только `ci.yml` после push оркестратора — тесты пиши полностью до коммита, ожидания литералами из этого пакета.

## Найдено читателем (проверено по `ffe9906`)

- **Контракт фронта уже есть (T3b, `4d5eba8`)**, `types.ts` и `api.ts` не меняются: `SiteKind = "blocked" | "study" | "video_social"`, `SiteDTO {domain, kind}`, `AdultFilterDTO {childId, name, on}`, `ListSitesReply {sites, children, blockedMax, adultSource}`, `AddSiteReply {message, domain, moved, sites}`, `RemoveSiteReply {message, domain, kind, undoUntilMs, sites}`, `SetAdultFilterReply {message, sites}` (`types.ts:663-709`). Обёртки (`api.ts:508-541`): `listSites(): Promise<ListSitesReply>`, `addSite(domain: string, kind: SiteKind): Promise<AddSiteReply>`, `removeSite(domain: string): Promise<RemoveSiteReply>`, `setAdultFilter(childId: number, on: boolean): Promise<SetAdultFilterReply>`; `sitesOf` приводит `null` к `[]`. Поле `prevKind` в ответе AddSite **нет** — есть только `moved: boolean`; прежний список узнаётся из `sites` до запроса, если понадобится (не нужен: текст переноса даёт служба).
- **Ответы службы** (`internal/service/handlers_m4.go`, тексты `internal/service/texts.go:75-81`, `sites_svc.go:170-174`): AddSite — успех `"Добавлено: <домен>"`, перенос `"Сайт перенесён в «Запрещённые»" | "«Учёбу»" | "«Видео и соцсети»"` (`moved: true`); отказы у поля `field: "domain"`: `"Это не похоже на адрес сайта"`, `"Этот сайт уже в списке"`, `"В списке запрещённых уже 1000 сайтов. Удалите ненужные"`; без поля: `"Не удалось сохранить. Попробуйте ещё раз"`. RemoveSite — `"Удалён <домен>"`, `kind` — прежний список, `undoUntilMs = сейчас + 10 с` (`undoLife`); отказ `"Этого сайта уже нет в списках"`. «Отменить» удаления = `addSite(domain, kind)` (S-15: отдельной записи отмены служба не ведёт). SetAdultFilter — `"<Имя>: сайты 18+ блокируются"` / `"<Имя>: сайты 18+ не блокируются"`; отказ `"Такого ребёнка нет в списке"`. ListSites — `adultSource = "Список сайтов для взрослых: UT1, Университет Тулуза Капитоль, лицензия CC BY-SA 4.0"`, `blockedMax = 1000`, сайты по списку и домену. Все ответы изменений несут свежий `sites` — экран берёт его, а не перечитывает.
- **Примеры контракта** (`internal/ipc/testdata/contract.json`): `ListSites` — `example.com`/blocked, `school.example`/study, `youtube.com`/video_social; дети `{1, "Миша", on: true}`, `{2, "Даша", on: false}`. `AddSite` (запрос `"https://www.Example.com/page?x=1"`), `AddSite#moved` (`youtube.com` → study), `AddSite#dup|#full|#invalid`, `RemoveSite` (`undoUntilMs: 1760000010000`), `SetAdultFilter` (`{childId: 2, on: false}`). `contract.test.ts` их уже разбирает — не трогать.
- **Подменная программа**: команды «Сайтов» в `PARENT_COMMANDS` (`backend.ts:628-631`), **обработчиков нет** — сейчас отдают пример контракта (`backend.ts:1377`), то есть «удалить» ничего не удаляет. В `MockChild` нет поля «18+», в `MockOptions`/`MockState` нет сайтов. События журнала мок пишет `addEvent(now, child, kind, text)` (`backend.ts:703`).
- **Тексты журнала строит служба** (`internal/service/journaltext.go:106-130`), фронт берёт `JournalEntry.text` как есть (`Journal.tsx:108`): для мока — `"Вы добавили сайт example.com в «Запрещённые»"`, `"Вы перенесли сайт youtube.com в «Учёбу»"`, `"Вы удалили сайт example.com из «Запрещённых»"`, `"Даша: сайты 18+ не блокируются"` (виды `site_added`, `site_moved`, `site_removed`, `site_adult_filter`). Нарушения (`lib/journal.ts` `VIOLATION_KINDS`) — задача T9.
- **Образец экрана** — `screens/Programs.tsx`: `run(what, action, onError)` — один запрос за раз (`busyRef`), `Feedback {seq, text, error, where, undo}`, `UndoLine` (`components/UndoLine.tsx`: `text, error, undoMs, onUndo, disabled, className`; ошибка — `role="alert"`, успех — `role="status"`, «Отменить» живёт `undoMs` и не пропадает, пока в нём фокус), `UNDO_MS = 10_000`, `left = Math.min(undoUntilMs - Date.now(), UNDO_MS)`, фокус после ответа через `focusTo` + `useLayoutEffect` (`placeFocus`), загрузка «Загружаю данные», ошибка загрузки `Notice tone="warn" urgent` + «Повторить», поиск `type="search"` + «Ничего не найдено» + «Сбросить поиск», вкладки-фильтры `className="tab"`. Ошибка с полем — `CallError.field` (`lib/errors.ts`), текст — `errorText(e)`.
- **Меню**: `type Screen` (`App.tsx:192`) и `MENU` (`App.tsx:195-203`) — сейчас «Сегодня», «Дети», «Программы», «Копилка», «Статистика», «Журнал», «Настройки»; экраны — цепочка `{screen === "…" ? <… /> : null}` (`App.tsx:283-300`). Тест меню — `App.test.tsx:228-242` (список литералами).
- **Стили**: токены 8.0 п.9 уже в `styles.css`; `.panel`, `.page-head`, `.panel-title`, `.section-title`, `.programs-block`, `.field`, `.field-label`, `.field-input`, `.field-hint`, `.field-error`, `.btn`, `.btn-primary`, `.btn-small`, `.tab`, `.tabs`, `.switch`. Медиа-запрос узкого окна — `@media (max-width: 640px)` (`styles.css:1973`). Окно родителя: 1200×820, наименьшее 960×620 (`internal/ui/app.go:172`).
- **Снимки**: `frontend/e2e/screens.spec.ts`, job «Снимки экранов (браузер)» в `.github/workflows/ci.yml:122-156` (`npx vite build && npx playwright test`; файлы `frontend/screens/*.png` → ветка `screenshots`, папка `screens/feature-m4-sites/`). Помощники: `mockProgram(page, options)`, `noHorizontalScroll(page)` (и PL-25: документ не прокручивается), `wholePage(page)`, `inFrame`, `boxOf`, `nothingSticksOut`, `family()`, `openPrograms` (`:1174`, образец перехода по меню). Сцена — `test("…", async ({ page }) => { … await page.screenshot({ path: \`${SCREENS}/<имя>.png\`, fullPage: true }) })`.
- Скан текстов `python3 tools/scan_ui_texts.py` проверяет `.ts/.tsx` в `frontend/src` (кроме тестов) на длинное и среднее тире. Карта кода `bash tools/codemap.sh` перечисляет файлы `frontend/src` — новые файлы требуют её пересборки (CI на PR сверяет).

## Решения читателя (оркестратор подтверждает до пуска; цена ошибки — в скобках)

- **T8-1 Экран не разбирает домен сам (PL-39, RM4-4).** Поле отправляется службе как есть (`addSite(raw.trim(), kind)`); пустое поле — без запроса, ошибка у поля `"Это не похоже на адрес сайта"` (тот же текст, что у службы). Разборщик `cleanDomain` (T8-6) экран не зовёт. (Цена второго разборщика на экране: экран и служба по-разному поймут IDN или `www.www.`, родитель увидит в сообщении не тот домен, что в списке.)
- **T8-2 Состояние экрана — только из ответов службы.** После `addSite`/`removeSite`/`setAdultFilter`/«Отменить» экран кладёт `reply.sites` целиком (как `applyReply` в «Программах»), не правит списки сам и не перечитывает `listSites`. Переключатель «18+» — управляемый по `sites.children[i].on`: пока запрос идёт, все переключатели недоступны; отказ — переключатель остаётся как был, ошибка в строке сообщений блока «18+». (Цена оптимистичного переключателя: при отказе службы на экране «блокируется», а в бою нет — ребёнок видит сайты 18+.)
- **T8-3 Вид.** Порядок сверху вниз: заголовок `h2` «Сайты»; строка-пояснение `"Программа следит за сайтами только в Brave. Другие браузеры у детей закрываются"`; блок «18+» (`section`, `h3` «Блокировать сайты 18+», `role="group"` с тем же именем, у каждого ребёнка переключатель `.switch` с подписью — имя ребёнка; под ним `p.field-hint.sites-source` с `adultSource` из ответа, если не пусто; детей нет — `"Детей пока нет. Добавьте их в разделе «Настройки» → «Детские учётки»"`); строка сообщений блока «18+»; блок списков (`section`, `role="tablist"` с именем `"Списки сайтов"`, три вкладки `role="tab"` с видимым текстом `<название> <число>`, по умолчанию «Запрещённые»; `role="tabpanel"`). В панели: пояснение списка; у «Запрещённых» — `"<N> из <blockedMax>"`; форма добавления (`form`, поле `TextField` «Добавить сайт», подсказка, кнопка `btn-primary` «Добавить», Enter отправляет — 8.0 п.8); строка сообщений списка; поиск `"Найти в списке"` (только если в списке есть сайты); список `ul` с именем `"<название>: сайты"`, строка — домен (`font-variant-numeric` не нужен, обычный текст) и кнопка `btn btn-small` «Удалить» с `aria-label="Удалить <домен>"`. Цвет статуса не используется (8.0 п.3): ни красного у «Запрещённых», ни янтаря у «999 из 1000». Ширина колонки домена — без обрезки до 253 знаков: перенос `overflow-wrap: anywhere`. (Цена красного у списка: родитель примет список за ошибку; цвет зарезервирован.)
- **T8-4 Тексты экрана (литералы, все — родителю на «вы» или безличные, без длинных тире).**

  | где | текст |
  |---|---|
  | вкладки | `Запрещённые`, `Учёба`, `Видео и соцсети` |
  | пояснение «Запрещённые» | `Эти сайты закрываются в Brave у всех детей. Сайт закрывается вместе со всеми его поддоменами` |
  | пояснение «Учёба» | `Время на этих сайтах не тратит лимит. Когда время вышло и выбрано «Оставить только учёбу», в Brave открываются только они` |
  | пояснение «Видео и соцсети» | `Время на этих сайтах идёт в лимит «Видео и соцсети» (раздел «Дети», вкладка «Лимиты») и в общее время` |
  | счётчик «Запрещённых» | `3 из 1000` |
  | подпись поля | `Добавить сайт` |
  | подсказка поля | `Можно вставить адрес страницы целиком: программа возьмёт из него только сайт` |
  | кнопка | `Добавить` / во время запроса `Сохраняю…` |
  | пусто «Запрещённые» | `Запрещённых сайтов пока нет` |
  | пусто «Учёба» | `Учебных сайтов пока нет` |
  | пусто «Видео и соцсети» | `В этом списке пока нет сайтов` |
  | поиск | подпись `Найти в списке`; пусто — `Ничего не найдено` + кнопка `Сбросить поиск` |
  | загрузка | `Загружаю данные`; ошибка — текст `errorText` + кнопка `Повторить` |

  Сообщения об успехе и отказах — только текст службы (`reply.message`, `errorText(e)`), без своих формулировок. (Цена своих текстов: расхождение с журналом и со службой, PL-29 повтор.)
- **T8-5 Отмена и фокус (8.0 п.7, L-74).** Удаление: сообщение `reply.message` с «Отменить», `undoMs = Math.min(reply.undoUntilMs - Date.now(), 10_000)`; ≤ 0 — без кнопки. «Отменить» → `addSite(reply.domain, reply.kind)`; ответ — `reply.message` («Добавлено: example.com») без новой «Отменить», вкладка — та, что была при удалении. Добавление: поле очищается, фокус остаётся в поле; отказ с `field === "domain"` — текст у поля (`TextField error`), поле не очищается, фокус в поле; отказ без поля — строка сообщений списка (`UndoLine error`). После удаления фокус — на «Удалить» следующей строки (или предыдущей, если удалена последняя), список опустел — на поле «Добавить сайт». Одно сообщение на экране (как `Feedback.where` в «Программах»): `where: "adult" | "list"`. Смена вкладки сбрасывает поиск, ошибку поля и сообщение списка. (Цена без перевода фокуса: после «Удалить» фокус пропадает в `body`, клавиатурой родитель теряется — урок «Программ».)
- **T8-6 `lib/sites.ts` — общий для T8 (мок) и T9 (мастер).** `cleanDomain(raw: string): string` — упрощённое зеркало `sites.NormalizeInput` для мастера и подменной программы (не для экрана, T8-1): обрезать пробелы; снять `view-source:`; пробел внутри — «»; без схемы — приписать `http://`; `new URL(...)`, протокол только `http:`/`https:`, иначе «»; `hostname` (нижний регистр и punycode даёт сам `URL`); снять точку в конце; снимать `www.`, пока после него остаётся точка; нет точки, длина > 253 или `[` в начале — «». `DEFAULT_VIDEO_SITES` — копия `ipc.DefaultVideoSites` (`internal/ipc/commands_m4.go:41-43`); `ADULT_SOURCE_TEXT` — копия `textAdultSource` (`internal/service/texts.go:79`); `SITE_LISTS: { kind: SiteKind; title: string; lead: string; empty: string }[]` (T8-4, порядок вкладок); `NOT_DOMAIN_TEXT = "Это не похоже на адрес сайта"`, `DUPLICATE_TEXT = "Этот сайт уже в списке"`. Копии сверяет тест чтением Go-файлов (образец — `lib/rest.test.ts:107`, `readFileSync`). (Цена без сверки: мастер покажет другой стартовый набор, чем кладёт миграция 009.)
- **T8-7 Подменная программа — по образцу службы.** `MockOptions.sites?: SiteDTO[]` (нет — сайты примера `ListSites`), `MockState.sites: SiteDTO[]`, `MockChild.adultFilter?: boolean` (нет — `true`, R14). Обработчики `ListSites`, `AddSite`, `RemoveSite`, `SetAdultFilter` повторяют `handlers_m4.go`: разбор `cleanDomain`; неверный `kind` — `fail("Не удалось разобрать запрос. Попробуйте ещё раз")`; «уже в списке» и «1000» — с полем `"domain"`; перенос меняет `kind` и даёт `moved: true`, `"Сайт перенесён в «Учёбу»"` (падежи как `siteListNames`); удаление — `undoUntilMs: now + 10_000`; события журнала с текстами из «Найдено» (виды `site_added`, `site_moved`, `site_removed`, `site_adult_filter`); сайты в ответе — сортировка по `kind` в порядке `blocked, study, video_social`, затем по домену (как `ListSiteRules`). (Цена без обработчиков: тест «удалить» зелёный на примере контракта, где домен не удаляется, — ловушка PL-9.)
- **T8-8 Меню.** `type Screen` + `"sites"`; в `MENU` после `{ id: "programs", label: "Программы" }` — `{ id: "sites", label: "Сайты" }` (8.3.2 п.2); `{screen === "sites" ? <Sites /> : null}`. Несохранённых правок у экрана нет (всё сохраняется сразу), охрана перехода не нужна.

## Files

Create: `frontend/src/screens/Sites.tsx`, `frontend/src/screens/Sites.test.tsx`, `frontend/src/lib/sites.ts`, `frontend/src/lib/sites.test.ts`.
Modify: `frontend/src/App.tsx`, `frontend/src/App.test.tsx` (список меню), `frontend/src/__mocks__/backend.ts`, `frontend/src/__mocks__/backend.test.ts` (обработчики), `frontend/src/styles.css` (классы `sites-*`), `frontend/e2e/screens.spec.ts` (сцены m4-sites), `docs/CODEMAP.md` (только `bash tools/codemap.sh`).
Не трогать: `frontend/src/types.ts`, `frontend/src/api.ts`, `frontend/src/contract.test.ts` (контракт полон; нужна правка — эскалация), всё вне `frontend/` кроме CODEMAP, `internal/`, `docs/` (кроме CODEMAP), `plans/`, workflows. Экраны T9 (лимиты, мастер, «Сегодня», статистика, журнал, «Что нового») — не трогать.

## Interfaces (новое; цель писателю)

```ts
// lib/sites.ts
export const NOT_DOMAIN_TEXT = "Это не похоже на адрес сайта";
export const DUPLICATE_TEXT = "Этот сайт уже в списке";
export const ADULT_SOURCE_TEXT = "Список сайтов для взрослых: UT1, Университет Тулуза Капитоль, лицензия CC BY-SA 4.0";
export const DEFAULT_VIDEO_SITES: readonly string[]; // youtube.com, vk.com, tiktok.com, rutube.ru, ok.ru, telegram.org, web.telegram.org, discord.com, twitch.tv
export interface SiteList { kind: SiteKind; title: string; lead: string; empty: string }
export const SITE_LISTS: SiteList[]; // blocked, study, video_social (T8-4)
export function cleanDomain(raw: string): string; // «» — не домен
export function sitesOf(list: SiteDTO[], kind: SiteKind): SiteDTO[]; // порядок ответа сохраняется
export function filterSites(list: SiteDTO[], query: string): SiteDTO[]; // подстрока, без учёта регистра, по обрезанному запросу

// screens/Sites.tsx
export default function Sites(): JSX.Element;

// __mocks__/backend.ts
interface MockOptions { /* … */ sites?: SiteDTO[] }
interface MockState { /* … */ sites: SiteDTO[] }
interface MockChild { /* … */ adultFilter?: boolean }
```

## Шаги (каждый: тесты до кода → код → мутации чтением → коммит)

Локально прогнать нельзя (RM4-0): «RED» здесь — тест написан до кода и перечитан против кода (что упадёт без строки кода); после push оркестратора `ci.yml` прогоняет всё. Мутационную проверку сделай мысленно по списку и запиши в вердикт, какой тест ловит.

### A. `lib/sites.ts` и сверка копий (T8-6)

Тесты `lib/sites.test.ts`:
- `cleanDomain`, таблица литералами: `"https://www.Example.com/page?x=1"` → `"example.com"`; `"YouTube.COM"` → `"youtube.com"`; `"  vk.com  "` → `"vk.com"`; `"www.www.a.ru"` → `"a.ru"`; `"www.ru"` → `"www.ru"` (после `www.` точки нет); `"пример.рф"` → `"xn--e1afmkfd.xn--p1ai"`; `"user@host.ru/x"` → `"host.ru"`; `"192.168.0.1:8080"` → `"192.168.0.1"`; `"example.com."` → `"example.com"`; `"view-source:https://a.ru/"` → `"a.ru"`; `"котики видео"` → `""`; `"localhost"` → `""`; `"brave://newtab"` → `""`; `"file:///c:/x.html"` → `""`; `""` → `""`; `"a".repeat(250) + ".com"` → `""`.
- Сверка с Go: `readFileSync(resolve(__dirname, "../../../internal/ipc/commands_m4.go"), "utf8")`, `/DefaultVideoSites = \[\]string\{([^}]*)\}/` → домены в кавычках по порядку `toEqual([...DEFAULT_VIDEO_SITES])`; `internal/service/texts.go`, `/textAdultSource\s*=\s*"([^"]+)"/` → `ADULT_SOURCE_TEXT`; `textNotDomain`, `textSiteDuplicate` → `NOT_DOMAIN_TEXT`, `DUPLICATE_TEXT`.
- `SITE_LISTS.map((l) => l.title)` → `["Запрещённые", "Учёба", "Видео и соцсети"]`, `kind` → `["blocked", "study", "video_social"]`.
- `filterSites([{domain: "youtube.com", kind: "video_social"}, {domain: "vk.com", kind: "video_social"}], " YOU ")` → только `youtube.com`; `""` → оба.

Мутации A: без снятия `www.` в цикле → `"www.www.a.ru"`; без проверки протокола → `"brave://newtab"`; копия списка без `twitch.tv` → сверка с Go.

Коммит: `feat: помощники «Сайтов» во фронте и сверка с программой`.

### B. Подменная программа (T8-7)

`backend.ts`: поля и обработчики. Тесты `__mocks__/backend.test.ts` (образец — существующие тесты там же: `installBackend(...)` + `uninstallBackend()` в `afterEach`; вызов `await backend.call(command, JSON.stringify(req))`, ответ `JSON.parse`; `createBackend` тоже экспортирован):
- `ListSites` без опций → `sites` примера (3 сайта), `children` `[{childId: 1, name: "Миша", on: true}]`, `blockedMax 1000`, `adultSource` = `ADULT_SOURCE_TEXT`.
- `AddSite {domain: "https://www.Example.org/a", kind: "blocked"}` → `ok`, `message "Добавлено: example.org"`, `domain "example.org"`, `moved false`, в `sites` есть `{domain: "example.org", kind: "blocked"}`; повтор → `{ok: false, error: "Этот сайт уже в списке", field: "domain"}`; `"просто текст"` → `"Это не похоже на адрес сайта"`, `field "domain"`; `{domain: "youtube.com", kind: "study"}` → `moved true`, `message "Сайт перенесён в «Учёбу»"`, у `youtube.com` `kind "study"`, одна запись; 1000 запрещённых (`sites` из 1000 `s<i>.test`) + новый → `"В списке запрещённых уже 1000 сайтов. Удалите ненужные"`, `field "domain"`.
- `RemoveSite {domain: "example.com"}` → `message "Удалён example.com"`, `kind "blocked"`, `undoUntilMs === now + 10000` (фальшивые часы), домена нет в `sites`; повтор → `{ok: false, error: "Этого сайта уже нет в списках"}`.
- `SetAdultFilter {childId: 1, on: false}` → `message "Миша: сайты 18+ не блокируются"`, `children[0].on false`; `{childId: 9, on: true}` → `"Такого ребёнка нет в списке"`.
- Журнал: после AddSite, переноса, удаления и переключателя в `state.events` (новые сверху) тексты `"Вы добавили сайт example.org в «Запрещённые»"`, `"Вы перенесли сайт youtube.com в «Учёбу»"`, `"Вы удалили сайт example.com из «Запрещённых»"`, `"Миша: сайты 18+ не блокируются"`, виды `site_added`, `site_moved`, `site_removed`, `site_adult_filter`.
- Без сессии `ListSites` → `"Сессия закончилась. Введите пароль"` (уже даёт `PARENT_COMMANDS`; тест закрепляет).

Мутации B: перенос без `moved` → тест переноса; удаление без `undoUntilMs` → литерал; дубль без поля → `field`.

Коммит: `test: подменная программа отвечает на команды «Сайтов» как служба`.

### C. Экран «Сайты» и меню (T8-1…T8-5, T8-8)

Тесты `screens/Sites.test.tsx` (образец `Programs.test.tsx`: `vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW })`, `installBackend({ sessionActive: true, children: [mockChild(), mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", adultFilter: false })], ...opts })`, `userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) })`, `render(<Sites />)`; `NOW = new Date(2026, 10, 5, 12, 0, 0)`):
1. Заголовок `heading "Сайты"`; пояснение `"Программа следит за сайтами только в Brave. Другие браузеры у детей закрываются"`; `group "Блокировать сайты 18+"` с `checkbox "Миша"` (`checked`) и `checkbox "Даша"` (не `checked`); под группой текст `"Список сайтов для взрослых: UT1, Университет Тулуза Капитоль, лицензия CC BY-SA 4.0"`; блок «18+» в документе раньше `tablist "Списки сайтов"` (`compareDocumentPosition`).
2. Вкладки: `tablist "Списки сайтов"` → `tab` с текстами `["Запрещённые 1", "Учёба 1", "Видео и соцсети 1"]`, выбрана первая (`aria-selected="true"`); в панели `"1 из 1000"`, `list "Запрещённые: сайты"` → `["example.com"]`; на «Учёба» — `"school.example"`, счётчика «из 1000» нет; пояснения — литералы T8-4.
3. Добавление: в поле `"Добавить сайт"` ввести `"https://www.Example.org/page"` + Enter → `status` `"Добавлено: example.org"`, вкладка `"Запрещённые 2"`, `"2 из 1000"`, поле пусто, `document.activeElement` — поле; в `mock.calls` `AddSite` с `{domain: "https://www.Example.org/page", kind: "blocked"}` (экран не нормализует, T8-1).
4. Отказы у поля: `"просто текст"` → у поля `"Это не похоже на адрес сайта"`, `aria-invalid="true"`, поле не очищено; `"example.com"` → `"Этот сайт уже в списке"`; пустое поле + «Добавить» → `"Это не похоже на адрес сайта"`, `mock.count("AddSite") === 0`; ввод после ошибки снимает её.
5. Перенос: на вкладке «Учёба» добавить `"youtube.com"` → `"Сайт перенесён в «Учёбу»"`, вкладки `["Запрещённые 1", "Учёба 2", "Видео и соцсети 0"]`; на «Видео и соцсети» — `"В этом списке пока нет сайтов"`.
6. Удаление и «Отменить»: `button "Удалить example.com"` → `"Удалён example.com"`, кнопка `"Отменить"`, `"Запрещённых сайтов пока нет"`, фокус на поле «Добавить сайт»; «Отменить» → `"Добавлено: example.com"`, `list "Запрещённые: сайты"` → `["example.com"]`, «Отменить» больше нет; второй прогон: через `9_000` мс «Отменить» есть, через ещё `1_100` — нет, текст `"Удалён example.com"` остаётся.
7. Фокус после удаления из середины: сайты `a.test`, `b.test`, `c.test` (blocked) → «Удалить b.test» → фокус на `"Удалить c.test"`; затем «Удалить c.test» → фокус на `"Удалить a.test"`.
8. «18+»: снять `checkbox "Миша"` → `status` `"Миша: сайты 18+ не блокируются"`, `checked false`, в `mock.calls` `SetAdultFilter {childId: 1, on: false}`; пока ответ задержан (`gate("SetAdultFilter")` по образцу `Programs.test.tsx:70`) — оба переключателя `disabled`; отказ (`mock.override("SetAdultFilter", { ok: false, error: "Не удалось сохранить. Попробуйте ещё раз" }, 1)`) → `alert` с этим текстом, `checkbox "Миша"` снова `checked`.
9. Один запрос за раз: `gate("AddSite")`, два нажатия «Добавить» → `counter.calls === 1`, кнопка `"Сохраняю…"`.
10. Поиск: 12 сайтов `s1.test`…`s12.test` (blocked) → поле `"Найти в списке"`, ввод `"s1"` → `["s1.test", "s10.test", "s11.test", "s12.test"]`; `"zzz"` → `"Ничего не найдено"` и `button "Сбросить поиск"` → снова 12; смена вкладки очищает поиск; пустой список — поля поиска нет.
11. Нет детей (`children: []`) → `"Детей пока нет. Добавьте их в разделе «Настройки» → «Детские учётки»"`, групп с переключателями нет, вкладки есть.
12. Загрузка и ошибки: до ответа `"Загружаю данные"`; `linkDown: true` → `"Программа не отвечает. Попробуйте ещё раз через минуту"` и `button "Повторить"` → после `mock.state.linkDown = false` и нажатия список показан; `ListSites` с `sites: null` (`mock.override`) → не падает, `"Запрещённых сайтов пока нет"`.
13. Тексты: во всём `document.body.textContent` нет `—` и `–`.

`App.test.tsx:228`: меню → `["Сегодня", "Дети", "Программы", "Сайты", "Копилка", "Статистика", "Журнал", "Настройки", "Что нового"]` (название теста — так же); новый тест: «Сайты» в меню → `heading "Сайты"`, `aria-current="page"`.

Мутации C: экран нормализует ввод → шаг 3 (`mock.calls`); оптимистичный переключатель → шаг 8 отказ; без `Math.min(…, 10_000)` при `undoUntilMs` далеко в будущем (подмени ответ `RemoveSite` с `undoUntilMs: Date.now() + 60_000`) → «Отменить» через 10,1 с всё ещё видна; без сброса поиска при смене вкладки → шаг 10.

Коммит: `feat: экран «Сайты» со списками, переключателями 18+ и отменой удаления`.

### D. Снимки Playwright

`frontend/e2e/screens.spec.ts`, новые сцены (переход по меню — как `openPrograms`; дети — `family()` с `adultFilter: false` у Даши; сайты — свой набор):
- `m4 сайты: списки и 18+` — окно 1200×820, `sites`: 4 запрещённых (`example.com`, `casino.example`, один домен длиной 120 знаков `"очень-длинный-поддомен-…".test` латиницей, `ads.example`), 2 учебных, стартовый набор видео; проверки: `noHorizontalScroll`, длинный домен целиком внутри строки (правый край ≤ правого края списка + 0,5), кнопки «Удалить» в одну колонку (правые края равны ± 0,5), подпись источника видна под группой. Снимок `m4-sites.png` (`wholePage`).
- `m4 сайты: тёмная тема` — `emulateMedia({ colorScheme: "dark" })`, тот же набор → `m4-sites-dark.png`.
- `m4 сайты: узкое окно` — 960×620 → `noHorizontalScroll`, поле и кнопка «Добавить» в одну строку (верх ± 0,5) → `m4-sites-960.png`.
- `m4 сайты: ошибка у поля и отмена удаления` — ввод `"просто текст"` + Enter → ошибка у поля видна → `m4-sites-error.png`; «Удалить example.com» → сообщение с «Отменить» → `m4-sites-undo.png`; после прокрутки к полю (`scrollIntoView({ block: "start" })`) шапка `top = 0` (PL-25).
- `m4 сайты: вкладка «Видео и соцсети»` — стартовый набор из 9 доменов → `m4-sites-video.png`.

Разбор снимков — `visual-reviewer` у оркестратора (ты картинки не читаешь).

Коммит: `test: снимки экрана «Сайты»`; затем `bash tools/codemap.sh` и `docs: обновить карту кода (T8)` — или вместе с последним коммитом кода.

## Среда и команды

```
cd /home/claude/pc_block_soft
python3 tools/scan_ui_texts.py
bash tools/codemap.sh
git add <пути явно>; git commit   # трейлеры ниже
```
npm, vitest, tsc, Playwright локально **не работают** (RM4-0) — не пытайся ставить. Проверка типов — чтением: импорты, имена из `types.ts`, `api.ts` дословно; в `frontend/tsconfig.json` включены `strict`, `noUnusedLocals`, `noUnusedParameters` — ни одной неиспользуемой переменной и параметра (непрогнанный `tsc` уронит весь `ci.yml`).

## Review Focus (самопроверка)

1. Экран не нормализует домен, всё состояние — из `reply.sites`; переключатель не оптимистичный (T8-1, T8-2).
2. 8.0: п.3 цвет только статус; п.5 недоступная кнопка объясняет (кнопки во время запроса — «Сохраняю…»); п.7 результат каждого действия с «Отменить» у удаления; п.8 Enter — «Добавить»; ошибки у поля простыми словами (тексты службы).
3. Доступность: имена у `group`, `tablist`, `tab`, `tabpanel` (`aria-labelledby` вкладки), `list`; стрелки влево/вправо, Home, End переключают вкладки списков (как `Totals` в `MyStats.tsx`); «Удалить <домен>» различимы для диктора; фокус после действий (T8-5).
4. Длинный домен (до 253 знаков) не ломает строку; окно 960×620 без прокрутки вбок; документ не прокручивается (PL-25).
5. Мок отвечает как служба (T8-7): тесты экрана не проходят на «примере контракта».

## Global Constraints (дословно из плана)

- Ядро `internal/core` и новый пакет `internal/sites` — только стандартная библиотека; тесты идут локально (`GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 ./internal/core/... ./internal/sites/...`, Go 1.27 в `~/.local/go1.27`). Правила, цифры и тексты считает ядро и служба; страж, значок, окна — только показывают.
- Тексты ребёнку на «ты», родителю на «вы», без длинных тире (`python3 tools/scan_ui_texts.py`), без технических ошибок. Тексты плашек — дословно ТЗ 8.2.5.
- R23: страж читает адрес вкладки только в памяти и сразу оставляет домен; ни полный адрес, ни заголовок окна не передаются, не пишутся в журнал работы и не хранятся. Хранится только домен.
- NFR-1: ни одного сетевого соединения от программы. Список «18+» встроен в программу при сборке (D-67); загрузка UT1 — только в CI при сборке, никогда в программе.
- NFR-4 установщик ≤ 40 МБ; NFR-5 служба ≤ 50 МБ, страж ≤ 40 МБ памяти; NFR-6 вкладка закрыта ≤ 2 с, программа ≤ 1 с; ST2 проверка домена ≤ 1 мс (99-й процентиль), загрузка списка ≤ 2 с.
- Репозиторий публичный; self-hosted runner заказчика к workflow НЕ привязывать; на ПК заказчика ничего не ставить и блокировки не запускать; приёмка только в CI на windows-latest в тестовом режиме. Запуски `windows.yml` — по одному.
- Аварийный выход (NFR-10) в каждой версии: S3, S7, S25, S26 в регрессе. Новый код не может заблокировать компьютер без выхода и не закрывает окна PC Block, WebView2 и системные процессы (PL-18, PL-19).
- Наблюдатель в тестовом режиме трогает только `WatchDir`; приёмка браузеров — копиями тестового exe с именем браузера в `WatchDir` (RM3-8).
- Предупреждения о видео: пороги 30/15/5/1 (D-54), табло за 30/15/5, плашка 1 мин, звук заказчика — тот же путь, что у игр; порог взводится, только если до события строго больше его минут (PL-14).
- Плашки стража имеют старшинство (PL-34, Ruling FB14-9); новые плашки сайтов — места в RM4-9.
- Миграции — следующий номер 009; ТЗ правится только заменой (D-35); документы — коммит в main, код — ветка и PR.
- Версия 0.4.0 — `internal/version/version.go` и `wails.json` вместе (тест сверяет).
- `go.sum` обновляет CI; GraphQL закрыт — PR, слияние, выпуск через `gh api` (CLAUDE.md). Журналы Actions не скачиваются — пометки сборки (`tools/annotate-errors.sh`).
- Нестабильные: B-20 (`TestRunStartsOneGuardAndStopsOnClose`), B-47 (снимок окна) — перезапускать только упавшее задание.
- Писатель коммитит пути явно (`git add <пути>`, никогда `-A`), не пушит; пушит оркестратор. Трейлеры коммитов: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze`.
- Среда этого чата: npm-реестр из песочницы закрыт, поэтому фронт (`tsc`, `vitest`, `vite build`) проверяется только в `ci.yml` после push (Ruling RM4-0).

## Skills

Загрузи `anthropic-skills:uiux-design` в режиме «рабочее приложение» (ТЗ 8.0: сдержанная светлая тема, без градиентов, стекла и «пилюль», скругление кнопок 6 px, Segoe UI Variable, цвет только статус, токены 8.0 п.9). `anthropic-skills:in-app-guidance` **не** грузить (D-56). Коммиты — Conventional Commits, описание по-русски в повелительном наклонении (`anthropic-skills:git-guru` по желанию).

## Уроки (docs/LESSONS.md)

- **PL-25** (C): область прокрутки — позиционированный предок; скрытые подписи (`.visually-hidden`) только внутри `main.content`; в снимке «ошибка у поля» проверить шапку после `scrollIntoView` к полю.
- **PL-26** (C): длинный домен рядом с «Удалить» (`flex: none`) мерить на 960×620; домен переносится, а не съедается до нуля.
- **PL-27** (E): в e2e искать внутри группы и по точному имени (`exact: true`): вкладка «Учёба» есть и в «Итогах», кнопка «Удалить» — с доменом; ждать окончания переходов перед замером.
- **PL-34** (B): сообщения экрана — одно на место (`where`); ошибка не вытесняется сообщением другого блока.
- **PL-36** (C): переключатели «18+» — в своём блоке рядом с подписью источника; `tabpanel` охватывает только меняющееся (список и форму добавления), блок «18+» вне панели; фокус ошибки — на поле; у строки сообщений `role="status"`, а не `aria-live` на весь список.
- **PL-29** (A): один текст — одно место: сообщения — от службы; копии констант во фронте (`DEFAULT_VIDEO_SITES`, `ADULT_SOURCE_TEXT`) сверяет тест с Go.
- **PL-39** (D): один разборщик домена на вход — у службы; `cleanDomain` только там, где службы нет (мастер, мок).
- **PL-9** (E): тест, зелёный на «примере контракта», ничего не доказывает — мок с настоящим состоянием (T8-7).

## Open (передать дальше, не делать в T8)

- Виды нарушений журнала (`site_forbidden_try`), колонка видео, мастер, «Итоги», «Что нового» — T9.
- `cleanDomain` расходится с `sites.FromOmnibox` в редких случаях (IDN с заглавными, нестандартные IP вроде `0x7f.1`): на экране не используется; мастер ловит отказ службы по полю `domain` (T9).

## Вердикт (≤ 30 строк)

```
TASK T8: done | blocked | failed
Diff: sha + строка на коммит
Evidence: scan_ui_texts, codemap (diff только frontend-строки), список тестов vitest по файлам (имена), сцены Playwright и имена снимков, мутации (какой тест ловит), «прогон только в ci.yml (RM4-0)»
Interfaces: для T9 (lib/sites.ts: cleanDomain, DEFAULT_VIDEO_SITES, ADULT_SOURCE_TEXT, NOT_DOMAIN_TEXT, DUPLICATE_TEXT; MockChild.adultFilter; MockOptions.sites)
Rulings: T8-1…T8-8 подтверждены или изменены, своя цена ошибки
Escalations: правка ожиданий старых тестов (App.test меню), любые правки вне Files
Open: что осталось на T9 и P1
```
