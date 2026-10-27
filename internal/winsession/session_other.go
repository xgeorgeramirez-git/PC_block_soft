//go:build !windows

package winsession

import "errors"

// Session описывает интерактивный сеанс Windows (номер, пользователь, SID, активность).
type Session struct {
	ID     uint32
	User   string
	SID    string
	Active bool
	State  uint32 // состояние WTS: 0 активен, 4 отключён
}

var errOnlyWindows = errors.New("winsession: доступно только в Windows")

// ActiveSessions возвращает активные сеансы. Вне Windows возвращает ошибку.
func ActiveSessions() ([]Session, error) { return nil, errOnlyWindows }

// LaunchAsSystem запускает процесс от имени системы в сеансе. Вне Windows возвращает ошибку.
func LaunchAsSystem(sessionID uint32, exe string, args []string) (uint32, error) {
	return 0, errOnlyWindows
}

// LaunchAsUser запускает процесс от имени пользователя сеанса. Вне Windows возвращает ошибку.
func LaunchAsUser(sessionID uint32, exe string, args []string) (uint32, error) {
	return 0, errOnlyWindows
}
