import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import HintCard from "./HintCard";
import { HINTS, LABELS, hintById, type HintEntry } from "./catalog";
import type { Candidate } from "./engine";

// Карточка подсказки (ТЗ 8.4 п.1): заголовок, 1-2 строки, до двух действий, «Позже», крестик,
// у повторяющихся «Больше не напоминать»; Esc равен «Позже».

function candidate(entry: HintEntry, name = "Миша"): Candidate {
  return { key: entry.id, entry, subject: "7", name, slot: `${entry.anchor}|7` };
}

function setup(entry: HintEntry, name?: string) {
  const handlers = {
    onLater: vi.fn(),
    onClose: vi.fn(),
    onMute: vi.fn(),
    onAction: vi.fn(),
  };
  const view = render(<HintCard candidate={candidate(entry, name)} {...handlers} />);
  return { ...view, ...handlers, user: userEvent.setup() };
}

const card = () => screen.getByRole("status");

describe("HintCard: состав", () => {
  // Ловит: карточка без заголовка или с лишним текстом сверх двух строк каталога.
  it("заголовок и текст из каталога", () => {
    const h = hintById("H-M1-03")!;
    setup(h);
    expect(within(card()).getByText(h.title)).toBeTruthy();
    expect(within(card()).getByText(h.body)).toBeTruthy();
  });

  // Ловит: в заголовке остаются «{nameGen}» или имя не в родительном падеже (урок L-36).
  it("имя ребёнка подставлено в родительном падеже", () => {
    setup(hintById("D-M1-01")!, "Миша");
    expect(within(card()).getByText("Похоже, у Миши нет ограничений ни в один день")).toBeTruthy();
    expect(card().textContent).not.toMatch(/[{}]/);
  });

  // Ловит: очень длинное имя ломает карточку (усечение обязательно, ТЗ 8.4 п.8).
  it("длинное имя усекается", () => {
    setup(hintById("D-M1-02")!, "Ы".repeat(80));
    expect(card().textContent).toContain("…");
    expect(card().textContent!.length).toBeLessThan(260);
  });

  it("не больше двух кнопок-действий", () => {
    for (const h of HINTS) {
      const { unmount } = setup(h);
      const own = within(card())
        .queryAllByRole("button")
        .filter((b) => h.actions.some((a) => a.label === b.textContent));
      expect(own.length).toBe(h.actions.length);
      expect(own.length).toBeLessThanOrEqual(2);
      unmount();
    }
  });

  // Ловит: карточка есть, а значка нет, или значок читается скринридером как лишний текст (L-45).
  it("значок один, декоративный", () => {
    setup(hintById("H-M1-02")!);
    const icons = card().querySelectorAll("svg");
    expect(icons.length).toBeGreaterThanOrEqual(1);
    icons.forEach((i) => expect(i.getAttribute("aria-hidden")).toBe("true"));
  });

  it("объявляется скринридеру вежливо: role=status, а не alert", () => {
    setup(hintById("D-M1-02")!);
    expect(card().getAttribute("aria-live")).toBe("polite");
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("HintCard: ответы", () => {
  it("«Позже» у каждой карточки", async () => {
    for (const h of HINTS) {
      const { user, onLater, unmount } = setup(h);
      await user.click(within(card()).getByRole("button", { name: LABELS.later }));
      expect(onLater).toHaveBeenCalledTimes(1);
      unmount();
    }
  });

  it("крестик у каждой карточки закрывает", async () => {
    const { user, onClose } = setup(hintById("H-M1-02")!);
    await user.click(within(card()).getByRole("button", { name: LABELS.close }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  // Ловит: «Больше не напоминать» у одноразовой подсказки (там нечего отключать) или нет у повторяющейся.
  it("«Больше не напоминать» только у повторяющихся", async () => {
    for (const h of HINTS) {
      const { user, onMute, unmount } = setup(h);
      const mute = within(card()).queryByRole("button", { name: LABELS.mute });
      if (h.repeat) {
        expect(mute).toBeTruthy();
        await user.click(mute!);
        expect(onMute).toHaveBeenCalledTimes(1);
      } else {
        expect(mute).toBeNull();
      }
      unmount();
    }
  });

  it("действие передаёт свой номер", async () => {
    const { user, onAction } = setup(hintById("D-M1-01")!);
    await user.click(within(card()).getByRole("button", { name: "Задать лимиты" }));
    expect(onAction).toHaveBeenCalledWith("open-limits");
  });
});

describe("HintCard: Esc", () => {
  // Ловит: Esc не равен «Позже» (ТЗ 8.4 п.1), карточку нельзя убрать с клавиатуры.
  it("Esc равен «Позже»", () => {
    const { onLater, onClose } = setup(hintById("H-M1-02")!);
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(onLater).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("Esc с фокусом на кнопке карточки тоже «Позже»", async () => {
    const { user, onLater } = setup(hintById("H-M1-02")!);
    await user.tab();
    await user.keyboard("{Escape}");
    expect(onLater).toHaveBeenCalledTimes(1);
  });

  // Ловит: Esc в поле ввода убирает подсказку, хотя человек им отменял что-то своё.
  it("Esc в поле ввода карточку не трогает", () => {
    const { onLater } = setup(hintById("H-M1-02")!);
    const input = document.createElement("input");
    document.body.append(input);
    input.focus();
    fireEvent.keyDown(input, { key: "Escape" });
    expect(onLater).not.toHaveBeenCalled();
    input.remove();
  });

  it("Esc, который уже обработан другим окном, не считается", () => {
    const { onLater } = setup(hintById("H-M1-02")!);
    const ev = new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true });
    ev.preventDefault();
    document.body.dispatchEvent(ev);
    expect(onLater).not.toHaveBeenCalled();
  });

  it("после снятия карточки Esc больше не слушается", () => {
    const { onLater, unmount } = setup(hintById("H-M1-02")!);
    unmount();
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(onLater).not.toHaveBeenCalled();
  });
});
