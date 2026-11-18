import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ChildLimits from "./ChildLimits";
import { UnsavedGuardProvider } from "../guard";
import { installBackend, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// Вкладка «Автозагрузка» в правилах ребёнка (FR-39, 8.3.11): последняя, вне черновика «Сохранить».

let mock: MockBackend;

function setup() {
  mock = installBackend({ sessionActive: true });
  const user = userEvent.setup();
  render(
    <UnsavedGuardProvider>
      <ChildLimits childId={1} />
    </UnsavedGuardProvider>,
  );
  return user;
}

const minutes = (day: string) => screen.getByLabelText(`Минуты, ${day}`) as HTMLInputElement;
const refreshes = () => mock.calls.filter((c) => c.command === "ListStartup" && c.payload.refresh === true).length;
const openStartup = (user: ReturnType<typeof userEvent.setup>) =>
  user.click(within(screen.getByRole("tablist", { name: "Правила ребёнка" })).getByRole("tab", { name: "Автозагрузка" }));

beforeEach(() => uninstallBackend());
afterEach(() => uninstallBackend());

describe("ChildLimits: вкладка «Автозагрузка»", () => {
  // Ловит: вкладка не последняя; панель не связана с вкладкой; список не проверяется заново при
  // каждом открытии вкладки (RM5b-5).
  it("последняя вкладка; каждое открытие просит службу проверить Windows", async () => {
    const user = setup();
    await screen.findByRole("table");
    const tabs = within(screen.getByRole("tablist", { name: "Правила ребёнка" })).getAllByRole("tab");
    expect(tabs.at(-1)?.textContent).toBe("Автозагрузка");
    expect(refreshes()).toBe(0);
    await openStartup(user);
    expect(screen.getByRole("tabpanel", { name: "Автозагрузка" }).id).toBe("limits-panel");
    expect(await screen.findByRole("heading", { name: "Учётка Миши" })).toBeTruthy();
    expect(refreshes()).toBe(1);
    await user.click(screen.getByRole("tab", { name: "Лимиты" }));
    await openStartup(user);
    await screen.findByRole("heading", { name: "Учётка Миши" });
    expect(refreshes()).toBe(2);
  });

  // Ловит: выключатель делает правила «несохранёнными» и показывает панель «Сохранить».
  it("выключатель не делает правки несохранёнными", async () => {
    const user = setup();
    await screen.findByRole("table");
    await openStartup(user);
    await user.click(await screen.findByRole("switch", { name: "Discord" }));
    expect((await screen.findAllByText("Не будет запускаться при входе Миши")).length).toBeGreaterThan(0);
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
    expect(screen.queryByRole("button", { name: "Сохранить" })).toBeNull();
    expect(mock.calls.filter((c) => c.command === "SaveLimits")).toEqual([]);
  });

  // Ловит: «Отменить изменения» других вкладок возвращает и решение автозагрузки или шлёт его службе.
  it("«Отменить изменения» лимитов не трогает решение автозагрузки", async () => {
    const user = setup();
    await screen.findByRole("table");
    await user.clear(minutes("вторник"));
    await user.type(minutes("вторник"), "45");
    await openStartup(user);
    await user.click(await screen.findByRole("switch", { name: "Discord" }));
    await screen.findAllByText("Не будет запускаться при входе Миши");
    expect(screen.getByText("Есть несохранённые изменения")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Отменить изменения" }));
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
    expect(screen.getByRole("switch", { name: "Discord" }).getAttribute("aria-checked")).toBe("false");
    expect(mock.calls.filter((c) => c.command === "SetStartup")).toHaveLength(1);
    expect(mock.state.startup.get(1)?.find((x) => x.id === 2)?.decision).toBe("off");
  });
});
