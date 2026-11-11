package service

import (
	"pcblock/internal/core"
	"pcblock/internal/ipc"
)

// journalGroups — фильтр «Что» журнала (FR-24, 8.3.7 п.2, RM5-5): группа → виды событий. Каждый
// вид из allEventKinds — ровно в одной группе; «Все события» (ipc.JournalGroupAll) — без отбора
// по виду, туда же попадают виды старых версий, которых нет в перечне.
var journalGroups = map[string][]string{
	// Тот же набор, что VIOLATION_KINDS окна (frontend/src/lib/journal.ts).
	ipc.JournalGroupViolations: {core.EvBypass, evProgramForbiddenTry, evSiteForbiddenTry},
	ipc.JournalGroupTime: {
		core.EvExtraTime, evExtraUndone, core.EvPause, core.EvPauseEnd, core.EvNoForbidden, core.EvDayClosed,
		core.EvBonusSpent, core.EvBonusGranted, evBonusGrant, evBonusUndo, evBonusClear, evBonusReduce,
		core.EvPromoRedeem, evPromoCreate, evPromoGenerate, evPromoHistoryCleared,
		core.EvRestBreak, core.EvRestEnded,
	},
	ipc.JournalGroupPrograms: {
		evProgramDecided, evProgramUndone, evProgramHidden, evProgramShown, evProgramRenamed, evProgramPending,
		evProgramAutoClosed, evProgramClosedGames, evProgramClosedStudyOnly,
		evSiteClosedVideo, evSiteClosedStudyOnly, evSiteUnread, evSiteAdded, evSiteMoved, evSiteRemoved,
		evSiteAdultFilter, evBraveRestored,
	},
	// Отметка «детская» снята или учётка стала администратором (R1) — здесь, не нарушение.
	ipc.JournalGroupSettings: {
		evSetupDone, evPasswordFailed, evPasswordChanged, evPasswordReset, evUninstallForgot,
		evSettingsChanged, evChildrenChanged, evChildAdmin, evChildAdminOff, evStatsCleared, evJournalCleared,
		core.EvBlockCleared,
	},
	ipc.JournalGroupSystem: {core.EvSafeMode, evInternalError, evDBRecovered},
}
