import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ChildLimits from "./ChildLimits";
import { UnsavedGuardProvider, useAttemptNavigation } from "../guard";
import { installBackend, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-16, FR-19 (L-44): таблица 7 дней, одна панель «Сохранить», ошибки у поля, диалог при уходе.

let mock: MockBackend;
let leave: Mock<() => void>;

function Leaver() {
  const attempt = useAttemptNavigation();
  return (
    <button type="button" onClick={() => attempt(() => leave())}>
      Уйти со страницы
    </button>
  );
}

function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({ sessionActive: true, ...opts });
  leave = vi.fn<() => void>();
  const user = userEvent.setup();
  render(
    <UnsavedGuardProvider>
      <ChildLimits childId={1} />
      <Leaver />
    </UnsavedGuardProvider>,
  );
  return user;
}

const row = (name: RegExp) => screen.getByRole("row", { name });
const hours = (day: string) => screen.getByLabelText(`Часы, ${day}`) as HTMLInputElement;
const minutes = (day: string) => screen.getByLabelText(`Минуты, ${day}`) as HTMLInputElement;
const unlimited = (day: string) => screen.getByLabelText(`Без лимита, ${day}`) as HTMLInputElement;

async function ready() {
  await screen.findByRole("table");
}

beforeEach(() => {
  uninstallBackend();
});

afterEach(() => {
  uninstallBackend();
});

describe("ChildLimits: таблица", () => {
  it("показывает 7 дней с часами, минутами и «Без лимита»", async () => {
    setup();
    await ready();
    expect(screen.getAllByRole("row")).toHaveLength(8); // шапка и 7 дней
    expect(screen.getByRole("columnheader", { name: "Время в день" })).toBeTruthy();
    expect(hours("понедельник").value).toBe("2");
    expect(minutes("понедельник").value).toBe("0");
    expect(hours("среда").value).toBe("1");
    expect(minutes("среда").value).toBe("30");
    expect(hours("суббота").value).toBe("4");
    expect(unlimited("воскресенье").checked).toBe(true);
    expect(hours("воскресенье").disabled).toBe(true);
    expect(minutes("воскресенье").disabled).toBe(true);
    expect(unlimited("понедельник").checked).toBe(false);
  });

  // Ловит: заголовок без имени ребёнка (родитель не понимает, чьи правила правит).
  it("называет ребёнка", async () => {
    setup();
    expect(await screen.findByRole("heading", { name: /Миша/ })).toBeTruthy();
  });

  it("без правок нижней панели нет", async () => {
    setup();
    await ready();
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
    expect(screen.queryByRole("button", { name: "Сохранить" })).toBeNull();
  });

  // Ловит: подсказка про 0 минут не показана (родитель думает, что 0 значит «без ограничений»).
  it("0 минут объясняет, что компьютером пользоваться нельзя", async () => {
    const user = setup();
    await ready();
    expect(screen.queryByText(/0 мин: в этот день/)).toBeNull();
    await user.clear(hours("понедельник"));
    await user.type(hours("понедельник"), "0");
    expect(within(row(/Понедельник/)).getByText("0 мин: в этот день компьютером пользоваться нельзя")).toBeTruthy();
  });

  it("«Без лимита» не показывает подсказку про 0 минут", async () => {
    setup();
    await ready();
    expect(within(row(/Воскресенье/)).queryByText(/0 мин: в этот день/)).toBeNull();
  });

  it("лишние минуты переходят в часы", async () => {
    const user = setup();
    await ready();
    await user.clear(minutes("вторник"));
    await user.type(minutes("вторник"), "75");
    expect(hours("вторник").value).toBe("3");
    expect(minutes("вторник").value).toBe("15");
  });
});

describe("ChildLimits: быстрые кнопки", () => {
  it("«Как в понедельник: на все будни»", async () => {
    const user = setup();
    await ready();
    await user.click(screen.getByRole("button", { name: "Как в понедельник: на все будни" }));
    for (const day of ["вторник", "среда", "четверг", "пятница"]) {
      expect(hours(day).value).toBe("2");
      expect(minutes(day).value).toBe("0");
    }
    expect(hours("суббота").value).toBe("4");
    expect(screen.getByText("Есть несохранённые изменения")).toBeTruthy();
  });

  it("«Как в субботу: на воскресенье» переносит и «без лимита»", async () => {
    const user = setup();
    await ready();
    await user.click(screen.getByRole("button", { name: "Как в субботу: на воскресенье" }));
    expect(unlimited("воскресенье").checked).toBe(false);
    expect(hours("воскресенье").value).toBe("4");
  });
});

describe("ChildLimits: одна панель сохранения (L-44)", () => {
  it("правка показывает панель с двумя кнопками, «Сохранить» одна на странице", async () => {
    const user = setup();
    await ready();
    await user.clear(hours("понедельник"));
    await user.type(hours("понедельник"), "1");
    expect(screen.getByText("Есть несохранённые изменения")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "Сохранить" })).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Отменить изменения" })).toBeTruthy();
  });

  it("возврат к прежнему значению прячет панель", async () => {
    const user = setup();
    await ready();
    await user.click(unlimited("понедельник"));
    expect(screen.getByText("Есть несохранённые изменения")).toBeTruthy();
    await user.click(unlimited("понедельник"));
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
  });

  it("«Отменить изменения» возвращает сохранённое", async () => {
    const user = setup();
    await ready();
    await user.click(unlimited("понедельник"));
    await user.click(screen.getByRole("button", { name: "Отменить изменения" }));
    expect(unlimited("понедельник").checked).toBe(false);
    expect(hours("понедельник").value).toBe("2");
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
    expect(mock.count("SaveLimits")).toBe(0);
  });

  // Ловит: в программу уходит не то, что в таблице, или сообщение «Сохранено» не показано.
  it("«Сохранить» отправляет все 7 дней и говорит, что правила действуют", async () => {
    const user = setup();
    await ready();
    await user.clear(hours("понедельник"));
    await user.type(hours("понедельник"), "1");
    await user.clear(minutes("понедельник"));
    await user.type(minutes("понедельник"), "30");
    await user.click(screen.getByRole("button", { name: "Сохранить" }));

    expect(await screen.findByText("Сохранено. Новые правила уже действуют")).toBeTruthy();
    const sent = mock.calls.find((c) => c.command === "SaveLimits")?.payload;
    expect(sent.childId).toBe(1);
    expect(sent.limits).toHaveLength(7);
    expect(sent.limits[0]).toEqual({ weekday: 1, totalMin: 90, unlimited: false });
    expect(sent.limits[6]).toEqual({ weekday: 7, totalMin: 0, unlimited: true });
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
    expect(mock.state.children[0].limits[0].totalMin).toBe(90);
  });

  // Ловит: итог сохранения виден только вверху страницы, а родитель смотрит на нижнюю панель
  // (ТЗ 8.3.3 п.5): нажал «Сохранить» и не понял, получилось ли.
  it("итог «Сохранено» показан на самой панели внизу, кнопок на ней уже нет", async () => {
    const user = setup();
    await ready();
    await user.click(unlimited("понедельник"));
    const bar = screen.getByRole("region", { name: "Сохранение изменений" });
    await user.click(within(bar).getByRole("button", { name: "Сохранить" }));
    const done = await within(screen.getByRole("region", { name: "Сохранение изменений" })).findByText(
      "Сохранено. Новые правила уже действуют",
    );
    expect(done.closest("[role=status]")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Сохранить" })).toBeNull();
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
    // Одно сообщение, а не два: вверху страницы его больше нет.
    expect(screen.getAllByText("Сохранено. Новые правила уже действуют")).toHaveLength(1);
  });

  // Ловит: ошибка сохранения появляется вверху, пока родитель смотрит на нижнюю панель (L-121).
  it("общий сбой сохранения показан на панели, рядом с кнопкой", async () => {
    const user = setup();
    await ready();
    mock.override("SaveLimits", "reject", 1);
    await user.click(unlimited("понедельник"));
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    const bar = screen.getByRole("region", { name: "Сохранение изменений" });
    expect((await within(bar).findByRole("alert")).textContent).toBe("Программа не отвечает. Попробуйте ещё раз через минуту");
    expect(within(bar).getByRole("button", { name: "Сохранить" })).toBeTruthy();
  });

  it("сообщение о сохранении уходит, когда родитель снова правит", async () => {
    const user = setup();
    await ready();
    await user.click(unlimited("понедельник"));
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    await screen.findByText("Сохранено. Новые правила уже действуют");
    await user.click(unlimited("вторник"));
    expect(screen.queryByText("Сохранено. Новые правила уже действуют")).toBeNull();
  });
});

describe("ChildLimits: ошибки у поля (L-121)", () => {
  // Ловит: лимит больше суток уходит в программу или ошибка видна не у дня.
  it("больше 24 часов: ошибка у дня, в программу не отправляется", async () => {
    const user = setup();
    await ready();
    await user.clear(hours("среда"));
    await user.type(hours("среда"), "25");
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    const alert = within(row(/Среда/)).getByRole("alert");
    expect(alert.textContent).toBe("Не больше 24 ч");
    expect(hours("среда").getAttribute("aria-invalid")).toBe("true");
    expect(mock.count("SaveLimits")).toBe(0);
    // Правки на месте, панель на месте.
    expect(screen.getByRole("button", { name: "Сохранить" })).toBeTruthy();
  });

  it("ошибка службы с полем limits.N показана у этого дня", async () => {
    const user = setup();
    await ready();
    mock.override("SaveLimits", { ok: false, error: "Не больше 24 ч", field: "limits.4" }, 1);
    await user.click(unlimited("понедельник"));
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    expect((await within(row(/Четверг/)).findByRole("alert")).textContent).toBe("Не больше 24 ч");
  });

  it("общий сбой: простыми словами, правки остаются", async () => {
    const user = setup();
    await ready();
    mock.override("SaveLimits", "reject", 1);
    await user.click(unlimited("понедельник"));
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    expect(unlimited("понедельник").checked).toBe(true);
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText("Сохранено. Новые правила уже действуют")).toBeTruthy();
  });

  it("нет связи при загрузке: понятный текст и «Повторить»", async () => {
    mock = installBackend({ sessionActive: true });
    mock.override("GetChildRules", "reject", 1);
    render(
      <UnsavedGuardProvider>
        <ChildLimits childId={1} />
      </UnsavedGuardProvider>,
    );
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByRole("table")).toBeTruthy();
  });
});

describe("ChildLimits: уход со страницы (FR-19)", () => {
  it("без правок уходит сразу", async () => {
    const user = setup();
    await ready();
    await user.click(screen.getByRole("button", { name: "Уйти со страницы" }));
    expect(leave).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("с правками спрашивает «Сохранить изменения?» и не уходит", async () => {
    const user = setup();
    await ready();
    await user.click(unlimited("понедельник"));
    await user.click(screen.getByRole("button", { name: "Уйти со страницы" }));
    const dialog = screen.getByRole("dialog", { name: "Сохранить изменения?" });
    expect(within(dialog).getByRole("button", { name: "Сохранить" })).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: "Не сохранять" })).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: "Остаться" })).toBeTruthy();
    expect(leave).not.toHaveBeenCalled();
  });

  it("«Сохранить» сохраняет и уходит", async () => {
    const user = setup();
    await ready();
    await user.click(unlimited("понедельник"));
    await user.click(screen.getByRole("button", { name: "Уйти со страницы" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Сохранить" }));
    await vi.waitFor(() => expect(leave).toHaveBeenCalledTimes(1));
    expect(mock.count("SaveLimits")).toBe(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("«Не сохранять» уходит без сохранения", async () => {
    const user = setup();
    await ready();
    await user.click(unlimited("понедельник"));
    await user.click(screen.getByRole("button", { name: "Уйти со страницы" }));
    await user.click(screen.getByRole("button", { name: "Не сохранять" }));
    expect(leave).toHaveBeenCalledTimes(1);
    expect(mock.count("SaveLimits")).toBe(0);
  });

  it("«Остаться» оставляет правки на месте", async () => {
    const user = setup();
    await ready();
    await user.click(unlimited("понедельник"));
    await user.click(screen.getByRole("button", { name: "Уйти со страницы" }));
    await user.click(screen.getByRole("button", { name: "Остаться" }));
    expect(leave).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(unlimited("понедельник").checked).toBe(true);
  });

  it("Esc в диалоге - «Остаться»", async () => {
    const user = setup();
    await ready();
    await user.click(unlimited("понедельник"));
    await user.click(screen.getByRole("button", { name: "Уйти со страницы" }));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(leave).not.toHaveBeenCalled();
  });

  // Ловит: диалог-тупик (L-119) - «Сохранить» не удалось, а родитель остался ни с чем.
  it("если сохранить не получилось, диалог закрывается и ошибка видна", async () => {
    const user = setup();
    await ready();
    mock.override("SaveLimits", "reject", 1);
    await user.click(unlimited("понедельник"));
    await user.click(screen.getByRole("button", { name: "Уйти со страницы" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(leave).not.toHaveBeenCalled();
  });
});
