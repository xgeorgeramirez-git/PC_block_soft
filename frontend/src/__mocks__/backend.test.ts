import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "../api";
import { ADULT_SOURCE_TEXT } from "../lib/sites";
import { DEFAULT_PASSWORD, installBackend, mockChild, statusOf, uninstallBackend } from "./backend";
import type { ProgramDTO } from "../types";

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
  // Вторник 20.10.2026, 12:00 (лимит буднего дня 120 мин): фиксируем день, иначе статус
  // зависит от дня прогона и тест краснеет в дни с другим лимитом (дат-бомба, L-59).
  const now = new Date(2026, 9, 20, 12, 0, 0).getTime();
  const hour = 3600;

  it.each([
    [{ usedS: 2100 }, "ok", "Можно"],
    [{ usedS: 2 * hour - 600 }, "low", "Осталось 10 мин"],
    [{ usedS: 2 * hour }, "timeUp", "Время вышло"],
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
    expect(undone.message).toBe("Изменение доп. времени отменено");

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

  // Ловит: подмена принимает шаг «Сайты» не как служба (тесты мастера зелёные на выдумке, PL-9).
  it("Setup с sites: «18+» выключен у названных детей, видео заменяет набор; не домен - отказ у поля", async () => {
    const mock = installBackend({ configured: false, sites: [{ domain: "example.com", kind: "blocked" }, { domain: "youtube.com", kind: "video_social" }] });
    const limits = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 120, unlimited: false }));
    const kids = [
      { sid: "S-1", name: "Миша", limits },
      { sid: "S-2", name: "Даша", limits },
    ];
    await expect(api.setup("123456", kids, undefined, { adultOff: [], video: ["котики видео"] })).rejects.toMatchObject({
      message: "Это не похоже на адрес сайта",
      field: "domain",
    });
    expect(mock.state.configured).toBe(false);
    await api.setup("123456", kids, undefined, { adultOff: ["S-2"], video: ["vk.com", "https://VK.com/feed", "kids.example"] });
    expect(mock.state.children.map((c) => [c.name, c.adultFilter])).toEqual([
      ["Миша", undefined],
      ["Даша", false],
    ]);
    expect(mock.state.sites).toEqual([
      { domain: "example.com", kind: "blocked" },
      { domain: "vk.com", kind: "video_social" },
      { domain: "kids.example", kind: "video_social" },
    ]);
  });

  // Ловит: подмена теряет лимит видео при сохранении (тесты «Сохранить» зелёные на данных без видео, PL-9).
  it("SaveLimits хранит видео; больше суток - отказ у поля video.N", async () => {
    installBackend({ sessionActive: true });
    const limits = (await api.getChildRules(1)).limits.map((l) => (l.weekday === 1 ? { ...l, videoMin: 20, videoUnlimited: false } : l));
    await api.saveLimits(1, limits);
    expect((await api.getChildRules(1)).limits[0]).toMatchObject({ videoMin: 20, videoUnlimited: false });
    const tooMuch = limits.map((l) => (l.weekday === 2 ? { ...l, videoMin: 1500, videoUnlimited: false } : l));
    await expect(api.saveLimits(1, tooMuch)).rejects.toMatchObject({ message: "Не больше 24 ч", field: "video.2" });
  });

  it("учётки: у уже добавленного ребёнка стоит отметка", async () => {
    installBackend();
    const accounts = await api.listAccounts();
    expect(accounts.find((a) => a.name === "Миша")?.child).toBe(true);
    expect(accounts.find((a) => a.name === "Даша")?.child).toBe(false);
  });
});

describe("согласованность подменной программы", () => {
  // Ловит: на снимках экранов у одного и того же ребёнка разные версии программы на входе и в настройках.
  it("версия по умолчанию одна для Version() и Hello()", async () => {
    installBackend();
    expect(await api.version()).toBe("0.0.1");
    expect((await api.hello()).version).toBe("0.0.1");
  });

  // Ловит: «Сегодня» показывает ребёнка, которого нет среди детских учёток в «Настройках».
  it("каждый ребёнок с «Сегодня» есть в учётках с отметкой «ребёнок», и наоборот", async () => {
    installBackend({
      sessionActive: true,
      children: [
        mockChild(),
        mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша" }),
        mockChild({ childId: 3, sid: "S-1-5-21-111-222-333-1077", name: "Егор" }),
      ],
    });
    const today = (await api.getToday()).children.map((c) => c.name).sort();
    const accounts = await api.listAccounts();
    expect(accounts.filter((a) => a.child).map((a) => a.name).sort()).toEqual(today);
    // Учётка Егора в списке контракта не названа: подменная программа заводит её сама.
    expect(accounts.find((a) => a.name === "Егор")).toMatchObject({ admin: false, canBeChild: true, child: true });
  });

  it("у администратора отметки «ребёнок» нет", async () => {
    installBackend({ sessionActive: true });
    const admin = (await api.listAccounts()).find((a) => a.admin);
    expect(admin?.child).toBe(false);
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

  // Ловит (FR-24, RM5-5): мок не отбирает по группе «Что», повторяет записи на следующей странице,
  // врёт про «Показать ещё» или не даёт записям номера.
  it("журнал: группа «Что», курсор «Показать ещё» и more", async () => {
    const t = Date.now();
    const ev = (atMs: number, kind: string) => ({ atMs, tzOffsetMin: 180, childId: 1, childName: "Миша", kind, text: kind });
    installBackend({
      sessionActive: true,
      events: [ev(t - 1000, "bypass"), ev(t - 2000, "extra_time"), ev(t - 3000, "site_forbidden_try"), ev(t - 4000, "old_kind"), ev(t - 5000, "bypass")],
    });
    const first = await api.listEventsPage({ group: "violations", limit: 2 });
    expect(first.events.map((e) => e.kind)).toEqual(["bypass", "site_forbidden_try"]);
    expect(first.more).toBe(true);
    expect(new Set(first.events.map((e) => e.id)).size).toBe(2);
    const last = first.events[first.events.length - 1];
    const second = await api.listEventsPage({ group: "violations", limit: 2, beforeAtMs: last.atMs, beforeId: last.id });
    expect(second.events.map((e) => e.atMs)).toEqual([t - 5000]);
    expect(second.more).toBe(false);
    expect((await api.listEventsPage({ group: "time" })).events.map((e) => e.kind)).toEqual(["extra_time"]);
    expect((await api.listEventsPage({ group: "system" })).events).toEqual([]);
    expect((await api.listEvents()).map((e) => e.kind)).toContain("old_kind");
    await expect(api.listEventsPage({ group: "nope" as never })).rejects.toMatchObject({ message: "Не удалось разобрать запрос. Попробуйте ещё раз" });
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

  it("неизвестная команда отклоняется", async () => {
    const mock = installBackend({ sessionActive: true });
    expect(JSON.parse(await mock.call("Nope", "{}")).ok).toBe(false);
  });
});

describe("ребёнок о себе: MyToday, FirstRunSeen, Finish", () => {
  // Ловит: подмена отдаёт одни и те же цифры всем детям или считает их мимо общего источника статуса.
  it("MyToday: статус и правила своего ребёнка, без команд родителя и без сессии", async () => {
    installBackend({
      children: [
        mockChild(),
        mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", usedS: 600 }),
      ],
    });
    const mine = await api.myToday();
    expect(mine.status.name).toBe("Миша");
    expect(mine.status).toEqual(statusOf(mockChild(), Date.now()));
    expect(mine.today).toEqual(["Сегодня осталось 1 ч 25 мин из 2 ч", "Использовано 35 мин"]);
    expect(mine.rules.map((r) => [r.day, r.text])).toEqual([
      ["пн", "2 ч"],
      ["вт", "2 ч"],
      ["ср", "1 ч 30 мин"],
      ["чт", "2 ч"],
      ["пт", "2 ч"],
      ["сб", "4 ч"],
      ["вс", "Без лимита"],
      ["", "Игры: пн до 1 ч, ср нельзя, сб до 2 ч"],
      ["", "Видео и соцсети: пн до 1 ч, ср нельзя"],
      ["", "Когда время вышло: экран блокировки"],
    ]);
    expect(mine.rules.map((r) => r.weekday)).toEqual([1, 2, 3, 4, 5, 6, 7, 0, 0, 0]);
  });

  // Ловит: подмена считает видео не как служба (остаток, закрытие, подсказка значка, строка карточки).
  it("видео: остаток с доп. временем, закрытие при нуле, подсказка и строка карточки", () => {
    const st = statusOf(mockChild({ videoUsedS: 600, extraMin: 10, gamesUsedS: 0 }), Date.now());
    expect(st).toMatchObject({ videoUnlimited: false, videoLimitS: 4200, videoRemainingS: 3600, closeVideo: false });
    expect(st.tooltip).toBe("Осталось 1 ч 35 мин\nИгры: 1 ч 10 мин\nВидео: 1 ч 00 мин");
    const used = statusOf(mockChild({ videoUsedS: 3600 }), Date.now());
    expect(used).toMatchObject({ videoRemainingS: 0, closeVideo: true });
    const free = statusOf(mockChild({ limits: [{ weekday: 1, totalMin: 120, unlimited: false }] }), Date.now());
    expect(free).toMatchObject({ videoUnlimited: true, videoLimitS: 0, closeVideo: false });
    expect(free.tooltip).toBe("Осталось 1 ч 25 мин");
    expect(statusOf(mockChild({ admin: true }), Date.now())).toMatchObject({ videoUnlimited: true, closeVideo: false });
  });

  it("GetToday: строка видео как ipc.VideoCardText", async () => {
    installBackend({ sessionActive: true, children: [mockChild({ videoUsedS: 600 })] });
    expect((await api.getToday()).children[0].videoText).toBe("Видео 10 мин из 1 ч");
  });

  it("MyToday: другой ребёнок видит только себя", async () => {
    installBackend({
      myChildId: 2,
      children: [mockChild(), mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", usedS: 600 })],
    });
    const mine = await api.myToday();
    expect(mine.status.name).toBe("Даша");
    expect(JSON.stringify(mine)).not.toContain("Миша");
  });

  it("MyToday: ноль минут - «Нельзя»; время вышло - строка «Можно снова»", async () => {
    const limits = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 0, unlimited: false }));
    installBackend({ children: [mockChild({ limits })] });
    const mine = await api.myToday();
    expect(mine.rules.filter((r) => r.weekday > 0).every((r) => r.text === "Нельзя")).toBe(true);
    expect(mine.today).toContain("Можно снова: завтра");
  });

  it("MyToday из сеанса не ребёнка: пустые списки, а не null", async () => {
    installBackend({ myChildId: null });
    const mine = await api.myToday();
    expect(mine.status.status).toBe("notChild");
    expect(mine.today).toEqual([]);
    expect(mine.rules).toEqual([]);
    expect(mine.programs).toEqual([]);
  });

  // Ловит: подмена считает «Прочее» и «Всего» не как служба (D-7: учёба вне общего, но во «Всего»),
  // теряет время без программы (у службы это строка «Остальное время»).
  it("MyToday: итоги вкладок и время программ как у службы", async () => {
    const programsToday = [{ path: "c:\\games\\game.exe", name: "game", category: "games" as const, seconds: 600 }];
    installBackend({ children: [mockChild({ usedS: 1200, gamesUsedS: 900, studyUsedS: 180, programsToday })] });
    const mine = await api.myToday();
    expect([mine.gamesS, mine.studyS, mine.otherS, mine.allS]).toEqual([900, 180, 300, 1380]);
    expect(mine.programs).toEqual([
      ...programsToday,
      { path: "", name: "Остальное время", category: "games", seconds: 300 },
      { path: "", name: "Остальное время", category: "study", seconds: 180 },
      { path: "", name: "Остальное время", category: "other", seconds: 300 },
    ]);
  });

  // Ловит: подмена оставляет видео в «Прочем» (плитка «Прочее» больше на видео) или теряет «Остальное время» видео.
  it("MyToday: видео за период и «Прочее» без видео, как у службы", async () => {
    const programsToday = [{ path: "youtube.com", name: "youtube.com", category: "video_social" as const, seconds: 600, kind: "site" as const }];
    const history = [{ day: "2026-10-12", totalS: 600, videoS: 300 }];
    installBackend({ children: [mockChild({ usedS: 2100, gamesUsedS: 600, videoUsedS: 900, studyUsedS: 300, programsToday, history })] });
    const today = await api.myToday();
    expect([today.gamesS, today.videoS, today.studyS, today.otherS, today.allS]).toEqual([600, 900, 300, 600, 2400]);
    expect(today.programs.filter((p) => p.category === "video_social")).toEqual([
      ...programsToday,
      { path: "", name: "Остальное время", category: "video_social", seconds: 300 },
    ]);
    const all = await api.myToday("all");
    expect([all.videoS, all.otherS]).toEqual([1200, 900]);
  });

  // Ловит (M5, RM5-4): подмена отдаёт столбики не как служба: сумма столбиков не равна «Итогам», сегодня
  // не последним столбиком, «Всё время» не по месяцам; не ребёнок получает null вместо [].
  it("MyToday: столбики сходятся с итогами, сегодня - последний", async () => {
    const history = [{ day: "2026-10-12", totalS: 600, studyS: 300 }];
    installBackend({ children: [mockChild({ usedS: 1200, studyUsedS: 180, history })] });
    for (const period of ["today", "week", "all"] as const) {
      const mine = await api.myToday(period);
      const bars = mine.bars ?? [];
      expect(bars.reduce((n, b) => n + b.totalS, 0)).toBe(mine.usedS);
      expect(bars.reduce((n, b) => n + b.studyS, 0)).toBe(mine.studyS);
      expect(bars[bars.length - 1].today).toBe(true);
      expect(mine.barsBy).toBe(period === "all" ? "month" : "day");
    }
    expect((await api.myToday("today")).bars).toHaveLength(1);
    installBackend({ myChildId: null });
    const none = await api.myToday("week");
    expect([none.bars, none.violations, none.bonus]).toEqual([[], [], []]);
  });

  it("FirstRunSeen запоминается, Finish отдаёт код окну", async () => {
    const mock = installBackend();
    expect(mock.state.firstRunSeen).toBe(false);
    await api.firstRunSeen();
    expect(mock.state.firstRunSeen).toBe(true);
    expect(mock.state.finishCode).toBeNull();
    await api.finish(0);
    expect(mock.state.finishCode).toBe(0);
  });

  it("Finish без моста ничего не делает", async () => {
    installBackend();
    delete window.go!.ui!.App!.Finish;
    await expect(api.finish(0)).resolves.toBeUndefined();
  });
});

describe("учётка ребёнка стала администратором (R1)", () => {
  it("статус «admin» с тем же словом, что у службы, и без лимитов", () => {
    const status = statusOf(mockChild({ admin: true }), Date.now());
    expect(status).toMatchObject({
      status: "admin",
      statusText: "Администратор: ограничения не действуют",
      lock: "none",
      unlimited: true,
    });
  });

  it("GetToday отдаёт такую карточку", async () => {
    installBackend({ sessionActive: true, children: [mockChild({ admin: true })] });
    const today = await api.getToday();
    expect(today.children[0].status.status).toBe("admin");
  });
});

// Окно ребёнка: «Попросить ещё», «Взять из копилки», «Настройки для родителя» (FR-10, FR-11, D-53).
describe("команды окна ребёнка", () => {
  it("AskMore: ребёнок без номера - отказ, как у службы", async () => {
    installBackend();
    await expect(api.askMore(0, 30, DEFAULT_PASSWORD)).rejects.toMatchObject({ message: "Ребёнок не найден" });
  });

  it("AskMore: неверный пароль - ошибка у поля, верный - доп. время и сообщение", async () => {
    const mock = installBackend();
    await expect(api.askMore(1, 30, "x")).rejects.toMatchObject({ field: "password" });
    expect(mock.state.children[0].extraMin).toBe(0);
    const r = await api.askMore(1, 30, DEFAULT_PASSWORD);
    expect(r.message).toBe("Добавлено 30 минут");
    expect(mock.state.children[0].extraMin).toBe(30);
    await expect(api.askMore(1, 241, DEFAULT_PASSWORD)).rejects.toMatchObject({ field: "minutes" });
  });

  it("SpendBonus: списание из копилки и прибавка к лимиту; сверх баланса отказ", async () => {
    const mock = installBackend({ children: [mockChild({ bonusBalance: 45 })] });
    const r = await api.spendBonus(0, 30);
    expect(r).toMatchObject({ message: "Взято из копилки 30 мин", balanceAfter: 15 });
    expect(mock.state.children[0].extraMin).toBe(30);
    await expect(api.spendBonus(0, 30)).rejects.toMatchObject({ message: "В копилке 15 мин, столько взять нельзя", field: "minutes" });
  });

  it("SpendBonus в запретный интервал: отказ с часом", async () => {
    installBackend({ children: [mockChild({ bonusBalance: 45, forbiddenUntil: "Можно снова: сегодня в 07:00" })] });
    await expect(api.spendBonus(0, 15)).rejects.toMatchObject({ message: "Из копилки можно взять с 07:00" });
  });

  it("запретные часы сейчас: статус forbidden с блокировкой и временем конца", () => {
    const st = statusOf(mockChild({ forbiddenUntil: "Можно снова: сегодня в 07:00" }), Date.now());
    expect(st).toMatchObject({ status: "forbidden", lock: "forbidden", until: "Можно снова: сегодня в 07:00" });
  });

  it("OpenWindow: запоминает вид окна; незнакомый вид отклоняется", async () => {
    const mock = installBackend();
    await expect(api.openWindow("parent")).resolves.toEqual({ opened: true });
    expect(mock.state.openedWindows).toEqual(["parent"]);
    await expect(api.openWindow("нет такого" as never)).rejects.toBeTruthy();
  });
});

// T8-7: «Сайты» отвечают как служба (handlers_m4.go), а не примером контракта: иначе тест экрана
// «удалить» зелёный там, где домен не удаляется (PL-9).
describe("сайты (FR-21)", () => {
  const thousand = () => Array.from({ length: 1000 }, (_, i) => ({ domain: `s${i}.test`, kind: "blocked" as const }));

  it("ListSites без опций: сайты примера, «18+» детей, предел и подпись источника", async () => {
    installBackend({ sessionActive: true });
    const r = await api.listSites();
    expect(r.sites).toEqual([
      { domain: "example.com", kind: "blocked" },
      { domain: "school.example", kind: "study" },
      { domain: "youtube.com", kind: "video_social" },
    ]);
    expect(r.children).toEqual([{ childId: 1, name: "Миша", on: true }]);
    expect(r.blockedMax).toBe(1000);
    expect(r.adultSource).toBe(ADULT_SOURCE_TEXT);
  });

  it("без сессии: «Сессия закончилась. Введите пароль»", async () => {
    installBackend();
    await expect(api.listSites()).rejects.toMatchObject({ message: "Сессия закончилась. Введите пароль" });
  });

  it("AddSite: домен из адреса, повтор и не домен — отказ у поля", async () => {
    installBackend({ sessionActive: true });
    const r = await api.addSite("https://www.Example.org/a", "blocked");
    expect(r).toMatchObject({ message: "Добавлено: example.org", domain: "example.org", moved: false });
    expect(r.sites.sites).toContainEqual({ domain: "example.org", kind: "blocked" });
    await expect(api.addSite("example.org", "blocked")).rejects.toMatchObject({ message: "Этот сайт уже в списке", field: "domain" });
    await expect(api.addSite("просто текст", "blocked")).rejects.toMatchObject({ message: "Это не похоже на адрес сайта", field: "domain" });
    await expect(api.addSite("a.ru", "games" as never)).rejects.toMatchObject({ message: "Не удалось разобрать запрос. Попробуйте ещё раз" });
  });

  it("AddSite: домен из другого списка переносится, запись одна", async () => {
    installBackend({ sessionActive: true });
    const r = await api.addSite("youtube.com", "study");
    expect(r).toMatchObject({ message: "Сайт перенесён в «Учёбу»", domain: "youtube.com", moved: true });
    expect(r.sites.sites.filter((s) => s.domain === "youtube.com")).toEqual([{ domain: "youtube.com", kind: "study" }]);
  });

  it("AddSite: сайты в ответе по списку, затем по домену", async () => {
    installBackend({ sessionActive: true });
    const r = await api.addSite("aaa.test", "video_social");
    expect(r.sites.sites.map((s) => s.domain)).toEqual(["example.com", "school.example", "aaa.test", "youtube.com"]);
  });

  it("AddSite: в «Запрещённых» 1000 — новый не добавляется, отказ у поля", async () => {
    installBackend({ sessionActive: true, sites: thousand() });
    await expect(api.addSite("new.test", "blocked")).rejects.toMatchObject({
      message: "В списке запрещённых уже 1000 сайтов. Удалите ненужные",
      field: "domain",
    });
    await expect(api.addSite("s5.test", "blocked")).rejects.toMatchObject({ message: "Этот сайт уже в списке", field: "domain" });
    await expect(api.addSite("study.test", "study")).resolves.toMatchObject({ message: "Добавлено: study.test" });
  });

  it("RemoveSite: прежний список и «Отменить» на 10 с; повтор — сайта уже нет", async () => {
    installBackend({ sessionActive: true });
    const r = await api.removeSite("example.com");
    expect(r).toMatchObject({ message: "Удалён example.com", domain: "example.com", kind: "blocked" });
    expect(r.undoUntilMs).toBe(Date.now() + 10_000);
    expect(r.sites.sites.map((s) => s.domain)).not.toContain("example.com");
    await expect(api.removeSite("example.com")).rejects.toMatchObject({ message: "Этого сайта уже нет в списках" });
  });

  it("SetAdultFilter: переключатель ребёнка; незнакомый ребёнок — отказ", async () => {
    installBackend({ sessionActive: true });
    const r = await api.setAdultFilter(1, false);
    expect(r.message).toBe("Миша: сайты 18+ не блокируются");
    expect(r.sites.children).toEqual([{ childId: 1, name: "Миша", on: false }]);
    await expect(api.setAdultFilter(9, true)).rejects.toMatchObject({ message: "Такого ребёнка нет в списке" });
  });

  it("MockChild.adultFilter: false — переключатель выключен", async () => {
    installBackend({ sessionActive: true, children: [mockChild({ adultFilter: false })] });
    expect((await api.listSites()).children).toEqual([{ childId: 1, name: "Миша", on: false }]);
  });

  it("журнал: тексты и виды событий, как у службы", async () => {
    const mock = installBackend({ sessionActive: true });
    await api.addSite("https://www.Example.org/a", "blocked");
    await api.addSite("youtube.com", "study");
    await api.removeSite("example.com");
    await api.setAdultFilter(1, false);
    expect(mock.state.events.slice(0, 4).map((e) => [e.kind, e.text])).toEqual([
      ["site_adult_filter", "Миша: сайты 18+ не блокируются"],
      ["site_removed", "Вы удалили сайт example.com из «Запрещённых»"],
      ["site_moved", "Вы перенесли сайт youtube.com в «Учёбу»"],
      ["site_added", "Вы добавили сайт example.org в «Запрещённые»"],
    ]);
    expect(mock.state.events[0].childId).toBe(1);
  });
});

describe("программы: пустая смена категории (B-41)", () => {
  const UCHI = "c:\\program files\\uchi\\uchi.exe";
  const BRAVE = "c:\\program files\\bravesoftware\\brave-browser\\application\\brave.exe";
  const list = (): ProgramDTO[] => [
    { path: UCHI, name: "Учи.ру", publisher: "Учи.ру", category: "study", source: "parent", weekS: 0 },
    { path: BRAVE, name: "Brave", publisher: "Brave Software, Inc.", category: "other", source: "snapshot", weekS: 0 },
  ];
  const undoOf = (r: { message: string; undoId: string; undoUntilMs: number }) => ({
    message: r.message,
    undoId: r.undoId,
    undoUntilMs: r.undoUntilMs,
  });

  // Ловит: мок, как служба до B-41, даёт «Изменено программ: 0» с «Отменить» и пишет журнал.
  it("та же категория решением родителя: «Категория не изменилась» без «Отменить» и журнала", async () => {
    const mock = installBackend({ sessionActive: true, programs: list() });
    const unchanged = { message: "Категория не изменилась", undoId: "", undoUntilMs: 0 };
    expect(undoOf(await api.setProgramsCategory([UCHI], "study"))).toEqual(unchanged);
    expect(undoOf(await api.decideProgram(UCHI, "study"))).toEqual(unchanged);
    expect(mock.state.events.filter((e) => e.kind === "program_decided")).toEqual([]);

    const mixed = await api.setProgramsCategory([UCHI, BRAVE], "study");
    expect(mixed.message).toBe("Изменено программ: 1");
    expect(mixed.undoId).not.toBe("");
  });
});

describe("«Придумать коды»: состав знаков, как у службы (ТЗ 8.3.10)", () => {
  // Ловит: мок придумывает коды с похожими знаками I, O, 0, 1, а служба нет (или «Только цифры» без
  // какой-то цифры) - снимки и тесты экрана показывают не то, что увидит родитель.
  it("с буквами нет I, O, 0, 1; в «Только цифры» все десять цифр", async () => {
    installBackend({ sessionActive: true });
    for (const charset of ["letters", "mixed"] as const) {
      for (let i = 0; i < 20; i++) {
        const r = await api.generatePromos({ count: 50, length: 12, charset, minutes: 10 });
        for (const p of r.promos) expect(p.codeDisplay).toMatch(/^[A-HJ-NP-Z2-9]{12}$/);
      }
    }
    const digits = await api.generatePromos({ count: 50, length: 12, charset: "digits", minutes: 10 });
    const all = digits.promos.map((p) => p.codeDisplay).join("");
    for (const d of "0123456789") expect(all).toContain(d);
  });
});
