-- Назначение: фидбек-раунд 0.2.2 — автобонусы (R25), снятие запрета на день (R24), промокоды (R26).
-- Правки схемы идут новой миграцией, 001 не переписывается (L-131).

-- day_state: отметка «автобонус за этот день начислен» (R25, D-49) и «запретные часы сняты на сегодня» (R24, D-51).
ALTER TABLE day_state ADD COLUMN bonus_granted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE day_state ADD COLUMN no_forbidden INTEGER NOT NULL DEFAULT 0;

-- Промокоды на время (R26, D-50, FR-35): родитель создаёт код и минуты, ребёнок вводит один раз.
CREATE TABLE promo_codes (
  id INTEGER PRIMARY KEY,
  code_norm TEXT NOT NULL UNIQUE,                    -- код без регистра и пробелов по краям (для сравнения)
  code_display TEXT NOT NULL,                        -- как ввёл родитель
  minutes INTEGER NOT NULL CHECK (minutes BETWEEN 1 AND 600),
  created_at INTEGER NOT NULL,                       -- UTC мс
  used_at INTEGER,                                   -- NULL = действующий; иначе момент использования (UTC мс)
  used_by_child INTEGER REFERENCES children(id)      -- кто использовал; NULL — ещё не использован
);
