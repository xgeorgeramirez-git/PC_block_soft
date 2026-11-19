// Блок «Телефон» в «Настройках» (M7, FR-40): чистые функции текстов и опроса. Коды состояний и
// ошибок - internal/ipc/commands_m7.go; тексты родителю на «вы», без техники.
import type { PairError, PairStage, RemoteError, RemotePhone, RemoteView } from "../types";
import { formatClock, formatTimeOfDay, localOffsetMin, sameLocalDay } from "./format";

/** Адрес сервера связи, если родитель не вводил свой (D-92, ipc.RemoteDefaultURL). */
export const REMOTE_DEFAULT_URL = "https://pcblock.atlas404.ru";

/** Опрос GetRemote: раз в секунду, пока идёт привязка, иначе раз в 5 секунд. */
export const POLL_PAIRING_MS = 1000;
export const POLL_IDLE_MS = 5000;

/** Шесть цифр «123456» → «123 456»; другое - как есть. */
export function formatCode(code: string): string {
  return /^\d{6}$/.test(code) ? `${code.slice(0, 3)} ${code.slice(3)}` : code;
}

/** Число сверки - так же, как код: «731 004». */
export const formatSas = formatCode;

/** Отсчёт до expiresMs: «9:41»; доли секунды вверх, после срока «0:00». */
export function countdownText(expiresMs: number, nowMs: number): string {
  return formatClock(Math.ceil(Math.max(0, expiresMs - nowMs) / 1000));
}

function pad2(n: number): string {
  return n < 10 ? `0${n}` : `${n}`;
}

/** Неразрывный пробел: дата и время не расходятся по строкам. */
const NBSP = "\u00a0";

/** «18.11 21:40» в поясе, через неразрывный пробел. */
function dayTime(ms: number, tzOffsetMin: number): string {
  const d = new Date(ms + tzOffsetMin * 60_000);
  return `${pad2(d.getUTCDate())}.${pad2(d.getUTCMonth() + 1)}${NBSP}${formatTimeOfDay(ms, tzOffsetMin)}`;
}

/**
 * Строка состояния: «Связь выключена», «Подключаемся…» (ещё ни одного удачного обмена), «На связи»,
 * «Нет связи с 19:05» (в другой день - с датой), «Нет связи с сервером» (удачного обмена не было).
 */
export function statusLine(v: RemoteView, nowMs: number, tzOffsetMin: number = localOffsetMin()): string {
  if (!v.enabled || v.state === "off") return "Связь выключена";
  if (v.state === "online") return "На связи";
  if (v.lastOkMs <= 0) return v.state === "connecting" ? "Подключаемся…" : "Нет связи с сервером";
  const when = sameLocalDay(v.lastOkMs, nowMs, tzOffsetMin) ? formatTimeOfDay(v.lastOkMs, tzOffsetMin) : dayTime(v.lastOkMs, tzOffsetMin);
  return `Нет связи с ${when}`;
}

/** Цвет строки состояния и причины под ней (классы .status-*): красный оставлен ошибкам действий. */
export type RemoteTone = "ok" | "warn" | "neutral";

export function statusTone(v: RemoteView): RemoteTone {
  if (!v.enabled || v.state === "off") return "neutral";
  if (v.state === "online") return "ok";
  if (v.state === "connecting" && v.lastOkMs <= 0) return "neutral";
  return "warn";
}

const NO_SERVER_TEXT = "Не удаётся связаться с сервером. Проверьте адрес и интернет";

/** Ошибка связи словами; "" - ошибки нет. */
export function remoteErrorText(code: RemoteError): string {
  switch (code) {
    case "":
      return "";
    case "bad_address":
      return "По этому адресу нет сервера связи. Проверьте адрес";
    case "lost":
      return "Сервер не узнал этот компьютер, телефон отвязан. Подключите телефон заново";
    default:
      return NO_SERVER_TEXT;
  }
}

/** Причина неудачной привязки словами; "" - нет. */
export function pairErrorText(code: PairError): string {
  switch (code) {
    case "":
      return "";
    case "mismatch":
      return "Подключение отменено. Начните заново";
    case "expired":
      return "Время на подключение вышло. Начните заново";
    case "no_code":
      return "Не удалось получить код: сервер не отвечает. Проверьте адрес и интернет";
    case "no_server":
      return "Нет связи с сервером. Попробуйте ещё раз";
    default:
      return "Подключение прервано. Начните заново";
  }
}

/** «Телефон подключён: Pixel 7, с 18.11 21:40». */
export function pairedLine(phone: RemotePhone, tzOffsetMin: number = localOffsetMin()): string {
  const since = `с${NBSP}${dayTime(phone.pairedAtMs, tzOffsetMin)}`;
  return phone.name ? `Телефон подключён: ${phone.name}, ${since}` : `Телефон подключён, ${since}`;
}

/** Идёт привязка: код запрашивается, показан или идёт сверка числа. */
export function isPairing(stage: PairStage): boolean {
  return stage === "starting" || stage === "code" || stage === "compare";
}

/** Пауза до следующего GetRemote. */
export function pollMs(v: RemoteView): number {
  return isPairing(v.pair.stage) ? POLL_PAIRING_MS : POLL_IDLE_MS;
}

/** Подпись кнопки подключения: другой телефон, заново после неудачи или первый. */
export function pairButtonLabel(v: RemoteView): string {
  if (v.phone) return "Подключить другой телефон";
  return v.pair.stage === "failed" ? "Подключить заново" : "Подключить телефон";
}

/** Можно ли нажать кнопку подключения, и если нет - почему (подсказка под кнопкой, её же подписью). */
export function canPair(v: RemoteView): { ok: boolean; why: string } {
  if (!v.enabled || v.state === "off") return { ok: false, why: "Чтобы подключить телефон, включите связь и сохраните адрес сервера" };
  if (v.state === "online") return { ok: true, why: "" };
  const label = pairButtonLabel(v);
  if (v.state === "connecting" && v.lastOkMs <= 0) {
    return { ok: false, why: `${label} можно, когда программа выйдет на связь с сервером` };
  }
  return { ok: false, why: `${label} можно, когда связь с сервером восстановится` };
}
