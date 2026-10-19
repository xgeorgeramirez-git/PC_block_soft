import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import LoginDialog from "./LoginDialog";
import type { LoginReply } from "../types";
import { DEFAULT_PASSWORD, installBackend, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-14, R19: вход родителя. Ошибки у поля, Enter = «Войти», пауза с отсчётом.

let mock: MockBackend;
let onSuccess: Mock<(reply: LoginReply) => void>;

function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend(opts);
  onSuccess = vi.fn<(reply: LoginReply) => void>();
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<LoginDialog onSuccess={onSuccess} />);
  return { user, input: screen.getByLabelText("Пароль родителя") as HTMLInputElement };
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: new Date(2026, 9, 19, 12, 0, 0) });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

describe("LoginDialog", () => {
  // Ловит: пароль виден на экране.
  it("поле пароля скрывает ввод", () => {
    const { input } = setup();
    expect(input.type).toBe("password");
    expect(screen.getByRole("button", { name: "Войти" })).toBeTruthy();
  });

  // Ловит: Enter в поле не входит (урок L-112, ТЗ 8.0 п.8).
  it("Enter нажимает «Войти»", async () => {
    const { user, input } = setup();
    await user.type(input, `${DEFAULT_PASSWORD}{Enter}`);
    expect(mock.count("Login")).toBe(1);
    expect(onSuccess).toHaveBeenCalledTimes(1);
  });

  it("кнопка «Войти» тоже входит", async () => {
    const { user, input } = setup();
    await user.type(input, DEFAULT_PASSWORD);
    await user.click(screen.getByRole("button", { name: "Войти" }));
    expect(onSuccess).toHaveBeenCalledTimes(1);
  });

  // Ловит: ошибка пароля выводится не у поля или без числа попыток (L-121).
  it("неверный пароль: текст под полем, рамка, поле очищено", async () => {
    const { user, input } = setup();
    await user.type(input, "не тот{Enter}");
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Неверный пароль. Осталось попыток: 4");
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(input.getAttribute("aria-describedby")).toContain(alert.id);
    expect(input.value).toBe("");
    expect(document.activeElement).toBe(input);
    expect(onSuccess).not.toHaveBeenCalled();
  });

  it("число попыток уменьшается с каждой ошибкой", async () => {
    const { user, input } = setup();
    await user.type(input, "a{Enter}");
    await screen.findByText("Неверный пароль. Осталось попыток: 4");
    await user.type(input, "b{Enter}");
    expect(await screen.findByText("Неверный пароль. Осталось попыток: 3")).toBeTruthy();
  });

  it("пустой пароль не отправляется, говорит что делать", async () => {
    const { user, input } = setup();
    await user.click(screen.getByRole("button", { name: "Войти" }));
    expect((await screen.findByRole("alert")).textContent).toBe("Введите пароль");
    expect(mock.count("Login")).toBe(0);
    expect(input.getAttribute("aria-invalid")).toBe("true");
  });

  // Ловит: после серии ошибок можно продолжать подбор, пауза не включилась.
  it("пауза: поле недоступно, отсчёт идёт, потом ввод открывается", async () => {
    const { user, input } = setup({ attempts: 1 });
    await user.type(input, "не тот{Enter}");
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Слишком много попыток. Повторить можно через 1:00",
    );
    expect(input.disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Войти" }) as HTMLButtonElement).disabled).toBe(true);

    await act(async () => {
      vi.advanceTimersByTime(1000);
    });
    expect(screen.getByRole("alert").textContent).toBe("Слишком много попыток. Повторить можно через 0:59");

    await act(async () => {
      vi.advanceTimersByTime(59_000);
    });
    expect(screen.queryByRole("alert")).toBeNull();
    expect(input.disabled).toBe(false);
    expect(document.activeElement).toBe(input);
  });

  it("после паузы верный пароль пускает", async () => {
    const { user, input } = setup({ attempts: 1 });
    await user.type(input, "не тот{Enter}");
    await screen.findByText(/Слишком много попыток/);
    await act(async () => {
      vi.advanceTimersByTime(60_000);
    });
    await user.type(input, `${DEFAULT_PASSWORD}{Enter}`);
    expect(onSuccess).toHaveBeenCalledTimes(1);
  });

  // Ловит: технический текст сбоя связи доходит до родителя (L-112).
  it("нет связи с программой: простыми словами", async () => {
    const { user, input } = setup({ linkDown: true });
    await user.type(input, "что-то{Enter}");
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Программа не отвечает. Попробуйте ещё раз через минуту");
    expect(alert.textContent).not.toMatch(/pipe|ipc/);
    expect(input.disabled).toBe(false);
  });

  // Ловит: экран-тупик (L-119): забыл пароль - а сказано только «неверно».
  it("подсказывает, как сбросить забытый пароль", () => {
    setup();
    expect(screen.getByText(/Сбросить его можно только из учётной записи администратора/)).toBeTruthy();
  });

  it("повторная отправка во время запроса игнорируется", async () => {
    const { user, input } = setup();
    let release: () => void = () => undefined;
    mock.override("Login", () => new Promise((resolve) => (release = () => resolve({ ok: true, payload: { token: "t", expiresMs: 0, idleTimeoutS: 600 } }))), 1);
    await user.type(input, "x{Enter}");
    await user.click(screen.getByRole("button", { name: "Войти" }));
    expect(mock.count("Login")).toBe(1);
    await act(async () => release());
    expect(onSuccess).toHaveBeenCalledTimes(1);
  });
});
