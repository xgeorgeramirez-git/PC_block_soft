import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import DayBars from "./DayBars";
import { parentStats } from "../api";
import { G15_NOW, g15Child, installBackend, uninstallBackend } from "../__mocks__/backend";
import type { DayBar, MyTodayReply } from "../types";

// «По дням» (8.2.6 п.4, D-69, D-78, S21): столбики недели G15 из ответа подменной программы (как служба).

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: G15_NOW });
  installBackend({ sessionActive: true, children: [g15Child()] });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

async function week(): Promise<MyTodayReply> {
  return parentStats(1, "week");
}

function hits(): HTMLElement[] {
  return within(screen.getByRole("group", { name: "Столбики: стрелки влево и вправо" })).getAllByRole("img");
}

const tip = () => document.querySelector(".daybars-tip");

describe("DayBars: неделя G15", () => {
  // Ловит (RM5-3): подсказки столбиков собраны окном, а не взяты у службы; столбиков не 7.
  it("7 столбиков с подсказками службы", async () => {
    render(<DayBars reply={await week()} />);
    expect(hits().map((h) => h.getAttribute("aria-label"))).toEqual([
      "пн, 16 ноября: 1 ч 35 мин, учёба 30 мин",
      "вт, 17 ноября: 2 ч 20 мин, учёба 0 мин",
      "ср, 18 ноября: 0 мин, учёба 0 мин",
      "чт, 19 ноября: 2 ч 08 мин, учёба 45 мин",
      "пт, 20 ноября: 3 ч 00 мин, учёба 20 мин",
      "сб, 21 ноября: 3 ч 20 мин, учёба 0 мин",
      "вс, 22 ноября: 10 мин, учёба 0 мин",
    ]);
    expect(hits()[6].getAttribute("aria-current")).toBe("date");
    expect(hits().filter((h) => h.getAttribute("aria-current"))).toHaveLength(1);
  });

  it("итог графика для диктора - те же числа, что в «Итогах»", async () => {
    const { container } = render(<DayBars reply={await week()} />);
    expect(container.querySelector("svg.daybars-svg")?.getAttribute("aria-label")).toBe("По дням за неделю: 12 ч 33 мин в счёт лимита, учёба 1 ч 35 мин");
    expect(container.querySelector("svg.daybars-svg")?.getAttribute("role")).toBe("img");
  });

  // Ловит (D-72, D-78): на графике линия лимита или отметка «сверх лимита».
  it("только время: столбики и учёба, пустая среда - отметка без столбика", async () => {
    const { container } = render(<DayBars reply={await week()} />);
    const svg = container.querySelector("svg.daybars-svg")!;
    expect(svg.querySelectorAll("rect.daybar-total")).toHaveLength(6);
    // Учёба: пн, чт, пт.
    expect(svg.querySelectorAll("rect.daybar-study")).toHaveLength(3);
    expect(svg.querySelectorAll("line.daybar-empty")).toHaveLength(1);
    expect(svg.querySelectorAll("line.daybars-grid")).toHaveLength(5);
    expect(container.textContent).not.toMatch(/лимит дня|сверх/i);
  });

  it("наведение показывает подсказку ровно как у службы, уход прячет", async () => {
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<DayBars reply={await week()} />);
    expect(tip()).toBeNull();
    await user.hover(hits()[1]);
    expect(tip()?.textContent).toBe("вт, 17 ноября: 2 ч 20 мин, учёба 0 мин");
    await user.unhover(hits()[1]);
    expect(tip()).toBeNull();
  });

  // Ловит (WCAG 2.1.1, 1.4.13): подсказка только мышью; Tab попадает на каждый столбик; Esc не прячет.
  it("клавиатура: Tab на «сегодня», стрелки, Home и End, Esc прячет подсказку", async () => {
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(
      <>
        <button type="button">До графика</button>
        <DayBars reply={await week()} />
      </>,
    );
    screen.getByRole("button", { name: "До графика" }).focus();
    await user.tab();
    expect(document.activeElement).toBe(hits()[6]);
    expect(tip()?.textContent).toBe("вс, 22 ноября: 10 мин, учёба 0 мин");
    expect(hits().filter((h) => h.tabIndex === 0)).toHaveLength(1);
    await user.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(hits()[5]);
    expect(tip()?.textContent).toBe("сб, 21 ноября: 3 ч 20 мин, учёба 0 мин");
    await user.keyboard("{Home}");
    expect(document.activeElement).toBe(hits()[0]);
    await user.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(hits()[0]);
    await user.keyboard("{End}");
    expect(document.activeElement).toBe(hits()[6]);
    await user.keyboard("{Home}{ArrowRight}");
    expect(document.activeElement).toBe(hits()[1]);
    expect(hits().map((h) => h.tabIndex)).toEqual([-1, 0, -1, -1, -1, -1, -1]);
    await user.keyboard("{Escape}");
    expect(tip()).toBeNull();
    expect(document.activeElement).toBe(hits()[1]);
  });

  // Ловит (WCAG 1.4.13): подсказку, открытую наведением мыши, Esc не прячет - фокус не на столбике.
  it("Esc прячет подсказку, открытую наведением мыши", async () => {
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<DayBars reply={await week()} />);
    await user.hover(hits()[2]);
    expect(tip()).not.toBeNull();
    expect(document.activeElement).toBe(document.body);
    await user.keyboard("{Escape}");
    expect(tip()).toBeNull();
    // Подсказки нет - Esc больше не слушается: обработчик снят.
    await user.unhover(hits()[2]);
    await user.hover(hits()[3]);
    expect(tip()?.textContent).toBe(hits()[3].getAttribute("aria-label"));
  });

  it("подсказка у краёв выровнена внутрь графика", async () => {
    render(<DayBars reply={await week()} />);
    fireEvent.mouseEnter(hits()[0]);
    expect(tip()?.className).toContain("daybars-tip-start");
    fireEvent.mouseEnter(hits()[6]);
    expect(tip()?.className).toContain("daybars-tip-end");
    fireEvent.mouseEnter(hits()[3]);
    expect(tip()?.className).toContain("daybars-tip-center");
  });

  // Ловит: диктор не может прочитать числа графика (только картинка).
  it("скрытая таблица тех же чисел: 7 строк, пн 16 - 1 ч 35 мин и 30 мин", async () => {
    render(<DayBars reply={await week()} />);
    const table = screen.getByRole("table", { name: "По дням" });
    expect(within(table).getAllByRole("columnheader").map((c) => c.textContent)).toEqual(["День", "В счёт лимита", "Учёба"]);
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(7);
    expect(Array.from(rows[0].children).map((c) => c.textContent)).toEqual(["пн 16", "1 ч 35 мин", "30 мин"]);
    expect(Array.from(rows[2].children).map((c) => c.textContent)).toEqual(["ср 18", "0 мин", "0 мин"]);
  });

  it("подписи под столбиками - все 7, сегодня выделено", async () => {
    const { container } = render(<DayBars reply={await week()} />);
    const labels = Array.from(container.querySelectorAll(".daybars-label"));
    expect(labels.map((l) => l.textContent)).toEqual(["пн 16", "вт 17", "ср 18", "чт 19", "пт 20", "сб 21", "вс 22"]);
    expect(container.querySelectorAll(".daybars-label-today")).toHaveLength(1);
    expect(container.querySelector(".daybars-label-today")?.textContent).toBe("вс 22");
  });

  // Ловит (D-7): учёба отличается только цветом и не подписана.
  it("легенда: «В счёт лимита» и учёба, если учёба была", async () => {
    const { container } = render(<DayBars reply={await week()} />);
    const legend = container.querySelector("figcaption.daybars-legend")!;
    expect(legend.textContent).toBe("В счёт лимитаУчёба, не в счёт лимита");
  });
});

describe("DayBars: без учёбы и длинные периоды", () => {
  const bars = (n: number, label: (i: number) => string): DayBar[] =>
    Array.from({ length: n }, (_, i) => ({
      key: `k${i}`,
      label: label(i),
      title: `t${i}`,
      totalS: (i % 5) * 600,
      studyS: 0,
      today: i === n - 1,
    }));
  const replyOf = async (b: DayBar[], by: "day" | "month"): Promise<MyTodayReply> => ({ ...(await week()), bars: b, barsBy: by, period: by === "day" ? "month" : "all" });

  it("учёбы нет - в легенде её нет, штриховки нет", async () => {
    const { container } = render(<DayBars reply={await replyOf(bars(7, (i) => `d${i}`), "day")} />);
    expect(container.querySelector("figcaption.daybars-legend")?.textContent).toBe("В счёт лимита");
    expect(container.querySelectorAll("rect.daybar-study")).toHaveLength(0);
  });

  it("36 месяцев: подпись у каждого третьего, таблица «По месяцам»", async () => {
    const { container } = render(<DayBars reply={await replyOf(bars(36, (i) => `m${i}`), "month")} />);
    expect(hits()).toHaveLength(36);
    const labels = Array.from(container.querySelectorAll(".daybars-label")).map((l) => l.textContent);
    expect(labels.filter(Boolean)).toEqual(["m0", "m3", "m6", "m9", "m12", "m15", "m18", "m21", "m24", "m27", "m30", "m33"]);
    expect(screen.getByRole("table", { name: "По месяцам" })).toBeTruthy();
    expect(within(screen.getByRole("table", { name: "По месяцам" })).getAllByRole("columnheader")[0].textContent).toBe("Месяц");
  });

  it("31 день: подписи 1, 8, 15, 22 и 29 числа", async () => {
    const { container } = render(<DayBars reply={await replyOf(bars(31, (i) => `${i + 1}`), "day")} />);
    const labels = Array.from(container.querySelectorAll(".daybars-label")).map((l) => l.textContent);
    expect(labels.filter(Boolean)).toEqual(["1", "8", "15", "22", "29"]);
  });

  it("нет столбиков - графика нет", async () => {
    const { container } = render(<DayBars reply={await replyOf([], "day")} />);
    expect(container.querySelector(".daybars")).toBeNull();
  });
});
