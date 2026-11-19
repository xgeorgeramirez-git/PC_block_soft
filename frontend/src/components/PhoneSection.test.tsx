import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import PhoneSection from "./PhoneSection";
import {
  MOCK_COMPARE_AFTER_MS,
  MOCK_CONNECT_AFTER_MS,
  REMOTE_URL_ERROR,
  installBackend,
  uninstallBackend,
  type MockBackend,
} from "../__mocks__/backend";
import { REMOTE_DEFAULT_URL } from "../lib/remote";
import { formatTimeOfDay, localOffsetMin } from "../lib/format";
import { LINK_ERROR_TEXT } from "../lib/errors";
import type { RemoteView } from "../types";

// M7, T5: блок «Телефон» в «Настройках» (UX-сценарий плана M7, шаги 1-5, 8).

const NOW = new Date(2026, 10, 18, 21, 40, 0);
let mock: MockBackend;

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
});

afterEach(() => {
  uninstallBackend();
  vi.useRealTimers();
});

const online: Partial<RemoteView> = { enabled: true, state: "online", lastOkMs: NOW.getTime() };

async function setup(remote?: Partial<RemoteView>) {
  mock = installBackend({ sessionActive: true, remote });
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  const view = render(<PhoneSection />);
  await screen.findByRole("switch", { name: "Связь с телефоном" });
  await vi.waitFor(() => expect((screen.getByRole("switch", { name: "Связь с телефоном" }) as HTMLInputElement).disabled).toBe(false));
  return { user, view };
}

const box = () => screen.getByRole("region", { name: "Телефон" });
const toggle = () => screen.getByRole("switch", { name: "Связь с телефоном" }) as HTMLInputElement;
const status = () => box().querySelector(".phone-message") as HTMLElement;
const tick = (ms: number) => act(() => vi.advanceTimersByTimeAsync(ms));

describe("PhoneSection: выключено", () => {
  it("выключатель выключен, пояснение про ваш сервер, ни адреса, ни подключения", async () => {
    await setup();
    expect(toggle().checked).toBe(false);
    expect(within(box()).getByText("Программа будет связываться только с вашим сервером. Выключено: программа совсем без интернета")).toBeTruthy();
    expect(screen.queryByLabelText("Адрес сервера")).toBeNull();
    expect(screen.queryByRole("button", { name: /Подключить/ })).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  // PL-44: живая область статуса есть в DOM до первого действия, пустая.
  it("постоянная область статуса пустая до действия", async () => {
    await setup();
    expect(status().getAttribute("role")).toBe("status");
    expect(status().textContent).toBe("");
  });
});

describe("PhoneSection: включение и адрес", () => {
  it("включение: адрес по умолчанию, «Подключаемся…», затем «На связи»; фокус снова на выключателе (PL-68)", async () => {
    const { user } = await setup();
    await user.click(toggle());
    await vi.waitFor(() => expect(toggle().checked).toBe(true));
    expect(mock.calls.find((c) => c.command === "SetRemote")?.payload).toEqual({ enabled: true, url: REMOTE_DEFAULT_URL });
    expect((screen.getByLabelText("Адрес сервера") as HTMLInputElement).value).toBe(REMOTE_DEFAULT_URL);
    expect(within(box()).getByText("Подключаемся…")).toBeTruthy();
    expect(status().textContent).toBe("Связь с телефоном включена");
    await vi.waitFor(() => expect(document.activeElement).toBe(toggle()));
    await tick(MOCK_CONNECT_AFTER_MS + 5000);
    await vi.waitFor(() => expect(within(box()).getByText("На связи")).toBeTruthy());
  });

  it("выключение: связь выключена, адреса нет", async () => {
    const { user } = await setup(online);
    await user.click(toggle());
    await vi.waitFor(() => expect(toggle().checked).toBe(false));
    expect(status().textContent).toBe("Связь с телефоном выключена");
    expect(screen.queryByLabelText("Адрес сервера")).toBeNull();
  });

  it("неверный адрес: ошибка у поля, фокус в поле, связь не тронута", async () => {
    const { user } = await setup(online);
    const field = screen.getByLabelText("Адрес сервера") as HTMLInputElement;
    await user.clear(field);
    await user.type(field, "http://pcblock.example.ru");
    await user.click(within(box()).getByRole("button", { name: "Сохранить" }));
    const err = await within(box()).findByText(REMOTE_URL_ERROR);
    expect(err.getAttribute("role")).toBe("alert");
    expect(field.getAttribute("aria-invalid")).toBe("true");
    await vi.waitFor(() => expect(document.activeElement).toBe(field));
    expect(mock.state.remote.view.url).toBe(REMOTE_DEFAULT_URL);
    // Правка поля убирает ошибку.
    await user.type(field, "x");
    expect(within(box()).queryByText(REMOTE_URL_ERROR)).toBeNull();
  });

  it("новый адрес сохраняется Enter-ом", async () => {
    const { user } = await setup(online);
    const field = screen.getByLabelText("Адрес сервера") as HTMLInputElement;
    await user.clear(field);
    await user.type(field, "https://relay.example.ru{Enter}");
    await vi.waitFor(() => expect(mock.state.remote.view.url).toBe("https://relay.example.ru"));
    expect(status().textContent).toBe("Адрес сохранён");
  });
});

describe("PhoneSection: подключение телефона", () => {
  it("нет связи с сервером: кнопка недоступна, но в Tab-порядке, причина рядом и зачитывается", async () => {
    const { user } = await setup({ enabled: true, state: "connecting", lastOkMs: 0 });
    const btn = within(box()).getByRole("button", { name: "Подключить телефон" }) as HTMLButtonElement;
    expect(btn.getAttribute("aria-disabled")).toBe("true");
    expect(btn.disabled).toBe(false);
    const hint = within(box()).getByText("Подключить телефон можно, когда программа выйдет на связь с сервером");
    expect(hint.className).toBe("phone-why");
    expect(btn.getAttribute("aria-describedby")).toBe(hint.id);
    await user.click(btn);
    expect(mock.count("RemotePairStart")).toBe(0);
  });

  it("код крупно, «Действует 10 минут», отсчёт идёт; затем сверка и «Совпадает»", async () => {
    const { user } = await setup(online);
    await user.click(within(box()).getByRole("button", { name: "Подключить телефон" }));
    const code = await within(box()).findByText("482 913");
    expect(code.className).toContain("phone-code");
    expect(within(box()).getByText("Действует 10 минут")).toBeTruthy();
    expect(within(box()).getByText("Осталось 10:00")).toBeTruthy();
    // Фокус на «Отмена»: прежняя кнопка исчезла, фокус не потерян (PL-68).
    await vi.waitFor(() => expect(document.activeElement).toBe(within(box()).getByRole("button", { name: "Отмена" })));
    await tick(1000);
    await vi.waitFor(() => expect(within(box()).getByText("Осталось 9:59")).toBeTruthy());

    await tick(MOCK_COMPARE_AFTER_MS);
    const sas = await within(box()).findByText("731 004");
    expect(sas.className).toContain("phone-code");
    expect(within(box()).getByText("На телефоне должно быть число")).toBeTruthy();
    expect(within(box()).getByText("Нажмите «Совпадает» и на телефоне")).toBeTruthy();
    expect(within(box()).queryByText("482 913")).toBeNull();
    // «Отмена» исчезла вместе с кодом: фокус на «Совпадает», а не потерян.
    await vi.waitFor(() => expect(document.activeElement).toBe(within(box()).getByRole("button", { name: "Совпадает" })));
    await user.click(within(box()).getByRole("button", { name: "Совпадает" }));
    await within(box()).findByText(/^Телефон подключён: Pixel 7, с 18\.11 \d\d:\d\d$/);
    expect(mock.calls.filter((c) => c.command === "RemotePairConfirm").map((c) => c.payload)).toEqual([{ match: true }]);
    expect(within(box()).getByRole("button", { name: "Отвязать" })).toBeTruthy();
    await vi.waitFor(() => expect(document.activeElement).toBe(within(box()).getByRole("button", { name: "Отвязать" })));
  });

  it("«Не совпадает»: ошибка сразу (alert), «Подключить заново»", async () => {
    const { user } = await setup(online);
    await user.click(within(box()).getByRole("button", { name: "Подключить телефон" }));
    await within(box()).findByText("482 913");
    await tick(MOCK_COMPARE_AFTER_MS + 1000);
    await user.click(await within(box()).findByRole("button", { name: "Не совпадает" }));
    const alert = await within(box()).findByRole("alert");
    expect(alert.textContent).toBe("Подключение отменено. Начните заново");
    // Ошибка - рядом с областью статуса, не внутри неё (PL-75).
    expect(status().contains(alert)).toBe(false);
    const again = within(box()).getByRole("button", { name: "Подключить заново" });
    await vi.waitFor(() => expect(document.activeElement).toBe(again));
    expect(mock.state.remote.view.phone).toBeUndefined();
  });

  it("срок кода вышел: «Время на подключение вышло», фокус на «Подключить заново»", async () => {
    const { user } = await setup(online);
    await user.click(within(box()).getByRole("button", { name: "Подключить телефон" }));
    await within(box()).findByText("482 913");
    // Мок ставит срок при нажатии; телефон так и не пришёл.
    mock.state.remote.compareAtMs = Number.POSITIVE_INFINITY;
    await tick(10 * 60_000 + 1000);
    const alert = await within(box()).findByRole("alert");
    expect(alert.textContent).toBe("Время на подключение вышло. Начните заново");
    await vi.waitFor(() => expect(document.activeElement).toBe(within(box()).getByRole("button", { name: "Подключить заново" })));
  });

  it("«Отмена» кода: снова «Подключить телефон»", async () => {
    const { user } = await setup(online);
    await user.click(within(box()).getByRole("button", { name: "Подключить телефон" }));
    await within(box()).findByText("482 913");
    await user.click(within(box()).getByRole("button", { name: "Отмена" }));
    const btn = await within(box()).findByRole("button", { name: "Подключить телефон" });
    expect(within(box()).queryByText("482 913")).toBeNull();
    await vi.waitFor(() => expect(document.activeElement).toBe(btn));
  });

  it("сбой связи с программой: alert рядом со статусом, кнопка снова в работе и в фокусе", async () => {
    const { user } = await setup(online);
    mock.override("RemotePairStart", "reject", 1);
    const btn = within(box()).getByRole("button", { name: "Подключить телефон" });
    await user.click(btn);
    const alert = await within(box()).findByRole("alert");
    expect(alert.textContent).toBe(LINK_ERROR_TEXT);
    expect(status().contains(alert)).toBe(false);
    await vi.waitFor(() => expect(document.activeElement).toBe(within(box()).getByRole("button", { name: "Подключить телефон" })));
  });
});

describe("PhoneSection: подключённый телефон", () => {
  const paired: Partial<RemoteView> = { ...online, phone: { name: "Pixel 7", pairedAtMs: NOW.getTime() - 3_600_000 } };

  it("один телефон: «Подключить другой телефон» и пояснение про прежний", async () => {
    await setup(paired);
    expect(within(box()).getByText("Телефон подключён: Pixel 7, с 18.11 20:40")).toBeTruthy();
    expect(within(box()).getByRole("button", { name: "Подключить другой телефон" })).toBeTruthy();
    expect(within(box()).getByText("Прежний телефон будет отвязан, когда вы подключите новый")).toBeTruthy();
    expect(within(box()).queryByRole("button", { name: "Подключить телефон" })).toBeNull();
  });

  it("«Отвязать» спрашивает подтверждение; «Отмена» оставляет телефон", async () => {
    const { user } = await setup(paired);
    await user.click(within(box()).getByRole("button", { name: "Отвязать" }));
    const dialog = screen.getByRole("dialog", { name: "Отвязать телефон?" });
    expect(within(dialog).getByText("С него больше нельзя будет менять время")).toBeTruthy();
    // Безопасный выбор в фокусе.
    expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Отмена" }));
    await user.click(within(dialog).getByRole("button", { name: "Отмена" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(mock.count("RemoteUnpair")).toBe(0);
    expect(within(box()).getByText(/^Телефон подключён: Pixel 7/)).toBeTruthy();

    await user.click(within(box()).getByRole("button", { name: "Отвязать" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Отвязать" }));
    await vi.waitFor(() => expect(status().textContent).toBe("Телефон отвязан"));
    expect(mock.count("RemoteUnpair")).toBe(1);
    const btn = within(box()).getByRole("button", { name: "Подключить телефон" });
    await vi.waitFor(() => expect(document.activeElement).toBe(btn));
  });

  it("нет связи с HH:MM по местному времени, текст ошибки, другой телефон подключить нельзя", async () => {
    const last = NOW.getTime() - 35 * 60_000;
    await setup({ ...paired, state: "offline", lastOkMs: last, error: "no_server" });
    expect(within(box()).getByText(`Нет связи с ${formatTimeOfDay(last, localOffsetMin(NOW))}`)).toBeTruthy();
    expect(within(box()).getByText("Не удаётся связаться с сервером. Проверьте адрес и интернет")).toBeTruthy();
    // Нет связи не значит «отвязан» (Review Focus 2).
    expect(within(box()).getByText(/^Телефон подключён: Pixel 7/)).toBeTruthy();
    expect(within(box()).getByRole("button", { name: "Подключить другой телефон" }).getAttribute("aria-disabled")).toBe("true");
    expect(within(box()).getByText("Подключить телефон можно, когда связь с сервером восстановится")).toBeTruthy();
  });
});

describe("PhoneSection: опрос", () => {
  it("раз в 5 с без привязки, раз в 1 с во время привязки; после размонтирования опроса нет", async () => {
    const { user, view } = await setup(online);
    const n0 = mock.count("GetRemote");
    await tick(5000);
    expect(mock.count("GetRemote")).toBe(n0 + 1);
    await user.click(within(box()).getByRole("button", { name: "Подключить телефон" }));
    await within(box()).findByText("482 913");
    const n1 = mock.count("GetRemote");
    await tick(1000);
    await tick(1000);
    expect(mock.count("GetRemote")).toBe(n1 + 2);
    view.unmount();
    const n2 = mock.count("GetRemote");
    await tick(20_000);
    expect(mock.count("GetRemote")).toBe(n2);
  });

  it("сбой чтения состояния: человеческий текст, опрос продолжается", async () => {
    mock = installBackend({ sessionActive: true, remote: online });
    mock.override("GetRemote", "reject", 1);
    render(<PhoneSection />);
    await screen.findByText("Не удалось узнать состояние связи. Пробуем ещё раз");
    await tick(5000);
    await vi.waitFor(() => expect(within(box()).getByText("На связи")).toBeTruthy());
    expect(screen.queryByText("Не удалось узнать состояние связи. Пробуем ещё раз")).toBeNull();
  });
});

// R11: описание элемента по aria-describedby (как его прочитает экранный диктор).
function described(el: Element): string {
  return (el.getAttribute("aria-describedby") ?? "")
    .split(/\s+/)
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ");
}

describe("PhoneSection: доступность (R11)", () => {
  it("выключатель — switch с aria-checked, пояснение читается описанием", async () => {
    const { user } = await setup();
    expect(toggle().getAttribute("aria-checked")).toBe("false");
    expect(described(toggle())).toBe("Программа будет связываться только с вашим сервером. Выключено: программа совсем без интернета");
    await user.click(toggle());
    await vi.waitFor(() => expect(toggle().getAttribute("aria-checked")).toBe("true"));
  });

  it("появившийся код и число сверки зачитываются вместе с кнопкой в фокусе", async () => {
    const { user } = await setup(online);
    await user.click(within(box()).getByRole("button", { name: "Подключить телефон" }));
    await within(box()).findByText("482 913");
    const cancel = within(box()).getByRole("button", { name: "Отмена" });
    await vi.waitFor(() => expect(document.activeElement).toBe(cancel));
    expect(described(cancel)).toContain("482 913");
    expect(described(cancel)).toContain("Действует 10 минут");
    await tick(MOCK_COMPARE_AFTER_MS);
    await within(box()).findByText("731 004");
    const match = within(box()).getByRole("button", { name: "Совпадает" });
    expect(described(match)).toContain("На телефоне должно быть число");
    expect(described(match)).toContain("731 004");
  });
});
