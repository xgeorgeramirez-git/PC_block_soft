import { describe, expect, it } from "vitest";
import {
  ASK_PRESETS,
  PERIODS,
  allWithStudyText,
  periodEmpty,
  periodOf,
  askMinutes,
  EMPTY_TAB,
  TOTALS_TABS,
  firstRunLines,
  spendBlockedNote,
  spendChoices,
  spendConfirmText,
  summaryLines,
  todayRows,
  splitRules,
  tabRows,
  tabSeconds,
  todayRuleWeekday,
  usageRows,
} from "./mytoday";
import { formatDuration } from "./format";
import type { ChildStatus, MyTodayReply, ProgramTime } from "../types";

// Окна ребёнка (FR-12, FR-32): тексты собираются из ответа MyToday, а не пересчитываются.

function status(over: Partial<ChildStatus> = {}): ChildStatus {
  return {
    name: "Миша",
    status: "ok",
    statusText: "Можно",
    label: "85",
    tooltip: "Осталось 1 ч 25 мин",
    summary: "Сегодня осталось 1 ч 25 мин из 2 ч",
    day: "2026-10-19",
    remainingS: 5100,
    limitS: 7200,
    usedS: 2100,
    unlimited: false,
    lock: "none",
    reason: "",
    detail: "",
    until: "",
    pauseUntil: "",
    bonusBalance: 0,
    forbiddenNote: "",
    used: "Сегодня: 35 мин из 2 ч",
    gamesRemainingS: 0,
    gamesLimitS: 0,
    gamesUnlimited: true,
    endMode: "lock",
    closeGames: false,
    studyOnly: false,
    ...over,
  };
}

function reply(over: Partial<ChildStatus> = {}, today: string[] = []): MyTodayReply {
  const st = status(over);
  return {
    status: st,
    today,
    rules: [],
    period: "today",
    usedS: st.usedS,
    gamesS: 0,
    studyS: 0,
    otherS: 0,
    allS: 0,
    extraMin: 0,
    bonusSpentMin: 0,
    programs: [],
    bars: [],
    barsBy: "day",
    violations: [],
    bonus: [],
    statsSinceDay: "",
    statsSinceText: "",
  };
}

describe("firstRunLines: пересказ правил на сегодня", () => {
  // Ловит: пересказ берёт лимит не из ответа службы (расхождение с значком).
  it("лимит на сегодня и где смотреть остаток", () => {
    expect(firstRunLines(reply())).toEqual(["Сегодня можно 2 ч.", "Нажми на значок у часов: увидишь, сколько осталось."]);
  });

  it("доп. время уже входит в лимит дня", () => {
    expect(firstRunLines(reply({ limitS: 9000 }))[0]).toBe("Сегодня можно 2 ч 30 мин.");
  });

  it("лимит меньше часа", () => {
    expect(firstRunLines(reply({ limitS: 1800 }))[0]).toBe("Сегодня можно 30 мин.");
  });

  it("без лимита: значок без цифр, поэтому про значок не говорим", () => {
    expect(firstRunLines(reply({ status: "unlimited", unlimited: true, limitS: 0 }))).toEqual([
      "Сегодня без ограничений по времени.",
    ]);
  });

  it("0 минут на день: пользоваться нельзя", () => {
    expect(firstRunLines(reply({ limitS: 0 }))).toEqual(["Сегодня компьютером пользоваться нельзя."]);
  });

  // Ловит: окно обещает счёт времени там, где ограничения не действуют (администратор, правила не прочитаны).
  it.each(["admin", "safe", "notChild"] as const)("статус %s: честно, что ограничения не действуют", (kind) => {
    expect(firstRunLines(reply({ status: kind, unlimited: true, limitS: 0 }))).toEqual([
      "Сейчас ограничения не действуют. Если это ошибка, скажи родителю.",
    ]);
  });

  // Ловит: запретные часы в M1 (их ещё нет) попадают в пересказ.
  it("запретные часы не упоминаются", () => {
    const text = firstRunLines(reply()).join(" ");
    expect(text).not.toMatch(/запретн|с \d\d:\d\d до/i);
  });
});

describe("summaryLines: «Итоги»", () => {
  // Ловит: «Использовано» показано дважды (в строках и в строке «Использовано сегодня»), а «Сегодня
  // осталось …» повторяет строку «Осталось всего» из верхней панели: три числа рядом путали ребёнка.
  it("убирает «Использовано …» и повтор остатка, пока время есть", () => {
    const r = reply({ usedS: 2100 }, ["Сегодня осталось 1 ч 25 мин из 2 ч", "Использовано 35 мин"]);
    expect(summaryLines(r)).toEqual([]);
  });

  it("строку про «Можно снова» оставляет", () => {
    const r = reply({ usedS: 7200, status: "timeUp" }, [
      "Время на сегодня закончилось",
      "Использовано 2 ч 00 мин",
      "Можно снова: завтра",
    ]);
    expect(summaryLines(r)).toEqual(["Время на сегодня закончилось", "Можно снова: завтра"]);
  });

  it("пустой ответ - пустой список", () => {
    expect(summaryLines(reply({}, []))).toEqual([]);
  });

  // Ловит: во время перерыва на отдых (R27) время не кончилось, а «Сегодня осталось …» не убрана и
  // повторяет «Осталось всего» (у родителя в «Итогах» - дважды).
  it("перерыв на отдых: повтор остатка убран, как когда время есть", () => {
    const r = reply({ usedS: 2100, lock: "rest" }, ["Сегодня осталось 1 ч 25 мин из 2 ч", "Использовано 35 мин"]);
    expect(summaryLines(r)).toEqual([]);
  });
});

// Ловит FB-10 (0.3.1): «Использовано сегодня 22 мин» и вкладка «Всего 25 мин» рядом без объяснения,
// пояснение было только внутри «Всего»; числа в пояснении должны совпадать с теми, что на экране.
describe("allWithStudyText: «Всего» с учёбой", () => {
  const r = (usedS: number, studyS: number, allS: number): MyTodayReply => ({ ...reply({ usedS }), studyS, allS });

  // Ловит (D-61): за неделю пояснение берёт «в счёт лимита» из статуса (сегодня), а не за период.
  it("за неделю - время в счёт лимита за неделю, не за сегодня", () => {
    const week: MyTodayReply = { ...reply({ usedS: 600 }), period: "week", usedS: 3900, studyS: 2400, allS: 6300 };
    expect(allWithStudyText(week)).toBe("Всего = 1 ч 05 мин в счёт лимита + 40 мин учёбы вне лимита");
  });

  it("22 мин в лимите и 3 мин учёбы", () => {
    expect(allWithStudyText(r(1320, 180, 1500))).toBe("Всего = 22 мин в счёт лимита + 3 мин учёбы вне лимита");
  });

  it("часы: «1 ч 05 мин» и «40 мин»", () => {
    expect(allWithStudyText(r(3900, 2400, 6300))).toBe("Всего = 1 ч 05 мин в счёт лимита + 40 мин учёбы вне лимита");
  });

  // Минуты в пояснении - разница чисел на экране: 22 мин 40 с + 2 мин 40 с показаны как 22 и 25 мин.
  it("секунды не теряют минуту: слагаемые дают число «Всего» на экране", () => {
    expect(allWithStudyText(r(1360, 160, 1520))).toBe("Всего = 22 мин в счёт лимита + 3 мин учёбы вне лимита");
  });

  it("без учёбы или когда на экране разницы нет: пусто", () => {
    expect(allWithStudyText(r(1320, 0, 1320))).toBe("");
    expect(allWithStudyText(r(1260, 30, 1290))).toBe("");
  });
});

describe("день недели", () => {
  it("понедельник 19 октября 2026 - 1", () => {
    expect(todayRuleWeekday(new Date(2026, 9, 19, 12, 0, 0))).toBe(1);
  });

  it("воскресенье - 7", () => {
    expect(todayRuleWeekday(new Date(2026, 9, 25, 12, 0, 0))).toBe(7);
  });
});

// Окно «Моя статистика» на «Сегодня» (FR-12, ТЗ 8.2.6 п.1, D-53): строки остатка, лимитов, запретных
// часов и копилки собираются из ответа MyToday, как значок: остаток округляется вверх (B-5).
describe("todayRows: остаток, лимиты, запретные часы, копилка", () => {
  const rows = (over: Partial<ChildStatus> = {}) => Object.fromEntries(todayRows(status(over)).map((r) => [r.label, r.value]));

  // Ловит: остаток в окне расходится со значком и табло (остался 1 ч 25 мин, а в окне другое).
  it("обычный день", () => {
    expect(rows({ bonusBalance: 45, forbiddenNote: "Сегодня нельзя с 21:00 до 07:00" })).toEqual({
      "Осталось всего": "1 ч 25 мин",
      "Игры": "Без отдельного лимита",
      "Видео и соцсети": "Без отдельного лимита",
      "Лимит дня": "2 ч",
      "Запретные часы": "Сегодня нельзя с 21:00 до 07:00",
      "Копилка": "45 мин",
    });
  });

  // Ловит: «осталось 5 с» показано как «0 мин» при ещё идущем времени (единое правило B-5: вверх).
  it("секунды округляются вверх, как у значка", () => {
    expect(rows({ remainingS: 5 })["Осталось всего"]).toBe("1 мин");
    expect(rows({ remainingS: 0, status: "timeUp", lock: "timeUp" })["Осталось всего"]).toBe("0 мин");
  });

  it("доп. время уже в лимите дня; без запретных часов строка честная", () => {
    const r = rows({ limitS: 9000 });
    expect(r["Лимит дня"]).toBe("2 ч 30 мин");
    expect(r["Запретные часы"]).toBe("Сегодня запретных часов нет");
    expect(r["Копилка"]).toBe("0 мин");
  });

  it("без лимита: остаток и лимит не выдуманы", () => {
    const r = rows({ status: "unlimited", unlimited: true, remainingS: 0, limitS: 0 });
    expect(r["Осталось всего"]).toBe("Без ограничений");
    expect(r["Лимит дня"]).toBe("Без лимита");
  });

  it("ограничения не действуют: администратор и непрочитанные правила", () => {
    for (const st of ["admin", "safe"] as const) {
      const r = rows({ status: st, unlimited: true, remainingS: 0, limitS: 0 });
      expect(r["Осталось всего"], st).toBe("Ограничения не действуют");
      expect(r["Лимит дня"], st).toBe("Не действует");
    }
  });

  // R5, Review Focus 5: остаток игр тот же, что в подсказке значка «Игры: 20 мин» (вверх до минуты).
  it("игры с лимитом: остаток из ответа службы и лимит игр дня", () => {
    const g = { gamesUnlimited: false, gamesLimitS: 3600, gamesRemainingS: 1200 };
    expect(rows(g)["Игры"]).toBe("Осталось 20 мин из 1 ч");
    expect(rows({ ...g, gamesRemainingS: 1141 })["Игры"]).toBe("Осталось 20 мин из 1 ч");
    expect(rows({ ...g, gamesRemainingS: 0, closeGames: true })["Игры"]).toBe("Осталось 0 мин из 1 ч");
    // Доп. время уже в лимите игр.
    expect(rows({ ...g, gamesLimitS: 5400 })["Игры"]).toBe("Осталось 20 мин из 1 ч 30 мин");
  });

  it("игры: 0 минут в лимите - «Сегодня нельзя»; время вышло или только учёба - 0", () => {
    expect(rows({ gamesUnlimited: false, gamesLimitS: 0, gamesRemainingS: 0 })["Игры"]).toBe("Сегодня нельзя");
    const g = { gamesUnlimited: false, gamesLimitS: 3600, gamesRemainingS: 1200 };
    expect(rows({ ...g, status: "timeUp", lock: "timeUp" })["Игры"]).toBe("Осталось 0 мин из 1 ч");
    expect(rows({ ...g, studyOnly: true })["Игры"]).toBe("Осталось 0 мин из 1 ч");
    expect(rows({ ...g, status: "admin" })["Игры"]).toBe("Не действует");
  });

  // Ловит: во время перерыва на отдых (R27) остаток показан «0 мин», как будто время вышло.
  it("перерыв на отдых: остаток общего времени и игр не обнуляется", () => {
    const r = rows({ lock: "rest", gamesUnlimited: false, gamesLimitS: 3600, gamesRemainingS: 1200 });
    expect(r["Осталось всего"]).toBe("1 ч 25 мин");
    expect(r["Игры"]).toBe("Осталось 20 мин из 1 ч");
  });

  // R6, Review Focus плана п. 9: остаток видео тот же, что в подсказке значка «Видео: 10 мин».
  it("видео с лимитом: остаток из ответа службы и лимит видео дня", () => {
    const v = { videoUnlimited: false, videoLimitS: 1200, videoRemainingS: 590 };
    expect(rows(v)["Видео и соцсети"]).toBe("Осталось 10 мин из 20 мин");
    expect(rows({ ...v, lock: "rest" })["Видео и соцсети"]).toBe("Осталось 10 мин из 20 мин");
    expect(rows({ ...v, status: "timeUp", lock: "timeUp" })["Видео и соцсети"]).toBe("Осталось 0 мин из 20 мин");
    expect(rows({ ...v, studyOnly: true })["Видео и соцсети"]).toBe("Осталось 0 мин из 20 мин");
    expect(rows({ ...v, status: "admin" })["Видео и соцсети"]).toBe("Не действует");
    expect(rows({ ...v, status: "safe" })["Видео и соцсети"]).toBe("Не действует");
  });

  // Ловит: «лимит 0» без закрытия видео показан «Сегодня нельзя» (у службы это «лимита нет», пример GetToday).
  it("видео: лимит 0 - «нельзя» только при закрытом видео; нет полей - без отдельного лимита", () => {
    expect(rows({ videoUnlimited: false, videoLimitS: 0, closeVideo: true })["Видео и соцсети"]).toBe("Сегодня нельзя");
    expect(rows({ videoUnlimited: false, videoLimitS: 0, closeVideo: false })["Видео и соцсети"]).toBe("Без отдельного лимита");
    expect(rows({ videoUnlimited: true, videoLimitS: 1200, videoRemainingS: 600 })["Видео и соцсети"]).toBe("Без отдельного лимита");
    expect(rows({})["Видео и соцсети"]).toBe("Без отдельного лимита");
  });

  it("запретные часы сейчас: остаток не показывается как «0 мин»", () => {
    const r = rows({ status: "forbidden", lock: "forbidden", remainingS: 3000 });
    expect(r["Осталось всего"]).toBe("Сейчас нельзя");
  });
});

describe("«Взять из копилки» (FR-11)", () => {
  // Ловит: варианты не 15, 30, 60 или весь баланс; больше баланса предлагать нельзя.
  it("15, 30, 60 или весь баланс, но не больше баланса", () => {
    expect(spendChoices(0)).toEqual([]);
    expect(spendChoices(10)).toEqual([{ minutes: 10, label: "Всё: 10 мин" }]);
    expect(spendChoices(15)).toEqual([{ minutes: 15, label: "Всё: 15 мин" }]);
    expect(spendChoices(45)).toEqual([
      { minutes: 15, label: "15 мин" },
      { minutes: 30, label: "30 мин" },
      { minutes: 45, label: "Всё: 45 мин" },
    ]);
    expect(spendChoices(100)).toEqual([
      { minutes: 15, label: "15 мин" },
      { minutes: 30, label: "30 мин" },
      { minutes: 60, label: "60 мин" },
      { minutes: 100, label: "Всё: 100 мин" },
    ]);
  });

  // Ловит: подтверждение не по ТЗ («Взять из копилки 30 мин? Останется 15 мин»).
  it("подтверждение с остатком копилки", () => {
    expect(spendConfirmText(30, 45)).toBe("Взять из копилки 30 мин? Останется 15 мин");
    expect(spendConfirmText(45, 45)).toBe("Взять из копилки 45 мин? Останется 0 мин");
  });

  // Ловит: в запретный интервал кнопка недоступна без пояснения (урок L-17) или с неверным часом.
  it("пояснение, почему из копилки сейчас нельзя", () => {
    const forb = (until: string) => status({ status: "forbidden", lock: "forbidden", until });
    expect(spendBlockedNote(forb("Можно снова: сегодня в 07:00"), 45)).toBe("Из копилки можно взять с 07:00");
    expect(spendBlockedNote(forb("Можно снова: завтра в 07:00"), 45)).toBe("Из копилки можно взять завтра с 07:00");
    expect(spendBlockedNote(forb(""), 45)).toBe("Из копилки можно взять, когда закончатся запретные часы");
    expect(spendBlockedNote(status(), 0)).toBe("В копилке пока пусто");
    expect(spendBlockedNote(status(), 45)).toBe("");
  });
});

describe("«Попросить ещё» (FR-10)", () => {
  it("быстрые кнопки +15, +30, +60", () => {
    expect([...ASK_PRESETS]).toEqual([15, 30, 60]);
  });

  // Ловит: «своё число» принимает 0, 241, дробь и буквы: служба отвечает только 1…240.
  it("своё число минут от 1 до 240", () => {
    expect(askMinutes("1")).toBe(1);
    expect(askMinutes(" 45 ")).toBe(45);
    expect(askMinutes("240")).toBe(240);
    for (const bad of ["", "0", "241", "-5", "1.5", "abc", "12a"]) expect(askMinutes(bad), bad).toBeNull();
  });
});

describe("splitRules: «Мои правила» (R21)", () => {
  it("дни недели в таблицу, общие правила строками по порядку", () => {
    const got = splitRules([
      { weekday: 1, day: "пн", text: "2 ч" },
      { weekday: 7, day: "вс", text: "Без лимита" },
      { weekday: 0, day: "", text: "Игры: до 1 ч в день" },
      { weekday: 0, day: "", text: "Когда время вышло: остаётся только учёба" },
    ]);
    expect(got.days.map((d) => d.weekday)).toEqual([1, 7]);
    expect(got.general).toEqual(["Игры: до 1 ч в день", "Когда время вышло: остаётся только учёба"]);
  });
});

// Вкладки «Итогов» (D-58, S39): сумма категории и каждая программа «название - время - доля».
function totalsReply(programs: ProgramTime[], over: Partial<MyTodayReply> = {}): MyTodayReply {
  return { ...reply(), gamesS: 900, studyS: 180, otherS: 300, allS: 1380, programs, ...over };
}

const S39: ProgramTime[] = [
  { path: "c:\\games\\game.exe", name: "game", category: "games", seconds: 600 },
  { path: "c:\\games\\game2.exe", name: "game2", category: "games", seconds: 300 },
  { path: "c:\\school\\study.exe", name: "study", category: "study", seconds: 180 },
  { path: "c:\\tools\\brave.exe", name: "Brave", category: "other", seconds: 120 },
  { path: "", name: "Остальное время", category: "other", seconds: 180 },
];

describe("вкладки «Итогов»", () => {
  it("вкладки «Всего», «Игры», «Видео и соцсети», «Учёба», «Прочее» с суммами от службы", () => {
    const r = totalsReply(S39);
    expect(TOTALS_TABS.map((t) => [t.label, formatDuration(tabSeconds(r, t.id))])).toEqual([
      ["Всего", "23 мин"],
      ["Игры", "15 мин"],
      ["Видео и соцсети", "0 мин"],
      ["Учёба", "3 мин"],
      ["Прочее", "5 мин"],
    ]);
    expect(formatDuration(tabSeconds(totalsReply(S39, { videoS: 900 }), "video_social"))).toBe("15 мин");
  });

  it("имена списков вкладок для диктора", () => {
    expect(TOTALS_TABS.map((t) => t.list)).toEqual([
      "Всего: по программам и сайтам",
      "Игры: по программам",
      "Видео и соцсети: по сайтам",
      "Учёба: по программам",
      "Прочее: по программам",
    ]);
  });

  // Ловит (PL-28): «Всего» без сайтов не сходится с плиткой; у видео «Остальное время» не последнее.
  it("сайты: строки «Видео и соцсетей» и во «Всего», «Остальное время» последним", () => {
    const sites: ProgramTime[] = [
      { path: "youtube.com", name: "youtube.com", category: "video_social", seconds: 600, kind: "site" },
      { path: "vk.com", name: "vk.com", category: "video_social", seconds: 240, kind: "site" },
      { path: "", name: "Остальное время", category: "video_social", seconds: 60 },
    ];
    const r = totalsReply([...S39, ...sites], { videoS: 900, allS: 2280 });
    expect(tabRows(r, "video_social").map((x) => [x.name, x.time, x.share])).toEqual([
      ["youtube.com", "10 мин", "67%"],
      ["vk.com", "4 мин", "27%"],
      ["Остальное время", "1 мин", "7%"],
    ]);
    const all = tabRows(r, "all");
    expect(all.map((x) => x.name)).toEqual(["game", "youtube.com", "game2", "vk.com", "study", "Brave", "Остальное время"]);
    expect(all.find((x) => x.name === "youtube.com")?.key).toBe("youtube.com");
    expect(all.at(-1)).toMatchObject({ time: "4 мин", rest: true });
  });

  // Ловит (S39): доля не от суммы категории, проценты дробные или не округлены; не по убыванию.
  it("«Игры»: game 10 мин 67%, game2 5 мин 33%", () => {
    expect(tabRows(totalsReply(S39), "games").map((x) => [x.name, x.time, x.share])).toEqual([
      ["game", "10 мин", "67%"],
      ["game2", "5 мин", "33%"],
    ]);
  });

  it("«Учёба»: study 3 мин 100%", () => {
    expect(tabRows(totalsReply(S39), "study").map((x) => [x.name, x.time, x.share])).toEqual([["study", "3 мин", "100%"]]);
  });

  // Ловит: «Остальное время» встаёт по времени выше программы (это не программа) или пропадает.
  it("«Прочее»: «Остальное время» последней строкой, даже если дольше", () => {
    expect(tabRows(totalsReply(S39), "other").map((x) => [x.name, x.time, x.share])).toEqual([
      ["Brave", "2 мин", "40%"],
      ["Остальное время", "3 мин", "60%"],
    ]);
  });

  // Ловит: «Всего» показывает «Остальное время» каждой категории отдельной строкой, программа из двух
  // категорий (сменили категорию днём) идёт двумя строками, доли не от «Всего».
  it("«Всего»: все программы, одна программа одной строкой, «Остальное время» одно и последнее", () => {
    const rows = [
      ...S39,
      { path: "c:\\games\\game.exe", name: "game", category: "other" as const, seconds: 60 },
      { path: "", name: "Остальное время", category: "games" as const, seconds: 60 },
    ];
    expect(tabRows(totalsReply(rows, { allS: 1500 }), "all").map((x) => [x.name, x.time, x.share])).toEqual([
      ["game", "11 мин", "44%"],
      ["game2", "5 мин", "20%"],
      ["study", "3 мин", "12%"],
      ["Brave", "2 мин", "8%"],
      ["Остальное время", "4 мин", "16%"],
    ]);
  });

  // Ловит: сумма округлённых долей обязана быть 100 и подгоняется (8.2.6: округление честное, 99-101).
  it("три равные программы: по 33%, сумма 99", () => {
    const three: ProgramTime[] = ["a", "b", "c"].map((n) => ({ path: `c:\\${n}.exe`, name: n, category: "games" as const, seconds: 200 }));
    expect(tabRows(totalsReply(three, { gamesS: 600 }), "games").map((x) => x.share)).toEqual(["33%", "33%", "33%"]);
  });

  it("меньше минуты: «меньше 1 мин», а не «0 мин»", () => {
    const one: ProgramTime[] = [{ path: "c:\\a.exe", name: "a", category: "games", seconds: 59 }];
    expect(tabRows(totalsReply(one, { gamesS: 59 }), "games").map((x) => [x.time, x.share])).toEqual([["меньше 1 мин", "100%"]]);
  });

  it("пустая категория и старая служба без списка: строк нет", () => {
    expect(tabRows(totalsReply([], { gamesS: 0 }), "games")).toEqual([]);
    expect(tabRows(totalsReply(S39.filter((x) => x.category !== "study"), { studyS: 0 }), "study")).toEqual([]);
    expect(EMPTY_TAB).toBe("Пока ничего не запускалось");
  });
});

// Периоды «Итогов» (8.2.6 п.2-3, D-61): подписи строки «Использовано» и сумм вкладок, пустой период.
describe("периоды «Итогов»", () => {
  it("пять периодов по порядку", () => {
    expect(PERIODS.map((p) => p.label)).toEqual(["Сегодня", "Неделя", "Месяц", "Год", "Всё время"]);
    expect(PERIODS.map((p) => p.id)).toEqual(["today", "week", "month", "year", "all"]);
  });

  // Ловит: на неделе строка «Использовано сегодня» (8.2.6 п.3) или «Игры за сегодня» под суммой недели.
  it("подписи «Использовано …» и сумм вкладок", () => {
    expect(PERIODS.map((p) => p.used)).toEqual([
      "Использовано сегодня",
      "Использовано за неделю",
      "Использовано за месяц",
      "Использовано за год",
      "Использовано за всё время",
    ]);
    expect(PERIODS.map((p) => p.sum)).toEqual(["за сегодня", "за неделю", "за месяц", "за год", "за всё время"]);
    expect(periodOf("year").used).toBe("Использовано за год");
    expect(periodOf("нет такого" as never).id).toBe("today");
  });

  // Ловит: пустая неделя показывает нули во вкладках вместо «За этот период пока нет данных» (8.2.6 п.9);
  // «Сегодня» с нулями пустым не считается (вкладки и «Пока ничего не запускалось», как раньше).
  it("пустой период", () => {
    expect(periodEmpty({ ...reply({ usedS: 0 }), period: "week" })).toBe(true);
    expect(periodEmpty({ ...reply({ usedS: 0 }), period: "today" })).toBe(false);
    expect(periodEmpty({ ...reply({ usedS: 0 }), period: "all", allS: 60 })).toBe(false);
    expect(periodEmpty({ ...reply({ usedS: 0 }), period: "month", extraMin: 15 })).toBe(false);
  });
});

// «Программы» и «Сайты» за период (8.2.6 п.5, RM5-4): строки G15 (testdata/golden/G15.json) за неделю.
describe("usageRows: таблицы «Программы» и «Сайты»", () => {
  const G15_WEEK: ProgramTime[] = [
    { path: "c:\\games\\game.exe", name: "game.exe", category: "games", seconds: 285 * 60, kind: "program" },
    { path: "youtube.com", name: "youtube.com", category: "video_social", seconds: 220 * 60, kind: "site" },
    { path: "c:\\school\\study.exe", name: "study.exe", category: "study", seconds: 95 * 60, kind: "program" },
    { path: "c:\\tools\\other.exe", name: "other.exe", category: "other", seconds: 248 * 60, kind: "program" },
  ];
  const week = (programs: ProgramTime[]): MyTodayReply => ({ ...reply(), period: "week", programs });
  const cells = (rows: ReturnType<typeof usageRows>) => rows.map((r) => [r.name, r.time, r.share]);

  // Ловит (S21): порядок не по времени, доля от «Всего» за период, а не от суммы таблицы.
  it("G15 «Программы»: game 4 ч 45 мин 45 %, other 4 ч 08 мин 39 %, study 1 ч 35 мин 15 %", () => {
    expect(cells(usageRows(week(G15_WEEK), "program"))).toEqual([
      ["game.exe", "4 ч 45 мин", "45%"],
      ["other.exe", "4 ч 08 мин", "39%"],
      ["study.exe", "1 ч 35 мин", "15%"],
    ]);
  });

  it("G15 «Сайты»: youtube.com 3 ч 40 мин 100 %", () => {
    expect(cells(usageRows(week(G15_WEEK), "site"))).toEqual([["youtube.com", "3 ч 40 мин", "100%"]]);
  });

  // Ловит (RM5-4): программа сменила категорию за период и стоит двумя строками.
  it("программа в двух категориях - одна строка 15 мин", () => {
    const rows = usageRows(
      week([
        { path: "c:\\x\\app.exe", name: "app", category: "games", seconds: 600, kind: "program" },
        { path: "c:\\x\\app.exe", name: "app", category: "other", seconds: 300, kind: "program" },
      ]),
      "program",
    );
    expect(cells(rows)).toEqual([["app", "15 мин", "100%"]]);
  });

  // Ловит: «Остальное время» (время без программы) - строкой таблицы, а сайт - в «Программах».
  it("«Остальное время» не строка; сайт не в «Программах»; без kind с путём - программа", () => {
    const r = week([
      { path: "", name: "Остальное время", category: "other", seconds: 900 },
      { path: "c:\\x\\old.exe", name: "old", category: "games", seconds: 120 },
      { path: "vk.com", name: "vk.com", category: "video_social", seconds: 60, kind: "site" },
    ]);
    expect(cells(usageRows(r, "program"))).toEqual([["old", "2 мин", "100%"]]);
    expect(cells(usageRows(r, "site"))).toEqual([["vk.com", "1 мин", "100%"]]);
  });

  it("при равном времени - по имени; нулевые строки выкинуты; меньше минуты словами", () => {
    const rows = usageRows(
      week([
        { path: "c:\\b.exe", name: "Бета", category: "other", seconds: 30, kind: "program" },
        { path: "c:\\a.exe", name: "Альфа", category: "other", seconds: 30, kind: "program" },
        { path: "c:\\z.exe", name: "Ноль", category: "other", seconds: 0, kind: "program" },
      ]),
      "program",
    );
    expect(cells(rows)).toEqual([
      ["Альфа", "меньше 1 мин", "50%"],
      ["Бета", "меньше 1 мин", "50%"],
    ]);
  });

  it("пусто - пустой список", () => {
    expect(usageRows(week([]), "program")).toEqual([]);
    expect(usageRows(week([]), "site")).toEqual([]);
  });
});
