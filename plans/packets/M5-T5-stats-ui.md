# Пакет M5-T5: статистика у ребёнка и родителя, «По дням», «Очистить статистику» (+ раздел T6 журнал)

Писатель интерфейса. Скилл `uiux-design` в режиме «рабочее приложение» (ТЗ 8.0, запреты D-30: без градиентов, стекла, «пилюль», кремового фона, нумерованных меток, курсива в заголовках; анимаций нет). Помощник не трогать (D-56). Источники: план `plans/2026-11-10-M5.md` (RM5-2…RM5-4, RM5-8, RM5-10, UX-сценарий), ТЗ 8.2.6, 8.3.7, FR-12, FR-23, FR-38; уроки PL-25, PL-26, PL-28, PL-36, PL-38, PL-44, PL-49.

## 0. Что уже даёт T2b/T2c (сверить с кодом до работы, PL-4)
- `types.ts`: `DayBar {key,label,title,totalS,studyS,today}`; в `MyTodayReply`: `bars: DayBar[]`, `barsBy: "day"|"month"`, `violations: JournalEntry[]`, `bonus: BonusEntry[]`, `statsSinceDay: string`, `statsSinceText: string`.
- `api.ts`: `clearStats(childId): Promise<{message:string}>`. **Проверить `myTodayOf` (api.ts ~стр. 305–322)**: новые поля должны приводиться `?? []` / `?? "day"` / `?? ""`. Если T2b не добавил — добавить здесь (+ строка в `api.test.ts`: `null` → `[]`).
- Окно ничего не считает и не сочиняет: подписи столбиков (`label`), подсказки (`title`), «Статистика ведётся с …» (`statsSinceText`), текст сообщения очистки (`message`) — из ответа.

## 1. Порядок разделов

Ребёнок (`MyStats.tsx`, return стр. 139–156):
1. «Сколько осталось», «Нужно больше времени?» (`Remaining`, без изменений).
2. Строка `<p className="stats-since">{data.statsSinceText}</p>` — над карточкой «Итоги» (8.2.6 п.10); пустая строка — не выводить.
3. Карточка периода `Totals` (одна `section.panel.totals`, region «Итоги»): заголовок «Итоги» + признак загрузки → `PeriodTabs` → `div#period-panel role=tabpanel`, внутри по порядку: содержимое «Итогов» (как сейчас) → подразделы «По дням», «Программы», «Сайты», «Нарушения», «Копилка». Решение (PL-36): всё, что зависит от периода, — внутри одной панели периода; постоянное («Сколько осталось», «Бонусы», «Мои правила») — вне. Подраздел = `<section className="stat-sub" aria-labelledby="sub-<id>">` с `<h3 id className="section-title">`; `.stat-sub { border-top: 1px solid var(--line); margin-top: var(--space-6); padding-top: var(--space-4) }`.
4. «Бонусы» (`Bonuses`, стр. 359–384): остаются баланс «Копилка: N мин», пояснение и `PromoRedeem`; **список истории (стр. 372–381) убрать** — история теперь в подразделе «Копилка» за период (одно место). Тест «баланс копилки и автобонус простыми словами» (MyStats.test.tsx ~стр. 619) переписать: строки истории искать в region «Копилка» на периоде «Всё время».
5. «Мои правила», «Настройки для родителя» — как есть.

Родитель (`ParentStats.tsx`):
- Строка выбора (стр. 77–91) превращается в `div.stat-child-row`: слева группа «Ребёнок» (только если детей > 1, как сейчас), справа (`topbar-spacer`) кнопка `btn` «Очистить статистику» — есть всегда, когда выбран ребёнок и `data` загружено (и при одном ребёнке).
- Убрать внешний `PeriodTabs` (стр. 93) и внешнюю панель (стр. 105–109): `Totals` получает `periods={{ value: period, onChange: setPeriod }}` как у ребёнка. Пока `data === null` — `<p className="panel-text">Загружаю данные</p>` (вкладок нет, панели нет — aria-controls не висит, B-46 соблюдён). Тест «пока данные идут: вкладки… ссылаются на существующую панель» (ParentStats.test.tsx стр. 127) заменить: «пока данные идут: ни вкладок, ни висячей ссылки, только „Загружаю данные“».
- Затем `stats-since` (над карточкой), карточка, `Rules`. Снять в комментарии стр. 16 «„По дням“ … — в вехе M5».
- Сообщение результата очистки — постоянная живая область (PL-44): `<p className="child-feedback" role="status">{clearMsg}</p>` в DOM всегда (пустая строка до действия), под строкой выбора. Смена ребёнка сбрасывает сообщение.

## 2. `lib/daybars.ts` (новый, чистые функции, без DOM)
```ts
import type { DayBar, MyTodayReply } from "../types";
/** Шкала: потолок оси и деления (секунды). Шаг — наименьший из STEPS, при котором ceil(max/шаг) ≤ 4; max = наибольшее totalS+studyS; все нули — {maxS: 3600, ticks: [0, 1800, 3600]}. */
export const STEPS = [600, 900, 1800, 3600, 7200, 10800, 14400, 21600, 36000, 72000, 180000, 360000, 720000];
export function barScale(bars: DayBar[]): { maxS: number; ticks: number[] }; // ticks от 0 до maxS включительно
/** Подпись деления: 0 → «0», < 1 ч → «30 мин», целые часы → «2 ч», иначе «1 ч 30 мин». */
export function axisLabel(seconds: number): string;
/** Высоты сегментов в процентах высоты графика (0…100, до 0,1). Ненулевой сегмент — не меньше 1 %. empty = totalS + studyS === 0. */
export function barHeights(bar: DayBar, maxS: number): { totalPct: number; studyPct: number; empty: boolean };
/** Показывать подпись под столбиком: n ≤ 12 — все; иначе by "day" — i % 7 === 0, by "month" — i % 3 === 0. */
export function showLabel(index: number, count: number, by: "day" | "month"): boolean;
/** Выравнивание подсказки у краёв: первый столбик — "start", последний — "end", иначе "center" (n === 1 — "center"). */
export function tipAlign(index: number, count: number): "start" | "center" | "end";
/** Итоговая подпись графика (role="img"): «По дням за неделю: 12 ч 33 мин в счёт лимита, учёба 1 ч 35 мин» / «По месяцам за год: …». Числа — reply.usedS и reply.studyS (те же, что в «Итогах»; сумма столбиков = «Итоги», RM5-4), слова периода — periodOf(reply.period).sum. */
export function barsSummary(reply: MyTodayReply): string;
```
Тесты `lib/daybars.test.ts` (литералы G15, вс 22.11.2026): максимум 200 мин (пт 180+20, сб 200) → шаг 3600, `maxS` 14400, `ticks` [0, 3600, 7200, 10800, 14400]; пн 95 мин + 30 учёбы → `totalPct` 39.6, `studyPct` 12.5; ср → `empty: true`, 0 и 0; сегмент 30 с при max 4 ч → 1; `axisLabel` 0/1800/7200/5400 → «0», «30 мин», «2 ч», «1 ч 30 мин»; `showLabel` 7 дней все, 31 день — 0, 7, 14, 21, 28; 36 месяцев — каждый третий; `tipAlign`; `barsSummary` недели G15 — «По дням за неделю: 12 ч 33 мин в счёт лимита, учёба 1 ч 35 мин», года — «По месяцам за год: …».

## 3. `components/DayBars.tsx`
Пропсы: `{ reply: MyTodayReply }` (берёт `bars`, `barsBy`). Разметка:
```
<figure className="daybars">                                   // position: relative (PL-25: скрытая таблица внутри)
  <div className="daybars-scroll" style={{"--n": n, "--col-min": by==="day" ? "13px" : "28px"}}>  // overflow-x: auto — прокрутка только здесь
    <div className="daybars-plot">                              // display:grid; grid-template-columns: 48px 1fr; min-width: calc(48px + var(--n) * var(--col-min))
      <div className="daybars-axis" aria-hidden="true">          // подписи делений axisLabel, top по проценту
      <div className="daybars-area">                              // position: relative; height: 160px
        <svg viewBox={`0 0 ${n*10} 100`} preserveAspectRatio="none" width="100%" height="100%" role="img" aria-label={barsSummary(reply)}>
          <defs><pattern id={useId()+"study"} width="10" height="4" patternUnits="userSpaceOnUse">
            <rect className="daybar-hatch" width="10" height="2" /></pattern></defs>
          линии делений: <line className="daybars-grid" x1=0 x2={n*10} y={100 - t/maxS*100} vectorEffect="non-scaling-stroke" />
          на столбик i (x = i*10 + 2, ширина 6):
            empty → <line className="daybar-empty" x1={x} x2={x+6} y1=100 y2=100 vectorEffect="non-scaling-stroke" />
            иначе <rect className="daybar-total" y={100-totalPct} height={totalPct}/>
                  <rect className="daybar-study" y={100-totalPct-studyPct} height={studyPct} fill={`url(#…)`}/>  // учёба поверх
        </svg>
        <div className="daybars-hits" role="group" aria-label="Столбики: стрелки влево и вправо">  // поверх svg, grid repeat(var(--n),1fr)
          на столбик: <div className="daybar-hit" role="img" aria-label={bar.title} aria-current={bar.today ? "date" : undefined}
                           tabIndex={i === active ?? last ? 0 : -1} onFocus/onMouseEnter→setTip(i) onBlur/onMouseLeave→setTip(null) onKeyDown />
        </div>
        {tip !== null ? <div className={`daybars-tip daybars-tip-${tipAlign(tip,n)}`} aria-hidden="true" style={{left: `${(tip+0.5)/n*100}%`}}>{bars[tip].title}</div> : null}
      </div>
      <div /> <div className="daybars-labels" aria-hidden="true">  // grid repeat(var(--n),1fr); подпись bar.label при showLabel, у today — font-weight 600
    </div>
  </div>
  <figcaption className="daybars-legend">  // ▮ «В счёт лимита»; ▤ «Учёба, не в счёт лимита» (только если у какого-то столбика studyS > 0); образцы — маленькие svg 12×12 тех же классов
  <table className="visually-hidden"><caption>{by==="day" ? "По дням" : "По месяцам"}</caption>
    thead: «День»|«Месяц», «В счёт лимита», «Учёба»; строки: th scope=row bar.label, td formatDuration(totalS), td formatDuration(studyS)
</figure>
```
- Клавиатура (одна остановка Tab, «бегающий» tabIndex как `PeriodTabs`): ←/→ — соседний столбик с фокусом, Home/End — крайние, Esc — спрятать подсказку (WCAG 1.4.13). Первый в порядке Tab — столбик «сегодня» (последний).
- Подсказка — ровно `bar.title`; `.daybars-tip` — `position:absolute; bottom: calc(100% + 4px)` от верха области (над графиком, не перекрывает столбики), `center` — `translateX(-50%)`, `start` — `translateX(-8px)`, `end` — `translateX(calc(-100% + 8px))`; фон `--surface`, рамка `--line`, текст `--text`, 14 px, `white-space: nowrap`, `pointer-events: none`, z-index над столбиками.
- Ширина: `.my-stats` max 640 px → в 960 px области графика ~540 px; 31 × 13 + 48 = 451 — без прокрутки и в окне 800 px (~440 px). 36 месяцев × 28 + 48 = 1056 → прокрутка только `.daybars-scroll` (`overflow-x: auto; overflow-y: hidden`), документ и `.content` не прокручиваются вбок. При открытии прокрутить `.daybars-scroll` в конец (`scrollLeft = scrollWidth`) — сегодня виден.
- Новые токены `styles.css` (hex, иначе `contrast.test.ts` их не прочитает):
  `:root { --chart-bar: #59616a; }` (на белом 6,2:1), тёмная: `--chart-bar: #a3abb3;` (на #1f2327 ≈ 7,3:1).
  Классы: `.daybar-total { fill: var(--chart-bar) }`, `.daybar-hatch { fill: var(--chart-bar) }`, `.daybar-study { stroke: var(--chart-bar); stroke-width: 1; vector-effect: non-scaling-stroke }` (фон штриховки — прозрачный → `--surface`), `.daybar-empty { stroke: var(--field-border); stroke-width: 2 }`, `.daybars-grid { stroke: var(--line); stroke-width: 1 }`. Подписи — `--text-muted`, 12 px, `tabular-nums`. Штриховка горизонтальная: не искажается при `preserveAspectRatio="none"`. Цвета статуса (янтарный, красный, зелёный) на графике не использовать (ТЗ 8.0 п.3); «сегодня» — только жирной подписью.
  `contrast.test.ts`: новый тест «столбики заметны на поверхности в обеих темах (3:1)» — `chart-bar` к `surface`.
- Фокус: `.daybar-hit:focus-visible { outline: 2px solid var(--text); outline-offset: -2px }`.

Тесты `components/DayBars.test.tsx`: 7 столбиков G15 (`getAllByRole("img")` внутри group = 7, имена — литералы `title` из мока, напр. «вт, 17 ноября: 2 ч 20 мин, учёба 0 мин»); наведение и фокус показывают подсказку ровно `title`, Esc прячет; Tab попадает на столбик «сегодня», ←/Home/End двигают фокус, `tabIndex=0` ровно у одного; ср — есть `line.daybar-empty`, нет `rect`; у svg `aria-label` = `barsSummary`; скрытая таблица: 7 строк, «пн 16 | 1 ч 35 мин | 30 мин»; легенда учёбы есть только при учёбе; 36 месяцев — подписи каждого третьего.

## 4. «Программы», «Сайты» — `lib/mytoday.ts`
Добавить рядом с `tabRows` (после стр. 180), `rowTime` (стр. 143) переиспользовать:
```ts
export interface UsageRow { key: string; name: string; time: string; share: string; }
/** Таблица «Программы» (kind "program", пустой kind у строки с путём — тоже программа) или «Сайты» (kind "site") за период (8.2.6 п.5, RM5-4): строка — сумма секунд одного path из всех категорий; строки «Остальное время» (path "") не входят; доля — от суммы строк этой таблицы, каждая округлена сама (99–101 % допустимо); самые долгие сверху, при равенстве по имени, затем по path. Нули выкинуть. */
export function usageRows(reply: MyTodayReply, kind: "program" | "site"): UsageRow[];
```
Скрытые и запрещённые служба уже не отдаёт строками (PL-38) — окно не фильтрует повторно.
Разметка (в `MyStats.tsx`, функция `UsageTable({ id, title, rows, empty })`, экспорт для родителя): `<table className="usage-table" aria-labelledby="sub-<id>">`, `thead`: «Название», «Время», «Доля»; `td` имени — `LineText` (PL-26); `table-layout: fixed; width: 100%`, колонки времени 7.5rem и доли 4.5rem справа `tabular-nums`; полоса доли как `.totals-item::before` не нужна. Пусто: «Программ за этот период нет» / «Сайтов за этот период нет».
Тесты `mytoday.test.ts`: G15 «Программы» — game 4 ч 45 мин 45 %, other 4 ч 08 мин 39 %, study 1 ч 35 мин 15 %; «Сайты» — youtube.com 3 ч 40 мин 100 %; программа в двух категориях (games 600 + other 300) — одна строка 15 мин; «Остальное время» не строкой; сайт не в «Программах».

## 5. «Нарушения», «Копилка»
- «Нарушения»: `reply.violations` разметкой журнала (`ul.journal > li.journal-item`, `time` = `formatEventTime(atMs, tzOffsetMin)`, текст `e.text`); первые 20, при большем — кнопка `btn link` «Показать все (N)» (это не поле ввода; S22 не нарушает). Пусто: «Нарушений за этот период нет». Цвет — обычный текст.
- «Копилка»: строка `Сейчас в копилке: ${formatLimit(reply.status.bonusBalance)}` и история `reply.bonus`. Вынести `BonusHistory` из `Bonus.tsx` (стр. 357–372) в `components/BonusHistory.tsx` (экспорт, тот же `historyText` из `lib/bonus.ts`, «Баланс: …»), `Bonus.tsx` импортирует его. Пусто: «За этот период копилка не менялась».
- Пустой период (`periodEmpty`, стр. 99): содержимое «Итогов» + «По дням» + «Программы» + «Сайты» — одной строкой «За этот период пока нет данных» (дословно, 8.2.6 п.9); «Нарушения» и «Копилка» показываются со своими пустыми текстами. `notChild` — только «За этот период пока нет данных», подразделов нет.

## 6. «Очистить статистику» (родитель)
- `Dialog.tsx` стр. 23: сначала искать `.dialog-actions [data-autofocus]`, иначе первую кнопку (как сейчас). Новый `components/Dialog.test.tsx`: фокус на `data-autofocus`; без него — первая кнопка; Esc вызывает `onClose`.
- Окно: `title={`Удалить всю статистику ${genitiveName(name)}?`}`, тело `<p>Остаток на сегодня и копилка не изменятся. Это нельзя отменить.</p>`, действия: `btn btn-primary` «Удалить», `btn` «Отмена» с `data-autofocus`; Esc и «Отмена» закрывают без вызова. Кнопка «Удалить» не красная (8.0 п.3).
- «Удалить» → `busy`, `clearStats(childId)` один раз → `clearMsg = reply.message` («Статистика Миши очищена») → `load(childId, period)`; ошибка → `Notice tone="warn"` с `errorText(e)`, `data` прежние. Повторный клик во время запроса — кнопка `disabled`.
- Ребёнок: кнопки нет и не может быть (компонент `MyStats` её не содержит).

## 7. Мок `__mocks__/backend.ts`
- `myToday()` (стр. 594–647) дополнить: `bars` строит функция мока `mockBars(child, period, nowMs)` по RM5-2/RM5-3 из `history` + сегодня (подписи «пн 16», «ноя», «ноя 2025», подсказки как у службы — мок подражает службе), `barsBy`, `violations` (события ребёнка из `events` с видами `VIOLATION_KINDS` от полуночи начала периода, новые сверху), `bonus` (из `bonusLedger` от начала периода), `statsSinceDay/Text` (`clearedDay` ребёнка или первый день `history`, иначе сегодня). Пустой ребёнок — `[]`.
- `MockChild.clearedDay?: string`; команда `ClearStats` (родительская): `history = []`, сегодняшние счётчики статистики в ноль, `clearedDay = today`, событие `stats_cleared`, ответ `{message: "Статистика <род.> очищена"}`; нет ребёнка — отказ «Такого ребёнка нет». В `PARENT_COMMANDS` (стр. ~650).
- Данные G15: `g15History()`, `G15_NOW = new Date(2026, 10, 22, 15, 0, 0)`, `g15Child()` — по таблице `docs/04_Эталоны_учёта.md` G15 (как `s42History`), плюс одна запись нарушения и две записи копилки на неделе.

## 8. Тесты vitest
- `MyStats.test.tsx`: порядок подразделов внутри панели периода (Итоги → По дням → Программы → Сайты → Нарушения → Копилка), «Бонусы» и «Мои правила» вне панели (дополнить тест стр. 169); «Статистика ведётся с …» над region «Итоги»; S22 по разметке — нет `input` кроме промокода, нет `select`, нет кнопок «Очистить статистику» и выбора ребёнка; неделя G15: 7 столбиков, таблицы, нарушение, копилка; пустой период — одна строка «За этот период пока нет данных» и пустые тексты «Нарушений…», «копилка не менялась»; поздний ответ прошлого периода не перекрывает столбики выбранного (как тест стр. 189, проверка по `aria-label` графика).
- `ParentStats.test.tsx`: кнопка «Очистить статистику» при одном и при двух детях; окно с текстом 8.3.7 п.3 дословно, фокус на «Отмена»; «Отмена» и Esc — `ClearStats` не вызван; «Удалить» — вызван один раз с `childId`, сообщение «Статистика Миши очищена» в `role=status`, «Итоги» нули, подпись «Статистика ведётся с 22 ноября 2026»; ошибка — уведомление, данные прежние; смена ребёнка сохраняет период (стр. 144, без изменений).
- Перед коммитом — `grep -rn` прежних литералов (PL-49), `python3 tools/scan_ui_texts.py`.

## 9. e2e-снимки (`frontend/e2e/screens.spec.ts`, папка `screens/`)
Новые (имена в стиле `sNN-…`), часы `page.clock.setFixedTime(G15_NOW)`:
- `s21-parent-stats-week.png` (960×900, G15, «Неделя»), `s21-parent-stats-week-dark.png`;
- `s21-parent-stats-year.png` (960, «Год», 11 месяцев);
- `s21-parent-stats-month31.png` (960, часы 31.10.2026, 31 столбик; проверка `.daybars-scroll` scrollWidth ≤ clientWidth и `noHorizontalScroll`), то же без снимка в 800×640 и в окне ребёнка 640;
- `s21-parent-stats-all36.png` (36 месяцев: `.daybars-scroll` прокручивается, документ нет);
- `s21-parent-stats-tip.png` (фокус Tab на столбик → подсказка «вт, 17 ноября: …» видна, внутри окна);
- `s22-my-stats-week.png`, `s22-my-stats-week-dark.png` (окно ребёнка 800×640, `wholePage`);
- `s48-clear-stats-dialog.png`, `s48-clear-stats-done.png`.
Заменить: `s31-parent-stats-year.png` (стр. 951) — снимок оставить, но ожидания дополнить подразделами (или перенести в `s21-parent-stats-year.png` и снимок s31 удалить — одно на выбор, не дублировать). Существующие `s06-my-stats-week*.png` (стр. 915–920) остаются: проверить, что `wholePage` и «шапка на месте» (PL-25) проходят с более длинной страницей.
Снимки → `visual-reviewer` (главный агент картинки не читает).

## 10. T6 (кратко): журнал с фильтрами и «Очистить журнал» — `Journal.tsx`
- Шапка: заголовок «Журнал»; под ним вкладки периода (существующие стр. 75–88 + стрелки, `aria-controls`); строка фильтров `div.journal-filters`: `<label>Ребёнок <select>` («Все дети» + дети из `getToday`) и `<label>Что <select>` (группы RM5-5: «Все события», «Нарушения», «Время и копилка», «Программы и сайты», «Пароль и настройки», «Работа программы»; значения — коды групп службы из T3); справа при выбранном ребёнке — `btn` «Очистить журнал». Кнопку-переключатель «Показать только нарушения» (стр. 68–72) убрать.
- Запрос: `listEvents({ childId, fromMs: periodStartMs(...), group, limit: 200 })` при каждой смене фильтра, со счётчиком `seq` (как `MyStats`); фильтр по `isViolation` на экране убрать (стр. 60). Список с ключом `e.id`.
- Внизу при `more` — `btn` «Показать ещё» → тот же запрос с `beforeAtMs`/`beforeId` последней строки, дописать в конец; ошибка — уведомление и «Повторить», уже загруженное не теряется.
- Пусто — «За этот период таких событий нет» (одна строка вместо «Пока событий нет»/«Нарушений пока нет»: поиск литералов по тестам, PL-49).
- Ссылка «Нарушения сегодня» (`Today.tsx` стр. 185 → `App.tsx` `go("journal", {violations:true})`): `Journal` с `violationsOnly` стартует с «Что: Нарушения» и вкладкой «Сегодня» (сейчас по умолчанию «Всё», стр. 38).
- «Очистить журнал»: `Dialog` «Удалить весь журнал <род.>?» / «Это нельзя отменить.», «Удалить» и «Отмена» (`data-autofocus`) → `clearJournal(childId)` → `role=status` (постоянная область) с `message` → перезагрузка; при «Все дети» кнопки нет.
- Тесты `Journal.test.tsx`: фильтры уходят в запрос (`ListEvents` получает `childId`, `group`, `fromMs`); «Показать ещё» передаёт курсор и дописывает; ссылка о нарушениях — группа и «Сегодня»; очистка — подтверждение, один вызов, одна строка «Журнал очищен»; переписать тесты стр. 59, 72, 101, 155. Мок: `ListEvents` с `group`/курсором/`more`, `ClearJournal`. Снимки: `s04-journal.png` (заменить, с фильтрами), `s48-clear-journal-dialog.png`.
