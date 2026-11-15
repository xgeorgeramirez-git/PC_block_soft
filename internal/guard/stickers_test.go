package guard

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
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

// Ловит: кадр по времени считается не от начала показа, не 30 в секунду, останавливается на
// последнем вместо круга (D-84); при выключенных анимациях Windows стикер всё равно движется.
func TestStickerFrameAt(t *testing.T) {
	for _, c := range []struct {
		ms, frames int
		still      bool
		want       int
	}{
		{0, 81, false, 0},
		{33, 81, false, 0},
		{34, 81, false, 1},
		{1000, 81, false, 30},
		{2699, 81, false, 80},
		{2700, 81, false, 0}, // второй круг
		{2800, 81, false, 3},
		{9000, 81, false, 27}, // 270 по модулю 81
		{3600000, 90, false, 0},
		{-50, 81, false, 0},
		{0, 81, true, 80},
		{9000, 81, true, 80},
		{0, 0, false, 0},
		{500, 1, false, 0},
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

// Ловит: в кэше лежат кадры двух стикеров сразу (при 150 % один стикер из 90 кадров — 32 МБ,
// два — 65 МБ в памяти стража): держится только последний набор.
func TestStickerCacheHoldsOneSet(t *testing.T) {
	ClearStickerCache()
	defer ClearStickerCache()
	if stickerCacheSize != 1 {
		t.Fatalf("stickerCacheSize = %d, want 1", stickerCacheSize)
	}
	if _, err := StickerFramesBGRA("050", 200); err != nil {
		t.Fatal(err)
	}
	if _, err := StickerFramesBGRA("093", 200); err != nil {
		t.Fatal(err)
	}
	if _, ok := stickerFramesCached("050", 200); ok {
		t.Fatal("прошлый стикер остался в кэше")
	}
	if _, ok := stickerFramesCached("093", 200); !ok {
		t.Fatal("последнего стикера в кэше нет")
	}
	if _, err := StickerFramesBGRA("093", 250); err != nil {
		t.Fatal(err)
	}
	if _, ok := stickerFramesCached("093", 200); ok {
		t.Fatal("тот же стикер другого размера остался в кэше вторым набором")
	}
	stickerCacheMu.Lock()
	n := len(stickerCache)
	stickerCacheMu.Unlock()
	if n != 1 {
		t.Fatalf("наборов в кэше %d, want 1", n)
	}
}

// Ловит: быстрый разбор палитрового спрайта даёт другие пиксели, чем общий путь через NRGBA
// (предумножение, порядок BGRA, усреднение по площади).
func TestStickerDecodePalettedMatchesGeneric(t *testing.T) {
	data, err := stickerSprite("050")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	pal, ok := img.(*image.Paletted)
	if !ok {
		t.Fatalf("спрайт не палитровый: %T", img)
	}
	src := image.NewNRGBA(img.Bounds())
	draw.Draw(src, src.Bounds(), img, img.Bounds().Min, draw.Src)
	for _, px := range []int{200, 250, 300} {
		for _, f := range []int{0, 30, 58} {
			want := scaleBGRA(src, f*StickerFramePx, StickerFramePx, px)
			got := scalePalettedBGRA(pal, f*StickerFramePx, StickerFramePx, px)
			if !bytes.Equal(got, want) {
				t.Fatalf("кадр %d, %d px: быстрый разбор отличается от общего", f, px)
			}
		}
	}
}

// Замер (Ruling 0.6.1): разбор самого длинного стикера (90 кадров) при 100 % и 150 %. Разбор идёт
// вне потока окон; порог — с большим запасом, чтобы ловить только поломку (секунды вместо долей).
func TestStickerDecodeTime(t *testing.T) {
	for _, px := range []int{200, 300} {
		ClearStickerCache()
		start := time.Now()
		frames, err := StickerFramesBGRA("096", px)
		took := time.Since(start)
		if err != nil || len(frames) != 90 {
			t.Fatalf("096 при %d px: %d кадров, %v", px, len(frames), err)
		}
		bytesTotal := 0
		for _, f := range frames {
			bytesTotal += len(f)
		}
		if want := 90 * px * px * 4; bytesTotal != want {
			t.Fatalf("096 при %d px: %d байт кадров, want %d", px, bytesTotal, want)
		}
		t.Logf("096, 90 кадров, %d px: разбор %v, кадры %.1f МБ", px, took.Round(time.Millisecond), float64(bytesTotal)/(1<<20))
		if took > 10*time.Second {
			t.Fatalf("разбор 90 кадров при %d px занял %v", px, took)
		}
	}
	ClearStickerCache()
}

// Ловит: полоса кадров в памяти — одна строка шириной до 27000 px (90 кадров по 300): кадры лежат
// сеткой по 8 в ряд, клетка кадра считается по номеру.
func TestStickerStripGrid(t *testing.T) {
	if StickerStripCols != 8 {
		t.Fatalf("StickerStripCols = %d, want 8", StickerStripCols)
	}
	for _, c := range []struct{ frames, w, h int }{{90, 8, 12}, {81, 8, 11}, {8, 8, 1}, {9, 8, 2}, {5, 5, 1}, {1, 1, 1}, {0, 0, 0}} {
		if w, h := StickerStripSize(c.frames); w != c.w || h != c.h {
			t.Fatalf("StickerStripSize(%d) = %d×%d, want %d×%d", c.frames, w, h, c.w, c.h)
		}
	}
	for f, want := range map[int][2]int{0: {0, 0}, 7: {7, 0}, 8: {0, 1}, 89: {1, 11}} {
		if col, row := StickerStripCell(f); col != want[0] || row != want[1] {
			t.Fatalf("StickerStripCell(%d) = %d,%d, want %v", f, col, row, want)
		}
	}
}

// Ловит: полоса кадров собрана не на фоне экрана (прозрачное даёт чёрное), кадр лежит не в своей
// клетке или строки кадра сдвинуты; размер полосы не по сетке.
func TestStickerStripOver(t *testing.T) {
	const px = 3
	frames := make([][]byte, 10)
	for f := range frames {
		fr := make([]byte, px*px*4)
		for i := 0; i < px*px; i++ {
			// Пиксель 0 каждого кадра прозрачный, остальные непрозрачные: синий = номер кадра, зелёный = номер пикселя.
			if i > 0 {
				fr[i*4], fr[i*4+1], fr[i*4+2], fr[i*4+3] = byte(f+1), byte(i), 7, 255
			}
		}
		frames[f] = fr
	}
	strip, w, h := StickerStripOver(frames, px, 0x12, 0x34, 0x56)
	if w != 8*px || h != 2*px || len(strip) != w*h*4 {
		t.Fatalf("полоса %d×%d, %d байт, want 24×6", w, h, len(strip))
	}
	at := func(x, y int) [4]byte { o := (y*w + x) * 4; return [4]byte(strip[o : o+4]) }
	// Кадр 9 — клетка (1, 1): пиксель 0 — фон экрана (BGR 56 34 12), пиксель 5 (x=2, y=1) — свой.
	if got := at(1*px, 1*px); got != [4]byte{0x56, 0x34, 0x12, 255} {
		t.Fatalf("прозрачный пиксель кадра 9: %v, ждали фон", got)
	}
	if got := at(1*px+2, 1*px+1); got != [4]byte{10, 5, 7, 255} {
		t.Fatalf("пиксель 5 кадра 9: %v", got)
	}
	if got := at(7*px+1, 0); got != [4]byte{8, 1, 7, 255} {
		t.Fatalf("пиксель 1 кадра 7: %v", got)
	}
	// Клетки без кадров (10…15) залиты фоном.
	if got := at(5*px+1, 1*px+1); got != [4]byte{0x56, 0x34, 0x12, 255} {
		t.Fatalf("пустая клетка: %v", got)
	}
	// Клетка совпадает с OverBackground кадра.
	want := OverBackground(frames[3], 0x12, 0x34, 0x56)
	for y := 0; y < px; y++ {
		for x := 0; x < px; x++ {
			if got := at(3*px+x, y); got != [4]byte(want[(y*px+x)*4:(y*px+x)*4+4]) {
				t.Fatalf("кадр 3, пиксель %d,%d: %v", x, y, got)
			}
		}
	}
	if s, w, h := StickerStripOver(nil, px, 0, 0, 0); len(s) != 0 || w != 0 || h != 0 {
		t.Fatal("полоса без кадров не пустая")
	}
}
