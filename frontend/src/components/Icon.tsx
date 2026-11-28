// Линейные значки окна родителя (ТЗ 8.0 п. 4): один векторный набор, встроенный в программу, без эмодзи.
// Пути — из макета (plans/design-0.8.4/kit2.py), сетка 24 × 24, обводка цветом текста (currentColor).

const PATHS = {
  home: <path d="M3 11 12 4l9 7M6 9.5V20h12V9.5" />,
  kids: (
    <>
      <circle cx="9" cy="8" r="3.5" />
      <path d="M2.5 20c.6-3.6 3.2-5.5 6.5-5.5s5.9 1.9 6.5 5.5M16 4.8a3.2 3.2 0 0 1 0 6.4M18 14.6c2 .6 3.2 2.4 3.5 5.4" />
    </>
  ),
  grid: (
    <>
      <rect x="4" y="4" width="7" height="7" rx="1.5" />
      <rect x="13" y="4" width="7" height="7" rx="1.5" />
      <rect x="4" y="13" width="7" height="7" rx="1.5" />
      <rect x="13" y="13" width="7" height="7" rx="1.5" />
    </>
  ),
  globe: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3c2.5 2.6 3.7 5.6 3.7 9s-1.2 6.4-3.7 9c-2.5-2.6-3.7-5.6-3.7-9S9.5 5.6 12 3z" />
    </>
  ),
  bank: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M12 7.5V12l3 2" />
    </>
  ),
  key: (
    <>
      <circle cx="8" cy="15" r="4" />
      <path d="M11 12l8-8M16 7l2 2M14 9l2 2" />
    </>
  ),
  chart: <path d="M5 20V11M12 20V5M19 20v-7" />,
  list: <path d="M9 6h11M9 12h11M9 18h11M4.5 6h.01M4.5 12h.01M4.5 18h.01" />,
  gear: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3M5.3 5.3l2.1 2.1M16.6 16.6l2.1 2.1M5.3 18.7l2.1-2.1M16.6 7.4l2.1-2.1" />
    </>
  ),
  star: <path d="M12 3l2.6 5.6 6.1.7-4.5 4.2 1.2 6L12 16.6 6.6 19.5l1.2-6L3.3 9.3l6.1-.7z" />,
  shield: <path d="M12 3 5 6v5c0 4.5 3 8.3 7 10 4-1.7 7-5.5 7-10V6z" />,
  moon: <path d="M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z" />,
  logout: <path d="M10 5H5v14h5M14 8l4 4-4 4M18 12H9" />,
} as const;

export type IconName = keyof typeof PATHS;

/** Линейный значок: декор, читается подписью рядом (aria-hidden). */
export function Icon({ name, size = 20, strokeWidth = 1.8, className }: { name: IconName; size?: number; strokeWidth?: number; className?: string }) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {PATHS[name]}
    </svg>
  );
}

/** Монета копилки с часами (8.0 п. 1, макет kit2.coin): золото, обводка и стрелки тёмным золотом. */
export function Coin({ size = 18 }: { size?: number }) {
  return (
    <svg className="coin" width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <circle cx="12" cy="12" r="10" style={{ fill: "var(--coin)" }} stroke="#B87800" strokeWidth="1.5" />
      <path d="M12 7.5V12l3 2" fill="none" stroke="#8A5A00" strokeWidth="1.8" strokeLinecap="round" />
    </svg>
  );
}
