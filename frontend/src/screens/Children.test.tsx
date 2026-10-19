import { afterEach, describe, expect, it } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Children from "./Children";
import { UnsavedGuardProvider } from "../guard";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// Раздел «Дети» (сценарий 8.3.3): выбор ребёнка и его лимиты; смена ребёнка с правками спрашивает.

let mock: MockBackend;

function setup(opts: Parameters<typeof installBackend>[0] = {}, onOpenSettings = () => undefined) {
  mock = installBackend({ sessionActive: true, ...opts });
  render(
    <UnsavedGuardProvider>
      <Children onOpenSettings={onOpenSettings} />
    </UnsavedGuardProvider>,
  );
  return userEvent.setup();
}

const two = () => [mockChild(), mockChild({ childId: 2, sid: "S-2", name: "Даша" })];

afterEach(() => {
  uninstallBackend();
});

describe("Children", () => {
  it("один ребёнок: сразу его лимиты, без выбора", async () => {
    setup();
    expect(await screen.findByRole("table")).toBeTruthy();
    expect(screen.queryByRole("tablist")).toBeNull();
  });

  it("несколько детей: вкладки с именами, открыт первый", async () => {
    setup({ children: two() });
    const tabs = await screen.findAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual(["Миша", "Даша"]);
    expect(tabs[0].getAttribute("aria-selected")).toBe("true");
    expect(await screen.findByRole("heading", { name: /Лимиты времени, Миша/ })).toBeTruthy();
  });

  it("переключение показывает лимиты другого ребёнка", async () => {
    const user = setup({ children: two() });
    await screen.findByRole("table");
    await user.click(screen.getByRole("tab", { name: "Даша" }));
    expect(await screen.findByRole("heading", { name: /Лимиты времени, Даша/ })).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Даша" }).getAttribute("aria-selected")).toBe("true");
    expect(mock.calls.filter((c) => c.command === "GetChildRules").map((c) => c.payload.childId)).toEqual([1, 2]);
  });

  it("вкладки управляются стрелками, фокус только у выбранной", async () => {
    const user = setup({ children: two() });
    await screen.findByRole("table");
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((t) => t.tabIndex)).toEqual([0, -1]);
    tabs[0].focus();
    await user.keyboard("{ArrowRight}");
    expect(await screen.findByRole("heading", { name: /Лимиты времени, Даша/ })).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Даша" }));
    expect(screen.getByRole("tabpanel", { name: "Даша" })).toBeTruthy();
    await user.keyboard("{ArrowRight}");
    expect(await screen.findByRole("heading", { name: /Лимиты времени, Миша/ })).toBeTruthy();
    await user.keyboard("{ArrowLeft}");
    expect(await screen.findByRole("heading", { name: /Лимиты времени, Даша/ })).toBeTruthy();
  });

  // Ловит: переключение на другого ребёнка молча теряет несохранённые правки (FR-19).
  it("с несохранёнными правками переключение спрашивает «Сохранить изменения?»", async () => {
    const user = setup({ children: two() });
    await screen.findByRole("table");
    await user.click(screen.getByLabelText("Без лимита, понедельник"));
    await user.click(screen.getByRole("tab", { name: "Даша" }));
    const dialog = screen.getByRole("dialog", { name: "Сохранить изменения?" });
    await user.click(within(dialog).getByRole("button", { name: "Остаться" }));
    expect(screen.getByRole("heading", { name: /Лимиты времени, Миша/ })).toBeTruthy();
    expect((screen.getByLabelText("Без лимита, понедельник") as HTMLInputElement).checked).toBe(true);

    await user.click(screen.getByRole("tab", { name: "Даша" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Не сохранять" }));
    expect(await screen.findByRole("heading", { name: /Лимиты времени, Даша/ })).toBeTruthy();
  });

  it("детей нет: объясняет и ведёт в настройки", async () => {
    let opened = 0;
    const user = setup({ children: [] }, () => {
      opened += 1;
    });
    expect(await screen.findByText("Пока нет ни одного ребёнка")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Открыть настройки" }));
    expect(opened).toBe(1);
  });

  it("нет связи: простыми словами и «Повторить»", async () => {
    mock = installBackend({ sessionActive: true });
    mock.override("GetToday", "reject", 1);
    render(
      <UnsavedGuardProvider>
        <Children onOpenSettings={() => undefined} />
      </UnsavedGuardProvider>,
    );
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByRole("table")).toBeTruthy();
  });
});
