import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import Dialog from "../components/Dialog";
import { installBackend, mockChild, uninstallBackend, type MockBackend, type MockOptions } from "../__mocks__/backend";
import type { WeekdayLimit } from "../types";
import { LABELS, hintById, type ScreenId } from "./catalog";
import type { Risk } from "./detectors";
import GuidanceProvider, { SETTLE_MS } from "./GuidanceProvider";
import HintSlot from "./HintSlot";
import { HelperEngine } from "./engine";
import { apiHelperIO } from "./io";
import { useGuidance, type GuidanceApi, type NavigateOptions } from "./context";

// Помощник в окне (ТЗ 8.4): что родитель реально увидит. Логику вежливости по отдельности
// проверяет engine.test.ts, здесь - как она собрана с экраном: приглашение, «Позже», тихие места,
// напоминания о риске, тур, выключатель.

let mock: MockBackend;

const UNLIMITED: WeekdayLimit[] = Array.from({ length: 7 }, (_, i) => ({ weekday: i + 1, totalMin: 0, unlimited: true }));

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: new Date(2026, 9, 19, 12, 0, 0) });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

interface HarnessProps {
  engine: HelperEngine;
  onNavigate?: (screen: ScreenId, opts?: NavigateOptions) => void;
  startOn?: ScreenId;
  children?: { id: number; name: string }[];
  withDialog?: boolean;
  readRisks?: () => Promise<Risk[]>;
}

/** Окно родителя в миниатюре: меню, якоря и места для карточек на тех же местах, что в настоящем. */
function Harness({ engine, onNavigate, startOn = "today", children = [], withDialog = false, readRisks }: HarnessProps) {
  const [current, setCurrent] = useState<ScreenId>(startOn);
  const [dialog, setDialog] = useState(false);
  return (
    <GuidanceProvider
      engine={engine}
      screen={current}
      readRisks={readRisks}
      navigate={(s, o) => {
        onNavigate?.(s, o);
        setCurrent(s);
      }}
    >
      <header data-help-anchor="topbar">PC Block</header>
      <HintSlot anchor="topbar" />
      <nav>
        <button type="button" data-help-anchor="menu-today" onClick={() => setCurrent("today")}>
          Сегодня
        </button>
        <button type="button" data-help-anchor="menu-children" onClick={() => setCurrent("children")}>
          Дети
        </button>
        <button type="button" onClick={() => setCurrent("settings")}>
          Настройки
        </button>
      </nav>
      <main>
        {current === "today"
          ? children.map((c, i) => (
              <article key={c.id} aria-label={c.name} data-help-anchor="child-card">
                <HintSlot anchor="child-card" subject={String(c.id)} name={c.name} />
                <div data-help-anchor="extra-buttons">
                  <button type="button">+15</button>
                </div>
                {i === 0 ? <HintSlot anchor="extra-buttons" /> : null}
              </article>
            ))
          : null}
        {current === "children" ? (
          <section>
            <HintSlot anchor="limits-table" />
            <table data-help-anchor="limits-table" />
          </section>
        ) : null}
        {current === "settings" ? <input type="password" aria-label="Пароль" /> : null}
        {withDialog ? (
          <button type="button" onClick={() => setDialog(true)}>
            Открыть окно
          </button>
        ) : null}
      </main>
      <div data-help-anchor="savebar" />
      {dialog ? (
        <Dialog title="Вопрос" onClose={() => setDialog(false)} actions={<button onClick={() => setDialog(false)}>Закрыть окно</button>}>
          <p>Текст</p>
        </Dialog>
      ) : null}
    </GuidanceProvider>
  );
}

function start(helper: Record<string, unknown> = {}, opts: MockOptions = {}, windowId = 1) {
  mock = installBackend({ sessionActive: true, helper, ...opts });
  const engine = new HelperEngine(apiHelperIO, { windowId });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  return { engine, user };
}

/** Следующее открытие окна: тот же профиль, новая метка окна. */
function reopen(windowId: number) {
  return new HelperEngine(apiHelperIO, { windowId });
}

/** Помощник без экрана вокруг: достаёт его программный интерфейс, как это делают Помощь и Настройки. */
function renderProbe(engine: HelperEngine, navigate: (screen: ScreenId, opts?: NavigateOptions) => void) {
  let api: GuidanceApi | null = null;
  const Probe = () => {
    api = useGuidance();
    return null;
  };
  render(
    <GuidanceProvider engine={engine} screen="today" navigate={navigate}>
      <Probe />
      <div data-help-anchor="menu-today" />
    </GuidanceProvider>,
  );
  return { api: () => api };
}

const card = () => screen.queryByRole("status");
/** Время идёт кусочками, как в настоящем окне: между ними экран успевает перерисоваться. */
const settle = async (ms = SETTLE_MS + 100) => {
  for (let spent = 0; spent < ms; spent += 50) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(50);
    });
  }
};
const saved = () => mock.state.helper as Record<string, unknown>;

describe("Помощник: приглашение в тур", () => {
  // Ловит: на первом входе нет приглашения, о туре нигде не сказано.
  it("на первом входе показывает приглашение, необязательное и с кнопкой «Начать»", async () => {
    const { engine } = start();
    render(<Harness engine={engine} />);
    const c = await screen.findByRole("status");
    expect(within(c).getByText(hintById("H-M1-01")!.title)).toBeTruthy();
    expect(within(c).getByRole("button", { name: "Начать" })).toBeTruthy();
    expect(within(c).getByRole("button", { name: LABELS.later })).toBeTruthy();
    expect(within(c).getByRole("button", { name: LABELS.close })).toBeTruthy();
  });

  // Ловит: карточка перекрывает окно и мешает работать (Помощник не блокирует).
  it("карточка стоит в потоке страницы, а не поверх неё", async () => {
    const { engine } = start();
    render(<Harness engine={engine} />);
    const c = await screen.findByRole("status");
    expect(c.closest("header")).toBeNull();
    expect(c.className).not.toMatch(/overlay|modal/);
  });

  it("показ записан в базу: подсказка «показана», журнал показов пополнился", async () => {
    const { engine } = start();
    render(<Harness engine={engine} />);
    await screen.findByRole("status");
    await engine.flush();
    expect(saved().seen).toEqual(["H-M1-01"]);
    expect((saved().log as { hint: number[] }).hint).toHaveLength(1);
  });

  it("«Начать» запускает тур и записывает, что приглашение принято", async () => {
    const { engine, user } = start();
    render(<Harness engine={engine} />);
    await user.click(within(await screen.findByRole("status")).getByRole("button", { name: "Начать" }));
    expect(await screen.findByRole("dialog", { name: "Короткий тур по окну родителя" })).toBeTruthy();
    expect(card()).toBeNull();
    await engine.flush();
    expect(saved().done).toContain("H-M1-01");
  });

  // Ловит: Esc, крестик и «Позже» ведут себя по-разному: приглашение «залипает» или возвращается сразу.
  it("«Позже» убирает карточку, и в этом окне она не возвращается", async () => {
    const { engine, user } = start();
    render(<Harness engine={engine} />);
    await user.click(within(await screen.findByRole("status")).getByRole("button", { name: LABELS.later }));
    expect(card()).toBeNull();
    await settle(2000);
    expect(card()).toBeNull();
    await engine.flush();
    expect(saved().snoozed).toHaveProperty("H-M1-01");
  });

  it("«Позже» возвращает приглашение в следующем окне, один раз", async () => {
    const first = start();
    const view = render(<Harness engine={first.engine} />);
    await first.user.click(within(await screen.findByRole("status")).getByRole("button", { name: LABELS.later }));
    await first.engine.flush();
    view.unmount();

    const second = reopen(2);
    const view2 = render(<Harness engine={second} />);
    const again = await screen.findByRole("status");
    await first.user.click(within(again).getByRole("button", { name: LABELS.later }));
    await second.flush();
    view2.unmount();

    render(<Harness engine={reopen(3)} />);
    await settle(1500);
    expect(card()).toBeNull();
  });

  it("крестик закрывает приглашение насовсем", async () => {
    const first = start();
    const view = render(<Harness engine={first.engine} />);
    await first.user.click(within(await screen.findByRole("status")).getByRole("button", { name: LABELS.close }));
    await first.engine.flush();
    view.unmount();
    render(<Harness engine={reopen(2)} />);
    await settle(1500);
    expect(card()).toBeNull();
  });

  it("Esc равен «Позже»", async () => {
    const { engine } = start();
    render(<Harness engine={engine} />);
    await screen.findByRole("status");
    // Слушатель клавиши ставится после показа карточки: даём React закончить.
    await act(async () => undefined);
    fireEvent.keyDown(document.body, { key: "Escape" });
    await waitFor(() => expect(card()).toBeNull());
    await engine.flush();
    expect(saved().snoozed).toHaveProperty("H-M1-01");
  });
});

describe("Помощник: подсказки по экранам", () => {
  // Ловит: после «Позже» следующая подсказка выскакивает на том же экране (нет передышки).
  it("после ответа следующая карточка ждёт нового экрана", async () => {
    const { engine, user } = start();
    render(<Harness engine={engine} startOn="children" />);
    await user.click(within(await screen.findByRole("status")).getByRole("button", { name: LABELS.later }));
    await settle(1500);
    expect(card()).toBeNull();
  });

  it("S28: приглашение, «Позже», переход в «Дети» показывает подсказку про лимиты", async () => {
    const { engine, user } = start();
    render(<Harness engine={engine} />);
    await user.click(within(await screen.findByRole("status")).getByRole("button", { name: LABELS.later }));
    await user.click(screen.getByRole("button", { name: "Дети" }));
    const c = await screen.findByRole("status");
    expect(within(c).getByText(hintById("H-M1-02")!.title)).toBeTruthy();
  });

  // Ловит: подсказка остаётся на экране, на котором ей нечего объяснять.
  it("при уходе с экрана карточка снимается и не возвращается", async () => {
    const { engine, user } = start({ done: ["H-M1-01"] });
    render(<Harness engine={engine} startOn="children" />);
    await screen.findByText(hintById("H-M1-02")!.title);
    await user.click(screen.getByRole("button", { name: "Сегодня" }));
    expect(screen.queryByText(hintById("H-M1-02")!.title)).toBeNull();
    await user.click(screen.getByRole("button", { name: "Дети" }));
    await settle(1500);
    expect(screen.queryByText(hintById("H-M1-02")!.title)).toBeNull();
  });

  it("подсказка про кнопки «+15» стоит у кнопок и только на «Сегодня»", async () => {
    const { engine } = start({ done: ["H-M1-01"] }, { children: [mockChild()] });
    render(<Harness engine={engine} children={[{ id: 1, name: "Миша" }]} />);
    const c = await screen.findByRole("status");
    expect(within(c).getByText(hintById("H-M1-03")!.title)).toBeTruthy();
    expect(c.closest("article")).toBeTruthy();
  });

  // Ловит: уже объяснённое туром снова всплывает подсказкой. H-M2-08 (предупреждения 0.2.5) у той же
  // таблицы лимитов тур T-M1-01 не объяснял: это новое знание, она здесь отмечена принятой, чтобы
  // проверять только то, что гасит тур.
  it("пройденный тур гасит подсказки, которые он объяснил", async () => {
    const { engine } = start({ done: ["H-M1-01", "H-M1-02", "H-M1-03", "H-M2-08"], tours: ["T-M1-01"] });
    render(<Harness engine={engine} startOn="children" />);
    await settle(1500);
    expect(card()).toBeNull();
  });

  // Ловит: новая подсказка о предупреждениях (не объяснена туром) погашена пройденным туром.
  it("подсказка, которую тур не объяснял, после тура показывается", async () => {
    const { engine } = start({ done: ["H-M1-01", "H-M1-02", "H-M1-03"], tours: ["T-M1-01"] });
    render(<Harness engine={engine} startOn="children" />);
    expect((await screen.findByRole("status")).getAttribute("data-hint-id")).toBe("H-M2-08");
  });
});

describe("Помощник: выключатель", () => {
  // Ловит: подсказки выключены, а карточки идут (S28).
  it("выключено в настройках - ни приглашения, ни подсказок, ни напоминаний", async () => {
    const { engine } = start({ enabled: false }, { children: [mockChild({ limits: UNLIMITED })] });
    render(<Harness engine={engine} children={[{ id: 1, name: "Миша" }]} />);
    await settle(2000);
    expect(card()).toBeNull();
  });

  it("выключили при открытой карточке - она сразу пропадает", async () => {
    const { engine } = start();
    render(<Harness engine={engine} />);
    await screen.findByRole("status");
    await act(async () => {
      await engine.setEnabled(false);
    });
    expect(card()).toBeNull();
  });

  // Ловит: выключатель отключает и тур (его просят явно, он не «подсказка»).
  it("при выключенных подсказках тур всё равно можно запустить", async () => {
    const { engine } = start({ enabled: false });
    const probe = renderProbe(engine, () => undefined);
    await waitFor(() => expect(probe.api()?.engine.ready).toBe(true));
    act(() => probe.api()!.startTour());
    expect(await screen.findByRole("dialog", { name: "Короткий тур по окну родителя" })).toBeTruthy();
  });
});

describe("Помощник: тихие места", () => {
  // Ловит: карточка поверх открытого окна вопроса (ТЗ 8.4 п.4).
  it("пока открыт диалог, карточки нет; закрыли - вернулась", async () => {
    const { engine, user } = start();
    render(<Harness engine={engine} withDialog />);
    await screen.findByRole("status");
    await user.click(screen.getByRole("button", { name: "Открыть окно" }));
    expect(card()).toBeNull();
    await user.click(screen.getByRole("button", { name: "Закрыть окно" }));
    expect(await screen.findByRole("status")).toBeTruthy();
  });

  it("диалог не засчитывает карточку «показанной» второй раз", async () => {
    const { engine, user } = start();
    render(<Harness engine={engine} withDialog />);
    await screen.findByRole("status");
    await user.click(screen.getByRole("button", { name: "Открыть окно" }));
    await user.click(screen.getByRole("button", { name: "Закрыть окно" }));
    await screen.findByRole("status");
    await engine.flush();
    expect((saved().log as { hint: number[] }).hint).toHaveLength(1);
  });

  it("Esc при открытом диалоге закрывает диалог, а карточку не трогает", async () => {
    const { engine, user } = start();
    render(<Harness engine={engine} withDialog />);
    await screen.findByRole("status");
    await user.click(screen.getByRole("button", { name: "Открыть окно" }));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(await screen.findByRole("status")).toBeTruthy();
    await engine.flush();
    expect(saved().snoozed ?? {}).toEqual({});
  });

  // Ловит: подсказка выскакивает, пока человек вводит пароль.
  it("поле пароля в фокусе - карточки нет", async () => {
    const { engine, user } = start({ done: ["H-M1-01"] });
    render(<Harness engine={engine} startOn="settings" />);
    await settle();
    // На «Настройки» подсказок нет; проверяем на «Дети» с фокусом, перенесённым в пароль.
    await user.click(screen.getByRole("button", { name: "Дети" }));
    await screen.findByRole("status");
    const password = document.createElement("input");
    password.type = "password";
    document.body.append(password);
    act(() => password.focus());
    expect(card()).toBeNull();
    act(() => password.blur());
    expect(await screen.findByRole("status")).toBeTruthy();
    password.remove();
  });

  it("идущий тур - тоже тихое место", async () => {
    const { engine, user } = start();
    render(<Harness engine={engine} />);
    await user.click(within(await screen.findByRole("status")).getByRole("button", { name: "Начать" }));
    await screen.findByRole("dialog", { name: "Короткий тур по окну родителя" });
    expect(card()).toBeNull();
  });
});

describe("Помощник: напоминания о риске", () => {
  const misha = { id: 1, name: "Миша" };

  // Ловит: у ребёнка «Без лимита» во все дни, а родителю никто не сказал (D-M1-01).
  it("D-M1-01: «Без лимита» во все дни - напоминание на карточке ребёнка", async () => {
    const { engine } = start({ done: ["H-M1-01"] }, { children: [mockChild({ limits: UNLIMITED })] });
    render(<Harness engine={engine} children={[misha]} />);
    const c = await screen.findByRole("status");
    expect(within(c).getByText("Похоже, у Миши нет ограничений ни в один день")).toBeTruthy();
    expect(c.closest("article")?.getAttribute("aria-label")).toBe("Миша");
  });

  it("напоминание обгоняет приглашение, даже если приглашение ещё не показано", async () => {
    const { engine } = start({}, { children: [mockChild({ limits: UNLIMITED })] });
    render(<Harness engine={engine} children={[misha]} />);
    const c = await screen.findByRole("status");
    expect(c.getAttribute("data-hint-id")).toBe("D-M1-01");
  });

  // Ловит: данные о рисках пришли позже паузы, и обычная подсказка заняла место раньше напоминания.
  it("риски читаются долго: приглашение ждёт, напоминание идёт первым", async () => {
    const { engine } = start({}, { children: [mockChild({ limits: UNLIMITED })] });
    const slow = () => new Promise<Risk[]>((resolve) => setTimeout(() => resolve([{ entryId: "D-M1-01", childId: 1, name: "Миша" }]), 1500));
    render(<Harness engine={engine} children={[misha]} readRisks={slow} />);
    await settle(SETTLE_MS * 3);
    expect(card()).toBeNull();
    await settle(1500);
    expect((await screen.findByRole("status")).getAttribute("data-hint-id")).toBe("D-M1-01");
  });

  it("правила в порядке - напоминания нет", async () => {
    const { engine } = start({ done: ["H-M1-01", "H-M1-03"] }, { children: [mockChild()] });
    render(<Harness engine={engine} children={[misha]} />);
    await settle(1500);
    expect(card()).toBeNull();
  });

  // Ловит: «Задать лимиты» ведёт «куда-то», а не к лимитам этого ребёнка (L-08).
  it("«Задать лимиты» ведёт в «Дети» к этому ребёнку", async () => {
    const onNavigate = vi.fn();
    const { engine, user } = start({ done: ["H-M1-01"] }, { children: [mockChild({ limits: UNLIMITED })] });
    render(<Harness engine={engine} children={[misha]} onNavigate={onNavigate} />);
    await user.click(within(await screen.findByRole("status")).getByRole("button", { name: "Задать лимиты" }));
    expect(onNavigate).toHaveBeenCalledWith("children", { childId: 1 });
  });

  it("D-M1-02: администратор - напоминание и «Как исправить» с шагами", async () => {
    const { engine, user } = start({ done: ["H-M1-01"] }, { children: [mockChild({ admin: true })] });
    render(<Harness engine={engine} children={[misha]} />);
    const c = await screen.findByRole("status");
    expect(within(c).getByText("У Миши теперь права администратора: ограничения не работают")).toBeTruthy();
    await user.click(within(c).getByRole("button", { name: "Как исправить" }));
    const d = await screen.findByRole("dialog", { name: "Как вернуть обычные права" });
    expect(within(d).getAllByRole("listitem")).toHaveLength(4);
    expect(card()).toBeNull();
    await user.click(within(d).getByRole("button", { name: "Закрыть" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("«Больше не напоминать» гасит напоминание для этого ребёнка навсегда", async () => {
    const first = start({ done: ["H-M1-01", "H-M1-03"] }, { children: [mockChild({ limits: UNLIMITED })] });
    const view = render(<Harness engine={first.engine} children={[misha]} />);
    await first.user.click(within(await screen.findByRole("status")).getByRole("button", { name: LABELS.mute }));
    await first.engine.flush();
    expect(saved().muted).toEqual(["D-M1-01:1"]);
    view.unmount();
    render(<Harness engine={reopen(2)} children={[misha]} />);
    await settle(1500);
    expect(card()).toBeNull();
  });

  it("риск остался - напоминание возвращается в следующем окне", async () => {
    const first = start({ done: ["H-M1-01"] }, { children: [mockChild({ limits: UNLIMITED })] });
    const view = render(<Harness engine={first.engine} children={[misha]} />);
    await first.user.click(within(await screen.findByRole("status")).getByRole("button", { name: "Задать лимиты" }));
    await first.engine.flush();
    view.unmount();
    render(<Harness engine={reopen(2)} children={[misha]} />);
    expect((await screen.findByRole("status")).getAttribute("data-hint-id")).toBe("D-M1-01");
  });

  // Ловит: закрытое дважды крестиком напоминание всё равно возвращается (Clippy).
  it("закрыто крестиком два раза - больше не появляется", async () => {
    const limits = { limits: UNLIMITED };
    const first = start({ done: ["H-M1-01", "H-M1-03"] }, { children: [mockChild(limits)] });
    let view = render(<Harness engine={first.engine} children={[misha]} />);
    await first.user.click(within(await screen.findByRole("status")).getByRole("button", { name: LABELS.close }));
    await first.engine.flush();
    view.unmount();
    view = render(<Harness engine={reopen(2)} children={[misha]} />);
    await first.user.click(within(await screen.findByRole("status")).getByRole("button", { name: LABELS.close }));
    await first.engine.flush();
    view.unmount();
    render(<Harness engine={reopen(3)} children={[misha]} />);
    await settle(1500);
    expect(card()).toBeNull();
  });

  it("сбой чтения правил не ломает окно: просто без напоминания", async () => {
    const { engine } = start({ done: ["H-M1-01", "H-M1-03"] }, { children: [mockChild({ limits: UNLIMITED })] });
    mock.override("GetChildRules", "reject", 5);
    render(<Harness engine={engine} children={[misha]} />);
    await settle(1500);
    expect(card()).toBeNull();
    expect(screen.getByRole("article", { name: "Миша" })).toBeTruthy();
  });
});

describe("Помощник: вежливость", () => {
  // Ловит: больше двух подсказок в час (ТЗ 8.4 п.3).
  it("в журнале уже две подсказки за час - новая подсказка ждёт", async () => {
    const now = Date.now();
    const { engine } = start({ log: { hint: [now - 600_000, now - 300_000], remind: [] }, done: ["H-M1-01"] });
    render(<Harness engine={engine} startOn="children" />);
    await settle(1500);
    expect(card()).toBeNull();
  });

  it("а напоминание о риске при той же квоте проходит", async () => {
    const now = Date.now();
    const { engine } = start(
      { log: { hint: [now - 600_000, now - 300_000], remind: [] }, done: ["H-M1-01"] },
      { children: [mockChild({ limits: UNLIMITED })] },
    );
    render(<Harness engine={engine} children={[{ id: 1, name: "Миша" }]} />);
    expect((await screen.findByRole("status")).getAttribute("data-hint-id")).toBe("D-M1-01");
  });

  // Ловит: состояние не прочиталось, а Помощник пишет «с чистого листа» и затирает сохранённое.
  it("состояние не прочиталось - карточек нет и в базу ничего не пишется", async () => {
    const { engine } = start({ done: ["H-M1-01"], seen: ["H-M1-02"] });
    mock.override("GetHelperState", "reject", 3);
    render(<Harness engine={engine} startOn="children" />);
    await settle(2000);
    expect(card()).toBeNull();
    expect(mock.count("SaveHelperState")).toBe(0);
  });
});

describe("Помощник: тур до конца", () => {
  it("пройденный тур записывается, а объяснённые им подсказки гасятся", async () => {
    const { engine, user } = start();
    render(<Harness engine={engine} />);
    await user.click(within(await screen.findByRole("status")).getByRole("button", { name: "Начать" }));
    const tour = await screen.findByRole("dialog", { name: "Короткий тур по окну родителя" });
    for (let i = 0; i < 5; i++) await user.click(within(tour).getByRole("button", { name: LABELS.tourNext }));
    await user.click(within(tour).getByRole("button", { name: LABELS.tourDone }));
    expect(screen.queryByRole("dialog")).toBeNull();
    await engine.flush();
    expect(saved().tours).toEqual(["T-M1-01"]);
    expect(saved().done).toEqual(expect.arrayContaining(["H-M1-01", "H-M1-02", "H-M1-03"]));
  });

  // Ловит: пропуск тура засчитывается как прохождение.
  it("пропущенный тур не считается пройденным", async () => {
    const { engine, user } = start();
    render(<Harness engine={engine} />);
    await user.click(within(await screen.findByRole("status")).getByRole("button", { name: "Начать" }));
    const tour = await screen.findByRole("dialog", { name: "Короткий тур по окну родителя" });
    await user.click(within(tour).getByRole("button", { name: LABELS.tourSkip }));
    expect(screen.queryByRole("dialog")).toBeNull();
    await engine.flush();
    expect(saved().tours ?? []).toEqual([]);
  });

  it("тур ведёт по экранам: шаг про «Дети» открывает «Дети»", async () => {
    const onNavigate = vi.fn();
    const { engine, user } = start({ done: ["H-M1-01"] });
    const probe = renderProbe(engine, onNavigate);
    await waitFor(() => expect(probe.api()?.engine.ready).toBe(true));
    act(() => probe.api()!.startTour());
    const tour = await screen.findByRole("dialog", { name: "Короткий тур по окну родителя" });
    for (let i = 0; i < 3; i++) await user.click(within(tour).getByRole("button", { name: LABELS.tourNext }));
    expect(onNavigate).toHaveBeenCalledWith("children");
  });
});
