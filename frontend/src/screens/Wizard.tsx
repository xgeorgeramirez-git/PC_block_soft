import { useCallback, useEffect, useId, useRef, useState, type FormEvent } from "react";
import { listAccounts, setup } from "../api";
import LimitsTable from "../components/LimitsTable";
import Notice from "../components/Notice";
import TextField from "../components/TextField";
import { CallError, errorText } from "../lib/errors";
import { templateLimits, validateLimits, wireLimits } from "../lib/limits";
import { validatePasswordPair, type PasswordPairErrors } from "../lib/password";
import type { Account, WeekdayLimit } from "../types";

interface Props {
  /** Кнопка «Открыть настройки» на итоговом экране; получает только что заданный пароль, чтобы войти сразу. */
  onOpenSettings: (password: string) => void;
}

type Step = 1 | 2 | 3 | "done";

const ADMIN_NOTE =
  "Администратор: на такой учётке ограничения не работают. Сделайте её обычной в Параметрах Windows и добавьте позже в «Настройки → Детские учётки»";
const TEMPLATE_LABEL = "Шаблон: будни 2 ч, выходные 4 ч";

/** Мастер первоначальной настройки (FR-1, FR-2, сценарий 8.3.1): пароль, дети, время. */
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
  const [limitErrors, setLimitErrors] = useState<Record<number, string>>({});
  const [formError, setFormError] = useState("");
  const [busy, setBusy] = useState(false);

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

  const finish = async () => {
    const problems: Record<number, string> = {};
    for (const a of chosen) {
      Object.assign(problems, validateLimits(limits[a.sid] ?? []));
    }
    setLimitErrors(problems);
    if (Object.keys(problems).length > 0) {
      return;
    }
    setBusy(true);
    setFormError("");
    try {
      await setup(
        password,
        chosen.map((a) => ({ sid: a.sid, name: a.name, limits: wireLimits(limits[a.sid] ?? templateLimits()) })),
      );
      setStep("done");
    } catch (e) {
      const field = e instanceof CallError ? (e.field ?? "") : "";
      const text = errorText(e);
      const day = /^limits\.(\d)$/.exec(field);
      if (field === "password") {
        setPwErrors({ password: text });
        setStep(1);
      } else if (field === "children") {
        setChildrenError(text);
        setStep(2);
      } else if (day) {
        setLimitErrors({ [Number(day[1])]: text });
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

  const percent = step === 1 ? 33 : step === 2 ? 66 : 100;
  const titles = { 1: "Пароль родителя", 2: "Кто дети", 3: "Время" } as const;

  return (
    <section className="panel wizard" aria-label="Первоначальная настройка">
      <p className="wizard-progress" aria-live="polite">{`Шаг ${step} из 3`}</p>
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
            <button type="button" className="btn btn-primary" onClick={() => void finish()} disabled={busy}>
              Готово
            </button>
          </div>
        </div>
      ) : null}
    </section>
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
