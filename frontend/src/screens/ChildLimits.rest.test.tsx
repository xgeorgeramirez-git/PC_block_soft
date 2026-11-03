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
const toggle = () => screen.getByRole("checkbox", { name: "Перерывы на отдых" }) as HTMLInputElement;
const message = () => screen.getByLabelText("Сообщение на экране перерыва") as HTMLInputElement;
const sent = () => mock.calls.filter((c) => c.command === "SaveLimits").at(-1)?.payload;
const SELF = "Засчитать как отдых, если не играл подряд";

async function openRest(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByRole("table");
  await user.click(screen.getByRole("tab", { name: "Отдых" }));
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
    expect(tabs.map((t) => t.textContent)).toEqual(["Лимиты", "Запретные часы", "Когда время вышло", "Отдых"]);
    await user.click(screen.getByRole("tab", { name: "Отдых" }));
    expect(toggle().checked).toBe(false);
    expect([num("Через сколько").value, num("Предупредить за").value, num("Отдых").value, num(SELF).value]).toEqual(["60", "5", "15", "15"]);
    for (const l of ["Через сколько", "Предупредить за", "Отдых", SELF]) expect(num(l).disabled).toBe(true);
    expect((screen.getByRole("radio", { name: "Всё время, кроме учёбы" }) as HTMLInputElement).checked).toBe(true);
    expect(message().value).toBe("Разомнись, попей воды, посмотри в окно");
    expect(screen.getByText("Перерывы на отдых выключены")).toBeTruthy();
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
  });

  // Ловит: сценарий 8.3.3 п.7 — итог словами не совпадает с полями, образец «Как увидит ребёнок»
  // без сообщения, сохранение не отправляет «Отдых» или шлёт лимиты, которых не меняли.
  it("сценарий 8.3.3 п.7: итог словами, образец и сохранение", async () => {
    const user = setup();
    await openRest(user);
    await user.click(toggle());
    await user.click(screen.getByRole("radio", { name: "Только игры" }));
    await retype(user, num(SELF), "30");
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
    await retype(user, num(SELF), "10");
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText("Не меньше времени отдыха")).toBeTruthy();
    expect(num(SELF).getAttribute("aria-invalid")).toBe("true");
    expect(num(SELF).getAttribute("aria-describedby")).toContain(screen.getByText("Не меньше времени отдыха").id);
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
    expect(screen.getByRole("tab", { name: "Отдых" }).getAttribute("aria-selected")).toBe("true");
    expect(sent()).toBeUndefined();
  });

  // Ловит: при увеличении «Отдыха» выше порога порог остаётся ниже, и появляется ошибка без
  // действия родителя (Ruling FB14: порог поднимается вместе с отдыхом).
  it("«Отдых» выше порога поднимает порог", async () => {
    const user = setup(s43);
    await openRest(user);
    await retype(user, num("Отдых"), "45");
    expect(num(SELF).value).toBe("45");
    expect(screen.queryByText("Не меньше времени отдыха")).toBeNull();
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

  // Ловит: выключение теряет настроенные значения.
  it("выключенный переключатель сохраняет значения", async () => {
    const user = setup(s43);
    await openRest(user);
    await user.click(toggle());
    expect(num(SELF).disabled).toBe(true);
    expect(num(SELF).value).toBe("30");
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
    await user.click(screen.getByRole("tab", { name: "Отдых" }));
    await vi.waitFor(() => expect(toggle().checked).toBe(true));
    expect(num(SELF).value).toBe("30");
    expect(message().value).toBe("Пойди погуляй с собакой");
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    await screen.findByText("Сохранено. Новые правила уже действуют");
    expect(sent().break).toEqual(s43);
  });
});
