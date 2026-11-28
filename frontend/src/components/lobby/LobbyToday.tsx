import { useEffect, useRef, type CSSProperties } from "react";
import { energyColors, MASCOT_SRC, mascotOf } from "../../lib/mascot";
import { spendBlockedNote } from "../../lib/mytoday";
import { formatLimit } from "../../lib/format";
import type { ChildStatus, MyTodayReply } from "../../types";
import { Badge, Coin, Glyph, type GlyphName } from "./Glyph";
import { dateLine, greeting, mainShare, nightLine, remainBig, spentLines, ticksOf, tileGames, tileVideo, violationsLine, type Tile } from "./lobbyText";

interface Props {
  data: MyTodayReply;
  /** «Сейчас» для даты в шапке. */
  now: Date;
  onAsk: () => void;
  onSpend: () => void;
  onCode: () => void;
  onAll: () => void;
  /** Возврат из «Вся моя статистика»: при показе фокус на эту кнопку (кнопка «← Сегодня» пропала). */
  focusAll?: boolean;
  onParent: () => void;
}

/**
 * Блок «Сегодня» окна ребёнка «Игровое лобби» (ТЗ 8.2.6 п. 1, D-109, макет child.py): шапка с приветом,
 * копилкой и «Ввести код»; главный блок с талисманом, остатком и шкалой-энергией; плитки игр, видео и
 * учёбы; запретные часы; «Взять из копилки» и «Попросить ещё»; нарушения и ссылки. Только просмотр:
 * изменить статистику и правила ребёнок не может.
 */
export default function LobbyToday({ data, now, onAsk, onSpend, onCode, onAll, focusAll = false, onParent }: Props) {
  const all = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (focusAll) all.current?.focus();
  }, [focusAll]);
  const s = data.status;
  const child = s.status !== "notChild";
  const letter = s.name.trim().charAt(0).toUpperCase();
  return (
    <div className="lb-today">
      <header className="lb-head">
        <div className="lb-hello">
          {letter ? (
            <span className="lb-avatar lb-big" aria-hidden="true">
              {letter}
            </span>
          ) : null}
          <div className="lb-hello-text">
            <h2 className="lb-big lb-title">{greeting(s.name)}</h2>
            <p className="lb-date">{dateLine(now)}</p>
          </div>
        </div>
        {child ? (
          <div className="lb-head-side">
            <p className="lb-coins">
              <Coin size={38} />
              <span className="lb-coins-text">
                <span className="lb-coins-label">В копилке</span>
                <span className="lb-big lb-coins-value num">{formatLimit(s.bonusBalance)}</span>
              </span>
            </p>
            <button type="button" className="lb-btn lb-btn-blue lb-btn-code" onClick={onCode}>
              <Glyph name="key" size={22} />
              Ввести код
            </button>
          </div>
        ) : null}
      </header>

      {child ? (
        <>
          <Main s={s} />
          <div className="lb-tiles">
            <LimitTile icon="game" bg="#FF6A4A" title="Игры" tile={tileGames(s)} color="#FFB199" bar={["#FFC2A8", "#FF6A4A"]} />
            <LimitTile icon="video" bg="#3D86FF" title="Видео и соцсети" tile={tileVideo(s)} color="#A9CCFF" bar={["#B8D6FF", "#3D86FF"]} />
            <LimitTile
              icon="book"
              bg="#22B573"
              title="Учёба"
              tile={{ big: "Без лимита", share: null, free: "Занимайся сколько хочешь", sub: "Учёба не тратит твоё время" }}
              color="#9FF0C4"
              bar={["", ""]}
            />
          </div>
          <Actions s={s} onAsk={onAsk} onSpend={onSpend} />
        </>
      ) : (
        <section className="lb-panel lb-plain" aria-label="Сегодня">
          <p>Эта учётная запись не детская: здесь нет ограничений и правил.</p>
        </section>
      )}

      <footer className="lb-foot">
        {child ? (
          <p className="lb-violations">
            {data.violationsToday > 0 ? (
              <Badge name="alert" bg="#FF6A4A" size={28} glyph={16} />
            ) : (
              <Badge name="check" bg="#22B573" size={28} glyph={16} />
            )}
            {violationsLine(data.violationsToday)}
          </p>
        ) : (
          <span />
        )}
        <div className="lb-links">
          <button ref={all} type="button" className="lb-link lb-link-main" onClick={onAll}>
            Вся моя статистика
          </button>
          <button type="button" className="lb-link" onClick={onParent}>
            Настройки для родителя
          </button>
        </div>
      </footer>
    </div>
  );
}

/** Главный блок: талисман, «Осталось сегодня», крупное число, шкала-энергия с делениями по 15 мин. */
function Main({ s }: { s: ChildStatus }) {
  const mascot = mascotOf(s);
  const [a, b] = energyColors(mascot);
  const ticks = ticksOf(s);
  const share = mainShare(s);
  const spent = spentLines(s);
  const big = remainBig(s);
  // Слова вместо числа («Сейчас нельзя», «Пауза до 18:30») длиннее «1 ч 25 мин»: кегль меньше, чтобы
  // «потратил … из …» стояло справа в той же строке, как во всех состояниях.
  const word = !/^\d/.test(big);
  return (
    <section className="lb-panel lb-main" aria-labelledby="lb-left">
      <img className="lb-mascot" src={MASCOT_SRC[mascot]} data-mascot={mascot} alt="" width={210} height={210} draggable={false} />
      <div className="lb-main-row">
        <div className="lb-main-left">
          <h3 id="lb-left" className="lb-left-label">
            Осталось сегодня
          </h3>
          <p className={word ? "lb-big lb-left-value lb-left-word" : "lb-big lb-left-value num"}>{big}</p>
        </div>
        {spent.length > 0 ? (
          <p className="lb-spent num">
            {spent.map((line, i) => (
              <span key={line} className="lb-spent-line">
                {i > 0 ? " " : null}
                {line}
              </span>
            ))}
          </p>
        ) : null}
      </div>
      <Energy share={share} ticks={ticks} a={a} b={b} height={40} />
      {/* Под пустой шкалой (048, 104) объяснять деления незачем. */}
      {ticks > 0 && share > 0 ? <p className="lb-tick-note">Одно деление: 15 минут</p> : null}
    </section>
  );
}

/** Шкала-энергия (kit.py bar): дорожка с обводкой, заливка градиентом, деления поверх. Смысл — в тексте рядом. */
function Energy({ share, ticks, a, b, height }: { share: number; ticks: number; a: string; b: string; height: number }) {
  const pct = Math.round(Math.min(1, Math.max(0, share)) * 1000) / 10;
  return (
    <div className="lb-bar" style={{ height, borderRadius: height } as CSSProperties} aria-hidden="true">
      <div className="lb-bar-fill" style={{ width: `${pct}%`, borderRadius: height, background: `linear-gradient(180deg, ${a} 0%, ${b} 60%)` }} />
      {ticks > 0 ? <div className="lb-bar-ticks" style={{ borderRadius: height, "--lb-ticks": ticks } as CSSProperties} /> : null}
    </div>
  );
}

function LimitTile({
  icon,
  bg,
  title,
  tile,
  color,
  bar,
}: {
  icon: GlyphName;
  bg: string;
  title: string;
  tile: Tile;
  color: string;
  bar: [string, string];
}) {
  const id = `lb-tile-${icon}`;
  return (
    <section className="lb-panel lb-tile" aria-labelledby={id}>
      <div className="lb-tile-head">
        <Badge name={icon} bg={bg} size={40} glyph={22} />
        <h3 id={id} className="lb-tile-title">
          {title}
        </h3>
      </div>
      <p className="lb-big lb-tile-value num" style={{ color }}>
        {tile.big}
      </p>
      {tile.share !== null ? <Energy share={tile.share} ticks={0} a={bar[0]} b={bar[1]} height={22} /> : <p className="lb-tile-free">{tile.free ?? ""}</p>}
      <p className="lb-tile-sub">{tile.sub}</p>
    </section>
  );
}

/** Запретные часы и две главные кнопки; недоступная «Взять из копилки» объясняет почему (L-17). */
function Actions({ s, onAsk, onSpend }: { s: ChildStatus; onAsk: () => void; onSpend: () => void }) {
  const night = nightLine(s);
  const note = spendBlockedNote(s, s.bonusBalance);
  return (
    <>
      <div className="lb-row">
        {night ? (
          <section className="lb-panel lb-night" aria-labelledby="lb-night-title">
            <Badge name="moon" bg="#7B6CFF" size={46} glyph={26} />
            <div className="lb-night-text">
              <h3 id="lb-night-title" className="lb-night-title">
                Ночью компьютер спит
              </h3>
              <p className="lb-big lb-night-value num">{night}</p>
            </div>
          </section>
        ) : (
          <span className="lb-row-fill" />
        )}
        <div className="lb-actions">
          <div className="lb-actions-row">
            <button
              type="button"
              className="lb-btn lb-btn-green lb-btn-main"
              aria-disabled={note ? "true" : undefined}
              aria-describedby={note ? "spend-note" : undefined}
              onClick={note ? undefined : onSpend}
            >
              <Coin size={34} />
              Взять из копилки
            </button>
            <button type="button" className="lb-btn lb-btn-gold lb-btn-main" onClick={onAsk}>
              Попросить ещё
            </button>
          </div>
          {note ? (
            <p id="spend-note" className="lb-note" role="note">
              {note}
            </p>
          ) : null}
        </div>
      </div>
    </>
  );
}
