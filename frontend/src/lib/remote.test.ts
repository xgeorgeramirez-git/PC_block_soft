import { describe, expect, it } from "vitest";
import contract from "../../../internal/ipc/testdata/contract.json";
import type { RemoteView } from "../types";
import {
  REMOTE_DEFAULT_URL,
  canPair,
  countdownText,
  formatCode,
  formatSas,
  isPairing,
  pairErrorText,
  pairButtonLabel,
  pairedLine,
  pollMs,
  remoteErrorText,
  statusLine,
  statusTone,
} from "./remote";

// Москва: UTC+3. Моменты - константами (не у границы суток).
const MSK = 180;
const AT_2140 = Date.UTC(2026, 10, 18, 18, 40, 0); // 18.11.2026 21:40 по Москве
const AT_1905 = Date.UTC(2026, 10, 18, 16, 5, 0); // 18.11.2026 19:05 по Москве

const view = (over: Partial<RemoteView> = {}): RemoteView => ({
  enabled: true,
  url: REMOTE_DEFAULT_URL,
  state: "online",
  lastOkMs: AT_1905,
  error: "",
  pair: { stage: "idle", code: "", expiresMs: 0, sas: "", phoneName: "", error: "" },
  ...over,
});

describe("код и число сверки", () => {
  it("делит шесть цифр пробелом посередине", () => {
    expect(formatCode("123456")).toBe("123 456");
    expect(formatCode("482913")).toBe("482 913");
    expect(formatSas("731004")).toBe("731 004");
    expect(formatSas("000017")).toBe("000 017");
  });
  it("не шесть цифр и уже разбитое - как есть", () => {
    expect(formatCode("")).toBe("");
    expect(formatSas("731 004")).toBe("731 004");
    expect(formatCode("12345")).toBe("12345");
  });
});

describe("отсчёт до конца кода", () => {
  it("минуты:секунды, секунды двумя цифрами", () => {
    expect(countdownText(AT_2140 + 10 * 60_000, AT_2140)).toBe("10:00");
    expect(countdownText(AT_2140 + 9 * 60_000 + 41_000, AT_2140)).toBe("9:41");
    expect(countdownText(AT_2140 + 5_000, AT_2140)).toBe("0:05");
  });
  it("доли секунды округляются вверх: 0:00 только когда срок вышел", () => {
    expect(countdownText(AT_2140 + 400, AT_2140)).toBe("0:01");
    expect(countdownText(AT_2140, AT_2140)).toBe("0:00");
    expect(countdownText(AT_2140 - 3_000, AT_2140)).toBe("0:00");
  });
});

describe("строка состояния", () => {
  it("на каждое состояние своя фраза", () => {
    expect(statusLine(view({ enabled: false, state: "off" }), AT_2140, MSK)).toBe("Связь выключена");
    expect(statusLine(view({ state: "connecting", lastOkMs: 0 }), AT_2140, MSK)).toBe("Подключаемся…");
    expect(statusLine(view({ state: "online" }), AT_2140, MSK)).toBe("На связи");
    expect(statusLine(view({ state: "offline" }), AT_2140, MSK)).toBe("Нет связи с 19:05");
  });
  it("переподключение после удачного обмена - «Нет связи с HH:MM», а не «Подключаемся»", () => {
    expect(statusLine(view({ state: "connecting" }), AT_2140, MSK)).toBe("Нет связи с 19:05");
  });
  it("связи не было ни разу - без времени", () => {
    expect(statusLine(view({ state: "offline", lastOkMs: 0 }), AT_2140, MSK)).toBe("Нет связи с сервером");
  });
  it("связь пропала в другой день - с датой", () => {
    expect(statusLine(view({ state: "offline", lastOkMs: AT_1905 - 86_400_000 }), AT_2140, MSK)).toBe("Нет связи с 17.11\u00a019:05");
  });
});

describe("тексты ошибок - на «вы», без техники", () => {
  it("ошибки связи", () => {
    expect(remoteErrorText("")).toBe("");
    expect(remoteErrorText("no_server")).toBe("Не удаётся связаться с сервером. Проверьте адрес и интернет");
    expect(remoteErrorText("bad_address")).toBe("По этому адресу нет сервера связи. Проверьте адрес");
    expect(remoteErrorText("lost")).toBe("Сервер не узнал этот компьютер, телефон отвязан. Подключите телефон заново");
  });
  it("ошибки привязки", () => {
    expect(pairErrorText("")).toBe("");
    expect(pairErrorText("mismatch")).toBe("Подключение отменено. Начните заново");
    expect(pairErrorText("expired")).toBe("Время на подключение вышло. Начните заново");
    expect(pairErrorText("aborted")).toBe("Подключение прервано. Начните заново");
    expect(pairErrorText("no_code")).toBe("Не удалось получить код: сервер не отвечает. Проверьте адрес и интернет");
    // Код уже был показан, оборвался ответ телефону: «получить код» здесь неверно (R9).
    expect(pairErrorText("no_server")).toBe("Нет связи с сервером. Попробуйте ещё раз");
  });
  it("неизвестный код - общая фраза, не код", () => {
    expect(remoteErrorText("timeout" as never)).toBe("Не удаётся связаться с сервером. Проверьте адрес и интернет");
    expect(pairErrorText("weird" as never)).toBe("Подключение прервано. Начните заново");
  });
});

describe("подключённый телефон", () => {
  it("имя и момент привязки в местном времени", () => {
    expect(pairedLine({ name: "Pixel 7", pairedAtMs: AT_2140 }, MSK)).toBe("Телефон подключён: Pixel 7, с\u00a018.11\u00a021:40");
  });
  it("без имени - просто «Телефон подключён»", () => {
    expect(pairedLine({ name: "", pairedAtMs: AT_2140 }, MSK)).toBe("Телефон подключён, с\u00a018.11\u00a021:40");
  });
});

describe("опрос и доступность «Подключить телефон»", () => {
  it("раз в секунду, пока идёт привязка, иначе раз в 5 секунд", () => {
    for (const stage of ["starting", "code", "compare"] as const) {
      expect(isPairing(stage)).toBe(true);
      expect(pollMs(view({ pair: { ...view().pair, stage } }))).toBe(1000);
    }
    for (const stage of ["idle", "failed"] as const) {
      expect(isPairing(stage)).toBe(false);
      expect(pollMs(view({ pair: { ...view().pair, stage } }))).toBe(5000);
    }
  });
  it("подключить можно только при включённой связи на связи", () => {
    expect(canPair(view())).toEqual({ ok: true, why: "" });
    expect(canPair(view({ enabled: false, state: "off" })).ok).toBe(false);
    expect(canPair(view({ enabled: false, state: "off" })).why).toBe("Чтобы подключить телефон, включите связь и сохраните адрес сервера");
    expect(canPair(view({ state: "connecting", lastOkMs: 0 })).why).toBe("Подключить телефон можно, когда программа выйдет на связь с сервером");
    expect(canPair(view({ state: "offline" })).why).toBe("Подключить телефон можно, когда связь с сервером восстановится");
  });
  it("причина называет кнопку так же, как она подписана", () => {
    const pixel = { name: "Pixel 7", pairedAtMs: AT_2140 };
    expect(pairButtonLabel(view())).toBe("Подключить телефон");
    expect(pairButtonLabel(view({ phone: pixel }))).toBe("Подключить другой телефон");
    const failed = { ...view().pair, stage: "failed" as const, error: "expired" as const };
    expect(pairButtonLabel(view({ pair: failed }))).toBe("Подключить заново");
    expect(canPair(view({ state: "offline", phone: pixel })).why).toBe("Подключить другой телефон можно, когда связь с сервером восстановится");
    expect(canPair(view({ state: "connecting", lastOkMs: 0, pair: failed })).why).toBe(
      "Подключить заново можно, когда программа выйдет на связь с сервером",
    );
  });
});

describe("цвет состояния", () => {
  it("на связи - зелёный, нет связи - предупреждение, остальное - приглушённый", () => {
    expect(statusTone(view())).toBe("ok");
    expect(statusTone(view({ state: "offline" }))).toBe("warn");
    expect(statusTone(view({ state: "connecting" }))).toBe("warn");
    expect(statusTone(view({ state: "connecting", lastOkMs: 0 }))).toBe("neutral");
    expect(statusTone(view({ state: "offline", lastOkMs: 0 }))).toBe("warn");
    expect(statusTone(view({ enabled: false, state: "off" }))).toBe("neutral");
  });
});

describe("сверка с контрактом", () => {
  it("адрес по умолчанию - как у службы (D-92)", () => {
    const ex = (contract as unknown as Record<string, { reply: { payload: RemoteView } }>).GetRemote;
    expect(ex.reply.payload.url).toBe(REMOTE_DEFAULT_URL);
    expect(REMOTE_DEFAULT_URL).toBe("https://pcblock.atlas404.ru");
  });
});
