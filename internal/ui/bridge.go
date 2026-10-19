//go:build windows

package ui

import (
	"time"

	"pcblock/internal/ipc"
)

// dialTimeout — сколько окно ждёт канал службы. Дольше не стоит: человек нажал кнопку и ждёт,
// а службы может не быть (остановлена, удаляется).
const dialTimeout = 3 * time.Second

// dialService открывает канал связи со службой. До отправки данных клиент проверяет, что канал
// держит система, а не чужой процесс (Ruling 17), поэтому пароль родителя чужому не уйдёт.
func dialService() (ipc.Caller, error) {
	c, err := ipc.Dial(ipc.PipeName, dialTimeout)
	if err != nil {
		return nil, err
	}
	return c, nil
}
