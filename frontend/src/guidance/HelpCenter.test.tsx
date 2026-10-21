import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { installBackend, mockChild, uninstallBackend, type MockBackend, type MockOptions } from "../__mocks__/backend";
import { TOURS, WHATS_NEW, type ScreenId } from "./catalog";
import GuidanceProvider from "./GuidanceProvider";
import HelpCenter from "./HelpCenter";
import { HelperEngine } from "./engine";
import { apiHelperIO } from "./io";

// «Помощь» (ТЗ 8.4 п.6): туры, «Первые шаги» с автоотметками, «Что нового». Работает всегда,
// даже при выключенных подсказках (S28).

let mock: MockBackend;

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: new Date(2026, 9, 19, 12, 0, 0) });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

function setup(helper: Record<string, unknown> = {}, opts: MockOptions = {}) {
  mock = installBackend({ sessionActive: true, helper, ...opts });
  const navigate = vi.fn<(screen: ScreenId) => void>();
  const engine = new HelperEngine(apiHelperIO);
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(
    <GuidanceProvider engine={engine} screen="help" navigate={navigate}>
      <div data-help-anchor="menu-today" />
      <HelpCenter />
    </GuidanceProvider>,
  );
  return { user, navigate, engine };
}

const region = (name: string) => screen.getByRole("region", { name });

/** Строка конкретного тура в «Обучении»: туров теперь несколько, кнопка «Начать» у каждого. */
const tourRow = (title: string): HTMLElement =>
  within(region("Обучение")).getByText(title).closest("li") as HTMLElement;

describe("Помощь: состав", () => {
  it("заголовок и три раздела", async () => {
    setup();
    expect(screen.getByRole("heading", { level: 2, name: "Помощь" })).toBeTruthy();
    for (const name of ["Обучение", "Первые шаги", "Что нового"]) {
      expect(region(name)).toBeTruthy();
    }
  });

  it("список туров: название, число шагов, «Начать»", async () => {
    setup();
    const box = region("Обучение");
    expect(within(box).getByText(TOURS[0].title)).toBeTruthy();
    expect(within(box).getByText(/6 шагов, около минуты/)).toBeTruthy();
    expect(within(tourRow(TOURS[0].title)).getByRole("button", { name: "Начать" })).toBeTruthy();
  });

  // Ловит: тур «Начать» в «Помощи» ничего не делает.
  it("«Начать» запускает тур", async () => {
    const { user } = setup();
    await user.click(within(tourRow(TOURS[0].title)).getByRole("button", { name: "Начать" }));
    expect(await screen.findByRole("dialog", { name: TOURS[0].title })).toBeTruthy();
  });

  it("пройденный тур отмечен, а запустить можно снова", async () => {
    setup({ tours: [TOURS[0].id] });
    const row = tourRow(TOURS[0].title);
    expect(await within(row).findByText("Пройден")).toBeTruthy();
    expect(within(row).getByRole("button", { name: "Пройти ещё раз" })).toBeTruthy();
    expect(within(row).queryByRole("button", { name: "Начать" })).toBeNull();
  });

  it("«Что нового»: версия, название и пункты", async () => {
    setup();
    const box = region("Что нового");
    const news = WHATS_NEW[0];
    expect(within(box).getByRole("heading", { level: 4, name: `Версия ${news.version}. ${news.title}` })).toBeTruthy();
    for (const item of news.items) expect(within(box).getByText(item)).toBeTruthy();
  });
});

describe("Помощь: при выключенных подсказках", () => {
  // Ловит: выключатель в настройках гасит и справку, и тур (S28).
  it("тур и разделы работают, а выключенность объяснена", async () => {
    const { user, navigate } = setup({ enabled: false });
    expect(await screen.findByText(/Подсказки в окне сейчас выключены/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Открыть «Настройки»" }));
    expect(navigate).toHaveBeenCalledWith("settings");
    await user.click(within(tourRow(TOURS[0].title)).getByRole("button", { name: "Начать" }));
    expect(await screen.findByRole("dialog", { name: TOURS[0].title })).toBeTruthy();
    expect(region("Что нового")).toBeTruthy();
  });

  it("при включённых подсказках пояснения про выключение нет", async () => {
    setup({ enabled: true });
    await vi.waitFor(() => expect(screen.queryByText("Загружаю данные")).toBeNull());
    expect(screen.queryByText(/Подсказки в окне сейчас выключены/)).toBeNull();
  });
});

describe("Помощь: первые шаги", () => {
  const stepRow = (label: string) => within(region("Первые шаги")).getByText(label).closest("li")!;

  // Ловит: отметки не по делу (шаг сделан, а в списке «не сделано», и наоборот).
  it("отмечает сделанное по данным программы", async () => {
    setup({}, { children: [mockChild()] });
    await vi.waitFor(() => expect(screen.queryByText("Загружаю данные")).toBeNull());
    expect(stepRow("Пароль родителя задан").getAttribute("data-done")).toBe("true");
    expect(stepRow("Дети отмечены").getAttribute("data-done")).toBe("true");
    expect(stepRow("Лимиты заданы").getAttribute("data-done")).toBe("true");
    expect(stepRow("Время добавлено с главной или с экрана блокировки").getAttribute("data-done")).toBe("false");
  });

  it("смысл отметки передан словами, не только значком", async () => {
    setup({}, { children: [mockChild()] });
    await vi.waitFor(() => expect(screen.queryByText("Загружаю данные")).toBeNull());
    expect(within(stepRow("Пароль родителя задан")).getByText("Сделано")).toBeTruthy();
    expect(within(stepRow("Время добавлено с главной или с экрана блокировки")).getByText("Пока не сделано")).toBeTruthy();
  });

  it("у несделанного шага кнопка ведёт туда, где его сделать", async () => {
    const { user, navigate } = setup({}, { children: [] });
    await vi.waitFor(() => expect(screen.queryByText("Загружаю данные")).toBeNull());
    await user.click(within(stepRow("Дети отмечены")).getByRole("button", { name: "Открыть «Настройки»" }));
    expect(navigate).toHaveBeenCalledWith("settings");
  });

  // Ловит: пустой прогресс «0 из 5» выглядит как упрёк (правило каталога: пустое не показывать как «0»).
  it("пока ничего не сделано, счётчика нет", async () => {
    setup({}, { children: [], configured: false });
    await vi.waitFor(() => expect(screen.queryByText("Загружаю данные")).toBeNull());
    expect(within(region("Первые шаги")).queryByText(/Сделано \d+ из/)).toBeNull();
  });

  it("часть сделана - счётчик «Сделано N из 5»", async () => {
    setup({}, { children: [mockChild()] });
    await vi.waitFor(() => expect(screen.queryByText("Загружаю данные")).toBeNull());
    expect(within(region("Первые шаги")).getByText(/^Сделано \d из 5$/)).toBeTruthy();
  });

  it("не прочиталось: простыми словами и «Повторить»", async () => {
    mock = installBackend({ sessionActive: true, children: [mockChild()] });
    mock.override("GetToday", "reject", 1);
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(
      <GuidanceProvider engine={new HelperEngine(apiHelperIO)} screen="help" navigate={() => undefined}>
        <HelpCenter />
      </GuidanceProvider>,
    );
    const box = region("Первые шаги");
    expect(await within(box).findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await user.click(within(box).getByRole("button", { name: "Повторить" }));
    await waitFor(() => expect(within(box).queryByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeNull());
    expect(within(box).getByText("Пароль родителя задан")).toBeTruthy();
  });
});

describe("Помощь: без Помощника вокруг", () => {
  it("списки на месте, кнопок запуска нет", async () => {
    mock = installBackend({ sessionActive: true });
    render(<HelpCenter />);
    expect(region("Обучение")).toBeTruthy();
    expect(within(region("Обучение")).queryByRole("button")).toBeNull();
  });
});
