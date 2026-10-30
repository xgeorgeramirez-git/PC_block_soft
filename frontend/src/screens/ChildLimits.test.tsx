import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ChildLimits from "./ChildLimits";
import { UnsavedGuardProvider, useAttemptNavigation } from "../guard";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";
import type { TodayCard } from "../types";

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
const gHours = (day: string) => screen.getByLabelText(`Часы игр, ${day}`) as HTMLInputElement;
const gMinutes = (day: string) => screen.getByLabelText(`Минуты игр, ${day}`) as HTMLInputElement;
const gUnlimited = (day: string) => screen.getByLabelText(`Игры без лимита, ${day}`) as HTMLInputElement;
const sentLimits = () => mock.calls.filter((c) => c.command === "SaveLimits").at(-1)?.payload;

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
    expect(screen.getByRole("columnheader", { name: "Общее" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "Игры" })).toBeTruthy();
    expect(screen.queryByRole("columnheader", { name: /Видео/ })).toBeNull();
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
    expect(screen.queryByText(/компьютером пользоваться нельзя/)).toBeNull();
    await user.clear(hours("понедельник"));
    await user.type(hours("понедельник"), "0");
    expect(within(row(/Понедельник/)).getByText("0 мин: в этот день компьютером пользоваться нельзя")).toBeTruthy();
  });

  it("«Без лимита» не показывает подсказку про 0 минут", async () => {
    setup();
    await ready();
    expect(within(row(/Воскресенье/)).queryByText(/0 мин: в этот день/)).toBeNull();
  });

  // FB-1 (D-45). Ловит: в поле минут остаётся «75» или «99», а в поле часов «25».
  it("минуты не больше 59, часы не больше 23", async () => {
    const user = setup();
    await ready();
    await user.clear(minutes("вторник"));
    await user.type(minutes("вторник"), "75");
    expect(minutes("вторник").value).toBe("59");
    expect(hours("вторник").value).toBe("2");
    await user.clear(hours("вторник"));
    await user.type(hours("вторник"), "25");
    expect(hours("вторник").value).toBe("23");
  });

  // FB-1 (D-45). Ловит: буквы попадают в поле.
  it("буквы в поля часов и минут не вводятся", async () => {
    const user = setup();
    await ready();
    await user.clear(minutes("вторник"));
    await user.type(minutes("вторник"), "а3б0");
    expect(minutes("вторник").value).toBe("30");
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
    // Лимит игр понедельника (1 ч) не стёрт правкой общего времени (RM3-9).
    expect(sent.limits[0]).toEqual({ weekday: 1, totalMin: 90, unlimited: false, gamesMin: 60, gamesUnlimited: false });
    expect(sent.limits[2]).toEqual({ weekday: 3, totalMin: 90, unlimited: false, gamesMin: 0, gamesUnlimited: false });
    expect(sent.limits[6]).toEqual({ weekday: 7, totalMin: 0, unlimited: true, gamesMin: 0, gamesUnlimited: true });
    expect(sent.endMode).toBe("lock");
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
  // Ловит: лимит больше суток уходит в программу (D-45: поле само держит 23 ч).
  it("больше 24 часов набрать нельзя: поле держит 23", async () => {
    const user = setup();
    await ready();
    await user.clear(hours("среда"));
    await user.type(hours("среда"), "25");
    expect(hours("среда").value).toBe("23");
    expect(within(row(/Среда/)).queryByRole("alert")).toBeNull();
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

describe("ChildLimits: столбец «Игры» (FR-16)", () => {
  it("показывает лимит игр из правил: 1 ч, без лимита, 0", async () => {
    setup();
    await ready();
    expect(gHours("понедельник").value).toBe("1");
    expect(gMinutes("понедельник").value).toBe("0");
    expect(gUnlimited("понедельник").checked).toBe(false);
    expect(gUnlimited("вторник").checked).toBe(true);
    expect(gHours("вторник").disabled).toBe(true);
    expect(gMinutes("вторник").disabled).toBe(true);
    expect(gHours("суббота").value).toBe("2");
    // Среда: игры 0 - отдельная подсказка, у общего времени её нет.
    const wed = row(/Среда/);
    expect(within(wed).getByText("0 мин: в этот день играть нельзя")).toBeTruthy();
    expect(within(wed).queryByText(/компьютером пользоваться нельзя/)).toBeNull();
  });

  it("под таблицей пояснение про игры и учёбу", async () => {
    setup();
    await ready();
    expect(screen.getByText("Время игр входит в общее. Учёба в общее время не входит")).toBeTruthy();
    expect(screen.queryByText(/Школа и учёба тоже считаются/)).toBeNull();
  });

  // Review Focus 1: по строке табуляция идёт час → минуты → «Без лимита», общее, затем игры.
  it("табуляция по строке: общее время, затем игры, затем следующий день", async () => {
    const user = setup();
    await ready();
    hours("понедельник").focus();
    const order = [minutes("понедельник"), unlimited("понедельник"), gHours("понедельник"), gMinutes("понедельник"), gUnlimited("понедельник"), hours("вторник")];
    for (const el of order) {
      await user.tab();
      expect(document.activeElement).toBe(el);
    }
  });

  // Ловит: поля игр принимают буквы и больше 23 ч / 59 мин (D-45).
  it("поля игр: только цифры, часы до 23, минуты до 59", async () => {
    const user = setup();
    await ready();
    await user.clear(gMinutes("понедельник"));
    await user.type(gMinutes("понедельник"), "а7б5");
    expect(gMinutes("понедельник").value).toBe("59");
    await user.clear(gHours("понедельник"));
    await user.type(gHours("понедельник"), "30");
    expect(gHours("понедельник").value).toBe("23");
  });

  // Ловит: правка игр уходит в общий лимит (столбцы перепутаны).
  it("правка игр сохраняется в gamesMin, общий лимит дня прежний", async () => {
    const user = setup();
    await ready();
    await user.click(gUnlimited("вторник"));
    expect(gHours("вторник").value).toBe("1");
    await user.clear(gMinutes("вторник"));
    await user.type(gMinutes("вторник"), "15");
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    await screen.findByText("Сохранено. Новые правила уже действуют");
    const sent = sentLimits();
    expect(sent.limits[1]).toEqual({ weekday: 2, totalMin: 120, unlimited: false, gamesMin: 75, gamesUnlimited: false });
    expect(sent.limits[0]).toEqual({ weekday: 1, totalMin: 120, unlimited: false, gamesMin: 60, gamesUnlimited: false });
    expect(mock.state.children[0].limits[1]).toEqual({ weekday: 2, totalMin: 120, unlimited: false, gamesMin: 75, gamesUnlimited: false });
  });

  // Ловит: «Без лимита» у игр оставляет в программе прежние минуты.
  it("«Без лимита» у игр отправляет gamesMin 0", async () => {
    const user = setup();
    await ready();
    await user.click(gUnlimited("понедельник"));
    expect(gHours("понедельник").disabled).toBe(true);
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    await screen.findByText("Сохранено. Новые правила уже действуют");
    expect(sentLimits().limits[0]).toEqual({ weekday: 1, totalMin: 120, unlimited: false, gamesMin: 0, gamesUnlimited: true });
  });

  it("игры больше общего лимита сохраняются без ошибки", async () => {
    const user = setup();
    await ready();
    await user.clear(gHours("понедельник"));
    await user.type(gHours("понедельник"), "5");
    expect(within(row(/Понедельник/)).queryByRole("alert")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText("Сохранено. Новые правила уже действуют")).toBeTruthy();
    expect(sentLimits().limits[0].gamesMin).toBe(300);
  });

  it("«Как в понедельник» и «Как в субботу» копируют и игры", async () => {
    const user = setup();
    await ready();
    await user.click(screen.getByRole("button", { name: "Как в понедельник: на все будни" }));
    for (const day of ["вторник", "среда", "четверг", "пятница"]) {
      expect(gUnlimited(day).checked).toBe(false);
      expect(gHours(day).value).toBe("1");
      expect(gMinutes(day).value).toBe("0");
    }
    await user.click(screen.getByRole("button", { name: "Как в субботу: на воскресенье" }));
    expect(gUnlimited("воскресенье").checked).toBe(false);
    expect(gHours("воскресенье").value).toBe("2");
  });

  it("ошибка службы games.N показана у игр этого дня", async () => {
    const user = setup();
    await ready();
    mock.override("SaveLimits", { ok: false, error: "Не больше 24 ч", field: "games.2" }, 1);
    await user.click(gUnlimited("вторник"));
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    const alert = await within(row(/Вторник/)).findByRole("alert");
    expect(alert.textContent).toBe("Не больше 24 ч");
    expect(alert.closest("td")?.contains(gHours("вторник"))).toBe(true);
    expect(gHours("вторник").getAttribute("aria-invalid")).toBe("true");
    expect(hours("вторник").getAttribute("aria-invalid")).toBeNull();
  });
});

describe("ChildLimits: «Когда время вышло» (FR-18, R8)", () => {
  const openEnd = async (user: ReturnType<typeof userEvent.setup>) => {
    await ready();
    await user.click(screen.getByRole("tab", { name: "Когда время вышло" }));
  };
  const lockRadio = () => screen.getByRole("radio", { name: /Блокировать экран/ }) as HTMLInputElement;
  const studyRadio = () => screen.getByRole("radio", { name: /Оставить только учёбу/ }) as HTMLInputElement;

  it("два варианта с пояснениями, выбран сохранённый", async () => {
    const user = setup();
    await openEnd(user);
    expect(screen.getAllByRole("tab").map((t) => t.textContent)).toEqual(["Лимиты", "Запретные часы", "Когда время вышло"]);
    expect(lockRadio().checked).toBe(true);
    expect(studyRadio().checked).toBe(false);
    expect(screen.getByText("Компьютер закрывается экраном блокировки, игры закрываются, остальные окна сохраняются")).toBeTruthy();
    expect(screen.getByText("Работают только учебные программы и сайты, всё остальное закрывается")).toBeTruthy();
    expect(lockRadio().getAttribute("aria-describedby")).toBe("end-mode-lock-text");
  });

  it("сохранённый режим «Оставить только учёбу» выбран при открытии", async () => {
    const user = setup({ children: [mockChild({ endMode: "study_only" })] });
    await openEnd(user);
    expect(studyRadio().checked).toBe(true);
  });

  // Ловит: родитель выбирает «только учёбу», не зная, что учебных программ нет и закроется всё.
  it("нет учебных программ: предупреждение у «Оставить только учёбу» и кнопка «Программы»", async () => {
    const user = setup();
    await openEnd(user);
    const warn = await screen.findByText("Учебных программ и сайтов пока нет: в этом режиме будет закрыто всё");
    expect(warn.closest(".end-mode")?.contains(studyRadio())).toBe(true);
    expect(studyRadio().getAttribute("aria-describedby")).toContain("end-mode-study_only-warn");
    expect(screen.getAllByText(/Учебных программ и сайтов пока нет/)).toHaveLength(1);
  });

  // Ловит: в нижней панели склеиваются две фразы или искажено «Есть несохранённые изменения»
  // (приёмка P2 п.3, кадр s12-end-mode-no-study): после выбора режима на панели ровно одна фраза.
  it("выбран «Оставить только учёбу»: на панели ровно «Есть несохранённые изменения» и две кнопки", async () => {
    const user = setup();
    await openEnd(user);
    await user.click(studyRadio());
    const bar = screen.getByRole("region", { name: "Сохранение изменений" });
    expect(bar.querySelector(".savebar-main")?.textContent).toBe("Есть несохранённые изменения");
    expect(within(bar).queryByRole("status")).toBeNull();
    expect(within(bar).getAllByRole("button").map((b) => b.textContent)).toEqual(["Отменить изменения", "Сохранить"]);
  });

  it("есть учебная программа: предупреждения нет", async () => {
    const user = setup({
      programs: [{ path: "c:\\program files\\geo\\geo.exe", name: "ГеоКласс", publisher: "Школа", category: "study", source: "parent", weekS: 0 }],
    });
    await openEnd(user);
    await vi.waitFor(() => expect(mock.count("ListPrograms")).toBe(1));
    expect(screen.queryByText(/Учебных программ и сайтов пока нет/)).toBeNull();
  });

  it("список программ не прочитался: правила открыты, предупреждения нет", async () => {
    mock = installBackend({ sessionActive: true });
    mock.override("ListPrograms", "reject", 1);
    const user = userEvent.setup();
    render(
      <UnsavedGuardProvider>
        <ChildLimits childId={1} />
      </UnsavedGuardProvider>,
    );
    await openEnd(user);
    expect(lockRadio().checked).toBe(true);
    expect(screen.queryByText(/Учебных программ и сайтов пока нет/)).toBeNull();
  });

  // Review Focus 2: смена только режима не стирает лимиты и расписание.
  it("смена режима: одна панель «Сохранить», в программу уходят все лимиты и режим", async () => {
    const user = setup({ children: [mockChild({ schedule: [{ weekday: 1, startMin: 1260, endMin: 420 }] })] });
    await openEnd(user);
    await user.click(studyRadio());
    expect(screen.getByText("Есть несохранённые изменения")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText("Сохранено. Новые правила уже действуют")).toBeTruthy();
    const sent = sentLimits();
    expect(sent.endMode).toBe("study_only");
    expect(sent.limits).toHaveLength(7);
    expect(sent.limits[0]).toEqual({ weekday: 1, totalMin: 120, unlimited: false, gamesMin: 60, gamesUnlimited: false });
    expect(sent.limits[6]).toEqual({ weekday: 7, totalMin: 0, unlimited: true, gamesMin: 0, gamesUnlimited: true });
    expect(mock.count("SaveSchedule")).toBe(0);
    expect(mock.state.children[0].endMode).toBe("study_only");
    expect(mock.state.children[0].limits[0].gamesMin).toBe(60);
    expect(mock.state.children[0].schedule).toEqual([{ weekday: 1, startMin: 1260, endMin: 420 }]);
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
  });

  it("правка лимита и режима сохраняются одним вызовом", async () => {
    const user = setup();
    await ready();
    await user.click(unlimited("понедельник"));
    await user.click(screen.getByRole("tab", { name: "Когда время вышло" }));
    await user.click(studyRadio());
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    await screen.findByText("Сохранено. Новые правила уже действуют");
    expect(mock.count("SaveLimits")).toBe(1);
    expect(sentLimits().endMode).toBe("study_only");
    expect(sentLimits().limits[0].unlimited).toBe(true);
  });

  it("возврат к прежнему режиму прячет панель, «Отменить изменения» возвращает режим", async () => {
    const user = setup();
    await openEnd(user);
    await user.click(studyRadio());
    await user.click(lockRadio());
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
    await user.click(studyRadio());
    await user.click(screen.getByRole("button", { name: "Отменить изменения" }));
    expect(lockRadio().checked).toBe(true);
    expect(mock.count("SaveLimits")).toBe(0);
  });

  it("ошибка службы по режиму показана у вариантов", async () => {
    const user = setup();
    await openEnd(user);
    mock.override("SaveLimits", { ok: false, error: "Выберите, что делать, когда время вышло", field: "endMode" }, 1);
    await user.click(studyRadio());
    await user.click(screen.getByRole("tab", { name: "Лимиты" }));
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    const alert = await screen.findByText("Выберите, что делать, когда время вышло");
    expect(screen.getByRole("tab", { name: "Когда время вышло" }).getAttribute("aria-selected")).toBe("true");
    expect(alert.closest("fieldset")?.contains(studyRadio())).toBe(true);
  });

  // Review Focus 3: ссылка «Программы» с несохранёнными правками спрашивает «Сохранить изменения?».
  it("«Программы» с несохранённой правкой спрашивает, без правок уходит сразу", async () => {
    mock = installBackend({ sessionActive: true });
    const openPrograms = vi.fn<() => void>();
    function WithLink() {
      const attempt = useAttemptNavigation();
      return <ChildLimits childId={1} onOpenPrograms={() => attempt(openPrograms)} />;
    }
    const user = userEvent.setup();
    render(
      <UnsavedGuardProvider>
        <WithLink />
      </UnsavedGuardProvider>,
    );
    await openEnd(user);
    await user.click(await screen.findByRole("button", { name: "Программы" }));
    expect(openPrograms).toHaveBeenCalledTimes(1);
    await user.click(studyRadio());
    await user.click(screen.getByRole("button", { name: "Программы" }));
    expect(screen.getByRole("dialog", { name: "Сохранить изменения?" })).toBeTruthy();
    expect(openPrograms).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole("button", { name: "Не сохранять" }));
    expect(openPrograms).toHaveBeenCalledTimes(2);
  });
});

describe("ChildLimits: «Скопировать правила с…» (FR-33)", () => {
  // Ловит: копирование переносит только общее время, а игры и режим остаются прежними.
  it("переносит в черновик игры и режим, сохранение отправляет их", async () => {
    mock = installBackend({
      sessionActive: true,
      children: [
        mockChild(),
        mockChild({
          childId: 2,
          sid: "S-1-5-21-111-222-333-1002",
          name: "Даша",
          endMode: "study_only",
          limits: [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 90, unlimited: false, gamesMin: 30, gamesUnlimited: false })),
        }),
      ],
    });
    const others = [{ childId: 2, name: "Даша" } as TodayCard];
    const user = userEvent.setup();
    render(
      <UnsavedGuardProvider>
        <ChildLimits childId={1} others={others} />
      </UnsavedGuardProvider>,
    );
    await ready();
    await user.click(screen.getByRole("button", { name: "Скопировать правила с…" }));
    await user.click(screen.getByRole("button", { name: "Даша" }));
    await vi.waitFor(() => expect(gUnlimited("вторник").checked).toBe(false));
    // Копирование изменило поля: панель говорит и о правках, и о том, что правила скопированы.
    expect(screen.getByText("Есть несохранённые изменения")).toBeTruthy();
    expect(screen.getByText("Правила скопированы. Проверьте и сохраните").closest('[role="status"]')).toBeTruthy();
    expect(gMinutes("вторник").value).toBe("30");
    expect(hours("вторник").value).toBe("1");
    await user.click(screen.getByRole("tab", { name: "Когда время вышло" }));
    expect((screen.getByRole("radio", { name: /Оставить только учёбу/ }) as HTMLInputElement).checked).toBe(true);
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    await screen.findByText("Сохранено. Новые правила уже действуют");
    const sent = sentLimits();
    expect(sent.endMode).toBe("study_only");
    expect(sent.limits[6]).toEqual({ weekday: 7, totalMin: 90, unlimited: false, gamesMin: 30, gamesUnlimited: false });
  });
});
