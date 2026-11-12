import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Dialog from "./Dialog";

// Модальное окно (ТЗ 8.0 п.8): фокус внутри, Esc закрывает; у необратимого действия фокус на «Отмене».

describe("Dialog", () => {
  // Ловит (8.3.7 п.3): Enter сразу после открытия удаляет статистику, потому что фокус на «Удалить».
  it("фокус на кнопке с data-autofocus", () => {
    render(
      <Dialog
        title="Удалить всю статистику Миши?"
        actions={
          <>
            <button type="button">Удалить</button>
            <button type="button" data-autofocus>
              Отмена
            </button>
          </>
        }
      />,
    );
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Отмена" }));
  });

  it("без data-autofocus - первая кнопка", () => {
    render(
      <Dialog
        title="Обнулить копилку?"
        actions={
          <>
            <button type="button">Обнулить</button>
            <button type="button">Отмена</button>
          </>
        }
      />,
    );
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Обнулить" }));
  });

  it("Esc вызывает onClose", async () => {
    const onClose = vi.fn();
    const user = userEvent.setup();
    render(<Dialog title="Окно" onClose={onClose} actions={<button type="button">Хорошо</button>} />);
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
