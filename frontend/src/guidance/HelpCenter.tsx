// Раздел «Помощь» (ТЗ 8.4 п.6): туры, «Первые шаги» с отметками, «Что нового». Это справка по
// запросу: она работает всегда, даже когда подсказки выключены в настройках.
import { useCallback, useEffect, useRef, useState } from "react";
import Notice from "../components/Notice";
import { errorText } from "../lib/errors";
import { TOURS, WHATS_NEW } from "./catalog";
import { useGuidance } from "./context";
import { IconCheck, IconCircle } from "./icons";
import { newestFirst, plural } from "./lists";
import { loadFirstSteps, type FirstStep } from "./steps";

const STEPS_WORD = ["шаг", "шага", "шагов"] as const;

export default function HelpCenter() {
  const g = useGuidance();
  const [steps, setSteps] = useState<FirstStep[] | null>(null);
  const [stepsError, setStepsError] = useState("");
  const alive = useRef(true);

  const loadSteps = useCallback(async () => {
    try {
      const next = await loadFirstSteps();
      if (!alive.current) return;
      setSteps(next);
      setStepsError("");
    } catch (e) {
      if (alive.current) setStepsError(errorText(e));
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    void loadSteps();
    // Вернулись в окно: пока смотрели в другое место, что-то могли сделать.
    const onFocus = () => void loadSteps();
    window.addEventListener("focus", onFocus);
    return () => {
      alive.current = false;
      window.removeEventListener("focus", onFocus);
    };
  }, [loadSteps]);

  const doneTours = g?.engine.data.tours;
  const doneCount = steps?.filter((s) => s.done).length ?? 0;

  return (
    <section aria-labelledby="help-title">
      <h2 id="help-title" className="panel-title">
        Помощь
      </h2>

      {g && g.engine.ready && !g.enabled ? (
        <p className="field-hint help-off">
          Подсказки в окне сейчас выключены. Обучение и справка работают как обычно.{" "}
          <button type="button" className="link" onClick={() => g.navigate("settings")}>
            Открыть «Настройки»
          </button>
        </p>
      ) : null}

      <section className="panel" aria-labelledby="help-tours">
        <h3 id="help-tours" className="section-title">
          Обучение
        </h3>
        <ul className="help-list">
          {TOURS.map((t) => {
            const done = doneTours?.has(t.id) ?? false;
            return (
              <li key={t.id} className="help-item">
                <div className="help-item-main">
                  <p className="help-item-title">{t.title}</p>
                  <p className="field-hint">
                    {`${t.steps.length} ${plural(t.steps.length, STEPS_WORD)}, около минуты. Тур только показывает, ничего не меняет.`}
                  </p>
                </div>
                {done ? <span className="help-done">Пройден</span> : null}
                {g ? (
                  <button type="button" className={done ? "btn btn-small" : "btn btn-small btn-primary"} onClick={() => g.startTour(t.id)}>
                    {done ? "Пройти ещё раз" : "Начать"}
                  </button>
                ) : null}
              </li>
            );
          })}
        </ul>
      </section>

      <section className="panel" aria-labelledby="help-first">
        <h3 id="help-first" className="section-title">
          Первые шаги
        </h3>
        {stepsError ? (
          <>
            <Notice tone="warn" urgent>
              {stepsError}
            </Notice>
            <button type="button" className="btn btn-small" onClick={() => void loadSteps()}>
              Повторить
            </button>
          </>
        ) : steps === null ? (
          <p className="panel-text">Загружаю данные</p>
        ) : (
          <>
            {doneCount === steps.length ? (
              <p className="field-hint">Все первые шаги сделаны.</p>
            ) : doneCount > 0 ? (
              <p className="field-hint">{`Сделано ${doneCount} из ${steps.length}`}</p>
            ) : null}
            <ul className="help-list">
              {steps.map((s) => (
                <li key={s.id} className="help-item" data-done={s.done}>
                  <span className={s.done ? "help-mark help-mark-done" : "help-mark"}>{s.done ? <IconCheck /> : <IconCircle />}</span>
                  <div className="help-item-main">
                    <p className="help-item-title">{s.label}</p>
                    <p className="field-hint">{s.done ? "Сделано" : "Пока не сделано"}</p>
                  </div>
                  {!s.done && s.go && s.goLabel && g ? (
                    <button type="button" className="btn btn-small" onClick={() => g.navigate(s.go!)}>
                      {s.goLabel}
                    </button>
                  ) : null}
                </li>
              ))}
            </ul>
          </>
        )}
      </section>

      <section className="panel" aria-labelledby="help-new">
        <h3 id="help-new" className="section-title">
          Что нового
        </h3>
        {newestFirst(WHATS_NEW).map((n) => (
          <div key={n.id} className="help-news">
            <h4 className="subsection-title">{`Версия ${n.version}. ${n.title}`}</h4>
            <ul className="help-bullets">
              {n.items.map((item) => (
                <li key={item}>{item}</li>
              ))}
            </ul>
          </div>
        ))}
      </section>
    </section>
  );
}
