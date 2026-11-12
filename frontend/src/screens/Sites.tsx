import { useCallback, useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent } from "react";
import { addSite, listSites, removeSite, setAdultFilter } from "../api";
import Notice from "../components/Notice";
import UndoLine from "../components/UndoLine";
import { errorField, errorText } from "../lib/errors";
import { NOT_DOMAIN_TEXT, SITE_HINT_TEXT, SITE_LISTS, filterSites, sitesOf, sourceDisplay } from "../lib/sites";
import type { ListSitesReply, SiteKind } from "../types";

const UNDO_MS = 10_000;
const INPUT_ID = "sites-add-input";
const PANEL_ID = "sites-panel";
const NO_CHILDREN = "Детей пока нет. Добавьте их в разделе «Настройки» → «Детские учётки»";

interface Feedback {
  seq: number;
  text: string;
  error: boolean;
  /** Где сообщение: в блоке «18+» или над списком, рядом с тем, что нажали (PL-34). */
  where: "adult" | "list";
  /** «Отменить» удаления: AddSite с доменом и прежним списком (S-15). */
  undo?: { domain: string; kind: SiteKind; ms: number };
}

/** Что сейчас в пути к службе: пока запрос идёт, остальные действия недоступны. */
type Busy = { kind: "add" } | { kind: "remove"; domain: string } | { kind: "adult"; childId: number } | { kind: "undo" };

/** Куда поставить фокус после ответа: элемент, на котором он был, мог исчезнуть. */
type FocusTo =
  | { to: "field" }
  | { to: "row"; index: number; domain?: string }
  | { to: "domain"; domain: string }
  | { to: "adult"; childId: number };

/**
 * Экран «Сайты» (FR-21, сценарий 8.3.5): переключатели «18+» по детям, три списка вкладками,
 * добавление и удаление с «Отменить». Домен разбирает и все тексты даёт служба: экран кладёт
 * списки из её ответа и ничего не считает сам (T8-1, T8-2).
 */
export default function Sites() {
  const [data, setData] = useState<ListSitesReply | null>(null);
  const [loadError, setLoadError] = useState("");
  const [busy, setBusyState] = useState<Busy | null>(null);
  const [feedback, setFeedback] = useState<Feedback | null>(null);
  const [tab, setTab] = useState<SiteKind>("blocked");
  const [query, setQuery] = useState("");
  const [draft, setDraft] = useState("");
  const [fieldError, setFieldError] = useState("");
  const alive = useRef(true);
  const seq = useRef(0);
  const busyRef = useRef(false);
  const root = useRef<HTMLElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const focusTo = useRef<FocusTo | null>(null);
  const tabs = useRef(new Map<SiteKind, HTMLButtonElement>());

  const load = useCallback(async () => {
    setLoadError("");
    try {
      const reply = await listSites();
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

  // Фокус после ответа службы: на соседнюю строку, вернувшийся сайт, переключатель или поле.
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
  const run = async (what: Busy, where: Feedback["where"], action: () => Promise<FocusTo | null>, onError: FocusTo | null) => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusyState(what);
    try {
      focusTo.current = await action();
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

  const add = () => {
    if (busyRef.current) return;
    const raw = draft.trim();
    if (raw === "") {
      // Пустое поле — без запроса, тот же текст, что у службы.
      setFieldError(NOT_DOMAIN_TEXT);
      input.current?.focus();
      return;
    }
    const kind = tab;
    void run(
      { kind: "add" },
      "list",
      async () => {
        try {
          const reply = await addSite(raw, kind);
          if (!alive.current) return null;
          setData(reply.sites);
          setDraft("");
          setFieldError("");
          say("list", reply.message);
        } catch (e) {
          if (!alive.current) return null;
          if (errorField(e) !== "domain") throw e;
          setFieldError(errorText(e));
        }
        return { to: "field" };
      },
      { to: "field" },
    );
  };

  const remove = (domain: string, index: number) =>
    void run(
      { kind: "remove", domain },
      "list",
      async () => {
        const reply = await removeSite(domain);
        if (!alive.current) return null;
        setData(reply.sites);
        // Ошибка поля относилась к прежнему вводу: рядом с «Удалён …» она сбивает с толку.
        setFieldError("");
        const left = Math.min(reply.undoUntilMs - Date.now(), UNDO_MS);
        say("list", reply.message, false, left > 0 ? { domain: reply.domain, kind: reply.kind, ms: left } : undefined);
        return { to: "row", index };
      },
      { to: "row", index, domain },
    );

  const undo = (u: { domain: string; kind: SiteKind }) =>
    void run(
      { kind: "undo" },
      "list",
      async () => {
        const reply = await addSite(u.domain, u.kind);
        if (!alive.current) return null;
        setData(reply.sites);
        say("list", reply.message);
        return { to: "domain", domain: u.domain };
      },
      { to: "field" },
    );

  const toggleAdult = (childId: number, on: boolean) =>
    void run(
      { kind: "adult", childId },
      "adult",
      async () => {
        const reply = await setAdultFilter(childId, on);
        if (!alive.current) return null;
        setData(reply.sites);
        say("adult", reply.message);
        return { to: "adult", childId };
      },
      { to: "adult", childId },
    );

  /**
   * Другая вкладка: поиск, набранный адрес, ошибка поля и сообщение списка относились к прежней.
   * Пока идёт запрос, вкладка не меняется: ответ («Удалён …», «Отменить») относится к этой.
   */
  const chooseTab = (kind: SiteKind) => {
    if (kind === tab || busyRef.current) return;
    setTab(kind);
    setQuery("");
    setDraft("");
    setFieldError("");
    setFeedback((f) => (f?.where === "list" ? null : f));
  };

  const onTabKey = (e: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const last = SITE_LISTS.length - 1;
    let next: number;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") next = index === last ? 0 : index + 1;
    else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = index === 0 ? last : index - 1;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = last;
    else return;
    e.preventDefault();
    if (busyRef.current) return;
    const kind = SITE_LISTS[next].kind;
    chooseTab(kind);
    tabs.current.get(kind)?.focus();
  };

  /**
   * Одно сообщение на экране: там, где было действие. Живая область каждого места есть в DOM до
   * сообщения, меняется только её содержимое (диктор читает и первое «Удалён …»). Ошибка - отдельно,
   * role="alert" читается при появлении.
   */
  const message = (where: Feedback["where"]) => {
    const f = feedback && feedback.where === where ? feedback : null;
    return (
      <>
        <div className="sites-live" role="status">
          {f && !f.error ? (
            <UndoLine
              key={f.seq}
              live={false}
              className="programs-feedback sites-feedback"
              text={f.text}
              undoMs={f.undo?.ms}
              disabled={busy !== null}
              onUndo={f.undo ? () => undo(f.undo!) : undefined}
            />
          ) : null}
        </div>
        {f && f.error ? <UndoLine key={f.seq} className="programs-feedback sites-feedback" text={f.text} error /> : null}
      </>
    );
  };

  const current = SITE_LISTS.find((l) => l.kind === tab) ?? SITE_LISTS[0];
  const all = data?.sites ?? [];
  const items = sitesOf(all, current.kind);
  const shown = filterSites(items, query);
  const errorId = `${INPUT_ID}-error`;
  const hintId = `${INPUT_ID}-hint`;

  return (
    <section ref={root} aria-labelledby="sites-title" className="sites-page">
      <div className="page-head">
        <h2 id="sites-title" className="panel-title">
          Сайты
        </h2>
      </div>
      <p className="programs-lead sites-intro">Программа следит за сайтами только в Brave. Другие браузеры у детей закрываются</p>

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
        <p className="panel-text">Загружаю данные…</p>
      ) : (
        <>
          <section className="programs-block sites-adult" aria-labelledby="sites-adult-title">
            <h3 id="sites-adult-title" className="section-title">
              Блокировать сайты 18+
            </h3>
            {data.children.length === 0 ? (
              <p className="panel-text">{NO_CHILDREN}</p>
            ) : (
              <div className="sites-switches" role="group" aria-labelledby="sites-adult-title">
                {data.children.map((c) => (
                  <label key={c.childId} className="switch">
                    <input
                      type="checkbox"
                      data-child={c.childId}
                      checked={c.on}
                      disabled={busy !== null}
                      onChange={() => toggleAdult(c.childId, !c.on)}
                    />
                    {c.name}
                  </label>
                ))}
              </div>
            )}
            {data.adultSource ? <p className="field-hint sites-source">{sourceDisplay(data.adultSource)}</p> : null}
            {message("adult")}
          </section>

          <div className="programs-block sites-lists">
            <div className="tabs sites-tabs" role="tablist" aria-label="Списки сайтов">
              {SITE_LISTS.map((l, i) => (
                <button
                  key={l.kind}
                  ref={(el) => {
                    if (el) tabs.current.set(l.kind, el);
                    else tabs.current.delete(l.kind);
                  }}
                  type="button"
                  role="tab"
                  id={`sites-tab-${l.kind}`}
                  className="tab"
                  aria-selected={l.kind === tab}
                  aria-controls={PANEL_ID}
                  tabIndex={l.kind === tab ? 0 : -1}
                  onClick={() => chooseTab(l.kind)}
                  onKeyDown={(e) => onTabKey(e, i)}
                >
                  {`${l.title} ${sitesOf(all, l.kind).length}`}
                </button>
              ))}
            </div>

            <div id={PANEL_ID} className="sites-panel" role="tabpanel" aria-labelledby={`sites-tab-${current.kind}`}>
              <p className="programs-lead">{current.lead}</p>
              {current.kind === "blocked" ? (
                <p className="sites-limit">{`Сайтов: ${items.length} из ${data.blockedMax}`}</p>
              ) : null}

              <form
                className={fieldError ? "sites-add field-invalid" : "sites-add"}
                noValidate
                onSubmit={(e) => {
                  e.preventDefault();
                  add();
                }}
              >
                <label className="field-label sites-add-label" htmlFor={INPUT_ID}>
                  Добавить сайт
                </label>
                <input
                  id={INPUT_ID}
                  ref={input}
                  type="text"
                  name="site"
                  className="field-input sites-add-input"
                  autoComplete="off"
                  spellCheck={false}
                  value={draft}
                  aria-invalid={fieldError ? "true" : undefined}
                  aria-describedby={fieldError ? `${errorId} ${hintId}` : hintId}
                  onChange={(e) => {
                    setDraft(e.target.value);
                    if (fieldError) setFieldError("");
                  }}
                />
                <button type="submit" className="btn btn-primary sites-add-button" aria-disabled={busy !== null || undefined}>
                  {busy?.kind === "add" ? "Сохраняю…" : "Добавить"}
                </button>
                {/* Ошибка у поля и подсказка с примером под ней: подсказка не пропадает при ошибке. */}
                {fieldError ? (
                  <p id={errorId} className="field-error sites-add-note" role="alert">
                    {fieldError}
                  </p>
                ) : null}
                <p id={hintId} className="field-hint sites-add-note">
                  {SITE_HINT_TEXT}
                </p>
              </form>

              {message("list")}

              {items.length === 0 ? (
                <p className="panel-text sites-empty">{current.empty}</p>
              ) : (
                <>
                  <div className="field sites-search">
                    <label className="field-label" htmlFor="sites-search">
                      Найти в списке
                    </label>
                    <input
                      id="sites-search"
                      type="search"
                      className="field-input"
                      autoComplete="off"
                      spellCheck={false}
                      value={query}
                      onChange={(e) => setQuery(e.target.value)}
                    />
                  </div>
                  {shown.length === 0 ? (
                    <div className="programs-empty">
                      <p className="panel-text">Ничего не найдено</p>
                      <button type="button" className="btn" onClick={() => setQuery("")}>
                        Сбросить поиск
                      </button>
                    </div>
                  ) : (
                    <ul className="sites-list" aria-label={`${current.title}: сайты`}>
                      {shown.map((s, i) => (
                        <li key={s.domain} className="sites-row" data-domain={s.domain}>
                          <span className="sites-domain">{s.domain}</span>
                          <button
                            type="button"
                            className="btn btn-small"
                            aria-label={`Удалить ${s.domain}`}
                            disabled={busy !== null}
                            onClick={() => remove(s.domain, i)}
                          >
                            Удалить
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </>
              )}
            </div>
          </div>
        </>
      )}
    </section>
  );
}

function placeFocus(root: HTMLElement, req: FocusTo) {
  const rows = Array.from(root.querySelectorAll<HTMLElement>(".sites-row"));
  const removeButton = (row: HTMLElement | undefined) => row?.querySelector<HTMLElement>("button") ?? null;
  let target: HTMLElement | null = null;
  if (req.to === "row") {
    const same = req.domain ? rows.find((r) => r.dataset.domain === req.domain) : undefined;
    target = removeButton(same ?? rows[Math.min(req.index, rows.length - 1)]);
  } else if (req.to === "domain") {
    target = removeButton(rows.find((r) => r.dataset.domain === req.domain));
  } else if (req.to === "adult") {
    target = root.querySelector<HTMLElement>(`input[data-child="${req.childId}"]`);
  }
  // Строк не осталось, сайт скрыт поиском или фокус был в поле: в поле «Добавить сайт».
  (target ?? root.querySelector<HTMLElement>(`#${INPUT_ID}`))?.focus();
}
