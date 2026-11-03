-- Назначение: фидбек-раунд 0.3.2 (FB-14, D-62): перерыв на отдых (R27, FR-37).
-- Правки схемы идут новой миграцией, 001–007 не переписываются (L-131).

-- break_rules: настройки «Отдых» ребёнка (FR-37); строки нет — настройки по умолчанию, перерывы
-- выключены. message NULL — стандартный текст экрана перерыва.
CREATE TABLE break_rules (
  child_id INTEGER PRIMARY KEY REFERENCES children(id) ON DELETE CASCADE,
  enabled INTEGER NOT NULL DEFAULT 0,
  after_min INTEGER NOT NULL DEFAULT 60 CHECK (after_min BETWEEN 15 AND 240),
  counts TEXT NOT NULL DEFAULT 'all' CHECK (counts IN ('all','games')),
  warn_min INTEGER NOT NULL DEFAULT 5 CHECK (warn_min BETWEEN 1 AND 15),
  rest_min INTEGER NOT NULL DEFAULT 15 CHECK (rest_min BETWEEN 5 AND 60),
  self_rest_min INTEGER NOT NULL DEFAULT 15 CHECK (self_rest_min BETWEEN 5 AND 120 AND self_rest_min >= rest_min),
  message TEXT CHECK (message IS NULL OR length(message) BETWEEN 1 AND 120)
);

-- break_state: счёт к перерыву (не по дням: идёт и через полночь). Моменты — UTC мс.
-- quiet_s — тишина, накопленная до паузы; quiet_since — с какого момента нет засчитанных секунд;
-- warned и break_at — плашка показана и момент начала перерыва по часам (Ruling FB14-1: начало
-- перерыва идёт по часам от плашки); rest_until — конец идущего перерыва, сохраняется сразу (RF14-2).
CREATE TABLE break_state (
  child_id INTEGER PRIMARY KEY REFERENCES children(id) ON DELETE CASCADE,
  counted_s INTEGER NOT NULL DEFAULT 0,
  quiet_s INTEGER NOT NULL DEFAULT 0,
  quiet_since INTEGER,
  warned INTEGER NOT NULL DEFAULT 0,
  break_at INTEGER,
  rest_until INTEGER
);
