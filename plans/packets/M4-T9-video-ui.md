# Пакет: T9 вехи M4 «Видео-лимит, мастер «Сайты», «Итоги» по сайтам» | слот: писатель | исполнитель: новый агент

Репозиторий `/home/claude/pc_block_soft`, ветка `feature/m4-sites`. Прочитай `plans/2026-11-04-M4.md`: Global Constraints, RM4-0, RM4-4, RM4-6, RM4-7, UX-сценарий, Review Focus п. 3, 4, 9; строку T9. ТЗ (`docs/01_ТЗ.md`): R2, R3, R6, R14, FR-2, FR-12, FR-15, FR-16, FR-23, FR-33, 8.0, 8.2.6 п.1–3 и 8, 8.3.1 п.4–7, 8.3.2 п.3, 8.3.3 п.1–2, 8.3.9. Уроки — раздел «Уроки» ниже. **T8 уже слит в ветку** (экран «Сайты», `lib/sites.ts`, мок сайтов): сверь, что из «Interfaces T8» есть в коде. Чужие правки Go в дереве (`internal/programs`, `internal/service`) не трогать, `git add` — только свои пути.

Перед работой сверь адреса (PL-4): `git log --oneline -8`, `grep -n "export function\|export const" frontend/src/lib/limits.ts frontend/src/lib/mytoday.ts frontend/src/lib/sites.ts`, `grep -n "function statusCore\|function gamesOf\|function cardOf\|function myRules\|function gamesRuleText\|function myToday\|function limitsWithGames\|const limitsError\|CopyRules:" frontend/src/__mocks__/backend.ts`, `grep -n "type Step\|const STEPS\|const titles\|finish = async" frontend/src/screens/Wizard.tsx`, `grep -n "TOTALS_TABS\|NOT_COUNTED" -r frontend/src frontend/e2e`.

## Бриф

Окно родителя и окно ребёнка показывают лимит «Видео и соцсети» (R6): колонка в «Дети → Лимиты» (FR-16) и в копировании правил (FR-33), шаг мастера 5 «Сайты» (FR-2), строка «Видео 10 мин из 20 мин» на «Сегодня», строка «Видео и соцсети» в «Сколько осталось» и «Мои правила» ребёнка, вкладка «Видео и соцсети» в «Итогах» со строками сайтов (больше не «Пока не считается»), нарушение «попытка открыть запрещённый сайт» в журнале, запись «Что нового» 0.4.0. Цифры и тексты — от службы (T2–T4 готовы); фронт раскладывает. **npm закрыт (RM4-0)**: тесты пишутся полностью до коммита, ожидания — литералы этого пакета; прогон — `ci.yml` после push оркестратора.

## Найдено читателем (проверено по `ffe9906`)

- **Ловушка потери видео-лимита.** Служба читает день **без ключа `videoUnlimited` как «видео без лимита»** (`ipc.WeekdayLimit.UnmarshalJSON`, `internal/ipc/commands_m4.go`, S-3). А фронт выбрасывает поля видео: `normalizeLimits` и `wireLimits` (`lib/limits.ts:59-78, 138-149`) собирают только `weekday, totalMin, unlimited, gamesMin, gamesUnlimited`; `copyDay` (`:152`), `limitsEqual` (`:171`), `validateLimits` (`:193`), `setDay` (`:81`) видео не знают; `limitErrorKey` (`:207`) принимает только `limits|games`. Значит, **сейчас любое «Сохранить» на «Лимитах» стирает лимит видео ребёнка**, а «Скопировать правила» его не переносит (служба копирует: `limitsView`, `internal/service/service.go:674`). Мок повторяет ловушку: `limitsWithGames` (`backend.ts:295`) идёт через `normalizeLimits`. Поля в `types.ts`: `WeekdayLimit.videoMin?`, `videoUnlimited?` (`:107-111`); `api.withGames` (`api.ts:168`) дописывает только игры.
- Ошибки сохранения у поля видео: `field = "video.<день>"` (`ipc.VideoField`, `commands.go:135`), тексты `"Не больше 24 ч"`-подобные — те же, что у игр (`handlers.go:185-195`).
- **Таблица лимитов** — `components/LimitsTable.tsx`: колонки `"total" | "games"` (`LimitColumn`), `ZERO_HINT`, `LABELS`, `GAMES_NOTE = "Время игр входит в общее. Учёба в общее время не входит"`; одна таблица и в «Дети → Лимиты» (`ChildLimits.tsx:302`), и в мастере (`Wizard.tsx:340`). Ширина: `.limits-wrap { max-width: 640px }`, `.limits-num { width: 60px }`, ячейки `padding: var(--space-1) var(--space-3)` (`styles.css:1001-1056`), в узком окне 56 px (`:2019-2029`). Расчёт: ячейка ≈ 2×60 + «ч» + «мин» + 3 зазора 8 + 24 ≈ 205 px, три колонки + день ≈ 740 px, а область «Дети» в окне 960 — ≈ 712 px (меню 200, поля 2×24): **третья колонка не влезет без правки размеров**. Окно мастера — от 720 px (`internal/ui/app.go:178`), e2e `s12 мастер, шаг времени в узком окне` (`screens.spec.ts:1646`) требует таблицу внутри панели.
- Тест `ChildLimits.test.tsx:64` закрепляет `columnheader /Видео/` = `null` — переворачивается. Пояснение вкладки: `"Сколько времени в день можно пользоваться компьютером и сколько из него можно играть."` (`ChildLimits.tsx:293`). e2e `s12 лимиты с играми` (`:1620`) ищет текст `GAMES_NOTE` дословно.
- **Мастер** (`Wizard.tsx`): `type Step = 1 | 2 | 3 | 4 | "done"`, `STEPS = 4`, `titles = {1: "Пароль родителя", 2: "Кто дети", 3: "Время", 4: "Программы"}`; на шаге 4 кнопки «Назад», «Настрою позже» (`finish(false)` — без программ), «Готово» (`finish(true)`); `finish` зовёт `setup(password, children, programs?)`, отказы по полю: `password` → шаг 1, `children` → шаг 2, `limitErrorKey` → шаг 3, иначе `formError`. `api.setup` уже принимает 4-й аргумент `sites?: SetupSites` (`api.ts:172`), `SetupSites {adultOff: string[] (SID), video: string[]}`. Служба (`handlers.go:249, 312-360`): `sites` нет — «18+» включён всем, «Видео и соцсети» — стартовый набор миграции 009; есть — `video` **заменяет** список целиком, неразборный домен → отказ `"Это не похоже на адрес сайта"` с `field: "domain"`, повторы схлопывает. До `Setup` родитель не вошёл: команды «Сайтов» мастеру недоступны — разбор только `cleanDomain` (`lib/sites.ts`, T8-6). Пример `Setup#sites` в контракте уже есть. Тесты мастера: `Wizard.test.tsx` — помощники до «Шаг 4 из 4» (`:28-46`), `done = () => getByRole("button", { name: "Готово" })`, 15+ тестов жмут «Готово»/«Настрою позже» на шаге 4; e2e `s01` (`:113` «Шаг 1 из 4»), `s12 мастер, шаг программ` (`:1804-1838`: «Шаг 3 из 4», «Шаг 4 из 4», кнопки `["Назад", "Настрою позже", "Готово"]`).
- **«Сегодня»** (`Today.tsx:270`): `{card.gamesText ? <p className="child-used child-games">…` — строки видео нет. `TodayCard.videoText?` (`types.ts:180`), служба: `ipc.VideoCardText` = `"Видео " + FormatDuration(usedS) + " из " + FormatLimit(limitS/60)`, без лимита видео — `""` (`internal/ipc/status.go:170`). Мок `cardOf` (`backend.ts:459`) `videoText` не даёт.
- **Окно ребёнка** (`lib/mytoday.ts`): `todayRows` даёт `{ label: "Видео и соцсети", value: NOT_COUNTED }` (`:238`), `NOT_COUNTED = "Пока не считается"` (`:53`); `gamesLeftText` (`:188`) — образец; правило лимита у службы `hasVideoLimit = !VideoUnlimited && (VideoLimitS > 0 || CloseVideo)` (`status.go:154`). Поля `ChildStatus.videoRemainingS?, videoLimitS?, videoUnlimited?, closeVideo?` (`types.ts:75-81`); в примере `GetToday` у Миши `videoUnlimited: false, videoLimitS: 0, closeVideo: false` — **это «лимита нет»** по правилу службы, не «нельзя». Подсказка значка у службы — `"\nВидео: " + …` после игр (`status.go:104-107`); мок `statusCore` (`backend.ts:466-471`) видео не дописывает.
- **«Мои правила»** строит служба: общие правила `gamesRuleText`, затем `videoRuleText` = `catRuleText(r, "Видео и соцсети", …)` (`handlers.go:978-997`), затем режим окончания; фронт показывает `rules` как есть (`MyStats.tsx:62-110`). Мок `myRules` (`backend.ts:482-497`) видео не пишет; `gamesRuleText` мока (`:500`) — образец (служба: все 7 дней с лимитом и одинаковы → `"<Т>: до 1 ч в день"` / `"<Т>: нельзя"`, иначе `"<Т>: пн до 1 ч, ср нельзя"`).
- **«Итоги»** (`MyStats.tsx:222-330`, общие с `ParentStats.tsx`): `TOTALS_TABS` — `all, games, study, other` (`mytoday.ts:59`); строка `stat-row-muted` «Видео и соцсети — Пока не считается» (`MyStats.tsx:274-277`); `tabRows` пропускает `video_social` во «Всего» (`mytoday.ts:154`); `tabSeconds` видео не знает; `TotalsTab` — без видео. Служба (`internal/service/stats.go`): `videoS` за период, строки `ProgramTime {path, name, category: "video_social", seconds, kind: "site"}` (у сайта `path` = домен), «Остальное время» у каждой категории, порядок категорий `games, video_social, study, other`; `otherS` = общее без игр и видео; `allS` = общее + учёба. `api.myTodayOf` уже даёт `videoS ?? 0` (`api.ts:314`). Мок `myToday` (`backend.ts:534-579`): `otherS = usedS − gamesS` («видео в M3 нет»), «Остальное время» только для `games, study, other`. Тесты, которые меняются: `MyStats.test.tsx:244-256` («Пока не считается», вкладки «Видео» нет), `:316` и `:358` (список вкладок), `:607-622` (строки «Сколько осталось»), `:428-436` (общие правила); `mytoday.test.ts:193`; e2e `:755`, `:962` (`toHaveCount(4)`), `:977` (имена вкладок). Помощник `listRows` (`MyStats.test.tsx:293`) ищет список по `/по программам$/`.
- **Журнал**: тексты событий строит служба (`internal/service/journaltext.go:106-130`), фронт превращает событие в текст **нигде** — показывает `JournalEntry.text` (`Journal.tsx:108`). Фронт решает только, что нарушение: `VIOLATION_KINDS = ["bypass", "program_forbidden_try"]` (`lib/journal.ts:6`), по нему — фильтр «Журнал: нарушения» (`Journal.tsx:60`) и счётчик «Нарушения сегодня: N» (`Today.tsx:55`). У службы нарушение из сайтов — только `site_forbidden_try` (`sites_svc.go:152-163`); прочие новые виды: `site_closed_video`, `site_closed_study_only`, `site_unread`, `site_added`, `site_moved`, `site_removed`, `site_adult_filter`, `brave_restored` — не нарушения. Тексты для мока: `"Миша: попытка открыть запрещённое: сайт example.com"`, `"Миша: попытка открыть запрещённое: сайт для взрослых adult.example"`, `"Миша: закрыт сайт youtube.com: время на видео и соцсети закончилось"`, `"Миша: закрыт сайт youtube.com: время вышло, доступна только учёба"`, `"Правила Brave восстановлены"`.
- **«Что нового»** (`whatsnew/entries.ts`): `{ version, title, items[] }`, новая сверху; Go-тест `internal/version/whatsnew_test.go` требует только запись текущей `Version` (сейчас 0.3.3; 0.4.0 поднимается в Фазе 8) — запись 0.4.0 сверху его не ломает. Тесты, которые меняются: `entries.test.ts:27` (список версий), `:31-58` (индексы `WHATS_NEW[0]`, `[1]`…), `:147` (латиница только «PC Block», «Windows», «Brave»), `App.test.tsx:285`, `WhatsNew.test.tsx:17`, e2e `:322` («Версия 0.3.3. …» первой).
- Мок по умолчанию: `mockChild()` берёт лимиты из примера `GetChildRules` — **у Миши уже есть видео**: пн `videoMin 60`, ср `videoMin 0` (нельзя), остальные без лимита. После правки мока тесты «Сегодня», «Моей статистики» в понедельник увидят «Видео 0 мин из 1 ч» и «Осталось 1 ч 00 мин из 1 ч» — это ожидаемо, менять литералы. e2e `child()` (`screens.spec.ts:69`) — лимиты без полей видео → видео без лимита, старые снимки не меняются.

## Решения читателя (оркестратор подтверждает до пуска; цена ошибки — в скобках)

- **T9-1 Лимиты видео — по образцу игр, но по правилу службы для «нет поля».** `LimitColumn = "total" | "games" | "video"`; `videoOff(l) = l.videoUnlimited ?? true` (нет ключа — без лимита, S-3; у игр иначе: `gamesUnlimited ?? gamesMin === undefined`). `templateLimits` → `videoMin: 0, videoUnlimited: true`; `normalizeLimits`, `wireLimits`, `copyDay`, `limitsEqual`, `validateLimits` (`"Не больше 24 ч"` у `video.N`), `setDay` (тип `change` + видео), `columnMinutes/columnUnlimited/setMinutes/toggleUnlimited` (снятие «Без лимита» при 0 мин → `VIDEO_FALLBACK_MIN = 60`), `limitField("video", d)` → `"video.<d>"`, `limitErrorKey` → `/^(limits|games|video)\.[1-7]$/`. `wireLimits` **всегда** шлёт оба поля видео. `api.withGames` дописывает и `videoMin: 0, videoUnlimited: true` перед `...l` (примеры контракта уже с видео — `contract.test.ts` не меняется). Мок: `limitsWithGames` сохраняет видео по тому же правилу; `limitsError` проверяет `video.N`. (Цена: без этого первое «Сохранить» после обновления молча снимает лимит видео — R6 не работает, а родитель видит «Сохранено».)
- **T9-2 Колонка «Видео и соцсети» только в «Дети → Лимиты»; мастер — две колонки.** `LimitsTable` получает `columns?: LimitColumn[]` (по умолчанию `["total", "games", "video"]`), мастер передаёт `["total", "games"]` и шлёт видео без лимита. Подписи: заголовок `Видео и соцсети`; `LABELS.video = { hours: "Часы видео", minutes: "Минуты видео", unlimited: "Видео без лимита" }`; `ZERO_HINT.video = "0 мин: в этот день видео и соцсети закрыты"`. Пояснение под таблицей: с колонкой видео — `"Время игр и видео входит в общее. Учёба в общее время не входит"`, без неё — прежний `GAMES_NOTE`. Пояснение вкладки «Лимиты»: `"Сколько времени в день можно пользоваться компьютером и сколько из него можно играть и смотреть видео и соцсети."`. Вёрстка: `.limits-wrap { max-width: 760px }`, `.limits-num { width: 52px }`, ячейки `padding: var(--space-1) var(--space-2)` (расчёт: ячейка ≈ 173 px, таблица ≈ 635 px из 712 в окне 960). (Цена трёх колонок в мастере: окно 720 px даёт прокрутку вбок, `s12 мастер, шаг времени в узком окне` красный; цена двух — видео-лимит задаётся только после мастера, шаг «Сайты» говорит, где.)
- **T9-3 Шаг мастера 5 «Сайты» (FR-2, 8.3.1 п.5–7).** `Step = 1 | 2 | 3 | 4 | 5 | "done"`, `STEPS = 5`, `titles[5] = "Сайты"`. Шаг 4: кнопки «Назад», «Настрою позже» (программы пропущены → шаг 5, флаг `skipPrograms`), «Далее» (`btn-primary` → шаг 5). Шаг 5: пояснение `"Программа следит за сайтами в Brave. Другие браузеры у детей закрываются. Лимит на видео и соцсети можно задать позже в разделе «Дети», вкладка «Лимиты»"`; группа `"Блокировать сайты 18+"` — переключатель на каждого выбранного ребёнка (подпись — имя, по умолчанию включён, R14), под ней `ADULT_SOURCE_TEXT`; блок `h3` «Видео и соцсети» с пояснением `"Время на этих сайтах можно ограничить отдельно. Список можно изменить позже в разделе «Сайты»"`, список `"Видео и соцсети: сайты"` из `DEFAULT_VIDEO_SITES`, у строки кнопка «Убрать» (`aria-label="Убрать <домен>"`), поле `"Добавить сайт"` + «Добавить» (Enter): `cleanDomain` → «» — ошибка у поля `NOT_DOMAIN_TEXT`; уже в списке — `DUPLICATE_TEXT`; иначе домен в конец списка, поле пусто, фокус в поле. Кнопки: «Назад», «Настрою позже» (`setup` без `sites`), «Готово» (`setup(..., programs, { adultOff: <SID выключенных>, video: <список> })`). Отказ `Setup` с `field === "domain"` → шаг 5, ошибка у поля «Добавить сайт». Возврат «Назад» и «Далее» сохраняет выбор (как отметки программ). Дети, снятые на шаге 2, не попадают в `adultOff`. (Цена `sites` при «Настрою позже»: служба заменила бы стартовый набор урезанным списком.)
- **T9-4 «Сегодня»:** `{card.videoText ? <p className="child-used child-video">{card.videoText}</p> : null}` сразу после строки игр. Мок `cardOf`: `videoText` = `""` без лимита видео дня, иначе `` `Видео ${formatDuration(videoUsedS)} из ${formatLimit(videoLimitS / 60)}` `` (как `VideoCardText`, при лимите 0 — «Видео 0 мин из 0 мин», так же, как служба). (Цена своей сборки строки: расхождение со службой.)
- **T9-5 Окно ребёнка.** `hasVideoLimit(s) = s.videoUnlimited === false && ((s.videoLimitS ?? 0) > 0 || s.closeVideo === true)` — копия правила службы. `videoLeftText(s)`: admin/safe → `"Не действует"`; `!hasVideoLimit` → `"Без отдельного лимита"`; `videoLimitS <= 0` → `"Сегодня нельзя"`; иначе `` `Осталось ${formatDuration(left)} из ${formatLimit(Math.round(videoLimitS / 60))}` ``, `left = timeOpen(s) && !s.studyOnly ? upToMinute(videoRemainingS) : 0`. `todayRows` — строка `"Видео и соцсети"` = `videoLeftText`. `NOT_COUNTED` удаляется (grep — других потребителей нет). Мок `statusCore`: `videoOf(child, nowMs)` по образцу `gamesOf` (`MockChild.videoUsedS?`, лимит видео + доп. время), `closeVideo = лимит есть && остаток 0`, подсказка `"\nВидео: <мин>"` после игр при `hasVideoLimit`; админ — `videoUnlimited: true`. «Мои правила»: мок `myRules` после игр — `videoRuleText` (общий помощник `catRuleText(limits, title, pick)` вместо копии `gamesRuleText`). (Цена «лимит 0 ⇒ нельзя» без проверки `videoUnlimited`: у старой службы и в примере `GetToday` ребёнок увидел бы «Сегодня нельзя» без лимита.)
- **T9-6 «Итоги»: вкладка «Видео и соцсети».** `TotalsTab = "all" | "games" | "video_social" | "study" | "other"`; `TOTALS_TABS` — `Всего, Игры, Видео и соцсети, Учёба, Прочее` (порядок 8.2.6 п.3 и `statsCatOrder`); `tabSeconds("video_social") = reply.videoS ?? 0`; `tabRows("all")` больше **не** пропускает `video_social` (сайты — строки «Всего»; ключ строки — `path`, у сайта домен). Строка «Видео и соцсети — Пока не считается» из `stat-list` убирается. Имя списка вкладки: `"Видео и соцсети: по сайтам"`, у остальных — прежнее `"<вкладка>: по программам"` (и у «Всего», хотя там есть сайты — чтобы не трогать помощники тестов; цена: диктор говорит «по программам» у смешанного списка). Сетка вкладок `minmax(112px, 1fr)` — подпись «Видео и соцсети» переносится внутри вкладки, это допустимо; e2e на 640×520 подтверждает. Мок `myToday`: `videoS = (child.videoUsedS ?? 0) + sum(d.videoS)`, `otherS = max(0, usedS − gamesS − videoS)`, «Остальное время» и для `video_social`, `MockDay.videoS?`. (Цена: без правки «Всего» строки сайтов не сходятся с плиткой «Всего», PL-28.)
- **T9-7 Журнал.** `VIOLATION_KINDS = ["bypass", "program_forbidden_try", "site_forbidden_try"]`; комментарий — какие виды не нарушения (список «Найдено»). (Цена: «Нарушения сегодня» молчит о попытках открыть запрещённые сайты.)
- **T9-8 «Что нового» 0.4.0** — первой записью:
  ```ts
  {
    version: "0.4.0",
    title: "Сайты и Brave",
    items: [
      "Новый раздел «Сайты»: списки «Запрещённые», «Учёба» и «Видео и соцсети». Можно вставить адрес страницы целиком, программа возьмёт из него сайт",
      "Сайты для взрослых закрываются. Переключатель «Блокировать сайты 18+» у каждого ребёнка свой, сначала включён у всех",
      "Отдельный лимит на видео и соцсети по дням недели во вкладке «Лимиты». Предупреждения за 30, 15, 5 и 1 минуту, как у игр",
      "Время на учебных сайтах не тратит лимит",
      "У детей работает только браузер Brave: другие браузеры закрываются с сообщением «Этот браузер запрещён. Пользуйся Brave»",
      "В Brave у детей нет окон Tor и кнопки VPN, а расширения ставить нельзя",
      "Программа из магазина приложений Windows теперь одной строкой в «Программах», даже после обновлений",
    ],
  },
  ```
  Тест латиницы (`entries.test.ts:147`) получает исключения `Tor|VPN` (названия кнопок в самом Brave) — **эскалация: правка ожидания старого теста**. (Цена описательных замен «анонимные окна», «обход блокировок»: родитель не узнает пункты меню Brave.)

## Files

Create: `frontend/e2e` — только новые сцены в `screens.spec.ts` (см. шаг G).
Modify: `frontend/src/lib/limits.ts`, `lib/limits.test.ts`, `components/LimitsTable.tsx`, `screens/ChildLimits.tsx`, `screens/ChildLimits.test.tsx`, `screens/Wizard.tsx`, `screens/Wizard.test.tsx`, `screens/Today.tsx`, `screens/Today.test.tsx`, `lib/mytoday.ts`, `lib/mytoday.test.ts`, `screens/MyStats.tsx`, `screens/MyStats.test.tsx`, `screens/ParentStats.test.tsx` (если ломается список вкладок), `lib/journal.ts`, `lib/journal.test.ts`, `screens/Journal.test.tsx`, `whatsnew/entries.ts`, `whatsnew/entries.test.ts`, `whatsnew/WhatsNew.test.tsx`, `App.test.tsx` (строка «Что нового»), `api.ts` (только `withGames`), `api.test.ts` (если там литерал `withGames`), `__mocks__/backend.ts`, `__mocks__/backend.test.ts`, `styles.css` (`.limits-*`, `.child-video`, `.wizard-sites-*`), `frontend/e2e/screens.spec.ts`, `docs/CODEMAP.md` (только `bash tools/codemap.sh`).
Не трогать: `types.ts`, `contract.test.ts`, `screens/Sites.tsx` и `lib/sites.ts` (T8; нужна правка — эскалация), всё вне `frontend/` кроме CODEMAP, `internal/version` (версия — Фаза 8), `CHANGELOG.md`, `plans/`, workflows.

## Interfaces (новое; цель писателю)

```ts
// lib/limits.ts
export type LimitColumn = "total" | "games" | "video";
export const VIDEO_FALLBACK_MIN = 60;
export function limitField(column: LimitColumn, weekday: number): string; // "limits.3" | "games.3" | "video.3"
// templateLimits/normalizeLimits/wireLimits/copyDay: все семь полей дня, videoUnlimited ?? true

// components/LimitsTable.tsx
interface Props { /* … */ columns?: LimitColumn[] } // по умолчанию ["total", "games", "video"]
export const GAMES_NOTE: string;          // прежний
export const GAMES_VIDEO_NOTE = "Время игр и видео входит в общее. Учёба в общее время не входит";

// lib/mytoday.ts
export type TotalsTab = "all" | "games" | "video_social" | "study" | "other";
export function hasVideoLimit(s: ChildStatus): boolean;
export function videoLeftText(s: ChildStatus): string;

// __mocks__/backend.ts
interface MockChild { /* … */ videoUsedS?: number }
interface MockDay { /* … */ videoS?: number }
```

## Шаги (каждый: тесты до кода → код → мутации чтением → коммит)

### A. Лимиты видео в данных (T9-1)

`lib/limits.test.ts`, литералы: `templateLimits()[0]` → `{weekday: 1, totalMin: 120, unlimited: false, gamesMin: 0, gamesUnlimited: true, videoMin: 0, videoUnlimited: true}`; `normalizeLimits([{weekday: 1, totalMin: 90, unlimited: false, gamesMin: 30, gamesUnlimited: false, videoMin: 20}])[0]` → `videoMin: 0, videoUnlimited: true` (нет ключа — без лимита); с `videoUnlimited: false` → `videoMin: 20, videoUnlimited: false`; `wireLimits` дня `{… videoMin: 20, videoUnlimited: false}` → те же поля; дня без полей видео → `videoMin: 0, videoUnlimited: true`; `copyDay(limits, 1, [2])` переносит `videoMin: 20, videoUnlimited: false`; `limitsEqual` различает только видео (20 и 30) → `false`, `videoUnlimited: true` с разными минутами → `true`; `validateLimits` `videoMin: 1500, videoUnlimited: false` → `{"video.4": "Не больше 24 ч"}`; `toggleUnlimited(l, 3, false, "video")` при `videoMin 0` → `60`; `limitErrorKey("video.7")` → `"video.7"`, `"video.8"` → `null`. `api.test.ts`/`contract.test.ts`: `saveLimits` с днём без полей видео шлёт `videoMin: 0, videoUnlimited: true`. Мок (`backend.test.ts`): `SaveLimits` с видео пн 20 → `GetChildRules` возвращает `videoMin 20, videoUnlimited false`; `video.2` > 1440 → `fail("Не больше 24 ч", "video.2")`.

Мутации A: `videoUnlimited ?? false` → тест «нет ключа»; `wireLimits` без видео → литерал; `limitsEqual` без видео → `false` не ловится.

Коммит: `fix: не терять лимит видео при сохранении и копировании правил`.

### B. Колонка «Видео и соцсети» (T9-2) и копирование (FR-33)

`ChildLimits.test.tsx`: шапка `["День", "Общее", "Игры", "Видео и соцсети"]` (вместо `toBeNull` на `:64`); у Миши `"Часы видео, понедельник"` = `"1"`, `"Минуты видео, понедельник"` = `"0"`, `"Видео без лимита, вторник"` отмечен и поля дня `disabled`; в среде подсказка `"0 мин: в этот день видео и соцсети закрыты"`; под таблицей `"Время игр и видео входит в общее. Учёба в общее время не входит"`; ввод `"Минуты видео, понедельник"` = `20` → панель «Есть несохранённые изменения» → «Сохранить» → в `mock.calls` `SaveLimits` день 1 `{videoMin: 80, videoUnlimited: false}`, `"Сохранено. Новые правила уже действуют"`; отказ `mock.override("SaveLimits", {ok: false, error: "Не больше 24 ч", field: "video.1"}, 1)` → ошибка в ячейке видео понедельника; «Как в понедельник: на все будни» переносит видео; «Скопировать правила с…» → Даша (`limits` с видео пн 20, `videoUnlimited false`) → `"Минуты видео, понедельник"` = `"20"`, панель несохранённых изменений видна. `Wizard.test.tsx` шаг 3: колонки `["День", "Общее", "Игры"]`, `columnheader /Видео/` нет.

Коммит: `feat: колонка «Видео и соцсети» в лимитах ребёнка`.

### C. Шаг мастера «Сайты» (T9-3)

`Wizard.test.tsx`: помощники доводят до `"Шаг 5 из 5"`; все тесты, жавшие «Готово»/«Настрою позже» на шаге 4, идут через «Далее» на шаг 5 (переписать помощником `toSites(user)`); прогресс везде `из 5`. Новые: шаг 4 — кнопки `["Назад", "Настрою позже", "Далее"]`; «Настрою позже» на шаге 4 → `"Шаг 5 из 5"`, затем «Готово» → `Setup` без `programs`, с `sites`; шаг 5: `heading "Сайты"`, `group "Блокировать сайты 18+"` с `checkbox "Миша"` (`checked`), подпись `"Список сайтов для взрослых: UT1, Университет Тулуза Капитоль, лицензия CC BY-SA 4.0"`, `list "Видео и соцсети: сайты"` = 9 доменов `DEFAULT_VIDEO_SITES` по порядку; «Убрать tiktok.com» → 8; добавить `"https://Kids.Example/video"` → `"kids.example"` в конце, поле пусто; `"котики видео"` → у поля `"Это не похоже на адрес сайта"`; `"YouTube.com"` → `"Этот сайт уже в списке"`; снять «Миша», «Готово» → `Setup.sites` `{adultOff: ["S-1-5-21-111-222-333-1001"], video: [8 доменов без tiktok.com, "kids.example"]}`, итог `"Готово. Правила начнут действовать при следующем входе детей"`; «Настрою позже» на шаге 5 → `Setup.sites` `undefined`; `mock.override("Setup", {ok: false, error: "Это не похоже на адрес сайта", field: "domain"}, 1)` → снова `"Шаг 5 из 5"`, ошибка у поля «Добавить сайт»; «Назад» на шаг 4 и «Далее» — правки списка и переключателей на месте. Мок `Setup`: принимает `sites` (пишет `state.sites` видео и `adultFilter` детей), неразборный домен — отказ с `field "domain"`.

Мутации C: `adultOff` из включённых → литерал SID; `sites` при «Настрою позже» → `undefined`; «Убрать» по индексу при дубле — тест порядка.

Коммит: `feat: шаг мастера «Сайты»: 18+ по детям и стартовый набор видео`.

### D. «Сегодня» и окно ребёнка (T9-4, T9-5)

`Today.test.tsx` (`NOW` — пн): `mockChild({ limits: <пн: totalMin 120, gamesMin 60, gamesUnlimited false, videoMin 20, videoUnlimited false>, gamesUsedS: 2400, videoUsedS: 600 })` → `"Видео 10 мин из 20 мин"` после `"Игры 40 мин из 1 ч"` (`compareDocumentPosition`); лимиты без полей видео → `queryByText(/^Видео /)` = `null`. `mytoday.test.ts` `todayRows`: обычный статус без полей видео → `"Видео и соцсети": "Без отдельного лимита"`; `{videoUnlimited: false, videoLimitS: 1200, videoRemainingS: 590}` → `"Осталось 10 мин из 20 мин"`; `{videoUnlimited: false, videoLimitS: 0, closeVideo: true}` → `"Сегодня нельзя"`; `{videoUnlimited: false, videoLimitS: 0, closeVideo: false}` → `"Без отдельного лимита"`; `lock: "timeUp"` с лимитом → `"Осталось 0 мин из 20 мин"`; `studyOnly: true` → `"Осталось 0 мин из 20 мин"`; `status: "admin"` → `"Не действует"`. `MyStats.test.tsx:607` → `"Видео и соцсети": "Осталось 1 ч 00 мин из 1 ч"`; новый: подсказка значка (`myTodayStatus().tooltip.split("\n")[2]`) = `"Видео: 1 ч 00 мин"` и совпадает с окном; «Мои правила» (`:428`) → `["Игры: пн до 1 ч, ср нельзя, сб до 2 ч", "Видео и соцсети: пн до 1 ч, ср нельзя", "Когда время вышло: экран блокировки"]`; все 7 дней видео 30 → `"Видео и соцсети: до 30 мин в день"`.

Коммит: `feat: видео и соцсети на «Сегодня» и в окне ребёнка`.

### E. «Итоги» по сайтам (T9-6)

`MyStats.test.tsx`: вкладки `["Всего", "Игры", "Видео и соцсети", "Учёба", "Прочее"]`; `mockChild({ usedS: 2100, gamesUsedS: 600, videoUsedS: 900, studyUsedS: 300, programsToday: [{path: "youtube.com", name: "youtube.com", category: "video_social", seconds: 600, kind: "site"}, {path: "vk.com", name: "vk.com", category: "video_social", seconds: 300, kind: "site"}] })` → плитки `"Всего" 40 мин`, `"Игры" 10 мин`, `"Видео и соцсети" 15 мин`, `"Учёба" 5 мин`, `"Прочее" 10 мин`; вкладка «Видео и соцсети» → `"Видео и соцсети за сегодня: 15 мин"`, `list "Видео и соцсети: по сайтам"` → `[["youtube.com", "10 мин", "67%"], ["vk.com", "5 мин", "33%"]]`; «Всего» содержит `youtube.com` и `vk.com`; текста `"Пока не считается"` нет нигде (`queryByText`); видео без строк → `"Пока ничего не запускалось"`; «Неделя» с `history: [{day: <прошлый пн>, totalS: 1200, videoS: 600, programs: [{path: "youtube.com", …, seconds: 600, kind: "site"}]}]` → «Видео и соцсети за неделю: 25 мин» (PL-31). `ParentStats.test.tsx` — тот же список вкладок. Мок: `myToday` по T9-6, тест мока `otherS` без видео.

Мутации E: `tabRows("all")` с пропуском видео → «Всего» без сайтов; `otherS` без вычета видео → плитка «Прочее» 25 мин.

Коммит: `feat: вкладка «Видео и соцсети» в «Итогах» со строками сайтов`.

### F. Журнал и «Что нового» (T9-7, T9-8)

`journal.test.ts`: `isViolation("site_forbidden_try")` → `true`; `"site_closed_video"`, `"site_closed_study_only"`, `"site_unread"`, `"site_added"`, `"site_moved"`, `"site_removed"`, `"site_adult_filter"`, `"brave_restored"` → `false`. `Journal.test.tsx`: события пяти текстов из «Найдено» → все видны во «всех событиях», в «нарушениях» — только два `site_forbidden_try`. `Today.test.tsx`: два `site_forbidden_try` сегодня и `site_closed_video` → `"Нарушения сегодня: 2"`. `entries.test.ts`: версии `["0.4.0", "0.3.3", …]`, запись 0.4.0 литералом T9-8; старые записи проверять по `WHATS_NEW.find((e) => e.version === "0.3.3")` вместо индексов; исключения латиницы `PC Block|Windows|Brave|Tor|VPN`. `WhatsNew.test.tsx`, `App.test.tsx:285`: первой `"Версия 0.4.0. Сайты и Brave"`.

Коммит: `feat: нарушения о сайтах в журнале и «Что нового» 0.4.0`.

### G. Снимки Playwright

Правки старых сцен: `s01` `"Шаг 1 из 5"`; `s12 мастер, шаг программ` — `"Шаг 3 из 5"`, `"Шаг 4 из 5"`, кнопки `["Назад", "Настрою позже", "Далее"]`; `s12 лимиты с играми` — пояснение `"Время игр и видео входит в общее. Учёба в общее время не входит"`, колонки `["Общее", "Игры", "Видео и соцсети"]` в окне; `toHaveCount(4)` → `5` (`:755`, `:962`), имена вкладок `:977` + «Видео и соцсети»; `s07` — `"Версия 0.4.0. Сайты и Brave"`.
Новые сцены (все с `noHorizontalScroll`):
- `m4 лимиты с видео` — 960×640, видео пн 20, ср 0, вс без лимита → `limitCellsFit`, правый край каждой шапки ≤ 960, подсказка `"0 мин: в этот день видео и соцсети закрыты"` в среде → `m4-limits-video.png` (`wholePage`); то же в 1200×820 → `m4-limits-video-1200.png`.
- `m4 мастер, шаг сайтов` — 880×680 → кнопки шага в одну строку и одной высоты (как `:1827-1832`), список и поле видны → `m4-wizard-sites.png`; 720×560 → `m4-wizard-sites-720.png`; шаг 3 в 720×560 — две колонки (сцена `s12 … в узком окне` зелёная без правки).
- `m4 сегодня с видео` — `"Видео 10 мин из 20 мин"` ниже строки игр → `m4-today-video.png`.
- `m4 моя статистика: видео` — 800×640 и 640×520, вкладка «Видео и соцсети» выбрана, `tabListFits` → `m4-my-stats-video.png`, `m4-my-stats-video-small.png`; строка «Видео и соцсети» в «Сколько осталось».
- `m4 статистика родителя: видео` — 960×620 → `m4-parent-stats-video.png`.

Затем `bash tools/codemap.sh` (если менялся состав файлов `frontend/src`) и `python3 tools/scan_ui_texts.py`.

Коммит: `test: снимки видео-лимита, шага «Сайты» и «Итогов» по сайтам`.

## Среда и команды

```
cd /home/claude/pc_block_soft
python3 tools/scan_ui_texts.py
bash tools/codemap.sh
git add <пути явно>; git commit   # трейлеры ниже
```
npm, vitest, tsc, Playwright локально **не работают** (RM4-0). `frontend/tsconfig.json`: `strict`, `noUnusedLocals`, `noUnusedParameters` — после удаления `NOT_COUNTED` и правок `TotalsTab` проверь чтением все `switch` по `TotalsTab` (исчерпывающие) и импорты.

## Review Focus (самопроверка)

1. Ни один путь сохранения (лимиты, копирование, мастер) не теряет поля видео: `wireLimits` всегда шлёт оба (T9-1).
2. Review Focus плана п. 9: остаток видео одинаков в подсказке значка, «Сколько осталось», «Сегодня» (по `videoText` службы), «Мои правила», копировании и «Итогах».
3. «Итоги»: сумма строк вкладки = плитка (PL-28), «Всего» включает сайты, период — прошлые дни + сегодня (PL-31); перенос сайта между списками прошлое не двигает — показ строк по данным службы, фронт не пересчитывает.
4. Мастер: «Настрою позже» не шлёт `sites`; `adultOff` — только SID выбранных детей; отказ службы по `domain` возвращает на шаг 5.
5. Таблица лимитов в 960×640 без прокрутки вбок, мастер 720×560 — две колонки (PL-26).
6. Тексты: родителю на «вы»/безлично, ребёнку на «ты», без длинных тире; «Что нового» без латиницы, кроме названий.

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

`anthropic-skills:uiux-design` в режиме «рабочее приложение» (ТЗ 8.0). `anthropic-skills:in-app-guidance` **не** грузить (D-56). Коммиты — Conventional Commits по-русски в повелительном наклонении.

## Уроки (docs/LESSONS.md)

- **PL-28** (A): у вкладки «Видео и соцсети» строка «Остальное время» последней, сумма строк = плитка, доли не подгоняются.
- **PL-31** (D): «Неделя» по видео = прошлые дни (`history`) + сегодня; мок не берёт сегодняшний день из `history`.
- **PL-35** (A): новое состояние (лимит видео) — все подписи, где показан остаток: «Сколько осталось», «Сегодня», подсказка значка в моке, «Мои правила», пояснения таблицы; в «только учёбе» и при «время вышло» — 0, как у игр.
- **PL-29** (A): строки «Видео 10 мин из 20 мин» и «Мои правила» — от службы; мок повторяет её помощники, экран не собирает своих.
- **PL-34** (B): шаг мастера — ошибка у своего поля; сообщения не вытесняют друг друга.
- **PL-25, PL-26** (C): третья колонка мерится в самом узком окне родителя 960 px, мастер — 720 px; документ не прокручивается.
- **PL-36** (C): переключатели «18+» в мастере — рядом с подписью источника; список видео и поле добавления — одним блоком.
- **PL-27** (E): e2e по точному имени внутри группы (`tablist "Итоги по категориям"` — «Видео и соцсети»; кнопки «Убрать <домен>»).
- **PL-9** (E): мок с видео-полями — иначе тесты «Сохранить» зелёные на данных без видео (ловушка T9-1 прошла бы незамеченной).

## Open (передать дальше, не делать в T9)

- Строки «Сайты» в полной статистике (FR-12 п.5, FR-23 полная) — M5.
- Подсказка значка «Видео: N мин» — страж (T7, сделано); здесь только мок.
- `CHANGELOG.md`, версия 0.4.0, README «Отклонения от ТЗ» — Фаза 8.
- Если оркестратор выберет три колонки и в мастере (T9-2) — нужна отдельная вёрстка узкой таблицы (P1).

## Вердикт (≤ 30 строк)

```
TASK T9: done | blocked | failed
Diff: sha + строка на коммит
Evidence: scan_ui_texts, codemap, тесты vitest по файлам (имена), сцены Playwright и имена снимков, мутации (какой тест ловит), «прогон только в ci.yml (RM4-0)»
Interfaces: для T10/P1 (LimitColumn "video", поле ошибки video.N, Setup.sites из мастера, TotalsTab video_social, VIOLATION_KINDS)
Rulings: T9-1…T9-8 подтверждены или изменены, своя цена ошибки
Escalations: изменённые ожидания старых тестов (Wizard из 5 шагов, ChildLimits без «Видео», «Пока не считается», вкладки 4→5, entries латиница Tor|VPN, «Что нового» 0.3.3 первой), правки вне Files
Open: что осталось на P1 и M5
```

## Решения оркестратора по вопросам пакета

- **T9-2 утверждено:** колонка «Видео и соцсети» — только в «Дети → Лимиты» (сжатые ячейки, `max-width` 760, поля 52 px); в мастере остаются две колонки (окно 720 без прокрутки вбок); лимит видео в мастере — «без лимита» по умолчанию, настраивается на «Лимитах».
- **«Что нового»:** исключения латиницы для «Tor», «VPN» и «Brave» в тесте записи 0.4.0 утверждены.
- **Имя списка вкладки «Всего» для диктора:** переименовать в «по программам и сайтам» (честное имя), помощники тестов поправить вместе с ним.
- **T9-1 (лимит видео стирается при «Сохранить») — первым шагом, с RED-тестом; это дефект, найденный до выпуска.**
