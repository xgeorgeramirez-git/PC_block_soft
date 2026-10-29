package ipc

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"pcblock/internal/core"
)

// Контракт между Go и TypeScript (Ruling 14): для каждой команды пример запроса и ответа
// в testdata/contract.json. Файл коммитится; vitest разбирает те же примеры в api.ts.
// Обновить: go test ./internal/ipc/ -run Contract -update
var update = flag.Bool("update", false, "перезаписать testdata/contract.json")

const contractPath = "testdata/contract.json"

// example — запись контракта: Request — нагрузка запроса (нет у команд без данных),
// Reply — ответ целиком, как его получает окно. Ключ записи — имя команды; после «#»
// идёт вариант: пустые массивы, отказ.
type example struct {
	Request json.RawMessage `json:"request,omitempty"`
	Reply   Reply           `json:"reply"`

	reqType, replyType reflect.Type
}

func typeOf(v any) reflect.Type {
	if v == nil {
		return nil
	}
	return reflect.TypeOf(v)
}

// ok строит запись с успешным ответом.
func ok(req, reply any) example {
	return example{Request: Payload(req), Reply: OK(reply), reqType: typeOf(req), replyType: typeOf(reply)}
}

// fail строит запись с отказом; details — тип нагрузки отказа (может быть nil).
func fail(req any, text, field string, details any) example {
	r := Reply{Error: text, Field: field}
	if details != nil {
		r = FailWith(text, field, details)
	}
	return example{Request: Payload(req), Reply: r, reqType: typeOf(req), replyType: typeOf(details)}
}

const (
	t0 = int64(1_760_000_000_000) // пн 2025-10-09, момент для примеров
)

func weekLimits() []WeekdayLimit {
	return []WeekdayLimit{
		{Weekday: 1, TotalMin: 120}, {Weekday: 2, TotalMin: 120}, {Weekday: 3, TotalMin: 90},
		{Weekday: 4, TotalMin: 120}, {Weekday: 5, TotalMin: 120}, {Weekday: 6, TotalMin: 240},
		{Weekday: 7, Unlimited: true},
	}
}

func okStatus() ChildStatus {
	return ChildStatusFrom("Миша", core.Decision{
		Day: "2026-10-19", Status: core.StatusOK, RemainingS: 85 * 60, LimitS: 120 * 60, UsedS: 35 * 60,
		Used: "Сегодня: 35 мин из 2 ч",
	})
}

func lockedStatus() ChildStatus {
	return ChildStatusFrom("Миша", core.Decision{
		Day: "2026-10-19", Status: core.StatusTimeUp, Lock: core.LockTimeUp, Reason: core.ReasonTimeUp,
		Until: "Можно снова: завтра", Used: "Сегодня: 2 ч 00 мин из 2 ч", LimitS: 7200, UsedS: 7200,
	})
}

func bypassStatus() ChildStatus {
	return ChildStatusFrom("Миша", core.Decision{
		Day: "2026-10-19", Status: core.StatusBypass, Lock: core.LockBypass, Reason: core.ReasonBypass,
		Detail: "Изменено системное время", Until: core.UntilBypass, Used: "Сегодня: 35 мин из 2 ч", LimitS: 7200, UsedS: 2100,
	})
}

func card(st ChildStatus, extra int, canClear bool) TodayCard {
	return TodayCard{ChildID: 1, SID: "S-1-5-21-111-222-333-1001", Name: "Миша", Status: st, ExtraMin: extra, CanClearBlock: canClear}
}

func childRules() ChildRules {
	return ChildRules{ChildID: 1, Name: "Миша", Limits: weekLimits()}
}

// examples — по одному примеру на каждую команду из AllCommands и варианты с пустыми массивами и отказами.
func examples() map[string]example {
	return map[string]example{
		CmdHello:                  ok(nil, HelloReply{Protocol: ProtocolVersion, Version: "0.1.0", TestMode: false, Configured: true}),
		CmdGetChildStatus:         ok(nil, okStatus()),
		"GetChildStatus#notChild": ok(nil, ChildStatusNotChild()),
		CmdOpenWindow:             ok(OpenWindowRequest{Kind: WindowParent}, OpenWindowReply{Opened: true}),
		CmdWindowClosing:          ok(WindowClosingRequest{Kind: WindowMyStats}, Empty{}),

		CmdListAccounts: ok(nil, ListAccountsReply{Accounts: []Account{
			{SID: "S-1-5-21-111-222-333-1000", Name: "Анна", Admin: true, Current: true},
			{SID: "S-1-5-21-111-222-333-1001", Name: "Миша", Child: true, CanBeChild: true},
			{SID: "S-1-5-21-111-222-333-1002", Name: "Даша", CanBeChild: true},
		}}),
		"ListAccounts#empty": ok(nil, ListAccountsReply{}),
		CmdSetup: ok(SetupRequest{
			Password: "секрет-123",
			Children: []SetupChild{{SID: "S-1-5-21-111-222-333-1001", Name: "Миша", Limits: weekLimits()}},
		}, Empty{}),
		"Setup#invalid":   fail(SetupRequest{Password: "123"}, "Не меньше 6 символов", FieldPassword, nil),
		CmdResetPassword:  ok(ResetPasswordRequest{NewPassword: "новый-секрет"}, Empty{}),
		CmdUninstallCheck: ok(UninstallCheckRequest{Password: "секрет-123"}, UninstallCheckReply{Allowed: true}),
		"UninstallCheck#denied": fail(UninstallCheckRequest{Password: "не тот"}, "Неверный пароль. Осталось попыток: 4", FieldPassword,
			LoginDenied{AttemptsLeft: 4}),

		CmdLogin: ok(LoginRequest{Password: "секрет-123"}, LoginReply{Token: "9f2c1b7e4a6d4c0e8b53", ExpiresMs: t0 + 600_000, IdleTimeoutS: 600}),
		"Login#denied": fail(LoginRequest{Password: "не тот"}, "Неверный пароль. Осталось попыток: 3", FieldPassword,
			LoginDenied{AttemptsLeft: 3}),
		"Login#paused": fail(LoginRequest{Password: "не тот"}, "Слишком много попыток. Повторить можно через 0:60", FieldPassword,
			LoginDenied{PauseS: 60}),
		CmdExtendSession: ok(nil, SessionStateReply{Active: true, ExpiresMs: t0 + 600_000, RemainingS: 600}),
		CmdLogout:        ok(nil, Empty{}),
		CmdSessionState:  ok(nil, SessionStateReply{Active: true, ExpiresMs: t0 + 540_000, RemainingS: 540}),
		CmdGetToday: ok(nil, GetTodayReply{
			Children: []TodayCard{card(okStatus(), 0, false), card(bypassStatus(), 30, true)},
			Notices:  []string{},
		}),
		"GetToday#notices": ok(nil, GetTodayReply{
			Children: []TodayCard{card(lockedStatus(), 0, false)},
			Notices:  []string{"Не удалось прочитать правила, поэтому сейчас ограничения не действуют. Прежние данные сохранены. Задайте правила заново в разделе «Дети»"},
		}),
		"GetToday#empty": ok(nil, GetTodayReply{}),
		CmdGetChildRules: ok(ChildRequest{ChildID: 1}, childRules()),
		CmdSaveLimits: ok(SaveLimitsRequest{ChildID: 1, Limits: weekLimits()}, SaveLimitsReply{
			Rules: childRules(), Card: card(okStatus(), 0, false), Message: "Сохранено. Новые правила уже действуют",
		}),
		"SaveLimits#invalid": fail(SaveLimitsRequest{ChildID: 1, Limits: []WeekdayLimit{{Weekday: 3, TotalMin: 2000}}},
			"Не больше 24 ч", LimitField(3), nil),
		CmdAddExtra: ok(AddExtraRequest{ChildID: 1, Minutes: 30}, AddExtraReply{
			Card: card(okStatus(), 30, false), Message: "Добавлено 30 минут", ExtraID: "e-17", UndoUntilMs: t0 + 10_000,
		}),
		"AddExtra#invalid": fail(AddExtraRequest{ChildID: 1, Minutes: 500}, "Можно добавить от 1 до 240 минут", FieldMinutes, nil),
		CmdReduceExtra: ok(ReduceExtraRequest{ChildID: 1, Minutes: 30}, AddExtraReply{
			Card: card(okStatus(), 0, false), Message: "Убрано 30 минут", ExtraID: "e-18", UndoUntilMs: t0 + 10_000,
		}),
		"ReduceExtra#invalid": fail(ReduceExtraRequest{ChildID: 1, Minutes: 500}, "Можно убрать от 1 до 240 минут", FieldMinutes, nil),
		CmdClearExtra: ok(ClearExtraRequest{ChildID: 1}, AddExtraReply{
			Card: card(okStatus(), 0, false), Message: "Доп. время убрано", ExtraID: "e-19", UndoUntilMs: t0 + 10_000,
		}),
		CmdGetDiag:  ok(Empty{}, DiagReply{Verbose: false, LogsPath: `C:\ProgramData\PCBlock\logs`}),
		CmdSetDiag:  ok(SetDiagRequest{Verbose: true}, DiagReply{Verbose: true, LogsPath: `C:\ProgramData\PCBlock\logs`}),
		CmdOpenLogs: ok(Empty{}, OpenLogsReply{Path: `C:\ProgramData\PCBlock\logs`, Message: "Открываю папку с логами"}),

		CmdSetNoForbidden: ok(SetNoForbiddenRequest{ChildID: 1, On: true}, SetNoForbiddenReply{
			Card: card(okStatus(), 0, false), Message: "Сегодня запретные часы не действуют",
		}),
		CmdClearBonus:  ok(ClearBonusRequest{ChildID: 1}, BonusChangeReply{BalanceAfter: 0, Message: "Копилка обнулена"}),
		CmdReduceBonus: ok(ReduceBonusRequest{ChildID: 1, Minutes: 30}, BonusChangeReply{BalanceAfter: 45, Message: "Убавлено 30 минут. Баланс 45 мин"}),
		CmdCreatePromo: ok(CreatePromoRequest{Code: "a1234", Minutes: 30}, CreatePromoReply{
			Promo:   PromoView{CodeDisplay: "a1234", Minutes: 30, CreatedAtMs: t0},
			Message: "Промокод «a1234» на 30 минут готов",
		}),
		"CreatePromo#dup": fail(CreatePromoRequest{Code: "a1234", Minutes: 15}, "Такой код уже есть, придумайте другой", FieldCode, nil),
		CmdListPromos: ok(Empty{}, ListPromosReply{
			Active: []PromoView{{CodeDisplay: "b5678", Minutes: 60, CreatedAtMs: t0}},
			Used:   []PromoView{{CodeDisplay: "a1234", Minutes: 30, CreatedAtMs: t0, UsedAtMs: t0 + 3_600_000, UsedByChild: 1, UsedByName: "Миша"}},
		}),
		"ListPromos#empty": ok(Empty{}, ListPromosReply{Active: []PromoView{}, Used: []PromoView{}}),
		CmdRedeemPromo: ok(RedeemPromoRequest{ChildID: 1, Code: "a1234"}, RedeemPromoReply{
			Card: card(okStatus(), 30, false), AddedMin: 30, Message: "Добавлено 30 минут",
		}),
		"RedeemPromo#bad": fail(RedeemPromoRequest{ChildID: 1, Code: "нет"}, "Код не подходит", FieldCode, nil),
		CmdGetBonusRate:   ok(Empty{}, BonusRateReply{Minutes: 5}),
		CmdSetBonusRate:   ok(SetBonusRateRequest{Minutes: 10}, BonusRateReply{Minutes: 10}),
		CmdUndoExtra: ok(UndoExtraRequest{ChildID: 1, ExtraID: "e-17"}, CardReply{
			Card: card(okStatus(), 0, false), Message: "Добавленное время отменено",
		}),
		CmdClearBlock: ok(ChildRequest{ChildID: 1}, CardReply{
			Card: card(okStatus(), 0, false), Message: "Блокировка снята",
		}),
		CmdListEvents: ok(ListEventsRequest{ChildID: 0, FromMs: t0 - 86_400_000, ToMs: 0, Limit: 50}, ListEventsReply{Events: []JournalEntry{
			{AtMs: t0, TzOffsetMin: 180, ChildID: 1, ChildName: "Миша", Kind: core.EvBypass, Text: "Миша: попытка обойти защиту. Экран заблокирован"},
			{AtMs: t0 - 3_600_000, TzOffsetMin: 180, ChildID: 1, ChildName: "Миша", Kind: core.EvExtraTime, Text: "Миша: доп. время +30 мин"},
		}}),
		"ListEvents#empty": ok(ListEventsRequest{}, ListEventsReply{}),
		CmdChangePassword:  ok(ChangePasswordRequest{OldPassword: "секрет-123", NewPassword: "новый-секрет"}, Empty{}),
		CmdSetChildren: ok(SetChildrenRequest{
			Add:    []ChildAdd{{SID: "S-1-5-21-111-222-333-1002", Name: "Даша"}},
			Remove: []int64{3},
			Rename: []ChildRename{{ChildID: 1, Name: "Михаил"}},
		}, SetChildrenReply{Children: []ChildInfo{
			{ChildID: 1, SID: "S-1-5-21-111-222-333-1001", Name: "Михаил"},
			{ChildID: 4, SID: "S-1-5-21-111-222-333-1002", Name: "Даша"},
		}}),
		"SetChildren#empty": ok(SetChildrenRequest{}, SetChildrenReply{}),

		// M2: расписание, пауза, копилка.
		CmdGetSchedule: ok(GetScheduleRequest{ChildID: 1}, GetScheduleReply{ChildID: 1, Intervals: []Interval{
			{Weekday: 1, StartMin: 1260, EndMin: 420},
		}}),
		"GetSchedule#empty": ok(GetScheduleRequest{ChildID: 1}, GetScheduleReply{ChildID: 1, Intervals: []Interval{}}),
		CmdSaveSchedule: ok(SaveScheduleRequest{ChildID: 1, Intervals: []Interval{{Weekday: 1, StartMin: 1260, EndMin: 420}}}, SaveScheduleReply{
			Intervals: []Interval{{Weekday: 1, StartMin: 1260, EndMin: 420}}, Card: card(okStatus(), 0, false), Message: "Сохранено. Новые правила уже действуют",
		}),
		"SaveSchedule#invalid": fail(SaveScheduleRequest{ChildID: 1, Intervals: []Interval{{Weekday: 1, StartMin: 600, EndMin: 600}}},
			"Начало и конец совпадают", FieldEnd, nil),
		CmdPause: ok(PauseRequest{ChildID: 1, Preset: PauseHour1}, PauseReply{Card: card(okStatus(), 0, false), Message: "Пауза до 18:30"}),
		CmdPauseAll: ok(PauseAllRequest{Preset: PauseHour1}, PauseAllReply{
			Children: []TodayCard{card(okStatus(), 0, false)}, Message: "Пауза для всех детей на 1 ч",
		}),
		"PauseAll#empty": ok(PauseAllRequest{Preset: PauseHour2}, PauseAllReply{}),
		CmdGrantBonus: ok(GrantBonusRequest{ChildID: 1, Minutes: 30, Reason: "за уборку"}, GrantBonusReply{
			BalanceAfter: 75, Message: "Начислено 30 минут. Баланс 75 мин", BonusID: "b-21", UndoUntilMs: t0 + 10_000,
		}),
		"GrantBonus#invalid": fail(GrantBonusRequest{ChildID: 1, Minutes: 600, Reason: "много"}, "Можно начислить не больше 540 мин", FieldMinutes, nil),
		CmdUndoBonus:         ok(UndoBonusRequest{ChildID: 1, BonusID: "b-21"}, UndoBonusReply{BalanceAfter: 45, Message: "Начисление отменено"}),
		CmdGetBonus: ok(GetBonusRequest{ChildID: 1}, GetBonusReply{ChildID: 1, Balance: 45, Entries: []BonusEntry{
			{AtMs: t0, TzOffset: 180, DeltaMin: 30, Reason: "за уборку", Balance: 45},
			{AtMs: t0 - 3_600_000, TzOffset: 180, DeltaMin: -15, Reason: "взято из копилки", Balance: 15},
		}}),
		"GetBonus#empty": ok(GetBonusRequest{ChildID: 1}, GetBonusReply{ChildID: 1, Entries: []BonusEntry{}}),
		CmdCopyRules: ok(CopyRulesRequest{FromChildID: 2, ToChildID: 1}, CopyRulesReply{
			Limits: weekLimits(), Intervals: []Interval{{Weekday: 1, StartMin: 1260, EndMin: 420}}, Message: "Правила скопированы. Проверьте и сохраните",
		}),
		CmdAskMore: ok(AskMoreRequest{ChildID: 1, Minutes: 30, Password: "секрет-123"}, AskMoreReply{
			Card: card(okStatus(), 30, false), Message: "Добавлено 30 минут",
		}),
		"AskMore#denied": fail(AskMoreRequest{ChildID: 1, Minutes: 30, Password: "не тот"}, "Неверный пароль. Осталось попыток: 4", FieldPassword,
			LoginDenied{AttemptsLeft: 4}),
		CmdSpendBonus: ok(SpendBonusRequest{ChildID: 1, Minutes: 30}, SpendBonusReply{
			Card: card(okStatus(), 30, false), Message: "Взято из копилки 30 мин", BalanceAfter: 15,
		}),
		"SpendBonus#forbidden": fail(SpendBonusRequest{ChildID: 1, Minutes: 30}, "Из копилки можно взять с 07:00", FieldMinutes, nil),

		CmdMyToday: ok(nil, MyTodayReply{
			Status: okStatus(),
			Today:  []string{"Сегодня осталось 1 ч 25 мин из 2 ч", "Использовано 35 мин"},
			Rules: []MyRule{
				{Weekday: 1, Day: "пн", Text: "2 ч"}, {Weekday: 2, Day: "вт", Text: "2 ч"}, {Weekday: 3, Day: "ср", Text: "1 ч 30 мин"},
				{Weekday: 4, Day: "чт", Text: "2 ч"}, {Weekday: 5, Day: "пт", Text: "2 ч"}, {Weekday: 6, Day: "сб", Text: "4 ч"},
				{Weekday: 7, Day: "вс", Text: "Без лимита"},
			},
		}),
		"MyToday#empty": ok(nil, MyTodayReply{Status: ChildStatusNotChild()}),
		CmdParentStats: ok(ParentStatsRequest{ChildID: 1}, MyTodayReply{
			Status: okStatus(),
			Today:  []string{"Сегодня осталось 1 ч 25 мин из 2 ч", "Использовано 35 мин"},
			Rules: []MyRule{
				{Weekday: 1, Day: "пн", Text: "2 ч"}, {Weekday: 2, Day: "вт", Text: "2 ч"}, {Weekday: 3, Day: "ср", Text: "1 ч 30 мин"},
				{Weekday: 4, Day: "чт", Text: "2 ч"}, {Weekday: 5, Day: "пт", Text: "2 ч"}, {Weekday: 6, Day: "сб", Text: "4 ч"},
				{Weekday: 7, Day: "вс", Text: "Без лимита"},
			},
		}),
		CmdFirstRunSeen: ok(nil, Empty{}),

		CmdGuardPoll: ok(GuardPollRequest{
			Sample: SampleFromCore(core.SessionSample{AtMs: t0, TzOffsetMin: 180, LastInputAgoMs: 1200, Fullscreen: true, Foreground: core.CatGames, SessionActive: true}),
			Events: []GuardEvent{
				{Kind: GuardEvWarningShown, AtMs: t0 - 2000, Threshold: 5},
				{Kind: GuardEvButton, AtMs: t0 - 500, Button: ButtonWarningClose},
				{Kind: GuardEvBypass, AtMs: t0 - 100, Detail: "Изменено системное время"},
			},
		}, GuardPollReply{
			Status:   lockedStatus(),
			Warnings: []GuardWarning{NewGuardWarning(1)},
			Commands: []GuardCommand{
				{Kind: GuardCmdNotice, Text: "Добавлено 30 минут"},
				{Kind: GuardCmdHideLock},
				{Kind: GuardCmdTest, Test: &TestGuardAction{Action: TestType, Target: "password", Text: "секрет-123"}},
			},
		}),
		"GuardPoll#empty":  ok(GuardPollRequest{Sample: SampleFromCore(core.SessionSample{AtMs: t0})}, GuardPollReply{Status: okStatus()}),
		CmdGuardLogoff:     ok(nil, Empty{}),
		CmdGuardShutdown:   ok(nil, Empty{}),
		CmdGuardSwitchUser: ok(nil, Empty{}),
		CmdGuardClosing:    ok(nil, Empty{}),
		CmdTrayClosing:     ok(nil, Empty{}),

		CmdTestGuard:   ok(TestGuardRequest{SessionID: 2, Action: TestGuardAction{Action: TestClick, Target: ButtonParentPassword}}, Empty{}),
		CmdTestClock:   ok(TestClockRequest{Paused: true}, TestClockReply{Paused: true, NowMs: t0, Speed: 60}),
		CmdTestSession: ok(TestSessionRequest{Event: TestSessionLogoff, SessionID: 7, SID: "S-1-5-21-1-2-3-1001"}, Empty{}),
		CmdGuardState: ok(nil, GuardStateReply{
			Warnings:  []ShownWarning{{AtMs: t0 - 60_000, Threshold: 1, Text: "Осталась 1 минута: 0:59"}},
			Sounds:    []ShownSound{{AtMs: t0 - 60_000, Threshold: 1, OK: true}},
			LockShown: true,
			LockKind:  LockTimeUp,
			Buttons:   []string{ButtonParentPassword, ButtonShutdown, ButtonLogoff, ButtonSwitchUser},
			Fields:    []GuardField{{ID: "password", Text: ""}},
			Exits:     []string{ExitSwitchUser},
		}),
		"GuardState#empty": ok(nil, GuardStateReply{}),
	}
}

func marshalContract(t *testing.T) []byte {
	t.Helper()
	b, err := json.MarshalIndent(examples(), "", "  ")
	if err != nil {
		t.Fatalf("контракт не записывается: %v", err)
	}
	return append(b, '\n')
}

func TestContractMatchesFile(t *testing.T) {
	got := marshalContract(t)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(contractPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("контракт обновлён: %s (%d байт)", contractPath, len(got))
		return
	}
	want, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("нет %s: запустите go test ./internal/ipc/ -run Contract -update (%v)", contractPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("контракт устарел: типы или примеры изменились. Запустите go test ./internal/ipc/ -run Contract -update, " +
			"проверьте diff testdata/contract.json и поправьте frontend/src/api.ts")
	}
}

func commandOf(key string) string { return strings.SplitN(key, "#", 2)[0] }

func TestContractHasEveryCommand(t *testing.T) {
	have := map[string]bool{}
	for key := range examples() {
		have[commandOf(key)] = true
	}
	seen := map[string]bool{}
	for _, c := range AllCommands {
		if c == "" || seen[c] {
			t.Errorf("команда %q пустая или повторяется в AllCommands", c)
		}
		seen[c] = true
		if !have[c] {
			t.Errorf("у команды %s нет примера в контракте: добавьте её в examples()", c)
		}
	}
	for c := range have {
		if !seen[c] {
			t.Errorf("пример для %s есть, а в AllCommands её нет", c)
		}
	}
	if len(AllCommands) != 57 {
		t.Errorf("команд %d, ждали 57 (46 из M1/M2/0.2.1 без команд Помощника, 9 из фидбек-раунда 0.2.2: OpenLogs, SetNoForbidden, ClearBonus, ReduceBonus, CreatePromo, ListPromos, RedeemPromo, GetBonusRate, SetBonusRate, GuardClosing из раунда 0.2.3 и тестовая TestSession из 0.2.5)", len(AllCommands))
	}
}

// Ловит: команды удалённого Помощника (D-56) остались в канале или в примерах контракта.
func TestNoHelperCommands(t *testing.T) {
	gone := map[string]bool{"GetHelperState": true, "SaveHelperState": true}
	for _, c := range AllCommands {
		if gone[c] {
			t.Errorf("команда %s осталась в AllCommands", c)
		}
	}
	for key := range examples() {
		if gone[commandOf(key)] {
			t.Errorf("пример %s остался в контракте", key)
		}
	}
}

func TestContractHasBothEmptyAndFilledArrays(t *testing.T) {
	text := string(marshalContract(t))
	if !regexp.MustCompile(`\[\]`).MatchString(text) || !regexp.MustCompile(`\[\s*\{|\[\s*"|\[\s*\d`).MatchString(text) {
		t.Fatalf("в контракте должны быть и пустые, и непустые массивы")
	}
}

func TestContractHasNoNull(t *testing.T) {
	// null в ответе — это nil-срез или забытое поле (L-22).
	if strings.Contains(string(marshalContract(t)), "null") {
		t.Fatalf("в контракте есть null")
	}
}

func TestContractPayloadsMatchGoTypes(t *testing.T) {
	// Каждое поле примера принадлежит Go-типу и наоборот: строгий разбор и обратная запись дают тот же JSON.
	for key, ex := range examples() {
		check := func(what string, raw []byte, typ reflect.Type) {
			if typ == nil || len(raw) == 0 {
				return
			}
			ptr := reflect.New(typ)
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			if err := dec.Decode(ptr.Interface()); err != nil {
				t.Errorf("%s (%s): не разбирается в %s: %v", key, what, typ, err)
				return
			}
			back := Payload(ptr.Elem().Interface())
			var a, b bytes.Buffer
			if err := json.Compact(&a, raw); err != nil {
				t.Errorf("%s (%s): %v", key, what, err)
				return
			}
			_ = json.Compact(&b, back)
			if a.String() != b.String() {
				t.Errorf("%s (%s): после круга JSON другой\nбыло:  %s\nстало: %s", key, what, a.String(), b.String())
			}
		}
		check("запрос", ex.Request, ex.reqType)
		check("ответ", ex.Reply.Payload, ex.replyType)
		if ex.Reply.OK == (ex.Reply.Error != "") {
			t.Errorf("%s: ответ должен быть либо успехом, либо отказом с текстом: %+v", key, ex.Reply)
		}
	}
}

var lowerCamel = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)

// walkTags обходит тип и проверяет, что у каждого экспортируемого поля есть тег в нижнем верблюжьем регистре.
func walkTags(t *testing.T, typ reflect.Type, seen map[reflect.Type]bool) {
	if typ == nil || seen[typ] {
		return
	}
	seen[typ] = true
	switch typ.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		walkTags(t, typ.Elem(), seen)
	case reflect.Map:
		walkTags(t, typ.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if !f.IsExported() {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				t.Errorf("%s.%s: нет json-тега", typ, f.Name)
				continue
			}
			if !lowerCamel.MatchString(name) {
				t.Errorf("%s.%s: ключ %q не в нижнем верблюжьем регистре", typ, f.Name, name)
			}
			walkTags(t, f.Type, seen)
		}
	}
}

func TestJSONKeysAreLowerCamel(t *testing.T) {
	seen := map[reflect.Type]bool{}
	for _, ex := range examples() {
		walkTags(t, ex.reqType, seen)
		walkTags(t, ex.replyType, seen)
	}
	walkTags(t, reflect.TypeOf(Msg{}), seen)
	walkTags(t, reflect.TypeOf(Reply{}), seen)
	if len(seen) < 30 {
		t.Errorf("обойдено всего %d типов: проверка ничего не охватывает", len(seen))
	}
}

func TestContractKeysAreSorted(t *testing.T) {
	// Файл детерминирован: ключи упорядочены, повторная запись даёт те же байты.
	a, b := marshalContract(t), marshalContract(t)
	if !bytes.Equal(a, b) {
		t.Fatalf("запись контракта недетерминирована")
	}
	var keys []string
	for k := range examples() {
		keys = append(keys, k)
	}
	if !sort.StringsAreSorted(keys) {
		sort.Strings(keys)
	}
	if len(keys) < len(AllCommands) {
		t.Fatalf("записей %d меньше команд %d", len(keys), len(AllCommands))
	}
}
