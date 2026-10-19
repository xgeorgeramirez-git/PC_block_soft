import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "../api";
import { DEFAULT_PASSWORD, installBackend, mockChild, statusOf, uninstallBackend } from "./backend";

// Подмена программы сама должна вести себя как служба, иначе тесты экранов проверяют выдумку.

beforeEach(() => {
  vi.useFakeTimers({ now: new Date(2026, 9, 19, 12, 0, 0) }); // пн, 19 октября 2026
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

describe("вход и пауза (R19)", () => {
  it("после серии ошибок ввод закрыт, пауза удваивается", async () => {
    installBackend({ attempts: 2 });
    await api.login("1").catch(() => undefined);
    const err = await api.login("2").catch((e: unknown) => e);
    expect(api.loginDenied(err)).toEqual({ attemptsLeft: 0, pauseS: 60 });
    // Во время паузы даже верный пароль не принимается.
    const during = await api.login(DEFAULT_PASSWORD).catch((e: unknown) => e);
    expect(api.loginDenied(during)?.pauseS).toBe(60);
    vi.advanceTimersByTime(60_000);
    await expect(api.login(DEFAULT_PASSWORD)).resolves.toMatchObject({ idleTimeoutS: 600 });
  });

  it("следующая серия закрывает ввод на удвоенное время", async () => {
    installBackend({ attempts: 1 });
    await api.login("x").catch(() => undefined);
    vi.advanceTimersByTime(60_000);
    const err = await api.login("y").catch((e: unknown) => e);
    expect(api.loginDenied(err)?.pauseS).toBe(120);
  });
});

describe("родительская сессия", () => {
  it("команды настроек без входа отклоняются понятным текстом", async () => {
    installBackend();
    await expect(api.getToday()).rejects.toMatchObject({ message: "Сессия закончилась. Введите пароль" });
  });

  it("сессия кончается через 10 минут бездействия, любая команда продлевает", async () => {
    installBackend();
    await api.login(DEFAULT_PASSWORD);
    vi.advanceTimersByTime(9 * 60_000);
    await api.getToday();
    vi.advanceTimersByTime(9 * 60_000);
    expect((await api.sessionState()).active).toBe(true);
    vi.advanceTimersByTime(2 * 60_000);
    expect(await api.sessionState()).toEqual({ active: false, expiresMs: 0, remainingS: 0 });
  });

  it("запрос состояния сессию не продлевает", async () => {
    installBackend();
    await api.login(DEFAULT_PASSWORD);
    vi.advanceTimersByTime(5 * 60_000);
    const state = await api.sessionState();
    expect(state.remainingS).toBe(300);
  });
});

describe("статус ребёнка собирается в одном месте", () => {
  const now = Date.now();
  const hour = 3600;

  it.each([
    [{ usedS: 2100 }, "ok", "Можно"],
    [{ usedS: 2 * hour - 600 }, "low", "Осталось 10 мин"],
    [{ usedS: 2 * hour }, "timeUp", "Время вышло"],
    [{ blocked: true }, "bypass", "Заблокирован за обход защиты"],
  ])("%j -> %s", (over, status, text) => {
    const s = statusOf(mockChild(over), now);
    expect(s.status).toBe(status);
    expect(s.statusText).toBe(text);
  });

  it("доп. время увеличивает лимит и возвращает время", () => {
    const s = statusOf(mockChild({ usedS: 2 * hour, extraMin: 30 }), now);
    expect(s.status).toBe("ok");
    expect(s.limitS).toBe(150 * 60);
    expect(s.used).toBe("Сегодня: 2 ч 00 мин из 2 ч 30 мин");
  });

  it("без лимита", () => {
    const sunday = new Date(2026, 9, 25, 12).getTime();
    const s = statusOf(mockChild(), sunday);
    expect(s.status).toBe("unlimited");
    expect(s.unlimited).toBe(true);
  });
});

describe("доп. время", () => {
  it("добавляется, отменяется в течение 10 секунд, потом уже нет", async () => {
    installBackend({ sessionActive: true });
    const added = await api.addExtra(1, 30);
    expect(added.message).toBe("Добавлено 30 минут");
    expect(added.card.extraMin).toBe(30);
    expect(added.undoUntilMs).toBe(Date.now() + 10_000);
    const undone = await api.undoExtra(1, added.extraId);
    expect(undone.card.extraMin).toBe(0);
    expect(undone.message).toBe("Добавленное время отменено");

    const second = await api.addExtra(1, 15);
    vi.advanceTimersByTime(10_001);
    await expect(api.undoExtra(1, second.extraId)).rejects.toMatchObject({ message: /Отменить уже нельзя/ });
  });

  it("склонение: 1 минуту, 2 минуты, 5 минут", async () => {
    installBackend({ sessionActive: true });
    expect((await api.addExtra(1, 1)).message).toBe("Добавлено 1 минуту");
    expect((await api.addExtra(1, 2)).message).toBe("Добавлено 2 минуты");
    expect((await api.addExtra(1, 5)).message).toBe("Добавлено 5 минут");
    expect((await api.addExtra(1, 11)).message).toBe("Добавлено 11 минут");
  });

  it("событие попадает в журнал", async () => {
    installBackend({ sessionActive: true });
    await api.addExtra(1, 30);
    const events = await api.listEvents();
    expect(events[0]).toMatchObject({ kind: "extra_time", text: "Миша: доп. время +30 мин" });
  });
});

describe("мастер", () => {
  it("проверки настройки", async () => {
    installBackend({ configured: false });
    const limits = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 120, unlimited: false }));
    const mini = [{ sid: "S-1", name: "Миша", limits }];
    await expect(api.setup("123", mini)).rejects.toMatchObject({ field: "password" });
    await expect(api.setup("123456", [])).rejects.toMatchObject({ field: "children" });
    const tooMuch = [{ sid: "S-1", name: "Миша", limits: limits.map((l) => ({ ...l, totalMin: 2000 })) }];
    await expect(api.setup("123456", tooMuch)).rejects.toMatchObject({ field: "limits.1" });
    await api.setup("123456", mini);
    expect((await api.hello()).configured).toBe(true);
    await expect(api.setup("123456", mini)).rejects.toMatchObject({ message: "Программа уже настроена" });
  });

  it("учётки: у уже добавленного ребёнка стоит отметка", async () => {
    installBackend();
    const accounts = await api.listAccounts();
    expect(accounts.find((a) => a.name === "Миша")?.child).toBe(true);
    expect(accounts.find((a) => a.name === "Даша")?.child).toBe(false);
  });
});

describe("дети и пароль", () => {
  it("добавить, переименовать, убрать", async () => {
    installBackend({ sessionActive: true });
    const added = await api.setChildren({ add: [{ sid: "S-1-5-21-111-222-333-1002", name: "Даша" }] });
    expect(added.map((c) => c.name)).toEqual(["Миша", "Даша"]);
    const renamed = await api.setChildren({ rename: [{ childId: 1, name: "Михаил" }] });
    expect(renamed[0].name).toBe("Михаил");
    const removed = await api.setChildren({ remove: [1] });
    expect(removed.map((c) => c.name)).toEqual(["Даша"]);
  });

  it("администратора детской сделать нельзя", async () => {
    installBackend({ sessionActive: true });
    await expect(api.setChildren({ add: [{ sid: "S-1-5-21-111-222-333-1000", name: "Анна" }] })).rejects.toMatchObject({
      field: "children",
    });
  });

  it("смена пароля: ошибки по полям и успех", async () => {
    installBackend({ sessionActive: true });
    await expect(api.changePassword("не тот", "новый-секрет")).rejects.toMatchObject({ field: "oldPassword" });
    await expect(api.changePassword(DEFAULT_PASSWORD, "123")).rejects.toMatchObject({ field: "newPassword" });
    await api.changePassword(DEFAULT_PASSWORD, "новый-секрет");
    await api.logout();
    await expect(api.login(DEFAULT_PASSWORD)).rejects.toMatchObject({ field: "password" });
    await expect(api.login("новый-секрет")).resolves.toBeDefined();
  });
});

describe("журнал и удаление", () => {
  it("журнал: фильтры и новые сверху", async () => {
    const t = Date.now();
    const ev = (atMs: number, childId: number, text: string) => ({
      atMs,
      tzOffsetMin: 180,
      childId,
      childName: "",
      kind: "k",
      text,
    });
    installBackend({ sessionActive: true, events: [ev(t - 3000, 1, "старое"), ev(t - 1000, 2, "новое"), ev(t - 2000, 1, "среднее")] });
    expect((await api.listEvents()).map((e) => e.text)).toEqual(["новое", "среднее", "старое"]);
    expect((await api.listEvents({ childId: 1 })).map((e) => e.text)).toEqual(["среднее", "старое"]);
    expect((await api.listEvents({ fromMs: t - 2500, toMs: t - 1500 })).map((e) => e.text)).toEqual(["среднее"]);
    expect(await api.listEvents({ limit: 1 })).toHaveLength(1);
  });

  it("удаление: ненастроенная программа - без пароля, настроенная - по паролю или «забыл»", async () => {
    installBackend({ configured: false });
    await expect(api.uninstallCheck("", false)).resolves.toEqual({ allowed: true });
    uninstallBackend();
    installBackend();
    await expect(api.uninstallCheck("не тот", false)).rejects.toMatchObject({ field: "password" });
    await expect(api.uninstallCheck(DEFAULT_PASSWORD, false)).resolves.toEqual({ allowed: true });
    await expect(api.uninstallCheck("", true)).resolves.toEqual({ allowed: true });
  });

  it("сброс пароля", async () => {
    installBackend({ attempts: 1 });
    await expect(api.resetPassword("123")).rejects.toMatchObject({ field: "newPassword" });
    await api.login("x").catch(() => undefined); // пауза
    await api.resetPassword("новый-секрет");
    await expect(api.login("новый-секрет")).resolves.toBeDefined();
  });

  it("неизвестная команда и команды ребёнка отдают пример контракта", async () => {
    const mock = installBackend({ sessionActive: true });
    const raw = await mock.call("MyToday", "{}");
    expect(JSON.parse(raw).ok).toBe(true);
    expect(JSON.parse(await mock.call("Nope", "{}")).ok).toBe(false);
  });

  it("помощник хранит состояние как есть", async () => {
    installBackend({ sessionActive: true, helper: { enabled: true, seen: ["H-1"] } });
    expect(await api.getHelperState()).toEqual({ enabled: true, seen: ["H-1"] });
    await api.saveHelperState({ enabled: false });
    expect(await api.getHelperState()).toEqual({ enabled: false });
  });
});
