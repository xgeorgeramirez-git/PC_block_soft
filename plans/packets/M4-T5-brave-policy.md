# Пакет: T5 вехи M4 «Правила Brave» | слот: писатель | исполнитель: новый агент

Репозиторий `/home/claude/pc_block_soft`, ветка `feature/m4-sites`. Прочитай `plans/2026-11-04-M4.md`: Global Constraints, RM4-10 и **RM4-10-итог** (HKCU действует, кнопки VPN нет, запрещённый домен — `chrome-error://chromewebdata/`), Review Focus п. 2 и 6; строку T5. ТЗ (`docs/01_ТЗ.md`): R15, FR-31, 3.4 (строка «Правила Brave для ребёнка»: ребёнок может читать, не может менять), S20. Пакет T4 (`plans/packets/M4-T4-service.md`): S-11, **S-12** (`sitesChanged`/`braveDue` — T5 их потребитель).

**Зависимость:** T5 идёт после T4a (нужны `reloadSites`, `sitesChanged`, `braveDue`). Перед работой проверь, что они есть: `grep -n "braveDue\|func (s \*Service) sitesChanged\|func (s \*Service) reloadSites" internal/service/*.go`. Нет — `blocked`, к оркестратору.

## Бриф

Служба пишет правила Brave в ветку реестра каждого отмеченного ребёнка (`HKU\<SID>\Software\Policies\BraveSoftware\Brave`): окна Tor и VPN выключены, защищённый DNS выключен, расширения не ставятся, «Запрещённые» домены — в чёрном списке Brave. Пишет при старте, при входе ребёнка, при изменении списков (`braveDue`), сверяет раз в 60 с и восстанавливает расхождение с записью в журнал родителя; снимает ветку при снятии отметки «детская» и при удалении программы. Сравнение и план записи — чистые функции с тестами на Linux; реестр — адаптер `winhost`, проверка на windows-latest.

Сверь адреса (PL-4, в дереве недавно работал писатель T4a — номера строк могли сдвинуться): `git log --oneline -15`, `grep -n "func (s \*Service) applyPolicy\|s.applyPolicy(\|type Policy interface\|func (s \*Service) Housekeep\|SetTaskMgrDisabled" -r internal`.

## Образец в коде (проверено читателем)

- `internal/service/ports.go:360-365` — `type Policy interface { SetTaskMgrDisabled(sid string, disabled bool) error }` (в брифе было `:342` — уже не так).
- `internal/service/service.go:73` `Config.Policy` (nil → `noPolicy{}`, `:218`), `:255-257` `noPolicy`, `:292-296` цикл `Start` по `childOrder()` с `checkAdmin` и `applyPolicy(cs.child.SID, true)` (он идёт **после** `ensurePrograms` `:287`, а T4a кладёт `reloadSites` сразу после `ensurePrograms` — списки к этому циклу уже прочитаны), `:451-455` `applyPolicy` → `logRate("policy"+sid, …)`; `:20-22` `adminCheckEvery`/`rulesRetryEvery`/`errLogEvery` = 60 с; `:538` `logRate` (ограничитель `errLogEvery`).
- Вызовы `applyPolicy`: `sessions.go:246` (новый ребёнок в сеансе, `prev != ss.childID`), `handlers.go:292` (`hSetup`, после `loadChildren`), `handlers.go:800` (`hSetChildren`, снятие отметки: `applyPolicy(cs.child.SID, false)` после `RemoveChild` и `delete(s.children, id)`), `handlers.go:819` (`hSetChildren`, новый ребёнок).
- **Период `Housekeep`:** `loop.go:273` `house := time.NewTicker(time.Second)` → `:283` `s.Housekeep(s.cfg.RealNow())` — раз в настоящую секунду; 60 с отсчитывается полем момента, как `cs.adminAt`/`adminCheckEvery` (`loop.go:223`). Тело `Housekeep` — `loop.go:198-218`, шаги через `s.guard("<место>", nil, func(){…})` (`loop.go:54`, ловит панику).
- `internal/service/winhost/policy_windows.go`: `policy{mu sync.Mutex}` (`:37`), `SetTaskMgrDisabled` (`:42`: `writePolicy(sid)` → `registry.ErrNotExist` → `viaProfile`), `writePolicy(hiveName, disabled)` (`:56`: `OpenKey(USERS, hive)` — проверка, что ветка загружена), `viaProfile(sid, disabled)` (`:85`: ProfileList → `NTUSER.DAT` → профиля нет — `nil`; `enablePrivilege`; `loadHive(tempHivePrefix+sid)` → запись → `unloadHive`), `loadHive`/`unloadHive` (`:129`, `:145`). Подключён `host_windows.go:78` `Policy: &policy{}`.
- Удаление программы: `install_windows.go:181` `note(clearChildPolicies(dataDir, info))`, `:240-266` `clearChildPolicies` — `var p policy`, по `st.ListChildren()` → `p.SetTaskMgrDisabled(c.SID, false)`, `info["policyCleared"]`.
- Реестр в коде (x/sys `registry`, проверенные вызовы): `registry.OpenKey`, `registry.CreateKey(root, path, access) (Key, bool, error)`, `k.SetDWordValue`, `k.SetStringValue`, `k.DeleteValue`, `k.GetValue(name, nil) (n, valtype, err)` + `registry.ErrShortBuffer` (`testmode_windows.go:30-34`), `k.GetIntegerValue`, `k.GetStringValue`, `registry.DeleteKey(root, path)` (только лист без подключей, `install_windows.go:134`). `ReadValueNames`/`ReadSubKeyNames` в коде ещё нигде не вызывались — сигнатуру в x/sys v0.48.0 (`go.mod:8`) писатель не проверит локально (модульного кэша x/sys в песочнице нет): пиши `k.ReadValueNames(-1)` / `k.ReadSubKeyNames(-1)` (`n <= 0` — все) и смотри `GOOS=windows go vet` в CI.
- Образец записи правил Brave (проба T0): `internal/guard/brave_probe_windows_test.go:38` `bravePolicy = \`Software\Policies\BraveSoftware\Brave\``, `:478-503` `setBravePolicies(root)` (`CreateKey(... registry.ALL_ACCESS|registry.WOW64_64KEY)`), `:506-517` `removeBravePolicies` (HKCU и HKLM). **`TestBraveProbe` пишет и удаляет тот же ключ в HKCU раннера** (только при `PCB_BRAVE_PROBE=1`).
- Тест-раннер Windows: `windows.yml:158-163` «Тесты Go на Windows» — `go test -count=1 -tags desktop,production $(go list ./...)`: пакеты идут **параллельно** (`-p` по умолчанию), от имени администратора раннера (ветка `HKU\<свой SID>` загружена и доступна на запись). Отдельного теста адаптера `DisableTaskMgr` нет: его проверяют приёмка `s07-parent-account.ps1:35`, `s26-uninstall.ps1:33-44` и уборка `common.ps1:535` (ребёнок приёмки — сама учётка раннера, `$script:MySid`).
- Фейк: `fakes_test.go:1184-1208` `fakePolicy{mu, state map[string]bool, log []string, err error}`, `newPolicy()`, `disabled(sid)`; `harness_test.go:42,60,73` `h.policy`. Образец теста политики — `sessions_test.go:658` `TestPolicySetAtStartAndLogon`, `:674` `TestPolicyErrorDoesNotStopService`; `misc_test.go:124` проверяет `noPolicy`.
- Журнал: `journaltext.go:44` `eventText(kind, name, d)` — `case` по виду; событие ребёнка печатается с префиксом имени («Миша: …», с маленькой буквы).
- `ipc.MaxBlockedSites = 1000` (`internal/ipc/commands_m4.go:30`), `ipc.SiteKindBlocked = "blocked"`.

## Ruling читателя (оркестратор подтверждает до пуска; цена ошибки — в скобках)

- **T5-1 Порт — три метода, не два.** `Policy` получает `SetBraveRules(sid string, r BraveRules) error`, `BraveRulesOK(sid string, r BraveRules) (bool, error)` и `RemoveBraveRules(sid string) error`. RM4-10 называет два; снятие ветки (удаление программы, снятие отметки) через «пустые правила» неотличимо от «запрещённых нет». (Цена: отклонение от буквы RM4-10 — сообщить в вердикте; иначе снятие отметки оставляло бы Tor/VPN выключенными у взрослой учётки.)
- **T5-2 Тип и чистые функции — в `internal/service/brave.go`** (стандартная библиотека, тесты на Linux): `type BraveRules struct{ Blocked []string }`; `type BraveValue struct{ Key, Name string; DWord bool; Num uint32; Str string }` (`Key` — «» для самой ветки, `ExtensionInstallBlocklist`, `URLBlocklist`); `func (r BraveRules) Values() []BraveValue` — полный набор значений ветки в порядке: `TorDisabled` DWORD 1, `BraveVPNDisabled` DWORD 1, `DnsOverHttpsMode` SZ "off", `ExtensionInstallBlocklist\1` SZ "*", `URLBlocklist\1..N` SZ домены (сортировка, повторы убраны, пустые отброшены, не больше `ipc.MaxBlockedSites`); `func BravePlan(have, want []BraveValue) (set []BraveValue, del []BraveValue, delKeys []string)` — что записать, какие лишние значения удалить, какие лишние подключи удалить целиком. Адаптер `winhost` только читает ветку в `[]BraveValue`, исполняет план и сравнивает (`len(set)+len(del)+len(delKeys) == 0` — совпадает). Отдельный пакет не нужен: `winhost` уже импортирует `service` (`host_windows.go`). (Цена отдельного пакета — лишний порог покрытия и импорт; цена логики в адаптере — сравнение без тестов на Linux.)
- **T5-3 Запись — «привести к цели», а не «удалить ветку и записать».** RM4-10 говорит «удалить и записать»; но между удалением и записью Brave (он следит за ветками политик и перечитывает их) может на миг снять запрет Tor/VPN. План T5-2 удаляет только лишнее и пишет недостающее/отличное, ветка целиком принадлежит программе (чужие значения и подключи удаляются). Подключ `URLBlocklist` при смене списка — переписываются номера `1..N`, лишние номера удаляются. (Цена: больше кода, чем `DeleteKey`+запись; без этого — окно без правил на каждое изменение списка.)
- **T5-4 Сверка раз в 60 с — только для загруженной ветки.** `BraveRulesOK` у адаптера: ветка `HKU\<SID>` не загружена (ребёнок не вошёл) → `(true, nil)`, без `viaProfile`. Запись через `viaProfile` — только при старте службы, по `braveDue` и при снятии. Причина: `RegLoadKey` профиля каждые 60 с держал бы `NTUSER.DAT` под временным именем — вход ребёнка в эту секунду сломался бы («Служба профилей пользователей не смогла войти»). Вход ребёнка (`sessions.go:246`) и так пишет правила в загруженную ветку. (Цена: правку ветки, сделанную администратором при выключенной учётке ребёнка, программа заметит только при его входе — тогда же она и действует.)
- **T5-5 Источник доменов — прошлое удачное чтение списков.** T5 дополняет `reloadSites` T4a: при удаче — `s.braveBlocked = <домены kind == blocked>` (по порядку хранилища) и `s.braveReady = true`; при ошибке — прежние. Пока `!s.braveReady` (списки ни разу не прочитались) правила **не пишутся и не сверяются** (`logRate("brave-wait", …)`): иначе первая неудача чтения базы стёрла бы «Запрещённые» в Brave. (Цена: при сломанной базе на первом старте Tor/VPN не выключаются до первого удачного чтения; служба в это время всё равно в безопасном режиме.)
- **T5-6 Кому пишутся.** Все дети `s.children` (отмеченные), в том числе ребёнок-администратор (как `DisableTaskMgr`: R1 снимает ограничения учёта, а правило браузера безвредно). Родитель и неотмеченные учётки — никогда (Review Focus 2). Новый ребёнок в `hSetChildren` — запись сразу; снятый — `RemoveBraveRules` рядом с `applyPolicy(cs.child.SID, false)` (`handlers.go:800`). Мастер — через `braveDue` (T4b ставит `sitesChanged` после `loadChildren`); явный вызов не нужен.
- **T5-7 Журнал «Правила Brave восстановлены».** Вид `evBraveRestored = "brave_restored"`, подробности `{"text": "правила Brave восстановлены"}`, `eventText` → «Миша: правила Brave восстановлены». Пишется только при сверке (не при первой записи на старте и не по `braveDue`, где расхождение ожидаемо) и **не чаще раза в 10 мин на ребёнка** — своё поле `braveLogAt map[int64]time.Time` (`watchOnce` не годится: `pruneWatchLimits` забывает ключи старше 60 с). В журнал работы — каждое восстановление: `Logf("правила Brave у %s восстановлены: записано %d, удалено %d значений, %d подключей", …)`. Сверка без расхождения не пишет ничего (Review Focus 6). (Цена: два восстановления за 10 мин — одна запись родителю; без ограничителя сломанная запись давала бы запись раз в минуту.)
- **T5-8 Порядок в `Housekeep`.** Новый шаг `s.guard("brave", nil, func(){ s.housekeepBrave(real) })` **после** шага `sites` T4a и до `parents`: `braveDue` → запись всем, `braveDue = false`, `braveAt = real`; иначе `real.Sub(s.braveAt) >= braveCheckEvery` (`= 60 * time.Second`, новая константа рядом с `adminCheckEvery`) → для каждого ребёнка `BraveRulesOK` → `false` → `SetBraveRules` + журнал (T5-7); `braveAt = real`. Ошибка адаптера — `logRate("brave"+sid, "правила Brave для %s не записаны: %v", …)`; `braveDue` сбрасывается всегда (повтор каждую секунду был бы шумом), ребёнок с ошибкой записи попадает в сверку через 60 с. `Start`: запись всем в цикле `:292-296` (`applyBrave(cs)`), `braveAt = real`.
- **T5-9 Права ветки.** Своих ACL не ставить: ключ создаётся внутри `Software\Policies`, у которого по умолчанию пользователь только читает, и наследует это (ТЗ 3.4). Если `Software\Policies` в ветке ребёнка нет (не бывает у профилей Windows 10/11, но) — `CreateKey` создаст его с правами `Software` (ребёнок сможет менять): адаптер тогда пишет в журнал работы «в ветке %s не было Software\Policies» (один раз за запись). Проверка прав ребёнка — в T10 (S20), если учётка ребёнка приёмки не администратор; иначе запись в README «Отклонения от ТЗ». (Цена: ребёнок удаляет правила — сверка вернёт их через ≤ 60 с с журналом.)
- **T5-10 Тест адаптера — в своей тестовой ветке, а не в настоящей.** Адаптер работает с путём `p.base` (поле `policy`, по умолчанию `bravePolicyPath = \`Software\Policies\BraveSoftware\Brave\``); тест `winhost` подставляет `\`Software\PCBlockTest\BraveSoftware-<pid>\`` в ветке текущего пользователя (`HKU\<SID процесса>`; SID — `windows.GetCurrentProcessToken().GetTokenUser()` → `.User.Sid.String()`) и удаляет её в `t.Cleanup`. Причина: тесты пакетов идут параллельно, `TestBraveProbe` (`internal/guard`) в это время пишет и удаляет настоящий ключ HKCU. Что Brave эти значения применяет, доказала проба T0 (RM4-10-итог); поведение Brave с правилами программы проверяет S20 в T10. Отдельный тест сверяет константу: `bravePolicyPath == \`Software\Policies\BraveSoftware\Brave\``. (Цена: тест адаптера не открывает Brave — компенсирует S20; запись в настоящий ключ из теста ломала бы пробу.)
- **T5-11 `viaProfile` общий.** `viaProfile(sid, disabled)` переделывается в `withHive(sid string, fn func(hive string) error) error` (ProfileList → `NTUSER.DAT` → привилегии → `loadHive` → `fn(temp)` → `unloadHive`; профиля нет — `nil`); `SetTaskMgrDisabled` и `SetBraveRules`/`RemoveBraveRules` — через него. `BraveRulesOK` его не вызывает (T5-4). Тексты ошибок `DisableTaskMgr` не меняются (по ним ищут в журналах прошлых прогонов).
- **T5-12 Удаление программы.** `clearChildPolicies` (`install_windows.go:240`): для каждого ребёнка ещё `p.RemoveBraveRules(c.SID)`; `info["braveCleared"]`. Функция и комментарий — «запрет Диспетчера задач и правила Brave». Приёмка: в `tools/accept/common.ps1` уборка (рядом со строкой `:535`) — `Remove-Item -Path "Registry::HKEY_USERS\$($script:MySid)\Software\Policies\BraveSoftware\Brave" -Recurse -Force -ErrorAction SilentlyContinue`, и в `s26-uninstall.ps1` `Assert-Removed` — `Assert-That (-not (Test-Path -Path "Registry::HKEY_USERS\$($script:MySid)\Software\Policies\BraveSoftware\Brave")) "${When}: остались правила Brave"`. Остальное S20 — T10. (Цена без уборки: правила службы из одного сценария остаются в HKCU раннера для следующих сценариев и пробы.)

## Files

Create: `internal/service/brave.go`, `internal/service/brave_test.go`, `internal/service/brave_svc_test.go`, `internal/service/winhost/policy_brave_windows_test.go`.
Modify: `internal/service/ports.go` (порт), `internal/service/service.go` (`noPolicy`, поля `braveBlocked`, `braveReady`, `braveAt`, `braveLogAt`, `Start`, `braveCheckEvery`), `internal/service/sites_svc.go` (`reloadSites` — T5-5; файл T4a), `internal/service/loop.go` (`Housekeep`), `internal/service/sessions.go:246`, `internal/service/handlers.go` (`hSetChildren` `:800`, `:819`), `internal/service/journaltext.go` (`brave_restored`), `internal/service/fakes_test.go` (`fakePolicy`), `internal/service/misc_test.go:124` (`noPolicy` — три новых метода), `internal/service/winhost/policy_windows.go`, `internal/service/winhost/install_windows.go`, `tools/accept/common.ps1`, `tools/accept/s26-uninstall.ps1`.
Не трогать: `internal/core`, `internal/sites`, `internal/storage`, `internal/ipc`, `internal/guard`, `frontend/`, `docs/`, `plans/`, версию, `windows.yml`.

## Interfaces

```go
// ports.go
type Policy interface {
	SetTaskMgrDisabled(sid string, disabled bool) error
	// SetBraveRules приводит ветку HKU\<sid>\Software\Policies\BraveSoftware\Brave к r (RM4-10, T5-3);
	// ветка не загружена — через профиль (viaProfile); профиля нет — nil.
	SetBraveRules(sid string, r BraveRules) error
	// BraveRulesOK — ветка совпадает с r; ветка пользователя не загружена — (true, nil) (T5-4).
	BraveRulesOK(sid string, r BraveRules) (bool, error)
	RemoveBraveRules(sid string) error // снять ветку целиком; нет — nil
}
// brave.go
type BraveRules struct{ Blocked []string }
type BraveValue struct { Key, Name string; DWord bool; Num uint32; Str string }
func (r BraveRules) Values() []BraveValue
func BravePlan(have, want []BraveValue) (set, del []BraveValue, delKeys []string)
func (s *Service) braveRules() BraveRules          // из s.braveBlocked
func (s *Service) applyBrave(cs *childState)       // запись + logRate; под s.mu
func (s *Service) housekeepBrave(real time.Time)   // T5-8
// winhost/policy_windows.go
type policy struct { mu sync.Mutex; base string } // base "" — bravePolicyPath
func readBrave(root registry.Key, path string) ([]service.BraveValue, bool /*ветка есть*/, error)
func applyBravePlan(root registry.Key, path string, set, del []service.BraveValue, delKeys []string) error
```
`&policy{}` в `host_windows.go:78` остаётся (нулевой `base` — настоящий путь).

## Шаги (каждый: RED → GREEN → мутации → коммит)

### A. Чистые функции (`brave.go`, T5-2, T5-3)

RED `TestBraveValues`: `BraveRules{Blocked: []string{"b.example", "a.example", "", "a.example"}}.Values()` = `[{"" TorDisabled DWord 1} {"" BraveVPNDisabled DWord 1} {"" DnsOverHttpsMode Str "off"} {ExtensionInstallBlocklist 1 Str "*"} {URLBlocklist 1 Str "a.example"} {URLBlocklist 2 Str "b.example"}]`; `BraveRules{}` — первые 4 без `URLBlocklist`; 1001 домен `fmt.Sprintf("s%04d.example", i)` → `URLBlocklist` ровно 1000, последний `"s0999.example"`.
RED `TestBravePlan` (таблица; `want := BraveRules{Blocked: []string{"a.example"}}.Values()`):
- `have = nil` (ветки нет) → `set == want`, `del`, `delKeys` пусты;
- `have == want` → всё пусто;
- `have` = want без `TorDisabled` → `set = [{"" TorDisabled DWord 1}]`;
- `have` с `TorDisabled` DWORD 0 → `set = [TorDisabled 1]`; с `TorDisabled` **строкой** "1" → `set = [TorDisabled DWord 1]` (тип важен);
- `have` = want + `{URLBlocklist 2 Str "old.example"}` → `del = [{URLBlocklist 2 …}]`;
- `have` = want + `{"" BraveRewardsDisabled DWord 1}` → `del` его (ветка целиком программы);
- `have` = want + подключ `{Proxy, …}` → `delKeys = ["Proxy"]`; лишнее значение в нём в `del` не дублируется;
- `have` с `URLBlocklist 1 = "b.example"` при want `a.example` → `set = [{URLBlocklist 1 Str "a.example"}]`.
Сравнение `Key`/`Name` — без учёта регистра (реестр регистронезависим): `have` с `torDisabled` DWORD 1 → совпадает.

Мутации A: без сортировки → порядок `URLBlocklist`; без предела 1000 → длина; сравнение без типа → случай «строкой»; регистрозависимое сравнение → `torDisabled` даёт `set`.
Коммит: `feat: правила Brave и план их записи как чистые данные`.

### B. Порт, фейк, служба (T5-1, T5-4…T5-8)

`fakePolicy`: поля `brave map[string][]BraveValue` (что лежит в ветке), `braveErr error`, `unloaded map[string]bool` (ветка не загружена: `BraveRulesOK` → `(true, nil)`), `braveLog []string` (`"set <sid> N"`, `"ok <sid>"`, `"rm <sid>"`); `SetBraveRules` кладёт `r.Values()`, `BraveRulesOK` сравнивает через `BravePlan`. Помощник `tamper(sid)` — убрать `TorDisabled`. `noPolicy` — три метода (`nil`, `(true, nil)`, `nil`).

RED `TestBraveRulesAtStart` (`siteHarness` из T4a или `newHarness(t, configured(), withChild(childSID, "Миша", 120), withChild(child2SID, "Даша", 60))` + `store.sites["example.com"]="blocked"`, `store.sites["school.example"]="study"`): после `start` → `h.policy.brave[childSID]` = `BraveRules{Blocked: ["example.com"]}.Values()`, у Даши так же, у `parentSID` — нет записи; журнал `brave_restored` пуст.
RED `TestBraveRulesFollowLists`: `AddSite("other.example","blocked")` (T4b; если T4b ещё нет — `store.sites[...]` + `h.svc.sitesChanged(h.real.Now())` под `mu`) → один `step` → `URLBlocklist` = `["example.com", "other.example"]`; `RemoveSite("example.com")` → шаг → `["other.example"]`; `braveDue == false` после шага.
RED `TestBraveRulesRestoredOncePer10Min`: `h.policy.tamper(childSID)`; 59 шагов — без записи; 60-й — ветка снова полная, `h.eventTexts("brave_restored") = ["Миша: правила Brave восстановлены"]`, в `h.logText()` «правила Brave у Миша восстановлены»; ещё `tamper` + 60 шагов → ветка полная, записей журнала по-прежнему 1; через 10 мин от первой (`h.run(600, …)` с `tamper` перед последней минутой) → 2. Без `tamper` 180 шагов — `braveLog` содержит только `ok`, журнал пуст (Review Focus 6).
RED `TestBraveRulesUnloadedNotChecked`: `h.policy.unloaded[childSID] = true`, `tamper` → 120 шагов → записи нет (T5-4); `childLogsIn` (снимается `unloaded`) → правила записаны в тот же `Housekeep` (`sessions.go:246`).
RED `TestBraveRulesWaitForLists`: `failSites = true` до `start` → `brave` пуст, `braveLog` пуст, в журнале работы «правила Brave не пишутся: списки сайтов ещё не прочитаны»; снять флаг, `h.real.Advance(rulesRetryEvery)`, шаг → записано. Второй подтест: после удачного старта `failSites = true`, `sitesChanged` → шаг → ветка с прежним `example.com` (T5-5).
RED `TestBraveRulesRemovedWithChild`: `SetChildren{Remove: [<Даша>]}` → `braveLog` содержит `"rm " + child2SID`, `h.policy.brave[child2SID]` нет; `Add` новой учётки → `"set <sid> 5"` сразу в ответе (без шага).
RED `TestBraveRulesErrorDoesNotStopService`: `h.policy.braveErr = errFake` → `start`, `run(3, activeSample)` — учёт идёт, `h.logText()` содержит «правила Brave для», не чаще раза в 60 с на SID.

Мутации B: сверка без проверки периода → `braveLog` растёт каждую секунду; журнал по `watchOnce` → вторая запись раньше 10 мин (после `pruneWatchLimits`); запись при `!braveReady` → пустой `URLBlocklist` после сбоя; без `RemoveBraveRules` в `hSetChildren` → ветка у снятой; журнал при `braveDue` → запись при `AddSite`.
Коммиты: `feat: писать правила Brave при старте, входе и изменении списков`, `feat: сверять правила Brave раз в минуту и восстанавливать`.

### C. Адаптер Windows (T5-3, T5-9…T5-12)

`policy_windows.go`: константа `bravePolicyPath`, `withHive` (T5-11), `readBrave` (значения ветки: DWORD → `GetIntegerValue`, SZ/EXPAND_SZ → `GetStringValue`, иное — значение с `DWord=false, Str=""` и именем, чтобы план его удалил; подключи — по одному уровню вглубь; подключ глубже — в `delKeys` целиком), `applyBravePlan` (удаление подключа — рекурсивно: `ReadSubKeyNames` → `DeleteKey` с листьев), `SetBraveRules` = загружена? `readBrave`+план+исполнение : `withHive`; `BraveRulesOK` (T5-4); `RemoveBraveRules` — рекурсивное удаление `bravePolicyPath`, затем пустой `Software\Policies\BraveSoftware` тоже удалить, если в нём ничего нет (не трогать, если там есть другие подключи). `install_windows.go` — T5-12.

RED (windows-latest) `policy_brave_windows_test.go`, `p := &policy{base: \`Software\PCBlockTest\BraveSoftware-\` + strconv.Itoa(os.Getpid())}`, `sid` — свой:
- `TestBravePolicyPath`: `bravePolicyPath == \`Software\Policies\BraveSoftware\Brave\``.
- `TestBraveAdapterWriteCheckRemove`: `BraveRulesOK(sid, r)` до записи → `false`; `SetBraveRules(sid, BraveRules{Blocked: {"a.example","b.example"}})` → `nil`; прочитать `registry.USERS, sid+\`\`+base`: `TorDisabled` (`GetIntegerValue` → 1, тип `registry.DWORD`), `BraveVPNDisabled` 1, `DnsOverHttpsMode` "off" (`registry.SZ`), `ExtensionInstallBlocklist\1` "*", `URLBlocklist\1` "a.example", `\2` "b.example"; `BraveRulesOK` → `true`; дописать руками `URLBlocklist\3="c.example"` и подключ `Proxy\x=1` → `BraveRulesOK` → `false`; `SetBraveRules` с одним `a.example` → `URLBlocklist\2`, `\3`, `Proxy` исчезли, `TorDisabled` всё время на месте (прочитать до и после — `1`); `RemoveBraveRules` → ключа нет, `BraveRulesOK` → `false`; второй `RemoveBraveRules` → `nil`.
- `TestBraveAdapterUnloadedHive`: `BraveRulesOK("S-1-5-21-0-0-0-4242", r)` → `(true, nil)`; `SetBraveRules` с тем же SID → `nil` (профиля нет — запись отложена).

Мутации C (руками, по одной, в CI не гонять — описать в вердикте, что проверено чтением): удаление подключа без рекурсии → `Proxy` остаётся (тест падает); `BraveRulesOK` через `withHive` → второй тест загружает профиль (у несуществующего SID — `nil`, мутацию ловит только ревью: отметить).
Коммит: `feat: адаптер правил Brave в реестре учётки ребёнка`, `feat: снимать правила Brave при удалении программы` (вместе с `common.ps1`, `s26-uninstall.ps1`).

Прогон Windows-шага — через оркестратора: push → `ci.yml` (vet `GOOS=windows`) → `gh workflow run windows.yml --ref feature/m4-sites` (без сценария: тесты Go на Windows), затем `-f scenario=accept -f only=S26,S7` (уборка и удаление).

## Тексты (без длинных тире, родителю на «вы», `python3 tools/scan_ui_texts.py`)

- журнал родителя: «Миша: правила Brave восстановлены»;
- журнал работы: «правила Brave у %s восстановлены: записано %d, удалено %d значений, %d подключей», «правила Brave для %s не записаны: %v», «правила Brave не пишутся: списки сайтов ещё не прочитаны», «в ветке %s не было Software\Policies».

## Среда и команды прогона

```
export PATH=~/.local/go1.27/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-mod=mod GOPROXY=off
go test -race -count=1 ./internal/service/ ./internal/service/winhost/
go vet ./internal/service/ ./internal/service/winhost/
gofmt -l internal
python3 tools/scan_ui_texts.py
bash tools/codemap.sh   # docs/CODEMAP.md — вместе с последним коммитом кода
```
`./internal/service/winhost/` на Linux собирает только файлы без `_windows` (стандартная библиотека) — локально идёт. `policy_windows.go` и тест адаптера проверяет только CI (`ci.yml` — `GOOS=windows go vet`, `windows.yml` — тесты).

## Review Focus (самопроверка)

1. PL-21: запись не теряет соседние значения ветки — план только добавляет недостающее и удаляет чужое; `TorDisabled` не пропадает ни на один шаг записи.
2. PL-18/PL-19, Review Focus 2: правила только у отмеченных детей; родитель и неотмеченные учётки не получают ветку никогда; снятие отметки снимает её.
3. Сверка без расхождения ничего не пишет; журнал родителя не чаще раза в 10 мин на ребёнка.
4. Сбой базы (`!braveReady`) не стирает «Запрещённые» в Brave.
5. `RegLoadKey` не чаще, чем при старте, `braveDue` и снятии (T5-4) — вход ребёнка не ломается.
6. Гонки: `housekeepBrave` под `s.mu`; адаптер держит свой `p.mu` (одна загрузка профиля за раз).

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

- **PL-18** (служебное — по точным префиксам, сухой прогон): правила пишутся только в ветки SID из `s.children`; никаких HKLM, никаких «всем пользователям».
- **PL-19** (владелец по всем признакам): ребёнок определяется SID, не именем; сеанс другого пользователя не даёт записи.
- **PL-21** (частичная пересборка теряет соседние поля): ветка приводится к цели планом T5-3, а не стирается; сверка сравнивает всю ветку, а не пару значений.
- **PL-30** (строгий флаг): при сбое чтения списков действует прежний (строгий) набор «Запрещённых», а не пустой.
- **PL-41** (непроверенная ветка сломана): `RemoveBraveRules` и путь «ветка не загружена» исполняются тестом адаптера; `s26` проверяет снятие при удалении.
- **PL-9** («не знаю» = «не трогать»): неизвестный SID профиля — `nil`, без ошибок в журнале родителя.

## Коммиты

Conventional Commits, описание по-русски в повелительном наклонении; `git add <пути>` явно (никогда `-A`), **не пушить**. Каждый коммит оканчивается строками:
```
Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01S7we7pcbdmhag3As3Dpmze
```

## Вердикт (≤ 40 строк)

```
TASK T5: done | blocked | failed
Diff: sha + строка на коммит
Evidence: go test -race -count=1 ./internal/service/ ./internal/service/winhost/ (счётчики), go vet, gofmt -l, scan_ui_texts, RED-запуски (тест и причина), мутации (какой тест упал); CI: ci.yml (vet windows), windows.yml (TestBraveAdapter*, S26, S7) — номера прогонов от оркестратора
Interfaces: Policy (3 метода), BraveRules/BraveValue/BravePlan, evBraveRestored; для T10: путь ветки, что проверяет S20, уборка common.ps1
Rulings: T5-1…T5-12 подтверждены или изменены, своя цена ошибки
Escalations: отклонение от RM4-10 (третий метод, «привести к цели» вместо «удалить и записать»), изменённые ожидания старых тестов, сигнатуры x/sys, которые пришлось менять
Open: S20 (T10): кнопка VPN, пункт Tor, страница ошибки, права ребёнка на ветку; README «Отклонения от ТЗ» (T5-4, T5-9) — закрытие вехи
```
