import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "../App";
import { DEFAULT_PASSWORD, installBackend, mockChild, uninstallBackend } from "../__mocks__/backend";
import { ANCHORS, HINTS, TOURS, type AnchorId, type ScreenId } from "./catalog";

// Регресс «все якоря каталога существуют» (ТЗ 8.4 п.8): переименовали или потеряли элемент в
// экране, а подсказка или шаг тура остались ни к чему не привязаны. Проверяем на настоящем окне.

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: new Date(2026, 9, 19, 12, 0, 0) });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

async function openWindow() {
  // Подсказки выключены: проверяем только разметку, карточки не мешают.
  // Запретные часы каждый день: на карточке есть кнопка «Снять запрет на сегодня» (якорь no-forbidden).
  const schedule = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, startMin: 21 * 60, endMin: 7 * 60 }));
  installBackend({ helper: { enabled: false }, children: [mockChild({ schedule })] });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<App />);
  await user.type(await screen.findByLabelText("Пароль родителя"), `${DEFAULT_PASSWORD}{Enter}`);
  await screen.findByRole("heading", { name: "Сегодня" });
  return user;
}

const MENU_LABEL: Partial<Record<ScreenId, string>> = { today: "Сегодня", children: "Дети", bonus: "Копилка", settings: "Настройки", journal: "Журнал", help: "Помощь" };

async function openScreen(user: ReturnType<typeof userEvent.setup>, id: ScreenId, anchor = "") {
  await user.click(screen.getByRole("button", { name: MENU_LABEL[id]! }));
  if (id === "bonus") {
    // Экран «Копилка» разбит на вкладки: якорь формы промокода лежит во вкладке «Бонусы», остальные в «Копилке».
    const tab = anchor === "promo-create" ? "Бонусы" : "Копилка";
    await user.click(screen.getByRole("tab", { name: tab }));
    await screen.findByRole("tabpanel", { name: tab });
  }
}

const has = (anchor: string) => document.querySelector(`[data-help-anchor="${anchor}"]`) !== null;

describe("якоря каталога есть в окне", () => {
  // Ловит: якорь из каталога пропал из разметки экрана.
  it("каждый известный якорь находится на своём экране", async () => {
    const user = await openWindow();
    for (const a of ANCHORS) {
      const screenId: ScreenId = a.screen === "any" ? "today" : a.screen;
      await openScreen(user, screenId, a.id);
      if (a.id === "limits-table") await screen.findByRole("table");
      if (a.id === "child-card") await screen.findByRole("article", { name: "Миша" });
      expect(has(a.id), `якоря ${a.id} нет на экране «${screenId}»`).toBe(true);
    }
  });

  it("якоря общего назначения видны на каждом экране", async () => {
    const user = await openWindow();
    for (const id of ["today", "children", "journal", "settings", "help"] as const) {
      await openScreen(user, id);
      for (const a of ANCHORS.filter((x) => x.screen === "any")) {
        expect(has(a.id), `якоря ${a.id} нет на экране «${id}»`).toBe(true);
      }
    }
  });

  // Ловит: подсказка привязана к якорю, которого нет в списке известных.
  it("каждая подсказка и шаг тура привязаны к известному якорю", () => {
    const known = new Set<string>(ANCHORS.map((a) => a.id));
    for (const h of HINTS) expect(known.has(h.anchor), `${h.id}: ${h.anchor}`).toBe(true);
    for (const t of TOURS) for (const s of t.steps) expect(known.has(s.anchor), `${t.id}: ${s.anchor}`).toBe(true);
  });

  // Ловит: якорь шага тура есть, но на экране шага его нет - тур подсветил бы пустоту.
  it("якорь каждого шага тура есть на экране этого шага", async () => {
    const user = await openWindow();
    for (const t of TOURS) {
      for (const s of t.steps) {
        await openScreen(user, s.screen, s.anchor);
        if (s.anchor === "limits-table") await screen.findByRole("table");
        if (s.anchor === "child-card") await screen.findByRole("article", { name: "Миша" });
        expect(has(s.anchor), `${t.id}: якоря ${s.anchor} нет на экране «${s.screen}»`).toBe(true);
      }
    }
  });

  // Ловит: якорь размножился (два элемента с одним якорем на экране «Дети»), тур подсветит не тот.
  it("на экране «Дети» якорь таблицы один", async () => {
    const user = await openWindow();
    await openScreen(user, "children");
    await screen.findByRole("table");
    expect(document.querySelectorAll('[data-help-anchor="limits-table"]')).toHaveLength(1);
  });

  it("список известных якорей без повторов", () => {
    const ids: AnchorId[] = ANCHORS.map((a) => a.id);
    expect(new Set(ids).size).toBe(ids.length);
  });
});
