package service

import "pcblock/internal/ipc"

// Обработчики команд вехи M4 «Сайты и Brave». Заглушки T3: контракт канала и права уже есть, чтобы
// страж, окна и приёмка не меняли его в обратную сторону; обработчики сайтов — T4,
// TestSeedPrograms — T6.

// textNotReadyM4 — ответ заглушки, пока обработчик не написан.
const textNotReadyM4 = "Пока не готово"

// hListSites — «Сайты»: списки и переключатели 18+ (FR-21). Заглушка T3; обработчик — T4.
func (s *Service) hListSites(*callCtx) ipc.Reply { return ipc.Fail(textNotReadyM4) }

// hAddSite — добавить или перенести домен. Заглушка T3; обработчик — T4.
func (s *Service) hAddSite(*callCtx) ipc.Reply { return ipc.Fail(textNotReadyM4) }

// hRemoveSite — удалить домен из списка. Заглушка T3; обработчик — T4.
func (s *Service) hRemoveSite(*callCtx) ipc.Reply { return ipc.Fail(textNotReadyM4) }

// hSetAdultFilter — «Блокировать сайты 18+» ребёнку. Заглушка T3; обработчик — T4.
func (s *Service) hSetAdultFilter(*callCtx) ipc.Reply { return ipc.Fail(textNotReadyM4) }

// hTestSeedPrograms — программы «до обновления» в тестовом режиме (RM4-11, S46). Заглушка T3;
// обработчик — T6.
func (s *Service) hTestSeedPrograms(*callCtx) ipc.Reply { return ipc.Fail(textNotReadyM4) }
