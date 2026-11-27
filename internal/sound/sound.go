// Package sound — звук предупреждения заказчика (D-55, FR-9) и звуки сообщения от родителя (FR-41,
// D-101). Файлы заказчика assets/sounds/warning.mp3 и assets/sounds/message/N.mp3 один раз
// переведены в wav скриптами tools/sound-wav.sh и tools/sound-message-wav.sh и встроены в
// программу: Windows играет их из памяти (PlaySoundW, SND_MEMORY), без временных файлов и без
// MCI, которому нужен файл на диске и DirectShow (страж работает от имени системы).
package sound

import (
	"embed"
	"encoding/binary"
	"errors"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Звуки схемы Windows для плашек «Игрового лобби» (ТЗ 8.2.4, 8.2.5, D-111): имена событий раздела
// «Звуки» панели управления. Какой файл играет и играет ли вообще, решает схема пользователя.
const (
	AliasExclamation = "SystemExclamation"    // «Восклицание»: программа закрыта, сайт закрыт, время вышло
	AliasNotify      = "Notification.Default" // «Уведомление»: родитель разрешил программу
)

//go:embed warning.wav
var warningWAV []byte // живёт всё время работы программы: PlaySoundW с SND_ASYNC читает его после возврата

// WarningWAV возвращает встроенный звук предупреждения (не менять: буфер общий).
func WarningWAV() []byte { return warningWAV }

// Звуки сообщения: набор — файлы message/N.wav (номер 1…99), не список в коде: новый N.mp3 и
// пересборка скриптом дают новый звук без правки кода (D-101).
//
//go:embed message/*.wav
var messageFS embed.FS

var (
	messageNums = scanMessages() // номера встроенных звуков по возрастанию

	messageMu  sync.Mutex
	messageWAV = map[int][]byte{} // разобранные из встроенных файлов; живут всё время работы (SND_ASYNC)
)

// scanMessages читает имена встроенных wav: «N.wav», N — 1…99 без ведущего нуля.
func scanMessages() []int {
	ents, err := fs.ReadDir(messageFS, "message")
	if err != nil {
		return []int{}
	}
	out := []int{}
	for _, e := range ents {
		name, ok := strings.CutSuffix(e.Name(), ".wav")
		n, err := strconv.Atoi(name)
		if !ok || err != nil || n < 1 || n > 99 || strconv.Itoa(n) != name {
			continue
		}
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// MessageSounds — номера встроенных звуков сообщения по возрастанию (копия: можно менять).
func MessageSounds() []int { return slices.Clone(messageNums) }

// HasMessage: звук сообщения с номером n встроен. 0 («без звука») и чужие номера — false.
func HasMessage(n int) bool {
	_, ok := slices.BinarySearch(messageNums, n)
	return ok
}

// MessageWAV возвращает встроенный звук сообщения n; nil — такого нет. Буфер читается из
// встроенных файлов при первом запросе и дальше общий (не менять): в памяти только звуки, которые
// уже играли.
func MessageWAV(n int) []byte {
	if !HasMessage(n) {
		return nil
	}
	messageMu.Lock()
	defer messageMu.Unlock()
	if b, ok := messageWAV[n]; ok {
		return b
	}
	b, err := messageFS.ReadFile("message/" + strconv.Itoa(n) + ".wav")
	if err != nil || len(b) == 0 {
		return nil
	}
	messageWAV[n] = b
	return b
}

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

// Length - сколько звучит wav b по его шапке; испорченный или пустой - 0.
func Length(b []byte) time.Duration {
	f, err := ParseWAV(b)
	if err != nil {
		return 0
	}
	return time.Duration(f.Seconds() * float64(time.Second))
}

// WarningLength - сколько звучит звук предупреждения: страж не начинает следующий звук раньше
// (PlaySoundW с SND_ASYNC обрывает текущий).
func WarningLength() time.Duration { return Length(warningWAV) }

// MessageLength - сколько звучит встроенный звук сообщения n; такого нет - 0.
func MessageLength(n int) time.Duration { return Length(MessageWAV(n)) }

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
