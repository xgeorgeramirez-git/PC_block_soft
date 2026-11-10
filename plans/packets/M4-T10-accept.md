# Пакет: T10 вехи M4 «Приёмка сайтов, браузеров, правил Brave и магазина» | слот: писатель | исполнитель: новый агент (T10a, затем T10b)

Репозиторий `/home/claude/pc_block_soft`, ветка `feature/m4-sites`. Прочитай `plans/2026-11-04-M4.md`: Global Constraints, RM4-1…RM4-12 с итогами пробы (RM4-1-итог, RM4-2-итог, RM4-10-итог), Review Focus 2, 4, 8, строку T10. ТЗ (`docs/01_ТЗ.md`): раздел 11 — S16…S20, S46 (дословно ниже), 8.2.2, 8.2.4 п. 3, 8.2.5, R6, R13, R14, R15, R23, NFR-6, FR-31. Образцы: `tools/accept/common.ps1`, `s45-forbidden-autostart.ps1` (ручной и автозапуск, подмена переднего плана), `s12-games-limit.ps1` (лимит категории, пороги, звуки), `s15-study-only.ps1`, `s38-hidden.ps1` (`SetProgramsHidden`), `s42-stats-periods.ps1` (тестовая команда без токена), `probe-brave.ps1`, `run-all.ps1`; `.github/workflows/windows.yml`; `docs/06_Приёмка.md`; `build/windows/installer/project.nsi`. В дереве может работать писатель фронта: **`frontend/` не трогать**, `git add` — только свои пути.

Перед работой сверь адреса (PL-4): `git log --oneline -5`; `grep -n "^function" tools/accept/common.ps1`; `grep -n "textPlate\|textAdultSource" internal/service/texts.go`; `grep -n "evSite\|evBraveRestored" internal/service/sites_svc.go internal/service/brave.go`; `grep -n "ClosedTab\|ForegroundDomain" internal/ipc/commands.go`; `grep -n "PCB_ADULT_SAMPLE\|probe-site.test" .github/workflows/windows.yml`.

## ТЗ, раздел 11 (дословно)

- S16 | Дано: список 18+ включён. Когда: открыт домен из списка в Brave. Тогда: вкладка закрыта ≤ 2 с, плашка, запись в журнале
- S17 | Дано: домен в «Запрещённых». Когда: открыт поддомен. Тогда: вкладка закрыта
- S18 | Дано: лимит «Видео и соцсети» 20 мин. Когда: 20 мин на youtube.com. Тогда: вкладка закрыта; другие сайты открываются
- S19 | Когда: запуск Edge в детском сеансе. Тогда: закрыт ≤ 1 с, плашка «Этот браузер запрещён»
- S20 | Тогда: в Brave детской учётки нет пункта окна Tor и кнопки VPN; правила видны на странице `brave://policy`
- S46 | Дано: в «Программах» строки msedge_proxy из трёх папок `WindowsApps\Microsoft.MicrosoftEdge.Stable_<версия>_…` разных версий; родитель скрыл одну. Когда: служба обновилась до версии M4; затем Edge обновился в новую папку и msedge_proxy запустилась оттуда. Тогда: в «Программах» одна строка msedge_proxy, издатель Microsoft, она скрыта (решение сохранилось), в «Ждут решения» её нет; время за неделю — сумма по всем версиям (D-66)

Плашки (8.2.5, дословно): п. 1 «Сайт example.com закрыт: он в списке запрещённых»; п. 2 «Сайт закрыт: он в списке сайтов для взрослых»; п. 3 «Время на видео и соцсети на сегодня закончилось. Общего времени осталось 40 мин»; п. 4 «Этот браузер запрещён. Пользуйся Brave».

## Бриф

Шесть сценариев приёмки на настоящем Brave раннера (пользователь раннера — ребёнок) и копиях тестовой программы в `WatchDir`: «18+» (S16), поддомен «Запрещённых», учёба на сайте и запасная ветка подмены домена (S17), лимит видео (S18), другие браузеры (S19), правила Brave (S20), приложения магазина (S46); общие помощники Brave в `common.ps1`; домены приёмки в `hosts`; `LICENSE-UT1.txt` рядом с программой; строки `run-all.ps1` и `docs/06_Приёмка.md`. Код программы не меняется; дефект продукта — эскалация, а не правка.

**Разбиение (предложение читателя):** **T10a** — помощники Brave в `common.ps1`, `hosts`, S16, S17, S18 (первый живой прогон закрытия вкладки от SYSTEM — самый рискованный, гонять одним `only=S16,S17,S18`). **T10b** — S19, S20, S46, `Restart-PcbService`, `LICENSE-UT1.txt` (NSIS и шаг «Тихая установка»), итоговые строки `run-all.ps1`, `06_Приёмка.md`. T10b стартует после зелёного T10a (общие помощники).

## Найдено читателем (проверено по `d8955260`)

- **Нумерация ТЗ расходится с заданием оркестратора:** S17 в ТЗ — «поддомен запрещённого», не «учёба не считается». Ниже S16 = «18+», S17 = поддомен «Запрещённых» (+ учёба на сайте как часть (б) и подмена `domain` как (в)) — T10-1.
- **Образец «18+» уже есть:** `windows.yml:80` — `go run ./tools/adultlist … -sample "$RUNNER_TEMP/adult-sample.txt"` и `PCB_ADULT_SAMPLE=<первый>,<средний>,<последний>` в `$GITHUB_ENV` (`tools/adultlist/main.go:66-70`: порядок файлов UT1, не сортировка). Локальная склейка (`…/scratchpad/adult.txt`) даёт `0-12kids.com`, около `k9sluts.blogspot.com.by`, `zzzzzzzxxs.kali-shop.ru`; в списке 7 227 строк-IP — по IP `hosts` не работает, Brave ушёл бы в сеть (T10-3). Шаг «Brave» (`windows.yml:126`) идёт после шага «18+», переменная там уже видна.
- **`hosts` сейчас:** `probe-site.test probe-two.test probe-app.test blocked-probe.test` (`windows.yml:155`). Нужны ещё `forbidden-site.test video.forbidden-site.test study-site.test youtube.com www.youtube.com` и домены образца (без IP).
- **Brave на раннере — от choco в профиле раннера** (RM4-1-итог: `%LOCALAPPDATA%\BraveSoftware\…\brave.exe`). Сайтами служба управляет только у Brave с вердиктом «разрешено»/«скрыто» (`sites_svc.go:38` `isBrave`; правило профиля PL-20 по издателю «Brave Software»). Реестр T6: «сухой прогон без издателя считает Brave в профиле незнакомым… проверит S16» — **первый живой прогон может показать, что служба не управляет Brave**; диагностика — шаг «Brave разрешён службой» (T10-6).
- **Служба:** `closeTab{Domain: <домен вкладки>}` в каждом ответе, пока вкладка под запретом на переднем плане; плашка `ShowS: 10` не чаще 5 с на домен; запись журнала не чаще 60 с на домен (`sites_svc.go:188-260`). Не закрывает на паузе, в безопасном режиме, под экраном блокировки, у администратора. Тексты: `texts.go:69-73` (плашки), `ipc/status.go:259-330` (пороги видео: 15 «Через 15 минут время на видео и соцсети закончится», 5 «Через 5 минут время на видео и соцсети закончится», 1 «Осталась 1 минута на видео и соцсети», страж дописывает «: 0:59»), `core/texts.go:36` `VideoOverPlateText` (минуты — `RemainingMinutes`, округление вверх). Журнал (`journaltext.go:106-115, 129`): `site_forbidden_try` «попытка открыть запрещённое: сайт <домен>» / «…: сайт для взрослых <домен>», `site_closed_video` «закрыт сайт <домен>: время на видео и соцсети закончилось», `brave_restored` «правила Brave восстановлены», `site_adult_filter` «сайты 18+ не блокируются». У плашки п. 1 — **запись списка** (`entry`), у журнала — **домен вкладки**.
- **Канал:** `GuardState` — `plate`, `plateKind` (`site`, `adult`, `videoOver`, `browser`, `board`, `oneMinute`), `warnings[{threshold,text}]`, `sounds`, `foregroundPath`, `foregroundDomain` (после подмены `domain`), `closedTab`, `closedTabWay` (`ctrlW`|`invoke`|`wmClose`; **последняя исполненная**, не сбрасывается — PL-16). `GetChildStatus`: `videoRemainingS`, `videoLimitS`, `videoUnlimited`, `closeVideo`. `MyToday`: `videoS`, `studyS`, `programs[{path,name,category,seconds,kind}]` (у сайта `kind = "site"`, `path` — домен, `category` `video_social`|`study`|`other`). `WeekdayLimit`: `videoMin`, `videoUnlimited` (без ключа — без лимита, `UnmarshalJSON`). Команды родителя: `ListSites` (`sites[{domain,kind}]`, `children[{childId,name,on}]`), `AddSite{domain,kind}` → `{message,domain,moved}`, `SetAdultFilter{childId,on}`, `Pause{childId,preset:"hour1"}` (S11), `SetProgramsHidden{paths,hidden}`. Тестовые без токена (как `TestSeedDay`): `TestSeedPrograms{programs,usage,pending}` (`commands_m4.go:157-200`; сбрасывает флаг `programs.storemerged` в «0», `handlers_m4.go:224`). `TestGuard` с `domain` проходит `testActions` (`handlers.go:1146`).
- **Слияние магазина** — только при старте службы (`programs.go:119` `mergeStorePrograms`), строка журнала работы «Повторы приложений магазина слиты: N строк в M» (`programs.go:260`). `StoreRoot` читается при старте, только папка внутри `WatchDir` (`testmode.go:138-143`), подставляется в корни (`winhost/host_windows.go:71`). Папка версии — `<имя>_<версия>_<арх>_<ресурс>_<издатель>` (5 частей через «_»), ключ — `<StoreRoot>\<имя>__<издатель>\<exe>` нижним регистром (`programs/path.go:187-216`). Издатель — `Properties/PublisherDisplayName` манифеста (`winhost/scan.go:649-657`, теги без пространства имён); у `8wekyb3d8bbwe` без манифеста — «Microsoft Corporation», то есть по Edge путь «из манифеста» неотличим от запасного (T10-11). Слияние: категория — последнее решение родителя, скрыта — если скрыта хоть одна, учёт суммируется, «Ждут решения» группы при программе удаляются (`programs_merge.go:15-27`). `ProgramDTO.weekS` у скрытой — 0 (`commands_m3.go:102`).
- **Правила Brave:** значения `TorDisabled`=1, `BraveVPNDisabled`=1 (DWORD), `DnsOverHttpsMode`="off", `ExtensionInstallBlocklist\1`="*", `URLBlocklist\1..N` по алфавиту (`brave.go:39-61`). Сверка — `Housekeep` раз в настоящую секунду (`loop.go:290-300`), сама сверка раз в 60 с (`service.go:22`), не зависит от паузы часов ядра. Журнал родителя — не чаще 10 мин. `Reset-Machine` ветку уже удаляет (`common.ps1:536`), но Brave не закрывает.
- **`common.ps1`:** нет ни одного помощника Brave (они в `probe-brave.ps1`: `Find-Brave`, `Stop-Brave`, `Get-BraveWindow`); `New-DayLimit`/`Invoke-Setup`/`Start-Child` не знают видео; `Restart-Service` есть только внутри `Enable-TestMode` (перезапись `ClockRealStart`/`ClockStart`, то есть часы ядра пойдут с 15:00 снова — для S46 не годится, T10-12).
- **Размер установщика ≤ 40 МБ уже проверяется** (`windows.yml:88-99`, 41943040 байт). `LICENSE-UT1.txt` в установщик не кладётся: `project.nsi:157-159` — `SetOutPath $INSTDIR` + `!insertmacro wails.files`; удаление — `project.nsi:295-297` (`RMDir /REBOOTOK "$INSTDIR"` не удалит папку с лишним файлом). Файл `.nsi` — UTF-8 с BOM (`tools/check_nsis_bom.sh`); комментарий, оканчивающийся на `\`, съедает следующую строку (gotchas). Рабочая папка makensis — `build/windows/installer` (`OutFile "..\..\bin\…"`, `MUI_ICON "..\icon.ico"`).
- `pwsh` в песочнице нет: синтаксис `.ps1` проверяет только прогон `windows.yml` (оркестратор). Файлы `.ps1` — UTF-8 с BOM, как соседние.

## Решения читателя (оркестратор подтверждает до пуска; цена ошибки — в скобках)

- **T10-1 Нумерация по ТЗ.** S16 — «18+» (+ выключенный «18+» у ребёнка не закрывает); S17 — поддомен «Запрещённых» с плашкой 8.2.5 п. 1, (б) учёба на сайте не тратит лимит (R3, RM4-7), (в) запасная ветка подмены `domain` (PL-41). Файлы `s16-adult.ps1`, `s17-forbidden-sub.ps1`, `s18-video-limit.ps1`, `s19-browsers.ps1`, `s20-brave-rules.ps1`, `s46-store-app.ps1`. (Цена иной нумерации: строки 06_Приёмка и отчёт вехи не сходятся с ТЗ, заказчик принимает не то.)
- **T10-2 Вкладка проверяется в окне с разрешённой вкладкой.** Сначала новое окно `http://probe-site.test/` (прогрев дерева доступности: холодное до 3,5 с, RM4-1-итог), ждём `foregroundDomain == "probe-site.test"`, затем проверяемый адрес **новой вкладкой того же окна** (тот же `--user-data-dir`). Закрыто = `closedTab == <домен вкладки>` **и** `foregroundDomain == "probe-site.test"` (активной стала разрешённая вкладка — «другие сайты открываются», окно не закрыто `WM_CLOSE`). `closedTabWay` ∈ {`ctrlW`, `invoke`}: `invoke` — `::warning title=Ctrl+W от SYSTEM::` (Open T7), `wmClose` — FAIL. Время — от `Start-Process` новой вкладки, допуск раннера `$script:TabToleranceMs = 3000` (NFR-6 2000; в журнал — фактическое, как `CloseToleranceMs`). (Цена: без второй вкладки Ctrl+W закрыл бы окно, и «закрыто» нельзя отличить от «Brave упал»; допуск 2000 дал бы ложные FAIL на медленном раннере, 3000 пропустит регресс 2→3 с — видно по журналу.)
- **T10-3 Домен «18+» — из образца сборки, не литералом (PL-22).** `$env:PCB_ADULT_SAMPLE` → первый, кто подходит под `^(?!\d+(\.\d+){3}$)[a-z0-9-]+(\.[a-z0-9-]+)+$` и не оканчивается на `.test`; пусто — FAIL «PCB_ADULT_SAMPLE пуст: шаг windows.yml «Полный список 18+» не передал образец». Шаг «Brave» дописывает в `hosts` те же домены (без IP). Перед проверкой — домена нет в `ListSites.sites` (свои списки сильнее «18+», RM4-5). (Цена литерала: другой коммит UT1 — ложный FAIL; IP без фильтра — настоящий запрос в сеть с раннера.)
- **T10-4 Повторное открытие доказывается новым следствием (PL-16).** `closedTab` не сбрасывается, поэтому повтор того же домена проверяется так: дождаться, пока прежняя плашка исчезнет (`plate == ""` или другой вид, ≤ 12 с), открыть снова, ждать **ту же плашку заново** и `foregroundDomain == "probe-site.test"`. «Не закрывается» — `foregroundDomain == <домен>` непрерывно 6 с и нет плашки этого вида (`Assert-TabStays`). (Цена: проверка по `closedTab` прошла бы до события.)
- **T10-5 Родитель — только вне проверки плашек (Ruling 19).** Все `AddSite`, `SetAdultFilter`, `Pause`, `ListSites`, журнал — через `Use-Parent` до открытия вкладки или после проверки плашки. (Цена: открытая родительская сессия прячет плашки — ложный FAIL.)
- **T10-6 «Brave разрешён службой» — диагностика, а не условие.** В S16 после `Start-Child`: строка `ListPrograms` с путём Brave (`(Find-Brave).ToLowerInvariant()`) — категория, источник, издатель; в `pending` её нет. Есть в `pending` — FAIL «служба считает Brave незнакомым (правило профиля PL-20, T6-5)»; строки нет в `programs` — только запись в журнал (снимок мог Brave в профиле не взять). Итоговое доказательство — закрытие вкладки. (Цена без шага: FAIL «вкладка не закрыта» без причины, лишний прогон.)
- **T10-7 Видео: общий лимит 60 мин, видео 20, часы ×60.** Тогда после 20 мин видео общего остаётся 40 мин — текст 8.2.5 п. 3 литералом с допуском часов: `^Время на видео и соцсети на сегодня закончилось\. Общего времени осталось (39|40|41) мин$`; пороги видео 15, 5, 1 (30 не взводится при лимите ровно 20, PL-14); общие пороги не наступают (общий 30 — после 30 мин). Заголовок табло «ОСТАЛОСЬ НА ВИДЕО:» в `GuardState` не виден — снимок `s18-video-board` на пороге 15 разбирает `visual-reviewer`. (Цена общего 120: текст «1 ч 40 мин» не совпал бы с примером ТЗ.)
- **T10-8 Запасная ветка `domain` (PL-41)** — S17 (в): настоящая вкладка `probe-two.test` на переднем плане, `Invoke-TestGuard "domain" "" "forbidden-site.test"` → служба просит закрыть `forbidden-site.test`, страж (настоящий Brave на переднем плане) закрывает **настоящую** активную вкладку; ждём `closedTab == "forbidden-site.test"` и сразу снимаем подмену (`Invoke-TestGuard "domain"`). Шаг последний в S17. (Цена: пока подмена висит, страж закрывает вкладку за вкладкой — поэтому снять сразу; закрытие лишней вкладки допустимо.)
- **T10-9 R23 в приёмке (Review Focus 8).** S17 (а) открывает `http://video.forbidden-site.test/pcb-secret-path?q=pcb-secret-query`; после закрытия: ни в одном файле `$script:Data\logs\*`, ни в тексте журнала родителя нет `pcb-secret`. (Цена без шага: утечка адреса страницы в журнал работы не ловится ничем, кроме ревью.)
- **T10-10 S20 «нет VPN» — с контролем (PL-12, PL-16).** До `Setup` (ребёнка нет, правил нет) Brave со свежим профилем: UIA (`System.Windows.Automation`, `Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes` в pwsh 7) — кнопки окна, имя содержит `VPN` без учёта регистра ≥ 1 (контроль; нет — `::warning title=S20 контроль VPN::` и проверка VPN только по реестру). После правил — кнопок ≥ 5 (дерево готово, ждать до 15 с) и с `VPN` 0. Tor: `Alt+Shift+N` (`[Windows.Forms.SendKeys]::SendWait("%+n")`) в окне Brave — до правил новое окно Brave появляется (контроль, мягкий, как VPN), после — окон не прибавилось за 5 с; жёстко — `TorDisabled = 1` в ветке. `brave://policy` — снимок `s20-brave-policy` (UIA его не читает, RM4-10-итог). `Add-Type` UIA не загрузилась — FAIL с текстом ошибки (запасного пути нет: PL-41). (Цена без контроля: «нет кнопки VPN» зелёно и в версии Brave, где кнопки нет вообще.)
- **T10-11 S20 URLBlocklist — сервером на порту 8765 и паузой.** Порт 80 на раннере не поднимается (RM4-10-итог), поэтому `TcpListener` на `127.0.0.1:8765` (в `Start-ThreadJob`) отдаёт `<title>probe-ok</title>` и пишет заголовок `Host` каждого запроса. Пауза родителя (`Pause hour1`) — служба на паузе вкладки не закрывает (`sites_svc.go`, `StatusPaused`), поэтому страницу Brave видно. Контроль: `http://probe-site.test:8765/` — заголовок окна Brave начинается с `probe-ok` за 10 с; затем `http://blocked-probe.test:8765/` — `foregroundDomain == "blocked-probe.test"`, заголовок окна 3 с не начинается с `probe-ok`, и у сервера нет ни одного запроса с `Host: blocked-probe.test`. Контроль не прошёл (Brave повышает до https или не грузит) — FAIL с заголовком окна (не обходить). (Цена: без паузы страж закроет вкладку раньше, чем видна страница; без контроля «не грузится» неотличимо от «сервер не отвечает».)
- **T10-12 S46 — свой перезапуск службы.** Новый `Restart-PcbService` в `common.ps1`: `Close-PipeConn`, `Restart-Service PCBlock -Force`, ожидание `Running` 40 с и `Hello.testMode` 40 с (как `Enable-TestMode:498-506`), **без** перезаписи значений `TestMode` (часы ядра продолжаются, PL-41 этого пути нет — он первый раз исполняется здесь, гонять `only=S46` одним). `StoreRoot` = `$script:Apps\WindowsApps` (внутри `WatchDir`) пишется в `TestKey` и включается тем же `Restart-PcbService` до `Setup`. Издатель «из манифеста» доказывается вторым пакетом `Contoso.Notes_1.0.0.0_x64__abcd1234efgh` с `PublisherDisplayName` «Contoso Ltd» (у Edge манифест неотличим от запасного «Microsoft Corporation»). Манифесты — без `<Applications>`, чтобы снимок `Setup` не добавил строк сам. (Цена `Enable-TestMode` для перезапуска: часы назад, `ExpectFresh` и сброс состояния; без Contoso чтение манифеста в приёмке не проверено.)
- **T10-13 S19: ручной и автозапуск по образцу S45; решение родителя сильнее каталога (RM4-8).** `msedge.exe`, `chrome.exe`, `firefox.exe` — `New-TestApp`; `firefox.exe` — «Прочее» шагом мастера (источник `parent` — сверить по `ListPrograms`), она работает. Автозапуск: `msedge.exe` при простое 30 с и Проводнике на переднем плане — закрыт, плашки нет, `program_auto_closed` 1. Ручной: `msedge.exe`, `chrome.exe` при вводе и PowerShell на переднем плане — закрыты, плашка «Этот браузер запрещён. Пользуйся Brave» (`browser`), `program_forbidden_try` по одной на программу. Имя в журнале — из ответа программы (`Get-ProgramName` после запуска, PL-22); считать по `kind`. (Цена без `firefox`: «решение родителя сильнее каталога» в приёмке не проверено; без автозапуска R13 для браузеров не проверено.)
- **T10-14 `LICENSE-UT1.txt` рядом с exe.** `project.nsi` после `!insertmacro wails.files`: `File "..\..\..\internal\sites\adult\LICENSE-UT1.txt"`; в `Section "uninstall"` перед `Delete /REBOOTOK "$INSTDIR\${PRODUCT_EXECUTABLE}"`: `Delete "$INSTDIR\LICENSE-UT1.txt"`; комментарий по-русски, не оканчивается на `\`; BOM сохранить (`bash tools/check_nsis_bom.sh`). Шаг «Тихая установка и удаление» (`windows.yml:206`): после проверки папки — `$lic = Join-Path $dir "LICENSE-UT1.txt"`, нет файла или нет `CC BY-SA 4.0` — `::error`; проверка «осталось: папка» после удаления уже ловит недостающий `Delete`. Размер ≤ 40 МБ уже есть — не трогать. (Цена: без `Delete` удаление оставит `Program Files\PCBlock` — шаг удаления красный.)

## Files

T10a — Create: `tools/accept/s16-adult.ps1`, `tools/accept/s17-forbidden-sub.ps1`, `tools/accept/s18-video-limit.ps1`. Modify: `tools/accept/common.ps1` (помощники Brave, видео в лимитах, `Reset-Machine`), `tools/accept/probe-brave.ps1` (убрать свои `Find-Brave`/`Stop-Brave`/`Get-BraveWindow` — теперь в `common.ps1`), `tools/accept/run-all.ps1`, `.github/workflows/windows.yml` (только строка `hosts` шага «Brave» и описания `scenario`/приёмки), `docs/06_Приёмка.md`.
T10b — Create: `tools/accept/s19-browsers.ps1`, `tools/accept/s20-brave-rules.ps1`, `tools/accept/s46-store-app.ps1`. Modify: `tools/accept/common.ps1` (`Restart-PcbService`, UIA-помощники), `tools/accept/run-all.ps1`, `build/windows/installer/project.nsi`, `.github/workflows/windows.yml` (шаг «Тихая установка»), `docs/06_Приёмка.md`.
Не трогать: `internal/`, `frontend/`, `main.go`, `docs/` (кроме 06), `plans/`, `ci.yml`, `full-series.yml`.

## Interfaces (новое в `common.ps1`; сигнатуры — писателю как цель)

```powershell
$script:BraveProfile = Join-Path $script:Work "brave-profile"
$script:TabToleranceMs = 3000             # NFR-6: вкладка ≤ 2000 мс, допуск раннера 3000 мс
function Find-Brave                        # из probe-brave.ps1 как есть: путь brave.exe или ""
function Stop-Brave                        # из probe-brave.ps1; ничего не бросает
function Get-BraveWindow                   # из probe-brave.ps1: "pid|класс|рамка|заголовок" или $null
function Get-BraveHandle                   # [IntPtr] MainWindowHandle первого brave с окном, иначе Zero
function Get-BraveTitle                    # заголовок окна переднего плана Brave (только приёмка; R23 — о программе)
function Open-Brave([string]$Url, [switch]$NewWindow)   # Start-Process от имени ребёнка (раннера):
    # --no-first-run --no-default-browser-check --user-data-dir=$script:BraveProfile [--new-window] $Url;
    # возвращает Stopwatch с момента запуска (замер NFR-6)
function Set-BraveForeground               # ForceForeground(Get-BraveHandle), иначе AppActivate (как PROBE) 10 раз;
    # ждёт GuardState.foregroundPath, оканчивающийся на \brave.exe, 5 с
function Wait-BraveDomain([string]$Domain, [double]$Seconds = 15)   # Wait-GuardState foregroundDomain -eq
function Wait-TabClosed([string]$Domain, $Since, [string]$Remain = "probe-site.test")
    # ждёт closedTab -eq Domain и foregroundDomain -eq Remain до 8 с; ms = $Since.ElapsedMilliseconds;
    # журнал «вкладка X закрыта за N мс (NFR-6 ≤ 2000, допуск 3000), способ W»; Assert ms ≤ TabToleranceMs,
    # way ∈ ctrlW|invoke (invoke — ::warning); возвращает состояние
function Assert-TabStays([string]$Domain, [double]$Seconds = 6, [string]$NoPlateKind = "")
function Add-PcbSite([string]$Domain, [string]$Kind)   # Use-Parent AddSite; Assert-Reply; возвращает payload.domain
function Restart-PcbService                # T10b, T10-12
# Видео в лимитах (обратная совместимость: по умолчанию без лимита видео):
function New-DayLimit([int]$Weekday, [int]$TotalMin, [int]$GamesMin = -1, [int]$VideoMin = -1)
    # + videoMin = max(VideoMin,0); videoUnlimited = (VideoMin -lt 0)
function Invoke-Setup(... , $Programs = $null, [int]$VideoMin = -1)   # передаёт в New-DayLimit
function Start-Child([int]$LimitMin, [bool]$Pinging = $true, [int]$GamesMin = -1, $Programs = $null, [int]$VideoMin = -1)
```

`Reset-Machine`: `try { Stop-Brave } catch {}` до удаления папок и `Remove-Item $script:BraveProfile -Recurse -Force -ErrorAction SilentlyContinue` (ничего не бросает). `windows.yml:155` → `adult=$(printf '%s' "$PCB_ADULT_SAMPLE" | tr ',' '\n' | grep -Ev '^[0-9.]+$' | tr '\n' ' ')` и `printf '\r\n127.0.0.1 probe-site.test probe-two.test probe-app.test blocked-probe.test forbidden-site.test video.forbidden-site.test study-site.test youtube.com www.youtube.com %s\r\n' "$adult" >> …/hosts`, `echo "hosts: …"` в журнал.

## Сценарии (каждый: `Invoke-Scenario`, шаги `Step`, ожидание по содержимому; PASS — дошёл до конца)

Общее начало S16…S18: `Initialize-Scenario 60`; `Test-InputSim`; `Find-Brave` непустой (иначе FAIL «шаг windows.yml «Brave» не поставил Brave»), `Stop-Brave`; `Start-Child …` (часы на паузе); `Open-Brave "http://probe-site.test/" -NewWindow`; `Wait-Until { Get-BraveWindow } 20`; `Set-BraveForeground`; `Wait-BraveDomain "probe-site.test" 15`.

### S16 «18+» (`s16-adult.ps1`, ≈ 3 мин) — T10a
Дано: лимит 120 мин; «18+» у ребёнка включён по умолчанию (`ListSites.children[0].on -eq $true`); домен A по T10-3, не в `ListSites.sites`. Шаг T10-6.
1. Когда: `$sw = Open-Brave "http://$A/"`. Тогда: `Wait-TabClosed $A $sw`; `Wait-GuardState { $s.plate -eq "Сайт закрыт: он в списке сайтов для взрослых" } 5` → `plateKind -eq "adult"`, `plate` не содержит A; `Save-Screen "s16-adult-plate"`.
2. Журнал (`Use-Parent`, ждать до 5 с): записей `kind -eq "site_forbidden_try"` с `text` содержит `"попытка открыть запрещённое: сайт для взрослых $A"` — ровно 1 (closeTab повторялся, запись — раз в 60 с, PL-12).
3. Когда: `SetAdultFilter @{ childId = (Get-ChildId); on = $false }` (`Assert-Reply`), в журнале «сайты 18+ не блокируются»; ждать, пока плашка `adult` исчезнет (≤ 12 с, T10-4); `Open-Brave "http://$A/"`. Тогда: `Wait-BraveDomain $A 10`, `Assert-TabStays $A 6 "adult"`; нарушений про A по-прежнему 1.

### S17 поддомен «Запрещённых», учёба, подмена домена (`s17-forbidden-sub.ps1`, ≈ 3 мин) — T10a
Дано: лимит 120; `$F = Add-PcbSite "forbidden-site.test" "blocked"` (ждём `$F -eq "forbidden-site.test"`), `$S = Add-PcbSite "study-site.test" "study"` — **до** открытия Brave (T10-5).
(а) Когда: `$sw = Open-Brave "http://video.forbidden-site.test/pcb-secret-path?q=pcb-secret-query"`. Тогда: `Wait-TabClosed "video.forbidden-site.test" $sw`; плашка `"Сайт $F закрыт: он в списке запрещённых"` (из ответа `AddSite`, PL-22), `plateKind "site"`; `Save-Screen "s17-site-plate"`; журнал: 1 запись `site_forbidden_try` с `"попытка открыть запрещённое: сайт video.forbidden-site.test"`; R23 (T10-9): `Select-String -Path "$($script:Data)\logs\*" -Pattern "pcb-secret" -SimpleMatch` пусто и `Format-Events` без `pcb-secret`.
(б) Учёба: `Open-Brave "http://$S/"`, `Wait-BraveDomain $S`; `$st0 = Get-Status; $m0 = Get-MyToday`; `Resume-Clock`; `Start-Sleep 10` (≈ 10 мин ядра); `Suspend-Clock`; `$st1`, `$m1`. Тогда: `[Math]::Abs($st1.remainingS - $st0.remainingS) -le 60` (учёба не тратит лимит, R3); `$m1.studyS - $m0.studyS` в 540…660; в `$m1.programs` строка `kind "site"`, `path $S`, `category "study"`, `seconds ≥ 540` (RM4-7); `closedTab` не стал `$S`.
(в) Подмена (T10-8): `Open-Brave "http://probe-two.test/"`, `Wait-BraveDomain "probe-two.test"`; `Invoke-TestGuard "domain" "" $F`; `Wait-GuardState { $s.closedTab -eq $F } 6`; сразу `Invoke-TestGuard "domain"`; в журнал — `closedTabWay`; журнал родителя: запись `"сайт $F"` ≥ 1.

### S18 лимит видео (`s18-video-limit.ps1`, ≈ 3 мин) — T10a
Дано: `Start-Child 60 $true -1 $null 20`; `GetChildStatus`: `videoLimitS -eq 1200`, `videoRemainingS -eq 1200`, `-not videoUnlimited`, `-not closeVideo`; `ListSites.sites` содержит `youtube.com` / `video_social` (стартовый набор R14 — из ответа программы). После общего начала — `Open-Brave "http://youtube.com/"`, `Wait-BraveDomain "youtube.com"`.
1. Когда: `Resume-Clock`. Тогда: `Wait-GuardState { (Get-WarnThresholds $s) -contains 15 } 30` → сразу `Save-Screen "s18-video-board"` (заголовок «ОСТАЛОСЬ НА ВИДЕО:», плитки, текст — `visual-reviewer`); затем `Wait-GuardState { … -contains 1 } 30`: пороги ровно `"15,5,1"`; `w[0].text -eq "Через 15 минут время на видео и соцсети закончится"`, `w[1].text -eq "Через 5 минут время на видео и соцсети закончится"`, `w[2].text.StartsWith("Осталась 1 минута на видео и соцсети")`; звуки `"15,5,1"`; `-not lockShown`.
2. Когда: `$st = Wait-Status { $_.closeVideo } 60`; `$since = Stopwatch`; `Suspend-Clock`. Тогда: `Wait-TabClosed "youtube.com" $since`; плашка по регулярному выражению T10-7, `plateKind "videoOver"`; `-not lockShown`; `Save-Screen "s18-video-over"`; статус: `remainingS` 2340…2460, `videoRemainingS -eq 0`; `MyToday.videoS` 1140…1200; строка `programs` `kind "site"`, `path "youtube.com"`, `category "video_social"`.
3. Другие сайты: `Assert-TabStays "probe-site.test" 6 "videoOver"` (после исчезновения плашки, T10-4). Повтор: плашка ушла → `Open-Brave "http://youtube.com/"` → та же плашка `videoOver` заново ≤ 5 с и `foregroundDomain -eq "probe-site.test"`.
4. Журнал: `site_closed_video` с `"закрыт сайт youtube.com: время на видео и соцсети закончилось"` ≥ 1; `site_forbidden_try` 0; `попытка обойти защиту` 0.

### S19 другие браузеры (`s19-browsers.ps1`, ≈ 2,5 мин) — T10b
Дано (T10-13): `Initialize-Scenario 60`; `Test-InputSim`; `New-TestApp "msedge"`, `"chrome"`, `"firefox"`; `Start-Child 120 $false -1 @(@{ path = $FirefoxPath; category = "other" })`; `Wait-ProgramsReady`; строка `firefox.exe` в `ListPrograms` — `category "other"` (источник — в журнал). `Set-ForegroundPath` — как в S45 (локальная функция).
(а) Автозапуск: `Stop-Input`; передний план `explorer.exe`; простой ≥ 30 с (`Wait-Until … 60`); `msedge.exe` — `Assert-ClosedFast` (≤ 1500 мс); 3 с нет плашки `browser` и текста «Этот браузер запрещён»; через 3 с журнал: `program_auto_closed` 1, `program_forbidden_try` 0.
(б) Ручной: `Start-Input 1000`; передний план `$script:PsExe`; ввод < 2 с; `Start-Sleep 2`; `msedge.exe` — `Assert-ClosedFast`; `Wait-GuardState { $s.plate -eq "Этот браузер запрещён. Пользуйся Brave" } 6` → `plateKind "browser"`; `Save-Screen "s19-browser-plate"`. Затем `chrome.exe` — `Assert-ClosedFast`. Журнал: `program_forbidden_try` ровно 2 (по одной на программу), тексты начинаются с «попытка открыть запрещённое: «»; `program_auto_closed` по-прежнему 1.
(в) `firefox.exe` (решение родителя): `Start-TestApp`, `Start-Sleep 3`, `Assert-AppAlive`; плашки `browser` нет.
Снять подмену переднего плана (`Invoke-TestGuard "foreground"`).

### S20 правила Brave (`s20-brave-rules.ps1`, ≈ 4 мин) — T10b
Ветка: `$B = "Registry::HKEY_USERS\$($script:MySid)\Software\Policies\BraveSoftware\Brave"`.
1. Контроль до правил (T10-10): `Initialize-Scenario 60` (ребёнка нет); `Test-Path $B` — `$false`; Brave новым окном на `probe-site.test`; UIA-кнопки (до 15 с, пока их ≥ 5): `VPN` ≥ 1 → `$script:VpnControl = $true`, иначе `::warning`; `Alt+Shift+N` → окон Brave прибавилось за 5 с → `$script:TorControl`; `Stop-Brave`; профиль удалить.
2. Когда: `Start-Child 120`; `Add-PcbSite "blocked-probe.test" "blocked"`. Тогда (≤ 10 с, `Wait-Until`): `TorDisabled` 1 и `BraveVPNDisabled` 1 типа DWord, `DnsOverHttpsMode` `"off"`, `$B\ExtensionInstallBlocklist` `1` = `"*"`, `$B\URLBlocklist` `1` = `"blocked-probe.test"`.
3. Brave новым окном (свежий профиль) на `probe-site.test`, `Set-BraveForeground`: кнопок ≥ 5, с `VPN` 0 (при `VpnControl`); `Alt+Shift+N` — окон не прибавилось за 5 с (при `TorControl`); `Save-Screen "s20-brave-toolbar"`.
4. URLBlocklist (T10-11): `Use-Parent { Pause hour1 }`; сервер 8765; `Open-Brave "http://probe-site.test:8765/"` — заголовок окна начинается с `probe-ok` за 10 с; `Open-Brave "http://blocked-probe.test:8765/"` — `Wait-BraveDomain "blocked-probe.test"`, 3 с заголовок не `probe-ok…`, у сервера нет `Host: blocked-probe.test`; остановить сервер.
5. `Open-Brave "brave://policy" -NewWindow`; `Start-Sleep 4`; `Save-Screen "s20-brave-policy"` (пять правил со статусом «OK» — `visual-reviewer`); `Stop-Brave`.
6. Стирание ребёнком: `Remove-Item $B -Recurse -Force` (от имени раннера = ребёнка); `-not (Test-Path $B)`; `$sw`; `Wait-Until { все значения шага 2 } 75`; журнал в лог (`Assert-Between $sw 0 65`, сверка раз в 60 с); журнал родителя: `brave_restored` с текстом «правила Brave восстановлены» ровно 1; `service.log` — строка `правила Brave у Тест восстановлены`.

### S46 приложение магазина (`s46-store-app.ps1`, ≈ 3 мин) — T10b
Константы: `$V = "128.0.2739.42","129.0.2792.52","130.0.2849.46"`, новая `"131.0.2903.51"`; папка `Microsoft.MicrosoftEdge.Stable_<v>_neutral__8wekyb3d8bbwe`; манифест (UTF-8 без BOM): `<?xml version="1.0" encoding="utf-8"?><Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"><Properties><DisplayName>Microsoft Edge</DisplayName><PublisherDisplayName>Microsoft Corporation</PublisherDisplayName></Properties></Package>`; exe — `Copy-TestExe (Get-TestAppSource) <папка> "msedge_proxy"`. Contoso: `Contoso.Notes_1.0.0.0_x64__abcd1234efgh\notes.exe`, издатель «Contoso Ltd».
1. Дано: `Initialize-Scenario 60`; папки трёх версий и Contoso в `$Store = $script:Apps\WindowsApps`; `New-ItemProperty $script:TestKey StoreRoot <длинный путь>`; `Restart-PcbService`; `Start-Child 120 $false`; `Wait-ProgramsReady`; в `service.log` нет «StoreRoot должен быть папкой».
2. «До обновления»: `Send-Pcb "TestSeedPrograms"` (`Assert-Reply`): программы — v128 `other`/`snapshot`, v129 `other`/`parent` `hidden = $true` `decidedAt = 1760000000000`, v130 `other`/`snapshot`, у всех `name "msedge_proxy"`, `publisher ""`; учёт ребёнка (`Get-ChildId`) `other`: v128 600 с `2026-10-16`, v129 300 с `2026-10-17`, v130 120 с `2026-10-18`; ждут решения: v130, попыток 2, `lastAttemptAt 1760000000000`. Проверка до: в `ListPrograms` три строки msedge_proxy (PL-16).
3. Когда: `Restart-PcbService`; `Wait-Until` строки `Повторы приложений магазина слиты` в `service.log` 30 с. Тогда (`ListPrograms`): строк с `msedge_proxy.exe` и `microsoft.microsoftedge.stable` — 1; `path -eq "$($Store.ToLowerInvariant())\microsoft.microsoftedge.stable__8wekyb3d8bbwe\msedge_proxy.exe"`; `publisher -eq "Microsoft Corporation"`; `hidden -eq $true`; `category -eq "other"`; в `pending` строк msedge_proxy нет.
4. Edge обновился: папка v131 с манифестом и `msedge_proxy.exe`; `Start-TestApp` из неё; `Start-Sleep 3`; `Assert-AppAlive` (скрытая, «Прочее», ключ тот же); снова одна строка, в `pending` нет.
5. Сумма за неделю: `SetProgramsHidden @{ paths = @(<ключ>); hidden = $false }`; строка ключа `weekS -eq 1020` (600 + 300 + 120; у скрытой 0, `commands_m3.go:102`).
6. Издатель из манифеста (T10-12): `Start-TestApp` Contoso `notes.exe` → `Assert-ClosedFast` (незнакомая); в `pending` строка `path` `"…\contoso.notes__abcd1234efgh\notes.exe"` с `publisher -eq "Contoso Ltd"`.

### Регресс и строки
- `run-all.ps1`: после `S45` — `S16 s16-adult.ps1`, `S17 s17-forbidden-sub.ps1`, `S18 s18-video-limit.ps1`, `S19 s19-browsers.ps1`, `S20 s20-brave-rules.ps1`, `S46 s46-store-app.ps1`; `PROBE` и `S33` — как были (S33 последним); шапка и итоговый `::notice` — «S1…S20, S25…S27, S33…S46, LE1, LE2, FB1, FB5».
- `docs/06_Приёмка.md`: заменить `- [ ] S16, S17, S18, S19, S20` разделом: строка «Автоприёмка: `gh workflow run windows.yml --ref <ветка> -f scenario=accept -f only=S16,S17,S18,S19,S20,S46`» и по строке `- [ ] S16 … (CI: …; снимки …)` на каждый сценарий в формате соседних строк (что доказано в CI, что вручную на запасном ПК: настоящий Edge из «Пуска», настоящий YouTube 20 мин, меню Brave руками); `S46`, `LICENSE-UT1.txt` рядом с программой; регресс M4: S41…S45, S12…S15, S2, S35, S3, S7, S25, S26 (номера прогонов вписывает оркестратор, галочки — после зелёного). В шапке «Сценарии S1…S43» → «S1…S46».

## Как запускать (только оркестратор, по одному)

`gh workflow run windows.yml --ref feature/m4-sites -f scenario=accept -f only=S16,S17,S18` (T10a), затем `-f only=S19,S20,S46` (T10b, `LICENSE` — шаг «Тихая установка» в том же прогоне), затем полный `-f scenario=accept` перед закрытием вехи. Первый прогон нового сценария с запасной веткой — одним (`only=S17`, `only=S46`, PL-41). Пометки: `gh api repos/xgeorgeramirez-git/PC_block_soft/check-runs/<job-id>/annotations`; снимки — ветка `screenshots`, папка `screens/feature-m4-sites/`.

## Review Focus (самопроверка писателя)

1. PL-16: каждое ожидание — признак, которого не было до события (`closedTab` нового домена, плашка после исчезновения прежней, строка `service.log` после перезапуска, счётчик журнала).
2. PL-22: тексты плашек — литералы ТЗ/служб с указанием файла в комментарии; домен плашки п. 1 — из ответа `AddSite`; домен «18+» — из образца сборки; имя программы — `Get-ProgramName`.
3. PL-12: «ровно 1 запись» — после повторов closeTab; автозапуск и ручной в S19 — разные `kind`.
4. Ruling 19: ни одной родительской сессии во время ожидания плашки (`Use-Parent` закрывает сразу).
5. Ни одного запроса в сеть: все адреса — домены из `hosts` или `127.0.0.1:8765`; IP из образца отфильтрованы; Brave без входа в аккаунт и синхронизации (`--no-first-run`).
6. Чистота: `Reset-Machine` закрывает Brave и удаляет профиль; подмены `foreground`/`domain` снимаются в своём шаге; сервер 8765 останавливается в `finally` шага.
7. Сообщения FAIL содержат `Format-GuardState`, `closedTab`, `closedTabWay`, `foregroundDomain`, `Format-Events` и строки `service.log` про сайты (`sites|сайт|Brave`) — журналы Actions не скачиваются (PL-1).

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

## Уроки класса E (docs/LESSONS.md)

- **PL-16** (ждать содержимое, а не окно; общее состояние убирать): `closedTab` — последняя исполненная, не сбрасывается — повтор доказывается плашкой после её исчезновения (T10-4); Brave, профиль, подмены, сервер, ветка реестра — убираются в конце.
- **PL-22** (ищем то, что пишет программа): плашка п. 1 с записью списка, журнал — с доменом вкладки; домен «18+» — образец сборки; имя браузера в журнале — из `ListPrograms`; тексты — ссылкой на файл службы в комментарии сценария.
- **PL-27** (красный ранний шаг скрывает непрогнанное; точное совпадение): шаги S20 и S46 независимы по смыслу — контрольные `::warning` не роняют сценарий, но FAIL раннего шага скрывает поздние: при FAIL оркестратор проверяет пропущенные шаги отдельным `only=`; сравнения строк — `-eq`/`StartsWith`, не `-match` по подстроке (кроме регулярного выражения T10-7 с якорями).
- **PL-41** (запасную ветку, которую не исполняли, считать сломанной): подмена `domain` исполняется в S17 (в); `Restart-PcbService` и `TestSeedPrograms` впервые исполняются в S46 — первый прогон `only=S46` одним; запасного пути у UIA в S20 нет намеренно.

## Коммиты

Conventional Commits, описание по-русски в повелительном наклонении, пути явно (`git add tools/accept/s16-adult.ps1 tools/accept/common.ps1 …`), не пушить, трейлеры:
```
Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze
```
T10a: `test: помощники Brave и видео-лимит в общих функциях приёмки`, `ci: домены приёмки сайтов в hosts раннера`, `test: приёмка S16 сайты 18+ в Brave`, `test: приёмка S17 поддомен запрещённого сайта, учёба и подмена домена`, `test: приёмка S18 лимит видео и соцсетей`. T10b: `test: приёмка S19 другие браузеры`, `test: приёмка S20 правила Brave`, `test: приёмка S46 одна строка приложения магазина`, `build: лицензия списка 18+ рядом с программой`, `test: сценарии M4 в общем прогоне и чек-листе приёмки` (`06_Приёмка.md` — в ветку вместе с `run-all.ps1`, как в плане; перенос в main — решение оркестратора).

## Open (передать дальше)

- `SendInput` от SYSTEM (Open T7): итог — `closedTabWay` в S16 (`ctrlW` или `invoke` с `::warning`); при `invoke` — строка в README «Отклонения от ТЗ» не нужна, но факт — в отчёт вехи.
- Если S16 покажет, что служба не управляет Brave из профиля (T10-6) — дефект T6 (правило профиля по издателю), возврат агенту T6, не правка в T10.
- Значок «Видео: N мин» (UX-сценарий плана) приёмкой не проверяется: юнит `TestDisplayForLimitLines` (T7); снимок значка — по желанию `Save-TrayCorner` в S18 после шага 1.
- «Сверка без расхождения журнал не пишет» (Review Focus 6) — юнит-тест службы; в S20 не ждём лишние 60 с.

## Вердикт (≤ 40 строк)

```
TASK T10a|T10b: done | blocked | failed
Diff: sha + строка на коммит
Evidence: что проверено локально (BOM .ps1/.nsi — tools/check_nsis_bom.sh, scan_ui_texts, чтение глазами: pwsh в песочнице нет); что ждёт прогона windows.yml (only=…)
Interfaces: новые функции common.ps1 (точные имена и параметры), изменённые New-DayLimit/Invoke-Setup/Start-Child, строка hosts
Rulings: T10-1…T10-14 подтверждены или изменены, своя цена ошибки
Escalations: расхождения с продуктом, найденные при чтении кода (тексты, kind, поля), правки вне Files
Open: что проверяется только вручную на запасном ПК; что оркестратор должен проверить по пометкам первого прогона
```

## Решения оркестратора (новый чат, 06.11.2026)

- T10-1…T10-14 утверждены как предложены. Разбиение T10a / T10b — утверждено; T10a начинается заново (в дереве от прошлого писателя ничего нет).
- Первый прогон T10a — `only=S16` одним (первое живое закрытие вкладки от SYSTEM, PL-41); затем `only=S17`, затем `only=S18`. Прогоны запускает только оркестратор.
- Трейлер сессии в коммитах нового чата: `Claude-Session: https://claude.ai/code/session_01XuHYMfbECJBCZpbwSnf2sU` (вместо ссылки выше).
- Если S16 покажет, что служба не управляет Brave из профиля — `blocked` с Escalations; продукт не правим в T10.
