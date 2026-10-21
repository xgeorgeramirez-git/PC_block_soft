import { useCallback, useEffect, useRef, useState } from "react";
import { copyRules, getChildRules, getSchedule, saveLimits, saveSchedule } from "../api";
import LimitsTable from "../components/LimitsTable";
import Notice from "../components/Notice";
import SaveBar from "../components/SaveBar";
import ScheduleEditor from "../components/ScheduleEditor";
import { useRegisterGuard } from "../guard";
import HintSlot from "../guidance/HintSlot";
import { CallError, errorText } from "../lib/errors";
import { copyDay, limitsEqual, normalizeLimits, validateLimits, wireLimits, WORKDAYS } from "../lib/limits";
import { schedulesEqual } from "../lib/schedule";
import type { Interval, TodayCard, WeekdayLimit } from "../types";

interface Props {
  childId: number;
  /** Остальные дети для «Скопировать правила с…» (FR-33). */
  others?: TodayCard[];
  /** Сохранено: служба вернула новые правила (экран «Сегодня» перечитает карточку сам). */
  onSaved?: () => void;
}

const SAVED_TEXT = "Сохранено. Новые правила уже действуют";

type Tab = "limits" | "schedule";

/** Правила ребёнка (FR-16, FR-17, FR-19, FR-33): вкладки «Лимиты» и «Запретные часы», одна панель «Сохранить». */
export default function ChildLimits({ childId, others = [], onSaved }: Props) {
  const [tab, setTab] = useState<Tab>("limits");
  const [name, setName] = useState("");
  const [savedLimits, setSavedLimits] = useState<WeekdayLimit[] | null>(null);
  const [draftLimits, setDraftLimits] = useState<WeekdayLimit[]>([]);
  const [savedSchedule, setSavedSchedule] = useState<Interval[]>([]);
  const [draftSchedule, setDraftSchedule] = useState<Interval[]>([]);
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
    setSavedLimits(null);
    try {
      const [rules, sched] = await Promise.all([getChildRules(childId), getSchedule(childId)]);
      if (!alive.current) return;
      const limits = normalizeLimits(rules.limits);
      setName(rules.name);
      setSavedLimits(limits);
      setDraftLimits(limits);
      setSavedSchedule(sched.intervals);
      setDraftSchedule(sched.intervals);
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

  const limitsDirty = savedLimits !== null && !limitsEqual(draftLimits, savedLimits);
  const scheduleDirty = !schedulesEqual(draftSchedule, savedSchedule);
  const dirty = limitsDirty || scheduleDirty;

  const changeLimits = (next: WeekdayLimit[]) => {
    setDraftLimits(next);
    setErrors({});
    setFormError("");
    setMessage("");
  };
  const changeSchedule = (next: Interval[]) => {
    setDraftSchedule(next);
    setFormError("");
    setMessage("");
  };

  const discard = () => {
    if (savedLimits) setDraftLimits(savedLimits);
    setDraftSchedule(savedSchedule);
    setErrors({});
    setFormError("");
  };

  const save = async (): Promise<boolean> => {
    const problems = validateLimits(draftLimits);
    if (Object.keys(problems).length > 0) {
      setErrors(problems);
      setTab("limits");
      return false;
    }
    setSaving(true);
    setFormError("");
    try {
      if (limitsDirty) {
        const reply = await saveLimits(childId, wireLimits(draftLimits));
        if (!alive.current) return true;
        const limits = reply.rules.limits.length > 0 ? normalizeLimits(reply.rules.limits) : draftLimits;
        setSavedLimits(limits);
        setDraftLimits(limits);
      }
      if (scheduleDirty) {
        const reply = await saveSchedule(childId, draftSchedule);
        if (!alive.current) return true;
        setSavedSchedule(reply.intervals);
        setDraftSchedule(reply.intervals);
      }
      if (!alive.current) return true;
      setErrors({});
      setMessage(SAVED_TEXT);
      onSaved?.();
      return true;
    } catch (e) {
      if (!alive.current) return false;
      const day = e instanceof CallError ? /^limits\.(\d)$/.exec(e.field ?? "") : null;
      if (day) {
        setErrors({ [Number(day[1])]: errorText(e) });
        setTab("limits");
      } else {
        setFormError(errorText(e));
      }
      return false;
    } finally {
      if (alive.current) setSaving(false);
    }
  };

  useRegisterGuard({ isDirty: () => dirty, save, discard });

  const applyCopy = async (fromChildId: number) => {
    setFormError("");
    try {
      const r = await copyRules(fromChildId, childId);
      if (!alive.current) return;
      setDraftLimits(normalizeLimits(r.limits));
      setDraftSchedule(r.intervals);
      setMessage(r.message || "Правила скопированы. Проверьте и сохраните");
    } catch (e) {
      if (alive.current) setFormError(errorText(e));
    }
  };

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
  if (savedLimits === null) {
    return <p className="panel-text">Загружаю правила</p>;
  }

  return (
    <section className="limits-page" aria-labelledby="limits-title">
      <div className="page-head">
        <h2 id="limits-title" className="panel-title">
          Правила, {name}
        </h2>
        {others.length > 0 ? <CopyRulesButton others={others} onPick={applyCopy} /> : null}
      </div>

      <div className="tabs" role="tablist" aria-label="Правила ребёнка">
        <button
          type="button"
          role="tab"
          className="tab"
          aria-selected={tab === "limits"}
          onClick={() => setTab("limits")}
        >
          Лимиты
        </button>
        <button
          type="button"
          role="tab"
          className="tab"
          aria-selected={tab === "schedule"}
          onClick={() => setTab("schedule")}
          data-help-anchor="schedule-tab"
        >
          Запретные часы
        </button>
      </div>

      {tab === "limits" ? (
        <div role="tabpanel" aria-label="Лимиты">
          <p className="panel-text">Сколько времени в день можно пользоваться компьютером. Школа и учёба тоже считаются.</p>
          <div className="quick-actions">
            <button type="button" className="btn btn-small" onClick={() => changeLimits(copyDay(draftLimits, 1, WORKDAYS.filter((d) => d !== 1)))}>
              Как в понедельник: на все будни
            </button>
            <button type="button" className="btn btn-small" onClick={() => changeLimits(copyDay(draftLimits, 6, [7]))}>
              Как в субботу: на воскресенье
            </button>
          </div>
          <HintSlot anchor="limits-table" />
          <LimitsTable childName={name} limits={draftLimits} onChange={changeLimits} errors={errors} />
        </div>
      ) : (
        <div role="tabpanel" aria-label="Запретные часы">
          <p className="panel-text">Время, когда компьютером нельзя пользоваться, даже если лимит ещё не исчерпан.</p>
          <ScheduleEditor childName={name} schedule={draftSchedule} onChange={changeSchedule} />
        </div>
      )}

      <SaveBar dirty={dirty} saving={saving} savedText={message} error={formError} onSave={() => void save()} onDiscard={discard} />
    </section>
  );
}

function CopyRulesButton({ others, onPick }: { others: TodayCard[]; onPick: (childId: number) => void }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="copy-rules" data-help-anchor="copy-rules">
      <button type="button" className="btn btn-small" onClick={() => setOpen((v) => !v)} aria-expanded={open}>
        Скопировать правила с…
      </button>
      {open ? (
        <ul className="copy-rules-menu">
          {others.map((c) => (
            <li key={c.childId}>
              <button
                type="button"
                className="btn btn-small"
                onClick={() => {
                  setOpen(false);
                  onPick(c.childId);
                }}
              >
                {c.name}
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
