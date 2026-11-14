import { gzipSync } from "node:zlib";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FAIL, SUCCESS } from "./stickers";
import { loadStickerData, resetStickerDataForTest, STICKER_URLS } from "./stickerData";

// Файлы стикеров промокода (RM5b-8): ровно 17 из набора R26, .tgs (gzip) распаковывается в JSON.

afterEach(() => {
  resetStickerDataForTest();
});

const tgs = (obj: unknown) => new Response(new Uint8Array(gzipSync(Buffer.from(JSON.stringify(obj), "utf8"))));

describe("STICKER_URLS", () => {
  // Ловит: стикер набора без файла в сборке (ребёнок видит только надпись) или лишний файл.
  it("ровно стикеры удачи и неудачи, у каждого свой адрес .tgs", () => {
    expect(Object.keys(STICKER_URLS).sort()).toEqual([...SUCCESS, ...FAIL].sort());
    for (const [n, url] of Object.entries(STICKER_URLS)) {
      expect(url).toMatch(new RegExp(`${n}[^/]*\\.tgs`));
    }
    expect(new Set(Object.values(STICKER_URLS)).size).toBe(17);
  });
});

describe("loadStickerData", () => {
  // Ловит: .tgs отдаётся проигрывателю без распаковки (gzip вместо JSON).
  it.runIf(typeof DecompressionStream === "function")("распаковывает gzip в данные анимации", async () => {
    const fetchFn = vi.fn((url: string) => {
      void url;
      return Promise.resolve(tgs({ v: "5.5.2", op: 180, name: "кот" }));
    });
    await expect(loadStickerData("017", fetchFn)).resolves.toEqual({ v: "5.5.2", op: 180, name: "кот" });
    expect(fetchFn).toHaveBeenCalledWith(STICKER_URLS["017"]);
  });

  // Ловит: файл, уже распакованный сервером (Content-Encoding), снова отдаётся распаковщику и ломается.
  it("уже распакованный JSON читается как есть", async () => {
    const fetchFn = vi.fn(() => Promise.resolve(new Response(JSON.stringify({ op: 60 }))));
    await expect(loadStickerData("044", fetchFn)).resolves.toEqual({ op: 60 });
  });

  // Ловит: стикер скачивается заново при каждом показе.
  it.runIf(typeof DecompressionStream === "function")("второй показ того же стикера без загрузки", async () => {
    const fetchFn = vi.fn(() => Promise.resolve(tgs({ op: 1 })));
    await loadStickerData("023", fetchFn);
    await loadStickerData("023", fetchFn);
    expect(fetchFn).toHaveBeenCalledTimes(1);
  });

  // Ловит: сбой запоминается навсегда; номер вне набора уходит в сеть.
  it("сбой загрузки: ошибка, следующий показ пробует снова; чужой номер - ошибка без загрузки", async () => {
    const fetchFn = vi.fn(() => Promise.resolve(new Response("нет", { status: 404 })));
    await expect(loadStickerData("028", fetchFn)).rejects.toThrow("стикер 028: ответ 404");
    await expect(loadStickerData("028", fetchFn)).rejects.toThrow("стикер 028: ответ 404");
    expect(fetchFn).toHaveBeenCalledTimes(2);
    await expect(loadStickerData("001", fetchFn)).rejects.toThrow("стикер 001: нет в наборе");
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });
});
