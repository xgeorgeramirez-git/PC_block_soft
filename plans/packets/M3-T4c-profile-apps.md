# Пакет M3-T4c: программы в профиле пользователя и компоненты Защитника (RM3-13) | слот: писатель | дерево: `/home/claude/pc_block_soft`, ветка `feature/m3-programs`

## Контекст

PC_block_soft — родительский контроль для Windows 11. Веха M3: незнакомые программы ждут решения родителя (R12). Прочитай `plans/2026-10-29-M3.md` (RM3-1, RM3-12, **RM3-13**) и `git log --stat -30` (T3b `internal/programs`, T4a служба-наблюдатель, T4b Windows-адаптер). Сухой прогон на windows-latest (прогон 40954826900) показал «незнакомыми»: `c:\programdata\microsoft\windows defender\platform\4.18.26090.9-0\defendersessionhelper.exe`, а у заказчика так же будут программы в `%LOCALAPPDATA%`/`%APPDATA%` (OneDrive, Discord, Roblox, лаунчеры), которые обновляются в новые папки версий — решение родителя потерялось бы при каждом обновлении.

Отвечай по-русски. Методика: TDD (литералы, мутации, маленькие коммиты).

## 0. Среда

Дерево `/home/claude/pc_block_soft`, ветка `feature/m3-programs`; ты единственный писатель. `plans/`, ТЗ не правь. Коммить пути явно; не пушь. Go: `export PATH=/home/claude/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off`; `go test -race -count=1 ./internal/programs/... ./internal/service/...`, `gofmt`, `go vet`, Windows — кросс-сборкой (`scratchpad/go.winvet.mod`). Трейлеры `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01XuHYMinhbS8zjLqh6nAgCY`.

## 1. Что сделать

1. `programs.Roots` получает `ProgramData` и список профилей (`LocalAppData`, `RoamingAppData` — для сеанса ребёнка путь профиля приходит с процессом, поэтому определяй корни профиля **по форме пути**: `c:\users\<имя>\appdata\local\`, `…\appdata\roaming\`, `…\appdata\local\programs\`; имя профиля любое, `c:\users\` берётся из KnownFolder `UserProfiles`/`%SystemDrive%\Users`).
2. Класс «система»: точные префиксы `<ProgramData>\microsoft\windows defender\` и `<ProgramData>\microsoft\windows defender advanced threat protection\` (граница разделителя; `…\windows defender games\` — НЕ система). Таблица тестов.
3. Корень приложения в профиле: первая папка под `appdata\local\` или `appdata\roaming\`; под `appdata\local\programs\` — вторая (`…\programs\discord\`); `appdata\local\microsoft\` — вторая (`…\microsoft\onedrive\`, `…\microsoft\teams\`), `appdata\local\packages\` — не корень (данные Store, exe там не запускаются; Unknown).
4. Наследование решения родителя в корне профиля: exe с **тем же именем файла** и **тем же издателем** (`CompanyName` через `FileMeta`; пустой издатель у решения или у exe — НЕ наследовать) внутри того же корня получает категорию решения (все категории, включая `blocked`); иной exe → Unknown. Запись решения хранит `AppRoot` (корень профиля) и издателя (поле `publisher` в `programs`, уже есть). Отпечаток сверяется только у точного пути (как сейчас). В «Загрузках», на рабочем столе и прочих папках — только точный путь с отпечатком.
5. `Classify` получает издателя запускаемого exe (служба берёт его из `FileMeta` наблюдателя; кэш по `PID+Stamp` уже есть, издатель читай только для путей в корнях профиля и только если в этом корне есть решения — не замедляй опрос).
6. Тесты (литералы): Roblox `…\local\roblox\versions\version-111\robloxplayerbeta.exe` «Игра» → `version-222\robloxplayerbeta.exe` того же издателя → Игра; другой издатель → Unknown; `game.exe` рядом → Unknown; Discord `…\local\discord\app-1.0.9001\discord.exe` «Прочее» → `app-1.0.9002\discord.exe` → Прочее; `…\local\programs\cursor\cursor.exe`; OneDrive; защитник → System; `…\windows defender games\x.exe` → Unknown. Мутации: убрать сверку издателя → падает «другой издатель»; наследовать по корню без имени → падает `game.exe` рядом; убрать границу префикса защитника → падает `windows defender games`.

## 2. Review Focus

1. Обход: ребёнок кладёт `game.exe`, переименованный в `discord.exe`, в папку Discord — издатель у его файла другой/пустой → Unknown (тест).
2. Производительность: `FileMeta` вызывается только при новом процессе в корне профиля с решениями; кэш.
3. Совместимость: старые решения без `AppRoot` профиля работают как точный путь.

## 3. Вердикт (≤ 25 строк)

done/blocked/failed; Diff; Evidence (тесты, мутации, кросс-сборка); Rulings; Escalations.
