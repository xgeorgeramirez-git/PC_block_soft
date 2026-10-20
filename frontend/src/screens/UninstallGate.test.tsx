import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import UninstallGate, { type UninstallDecision } from "./UninstallGate";
import { DEFAULT_PASSWORD, installBackend, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-4: удаление требует пароль родителя; забыл пароль - отдельное подтверждение администратора;
// не настроена - без пароля. Галочка «Удалить статистику».

let mock: MockBackend;
let onDecision: Mock<(decision: UninstallDecision) => void>;

function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({ mode: "uninstall-gate", ...opts });
  onDecision = vi.fn<(decision: UninstallDecision) => void>();
  render(<UninstallGate onDecision={onDecision} />);
  return userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
}

const password = () => screen.findByLabelText("Пароль родителя") as Promise<HTMLInputElement>;

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: new Date(2026, 9, 19, 12, 0, 0) });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

describe("UninstallGate: программа настроена", () => {
  it("просит пароль родителя и показывает галочку «Удалить статистику» (по умолчанию не отмечена)", async () => {
    setup();
    expect(((await password()) as HTMLInputElement).type).toBe("password");
    const box = screen.getByRole("checkbox", { name: "Удалить статистику" }) as HTMLInputElement;
    expect(box.checked).toBe(false);
    expect(screen.getByText(/Чтобы отказаться от удаления, закройте это окно/)).toBeTruthy();
  });

  it("верный пароль: удаление разрешено, статистика остаётся", async () => {
    const user = setup();
    await user.type(await password(), `${DEFAULT_PASSWORD}{Enter}`);
    expect(await screen.findByText(/Пароль верный/)).toBeTruthy();
    expect(onDecision).toHaveBeenCalledWith({ allowed: true, deleteData: false });
    expect(mock.calls.find((c) => c.command === "UninstallCheck")?.payload).toEqual({ password: DEFAULT_PASSWORD, forgot: false });
  });

  // Ловит: галочка «Удалить статистику» не доходит до решения (база удалится не так, как просили).
  it("с галочкой «Удалить статистику» решение несёт признак", async () => {
    const user = setup();
    await user.click(await screen.findByRole("checkbox", { name: "Удалить статистику" }));
    await user.type(await password(), `${DEFAULT_PASSWORD}{Enter}`);
    await screen.findByText(/Пароль верный/);
    expect(onDecision).toHaveBeenCalledWith({ allowed: true, deleteData: true });
  });

  // Ловит: неверный пароль пропускает удаление или молчит.
  it("неверный пароль: ошибка у поля с числом попыток, решения нет", async () => {
    const user = setup();
    const input = await password();
    await user.type(input, "не тот{Enter}");
    expect((await screen.findByRole("alert")).textContent).toBe("Неверный пароль. Осталось попыток: 4");
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(onDecision).not.toHaveBeenCalled();
  });

  it("пустой пароль не отправляется", async () => {
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Продолжить удаление" }));
    expect((await screen.findByRole("alert")).textContent).toBe("Введите пароль");
    expect(mock.count("UninstallCheck")).toBe(0);
  });

  it("серия ошибок: пауза с отсчётом, поле недоступно", async () => {
    const user = setup({ attempts: 1 });
    const input = await password();
    await user.type(input, "не тот{Enter}");
    expect((await screen.findByRole("alert")).textContent).toBe("Слишком много попыток. Повторить можно через 1:00");
    expect(input.disabled).toBe(true);
    await act(async () => {
      vi.advanceTimersByTime(60_000);
    });
    expect(input.disabled).toBe(false);
  });

  describe("забыли пароль", () => {
    it("отдельное подтверждение администратора без пароля", async () => {
      const user = setup();
      await user.click(await screen.findByRole("button", { name: "Не помню пароль" }));
      expect(screen.queryByLabelText("Пароль родителя")).toBeNull();
      expect(screen.getByText(/только из учётной записи администратора/)).toBeTruthy();
      await user.click(screen.getByRole("button", { name: "Подтвердить удаление без пароля" }));
      expect(await screen.findByText(/Удаление разрешено/)).toBeTruthy();
      expect(mock.calls.find((c) => c.command === "UninstallCheck")?.payload).toEqual({ password: "", forgot: true });
      expect(onDecision).toHaveBeenCalledWith({ allowed: true, deleteData: false });
    });

    it("можно вернуться к вводу пароля", async () => {
      const user = setup();
      await user.click(await screen.findByRole("button", { name: "Не помню пароль" }));
      await user.click(screen.getByRole("button", { name: "Вернуться к паролю" }));
      expect(await screen.findByLabelText("Пароль родителя")).toBeTruthy();
    });

    it("служба не подтвердила (не администратор): простыми словами, решения нет", async () => {
      const user = setup();
      mock.override("UninstallCheck", { ok: false, error: "Это можно сделать только из учётной записи администратора" }, 1);
      await user.click(await screen.findByRole("button", { name: "Не помню пароль" }));
      await user.click(screen.getByRole("button", { name: "Подтвердить удаление без пароля" }));
      expect(await screen.findByText("Это можно сделать только из учётной записи администратора")).toBeTruthy();
      expect(onDecision).not.toHaveBeenCalled();
    });
  });
});

describe("UninstallGate: программа не настроена", () => {
  // Ловит: пароль просят у программы, у которой его ещё нет (тупик).
  it("пароль не нужен", async () => {
    const user = setup({ configured: false });
    expect(await screen.findByText(/пароль не нужен/)).toBeTruthy();
    expect(screen.queryByLabelText("Пароль родителя")).toBeNull();
    expect(screen.queryByRole("button", { name: "Не помню пароль" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Продолжить удаление" }));
    await screen.findByText(/Удаление разрешено/);
    expect(mock.calls.find((c) => c.command === "UninstallCheck")?.payload).toEqual({ password: "", forgot: false });
    expect(onDecision).toHaveBeenCalledWith({ allowed: true, deleteData: false });
  });
});

describe("UninstallGate: сбои", () => {
  it("не удалось узнать состояние программы: простыми словами и «Повторить»", async () => {
    mock = installBackend({ mode: "uninstall-gate" });
    mock.override("Hello", "reject", 1);
    render(<UninstallGate onDecision={vi.fn()} />);
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByLabelText("Пароль родителя")).toBeTruthy();
  });
});
