import { describe, expect, it } from "vitest";
import { humanNotice, RULES_NOT_READ_TEXT } from "./notices";

// Важные сообщения службы при входе (Ruling 25, R18) показываются родителю человеческими словами (NFR-11, L-112).

describe("humanNotice", () => {
  // Ловит: в окне родителя «база данных» и «служба» (технические слова), как в ответе службы.
  it("сообщение об испорченной базе заменяется человеческим и ведёт в «Дети»", () => {
    const raw = "База данных была повреждена и восстановлена. Ограничения не действовали, пока служба не смогла её прочитать";
    const n = humanNotice(raw);
    expect(n.text).toBe(RULES_NOT_READ_TEXT);
    expect(n.action).toBe("children");
  });

  it("текст по смыслу R18: не прочитали, ограничений сейчас нет, данные целы, что делать", () => {
    expect(RULES_NOT_READ_TEXT).toMatch(/не удалось прочитать правила/i);
    expect(RULES_NOT_READ_TEXT).toMatch(/ограничения (сейчас )?не действуют/i);
    expect(RULES_NOT_READ_TEXT).toMatch(/данные сохранены/i);
    expect(RULES_NOT_READ_TEXT).toMatch(/в разделе «Дети»/);
  });

  it("в тексте для родителя нет технических слов и длинных тире", () => {
    expect(RULES_NOT_READ_TEXT).not.toMatch(/баз[аы] данных|служб|sqlite|ошибк/i);
    expect(RULES_NOT_READ_TEXT).not.toMatch(/[–—]/);
  });

  it("узнаёт сообщение и по другому порядку слов службы", () => {
    expect(humanNotice("Не удалось открыть базу данных").action).toBe("children");
    expect(humanNotice("Служба не смогла прочитать правила").text).toBe(RULES_NOT_READ_TEXT);
  });

  it("уже человеческий текст службы показывается как есть, без кнопки", () => {
    const n = humanNotice("  Установлено обновление  ");
    expect(n).toEqual({ text: "Установлено обновление" });
  });
});
