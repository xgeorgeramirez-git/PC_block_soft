import { afterEach, describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ResetPassword from "./ResetPassword";
import { installBackend, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-26, R20, сценарий 8.3.8 п.2: администратор задаёт новый пароль дважды, без старого.

let mock: MockBackend;

function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({ mode: "reset-password", ...opts });
  render(<ResetPassword />);
  return userEvent.setup();
}

const fresh = () => screen.getByLabelText("Новый пароль") as HTMLInputElement;
const again = () => screen.getByLabelText("Повторите пароль") as HTMLInputElement;
const submit = () => screen.getByRole("button", { name: "Задать новый пароль" });

afterEach(() => {
  uninstallBackend();
});

describe("ResetPassword", () => {
  // Ловит: просят старый пароль (его как раз забыли) или показывают пароль на экране.
  it("два поля нового пароля, старого пароля нет", () => {
    setup();
    expect(fresh().type).toBe("password");
    expect(again().type).toBe("password");
    expect(screen.queryByLabelText(/Старый/)).toBeNull();
    expect(screen.getByRole("heading", { name: "Сброс пароля родителя" })).toBeTruthy();
  });

  it("короткий пароль: ошибка у поля, команда не уходит", async () => {
    const user = setup();
    await user.type(fresh(), "123");
    await user.type(again(), "123");
    await user.click(submit());
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Не меньше 6 символов");
    expect(fresh().getAttribute("aria-describedby")).toContain(alert.id);
    expect(mock.count("ResetPassword")).toBe(0);
  });

  it("разные пароли: «Пароли не совпадают» у второго поля", async () => {
    const user = setup();
    await user.type(fresh(), "новый-секрет");
    await user.type(again(), "новый-секрет!");
    await user.click(submit());
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Пароли не совпадают");
    expect(again().getAttribute("aria-describedby")).toContain(alert.id);
  });

  it("успех: «Пароль изменён», форма заменена, новый пароль действует", async () => {
    const user = setup();
    await user.type(fresh(), "новый-секрет");
    await user.type(again(), "новый-секрет{Enter}");
    expect(await screen.findByRole("heading", { name: "Пароль изменён" })).toBeTruthy();
    expect(screen.queryByLabelText("Новый пароль")).toBeNull();
    expect(mock.calls.find((c) => c.command === "ResetPassword")?.payload).toEqual({ newPassword: "новый-секрет" });
    expect(mock.state.password).toBe("новый-секрет");
    expect(screen.getByText(/Это окно можно закрыть/)).toBeTruthy();
  });

  it("ошибка службы по паролю - у поля", async () => {
    const user = setup();
    mock.override("ResetPassword", { ok: false, error: "Не меньше 6 символов", field: "newPassword" }, 1);
    await user.type(fresh(), "новый-секрет");
    await user.type(again(), "новый-секрет");
    await user.click(submit());
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Не меньше 6 символов");
    expect(fresh().getAttribute("aria-describedby")).toContain(alert.id);
  });

  // Ловит: нет прав (команду отклонила служба) - и родитель видит технический текст.
  it("отказ службы без поля: простыми словами", async () => {
    const user = setup();
    mock.override("ResetPassword", { ok: false, error: "Сбросить пароль можно только из учётной записи администратора" }, 1);
    await user.type(fresh(), "новый-секрет");
    await user.type(again(), "новый-секрет");
    await user.click(submit());
    expect(await screen.findByText("Сбросить пароль можно только из учётной записи администратора")).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Пароль изменён" })).toBeNull();
  });

  it("нет связи: простыми словами, поля остаются", async () => {
    const user = setup({ linkDown: true });
    await user.type(fresh(), "новый-секрет");
    await user.type(again(), "новый-секрет");
    await user.click(submit());
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    expect(fresh().value).toBe("новый-секрет");
  });
});
