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
  ["Setup", "Setup", (r) => api.setup(r.password, r.children, r.programs)],
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
  ["SaveLimits", "SaveLimits", (r) => api.saveLimits(r.childId, r.limits as WeekdayLimit[], r.endMode)],
  ["SaveLimits", "SaveLimits#endMode", (r) => api.saveLimits(r.childId, r.limits as WeekdayLimit[], r.endMode)],
  ["AddExtra", "AddExtra", (r) => api.addExtra(r.childId, r.minutes)],
  ["ReduceExtra", "ReduceExtra", (r) => api.reduceExtra(r.childId, r.minutes)],
  ["ClearExtra", "ClearExtra", (r) => api.clearExtra(r.childId)],
  ["UndoExtra", "UndoExtra", (r) => api.undoExtra(r.childId, r.extraId)],
  ["ClearBlock", "ClearBlock", (r) => api.clearBlock(r.childId)],
  ["ParentStats", "ParentStats", (r) => api.parentStats(r.childId)],
  ["GetDiag", "GetDiag", () => api.getDiag()],
  ["SetDiag", "SetDiag", (r) => api.setDiag(r.verbose)],
  ["ListEvents", "ListEvents", (r) => api.listEvents(r)],
  ["ListEvents", "ListEvents#empty", (r) => api.listEvents(r)],
  ["ChangePassword", "ChangePassword", (r) => api.changePassword(r.oldPassword, r.newPassword)],
  ["SetChildren", "SetChildren", (r) => api.setChildren(r)],
  ["SetChildren", "SetChildren#empty", (r) => api.setChildren(r)],
  ["MyToday", "MyToday", () => api.myToday()],
  ["MyToday", "MyToday#empty", () => api.myToday()],
  ["FirstRunSeen", "FirstRunSeen", () => api.firstRunSeen()],
  ["GetSchedule", "GetSchedule", (r) => api.getSchedule(r.childId)],
  ["GetSchedule", "GetSchedule#empty", (r) => api.getSchedule(r.childId)],
  ["SaveSchedule", "SaveSchedule", (r) => api.saveSchedule(r.childId, r.intervals)],
  ["Pause", "Pause", (r) => api.pause(r.childId, r.preset)],
  ["PauseAll", "PauseAll", (r) => api.pauseAll(r.preset)],
  ["PauseAll", "PauseAll#empty", (r) => api.pauseAll(r.preset)],
  ["GrantBonus", "GrantBonus", (r) => api.grantBonus(r.childId, r.minutes, r.reason)],
  ["UndoBonus", "UndoBonus", (r) => api.undoBonus(r.childId, r.bonusId)],
  ["GetBonus", "GetBonus", (r) => api.getBonus(r.childId)],
  ["GetBonus", "GetBonus#empty", (r) => api.getBonus(r.childId)],
  ["CopyRules", "CopyRules", (r) => api.copyRules(r.fromChildId, r.toChildId)],
  ["AskMore", "AskMore", (r) => api.askMore(r.childId, r.minutes, r.password)],
  ["SpendBonus", "SpendBonus", (r) => api.spendBonus(r.childId, r.minutes)],
  ["OpenLogs", "OpenLogs", () => api.openLogs()],
  ["OpenWindow", "OpenWindow", (r) => api.openWindow(r.kind)],
  ["OpenWindow", "OpenWindow#programs", (r) => api.openWindow(r.kind, r.screen)],
  ["OpenWindow", "OpenWindow#badScreen", (r) => api.openWindow(r.kind, r.screen)],
  ["ListPrograms", "ListPrograms", () => api.listPrograms()],
  ["ListPrograms", "ListPrograms#empty", () => api.listPrograms()],
  ["DecideProgram", "DecideProgram", (r) => api.decideProgram(r.path, r.category)],
  ["DecideProgram", "DecideProgram#invalid", (r) => api.decideProgram(r.path, r.category)],
  ["SetProgramsCategory", "SetProgramsCategory", (r) => api.setProgramsCategory(r.paths, r.category)],
  ["SetProgramsHidden", "SetProgramsHidden", (r) => api.setProgramsHidden(r.paths, r.hidden)],
  ["RenameProgram", "RenameProgram", (r) => api.renameProgram(r.path, r.name)],
  ["RenameProgram", "RenameProgram#tooLong", (r) => api.renameProgram(r.path, r.name)],
  ["UndoProgram", "UndoProgram", (r) => api.undoProgram(r.undoId)],
  ["ScanPrograms", "ScanPrograms", () => api.scanPrograms()],
  ["ScanPrograms", "ScanPrograms#empty", () => api.scanPrograms()],
  ["SetNoForbidden", "SetNoForbidden", (r) => api.setNoForbidden(r.childId, r.on)],
  ["ClearBonus", "ClearBonus", (r) => api.clearBonus(r.childId)],
  ["ReduceBonus", "ReduceBonus", (r) => api.reduceBonus(r.childId, r.minutes)],
  ["CreatePromo", "CreatePromo", (r) => api.createPromo(r.code, r.minutes)],
  ["ListPromos", "ListPromos", () => api.listPromos()],
  ["ListPromos", "ListPromos#empty", () => api.listPromos()],
  ["RedeemPromo", "RedeemPromo", (r) => api.redeemPromo(r.childId, r.code)],
  ["GetBonusRate", "GetBonusRate", () => api.getBonusRate()],
  ["SetBonusRate", "SetBonusRate", (r) => api.setBonusRate(r.minutes)],
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
    case "FirstRunSeen":
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
    ["ReduceExtra", "ReduceExtra#invalid", (r) => api.reduceExtra(r.childId, r.minutes)],
    ["SaveLimits", "SaveLimits#invalid", (r) => api.saveLimits(r.childId, r.limits as WeekdayLimit[])],
    ["Setup", "Setup#invalid", (r) => api.setup(r.password, r.children)],
    ["SaveSchedule", "SaveSchedule#invalid", (r) => api.saveSchedule(r.childId, r.intervals)],
    ["GrantBonus", "GrantBonus#invalid", (r) => api.grantBonus(r.childId, r.minutes, r.reason)],
    ["AskMore", "AskMore#denied", (r) => api.askMore(r.childId, r.minutes, r.password)],
    ["SpendBonus", "SpendBonus#forbidden", (r) => api.spendBonus(r.childId, r.minutes)],
    ["CreatePromo", "CreatePromo#dup", (r) => api.createPromo(r.code, r.minutes)],
    ["RedeemPromo", "RedeemPromo#bad", (r) => api.redeemPromo(r.childId, r.code)],
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
  // Ловит: в контракте остались команды удалённого Помощника (D-56).
  it("команд Помощника в контракте нет", () => {
    expect(Object.keys(examples).filter((k) => /HelperState/.test(k))).toEqual([]);
  });

  // Ловит: новая команда окна родителя появилась в контракте, а обёртки для неё нет.
  it("у каждой команды окон родителя и ребёнка есть пример и обёртка", () => {
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
      "MyToday",
      "FirstRunSeen",
      "OpenLogs",
      "OpenWindow",
      "SetNoForbidden",
      "ClearBonus",
      "ReduceBonus",
      "CreatePromo",
      "ListPromos",
      "RedeemPromo",
      "GetBonusRate",
      "SetBonusRate",
      "ListPrograms",
      "DecideProgram",
      "SetProgramsCategory",
      "UndoProgram",
      "SetProgramsHidden",
      "RenameProgram",
      "ScanPrograms",
    ]) {
      expect(examples[command], `нет примера ${command}`).toBeDefined();
      expect(wrapped.has(command), `нет обёртки ${command}`).toBe(true);
    }
  });
});
