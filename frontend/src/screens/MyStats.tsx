import { useCallback, useEffect, useRef, useState, type CSSProperties, type KeyboardEvent, type ReactNode } from "react";
import { getBonus, myToday, openWindow } from "../api";
import AskMoreDialog from "../components/AskMoreDialog";
import BonusHistory from "../components/BonusHistory";
import DayBars from "../components/DayBars";
import { LineText } from "../components/FitText";
import Notice from "../components/Notice";
import PeriodTabs from "../components/PeriodTabs";
import PromoRedeem from "../components/PromoRedeem";
import SpendBonusDialog from "../components/SpendBonusDialog";
import { childErrorText } from "../lib/errors";
import { formatDuration, formatEventTime, formatLimit, weekdayName } from "../lib/format";
import {
  allWithStudyText,
  EMPTY_TAB,
  TOTALS_TABS,
  periodEmpty,
  periodOf,
  spendBlockedNote,
  splitRules,
  summaryLines,
  tabRows,
  tabSeconds,
  todayRows,
  todayRuleWeekday,
  usageRows,
  type TotalsTab,
  type UsageRow,
} from "../lib/mytoday";
import type { GetBonusReply, JournalEntry, MyTodayReply, StatsPeriod } from "../types";

const LOAD_ERROR = "Не удалось получить данные. Попробуй через минуту";
const NO_DATA = "За этот период пока нет данных";
/** Признак загрузки периода (короткий, на месте: страница не мигает). */
export const LOADING_PERIOD = "Загружаю…";
const OPEN_FAILED = "Не получилось открыть окно. Попробуй ещё раз";
/** id панели периода «Итогов»: на неё указывают вкладки периодов (aria-controls). */
export const PERIOD_PANEL = "period-panel";
/** Сколько нарушений показано сразу; остальные - кнопкой «Показать все». */
const VIOLATIONS_SHOWN = 20;

/**
 * Окно «Моя статистика» (FR-12, R21, сценарий 8.2.6): только просмотр, свои данные. Период (D-61) меняет
 * карточку «Итоги» с разделами «По дням», «Программы», «Сайты», «Нарушения», «Копилка»; остаток, бонусы
 * и «Мои правила» - за сегодня. Ни выбора ребёнка, ни очистки, ни полей статистики (R21, S22).
 */
export default function MyStats() {
  const [data, setData] = useState<MyTodayReply | null>(null);
  const [bonus, setBonus] = useState<GetBonusReply | null>(null);
  const [error, setError] = useState("");
  const [dialog, setDialog] = useState<"ask" | "spend" | null>(null);
  const [done, setDone] = useState("");
  const [openError, setOpenError] = useState("");
  const [period, setPeriod] = useState<StatsPeriod>("today");
  const [busy, setBusy] = useState(false);
  const alive = useRef(true);
  const periodRef = useRef<StatsPeriod>("today");
  // Номер последнего запроса: поздний ответ прошлого периода не перекрывает выбранный.
  const seq = useRef(0);
  // Период цифр на экране: при сбое вкладка возвращается к нему, вкладка и цифры - одного периода.
  const shown = useRef<StatsPeriod | null>(null);

  const loadStats = useCallback(async (p: StatsPeriod) => {
    const my = ++seq.current;
    setBusy(true);
    setError("");
    try {
      const reply = await myToday(p);
      if (alive.current && my === seq.current) {
        shown.current = reply.period;
        setData(reply);
      }
    } catch (e) {
      // Сбой связи говорит «Не удалось получить данные»; слова самой программы оставляем (L-112).
      if (alive.current && my === seq.current) {
        setError(LOAD_ERROR_FOR(e));
        if (shown.current) {
          periodRef.current = shown.current;
          setPeriod(shown.current);
        }
      }
    } finally {
      if (alive.current && my === seq.current) setBusy(false);
    }
  }, []);

  const load = useCallback(async () => {
    await loadStats(periodRef.current);
    // Своя копилка (номер 0 — «моя»): раздел «Бонусы» (R25, R26). Не вышло — раздел без истории.
    try {
      const b = await getBonus(0);
      if (alive.current) setBonus(b);
    } catch {
      if (alive.current) setBonus(null);
    }
  }, [loadStats]);

  const choosePeriod = (p: StatsPeriod) => {
    periodRef.current = p;
    setPeriod(p);
    void loadStats(p);
  };

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

      {data === null ? (
        error ? null : <p className="panel-text">Загружаю данные…</p>
      ) : (
        <>
          {data.status.status === "notChild" ? null : (
            <Remaining data={data} onAsk={() => setDialog("ask")} onSpend={() => setDialog("spend")} />
          )}
          <StatsSince data={data} />
          {/* Вкладки периодов - внутри «Итогов», рядом с данными, которые они меняют (UX-аудит FB-13). */}
          <Totals data={data} busy={busy} periods={{ value: period, onChange: choosePeriod }} />
          {data.status.status === "notChild" ? null : <Bonuses bonus={bonus} onChange={() => void load()} />}
          <Rules data={data} />
          <p className="my-foot">
            <button type="button" className="btn link" onClick={() => void openParent()}>
              Настройки для родителя
            </button>
          </p>
        </>
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

/**
 * «Итоги» (8.2.6 п.3, D-58, D-61) за период ответа: «Всего», «Игры», «Видео и соцсети», «Учёба», «Прочее» -
 * вкладки; выбранная показывает сумму и каждую программу или сайт «название, время, доля», самые долгие
 * сверху. busy - идёт загрузка другого периода: прежние цифры стоят, у заголовка короткий признак. Общий
 * для окна ребёнка и статистики родителя.
 */
export function Totals({
  data,
  busy = false,
  periods,
}: {
  data: MyTodayReply;
  busy?: boolean;
  /** Вкладки периодов под заголовком; содержимое «Итогов» - их панель (id PERIOD_PANEL). */
  periods?: { value: StatsPeriod; onChange: (period: StatsPeriod) => void };
}) {
  const notChild = data.status.status === "notChild";
  const empty = notChild || data.today.length === 0 || periodEmpty(data);
  const info = periodOf(data.period);
  const [tab, setTab] = useState<TotalsTab>("all");
  const tabs = useRef(new Map<TotalsTab, HTMLButtonElement>());
  const onKey = (e: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const last = TOTALS_TABS.length - 1;
    let next: number;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") next = index === last ? 0 : index + 1;
    else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = index === 0 ? last : index - 1;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = last;
    else return;
    e.preventDefault();
    const id = TOTALS_TABS[next].id;
    setTab(id);
    tabs.current.get(id)?.focus();
  };
  const current = TOTALS_TABS.find((t) => t.id === tab) ?? TOTALS_TABS[0];
  const rows = empty ? [] : tabRows(data, current.id);
  const studyNote = empty ? "" : allWithStudyText(data);
  const totals = (
    empty ? (
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
              <dt>{info.used}</dt>
              <dd>{formatDuration(data.usedS)}</dd>
            </div>
            <div className="stat-row">
              <dt>Доп. время</dt>
              <dd>{formatLimit(data.extraMin)}</dd>
            </div>
            <div className="stat-row">
              <dt>Взято из копилки</dt>
              <dd>{formatLimit(data.bonusSpentMin)}</dd>
            </div>
          </dl>
          <div className="totals-tabs" role="tablist" aria-label="Итоги по категориям">
            {TOTALS_TABS.map((t, i) => (
              <button
                key={t.id}
                ref={(el) => {
                  if (el) tabs.current.set(t.id, el);
                  else tabs.current.delete(t.id);
                }}
                id={`totals-tab-${t.id}`}
                type="button"
                role="tab"
                className="totals-tab"
                aria-selected={t.id === current.id}
                aria-controls="totals-panel"
                aria-describedby={t.id === "all" && studyNote ? "totals-value-all totals-study-note" : `totals-value-${t.id}`}
                tabIndex={t.id === current.id ? 0 : -1}
                onClick={() => setTab(t.id)}
                onKeyDown={(e) => onKey(e, i)}
              >
                <span className="totals-tab-label">{t.label}</span>
                <span id={`totals-value-${t.id}`} className="totals-tab-value" aria-hidden="true">
                  <DurationParts text={formatDuration(tabSeconds(data, t.id))} />
                </span>
              </button>
            ))}
          </div>
          {/* Плитка «Всего» видна при любой вкладке, поэтому и пояснение к ней - под вкладками, не в панели. */}
          {studyNote ? (
            <p id="totals-study-note" className="field-hint totals-note">
              {studyNote}
            </p>
          ) : null}
          <div id="totals-panel" className="totals-panel" role="tabpanel" tabIndex={0} aria-labelledby={`totals-tab-${current.id}`}>
            <p className="totals-sum">{`${current.label} ${info.sum}: ${formatDuration(tabSeconds(data, current.id))}`}</p>
            {rows.length === 0 ? (
              <p className="panel-text totals-empty">{EMPTY_TAB}</p>
            ) : (
              <ul className="totals-list" aria-label={current.list}>
                {rows.map((r) => (
                  <li key={r.key} className={r.rest ? "totals-item totals-item-rest" : "totals-item"} style={{ "--share": r.share } as CSSProperties}>
                    <LineText className="totals-name" text={r.name} />
                    <span className="totals-time">{r.time}</span>
                    <span className="totals-share">{r.share}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
          <StatSub id="days" title="По дням">
            <DayBars reply={data} />
          </StatSub>
          <StatSub id="programs" title="Программы">
            <UsageTable id="programs" rows={usageRows(data, "program")} empty="Программ за этот период нет" />
          </StatSub>
          <StatSub id="sites" title="Сайты">
            <UsageTable id="sites" rows={usageRows(data, "site")} empty="Сайтов за этот период нет" />
          </StatSub>
        </>
      )
  );
  // Всё, что зависит от периода, - в одной панели периода (PL-36): сначала «Итоги» с «По дням»,
  // «Программами» и «Сайтами» (пустой период - одна строка, 8.2.6 п.9), затем «Нарушения» и «Копилка»
  // со своими пустыми текстами. Сеанс не ребёнка - только строка о пустом периоде.
  const content = notChild ? (
    totals
  ) : (
    <>
      {totals}
      <StatSub id="violations" title="Нарушения">
        <Violations key={data.period} entries={data.violations} />
      </StatSub>
      <StatSub id="bonus" title="Копилка">
        <p className="stat-lead">{`Сейчас в копилке: ${formatLimit(data.status.bonusBalance)}`}</p>
        <BonusHistory entries={data.bonus} empty="За этот период копилка не менялась" label="История копилки за период" />
      </StatSub>
    </>
  );
  return (
    <section className="panel stat-panel totals" aria-labelledby="my-totals" aria-busy={busy}>
      <div className="totals-head">
        <h3 id="my-totals" className="section-title">
          Итоги
        </h3>
        <span className="totals-loading" role="status">
          {busy ? LOADING_PERIOD : ""}
        </span>
      </div>
      {periods ? <PeriodTabs value={periods.value} onChange={periods.onChange} panelId={PERIOD_PANEL} /> : null}
      {periods ? (
        <div id={PERIOD_PANEL} role="tabpanel" aria-labelledby={`period-${periods.value}`}>
          {content}
        </div>
      ) : (
        content
      )}
    </section>
  );
}

/**
 * «Бонусы» (R25, R26, D-49, D-50): баланс копилки, как приходит автобонус, и поле промокода —
 * единственное поле окна «Моя статистика». История копилки - в разделе «Копилка» за период (одно место).
 */
export function Bonuses({ bonus, onChange }: { bonus: GetBonusReply | null; onChange: () => void }) {
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

/** «Статистика ведётся с 11 ноября 2026» над карточкой «Итогов» (8.2.6 п.10, FR-38); нет текста - нет строки. */
export function StatsSince({ data }: { data: MyTodayReply }) {
  if (data.status.status === "notChild" || !data.statsSinceText) return null;
  return <p className="stats-since">{data.statsSinceText}</p>;
}

/** Раздел карточки периода: заголовок и содержимое, region с именем заголовка. */
function StatSub({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <section className="stat-sub" aria-labelledby={`sub-${id}`}>
      <h4 id={`sub-${id}`} className="section-title">
        {title}
      </h4>
      {children}
    </section>
  );
}

/**
 * Таблица «Программы» или «Сайты» (8.2.6 п.5): «название, время, доля», самые долгие сверху. Название -
 * одной строкой с обрезкой одним помощником (PL-26), время и доля - справа ровными цифрами.
 */
export function UsageTable({ id, rows, empty }: { id: string; rows: UsageRow[]; empty: string }) {
  if (rows.length === 0) return <p className="panel-text">{empty}</p>;
  return (
    <table className="usage-table" aria-labelledby={`sub-${id}`}>
      <colgroup>
        <col />
        <col className="usage-col-time" />
        <col className="usage-col-share" />
      </colgroup>
      <thead>
        <tr>
          <th scope="col">Название</th>
          <th scope="col" className="usage-num">
            Время
          </th>
          <th scope="col" className="usage-num">
            Доля
          </th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.key}>
            <td>
              <LineText className="usage-name" text={r.name} />
            </td>
            <td className="usage-num">{r.time}</td>
            <td className="usage-num usage-share">{r.share}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/**
 * «Нарушения» за период (8.2.6 п.6): «когда - что» текстом журнала, новые сверху; первые 20, остальные -
 * кнопкой. Обычный цвет текста: красный - у статуса, а не у списка (ТЗ 8.0 п.3).
 */
function Violations({ entries }: { entries: JournalEntry[] }) {
  const [all, setAll] = useState(false);
  if (entries.length === 0) return <p className="panel-text">Нарушений за этот период нет</p>;
  const shown = all ? entries : entries.slice(0, VIOLATIONS_SHOWN);
  return (
    <>
      <ul className="journal stat-journal" aria-labelledby="sub-violations">
        {shown.map((e) => (
          <li key={e.id} className="journal-item">
            <time className="journal-time" dateTime={new Date(e.atMs).toISOString()}>
              {formatEventTime(e.atMs, e.tzOffsetMin)}
            </time>
            <span className="journal-text">{e.text}</span>
          </li>
        ))}
      </ul>
      {!all && entries.length > VIOLATIONS_SHOWN ? (
        <button type="button" className="btn link stat-more" onClick={() => setAll(true)}>
          {`Показать все (${entries.length})`}
        </button>
      ) : null}
    </>
  );
}

/**
 * Длительность плитки «Итогов» частями «14 ч» и «08 мин»: длинное значение («1625 ч 00 мин») переносится
 * между частями и не обрезается рамкой плитки; число с единицей не разрывается. Текст тот же.
 */
function DurationParts({ text }: { text: string }) {
  const m = /^(\d+ ч) (\d+ мин)$/.exec(text);
  if (!m) return <span className="totals-tab-part">{text}</span>;
  return (
    <>
      <span className="totals-tab-part">{m[1]}</span> <span className="totals-tab-part">{m[2]}</span>
    </>
  );
}
