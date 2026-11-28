import { useEffect, useRef, type ReactNode } from "react";

interface Props {
  /** Разделы периода: «Статистика ведётся с», «Итоги» с периодами и разделами, «Мои правила». */
  children: ReactNode;
  onBack: () => void;
  onParent: () => void;
}

/**
 * «Вся моя статистика» окна ребёнка (ТЗ 8.2.6 п. 2–10) в оформлении «Игрового лобби»: кнопка «← Сегодня»,
 * заголовок и прежние разделы (периоды, «Итоги», «По дням», «Программы», «Сайты», «Нарушения»,
 * «Копилка», «Мои правила») в тёмных панелях. Только просмотр. Фокус при открытии — на «← Сегодня»:
 * ребёнок с клавиатуры сразу видит, где он и как вернуться.
 */
export default function LobbyAll({ children, onBack, onParent }: Props) {
  const back = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    back.current?.focus({ preventScroll: true });
  }, []);
  return (
    <div className="lb-all">
      <div className="lb-all-head">
        <button ref={back} type="button" className="lb-btn lb-btn-blue lb-back" aria-label="Вернуться к «Сегодня»" onClick={onBack}>
          <span aria-hidden="true">←</span> Сегодня
        </button>
        <h2 className="lb-big lb-title">Вся моя статистика</h2>
      </div>
      {children}
      <p className="lb-autobonus">В полночь в копилку приходит бонус за неизрасходованное за день время.</p>
      <footer className="lb-foot lb-foot-all">
        <span />
        <div className="lb-links">
          <button type="button" className="lb-link" onClick={onParent}>
            Настройки для родителя
          </button>
        </div>
      </footer>
    </div>
  );
}
