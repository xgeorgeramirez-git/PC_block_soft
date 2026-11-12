-- Назначение: веха M5 (FR-38, FR-24): снимок очистки статистики и индексы журнала.
-- Автобонус без начисления задним числом (D-82) — не здесь: «сегодня» знает только служба (её пояс),
-- границу она пишет в settings при первом запуске (bonus.autobonus_from_day).
-- Правки схемы идут новой миграцией, 001–009 не переписываются (L-131).

-- stats_reset (FR-38, D-73, D-79, RM5-8): последняя очистка статистики ребёнка. Учёт лимита (day_state
-- дня очистки) не трогается: статистика дня очистки считается как «счётчики минус снимок».
CREATE TABLE stats_reset (
  child_id INTEGER PRIMARY KEY REFERENCES children(id),
  cleared_at INTEGER NOT NULL,
  day TEXT NOT NULL,
  total_s INTEGER NOT NULL, games_s INTEGER NOT NULL, video_s INTEGER NOT NULL, study_s INTEGER NOT NULL,
  extra_min INTEGER NOT NULL
);

-- Журнал с фильтрами и курсором (FR-24, RM5-5): страница по (at, id) сверху вниз — по ребёнку и по всем.
CREATE INDEX events_by_child_at ON events(child_id, at, id);
CREATE INDEX events_by_at ON events(at, id);
