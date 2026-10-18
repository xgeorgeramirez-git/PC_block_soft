import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

const getVersion = vi.fn<() => Promise<string>>();
vi.mock("./api", () => ({ getVersion: () => getVersion() }));

import App from "./App";

describe("окно-скелет", () => {
  beforeEach(() => {
    getVersion.mockReset();
  });

  // Ловит: окно не показывает название программы (пустое окно или чужой заголовок).
  it("показывает название программы", async () => {
    getVersion.mockResolvedValue("0.0.1");
    render(<App />);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("PC Block");
  });

  // Ловит: версия не берётся из программы (захардкожена во фронте или не выводится).
  it("показывает версию, полученную от программы", async () => {
    getVersion.mockResolvedValue("9.8.7");
    render(<App />);
    expect(await screen.findByText("Версия 9.8.7")).toBeTruthy();
  });

  // Ловит: сбой связи с программой роняет окно или показывает технический текст.
  it("при сбое говорит простыми словами, что версию узнать не удалось", async () => {
    getVersion.mockRejectedValue(new Error("window.go is undefined"));
    render(<App />);
    expect(await screen.findByText("Версию узнать не удалось")).toBeTruthy();
    expect(screen.queryByText(/undefined/)).toBeNull();
  });
});
