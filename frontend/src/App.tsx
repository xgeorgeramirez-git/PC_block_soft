import { useEffect, useState } from "react";
import { getVersion } from "./api";

type VersionState = { kind: "loading" } | { kind: "ready"; value: string } | { kind: "failed" };

export default function App() {
  const [version, setVersion] = useState<VersionState>({ kind: "loading" });

  useEffect(() => {
    let active = true;
    getVersion()
      .then((value) => active && setVersion({ kind: "ready", value }))
      .catch(() => active && setVersion({ kind: "failed" }));
    return () => {
      active = false;
    };
  }, []);

  return (
    <div className="shell">
      <header className="topbar">
        <h1 className="product">PC Block</h1>
        <span className="product-note">Родительский контроль</span>
      </header>
      <main className="content">
        <section className="panel" aria-labelledby="status-title">
          <h2 id="status-title" className="panel-title">
            Программа в разработке
          </h2>
          <p className="panel-text">
            Это первая сборка: окно открывается, установщик собирается. Лимиты времени и экран
            блокировки появятся в версии 0.1.
          </p>
          <dl className="facts">
            <div className="fact">
              <dt>Сборка</dt>
              <dd>
                {version.kind === "loading" && "Узнаю версию"}
                {version.kind === "ready" && `Версия ${version.value}`}
                {version.kind === "failed" && "Версию узнать не удалось"}
              </dd>
            </div>
            <div className="fact">
              <dt>Данные</dt>
              <dd>Хранятся только на этом компьютере</dd>
            </div>
          </dl>
        </section>
      </main>
    </div>
  );
}
