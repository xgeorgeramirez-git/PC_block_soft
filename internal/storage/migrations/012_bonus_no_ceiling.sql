-- Назначение: раунд 0.8.4 (D-113): у копилки нет предела баланса — CHECK (balance_after >= 0) вместо 0…600.
-- SQLite не меняет CHECK у существующей таблицы: новая таблица, перенос строк с теми же id, замена.
-- Индексов у bonus_ledger нет (001–011), внешних ключей на неё нет. Старые миграции не трогаются (L-131).
CREATE TABLE bonus_ledger_new (
  id INTEGER PRIMARY KEY,
  child_id INTEGER NOT NULL REFERENCES children(id),
  at INTEGER NOT NULL,
  delta_min INTEGER NOT NULL,                        -- +начисление, -трата
  reason TEXT NOT NULL,
  balance_after INTEGER NOT NULL CHECK (balance_after >= 0)
);
INSERT INTO bonus_ledger_new (id, child_id, at, delta_min, reason, balance_after)
  SELECT id, child_id, at, delta_min, reason, balance_after FROM bonus_ledger;
DROP TABLE bonus_ledger;
ALTER TABLE bonus_ledger_new RENAME TO bonus_ledger;
