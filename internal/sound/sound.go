// Package sound — звук предупреждения заказчика (D-55, FR-9). Файл заказчика
// assets/sounds/warning.mp3 один раз переведён в wav скриптом tools/sound-wav.sh и встроен в
// программу: Windows играет его из памяти (PlaySoundW, SND_MEMORY), без временных файлов и без
// MCI, которому нужен файл на диске и DirectShow (страж работает от имени системы).
package sound

import (
	_ "embed"
	"encoding/binary"
	"errors"
)

//go:embed warning.wav
var warningWAV []byte // живёт всё время работы программы: PlaySoundW с SND_ASYNC читает его после возврата

// WarningWAV возвращает встроенный звук предупреждения (не менять: буфер общий).
func WarningWAV() []byte { return warningWAV }

// WAVFormat — то, что нужно знать о wav для проверки.
type WAVFormat struct {
	Format     uint16 // 1 = PCM
	Channels   uint16
	SampleRate uint32
	Bits       uint16
	DataBytes  uint32
	ByteRate   uint32
}

// Seconds — длительность звука в секундах.
func (f WAVFormat) Seconds() float64 {
	if f.ByteRate == 0 {
		return 0
	}
	return float64(f.DataBytes) / float64(f.ByteRate)
}

var errWAV = errors.New("файл не wav PCM")

// ParseWAV разбирает шапку RIFF/WAVE: блоки fmt и data. Испорченный файл даёт ошибку.
func ParseWAV(b []byte) (WAVFormat, error) {
	var f WAVFormat
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return f, errWAV
	}
	haveFmt := false
	for i := 12; i+8 <= len(b); {
		id := string(b[i : i+4])
		size := int(binary.LittleEndian.Uint32(b[i+4 : i+8]))
		body := i + 8
		if size < 0 || body+size > len(b) {
			return f, errWAV
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return f, errWAV
			}
			c := b[body:]
			f.Format = binary.LittleEndian.Uint16(c[0:2])
			f.Channels = binary.LittleEndian.Uint16(c[2:4])
			f.SampleRate = binary.LittleEndian.Uint32(c[4:8])
			f.ByteRate = binary.LittleEndian.Uint32(c[8:12])
			f.Bits = binary.LittleEndian.Uint16(c[14:16])
			haveFmt = true
		case "data":
			if !haveFmt {
				return f, errWAV
			}
			f.DataBytes = uint32(size)
			return f, nil
		}
		i = body + size + size&1
	}
	return f, errWAV
}
