import { describe, expect, it } from "vitest";
import {
  FAIL,
  FAIL_TEXTS,
  minutesPhrase,
  pickNext,
  StickerPicker,
  SUCCESS,
  SUCCESS_TEXTS,
  successText,
} from "./stickers";

// Ответ ребёнку на промокод (R26, D-83): наборы и надписи дословно из ТЗ, верная форма слова
// «минута», неповтор прошлого стикера и прошлой надписи отдельно для удачи и неудачи.

/** Источник «случайности» по списку значений, по кругу. */
const seq = (...xs: number[]) => {
  let i = 0;
  return () => xs[i++ % xs.length];
};

describe("наборы R26", () => {
  // Ловит: опечатка в номере стикера или надписи, лишний или потерянный пункт.
  it("стикеры и надписи дословно", () => {
    expect(SUCCESS).toEqual(["017", "033", "044", "049", "050", "077", "093", "096", "112"]);
    expect(FAIL).toEqual(["023", "028", "048", "069", "082", "084", "095", "110"]);
    expect(SUCCESS_TEXTS.map((_, i) => successText(i, 30))).toEqual([
      "Ура! +30 минут",
      "Есть! Тебе +30 минут",
      "Код сработал! +30 минут",
      "Отлично, лови +30 минут!",
      "Вот это да! +30 минут",
      "Супер! Твои +30 минут",
    ]);
    expect(FAIL_TEXTS).toEqual([
      "Такого кода нет. Проверь буквы и цифры",
      "Хм, код не подходит. Посмотри внимательнее",
      "Не получилось. Проверь, нет ли опечатки",
      "Этот код не работает. Попробуй ещё раз",
      "Ой, мимо! Сверь код с запиской",
      "Код не сработал. Может, его уже использовали?",
    ]);
  });

  // Ловит: длинное тире в тексте ребёнку (ТЗ 8.0, скан текстов).
  it("без длинных тире", () => {
    for (const t of [...SUCCESS_TEXTS, ...FAIL_TEXTS]) expect(t).not.toMatch(/[—–]/);
  });
});

describe("minutesPhrase: форма слова «минута»", () => {
  // Ловит: «+1 минут», «+2 минут», «+11 минута», «+21 минут», «+111 минута».
  it.each<[number, string]>([
    [1, "1 минута"],
    [2, "2 минуты"],
    [3, "3 минуты"],
    [4, "4 минуты"],
    [5, "5 минут"],
    [11, "11 минут"],
    [12, "12 минут"],
    [14, "14 минут"],
    [21, "21 минута"],
    [22, "22 минуты"],
    [30, "30 минут"],
    [101, "101 минута"],
    [111, "111 минут"],
    [112, "112 минут"],
    [600, "600 минут"],
  ])("%i → %s", (n, want) => {
    expect(minutesPhrase(n)).toBe(want);
  });

  it("successText подставляет минуты в нужной форме", () => {
    expect(successText(0, 1)).toBe("Ура! +1 минута");
    expect(successText(3, 3)).toBe("Отлично, лови +3 минуты!");
    expect(successText(5, 5)).toBe("Супер! Твои +5 минут");
  });
});

describe("pickNext", () => {
  // Ловит: выбор повторяет прошлый элемент.
  it("никогда не равен прошлому", () => {
    for (const r of [0, 0.2, 0.5, 0.99, 0.999999]) {
      expect(pickNext(["a", "b", "c"], "a", () => r)).not.toBe("a");
      expect(pickNext(["a", "b", "c"], "c", () => r)).not.toBe("c");
    }
  });

  // Ловит: выход за край списка при rnd, близком к 1; не тот элемент по rnd.
  it("берёт элемент по rnd среди оставшихся", () => {
    expect(pickNext(["a", "b", "c"], "a", () => 0)).toBe("b");
    expect(pickNext(["a", "b", "c"], "a", () => 0.999999)).toBe("c");
    expect(pickNext(["a", "b", "c"], undefined, () => 0.5)).toBe("b");
    expect(pickNext(["a", "b", "c"], undefined, () => 1)).toBe("c");
  });

  // Ловит: зацикливание или undefined на списке из одного.
  it("список из одного: тот же элемент", () => {
    expect(pickNext(["a"], "a", () => 0.5)).toBe("a");
  });
});

describe("StickerPicker", () => {
  // Ловит: стикер удачи из набора неудачи и наоборот; надпись не с минутами кода.
  it("удача и неудача из своих наборов", () => {
    const p = new StickerPicker(seq(0));
    expect(p.success(1)).toEqual({ sticker: "017", text: "Ура! +1 минута", ok: true });
    expect(p.fail()).toEqual({ sticker: "023", text: "Такого кода нет. Проверь буквы и цифры", ok: false });
  });

  // Ловит: два неверных кода подряд показывают тот же стикер или ту же надпись (8.2.6 п.11).
  it("два неверных подряд: другие стикер и надпись", () => {
    const p = new StickerPicker(seq(0));
    const a = p.fail();
    const b = p.fail();
    expect(b.sticker).not.toBe(a.sticker);
    expect(b.text).not.toBe(a.text);
    expect(b).toEqual({ sticker: "028", text: "Хм, код не подходит. Посмотри внимательнее", ok: false });
  });

  // Ловит: память общая на удачу и неудачу (верный код между двумя неверными «сбрасывает» неповтор)
  // или удача повторяет прошлую удачу.
  it("прошлое помнится отдельно для удачи и неудачи", () => {
    const p = new StickerPicker(seq(0));
    const bad1 = p.fail();
    const ok1 = p.success(30);
    const bad2 = p.fail();
    const ok2 = p.success(30);
    expect(bad2.sticker).not.toBe(bad1.sticker);
    expect(bad2.text).not.toBe(bad1.text);
    expect(ok2.sticker).not.toBe(ok1.sticker);
    expect(ok2.text).not.toBe(ok1.text);
    expect(ok1).toEqual({ sticker: "017", text: "Ура! +30 минут", ok: true });
    expect(ok2).toEqual({ sticker: "033", text: "Есть! Тебе +30 минут", ok: true });
  });

  // Ловит: неповтор только с соседним вызовом при любой «случайности» (длинная серия).
  it("серия из 50 ответов: соседние одного вида не совпадают", () => {
    const p = new StickerPicker(seq(0.1, 0.9, 0.4, 0.7, 0.0, 0.99));
    let prev = p.fail();
    for (let i = 0; i < 50; i++) {
      const next = p.fail();
      expect(next.sticker).not.toBe(prev.sticker);
      expect(next.text).not.toBe(prev.text);
      expect(FAIL).toContain(next.sticker);
      prev = next;
    }
  });

  it("reset забывает прошлое", () => {
    const p = new StickerPicker(seq(0));
    p.success(5);
    p.reset();
    expect(p.success(5).sticker).toBe("017");
  });
});
