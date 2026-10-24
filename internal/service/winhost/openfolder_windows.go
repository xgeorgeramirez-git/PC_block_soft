//go:build windows

package winhost

import (
	"os"
	"path/filepath"

	"pcblock/internal/winsession"
)

// openFolder открывает папку в проводнике в сеансе sessionID от имени пользователя этого
// сеанса (FR-25, D-48, «Открыть папку с логами»). Служба работает от имени системы в
// служебном сеансе: свой проводник она показала бы никому, поэтому запуск идёт тем же
// способом, что у окна и значка (winsession.LaunchAsUser), на рабочем столе родителя.
// Если у учётки нет доступа к папке (данные закрыты для всех, кроме Администраторов),
// проводник сам предложит подтвердить доступ.
func openFolder(sessionID uint32, path string) error {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	_, err := winsession.LaunchAsUser(sessionID, filepath.Join(root, "explorer.exe"), []string{path})
	return err
}
