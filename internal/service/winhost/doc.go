// Package winhost — Windows-обвязка службы PCBlock: всё, что внутри internal/service
// нельзя проверить без настоящей Windows (ТЗ 3.2, 3.3, план M1 T9).
//
// Логика службы (учёт, разбор команд, родительская сессия, слежение за процессами) живёт в
// internal/service на портах и проверяется тестами на любой системе. Здесь эти порты получают
// настоящие реализации, а диспетчер служб получает обработчик:
//
//   - service_windows.go, host_windows.go: svc.Handler (Start, Stop, SessionChange), сборка
//     Config службы, запуск цикла учёта и сервера канала связи, остановка с завершением
//     стражей и значков (Ruling 15);
//   - install_windows.go: --install-service и --remove-service: регистрация службы с
//     автозапуском и перезапуском при сбое через 5, 10 и 30 с, папка данных с доступом только
//     для SYSTEM и Администраторов (NFR-9), запись для безопасного режима, повторный запуск
//     безопасен (Ruling 21);
//   - testmode_windows.go: чтение HKLM\SOFTWARE\PCBlock\TestMode (Ruling 4);
//   - sessions_windows.go, proc_windows.go: SessionLauncher над winsession: сеансы, запуск
//     стража от имени системы и значка с окнами от имени пользователя, ручки процессов,
//     момент создания процесса (пара PID и момент не повторяется), WTSLogoffSession,
//     WTSDisconnectSession (смена пользователя), ExitWindowsEx (выключение);
//   - admincheck_windows.go: AdminChecker (R1, NetUserGetLocalGroups с косвенным членством) и
//     список локальных учёток для мастера;
//   - policy_windows.go: политика Диспетчера задач DisableTaskMgr (FR-29, Ruling 20), при
//     незагруженном профиле через RegLoadKey;
//   - adapters_windows.go: Store над internal/storage и Hasher над auth/pwhash;
//   - procwatch_windows.go, procinfo_windows.go: ProcessWatcher наблюдателя программ (M3):
//     процессы сеанса через Toolhelp32 и ProcessIdToSessionId, путь exe с развёрнутыми 8.3-именами
//     и junction, завершение по номеру, моменту создания и пути через одну ручку; procpath.go —
//     разбор путей без Windows;
//   - scan.go, scan_windows.go: Scanner — снимок программ Program Files и пакетов Microsoft Store
//     (RM3-10), корни компьютера из KnownFolders; filemeta.go, filemeta_windows.go: FileMeta —
//     издатель и описание из ресурса версии, отпечаток файла; dryrun.go — сухой прогон
//     наблюдателя для приёмки (RM3-8, не боевой путь);
//   - testtools_windows.go: служебные ключи приёмки --test-setup, --test-ipc, --test-runas
//     (Ruling 23: результат пишется в файл --out, у программы нет стандартного вывода);
//   - common.go, result.go: значения без Windows (дескриптор папки данных, пути, JSON результата
//     служебных ключей), их тесты идут локально; other.go: заглушки для других систем.
//
// Слежение за значком, стражем и окнами (FR-27, R17в) целиком в internal/service (sessions.go):
// здесь только ручка процесса, которая отвечает «жив ли он». Пакет вынесен из internal/service,
// чтобы порог покрытия логики службы (85 %, tools/coverage-thresholds.txt) не размывался
// вызовами Win32: серия R4 меряет все файлы пакета, собираемые на Windows.
package winhost
