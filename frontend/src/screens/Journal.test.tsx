import { afterEach, describe, expect, it } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Journal from "./Journal";
import { installBackend, uninstallBackend, type MockBackend } from "../__mocks__/backend";
import type { JournalEntry } from "../types";

// FR-24 (M1: список), сценарий 8.3.7: события простыми словами, пустой журнал не краш.

let mock: MockBackend;

const event = (over: Partial<JournalEntry>): JournalEntry => ({
  atMs: Date.UTC(2026, 9, 19, 18, 14),
  tzOffsetMin: 180,
  childId: 1,
  childName: "Миша",
  kind: "bypass",
  text: "Миша: попытка обойти защиту. Экран заблокирован",
  ...over,
});

afterEach(() => {
  uninstallBackend();
});

describe("Journal", () => {
  it("события простыми словами: время и что произошло, новые сверху", async () => {
    mock = installBackend({
      sessionActive: true,
      events: [
        event({ atMs: Date.UTC(2026, 9, 19, 15, 0), kind: "extra_time", text: "Миша: доп. время +30 мин" }),
        event({}),
      ],
    });
    render(<Journal />);
    const items = await screen.findAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(within(items[0]).getByText("19 окт, 21:14")).toBeTruthy();
    expect(within(items[0]).getByText("Миша: попытка обойти защиту. Экран заблокирован")).toBeTruthy();
    expect(within(items[1]).getByText("19 окт, 18:00")).toBeTruthy();
    expect(within(items[1]).getByText("Миша: доп. время +30 мин")).toBeTruthy();
  });

  // Ловит: пустой журнал - пустой экран или падение (nil вместо списка, L-22).
  it("пустой журнал: «Пока событий нет»", async () => {
    installBackend({ sessionActive: true });
    render(<Journal />);
    expect(await screen.findByText("Пока событий нет")).toBeTruthy();
    expect(screen.queryByRole("list")).toBeNull();
  });

  it("null вместо списка событий не роняет экран", async () => {
    mock = installBackend({ sessionActive: true });
    mock.override("ListEvents", { ok: true, payload: { events: null } });
    render(<Journal />);
    expect(await screen.findByText("Пока событий нет")).toBeTruthy();
  });

  it("запрашивает события без ограничений по ребёнку и периоду", async () => {
    mock = installBackend({ sessionActive: true });
    render(<Journal />);
    await screen.findByText("Пока событий нет");
    expect(mock.calls.find((c) => c.command === "ListEvents")?.payload).toEqual({
      childId: 0,
      fromMs: 0,
      toMs: 0,
      limit: 200,
    });
  });

  it("нет связи: простыми словами и «Повторить»", async () => {
    mock = installBackend({ sessionActive: true, events: [event({})] });
    mock.override("ListEvents", "reject", 1);
    render(<Journal />);
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByText(/попытка обойти защиту/)).toBeTruthy();
  });

  // Ловит: ссылка «Нарушения сегодня» ведёт на журнал, где нарушения утонули среди прочего.
  it("из ссылки о нарушениях показывает только нарушения, можно показать всё", async () => {
    installBackend({
      sessionActive: true,
      events: [event({}), event({ atMs: Date.UTC(2026, 9, 19, 15, 0), kind: "extra_time", text: "Миша: доп. время +30 мин" })],
    });
    const user = userEvent.setup();
    render(<Journal violationsOnly />);
    expect(await screen.findByText(/попытка обойти защиту/)).toBeTruthy();
    expect(screen.queryByText(/доп. время/)).toBeNull();
    expect(screen.getByRole("heading", { name: "Журнал: нарушения" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Показать все события" }));
    expect(screen.getByText(/доп. время/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Показать только нарушения" }));
    expect(screen.queryByText(/доп. время/)).toBeNull();
  });

  it("нарушений нет: так и говорит", async () => {
    installBackend({ sessionActive: true, events: [event({ kind: "extra_time", text: "Миша: доп. время +30 мин" })] });
    render(<Journal violationsOnly />);
    expect(await screen.findByText("Нарушений пока нет")).toBeTruthy();
  });
});
