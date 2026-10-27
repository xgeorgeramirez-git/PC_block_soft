package winhost

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pcblock/internal/service"
	"pcblock/internal/winsession"
)

// Ловит: коды уведомлений диспетчера служб (WTS_SESSION_*) переведены в не те события службы:
// тогда вход ребёнка не запустил бы стража, а выход оставил бы его процессы.
func TestSessionEventOf(t *testing.T) {
	cases := map[uint32]service.SessionEvent{
		1: service.SessionConnect, 3: service.SessionConnect, 0xa: service.SessionConnect,
		2: service.SessionDisconnect, 4: service.SessionDisconnect,
		5: service.SessionLogon,
		6: service.SessionLogoff, 0xb: service.SessionLogoff,
		7: service.SessionLock,
		8: service.SessionUnlock,
	}
	for code, want := range cases {
		got, ok := sessionEventOf(code)
		if !ok || got != want {
			t.Errorf("код %d: %v %v, ждали %v", code, got, ok, want)
		}
	}
	for _, code := range []uint32{0, 9, 0xc, 0xffff} {
		if _, ok := sessionEventOf(code); ok {
			t.Errorf("код %d не должен давать событие", code)
		}
	}
}

// Ловит: отключённый сеанс (смена пользователя) пропадает из списка службы или выглядит
// подключённым: тогда служба не знает, что ребёнок не за компьютером (FB-3, D-52). Сеансы без
// пользователя (служебный, экран входа) в список не попадают.
func TestSessionInfosKeepDisconnectedUsers(t *testing.T) {
	list := []winsession.Session{
		{ID: 0, SID: "", Active: false},                            // служебный сеанс
		{ID: 1, User: `PC\Анна`, SID: "S-1-5-21-1", Active: true},  // на экране
		{ID: 3, User: `PC\Миша`, SID: "S-1-5-21-2", Active: false}, // отключён: смена пользователя
		{ID: 4, SID: "", Active: true},                             // экран входа без пользователя
	}
	want := []service.SessionInfo{
		{ID: 1, User: `PC\Анна`, SID: "S-1-5-21-1", Screen: service.ScreenOn},
		{ID: 3, User: `PC\Миша`, SID: "S-1-5-21-2", Screen: service.ScreenOff},
	}
	got := sessionInfos(list)
	if len(got) != len(want) {
		t.Fatalf("сеансов %d: %+v; want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("сеанс %d: %+v; want %+v", i, got[i], want[i])
		}
	}
	if got := sessionInfos(nil); got == nil || len(got) != 0 {
		t.Errorf("пустой список: %#v; want пустой не nil", got)
	}
}

// Ловит: перезапуск службы при сбое не 5, 10 и 30 с (ТЗ 3.2, Ruling 27) или сброс счётчика
// сбоев слишком короткий: после третьего сбоя служба перестала бы подниматься быстро.
func TestRecoveryDelays(t *testing.T) {
	want := []time.Duration{5 * time.Second, 10 * time.Second, 30 * time.Second}
	if len(recoveryDelays) != len(want) {
		t.Fatalf("задержек %d, ждали %d", len(recoveryDelays), len(want))
	}
	for i, d := range want {
		if recoveryDelays[i] != d {
			t.Errorf("задержка %d: %v, ждали %v", i, recoveryDelays[i], d)
		}
	}
	if recoveryResetS < 3600 {
		t.Errorf("счётчик сбоев обнуляется через %d с, ждали не меньше часа", recoveryResetS)
	}
}

// Ловит: дескриптор папки данных открывает её не только SYSTEM и Администраторам (NFR-9) или
// не защищён от наследования прав родителя: тогда база и журнал были бы доступны ребёнку.
func TestDataDirSDDL(t *testing.T) {
	if !strings.HasPrefix(dataDirSDDL, "D:P") {
		t.Errorf("список прав не защищён от наследования: %q", dataDirSDDL)
	}
	if strings.Count(dataDirSDDL, "(A;") != 2 || !strings.Contains(dataDirSDDL, ";SY)") || !strings.Contains(dataDirSDDL, ";BA)") {
		t.Errorf("права должны быть ровно у SY и BA: %q", dataDirSDDL)
	}
	for _, bad := range []string{";WD)", ";AU)", ";BU)", ";IU)", ";S-1-1-0)"} {
		if strings.Contains(dataDirSDDL, bad) {
			t.Errorf("в правах есть лишний субъект %s: %q", bad, dataDirSDDL)
		}
	}
}

// Ловит: результат служебного ключа записан с экранированием кириллицы и угловых скобок
// (проверяющий скрипт читает русский текст ошибки), не создаёт папку или портит пустой путь.
func TestWriteJSON(t *testing.T) {
	if err := writeJSON("", map[string]string{"a": "b"}); err != nil {
		t.Fatalf("пустой путь: %v", err)
	}
	path := filepath.Join(t.TempDir(), "вложенная", "out.json")
	if err := writeJSON(path, result{OK: false, Error: "Служба <не> отвечает & ждёт"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Служба <не> отвечает & ждёт") {
		t.Fatalf("текст экранирован: %s", raw)
	}
	var back result
	if err := json.Unmarshal(raw, &back); err != nil || back.OK || back.Error == "" {
		t.Fatalf("файл не читается обратно: %v %+v", err, back)
	}
}

// Ловит: итог служебного ключа теряет ошибку действия (скрипт приёмки решил бы, что всё
// удалось) или ошибка записи файла прячет успех действия, либо успех записывается без ok.
func TestFinish(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "r.json")
	if err := finish(out, map[string]any{"k": 1}, nil); err != nil {
		t.Fatal(err)
	}
	var okRes result
	raw, _ := os.ReadFile(out)
	if err := json.Unmarshal(raw, &okRes); err != nil || !okRes.OK || okRes.Info["k"] == nil {
		t.Fatalf("успех: %s", raw)
	}

	boom := errors.New("не вышло")
	if err := finish(out, nil, boom); !errors.Is(err, boom) {
		t.Fatalf("ошибка действия потеряна: %v", err)
	}
	var badRes result
	raw, _ = os.ReadFile(out)
	if err := json.Unmarshal(raw, &badRes); err != nil || badRes.OK || badRes.Error != "не вышло" {
		t.Fatalf("отказ: %s", raw)
	}

	// Путь, который нельзя создать: файл вместо папки.
	file := filepath.Join(dir, "файл")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := finish(filepath.Join(file, "r.json"), nil, nil); err == nil {
		t.Error("ошибка записи результата не вернулась при успехе действия")
	}
	if err := finish(filepath.Join(file, "r.json"), nil, boom); !errors.Is(err, boom) {
		t.Errorf("при отказе действия вернулась не его ошибка: %v", err)
	}
}

// Ловит: журнал запуска не создаётся (причина отказа службы запуститься осталась бы
// неизвестной) или перетирает прежние строки.
func TestAppendBootLog(t *testing.T) {
	dir := t.TempDir()
	appendBootLog(dir, "первая")
	appendBootLog(dir, "вторая")
	raw, err := os.ReadFile(filepath.Join(logDir(dir), bootLogName))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "первая") || !strings.HasSuffix(lines[1], "вторая") {
		t.Fatalf("журнал запуска: %q", raw)
	}
}

// Ловит: пути базы и журнала лежат не в папке данных (тестовый режим подменил бы только часть).
func TestPathsInsideDataDir(t *testing.T) {
	d := filepath.Join("x", "data")
	if got := dbPath(d); filepath.Dir(got) != d || filepath.Base(got) != "pcblock.db" {
		t.Errorf("база: %s", got)
	}
	if got := logDir(d); filepath.Dir(got) != d || filepath.Base(got) != "logs" {
		t.Errorf("журнал: %s", got)
	}
}

// Ловит: журнал работы лежит под другим именем, чем logs\service.log (проверяющий скрипт и
// родитель ищут его там), или хранит не пять файлов по десять мегабайт.
func TestServiceLogFileName(t *testing.T) {
	dir := t.TempDir()
	rl, err := service.OpenRotatingLog(logDir(dir), logBaseName, logMaxBytes, logFiles)
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Close()
	if _, err := rl.Write([]byte("строка\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "logs", "service.log")); err != nil {
		t.Fatalf("журнала работы нет по пути logs\\service.log: %v", err)
	}
	if logMaxBytes != 10<<20 || logFiles != 5 {
		t.Errorf("журнал по кругу: %d байт x %d файлов, ждали 10 МБ x 5", logMaxBytes, logFiles)
	}
}

// Ловит: номер сеанса из уведомления диспетчера служб (WTSSESSION_NOTIFICATION) читается не
// из того места или без проверки размера: в боевой службе тогда номер всегда 0, и события
// конца сеанса, блокировки и отключения не доходят до нужного сеанса (FB-1, FB-3).
func TestSessionIDOfNotification(t *testing.T) {
	cases := []struct {
		name   string
		raw    []byte
		wantID uint32
		wantOK bool
	}{
		{"cbSize 8, сеанс 3", []byte{8, 0, 0, 0, 3, 0, 0, 0}, 3, true},
		{"сеанс 258 (два байта)", []byte{8, 0, 0, 0, 2, 1, 0, 0}, 258, true},
		{"cbSize больше 8, лишний хвост", []byte{12, 0, 0, 0, 5, 0, 0, 0, 9, 9, 9, 9}, 5, true},
		{"пустые данные", nil, 0, false},
		{"короче структуры", []byte{8, 0, 0, 0, 3, 0, 0}, 0, false},
		{"cbSize меньше структуры", []byte{4, 0, 0, 0, 3, 0, 0, 0}, 0, false},
		{"cbSize больше данных", []byte{16, 0, 0, 0, 3, 0, 0, 0}, 0, false},
	}
	for _, c := range cases {
		id, ok := sessionIDOf(c.raw)
		if id != c.wantID || ok != c.wantOK {
			t.Errorf("%s: %d %v; want %d %v", c.name, id, ok, c.wantID, c.wantOK)
		}
	}
}
