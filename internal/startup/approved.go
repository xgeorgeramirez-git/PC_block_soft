package startup

import (
	"encoding/binary"
	"time"
)

// Значения StartupApproved (ключи ...\Explorer\StartupApproved\Run, Run32, StartupFolder):
// 12 байт, первый — состояние, с 5-го — FILETIME момента выключения (RM5b-2).

const approvedLen = 12

// DecodeApproved читает значение StartupApproved: первый байт 0x02 или 0x06 — включено,
// 0x03 или 0x07 — выключено. Не 12 байт или другой первый байт — ok = false (состояние
// неизвестно: решает отсутствие значения, то есть «включено» по умолчанию Windows).
func DecodeApproved(b []byte) (enabled, ok bool) {
	if len(b) != approvedLen {
		return false, false
	}
	switch b[0] {
	case 0x02, 0x06:
		return true, true
	case 0x03, 0x07:
		return false, true
	}
	return false, false
}

// filetimeEpoch — 1601-01-01 UTC в интервалах по 100 нс от 1970-01-01.
const filetimeEpoch = 116444736000000000

// EncodeApproved — значение StartupApproved, как пишет Диспетчер задач: выключено — 0x03, три
// нуля и FILETIME момента now (UTC); включено — 0x02 и нули.
func EncodeApproved(enabled bool, now time.Time) []byte {
	b := make([]byte, approvedLen)
	if enabled {
		b[0] = 0x02
		return b
	}
	b[0] = 0x03
	binary.LittleEndian.PutUint64(b[4:], uint64(now.UnixNano()/100+filetimeEpoch))
	return b
}
