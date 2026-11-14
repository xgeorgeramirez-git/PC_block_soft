import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import type { AnimationItem, LoadAnimationParams } from "../lib/lottie";

// Стикер-ответ на промокод (R26, D-83, план M5b T8): проигрыватель svg один раз, при выключенных
// анимациях - последний кадр, уничтожение при смене и размонтировании, сбой - только надпись.
// Настоящий проигрыватель и файлы .tgs в jsdom не грузятся: обе загрузки подменены.

const lottie = vi.hoisted(() => {
  const made: { params: LoadAnimationParams; item: AnimationItem & { calls: string[] } }[] = [];
  return {
    made,
    loadLottie: () => Promise.resolve({
      loadAnimation(params: LoadAnimationParams) {
        const calls: string[] = [];
        const item = {
          totalFrames: 90,
          calls,
          play: () => calls.push("play"),
          stop: () => calls.push("stop"),
          goToAndStop: (v: number, isFrame?: boolean) => calls.push(`goToAndStop ${v} ${String(isFrame)}`),
          addEventListener: (name: string) => calls.push(`on ${name}`),
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
  // читается диктором, надпись удачи без role="status".
  it("удача: svg, один раз, автозапуск; картинка скрыта от диктора, надпись - статус", async () => {
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
    expect(screen.getByRole("status").textContent).toBe("Ура! +30 минут");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  // Ловит: неудача объявляется вежливым статусом (диктор может промолчать).
  it("неудача: надпись - предупреждение", async () => {
    render(<PromoSticker sticker="023" text="Ой, мимо! Сверь код с запиской" ok={false} />);
    expect(screen.getByRole("alert").textContent).toBe("Ой, мимо! Сверь код с запиской");
    expect(screen.queryByRole("status")).toBeNull();
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
    expect(screen.getByRole("alert").textContent).toBe("Код не сработал. Может, его уже использовали?");
    expect(lottie.made).toHaveLength(0);
  });
});
