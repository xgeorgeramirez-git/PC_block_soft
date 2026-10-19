import type { ReactNode } from "react";

interface Props {
  tone?: "info" | "warn" | "ok";
  children: ReactNode;
  /** Важное сообщение читается сразу (alert), обычное - по ходу (status). */
  urgent?: boolean;
}

/** Строка сообщения: результат действия или важное предупреждение. Цвет - только как статус. */
export default function Notice({ tone = "info", children, urgent = false }: Props) {
  return (
    <div className={`notice notice-${tone}`} role={urgent ? "alert" : "status"}>
      {children}
    </div>
  );
}
