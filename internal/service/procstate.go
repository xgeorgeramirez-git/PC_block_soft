package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// procStateFile — память службы о запущенных ею страже, значках и окнах. После падения службы
// (S27, taskkill /F) процессы остаются в сеансах; новая служба по этому файлу находит и
// завершает их (Ruling 15: в сеансе ровно один страж и один значок), а PID без момента
// создания процесса мог бы совпасть с чужим процессом.
const procStateFile = "procs.json"

// procRecord — один запущенный процесс.
type procRecord struct {
	Role    string `json:"role"` // guard, tray, parent, mystats, firstrun
	Session uint32 `json:"session"`
	PID     uint32 `json:"pid"`
	Stamp   uint64 `json:"stamp"` // SessionLauncher.Stamp в момент запуска
}

// readProcRecords читает файл; нет файла или он испорчен: пустой список (тогда оставшиеся
// процессы не найдутся, но служба стартует: R18).
func readProcRecords(path string) []procRecord {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var recs []procRecord
	if json.Unmarshal(data, &recs) != nil {
		return nil
	}
	return recs
}

// writeProcRecords записывает список целиком (через временный файл, чтобы обрыв записи не
// оставил половину JSON); пустой список удаляет файл.
func writeProcRecords(path string, recs []procRecord) error {
	if len(recs) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.Marshal(recs)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
