-- Назначение: веха M5b (FR-39, D-76, RM5b-4): решения родителя по автозагрузке учёток детей.
-- Граница «найдено при первой проверке» — не здесь: её пишет служба в settings
-- (startup.baseline.<childID>), как bonus.autobonus_from_day (D-82). Старые миграции не трогаются (L-131).

-- startup_items (ТЗ раздел 4): запись автозагрузки ребёнка. Ключ записи — scope + source + name;
-- у общей записи (shared) строка на каждого ребёнка. decision: pending — ждёт решения родителя.
CREATE TABLE startup_items (
  id INTEGER PRIMARY KEY,
  child_id INTEGER NOT NULL REFERENCES children(id),
  scope TEXT NOT NULL CHECK (scope IN ('own','shared')),
  source TEXT NOT NULL,
  name TEXT NOT NULL,
  command TEXT NOT NULL,
  publisher TEXT NOT NULL DEFAULT '',
  decision TEXT NOT NULL CHECK (decision IN ('pending','on','off')),
  first_seen INTEGER NOT NULL,
  decided_at INTEGER,
  UNIQUE (child_id, scope, source, name)
);
CREATE INDEX startup_items_by_child ON startup_items(child_id);
