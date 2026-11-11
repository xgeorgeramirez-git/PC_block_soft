package service

import "pcblock/internal/core"

// Виды событий журнала (R22, FR-24, RM5-5) — один перечень. Каждое место записи в журнал
// передаёт вид константой отсюда или из ядра (core.Ev*); каждый вид перечня — ровно в одной
// группе журнала (journalGroups). Проверяет TestEventKindsSourceScan разбором исходников.

// Пароль, настройка, удаление программы.
const (
	evSetupDone       = "setup_done"       // программа настроена: пароль родителя задан
	evPasswordFailed  = "password_failed"  // неверный пароль родителя
	evPasswordChanged = "password_changed" // пароль родителя изменён
	evPasswordReset   = "password_reset"   // пароль сброшен администратором (FR-26)
	evUninstallForgot = "uninstall_forgot" // удаление без пароля: пароль забыт
	evSettingsChanged = "settings_changed" // изменены лимиты, режим «Когда время вышло», отдых, запретные часы
	evChildrenChanged = "children_changed" // изменён список детских учёток
	evChildAdmin      = "child_admin"      // учётка ребёнка стала администратором (R1): не нарушение
	evChildAdminOff   = "child_admin_off"  // права администратора сняты
	evStatsCleared    = "stats_cleared"    // «Очистить статистику» (FR-38, RM5-8)
	evJournalCleared  = "journal_cleared"  // «Очистить журнал» (FR-38, RM5-9)
)

// Время и копилка.
const (
	evExtraUndone         = "extra_undone"          // изменение доп. времени отменено
	evBonusGrant          = "bonus_grant"           // родитель подарил минуты в копилку
	evBonusUndo           = "bonus_undo"            // подарок в копилку отменён
	evBonusClear          = "bonus_clear"           // копилка обнулена
	evBonusReduce         = "bonus_reduce"          // копилка уменьшена
	evPromoCreate         = "promo_create"          // родитель создал промокод (R26)
	evPromoGenerate       = "promo_generate"        // «Придумать коды» (FR-35, T3b)
	evPromoHistoryCleared = "promo_history_cleared" // «Очистить историю» промокодов (FR-35, T3b)
)

// Программы (R22). Нарушение из них только program_forbidden_try (R13).
const (
	evProgramDecided         = "program_decided"
	evProgramUndone          = "program_undone"
	evProgramHidden          = "program_hidden"  // вы скрыли программу (D-57)
	evProgramShown           = "program_shown"   // вы снова показали программу (D-57)
	evProgramRenamed         = "program_renamed" // вы переименовали программу (D-59)
	evProgramPending         = "program_pending"
	evProgramForbiddenTry    = "program_forbidden_try"
	evProgramAutoClosed      = "program_auto_closed" // запрещённая запустилась сама и закрыта (D-65): не нарушение
	evProgramClosedGames     = "program_closed_games"
	evProgramClosedStudyOnly = "program_closed_study_only"
)

// Сайты (S-9, S-10). Нарушение — только site_forbidden_try: вкладка на переднем плане у ребёнка,
// «запустилась сама» (R13) к ней не относится.
const (
	evSiteForbiddenTry    = "site_forbidden_try"     // запрещённый или «18+»: подробности domain, kind
	evSiteClosedVideo     = "site_closed_video"      // время на видео и соцсети закончилось (R6)
	evSiteClosedStudyOnly = "site_closed_study_only" // «Оставить только учёбу» (R8)
	evSiteUnread          = "site_unread"            // адрес вкладки не читается (RM4-1, RK5)
	evSiteAdded           = "site_added"             // родитель добавил домен: domain, kind
	evSiteMoved           = "site_moved"             // родитель перенёс домен в другой список: domain, kind
	evSiteRemoved         = "site_removed"           // родитель удалил домен: domain, kind (прежний)
	evSiteAdultFilter     = "site_adult_filter"      // «Блокировать сайты 18+» ребёнку: on
	evBraveRestored       = "brave_restored"         // сверка нашла ветку правил Brave изменённой и вернула её (T5-7)
)

// Работа программы.
const (
	// evInternalError — сбой внутри программы: паника при обработке одного ребёнка или одного дела
	// обслуживания. Текст для родителя без техники (NFR-11).
	evInternalError = "internal_error"
	evDBRecovered   = "db_recovered" // база не прочиталась, начата заново, прежняя сохранена
)

// allEventKinds — все виды событий, которые пишут служба и ядро. Новый вид — константа выше (или
// core.Ev* в ядре), строка здесь и строка в journalGroups.
var allEventKinds = []string{
	// Пароль и настройки.
	evSetupDone, evPasswordFailed, evPasswordChanged, evPasswordReset, evUninstallForgot,
	evSettingsChanged, evChildrenChanged, evChildAdmin, evChildAdminOff, evStatsCleared, evJournalCleared,
	core.EvBlockCleared,
	// Время и копилка.
	core.EvExtraTime, evExtraUndone, core.EvPause, core.EvPauseEnd, core.EvNoForbidden, core.EvDayClosed,
	core.EvBonusSpent, core.EvBonusGranted, evBonusGrant, evBonusUndo, evBonusClear, evBonusReduce,
	core.EvPromoRedeem, evPromoCreate, evPromoGenerate, evPromoHistoryCleared,
	core.EvRestBreak, core.EvRestEnded,
	// Нарушения.
	core.EvBypass, evProgramForbiddenTry, evSiteForbiddenTry,
	// Программы и сайты.
	evProgramDecided, evProgramUndone, evProgramHidden, evProgramShown, evProgramRenamed, evProgramPending,
	evProgramAutoClosed, evProgramClosedGames, evProgramClosedStudyOnly,
	evSiteClosedVideo, evSiteClosedStudyOnly, evSiteUnread, evSiteAdded, evSiteMoved, evSiteRemoved,
	evSiteAdultFilter, evBraveRestored,
	// Работа программы.
	core.EvSafeMode, evInternalError, evDBRecovered,
}
