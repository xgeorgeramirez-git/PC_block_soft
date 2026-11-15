package guard

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// R26, D-83: наборы стикеров и надписи экрана блокировки — те же, что в окне ребёнка
// (frontend/src/lib/stickers.ts). Одна истина в двух языках: тест разбирает ts-файл.

// tsStringArray достаёт массив строк `export const NAME ... = [ ... ];` из исходника TypeScript.
func tsStringArray(t *testing.T, src, name string) []string {
	t.Helper()
	re := regexp.MustCompile(`(?s)export const ` + name + `\b[^=]*=\s*\[(.*?)\];`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("в stickers.ts нет массива %s", name)
	}
	out := []string{}
	for _, s := range regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(m[1], -1) {
		out = append(out, s[1])
	}
	return out
}

// Ловит: страж и окно показывают разные наборы или надписи (правка одного файла без другого).
func TestStickerTablesMatchFrontend(t *testing.T) {
	raw, err := os.ReadFile("../../frontend/src/lib/stickers.ts")
	if err != nil {
		t.Fatalf("stickers.ts: %v", err)
	}
	src := string(raw)
	for _, c := range []struct {
		name string
		got  []string
	}{
		{"SUCCESS", stickerSuccess},
		{"FAIL", stickerFail},
		{"SUCCESS_TEXTS", stickerSuccessTexts},
		{"FAIL_TEXTS", stickerFailTexts},
	} {
		want := tsStringArray(t, src, c.name)
		if strings.Join(c.got, "|") != strings.Join(want, "|") {
			t.Fatalf("%s: в Go %q, в stickers.ts %q", c.name, c.got, want)
		}
	}
}

// Ловит: таблица не совпадает с ТЗ R26 (число, номера); стикер без кадров на экране.
func TestStickerTablesLiteral(t *testing.T) {
	if strings.Join(stickerSuccess, ",") != "017,033,044,049,050,077,093,096,112" {
		t.Fatalf("удача: %v", stickerSuccess)
	}
	if strings.Join(stickerFail, ",") != "023,028,048,069,082,084,095,110" {
		t.Fatalf("неудача: %v", stickerFail)
	}
	if len(stickerSuccessTexts) != 6 || len(stickerFailTexts) != 6 {
		t.Fatalf("надписей %d и %d, ждали по 6", len(stickerSuccessTexts), len(stickerFailTexts))
	}
	counts, err := stickerFrameCounts()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range append(append([]string{}, stickerSuccess...), stickerFail...) {
		if counts[n] == 0 {
			t.Fatalf("у стикера %s нет кадров", n)
		}
	}
}

// Ловит: неверная форма слова «минута» в надписи удачи (Review Focus 5).
func TestStickerSuccessTextMinutes(t *testing.T) {
	for _, c := range []struct {
		i, n int
		want string
	}{
		{0, 30, "Ура! +30 минут"},
		{1, 1, "Есть! Тебе +1 минута"},
		{2, 3, "Код сработал! +3 минуты"},
		{3, 5, "Отлично, лови +5 минут!"},
		{4, 11, "Вот это да! +11 минут"},
		{5, 21, "Супер! Твои +21 минута"},
		{0, 112, "Ура! +112 минут"},
		{0, 24, "Ура! +24 минуты"},
	} {
		if got := stickerSuccessText(c.i, c.n); got != c.want {
			t.Fatalf("stickerSuccessText(%d, %d) = %q, want %q", c.i, c.n, got, c.want)
		}
	}
}

// seqRand — источник случайности тестов: отдаёт заданные числа по кругу (по модулю n).
func seqRand(vals ...int) func(int) int {
	i := 0
	return func(n int) int {
		v := vals[i%len(vals)]
		i++
		return v % n
	}
}

// Ловит: второй ответ того же вида повторяет стикер или надпись прошлого; верный код между
// двумя неверными сбивает неповтор (прошлое помнится отдельно для удачи и неудачи, R26).
func TestStickerPickerNoRepeat(t *testing.T) {
	p := newStickerPicker(seqRand(0)) // всегда первый из оставшихся
	a := p.fail()
	if a.OK || a.Sticker != "023" || a.Text != "Такого кода нет. Проверь буквы и цифры" {
		t.Fatalf("первая неудача %+v", a)
	}
	ok := p.success(30)
	if !ok.OK || ok.Sticker != "017" || ok.Text != "Ура! +30 минут" {
		t.Fatalf("удача %+v", ok)
	}
	b := p.fail()
	if b.Sticker == a.Sticker || b.Text == a.Text {
		t.Fatalf("вторая неудача повторила первую: %+v после %+v", b, a)
	}
	if b.Sticker != "028" || b.Text != "Хм, код не подходит. Посмотри внимательнее" {
		t.Fatalf("вторая неудача %+v", b)
	}
	ok2 := p.success(1)
	if ok2.Sticker == ok.Sticker || ok2.Text == "Ура! +1 минута" {
		t.Fatalf("вторая удача повторила первую: %+v", ok2)
	}
	if ok2.Sticker != "033" || ok2.Text != "Есть! Тебе +1 минута" {
		t.Fatalf("вторая удача %+v", ok2)
	}
	// Третья неудача может вернуться к первой (не повторяет только прошлую).
	if c := p.fail(); c.Sticker != "023" {
		t.Fatalf("третья неудача %+v", c)
	}
}

// Ловит: при любом случайном числе выбор выходит за список или повторяет прошлый.
func TestStickerPickerAlwaysInSet(t *testing.T) {
	for v := 0; v < 20; v++ {
		p := newStickerPicker(seqRand(v, v+3, v*7))
		prev := StickerPick{}
		for k := 0; k < 30; k++ {
			got := p.fail()
			if !contains(stickerFail, got.Sticker) || !contains(stickerFailTexts, got.Text) {
				t.Fatalf("вне набора: %+v", got)
			}
			if got.Sticker == prev.Sticker || got.Text == prev.Text {
				t.Fatalf("повтор %+v после %+v", got, prev)
			}
			prev = got
		}
	}
	if p := newStickerPicker(nil); !contains(stickerSuccess, p.success(5).Sticker) {
		t.Fatal("источник по умолчанию: стикер вне набора")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Ловит: кадры не той стороны или числа; уменьшение даёт непредумноженные пиксели или
// закрашенный фон; кэш разбирает спрайт заново на каждый показ.
func TestStickerFramesBGRA(t *testing.T) {
	counts, _ := stickerFrameCounts()
	frames, err := StickerFramesBGRA("017", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != counts["017"] {
		t.Fatalf("кадров %d, ждали %d", len(frames), counts["017"])
	}
	for i, f := range frames {
		if len(f) != 200*200*4 {
			t.Fatalf("кадр %d: %d байт", i, len(f))
		}
	}
	f := frames[len(frames)-1]
	if f[3] != 0 || f[len(f)-1] != 0 {
		t.Fatalf("углы последнего кадра не прозрачные: %v %v", f[:4], f[len(f)-4:])
	}
	opaque := 0
	for i := 0; i < len(f); i += 4 {
		if f[i] > f[i+3] || f[i+1] > f[i+3] || f[i+2] > f[i+3] {
			t.Fatalf("пиксель %d не предумножен: %v", i/4, f[i:i+4])
		}
		if f[i+3] > 200 {
			opaque++
		}
	}
	if opaque < 200*200/20 {
		t.Fatalf("непрозрачных пикселей %d: стикера почти нет", opaque)
	}
	again, _ := StickerFramesBGRA("017", 200)
	if &again[0][0] != &frames[0][0] {
		t.Fatal("второй запрос того же размера разобрал спрайт заново (нет кэша)")
	}
	other, _ := StickerFramesBGRA("017", 250)
	if len(other[0]) != 250*250*4 {
		t.Fatalf("кадр 250: %d байт", len(other[0]))
	}
	full, _ := StickerFramesBGRA("017", 400)
	if len(full[0]) != StickerFramePx*StickerFramePx*4 {
		t.Fatalf("больше 300 не увеличивается: %d байт", len(full[0]))
	}
	if _, err := StickerFramesBGRA("999", 200); err == nil {
		t.Fatal("нет ошибки у неизвестного стикера")
	}
	if fr, err := StickerFramesBGRA("017", 0); err != nil || len(fr) != 0 {
		t.Fatalf("размер 0: %d кадров, %v", len(fr), err)
	}
}

// Ловит: стикер при 100 % не 200 px, при 150 % больше 300 (кадры нарисованы на 300).
func TestStickerPxByDPI(t *testing.T) {
	for dpi, want := range map[int]int32{96: 200, 120: 250, 144: 300, 192: 300, 0: 200} {
		if got := StickerPx(dpi); got != want {
			t.Fatalf("StickerPx(%d) = %d, want %d", dpi, got, want)
		}
	}
}

// Ловит: кадр по времени считается не от начала показа, выходит за последний или крутится по
// кругу; при выключенных анимациях Windows стикер всё равно движется.
func TestStickerFrameAt(t *testing.T) {
	for _, c := range []struct {
		ms, frames int
		still      bool
		want       int
	}{
		{0, 33, false, 0},
		{82, 33, false, 0},
		{84, 33, false, 1},
		{1000, 33, false, 12},
		{2700, 33, false, 32},
		{9000, 33, false, 32},
		{-50, 33, false, 0},
		{0, 33, true, 32},
		{0, 0, false, 0},
	} {
		if got := stickerFrameAt(at(0), at(0).Add(msDur(c.ms)), c.frames, c.still); got != c.want {
			t.Fatalf("stickerFrameAt(%d мс, %d кадров, still=%v) = %d, want %d", c.ms, c.frames, c.still, got, c.want)
		}
	}
}

func msDur(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }

// Ловит: поток окон разбирает PNG сам (кэш не виден без разбора); после снятия экрана кэш держит
// кадры (до десятков МБ).
func TestStickerFramesCachedAndClear(t *testing.T) {
	ClearStickerCache()
	if _, ok := stickerFramesCached("033", 200); ok {
		t.Fatal("пустой кэш отдал кадры")
	}
	want, err := StickerFramesBGRA("033", 200)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := stickerFramesCached("033", 200)
	if !ok || &got[0][0] != &want[0][0] {
		t.Fatal("кадры в кэше не найдены")
	}
	if _, ok := stickerFramesCached("033", 400); ok {
		t.Fatal("400 px — те же 300, а в кэше только 200")
	}
	ClearStickerCache()
	if _, ok := stickerFramesCached("033", 200); ok {
		t.Fatal("после очистки кадры остались")
	}
}

// Ловит: проверка кэша ждёт идущий разбор (поток окон стоит ~100 мс с перехватом клавиш); два
// одновременных запроса разбирают один стикер дважды или получают разные срезы.
func TestStickerFramesConcurrent(t *testing.T) {
	ClearStickerCache()
	stickerCacheMu.Lock()
	done := make(chan struct{})
	stickerDecoding[stickerKey{"044", 200}] = done
	stickerCacheMu.Unlock()
	// Пока «идёт разбор», проверка кэша отвечает сразу.
	if _, ok := stickerFramesCached("044", 200); ok {
		t.Fatal("кадры есть до конца разбора")
	}
	stickerCacheMu.Lock()
	delete(stickerDecoding, stickerKey{"044", 200})
	close(done)
	stickerCacheMu.Unlock()

	ClearStickerCache()
	res := make(chan [][]byte, 2)
	for i := 0; i < 2; i++ {
		go func() {
			f, err := StickerFramesBGRA("044", 200)
			if err != nil {
				t.Error(err)
			}
			res <- f
		}()
	}
	a, b := <-res, <-res
	if len(a) == 0 || &a[0][0] != &b[0][0] {
		t.Fatal("два одновременных запроса получили разные кадры (разбор дважды)")
	}
	if _, err := StickerFramesBGRA("999", 200); err == nil {
		t.Fatal("нет ошибки у неизвестного стикера")
	}
	stickerCacheMu.Lock()
	n := len(stickerDecoding)
	stickerCacheMu.Unlock()
	if n != 0 {
		t.Fatalf("после разборов висят отметки: %d", n)
	}
}
