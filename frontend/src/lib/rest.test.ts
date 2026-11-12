import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  breakDraft,
  breakEqual,
  breakSummary,
  DEFAULT_BREAK,
  DEFAULT_BREAK_MESSAGE,
  selfRestHint,
  selfRestLabel,
  setBreakNumber,
  toggleBreak,
  validateBreak,
  wireBreak,
} from "./rest";

// Вкладка «Отдых» (FR-37, 8.3.3 п.7): пределы полей как CHECK миграции 008 (D-45). Ожидания — литералы.

const s43 = { enabled: true, afterMin: 60, counts: "games" as const, warnMin: 5, restMin: 15, selfRestMin: 30, message: "Пойди погуляй с собакой" };

describe("черновик «Отдыха»", () => {
  // Ловит: пустое сообщение службы («стандартное») показывается пустым полем, а не текстом по умолчанию.
  it("пустое сообщение показывает стандартный текст", () => {
    expect(DEFAULT_BREAK_MESSAGE).toBe("Разомнись, попей воды, посмотри в окно");
    expect(breakDraft({ ...s43, message: "" }).message).toBe("Разомнись, попей воды, посмотри в окно");
    expect(breakDraft(s43).message).toBe("Пойди погуляй с собакой");
  });

  // Ловит: службе уходит стандартный текст или пробелы вместо «» (стандартное), своё сообщение
  // теряет обрезку пробелов.
  it("в службу: стандартное и пустое — пустая строка", () => {
    expect(wireBreak(breakDraft({ ...s43, message: "" })).message).toBe("");
    expect(wireBreak({ ...s43, message: "   " }).message).toBe("");
    expect(wireBreak({ ...s43, message: "  Пойди погуляй с собакой " }).message).toBe("Пойди погуляй с собакой");
    expect(wireBreak(s43)).toEqual(s43);
  });

  // Ловит: черновик с тем же смыслом (стандартный текст и «») считается изменённым — панель
  // «Есть несохранённые изменения» висит без правок.
  it("сравнение по смыслу", () => {
    expect(breakEqual(breakDraft({ ...s43, message: "" }), { ...s43, message: "" })).toBe(true);
    expect(breakEqual(s43, { ...s43, restMin: 16 })).toBe(false);
    expect(breakEqual(s43, { ...s43, enabled: false })).toBe(false);
  });

  // Ловит: поле принимает буквы или больше верхнего предела; «Отдых» выше порога «сам отошёл»
  // оставляет порог ниже (ошибка на ровном месте) вместо того, чтобы поднять его.
  it("числа: только цифры, верхний предел, порог растёт вместе с отдыхом", () => {
    expect(setBreakNumber(s43, "afterMin", "9а9").afterMin).toBe(99);
    expect(setBreakNumber(s43, "afterMin", "999").afterMin).toBe(240);
    expect(setBreakNumber(s43, "warnMin", "20").warnMin).toBe(15);
    expect(setBreakNumber(s43, "restMin", "100").restMin).toBe(60);
    expect(setBreakNumber(s43, "selfRestMin", "500").selfRestMin).toBe(120);
    expect(setBreakNumber(s43, "afterMin", "").afterMin).toBe(0);
    const up = setBreakNumber(s43, "restMin", "45");
    expect([up.restMin, up.selfRestMin]).toEqual([45, 45]);
    const down = setBreakNumber(up, "restMin", "20");
    expect([down.restMin, down.selfRestMin]).toEqual([20, 45]);
    expect(setBreakNumber(s43, "restMin", "25").selfRestMin).toBe(30);
  });

  // Ловит: значения ниже нижнего предела сохраняются (служба ответит ошибкой без поля), порог
  // «сам отошёл» меньше отдыха проходит, сообщение длиннее 120 символов проходит.
  it("проверка пределов и порога у полей", () => {
    expect(validateBreak(s43)).toEqual({});
    expect(validateBreak({ ...s43, afterMin: 14, warnMin: 0, restMin: 4, selfRestMin: 4 })).toEqual({
      "break.afterMin": "Можно от 15 до 240 мин",
      "break.warnMin": "Можно от 1 до 15 мин",
      "break.restMin": "Можно от 5 до 60 мин",
      "break.selfRestMin": "Можно от 5 до 120 мин",
    });
    expect(validateBreak({ ...s43, restMin: 20, selfRestMin: 19 })).toEqual({ "break.selfRestMin": "Не меньше 20 мин: столько длится отдых" });
    expect(validateBreak({ ...s43, message: "я".repeat(121) })).toEqual({ "break.message": "Не больше 120 символов" });
    expect(validateBreak({ ...s43, message: "я".repeat(120) })).toEqual({});
  });

  // Ловит: выключение теряет настроенные значения или оставляет в недоступных полях ошибку,
  // которую не исправить.
  it("выключатель: значения на месте, неверные — как были сохранены", () => {
    const off = toggleBreak(s43, false, s43);
    expect(off).toEqual({ ...s43, enabled: false });
    const broken = { ...s43, afterMin: 3, restMin: 20, selfRestMin: 18 };
    expect(toggleBreak(broken, false, s43)).toEqual({ ...s43, enabled: false });
    expect(toggleBreak({ ...s43, enabled: false }, true, s43).enabled).toBe(true);
  });

  // Ловит: итог словами расходится с полями (8.3.3 п.7, FR-37): не тот вид счёта, минуты или
  // порог «сам отошёл»; в режиме «Всё время, кроме учёбы» итог обещает «не был за компьютером»,
  // а ядро засчитывает как отдых и время учёбы (R27).
  it("итог словами", () => {
    expect(breakSummary(s43)).toBe(
      "После 60 мин игр ребёнок увидит «Пора отдохнуть», через 5 мин компьютер закроется на 15 мин. Если сам не играл 30 мин подряд, это засчитается как отдых",
    );
    expect(breakSummary(DEFAULT_BREAK)).toBe("Перерыв на отдых выключен");
    expect(breakSummary({ ...DEFAULT_BREAK, enabled: true })).toBe(
      "После 60 мин за компьютером ребёнок увидит «Пора отдохнуть», через 5 мин компьютер закроется на 15 мин. Если сам 15 мин подряд не был за компьютером или занимался только учёбой, это засчитается как отдых",
    );
  });

  // Ловит (B-45): подпись поля «сам отошёл» в режиме «Всё время, кроме учёбы» говорит «не играл»,
  // хотя ядро засчитывает как отдых и время без компьютера, и учёбу (R27, как итог словами).
  it("подпись и подсказка поля отдыха по режиму", () => {
    expect(selfRestLabel("games")).toBe("Засчитать как отдых, если не играл подряд");
    expect(selfRestLabel("all")).toBe("Засчитать как отдых, если не был за компьютером подряд");
    expect(selfRestHint("games")).toBe("Если ребёнок сам не играл столько минут подряд, перерыв уже не нужен");
    expect(selfRestHint("all")).toBe(
      "Если ребёнок столько минут подряд не был за компьютером или занимался только учёбой, перерыв уже не нужен",
    );
  });

  it("по умолчанию как в ТЗ", () => {
    expect(DEFAULT_BREAK).toEqual({ enabled: false, afterMin: 60, counts: "all", warnMin: 5, restMin: 15, selfRestMin: 15, message: "" });
  });
});

// Ловит: образец «Как увидит ребёнок» показывает другую картинку, чем экран перерыва стража
// (копия в окне родителя разошлась с internal/guard/assets/rest-cat.svg).
it("картинка образца — та же, что у стража", () => {
  const guard = readFileSync(resolve(__dirname, "../../../internal/guard/assets/rest-cat.svg"), "utf8");
  const sample = readFileSync(resolve(__dirname, "../assets/rest-cat.svg"), "utf8");
  expect(sample).toBe(guard);
});
