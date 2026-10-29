import { useCallback, useEffect, useState } from "react";
import { finish, hello, login, logout, mode as readMode, sessionState, version as readVersion } from "./api";
import Notice from "./components/Notice";
import { SaveBarSlotContext } from "./components/SaveBar";
import { UnsavedGuardProvider, useAttemptNavigation } from "./guard";
import WhatsNew from "./whatsnew/WhatsNew";
import { errorText } from "./lib/errors";
import { SESSION_WARNING_TEXT } from "./session";
import Children from "./screens/Children";
import FirstRun from "./screens/FirstRun";
import Journal from "./screens/Journal";
import LoginDialog from "./screens/LoginDialog";
import MyStats from "./screens/MyStats";
import ParentStats from "./screens/ParentStats";
import ResetPassword from "./screens/ResetPassword";
import Bonus from "./screens/Bonus";
import Settings from "./screens/Settings";
import Today from "./screens/Today";
import UninstallGate from "./screens/UninstallGate";
import Wizard from "./screens/Wizard";
import { useSessionWatcher } from "./useSessionWatcher";

type Phase = "wizard" | "login" | "parent";

type Boot =
  | { kind: "loading" }
  | { kind: "failed"; text: string }
  | { kind: "simple"; mode: "reset-password" | "uninstall-gate" | "mystats" | "firstrun" }
  | { kind: "ready"; phase: Phase };

const SIMPLE_MODES = ["reset-password", "uninstall-gate", "mystats", "firstrun"] as const;

/** Что показать при запуске: по ключу запуска (режим окна) и состоянию программы. */
async function bootstrap(): Promise<Boot> {
  const mode = await readMode();
  const simple = SIMPLE_MODES.find((m) => m === mode);
  if (simple) {
    return { kind: "simple", mode: simple };
  }
  const state = await hello();
  if (!state.configured) {
    return { kind: "ready", phase: "wizard" };
  }
  if (mode === "parent") {
    // Окно открыто из родительской сессии (страж после верного пароля): вход не нужен.
    const session = await sessionState().catch(() => ({ active: false }));
    if (session.active) {
      return { kind: "ready", phase: "parent" };
    }
  }
  return { kind: "ready", phase: "login" };
}

type VersionState = { kind: "loading" } | { kind: "ready"; value: string } | { kind: "failed" };

/** Заголовок окна. В окнах ребёнка слов «Родительский контроль» нет: он видит только название и версию. */
function Topbar({ onLogout, childWindow = false }: { onLogout?: () => void; childWindow?: boolean }) {
  const [version, setVersion] = useState<VersionState>({ kind: "loading" });
  useEffect(() => {
    let alive = true;
    readVersion().then(
      (value) => alive && setVersion({ kind: "ready", value }),
      () => alive && setVersion({ kind: "failed" }),
    );
    return () => {
      alive = false;
    };
  }, []);
  return (
    <header className="topbar">
      <h1 className="product">PC Block</h1>
      {childWindow ? null : <span className="product-note">Родительский контроль</span>}
      <span className="topbar-spacer" />
      <span className="product-version">
        {version.kind === "loading" && "Узнаю версию"}
        {version.kind === "ready" && `Версия ${version.value}`}
        {version.kind === "failed" && "Версию узнать не удалось"}
      </span>
      {onLogout ? (
        <button type="button" className="btn btn-small" onClick={onLogout}>
          Выйти
        </button>
      ) : null}
    </header>
  );
}

export default function App() {
  const [boot, setBoot] = useState<Boot>({ kind: "loading" });
  const [phase, setPhase] = useState<Phase | null>(null);

  const start = useCallback(async () => {
    setBoot({ kind: "loading" });
    try {
      const next = await bootstrap();
      setBoot(next);
      setPhase(next.kind === "ready" ? next.phase : null);
    } catch (e) {
      setBoot({ kind: "failed", text: errorText(e) });
    }
  }, []);

  useEffect(() => {
    void start();
  }, [start]);

  // После мастера пароль только что задан: входим сами, а если не вышло, просим ввести.
  const openSettingsAfterWizard = async (password: string) => {
    try {
      await login(password);
      setPhase("parent");
    } catch {
      setPhase("login");
    }
  };

  let body;
  if (boot.kind === "loading") {
    body = <p className="panel-text">Запускаю</p>;
  } else if (boot.kind === "failed") {
    body = (
      <section className="panel">
        <Notice tone="warn" urgent>
          {boot.text}
        </Notice>
        <button type="button" className="btn" onClick={() => void start()}>
          Повторить
        </button>
      </section>
    );
  } else if (boot.kind === "simple") {
    body = <SimpleScreen mode={boot.mode} />;
  } else if (phase === "wizard") {
    body = <Wizard onOpenSettings={(pw) => void openSettingsAfterWizard(pw)} />;
  } else if (phase === "login") {
    body = <LoginDialog onSuccess={() => setPhase("parent")} />;
  } else if (phase === "parent") {
    return (
      <ParentRoot
        onLoggedOut={() => {
          setPhase("login");
        }}
      />
    );
  }

  const childWindow = boot.kind === "simple" && (boot.mode === "mystats" || boot.mode === "firstrun");
  return (
    <div className={childWindow ? "shell shell-child" : "shell"}>
      <Topbar childWindow={childWindow} />
      <div className="content-frame">
        <main
          className="content content-centered"
          tabIndex={childWindow ? -1 : undefined}
          ref={childWindow ? focusScroller : undefined}
        >
          {body}
        </main>
      </div>
    </div>
  );
}

/** Окно ребёнка: страница прокручивается внутри, фокус на ней, чтобы PageDown и End работали сразу. */
function focusScroller(el: HTMLElement | null) {
  el?.focus({ preventScroll: true });
}

function SimpleScreen({ mode }: { mode: "reset-password" | "uninstall-gate" | "mystats" | "firstrun" }) {
  switch (mode) {
    case "reset-password":
      return <ResetPassword />;
    case "uninstall-gate":
      // Установщик ждёт код выхода окна: 0 удалить и оставить статистику, 2 удалить вместе с ней (FR-4).
      return <UninstallGate onDecision={(d) => void finish(d.deleteData ? 2 : 0)} />;
    case "mystats":
      return <MyStats />;
    case "firstrun":
      return <FirstRun />;
  }
}

type Screen = "today" | "children" | "bonus" | "stats" | "journal" | "settings" | "whatsnew";

/** Основные разделы меню; «Что нового» стоит отдельно внизу (ТЗ 8.3.2 п.2). */
const MENU: { id: Screen; label: string }[] = [
  { id: "today", label: "Сегодня" },
  { id: "children", label: "Дети" },
  { id: "bonus", label: "Копилка" },
  { id: "stats", label: "Статистика" },
  { id: "journal", label: "Журнал" },
  { id: "settings", label: "Настройки" },
];

type Go = (next: Screen, opts?: { violations?: boolean; childId?: number }) => void;

/** Окно родителя целиком: переходы между разделами идут через охрану несохранённых правок. */
function ParentRoot({ onLoggedOut }: { onLoggedOut: () => void }) {
  return (
    <UnsavedGuardProvider>
      <ParentWindow onLoggedOut={onLoggedOut} />
    </UnsavedGuardProvider>
  );
}

/** Окно родителя: меню слева, раздел справа, родительская сессия под присмотром (R19). */
function ParentWindow({ onLoggedOut }: { onLoggedOut: () => void }) {
  const [screen, setScreen] = useState<Screen>("today");
  const [violationsOnly, setViolationsOnly] = useState(false);
  const [childToOpen, setChildToOpen] = useState<number | undefined>(undefined);
  const [saveSlot, setSaveSlot] = useState<HTMLElement | null>(null);
  const attempt = useAttemptNavigation();
  const { view, extend, restart } = useSessionWatcher();

  const go: Go = (next, opts = {}) =>
    attempt(() => {
      setViolationsOnly(Boolean(opts.violations));
      setChildToOpen(opts.childId);
      setScreen(next);
    });

  const leave = () =>
    attempt(() => {
      // Не вышло закрыть сессию (нет связи): окно всё равно просит пароль, сессия сама кончится по таймеру.
      void logout()
        .catch(() => undefined)
        .finally(onLoggedOut);
    });

  const ended = view.phase === "ended";

  return (
    <SaveBarSlotContext.Provider value={saveSlot}>
      <div className="shell">
        <Topbar onLogout={leave} />
        <div className="shell-body" inert={ended}>
          <nav className="menu" aria-label="Разделы">
            <ul className="menu-list">
              {MENU.map((item) => (
                <li key={item.id}>
                  <button
                    type="button"
                    className="menu-item"
                    aria-current={screen === item.id ? "page" : undefined}
                    onClick={() => go(item.id)}
                  >
                    {item.label}
                  </button>
                </li>
              ))}
            </ul>
            <button
              type="button"
              className="menu-item menu-foot"
              aria-current={screen === "whatsnew" ? "page" : undefined}
              onClick={() => go("whatsnew")}
            >
              Что нового
            </button>
          </nav>
          <div className="main-col">
            <main className="content">
              {view.phase === "warning" ? (
                <div className="session-warning" role="alert">
                  <span>{SESSION_WARNING_TEXT}</span>
                  <button type="button" className="btn btn-small" onClick={() => void extend()}>
                    Продолжить
                  </button>
                </div>
              ) : null}
              {screen === "today" ? (
                <Today
                  onOpenJournal={(o) => go("journal", o)}
                  onOpenSettings={() => go("settings")}
                  onOpenChildren={() => go("children")}
                  onOpenStats={(childId) => go("stats", { childId })}
                />
              ) : null}
              {screen === "children" ? (
                <Children onOpenSettings={() => go("settings")} />
              ) : null}
              {screen === "bonus" ? <Bonus /> : null}
              {screen === "stats" ? <ParentStats key={childToOpen} initialChildId={childToOpen} /> : null}
              {screen === "journal" ? <Journal key={String(violationsOnly)} violationsOnly={violationsOnly} /> : null}
              {screen === "settings" ? <Settings onOpenChildren={() => go("children")} /> : null}
              {screen === "whatsnew" ? <WhatsNew /> : null}
            </main>
            <div className="savebar-slot" ref={setSaveSlot} />
          </div>
        </div>
        {ended ? (
          <div className="overlay" role="dialog" aria-modal="true" aria-label="Сессия закончилась">
            <LoginDialog
              lead="Сессия закончилась. Введите пароль, чтобы продолжить: несохранённые правки на месте."
              onSuccess={restart}
            />
          </div>
        ) : null}
      </div>
    </SaveBarSlotContext.Provider>
  );
}
