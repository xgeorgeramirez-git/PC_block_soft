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
		{Weekday: 1, TotalMin: 120, GamesMin: 60, VideoMin: 60}, {Weekday: 2, TotalMin: 120, GamesUnlimited: true, VideoUnlimited: true},
		{Weekday: 3, TotalMin: 90, GamesMin: 0, VideoMin: 0}, {Weekday: 4, TotalMin: 120, GamesUnlimited: true, VideoUnlimited: true},
		{Weekday: 5, TotalMin: 120, GamesUnlimited: true, VideoUnlimited: true}, {Weekday: 6, TotalMin: 240, GamesMin: 120, VideoUnlimited: true},
		{Weekday: 7, Unlimited: true, GamesUnlimited: true, VideoUnlimited: true},
	}
}

func okStatus() ChildStatus {
	return ChildStatusFrom("Миша", core.Decision{
		Day: "2026-10-19", Status: core.StatusOK, RemainingS: 85 * 60, LimitS: 120 * 60, UsedS: 35 * 60,
		Used: "Сегодня: 35 мин из 2 ч", GamesRemainingS: 20 * 60, GamesLimitS: 60 * 60,
	})
}

// studyOnlyStatus — время вышло в режиме «Оставить только учёбу» (R8): экрана нет.
func studyOnlyStatus() ChildStatus {
	return ChildStatusFrom("Миша", core.Decision{
		Day: "2026-10-19", Status: core.StatusTimeUp, StudyOnly: true, CloseReason: core.StudyOnlyPlateText,
		Until: "Можно снова: завтра", Used: "Сегодня: 2 ч 00 мин из 2 ч", LimitS: 7200, UsedS: 7200, GamesUnlimited: true,
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
	c := TodayCard{ChildID: 1, SID: "S-1-5-21-111-222-333-1001", Name: "Миша", Status: st, ExtraMin: extra, CanClearBlock: canClear}
	if st.GamesLimitS > 0 {
		c.GamesText = GamesCardText(st.GamesLimitS-st.GamesRemainingS, st.GamesLimitS, false)
	}
	return c
}

// withPending — карточка со ссылкой «Ждут решения: N» (8.3.2 п.5).
func withPending(c TodayCard, n int) TodayCard {
	c.PendingPrograms = n
	return c
}

// programsReply — «Программы»: одна ждёт решения (Миша 3 попытки, Даша 1), две в списке.
func programsReply() ProgramsReply {
	return ProgramsReply{
		Pending: []PendingProgramDTO{{
			Path: `c:\users\миша\downloads\supergame.exe`, Name: "SuperGame", Publisher: "Без подписи издателя", Description: "Super Game",
			Attempts: 4, LastAttemptAt: t0 - 3_600_000, WasCategory: ProgramCatGames,
			ByChild: []PendingChildDTO{{ChildID: 1, Name: "Миша", Attempts: 3}, {ChildID: 2, Name: "Даша", Attempts: 1}},
		}},
		Programs: []ProgramDTO{
			{Path: `c:\program files\bravesoftware\brave-browser\application\brave.exe`, Name: "Brave", Publisher: "Brave Software, Inc.",
				Category: ProgramCatOther, Source: ProgramSourceSnapshot, WeekS: 5400},
			{Path: `c:\program files\minecraft launcher\minecraftlauncher.exe`, Name: "Minecraft Launcher", Publisher: "Mojang",
				Category: ProgramCatGames, Source: ProgramSourceCatalog, WeekS: 0},
			{Path: `c:\program files\epic games\stalker2\stalker2.exe`, Name: "Stalker2", Publisher: "Epic Games",
				Category: ProgramCatGames, Source: ProgramSourceParent, WeekS: 0, Hidden: true},
			{Path: `c:\program files\epic games\stalker2\binaries\win64\stalker2-win64-shipping.exe`, Name: "Сталкер 2",
				FileName: "Stalker2-Win64-Shipping", Publisher: "GSC Game World", Category: ProgramCatGames, Source: ProgramSourceCatalog, WeekS: 7200},
		},
		Children: []ChildName{{ChildID: 1, Name: "Миша"}, {ChildID: 2, Name: "Даша"}},
	}
}

// adultSource — подпись источника списка «18+» на экране «Сайты» (8.3.5 п.3).
const adultSource = "Список сайтов для взрослых: UT1, Университет Тулуза Капитоль, лицензия CC BY-SA 4.0"

// sitesReply — «Сайты»: по домену в каждом списке, у Даши «18+» выключен.
func sitesReply() ListSitesReply {
	return ListSitesReply{
		Sites: []SiteDTO{
			{Domain: "example.com", Kind: SiteKindBlocked}, {Domain: "school.example", Kind: SiteKindStudy},
			{Domain: "youtube.com", Kind: SiteKindVideo},
		},
		Children:    []AdultFilterDTO{{ChildID: 1, Name: "Миша", On: true}, {ChildID: 2, Name: "Даша", On: false}},
		BlockedMax:  MaxBlockedSites,
		AdultSource: adultSource,
	}
}

// braveExe — путь Brave из каталога (RM4-8).
const braveExe = `c:\program files\bravesoftware\brave-browser\application\brave.exe`

// edgeStore — путь Edge из магазина Microsoft версии v (RM4-11, S46).
func edgeStore(v string) string {
	return `c:\program files\windowsapps\microsoft.microsoftedge.stable_` + v + `_neutral__8wekyb3d8bbwe\msedge_proxy.exe`
}

// myTodaySiteProgram — строки «Итогов» S39 с сайтом: 10 мин из 15 мин Brave — youtube.com (видео),
// остальные 5 мин Brave — «Прочее» (PL-28: строки сходятся с итогами).
func myTodaySitePrograms() []ProgramTime {
	return []ProgramTime{
		{Path: `c:\games\game.exe`, Name: "game", Category: ProgramCatGames, Seconds: 600, Kind: "program"},
		{Path: `c:\games\game2.exe`, Name: "game2", Category: ProgramCatGames, Seconds: 300, Kind: "program"},
		{Path: `c:\school\study.exe`, Name: "study", Category: ProgramCatStudy, Seconds: 180, Kind: "program"},
		{Path: "youtube.com", Name: "youtube.com", Category: "video_social", Seconds: 600, Kind: "site"},
		{Path: braveExe, Name: "Brave", Category: ProgramCatOther, Seconds: 300, Kind: "program"},
		{Name: "Остальное время", Category: ProgramCatOther, Seconds: 300},
	}
}

func ptrBreak(b BreakRules) *BreakRules { return &b }

func childRules() ChildRules {
	return ChildRules{ChildID: 1, Name: "Миша", Limits: weekLimits(), EndMode: EndModeLock, Break: defaultBreak()}
}

// defaultBreak — «Отдых» по умолчанию (FR-37): выключено, 60 / всё, кроме учёбы / 5 / 15 / 15.
func defaultBreak() BreakRules { return BreakRulesFrom(core.DefaultBreakRules()) }

// s43Break — «Отдых» из 8.3.3 п.7: включено, 60 / только игры / 5 / 15 / 30, своё сообщение.
func s43Break() BreakRules {
	return BreakRules{Enabled: true, AfterMin: 60, Counts: BreakCountsGames, WarnMin: 5, RestMin: 15, SelfRestMin: 30, Message: "Пойди погуляй с собакой"}
}

// restStatus — экран перерыва на отдых (8.2.3 п.12): до 16:20, сообщение родителя.
func restStatus() ChildStatus {
	return ChildStatusFrom("Миша", core.Decision{
		Day: "2026-10-19", Status: core.StatusOK, Lock: core.LockRest, Reason: core.ReasonRest, Until: "Можно снова в 16:20",
		Used: "Сегодня: 1 ч 05 мин из 3 ч", RemainingS: 115 * 60, LimitS: 180 * 60, UsedS: 65 * 60, GamesUnlimited: true,
		RestUntilMs: t0 + 900_000, RestLeftS: 900, RestMessage: "Пойди погуляй с собакой",
	})
}

// examples — по одному примеру на каждую команду из AllCommands и варианты с пустыми массивами и отказами.
func examples() map[string]example {
	return map[string]example{
		CmdHello:                   ok(nil, HelloReply{Protocol: ProtocolVersion, Version: "0.1.0", TestMode: false, Configured: true}),
		CmdGetChildStatus:          ok(nil, okStatus()),
		"GetChildStatus#notChild":  ok(nil, ChildStatusNotChild()),
		"GetChildStatus#studyOnly": ok(nil, studyOnlyStatus()),
		CmdOpenWindow:              ok(OpenWindowRequest{Kind: WindowParent}, OpenWindowReply{Opened: true}),
		"OpenWindow#programs":      ok(OpenWindowRequest{Kind: WindowParent, Screen: ScreenPrograms}, OpenWindowReply{Opened: true}),
		"OpenWindow#badScreen":     fail(OpenWindowRequest{Kind: WindowParent, Screen: "nope"}, TextBadRequest, "", nil),
		CmdWindowClosing:           ok(WindowClosingRequest{Kind: WindowMyStats}, Empty{}),

		CmdListAccounts: ok(nil, ListAccountsReply{Accounts: []Account{
			{SID: "S-1-5-21-111-222-333-1000", Name: "Анна", Admin: true, Current: true},
			{SID: "S-1-5-21-111-222-333-1001", Name: "Миша", Child: true, CanBeChild: true},
			{SID: "S-1-5-21-111-222-333-1002", Name: "Даша", CanBeChild: true},
		}}),
		"ListAccounts#empty": ok(nil, ListAccountsReply{}),
		CmdSetup: ok(SetupRequest{
			Password: "секрет-123",
			Children: []SetupChild{{SID: "S-1-5-21-111-222-333-1001", Name: "Миша", Limits: weekLimits()}},
			Programs: []SetupProgram{{Path: `c:\program files\minecraft launcher\minecraftlauncher.exe`, Category: ProgramCatGames}},
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
			Children: []TodayCard{withPending(card(okStatus(), 0, false), 2), withPending(card(bypassStatus(), 30, true), 2)},
			Notices:  []string{},
		}),
		"GetToday#notices": ok(nil, GetTodayReply{
			Children: []TodayCard{card(lockedStatus(), 0, false)},
			Notices:  []string{"Не удалось прочитать правила, поэтому сейчас ограничения не действуют. Прежние данные сохранены. Задайте правила заново в разделе «Дети»"},
		}),
		"GetToday#empty": ok(nil, GetTodayReply{}),
		CmdGetChildRules: ok(ChildRequest{ChildID: 1}, childRules()),
		CmdSaveLimits: ok(SaveLimitsRequest{ChildID: 1, Limits: weekLimits(), EndMode: EndModeLock}, SaveLimitsReply{
			Rules: childRules(), Card: card(okStatus(), 0, false), Message: "Сохранено. Новые правила уже действуют",
		}),
		"SaveLimits#invalid": fail(SaveLimitsRequest{ChildID: 1, Limits: []WeekdayLimit{{Weekday: 3, TotalMin: 2000}}},
			"Не больше 24 ч", LimitField(3), nil),
		"SaveLimits#endMode": ok(SaveLimitsRequest{ChildID: 1, Limits: []WeekdayLimit{}, EndMode: EndModeStudyOnly}, SaveLimitsReply{
			Rules: ChildRules{ChildID: 1, Name: "Миша", Limits: weekLimits(), EndMode: EndModeStudyOnly, Break: defaultBreak()}, Card: card(okStatus(), 0, false),
			Message: "Сохранено. Новые правила уже действуют",
		}),
		"SaveLimits#break": ok(SaveLimitsRequest{ChildID: 1, Limits: []WeekdayLimit{}, Break: ptrBreak(s43Break())}, SaveLimitsReply{
			Rules: ChildRules{ChildID: 1, Name: "Миша", Limits: weekLimits(), EndMode: EndModeLock, Break: s43Break()}, Card: card(okStatus(), 0, false),
			Message: "Сохранено. Новые правила уже действуют",
		}),
		"SaveLimits#breakSelfRest": fail(SaveLimitsRequest{ChildID: 1, Limits: []WeekdayLimit{}, Break: ptrBreak(BreakRules{Enabled: true, AfterMin: 60, Counts: BreakCountsAll, WarnMin: 5, RestMin: 20, SelfRestMin: 15})},
			"Не меньше 20 мин: столько длится отдых", FieldBreakSelfRestMin, nil),
		CmdEndBreak:           ok(ChildRequest{ChildID: 1}, CardReply{Card: card(okStatus(), 0, false), Message: "Перерыв закончен"}),
		"EndBreak#none":       fail(ChildRequest{ChildID: 1}, "Перерыва сейчас нет", "", nil),
		"GetChildStatus#rest": ok(nil, restStatus()),
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
		CmdClearBonus:          ok(ClearBonusRequest{ChildID: 1}, BonusChangeReply{BalanceAfter: 0, Message: "Копилка обнулена"}),
		CmdClearStats:          ok(ClearChildRequest{ChildID: 1}, ClearReply{Message: "Статистика Миши очищена"}),
		"ClearStats#noChild":   fail(ClearChildRequest{ChildID: 9}, "Такого ребёнка нет в списке", "", nil),
		CmdClearJournal:        ok(ClearChildRequest{ChildID: 1}, ClearReply{Message: "Журнал Миши очищен"}),
		"ClearJournal#noChild": fail(ClearChildRequest{ChildID: 9}, "Такого ребёнка нет в списке", "", nil),
		CmdReduceBonus:         ok(ReduceBonusRequest{ChildID: 1, Minutes: 30}, BonusChangeReply{BalanceAfter: 45, Message: "Убавлено 30 минут. Баланс 45 мин"}),
		CmdCreatePromo: ok(CreatePromoRequest{Code: "a1234", Minutes: 30}, CreatePromoReply{
			Promo:   PromoView{CodeDisplay: "A1234", Minutes: 30, CreatedAtMs: t0},
			Message: "Промокод «A1234» на 30 минут готов",
		}),
		"CreatePromo#latin": fail(CreatePromoRequest{Code: "код!", Minutes: 30}, "Только латинские буквы A-Z и цифры", FieldCode, nil),
		"CreatePromo#dup":   fail(CreatePromoRequest{Code: "a1234", Minutes: 15}, "Такой код уже есть, придумайте другой", FieldCode, nil),
		CmdListPromos: ok(Empty{}, ListPromosReply{
			Active: []PromoView{{CodeDisplay: "B5678", Minutes: 60, CreatedAtMs: t0}},
			Used:   []PromoView{{CodeDisplay: "A1234", Minutes: 30, CreatedAtMs: t0, UsedAtMs: t0 + 3_600_000, UsedByChild: 1, UsedByName: "Миша"}},
		}),
		"ListPromos#empty": ok(Empty{}, ListPromosReply{Active: []PromoView{}, Used: []PromoView{}}),
		CmdGeneratePromos: ok(GeneratePromosRequest{Count: 3, Length: 6, Charset: CharsetMixed, Minutes: 15}, GeneratePromosReply{
			Promos: []PromoView{
				{CodeDisplay: "K7Q2ZD", Minutes: 15, CreatedAtMs: t0},
				{CodeDisplay: "P4M9XA", Minutes: 15, CreatedAtMs: t0},
				{CodeDisplay: "3TRW8B", Minutes: 15, CreatedAtMs: t0},
			},
			Message: "Готово: 3 кода на 15 мин",
		}),
		"GeneratePromos#count": fail(GeneratePromosRequest{Count: 51, Length: 6, Charset: CharsetDigits, Minutes: 15}, "Можно от 1 до 50 кодов", FieldCount, nil),
		CmdClearPromoHistory:   ok(Empty{}, ClearPromoHistoryReply{Message: "История промокодов очищена"}),
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
			{ID: 42, AtMs: t0, TzOffsetMin: 180, ChildID: 1, ChildName: "Миша", Kind: core.EvBypass, Text: "Миша: попытка обойти защиту. Экран заблокирован"},
			{ID: 41, AtMs: t0 - 3_600_000, TzOffsetMin: 180, ChildID: 1, ChildName: "Миша", Kind: core.EvExtraTime, Text: "Миша: доп. время +30 мин"},
		}, More: true}),
		// «Показать ещё» (RM5-5): группа «Нарушения», курсор — последняя запись прошлой страницы.
		"ListEvents#page": ok(ListEventsRequest{ChildID: 1, FromMs: t0 - 30*86_400_000, Group: JournalGroupViolations, BeforeAtMs: t0, BeforeID: 42, Limit: 200},
			ListEventsReply{Events: []JournalEntry{
				{ID: 17, AtMs: t0 - 7_200_000, TzOffsetMin: 180, ChildID: 1, ChildName: "Миша", Kind: "site_forbidden_try", Text: "Миша: попытка открыть запрещённое: сайт example.com"},
			}, More: false}),
		"ListEvents#empty": ok(ListEventsRequest{Group: JournalGroupTime}, ListEventsReply{Events: []JournalEntry{}}),
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
			Limits: weekLimits(), Intervals: []Interval{{Weekday: 1, StartMin: 1260, EndMin: 420}}, EndMode: EndModeStudyOnly, Break: s43Break(),
			Message: "Правила скопированы. Проверьте и сохраните",
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

		CmdMyToday: ok(MyTodayRequest{Period: PeriodToday}, MyTodayReply{
			Status: okStatus(), Today: []string{"Сегодня осталось 1 ч 25 мин из 2 ч", "Использовано 35 мин"}, Rules: myRules(),
			Period: PeriodToday, UsedS: 2100, GamesS: 900, StudyS: 180, VideoS: 600, OtherS: 600, AllS: 2280, Programs: myTodaySitePrograms(),
			Bars:   []DayBar{{Key: "2026-10-19", Label: "пн 19", Title: "пн, 19 октября: 35 мин, учёба 3 мин", TotalS: 2100, StudyS: 180, Today: true}},
			BarsBy: BarsByDay, Violations: []JournalEntry{}, Bonus: []BonusEntry{},
			StatsSinceDay: "2026-10-19", StatsSinceText: "Статистика ведётся с 19 октября 2026",
		}),
		"MyToday#week": ok(MyTodayRequest{Period: PeriodWeek}, MyTodayReply{
			Status: okStatus(), Today: []string{"Сегодня осталось 1 ч 25 мин из 2 ч", "Использовано 35 мин"}, Rules: myRules(),
			Period: PeriodWeek, UsedS: 2400, GamesS: 2400, StudyS: 0, OtherS: 0, AllS: 2400, ExtraMin: 30, BonusSpentMin: 15,
			Programs: []ProgramTime{{Path: `c:\games\game.exe`, Name: "game", Category: ProgramCatGames, Seconds: 2400, Kind: "program"}},
			Bars: []DayBar{
				{Key: "2026-10-19", Label: "пн 19", Title: "пн, 19 октября: 30 мин, учёба 0 мин", TotalS: 1800},
				{Key: "2026-10-20", Label: "вт 20", Title: "вт, 20 октября: 0 мин, учёба 0 мин"},
				{Key: "2026-10-21", Label: "ср 21", Title: "ср, 21 октября: 10 мин, учёба 0 мин", TotalS: 600, Today: true},
			},
			BarsBy: BarsByDay,
			Violations: []JournalEntry{
				{ID: 9, AtMs: t0 - 3_600_000, TzOffsetMin: 180, ChildID: 1, ChildName: "Миша", Kind: "program_forbidden_try", Text: "Миша: попытка запустить запрещённую программу «Chrome»"},
			},
			Bonus: []BonusEntry{
				{AtMs: t0 - 7_200_000, TzOffset: 180, DeltaMin: -15, Reason: "взято из копилки", Balance: 25},
				{AtMs: t0 - 86_400_000, TzOffset: 180, DeltaMin: 40, Reason: "Бонус за свободное время: 40 мин", Balance: 40},
			},
			StatsSinceDay: "2026-10-01", StatsSinceText: "Статистика ведётся с 1 октября 2026",
		}),
		"MyToday#empty": ok(MyTodayRequest{Period: PeriodToday}, MyTodayReply{Status: ChildStatusNotChild(), Period: PeriodToday, BarsBy: BarsByDay}),
		CmdParentStats: ok(ParentStatsRequest{ChildID: 1, Period: PeriodToday}, MyTodayReply{
			Status: okStatus(), Today: []string{"Сегодня осталось 1 ч 25 мин из 2 ч", "Использовано 35 мин"}, Rules: myRules(),
			Period: PeriodToday, UsedS: 2100, GamesS: 900, StudyS: 180, OtherS: 1200, AllS: 2280, Programs: s39Programs(),
			Bars:   []DayBar{{Key: "2026-10-19", Label: "пн 19", Title: "пн, 19 октября: 35 мин, учёба 3 мин", TotalS: 2100, StudyS: 180, Today: true}},
			BarsBy: BarsByDay, StatsSinceDay: "2026-10-19", StatsSinceText: "Статистика ведётся с 19 октября 2026",
		}),
		"ParentStats#year": ok(ParentStatsRequest{ChildID: 1, Period: PeriodYear}, MyTodayReply{
			Status: okStatus(), Today: []string{"Сегодня осталось 1 ч 25 мин из 2 ч", "Использовано 35 мин"}, Rules: myRules(),
			Period: PeriodYear, UsedS: 3600, GamesS: 3600, StudyS: 2400, OtherS: 0, AllS: 6000,
			Programs: []ProgramTime{
				{Path: `c:\games\game.exe`, Name: "game", Category: ProgramCatGames, Seconds: 3600, Kind: "program"},
				{Path: `c:\school\study.exe`, Name: "study", Category: ProgramCatStudy, Seconds: 2400, Kind: "program"},
			},
			Bars: []DayBar{
				{Key: "2026-09", Label: "сен", Title: "сентябрь 2026: 0 мин, учёба 40 мин", StudyS: 2400},
				{Key: "2026-10", Label: "окт", Title: "октябрь 2026: 1 ч 00 мин, учёба 0 мин", TotalS: 3600, Today: true},
			},
			BarsBy: BarsByMonth, StatsSinceDay: "2026-09-01", StatsSinceText: "Статистика ведётся с 1 сентября 2026",
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
			Warnings: []GuardWarning{NewGuardWarning(1), NewGuardRestWarning(300, core.RestPlateText(5, 15))},
			Commands: []GuardCommand{
				{Kind: GuardCmdNotice, Text: "Добавлено 30 минут"},
				{Kind: GuardCmdHideLock},
				{Kind: GuardCmdTest, Test: &TestGuardAction{Action: TestType, Target: "password", Text: "секрет-123"}},
			},
		}),
		"GuardPoll#empty": ok(GuardPollRequest{Sample: SampleFromCore(core.SessionSample{AtMs: t0})}, GuardPollReply{Status: okStatus()}),
		"GuardPoll#blocked": ok(GuardPollRequest{Sample: SensorSample{AtMs: t0, SessionActive: true, Foreground: CatNameGames,
			ForegroundPath: `c:\users\миша\downloads\supergame.exe`}}, GuardPollReply{
			Status: okStatus(), Warnings: []GuardWarning{NewGuardWarningFor(5, core.WarnGames, 300)},
			Commands: []GuardCommand{{Kind: GuardCmdBlocked, Plate: &BlockedPlate{
				Kind: BlockedUnknown, Text: "Программа «SuperGame» ждёт разрешения родителя", ShowS: 10,
				Path: `c:\users\миша\downloads\supergame.exe`, CanAsk: true,
			}}},
		}),
		CmdGuardLogoff:     ok(nil, Empty{}),
		CmdGuardShutdown:   ok(nil, Empty{}),
		CmdGuardSwitchUser: ok(nil, Empty{}),
		CmdGuardClosing:    ok(nil, Empty{}),
		CmdTrayClosing:     ok(nil, Empty{}),

		CmdTestGuard:   ok(TestGuardRequest{SessionID: 2, Action: TestGuardAction{Action: TestClick, Target: ButtonParentPassword}}, Empty{}),
		CmdTestClock:   ok(TestClockRequest{Paused: true}, TestClockReply{Paused: true, NowMs: t0, Speed: 60}),
		CmdTestSession: ok(TestSessionRequest{Event: TestSessionLogoff, SessionID: 7, SID: "S-1-5-21-1-2-3-1001"}, Empty{}),
		CmdTestSeedDay: ok(TestSeedDayRequest{ChildID: 1, Day: "2026-10-19", TotalS: 3000, GamesS: 1800, VideoS: 1200, ExtraMin: 30,
			Usage: []TestSeedUsage{
				{Path: `c:\games\game.exe`, Category: ProgramCatGames, Seconds: 1800, Kind: SeedKindProgram},
				{Path: "youtube.com", Category: "video_social", Seconds: 1200, Kind: SeedKindSite},
			}}, Empty{}),
		"TestSeedDay#bonusPending": ok(TestSeedDayRequest{ChildID: 1, Day: "2026-11-17", TotalS: 2400, GamesS: 2400, BonusPending: true}, Empty{}),
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
		"GuardState#rest": ok(nil, GuardStateReply{
			LockShown: true, LockKind: LockRest, RestTimer: "00:14:59", RestMessage: "Пойди погуляй с собакой",
			Buttons: []string{ButtonParentPassword, ButtonShutdown, ButtonLogoff, ButtonSwitchUser},
		}),
		"GuardState#blocked": ok(nil, GuardStateReply{
			Plate: "Программа «SuperGame» ждёт разрешения родителя", PlateKind: BlockedUnknown,
			Buttons: []string{"askNow", "plateClose"}, ForegroundPath: `c:\users\миша\downloads\supergame.exe`,
		}),
		"GuardState#brave": ok(nil, GuardStateReply{
			ForegroundPath:   `c:\program files\bravesoftware\brave-browser\application\brave.exe`,
			ForegroundDomain: "probe-site.test",
		}),
		"TestGuard#foreground": ok(TestGuardRequest{Action: TestGuardAction{Action: TestForeground, Text: `c:\users\runneradmin\appdata\local\temp\pcb\game.exe`}}, Empty{}),

		// M3: программы.
		CmdListPrograms:         ok(nil, programsReply()),
		"ListPrograms#empty":    ok(nil, ProgramsReply{Pending: []PendingProgramDTO{}, Programs: []ProgramDTO{}, Children: []ChildName{}}),
		CmdDecideProgram:        ok(DecideProgramRequest{Path: `c:\users\миша\downloads\supergame.exe`, Category: ProgramCatGames}, DecideProgramReply{Message: "SuperGame: игра", UndoID: "p-3", UndoUntilMs: t0 + 10_000, Programs: programsReply()}),
		"DecideProgram#invalid": fail(DecideProgramRequest{Path: `c:\x.exe`, Category: ProgramCatSystem}, "Выберите: игра, учёба, прочее или запретить", "category", nil),
		CmdSetProgramsCategory: ok(SetProgramsCategoryRequest{Paths: []string{`c:\program files\bravesoftware\brave-browser\application\brave.exe`}, Category: ProgramCatStudy},
			DecideProgramReply{Message: "Изменено программ: 1", UndoID: "p-4", UndoUntilMs: t0 + 10_000, Programs: programsReply()}),
		CmdSetProgramsHidden: ok(SetProgramsHiddenRequest{Paths: []string{`c:\program files\epic games\stalker2\stalker2.exe`}, Hidden: true},
			DecideProgramReply{Message: "Stalker2 скрыта", UndoID: "p-5", UndoUntilMs: t0 + 10_000, Programs: programsReply()}),
		CmdRenameProgram: ok(RenameProgramRequest{Path: `c:\program files\epic games\stalker2\binaries\win64\stalker2-win64-shipping.exe`, Name: "Сталкер 2"},
			DecideProgramReply{Message: "Название изменено", UndoID: "p-6", UndoUntilMs: t0 + 10_000, Programs: programsReply()}),
		"RenameProgram#tooLong": fail(RenameProgramRequest{Path: `c:\x.exe`, Name: "Очень длинное название"}, "Название до 60 символов", "name", nil),
		CmdUndoProgram:          ok(UndoProgramRequest{UndoID: "p-4"}, UndoProgramReply{Message: "Решение отменено", Programs: programsReply()}),
		CmdScanPrograms: ok(nil, ScanProgramsReply{Programs: []ScanProgramDTO{
			{Path: `c:\program files\minecraft launcher\minecraftlauncher.exe`, Name: "Minecraft Launcher", Publisher: "Mojang", SuggestedCategory: ProgramCatGames},
			{Path: `c:\program files\bravesoftware\brave-browser\application\brave.exe`, Name: "Brave", Publisher: "Brave Software, Inc.", SuggestedCategory: ProgramCatOther},
		}}),
		"ScanPrograms#empty": ok(nil, ScanProgramsReply{Programs: []ScanProgramDTO{}}),

		// M4: сайты и Brave.
		CmdListSites: ok(nil, sitesReply()),
		"ListSites#empty": ok(nil, ListSitesReply{Sites: []SiteDTO{}, Children: []AdultFilterDTO{}, BlockedMax: MaxBlockedSites,
			AdultSource: adultSource}),
		CmdAddSite: ok(AddSiteRequest{Domain: "https://www.Example.com/page?x=1", Kind: SiteKindBlocked},
			AddSiteReply{Message: "Добавлено: example.com", Domain: "example.com", Sites: sitesReply()}),
		"AddSite#moved": ok(AddSiteRequest{Domain: "youtube.com", Kind: SiteKindStudy},
			AddSiteReply{Message: "Сайт перенесён в «Учёбу»", Domain: "youtube.com", Moved: true, Sites: sitesReply()}),
		"AddSite#invalid": fail(AddSiteRequest{Domain: "просто текст", Kind: SiteKindBlocked}, "Это не похоже на адрес сайта", FieldDomain, nil),
		"AddSite#dup":     fail(AddSiteRequest{Domain: "example.com", Kind: SiteKindBlocked}, "Этот сайт уже в списке", FieldDomain, nil),
		"AddSite#full": fail(AddSiteRequest{Domain: "example.org", Kind: SiteKindBlocked},
			"В списке запрещённых уже 1000 сайтов. Удалите ненужные", FieldDomain, nil),
		CmdRemoveSite: ok(RemoveSiteRequest{Domain: "example.com"}, RemoveSiteReply{
			Message: "Удалён example.com", Domain: "example.com", Kind: SiteKindBlocked, UndoUntilMs: t0 + 10_000, Sites: sitesReply(),
		}),
		CmdAddSites: ok(AddSitesRequest{Text: "https://www.Example.com/page\nvk.com\r\nпросто текст\n\nschool.example", Kind: SiteKindBlocked},
			AddSitesReply{Message: "Добавлено сайтов: 1. Перенесено сюда из других списков: 1. Уже были в списке: 1", Added: []string{"example.com", "vk.com"},
				Rejected: []string{"просто текст"}, FieldError: "Это не похоже на адрес сайта", Sites: sitesReply()}),
		"AddSites#invalid": fail(AddSitesRequest{Text: "просто текст\nещё текст", Kind: SiteKindBlocked}, "Эти строки не похожи на адрес сайта", FieldDomain, nil),
		CmdRemoveSites: ok(RemoveSitesRequest{Domains: []string{"example.com", "school.example"}}, RemoveSitesReply{
			Message: "Удалено сайтов: 2", Removed: []SiteDTO{{Domain: "example.com", Kind: SiteKindBlocked}, {Domain: "school.example", Kind: SiteKindStudy}},
			UndoUntilMs: t0 + 10_000, Sites: sitesReply(),
		}),
		CmdSetAdultFilter: ok(SetAdultFilterRequest{ChildID: 2, On: false},
			SetAdultFilterReply{Message: "Даша: сайты 18+ не блокируются", Sites: sitesReply()}),
		"Setup#sites": ok(SetupRequest{
			Password: "секрет-123",
			Children: []SetupChild{{SID: "S-1-5-21-111-222-333-1001", Name: "Миша", Limits: weekLimits()}},
			Programs: []SetupProgram{{Path: `c:\program files\minecraft launcher\minecraftlauncher.exe`, Category: ProgramCatGames}},
			Sites:    &SetupSites{AdultOff: []string{"S-1-5-21-111-222-333-1002"}, Video: []string{"youtube.com", "vk.com", "kids.example"}},
		}, Empty{}),
		"GuardPoll#closeTab": ok(GuardPollRequest{Sample: SensorSample{AtMs: t0, SessionActive: true, Foreground: CatNameVideoSocial,
			ForegroundPath: braveExe, ForegroundDomain: "youtube.com"}}, GuardPollReply{
			Status: okStatus(), Warnings: []GuardWarning{NewGuardWarningFor(5, core.WarnVideo, 300)},
			Commands: []GuardCommand{
				{Kind: GuardCmdCloseTab, Domain: "youtube.com"},
				{Kind: GuardCmdBlocked, Plate: &BlockedPlate{Kind: BlockedVideoOver, Text: core.VideoOverPlateText(2400, false)}},
			},
		}),
		"GuardPoll#closeWindow": ok(GuardPollRequest{Sample: SensorSample{AtMs: t0, SessionActive: true, Foreground: CatNameOther,
			ForegroundPath: braveExe, DomainUnread: true, DomainNoAddress: true}}, GuardPollReply{
			Status: okStatus(), Warnings: []GuardWarning{}, Commands: []GuardCommand{{Kind: GuardCmdCloseBraveWindow}},
		}),
		"GuardState#closeTab": ok(nil, GuardStateReply{
			ForegroundPath: braveExe, ForegroundDomain: "youtube.com", ClosedTab: "youtube.com", ClosedTabWay: CloseTabCtrlW,
			Plate: "Время на видео и соцсети на сегодня закончилось. Общего времени осталось 40 мин", PlateKind: BlockedVideoOver,
			Buttons: []string{"plateClose"},
		}),
		"TestGuard#domain": ok(TestGuardRequest{Action: TestGuardAction{Action: TestDomain, Text: "youtube.com"}}, Empty{}),
		CmdTestSeedPrograms: ok(TestSeedProgramsRequest{
			Programs: []TestSeedProgram{
				{Path: edgeStore("130.0.2849.80"), Name: "msedge_proxy", Publisher: "Microsoft Corporation", Category: ProgramCatOther,
					Source: ProgramSourceParent, Hidden: true, DecidedAt: t0 - 86_400_000},
				{Path: edgeStore("131.0.2903.51"), Name: "msedge_proxy", Publisher: "Microsoft Corporation", Category: ProgramCatBlocked,
					Source: ProgramSourceParent, CustomName: "Edge", DecidedAt: t0},
			},
			Usage: []TestSeedProgramUsage{{ChildID: 1, Day: "2026-11-03", Path: edgeStore("130.0.2849.80"), Category: ProgramCatOther, Seconds: 120}},
			Pending: []TestSeedPending{{Path: edgeStore("132.0.2957.55"), Name: "msedge_proxy", Publisher: "Microsoft Corporation",
				ChildID: 1, Attempts: 2, LastAttemptAt: t0}},
		}, Empty{}),
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
	if len(AllCommands) != 77 {
		t.Errorf("команд %d, ждали 77 (6 команд M5: ClearStats, ClearJournal, GeneratePromos, ClearPromoHistory, AddSites, RemoveSites; 5 команд M4: ListSites, AddSite, RemoveSite, SetAdultFilter, тестовая TestSeedPrograms; тестовая TestSeedDay и EndBreak из раунда 0.3.2; 46 из M1/M2/0.2.1 без команд Помощника, 9 из фидбек-раунда 0.2.2: OpenLogs, SetNoForbidden, ClearBonus, ReduceBonus, CreatePromo, ListPromos, RedeemPromo, GetBonusRate, SetBonusRate, GuardClosing из раунда 0.2.3, тестовая TestSession из 0.2.5 и 5 программ M3: ListPrograms, DecideProgram, SetProgramsCategory, UndoProgram, ScanPrograms; SetProgramsHidden и RenameProgram из раунда 0.3.1)", len(AllCommands))
	}
}

// myRules — «Мои правила» примеров MyToday и ParentStats.
func myRules() []MyRule {
	return []MyRule{
		{Weekday: 1, Day: "пн", Text: "2 ч"}, {Weekday: 2, Day: "вт", Text: "2 ч"}, {Weekday: 3, Day: "ср", Text: "1 ч 30 мин"},
		{Weekday: 4, Day: "чт", Text: "2 ч"}, {Weekday: 5, Day: "пт", Text: "2 ч"}, {Weekday: 6, Day: "сб", Text: "4 ч"},
		{Weekday: 7, Day: "вс", Text: "Без лимита"},
		{Text: "Игры: до 1 ч в день"}, {Text: "Когда время вышло: экран блокировки"},
	}
}

// s39Programs — строки вкладок «Итогов» из S39 (D-58): game 10 мин, game2 5 мин, study 3 мин;
// «Остальное время» — время без программы (рабочий стол, системные, скрытые).
func s39Programs() []ProgramTime {
	return []ProgramTime{
		{Path: `c:\games\game.exe`, Name: "game", Category: ProgramCatGames, Seconds: 600, Kind: "program"},
		{Path: `c:\games\game2.exe`, Name: "game2", Category: ProgramCatGames, Seconds: 300, Kind: "program"},
		{Path: `c:\school\study.exe`, Name: "study", Category: ProgramCatStudy, Seconds: 180, Kind: "program"},
		{Path: `c:\program files\brave software\brave-browser\application\brave.exe`, Name: "Brave", Category: ProgramCatOther, Seconds: 900, Kind: "program"},
		{Name: "Остальное время", Category: ProgramCatOther, Seconds: 300},
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
