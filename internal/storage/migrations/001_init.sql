-- Назначение: начальная схема базы по ТЗ 4 целиком (таблицы M1 и поля M2+ сразу, чтобы не мигрировать повторно).
-- Номер миграции берётся из имени файла; после слияния в main файл не переписывается (L-131), правки схемы идут новой миграцией.

CREATE TABLE children (
  id INTEGER PRIMARY KEY,
  windows_sid TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  end_mode TEXT NOT NULL CHECK (end_mode IN ('lock','study_only')) DEFAULT 'lock',
  adult_filter INTEGER NOT NULL DEFAULT 1,          -- список 18+ включён
  bypass_detail TEXT,                                -- блокировка за обход (R17): что произошло; NULL — нет; снимает только родитель
  removed_at INTEGER,                                -- UTC мс: отметка «детская» снята (FR-25), статистика остаётся; NULL — учётка детская
  created_at INTEGER NOT NULL
);
CREATE TABLE day_limits (                            -- по дням недели, 1=пн..7=вс
  child_id INTEGER NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  weekday INTEGER NOT NULL CHECK (weekday BETWEEN 1 AND 7),
  total_min INTEGER CHECK (total_min BETWEEN 0 AND 1440),   -- NULL = без лимита
  games_min INTEGER CHECK (games_min BETWEEN 0 AND 1440),   -- NULL = без отдельного лимита
  video_min INTEGER CHECK (video_min BETWEEN 0 AND 1440),
  PRIMARY KEY (child_id, weekday)
);
CREATE TABLE schedule_intervals (                    -- запретные интервалы
  id INTEGER PRIMARY KEY,
  child_id INTEGER NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  weekday INTEGER NOT NULL CHECK (weekday BETWEEN 1 AND 7), -- день начала интервала
  start_min INTEGER NOT NULL CHECK (start_min BETWEEN 0 AND 1439),
  end_min INTEGER NOT NULL CHECK (end_min BETWEEN 0 AND 1439) -- end <= start: через полночь
);
CREATE TABLE programs (
  id INTEGER PRIMARY KEY,
  exe_path TEXT NOT NULL UNIQUE,                     -- нормализованный полный путь, нижний регистр
  display_name TEXT NOT NULL,
  category TEXT NOT NULL CHECK (category IN ('games','study','other','blocked','system')),
  source TEXT NOT NULL CHECK (source IN ('catalog','snapshot','parent')),
  decided_at INTEGER
);
CREATE TABLE pending_programs (                      -- «Ждут решения»
  id INTEGER PRIMARY KEY,
  exe_path TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  first_child_id INTEGER REFERENCES children(id),
  first_attempt_at INTEGER NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE site_rules (                            -- общие для всех детей
  id INTEGER PRIMARY KEY,
  domain TEXT NOT NULL,                              -- без www, нижний регистр, punycode
  kind TEXT NOT NULL CHECK (kind IN ('blocked','study','video_social')),
  UNIQUE (domain, kind)
);
CREATE TABLE usage_daily (                           -- статистика «за всё время»
  child_id INTEGER NOT NULL REFERENCES children(id),
  day TEXT NOT NULL,
  category TEXT NOT NULL CHECK (category IN ('games','video_social','study','other')),
  subject_kind TEXT NOT NULL CHECK (subject_kind IN ('program','site')),
  subject TEXT NOT NULL,                             -- путь программы или домен
  seconds INTEGER NOT NULL,
  PRIMARY KEY (child_id, day, category, subject_kind, subject)
);
CREATE TABLE day_state (                             -- состояние текущего и прошлых дней
  child_id INTEGER NOT NULL REFERENCES children(id),
  day TEXT NOT NULL,
  total_s INTEGER NOT NULL DEFAULT 0,
  games_s INTEGER NOT NULL DEFAULT 0,
  video_s INTEGER NOT NULL DEFAULT 0,
  study_s INTEGER NOT NULL DEFAULT 0,
  extra_min INTEGER NOT NULL DEFAULT 0,              -- доп. время + потраченные бонусы
  pause_until INTEGER,                               -- UTC мс
  PRIMARY KEY (child_id, day)
);
CREATE TABLE bonus_ledger (
  id INTEGER PRIMARY KEY,
  child_id INTEGER NOT NULL REFERENCES children(id),
  at INTEGER NOT NULL,
  delta_min INTEGER NOT NULL,                        -- +начисление, -трата
  reason TEXT NOT NULL,
  balance_after INTEGER NOT NULL CHECK (balance_after BETWEEN 0 AND 600)
);
CREATE TABLE events (                                -- журнал нарушений и действий
  id INTEGER PRIMARY KEY,
  child_id INTEGER REFERENCES children(id),
  at INTEGER NOT NULL,
  tz_offset_min INTEGER NOT NULL,
  kind TEXT NOT NULL,                                -- см. R22
  details TEXT NOT NULL                              -- JSON
);
CREATE TABLE parent_auth (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  hash TEXT NOT NULL,                                -- Argon2id, параметры внутри строки
  failed_count INTEGER NOT NULL DEFAULT 0,
  blocked_until INTEGER,
  changed_at INTEGER NOT NULL
);
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL);
