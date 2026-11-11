-- Назначение: веха M5 (FR-38, FR-24): снимок очистки статистики и индексы журнала; автобонус без начисления задним числом (D-82).
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

-- Автобонус за пропущенные полуночи (FB-21, R25, D-82): дни до установки 0.5.0 задним числом не
-- начисляются — прошлые строки помечаются начисленными; сегодняшняя строка не трогается.
UPDATE day_state SET bonus_granted = 1 WHERE bonus_granted = 0 AND day < date('now','localtime');
