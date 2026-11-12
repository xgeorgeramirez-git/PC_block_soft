import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ChildLimits from "./ChildLimits";
import { UnsavedGuardProvider } from "../guard";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";
import type { BreakRules, TodayCard } from "../types";

// Вкладка «Отдых» (FR-37, 8.3.3 п.7, FR-33): ожидания — литералы из ТЗ.

let mock: MockBackend;

const s43: BreakRules = { enabled: true, afterMin: 60, counts: "games", warnMin: 5, restMin: 15, selfRestMin: 30, message: "Пойди погуляй с собакой" };

function setup(brk?: BreakRules, others: TodayCard[] = [], extra = [] as ReturnType<typeof mockChild>[]) {
  mock = installBackend({ sessionActive: true, children: [mockChild(brk ? { break: brk } : {}), ...extra] });
  const user = userEvent.setup();
  render(
    <UnsavedGuardProvider>
      <ChildLimits childId={1} others={others} />
    </UnsavedGuardProvider>,
  );
  return user;
}

const num = (label: string) => screen.getByLabelText(label, { selector: "input" }) as HTMLInputElement;
const toggle = () => screen.getByRole("checkbox", { name: "Перерыв на отдых" }) as HTMLInputElement;
const message = () => screen.getByLabelText("Сообщение на экране перерыва") as HTMLInputElement;
const sent = () => mock.calls.filter((c) => c.command === "SaveLimits").at(-1)?.payload;
/** Поле «Засчитать как отдых…»: подпись зависит от «Что считать» (B-45), ищем по общему началу. */
const self = () => screen.getByLabelText(/^Засчитать как отдых, если не /, { selector: "input" }) as HTMLInputElement;
const SELF_GAMES = "Засчитать как отдых, если не играл подряд";
const SELF_ALL = "Засчитать как отдых, если не был за компьютером подряд";

async function openRest(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByRole("table");
  await user.click(screen.getByRole("tab", { name: "Перерыв на отдых" }));
}

async function retype(user: ReturnType<typeof userEvent.setup>, el: HTMLInputElement, text: string) {
  await user.clear(el);
  if (text) await user.type(el, text);
}

beforeEach(() => uninstallBackend());
afterEach(() => uninstallBackend());

describe("ChildLimits: вкладка «Отдых»", () => {
  // Ловит: вкладки нет или не после «Когда время вышло»; значения по умолчанию не из ТЗ; выключенный
  // переключатель оставляет поля доступными.
  it("по умолчанию выключено, поля недоступны и держат значения ТЗ", async () => {
    const user = setup();
    await screen.findByRole("table");
    const tabs = within(screen.getByRole("tablist", { name: "Правила ребёнка" })).getAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual(["Лимиты", "Запретные часы", "Когда время вышло", "Перерыв на отдых"]);
    await user.click(screen.getByRole("tab", { name: "Перерыв на отдых" }));
    expect(toggle().checked).toBe(false);
    expect([num("Через сколько").value, num("Предупредить за").value, num("Отдых").value, self().value]).toEqual(["60", "5", "15", "15"]);
    for (const l of ["Через сколько", "Предупредить за", "Отдых"]) expect(num(l).disabled).toBe(true);
    expect(self().disabled).toBe(true);
    expect((screen.getByRole("radio", { name: "Всё время, кроме учёбы" }) as HTMLInputElement).checked).toBe(true);
    expect(message().value).toBe("Разомнись, попей воды, посмотри в окно");
    expect(screen.getByText("Перерыв на отдых выключен")).toBeTruthy();
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
  });

  // Ловит: сценарий 8.3.3 п.7 — итог словами не совпадает с полями, образец «Как увидит ребёнок»
  // без сообщения, сохранение не отправляет «Отдых» или шлёт лимиты, которых не меняли.
  it("сценарий 8.3.3 п.7: итог словами, образец и сохранение", async () => {
    const user = setup();
    await openRest(user);
    await user.click(toggle());
    await user.click(screen.getByRole("radio", { name: "Только игры" }));
    await retype(user, self(), "30");
    await retype(user, message(), "Пойди погуляй с собакой");
    expect(
      screen.getByText(
        "После 60 мин игр ребёнок увидит «Пора отдохнуть», через 5 мин компьютер закроется на 15 мин. Если сам не играл 30 мин подряд, это засчитается как отдых",
      ),
    ).toBeTruthy();
    const sample = screen.getByRole("figure", { name: "Как увидит ребёнок" });
    expect(within(sample).getByText("Перерыв на отдых")).toBeTruthy();
    expect(within(sample).getByText("Пойди погуляй с собакой")).toBeTruthy();
    expect(within(sample).getByLabelText("Таймер 00:15:00")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    await screen.findByText("Сохранено. Новые правила уже действуют");
    expect(sent().break).toEqual(s43);
    expect(sent().limits).toEqual([]);
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
  });

  // Ловит: порог «сам отошёл» меньше отдыха уходит в службу; ошибка не у поля.
  it("«Засчитать как отдых» меньше «Отдыха»: ошибка у поля, не сохраняется", async () => {
    const user = setup(s43);
    await openRest(user);
    await retype(user, self(), "10");
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText("Не меньше 15 мин: столько длится отдых")).toBeTruthy();
    expect(self().getAttribute("aria-invalid")).toBe("true");
    expect(self().getAttribute("aria-describedby")).toContain(screen.getByText("Не меньше 15 мин: столько длится отдых").id);
    expect(document.activeElement).toBe(self());
    expect(sent()).toBeUndefined();
  });

  // Ловит: значение ниже предела сохраняется; ошибка не у поля; ошибка прячет вкладку.
  it("ниже предела: ошибка у поля, вкладка остаётся открытой", async () => {
    const user = setup(s43);
    await openRest(user);
    await retype(user, num("Через сколько"), "10");
    await user.click(screen.getByRole("tab", { name: "Лимиты" }));
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText("Можно от 15 до 240 мин")).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Перерыв на отдых" }).getAttribute("aria-selected")).toBe("true");
    // Фокус на первом неверном поле: родитель с клавиатурой сразу исправляет его.
    expect(document.activeElement).toBe(num("Через сколько"));
    expect(sent()).toBeUndefined();
  });

  // Ловит: при двух неверных полях фокус не на первом по порядку; после ошибки сохранения фокус
  // остаётся на кнопке «Сохранить».
  it("фокус на первое неверное поле по порядку", async () => {
    const user = setup(s43);
    await openRest(user);
    await retype(user, num("Отдых"), "4");
    await retype(user, self(), "3");
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText("Можно от 5 до 60 мин")).toBeTruthy();
    expect(document.activeElement).toBe(num("Отдых"));
  });

  // Ловит: ошибка службы у поля «Отдыха» открывает вкладку, но фокус не на поле с ошибкой.
  it("ошибка службы у поля: вкладка «Отдых» и фокус на поле", async () => {
    const user = setup(s43);
    await openRest(user);
    await retype(user, num("Предупредить за"), "7");
    await user.click(screen.getByRole("tab", { name: "Лимиты" }));
    mock.override("SaveLimits", { ok: false, error: "Можно от 1 до 15 мин", field: "break.warnMin" }, 1);
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText("Можно от 1 до 15 мин")).toBeTruthy();
    expect(document.activeElement).toBe(num("Предупредить за"));
  });

  // Ловит: итог словами - живая область: читалка экрана повторяет весь абзац на каждое нажатие
  // цифры; браузер предлагает в поле сообщения прошлые вводы (нет name и autoComplete="off").
  it("итог без живой области, поле сообщения без автозаполнения", async () => {
    const user = setup(s43);
    await openRest(user);
    const summary = screen.getByText(/^После 60 мин игр ребёнок увидит/);
    expect(summary.closest("[aria-live]")).toBeNull();
    expect(message().getAttribute("autocomplete")).toBe("off");
    expect(message().getAttribute("name")).toBe("breakMessage");
  });

  // Ловит: при увеличении «Отдыха» выше порога порог остаётся ниже, и появляется ошибка без
  // действия родителя (Ruling FB14: порог поднимается вместе с отдыхом).
  it("«Отдых» выше порога поднимает порог", async () => {
    const user = setup(s43);
    await openRest(user);
    await retype(user, num("Отдых"), "45");
    expect(self().value).toBe("45");
    expect(screen.queryByText("Не меньше 15 мин: столько длится отдых")).toBeNull();
  });

  // Ловит: поля принимают буквы и больше верхнего предела.
  it("только цифры и верхний предел", async () => {
    const user = setup(s43);
    await openRest(user);
    await retype(user, num("Предупредить за"), "4б0");
    expect(num("Предупредить за").value).toBe("15");
    await retype(user, num("Через сколько"), "999");
    expect(num("Через сколько").value).toBe("240");
  });

  // Ловит (B-45): подпись и подсказка поля «Засчитать как отдых…» одни для обоих режимов, хотя в
  // режиме «Всё время, кроме учёбы» отдыхом считается и время без компьютера, и учёба (R27, PL-35);
  // смена режима сбрасывает значение поля.
  it("подпись и подсказка поля отдыха по режиму «Что считать»", async () => {
    const user = setup();
    await openRest(user);
    await user.click(toggle());
    const field = num(SELF_ALL);
    expect(field.value).toBe("15");
    const hintOf = (el: HTMLInputElement) => document.getElementById(el.getAttribute("aria-describedby") ?? "")?.textContent;
    expect(hintOf(field)).toBe("Если ребёнок столько минут подряд не был за компьютером или занимался только учёбой, перерыв уже не нужен");
    expect(screen.queryByLabelText(SELF_GAMES)).toBeNull();

    await retype(user, field, "20");
    await user.click(screen.getByRole("radio", { name: "Только игры" }));
    const games = num(SELF_GAMES);
    expect(games.value).toBe("20");
    expect(hintOf(games)).toBe("Если ребёнок сам не играл столько минут подряд, перерыв уже не нужен");
    expect(screen.queryByLabelText(SELF_ALL)).toBeNull();

    await user.click(screen.getByRole("radio", { name: "Всё время, кроме учёбы" }));
    expect(num(SELF_ALL).value).toBe("20");
  });

  // Ловит: выключение теряет настроенные значения.
  it("выключенный переключатель сохраняет значения", async () => {
    const user = setup(s43);
    await openRest(user);
    await user.click(toggle());
    expect(self().disabled).toBe(true);
    expect(self().value).toBe("30");
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    await screen.findByText("Сохранено. Новые правила уже действуют");
    expect(sent().break).toEqual({ ...s43, enabled: false });
  });

  // Ловит: пустое сообщение уходит пустым полем образца или стандартным текстом в службу вместо «».
  it("пустое сообщение — стандартный текст", async () => {
    const user = setup(s43);
    await openRest(user);
    await user.clear(message());
    const sample = screen.getByRole("figure", { name: "Как увидит ребёнок" });
    expect(within(sample).getByText("Разомнись, попей воды, посмотри в окно")).toBeTruthy();
    expect(message().maxLength).toBe(120);
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    await screen.findByText("Сохранено. Новые правила уже действуют");
    expect(sent().break.message).toBe("");
  });

  // Ловит: «Скопировать правила с…» не переносит «Отдых» (FR-33).
  it("копирование правил переносит «Отдых»", async () => {
    const dasha = mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", break: s43 });
    const user = setup(undefined, [{ childId: 2, name: "Даша" } as TodayCard], [dasha]);
    await screen.findByRole("table");
    await user.click(screen.getByRole("button", { name: "Скопировать правила с…" }));
    await user.click(screen.getByRole("button", { name: "Даша" }));
    await user.click(screen.getByRole("tab", { name: "Перерыв на отдых" }));
    await vi.waitFor(() => expect(toggle().checked).toBe(true));
    expect(self().value).toBe("30");
    expect(message().value).toBe("Пойди погуляй с собакой");
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    await screen.findByText("Сохранено. Новые правила уже действуют");
    expect(sent().break).toEqual(s43);
  });
});
