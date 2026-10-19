// Родительская сессия окна (R19): окно следит за её сроком по ответам службы.
// Сессия кончается через 10 минут бездействия; за 1 минуту окно предлагает «Продолжить»
// (ExtendSession), по концу сообщает, что нужен вход. Время реальное: остаток приходит
// от службы в секундах, окно только опрашивает её чаще к концу срока.
import type { SessionStateReply } from "./types";

export type SessionPhase = "active" | "warning" | "ended";

export interface SessionView {
  phase: SessionPhase;
  remainingS: number;
}

export interface SessionWatcherOptions {
  fetchState: () => Promise<SessionStateReply>;
  extend: () => Promise<SessionStateReply>;
  /** Вызывается при смене фазы (и при каждом обновлении остатка на последней минуте). */
  onChange: (view: SessionView) => void;
  /** За сколько секунд до конца предупреждать (по умолчанию 60). */
  warnAtS?: number;
  /** Опрос вдали от конца срока, мс. */
  slowPollMs?: number;
  /** Опрос на последней минуте, мс. */
  fastPollMs?: number;
}

export interface SessionWatcher {
  start(): void;
  stop(): void;
  /** «Продолжить»: продлить сессию. Сбой продления не роняет окно, а уточняет состояние. */
  extend(): Promise<void>;
}

export const SESSION_WARNING_TEXT = "Сессия закончится через 1 минуту";

export function createSessionWatcher(opts: SessionWatcherOptions): SessionWatcher {
  const warnAt = opts.warnAtS ?? 60;
  const slow = opts.slowPollMs ?? 5000;
  const fast = opts.fastPollMs ?? 1000;

  let generation = 0;
  let running = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let view: SessionView | null = null;

  const clear = () => {
    if (timer !== undefined) {
      clearTimeout(timer);
      timer = undefined;
    }
  };

  const schedule = (delay: number) => {
    clear();
    const mine = generation;
    timer = setTimeout(() => {
      timer = undefined;
      if (running && mine === generation) {
        void poll();
      }
    }, delay);
  };

  /** Опрос чаще к концу срока; вдали от него реже, но так, чтобы не проспать границу предупреждения. */
  const delayFor = (remainingS: number) =>
    remainingS <= warnAt + 10 ? fast : Math.min(slow, (remainingS - warnAt) * 1000 + 100);

  const emit = (next: SessionView) => {
    const changed =
      view === null || view.phase !== next.phase || (next.phase === "warning" && view.remainingS !== next.remainingS);
    if (changed) {
      view = next;
      opts.onChange(next);
    }
  };

  const apply = (reply: SessionStateReply) => {
    if (!reply.active) {
      emit({ phase: "ended", remainingS: 0 });
      running = false;
      clear();
      return;
    }
    const remainingS = Math.max(0, reply.remainingS);
    emit({ phase: remainingS <= warnAt ? "warning" : "active", remainingS });
    schedule(delayFor(remainingS));
  };

  async function poll(): Promise<void> {
    const mine = generation;
    try {
      const reply = await opts.fetchState();
      if (running && mine === generation) {
        apply(reply);
      }
    } catch {
      // Сбой связи не конец сессии: пробуем позже, окно ничего не закрывает.
      if (running && mine === generation) {
        schedule(slow);
      }
    }
  }

  return {
    start() {
      if (running) {
        return;
      }
      running = true;
      generation += 1;
      view = null;
      void poll();
    },
    stop() {
      running = false;
      generation += 1;
      clear();
    },
    async extend() {
      const mine = generation;
      try {
        const reply = await opts.extend();
        if (running && mine === generation) {
          apply(reply);
        }
      } catch {
        if (running && mine === generation) {
          await poll();
        }
      }
    },
  };
}
