import { afterEach, describe, expect, it } from "vitest";
import { loadLottie, resetLottieForTest, type LottiePlayer } from "./lottie";

// Обёртка проигрывателя стикеров (RM5b-8, уточнение 13 плана M5b): UMD-файл грузится
// тегом <script> по адресу из сборки и кладёт проигрыватель в window.lottie.
// Настоящий проигрыватель в тестах не нужен: «загрузку» изображает событие load.

const fakePlayer: LottiePlayer = {
  loadAnimation: () => {
    throw new Error("не вызывается");
  },
};

function playerScripts(): HTMLScriptElement[] {
  return Array.from(document.head.querySelectorAll<HTMLScriptElement>("script[data-lottie]"));
}

afterEach(() => {
  resetLottieForTest();
  delete (window as { lottie?: LottiePlayer }).lottie;
  playerScripts().forEach((s) => s.remove());
});

describe("loadLottie", () => {
  // Ловит: скрипт не добавляется или добавляется с другим адресом, ответ не ждёт загрузки.
  it("добавляет один скрипт и отдаёт window.lottie после load", async () => {
    const p = loadLottie("/assets/lottie_light.min-abc.js");
    const scripts = playerScripts();
    expect(scripts).toHaveLength(1);
    expect(scripts[0].getAttribute("src")).toBe("/assets/lottie_light.min-abc.js");
    (window as { lottie?: LottiePlayer }).lottie = fakePlayer;
    scripts[0].dispatchEvent(new Event("load"));
    await expect(p).resolves.toBe(fakePlayer);
  });

  // Ловит: каждый стикер добавляет новый тег <script> (проигрыватель грузится заново).
  it("второй вызов не добавляет скрипт и отдаёт тот же проигрыватель", async () => {
    const p1 = loadLottie("/a.js");
    const p2 = loadLottie("/a.js");
    expect(playerScripts()).toHaveLength(1);
    (window as { lottie?: LottiePlayer }).lottie = fakePlayer;
    playerScripts()[0].dispatchEvent(new Event("load"));
    expect(await p1).toBe(fakePlayer);
    expect(await p2).toBe(fakePlayer);
  });

  // Ловит: сбой загрузки запоминается навсегда (стикеров больше не будет до перезапуска окна)
  // или обещание висит без ответа.
  it("сбой загрузки: ошибка, следующий вызов пробует снова", async () => {
    const p1 = loadLottie("/a.js");
    playerScripts()[0].dispatchEvent(new Event("error"));
    await expect(p1).rejects.toThrow("lottie: не загрузился /a.js");
    expect(playerScripts()).toHaveLength(0);
    const p2 = loadLottie("/a.js");
    expect(playerScripts()).toHaveLength(1);
    (window as { lottie?: LottiePlayer }).lottie = fakePlayer;
    playerScripts()[0].dispatchEvent(new Event("load"));
    await expect(p2).resolves.toBe(fakePlayer);
  });

  // Ловит: скрипт загрузился, но проигрывателя нет (не тот файл): обещание отдаёт undefined.
  it("загрузился без window.lottie: ошибка", async () => {
    const p = loadLottie("/a.js");
    playerScripts()[0].dispatchEvent(new Event("load"));
    await expect(p).rejects.toThrow("lottie: нет window.lottie после /a.js");
  });

  // Ловит: проигрыватель уже есть (страница загрузила его раньше), а обёртка всё равно ждёт скрипт.
  it("проигрыватель уже на странице: без скрипта", async () => {
    (window as { lottie?: LottiePlayer }).lottie = fakePlayer;
    await expect(loadLottie("/a.js")).resolves.toBe(fakePlayer);
    expect(playerScripts()).toHaveLength(0);
  });
});
