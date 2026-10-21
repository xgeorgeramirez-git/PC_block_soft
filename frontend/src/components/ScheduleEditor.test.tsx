import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ScheduleEditor from "./ScheduleEditor";
import type { Interval } from "../types";

// FR-17: список интервалов по дням, добавление с проверкой, полоса суток, копирование.

function setup(schedule: Interval[] = []) {
  const onChange = vi.fn<(next: Interval[]) => void>();
  const user = userEvent.setup();
  render(<ScheduleEditor childName="Миша" schedule={schedule} onChange={onChange} />);
  return { user, onChange };
}

describe("ScheduleEditor", () => {
  it("добавляет интервал по кнопке", async () => {
    const { user, onChange } = setup();
    // Первый день — понедельник; поля «с» и «до».
    const from = screen.getAllByLabelText(/Начало запретного времени/)[0];
    const to = screen.getAllByLabelText(/Конец запретного времени/)[0];
    await user.type(from, "21:00");
    await user.type(to, "07:00");
    await user.click(screen.getAllByRole("button", { name: "Добавить интервал" })[0]);
    expect(onChange).toHaveBeenCalled();
    const next = onChange.mock.calls[0][0];
    expect(next.some((iv) => iv.startMin === 1260 && iv.endMin === 420)).toBe(true);
  });

  it("ошибка когда начало равно концу — у поля, onChange не зовётся", async () => {
    const { user, onChange } = setup();
    const from = screen.getAllByLabelText(/Начало запретного времени/)[0];
    const to = screen.getAllByLabelText(/Конец запретного времени/)[0];
    await user.type(from, "10:00");
    await user.type(to, "10:00");
    await user.click(screen.getAllByRole("button", { name: "Добавить интервал" })[0]);
    expect(screen.getByText("Начало и конец совпадают")).toBeTruthy();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("показывает существующий интервал и удаляет его", async () => {
    const { user, onChange } = setup([{ weekday: 1, startMin: 1260, endMin: 420 }]);
    expect(screen.getByText("с 21:00 до 07:00")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: /Удалить интервал 21:00/ }));
    expect(onChange).toHaveBeenCalled();
    expect(onChange.mock.calls[0][0].filter((iv) => iv.weekday === 1)).toHaveLength(0);
  });

  it("мусор во времени даёт понятную ошибку", async () => {
    const { user, onChange } = setup();
    const from = screen.getAllByLabelText(/Начало запретного времени/)[0];
    await user.type(from, "абв");
    await user.click(screen.getAllByRole("button", { name: "Добавить интервал" })[0]);
    expect(screen.getByText(/не похоже на часы/)).toBeTruthy();
    expect(onChange).not.toHaveBeenCalled();
  });
});
