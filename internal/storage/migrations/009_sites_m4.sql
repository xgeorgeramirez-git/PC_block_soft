-- Назначение: веха M4 «Сайты и Brave»: один домен — один список, стартовый набор «Видео и соцсети», браузеры каталога запрещены.
-- Правки схемы идут новой миграцией, 001–008 не переписываются (L-131).

-- site_rules (R14, FR-21, RM4-5): у домена одна запись. Таблица пересоздаётся с UNIQUE(domain) и
-- проверкой длины 1…253. Если у домена было несколько записей, остаётся строгая: запрещённые,
-- потом учёба, потом видео (Ruling S-1); при равенстве — с меньшим id; id сохраняется.
CREATE TABLE site_rules_new (
  id INTEGER PRIMARY KEY,
  domain TEXT NOT NULL UNIQUE CHECK (length(domain) BETWEEN 1 AND 253), -- без www, нижний регистр, punycode
  kind TEXT NOT NULL CHECK (kind IN ('blocked','study','video_social'))
);
INSERT INTO site_rules_new (id, domain, kind)
  SELECT r.id, r.domain, r.kind FROM site_rules r
  WHERE r.id = (SELECT r2.id FROM site_rules r2 WHERE r2.domain = r.domain
                ORDER BY CASE r2.kind WHEN 'blocked' THEN 1 WHEN 'study' THEN 2 ELSE 3 END, r2.id LIMIT 1);
DROP TABLE site_rules;
ALTER TABLE site_rules_new RENAME TO site_rules;

-- Стартовый набор «Видео и соцсети» (R14) и при новой установке, и при обновлении. Домен, который
-- родитель уже держит в другом списке, остаётся там (OR IGNORE). Тот же набор — ipc.DefaultVideoSites.
INSERT OR IGNORE INTO site_rules (domain, kind) VALUES
  ('youtube.com','video_social'), ('vk.com','video_social'), ('tiktok.com','video_social'),
  ('rutube.ru','video_social'), ('ok.ru','video_social'), ('telegram.org','video_social'),
  ('web.telegram.org','video_social'), ('discord.com','video_social'), ('twitch.tv','video_social');

-- Браузеры, кроме Brave, — «Запрещено» (R15, D-68, RM4-8), если категорию ставил не родитель
-- (source catalog или snapshot): решение родителя в «Программах» сильнее. Совпадение по имени
-- файла после обратной косой (в LIKE без ESCAPE она обычный символ), поэтому msedge_proxy.exe,
-- msedgewebview2.exe и xmsedge.exe не задеты (PL-18). browser.exe — общее имя, поэтому только в
-- папке Яндекс Браузера. Скрытие, своё название, source и decided_at не меняются (PL-30).
UPDATE programs SET category = 'blocked'
  WHERE source IN ('catalog','snapshot') AND (
    exe_path LIKE '%\chrome.exe' OR exe_path LIKE '%\msedge.exe' OR exe_path LIKE '%\firefox.exe' OR
    exe_path LIKE '%\opera.exe' OR exe_path LIKE '%\vivaldi.exe' OR exe_path LIKE '%\iexplore.exe' OR
    exe_path LIKE '%\yandex\yandexbrowser\application\browser.exe');
