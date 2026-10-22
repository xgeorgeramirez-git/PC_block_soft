import { useCallback, useEffect, useRef, useState } from "react";
import { myToday } from "../api";
import Notice from "../components/Notice";
import { childErrorText } from "../lib/errors";
import { formatDuration, weekdayName } from "../lib/format";
import { summaryLines, todayRuleWeekday } from "../lib/mytoday";
import type { MyTodayReply } from "../types";

const LOAD_ERROR = "Не удалось получить данные. Попробуй через минуту";
const NO_DATA = "За этот период пока нет данных";
const PERIODS = ["Сегодня", "Неделя", "Месяц", "Год", "Всё время"] as const;
const PERIODS_NOTE = "Неделя, месяц, год и всё время появятся в полной статистике";
const NOT_COUNTED = "Пока не считается";
const END_MODE = "Когда время выйдет, экран закроется блокировкой. Открытые окна останутся под ней.";

/** Окно «Моя статистика» (FR-12, R21, сценарий 8.2.6): только просмотр, свои данные, в M1 - сегодня и правила. */
export default function MyStats() {
  const [data, setData] = useState<MyTodayReply | null>(null);
  const [error, setError] = useState("");
  const alive = useRef(true);

  const load = useCallback(async () => {
    setError("");
    try {
      const reply = await myToday();
      if (alive.current) setData(reply);
    } catch (e) {
      // Сбой связи говорит «Не удалось получить данные»; слова самой программы оставляем (L-112).
      if (alive.current) setError(LOAD_ERROR_FOR(e));
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    void load();
    // Окно могло долго лежать открытым: вернулись в него, и цифры должны быть свежими.
    const onFocus = () => void load();
    window.addEventListener("focus", onFocus);
    return () => {
      alive.current = false;
      window.removeEventListener("focus", onFocus);
    };
  }, [load]);

  return (
    <div className="my-stats">
      <div className="page-head">
        <h2 className="panel-title">Моя статистика</h2>
        <span className="read-only-badge">Только просмотр</span>
        {data?.status.name ? <span className="page-date">{data.status.name}</span> : null}
        <span className="topbar-spacer" />
        <button type="button" className="btn btn-small" onClick={() => void load()}>
          Обновить
        </button>
      </div>

      {error ? (
        <Notice tone="warn" urgent>
          {error}
        </Notice>
      ) : null}

      <div className="tabs" role="tablist" aria-label="Период">
        {PERIODS.map((name) => {
          const current = name === "Сегодня";
          return (
            <button
              key={name}
              type="button"
              role="tab"
              id={`period-${name}`}
              className="tab"
              aria-selected={current}
              aria-disabled={current ? undefined : "true"}
              aria-describedby={current ? undefined : "periods-note"}
              aria-controls={current ? "period-panel" : undefined}
              tabIndex={current ? 0 : -1}
              onClick={current ? undefined : (e) => e.preventDefault()}
            >
              {name}
            </button>
          );
        })}
      </div>
      <p id="periods-note" className="field-hint periods-note">
        {PERIODS_NOTE}
      </p>

      {data === null ? (
        error ? null : <p className="panel-text">Загружаю данные</p>
      ) : (
        <div id="period-panel" role="tabpanel" aria-labelledby="period-Сегодня">
          <Totals data={data} />
          <Rules data={data} />
        </div>
      )}
    </div>
  );
}

function LOAD_ERROR_FOR(e: unknown): string {
  const text = childErrorText(e);
  // Связь с программой оборвалась: тот же текст, что в мини-панели (8.2.1 п.5).
  return text.startsWith("Программа не отвечает") ? LOAD_ERROR : text;
}

export function Totals({ data }: { data: MyTodayReply }) {
  const empty = data.status.status === "notChild" || data.today.length === 0;
  return (
    <section className="panel stat-panel" aria-labelledby="my-totals">
      <h3 id="my-totals" className="section-title">
        Итоги
      </h3>
      {empty ? (
        <p className="panel-text">{NO_DATA}</p>
      ) : (
        <>
          {summaryLines(data).map((line) => (
            <p key={line} className="stat-lead">
              {line}
            </p>
          ))}
          <dl className="stat-list">
            <div className="stat-row">
              <dt>Всего</dt>
              <dd>{formatDuration(data.status.usedS)}</dd>
            </div>
            {["Игры", "Видео и соцсети", "Учёба"].map((label) => (
              <div key={label} className="stat-row stat-row-muted">
                <dt>{label}</dt>
                <dd>{NOT_COUNTED}</dd>
              </div>
            ))}
          </dl>
        </>
      )}
    </section>
  );
}

export function Rules({ data }: { data: MyTodayReply }) {
  const today = todayRuleWeekday();
  return (
    <section className="panel stat-panel" aria-labelledby="my-rules">
      <h3 id="my-rules" className="section-title">
        Мои правила
      </h3>
      {data.rules.length === 0 ? (
        <p className="panel-text">
          {data.status.status === "notChild"
            ? "Эта учётная запись не детская: здесь нет правил."
            : "Правила сейчас недоступны. Спроси у родителя."}
        </p>
      ) : (
        <>
          <table className="my-rules">
            <caption className="visually-hidden">Сколько времени можно в каждый день недели</caption>
            <thead>
              <tr>
                <th scope="col">День</th>
                <th scope="col">Сколько можно</th>
              </tr>
            </thead>
            <tbody>
              {data.rules.map((rule) => (
                <tr key={rule.weekday} aria-current={rule.weekday === today ? "date" : undefined}>
                  <th scope="row">{weekdayName(rule.weekday)}</th>
                  <td>
                    {rule.text}
                    {rule.weekday === today ? (
                      <>
                        {" "}
                        <span className="today-mark">(сегодня)</span>
                      </>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="panel-note">{END_MODE}</p>
        </>
      )}
    </section>
  );
}
