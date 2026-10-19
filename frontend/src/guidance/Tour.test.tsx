import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Tour, { FIND_MS } from "./Tour";
import { LABELS, TOURS, type ScreenId } from "./catalog";

// Тур (ТЗ 8.4 п.5): подсветка, не больше шести шагов, «Пропустить тур» на каждом шаге,
// только показывает.

const tour = TOURS[0];

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
  document.body.innerHTML = "";
});

function anchors(ids: string[]) {
  const host = document.createElement("div");
  for (const id of ids) {
    const el = document.createElement("div");
    el.setAttribute("data-help-anchor", id);
    host.append(el);
  }
  document.body.append(host);
  return host;
}

function setup(opts: { screen?: ScreenId; hidden?: boolean } = {}) {
  const navigate = vi.fn();
  const onEnd = vi.fn();
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  const view = render(<Tour tour={tour} screen={opts.screen ?? "today"} navigate={navigate} hidden={opts.hidden} onEnd={onEnd} />);
  return { user, navigate, onEnd, ...view };
}

const dialog = () => screen.getByRole("dialog");
const spot = () => document.querySelector<HTMLElement>(".tour-spot");
const next = () => within(dialog()).getByRole("button", { name: LABELS.tourNext });

describe("Tour: состав", () => {
  it("не больше шести шагов", () => {
    for (const t of TOURS) expect(t.steps.length).toBeLessThanOrEqual(6);
  });

  it("первый шаг: счётчик, заголовок и текст из каталога", () => {
    anchors(tour.steps.map((s) => s.anchor));
    setup();
    expect(within(dialog()).getByText("Шаг 1 из 6")).toBeTruthy();
    expect(within(dialog()).getByRole("heading", { name: tour.steps[0].title })).toBeTruthy();
    expect(within(dialog()).getByText(tour.steps[0].body)).toBeTruthy();
  });

  // Ловит: тур с неназванным диалогом, скринридер не знает, что открылось.
  it("диалог назван по туру", () => {
    anchors(tour.steps.map((s) => s.anchor));
    setup();
    expect(dialog().getAttribute("aria-label")).toBe(tour.title);
  });
});

describe("Tour: подсветка", () => {
  // Ловит: подсвечен не тот элемент, что назван в каталоге.
  it("подсвечивает элемент шага", () => {
    anchors(tour.steps.map((s) => s.anchor));
    setup();
    expect(spot()?.getAttribute("data-anchor")).toBe("menu-today");
  });

  it("следующий шаг подсвечивает следующий элемент", async () => {
    anchors(tour.steps.map((s) => s.anchor));
    const { user } = setup();
    await user.click(next());
    expect(spot()?.getAttribute("data-anchor")).toBe("child-card");
    expect(within(dialog()).getByText("Шаг 2 из 6")).toBeTruthy();
  });

  // Ловит: подсветка и затемнение читаются скринридером как содержимое.
  it("подсветка и затемнение скрыты от скринридера", () => {
    anchors(tour.steps.map((s) => s.anchor));
    setup();
    expect(spot()?.getAttribute("aria-hidden")).toBe("true");
    expect(document.querySelector(".tour-scrim")?.getAttribute("aria-hidden")).toBe("true");
  });

  // Ловит: элемент ещё грузится, а тур уже объявил «не вижу» (данные приходят позже окна).
  it("элемент появился не сразу: ждём, а не ругаемся", async () => {
    setup();
    expect(spot()).toBeNull();
    expect(screen.queryByText(LABELS.tourMissingNote)).toBeNull();
    anchors(["menu-today"]);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(400);
    });
    expect(spot()?.getAttribute("data-anchor")).toBe("menu-today");
  });

  // Ловит: тур падает или молчит, если нужного элемента нет (другой экран, пустой список).
  it("элемент не нашёлся: шаг по центру с пояснением, тур идёт дальше", async () => {
    const { user } = setup();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(FIND_MS + 400);
    });
    expect(spot()).toBeNull();
    expect(within(dialog()).getByText(LABELS.tourMissingNote)).toBeTruthy();
    expect(dialog().className).toContain("tour-card-center");
    await user.click(next());
    expect(within(dialog()).getByText("Шаг 2 из 6")).toBeTruthy();
  });
});

describe("Tour: выходы", () => {
  // Ловит: на каком-то шаге нет «Пропустить тур» (ТЗ 8.4 п.5).
  it("«Пропустить тур» есть на каждом шаге", async () => {
    anchors(tour.steps.map((s) => s.anchor));
    const { user } = setup();
    for (let i = 0; i < tour.steps.length; i++) {
      expect(within(dialog()).getByRole("button", { name: LABELS.tourSkip })).toBeTruthy();
      if (i < tour.steps.length - 1) await user.click(next());
    }
  });

  it("«Пропустить тур» заканчивает тур как непройденный", async () => {
    anchors(tour.steps.map((s) => s.anchor));
    const { user, onEnd } = setup();
    await user.click(within(dialog()).getByRole("button", { name: LABELS.tourSkip }));
    expect(onEnd).toHaveBeenCalledWith(false);
  });

  it("Esc пропускает тур", () => {
    anchors(tour.steps.map((s) => s.anchor));
    const { onEnd } = setup();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onEnd).toHaveBeenCalledWith(false);
  });

  it("на последнем шаге «Готово» заканчивает тур как пройденный", async () => {
    anchors(tour.steps.map((s) => s.anchor));
    const { user, onEnd } = setup();
    for (let i = 0; i < tour.steps.length - 1; i++) await user.click(next());
    expect(within(dialog()).queryByRole("button", { name: LABELS.tourNext })).toBeNull();
    await user.click(within(dialog()).getByRole("button", { name: LABELS.tourDone }));
    expect(onEnd).toHaveBeenCalledWith(true);
  });

  it("«Назад» возвращает на шаг; на первом шаге его нет", async () => {
    anchors(tour.steps.map((s) => s.anchor));
    const { user } = setup();
    expect(within(dialog()).queryByRole("button", { name: LABELS.tourBack })).toBeNull();
    await user.click(next());
    await user.click(within(dialog()).getByRole("button", { name: LABELS.tourBack }));
    expect(within(dialog()).getByText("Шаг 1 из 6")).toBeTruthy();
  });
});

describe("Tour: экраны", () => {
  // Ловит: шаг про «Дети» показан на «Сегодня» и подсвечивает пустоту.
  it("шаг другого экрана: переходит один раз", async () => {
    anchors(tour.steps.map((s) => s.anchor));
    const { user, navigate, rerender } = setup({ screen: "today" });
    expect(navigate).not.toHaveBeenCalled();
    for (let i = 0; i < 3; i++) await user.click(next());
    expect(navigate).toHaveBeenCalledTimes(1);
    expect(navigate).toHaveBeenCalledWith("children");
    // Экран сменился - повторного перехода нет.
    rerender(<Tour tour={tour} screen="children" navigate={navigate} onEnd={vi.fn()} />);
    expect(navigate).toHaveBeenCalledTimes(1);
  });

  // Ловит: переход не пустили (несохранённые правки), а тур снова и снова зовёт вопрос.
  it("переход не удался: второй раз не просит", async () => {
    anchors(tour.steps.map((s) => s.anchor));
    const { user, navigate, rerender } = setup({ screen: "today" });
    for (let i = 0; i < 3; i++) await user.click(next());
    // Окно родителя каждый раз передаёт новую функцию перехода: повтора всё равно быть не должно.
    rerender(<Tour tour={tour} screen="today" navigate={(s) => navigate(s)} onEnd={vi.fn()} />);
    rerender(<Tour tour={tour} screen="today" navigate={(s) => navigate(s)} onEnd={vi.fn()} />);
    expect(navigate).toHaveBeenCalledTimes(1);
  });
});

describe("Tour: тихие места и фокус", () => {
  // Ловит: тур поверх открытого диалога сохранения или поля пароля.
  it("в тихом месте тур прячется, а шаг остаётся", async () => {
    anchors(tour.steps.map((s) => s.anchor));
    const { user, rerender, navigate } = setup();
    await user.click(next());
    rerender(<Tour tour={tour} screen="today" navigate={navigate} hidden onEnd={vi.fn()} />);
    expect(screen.queryByRole("dialog")).toBeNull();
    rerender(<Tour tour={tour} screen="today" navigate={navigate} onEnd={vi.fn()} />);
    expect(within(dialog()).getByText("Шаг 2 из 6")).toBeTruthy();
  });

  it("фокус на «Далее» при показе, после тура возвращается", () => {
    anchors(tour.steps.map((s) => s.anchor));
    const before = document.createElement("button");
    document.body.append(before);
    before.focus();
    const { unmount } = setup();
    expect(document.activeElement).toBe(next());
    unmount();
    expect(document.activeElement).toBe(before);
  });

  it("Tab не уходит из карточки тура", async () => {
    anchors(tour.steps.map((s) => s.anchor));
    const { user } = setup();
    for (let i = 0; i < 6; i++) await user.tab();
    expect(dialog().contains(document.activeElement)).toBe(true);
    await user.tab({ shift: true });
    expect(dialog().contains(document.activeElement)).toBe(true);
  });
});

describe("Tour: пустая панель «Сохранить»", () => {
  // Ловит: у пустой панели нулевая высота, подсветка невидима.
  it("рамка не меньше 44 пикселей", async () => {
    anchors(tour.steps.map((s) => s.anchor));
    const { user } = setup({ screen: "children" });
    for (let i = 0; i < 5; i++) await user.click(next());
    expect(spot()?.getAttribute("data-anchor")).toBe("savebar");
    expect(parseInt(spot()!.style.width, 10)).toBeGreaterThanOrEqual(44);
    expect(parseInt(spot()!.style.height, 10)).toBeGreaterThanOrEqual(44);
  });
});
