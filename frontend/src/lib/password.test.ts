import { describe, expect, it } from "vitest";
import { MIN_PASSWORD, validatePasswordPair, validateNewPassword } from "./password";

describe("пароль родителя (R19)", () => {
  it("минимум 6 символов", () => {
    expect(MIN_PASSWORD).toBe(6);
    expect(validateNewPassword("12345")).toBe("Не меньше 6 символов");
    expect(validateNewPassword("123456")).toBeNull();
    expect(validateNewPassword("")).toBe("Не меньше 6 символов");
  });
  // Ловит: длину считают в байтах, и кириллица из 3 букв проходит как 6 символов.
  it("длина считается в символах, не в байтах", () => {
    expect(validateNewPassword("абв")).toBe("Не меньше 6 символов");
    expect(validateNewPassword("абвгде")).toBeNull();
  });
  it("пара: ошибка длины у первого поля, несовпадение у второго", () => {
    expect(validatePasswordPair("123", "123")).toEqual({ password: "Не меньше 6 символов" });
    expect(validatePasswordPair("123456", "1234567")).toEqual({ again: "Пароли не совпадают" });
    expect(validatePasswordPair("123456", "123456")).toEqual({});
  });
  it("короткий и несовпадающий: показываем сначала длину", () => {
    expect(validatePasswordPair("123", "321")).toEqual({ password: "Не меньше 6 символов" });
  });
});
