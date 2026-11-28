package sound

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
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

// mp3Numbers — номера файлов assets/sounds/message/N.mp3 по возрастанию (набор звуков заказчика).
func mp3Numbers(t *testing.T) []int {
	t.Helper()
	files, err := filepath.Glob("../../assets/sounds/message/*.mp3")
	if err != nil {
		t.Fatal(err)
	}
	var out []int
	for _, f := range files {
		n, err := strconv.Atoi(strings.TrimSuffix(filepath.Base(f), ".mp3"))
		if err != nil {
			t.Fatalf("имя %s не номер", f)
		}
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// Ловит: встроенный набор звуков сообщения разошёлся с файлами заказчика (добавили N.mp3 и не
// пересобрали, или wav лишний), wav не PCM 16 бит 44,1 кГц стерео, длина wav не равна длине mp3
// (обрезка запрещена, D-101), mp3 заменили без пересборки. Длины — из durations.json, который
// пишет tools/sound-message-wav.sh по ffprobe (FB28-4).
func TestMessageWavs(t *testing.T) {
	want := mp3Numbers(t)
	if len(want) == 0 {
		t.Fatal("нет звуков заказчика в assets/sounds/message")
	}
	if got := MessageSounds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("встроенные звуки %v, файлы заказчика %v: запустите bash tools/sound-message-wav.sh", got, want)
	}
	raw, err := os.ReadFile("message/durations.json")
	if err != nil {
		t.Fatalf("нет message/durations.json: %v", err)
	}
	var durs map[string]float64
	if err := json.Unmarshal(raw, &durs); err != nil {
		t.Fatalf("durations.json: %v", err)
	}
	if len(durs) != len(want) {
		t.Fatalf("в durations.json %d звуков, файлов %d", len(durs), len(want))
	}
	for _, n := range want {
		name := strconv.Itoa(n)
		if !HasMessage(n) {
			t.Errorf("звук %d: HasMessage = false", n)
		}
		f, err := ParseWAV(MessageWAV(n))
		if err != nil {
			t.Errorf("звук %d: wav не разобран: %v", n, err)
			continue
		}
		if f.Format != 1 || f.Channels != 2 || f.SampleRate != 44100 || f.Bits != 16 {
			t.Errorf("звук %d: формат %+v, ждали PCM, 2 канала, 44100 Гц, 16 бит", n, f)
		}
		d, ok := durs[name]
		if !ok || d <= 0 {
			t.Errorf("звук %d: нет длины в durations.json", n)
		} else if got := f.Seconds(); math.Abs(got-d) > 0.1 {
			t.Errorf("звук %d: длина wav %.3f с, mp3 %.3f с (допуск 0,1 с)", n, got, d)
		}
		mp3, err := os.ReadFile("../../assets/sounds/message/" + name + ".mp3")
		if err != nil {
			t.Fatal(err)
		}
		mark, err := os.ReadFile("message/" + name + ".mp3.sha256")
		if err != nil {
			t.Errorf("звук %d: нет отметки sha256: %v", n, err)
			continue
		}
		sum := sha256.Sum256(mp3)
		if got, w := hex.EncodeToString(sum[:]), strings.TrimSpace(string(mark)); got != w {
			t.Errorf("assets/sounds/message/%s.mp3 изменился (sha256 %s, отметка %s): запустите bash tools/sound-message-wav.sh", name, got, w)
		}
	}
}

// Ловит: номер вне набора считается встроенным (страж не подменит его на звук 1) или отдаёт чужой wav.
func TestMessageUnknown(t *testing.T) {
	for _, n := range []int{0, -1, 99, 100} {
		if HasMessage(n) || MessageWAV(n) != nil {
			t.Errorf("звук %d считается встроенным", n)
		}
	}
	if PlayMessage(99) {
		t.Error("звук 99 принят")
	}
	if a, b := MessageWAV(1), MessageWAV(2); len(a) == 0 || len(b) == 0 || bytes.Equal(a, b) {
		t.Error("звуки 1 и 2 пусты или одинаковы")
	}
}

// Ловит: на другой ОС звук сообщения паникует или сообщает, что принят.
func TestPlayMessageOtherOS(t *testing.T) {
	if isWindows {
		t.Skip("на Windows звук играет PlaySoundW")
	}
	if PlayMessage(1) {
		t.Fatal("на Linux звук не может быть принят")
	}
}

// Ловит (ревью M2 0.8.3): страж не знает, сколько звучит звук, и следующий PlaySoundW обрывает
// текущий: длина не из шапки wav (у сообщения 2 - 7,76 с по durations.json), у чужого номера не 0.
func TestSoundLengths(t *testing.T) {
	near := func(got time.Duration, sec float64) bool {
		return math.Abs(got.Seconds()-sec) < 0.01
	}
	if got := WarningLength(); got < 2300*time.Millisecond || got > 2500*time.Millisecond {
		t.Errorf("звук предупреждения %v, ждали около 2,4 с (D-55)", got)
	}
	if got := MessageLength(2); !near(got, 7.758367) {
		t.Errorf("звук сообщения 2: %v, ждали 7,758 с", got)
	}
	if got := MessageLength(1); !near(got, 2.429388) {
		t.Errorf("звук сообщения 1: %v, ждали 2,429 с", got)
	}
	for _, n := range []int{0, -1, 99, 100} {
		if got := MessageLength(n); got != 0 {
			t.Errorf("звук %d (нет такого): длина %v, ждали 0", n, got)
		}
	}
}

// Ловит (R-7, D-111): имена звуков схемы Windows для плашек «Игрового лобби» не те, что в
// разделе «Звуки» панели управления («Восклицание», «Уведомление»).
func TestSystemAliases(t *testing.T) {
	if AliasExclamation != "SystemExclamation" || AliasNotify != "Notification.Default" {
		t.Fatalf("%q %q", AliasExclamation, AliasNotify)
	}
	if !isWindows && PlaySystem(AliasExclamation) {
		t.Fatal("вне Windows системный звук принят")
	}
}
