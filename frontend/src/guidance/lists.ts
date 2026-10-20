// Мелкие помощники для списков «Помощи»: склонение числительных и порядок версий.
import type { WhatsNewEntry } from "./catalog";

/** «1 шаг», «2 шага», «5 шагов»: форма слова по числу. */
export function plural(n: number, forms: readonly [string, string, string]): string {
  if (n % 10 === 1 && n % 100 !== 11) return forms[0];
  if (n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 10 || n % 100 >= 20)) return forms[1];
  return forms[2];
}

/** Версии от новой к старой: 0.10.0 новее 0.9.0, а не наоборот, как вышло бы при сравнении строк. */
export function newestFirst(list: readonly WhatsNewEntry[]): WhatsNewEntry[] {
  const parts = (v: string) => v.split(".").map((x) => Number.parseInt(x, 10) || 0);
  return [...list].sort((a, b) => {
    const pa = parts(a.version);
    const pb = parts(b.version);
    for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
      const d = (pb[i] ?? 0) - (pa[i] ?? 0);
      if (d !== 0) return d;
    }
    return 0;
  });
}
