// Состояние Помощника одного профиля (ТЗ 8.4 п.7). Лежит в базе программы как произвольный
// объект (GetHelperState/SaveHelperState); смысл полей знает только этот файл. Окно читает его
// терпимо (битое - «как впервые») и меняет по одному полю, не затирая остальные.
//
//   enabled   - «Показывать подсказки» (по умолчанию включено)
//   seen      - ключи записей, показанных хотя бы раз
//   done      - принятые (нажали действие), больше не показываются
//   muted     - «Больше не напоминать»
//   snoozed   - «Позже»: ключ -> { n: сколько раз, w: в каком открытии окна }
//   closed    - крестик: ключ -> { n: сколько раз, w: в каком открытии окна }
//   tours     - пройденные туры
//   log       - когда показывали подсказки и напоминания за последние сутки (счётчики вежливости)
//
// Ключ записи: её номер, а у напоминания про ребёнка ещё и его id ("D-M1-01:7").
export type Raw = Record<string, unknown>;

export const HOUR_MS = 3_600_000;
export const DAY_MS = 24 * HOUR_MS;

export interface Counted {
  /** Сколько раз нажали. */
  n: number;
  /** В каком открытии окна нажали (метка окна, не время). */
  w: number;
}

export interface HelperData {
  enabled: boolean;
  seen: ReadonlySet<string>;
  done: ReadonlySet<string>;
  muted: ReadonlySet<string>;
  tours: ReadonlySet<string>;
  snoozed: Readonly<Record<string, Counted>>;
  closed: Readonly<Record<string, Counted>>;
  hintLog: readonly number[];
  remindLog: readonly number[];
}

type SetField = "seen" | "done" | "muted" | "tours";

export function keyOf(id: string, subject?: string): string {
  return subject ? `${id}:${subject}` : id;
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function strings(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
}

function numbers(v: unknown): number[] {
  return Array.isArray(v) ? v.filter((x): x is number => typeof x === "number" && Number.isFinite(x)) : [];
}

function counted(v: unknown): Record<string, Counted> {
  const out: Record<string, Counted> = {};
  if (!isObject(v)) return out;
  for (const [key, c] of Object.entries(v)) {
    if (isObject(c) && typeof c.n === "number" && typeof c.w === "number" && Number.isFinite(c.n) && Number.isFinite(c.w)) {
      out[key] = { n: c.n, w: c.w };
    }
  }
  return out;
}

/** Читает состояние. Что угодно, кроме объекта, - «как впервые». */
export function parse(raw: unknown): HelperData {
  const r = isObject(raw) ? raw : {};
  const log = isObject(r.log) ? r.log : {};
  return {
    enabled: r.enabled !== false,
    seen: new Set(strings(r.seen)),
    done: new Set(strings(r.done)),
    muted: new Set(strings(r.muted)),
    tours: new Set(strings(r.tours)),
    snoozed: counted(r.snoozed),
    closed: counted(r.closed),
    hintLog: numbers(log.hint),
    remindLog: numbers(log.remind),
  };
}

function base(raw: Raw): Raw {
  return isObject(raw) ? raw : {};
}

export function withEnabled(raw: Raw, on: boolean): Raw {
  return { ...base(raw), enabled: on };
}

export function withAdded(raw: Raw, field: SetField, key: string): Raw {
  const list = strings(base(raw)[field]);
  return list.includes(key) ? { ...base(raw) } : { ...base(raw), [field]: [...list, key] };
}

function bump(raw: Raw, field: "snoozed" | "closed", key: string, w: number): Raw {
  const map = counted(base(raw)[field]);
  const n = (map[key]?.n ?? 0) + 1;
  return { ...base(raw), [field]: { ...map, [key]: { n, w } } };
}

/** «Позже»: ещё раз, в таком-то открытии окна. */
export function withSnooze(raw: Raw, key: string, w: number): Raw {
  return bump(raw, "snoozed", key, w);
}

/** Закрыли крестиком: ещё раз, в таком-то открытии окна. */
export function withClosed(raw: Raw, key: string, w: number): Raw {
  return bump(raw, "closed", key, w);
}

/** Записывает показ карточки; показы старше суток забываются (счётчики не растут без конца). */
export function withShown(raw: Raw, kind: "hint" | "detector", now: number): Raw {
  const d = parse(raw);
  const fresh = (list: readonly number[]) => list.filter((t) => t > now - DAY_MS);
  const hint = fresh(d.hintLog);
  const remind = fresh(d.remindLog);
  (kind === "hint" ? hint : remind).push(now);
  return { ...base(raw), log: { hint, remind } };
}

/** «Вернуть скрытые подсказки»: отметки снимаются, выключатель, туры и счётчики остаются. */
export function withoutHidden(raw: Raw): Raw {
  const out: Raw = { ...base(raw) };
  for (const field of ["seen", "done", "muted", "snoozed", "closed"]) {
    delete out[field];
  }
  return out;
}
