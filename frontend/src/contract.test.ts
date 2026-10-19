import { afterEach, describe, expect, it } from "vitest";
import contract from "../../internal/ipc/testdata/contract.json";
import * as api from "./api";
import { CallError } from "./lib/errors";
import type { Reply, WeekdayLimit } from "./types";

// Контракт окна и службы: примеры internal/ipc/testdata/contract.json разбираются api.ts.
// Go-тест сверяет файл с типами службы, этот тест - с обёртками окна, поэтому рассинхрон
// имён полей и формы ответов виден до сборки.

type Example = { request?: unknown; reply: Reply };
const examples = contract as unknown as Record<string, Example>;

let sent: { command: string; payload: unknown }[] = [];

function serve(reply: Reply): void {
  sent = [];
  window.go = {
    ui: {
      App: {
        Call: (command, payload) => {
          sent.push({ command, payload: JSON.parse(payload) });
          return Promise.resolve(JSON.stringify(reply));
        },
        Mode: () => Promise.resolve("window"),
        Version: () => Promise.resolve("0.1.0"),
      },
    },
  };
}

afterEach(() => {
  delete window.go;
});

/** Команда, вариант примера, вызов обёртки по запросу примера. */
const CASES: [string, string, (req: any) => Promise<unknown>][] = [
  ["Hello", "Hello", () => api.hello()],
  ["ListAccounts", "ListAccounts", () => api.listAccounts()],
  ["ListAccounts", "ListAccounts#empty", () => api.listAccounts()],
  ["Setup", "Setup", (r) => api.setup(r.password, r.children)],
  ["ResetPassword", "ResetPassword", (r) => api.resetPassword(r.newPassword)],
  ["UninstallCheck", "UninstallCheck", (r) => api.uninstallCheck(r.password, r.forgot)],
  ["Login", "Login", (r) => api.login(r.password)],
  ["ExtendSession", "ExtendSession", () => api.extendSession()],
  ["Logout", "Logout", () => api.logout()],
  ["SessionState", "SessionState", () => api.sessionState()],
  ["GetToday", "GetToday", () => api.getToday()],
  ["GetToday", "GetToday#empty", () => api.getToday()],
  ["GetToday", "GetToday#notices", () => api.getToday()],
  ["GetChildRules", "GetChildRules", (r) => api.getChildRules(r.childId)],
  ["SaveLimits", "SaveLimits", (r) => api.saveLimits(r.childId, r.limits as WeekdayLimit[])],
  ["AddExtra", "AddExtra", (r) => api.addExtra(r.childId, r.minutes)],
  ["UndoExtra", "UndoExtra", (r) => api.undoExtra(r.childId, r.extraId)],
  ["ClearBlock", "ClearBlock", (r) => api.clearBlock(r.childId)],
  ["ListEvents", "ListEvents", (r) => api.listEvents(r)],
  ["ListEvents", "ListEvents#empty", (r) => api.listEvents(r)],
  ["ChangePassword", "ChangePassword", (r) => api.changePassword(r.oldPassword, r.newPassword)],
  ["SetChildren", "SetChildren", (r) => api.setChildren(r)],
  ["SetChildren", "SetChildren#empty", (r) => api.setChildren(r)],
  ["GetHelperState", "GetHelperState", () => api.getHelperState()],
  ["GetHelperState", "GetHelperState#empty", () => api.getHelperState()],
  ["SaveHelperState", "SaveHelperState", (r) => api.saveHelperState(r.state)],
];

describe("успешные ответы контракта", () => {
  const okCases = CASES.filter(([, key]) => examples[key].reply.ok);

  it.each(okCases)("%s: пример %s разобран", async (command, key, run) => {
    const ex = examples[key];
    serve(ex.reply);
    const got = await run(ex.request ?? {});
    expect(sent).toHaveLength(1);
    expect(sent[0].command).toBe(command);
    // Запрос обёртки совпадает с примером запроса (пустой объект, если запроса нет).
    expect(sent[0].payload).toEqual(ex.request ?? {});
    // Ответ - нагрузка примера (null в массивах уже заменены на списки).
    expect(got).toEqual(payloadOf(command, ex.reply.payload));
  });
});

/** Что должна вернуть обёртка: нагрузка ответа в виде, в котором её ждёт экран. */
function payloadOf(command: string, payload: unknown): unknown {
  const p = payload as Record<string, unknown>;
  switch (command) {
    case "ListAccounts":
      return p.accounts;
    case "ListEvents":
      return p.events;
    case "SetChildren":
      return p.children;
    case "GetHelperState":
      return p.state;
    case "SaveHelperState":
    case "Logout":
    case "ResetPassword":
    case "Setup":
    case "ChangePassword":
      return undefined;
    default:
      return p;
  }
}

describe("отказы контракта", () => {
  const denied = CASES.filter(([, key]) => !examples[key].reply.ok);
  const extra: [string, string, (req: any) => Promise<unknown>][] = [
    ["Login", "Login#denied", (r) => api.login(r.password)],
    ["Login", "Login#paused", (r) => api.login(r.password)],
    ["UninstallCheck", "UninstallCheck#denied", (r) => api.uninstallCheck(r.password, r.forgot)],
    ["AddExtra", "AddExtra#invalid", (r) => api.addExtra(r.childId, r.minutes)],
    ["SaveLimits", "SaveLimits#invalid", (r) => api.saveLimits(r.childId, r.limits as WeekdayLimit[])],
    ["Setup", "Setup#invalid", (r) => api.setup(r.password, r.children)],
  ];

  it.each([...denied, ...extra])("%s: пример %s - отказ с текстом и полем", async (_command, key, run) => {
    const ex = examples[key];
    serve(ex.reply);
    const err = (await run(ex.request ?? {}).catch((e: unknown) => e)) as CallError;
    expect(err).toBeInstanceOf(CallError);
    expect(err.message).toBe(ex.reply.error);
    expect(err.field).toBe(ex.reply.field);
    expect(err.payload).toEqual(ex.reply.payload);
  });

  it("отказ входа разбирается в попытки и паузу", async () => {
    serve(examples["Login#paused"].reply);
    const err = await api.login("не тот").catch((e: unknown) => e);
    expect(api.loginDenied(err)).toEqual({ attemptsLeft: 0, pauseS: 60 });
  });
});

describe("полнота", () => {
  // Ловит: новая команда окна родителя появилась в контракте, а обёртки для неё нет.
  it("у каждой команды окна родителя есть пример и обёртка", () => {
    const wrapped = new Set(CASES.map(([command]) => command));
    for (const command of [
      "Hello",
      "ListAccounts",
      "Setup",
      "ResetPassword",
      "UninstallCheck",
      "Login",
      "ExtendSession",
      "Logout",
      "SessionState",
      "GetToday",
      "GetChildRules",
      "SaveLimits",
      "AddExtra",
      "UndoExtra",
      "ClearBlock",
      "ListEvents",
      "ChangePassword",
      "SetChildren",
      "GetHelperState",
      "SaveHelperState",
    ]) {
      expect(examples[command], `нет примера ${command}`).toBeDefined();
      expect(wrapped.has(command), `нет обёртки ${command}`).toBe(true);
    }
  });
});
