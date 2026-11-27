// Значки «Игрового лобби» (ТЗ 8.0 п. 4, макет plans/design-0.8.4/kit.py): залитые белые значки в цветных
// плашках и монета копилки с часами. Встроены в программу, без эмодзи (L-45); все — украшение (aria-hidden),
// смысл несёт текст рядом.

const PATHS = {
  game: "M7 7h10a5 5 0 0 1 0 10c-1.6 0-2.4-1-3.2-2h-3.6c-.8 1-1.6 2-3.2 2A5 5 0 0 1 7 7zm-.5 3.2v1.3H5.2v1.6h1.3v1.3h1.6v-1.3h1.3v-1.6H8.1v-1.3zM16 10.5a1.1 1.1 0 1 0 0 2.2 1.1 1.1 0 0 0 0-2.2zm2.2 2.3a1.1 1.1 0 1 0 0 2.2 1.1 1.1 0 0 0 0-2.2z",
  video: "M5 5h14a3 3 0 0 1 3 3v8a3 3 0 0 1-3 3H5a3 3 0 0 1-3-3V8a3 3 0 0 1 3-3zm5 4v6l5-3z",
  book: "M5 3h11a3 3 0 0 1 3 3v15H8a3 3 0 0 1-3-3zm3 13a1 1 0 0 0 0 2h9v-2z",
  moon: "M20.5 14.6A8.5 8.5 0 1 1 9.4 3.5a7 7 0 0 0 11.1 11.1z",
  check: "M9.5 16.2 5.3 12l-1.8 1.8 6 6L21 8.3l-1.8-1.8z",
  key: "M14 3a7 7 0 1 1-3.3 13.2L9 18H7v2H5v2H2v-3.4l7.8-7.8A7 7 0 0 1 14 3zm2 3.5a1.8 1.8 0 1 0 0 3.6 1.8 1.8 0 0 0 0-3.6z",
  alert: "M10.4 3h3.2l-.5 11.5h-2.2zM12 16.6a2.1 2.1 0 1 1 0 4.2 2.1 2.1 0 0 1 0-4.2z",
} as const;

export type GlyphName = keyof typeof PATHS;

export function Glyph({ name, size = 24 }: { name: GlyphName; size?: number }) {
  return (
    <svg className="lb-glyph" width={size} height={size} viewBox="0 0 24 24" fill="#FFFFFF" aria-hidden="true" focusable="false">
      <path d={PATHS[name]} />
    </svg>
  );
}

/** Значок в цветной плашке со скруглением 30 % (kit.py badge). */
export function Badge({ name, bg, size = 46, glyph = 26 }: { name: GlyphName; bg: string; size?: number; glyph?: number }) {
  return (
    <span className="lb-badge" style={{ width: size, height: size, borderRadius: Math.round(size * 0.3), background: bg }} aria-hidden="true">
      <Glyph name={name} size={glyph} />
    </span>
  );
}

/** Монета копилки с часами (kit.py COIN). */
export function Coin({ size = 34 }: { size?: number }) {
  return (
    <svg className="lb-coin" width={size} height={size} viewBox="0 0 40 40" aria-hidden="true" focusable="false">
      <circle cx="20" cy="21.5" r="16" fill="#C97B00" stroke="#0B1D5C" strokeWidth="3" />
      <circle cx="20" cy="19" r="16" fill="#FFC53A" stroke="#0B1D5C" strokeWidth="3" />
      <circle cx="20" cy="19" r="10.5" fill="none" stroke="#E79A00" strokeWidth="2.5" />
      <path d="M20 13v6.2l3.6 2.2" fill="none" stroke="#A85E00" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
