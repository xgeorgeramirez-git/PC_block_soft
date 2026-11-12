import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import { copyRules, getChildRules, getSchedule, listPrograms, saveLimits, saveSchedule } from "../api";
import LimitsTable from "../components/LimitsTable";
import Notice from "../components/Notice";
import RestSettings from "../components/RestSettings";
import SaveBar from "../components/SaveBar";
import ScheduleEditor from "../components/ScheduleEditor";
import { useRegisterGuard } from "../guard";
import { CallError, errorText } from "../lib/errors";
import { copyDay, limitErrorKey, limitsEqual, normalizeLimits, validateLimits, wireLimits, WORKDAYS } from "../lib/limits";
import { breakDraft, breakEqual, DEFAULT_BREAK, validateBreak, wireBreak } from "../lib/rest";
import { schedulesEqual } from "../lib/schedule";
import type { BreakRules, EndMode, Interval, TodayCard, WeekdayLimit } from "../types";

interface Props {
  childId: number;
  /** Остальные дети для «Скопировать правила с…» (FR-33). */
  others?: TodayCard[];
  /** Сохранено: служба вернула новые правила (экран «Сегодня» перечитает карточку сам). */
  onSaved?: () => void;
  /** Ссылка «Программы» у предупреждения об учёбе; переход идёт через охрану несохранённых правок. */
  onOpenPrograms?: () => void;
}

const SAVED_TEXT = "Сохранено. Новые правила уже действуют";
export const NO_STUDY_TEXT = "Учебных программ и сайтов пока нет: в этом режиме будет закрыто всё";

/** «Когда время вышло» (ТЗ 8.3.3 п.4): два варианта с пояснением. */
const END_MODES: { mode: EndMode; title: string; text: string }[] = [
  {
    mode: "lock",
    title: "Блокировать экран",
    text: "Компьютер закрывается экраном блокировки, игры закрываются, остальные окна сохраняются",
  },
  {
    mode: "study_only",
    title: "Оставить только учёбу",
    text: "Работают только учебные программы и сайты, всё остальное закрывается",
  },
];

type Tab = "limits" | "schedule" | "end" | "rest";

const TABS: { id: Tab; label: string }[] = [
  { id: "limits", label: "Лимиты" },
  { id: "schedule", label: "Запретные часы" },
  { id: "end", label: "Когда время вышло" },
  { id: "rest", label: "Перерыв на отдых" },
];
const LIMITS_PANEL = "limits-panel";

/**
 * Правила ребёнка (FR-16…FR-19, FR-33, FR-37): вкладки «Лимиты», «Запретные часы», «Когда время
 * вышло» и «Отдых», одна панель «Сохранить».
 */
export default function ChildLimits({ childId, others = [], onSaved, onOpenPrograms }: Props) {
  const [tab, setTab] = useState<Tab>("limits");
  const [name, setName] = useState("");
  const [savedLimits, setSavedLimits] = useState<WeekdayLimit[] | null>(null);
  const [draftLimits, setDraftLimits] = useState<WeekdayLimit[]>([]);
  const [savedSchedule, setSavedSchedule] = useState<Interval[]>([]);
  const [draftSchedule, setDraftSchedule] = useState<Interval[]>([]);
  const [savedEndMode, setSavedEndMode] = useState<EndMode>("lock");
  const [draftEndMode, setDraftEndMode] = useState<EndMode>("lock");
  const [endModeError, setEndModeError] = useState("");
  const [savedBreak, setSavedBreak] = useState<BreakRules>(breakDraft(DEFAULT_BREAK));
  const [draftBreak, setDraftBreak] = useState<BreakRules>(breakDraft(DEFAULT_BREAK));
  /** Есть ли учебные программы (FR-18); null - не удалось узнать, предупреждения нет. */
  const [hasStudy, setHasStudy] = useState<boolean | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [message, setMessage] = useState("");
  const [loadError, setLoadError] = useState("");
  const [saving, setSaving] = useState(false);
  const alive = useRef(true);
  /** Вкладка «Отдых» с ошибкой: после показа фокус на первое неверное поле (по счётчику). */
  const restPanel = useRef<HTMLDivElement>(null);
  const tabRefs = useRef<(HTMLButtonElement | null)[]>([]);
  // Вкладки правил (B-38): стрелки, Home и End переключают и переводят фокус, в порядке Tab одна выбранная.
  const onTabKey = (e: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const last = TABS.length - 1;
    let next: number;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") next = index === last ? 0 : index + 1;
    else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = index === 0 ? last : index - 1;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = last;
    else return;
    e.preventDefault();
    setTab(TABS[next].id);
    tabRefs.current[next]?.focus();
  };
  const [focusRestError, setFocusRestError] = useState(0);
  useEffect(() => {
    if (focusRestError > 0) restPanel.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
  }, [focusRestError]);

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
      setSavedEndMode(rules.endMode ?? "lock");
      setDraftEndMode(rules.endMode ?? "lock");
      setSavedBreak(breakDraft(rules.break));
      setDraftBreak(breakDraft(rules.break));
      setEndModeError("");
      setErrors({});
      setFormError("");
      setMessage("");
    } catch (e) {
      if (alive.current) setLoadError(errorText(e));
      return;
    }
    // Учебные программы для предупреждения у «Оставить только учёбу». Не прочиталось - молчим:
    // правила от этого не зависят.
    try {
      const progs = await listPrograms();
      if (alive.current) setHasStudy(progs.programs.some((p) => p.category === "study" && p.hidden !== true));
    } catch {
      if (alive.current) setHasStudy(null);
    }
  }, [childId]);

  useEffect(() => {
    void load();
  }, [load]);

  const limitsDirty = savedLimits !== null && !limitsEqual(draftLimits, savedLimits);
  const scheduleDirty = !schedulesEqual(draftSchedule, savedSchedule);
  const endModeDirty = draftEndMode !== savedEndMode;
  const breakDirty = !breakEqual(draftBreak, savedBreak);
  const dirty = limitsDirty || scheduleDirty || endModeDirty || breakDirty;

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

  const changeEndMode = (next: EndMode) => {
    setDraftEndMode(next);
    setEndModeError("");
    setFormError("");
    setMessage("");
  };

  const changeBreak = (next: BreakRules) => {
    setDraftBreak(next);
    setErrors({});
    setFormError("");
    setMessage("");
  };

  const discard = () => {
    if (savedLimits) setDraftLimits(savedLimits);
    setDraftBreak(savedBreak);
    setDraftSchedule(savedSchedule);
    setDraftEndMode(savedEndMode);
    setEndModeError("");
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
    const breakProblems = validateBreak(draftBreak);
    if (Object.keys(breakProblems).length > 0) {
      setErrors(breakProblems);
      setTab("rest");
      setFocusRestError((n) => n + 1);
      return false;
    }
    setSaving(true);
    setFormError("");
    try {
      if (limitsDirty || endModeDirty || breakDirty) {
        // Правила целиком (RM3-9): оба столбца всех дней и режим, даже если менялось одно поле.
        // Меняли только «Отдых» — лимиты и режим не отправляются (служба их не трогает).
        const rules = limitsDirty || endModeDirty;
        const reply = await saveLimits(
          childId,
          rules ? wireLimits(draftLimits) : [],
          rules ? draftEndMode : undefined,
          breakDirty ? wireBreak(draftBreak) : undefined,
        );
        if (!alive.current) return true;
        const limits = reply.rules.limits.length > 0 ? normalizeLimits(reply.rules.limits) : draftLimits;
        const mode = reply.rules.endMode || draftEndMode;
        setSavedLimits(limits);
        setDraftLimits(limits);
        setSavedEndMode(mode);
        setDraftEndMode(mode);
        const brk = breakDraft(reply.rules.break);
        setSavedBreak(brk);
        setDraftBreak(brk);
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
      const field = e instanceof CallError ? e.field : undefined;
      const key = limitErrorKey(field);
      if (key) {
        setErrors({ [key]: errorText(e) });
        setTab("limits");
      } else if (field?.startsWith("break.")) {
        setErrors({ [field]: errorText(e) });
        setTab("rest");
        setFocusRestError((n) => n + 1);
      } else if (field === "endMode") {
        setEndModeError(errorText(e));
        setTab("end");
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
      if (r.endMode) setDraftEndMode(r.endMode);
      setDraftBreak(breakDraft(r.break));
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
    return <p className="panel-text">Загружаю правила…</p>;
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
        {TABS.map((t, i) => (
          <button
            key={t.id}
            ref={(el) => {
              tabRefs.current[i] = el;
            }}
            type="button"
            role="tab"
            id={`limits-tab-${t.id}`}
            className="tab"
            aria-selected={tab === t.id}
            aria-controls={LIMITS_PANEL}
            tabIndex={tab === t.id ? 0 : -1}
            onClick={() => setTab(t.id)}
            onKeyDown={(e) => onTabKey(e, i)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === "limits" ? (
        <div role="tabpanel" id={LIMITS_PANEL} aria-labelledby="limits-tab-limits">
          <p className="panel-text">Сколько времени в день можно пользоваться компьютером и сколько из него можно играть и смотреть видео и соцсети.</p>
          <div className="quick-actions">
            <button type="button" className="btn btn-small" onClick={() => changeLimits(copyDay(draftLimits, 1, WORKDAYS.filter((d) => d !== 1)))}>
              Как в понедельник: на все будни
            </button>
            <button type="button" className="btn btn-small" onClick={() => changeLimits(copyDay(draftLimits, 6, [7]))}>
              Как в субботу: на воскресенье
            </button>
          </div>
          <LimitsTable childName={name} limits={draftLimits} onChange={changeLimits} errors={errors} />
        </div>
      ) : null}
      {tab === "schedule" ? (
        <div role="tabpanel" id={LIMITS_PANEL} aria-labelledby="limits-tab-schedule">
          <p className="panel-text">Время, когда компьютером нельзя пользоваться, даже если лимит ещё не исчерпан.</p>
          <ScheduleEditor childName={name} schedule={draftSchedule} onChange={changeSchedule} />
        </div>
      ) : null}
      {tab === "end" ? (
        <div role="tabpanel" id={LIMITS_PANEL} aria-labelledby="limits-tab-end">
          <EndModeChoice
            value={draftEndMode}
            onChange={changeEndMode}
            error={endModeError}
            noStudy={hasStudy === false}
            onOpenPrograms={onOpenPrograms}
          />
        </div>
      ) : null}
      {tab === "rest" ? (
        <div role="tabpanel" id={LIMITS_PANEL} aria-labelledby="limits-tab-rest" ref={restPanel}>
          <RestSettings value={draftBreak} saved={savedBreak} onChange={changeBreak} errors={errors} />
        </div>
      ) : null}

      <SaveBar dirty={dirty} saving={saving} savedText={message} error={formError} onSave={() => void save()} onDiscard={discard} />
    </section>
  );
}

interface EndModeProps {
  value: EndMode;
  onChange: (next: EndMode) => void;
  error: string;
  /** Учебных программ нет: у «Оставить только учёбу» предупреждение. */
  noStudy: boolean;
  onOpenPrograms?: () => void;
}

/** «Когда время вышло» (FR-18, R8): экран блокировки или «Оставить только учёбу». */
function EndModeChoice({ value, onChange, error, noStudy, onOpenPrograms }: EndModeProps) {
  return (
    <fieldset className="end-modes" aria-describedby={error ? "end-mode-error" : undefined}>
      <legend className="panel-text">Что делать, когда время на сегодня закончилось. Правило действует для этого ребёнка каждый день.</legend>
      {END_MODES.map((o) => {
        const textId = `end-mode-${o.mode}-text`;
        const warnId = `end-mode-${o.mode}-warn`;
        const warn = o.mode === "study_only" && noStudy;
        return (
          <div key={o.mode} className={value === o.mode ? "end-mode end-mode-on" : "end-mode"}>
            <label className="end-mode-label">
              <input
                type="radio"
                name="end-mode"
                value={o.mode}
                checked={value === o.mode}
                aria-describedby={warn ? `${textId} ${warnId}` : textId}
                onChange={() => onChange(o.mode)}
              />
              <span className="end-mode-body">
                <span className="end-mode-title">{o.title}</span>
                <span id={textId} className="end-mode-text">
                  {o.text}
                </span>
              </span>
            </label>
            {warn ? (
              <p id={warnId} className="end-mode-warn">
                <span>{NO_STUDY_TEXT}</span>
                {onOpenPrograms ? (
                  <button type="button" className="btn btn-small" onClick={onOpenPrograms}>
                    Программы
                  </button>
                ) : null}
              </p>
            ) : null}
          </div>
        );
      })}
      {error ? (
        <p id="end-mode-error" className="field-error" role="alert">
          {error}
        </p>
      ) : null}
    </fieldset>
  );
}

function CopyRulesButton({ others, onPick }: { others: TodayCard[]; onPick: (childId: number) => void }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="copy-rules">
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
