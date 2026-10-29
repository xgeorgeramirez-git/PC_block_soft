import { useCallback, useEffect, useRef, useState } from "react";
import { getBonus, myToday, openWindow } from "../api";
import AskMoreDialog from "../components/AskMoreDialog";
import Notice from "../components/Notice";
import PromoRedeem from "../components/PromoRedeem";
import SpendBonusDialog from "../components/SpendBonusDialog";
import { historyText } from "../lib/bonus";
import { childErrorText } from "../lib/errors";
import { formatDuration, formatEventTime, formatLimit, weekdayName } from "../lib/format";
import { NOT_COUNTED, NOT_SHOWN, spendBlockedNote, splitRules, summaryLines, todayRows, todayRuleWeekday } from "../lib/mytoday";
import type { GetBonusReply, MyTodayReply } from "../types";

const LOAD_ERROR = "Не удалось получить данные. Попробуй через минуту";
const NO_DATA = "За этот период пока нет данных";
const PERIODS = ["Сегодня", "Неделя", "Месяц", "Год", "Всё время"] as const;
const PERIODS_NOTE = "Неделя, месяц, год и всё время появятся в полной статистике";
const OPEN_FAILED = "Не получилось открыть окно. Попробуй ещё раз";
/** Итоги по категориям: игры и учёба считаются с 0.3.0, видео и соцсети - с M4. */
const CATEGORY_TOTALS: { label: string; value: string }[] = [
  { label: "Игры", value: NOT_SHOWN },
  { label: "Видео и соцсети", value: NOT_COUNTED },
  { label: "Учёба", value: NOT_SHOWN },
];

/** Окно «Моя статистика» (FR-12, R21, сценарий 8.2.6): только просмотр, свои данные, в M1 - сегодня и правила. */
export default function MyStats() {
  const [data, setData] = useState<MyTodayReply | null>(null);
  const [bonus, setBonus] = useState<GetBonusReply | null>(null);
  const [error, setError] = useState("");
  const [dialog, setDialog] = useState<"ask" | "spend" | null>(null);
  const [done, setDone] = useState("");
  const [openError, setOpenError] = useState("");
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
    // Своя копилка (номер 0 — «моя»): раздел «Бонусы» (R25, R26). Не вышло — раздел без истории.
    try {
      const b = await getBonus(0);
      if (alive.current) setBonus(b);
    } catch {
      if (alive.current) setBonus(null);
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

  const finish = (message: string) => {
    setDialog(null);
    setDone(message);
    void load();
  };

  const openParent = async () => {
    setOpenError("");
    try {
      await openWindow("parent");
    } catch {
      setOpenError(OPEN_FAILED);
    }
  };

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

      {done ? <Notice tone="ok">{done}</Notice> : null}
      {openError ? (
        <Notice tone="warn" urgent>
          {openError}
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
          {data.status.status === "notChild" ? null : (
            <Remaining data={data} onAsk={() => setDialog("ask")} onSpend={() => setDialog("spend")} />
          )}
          <Totals data={data} />
          {data.status.status === "notChild" ? null : <Bonuses bonus={bonus} onChange={() => void load()} />}
          <Rules data={data} />
          <p className="my-foot">
            <button type="button" className="btn link" onClick={() => void openParent()}>
              Настройки для родителя
            </button>
          </p>
        </div>
      )}

      {dialog === "ask" ? <AskMoreDialog childId={bonus?.childId ?? null} onDone={finish} onClose={() => setDialog(null)} /> : null}
      {dialog === "spend" && data ? (
        <SpendBonusDialog balance={data.status.bonusBalance} onDone={finish} onClose={() => setDialog(null)} />
      ) : null}
    </div>
  );
}

function LOAD_ERROR_FOR(e: unknown): string {
  const text = childErrorText(e);
  // Связь с программой оборвалась: тот же текст, что в мини-панели (8.2.1 п.5).
  return text.startsWith("Программа не отвечает") ? LOAD_ERROR : text;
}

/**
 * «Сколько осталось» и кнопки «Попросить ещё» / «Взять из копилки» (ТЗ 8.2.6 п.1, FR-10, FR-11, D-53).
 * Недоступная кнопка остаётся на месте и объясняет почему (урок L-17).
 */
export function Remaining({ data, onAsk, onSpend }: { data: MyTodayReply; onAsk: () => void; onSpend: () => void }) {
  const rows = todayRows(data.status);
  const note = spendBlockedNote(data.status, data.status.bonusBalance);
  return (
    <>
      <section className="panel stat-panel" aria-labelledby="my-left">
        <h3 id="my-left" className="section-title">
          Сколько осталось
        </h3>
        <dl className="stat-list">
          {rows.map((r, i) => (
            <div key={r.label} className={i === 0 ? "stat-row stat-row-lead" : "stat-row"}>
              <dt>{r.label}</dt>
              <dd>{r.value}</dd>
            </div>
          ))}
        </dl>
      </section>
      <section className="panel stat-panel" aria-labelledby="my-more">
        <h3 id="my-more" className="section-title">
          Нужно больше времени?
        </h3>
        <div className="action-row">
          <button type="button" className="btn btn-primary" onClick={onAsk}>
            Попросить ещё
          </button>
          <button
            type="button"
            className="btn"
            aria-disabled={note ? "true" : undefined}
            aria-describedby={note ? "spend-note" : undefined}
            onClick={note ? undefined : onSpend}
          >
            Взять из копилки
          </button>
        </div>
        {note ? (
          <p id="spend-note" className="spend-note" role="note">
            {note}
          </p>
        ) : null}
      </section>
    </>
  );
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
              <dt>Использовано сегодня</dt>
              <dd>{formatDuration(data.status.usedS)}</dd>
            </div>
            {CATEGORY_TOTALS.map((c) => (
              <div key={c.label} className="stat-row stat-row-muted">
                <dt>{c.label}</dt>
                <dd>{c.value}</dd>
              </div>
            ))}
          </dl>
        </>
      )}
    </section>
  );
}

/**
 * «Бонусы» (R25, R26, D-49, D-50): копилка, автобонусы простыми словами и поле промокода —
 * единственное поле окна «Моя статистика».
 */
export function Bonuses({ bonus, onChange }: { bonus: GetBonusReply | null; onChange: () => void }) {
  const entries = bonus?.entries ?? [];
  return (
    <section className="panel stat-panel" aria-labelledby="my-bonuses">
      <h3 id="my-bonuses" className="section-title">
        Бонусы
      </h3>
      {bonus ? <p className="stat-lead">{`Копилка: ${formatLimit(bonus.balance)}`}</p> : null}
      <p className="panel-text">
        В полночь в копилку приходит бонус за неизрасходованное за день время. Если родитель дал тебе промокод,
        введи его здесь: время сразу добавится к сегодняшнему дню.
      </p>
      <PromoRedeem onRedeemed={onChange} />
      {entries.length > 0 ? (
        <ul className="bonus-history" aria-label="История копилки">
          {entries.slice(0, 20).map((e, i) => (
            <li key={i} className="bonus-history-item">
              <span>{formatEventTime(e.atMs, e.tzOffsetMin)}</span>
              <span>{historyText(e)}</span>
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  );
}

export function Rules({ data }: { data: MyTodayReply }) {
  const today = todayRuleWeekday();
  const { days, general } = splitRules(data.rules);
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
              {days.map((rule) => (
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
          {general.length > 0 ? (
            <ul className="my-rules-general" aria-label="Общие правила">
              {general.map((text) => (
                <li key={text}>{text}</li>
              ))}
            </ul>
          ) : null}
        </>
      )}
    </section>
  );
}
