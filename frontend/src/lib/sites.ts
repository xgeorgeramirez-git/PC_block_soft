// «Сайты» (FR-21, R14, RM4-4): списки, их пояснения и копии констант службы.
// Домен разбирает служба (sites.NormalizeInput); cleanDomain — упрощённое зеркало только там, где
// службы нет: шаг мастера и подменная программа (PL-39). Экран «Сайты» его не зовёт.
import type { SiteDTO, SiteKind } from "../types";

/** Отказ у поля, если ввод не домен (internal/service/texts.go textNotDomain). */
export const NOT_DOMAIN_TEXT = "Это не похоже на адрес сайта";
/** Отказ у поля при повторе в тот же список (textSiteDuplicate). */
export const DUPLICATE_TEXT = "Этот сайт уже в списке";
/** Подпись источника списка «18+» (textAdultSource, D-67). */
export const ADULT_SOURCE_TEXT = "Список сайтов для взрослых: UT1, Университет Тулуза Капитоль, лицензия CC BY-SA 4.0";

/** Подсказка под полем «Добавить сайт» на экране «Сайты» и в мастере: видна и при ошибке. */
export const SITE_HINT_TEXT = "Можно вставить адрес страницы целиком: программа возьмёт из него только сайт. Например: youtube.com";

/**
 * Подпись источника для показа: «лицензия CC BY-SA 4.0» через неразрывные пробелы и неразрывный
 * дефис (U+2011), чтобы фрагмент не рвался ни на пробеле, ни на дефисе «BY-SA» в окне 960 px.
 * Сам текст (копия службы) не меняется.
 */
export function sourceDisplay(text: string): string {
  return text.replace("лицензия CC BY-SA 4.0", "лицензия\u00a0CC\u00a0BY\u2011SA\u00a04.0");
}

/** Стартовый набор «Видео и соцсетей» (ipc.DefaultVideoSites, миграция 009). */
export const DEFAULT_VIDEO_SITES: readonly string[] = [
  "youtube.com",
  "vk.com",
  "tiktok.com",
  "rutube.ru",
  "ok.ru",
  "telegram.org",
  "web.telegram.org",
  "discord.com",
  "twitch.tv",
];

/**
 * Под пояснением каждого списка (8.3.5 п.2, B-50): сайт действует вместе с адресами внутри него
 * (R14, совпадение — домен и его поддомены), без слова «поддомены».
 */
export const SITE_SCOPE_TEXT =
  "Каждый сайт действует вместе со всеми адресами внутри него: например, youtube.com вместе с m.youtube.com и music.youtube.com";

export interface SiteList {
  kind: SiteKind;
  /** Название вкладки. */
  title: string;
  /** Пояснение под вкладками: что делает список. */
  lead: string;
  /** Текст пустого списка. */
  empty: string;
}

/** Три списка в порядке вкладок (8.3.5 п.2). */
export const SITE_LISTS: SiteList[] = [
  {
    kind: "blocked",
    title: "Запрещённые",
    lead: "Эти сайты закрываются в Brave у всех детей",
    empty: "Запрещённых сайтов пока нет",
  },
  {
    kind: "study",
    title: "Учёба",
    lead: "Время на этих сайтах не тратит лимит. Когда время вышло и выбрано «Оставить только учёбу», в Brave открываются только они",
    empty: "Учебных сайтов пока нет",
  },
  {
    kind: "video_social",
    title: "Видео и соцсети",
    lead: "Время на этих сайтах идёт в лимит «Видео и соцсети» и в общее время",
    empty: "В этом списке пока нет сайтов",
  },
];

const MAX_DOMAIN = 253;

/**
 * Домен из ввода: домен или вставленный адрес страницы. Нижний регистр, IDN в punycode (это делает
 * URL), без «www.» в начале (пока после него есть точка), без точки в конце, без порта и логина.
 * «» — не домен: пробел внутри, не http/https, нет точки, длиннее 253 знаков, IPv6.
 */
export function cleanDomain(raw: string): string {
  let s = raw.trim();
  if (s.toLowerCase().startsWith("view-source:")) s = s.slice("view-source:".length).trim();
  if (s === "" || /\s/.test(s)) return "";
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(s)) s = `http://${s}`;
  let host: string;
  try {
    const url = new URL(s);
    if (url.protocol !== "http:" && url.protocol !== "https:") return "";
    host = url.hostname.toLowerCase();
  } catch {
    return "";
  }
  if (host.endsWith(".")) host = host.slice(0, -1);
  while (host.startsWith("www.") && host.slice(4).includes(".")) host = host.slice(4);
  if (host.startsWith("[") || !host.includes(".") || host.length > MAX_DOMAIN) return "";
  return host;
}

/** Сайты одного списка в порядке ответа службы. */
export function sitesOf(list: SiteDTO[], kind: SiteKind): SiteDTO[] {
  return list.filter((s) => s.kind === kind);
}

/** Поиск по списку: подстрока домена без учёта регистра; пустой запрос — весь список. */
export function filterSites(list: SiteDTO[], query: string): SiteDTO[] {
  const q = query.trim().toLowerCase();
  if (q === "") return list;
  return list.filter((s) => s.domain.toLowerCase().includes(q));
}
