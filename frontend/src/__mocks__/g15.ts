// Эталонная неделя статистики G15 (docs/04_Эталоны_учёта.md, testdata/golden/G15.json; S21, D-69, D-78)
// для подменной программы: прошлые дни как их хранит служба (day_state и usage_daily) и сегодня из ядра.
// Отдельный модуль без зависимостей подменной программы: им пользуются и vitest, и снимки Playwright.
import { localOffsetMin } from "../lib/format";
import type { BonusEntry, JournalEntry, ProgramTime, WeekdayLimit } from "../types";
import type { MockChild, MockDay } from "./backend";

/** «Сейчас» G15: воскресенье 22.11.2026 15:00 по местному времени. */
export const G15_NOW = new Date(2026, 10, 22, 15, 0, 0);

const GAME = "c:\\games\\game.exe";
const STUDY = "c:\\school\\study.exe";
const OTHER = "c:\\tools\\other.exe";
const SITE = "youtube.com";

/** Лимиты G15: пн-чт 2 ч, пт 3 ч, сб-вс 4 ч; игры и видео без отдельного лимита. */
export const G15_LIMITS: WeekdayLimit[] = [120, 120, 120, 120, 180, 240, 240].map((totalMin, i) => ({
  weekday: i + 1,
  totalMin,
  unlimited: false,
}));

/** Время программ дня, как отдаёт служба: game.exe - «Игры», study.exe - «Учёба», other.exe - «Прочее», youtube.com - сайт. */
function programs(gamesMin: number, videoMin: number, studyMin: number, otherMin: number): ProgramTime[] {
  const rows: ProgramTime[] = [];
  if (gamesMin) rows.push({ path: GAME, name: "game.exe", category: "games", seconds: gamesMin * 60, kind: "program" });
  if (videoMin) rows.push({ path: SITE, name: SITE, category: "video_social", seconds: videoMin * 60, kind: "site" });
  if (studyMin) rows.push({ path: STUDY, name: "study.exe", category: "study", seconds: studyMin * 60, kind: "program" });
  if (otherMin) rows.push({ path: OTHER, name: "other.exe", category: "other", seconds: otherMin * 60, kind: "program" });
  return rows;
}

function day(d: string, extraMin: number, gamesMin: number, videoMin: number, studyMin: number, otherMin: number, bonusSpentMin = 0): MockDay {
  return {
    day: d,
    totalS: (gamesMin + videoMin + otherMin) * 60,
    gamesS: gamesMin * 60,
    videoS: videoMin * 60,
    studyS: studyMin * 60,
    extraMin,
    bonusSpentMin,
    programs: programs(gamesMin, videoMin, studyMin, otherMin),
  };
}

/** Прошлые дни недели G15 (пн 16 … сб 21); среда - строка дня с нулями. */
export function g15History(): MockDay[] {
  return [
    day("2026-11-16", 0, 50, 20, 30, 25),
    day("2026-11-17", 30, 60, 30, 0, 50),
    day("2026-11-18", 0, 0, 0, 0, 0),
    day("2026-11-19", 15, 45, 40, 45, 43, 15),
    day("2026-11-20", 0, 60, 60, 20, 60),
    day("2026-11-21", 0, 60, 70, 0, 70),
  ];
}

const at = (y: number, m: number, d: number, h: number, min = 0) => new Date(y, m, d, h, min).getTime();

/** Копилка недели: трата 15 мин в чт 19 и автобонус 20 мин в сб 21 (новые сверху). */
export function g15Bonus(): BonusEntry[] {
  const bonus = at(2026, 10, 21, 0, 0);
  const spend = at(2026, 10, 19, 16, 0);
  return [
    { atMs: bonus, tzOffsetMin: localOffsetMin(new Date(bonus)), deltaMin: 20, reason: "Бонус за свободное время: 20 мин", balanceAfter: 25 },
    { atMs: spend, tzOffsetMin: localOffsetMin(new Date(spend)), deltaMin: -15, reason: "взято из копилки", balanceAfter: 5 },
  ];
}

/** Поля ребёнка G15: сегодня game.exe 10 мин (из ядра), прошлые дни g15History, копилка 25 мин. */
export function g15Fields(): Partial<MockChild> {
  return {
    limits: G15_LIMITS.map((l) => ({ ...l })),
    usedS: 600,
    gamesUsedS: 600,
    studyUsedS: 0,
    videoUsedS: 0,
    extraMin: 0,
    programsToday: programs(10, 0, 0, 0),
    history: g15History(),
    bonusBalance: 25,
    bonusLedger: g15Bonus(),
  };
}

/** Журнал недели G15: одно нарушение (чт 19, 18:05) и одно обычное событие (не нарушение). */
export function g15Events(childId = 1, name = "Миша"): Omit<JournalEntry, "id">[] {
  const forbidden = at(2026, 10, 19, 18, 5);
  const extra = at(2026, 10, 17, 17, 30);
  return [
    {
      atMs: forbidden,
      tzOffsetMin: localOffsetMin(new Date(forbidden)),
      childId,
      childName: name,
      kind: "program_forbidden_try",
      text: `${name}: попытка открыть запрещённую программу Steam. Программа закрыта`,
    },
    { atMs: extra, tzOffsetMin: localOffsetMin(new Date(extra)), childId, childName: name, kind: "extra_time", text: `${name}: доп. время +30 мин` },
  ];
}
