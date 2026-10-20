package winhost

import (
	"fmt"
	"time"

	"pcblock/internal/launch"
)

// Hold держит процесс-заглушку стража или значка, пока его не завершит служба или система.
// Это временная замена guard.Run и systray.Run (задачи T10 и T11): служба запускает такие
// процессы в сеансах и следит за ними, ранний смоук проверяет, что они появились и пропали.
// Заглушка ничего не показывает, не ограничивает и не опрашивает службу.
func Hold(role string, opts launch.Options) error {
	if role == "" {
		return fmt.Errorf("не указана роль процесса")
	}
	for {
		time.Sleep(time.Hour)
	}
}
