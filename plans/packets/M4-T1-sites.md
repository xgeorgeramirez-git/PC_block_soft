# Пакет: T1 вехи M4 «Пакет internal/sites» | слот: писатель | исполнитель: новый агент

Репозиторий `/home/claude/pc_block_soft`, ветка `feature/m4-sites`. Прочитай `plans/2026-11-04-M4.md`: Global Constraints, RM4-1, RM4-3, RM4-4, RM4-5, Review Focus п. 1 и 10, строку T1. ТЗ: R13, R14, FR-21, ST2, NFR-1, NFR-4 (`docs/01_ТЗ.md`), D-67 (`docs/03_Решения_интервью.md`).

## Бриф

Новый пакет `internal/sites` (только стандартная библиотека) — единственное место, где из текста получается домен и где по домену решается «чей это сайт»: нормализация ввода родителя (FR-21), домен из поля адреса Brave (RM4-1), свой кодировщик punycode (RM4-4), свои списки родителя с правилом «самая длинная запись» (RM4-5) и встроенный список «18+» UT1 в компактном формате `adult.bin` (RM4-3, ST2). Плюс инструмент `tools/adultlist`, который в CI строит полный `adult.bin`, и шаг `windows.yml`. Службу, хранилище, страж и фронт **не трогай**: их подключают T3, T4, T7.

**Страж не трогать.** В `internal/guard/omnibox.go` уже лежит `omniboxDomain` из T0 (проверь: `grep -n "func omniboxDomain" internal/guard/omnibox.go`). `sites.FromOmnibox` — её перенос с одной разницей (ниже); замену в страже делает T7.

**Порядок с T0.** Писатель T0 правит `.github/workflows/windows.yml` (шаг «Brave», `PCB_BRAVE_PROBE`). Шаги `windows.yml` из этого пакета делай последними и перечитай файл перед правкой (`git log -3 -- .github/workflows/windows.yml`), чтобы не затереть T0.

## Files

Create:
- `internal/sites/doc.go` — `// Package sites — домены сайтов: …` (первая строка — роль для `tools/codemap.sh`).
- `internal/sites/domain.go` — `ErrNotDomain`, `MaxDomainLen`, `FromOmnibox`, `NormalizeInput`.
- `internal/sites/punycode.go` — `encodePunycode`, `toASCII`.
- `internal/sites/lists.go` — `Kind`, `Rule`, `Lists`, `NewLists`, `Match`.
- `internal/sites/adult.go` — формат, `HashDomain`, `EncodeAdult`, `OpenAdult`, `AdultList`, `Contains`, `Count`, `Embedded`, `//go:embed`.
- `internal/sites/adult/adult.bin` — пустой файл формата (16 байт, см. «Формат»).
- `internal/sites/adult/LICENSE-UT1.txt` — источник и лицензия (CC BY-SA 4.0).
- Тесты: `internal/sites/domain_test.go`, `punycode_test.go`, `lists_test.go`, `adult_test.go`, `adult_full_test.go`.
- `tools/adultlist/main.go`, `tools/adultlist/main_test.go`, `tools/adultlist/ut1.commit`.

Modify:
- `tools/coverage-thresholds.txt` — строка `internal/sites 90` (формат файла: «пакет порог_в_процентах», строки с `#` пропускаются; читает `tools/test-series.sh:check_coverage`).
- `.gitattributes` — строка `*.bin binary` (сейчас там `* text=auto eol=lf`: без неё git может портить байты `0x0D 0x0A`).
- `.github/workflows/windows.yml` — шаги «Полный список 18+» и «Размер установщика», `PCB_ADULT_FULL` в тестах.

## Interfaces

Produces (точные имена; T3, T4, T7 опираются на них):

```go
package sites

var ErrNotDomain = errors.New("sites: это не похоже на адрес сайта") // текст человеку даёт служба
const MaxDomainLen = 253

func FromOmnibox(text string) string                  // домен или «»
func NormalizeInput(s string) (domain string, err error) // домен или ErrNotDomain

type Kind int
const (
	KindBlocked Kind = iota + 1 // "blocked" — «Запрещённые»
	KindStudy                   // "study" — «Учёба»
	KindVideo                   // "video_social" — «Видео и соцсети»
)
func (k Kind) String() string            // ключ как в site_rules.kind (CHECK в 001_init.sql)
func KindFromString(s string) (Kind, bool)

type Rule struct{ Domain string; Kind Kind }
type Lists struct{ /* map[string]Kind */ }
func NewLists(rules []Rule) Lists         // домены уже нормализованы; пустые пропускаются
func (l Lists) Match(domain string) (Rule, bool) // самая длинная совпавшая запись: домен, затем родители

func HashDomain(domain string) uint64     // FNV-1a 64 бита по байтам домена (без выделений памяти)
func EncodeAdult(domains []string) []byte // нормализованные домены → файл формата (для инструмента и тестов)
func OpenAdult(data string) (AdultList, error) // ErrAdultFormat при порче
var ErrAdultFormat = errors.New("sites: файл списка 18+ повреждён")
type AdultList struct{ /* data string; count int; cache *adultCache */ }
func (a AdultList) Count() int
func (a AdultList) Contains(domain string) bool
func Embedded() (AdultList, error)        // OpenAdult встроенного файла, один раз (sync.Once)
```

Consumes: ничего из проекта (только стандартная библиотека). Слой человеческих сообщений — в службе: по образцу `core.ErrExtraRange` → `ipc.FailField(textMinutesRange, ipc.FieldMinutes)` (`internal/service/handlers.go:500`) служба в T4 отвечает `ipc.FailField(textNotDomain, ipc.FieldDomain)`, где `textNotDomain = "Это не похоже на адрес сайта"` живёт в `internal/service/texts.go`, а `FieldDomain` добавит T3 в `internal/ipc/commands.go` рядом с `FieldMinutes` (стр. 111–118). В `sites` человеческих текстов для окон нет: только сигнальная ошибка.

## Правила домена (RM4-4)

- Нижний регистр, без `www.` в начале (если после него остаётся точка: `www.ru` остаётся `www.ru`), без точки в конце, без схемы, пути, запроса, `#`, порта, логина `user:pw@`.
- **IDN → punycode** по меткам: `пример.рф` → `xn--e1afmkfd.xn--p1ai`. Это **единственная разница** с `omniboxDomain` из T0: там `https://пример.рф/путь` → `пример.рф` (поле адреса Brave показывает Юникод), а списки хранятся в punycode, поэтому `FromOmnibox` тоже отдаёт punycode. Метка уже вида `xn--…` остаётся как есть. Полной таблицы UTS 46 нет: только `strings.ToLower` и кодирование; это ограничение записать в комментарий.
- `FromOmnibox`: всё, что `omniboxDomain` отдаёт «», и здесь «» (служебные страницы, `brave://`, `about:`, `file:`, `mailto:`, поисковая фраза, `localhost:3000`, IPv6 в скобках). `view-source:` снимается. IPv4 — как есть (`192.168.0.1`).
- `NormalizeInput(s)` = тот же разбор: `d := FromOmnibox(s); if d == "" { return "", ErrNotDomain }` (PL-39: один разборщик на оба входа). Проверки длины — внутри общего разбора (значит, и в `FromOmnibox`): метка длиннее 63 знаков после кодирования, весь домен длиннее 253 после кодирования — «» / ошибка.
- Допустимые знаки метки после кодирования: `a-z`, `0-9`, `-`, `_`; пустая метка (`a..b`, `.a.b`) — «» / ошибка.

## Формат `adult.bin` (RM4-3)

Все целые — little-endian.

| Смещение | Размер | Что |
|---|---|---|
| 0 | 8 | магия `PCBADULT` (ASCII) |
| 8 | 4 | версия `uint32` = 1 |
| 12 | 4 | `count` — число записей `uint32` (уникальные 44-битные хеши) |
| 16 | 4 × (2²⁰ + 1) | только при `count > 0`: `offsets[b]` — индекс первой записи корзины `b`; `offsets[2²⁰] = count`; неубывающие, `offsets[0] = 0` |
| 16 + 4 194 308 | 3 × `count` | младшие 24 бита каждой записи, внутри корзины по возрастанию |

- Хеш: `h := HashDomain(d)` (FNV-1a 64: базис `0xcbf29ce484222325`, простое `0x100000001b3`; цикл по байтам, а не `hash/fnv` — `fnv.New64a` выделяет память); `h44 := h >> 20`; корзина `h44 >> 24` (старшие 20 бит), запись `h44 & 0xFFFFFF`.
- Литералы для тестов (посчитаны вне Go): `HashDomain("")` = `0xcbf29ce484222325`, `HashDomain("a")` = `0xaf63dc4c8601ec8c`, `HashDomain("example.com")` = `0x576846634e2714c6` → корзина `0x57684`, запись `0x6634e2`; `HashDomain("xn--e1afmkfd.xn--p1ai")` = `0xf36508a22f79dd1d` → корзина `0xf3650`, запись `0x8a22f7`.
- **Пустой файл — 16 байт без таблицы корзин** (находка читателя: с таблицей «пустой» файл весил бы 4 МБ в репозитории). `OpenAdult` принимает `count == 0` только при длине ровно 16; при `count > 0` длина ровно `16 + 4·(2²⁰+1) + 3·count`; проверяет магию, версию, монотонность смещений и последнее смещение — иначе `ErrAdultFormat`. Полный файл на 4,6 млн ≈ 18 МБ.
- `OpenAdult` не копирует данные: хранит `string` и читает байты индексом `data[i]`. Никаких `[]byte(data)` (это копия 18 МБ в кучу, NFR-5). Встраивание: `//go:embed adult/adult.bin` в `var adultBin string` (раздел только для чтения образа: страницы общие, не личная память процесса — замер приёмки S1 смотрит личный рабочий набор, `tools/accept/common.ps1:Get-MemoryInfo`).
- `Contains(d)`: проверяет `d`, затем родителей, отрезая левую метку, пока в остатке есть точка (голый `com` не проверяется). IPv4 (только цифры и точки) — только точное совпадение. Поиск: двоичный поиск в `[offsets[b], offsets[b+1])`. Кэш результата по исходному `d`: `map[string]bool` под `sync.Mutex`, при 4096 записях кэш очищается целиком (как `domainMemo` в `internal/guard/omnibox.go`). Нулевой `AdultList{}` — `Contains` = false, `Count` = 0.
- `EncodeAdult`: хеши → сортировка → удаление повторов 44-битных значений → файл. Пустой вход → 16 байт.

## Инструмент `tools/adultlist`

- `package main`, функция `run(args []string, stdout io.Writer) error` (тестируется без `os.Exit`), флаги: `-src <папка>` (читает все файлы `domains*` в ней; `*.gz` — через `compress/gzip`, потому что формат файлов зеркала из песочницы проверить не удалось: доступ к `olbat/ut1-blacklists` закрыт), `-out <файл>`, `-sample <файл>` (пишет 3 нормализованных домена входа: первый, средний, последний — для `TestAdultListFull`), `-empty` (пишет пустой файл формата; так создаётся `internal/sites/adult/adult.bin`).
- Строки: `\r\n` и пробелы по краям снимаются, пустые и `#…` пропускаются, домен — `sites.NormalizeInput`; ошибка — строка пропущена и посчитана.
- Без `-empty` ноль доменов — ошибка (иначе сборка выпуска тихо получит пустой список). Итог в stdout строкой `::notice title=Список 18+::строк N, пропущено M, записей K, размер B байт` (строки — непустые и не комментарии; записи — уникальные хеши).
- Без сети (NFR-1, `tools/check_imports.sh` проверяет `go list ./...` — пакет `pcblock/tools/adultlist` попадёт в проверку): загрузка — только `git` в шаге CI.
- Строки в `.go` (кроме тестов) проверяет `tools/scan_ui_texts.py`: **без длинных тире** и в инструменте тоже.
- `tools/adultlist/ut1.commit` — одна строка `03501c64d230df9d84124176558d758351027be1`.

## `LICENSE-UT1.txt`

По-русски и по-английски: список «adult» базы UT1, Université Toulouse 1 Capitole (автор Fabrice Prigent), `https://dsi.ut-capitole.fr/blacklists/`; зеркало `https://github.com/olbat/ut1-blacklists`, коммит из `ut1.commit`; лицензия CC BY-SA 4.0 `https://creativecommons.org/licenses/by-sa/4.0/`; изменения: домены нормализованы и сохранены как 44-битные хеши FNV-1a. В установщик файл кладёт T10, не ты.

## Шаги (RED → GREEN → мутации → коммит)

Среда: `export PATH=~/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off`.

1. **Punycode.** RED: `TestEncodePunycodeRFC3492` — сырой кодировщик `encodePunycode([]rune) string` без смены регистра, векторы RFC 3492 раздел 7.1 (сверены с кодеком Python):
   - (B) `他们为什么不说中文` → `ihqwcrb4cv8a8dqg056pqjye`
   - (D) `Pročprostěnemluvíčesky` → `Proprostnemluvesky-uyb24dma41a`
   - (I) `почемужеонинеговорятпорусски` → `b1abfaaepdrnnbgefbadotcwatmq2g4l` (в RFC — `…baDot…`: заглавная `D` там флаг регистра; сравнивай `strings.EqualFold` или нижний регистр)
   - (J) `PorquénopuedensimplementehablarenEspañol` → `PorqunopuedensimplementehablarenEspaol-fmd56a`
   - (L) `3年B組金八先生` → `3B-ww4c5e180e575a65lsy2b`
   - (Q) `なぜみんな日本語を話してくれないのか` → `n8jok5ay5dzabd5bym9f0cm5685rrjetr6pdxa`
   - (S) `-> $1.00 <-` → `-> $1.00 <--`
   `TestToASCII`: `пример` → `xn--e1afmkfd`, `рф` → `xn--p1ai`, `example` → `example`, `xn--p1ai` → `xn--p1ai`, `ПРИМЕР` → `xn--e1afmkfd`.
   Expected: RED (нет функции) → GREEN. Мутации: `initialBias` 72 → 73; `damp` 700 → 701; убрать `-` между базовыми и кодом — каждая роняет тест.
2. **Домен.** RED: `TestFromOmnibox` — таблица литералами: все 29 строк `TestOmniboxDomain` (`internal/guard/omnibox_test.go:11`), но `https://пример.рф/путь` → `xn--e1afmkfd.xn--p1ai`; плюс `ПРИМЕР.РФ` → `xn--e1afmkfd.xn--p1ai`, `https://www.youtube.com/watch?v=1&t=2` → `youtube.com`, `m.YouTube.com.` → `m.youtube.com`, `example.com:8080` → `example.com`. `TestNormalizeInput`: `example.com` → `example.com`; `https://WWW.Example.COM./a?b` → `example.com`; `пример.рф` → `xn--e1afmkfd.xn--p1ai`; `192.168.0.1` → `192.168.0.1`; ошибки `ErrNotDomain` (`errors.Is`): «», `котики`, `котики видео`, `localhost`, `brave://settings`, `a..b`, `*.example.com`, `https://[::1]/`, метка из 64 `a` + `.com`, домен 254 знака. Expected: RED → GREEN. Мутации: убрать снятие `www.`, порта, логина, кодирование IDN — каждая роняет тест.
3. **Фаззинги** в `domain_test.go`: `FuzzFromOmnibox` — не паникует; результат «» или `NormalizeInput(r) == (r, nil)` (согласованность двух входов) и `FromOmnibox(r) == r` (идемпотентность), только ASCII нижнего регистра, без `/:@\` и пробелов, ≤ 253. `FuzzNormalizeInput` — не паникует; при `err == nil` то же про результат; при ошибке — `errors.Is(err, ErrNotDomain)`. Затравки: строки из таблиц шага 2 и 2000 знаков. Прогон: `go test -run '^$' -fuzz FuzzFromOmnibox -fuzztime 30s ./internal/sites/` и то же для второго. Expected: без находок (серия R4 гоняет каждый `Fuzz*` по 60 с — `tools/test-series.sh`).
4. **Списки.** RED: `TestKindStrings` (`KindBlocked` ↔ `blocked`, `KindStudy` ↔ `study`, `KindVideo` ↔ `video_social`, `KindFromString("x")` → `(0, false)`). `TestListsMatch` на `NewLists([]Rule{{"youtube.com", KindVideo}, {"edu.youtube.com", KindStudy}, {"example.com", KindBlocked}, {"192.168.0.1", KindBlocked}})`: `youtube.com` → video; `m.youtube.com` → video (запись `youtube.com`); `a.edu.youtube.com` → study (запись `edu.youtube.com`, самая длинная); `notyoutube.com` → нет; `com` → нет; `x.example.com` → blocked (`Rule.Domain == "example.com"`); `192.168.0.1` → blocked; `1.192.168.0.1` → нет (домен только из цифр и точек сравнивается лишь точно, родители не проверяются, как в `Contains`); пустой домен → нет; `Lists{}` → нет. Expected: RED → GREEN. Мутации: сравнение по суффиксу строки без точки (`notyoutube.com` совпадёт); обход от короткого к длинному (`a.edu.youtube.com` даст video).
5. **Формат 18+.** RED в `adult_test.go`:
   - `TestHashDomain` — литералы из раздела «Формат».
   - `TestEncodeAdultEmpty` — `EncodeAdult(nil)` = `"PCBADULT" + 01 00 00 00 + 00 00 00 00` (16 байт литералом); `OpenAdult` → `Count() == 0`, `Contains("example.com") == false`.
   - `TestEncodeAdultLayout` — `EncodeAdult([]string{"example.com", "example.com", "xn--e1afmkfd.xn--p1ai"})`: длина `16 + 4194308 + 6`; `count` = 2; `offsets[0x57684]` = 0, `offsets[0x57685]` = 1, `offsets[0xf3650]` = 1, `offsets[0xf3651]` = 2, `offsets[1<<20]` = 2; записи: байты `e2 34 66` и `f7 22 8a`.
   - `TestAdultContainsParents` — список `{"porn.example", "bad.co.uk", "10.0.0.1"}`: `porn.example`, `www.porn.example` → `x.y.porn.example` → true; `example`, `co.uk`, `uk`, `good.co.uk` → false; `a.bad.co.uk` → true; `10.0.0.1` → true, `1.10.0.0.1` → false.
   - `TestOpenAdultRejects` — таблица: короче 16, чужая магия, версия 2, `count = 0` с хвостом, `count = 1` с неверной длиной, убывающие смещения, последнее смещение ≠ `count` → `errors.Is(err, ErrAdultFormat)`.
   - `TestAdultNoCopy` — `testing.AllocsPerRun(100, func(){ OpenAdult(s) })` ≤ 2 и `Contains` повторного домена (из кэша) = 0 выделений.
   - `TestAdultCacheBound` — 10 000 разных доменов → размер кэша ≤ 4096 (доступ к полю в тесте пакета).
   - `TestEmbeddedEmpty` — `t.Skip`, если `PCB_ADULT_FULL != ""`; иначе `Embedded()` без ошибки, `Count() == 0` (встроенный файл в репозитории пустой).
   Мутации: `h >> 21` вместо `>> 20`; поиск без родителей; проверка голого TLD (`co.uk` даст true); кэш без границы.
6. **ST2 и бенчмарк** (`adult_full_test.go`):
   - `TestAdultST2Synthetic` — 4 600 000 записей: 10 000 хешей настоящих доменов `d0000000.test`…`d0009999.test` + 4 590 000 случайных `uint64` (`math/rand/v2`, `rand.NewPCG(1, 2)`); собрать файл тем же кодом, что `EncodeAdult` (вынеси `encodeHashes([]uint64) []byte`, `EncodeAdult` его вызывает). `OpenAdult` ≤ 2 с; 10 000 проверок без кэша (внутренний `containsNoCache`) — 99-й процентиль ≤ 1 мс; все 10 000 настоящих найдены; `example.com` и `google.com` — нет (ложное совпадение ≈ 1 на миллион; на этих двух не ожидается). Под `-race` в `ci.yml` тест тоже идёт: если сборка дольше 60 с — сообщи цифры в вердикте, порог не ослабляй.
   - `TestAdultListFull` — `t.Skip`, если `PCB_ADULT_FULL != "1"`; `Embedded()`: `Count() ≥ 4_000_000`; время `OpenAdult` ≤ 2 с; каждый домен из `PCB_ADULT_SAMPLE` (через запятую) → `Contains == true`; `example.com` → false; 10 000 проверок — p99 ≤ 1 мс; факты пометкой `::notice title=Список 18+::…` в `$RUNNER_TEMP/pcb-adult.txt` (образец `writeNotice` в `internal/service/winhost/procwatch_windows_test.go:505`).
   - `BenchmarkAdultContains` — синтетический список (сборка один раз вне таймера), без кэша.
7. **Инструмент.** RED `tools/adultlist/main_test.go:TestRunSmallInput`: временная папка, файл `domains` (`# комментарий`, пустая строка, `Example.COM\r`, `www.example.com`, `porn.test`, `котики видео`, `пример.рф`) и `domains.2.gz` (`gz.test`), файл `urls` (`must.not.read`) → `run(["-src", dir, "-out", out, "-sample", s])` без ошибки; `OpenAdult` → `Count() == 4` (`example.com`, `porn.test`, `xn--e1afmkfd.xn--p1ai`, `gz.test`); `must.not.read` — false; stdout содержит `строк 6, пропущено 1, записей 4`; файл образца — 3 строки. `TestRunNoDomains` — папка без `domains*` → ошибка. `TestRunEmpty` — `-empty -out f` → 16 байт, равные `EncodeAdult(nil)`. Затем `go run ./tools/adultlist -empty -out internal/sites/adult/adult.bin`.
8. **Покрытие**: `go test -cover ./internal/sites/` ≥ 90% → строка в `tools/coverage-thresholds.txt`.
9. **`windows.yml`** (после T0, перечитав файл):
   - перед «Сборка программы и установщика» шаг «Полный список 18+ (UT1 на закреплённом коммите)», bash, `exec > >(tee -a "$RUNNER_TEMP/ci.log") 2>&1`: `commit=$(tr -d '[:space:]' < tools/adultlist/ut1.commit)`; `src="$RUNNER_TEMP/ut1"`; `git init -q "$src"`; `git -C "$src" remote add origin https://github.com/olbat/ut1-blacklists.git`; `git -C "$src" sparse-checkout set blacklists/adult`; `git -C "$src" fetch -q --depth 1 --filter=blob:none origin "$commit"`; `git -C "$src" checkout -q FETCH_HEAD`; `ls -la "$src/blacklists/adult"` (факт о файлах — в журнал); `go run ./tools/adultlist -src "$src/blacklists/adult" -out internal/sites/adult/adult.bin -sample "$RUNNER_TEMP/adult-sample.txt"`; `echo "PCB_ADULT_SAMPLE=$(paste -sd, "$RUNNER_TEMP/adult-sample.txt")" >> "$GITHUB_ENV"`. Клон — вне дерева репозитория. Шаг «Биндинги» коммитит только `frontend/wailsjs/go` (проверь: `git add frontend/wailsjs/go`), поэтому изменённый `adult.bin` в ветку не попадёт.
   - сразу после «Сборка программы и установщика» шаг «Размер установщика (NFR-4 ≤ 40 МБ)»: `inst=$(ls build/bin/*installer*.exe | head -1)` (тот же поиск, что в «Файлы выпуска»); `size=$(stat -c %s "$inst")`; `::notice title=Размер установщика::$size байт`; больше `41943040` → `::error title=Размер установщика::…` и `exit 1`. Сейчас 0.3.3 — 9 600 202 байта; хеши почти не сжимаются, ожидание ≈ 28 МБ.
   - «Тесты Go на Windows»: `env: PCB_ADULT_FULL: "1"` (рядом с переменной T0, если она уже есть); после шага — «Список 18+ (пометки)» `if: always()` выводит `$RUNNER_TEMP/pcb-adult.txt` по образцу «Сухой прогон наблюдателя (пометка)».
   Проверка — оркестратор запускает `windows.yml` (R3) после push; локально — только синтаксис (`python3 -c 'import yaml,sys; yaml.safe_load(open(".github/workflows/windows.yml"))'`, если `yaml` есть).
10. **Финал:** `go test -race -count=1 ./internal/sites/... ./tools/adultlist/...`; `go vet ./internal/sites/... ./tools/adultlist/...`; `gofmt -l internal/sites tools/adultlist`; `python3 tools/scan_ui_texts.py`; `bash tools/check_imports.sh` (если `go list` работает без сети). `docs/CODEMAP.md` не трогай: карту перегенерирует оркестратор (`bash tools/codemap.sh`).

## Global Constraints (дословно из плана)

- Ядро `internal/core` и новый пакет `internal/sites` — только стандартная библиотека; тесты идут локально (`GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 ./internal/core/... ./internal/sites/...`, Go 1.27 в `~/.local/go1.27`). Правила, цифры и тексты считает ядро и служба; страж, значок, окна — только показывают.
- Тексты ребёнку на «ты», родителю на «вы», без длинных тире (`python3 tools/scan_ui_texts.py`), без технических ошибок. Тексты плашек — дословно ТЗ 8.2.5.
- R23: страж читает адрес вкладки только в памяти и сразу оставляет домен; ни полный адрес, ни заголовок окна не передаются, не пишутся в журнал работы и не хранятся. Хранится только домен.
- NFR-1: ни одного сетевого соединения от программы. Список «18+» встроен в программу при сборке (D-67); загрузка UT1 — только в CI при сборке, никогда в программе.
- NFR-4 установщик ≤ 40 МБ; NFR-5 служба ≤ 50 МБ, страж ≤ 40 МБ памяти; NFR-6 вкладка закрыта ≤ 2 с, программа ≤ 1 с; ST2 проверка домена ≤ 1 мс (99-й процентиль), загрузка списка ≤ 2 с.
- Репозиторий публичный; self-hosted runner заказчика к workflow НЕ привязывать; на ПК заказчика ничего не ставить и блокировки не запускать; приёмка только в CI на windows-latest в тестовом режиме. Запуски `windows.yml` — по одному.
- Писатель коммитит пути явно (`git add <пути>`, никогда `-A`), не пушит; пушит оркестратор. Трейлеры коммитов: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` и `Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze`.
- `go.sum` обновляет CI; GraphQL закрыт — PR, слияние, выпуск через `gh api` (CLAUDE.md). Журналы Actions не скачиваются — пометки сборки (`tools/annotate-errors.sh`).

## Уроки (docs/LESSONS.md)

- PL-39 (класс C): одна величина — один переводчик. `NormalizeInput` и `FromOmnibox` — один разбор; фаззинг проверяет, что они согласны.
- PL-9 (класс D): «не знаю» — ничего не утверждать. Непонятный текст адреса → «», а не догадка; порченый `adult.bin` → ошибка, а не «пусто без слов».
- PL-3 (ресурсы): крупный буфер не держать в куче. Список читается из встроенной строки на месте; `TestAdultNoCopy` это стережёт.
- PL-41 (приёмка): запасную ветку, которую не исполняли, считать сломанной — ветка `.gz`, пустой файл и «ноль доменов — ошибка» покрыты тестом инструмента; полный список — `TestAdultListFull` на раннере.
- PL-1 (CI): упавший шаг сам говорит, почему: инструмент и тест пишут `::notice`/`::error`, шаг пишет в `ci.log`.

## Коммиты

Маленькие, Conventional Commits по-русски, повелительное наклонение (`feat: добавить пакет sites с нормализацией домена`, `feat: добавить формат и чтение списка 18+`, `build: строить полный список 18+ в windows.yml`). Пути явно: `git add internal/sites/… tools/adultlist/… tools/coverage-thresholds.txt .gitattributes .github/workflows/windows.yml` — только свои файлы (в дереве может работать другой писатель). Трейлеры в конце сообщения:

```
Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze
```

НЕ пушить.

## Вердикт (≤ 30 строк)

```
TASK T1: done | blocked | failed
Diff: файлы ±строк; sha коммитов (строка на коммит)
Evidence: go test -race (пакеты, счётчики), RED-запуски (тест и причина), мутации (какая → какой тест упал), фаззинг (время, находки), покрытие %, размер синтетического файла и время TestAdultST2Synthetic под -race, scan_ui_texts, check_imports
Interfaces: итоговые публичные имена sites (для T3, T4, T7), если отличаются от пакета
Rulings: свои решения с ценой ошибки (например, UTS 46 нет)
Escalations / Lesson / Open
```
