-- Назначение: веха M3 «Программы и игры»: издатель, отпечаток и корень приложения у программ, время последней попытки и попытки по детям у ждущих решения, индекс учёта по программам.
-- Правки схемы идут новой миграцией, 001–003 не переписываются (L-131).

-- programs: издатель из свойств файла (RM3-3, FR-20), отпечаток «размер:время изменения» для
-- решений родителя вне доверенных каталогов (RM3-12; пусто — не требуется), корень приложения
-- «<доверенный корень>\<первая папка>» для вспомогательных exe (RM3-1 п.3).
ALTER TABLE programs ADD COLUMN publisher TEXT NOT NULL DEFAULT '';
ALTER TABLE programs ADD COLUMN fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE programs ADD COLUMN app_root TEXT NOT NULL DEFAULT '';

-- pending_programs («Ждут решения», R12, FR-20): издатель и описание из файла, момент последней
-- попытки (UTC мс; новые сверху), прежняя категория при смене отпечатка («раньше: Игра», RM3-12).
ALTER TABLE pending_programs ADD COLUMN publisher TEXT NOT NULL DEFAULT '';
ALTER TABLE pending_programs ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE pending_programs ADD COLUMN last_attempt_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE pending_programs ADD COLUMN was_category TEXT NOT NULL DEFAULT '';

-- Кто из детей запускал ждущую решения программу и сколько раз (FR-20: «Миша, 3 попытки»).
-- Решение родителя удаляет строку pending_programs, попытки уходят каскадом.
CREATE TABLE pending_attempts (
  exe_path TEXT NOT NULL REFERENCES pending_programs(exe_path) ON DELETE CASCADE,
  child_id INTEGER NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  attempts INTEGER NOT NULL,
  last_attempt_at INTEGER NOT NULL,                  -- UTC мс
  PRIMARY KEY (exe_path, child_id)
);

-- Время за неделю по программе в «Программах» (FR-20): сумма usage_daily по subject и дню.
CREATE INDEX usage_by_subject ON usage_daily(subject_kind, subject, day);
