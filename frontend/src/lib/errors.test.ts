import { describe, expect, it } from "vitest";
import {
  CallError,
  GENERIC_ERROR_TEXT,
  LINK_ERROR_TEXT,
  errorField,
  errorText,
  isTechnical,
  linkError,
  replyError,
} from "./errors";

// Слой L-112: человек не видит технических текстов.
describe("isTechnical", () => {
  const technical = [
    "",
    "   ",
    "ipc: кадр нулевой длины",
    "rpc error: code = Unavailable",
    "runtime error: invalid memory address or nil pointer dereference",
    "TypeError: Cannot read properties of undefined",
    "Failed to fetch",
    "open \\\\.\\pipe\\PCBlock: The system cannot find the file specified",
    "panic: boom",
    "HRESULT 0x80070005",
    "window.go is undefined",
    "{\"ok\":false}",
    "at main.go:42",
    "unexpected EOF",
  ];
  it.each(technical)("технический: %j", (text) => {
    expect(isTechnical(text)).toBe(true);
  });

  const human = [
    "Неверный пароль. Осталось попыток: 3",
    "Не меньше 6 символов",
    "Можно добавить от 1 до 240 минут",
    "Не больше 24 ч",
    "Правила для Brave не удалось применить. Попробуйте ещё раз",
    "Ограничения для Миши перестанут действовать",
  ];
  it.each(human)("человеческий: %s", (text) => {
    expect(isTechnical(text)).toBe(false);
  });
});

describe("CallError и тексты", () => {
  it("ошибка связи говорит простыми словами", () => {
    const e = linkError();
    expect(e).toBeInstanceOf(CallError);
    expect(e.link).toBe(true);
    expect(e.message).toBe("Программа не отвечает. Попробуйте ещё раз через минуту");
    expect(LINK_ERROR_TEXT).toBe(e.message);
  });

  it("отказ программы: берём готовый текст и поле", () => {
    const e = replyError({ ok: false, error: "Не больше 24 ч", field: "limits.3" });
    expect(e.message).toBe("Не больше 24 ч");
    expect(e.field).toBe("limits.3");
    expect(e.link).toBe(false);
  });

  it("у отказа сохраняется нагрузка (попытки пароля)", () => {
    const e = replyError({ ok: false, error: "Неверный пароль", field: "password", payload: { attemptsLeft: 2, pauseS: 0 } });
    expect(e.payload).toEqual({ attemptsLeft: 2, pauseS: 0 });
  });

  // Ловит: технический текст отказа доходит до человека.
  it("технический текст отказа заменяется общим", () => {
    const e = replyError({ ok: false, error: "ipc: кадр оборван" });
    expect(e.message).toBe(GENERIC_ERROR_TEXT);
  });

  it("пустой текст отказа заменяется общим", () => {
    expect(replyError({ ok: false }).message).toBe(GENERIC_ERROR_TEXT);
  });

  it("errorText: любая ошибка превращается в читаемую строку", () => {
    expect(errorText(linkError())).toBe(LINK_ERROR_TEXT);
    expect(errorText(replyError({ ok: false, error: "Не меньше 6 символов", field: "password" }))).toBe("Не меньше 6 символов");
    expect(errorText(new Error("window.go is undefined"))).toBe(LINK_ERROR_TEXT);
    expect(errorText("boom")).toBe(LINK_ERROR_TEXT);
    expect(errorText(undefined)).toBe(LINK_ERROR_TEXT);
  });

  it("errorField: поле берётся только у отказа программы", () => {
    expect(errorField(replyError({ ok: false, error: "Не меньше 6 символов", field: "password" }))).toBe("password");
    expect(errorField(linkError())).toBeUndefined();
    expect(errorField(new Error("x"))).toBeUndefined();
  });
});
