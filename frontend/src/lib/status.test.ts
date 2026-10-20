import { describe, expect, it } from "vitest";
import { canAddTime, hasTimeLimits, statusTone } from "./status";
import type { StatusKind } from "../types";

describe("цвет статуса (ТЗ 8.0 п.3)", () => {
  const table: [StatusKind, string][] = [
    ["ok", "ok"],
    ["unlimited", "ok"],
    ["low", "warn"],
    ["timeUp", "danger"],
    ["bypass", "danger"],
    ["safe", "neutral"],
    ["admin", "neutral"],
    ["notChild", "neutral"],
  ];
  it.each(table)("%s -> %s", (status, tone) => {
    expect(statusTone(status)).toBe(tone);
  });

  // Ловит: кнопки «+время» у администратора и в безопасном режиме, где ограничения не действуют.
  it("учёт времени есть только у детей с действующими правилами", () => {
    expect(hasTimeLimits("ok")).toBe(true);
    expect(hasTimeLimits("timeUp")).toBe(true);
    expect(hasTimeLimits("bypass")).toBe(true);
    expect(hasTimeLimits("admin")).toBe(false);
    expect(hasTimeLimits("notChild")).toBe(false);
    expect(hasTimeLimits("safe")).toBe(false);
  });
});

describe("кнопки «+время» (сценарий 8.3.2)", () => {
  // Ловит: родитель добавляет минуты ребёнку, заблокированному за обход защиты: время не при чём,
  // нужно «Снять блокировку».
  it("при блокировке за обход «+время» нет", () => {
    expect(canAddTime("bypass")).toBe(false);
  });

  it("при обычном «время вышло» «+время» остаётся", () => {
    expect(canAddTime("timeUp")).toBe(true);
    expect(canAddTime("ok")).toBe(true);
    expect(canAddTime("low")).toBe(true);
  });

  it("там, где ограничения не действуют, «+время» тоже нет", () => {
    expect(canAddTime("admin")).toBe(false);
    expect(canAddTime("notChild")).toBe(false);
    expect(canAddTime("safe")).toBe(false);
  });
});

