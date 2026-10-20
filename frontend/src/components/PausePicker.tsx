import { useState } from "react";
import type { PausePreset } from "../types";

interface Props {
  /** С «15 минут» — только у отдельного ребёнка; «Пауза для всех» без него (FR-25). */
  withMin15?: boolean;
  disabled?: boolean;
  onPick: (preset: PausePreset) => void;
}

const LABELS: { preset: PausePreset; label: string; min15?: boolean }[] = [
  { preset: "min15", label: "15 минут", min15: true },
  { preset: "hour1", label: "1 час" },
  { preset: "hour2", label: "2 часа" },
  { preset: "endOfDay", label: "До конца дня" },
];

/** Выбор срока паузы (R11, FR-25): раскрывается по кнопке «Пауза». */
export default function PausePicker({ withMin15 = true, disabled, onPick }: Props) {
  const [open, setOpen] = useState(false);
  const options = LABELS.filter((o) => withMin15 || !o.min15);
  return (
    <div className="pause-picker-wrap" data-help-anchor="child-card-pause">
      <button type="button" className="btn" disabled={disabled} aria-expanded={open} onClick={() => setOpen((v) => !v)}>
        Пауза
      </button>
      {open ? (
        <div className="pause-picker" role="group" aria-label="На сколько поставить паузу">
          {options.map((o) => (
            <button
              key={o.preset}
              type="button"
              className="btn btn-small"
              disabled={disabled}
              onClick={() => {
                setOpen(false);
                onPick(o.preset);
              }}
            >
              {o.label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
