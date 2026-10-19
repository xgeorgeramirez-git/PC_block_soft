import { useCallback, useEffect, useRef, useState } from "react";
import { finish, firstRunSeen, myToday } from "../api";
import Notice from "../components/Notice";
import { childErrorText } from "../lib/errors";
import { firstRunLines } from "../lib/mytoday";

const RULES_ERROR = "Не удалось получить правила. Попробуй через минуту";
const DONE_TEXT = "Готово. Это окно можно закрыть.";

/** Окно первого входа ребёнка (FR-32): пересказ правил на сегодня и «Понятно». */
export default function FirstRun() {
  const [lines, setLines] = useState<string[] | null>(null);
  const [loadError, setLoadError] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const alive = useRef(true);

  const load = useCallback(async () => {
    setLoadError("");
    try {
      const reply = await myToday();
      if (alive.current) setLines(firstRunLines(reply));
    } catch {
      // Правил не получили, но окно не тупик (L-119): «Понятно» остаётся.
      if (alive.current) setLoadError(RULES_ERROR);
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    void load();
    return () => {
      alive.current = false;
    };
  }, [load]);

  const understood = async () => {
    if (busy || done) return;
    setBusy(true);
    setError("");
    try {
      await firstRunSeen();
      if (!alive.current) return;
      setDone(true);
      await finish(0);
    } catch (e) {
      if (alive.current) setError(childErrorText(e));
    } finally {
      if (alive.current) setBusy(false);
    }
  };

  return (
    <section className="panel first-run" aria-labelledby="first-run-title">
      <h2 id="first-run-title" className="panel-title">
        Теперь время за компьютером считается
      </h2>
      {loadError ? (
        <>
          <Notice tone="warn" urgent>
            {loadError}
          </Notice>
          <p>
            <button type="button" className="btn btn-small" onClick={() => void load()}>
              Обновить
            </button>
          </p>
        </>
      ) : lines === null ? (
        <p className="panel-text">Загружаю правила</p>
      ) : (
        <div className="first-run-lines">
          {lines.map((line) => (
            <p key={line}>{line}</p>
          ))}
        </div>
      )}
      {error ? (
        <p className="field-error" role="alert">
          {error}
        </p>
      ) : null}
      {done ? (
        <p className="panel-text">{DONE_TEXT}</p>
      ) : (
        <div className="form-actions">
          <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void understood()}>
            Понятно
          </button>
        </div>
      )}
    </section>
  );
}
