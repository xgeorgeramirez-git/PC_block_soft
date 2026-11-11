# Пакет M5-T6b: раздел «Промокоды» (FR-35, R26, 8.3.10, RM5-11…RM5-14)

Писатель интерфейса. Скилл `uiux-design`, режим «рабочее приложение», запреты D-30. Окно не придумывает ни кодов, ни текстов ошибок: коды, сообщения «Готово: …», «История промокодов очищена», ошибки полей — из ответов службы (`CallError.field`, `errorText`). Подсказками не покрывать (D-56).

## 0. Что даёт T3b (сверить с кодом, PL-4)
`api.ts`: `generatePromos({count, length, charset, minutes}): Promise<{promos: PromoView[]; message: string}>`, `clearPromoHistory(): Promise<{message: string}>`; `listPromos()` — `codeDisplay` заглавными, «Использованные» новые сверху; `createPromo` отказ «Только латинские буквы A–Z и цифры» у поля `code`. Поля ошибок: `code`, `minutes`, `count`, `length` (`FieldCount`, `FieldLength`). `charset`: `"digits" | "letters" | "mixed"`.

## 1. Место в меню — `frontend/src/App.tsx`
- Стр. 193: `type Screen = … | "bonus" | "promos" | "stats" | …`.
- Стр. 196–205 `MENU`: после `{ id: "bonus", label: "Копилка" }` — `{ id: "promos", label: "Промокоды" }`.
- Стр. 299: `{screen === "bonus" ? <Bonus onOpenPromos={() => go("promos")} /> : null}` и рядом `{screen === "promos" ? <Promos /> : null}`; импорт `Promos from "./screens/Promos"`.
- `App.test.tsx` стр. 230: название теста и список пунктов меню — добавить «Промокоды» после «Копилка».

## 2. `frontend/src/screens/Bonus.tsx`
- Стр. 6 убрать импорт `PromoAdmin`; стр. 31–34 комментарий: вкладка «Бонусы» — автобонусы и ссылка на «Промокоды».
- `export default function Bonus({ onOpenPromos }: { onOpenPromos?: () => void })`.
- Стр. 226 `<PromoAdmin />` заменить:
```tsx
<section className="panel promo-link" aria-label="Промокоды">
  <p className="panel-text">Промокоды теперь в отдельном разделе «Промокоды»</p>
  <button type="button" className="btn" onClick={onOpenPromos}>Открыть</button>
</section>
```
  Текст RM5-13 «Промокоды — в разделе «Промокоды»» содержит длинное тире, скан `tools/scan_ui_texts.py` его не пропустит, поэтому текст другой (Ruling читателя; оркестратору подтвердить). Строка и кнопка в одну линию (`display:flex; align-items:center; gap: var(--space-3); flex-wrap: wrap`).
- Вкладки «Копилка/Бонусы» (стр. 144–157) — по пути добавить стрелки (как в `Promos`), `id`/`aria-controls` (необязательно; если трогать — тесты Bonus).
- `Bonus.test.tsx`: тесты стр. 129 («создать промокод»), 146 («повтор кода»), 157 («Использованные») перенести в `Promos.test.tsx`; новый тест «на вкладке „Бонусы“ нет поля „Код“, есть „Открыть“, клик вызывает `onOpenPromos`».
- `PromoAdmin.tsx` удалить (`git rm`); `grep -rn PromoAdmin frontend` пусто.

## 3. `frontend/src/screens/Promos.tsx` (новый)
Состояние: `active: PromoView[] | null`, `used: PromoView[]`, `loadError`, `tab: "active" | "used"`, форма «Создать код» (`code`, `minutes`=30, `codeError`, `minutesError`), форма «Придумать коды» (`count`=10, `length`=6, `charset`="mixed", `genMinutes`=30, ошибки по полям), `generated: PromoView[]`, `message` (постоянная живая область, PL-44), `copyError`, `confirm: boolean`, `busy`. `load()` и `alive` — как в `PromoAdmin` (стр. 25–43).

Разметка (порядок):
```
<section aria-labelledby="promos-title">
  .page-head h2#promos-title «Промокоды»
  p.panel-text «Ребёнок вводит код на экране блокировки или в разделе «Бонусы», и минуты сразу добавляются к сегодняшнему дню. Каждый код действует один раз.»
  p.child-feedback role="status"  {message}            // в DOM всегда
  section.panel aria-labelledby «Создать код»          // h3.section-title
     .promo-form: поле «Код» (TextField-разметка: input autoComplete="off" spellCheck={false} autoCapitalize="characters" maxLength={16},
                  onChange — value как есть, без фильтра знаков: проверяет служба (RM5-11); aria-invalid, aria-describedby на ошибку),
                  поле «Минуты» (inputMode numeric, clampDigits(…, 600)), btn-primary «Создать»
     ошибки — у своего поля: p.field-error id=… под полем (не общий абзац, как в PromoAdmin стр. 116)
  section.panel aria-labelledby «Придумать коды»
     .promo-form: «Сколько» (clampDigits 50), «Длина» (clampDigits 12),
                  fieldset.promo-charset <legend>Из чего</legend> radio: «Только цифры» digits, «Только буквы» letters, «Буквы и цифры» mixed (по умолчанию),
                  «Минуты» (clampDigits 600), btn-primary «Придумать»
     generated.length > 0 → div.promo-generated: ul.promo-codes (aria-label «Новые коды») li > code.promo-code — по одному коду в строке, моноширинно;
                  btn «Скопировать все»; copyError → p.field-error «Не удалось скопировать, выделите коды вручную»
  div.tabs role="tablist" aria-label="Промокоды"
     tab#promos-tab-active «Действующие (N)», tab#promos-tab-used «Использованные (N)» — aria-selected, aria-controls, бегающий tabIndex, ←/→/Home/End (как PeriodTabs стр. 21–33)
  div role="tabpanel" id="promos-panel" aria-labelledby={выбранная}
     active: пусто «Действующих кодов нет»; иначе ul.promo-list aria-label «Действующие промокоды»
             li.promo-item: code.promo-code {codeDisplay} · «30 мин» · span.promo-when «создан 11 ноября в 18:00»
     used:   при used.length > 0 сверху справа btn «Очистить историю»; пусто «Использованных кодов нет» (кнопки нет)
             li.promo-item.promo-item-used: код · «20 мин» · «использован 11 ноября в 18:40, Миша»  (имя — usedByName; нет имени — без «, …»)
  confirm → Dialog title «Удалить все использованные коды?», тело «Действующие останутся.», «Удалить» (btn-primary), «Отмена» (data-autofocus, см. пакет T5 п.6)
</section>
```
- Время строк: «11 ноября в 18:40» — **новая** функция `lib/format.ts`: `export function formatDayAt(ms: number, tzOffsetMin: number): string` → `${день} ${MONTH_GENITIVE[м]} в ${чч}:${мм}` (через `shifted`, как `formatEventTime` стр. 96). `formatEventTime` даёт «11 ноя, 18:40» — не по 8.3.10. Смещение — `localOffsetMin(new Date(ms))`, как `PromoAdmin` стр. 74. Тест в `format.test.ts`.
- «Создать»: пустой код — «Введите код» у поля без вызова; минуты < 1 — «Можно от 1 до 600 минут» у поля. Ответ → `message = r.message`, код очищен, `load()`, вкладка «Действующие». Ошибка: `field === "minutes"` → у «Минуты», иначе у «Код»; фокус на поле с ошибкой (PL-36).
- «Придумать»: шлёт ровно выбранные `count/length/charset/minutes`; ошибка с `field` `count`/`length`/`minutes` → у своего поля, фокус на него; без поля → `Notice tone="warn"` над формой. Успех → `generated = r.promos`, `message = r.message` («Готово: 10 кодов на 30 мин»), `load()`.
- «Скопировать все»: `navigator.clipboard.writeText(generated.map(p => p.codeDisplay).join("\n"))`; успех — `message` «Коды скопированы»; отказ или нет `navigator.clipboard` — `copyError`.
- «Очистить историю» → окно → `clearPromoHistory()` → `message = r.message` («История промокодов очищена») → `load()`; ошибка — `Notice`, списки прежние.
- Enter в полях «Создать код» нажимает «Создать», в полях «Придумать коды» — «Придумать» (`<form onSubmit>` у каждой формы, `preventDefault`).

## 4. `styles.css`
- Новый токен `--font-mono: "Cascadia Mono", Consolas, "Courier New", monospace;` в `:root` (не hex — `contrast.test.ts` не затрагивает).
- `.promo-code { font-family: var(--font-mono); font-weight: 600; letter-spacing: 0.04em; }` (правка стр. ~2522).
- `.promo-codes { columns: 4 8rem; list-style: none; padding: var(--space-3); border: 1px solid var(--line); border-radius: var(--radius-control); background: var(--bg); user-select: text; }` — коды выделяются мышью.
- `.promo-charset { border: 0; padding: 0; margin: 0; display: flex; gap: var(--space-3); flex-wrap: wrap }`, `legend` как `.field-label`.
- Удалить `.promo-admin`, `.promo-lists`, `.promo-list-title` (стр. 2467, 2486, 2493), если больше нигде не используются (`grep`).
- Кнопки — скругление 6 px, «Удалить» не красная (ТЗ 8.0 п.3).

## 5. Мок `__mocks__/backend.ts`
- `GeneratePromos`: пределы как у службы (`count` 1–50, `length` 4–12, `minutes` 1–600 → отказ у поля с текстами RM5-12), коды из алфавита детерминированно (счётчик, а не `Math.random` — снимки стабильны), заглавные, не совпадают с имеющимися; событие `promo_generate`; ответ `{promos, message: "Готово: N кодов на M мин"}` (склонение — копия правила `codesText`).
- `ClearPromoHistory`: удаляет `usedAtMs`, `{message: "История промокодов очищена"}`.
- `CreatePromo`: проверка `/^[A-Za-z0-9]{3,16}$/` после `trim` → иначе отказ «Только латинские буквы A–Z и цифры» у `code`; хранит `toUpperCase()`. `ListPromos`: `codeDisplay.toUpperCase()`, «Использованные» по `usedAtMs` новые сверху. Обе новые команды — в `PARENT_COMMANDS`.
- Существующие тесты с кодами строчными (`"a1234"`, `"pizza"`) — PL-49: `grep -rn '"a1234"\|pizza' frontend/src frontend/e2e`, ожидания заглавными («A1234», «PIZZA»); ввод ребёнком (`PromoRedeem`) — без изменений.

## 6. Тесты `frontend/src/screens/Promos.test.tsx`
1. Вкладки: «Действующие (2)», «Использованные (1)»; стрелки и Home/End; пустые вкладки — «Действующих кодов нет» / «Использованных кодов нет», на пустой «Использованных» нет «Очистить историю».
2. Строки: «A1234», «30 мин», «создан 19 октября в 11:00»; использованный — «использован 19 октября в 11:40, Миша».
3. «Создать»: «ab12» → в «Действующих» «AB12» и сообщение ответа; «код!» → у поля «Код» «Только латинские буквы A–Z и цифры» (текст службы/мока), `aria-invalid`, фокус на поле, списки без изменений; повтор → «Такой код уже есть, придумайте другой».
4. «Придумать коды»: по умолчанию 10 / 6 / «Буквы и цифры» / 30; выбор 5, 4, «Только цифры», 15 → в `GeneratePromos` ушло `{count:5,length:4,charset:"digits",minutes:15}`; показаны 5 кодов ответа и «Готово: 5 кодов на 15 мин»; `count` 0 → ошибка у «Сколько», фокус.
5. «Скопировать все» → `writeText` с кодами через `\n`; отказ буфера → «Не удалось скопировать, выделите коды вручную».
6. «Очистить историю» → окно, фокус на «Отмена»; «Отмена» — без вызова; «Удалить» — `ClearPromoHistory` один раз, «История промокодов очищена», «Использованных кодов нет», действующие на месте.
7. Тексты на «вы», без длинных тире (как тест MyStats стр. 593, но регулярка «ты» для родителя).
`Bonus.test.tsx` — см. п.2. `App.test.tsx` — меню.

## 7. e2e-снимки (`frontend/e2e/screens.spec.ts`)
- Тест «s09 бонусы и промокоды» (стр. 361–384) переписать в «s09 бонусы: ссылка на промокоды»: вкладка «Бонусы» — автобонусы и кнопка «Открыть» → раздел «Промокоды» открыт; снимок `s09-bonuses-promos.png` заменить снимком `s09-bonuses-link.png` (старый файл больше не создаётся).
- Новые: `s49-promos-active.png` (960, 20 действующих — «Действующие (20)»), `s49-promos-used.png`, `s49-promos-generated.png` (после «Придумать»: 10 кодов моноширинно и «Скопировать все»), `s49-promos-code-error.png` (ошибка у поля «Код»), `s49-promos-clear-dialog.png`, `s49-promos-active-dark.png`, `s49-promos-generated-dark.png`; в окне 800×640 — `noHorizontalScroll` и прокрутка только `main.content` (снимок не нужен).
- Каждый тест — `noHorizontalScroll(page)`; снимки → `visual-reviewer`.
