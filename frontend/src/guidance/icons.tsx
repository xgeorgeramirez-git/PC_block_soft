// Единый набор значков Помощника: векторные, в цвет текста, без эмодзи (ТЗ 8.0 п.4, урок L-45).
import type { ReactNode } from "react";

function Icon({ children }: { children: ReactNode }) {
  return (
    <svg
      className="icon"
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

export function IconClose() {
  return (
    <Icon>
      <path d="M3.5 3.5l9 9M12.5 3.5l-9 9" />
    </Icon>
  );
}

export function IconInfo() {
  return (
    <Icon>
      <circle cx="8" cy="8" r="6.2" />
      <path d="M8 7.4v3.6M8 5.2h.01" />
    </Icon>
  );
}

export function IconWarn() {
  return (
    <Icon>
      <path d="M8 2.2l6.2 11H1.8z" />
      <path d="M8 6.6v3M8 11.5h.01" />
    </Icon>
  );
}

export function IconCheck() {
  return (
    <Icon>
      <circle cx="8" cy="8" r="6.2" />
      <path d="M5 8.3l2.2 2.2L11 6" />
    </Icon>
  );
}

export function IconCircle() {
  return (
    <Icon>
      <circle cx="8" cy="8" r="6.2" />
    </Icon>
  );
}
