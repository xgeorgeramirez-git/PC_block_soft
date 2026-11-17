package service

import (
	"fmt"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/programs"
)

// Системные инструменты в сеансе ребёнка (веха M6, R17г, FR-29, D-86): командную строку,
// PowerShell, Терминал, редактор реестра, консоль управления, taskkill и sc, которые открыл сам
// ребёнок, наблюдатель закрывает; ребёнку — плашка «Программа «X» недоступна», родителю — запись
// обхода. Инструмент, который запустила другая программа (игра, установщик, обновление), не
// трогается и программой не считается (не ждёт решения родителя). Распознавание по пути —
// programs.Roots.SystemTool.

// plateSysTool — вид закрытия «системный инструмент» (watchAct.plateKind).
const plateSysTool = "systool"

// toolTestDir — папка, где в тестовом режиме лежат копии инструментов (RM3-8); «» — не тестовый.
func (s *Service) toolTestDir() string {
	if s.cfg.Test.Enabled {
		return s.cfg.Test.WatchDir
	}
	return ""
}

// sysToolManual — инструмент e открыл ребёнок: запуск ручной (launchKind: свежий ввод и родитель
// Проводник не в первые 60 с после входа, программа на переднем плане или брокер Магазина), и
// родитель — Проводник или другой системный инструмент (по настоящим путям). Родитель — игра или
// установщик на переднем плане: запуск не ребёнка (D-86). Терминал Windows 11 открывает сама для
// любой консоли (без родителя в сеансе): ветка брокера Магазина для него не действует, его
// оболочки (cmd, PowerShell) закрываются своим правилом.
func sysToolManual(e *watchEntry, r programs.Roots) bool {
	f := e.facts
	if f.Parent == "" {
		return false
	}
	if programs.IsTerminal(f.Parent) {
		// Оболочка в Терминале (Win+X, «Пуск»): Терминал без родителя в сеансе и в первую секунду ещё
		// не на переднем плане по снимку стража — решает свежий ввод ребёнка.
		return !f.InputAt.IsZero() && f.At.Sub(f.InputAt) <= manualInputWindow
	}
	if !e.manual {
		return false
	}
	if f.Shell != "" && f.Parent == f.Shell {
		return true
	}
	_, ok := r.SystemTool(f.Parent, "")
	return ok
}

// recordToolClosed — плашка ребёнку и запись обхода о закрытом инструменте (под s.mu). Дочерний
// процесс того же exe (quiet) — без записей: они у главного.
func (s *Service) recordToolClosed(cs *childState, a *watchAct, real time.Time) {
	if a.e.quiet {
		return
	}
	s.blockedPlate(a.t.session, cs.child.ID, ipc.BlockedPlate{
		Kind: ipc.BlockedForbidden, ShowS: blockedPlateShowS, Path: a.e.key,
		Text: fmt.Sprintf(textPlateSystemTool, a.tool),
	}, real)
	if ss := s.sessions[a.t.session]; ss != nil {
		s.bypassIn(ss, fmt.Sprintf(detailSystemTool, a.tool), "system_tool", real)
	}
}
