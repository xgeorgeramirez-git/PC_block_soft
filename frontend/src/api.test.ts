import { afterEach, describe, expect, it } from "vitest";
import * as api from "./api";
import { CallError, LINK_ERROR_TEXT } from "./lib/errors";
import { installBackend, uninstallBackend, DEFAULT_PASSWORD } from "./__mocks__/backend";

afterEach(() => {
  uninstallBackend();
});

describe("call: сбои связи говорят простыми словами", () => {
  // Ловит: интерфейс запущен без моста (window.go нет) и падает с «undefined».
  it("нет моста", async () => {
    await expect(api.hello()).rejects.toMatchObject({ message: LINK_ERROR_TEXT, link: true });
  });

  it("мост отклонил вызов", async () => {
    const mock = installBackend({ linkDown: true });
    const err = await api.hello().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(CallError);
    expect((err as CallError).message).toBe("Программа не отвечает. Попробуйте ещё раз через минуту");
    expect(mock.count("Hello")).toBe(1);
  });

  it("ответ не JSON", async () => {
    const mock = installBackend();
    mock.override("Hello", "garbage");
    await expect(api.hello()).rejects.toMatchObject({ link: true });
  });

  it("ответ без разбора: reject моста с техническим текстом", async () => {
    const mock = installBackend();
    mock.override("Hello", "reject");
    const err = (await api.hello().catch((e: unknown) => e)) as CallError;
    expect(err.message).toBe(LINK_ERROR_TEXT);
    expect(err.message).not.toMatch(/ipc|reset/);
  });

  it("отказ программы приносит текст, поле и нагрузку", async () => {
    installBackend({ sessionActive: true });
    const err = (await api.addExtra(1, 500).catch((e: unknown) => e)) as CallError;
    expect(err.message).toBe("Можно добавить от 1 до 240 минут");
    expect(err.field).toBe("minutes");
    expect(err.link).toBe(false);
  });
});

describe("массивы в ответах: null становится пустым списком (L-22)", () => {
  it("getToday", async () => {
    const mock = installBackend({ sessionActive: true });
    mock.override("GetToday", { ok: true, payload: { children: null, notices: null } });
    expect(await api.getToday()).toEqual({ children: [], notices: [] });
  });

  it("getToday: пустой ответ без полей", async () => {
    const mock = installBackend({ sessionActive: true });
    mock.override("GetToday", { ok: true });
    expect(await api.getToday()).toEqual({ children: [], notices: [] });
  });

  it("listEvents", async () => {
    const mock = installBackend({ sessionActive: true });
    mock.override("ListEvents", { ok: true, payload: { events: null } });
    expect(await api.listEvents()).toEqual([]);
  });

  it("listAccounts", async () => {
    const mock = installBackend();
    mock.override("ListAccounts", { ok: true, payload: { accounts: null } });
    expect(await api.listAccounts()).toEqual([]);
  });

  it("getChildRules", async () => {
    const mock = installBackend({ sessionActive: true });
    mock.override("GetChildRules", { ok: true, payload: { childId: 1, name: "Миша", limits: null } });
    expect((await api.getChildRules(1)).limits).toEqual([]);
  });

  it("setChildren", async () => {
    const mock = installBackend({ sessionActive: true });
    mock.override("SetChildren", { ok: true, payload: { children: null } });
    expect(await api.setChildren({ remove: [1] })).toEqual([]);
  });
});

describe("запросы", () => {
  it("setChildren всегда шлёт три списка, не null", async () => {
    const mock = installBackend({ sessionActive: true });
    await api.setChildren({ rename: [{ childId: 1, name: "Михаил" }] });
    expect(mock.calls.at(-1)?.payload).toEqual({ add: [], remove: [], rename: [{ childId: 1, name: "Михаил" }] });
  });

  it("listEvents без условий просит все события", async () => {
    const mock = installBackend({ sessionActive: true });
    await api.listEvents();
    expect(mock.calls.at(-1)?.payload).toEqual({ childId: 0, fromMs: 0, toMs: 0, limit: 0 });
  });

  it("команды без данных шлют пустой объект", async () => {
    const mock = installBackend({ sessionActive: true });
    await api.getToday();
    expect(mock.calls.at(-1)).toEqual({ command: "GetToday", payload: {} });
  });
});

describe("вход и сессия", () => {
  it("верный пароль открывает сессию", async () => {
    installBackend();
    const reply = await api.login(DEFAULT_PASSWORD);
    expect(reply.idleTimeoutS).toBe(600);
    expect((await api.sessionState()).active).toBe(true);
  });

  it("неверный пароль: отказ с попытками", async () => {
    installBackend();
    const err = await api.login("не тот").catch((e: unknown) => e);
    expect(api.loginDenied(err)).toEqual({ attemptsLeft: 4, pauseS: 0 });
    expect((err as CallError).field).toBe("password");
  });

  it("loginDenied не принимает чужие ошибки", () => {
    expect(api.loginDenied(new Error("x"))).toBeNull();
    expect(api.loginDenied(new CallError("текст"))).toBeNull();
    expect(api.loginDenied(new CallError("текст", { payload: { attemptsLeft: "много" } }))).toBeNull();
  });

  it("mode и version приходят от моста", async () => {
    installBackend({ mode: "parent", version: "1.2.3" });
    expect(await api.mode()).toBe("parent");
    expect(await api.version()).toBe("1.2.3");
  });

  it("startScreen отдаёт экран окна родителя из ключа --screen (RM3-4)", async () => {
    installBackend({ mode: "parent", startScreen: "programs" });
    expect(await api.startScreen()).toBe("programs");
    installBackend({ mode: "parent" });
    expect(await api.startScreen()).toBe("");
    installBackend({ mode: "parent", startScreen: "settings" });
    expect(await api.startScreen()).toBe("");
  });

  it("startScreen без моста или без метода - главный экран, не ошибка", async () => {
    delete window.go;
    expect(await api.startScreen()).toBe("");
    window.go = { ui: { App: { Call: () => Promise.resolve("{}"), Mode: () => Promise.resolve("parent"), Version: () => Promise.resolve("0") } } };
    expect(await api.startScreen()).toBe("");
    window.go = {
      ui: { App: { Call: () => Promise.resolve("{}"), Mode: () => Promise.resolve("parent"), Version: () => Promise.resolve("0"), StartScreen: () => Promise.reject(new Error("x")) } },
    };
    expect(await api.startScreen()).toBe("");
  });

  it("mode и version без моста - ошибка связи", async () => {
    await expect(api.mode()).rejects.toMatchObject({ link: true });
    await expect(api.version()).rejects.toMatchObject({ link: true });
  });

  it("выход закрывает сессию", async () => {
    installBackend();
    await api.login(DEFAULT_PASSWORD);
    await api.logout();
    expect((await api.sessionState()).active).toBe(false);
  });
});

describe("лимиты игр и режим «Когда время вышло» (M3)", () => {
  // Ловит: день без полей игр уходит в программу как «игры нельзя» (gamesMin 0, gamesUnlimited
  // false по умолчанию в Go) или заданный лимит игр перезаписывается «без лимита».
  it("withGames ставит «без отдельного лимита» только там, где полей нет", () => {
    expect(
      api.withGames([
        { weekday: 1, totalMin: 60, unlimited: false },
        { weekday: 2, totalMin: 60, unlimited: false, gamesMin: 30, gamesUnlimited: false },
        { weekday: 3, totalMin: 60, unlimited: false, gamesMin: 0, gamesUnlimited: false },
      ]),
    ).toEqual([
      { weekday: 1, totalMin: 60, unlimited: false, gamesMin: 0, gamesUnlimited: true },
      { weekday: 2, totalMin: 60, unlimited: false, gamesMin: 30, gamesUnlimited: false },
      { weekday: 3, totalMin: 60, unlimited: false, gamesMin: 0, gamesUnlimited: false },
    ]);
  });

  // Ловит: подменная программа теряет лимит игр и режим при сохранении (T6 строит экраны на ней).
  it("подмена хранит игры и режим, только режим не трогает лимиты", async () => {
    const mock = installBackend({ sessionActive: true });
    const limits = (await api.getChildRules(1)).limits.map((l) =>
      l.weekday === 1 ? { ...l, gamesMin: 45, gamesUnlimited: false } : l,
    );
    const saved = await api.saveLimits(1, limits, "study_only");
    expect(saved.rules.endMode).toBe("study_only");
    expect(saved.rules.limits[0]).toMatchObject({ gamesMin: 45, gamesUnlimited: false });
    const modeOnly = await api.saveLimits(1, [], "lock");
    expect(modeOnly.rules.endMode).toBe("lock");
    expect(modeOnly.rules.limits[0]).toMatchObject({ gamesMin: 45, gamesUnlimited: false });
    expect(mock.calls.filter((c) => c.command === "SaveLimits").map((c) => c.payload.endMode)).toEqual(["study_only", "lock"]);
  });
});

describe("«Программы» в подменной программе (M3, FR-20)", () => {
  // Ловит: подмена не умеет решать и отменять (T6 строит экран на ней), карточка «Сегодня» не
  // считает «Ждут решения», ребёнок без сессии родителя читает программы.
  it("решение, смена категории и «Отменить»", async () => {
    const mock = installBackend({ sessionActive: true });
    const before = await api.listPrograms();
    expect(before.pending).toHaveLength(1);
    expect((await api.getToday()).children[0].pendingPrograms).toBe(1);
    const path = before.pending[0].path;
    const decided = await api.decideProgram(path, "games");
    expect(decided.message).toBe("SuperGame: игра");
    expect(decided.programs.pending).toEqual([]);
    expect(decided.programs.programs.find((p) => p.path === path)).toMatchObject({ category: "games", source: "parent" });
    expect((await api.getToday()).children[0].pendingPrograms).toBe(0);
    const undone = await api.undoProgram(decided.undoId);
    expect(undone.programs.pending.map((p) => p.path)).toEqual([path]);
    const brave = before.programs.find((p) => p.name === "Brave")!;
    const mass = await api.setProgramsCategory([brave.path], "study");
    expect(mass.programs.programs.find((p) => p.path === brave.path)?.category).toBe("study");
    await expect(api.decideProgram(path, "system")).rejects.toMatchObject({ field: "category" });
    mock.state.sessionActive = false;
    await expect(api.listPrograms()).rejects.toBeInstanceOf(CallError);
  });
});
