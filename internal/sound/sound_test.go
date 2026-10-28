package sound

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// Ловит: встроенный звук не RIFF/WAVE PCM 16 бит 48 кГц стерео (PlaySoundW из памяти его не
// сыграет) или его длительность не та, что у файла заказчика (2,4 с, D-55).
func TestWarningWavFormat(t *testing.T) {
	f, err := ParseWAV(WarningWAV())
	if err != nil {
		t.Fatalf("встроенный wav не разобран: %v", err)
	}
	if f.Format != 1 || f.Channels != 2 || f.SampleRate != 48000 || f.Bits != 16 {
		t.Fatalf("формат %+v, ждали PCM, 2 канала, 48000 Гц, 16 бит", f)
	}
	if d := f.Seconds(); d < 2.3 || d > 2.5 {
		t.Fatalf("длительность %.3f с, ждали 2,3…2,5 с", d)
	}
}

// Ловит: mp3 заказчика заменили, а встроенный wav не пересобрали (класс E: ресурс разошёлся с
// исходником).
func TestWarningWavMatchesMP3(t *testing.T) {
	mp3, err := os.ReadFile("../../assets/sounds/warning.mp3")
	if err != nil {
		t.Fatalf("нет звука заказчика: %v", err)
	}
	mark, err := os.ReadFile("warning.mp3.sha256")
	if err != nil {
		t.Fatalf("нет отметки warning.mp3.sha256: %v", err)
	}
	sum := sha256.Sum256(mp3)
	if got, want := hex.EncodeToString(sum[:]), strings.TrimSpace(string(mark)); got != want {
		t.Fatalf("assets/sounds/warning.mp3 изменился (sha256 %s, отметка %s): пересоберите wav командой bash tools/sound-wav.sh", got, want)
	}
}

// Ловит: разбор пропускает испорченный файл (без RIFF, без fmt или data, обрезанный кусок).
func TestParseWAVRejectsBroken(t *testing.T) {
	good := WarningWAV()
	cases := map[string][]byte{
		"пусто":        nil,
		"не RIFF":      append([]byte("RIFX"), good[4:]...),
		"не WAVE":      append(append(append([]byte{}, good[:8]...), []byte("AVI ")...), good[12:]...),
		"только шапка": good[:12],
		"обрезан fmt":  good[:24],
		"без data":     good[:36],
	}
	for name, data := range cases {
		if _, err := ParseWAV(data); err == nil {
			t.Errorf("%s: ошибки нет", name)
		}
	}
	// data длиннее файла: тоже ошибка.
	bad := append([]byte{}, good[:44]...)
	binary.LittleEndian.PutUint32(bad[40:], 1000)
	if _, err := ParseWAV(bad); err == nil {
		t.Error("data длиннее файла: ошибки нет")
	}
}

// Ловит: на другой ОС вызов звука паникует или сообщает, что звук принят.
func TestPlayWarningOtherOS(t *testing.T) {
	if isWindows {
		t.Skip("на Windows звук играет PlaySoundW")
	}
	if PlayWarning() {
		t.Fatal("на Linux звук не может быть принят")
	}
}
