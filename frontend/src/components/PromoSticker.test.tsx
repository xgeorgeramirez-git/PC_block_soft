import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import type { AnimationItem, LoadAnimationParams } from "../lib/lottie";

// Стикер-ответ на промокод (R26, D-83, D-85): проигрыватель svg один раз, после конца проигрывания
// картинка исчезает, надпись остаётся; при выключенных анимациях - последний кадр на время
// анимации (3 с) и тоже исчезает; уничтожение при смене и размонтировании, сбой - только надпись.
// Настоящий проигрыватель и файлы .tgs в jsdom не грузятся: обе загрузки подменены.

const lottie = vi.hoisted(() => {
  const made: {
    params: LoadAnimationParams;
    item: AnimationItem & { calls: string[]; on: Record<string, (() => void) | undefined> };
  }[] = [];
  return {
    made,
    loadLottie: () => Promise.resolve({
      loadAnimation(params: LoadAnimationParams) {
        const calls: string[] = [];
        const on: Record<string, (() => void) | undefined> = {};
        const item = {
          totalFrames: 90,
          calls,
          on,
          getDuration: () => 3,
          play: () => calls.push("play"),
          stop: () => calls.push("stop"),
          goToAndStop: (v: number, isFrame?: boolean) => calls.push(`goToAndStop ${v} ${String(isFrame)}`),
          addEventListener: (name: string, cb: () => void) => {
            calls.push(`on ${name}`);
            on[name] = cb;
          },
          destroy: () => calls.push("destroy"),
        };
        made.push({ params, item });
        return item;
      },
    }),
  };
});

vi.mock("../lib/lottie", () => ({
  loadLottie: () => lottie.loadLottie(),
}));

const data = vi.hoisted(() => ({ fail: false, loaded: [] as string[] }));

vi.mock("../lib/stickerData", () => ({
  loadStickerData: (n: string) => {
    data.loaded.push(n);
    return data.fail ? Promise.reject(new Error("нет файла")) : Promise.resolve({ sticker: n });
  },
}));

import PromoSticker from "./PromoSticker";

function setReducedMotion(on: boolean) {
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    writable: true,
    value: (q: string) => ({ matches: on && q === "(prefers-reduced-motion: reduce)", media: q }),
  });
}

beforeEach(() => {
  lottie.made.length = 0;
  data.loaded.length = 0;
  data.fail = false;
  setReducedMotion(false);
});

afterEach(() => {
  delete (window as { matchMedia?: unknown }).matchMedia;
});

describe("PromoSticker", () => {
  // Ловит: не тот проигрыватель (canvas), повтор по кругу, стикер не в своей рамке, картинка
  // читается диктором; надпись сама объявляет себя живой зоной (появляется вместе с текстом,
  // диктор может не прочитать: зона - постоянная, в PromoRedeem).
  it("удача: svg, один раз, автозапуск; картинка скрыта от диктора, надпись без своей живой зоны", async () => {
    const { container } = render(<PromoSticker sticker="017" text="Ура! +30 минут" ok />);
    await waitFor(() => expect(lottie.made).toHaveLength(1));
    const { params, item } = lottie.made[0];
    const box = container.querySelector(".promo-sticker");
    expect(params.container).toBe(box);
    expect(params.renderer).toBe("svg");
    expect(params.loop).toBe(false);
    expect(params.autoplay).toBe(true);
    expect(params.animationData).toEqual({ sticker: "017" });
    expect(box?.getAttribute("aria-hidden")).toBe("true");
    expect(item.calls).not.toContain("goToAndStop 89 true");
    expect(screen.getByText("Ура! +30 минут").getAttribute("role")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  // Ловит: неудача оформлена как удача (общий класс ответа).
  it("неудача: свой класс ответа, надпись без своей живой зоны", async () => {
    const { container } = render(<PromoSticker sticker="023" text="Ой, мимо! Сверь код с запиской" ok={false} />);
    expect(container.querySelector(".promo-answer-bad")?.textContent).toBe("Ой, мимо! Сверь код с запиской");
    expect(container.querySelector(".promo-answer-ok")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
    await waitFor(() => expect(lottie.made).toHaveLength(1));
  });

  // Ловит: при выключенных анимациях стикер всё равно двигается или стоит на первом (пустом) кадре.
  it("анимации выключены: без автозапуска, последний кадр", async () => {
    setReducedMotion(true);
    render(<PromoSticker sticker="017" text="Ура! +30 минут" ok />);
    await waitFor(() => expect(lottie.made).toHaveLength(1));
    const { params, item } = lottie.made[0];
    expect(params.autoplay).toBe(false);
    expect(item.calls).toContain("goToAndStop 89 true");
  });

  // Ловит: доигранный стикер остаётся на последнем кадре или оставляет пустую рамку 160 x 160;
  // вместе с картинкой пропадает надпись (D-85).
  it("конец проигрывания: картинки нет, анимация уничтожена, надпись осталась", async () => {
    const { container } = render(<PromoSticker sticker="017" text="Ура! +30 минут" ok />);
    await waitFor(() => expect(lottie.made).toHaveLength(1));
    const { item } = lottie.made[0];
    expect(item.calls).toContain("on complete");
    expect(container.querySelector(".promo-sticker")).not.toBeNull();
    act(() => item.on.complete?.());
    expect(container.querySelector(".promo-sticker")).toBeNull();
    expect(item.calls).toContain("destroy");
    expect(screen.getByText("Ура! +30 минут")).toBeTruthy();
  });

  // Ловит: после исчезнувшего стикера следующий ответ остаётся без картинки.
  it("новый стикер после исчезнувшего снова показан", async () => {
    const { container, rerender } = render(<PromoSticker sticker="023" text="Ой, мимо! Сверь код с запиской" ok={false} />);
    await waitFor(() => expect(lottie.made).toHaveLength(1));
    act(() => lottie.made[0].item.on.complete?.());
    expect(container.querySelector(".promo-sticker")).toBeNull();
    rerender(<PromoSticker sticker="028" text="Хм, код не подходит. Посмотри внимательнее" ok={false} />);
    await waitFor(() => expect(lottie.made).toHaveLength(2));
    expect(container.querySelector(".promo-sticker")?.getAttribute("data-sticker")).toBe("028");
    expect(lottie.made[1].params.container).toBe(container.querySelector(".promo-sticker"));
  });

  // Ловит: при выключенных анимациях стикер стоит вечно (события конца нет) или пропадает сразу.
  it("анимации выключены: стоит 3 с (длина анимации) и исчезает, надпись осталась", async () => {
    setReducedMotion(true);
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const { container } = render(<PromoSticker sticker="017" text="Ура! +30 минут" ok />);
      await waitFor(() => expect(lottie.made).toHaveLength(1));
      expect(lottie.made[0].item.calls).not.toContain("on complete");
      act(() => {
        vi.advanceTimersByTime(2900);
      });
      expect(container.querySelector(".promo-sticker")).not.toBeNull();
      act(() => {
        vi.advanceTimersByTime(200);
      });
      expect(container.querySelector(".promo-sticker")).toBeNull();
      expect(lottie.made[0].item.calls).toContain("destroy");
      expect(screen.getByText("Ура! +30 минут")).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });

  // Ловит: анимация живёт после закрытия ответа (утечка таймеров проигрывателя).
  it("размонтирование уничтожает анимацию", async () => {
    const { unmount } = render(<PromoSticker sticker="017" text="Ура! +30 минут" ok />);
    await waitFor(() => expect(lottie.made).toHaveLength(1));
    unmount();
    expect(lottie.made[0].item.calls).toContain("destroy");
  });

  // Ловит: при смене стикера старая анимация остаётся рядом с новой.
  it("смена стикера: старая уничтожена, новая загружена", async () => {
    const { rerender } = render(<PromoSticker sticker="023" text="Такого кода нет. Проверь буквы и цифры" ok={false} />);
    await waitFor(() => expect(lottie.made).toHaveLength(1));
    rerender(<PromoSticker sticker="028" text="Хм, код не подходит. Посмотри внимательнее" ok={false} />);
    await waitFor(() => expect(lottie.made).toHaveLength(2));
    expect(lottie.made[0].item.calls).toContain("destroy");
    expect(lottie.made[1].item.calls).not.toContain("destroy");
    expect(data.loaded).toEqual(["023", "028"]);
  });

  // Ловит: сбой загрузки оставляет пустую рамку 160 x 160 или прячет надпись.
  it("стикер не загрузился: только надпись", async () => {
    data.fail = true;
    const { container } = render(<PromoSticker sticker="110" text="Код не сработал. Может, его уже использовали?" ok={false} />);
    await waitFor(() => expect(container.querySelector(".promo-sticker")).toBeNull());
    expect(screen.getByText("Код не сработал. Может, его уже использовали?")).toBeTruthy();
    expect(lottie.made).toHaveLength(0);
  });
});
