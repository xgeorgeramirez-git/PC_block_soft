// Экран «Программы» (FR-20, ТЗ 8.3.4): подписи категорий, путь, «Запускали: …», время за неделю,
// поиск, фильтр и сортировка списка. Только показ: категорию и решение считает служба.
import { formatDuration, formatEventTime, formatHour } from "./format";
import type { PendingChildDTO, ProgramCategory, ProgramDTO } from "../types";

/** Категория, которую родитель может выбрать; «system» не показывается и не выбирается. */
export type ParentCategory = Exclude<ProgramCategory, "system">;

/** Кнопки решения у ждущей программы (8.3.4 п.1). */
export const DECISIONS: { category: ParentCategory; label: string }[] = [
  { category: "games", label: "Игра" },
  { category: "study", label: "Учёба" },
  { category: "other", label: "Прочее" },
  { category: "blocked", label: "Запретить" },
];

/** Выбор категории в строке списка и в панели «Выбрано: N». */
export const CATEGORY_OPTIONS: { category: ParentCategory; label: string }[] = [
  { category: "games", label: "Игры" },
  { category: "study", label: "Учёба" },
  { category: "other", label: "Прочее" },
  { category: "blocked", label: "Запрещено" },
];

export type ProgramFilter = "all" | ParentCategory;

export const FILTERS: { id: ProgramFilter; label: string }[] = [
  { id: "all", label: "Все" },
  ...CATEGORY_OPTIONS.map((o) => ({ id: o.category, label: o.label })),
];

/** Слово категории в сообщении «SuperGame: игра» и в пометке «Раньше было: игра». */
const CATEGORY_WORD: Record<ParentCategory, string> = {
  games: "игра",
  study: "учёба",
  other: "прочее",
  blocked: "запрещена",
};

function isParentCategory(c: string): c is ParentCategory {
  return Object.prototype.hasOwnProperty.call(CATEGORY_WORD, c);
}

/** «Игры», «Учёба», «Прочее», «Запрещено»; у системной категории подписи нет. */
export function categoryTitle(category: ProgramCategory): string {
  return CATEGORY_OPTIONS.find((o) => o.category === category)?.label ?? "";
}

/** «Раньше было: игра», если файл программы сменился после решения родителя (RM3-12). */
export function wasCategoryText(was: string): string {
  const w = was.trim();
  if (w === "" || w === "system") return "";
  const word = isParentCategory(w) ? CATEGORY_WORD[w] : w.toLowerCase();
  return `Раньше было: ${word}`;
}

/** Издатель из свойств файла или «Без подписи издателя» (RM3-3). */
export function publisherText(publisher: string): string {
  const p = publisher.trim();
  return p === "" ? "Без подписи издателя" : p;
}

/** «SuperGame: игра» (8.3.4 п.2); служба присылает такое же, это запасной текст. */
export function decisionMessage(name: string, category: ParentCategory): string {
  return `${name}: ${CATEGORY_WORD[category]}`;
}

/**
 * Длинный путь обрезан по центру: начало, «…», конец с именем файла (и папкой над ним, если
 * помещается). Итог не длиннее max знаков.
 */
export function shortenPath(path: string, max: number): string {
  if (path.length <= max) return path;
  const room = max - 1;
  const parts = path.split("\\");
  const file = parts.length > 1 ? `\\${parts[parts.length - 1]}` : "";
  const withParent = parts.length > 2 ? `\\${parts[parts.length - 2]}${file}` : "";
  let tail: string;
  if (withParent && withParent.length <= room * 0.6) {
    tail = withParent;
  } else if (file && file.length <= room - 8) {
    tail = file;
  } else {
    tail = path.slice(path.length - Math.floor(room / 2));
  }
  // Пробел из середины имени вплотную к «…» читается как «очень …\» или «… умножения.exe»:
  // его убираем с обеих сторон.
  return `${path.slice(0, room - tail.length).trimEnd()}…${tail.trimStart()}`;
}

/** Короче этого путь не обрезается: остаются диск, «…» и конец имени файла. */
const MIN_PATH_CHARS = 12;

/**
 * Путь под ширину строки: самый длинный вид shortenPath (не длиннее max), для которого fits
 * отвечает «помещается». Единственный способ обрезки пути на экране: CSS-многоточие поверх уже
 * обрезанного пути дало бы второе «…» и пробел перед ним. Не помещается ничего — самый короткий.
 */
export function fitPath(path: string, max: number, fits: (shown: string) => boolean): string {
  let last = "";
  for (let n = Math.min(max, path.length); n >= MIN_PATH_CHARS; n--) {
    const shown = shortenPath(path, n);
    if (shown === last) continue;
    last = shown;
    if (fits(shown)) return shown;
  }
  return shortenPath(path, MIN_PATH_CHARS);
}

/** «1 раз», «2 раза», «5 раз». */
export function timesWord(n: number): string {
  const mod100 = n % 100;
  const mod10 = n % 10;
  if (mod100 >= 11 && mod100 <= 14) return "раз";
  if (mod10 >= 2 && mod10 <= 4) return "раза";
  return "раз";
}

const DAY_MS = 86_400_000;

/** Номер местных суток момента: для «сегодня» и «вчера». */
function dayNumber(ms: number, tzOffsetMin: number): number {
  return Math.floor((ms + tzOffsetMin * 60_000) / DAY_MS);
}

/** «сегодня 09:05», «вчера 18:40», иначе «28 окт, 23:30». */
export function whenText(ms: number, nowMs: number, tzOffsetMin: number): string {
  const diff = dayNumber(nowMs, tzOffsetMin) - dayNumber(ms, tzOffsetMin);
  const clock = formatHour(Math.floor((((ms + tzOffsetMin * 60_000) % DAY_MS) + DAY_MS) % DAY_MS / 60_000));
  if (diff === 0) return `сегодня ${clock}`;
  if (diff === 1) return `вчера ${clock}`;
  return formatEventTime(ms, tzOffsetMin);
}

/** «Запускали: Миша 3 раза, Аня 1 раз; последняя попытка вчера 18:40». */
export function attemptsText(
  byChild: PendingChildDTO[],
  attempts: number,
  lastAttemptAt: number,
  nowMs: number,
  tzOffsetMin: number,
): string {
  let who = "";
  if (byChild.length > 0) {
    who = `Запускали: ${byChild.map((c) => `${c.name} ${c.attempts} ${timesWord(c.attempts)}`).join(", ")}`;
  } else if (attempts > 0) {
    who = `Запускали ${attempts} ${timesWord(attempts)}`;
  }
  if (who === "") return "";
  if (lastAttemptAt <= 0) return who;
  return `${who}; последняя попытка ${whenText(lastAttemptAt, nowMs, tzOffsetMin)}`;
}

/** Время всех детей за 7 дней: «2 ч 05 мин»; не запускали за неделю: «нет». */
export function weekText(seconds: number): string {
  if (seconds <= 0) return "нет";
  if (seconds < 60) return "меньше минуты";
  return formatDuration(seconds);
}

export interface ProgramSort {
  key: "name" | "week";
  dir: "asc" | "desc";
}

export interface ProgramView {
  query: string;
  filter: ProgramFilter;
  sort: ProgramSort;
}

/** Порядок названий как у службы: без учёта регистра, латиница раньше кириллицы. */
function byName(a: ProgramDTO, b: ProgramDTO): number {
  return a.name.localeCompare(b.name, "en", { sensitivity: "base" }) || a.path.localeCompare(b.path, "en");
}

/** Строки списка «Все программы»: без системных, с поиском, фильтром и сортировкой. */
export function visiblePrograms(list: ProgramDTO[], view: ProgramView): ProgramDTO[] {
  const q = view.query.trim().toLowerCase();
  const shown = list.filter(
    (p) =>
      p.category !== "system" &&
      (view.filter === "all" || p.category === view.filter) &&
      (q === "" || p.name.toLowerCase().includes(q) || p.path.toLowerCase().includes(q)),
  );
  const sign = view.sort.dir === "asc" ? 1 : -1;
  return shown.sort((a, b) => {
    if (view.sort.key === "week" && a.weekS !== b.weekS) {
      return sign * (a.weekS - b.weekS);
    }
    return view.sort.key === "name" ? sign * byName(a, b) : byName(a, b);
  });
}
