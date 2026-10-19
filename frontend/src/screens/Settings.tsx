import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { changePassword, getHelperState, getToday, listAccounts, saveHelperState, setChildren, version } from "../api";
import Dialog from "../components/Dialog";
import Notice from "../components/Notice";
import ServiceNotices from "../components/ServiceNotices";
import TextField from "../components/TextField";
import { CallError, errorText } from "../lib/errors";
import { genitiveName } from "../lib/format";
import { validatePasswordPair } from "../lib/password";
import type { Account, ChildInfo } from "../types";

/** Общие настройки (FR-25, сценарий 8.3.8): пароль, детские учётки, Помощник, О программе. */
export default function Settings({ onOpenChildren }: { onOpenChildren?: () => void }) {
  const [notices, setNotices] = useState<string[]>([]);
  const [children, setChildrenList] = useState<ChildInfo[] | null>(null);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loadError, setLoadError] = useState("");
  const alive = useRef(true);

  const load = useCallback(async () => {
    setLoadError("");
    try {
      const [today, accs] = await Promise.all([getToday(), listAccounts()]);
      if (!alive.current) return;
      setNotices(today.notices);
      setChildrenList(today.children.map((c) => ({ childId: c.childId, sid: c.sid, name: c.name })));
      setAccounts(accs);
    } catch (e) {
      if (alive.current) setLoadError(errorText(e));
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    void load();
    return () => {
      alive.current = false;
    };
  }, [load]);

  return (
    <div className="settings">
      <h2 className="panel-title">Настройки</h2>
      <ServiceNotices notices={notices} onOpenChildren={onOpenChildren} />
      <PasswordSection />
      <ChildrenSection
        list={children}
        accounts={accounts}
        loadError={loadError}
        onReload={() => void load()}
        onChanged={(list) => {
          setChildrenList(list);
          void listAccounts().then((a) => alive.current && setAccounts(a), () => undefined);
        }}
      />
      <HelperSection />
      <AboutSection />
    </div>
  );
}

// --- Сменить пароль ------------------------------------------------------------

function PasswordSection() {
  const [oldPw, setOldPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [again, setAgain] = useState("");
  const [errors, setErrors] = useState<{ old?: string; password?: string; again?: string }>({});
  const [formError, setFormError] = useState("");
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  const edit = (set: (v: string) => void) => (v: string) => {
    set(v);
    setErrors({});
    setFormError("");
    setDone(false);
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (busy) return;
    const next = validatePasswordPair(newPw, again);
    const found = { ...(oldPw === "" ? { old: "Введите старый пароль" } : {}), ...next };
    setErrors(found);
    if (Object.keys(found).length > 0) return;
    setBusy(true);
    try {
      await changePassword(oldPw, newPw);
      setOldPw("");
      setNewPw("");
      setAgain("");
      setDone(true);
    } catch (err) {
      const field = err instanceof CallError ? err.field : undefined;
      if (field === "oldPassword") setErrors({ old: errorText(err) });
      else if (field === "newPassword") setErrors({ password: errorText(err) });
      else setFormError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="panel" aria-labelledby="settings-password">
      <h3 id="settings-password" className="section-title">
        Сменить пароль
      </h3>
      <form onSubmit={submit} noValidate>
        <TextField label="Старый пароль" type="password" value={oldPw} onChange={edit(setOldPw)} error={errors.old} autoComplete="current-password" />
        <TextField
          label="Новый пароль"
          type="password"
          value={newPw}
          onChange={edit(setNewPw)}
          error={errors.password}
          hint="Не меньше 6 символов"
          autoComplete="new-password"
        />
        <TextField label="Повторите новый пароль" type="password" value={again} onChange={edit(setAgain)} error={errors.again} autoComplete="new-password" />
        {formError ? (
          <Notice tone="warn" urgent>
            {formError}
          </Notice>
        ) : null}
        {done ? <Notice tone="ok">Пароль изменён</Notice> : null}
        <div className="form-actions">
          <button type="submit" className="btn btn-primary" disabled={busy}>
            Сменить пароль
          </button>
        </div>
      </form>
    </section>
  );
}

// --- Детские учётки -------------------------------------------------------------

interface ChildrenProps {
  list: ChildInfo[] | null;
  accounts: Account[];
  loadError: string;
  onReload: () => void;
  onChanged: (list: ChildInfo[]) => void;
}

/** Строка под именем ребёнка: какая учётная запись Windows за ним стоит (имя в программе может быть другим). */
function accountLine(child: ChildInfo, accounts: Account[]): string {
  const account = accounts.find((a) => a.sid === child.sid);
  return account ? `Учётная запись Windows: ${account.name}` : "Учётная запись Windows не найдена на этом компьютере";
}

function ChildrenSection({ list, accounts, loadError, onReload, onChanged }: ChildrenProps) {
  const [renaming, setRenaming] = useState<number | null>(null);
  const [newName, setNewName] = useState("");
  const [nameError, setNameError] = useState("");
  const [removing, setRemoving] = useState<ChildInfo | null>(null);
  const [pickSid, setPickSid] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const candidates = accounts.filter((a) => a.canBeChild && !a.admin && !a.child);
  const picked = candidates.find((a) => a.sid === pickSid) ?? candidates[0];

  const apply = async (change: Parameters<typeof setChildren>[0], ok: string): Promise<boolean> => {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      onChanged(await setChildren(change));
      setMessage(ok);
      return true;
    } catch (e) {
      setError(errorText(e));
      return false;
    } finally {
      setBusy(false);
    }
  };

  const saveName = async (child: ChildInfo) => {
    const name = newName.trim();
    if (name === "") {
      setNameError("Введите имя");
      return;
    }
    if (await apply({ rename: [{ childId: child.childId, name }] }, `Имя изменено: ${name}`)) {
      setRenaming(null);
    }
  };

  const remove = async () => {
    const child = removing;
    setRemoving(null);
    if (child) {
      await apply({ remove: [child.childId] }, `Убрано: ${child.name}. Статистика сохранена`);
    }
  };

  const add = async () => {
    if (picked) {
      await apply({ add: [{ sid: picked.sid, name: picked.name }] }, `Добавлено: ${picked.name}. Задайте лимиты в разделе «Дети»`);
      setPickSid("");
    }
  };

  return (
    <section className="panel" aria-labelledby="settings-children">
      <h3 id="settings-children" className="section-title">
        Детские учётки
      </h3>
      {loadError ? (
        <div>
          <Notice tone="warn" urgent>
            {loadError}
          </Notice>
          <button type="button" className="btn" onClick={onReload}>
            Повторить
          </button>
        </div>
      ) : list === null ? (
        <p className="panel-text">Загружаю данные</p>
      ) : (
        <>
          {message ? <Notice tone="ok">{message}</Notice> : null}
          {error ? (
            <Notice tone="warn" urgent>
              {error}
            </Notice>
          ) : null}

          {list.length === 0 ? (
            <p className="panel-text">Детей пока нет</p>
          ) : (
            <ul className="child-rows">
              {list.map((c) => (
                <li key={c.childId} className="child-row">
                  {renaming === c.childId ? (
                    <form
                      className="rename-form"
                      onSubmit={(e) => {
                        e.preventDefault();
                        void saveName(c);
                      }}
                    >
                      <TextField
                        label="Новое имя"
                        value={newName}
                        onChange={(v) => {
                          setNewName(v);
                          setNameError("");
                        }}
                        error={nameError}
                        maxLength={40}
                        autoFocus
                      />
                      <div className="form-actions">
                        <button type="submit" className="btn btn-primary" disabled={busy}>
                          Сохранить имя
                        </button>
                        <button type="button" className="btn" onClick={() => setRenaming(null)}>
                          Отмена
                        </button>
                      </div>
                    </form>
                  ) : (
                    <>
                      <div className="child-row-info">
                        <span className="child-row-name">{c.name}</span>
                        <span className="child-row-account">{accountLine(c, accounts)}</span>
                      </div>
                      <span className="child-row-actions">
                        <button
                          type="button"
                          className="btn"
                          aria-label={`Переименовать ${c.name}`}
                          onClick={() => {
                            setRenaming(c.childId);
                            setNewName(c.name);
                            setNameError("");
                          }}
                        >
                          Переименовать
                        </button>
                        <button type="button" className="btn" aria-label={`Убрать ${c.name}`} onClick={() => setRemoving(c)}>
                          Убрать
                        </button>
                      </span>
                    </>
                  )}
                </li>
              ))}
            </ul>
          )}

          <div className="add-child">
            <h4 className="subsection-title">Добавить детскую учётку</h4>
            {candidates.length > 0 ? (
              <>
                <div className="field">
                  <label className="field-label" htmlFor="add-child-select">
                    Учётная запись Windows
                  </label>
                  <select
                    id="add-child-select"
                    className="field-input"
                    value={picked?.sid ?? ""}
                    onChange={(e) => setPickSid(e.target.value)}
                  >
                    {candidates.map((a) => (
                      <option key={a.sid} value={a.sid}>
                        {a.name}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="form-actions">
                  <button type="button" className="btn" onClick={() => void add()} disabled={busy}>
                    Добавить ребёнка
                  </button>
                </div>
              </>
            ) : (
              <>
                <p className="panel-text">
                  Подходящих обычных учётных записей нет. Создайте обычную учётную запись в Параметрах Windows и нажмите
                  «Обновить список». Администратора добавить нельзя: на такой учётке ограничения не работают.
                </p>
                <button type="button" className="btn" onClick={onReload}>
                  Обновить список
                </button>
              </>
            )}
          </div>
        </>
      )}

      {removing ? (
        <Dialog
          title="Убрать детскую учётку?"
          onClose={() => setRemoving(null)}
          actions={
            <>
              <button type="button" className="btn btn-primary" onClick={() => void remove()}>
                Убрать
              </button>
              <button type="button" className="btn" onClick={() => setRemoving(null)}>
                Отмена
              </button>
            </>
          }
        >
          <p>{`Ограничения для ${genitiveName(removing.name)} перестанут действовать. Статистика сохранится. Убрать?`}</p>
        </Dialog>
      ) : null}
    </section>
  );
}

// --- Помощник --------------------------------------------------------------------

function HelperSection() {
  const [state, setState] = useState<Record<string, unknown> | null>(null);
  const [error, setError] = useState("");
  const alive = useRef(true);

  useEffect(() => {
    alive.current = true;
    getHelperState().then(
      (s) => alive.current && setState(s),
      (e) => {
        if (alive.current) {
          setState({});
          setError(errorText(e));
        }
      },
    );
    return () => {
      alive.current = false;
    };
  }, []);

  const enabled = state === null ? true : state.enabled !== false;

  const toggle = async (on: boolean) => {
    const before = state ?? {};
    const next = { ...before, enabled: on };
    setState(next);
    setError("");
    try {
      await saveHelperState(next);
    } catch (e) {
      if (alive.current) {
        setState(before);
        setError(errorText(e));
      }
    }
  };

  return (
    <section className="panel" aria-labelledby="settings-helper">
      <h3 id="settings-helper" className="section-title">
        Помощник
      </h3>
      <label className="switch">
        <input type="checkbox" checked={enabled} disabled={state === null} onChange={(e) => void toggle(e.target.checked)} />
        <span>Показывать подсказки</span>
      </label>
      <p className="field-hint">
        Подсказки при первом открытии экранов и напоминания о важном, например когда у ребёнка не задано ни одного лимита.
      </p>
      {error ? <p className="field-error">{error}</p> : null}
    </section>
  );
}

// --- О программе ---------------------------------------------------------------------

function AboutSection() {
  const [text, setText] = useState("Узнаю версию");
  useEffect(() => {
    let alive = true;
    version().then(
      (v) => alive && setText(`Версия ${v}`),
      () => alive && setText("Версию узнать не удалось"),
    );
    return () => {
      alive = false;
    };
  }, []);
  return (
    <section className="panel" aria-labelledby="settings-about">
      <h3 id="settings-about" className="section-title">
        О программе
      </h3>
      <p>{text}</p>
      <p className="field-hint">Данные хранятся только на этом компьютере</p>
    </section>
  );
}
