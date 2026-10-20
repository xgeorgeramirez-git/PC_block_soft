import { useCallback, useEffect, useRef, useState } from "react";
import { getChildRules, saveLimits } from "../api";
import LimitsTable from "../components/LimitsTable";
import Notice from "../components/Notice";
import SaveBar from "../components/SaveBar";
import { useRegisterGuard } from "../guard";
import HintSlot from "../guidance/HintSlot";
import { CallError, errorText } from "../lib/errors";
import { copyDay, limitsEqual, normalizeLimits, validateLimits, wireLimits, WORKDAYS } from "../lib/limits";
import type { WeekdayLimit } from "../types";

interface Props {
  childId: number;
  /** Сохранено: служба вернула новые правила (экран «Сегодня» перечитает карточку сам). */
  onSaved?: () => void;
}

const SAVED_TEXT = "Сохранено. Новые правила уже действуют";

/** Лимиты ребёнка (FR-16, FR-19): таблица, одна панель «Сохранить», ошибки у поля. */
export default function ChildLimits({ childId, onSaved }: Props) {
  const [name, setName] = useState("");
  const [saved, setSaved] = useState<WeekdayLimit[] | null>(null);
  const [draft, setDraft] = useState<WeekdayLimit[]>([]);
  const [errors, setErrors] = useState<Record<number, string>>({});
  const [formError, setFormError] = useState("");
  const [message, setMessage] = useState("");
  const [loadError, setLoadError] = useState("");
  const [saving, setSaving] = useState(false);
  const alive = useRef(true);

  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);

  const load = useCallback(async () => {
    setLoadError("");
    setSaved(null);
    try {
      const rules = await getChildRules(childId);
      if (!alive.current) return;
      const limits = normalizeLimits(rules.limits);
      setName(rules.name);
      setSaved(limits);
      setDraft(limits);
      setErrors({});
      setFormError("");
      setMessage("");
    } catch (e) {
      if (alive.current) setLoadError(errorText(e));
    }
  }, [childId]);

  useEffect(() => {
    void load();
  }, [load]);

  const dirty = saved !== null && !limitsEqual(draft, saved);

  const change = (next: WeekdayLimit[]) => {
    setDraft(next);
    setErrors({});
    setFormError("");
    setMessage("");
  };

  const discard = () => {
    if (saved) {
      setDraft(saved);
    }
    setErrors({});
    setFormError("");
  };

  const save = async (): Promise<boolean> => {
    const problems = validateLimits(draft);
    if (Object.keys(problems).length > 0) {
      setErrors(problems);
      return false;
    }
    setSaving(true);
    setFormError("");
    try {
      const reply = await saveLimits(childId, wireLimits(draft));
      if (!alive.current) return true;
      const limits = reply.rules.limits.length > 0 ? normalizeLimits(reply.rules.limits) : draft;
      setSaved(limits);
      setDraft(limits);
      setErrors({});
      setMessage(reply.message || SAVED_TEXT);
      onSaved?.();
      return true;
    } catch (e) {
      if (!alive.current) return false;
      const day = e instanceof CallError ? /^limits\.(\d)$/.exec(e.field ?? "") : null;
      if (day) {
        setErrors({ [Number(day[1])]: errorText(e) });
      } else {
        setFormError(errorText(e));
      }
      return false;
    } finally {
      if (alive.current) setSaving(false);
    }
  };

  useRegisterGuard({ isDirty: () => dirty, save, discard });

  if (loadError) {
    return (
      <section className="panel">
        <Notice tone="warn" urgent>
          {loadError}
        </Notice>
        <button type="button" className="btn" onClick={() => void load()}>
          Повторить
        </button>
      </section>
    );
  }
  if (saved === null) {
    return <p className="panel-text">Загружаю правила</p>;
  }

  return (
    <section className="limits-page" aria-labelledby="limits-title">
      <h2 id="limits-title" className="panel-title">
        Лимиты времени, {name}
      </h2>
      <p className="panel-text">Сколько времени в день можно пользоваться компьютером. Школа и учёба тоже считаются.</p>
      <div className="quick-actions">
        <button type="button" className="btn btn-small" onClick={() => change(copyDay(draft, 1, WORKDAYS.filter((d) => d !== 1)))}>
          Как в понедельник: на все будни
        </button>
        <button type="button" className="btn btn-small" onClick={() => change(copyDay(draft, 6, [7]))}>
          Как в субботу: на воскресенье
        </button>
      </div>
      <HintSlot anchor="limits-table" />
      <LimitsTable childName={name} limits={draft} onChange={change} errors={errors} />
      <SaveBar
        dirty={dirty}
        saving={saving}
        savedText={message}
        error={formError}
        onSave={() => void save()}
        onDiscard={discard}
      />
    </section>
  );
}
