// Раздел «Что нового» (FR-36, ТЗ 8.3.9): версии программы от новой к старой, у каждой название и
// короткие пункты. Только текст из каталога entries.ts: ни кнопок, ни переключателей, ничего не хранит.
import { WHATS_NEW } from "./entries";

/** Номер элемента для заголовка версии: «0.2.5» → «whatsnew-0-2-5». */
function headingId(version: string): string {
  return `whatsnew-${version.replace(/\./g, "-")}`;
}

export default function WhatsNew() {
  return (
    <section aria-labelledby="whatsnew-title">
      <h2 id="whatsnew-title" className="panel-title">
        Что нового
      </h2>
      <div className="panel">
        {WHATS_NEW.map((e) => (
          <section key={e.version} className="whatsnew-entry" aria-labelledby={headingId(e.version)}>
            <h3 id={headingId(e.version)} className="subsection-title">
              {`Версия ${e.version}. ${e.title}`}
            </h3>
            <ul className="whatsnew-bullets">
              {e.items.map((item) => (
                <li key={item}>{item}</li>
              ))}
            </ul>
          </section>
        ))}
      </div>
    </section>
  );
}
