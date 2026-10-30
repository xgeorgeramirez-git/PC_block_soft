import { describe, expect, it } from "vitest";
import { render, screen, within } from "@testing-library/react";
import WhatsNew from "./WhatsNew";

// Раздел «Что нового» (FR-36, ТЗ 8.3.9): только текст, новая версия сверху.

describe("раздел «Что нового»", () => {
  it("заголовок «Что нового»", () => {
    render(<WhatsNew />);
    expect(screen.getByRole("heading", { level: 2 }).textContent).toBe("Что нового");
  });

  // Ловит: версии выведены от старой к новой, и родитель не видит свежие изменения сверху.
  it("версии от новой к старой", () => {
    render(<WhatsNew />);
    expect(screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent)).toEqual([
      "Версия 0.3.0. Программы и игры",
      "Версия 0.2.5. Предупреждения с табло и звуком",
      "Версия 0.2.4. Табло остатка у значка",
      "Версия 0.2.3. Меньше ложных записей и точный учёт",
      "Версия 0.2.2. Бонусы, промокоды и удобный ввод",
      "Версия 0.2.1. Правки после первых недель",
      "Версия 0.2.0. Расписание, доп. время и копилка",
      "Версия 0.1.0. Первая рабочая версия",
    ]);
  });

  it("пункты версии под её названием", () => {
    render(<WhatsNew />);
    const section = screen.getByRole("region", { name: "Версия 0.2.0. Расписание, доп. время и копилка" });
    expect(within(section).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "Запретные часы по дням недели",
      "Кнопка «Попросить ещё» и пауза для одного ребёнка и для всех",
      "Копилка бонусных минут: родитель начисляет, ребёнок берёт сам",
      "Копирование правил с другого ребёнка",
    ]);
  });

  // Ловит: в раздел вернули кнопку, переключатель или ссылку (8.3.9 п.2: только для чтения).
  it("нет кнопок, полей и ссылок", () => {
    const { container } = render(<WhatsNew />);
    expect(container.querySelectorAll("button, input, select, textarea, a").length).toBe(0);
  });
});
