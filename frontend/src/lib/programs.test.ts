import { describe, expect, it } from "vitest";
import {
  CATEGORY_OPTIONS,
  DECISIONS,
  FILTERS,
  attemptsText,
  categoryTitle,
  decisionMessage,
  publisherText,
  shortenPath,
  timesWord,
  visiblePrograms,
  wasCategoryText,
  weekText,
  whenText,
} from "./programs";
import type { ProgramDTO } from "../types";

// FR-20, ТЗ 8.3.4: подписи, путь, «Запускали: …», время за неделю, поиск, фильтр, сортировка.

const MSK = 180;
// пт, 30 октября 2026, 12:00 по Москве.
const NOW = Date.UTC(2026, 9, 30, 9, 0, 0);

const prog = (over: Partial<ProgramDTO>): ProgramDTO => ({
  path: "c:\\program files\\x\\x.exe",
  name: "X",
  publisher: "",
  category: "other",
  source: "snapshot",
  weekS: 0,
  ...over,
});

describe("подписи категорий", () => {
  it("кнопки решения: Игра, Учёба, Прочее, Запретить, в этом порядке и с этими категориями", () => {
    expect(DECISIONS).toEqual([
      { category: "games", label: "Игра" },
      { category: "study", label: "Учёба" },
      { category: "other", label: "Прочее" },
      { category: "blocked", label: "Запретить" },
    ]);
  });

  it("выбор в строке: Игры, Учёба, Прочее, Запрещено; системной категории нет", () => {
    expect(CATEGORY_OPTIONS).toEqual([
      { category: "games", label: "Игры" },
      { category: "study", label: "Учёба" },
      { category: "other", label: "Прочее" },
      { category: "blocked", label: "Запрещено" },
    ]);
  });

  it("фильтр: Все, Игры, Учёба, Прочее, Запрещено", () => {
    expect(FILTERS.map((f) => f.label)).toEqual(["Все", "Игры", "Учёба", "Прочее", "Запрещено"]);
    expect(FILTERS.map((f) => f.id)).toEqual(["all", "games", "study", "other", "blocked"]);
  });

  it("название категории для строки", () => {
    expect(categoryTitle("games")).toBe("Игры");
    expect(categoryTitle("blocked")).toBe("Запрещено");
    expect(categoryTitle("system")).toBe("");
  });

  it("«Раньше было: игра» по коду категории, пусто без пометки", () => {
    expect(wasCategoryText("games")).toBe("Раньше было: игра");
    expect(wasCategoryText("study")).toBe("Раньше было: учёба");
    expect(wasCategoryText("other")).toBe("Раньше было: прочее");
    expect(wasCategoryText("blocked")).toBe("Раньше было: запрещена");
    expect(wasCategoryText("")).toBe("");
    // Служба прислала уже слово, а не код: показываем его, но строчными.
    expect(wasCategoryText("Игра")).toBe("Раньше было: игра");
    // Системная категория родителю не показывается.
    expect(wasCategoryText("system")).toBe("");
  });

  it("издатель из файла или «Без подписи издателя»", () => {
    expect(publisherText("Mojang")).toBe("Mojang");
    expect(publisherText("")).toBe("Без подписи издателя");
    expect(publisherText("   ")).toBe("Без подписи издателя");
  });

  it("сообщение решения как в ТЗ 8.3.4 п.2", () => {
    expect(decisionMessage("SuperGame", "games")).toBe("SuperGame: игра");
    expect(decisionMessage("SuperGame", "study")).toBe("SuperGame: учёба");
    expect(decisionMessage("SuperGame", "other")).toBe("SuperGame: прочее");
    expect(decisionMessage("SuperGame", "blocked")).toBe("SuperGame: запрещена");
  });
});

describe("путь: обрезка по центру", () => {
  it("короткий путь не трогает", () => {
    expect(shortenPath("c:\\games\\a.exe", 40)).toBe("c:\\games\\a.exe");
  });

  it("длинный: начало, многоточие, конец с именем файла; длина не больше предела", () => {
    const path = "c:\\users\\миша\\appdata\\local\\programs\\очень длинная папка игры\\версия 1.2.3\\supergame.exe";
    const short = shortenPath(path, 48);
    expect(short).toBe("c:\\users\\миша\\appdat…\\версия 1.2.3\\supergame.exe");
    expect(short.length).toBe(48);
  });

  it("очень длинное имя файла: всё равно предел и многоточие посередине", () => {
    const path = `c:\\downloads\\${"a".repeat(80)}.exe`;
    const short = shortenPath(path, 30);
    expect(short.length).toBe(30);
    expect(short.startsWith("c:\\downloads")).toBe(true);
    expect(short.endsWith("aaa.exe")).toBe(true);
    expect(short).toContain("…");
  });
});

describe("кто запускал и когда", () => {
  it("склонение «раз»", () => {
    expect([1, 2, 3, 4, 5, 11, 12, 14, 21, 22, 25, 101, 111].map((n) => `${n} ${timesWord(n)}`)).toEqual([
      "1 раз",
      "2 раза",
      "3 раза",
      "4 раза",
      "5 раз",
      "11 раз",
      "12 раз",
      "14 раз",
      "21 раз",
      "22 раза",
      "25 раз",
      "101 раз",
      "111 раз",
    ]);
  });

  it("сегодня, вчера и раньше", () => {
    expect(whenText(Date.UTC(2026, 9, 30, 6, 5), NOW, MSK)).toBe("сегодня 09:05");
    expect(whenText(Date.UTC(2026, 9, 29, 15, 40), NOW, MSK)).toBe("вчера 18:40");
    // 28 октября 23:30 по Москве: позавчера, дата словами.
    expect(whenText(Date.UTC(2026, 9, 28, 20, 30), NOW, MSK)).toBe("28 окт, 23:30");
    // Граница суток: 00:10 по Москве 30-го (21:10 UTC 29-го) это уже «сегодня».
    expect(whenText(Date.UTC(2026, 9, 29, 21, 10), NOW, MSK)).toBe("сегодня 00:10");
  });

  it("«Запускали: Миша 3 раза, Аня 1 раз; последняя попытка вчера 18:40»", () => {
    const byChild = [
      { childId: 1, name: "Миша", attempts: 3 },
      { childId: 2, name: "Аня", attempts: 1 },
    ];
    expect(attemptsText(byChild, 4, Date.UTC(2026, 9, 29, 15, 40), NOW, MSK)).toBe(
      "Запускали: Миша 3 раза, Аня 1 раз; последняя попытка вчера 18:40",
    );
  });

  it("без списка детей: число попыток; без времени: без хвоста", () => {
    expect(attemptsText([], 4, 0, NOW, MSK)).toBe("Запускали 4 раза");
    expect(attemptsText([], 0, 0, NOW, MSK)).toBe("");
  });
});

describe("время за неделю", () => {
  it("«2 ч 05 мин», «40 мин», меньше минуты, ничего", () => {
    expect(weekText(2 * 3600 + 5 * 60 + 30)).toBe("2 ч 05 мин");
    expect(weekText(40 * 60)).toBe("40 мин");
    expect(weekText(30)).toBe("меньше минуты");
    expect(weekText(0)).toBe("нет");
  });
});

describe("список: поиск, фильтр, сортировка", () => {
  const list: ProgramDTO[] = [
    prog({ path: "c:\\program files\\brave\\brave.exe", name: "Brave", category: "other", weekS: 5400 }),
    prog({ path: "c:\\program files\\minecraft launcher\\minecraftlauncher.exe", name: "Minecraft Launcher", category: "games", weekS: 0 }),
    prog({ path: "c:\\users\\миша\\downloads\\учи.exe", name: "Учи.ру", category: "study", weekS: 1800 }),
    prog({ path: "c:\\windows\\system32\\notepad.exe", name: "Блокнот", category: "system", weekS: 600 }),
    prog({ path: "c:\\users\\миша\\downloads\\bad.exe", name: "Bad", category: "blocked", weekS: 60 }),
  ];
  const names = (l: ProgramDTO[]) => l.map((p) => p.name);
  const base = { query: "", filter: "all" as const, sort: { key: "name" as const, dir: "asc" as const } };

  it("системные программы не показываются никогда", () => {
    expect(names(visiblePrograms(list, base))).toEqual(["Bad", "Brave", "Minecraft Launcher", "Учи.ру"]);
  });

  it("фильтр по категории", () => {
    expect(names(visiblePrograms(list, { ...base, filter: "games" }))).toEqual(["Minecraft Launcher"]);
    expect(names(visiblePrograms(list, { ...base, filter: "study" }))).toEqual(["Учи.ру"]);
    expect(names(visiblePrograms(list, { ...base, filter: "other" }))).toEqual(["Brave"]);
    expect(names(visiblePrograms(list, { ...base, filter: "blocked" }))).toEqual(["Bad"]);
  });

  it("поиск по названию и по пути, без учёта регистра и пробелов по краям", () => {
    expect(names(visiblePrograms(list, { ...base, query: "  brAVE " }))).toEqual(["Brave"]);
    expect(names(visiblePrograms(list, { ...base, query: "DOWNLOADS" }))).toEqual(["Bad", "Учи.ру"]);
    expect(names(visiblePrograms(list, { ...base, query: "учи" }))).toEqual(["Учи.ру"]);
    // Блокнот системный: по поиску тоже не находится.
    expect(names(visiblePrograms(list, { ...base, query: "notepad" }))).toEqual([]);
  });

  it("поиск и фильтр вместе", () => {
    expect(names(visiblePrograms(list, { ...base, query: "downloads", filter: "study" }))).toEqual(["Учи.ру"]);
  });

  it("сортировка по названию в обе стороны", () => {
    expect(names(visiblePrograms(list, { ...base, sort: { key: "name", dir: "desc" } }))).toEqual([
      "Учи.ру",
      "Minecraft Launcher",
      "Brave",
      "Bad",
    ]);
  });

  it("сортировка по времени за неделю: больше сверху, равные по названию", () => {
    const more = [...list, prog({ path: "c:\\a\\a.exe", name: "Aaa", category: "other", weekS: 0 })];
    expect(names(visiblePrograms(more, { ...base, sort: { key: "week", dir: "desc" } }))).toEqual([
      "Brave",
      "Учи.ру",
      "Bad",
      "Aaa",
      "Minecraft Launcher",
    ]);
    expect(names(visiblePrograms(more, { ...base, sort: { key: "week", dir: "asc" } }))).toEqual([
      "Aaa",
      "Minecraft Launcher",
      "Bad",
      "Учи.ру",
      "Brave",
    ]);
  });

  it("не меняет исходный список", () => {
    const copy = list.map((p) => ({ ...p }));
    visiblePrograms(list, { ...base, sort: { key: "week", dir: "desc" } });
    expect(list).toEqual(copy);
  });
});
