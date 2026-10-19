// Охрана несохранённых правок (FR-19, урок L-119): уход со страницы с изменениями спрашивает
// «Сохранить изменения?» с кнопками «Сохранить», «Не сохранять», «Остаться».
// Страница с правками регистрирует себя через useRegisterGuard, а любой уход (меню, выбор
// другого ребёнка) проходит через useAttemptNavigation.
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import Dialog from "./components/Dialog";

export interface Guard {
  isDirty(): boolean;
  /** Сохраняет правки. false - не вышло, остаёмся на странице и показываем ошибку. */
  save(): Promise<boolean>;
  discard(): void;
}

interface GuardApi {
  setGuard(guard: Guard | null): void;
  attempt(proceed: () => void): void;
}

const GuardContext = createContext<GuardApi>({
  setGuard: () => undefined,
  attempt: (proceed) => proceed(),
});

export function UnsavedGuardProvider({ children }: { children: ReactNode }) {
  const guard = useRef<Guard | null>(null);
  const [pending, setPending] = useState<(() => void) | null>(null);
  const [saving, setSaving] = useState(false);

  const setGuard = useCallback((g: Guard | null) => {
    guard.current = g;
  }, []);

  const attempt = useCallback((proceed: () => void) => {
    if (guard.current?.isDirty()) {
      setPending(() => proceed);
    } else {
      proceed();
    }
  }, []);

  const api = useMemo(() => ({ setGuard, attempt }), [setGuard, attempt]);

  const close = () => setPending(null);

  const save = async () => {
    const proceed = pending;
    if (!proceed || !guard.current) {
      return;
    }
    setSaving(true);
    try {
      const ok = await guard.current.save();
      setPending(null);
      if (ok) {
        proceed();
      }
    } finally {
      setSaving(false);
    }
  };

  const discard = () => {
    const proceed = pending;
    guard.current?.discard();
    setPending(null);
    proceed?.();
  };

  return (
    <GuardContext.Provider value={api}>
      {children}
      {pending ? (
        <Dialog
          title="Сохранить изменения?"
          onClose={close}
          actions={
            <>
              <button type="button" className="btn btn-primary" onClick={save} disabled={saving}>
                Сохранить
              </button>
              <button type="button" className="btn" onClick={discard} disabled={saving}>
                Не сохранять
              </button>
              <button type="button" className="btn" onClick={close} disabled={saving}>
                Остаться
              </button>
            </>
          }
        >
          <p>Вы поменяли правила и ещё не сохранили их.</p>
        </Dialog>
      ) : null}
    </GuardContext.Provider>
  );
}

/** Страница с правками регистрирует охрану на время жизни. */
export function useRegisterGuard(guard: Guard) {
  const { setGuard } = useContext(GuardContext);
  const ref = useRef(guard);
  ref.current = guard;
  useEffect(() => {
    setGuard({
      isDirty: () => ref.current.isDirty(),
      save: () => ref.current.save(),
      discard: () => ref.current.discard(),
    });
    return () => setGuard(null);
  }, [setGuard]);
}

/** Возвращает функцию: attempt(() => перейти) спросит про правки, если они есть. */
export function useAttemptNavigation(): (proceed: () => void) => void {
  return useContext(GuardContext).attempt;
}
