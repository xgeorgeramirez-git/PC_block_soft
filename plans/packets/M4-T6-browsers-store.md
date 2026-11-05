# Пакет: T6 вехи M4 «Браузеры и приложения магазина» | слот: писатель | исполнитель: новый агент

Репозиторий `/home/claude/pc_block_soft`, ветка `feature/m4-sites`. Прочитай `plans/2026-11-04-M4.md`: Global Constraints, **RM4-8**, **RM4-11**, RM4-12, Review Focus п. 2, 3, 7; строку T6. ТЗ (`docs/01_ТЗ.md`): R12, R13, R15, FR-20, 8.2.5 п. 4 («Открыл другой браузер (Edge) → закрыт; «Этот браузер запрещён. Пользуйся Brave»»), S19, S46; решения D-63…D-66, D-68. Пакет T4 (`plans/packets/M4-T4-service.md`): S-4 (`isBrave`), S-7, S-14.

**Предложение читателя: T6 — два захода подряд** (пакет общий, вердикты раздельные; T6b не зависит от T6a, но оба трогают `classify.go` — по очереди):
- **T6a — браузеры** (разделы A–C): каталог, «браузер запрещён по имени в любом каталоге», Brave в профиле и его служебные exe, издатель для Brave в тестовом режиме, плашка браузера.
- **T6b — приложения магазина** (разделы D–H): `Roots.Store`, `StableKey` во всех точках ключа программы, издатель из `AppxManifest.xml`, однократное слияние повторов (новый метод хранилища), `TestMode\StoreRoot`, `TestSeedPrograms`.

Сверь адреса (PL-4; в дереве недавно работал писатель T4a): `git log --oneline -15`, `grep -n "func (c \*Classifier) Classify\|func (c \*Classifier) CatalogCategory\|func (c Catalog) Lookup\|func (r Roots) storePackage\|func (r Roots) AppRoot\|func (r Roots) ProfileRoot" internal/programs/*.go`, `grep -n "programs.Normalize" internal/service/*.go | grep -v _test`.

## Образец в коде (проверено читателем)

**`internal/programs`** (стандартная библиотека, тесты локально):
- `catalog.go:31` `(Catalog).Lookup(name) (category string, ok bool)` — браузер даёт `CatOther` (`:39-41`), `:55` `IsBrowser(name)` (не вызывается нигде, кроме тестов), `:61` `baseName`, `:67-76` `catalogBrowsers` (`brave.exe`, `chrome.exe`, `msedge.exe`, `firefox.exe`, `opera.exe`, `vivaldi.exe`, `iexplore.exe`, `browser.exe` = «Яндекс Браузер»). `msedgewebview2.exe`/`msedge_proxy.exe` в каталоге нет.
- `classify.go:217` `Classify(path, fingerprint, publisher) Result` — порядок: пусто → система; `:223` `IsSystem`; `:232` известный точный путь (решение родителя, скрытие, отпечаток); `:247` профиль (`bestProfileRule`, иначе `VerdictUnknown`); `:253` корень приложения; `:256` `CatalogCategory` — **только в доверенных каталогах** (`:265-277`, `IsTrusted`); иначе «незнакомая». `:180` `NeedsPublisher(path)` — только если в корне профиля есть решение (`len(c.profile[key]) > 0`); `:101` `profileKey(p)` = «корень без имени профиля|имя файла»; `:135` `Reload`; `:128` `Entry.hidden()` (запрет сильнее скрытия); `:280` `verdictOf`.
- `path.go:110` `IsSystem` (Windows, PC Block, `<PF>\microsoft\edgewebview`, пакеты `cw5n1h2txyewy`, Защитник); `:146` `storeDir = "windowsapps"`; `:149-162` `storePkg{dir, name, publisher}`, `family()` = `имя__издатель`; `:166` `storePackage(p)` (база — `<ProgramFiles>\windowsapps`, имя папки из 5 частей через «_»); `:194` `AppRoot` (в WindowsApps — `…\windowsapps\<family>`); `:229` `ProfileRoot`, `:239` `profileRel` (первая папка под `Local`: `…\appdata\local\bravesoftware` — корень Brave в профиле; правило PL-20/RM3-13).
- Тесты: `programs_test.go:10` `testRoots`, `:190` строка `{"chrome.exe", "other", true, true}`, `:239-241` «переименованный chrome в Загрузках» → `VerdictUnknown` (**изменится**), «chrome в Program Files по каталогу»; `profile_test.go`.

**`internal/service`**:
- `loop.go:137` `foregroundOf` — `Classify(norm, s.fpSeen[norm], s.pubSeen[norm])`; после T4a сигнатура `foregroundOf(cs, sm) (cat, usageSubject, hidden)`, предмет программы — `norm`.
- `watch.go:40-60` `watchEntry{path, norm, fp, pub, …}`; `:301` `watchDecide`: `:314` `norm := programs.Normalize(p.Path)`, **`:322` ранний выход тестового режима вне `WatchDir` стоит до чтения издателя `:325-333`** — в тестовом режиме издатель Brave в профиле раннера не читается никогда; `:345-347` вердикты → `plateKind`; `:500` `recordClosed` (`:503` плашка `Path: a.e.norm`, `:518` `BlockedForbidden` → `textPlateForbidden`, pending `Path: a.e.norm`, `GetProgram(a.e.norm)`), `:491` `closedName` (`details["path"] = a.e.norm`); `:583` `blockedPlate` (ключ повтора `plate|<сеанс>|<Path>`); `:202-209` `pubs`/`fps` по `e.norm`, `FileMeta(a.e.norm)` `:195`.
- `texts.go:67` `textPlateForbidden = "Программа «%s» запрещена родителем"`.
- `launchkind.go:114` `launchStoreApp(path, programFiles)` — свой `<PF>\windowsapps`.
- `programs.go:14` `settingProgramsSeeded = "programs.seeded"`; `:99` `ensurePrograms`, `:109` `startSeed` (флаг есть → `reloadPrograms`), `:163` `writeSeed`, `:185` `snapshotPrograms` (`:190` `Normalize`, `seen` по пути), `:227` `reloadPrograms`.
- Точки ключа программы (`programs.Normalize` в службе, кроме launchKind/testmode): `handlers_m3.go:158` (`hDecideProgram`), `:334` (`hSetProgramsCategory`), `:391` (`hSetProgramsHidden`), `:497` (`hRenameProgram`), `:655` (`setupPrograms`); `periods.go:132` (`hTestSeedDay`, учёт); `programs.go:190`; `loop.go:142`; `watch.go:314`. `handlers_m3.go:104` `ProgramSeconds(from)` — время за неделю по пути (FR-20), `:238` `fileMetaUnlocked(paths, all)` → `cfg.FileMeta(path)`.
- `handlers_m4.go:24-26` `hTestSeedPrograms` — заглушка; права `commands.go:291` `{testMode(admins()), (*Service).hTestSeedPrograms}`; контракт `ipc/commands_m4.go:152-189` (`TestSeedProgramsRequest{Programs, Usage, Pending}`, `TestSeedProgram{Path, Name, Publisher, Category, Source, Hidden, CustomName, DecidedAt}`, `TestSeedProgramUsage{ChildID, Day, Path, Category, Seconds}`, `TestSeedPending{Path, Name, Publisher, ChildID, Attempts, LastAttemptAt}`). Образец тестовой команды с записью в базу — `periods.go:117` `hTestSeedDay` и `periods_test.go:242` `TestTestSeedDay`.
- `testmode.go:16-37` значения ключа (`valWatchDir`, `TestModeValueNames`), `:86` `ParseTestMode` (`:126-132` проверка `WatchDir`); `winhost/testmode_windows.go:19` `readTestMode` (читает `TestModeValueNames`).
- Тесты, где Chrome сейчас «Прочее» и ожидания **изменятся** по RM4-8: `programs_test.go:49,74` (снимок: chrome `other|catalog` → `blocked|catalog`), `stats_test.go:148-196` («до запрета сегодня chrome=300» — chrome уже запрещён каталогом: взять другой не-браузер, напр. `pathGeoGebra`), `programs_handlers_test.go:116,417` (`SuggestedCategory: "other"` → `"blocked"`), `watch_plates_test.go:45` (плашка chrome → `{Kind: "browser", Text: "Этот браузер запрещён. Пользуйся Brave", ShowS: 10, Path: pathChrome}`), `watch_test.go:183-199`, `hidden_test.go:383-388`, `usage_test.go:156`, `watch_autostart_test.go` (chrome запрещена — останется, но плашек там нет; проверить). Константы путей — `programs_test.go:24-31` (`pathBrave = c:\program files\brave software\…` — **не** настоящая папка `bravesoftware`, оставить), `:14` `svcRoots` (`UserProfiles: c:\users`).

**`internal/service/winhost`** (файлы без `_windows` — стандартная библиотека, тесты идут на Linux):
- `scan.go:108-126` `programScanner{store string, …}`, `newProgramScanner(programFiles, store, installDir, meta, fsys)`; `:539` `walkStore`; `:634` `skipStorePackage`; `:649-659` `appxManifest` (только `Properties.Framework/ResourcePackage` и `Applications>Application@Executable`), `:663` `manifestExecutables`; `:77-78` `manifestMaxBytes`, `storeManifest`. `scan_test.go:290-305` — поддельные `AppxManifest.xml` (`manifestApp`, `manifestFramework`, `manifestResource`). `scan_windows.go:83-92` `newWinScanner` (`store = programFiles + \WindowsApps`), `:70` `machineFolders.roots()`.
- `filemeta_windows.go:21` `fileMeta(path) (publisher, description, fingerprint)` (CompanyName/FileDescription/отпечаток), `:29` `var _ service.FileMeta = fileMeta`; `filemeta.go` — чистые части (`pickVersionStrings`, `fingerprintOf`), тесты `filemeta_test.go`.
- `host_windows.go:66-91`: `folders := readMachineFolders()`, `roots := folders.roots()`, `Scanner: newWinScanner(folders)`, `FileMeta: fileMeta`, `Roots: roots`; `tm` (тестовый режим) уже есть до этого.
- `adapters_windows.go:99-110` — образец переноса типов `storage.Program(p)` ↔ `service.Program(p)` (одинаковые поля).

**`internal/storage`** (только CI или локальный мост, см. «Среда»):
- `programs.go:59-71` `Program{ID, Path, Name, Publisher, Category, Source, Fingerprint, AppRoot, DecidedAt, Hidden, CustomName}`, `:101` `programColumns`, `:115` `upsertProgram(ex execer, p)`, `:145` `SeedPrograms` (образец транзакции: `s.db.Begin()`, `defer tx.Rollback()`), `:326` `PendingProgram{…, Children []PendingChild}`, `:344` `AddPendingAttempt`, `:393` `listPending(q querier, path)`, `:483` `RestorePending` (складывает попытки), `:554` `addUsage(ex, childID, u)`; схема: `programs.exe_path UNIQUE`, `pending_programs.exe_path UNIQUE`, `pending_attempts(exe_path REFERENCES pending_programs ON DELETE CASCADE, child_id, attempts, last_attempt_at, PK(exe_path, child_id))`, `usage_daily PK(child_id, day, category, subject_kind, subject)`.
- Миграция 009 (`migrations/009_sites_m4.sql`, хвост) уже переводит браузеры каталога/снимка в `blocked` по окончанию пути; `browser.exe` — только `%\yandex\yandexbrowser\application\browser.exe`; `msedge_proxy.exe`, `msedgewebview2.exe` не задеты.

## Ruling читателя (оркестратор подтверждает до пуска; цена ошибки — в скобках)

### T6a — браузеры

- **T6-1 Каталог.** `Lookup`: `brave.exe` → `CatOther`; остальные браузеры → `CatBlocked`. Новый `(Catalog).ForbiddenBrowser(path string) (name string, ok bool)`: имя файла в `catalogBrowsers`, не `brave.exe`; `browser.exe` — только если путь оканчивается на `\yandex\yandexbrowser\application\browser.exe` (как миграция 009: имя общее). `Lookup("browser.exe")` по голому имени — `("", false)` (сканер зовёт `Lookup(e.name)`, `scan.go:461,519`). `IsBrowser` удалить или оставить для `brave.exe` тоже `true` — решить по вызовам (сейчас только тесты). (Цена: `browser.exe` из чужой игры без проверки папки был бы запрещён везде.)
- **T6-2 Порядок `Classify`** (комментарий функции переписать списком):
  1. пусто → система; 2. `IsSystem` (+ T6-4); 3. известный точный путь — **решение родителя сильнее каталога браузеров** (RM4-8); 4. **Brave в профиле** (T6-3); 5. **запрещённый браузер по имени в любом каталоге** (`ForbiddenBrowser`) → `{VerdictForbidden, CatBlocked, Name: NameFromPath}`; 6. решения в профиле; 7. корень приложения; 8. каталог (доверенные); 9. незнакомая.
  Браузер стоит до корня и решений профиля: Chrome в профиле (`%LOCALAPPDATA%\Google\Chrome\Application`) и в Загрузках — «Запрещено», а не «ждёт решения»; переименованный браузер вне доверенных — по-прежнему «незнакомая» (закрыт). (Цена: родитель разрешил Chrome по точному пути — новая папка Chrome снова запрещена, родитель решает ещё раз; Chrome обновляется на месте, это редкость.)
- **T6-3 Brave в профиле (PL-20, RM4-8).** Путь в `<UserProfiles>\<имя>\appdata\local\bravesoftware\…` и издатель с `publisherKey` с префиксом `"brave software"` (CompanyName — «Brave Software, Inc.»): относительный путь корня `brave-browser\application\brave.exe` → `{VerdictAllow, CatOther}`; любой другой exe этой папки (`update\braveupdate.exe`, `update\<версия>\bravecrashhandler*.exe`, `brave-browser\application\<версия>\notification_helper.exe`, установщик) → `VerdictSystem` (служебные части Brave: не закрываются ни «незнакомыми», ни «только учёбой», секунды — «Прочее»). Издатель пустой или чужой → дальше по порядку (обычно «незнакомая»). `brave.exe` в другом месте профиля или в Загрузках — «незнакомая». `NeedsPublisher` → `true` для путей этой папки, которых нет в точном списке без отпечатка. (Цена: подделка CompanyName — та же, что PL-20, README «Отклонения от ТЗ»; без издателя — копия игры с именем `brave.exe` в профиле шла бы «Прочим». Сбой чтения свойств файла — Brave закрыт как «незнакомая» до следующего процесса: как у любой программы профиля.)
- **T6-4 Служебные Brave в Program Files.** `<PF>\bravesoftware\update\…` и `<PF x86>\bravesoftware\update\…` — в `IsSystem` по префиксу (доверенные каталоги, ребёнок туда писать не может; системный Brave ставит туда `BraveUpdate.exe`). Сам Brave в `<PF>\bravesoftware\brave-browser\application\brave.exe` — как сейчас, каталог «Прочее». (Цена: в «только учёбе» обновлятор Brave закрывался бы с плашкой «Время вышло» ребёнку раз в час.)
- **T6-5 Издатель в тестовом режиме.** В `watchDecide` чтение свойств (`needFP`/`needPub`, `:325-333`) переносится **выше** раннего выхода тестового режима (`:322`): чтение файла ничего не закрывает, а без него Brave из профиля раннера (choco ставит его в `%LOCALAPPDATA%`, вне `WatchDir`) в `foregroundOf` — «незнакомая», и T4a `isBrave` ложь: S16…S18 не работали бы вовсе. Выход тестового режима остаётся до `Classify`/закрытия. (Цена: лишние чтения свойств только для путей, где `NeedsPublisher`/`NeedsFingerprint` = true.)
- **T6-6 Плашка и журнал браузера.** `recordClosed`, ветка `BlockedForbidden` (ручной запуск): если `classifier.Catalog().ForbiddenBrowser(a.e.norm)` — `plate.Kind = ipc.BlockedBrowser`, `plate.Text = textPlateBrowser = "Этот браузер запрещён. Пользуйся Brave"` (8.2.5 п. 4 дословно) — **при любом источнике запрета** (каталог или родитель). Журнал — как у программы: `evProgramForbiddenTry`, «Миша: попытка открыть запрещённое: «chrome»» (название — `DisplayName`; T6 его не меняет). Автозапуск (`a.auto`) — молча через `launchWait`, как сейчас (R13, D-65); дочерние процессы (`a.e.quiet`) — без записей (PL-40). Ключ плашки — по пути (`blockedPlate`), не по виду: Chrome и Edge подряд дают две плашки. (Цена: разные браузеры — две одинаковые плашки подряд; иначе вторая пропала бы.)
- **T6-7 WebView2 и msedge_proxy — не браузеры** (PL-18): `msedgewebview2.exe` в любом месте — не `ForbiddenBrowser` (в `<PF>\microsoft\edgewebview` — система, как сейчас); `msedge_proxy.exe` — не браузер (S46 ждёт её строкой «Программ» с издателем Microsoft). Тесты — литералами ниже.

### T6b — приложения магазина

- **T6-8 Папка пакетов — поле корней.** `Roots.Store string` — нормализованная папка пакетов (`""` → `<ProgramFiles>\windowsapps`); `func (r Roots) StoreDir() string`. Её используют `storePackage`, `AppRoot`, `IsSystem` (через `storePackage`), `launchStoreApp` (сигнатура → `launchStoreApp(path, storeDir string)`, вызов `watch.go` с `roots.StoreDir()`). Тестовый режим: `host_windows.go` ставит `roots.Store = tm.StoreRoot`, если задан, и сканер получает ту же папку (`newWinScanner(folders, store)`), чтобы снимок и классификатор видели одно. `IsTrusted` не меняется (тестовая папка в `C:\Temp` не доверенная — это нужно только каталогу по имени и отпечаткам; записи S46 засеваются без отпечатков). (Цена: без поля пришлось бы писать в защищённую TrustedInstaller `WindowsApps` раннера.)
- **T6-9 `StableKey` — один шаг ключа.** `func (r Roots) StableKey(path string) string` = `Normalize`, затем путь в пакете по образцу (5 частей) → `<StoreDir>\<имя>__<издатель>\<путь exe>`; папка не по образцу и всё вне `StoreDir` — `Normalize` как есть. Идемпотентна (`StableKey(StableKey(p)) == StableKey(p)`; папка `имя__издатель` — 3 части после разбиения, «не по образцу», не трогается). `Classifier.Reload`/`Classify`/`NeedsPublisher`/`NeedsFingerprint` берут `StableKey` внутри (принимают и путь версии, и ключ). В службе — `StableKey` вместо `Normalize` во **всех точках ключа**: `handlers_m3.go:158,334,391,497,655`, `periods.go:132`, `programs.go:190`, `loop.go:142` (предмет учёта; T4a `usageSubject` программы), `watch.go` — новое поле `watchEntry.key` (pending, `GetProgram`, `closedName` `path`, `plate.Path`, ключи ограничителей `ev|…|<key>`, `autoClosedOnce`); **реальный путь остаётся** в `e.norm` для `FileMeta`, `fps`/`pubs`, `launchParentOf`/`launchFacts.Self` (сравнение с путём родителя-процесса). `foregroundOf` ищет `fpSeen`/`pubSeen` по реальному `norm`. Тест-страж «больше нет `programs.Normalize` в точках ключа» — `grep` в вердикте. (Цена: пропущенная точка — снова повторы строк или «Ждут решения» по версии; единый метод проще проверить grep-ом.)
- **T6-10 Издатель из манифеста (RM4-11).** `appxManifest` получает `Properties.PublisherDisplayName string \`xml:"PublisherDisplayName"\``; `func manifestPublisher(data []byte) string` — пусто, если значение начинается с `ms-resource:` (локализованная ссылка). `FileMeta` в `winhost` становится методом `metaReader{store string}` (`host_windows.go`: `FileMeta: metaReader{store: roots.StoreDir()}.read`; сканер — тот же): путь в `store` → найти папку пакета (для пути версии — она сама; для ключа `имя__издатель` — `pickStoreDir(names []string, name, pub string) string`: папки `<имя>_<версия>_<арх>__<издатель>` с пустым ресурсом, наибольшая версия по числам «a.b.c.d»; ресурсные `…_split.scale-100_…` не берутся); издатель = `manifestPublisher(<папка>\AppxManifest.xml)`, иначе CompanyName exe из этой папки, иначе для издателя `8wekyb3d8bbwe` — «Microsoft Corporation»; описание и отпечаток — из exe найденной папки. Вне `store` — как сейчас. `pickStoreDir`, `manifestPublisher`, разбор ключа — в `filemeta.go`/`scan.go` (Linux-тесты). (Цена: «Microsoft Corporation» у пакета `8wekyb3d8bbwe` без манифеста — по RM4-11; ссылки `ms-resource:` не разворачиваются — тогда CompanyName.)
- **T6-11 Слияние повторов — план в Go, запись одной транзакцией.** Флаг `settingStoreMerged = "programs.storemerged"`. В `startSeed`: флаг снимка есть и `storemerged != "1"` → `s.mergeStorePrograms()` **до** `reloadPrograms`; при новой установке `writeSeed` ставит `storemerged = "1"` вместе с `programs.seeded` (повторов быть не может: снимок уже по ключам). План — чистая функция `planStoreMerge(roots programs.Roots, list []Program, pend []PendingProgram) []ProgramMerge` (`programs_merge.go`); группа — строки с одинаковым `StableKey`, у которых хотя бы один путь ≠ ключа (строка уже с ключом — тоже член группы):
  - категория и источник — у строки с наибольшим `DecidedAt` среди `source = parent`; решений нет — у строки с наибольшим путём (новейшая версия), источник её;
  - `Hidden` — `true`, если скрыта хоть одна (скрытие — решение родителя, переживает обновление); запрет сильнее скрытия обеспечивает классификатор (`Entry.hidden()`), флаг в записи не трогается (PL-30);
  - `CustomName` — непустое у строки с наибольшим `DecidedAt` (ничьё — у наибольшего пути); `Name`, `Publisher` — новейшей версии с непустым; `DecidedAt` — наибольший; `Fingerprint` — `""`; `AppRoot` — `roots.AppRoot(key)`;
  - «Ждут решения»: ключ группы есть среди программ → строки pending группы **удаляются** (S46: «в «Ждут решения» её нет»); программы нет, а pending несколько → одна строка pending с ключом: попытки по детям суммируются, первая попытка — самая ранняя, последняя — самая поздняя, имя/издатель — последней;
  - pending без строки programs с путём версии и одной строкой — тоже переводится на ключ;
  - `usage_daily` строк `subject_kind = 'program'` с предметом-путём группы — суммируется на ключ (`ON CONFLICT … seconds = seconds + excluded.seconds`), старые удаляются.
  Хранилище: `MergePrograms(ms []ProgramMerge) error`, `type ProgramMerge struct{ Into Program; From []string; Pending *PendingProgram /* nil — pending группы удалить */ ; PendingFrom []string }` — одна транзакция: `upsertProgram(Into)`; `DELETE FROM programs WHERE exe_path IN From AND exe_path <> Into.Path`; учёт (`INSERT … SELECT child_id, day, category, 'program', ?, SUM(seconds) … GROUP BY … ON CONFLICT DO UPDATE`, затем `DELETE`); pending: вставить/заменить строку ключа и её `pending_attempts`, затем удалить строки `PendingFrom` (каскад уберёт их попытки). Ошибка — ничего не изменилось, флаг не ставится, `logRate("storemerge", …)`, повтор при следующем старте; удача — `SetSetting(storemerged,"1")`, `Logf("Повторы приложений магазина слиты: %d строк в %d", …)`. Идемпотентно: второй запуск плана на слитой базе даёт `[]`. (Цена «последнего решения» вместо «строгого»: родитель запретил старую версию, потом разрешил новую — действует разрешение, что и было его последней волей.)
- **T6-12 `TestSeedPrograms` (PL-41).** Только тестовый режим (права уже в `commands.go:291`). Пишет как есть, **без `StableKey`** (это «база 0.3.3»): программы — `UpsertProgram` (путь `Normalize`, пустой или непригодный → `bad()`; категория/источник — из списков `ipc.ProgramCat*`/`ProgramSource*`, иначе `bad()`); учёт — `AddUsage(childID, day, category, "program", path, seconds)` (неизвестный ребёнок → `textNoSuchChild`, день — как в `hTestSeedDay`, но может быть и сегодня); pending — `RestorePending(PendingProgram{Path, Name, Publisher, FirstChildID: ChildID, FirstAttemptAt: LastAttemptAt, LastAttemptAt, Attempts, Children: [{ChildID, Attempts, LastAttemptAt}]})`; затем `SetSetting(settingStoreMerged, "0")` и `reloadPrograms()`. Слияние — при следующем старте службы (приёмка S46 перезапускает службу, T10). Ответ `ipc.OK(ipc.Empty{})`, журнал работы «Тестовые программы: %d, учёт %d, ждут решения %d». (Цена: без сброса флага S46 не проверил бы слияние вовсе.)
- **T6-13 `TestMode\StoreRoot`.** `valStoreRoot = "StoreRoot"` в `TestModeValueNames`; `TestMode.StoreRoot string` (нормализованный); `ParseTestMode`: непустой — полный путь (как `WatchDir`) и **внутри `WatchDir`** (иначе ошибка «Тестовый режим: StoreRoot должен быть папкой внутри WatchDir, а не %q») — наблюдатель трогает только `WatchDir`. Пустой — настоящая `WindowsApps`.

## Files

**T6a** — Modify: `internal/programs/catalog.go`, `classify.go`, `path.go` (T6-4), `programs_test.go`, `profile_test.go` (или новый `browsers_test.go` — Create); `internal/service/watch.go` (T6-5, T6-6), `texts.go` (`textPlateBrowser`), `watch_plates_test.go`, `programs_test.go`, `stats_test.go`, `programs_handlers_test.go`, `watch_test.go`, `hidden_test.go`, `usage_test.go` (ожидания по RM4-8), Create `internal/service/browsers_test.go`.
**T6b** — Modify: `internal/programs/path.go` (`Store`, `StoreDir`, `StableKey`), `classify.go`, тесты; `internal/service/handlers_m3.go`, `periods.go`, `programs.go`, `loop.go`, `watch.go`, `launchkind.go` (+ `launchkind_test.go`), `handlers_m4.go` (`hTestSeedPrograms`), `testmode.go` (+ `testmode_test.go`), `ports.go` (`ProgramMerge`, `MergePrograms`), `fakes_test.go` (`MergePrograms` фейка — та же семантика на картах); Create `internal/service/programs_merge.go`, `programs_merge_test.go`, `store_programs_test.go`; `internal/service/winhost/scan.go` (манифест), `scan_windows.go`, `filemeta.go`, `filemeta_windows.go` (`metaReader`), `host_windows.go`, `adapters_windows.go` (`MergePrograms`), `filemeta_test.go`, `scan_test.go`; `internal/storage/programs.go` (`ProgramMerge`, `MergePrograms`), Create `internal/storage/programs_merge_test.go`.
Не трогать: `internal/core`, `internal/sites`, `internal/ipc` (контракт `TestSeedPrograms*` уже есть), `internal/guard`, `frontend/`, `docs/`, `plans/`, миграции (новой миграции нет: слияние в Go), версию.

## Interfaces

```go
// programs
func (c Catalog) ForbiddenBrowser(path string) (name string, ok bool)
func (r Roots) StoreDir() string
func (r Roots) StableKey(path string) string
type Roots struct { …; Store string } // "" — <ProgramFiles>\windowsapps
// service
const settingStoreMerged = "programs.storemerged"
const textPlateBrowser = "Этот браузер запрещён. Пользуйся Brave"
type ProgramMerge struct { Into Program; From []string; Pending *PendingProgram; PendingFrom []string }
// Store: MergePrograms(ms []ProgramMerge) error — одной транзакцией; ошибка — ничего не изменилось
func planStoreMerge(roots programs.Roots, list []Program, pend []PendingProgram) []ProgramMerge
func (s *Service) mergeStorePrograms() // под s.mu, из startSeed
func launchStoreApp(path, storeDir string) bool
// TestMode: StoreRoot string
// winhost
type metaReader struct{ store string }
func (m metaReader) read(path string) (publisher, description, fingerprint string)
func manifestPublisher(data []byte) string
func pickStoreDir(names []string, name, pub string) string
```

## Шаги T6a (каждый: RED → GREEN → мутации → коммит)

### A. Каталог и порядок классификатора (T6-1…T6-4, T6-7)

RED `TestBrowsersClassify` (`internal/programs`, `testRoots`, каталог `DefaultCatalog`, пустой список; `brave = "Brave Software, Inc."`):

| путь | издатель | ждём |
|---|---|---|
| `c:\program files\google\chrome\application\chrome.exe` | — | `{Forbidden, blocked, Name "chrome"}` |
| `c:\users\миша\downloads\chrome.exe` | — | `{Forbidden, blocked}` (было Unknown, `programs_test.go:239`) |
| `c:\users\миша\appdata\local\google\chrome\application\chrome.exe` | — | `{Forbidden, blocked}` |
| `c:\program files (x86)\microsoft\edge\application\msedge.exe` | — | `{Forbidden, blocked, Name "msedge"}` |
| `c:\users\миша\appdata\local\yandex\yandexbrowser\application\browser.exe` | — | `{Forbidden, blocked}` |
| `c:\users\миша\downloads\browser.exe` | — | `{Unknown}` |
| `c:\program files (x86)\microsoft\edgewebview\application\130.0.2849.80\msedgewebview2.exe` | — | `{System}` |
| `c:\users\миша\appdata\local\teams\msedgewebview2.exe` | — | не `Forbidden` |
| `c:\program files\windowsapps\microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe\msedge_proxy.exe` | — | не `Forbidden` |
| `c:\program files\bravesoftware\brave-browser\application\brave.exe` | — | `{Allow, other}` |
| `c:\program files (x86)\bravesoftware\update\braveupdate.exe` | — | `{System}` |
| `c:\users\миша\appdata\local\bravesoftware\brave-browser\application\brave.exe` | brave | `{Allow, other, Name "brave"}` |
| то же | `""` | `{Unknown}` |
| то же | `Evil Games` | `{Unknown}` |
| `c:\users\миша\appdata\local\bravesoftware\update\braveupdate.exe` | brave | `{System}` |
| `c:\users\миша\appdata\local\bravesoftware\brave-browser\application\157.1.99.1\notification_helper.exe` | brave | `{System}` |
| `c:\users\миша\downloads\brave.exe` | brave | `{Unknown}` |

Плюс: после `Reload([{Path: <chrome PF>, Category: "other"}])` — chrome `{Allow, other, Known}` (решение родителя сильнее); `Reload([{Path: <chrome PF>, Category: "other", Hidden: true}])` → `{Hidden}`. `NeedsPublisher(<brave в профиле>) == true`, `NeedsPublisher(<brave PF>) == false`. `Lookup("chrome.exe") == ("blocked", true)`, `Lookup("brave.exe") == ("other", true)`, `Lookup("browser.exe") == ("", false)`, `Lookup("msedgewebview2.exe") == ("", false)`.

Мутации A: браузер после корня приложения → chrome в профиле `Unknown`; без проверки папки `browser.exe` → Загрузки `Forbidden`; Brave без издателя → «Evil Games» `Allow`; браузер раньше точного пути → решение родителя проигрывает; служебный Brave как `Allow` → тест `System`.
Коммит: `feat: запрещать браузеры, кроме Brave, в любом каталоге`, `feat: узнавать Brave в профиле по издателю`.

### B. Служба: издатель в тестовом режиме и плашка (T6-5, T6-6)

Константа теста `pathBraveProfile = \`c:\users\миша\appdata\local\bravesoftware\brave-browser\application\brave.exe\``.
RED `TestBrowserPlate` (`watchHarness`, `withRules(120, -1, "lock")`, ручной запуск по образцу `watch_plates_test.go:39-42`): `startManual(w, explorer, pathChrome)` → `h.watch()` → `platesOf(h.poll(activeSample)) = [{Kind: "browser", Text: "Этот браузер запрещён. Пользуйся Brave", ShowS: 10, Path: pathChrome}]`, `h.eventTexts(evProgramForbiddenTry) = ["Миша: попытка открыть запрещённое: «chrome»"]`, `w.alive(chrome) == false`; родитель запретил свою программу `pathGeoGebra` (`setCategory`) → плашка `{Kind: "forbidden", Text: "Программа «GeoGebra Classic» запрещена родителем", …}` (не браузер); автозапуск chrome (образец `watch_autostart_test.go:44`) → плашек 0, нарушений 0.
RED `TestBraveProfileReadsPublisherInTestMode`: `withTestMode(TestMode{WatchDir: \`c:\temp\pcb-programs\`})`, `withMeta(&fakeMeta{files: {pathBraveProfile: {"Brave Software, Inc.", "Brave Browser", "1:1"}}})`, `w.start(childSession, pathBraveProfile)` → `h.watch()` → `w.alive == true` (вне `WatchDir` не трогается), `h.svc.pubSeen[pathBraveProfile] == "Brave Software, Inc."`; `foregroundOf` для `braveSample` с этим путём и доменом `youtube.com` → `CatVideoSocial` (после T4a; если T4a не слит — проверка `Classify(…, pubSeen)` = `Allow`). Без `withMeta` → `pubSeen` пуст, `foregroundOf` → `CatOther`, предмет — путь (незнакомая не учитывается сайтами).
RED `TestStudyOnlyKeepsBraveHelpers` (`withRules(0, -1, "study_only")`): `c:\users\миша\appdata\local\bravesoftware\update\braveupdate.exe` (издатель Brave) жив после `h.watch()`; `pathMinecraft` закрыт.
Обновить ожидания старых тестов (список в «Образце»): каждое изменение — по RM4-8, перечислить в вердикте.

Мутации B: чтение свойств после раннего выхода → `pubSeen` пуст; `BlockedBrowser` только при источнике «каталог» → плашка родительского запрета Chrome — «Программа «chrome»…»; плашка браузера у автозапуска → тест автозапуска.
Коммит: `feat: плашка о запрещённом браузере`, `fix: читать издателя программ профиля и в тестовом режиме`.

### C. Регресс T6a

`go test -race -count=1 ./internal/programs/ ./internal/service/` зелёные; `TestWatcherDryRun` (`winhost`, на раннере) не должен найти новых закрываемых — посмотреть пометку «Сухой прогон наблюдателя» в прогоне `windows.yml` (оркестратор).

## Шаги T6b

### D. Папка пакетов и ключ (T6-8, T6-9)

RED `TestStableKey` (`internal/programs`, `wa = c:\program files\windowsapps`):
- `wa\microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe\msedge_proxy.exe` → `wa\microsoft.microsoftedge.stable__8wekyb3d8bbwe\msedge_proxy.exe`;
- `C:\Program Files\WindowsApps\Microsoft.MicrosoftEdge.Stable_131.0.2903.51_neutral__8wekyb3d8bbwe\msedge_proxy.exe` → тот же ключ;
- `wa\microsoft.minecraftuwp_1.21.4401.0_x64__8wekyb3d8bbwe\minecraft.windows.exe` → `wa\microsoft.minecraftuwp__8wekyb3d8bbwe\minecraft.windows.exe`;
- ключ → он же (идемпотентность); `wa\strange\x.exe` → как есть; `wa\x.exe` → как есть; `c:\program files\geogebra\geogebra.exe` → как есть;
- `Roots{…, Store: \`c:\temp\pcb-programs\store\`}`: `c:\temp\pcb-programs\store\contoso.tool_2.0.0.0_x64__abcdef\tool.exe` → `c:\temp\pcb-programs\store\contoso.tool__abcdef\tool.exe`; при этом настоящий `wa\…` — как есть; `AppRoot` и `storePackage` — по `Store`.
RED `TestClassifyStoreKey`: `Reload([{Path: <msedge_proxy 130>, Category: "other", Hidden: true}])` → `Classify(<msedge_proxy 131>) = {Hidden}` (сейчас — `Allow` по корню: скрытие корню не передаётся, RF9-2); `Reload([{Path: <ключ>, Category: "blocked"}])` → `Classify(<130>)` = `Forbidden`.
RED служба (`store_programs_test.go`): `TestStoreKeyEverywhere` — незнакомое приложение `wa\contoso.tool_1.0.0.0_x64__abcdef\tool.exe` запускается дважды из папок `1.0.0.0` и `2.0.0.0` → `store.pending` одна строка с ключом, попыток 2, `plate.Path` = ключ; решение родителя `DecideProgram{Path: <ключ>, Category: "games"}` → третий запуск из `3.0.0.0` — не закрыт (игры можно); `run(5, fgSample(<3.0.0.0 путь>, …))` → ключ учёта `"…|games|program|" + <ключ>`; `SetProgramsHidden([<путь версии 2.0.0.0>])` прячет строку ключа. `launchStoreApp(<ключ>, wa) == true`, `launchStoreApp(c:\program files\geogebra\geogebra.exe, wa) == false`.
Проверка в вердикте: `grep -n "programs.Normalize" internal/service/*.go | grep -v _test` — остаются только `launchkind.go`, `testmode.go`, `watch.go` (реальный путь `e.norm`, передний план `t.fg`), `loop.go` (если `fpSeen` по реальному пути) — каждое с комментарием «реальный путь, не ключ».

Мутации D: `StableKey` без проверки 5 частей → `wa\strange\x.exe` меняется; ключ только в `Classify`, но не в `recordClosed` → две строки pending; `e.norm` заменён ключом → `FileMeta` спрашивают несуществующий путь (тест `fakeMeta.asked`).
Коммит: `feat: один ключ на все версии приложения магазина`.

### E. Издатель из манифеста (T6-10)

RED (`winhost`, Linux): `manifestPublisher(<manifest с <Properties><PublisherDisplayName>Microsoft Corporation</PublisherDisplayName>…>)` = `"Microsoft Corporation"`; `ms-resource:PublisherName` → `""`; испорченный → `""`. `pickStoreDir(["microsoft.microsoftedge.stable_130.0.2849.80_neutral__8wekyb3d8bbwe", "microsoft.microsoftedge.stable_131.0.2903.51_neutral__8wekyb3d8bbwe", "microsoft.microsoftedge.stable_131.0.2903.51_neutral_split.scale-100_8wekyb3d8bbwe", "microsoft.microsoftedge.stable_99.0.0.0_neutral__otherpub"], "microsoft.microsoftedge.stable", "8wekyb3d8bbwe")` = `"…_131.0.2903.51_neutral__8wekyb3d8bbwe"`; версии `9.0.0.0` и `10.0.0.0` → `10.0.0.0` (по числам, не по строке); нет подходящих → `""`. `metaReader` на поддельной папке (через `scanFS` или `t.TempDir()` — `os` есть и на Linux): ключ → издатель из манифеста новейшей версии; ключ `8wekyb3d8bbwe` без папок → `("Microsoft Corporation", "", "")`; путь вне `store` → как `fileMeta` (на Linux `versionStrings` нет — вынести выбор ветки в чистую функцию `storeMetaSource(store, path string) (pkgDir, exe string, ok bool)` и тестировать её).
Коммит: `feat: издатель приложения магазина из манифеста пакета`.

### F. Слияние повторов (T6-11)

RED `TestPlanStoreMerge` (чистая, `programs_merge_test.go`; `p130`, `p131`, `p132` — `msedge_proxy.exe` из трёх папок, `key` — ключ):
- `list = [{p130, "msedge_proxy", "", other, snapshot}, {p131, "Microsoft Edge", "Microsoft Corporation", other, parent, DecidedAt 100, Hidden true}, {p132, "msedge_proxy", "", blocked, parent, DecidedAt 50}]` → один `ProgramMerge{Into: {Path: key, Name: "Microsoft Edge", Publisher: "Microsoft Corporation", Category: other, Source: parent, DecidedAt: 100, Hidden: true, AppRoot: wa\microsoft.microsoftedge.stable__8wekyb3d8bbwe, Fingerprint: ""}, From: [p130 p131 p132]}`;
- те же, но у `p132` `DecidedAt 200` → `Category: blocked`, `Hidden: true` (флаг остаётся, запрет сильнее — классификатор);
- `CustomName`: `p130` «Эдж» (`DecidedAt 10`), `p131` «» (`100`) → «Эдж»;
- pending `[{p131, 2 попытки Миша}, {p132, 1 попытка Даша}]` при программе в группе → `Pending == nil`, `PendingFrom = [p131 p132]`; без программы → `Pending{Path: key, Attempts 3, Children [{Миша 2} {Даша 1}], FirstAttemptAt min, LastAttemptAt max}`;
- одна строка с путём версии (без повторов) → тоже переводится на ключ; строки не магазина — не в плане; второй вызов на результате → `[]`.
RED `TestStoreMergeOnceAtStart` (служба, фейк): база «0.3.3» в фейке (три строки выше, `store.usage` `"<id>|2026-10-12|other|program|"+p130 = 600`, `…p131 = 300`, сегодня `p132 = 60`, pending p131), `store.settings["programs.seeded"]="1"`, флага слияния нет → `start` → `store.programs` одна строка `key` (Hidden, «Microsoft Edge»), `usage` `…|2026-10-12|other|program|`+key = 900, сегодня 60, pending пуст, `settings["programs.storemerged"] == "1"`; `ListPrograms` → одна строка, `Publisher` «Microsoft Corporation», `WeekS` = сумма; перезапуск службы → `MergePrograms` не вызывается (счётчик вызовов фейка = 1). Ошибка `MergePrograms` (`failMerge`) → флага нет, классификатор всё равно прочитан (`progReady`), в журнале работы «Повторы приложений магазина не слиты»; следующий старт без ошибки — слито.
RED хранилище (`internal/storage/programs_merge_test.go`): та же база через `UpsertProgram`/`AddUsage`/`RestorePending` → `MergePrograms(plan)` → `ListPrograms` одна строка, `DayUsage` сумма, `ListPending` пуст; ошибка посреди (второй `ProgramMerge` с недопустимой категорией `"x"`) → `ListPrograms` как до вызова (транзакция); повтор плана на слитой базе — без изменений.

Мутации F: «последнее решение» по пути, а не по `DecidedAt` → случай 2; `Hidden` от одной строки → случай 1; учёт без суммирования (`DO UPDATE SET seconds = excluded.seconds`) → 300 вместо 900; без транзакции → тест ошибки посреди; флаг до записи → ошибка не повторяется.
Коммиты: `feat: метод хранилища слить повторы программ одной транзакцией`, `feat: слить повторы приложений магазина при первом старте 0.4.0`.

### G. Тестовый режим (T6-12, T6-13)

RED `TestParseTestModeStoreRoot` (`testmode_test.go`): `{WatchDir: C:\Temp\pcb-programs, StoreRoot: C:\Temp\pcb-programs\store}` → `StoreRoot == c:\temp\pcb-programs\store`; `StoreRoot: C:\Temp\other` → ошибка с «StoreRoot должен быть папкой внутри WatchDir»; без `WatchDir` → та же ошибка; `TestModeValueNames` содержит `"StoreRoot"`.
RED `TestTestSeedPrograms` (по образцу `periods_test.go:242`): тестовый режим, `TestSeedProgramsRequest{Programs: [три строки msedge_proxy], Usage: [{<Миша>, "2026-10-12", p130, "other", 600}], Pending: [{p131, "msedge_proxy", "Microsoft Corporation", <Миша>, 2, <мс>}]}` → OK; в фейке строки **с путями версий** (без ключа), `settings["programs.storemerged"] == "0"`; новый `New`+`Start` на том же фейке → слито (как F). Вне тестового режима — отказ правами (`commands_test.go` уже покрывает команду в таблице прав — проверить, что тест есть: `commands_test.go:106`). Неизвестная категория → `ipc.TextBadRequest`; `ChildID: 999` → `textNoSuchChild`.
Коммит: `feat: тестовая папка магазина и засев программ «до обновления»`.

### H. Сборка Windows

`host_windows.go`: `roots.Store = tm.StoreRoot` (если не пусто), `metaReader{store: roots.StoreDir()}`, `newWinScanner(folders, roots.StoreDir(), meta)`; `adapters_windows.go` — `MergePrograms` с переносом типов. Проверка — `ci.yml` (`GOOS=windows go vet`), затем `windows.yml` без сценария (тесты Go на Windows, сухой прогон наблюдателя) и `-f scenario=accept -f only=S12,S13,S14,S15,S45` (регресс программ; S19, S46 пишет T10).

## Среда и команды прогона

```
export PATH=~/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off
go test -race -count=1 ./internal/programs/ ./internal/service/ ./internal/service/winhost/
go vet ./internal/programs/ ./internal/service/ ./internal/service/winhost/
gofmt -l internal
python3 tools/scan_ui_texts.py
bash tools/codemap.sh   # docs/CODEMAP.md — вместе с последним коммитом кода захода
```
**Хранилище локально — через мост sqlite писателя T3a** (заглушка драйвера `modernc.org/sqlite`, SQL исполняет `sqlite3` Python): `bash /tmp/claude-0/-home-claude/25f23a37-6a15-5cbe-89db-2e036117eed9/scratchpad/st.sh -run 'TestMergePrograms|TestSeedPrograms' -v` — скрипт сам ставит Go 1.27 в PATH, переходит в репозиторий и запускает `go test -modfile=<scratchpad>/vet.mod -mod=mod -count=1 <аргументы> ./internal/storage/` (мост — `<scratchpad>/bridge/bridge.py`, заглушка — `<scratchpad>/stub/sqlite`). Аргументы `go test` передаются как есть. Ограничения: без `-race` по умолчанию (можно добавить аргументом), версия SQLite Python может отличаться от modernc — истина остаётся за `ci.yml` (`go test -race` на ubuntu). `go.mod`/`go.sum` мост не трогает (свой `-modfile`); если `vet.mod` отстал от `go.mod` (новые зависимости), сообщить оркестратору, не править.

## Review Focus (самопроверка)

1. PL-18/PL-19: WebView2, `msedge_proxy`, служебные части Brave, PC Block не закрываются; сухой прогон на раннере — без новых закрываемых.
2. PL-20: Brave в профиле — имя + издатель; без издателя — «незнакомая»; в тестовом режиме издатель читается (T6-5), но вне `WatchDir` ничего не закрывается.
3. PL-30: слияние — скрытие сохраняется, запрет сильнее скрытия (классификатор); решение родителя сильнее каталога браузеров (точный путь — шаг 3).
4. PL-40: дочерние процессы браузера — одно нарушение и одна плашка; автозапуск браузера — молча.
5. Ключ: ни одна точка записи пути программы (pending, учёт, решения, журнал, плашка) не пишет путь версии магазина; `FileMeta` получает реальный путь или ключ, который `metaReader` разворачивает.
6. Review Focus 7 плана: слияние один раз и идемпотентно; ошибка — повтор при следующем старте, база не меняется частично.
7. PL-41: `TestSeedPrograms` проходит через обработчик службы в тесте; `StoreRoot` разбирается тестом `ParseTestMode`.

## Global Constraints (дословно из плана)

- Ядро `internal/core` и новый пакет `internal/sites` — только стандартная библиотека; тесты идут локально (`GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 ./internal/core/... ./internal/sites/...`, Go 1.27 в `~/.local/go1.27`). Правила, цифры и тексты считает ядро и служба; страж, значок, окна — только показывают.
- Тексты ребёнку на «ты», родителю на «вы», без длинных тире (`python3 tools/scan_ui_texts.py`), без технических ошибок. Тексты плашек — дословно ТЗ 8.2.5.
- R23: страж читает адрес вкладки только в памяти и сразу оставляет домен; ни полный адрес, ни заголовок окна не передаются, не пишутся в журнал работы и не хранятся. Хранится только домен.
- NFR-1: ни одного сетевого соединения от программы. Список «18+» встроен в программу при сборке (D-67); загрузка UT1 — только в CI при сборке, никогда в программе.
- NFR-4 установщик ≤ 40 МБ; NFR-5 служба ≤ 50 МБ, страж ≤ 40 МБ памяти; NFR-6 вкладка закрыта ≤ 2 с, программа ≤ 1 с; ST2 проверка домена ≤ 1 мс (99-й процентиль), загрузка списка ≤ 2 с.
- Репозиторий публичный; self-hosted runner заказчика к workflow НЕ привязывать; на ПК заказчика ничего не ставить и блокировки не запускать; приёмка только в CI на windows-latest в тестовом режиме. Запуски `windows.yml` — по одному.
- Аварийный выход (NFR-10) в каждой версии: S3, S7, S25, S26 в регрессе. Новый код не может заблокировать компьютер без выхода и не закрывает окна PC Block, WebView2 и системные процессы (PL-18, PL-19).
- Наблюдатель в тестовом режиме трогает только `WatchDir`; приёмка браузеров — копиями тестового exe с именем браузера в `WatchDir` (RM3-8).
- Предупреждения о видео: пороги 30/15/5/1 (D-54), табло за 30/15/5, плашка 1 мин, звук заказчика — тот же путь, что у игр; порог взводится, только если до события строго больше его минут (PL-14).
- Плашки стража имеют старшинство (PL-34, Ruling FB14-9); новые плашки сайтов — места в RM4-9.
- Миграции — следующий номер 009; ТЗ правится только заменой (D-35); документы — коммит в main, код — ветка и PR.
- Версия 0.4.0 — `internal/version/version.go` и `wails.json` вместе (тест сверяет).
- `go.sum` обновляет CI; GraphQL закрыт — PR, слияние, выпуск через `gh api` (CLAUDE.md). Журналы Actions не скачиваются — пометки сборки (`tools/annotate-errors.sh`).
- Нестабильные: B-20 (`TestRunStartsOneGuardAndStopsOnClose`), B-47 (снимок окна) — перезапускать только упавшее задание.
- Писатель коммитит пути явно (`git add <пути>`, никогда `-A`), не пушит; пушит оркестратор. Трейлеры коммитов: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze`.
- Среда этого чата: npm-реестр из песочницы закрыт, поэтому фронт (`tsc`, `vitest`, `vite build`) проверяется только в `ci.yml` после push (Ruling RM4-0).

## Уроки (docs/LESSONS.md)

- **PL-18** (служебные процессы — «система» по точным префиксам, сухой прогон): WebView2 в любом месте не браузер; служебные Brave — по префиксу своей папки и издателю; после T6a смотреть пометку сухого прогона.
- **PL-19** (процесс узнаётся по всем признакам владельца): чтение издателя вне `WatchDir` в тестовом режиме не меняет правило «процессы раннера не трогаются».
- **PL-20** (программа в папке пользователя обновляется в новую папку; решение — имя файла + издатель): Brave в профиле — так же; приложения магазина — ключом без версии.
- **PL-21** (частичная пересборка теряет соседние поля): слияние собирает строку из всех полей группы по правилу на каждое поле (T6-11), а не копирует одну строку.
- **PL-30** (из двух флагов действует строгий): скрытие сохраняется, запрет сильнее скрытия в классификаторе; решение родителя по точному пути сильнее каталога.
- **PL-40** (решение для программы, а не процесса): браузер и его дочерние процессы — одно нарушение, автозапуск молча; ключ магазина делает «программой» все версии пакета.
- **PL-41** (непроверенная запасная ветка сломана): `TestSeedPrograms` и `StoreRoot` — тестами службы; S46 первый раз гонять одним (`only=S46`, T10).

## Коммиты

Conventional Commits, описание по-русски в повелительном наклонении; `git add <пути>` явно (никогда `-A`), **не пушить**. Каждый коммит оканчивается строками:
```
Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze
```

## Вердикт (≤ 40 строк на заход)

```
TASK T6a|T6b: done | blocked | failed
Diff: sha + строка на коммит
Evidence: go test -race -count=1 ./internal/programs/ ./internal/service/ ./internal/service/winhost/ (счётчики), st.sh (storage), go vet, gofmt -l, scan_ui_texts, RED-запуски (тест и причина), мутации (какой тест упал), grep точек ключа (T6-9)
Interfaces: для T4a/T7 (Brave в профиле — Allow/other, isBrave остаётся по суффиксу), для T10 (StoreRoot, TestSeedPrograms + перезапуск службы, текст плашки браузера, ключ магазина, издатель)
Rulings: T6-1…T6-13 подтверждены или изменены, своя цена ошибки
Escalations: изменённые ожидания старых тестов (списком, по RM4-8/RM4-11), отклонения от RM4-11 (категория — последнее решение), сигнатуры, которые пришлось менять (launchStoreApp, FileMeta → metaReader)
Open: S19, S46 (T10); README «Отклонения от ТЗ»: RM4-8 решение родителя сильнее каталога, подделка издателя Brave (PL-20), ms-resource издателя магазина — закрытие вехи
```
