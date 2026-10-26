import { describe, expect, it } from "vitest";
import {
  ASK_PRESETS,
  askMinutes,
  firstRunLines,
  spendBlockedNote,
  spendChoices,
  spendConfirmText,
  summaryLines,
  todayRows,
  todayRuleWeekday,
} from "./mytoday";
import type { ChildStatus, MyTodayReply } from "../types";

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
    ...over,
  };
}

function reply(over: Partial<ChildStatus> = {}, today: string[] = []): MyTodayReply {
  return { status: status(over), today, rules: [] };
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
  // Ловит: «Использовано» показано дважды, в строках и в строке «Всего».
  it("убирает строку «Использовано …», она есть в «Всего»", () => {
    const r = reply({ usedS: 2100 }, ["Сегодня осталось 1 ч 25 мин из 2 ч", "Использовано 35 мин"]);
    expect(summaryLines(r)).toEqual(["Сегодня осталось 1 ч 25 мин из 2 ч"]);
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
      "Игры": "Пока не считается",
      "Видео и соцсети": "Пока не считается",
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
