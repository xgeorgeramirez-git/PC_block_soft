import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";
import { CHILD_LINK_ERROR_TEXT } from "../lib/errors";
import { promoPicker, StickerPicker } from "../lib/stickers";

// Промокод в окне ребёнка (R26, D-83, 8.2.6 п.11, RM5b-10): ответ - стикер и надпись. Удача -
// надпись с минутами кода, неудача - по полю ошибки «code», другие ошибки - прежний текст без
// стикера; ввод в поле скрывает ответ. Проигрыватель и файлы стикеров подменены (в jsdom их нет).

vi.mock("../lib/lottie", () => ({
  loadLottie: () =>
    Promise.resolve({
      loadAnimation: () => ({
        totalFrames: 90,
        play() {},
        stop() {},
        goToAndStop() {},
        addEventListener() {},
        destroy() {},
      }),
    }),
}));

vi.mock("../lib/stickerData", () => ({
  loadStickerData: (n: string) => Promise.resolve({ sticker: n }),
}));

import PromoRedeem from "./PromoRedeem";

let mock: MockBackend;
const NOW = new Date(2026, 10, 16, 12, 0, 0).getTime();

function setup(opts: { picker?: StickerPicker; myChildId?: number | null } = {}) {
  mock = installBackend({
    mode: "mystats",
    children: [mockChild()],
    myChildId: opts.myChildId,
    promos: [
      { codeDisplay: "AB12", minutes: 30, createdAtMs: NOW - 3600_000 },
      { codeDisplay: "CD34", minutes: 1, createdAtMs: NOW - 3600_000 },
    ],
  });
  const onRedeemed = vi.fn();
  const user = userEvent.setup();
  render(<PromoRedeem onRedeemed={onRedeemed} picker={opts.picker ?? new StickerPicker(() => 0)} />);
  const field = screen.getByRole("textbox", { name: "Промокод" }) as HTMLInputElement;
  const send = async (code: string) => {
    await user.clear(field);
    await user.type(field, code);
    await user.click(screen.getByRole("button", { name: "Ввести промокод" }));
  };
  return { user, field, send, onRedeemed };
}

/** Номер показанного стикера (рамка проигрывателя). */
const shownSticker = () => document.querySelector(".promo-sticker")?.getAttribute("data-sticker") ?? null;

beforeEach(() => {
  promoPicker.reset();
});

afterEach(() => {
  uninstallBackend();
});

describe("PromoRedeem: ответ стикером (R26)", () => {
  // Ловит: надпись службы «Добавлено 1 минуту» вместо надписи R26; «+1 минут»; остаток не обновлён.
  it("верный CD34: стикер удачи и «Ура! +1 минута», поле пустое, остаток перечитан", async () => {
    const { field, send, onRedeemed } = setup();
    await send("cd34");
    expect((await screen.findByRole("status")).textContent).toBe("Ура! +1 минута");
    expect(shownSticker()).toBe("017");
    expect(screen.queryByText("Добавлено 1 минуту")).toBeNull();
    expect(field.value).toBe("");
    expect(onRedeemed).toHaveBeenCalledTimes(1);
    expect(mock.state.children[0].extraMin).toBe(1);
  });

  // Ловит: использованный код показан текстом службы «Код не подходит» без стикера, код стёрт.
  it("повтор CD34: стикер неудачи и надпись, код в поле остаётся, время прежнее", async () => {
    const { field, send } = setup();
    await send("CD34");
    await screen.findByText("Ура! +1 минута");
    await send("cd34");
    expect((await screen.findByRole("alert")).textContent).toBe("Такого кода нет. Проверь буквы и цифры");
    expect(shownSticker()).toBe("023");
    expect(screen.queryByText("Код не подходит")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
    expect(field.value).toBe("cd34");
    expect(mock.state.children[0].extraMin).toBe(1);
  });

  // Ловит: второй неверный код подряд показывает тот же стикер и ту же надпись (8.2.6 п.11).
  it("два неверных подряд: разные стикер и надпись", async () => {
    const { send } = setup();
    await send("ZZZ1");
    await screen.findByText("Такого кода нет. Проверь буквы и цифры");
    const first = shownSticker();
    await send("ZZZ2");
    expect((await screen.findByRole("alert")).textContent).toBe("Хм, код не подходит. Посмотри внимательнее");
    expect(shownSticker()).toBe("028");
    expect(first).toBe("023");
  });

  // Ловит: выбор живёт в компоненте и начинается заново после перехода на другую вкладку
  // (уточнение 10 плана M5b: один выбор на окно).
  it("после повторного открытия раздела неудача не повторяет прошлую", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    try {
      const a = setup({ picker: promoPicker });
      await a.send("ZZZ1");
      await screen.findByText("Такого кода нет. Проверь буквы и цифры");
      expect(shownSticker()).toBe("023");
      cleanup();
      uninstallBackend();
      const b = setup({ picker: promoPicker });
      await b.send("ZZZ1");
      expect((await screen.findByRole("alert")).textContent).toBe("Хм, код не подходит. Посмотри внимательнее");
      expect(shownSticker()).toBe("028");
    } finally {
      vi.restoreAllMocks();
    }
  });

  // Ловит: ответ остаётся рядом с новым, ещё не отправленным кодом.
  it("ввод в поле скрывает стикер и надпись", async () => {
    const { user, field, send } = setup();
    await send("nope");
    await screen.findByRole("alert");
    expect(shownSticker()).not.toBeNull();
    await user.type(field, "1");
    expect(screen.queryByRole("alert")).toBeNull();
    expect(shownSticker()).toBeNull();
    expect(field.value).toBe("nope1");
  });

  // Ловит: пустой код уходит службе или показывает стикер неудачи (уточнение 9 плана M5b).
  it("пустой код: «Введи код» без стикера, в программу не уходит", async () => {
    const { user } = setup();
    await user.click(screen.getByRole("button", { name: "Ввести промокод" }));
    expect(screen.getByRole("alert").textContent).toBe("Введи код");
    expect(shownSticker()).toBeNull();
    expect(mock.count("RedeemPromo")).toBe(0);
  });

  // Ловит: отказ без поля «code» (служба не нашла ребёнка) выдан за неверный код.
  it("другой отказ службы: её текст без стикера", async () => {
    const { send } = setup({ myChildId: null });
    await send("AB12");
    expect((await screen.findByRole("alert")).textContent).toBe("Ребёнок не найден");
    expect(shownSticker()).toBeNull();
  });

  // Ловит: сбой связи показан стикером «код не подходит» (ребёнок решит, что код плохой).
  it("нет связи с программой: прежний текст без стикера", async () => {
    const { send } = setup();
    window.go!.ui!.App!.Call = () => Promise.reject(new Error("мост закрыт"));
    await send("AB12");
    expect((await screen.findByRole("alert")).textContent).toBe(CHILD_LINK_ERROR_TEXT);
    expect(shownSticker()).toBeNull();
  });

  // Ловит: стикер не дождался загрузки (рамка пустая) - проигрыватель получает рамку ответа.
  it("рамка стикера 160 x 160 в ответе, картинка скрыта от диктора", async () => {
    const { send } = setup();
    await send("AB12");
    await screen.findByText("Ура! +30 минут");
    await waitFor(() => expect(document.querySelector(".promo-answer .promo-sticker")).not.toBeNull());
    expect(document.querySelector(".promo-sticker")?.getAttribute("aria-hidden")).toBe("true");
  });
});
