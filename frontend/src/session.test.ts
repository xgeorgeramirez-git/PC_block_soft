import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createSessionWatcher, type SessionView } from "./session";
import { linkError } from "./lib/errors";
import type { SessionStateReply } from "./types";

// R19: сессия родителя кончается через 10 минут бездействия; за 1 минуту окно предлагает
// «Продолжить», по концу показывает вход. Время считается по часам компьютера (реальное),
// а остаток берётся из ответа службы.

/** Служба сессии на поддельных часах: остаток считается от крайнего срока. */
function fakeService(idleS: number, firstRemainingS: number) {
  let deadline = Date.now() + firstRemainingS * 1000;
  let down = false;
  const reply = (): SessionStateReply => {
    const left = Math.ceil((deadline - Date.now()) / 1000);
    return left > 0
      ? { active: true, expiresMs: deadline, remainingS: left }
      : { active: false, expiresMs: 0, remainingS: 0 };
  };
  return {
    fetchState: vi.fn(async () => {
      if (down) throw linkError();
      return reply();
    }),
    extend: vi.fn(async () => {
      if (down) throw linkError();
      if (reply().active) deadline = Date.now() + idleS * 1000;
      return reply();
    }),
    setLinkDown: (v: boolean) => {
      down = v;
    },
    touchedElsewhere: () => {
      deadline = Date.now() + idleS * 1000;
    },
  };
}

let views: SessionView[];
const onChange = (v: SessionView) => {
  views.push(v);
};
const last = () => views[views.length - 1];

beforeEach(() => {
  vi.useFakeTimers({ now: new Date(2026, 9, 19, 12, 0, 0) });
  views = [];
});

afterEach(() => {
  vi.useRealTimers();
});

async function advance(ms: number) {
  await vi.advanceTimersByTimeAsync(ms);
}

describe("наблюдение за сессией", () => {
  it("сразу спрашивает состояние и сообщает, что сессия идёт", async () => {
    const svc = fakeService(600, 540);
    const w = createSessionWatcher({ ...svc, onChange });
    w.start();
    await advance(0);
    expect(svc.fetchState).toHaveBeenCalledTimes(1);
    expect(last()).toEqual({ phase: "active", remainingS: 540 });
    w.stop();
  });

  // Ловит: предупреждение появляется поздно (после минуты) или раньше срока.
  it("за минуту до конца показывает предупреждение, не раньше", async () => {
    const svc = fakeService(600, 75);
    const w = createSessionWatcher({ ...svc, onChange });
    w.start();
    await advance(10_000);
    expect(last().phase).toBe("active");
    await advance(6_000);
    expect(last().phase).toBe("warning");
    expect(last().remainingS).toBeLessThanOrEqual(60);
    expect(last().remainingS).toBeGreaterThan(50);
    w.stop();
  });

  it("если сессия уже на последней минуте, предупреждение сразу", async () => {
    const svc = fakeService(600, 40);
    const w = createSessionWatcher({ ...svc, onChange });
    w.start();
    await advance(0);
    expect(last()).toEqual({ phase: "warning", remainingS: 40 });
    w.stop();
  });

  // Ловит: «Продолжить» не продлевает сессию или предупреждение остаётся на экране.
  it("«Продолжить» продлевает сессию и снимает предупреждение", async () => {
    const svc = fakeService(600, 40);
    const w = createSessionWatcher({ ...svc, onChange });
    w.start();
    await advance(0);
    await w.extend();
    expect(svc.extend).toHaveBeenCalledTimes(1);
    expect(last()).toEqual({ phase: "active", remainingS: 600 });
    await advance(30_000);
    expect(last().phase).toBe("active");
    w.stop();
  });

  // Ловит: окно считает конец сессии по себе и мешает работать, когда другие команды её продлили.
  it("если сессию продлили другие команды, предупреждение уходит само", async () => {
    const svc = fakeService(600, 50);
    const w = createSessionWatcher({ ...svc, onChange });
    w.start();
    await advance(0);
    expect(last().phase).toBe("warning");
    svc.touchedElsewhere();
    await advance(1_500);
    expect(last().phase).toBe("active");
    w.stop();
  });

  // Ловит: конец сессии не доходит до окна, и родитель продолжает править без пароля.
  it("конец сессии сообщается один раз, опрос прекращается", async () => {
    const svc = fakeService(600, 20);
    const w = createSessionWatcher({ ...svc, onChange });
    w.start();
    await advance(25_000);
    expect(last()).toEqual({ phase: "ended", remainingS: 0 });
    expect(views.filter((v) => v.phase === "ended")).toHaveLength(1);
    const calls = svc.fetchState.mock.calls.length;
    await advance(60_000);
    expect(svc.fetchState.mock.calls.length).toBe(calls);
    w.stop();
  });

  it("после повторного входа наблюдение запускается заново", async () => {
    const svc = fakeService(600, 5);
    const w = createSessionWatcher({ ...svc, onChange });
    w.start();
    await advance(8_000);
    expect(last().phase).toBe("ended");
    svc.touchedElsewhere(); // новая сессия после ввода пароля
    w.start();
    await advance(0);
    expect(last()).toEqual({ phase: "active", remainingS: 600 });
    w.stop();
  });

  // Ловит: сбой связи принят за конец сессии и закрывает работу родителя.
  it("сбой связи не считается концом сессии", async () => {
    const svc = fakeService(600, 300);
    const w = createSessionWatcher({ ...svc, onChange });
    w.start();
    await advance(0);
    svc.setLinkDown(true);
    await advance(30_000);
    expect(last().phase).toBe("active");
    svc.setLinkDown(false);
    const callsBefore = svc.fetchState.mock.calls.length;
    await advance(6_000);
    expect(svc.fetchState.mock.calls.length).toBeGreaterThan(callsBefore);
    expect(last().phase).toBe("active");
    w.stop();
  });

  it("если продлить не удалось, спрашивает состояние и узнаёт о конце", async () => {
    const svc = fakeService(600, 3);
    const w = createSessionWatcher({ ...svc, onChange });
    w.start();
    await advance(0);
    svc.extend.mockRejectedValueOnce(linkError());
    await advance(3_500); // срок вышел до нажатия
    await w.extend();
    expect(last().phase).toBe("ended");
    w.stop();
  });

  it("после остановки ничего не происходит", async () => {
    const svc = fakeService(600, 300);
    const w = createSessionWatcher({ ...svc, onChange });
    w.start();
    await advance(0);
    w.stop();
    const n = views.length;
    const calls = svc.fetchState.mock.calls.length;
    await advance(120_000);
    expect(views.length).toBe(n);
    expect(svc.fetchState.mock.calls.length).toBe(calls);
  });

  it("повторный запуск не плодит опросы", async () => {
    const svc = fakeService(600, 300);
    const w = createSessionWatcher({ ...svc, onChange });
    w.start();
    w.start();
    await advance(0);
    expect(svc.fetchState).toHaveBeenCalledTimes(1);
    w.stop();
  });
});
