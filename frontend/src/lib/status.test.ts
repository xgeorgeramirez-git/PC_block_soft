import { describe, expect, it } from "vitest";
import { hasTimeLimits, statusTone } from "./status";
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
