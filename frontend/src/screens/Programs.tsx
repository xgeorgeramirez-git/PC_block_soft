import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { decideProgram, listPrograms, setProgramsCategory, setProgramsHidden, undoProgram } from "../api";
import Notice from "../components/Notice";
import UndoLine from "../components/UndoLine";
import { errorText } from "../lib/errors";
import { localOffsetMin } from "../lib/format";
import {
  CATEGORY_OPTIONS,
  DECISIONS,
  FILTERS,
  attemptsText,
  categoryTitle,
  decisionMessage,
  hiddenMessage,
  fitPath,
  publisherText,
  shortenPath,
  visiblePrograms,
  wasCategoryText,
  weekText,
  type ParentCategory,
  type ProgramFilter,
  type ProgramSort,
} from "../lib/programs";
import type { PendingProgramDTO, ProgramDTO, ProgramsReply } from "../types";

interface Props {
  /** С какого фильтра открыть список (ссылка «Учебных программ пока нет…» ведёт на «Учёба»). */
  initialFilter?: ProgramFilter;
}

/** Предел длины пути в знаках: у ждущей программы строка шире, чем в таблице. */
const PENDING_PATH_MAX = 80;
const ROW_PATH_MAX = 40;
const UNDO_MS = 10_000;

interface Feedback {
  seq: number;
  text: string;
  error: boolean;
  /** Где сообщение: над «Ждут решения» или над таблицей, рядом с тем, что нажали. */
  where: "top" | "list";
  undo?: { undoId: string; ms: number };
}

/** Что сейчас в пути к службе: пока запрос идёт, остальные действия недоступны. */
type Busy =
  | { kind: "decide"; path: string; category: ParentCategory }
  | { kind: "row"; path: string }
  | { kind: "bulk"; hide?: boolean }
  | { kind: "undo" };

/** Куда поставить фокус после ответа: элемент, на котором он был, мог исчезнуть. */
type FocusTo =
  | { to: "pending"; index: number }
  | { to: "path"; path: string; category?: ParentCategory; index?: number }
  | { to: "selectAll" };

/** Последняя отменяемая операция: куда вернуть фокус после «Отменить». */
type LastOp = { kind: "one"; path: string } | { kind: "bulk" };

/** Экран «Программы» (FR-20, сценарий 8.3.4): ждут решения и все программы. */
export default function Programs({ initialFilter = "all" }: Props) {
  const [data, setData] = useState<ProgramsReply | null>(null);
  const [loadError, setLoadError] = useState("");
  const [busy, setBusyState] = useState<Busy | null>(null);
  const [feedback, setFeedback] = useState<Feedback | null>(null);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<ProgramFilter>(initialFilter);
  const [sort, setSort] = useState<ProgramSort>({ key: "name", dir: "asc" });
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [bulkCategory, setBulkCategory] = useState<ParentCategory | "">("");
  const [bulkError, setBulkError] = useState("");
  const alive = useRef(true);
  const seq = useRef(0);
  const busyRef = useRef(false);
  const root = useRef<HTMLElement>(null);
  const focusTo = useRef<FocusTo | null>(null);
  const lastOp = useRef<LastOp | null>(null);

  const load = useCallback(async () => {
    setLoadError("");
    try {
      const reply = await listPrograms();
      if (alive.current) setData(reply);
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

  // Фокус после ответа службы: на следующую ждущую, на ту же строку или на заголовок списка.
  useLayoutEffect(() => {
    const req = focusTo.current;
    if (!req || busy || !root.current) return;
    focusTo.current = null;
    placeFocus(root.current, req);
  });

  const say = (where: Feedback["where"], text: string, error = false, undo?: Feedback["undo"]) => {
    seq.current += 1;
    setFeedback({ seq: seq.current, where, text, error, undo });
  };

  /** Один запрос за раз: пока ответа нет, повторное нажатие ничего не отправляет. */
  const run = async (what: Busy, action: () => Promise<FocusTo | null>, onError: FocusTo | null) => {
    const where: Feedback["where"] = what.kind === "decide" ? "top" : what.kind === "undo" ? (feedback?.where ?? "top") : "list";
    if (busyRef.current) return;
    busyRef.current = true;
    setBusyState(what);
    try {
      const next = await action();
      focusTo.current = next;
    } catch (e) {
      if (alive.current) {
        say(where, errorText(e), true);
        focusTo.current = onError;
      }
    } finally {
      busyRef.current = false;
      if (alive.current) setBusyState(null);
    }
  };

  const applyReply = (programs: ProgramsReply) => {
    setData(programs);
    setSelected((prev) => {
      const known = new Set(programs.programs.map((p) => p.path));
      return new Set([...prev].filter((p) => known.has(p)));
    });
  };

  const sayDecided = (
    where: Feedback["where"],
    reply: { message: string; undoId: string; undoUntilMs: number },
    fallback: string,
  ) => {
    const left = Math.min(reply.undoUntilMs - Date.now(), UNDO_MS);
    say(where, reply.message || fallback, false, reply.undoId && left > 0 ? { undoId: reply.undoId, ms: left } : undefined);
  };

  const decide = (p: PendingProgramDTO, category: ParentCategory, index: number) =>
    void run(
      { kind: "decide", path: p.path, category },
      async () => {
        const reply = await decideProgram(p.path, category);
        if (!alive.current) return null;
        applyReply(reply.programs);
        lastOp.current = { kind: "one", path: p.path };
        sayDecided("top", reply, decisionMessage(p.name, category));
        return { to: "pending", index };
      },
      { to: "path", path: p.path, category },
    );

  const changeRow = (p: ProgramDTO, category: ParentCategory, index: number) =>
    void run(
      { kind: "row", path: p.path },
      async () => {
        const reply = await decideProgram(p.path, category);
        if (!alive.current) return null;
        applyReply(reply.programs);
        lastOp.current = { kind: "one", path: p.path };
        sayDecided("list", reply, decisionMessage(p.name, category));
        return { to: "path", path: p.path, index };
      },
      { to: "path", path: p.path, index },
    );

  /** «Скрыть» / «Показать» в строке (D-57): строка уходит из текущего фильтра, фокус на соседнюю. */
  const hideRow = (p: ProgramDTO, hidden: boolean, index: number) =>
    void run(
      { kind: "row", path: p.path },
      async () => {
        const reply = await setProgramsHidden([p.path], hidden);
        if (!alive.current) return null;
        applyReply(reply.programs);
        setSelected((prev) => {
          const next = new Set(prev);
          next.delete(p.path);
          return next;
        });
        lastOp.current = { kind: "one", path: p.path };
        sayDecided("list", reply, hiddenMessage(p.name, hidden));
        return { to: "path", path: p.path, index };
      },
      { to: "path", path: p.path, index },
    );

  /** «Скрыть» / «Показать» для отмеченных строк. */
  const hideBulk = (paths: string[], hidden: boolean) =>
    void run(
      { kind: "bulk", hide: true },
      async () => {
        const reply = await setProgramsHidden(paths, hidden);
        if (!alive.current) return null;
        applyReply(reply.programs);
        setSelected(new Set());
        setBulkCategory("");
        setBulkError("");
        lastOp.current = { kind: "bulk" };
        sayDecided("list", reply, "");
        return { to: "selectAll" };
      },
      { to: "selectAll" },
    );

  const applyBulk = (paths: string[]) => {
    if (bulkCategory === "") {
      setBulkError("Выберите категорию");
      return;
    }
    const category = bulkCategory;
    void run(
      { kind: "bulk" },
      async () => {
        const reply = await setProgramsCategory(paths, category);
        if (!alive.current) return null;
        applyReply(reply.programs);
        setSelected(new Set());
        setBulkCategory("");
        lastOp.current = { kind: "bulk" };
        sayDecided("list", reply, "");
        return { to: "selectAll" };
      },
      { to: "selectAll" },
    );
  };

  const undo = (undoId: string, where: Feedback["where"]) =>
    void run(
      { kind: "undo" },
      async () => {
        const reply = await undoProgram(undoId);
        if (!alive.current) return null;
        applyReply(reply.programs);
        say(where, reply.message);
        const op = lastOp.current;
        lastOp.current = null;
        return op?.kind === "one" ? { to: "path", path: op.path } : { to: "selectAll" };
      },
      null,
    );

  const resetSearch = () => {
    setQuery("");
    setFilter("all");
  };

  const hiddenView = filter === "hidden";
  // «Программ пока нет» только когда нет ни видимых, ни скрытых: иначе пропал бы фильтр «Скрытые».
  const list = data ? data.programs.filter((p) => p.category !== "system") : [];
  const shown = data ? visiblePrograms(data.programs, { query, filter, sort }) : [];
  const pending = data?.pending ?? [];
  const selectedShown = shown.filter((p) => selected.has(p.path));
  const allShownSelected = shown.length > 0 && selectedShown.length === shown.length;
  const someShownSelected = selectedShown.length > 0 && !allShownSelected;

  const toggleAll = () =>
    setSelected((prev) => {
      const next = new Set(prev);
      for (const p of shown) {
        if (allShownSelected) next.delete(p.path);
        else next.add(p.path);
      }
      return next;
    });

  const toggleOne = (path: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });

  const sortBy = (key: ProgramSort["key"]) =>
    setSort((s) =>
      s.key === key ? { key, dir: s.dir === "asc" ? "desc" : "asc" } : { key, dir: key === "week" ? "desc" : "asc" },
    );

  const ariaSort = (key: ProgramSort["key"]) =>
    sort.key === key ? (sort.dir === "asc" ? "ascending" : "descending") : "none";

  /** Одно сообщение на экране: там, где было действие. */
  const message = (where: Feedback["where"]) =>
    feedback && feedback.where === where ? (
      <UndoLine
        key={feedback.seq}
        className="programs-feedback"
        text={feedback.text}
        error={feedback.error}
        undoMs={feedback.undo?.ms}
        disabled={busy !== null}
        onUndo={feedback.undo ? () => undo(feedback.undo!.undoId, where) : undefined}
      />
    ) : null;

  const now = Date.now();
  const tz = localOffsetMin();

  return (
    <section ref={root} aria-labelledby="programs-title" className="programs-page">
      <div className="page-head">
        <h2 id="programs-title" className="panel-title">
          Программы
        </h2>
      </div>

      {message("top")}

      {loadError ? (
        <div className="panel">
          <Notice tone="warn" urgent>
            {loadError}
          </Notice>
          <button type="button" className="btn" onClick={() => void load()}>
            Повторить
          </button>
        </div>
      ) : data === null ? (
        <p className="panel-text">Загружаю данные</p>
      ) : (
        <>
          {pending.length > 0 ? (
            <section className="programs-block" aria-labelledby="programs-pending-title">
              <h3 id="programs-pending-title" className="section-title">
                {`Ждут решения: ${pending.length}`}
              </h3>
              <p className="programs-lead">Решение действует для всех детей</p>
              <ul className="pending-list">
                {pending.map((p, i) => (
                  <PendingItem
                    key={p.path}
                    program={p}
                    now={now}
                    tz={tz}
                    busy={busy}
                    onDecide={(c) => decide(p, c, i)}
                  />
                ))}
              </ul>
            </section>
          ) : null}

          <section className="programs-block" aria-labelledby="programs-all-title">
            <h3 id="programs-all-title" className="section-title" tabIndex={-1}>
              Все программы
            </h3>
            {list.length === 0 ? (
              <p className="panel-text">Программ пока нет: они появятся, когда ребёнок запустит что-то новое</p>
            ) : (
              <>
                <div className="programs-tools">
                  <div className="field programs-search">
                    <label className="field-label" htmlFor="programs-search">
                      Поиск по названию или папке
                    </label>
                    <input
                      id="programs-search"
                      type="search"
                      className="field-input"
                      value={query}
                      onChange={(e) => setQuery(e.target.value)}
                    />
                  </div>
                  <div className="programs-filter" role="group" aria-label="Показать">
                    {FILTERS.map((f) => (
                      <button
                        key={f.id}
                        type="button"
                        className="tab"
                        aria-pressed={filter === f.id}
                        onClick={() => setFilter(f.id)}
                      >
                        {f.label}
                      </button>
                    ))}
                  </div>
                </div>

                {message("list")}
                {shown.length === 0 && hiddenView && query.trim() === "" ? (
                  <div className="programs-empty">
                    <p className="panel-text">Скрытых программ нет</p>
                  </div>
                ) : shown.length === 0 ? (
                  <div className="programs-empty">
                    <p className="panel-text">Ничего не найдено</p>
                    <button type="button" className="btn" onClick={resetSearch}>
                      Сбросить поиск
                    </button>
                  </div>
                ) : (
                  <>
                    <div className="bulk-bar">
                      <label className="switch">
                        <input
                          type="checkbox"
                          data-focus="select-all"
                          checked={allShownSelected}
                          ref={(el) => {
                            if (el) el.indeterminate = someShownSelected;
                          }}
                          disabled={busy !== null}
                          onChange={toggleAll}
                        />
                        Выбрать всё в списке
                      </label>
                      {selectedShown.length > 0 ? (
                        <div className="bulk-actions">
                          <span className="bulk-count">{`Выбрано: ${selectedShown.length}`}</span>
                          {hiddenView ? null : (
                            <>
                              <select
                                className="field-input bulk-select"
                                aria-label="Новая категория для отмеченных"
                                aria-invalid={bulkError ? true : undefined}
                                aria-describedby={bulkError ? "bulk-error" : undefined}
                                value={bulkCategory}
                                disabled={busy !== null}
                                onChange={(e) => {
                                  setBulkCategory(e.target.value as ParentCategory | "");
                                  setBulkError("");
                                }}
                              >
                                <option value="">Категория…</option>
                                {CATEGORY_OPTIONS.map((o) => (
                                  <option key={o.category} value={o.category}>
                                    {o.label}
                                  </option>
                                ))}
                              </select>
                              <button
                                type="button"
                                className="btn btn-primary"
                                aria-disabled={busy !== null || undefined}
                                onClick={() => (busy ? undefined : applyBulk(selectedShown.map((p) => p.path)))}
                              >
                                {busy?.kind === "bulk" && !busy.hide ? "Сохраняю…" : "Применить"}
                              </button>
                            </>
                          )}
                          <button
                            type="button"
                            className="btn"
                            aria-label={hiddenView ? "Показать отмеченные" : "Скрыть отмеченные"}
                            aria-disabled={busy !== null || undefined}
                            onClick={() => (busy ? undefined : hideBulk(selectedShown.map((p) => p.path), !hiddenView))}
                          >
                            {busy?.kind === "bulk" && busy.hide ? "Сохраняю…" : hiddenView ? "Показать" : "Скрыть"}
                          </button>
                          {bulkError ? (
                            <span id="bulk-error" className="field-error bulk-error">
                              {bulkError}
                            </span>
                          ) : null}
                        </div>
                      ) : null}
                    </div>

                    <table className="programs-table">
                      <thead>
                        <tr>
                          <th scope="col" className="col-check">
                            <span className="visually-hidden">Отметка</span>
                          </th>
                          <th scope="col" aria-sort={ariaSort("name")}>
                            <SortButton label="Программа" dir={sort.key === "name" ? sort.dir : null} onClick={() => sortBy("name")} />
                          </th>
                          <th scope="col" className="col-category">
                            Категория
                          </th>
                          <th scope="col" className="col-week" aria-sort={ariaSort("week")}>
                            <SortButton label="За неделю" dir={sort.key === "week" ? sort.dir : null} onClick={() => sortBy("week")} />
                          </th>
                          <th scope="col" className="col-action">
                            <span className="visually-hidden">Действие</span>
                          </th>
                        </tr>
                      </thead>
                      <tbody>
                        {shown.map((p, i) => (
                          <ProgramRow
                            key={p.path}
                            program={p}
                            checked={selected.has(p.path)}
                            busy={busy}
                            onToggle={() => toggleOne(p.path)}
                            onCategory={(c) => changeRow(p, c, i)}
                            onHide={(hidden) => hideRow(p, hidden, i)}
                          />
                        ))}
                      </tbody>
                    </table>
                  </>
                )}
              </>
            )}
          </section>
        </>
      )}
    </section>
  );
}

function placeFocus(root: HTMLElement, req: FocusTo) {
  const heading = () => root.querySelector<HTMLElement>("#programs-all-title");
  const items = Array.from(root.querySelectorAll<HTMLElement>(".pending-item"));
  const rows = Array.from(root.querySelectorAll<HTMLElement>(".programs-table tbody tr"));
  let target: HTMLElement | null = null;
  if (req.to === "pending") {
    const item = items[Math.min(req.index, items.length - 1)];
    target = item?.querySelector<HTMLElement>("button") ?? heading();
  } else if (req.to === "path") {
    const item = items.find((el) => el.dataset.path === req.path);
    const row = rows.find((el) => el.dataset.path === req.path);
    if (item) {
      target =
        (req.category && item.querySelector<HTMLElement>(`button[data-category="${req.category}"]`)) ||
        item.querySelector<HTMLElement>("button");
    } else if (row) {
      target = rowControl(row);
    } else if (req.index !== undefined && rows.length > 0) {
      target = rowControl(rows[Math.min(req.index, rows.length - 1)]);
    }
    target = target ?? heading();
  } else {
    target = root.querySelector<HTMLElement>('[data-focus="select-all"]') ?? heading();
  }
  target?.focus();
}

/** Куда фокус в строке: выбор категории, а у скрытой (выбора нет) кнопка «Показать». */
function rowControl(row: HTMLElement): HTMLElement | null {
  return row.querySelector<HTMLElement>("select") ?? row.querySelector<HTMLElement>("button");
}

/** Общий значок программы (RM3-3): окно приложения, без цвета статуса. */
function AppIcon() {
  return (
    <svg className="program-icon" width="32" height="32" viewBox="0 0 32 32" aria-hidden="true" focusable="false">
      <rect x="3" y="5" width="26" height="22" rx="3" fill="none" stroke="currentColor" strokeWidth="2" />
      <path d="M3 11h26" stroke="currentColor" strokeWidth="2" />
      <circle cx="7.5" cy="8" r="1.2" fill="currentColor" />
      <circle cx="11.5" cy="8" r="1.2" fill="currentColor" />
    </svg>
  );
}

/** Делит показанный путь на начало и «\имя файла» (имя файла не разрывается). */
function splitPath(shown: string): [string, string] {
  const at = shown.lastIndexOf("\\");
  return at > 0 ? [shown.slice(0, at), shown.slice(at)] : [shown, ""];
}

/**
 * Путь одной строкой: длинный обрезан по центру одним помощником (fitPath): не длиннее max знаков
 * и не шире строки, многоточие одно, имя файла целиком. Ширина меряется временным зондом после
 * отрисовки и при смене размера окна; без раскладки (тесты) — вид на max знаков. Полный путь в
 * подсказке, у диктора и по фокусу клавиатурой.
 */
function PathText({ path, max }: { path: string; max: number }) {
  const [shown, setShown] = useState(() => shortenPath(path, max));
  const box = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const measure = () => {
      if (el.clientWidth === 0) {
        setShown(shortenPath(path, max));
        return;
      }
      // Зонд вне разметки React (её узлы не трогаются): тот же шрифт, одна строка; убирается сразу.
      const probe = document.createElement("span");
      probe.className = "path-probe";
      el.appendChild(probe);
      const room = el.clientWidth + 0.5;
      const fits = (candidate: string) => {
        probe.textContent = candidate;
        return probe.getBoundingClientRect().width <= room;
      };
      try {
        setShown(fitPath(path, max, fits));
      } finally {
        probe.remove();
      }
    };
    measure();
    const ro = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(measure);
    ro?.observe(el);
    void document.fonts?.ready.then(measure);
    return () => ro?.disconnect();
  }, [path, max]);
  const cut = shown !== path;
  const [head, tail] = splitPath(shown);
  return (
    <span className="program-path" title={path} tabIndex={cut ? 0 : undefined}>
      <span className="path-short" aria-hidden="true" ref={box}>
        <span className="path-head">{head}</span>
        <span className="path-tail">{tail}</span>
      </span>
      <span className="path-full">{path}</span>
    </span>
  );
}

interface PendingProps {
  program: PendingProgramDTO;
  now: number;
  tz: number;
  busy: Busy | null;
  onDecide: (category: ParentCategory) => void;
}

function PendingItem({ program: p, now, tz, busy, onDecide }: PendingProps) {
  const was = wasCategoryText(p.wasCategory);
  const attempts = attemptsText(p.byChild, p.attempts, p.lastAttemptAt, now, tz);
  const description = p.description.trim();
  return (
    <li className="pending-item" aria-label={p.name} data-path={p.path}>
      <AppIcon />
      <div className="pending-body">
        <p className="program-name">{p.name}</p>
        <p className="program-meta">{publisherText(p.publisher)}</p>
        {description && description !== p.name ? <p className="program-meta">{description}</p> : null}
        <p className="program-meta">
          <PathText path={p.path} max={PENDING_PATH_MAX} />
        </p>
        {attempts ? <p className="program-meta">{attempts}</p> : null}
        {was ? <p className="pending-was">{was}</p> : null}
        <div className="pending-actions" role="group" aria-label={`Решение для «${p.name}»`}>
          {DECISIONS.map((d) => {
            const pressed = busy?.kind === "decide" && busy.path === p.path && busy.category === d.category;
            return (
              <button
                key={d.category}
                type="button"
                data-category={d.category}
                className="btn"
                // Нажатая кнопка остаётся в фокусе и говорит «Сохраняю…», остальные недоступны.
                aria-disabled={pressed || undefined}
                disabled={busy !== null && !pressed}
                onClick={() => (busy ? undefined : onDecide(d.category))}
              >
                {pressed ? "Сохраняю…" : d.label}
              </button>
            );
          })}
        </div>
      </div>
    </li>
  );
}

function SortButton({ label, dir, onClick }: { label: string; dir: "asc" | "desc" | null; onClick: () => void }) {
  return (
    <button type="button" className="sort-button" onClick={onClick}>
      {label}
      <span className="sort-mark" aria-hidden="true">
        {dir === "asc" ? "↑" : dir === "desc" ? "↓" : ""}
      </span>
    </button>
  );
}

interface RowProps {
  program: ProgramDTO;
  checked: boolean;
  busy: Busy | null;
  onToggle: () => void;
  onCategory: (category: ParentCategory) => void;
  /** «Скрыть» (true) или «Показать» (false), D-57. */
  onHide: (hidden: boolean) => void;
}

function ProgramRow({ program: p, checked, busy, onToggle, onCategory, onHide }: RowProps) {
  const saving = busy?.kind === "row" && busy.path === p.path;
  const hidden = p.hidden === true;
  return (
    <tr aria-label={p.name} data-path={p.path}>
      <td className="col-check">
        <input type="checkbox" aria-label={`Отметить «${p.name}»`} checked={checked} disabled={busy !== null} onChange={onToggle} />
      </td>
      <td>
        <p className="program-name">{p.name}</p>
        <p className="program-meta">{publisherText(p.publisher)}</p>
        <p className="program-meta">
          <PathText path={p.path} max={ROW_PATH_MAX} />
        </p>
      </td>
      <td className="col-category">
        {hidden ? (
          <p className="program-category">{categoryTitle(p.category)}</p>
        ) : (
          <select
            className="field-input"
            aria-label={`Категория для «${p.name}»`}
            value={p.category}
            disabled={busy !== null}
            onChange={(e) => onCategory(e.target.value as ParentCategory)}
          >
            {CATEGORY_OPTIONS.map((o) => (
              <option key={o.category} value={o.category}>
                {o.label}
              </option>
            ))}
          </select>
        )}
        {saving ? <span className="program-saving">Сохраняю…</span> : null}
      </td>
      <td className="col-week">{hidden ? "" : weekText(p.weekS)}</td>
      <td className="col-action">
        <button
          type="button"
          className="btn btn-small"
          aria-label={hidden ? `Показать «${p.name}»` : `Скрыть «${p.name}»`}
          disabled={busy !== null}
          onClick={() => onHide(!hidden)}
        >
          {hidden ? "Показать" : "Скрыть"}
        </button>
      </td>
    </tr>
  );
}
