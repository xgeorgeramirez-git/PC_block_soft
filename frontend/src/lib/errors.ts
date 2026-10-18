// Человеческие тексты вместо технических ошибок (урок L-112).
// Программа отдаёт в Reply.Error готовый текст; всё, что выглядит техническим, и любой сбой
// моста заменяется простыми словами.
import type { Reply } from "../types";

export const LINK_ERROR_TEXT = "Программа не отвечает. Попробуйте ещё раз через минуту";
export const GENERIC_ERROR_TEXT = "Не получилось выполнить действие. Попробуйте ещё раз";

/** Отказ команды или сбой связи. message всегда пригоден для показа человеку. */
export class CallError extends Error {
  readonly field: string | undefined;
  readonly payload: unknown;
  readonly link: boolean;

  constructor(message: string, opts: { field?: string; payload?: unknown; link?: boolean } = {}) {
    super(message);
    this.name = "CallError";
    this.field = opts.field || undefined;
    this.payload = opts.payload;
    this.link = opts.link ?? false;
  }
}

const TECHNICAL: RegExp[] = [
  /\b(ipc|rpc|nil|null|undefined|panic|goroutine|errno|hresult|syscall|eof|exception|stacktrace)\b/i,
  /\b\w*error\b\s*[:(]/i,
  /failed to fetch|is not a function|cannot read propert/i,
  /0x[0-9a-f]{4,}/i,
  /\.(go|ts|tsx|js):\d+/i,
  /[{}[\]]/,
  /\\\\|:\\/,
  /^[a-z_.]+: /i,
  /\bthe system cannot\b/i,
];

/** Похож ли текст на технический (не для человека). Пустой текст тоже считается техническим. */
export function isTechnical(text: string): boolean {
  const t = text.trim();
  if (t === "") {
    return true;
  }
  return TECHNICAL.some((re) => re.test(t));
}

/** Ошибка связи с программой: сбой моста, оборванный или нечитаемый ответ. */
export function linkError(): CallError {
  return new CallError(LINK_ERROR_TEXT, { link: true });
}

/** Отказ программы (Reply.ok = false) → ошибка с текстом для человека и полем формы. */
export function replyError(reply: Reply): CallError {
  const text = typeof reply.error === "string" ? reply.error : "";
  return new CallError(isTechnical(text) ? GENERIC_ERROR_TEXT : text.trim(), {
    field: reply.field,
    payload: reply.payload,
  });
}

/** Любую пойманную ошибку превращает в строку для человека. */
export function errorText(e: unknown): string {
  return e instanceof CallError ? e.message : LINK_ERROR_TEXT;
}

/** Поле формы, к которому относится отказ программы (L-121); у сбоя связи поля нет. */
export function errorField(e: unknown): string | undefined {
  return e instanceof CallError ? e.field : undefined;
}
