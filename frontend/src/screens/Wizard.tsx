import { useCallback, useEffect, useId, useRef, useState, type FormEvent } from "react";
import { listAccounts, scanPrograms, setup } from "../api";
import LimitsTable from "../components/LimitsTable";
import Notice from "../components/Notice";
import TextField from "../components/TextField";
import { CallError, errorText } from "../lib/errors";
import { limitErrorKey, templateLimits, validateLimits, wireLimits } from "../lib/limits";
import { validatePasswordPair, type PasswordPairErrors } from "../lib/password";
import type { Account, ProgramCategory, ScanProgramDTO, SetupProgram, WeekdayLimit } from "../types";

interface Props {
  /** Кнопка «Открыть настройки» на итоговом экране; получает только что заданный пароль, чтобы войти сразу. */
  onOpenSettings: (password: string) => void;
}

type Step = 1 | 2 | 3 | 4 | "done";
const STEPS = 4;

/** Выбор на шаге «Программы» (FR-2): «Запретить» и системное здесь не предлагаются. */
type WizardCategory = Extract<ProgramCategory, "games" | "study" | "other">;
const WIZARD_CATEGORIES: { category: WizardCategory; label: string }[] = [
  { category: "games", label: "Игра" },
  { category: "study", label: "Учёба" },
  { category: "other", label: "Прочее" },
];

/** Предложенная службой категория или «Прочее» (по умолчанию). */
export function suggestedChoice(p: ScanProgramDTO): WizardCategory {
  return p.suggestedCategory === "games" || p.suggestedCategory === "study" ? p.suggestedCategory : "other";
}

/** Что отправить службе: только отметки, изменённые родителем; остальное служба решит сама (снимок). */
export function changedPrograms(list: ScanProgramDTO[], choices: Record<string, WizardCategory>): SetupProgram[] {
  return list
    .filter((p) => choices[p.path] !== undefined && choices[p.path] !== suggestedChoice(p))
    .map((p) => ({ path: p.path, category: choices[p.path] }));
}

export const PROGRAMS_NOTE = "Незнакомые программы будут ждать вашего решения. Это можно изменить позже в разделе «Программы»";
const SCANNING = "Ищу установленные программы";
const SCAN_EMPTY = "Программы не найдены. Их можно настроить позже в разделе «Программы»";

const ADMIN_NOTE =
  "Администратор: на такой учётке ограничения не работают. Сделайте её обычной в Параметрах Windows и добавьте позже в «Настройки → Детские учётки»";
const TEMPLATE_LABEL = "Шаблон: будни 2 ч, выходные 4 ч";

/** Мастер первоначальной настройки (FR-1, FR-2, сценарий 8.3.1): пароль, дети, время, программы. */
export default function Wizard({ onOpenSettings }: Props) {
  const [step, setStep] = useState<Step>(1);
  const [password, setPassword] = useState("");
  const [again, setAgain] = useState("");
  const [pwErrors, setPwErrors] = useState<PasswordPairErrors>({});

  const [accounts, setAccounts] = useState<Account[] | null>(null);
  const [accountsError, setAccountsError] = useState("");
  const [childrenError, setChildrenError] = useState("");
  const [selected, setSelected] = useState<string[]>([]);

  const [limits, setLimits] = useState<Record<string, WeekdayLimit[]>>({});
  const [limitErrors, setLimitErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [busy, setBusy] = useState(false);

  const [scan, setScan] = useState<ScanProgramDTO[] | null>(null);
  const [scanError, setScanError] = useState("");
  const [choices, setChoices] = useState<Record<string, WizardCategory>>({});

  const heading = useRef<HTMLHeadingElement>(null);
  const firstStep = useRef(true);
  const nextHint = useId();

  useEffect(() => {
    if (firstStep.current) {
      firstStep.current = false;
      return;
    }
    heading.current?.focus();
  }, [step]);

  const loadAccounts = useCallback(async () => {
    setAccountsError("");
    try {
      setAccounts(await listAccounts());
    } catch (e) {
      setAccounts(null);
      setAccountsError(errorText(e));
    }
  }, []);

  useEffect(() => {
    if (step === 2 && accounts === null && !accountsError) {
      void loadAccounts();
    }
  }, [step, accounts, accountsError, loadAccounts]);

  const loadPrograms = useCallback(async () => {
    setScanError("");
    try {
      const r = await scanPrograms();
      setScan(r.programs);
      setChoices((prev) => {
        const next: Record<string, WizardCategory> = {};
        for (const p of r.programs) {
          next[p.path] = prev[p.path] ?? suggestedChoice(p);
        }
        return next;
      });
    } catch (e) {
      setScan(null);
      setScanError(errorText(e));
    }
  }, []);

  // Программы ищутся один раз: «Назад» и снова «Далее» не теряют отметок; после ошибки - «Повторить».
  const scanAsked = useRef(false);
  useEffect(() => {
    if (step === 4 && !scanAsked.current) {
      scanAsked.current = true;
      void loadPrograms();
    }
  }, [step, loadPrograms]);

  const goStep1Next = (e: FormEvent) => {
    e.preventDefault();
    const errors = validatePasswordPair(password, again);
    setPwErrors(errors);
    if (!errors.password && !errors.again) {
      setStep(2);
    }
  };

  const toggle = (sid: string, on: boolean) => {
    setChildrenError("");
    setSelected((prev) => (on ? [...prev, sid] : prev.filter((s) => s !== sid)));
  };

  const goStep3 = () => {
    setLimits((prev) => {
      const next: Record<string, WeekdayLimit[]> = {};
      for (const sid of selected) {
        next[sid] = prev[sid] ?? templateLimits();
      }
      return next;
    });
    setLimitErrors({});
    setFormError("");
    setStep(3);
  };

  const chosen = (accounts ?? []).filter((a) => selected.includes(a.sid));

  const limitsValid = (): boolean => {
    const problems: Record<string, string> = {};
    for (const a of chosen) {
      Object.assign(problems, validateLimits(limits[a.sid] ?? []));
    }
    setLimitErrors(problems);
    return Object.keys(problems).length === 0;
  };

  const goStep4 = () => {
    if (!limitsValid()) {
      return;
    }
    setFormError("");
    setStep(4);
  };

  /** «Готово» отправляет изменённые отметки программ, «Настрою позже» - без программ (FR-2). */
  const finish = async (withPrograms: boolean) => {
    if (!limitsValid()) {
      setStep(3);
      return;
    }
    setBusy(true);
    setFormError("");
    try {
      await setup(
        password,
        chosen.map((a) => ({ sid: a.sid, name: a.name, limits: wireLimits(limits[a.sid] ?? templateLimits()) })),
        withPrograms ? changedPrograms(scan ?? [], choices) : undefined,
      );
      setStep("done");
    } catch (e) {
      const field = e instanceof CallError ? (e.field ?? "") : "";
      const text = errorText(e);
      const day = limitErrorKey(field);
      if (field === "password") {
        setPwErrors({ password: text });
        setStep(1);
      } else if (field === "children") {
        setChildrenError(text);
        setStep(2);
      } else if (day) {
        setLimitErrors({ [day]: text });
        setStep(3);
      } else {
        setFormError(text);
      }
    } finally {
      setBusy(false);
    }
  };

  if (step === "done") {
    return (
      <section className="panel wizard" aria-label="Первоначальная настройка">
        <h2 ref={heading} tabIndex={-1} className="panel-title">
          Готово. Правила начнут действовать при следующем входе детей
        </h2>
        <p className="panel-text">Дальше можно открыть настройки: там видно, сколько времени осталось у каждого ребёнка.</p>
        <div className="form-actions">
          <button type="button" className="btn btn-primary" onClick={() => onOpenSettings(password)}>
            Открыть настройки
          </button>
        </div>
      </section>
    );
  }

  const percent = Math.round((step / STEPS) * 100);
  const titles = { 1: "Пароль родителя", 2: "Кто дети", 3: "Время", 4: "Программы" } as const;

  return (
    <section className="panel wizard" aria-label="Первоначальная настройка">
      <p className="wizard-progress" aria-live="polite">{`Шаг ${step} из ${STEPS}`}</p>
      <div className="wizard-bar" aria-hidden="true">
        <span style={{ width: `${percent}%` }} />
      </div>
      <h2 ref={heading} tabIndex={-1} className="panel-title">
        {titles[step]}
      </h2>

      {step === 1 ? (
        <form onSubmit={goStep1Next} noValidate>
          <p className="panel-text">
            Не сообщайте пароль детям. Если забудете, сбросить его можно только из этой учётной записи администратора.
          </p>
          <TextField
            label="Пароль родителя"
            type="password"
            value={password}
            onChange={(v) => {
              setPassword(v);
              setPwErrors({});
            }}
            error={pwErrors.password}
            hint="Не меньше 6 символов"
            autoComplete="new-password"
            autoFocus
          />
          <TextField
            label="Повторите пароль"
            type="password"
            value={again}
            onChange={(v) => {
              setAgain(v);
              setPwErrors({});
            }}
            error={pwErrors.again}
            autoComplete="new-password"
          />
          <div className="form-actions">
            <button type="submit" className="btn btn-primary">
              Далее
            </button>
          </div>
        </form>
      ) : null}

      {step === 2 ? (
        <div>
          <p className="panel-text">Отметьте учётные записи Windows, на которых нужны ограничения.</p>
          {childrenError ? (
            <Notice tone="warn" urgent>
              {childrenError}
            </Notice>
          ) : null}
          {accountsError ? (
            <div>
              <Notice tone="warn" urgent>
                {accountsError}
              </Notice>
              <button type="button" className="btn" onClick={() => void loadAccounts()}>
                Повторить
              </button>
            </div>
          ) : accounts === null ? (
            <p className="panel-text">Читаю список учётных записей</p>
          ) : (
            <AccountList accounts={accounts} selected={selected} onToggle={toggle} onRefresh={() => void loadAccounts()} />
          )}
          <div className="form-actions">
            <button type="button" className="btn" onClick={() => setStep(1)}>
              Назад
            </button>
            <button
              type="button"
              className="btn btn-primary"
              disabled={selected.length === 0}
              aria-describedby={selected.length === 0 ? nextHint : undefined}
              onClick={goStep3}
            >
              Далее
            </button>
            {selected.length === 0 ? (
              <span id={nextHint} className="field-hint">
                Отметьте хотя бы одного ребёнка, чтобы продолжить
              </span>
            ) : null}
          </div>
        </div>
      ) : null}

      {step === 3 ? (
        <div>
          <p className="panel-text">Сколько времени в день можно пользоваться компьютером. Потом это можно изменить.</p>
          {formError ? (
            <Notice tone="warn" urgent>
              {formError}
            </Notice>
          ) : null}
          {chosen.map((a) => (
            <section key={a.sid} className="wizard-child" aria-labelledby={`wizard-child-${a.sid}`}>
              <div className="wizard-child-head">
                <h3 id={`wizard-child-${a.sid}`} className="child-name">
                  {a.name}
                </h3>
                <button
                  type="button"
                  className="btn"
                  onClick={() => {
                    setLimits((prev) => ({ ...prev, [a.sid]: templateLimits() }));
                    setLimitErrors({});
                  }}
                >
                  {TEMPLATE_LABEL}
                </button>
              </div>
              <LimitsTable
                childName={a.name}
                limits={limits[a.sid] ?? templateLimits()}
                onChange={(next) => {
                  setLimits((prev) => ({ ...prev, [a.sid]: next }));
                  setLimitErrors({});
                }}
                errors={limitErrors}
              />
            </section>
          ))}
          <div className="form-actions">
            <button type="button" className="btn" onClick={() => setStep(2)} disabled={busy}>
              Назад
            </button>
            <button type="button" className="btn btn-primary" onClick={goStep4} disabled={busy}>
              Далее
            </button>
          </div>
        </div>
      ) : null}

      {step === 4 ? (
        <div>
          <p className="panel-text">Отметьте, какие программы игры, а какие для учёбы. {PROGRAMS_NOTE}.</p>
          {formError ? (
            <Notice tone="warn" urgent>
              {formError}
            </Notice>
          ) : null}
          {scanError ? (
            <div className="wizard-scan-error">
              <Notice tone="warn" urgent>
                {scanError}
              </Notice>
              <button type="button" className="btn" onClick={() => void loadPrograms()} disabled={busy}>
                Повторить
              </button>
            </div>
          ) : scan === null ? (
            <p className="panel-text wizard-scanning" role="status">
              <span className="wizard-spinner" aria-hidden="true" />
              {SCANNING}
            </p>
          ) : scan.length === 0 ? (
            <p className="panel-text">{SCAN_EMPTY}</p>
          ) : (
            <ScanList
              programs={scan}
              choices={choices}
              disabled={busy}
              onChoose={(path, category) => setChoices((prev) => ({ ...prev, [path]: category }))}
            />
          )}
          <div className="form-actions">
            <button type="button" className="btn" onClick={() => setStep(3)} disabled={busy}>
              Назад
            </button>
            <button type="button" className="btn" onClick={() => void finish(false)} disabled={busy}>
              Настрою позже
            </button>
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => void finish(true)}
              disabled={busy}
            >
              Готово
            </button>
          </div>
        </div>
      ) : null}
    </section>
  );
}

interface ScanListProps {
  programs: ScanProgramDTO[];
  choices: Record<string, WizardCategory>;
  disabled: boolean;
  onChoose: (path: string, category: WizardCategory) => void;
}

/** Найденные программы: название, издатель и выбор «Игра», «Учёба», «Прочее» (FR-2). */
function ScanList({ programs, choices, disabled, onChoose }: ScanListProps) {
  return (
    <div className="wizard-programs">
      <table className="programs-table wizard-programs-table">
        <caption className="visually-hidden">Программы на этом компьютере</caption>
        <thead>
          <tr>
            <th scope="col">Программа</th>
            <th scope="col" className="col-category">
              Что это
            </th>
          </tr>
        </thead>
        <tbody>
          {programs.map((p) => (
            <tr key={p.path}>
              <td>
                <div className="program-name">{p.name}</div>
                <div className="program-meta" title={p.path}>
                  {p.publisher.trim() || "Без подписи издателя"}
                </div>
              </td>
              <td className="col-category">
                <select
                  className="field-input"
                  aria-label={`Что это: ${p.name}`}
                  value={choices[p.path] ?? suggestedChoice(p)}
                  disabled={disabled}
                  onChange={(e) => onChoose(p.path, e.target.value as WizardCategory)}
                >
                  {WIZARD_CATEGORIES.map((o) => (
                    <option key={o.category} value={o.category}>
                      {o.label}
                    </option>
                  ))}
                </select>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

interface ListProps {
  accounts: Account[];
  selected: string[];
  onToggle: (sid: string, on: boolean) => void;
  onRefresh: () => void;
}

function AccountList({ accounts, selected, onToggle, onRefresh }: ListProps) {
  const hasUsable = accounts.some((a) => a.canBeChild && !a.admin);
  return (
    <div>
      {!hasUsable ? (
        <div className="panel-note">
          <p className="panel-text">
            Обычных учётных записей пока нет. Создайте обычную учётную запись для ребёнка в Параметрах Windows и нажмите
            «Обновить список».
          </p>
          <button type="button" className="btn" onClick={onRefresh}>
            Обновить список
          </button>
        </div>
      ) : null}
      <ul className="account-list">
        {accounts.map((a) => {
          const blocked = a.admin || !a.canBeChild;
          const noteId = `acc-note-${a.sid}`;
          return (
            <li key={a.sid} className="account-item">
              <label className="account-label">
                <input
                  type="checkbox"
                  checked={selected.includes(a.sid)}
                  disabled={blocked}
                  aria-describedby={blocked ? noteId : undefined}
                  onChange={(e) => onToggle(a.sid, e.target.checked)}
                />
                <span className="account-name">{a.name}</span>
                {a.current ? <span className="account-tag">Это ваша учётная запись</span> : null}
              </label>
              {blocked ? (
                <p id={noteId} className="field-hint">
                  {a.admin ? ADMIN_NOTE : "Эту учётную запись нельзя сделать детской"}
                </p>
              ) : null}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
