// Файлы стикеров промокода (RM5b-8): 17 .tgs из assets/stickers/ идут в сборку как ассеты (?url),
// остальные стикеры заказчика в программу не попадают (tools/check-dist.sh --tgs-exact 17).
// .tgs - Lottie JSON в gzip; распаковка - DecompressionStream (WebView2, Chromium).
// В vitest этот модуль подменяют: в jsdom нет загрузки файлов по адресу сборки.
import u017 from "../../../assets/stickers/017.tgs?url";
import u033 from "../../../assets/stickers/033.tgs?url";
import u044 from "../../../assets/stickers/044.tgs?url";
import u049 from "../../../assets/stickers/049.tgs?url";
import u050 from "../../../assets/stickers/050.tgs?url";
import u077 from "../../../assets/stickers/077.tgs?url";
import u093 from "../../../assets/stickers/093.tgs?url";
import u096 from "../../../assets/stickers/096.tgs?url";
import u112 from "../../../assets/stickers/112.tgs?url";
import u023 from "../../../assets/stickers/023.tgs?url";
import u028 from "../../../assets/stickers/028.tgs?url";
import u048 from "../../../assets/stickers/048.tgs?url";
import u069 from "../../../assets/stickers/069.tgs?url";
import u082 from "../../../assets/stickers/082.tgs?url";
import u084 from "../../../assets/stickers/084.tgs?url";
import u095 from "../../../assets/stickers/095.tgs?url";
import u110 from "../../../assets/stickers/110.tgs?url";

/** Адрес файла стикера в сборке по номеру. */
export const STICKER_URLS: Readonly<Record<string, string>> = {
  "017": u017,
  "033": u033,
  "044": u044,
  "049": u049,
  "050": u050,
  "077": u077,
  "093": u093,
  "096": u096,
  "112": u112,
  "023": u023,
  "028": u028,
  "048": u048,
  "069": u069,
  "082": u082,
  "084": u084,
  "095": u095,
  "110": u110,
};

type FetchFn = (url: string) => Promise<Response>;

/**
 * Текст .tgs: gzip распаковывается потоком браузера. Если сервер уже распаковал файл сам
 * (заголовок Content-Encoding), байты - готовый JSON: признак gzip - первые байты 1f 8b.
 */
export async function tgsText(bytes: Uint8Array<ArrayBuffer>): Promise<string> {
  if (bytes.length < 2 || bytes[0] !== 0x1f || bytes[1] !== 0x8b) return new TextDecoder().decode(bytes);
  // Поток из Response, а не Blob.stream(): у Blob в jsdom (тесты) метода stream нет.
  const plain = new Response(bytes).body!.pipeThrough(new DecompressionStream("gzip"));
  return new Response(plain).text();
}

const cache = new Map<string, Promise<unknown>>();

/**
 * Данные анимации стикера (Lottie JSON). Один стикер грузится один раз; сбой не запоминается,
 * следующий показ пробует снова.
 */
export function loadStickerData(n: string, fetchFn: FetchFn = (u) => fetch(u)): Promise<unknown> {
  const hit = cache.get(n);
  if (hit) return hit;
  const p = (async () => {
    const url = STICKER_URLS[n];
    if (!url) throw new Error(`стикер ${n}: нет в наборе`);
    const res = await fetchFn(url);
    if (!res.ok) throw new Error(`стикер ${n}: ответ ${res.status}`);
    return JSON.parse(await tgsText(new Uint8Array(await res.arrayBuffer()))) as unknown;
  })();
  cache.set(n, p);
  p.catch(() => {
    if (cache.get(n) === p) cache.delete(n);
  });
  return p;
}

/** Только для тестов: забыть загруженные стикеры. */
export function resetStickerDataForTest(): void {
  cache.clear();
}
