// Движок вежливости Помощника (ТЗ 8.4 п.3, 4). Решает, какая карточка можно показать прямо
// сейчас и можно ли вообще: одна за раз, не больше 2 подсказок в час и 5 в день, напоминания о
// важном вне этого счёта (но не больше 3 в час), «Позже», крестик, «Больше не напоминать»,
// тихие места, выключатель. Экрана здесь нет: логика проверяется модульными тестами,
// а Provider только показывает выбранное.
import { errorText } from "../lib/errors";
import { INVITE_ID, type HintEntry, type TourEntry } from "./catalog";
import {
  DAY_MS,
  HOUR_MS,
  parse,
  withAdded,
  withClosed,
  withEnabled,
  withShown,
  withSnooze,
  withoutHidden,
  type HelperData,
  type Raw,
} from "./state";

/** Лимиты вежливости (ТЗ 8.4 п.3). */
export const LIMITS = { hintsPerHour: 2, hintsPerDay: 5, remindersPerHour: 3 } as const;

/** Тихие места, где Помощник молчит (ТЗ 8.4 п.4). */
export type QuietZone = "lock" | "warning" | "password" | "wizard" | "dialog" | "tour";
export const QUIET_ZONES: readonly QuietZone[] = ["lock", "warning", "password", "wizard", "dialog", "tour"];

/** Кого можно показать: запись каталога, для какого ребёнка и в каком месте экрана. */
export interface Candidate {
  /** Ключ состояния: номер записи и id ребёнка у напоминаний ("D-M1-01:7"). */
  key: string;
  entry: HintEntry;
  /** id ребёнка строкой или пусто. */
  subject: string;
  /** Имя ребёнка для подстановки в текст. */
  name: string;
  /** Место на экране, где карточка встаёт: "якорь|подчинённый". */
  slot: string;
}

export interface PickContext {
  data: HelperData;
  now: number;
  /** Метка открытия окна: «Позже» и крестик считаются по ней. */
  windowId: number;
  /** Что уже показывали в этом окне. */
  shown: ReadonlySet<string>;
  /** Открыто тихое место. */
  quiet: boolean;
  /** После ответа на карточку ждём следующего открытия экрана. */
  hold: boolean;
}

/** Хватает ли «бюджета» показов (по журналу последних суток). */
export function budgetAllows(kind: HintEntry["kind"], data: HelperData, now: number): boolean {
  if (kind === "detector") {
    return data.remindLog.filter((t) => t > now - HOUR_MS).length < LIMITS.remindersPerHour;
  }
  const hour = data.hintLog.filter((t) => t > now - HOUR_MS).length;
  const day = data.hintLog.filter((t) => t > now - DAY_MS).length;
  return hour < LIMITS.hintsPerHour && day < LIMITS.hintsPerDay;
}

/** Можно ли эту запись показывать по её собственной истории (без бюджета и тихих мест). */
export function isEligible(c: Candidate, ctx: Pick<PickContext, "data" | "windowId" | "shown">): boolean {
  const { data, windowId, shown } = ctx;
  const key = c.key;
  if (!data.enabled || data.done.has(key) || data.muted.has(key) || shown.has(key)) return false;
  const closed = data.closed[key];
  if (closed && (closed.n >= 2 || closed.w === windowId)) return false;
  const later = data.snoozed[key];
  if (later) {
    // «Позже»: не раньше следующего открытия окна и не больше одного возврата.
    return later.n < 2 && later.w !== windowId;
  }
  if (closed) return true; // закрыта один раз в прошлом окне: второй шанс
  if (c.entry.kind === "detector") return true; // риск напоминают, пока он есть
  return !data.seen.has(key);
}

/** Выбор следующей карточки: сначала напоминания о риске, потом подсказки по порядку каталога. */
export function pick(candidates: readonly Candidate[], ctx: PickContext): Candidate | null {
  if (!ctx.data.enabled || ctx.quiet || ctx.hold) return null;
  const ordered = [
    ...candidates.filter((c) => c.entry.kind === "detector"),
    ...candidates.filter((c) => c.entry.kind === "hint"),
  ];
  for (const c of ordered) {
    if (isEligible(c, ctx) && budgetAllows(c.entry.kind, ctx.data, ctx.now)) return c;
  }
  return null;
}

/** Открытые тихие места: тишина, пока открыто хотя бы одно. */
export class QuietZones {
  private counts = new Map<QuietZone, number>();
  private listeners = new Set<() => void>();

  /** Входит в тихое место. Возвращает выход; повторный выход ничего не меняет. */
  enter(zone: QuietZone): () => void {
    this.counts.set(zone, (this.counts.get(zone) ?? 0) + 1);
    this.emit();
    let left = false;
    return () => {
      if (left) return;
      left = true;
      const n = (this.counts.get(zone) ?? 1) - 1;
      if (n <= 0) this.counts.delete(zone);
      else this.counts.set(zone, n);
      this.emit();
    };
  }

  active(): QuietZone[] {
    return [...this.counts.keys()];
  }

  isQuiet(): boolean {
    return this.counts.size > 0;
  }

  subscribe(fn: () => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  private emit(): void {
    this.listeners.forEach((fn) => fn());
  }
}

/** Где лежит состояние: в программе, через GetHelperState и SaveHelperState. */
export interface HelperIO {
  load(): Promise<Raw>;
  save(raw: Raw): Promise<void>;
}

export interface EngineOptions {
  now?: () => number;
  /** Метка открытия окна. По умолчанию - момент создания движка. */
  windowId?: number;
}

/** Состояние Помощника профиля и всё, что с ним делают: выбор, ответы, записи в базу. */
export class HelperEngine {
  ready = false;
  loadFailed = false;
  /** Почему не прочиталось, словами для человека. */
  loadError = "";
  readonly windowId: number;
  private raw: Raw = {};
  private parsed: HelperData | null = null;
  private readonly shown = new Set<string>();
  private readonly listeners = new Set<() => void>();
  private queue: Promise<void> = Promise.resolve();
  private readonly now: () => number;

  constructor(
    private readonly io: HelperIO,
    opts: EngineOptions = {},
  ) {
    this.now = opts.now ?? Date.now;
    this.windowId = opts.windowId ?? this.now();
  }

  get data(): HelperData {
    this.parsed ??= parse(this.raw);
    return this.parsed;
  }

  get enabled(): boolean {
    return this.data.enabled;
  }

  /** Читает состояние. Не вышло - Помощник молчит и ничего не пишет (чтобы не затереть сохранённое). */
  async load(): Promise<void> {
    try {
      const raw = await this.io.load();
      this.raw = typeof raw === "object" && raw !== null && !Array.isArray(raw) ? raw : {};
      this.parsed = null;
      this.ready = true;
      this.loadFailed = false;
      this.loadError = "";
    } catch (e) {
      this.loadFailed = true;
      this.loadError = errorText(e);
    }
    this.emit();
  }

  subscribe(fn: () => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  /** Подождать, пока уйдут все записи в очереди. Сбои записей не прерывают ожидание. */
  flush(): Promise<void> {
    return this.queue;
  }

  /** Выбрать карточку для показа. Пока состояние не прочитано, не выбирает ничего. */
  pick(candidates: readonly Candidate[], quiet: boolean, hold: boolean): Candidate | null {
    if (!this.ready) return null;
    return pick(candidates, { data: this.data, now: this.now(), windowId: this.windowId, shown: this.shown, quiet, hold });
  }

  /** Карточка показана: в журнал, в «показано» и в базу. */
  markShown(c: Candidate): void {
    if (!this.ready) return;
    this.shown.add(c.key);
    let next = withShown(this.raw, c.entry.kind === "detector" ? "detector" : "hint", this.now());
    if (c.entry.kind === "hint") next = withAdded(next, "seen", c.key);
    this.commit(next);
  }

  /** «Позже». */
  later(c: Candidate): void {
    this.commit(withSnooze(this.raw, c.key, this.windowId));
  }

  /** Крестик. Приглашение, которое показывают один раз, закрывается насовсем. */
  close(c: Candidate): void {
    this.commit(c.entry.once ? withAdded(this.raw, "done", c.key) : withClosed(this.raw, c.key, this.windowId));
  }

  /** «Больше не напоминать». */
  mute(c: Candidate): void {
    this.commit(withAdded(this.raw, "muted", c.key));
  }

  /** Принято: нажали действие, больше не показывать. */
  accept(c: Candidate): void {
    this.commit(withAdded(this.raw, "done", c.key));
  }

  /** Тур запущен: приглашение в него принято. */
  startedTour(): void {
    this.commit(withAdded(this.raw, "done", INVITE_ID));
  }

  /** Тур пройден до конца: он записан, а подсказки, которые он объяснил, больше не нужны. */
  finishTour(tour: TourEntry): void {
    let next = withAdded(this.raw, "tours", tour.id);
    for (const id of tour.covers) next = withAdded(next, "done", id);
    this.commit(next);
  }

  /** «Вернуть скрытые подсказки». */
  async resetHidden(): Promise<void> {
    this.shown.clear();
    this.commit(withoutHidden(this.raw));
    await this.flush();
  }

  /**
   * Выключатель «Показывать подсказки». Меняет на экране сразу и пишет в базу; не записалось -
   * возвращает прежнее и бросает ошибку (её текст покажет экран настроек).
   */
  async setEnabled(on: boolean): Promise<void> {
    const before = { raw: this.raw, ready: this.ready, loadFailed: this.loadFailed, loadError: this.loadError };
    // Состояние не прочиталось: пользователь явно выбрал, начинаем его с чистого листа.
    this.raw = withEnabled(this.ready ? this.raw : {}, on);
    this.parsed = null;
    this.ready = true;
    this.loadFailed = false;
    this.loadError = "";
    this.emit();
    try {
      await this.io.save(this.raw);
    } catch (e) {
      Object.assign(this, { raw: before.raw, ready: before.ready, loadFailed: before.loadFailed, loadError: before.loadError });
      this.parsed = null;
      this.emit();
      throw e;
    }
  }

  private commit(next: Raw): void {
    // Состояние не прочиталось: запись перетёрла бы сохранённое, поэтому ничего не пишем.
    if (!this.ready) return;
    this.raw = next;
    this.parsed = null;
    this.emit();
    // Запись в очереди берёт самое новое состояние на момент отправки; сбой записи не мешает работе.
    this.queue = this.queue
      .then(() => this.io.save(this.raw))
      .catch(() => undefined);
  }

  private emit(): void {
    this.listeners.forEach((fn) => fn());
  }
}
