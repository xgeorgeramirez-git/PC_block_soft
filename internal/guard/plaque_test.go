package guard

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"pcblock/internal/ipc"
	"pcblock/internal/sound"
)

// Ловит (ТЗ 8.2.4, 8.2.5, D-111, D-112, D-114): плашка не того стикера, цвета заголовка или звука;
// плашка сбоя кнопки или предупреждение о времени нарисованы «Игровым лобби».
func TestPlaqueStyleTable(t *testing.T) {
	exc, notify := sound.AliasExclamation, sound.AliasNotify
	for _, c := range []struct {
		kind    string
		allowed bool
		want    PlaqueStyle
	}{
		{ipc.BlockedUnknown, false, PlaqueStyle{"031", 0xFFE066, exc}},
		{ipc.BlockedForbidden, false, PlaqueStyle{"110", 0xFFB199, exc}},
		{ipc.BlockedSite, false, PlaqueStyle{"110", 0xFFB199, exc}},
		{ipc.BlockedAdult, false, PlaqueStyle{"110", 0xFFB199, exc}},
		{ipc.BlockedBrowser, false, PlaqueStyle{"110", 0xFFB199, exc}},
		{ipc.BlockedVideoOver, false, PlaqueStyle{"048", 0xFFB199, exc}},
		{ipc.BlockedGamesOver, false, PlaqueStyle{"048", 0xFFB199, exc}},
		{ipc.BlockedStudyOnly, false, PlaqueStyle{"048", 0xFFB199, exc}},
		{ipc.BlockedDecided, true, PlaqueStyle{"033", 0x9FF0C4, notify}},
		{ipc.BlockedDecided, false, PlaqueStyle{"110", 0xFFB199, exc}},
	} {
		got, ok := PlaqueStyleOf(c.kind, c.allowed)
		if !ok || got != c.want {
			t.Errorf("%s (разрешил %v): %+v %v, ждали %+v", c.kind, c.allowed, got, ok, c.want)
		}
		if stickerFrames(got.Sticker) <= 0 {
			t.Errorf("%s: стикера %s нет среди встроенных", c.kind, got.Sticker)
		}
	}
	for _, k := range []string{ipc.PlateNotice, ipc.PlateBoard, ipc.PlateOneMinute, ipc.PlateAdded, ipc.PlateRest, ""} {
		if s, ok := PlaqueStyleOf(k, false); ok {
			t.Errorf("%q: прежняя плашка нарисована бы «Лобби»: %+v", k, s)
		}
	}
}

// fakePlaqueMeasure — мерка Windows для плашки: буква 0,55 размера шрифта (жирная 0,6), строка 1,3
// размера; перенос по словам, слово длиннее строки — по буквам (DT_EDITCONTROL). Ширина — самой длинной
// строки.
func fakePlaqueMeasure(dpi int) PlaqueMeasure {
	return func(font, weight int, width int32, s string) (int32, int32) {
		px := Scaled(font, dpi)
		cw := max(1, px*55/100)
		if weight >= 700 {
			cw = max(1, px*60/100)
		}
		cpl := int(max(1, width/cw))
		lines, widest := 0, 0
		for _, para := range strings.Split(s, "\n") {
			cur := 0
			for _, w := range strings.Fields(para) {
				n := len([]rune(w))
				for n > cpl { // слово длиннее строки
					if cur > 0 {
						lines++
						widest = max(widest, cur)
					}
					lines++
					widest = max(widest, cpl)
					n -= cpl
					cur = 0
				}
				switch {
				case cur == 0:
					cur = n
				case cur+1+n <= cpl:
					cur += 1 + n
				default:
					lines++
					widest = max(widest, cur)
					cur = n
				}
			}
			lines++
			widest = max(widest, cur)
		}
		return int32(widest) * cw, int32(lines) * (px * 13 / 10)
	}
}

// longDomain — домен из 253 знаков без пробелов (предел DNS): переносится по буквам.
func longDomain() string {
	return strings.Repeat(strings.Repeat("a", 62)+".", 4) + "ru"[:1]
}

// Ловит (8.2.4, макет «Плашки про программы и сайты»): плашка не в правом нижнем углу рабочей области,
// тень вне окна, ширина не 470 при 100 %, текст обрезан или вылез за панель, кнопка без «Попросить
// сейчас» или есть у плашки без кнопки, стикер не 108 или налезает на текст, крестик не в углу.
func TestFitPlaqueCorner(t *testing.T) {
	if n := len(longDomain()); n != 253 {
		t.Fatalf("домен %d знаков", n)
	}
	texts := []string{"Пользуйся Brave", "Программа «SuperGame» новая. Родитель должен решить, можно ли её запускать",
		longDomain() + " в списке запрещённых. Если он нужен, попроси родителей"}
	for _, sz := range [][2]int32{{1024, 768}, {1366, 768}, {1920, 1080}} {
		for _, dpi := range []int{96, 120, 144} {
			area := Rect{0, 0, sz[0], sz[1] - Scaled(40, dpi)}
			for _, text := range texts {
				for _, btn := range []string{"", textAskNow} {
					name := fmt.Sprintf("%dx%d dpi %d, %d знаков, кнопка %q", sz[0], sz[1], dpi, len([]rune(text)), btn)
					m := fakePlaqueMeasure(dpi)
					l, place := FitPlaque(dpi, area, "Ждёт разрешения", text, btn, m)
					checkPlaque(t, name, dpi, area, l, place, text, btn, m)
				}
			}
		}
	}
	// 100 %: размеры макета.
	l, _ := FitPlaque(96, Rect{0, 0, 1920, 1040}, "Сайт закрыт", "Этот сайт для взрослых", "", fakePlaqueMeasure(96))
	if l.W != 470 || l.Sticker.Right-l.Sticker.Left != 108 || l.Sticker.Left != 3+16 || l.Close != (Rect{421, 15, 455, 49}) ||
		l.Panel != (Rect{0, 0, 470, l.H - 5}) || l.Shadow != (Rect{0, 5, 470, l.H}) || l.Radius != 24 || l.Border != 3 ||
		l.Title.Left != 3+16+108+14 || l.Title.Right != 470-3-54 {
		t.Fatalf("раскладка 100 %%: %+v", l)
	}
	if l.Panel.Bottom-l.Panel.Top != 3+18+108+18+3 {
		t.Fatalf("короткий текст: панель %d в высоту, ждали по стикеру 150", l.Panel.Bottom-l.Panel.Top)
	}
}

// checkPlaque — плашка с тенью в правом нижнем углу области (поле 16 при 100 %), блоки на местах.
func checkPlaque(t *testing.T, name string, dpi int, area Rect, l PlaqueLayout, place Rect, text, btn string, m PlaqueMeasure) {
	t.Helper()
	s := func(px int) int32 { return Scaled(px, dpi) }
	if place.Right != area.Right-s(16) || place.Bottom != area.Bottom-s(16) || place.Right-place.Left != l.W || place.Bottom-place.Top != l.H {
		t.Fatalf("%s: место %+v не в правом нижнем углу %+v (окно %dx%d)", name, place, area, l.W, l.H)
	}
	if place.Top < area.Top || place.Left < area.Left {
		t.Fatalf("%s: окно %+v вне области %+v", name, place, area)
	}
	if l.W != s(470) {
		t.Fatalf("%s: ширина %d, ждали %d", name, l.W, s(470))
	}
	in := func(r, box Rect) bool {
		return r.Left >= box.Left && r.Top >= box.Top && r.Right <= box.Right && r.Bottom <= box.Bottom
	}
	win := Rect{0, 0, l.W, l.H}
	if !in(l.Panel, win) || !in(l.Shadow, win) || l.Shadow.Bottom != l.H || l.Shadow.Top-l.Panel.Top != s(5) {
		t.Fatalf("%s: панель %+v или тень %+v вне окна %dx%d", name, l.Panel, l.Shadow, l.W, l.H)
	}
	inner := Rect{l.Panel.Left + l.Border, l.Panel.Top + l.Border, l.Panel.Right - l.Border, l.Panel.Bottom - l.Border}
	for _, r := range []Rect{l.Sticker, l.Title, l.Text, l.Close} {
		if !in(r, inner) {
			t.Fatalf("%s: блок %+v вне панели %+v", name, r, inner)
		}
	}
	if side := min(s(108), StickerFramePx); l.Sticker.Right-l.Sticker.Left != side || l.Sticker.Bottom-l.Sticker.Top != side {
		t.Fatalf("%s: стикер %+v, ждали %d", name, l.Sticker, side)
	}
	if l.Sticker.Right > l.Title.Left || l.Title.Bottom > l.Text.Top || l.Text.Right > l.Close.Left && l.Text.Top < l.Close.Bottom {
		t.Fatalf("%s: блоки налезают: %+v", name, l)
	}
	colW := l.Text.Right - l.Text.Left
	if _, h := m(l.TextPx, 600, colW, text); h != l.Text.Bottom-l.Text.Top {
		t.Fatalf("%s: текст %d px, место %+v: не целиком", name, h, l.Text)
	}
	if btn == "" {
		if l.Btn != (Rect{}) {
			t.Fatalf("%s: кнопка без кнопки: %+v", name, l.Btn)
		}
	} else {
		bw, _ := m(l.BtnPx, 900, colW, btn)
		if !in(l.Btn, inner) || l.Btn.Top < l.Text.Bottom || l.Btn.Bottom-l.Btn.Top != s(46) || l.Btn.Right-l.Btn.Left < bw+2*s(16) ||
			l.Btn.Left != l.Text.Left || l.Btn.Bottom+s(4) > inner.Bottom {
			t.Fatalf("%s: кнопка %+v (подпись %d), текст %+v, панель %+v", name, l.Btn, bw, l.Text, inner)
		}
	}
	if c := l.Close; c.Right != inner.Right-s(12) || c.Top != inner.Top+s(12) || c.Right-c.Left != s(34) || c.Bottom-c.Top != s(34) {
		t.Fatalf("%s: крестик %+v", name, c)
	}
}

func plaquePx(buf []byte, w int32, x, y int32) [4]byte { return px(buf, w, x, y) }

func near4(a, b [4]byte, tol int) bool { return diff4(a, b) <= tol }

// Ловит (макет): угол вне скругления не прозрачен (там клики должны проходить к окнам под плашкой),
// панель, рамка, тень, кнопка и крестик не тех цветов.
func TestRenderPlaquePixels(t *testing.T) {
	l, _ := FitPlaque(96, Rect{0, 0, 1920, 1040}, "Ждёт разрешения", "Программа «SuperGame» новая", textAskNow, fakePlaqueMeasure(96))
	buf := RenderPlaqueBase(l, false, false)
	if len(buf) != int(l.W*l.H*4) {
		t.Fatalf("буфер %d", len(buf))
	}
	if a := plaquePx(buf, l.W, 0, 0)[3]; a != 0 {
		t.Fatalf("угол вне скругления: альфа %d", a)
	}
	if a := plaquePx(buf, l.W, 0, l.H-1)[3]; a != 0 {
		t.Fatalf("нижний угол тени вне скругления: альфа %d", a)
	}
	mid := (l.Panel.Top + l.Panel.Bottom) / 2
	if got := plaquePx(buf, l.W, l.Text.Left+2, l.Text.Bottom-2); got != bgra(PlaqueBgRGB) {
		t.Fatalf("панель %v, ждали %v", got, bgra(PlaqueBgRGB))
	}
	if got := plaquePx(buf, l.W, l.Panel.Left+1, mid); got != bgra(PlaqueInkRGB) {
		t.Fatalf("рамка %v", got)
	}
	if got := plaquePx(buf, l.W, l.W/2, l.Panel.Top+l.Border+1); !near4(got, bgra(PlaqueHiRGB), 2) {
		t.Fatalf("блик сверху %v, ждали %v", got, bgra(PlaqueHiRGB))
	}
	if got := plaquePx(buf, l.W, l.W/2, l.Panel.Bottom+2); got != bgra(PlaqueInkRGB) {
		t.Fatalf("тень под панелью %v", got)
	}
	b := l.Btn
	bx := (b.Left + b.Right) / 2
	if got := plaquePx(buf, l.W, bx, b.Top+l.Border+Scaled(3, 96)+1); !near4(got, bgra(0xFFE066), 24) {
		t.Fatalf("кнопка сверху %v, ждали около %v", got, bgra(0xFFE066))
	}
	if got := plaquePx(buf, l.W, bx, b.Bottom-l.Border-2); got != bgra(0xD98200) {
		t.Fatalf("кнопка снизу %v, ждали %v", got, bgra(0xD98200))
	}
	if got := plaquePx(buf, l.W, bx, b.Bottom+1); got != bgra(PlaqueInkRGB) {
		t.Fatalf("тень кнопки %v", got)
	}
	c := l.Close
	if got := plaquePx(buf, l.W, c.Left+l.Border+2, c.Top+l.Border+2); got != bgra(0x22388F) {
		t.Fatalf("крестик %v", got)
	}
	cc := plaquePx(buf, l.W, (c.Left+c.Right)/2, (c.Top+c.Bottom)/2)
	if !near4(cc, bgra(0xFFFFFF), 8) {
		t.Fatalf("середина крестика %v, ждали белый «✕»", cc)
	}
	hot := RenderPlaqueBase(l, true, true)
	if plaquePx(hot, l.W, bx, (b.Top+b.Bottom)/2) == plaquePx(buf, l.W, bx, (b.Top+b.Bottom)/2) ||
		plaquePx(hot, l.W, c.Left+l.Border+2, c.Top+l.Border+2) == plaquePx(buf, l.W, c.Left+l.Border+2, c.Top+l.Border+2) {
		t.Fatal("кнопка и крестик под мышью не отличаются")
	}
	// Без кнопки — на её месте панель.
	l2, _ := FitPlaque(96, Rect{0, 0, 1920, 1040}, "Сайт закрыт", "Этот сайт для взрослых", "", fakePlaqueMeasure(96))
	if got := RenderPlaqueBase(l2, false, false); len(got) != int(l2.W*l2.H*4) {
		t.Fatal("буфер без кнопки")
	}
}

// Ловит: кадр стикера положен вне своего места, затирает основу вокруг, теряет прозрачность или не
// уменьшается из кадра 300 px.
func TestPlaqueStickerOver(t *testing.T) {
	l, _ := FitPlaque(96, Rect{0, 0, 1920, 1040}, "Сайт закрыт", "Этот сайт для взрослых", "", fakePlaqueMeasure(96))
	base := RenderPlaqueBase(l, false, false)
	dst := append([]byte(nil), base...)
	patch := PlaquePatch(base, l)
	side := l.Sticker.Right - l.Sticker.Left
	frame := make([]byte, side*side*4)
	for y := int32(0); y < side; y++ {
		for x := side / 2; x < side; x++ {
			i := (y*side + x) * 4
			frame[i+2], frame[i+3] = 200, 200 // красный, альфа 200 (предумноженный)
		}
	}
	if !PlaqueStickerOver(dst, patch, l, frame, int(side)) {
		t.Fatal("кадр не положен")
	}
	s := l.Sticker
	if plaquePx(dst, l.W, s.Left+2, s.Top+2) != plaquePx(base, l.W, s.Left+2, s.Top+2) {
		t.Fatal("прозрачная часть кадра затёрла основу")
	}
	got := plaquePx(dst, l.W, s.Right-2, s.Top+2)
	under := plaquePx(base, l.W, s.Right-2, s.Top+2)
	want := [4]byte{byte(int(under[0]) * 55 / 255), byte(int(under[1]) * 55 / 255), 200 + byte(int(under[2])*55/255), 255}
	if !near4(got, want, 1) {
		t.Fatalf("кадр поверх основы %v, ждали %v", got, want)
	}
	if plaquePx(dst, l.W, s.Right+1, s.Top+2) != plaquePx(base, l.W, s.Right+1, s.Top+2) {
		t.Fatal("кадр задел основу за стикером")
	}
	// Кадр 300 px уменьшается в место стикера: однотонный остаётся однотонным.
	big := make([]byte, 300*300*4)
	for i := 0; i < len(big); i += 4 {
		big[i], big[i+3] = 100, 255
	}
	if !PlaqueStickerOver(dst, patch, l, big, 300) {
		t.Fatal("кадр 300 px не положен")
	}
	if got := plaquePx(dst, l.W, s.Left+side/2, s.Top+side/2); got != [4]byte{100, 0, 0, 255} {
		t.Fatalf("уменьшенный кадр %v", got)
	}
	if PlaqueStickerOver(dst, patch, l, big[:100], 300) || PlaqueStickerOver(dst[:10], patch, l, frame, int(side)) ||
		PlaqueStickerOver(dst, patch[:8], l, frame, int(side)) {
		t.Fatal("кадр, заплатка или буфер не того размера приняты")
	}
	// Повтор кадра не накапливает его поверх прошлого: под каждый кадр — заплатка основы.
	PlaqueStickerOver(dst, patch, l, frame, int(side))
	again := plaquePx(dst, l.W, s.Right-2, s.Top+2)
	if !near4(again, want, 1) {
		t.Fatalf("второй кадр поверх первого %v, ждали %v", again, want)
	}
}

// Ловит: текст шрифта системы оставил дыры (альфа 0) в панели или правка альфы открыла прозрачный
// угол окна.
func TestPlaqueFixTextAlpha(t *testing.T) {
	l, _ := FitPlaque(96, Rect{0, 0, 1920, 1040}, "Ждёт разрешения", "Программа «SuperGame» новая", textAskNow, fakePlaqueMeasure(96))
	buf := RenderPlaqueBase(l, false, false)
	for _, p := range [][2]int32{{l.Text.Left + 1, l.Text.Top + 1}, {l.Title.Left - 2, l.Title.Top - 2}, {l.Btn.Left + 5, l.Btn.Top + 5}} {
		buf[(p[1]*l.W+p[0])*4+3] = 0 // GDI «съел» альфу
	}
	PlaqueFixTextAlpha(buf, l, 96)
	for _, p := range [][2]int32{{l.Text.Left + 1, l.Text.Top + 1}, {l.Title.Left - 2, l.Title.Top - 2}, {l.Btn.Left + 5, l.Btn.Top + 5}} {
		if a := buf[(p[1]*l.W+p[0])*4+3]; a != 255 {
			t.Fatalf("точка %v: альфа %d", p, a)
		}
	}
	if a := plaquePx(buf, l.W, 0, 0)[3]; a != 0 {
		t.Fatalf("угол окна: альфа %d", a)
	}
}

// Ловит (8.0 п. 9): текст и заголовки плашки на панели ниже 4,5:1.
func TestPlaqueContrast(t *testing.T) {
	lum := func(rgb uint32) float64 {
		ch := func(v uint32) float64 {
			x := float64(v&0xFF) / 255
			if x <= 0.03928 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(rgb>>16) + 0.7152*ch(rgb>>8) + 0.0722*ch(rgb)
	}
	ratio := func(a, b uint32) float64 {
		la, lb := lum(a), lum(b)
		return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
	}
	for _, c := range []struct {
		name string
		a, b uint32
	}{
		{"текст", PlaqueTextRGB, PlaqueBgRGB},
		{"заголовок ждёт", 0xFFE066, PlaqueBgRGB},
		{"заголовок разрешил", 0x9FF0C4, PlaqueBgRGB},
		{"заголовок запрещено", 0xFFB199, PlaqueBgRGB},
		{"подпись кнопки", PlaqueInkRGB, 0xFFB800},
		{"крестик", 0xFFFFFF, 0x22388F},
	} {
		if r := ratio(c.a, c.b); r < 4.5 {
			t.Errorf("%s: контраст %.2f ниже 4,5", c.name, r)
		}
	}
}

// Ловит (D-111, Review Focus 8): «Восклицание» на каждую повторную отправку той же плашки (служба
// повторяет её раз в 5 с), нет звука у новой плашки, «Уведомление» у разрешения не то, плашка и звук
// под экраном блокировки; плашка сбоя кнопки со звуком.
func TestPlaqueSoundOncePerPlate(t *testing.T) {
	r := newCtRig()
	r.svc.test = true
	var got []string
	done := make(chan string, 16)
	r.ctl.SetSoundPlayers(func() bool { return true }, nil, func(alias string) bool { done <- alias; return true })
	t.Cleanup(r.ctl.StopSounds)
	r.svc.status = ctRunningStatus(3600)
	p := ipc.BlockedPlate{Kind: ipc.BlockedForbidden, Title: "Эта программа запрещена", Text: "Родитель запретил «X». Выбери другую игру", ShowS: 10, Path: `c:\x.exe`}
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdBlocked, Plate: &p}}
	v := r.poll()
	if v.Warning == nil || v.Warning.Sticker != "110" || v.Warning.Title != p.Title {
		t.Fatalf("плашка %+v", v.Warning)
	}
	r.clk.Advance(5 * time.Second)
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdBlocked, Plate: &p}} // та же, служба повторила
	r.poll()
	ok := ipc.BlockedPlate{Kind: ipc.BlockedDecided, Title: "Можно играть!", Text: "Родитель разрешил «X»: это игра. Запусти её снова", ShowS: 10, Path: `c:\x.exe`, Allowed: true}
	r.clk.Advance(time.Second)
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdBlocked, Plate: &ok}}
	if v := r.poll(); v.Warning == nil || v.Warning.Sticker != "033" || !v.Warning.Allowed {
		t.Fatalf("плашка решения %+v", v.Warning)
	}
	r.ctl.model.OnPlateNotice(r.clk.Now(), "Не получилось. Попробуй ещё раз") // сбой кнопки: прежняя плашка, без звука
	if v := r.ctl.View(); v.Warning == nil || v.Warning.Kind != ipc.PlateNotice || v.Warning.Sticker != "" {
		t.Fatalf("плашка сбоя %+v", v.Warning)
	}
	// Под экраном блокировки: ни плашки, ни звука.
	r.clk.Advance(time.Second)
	r.svc.status = ctTimeUpStatus()
	r.poll()
	r.svc.cmds = []ipc.GuardCommand{{Kind: ipc.GuardCmdBlocked, Plate: &p}}
	if v := r.poll(); v.Warning != nil {
		t.Fatalf("под экраном блокировки плашка %+v", v.Warning)
	}
	for end := time.Now().Add(time.Second); time.Now().Before(end) && len(got) < 3; {
		select {
		case a := <-done:
			got = append(got, a)
		case <-time.After(50 * time.Millisecond):
		}
	}
	if strings.Join(got, ",") != sound.AliasExclamation+","+sound.AliasNotify {
		t.Fatalf("звуки %v, ждали «Восклицание» и «Уведомление» по разу", got)
	}
}

// Ловит (NFR-5, B-84): плашка «Лобби» держит больше 10 МБ (все кадры в BGRA, основа окна целиком
// вдобавок к DIB) или вместе с окном сообщения больше 26 МБ. Плашка не бывает под экраном блокировки
// (там полоса кадров промокода): худшее сочетание с плашкой — плашка и сообщение.
func TestPlaqueMemoryBudget(t *testing.T) {
	const dpi = 144
	frames := 3 * StickerFPS // самый длинный стикер набора
	for _, n := range []string{"031", "033", "048", "110"} {
		if f := stickerFrames(n); f > frames {
			t.Fatalf("стикер %s: %d кадров больше %d", n, f, frames)
		}
	}
	area := Rect{0, 0, 1920, 1080 - Scaled(40, dpi)}
	worst := 0
	for _, text := range []string{"Пользуйся Brave", longDomain() + " в списке запрещённых. Если он нужен, попроси родителей"} {
		l, _ := FitPlaque(dpi, area, "Ждёт разрешения", text, textAskNow, fakePlaqueMeasure(dpi))
		m := PlaqueMemory(frames, l)
		worst = max(worst, m)
		if m > PlaqueMemoryBudget {
			t.Fatalf("плашка %dx%d: %d байт, бюджет %d", l.W, l.H, m, PlaqueMemoryBudget)
		}
	}
	note := NoteMemory(frames, NoteLayout{W: 1920 * 80 / 100, H: 1080 * 55 / 100, StickerPx: NoteStickerPx(dpi)})
	t.Logf("плашка %.1f МБ, сообщение %.1f МБ", float64(worst)/(1<<20), float64(note)/(1<<20))
	if worst+note > 26<<20 {
		t.Fatalf("плашка и сообщение вместе %d байт, больше 26 МБ", worst+note)
	}
	// Под экраном блокировки плашки нет: полоса промокода с ней не встречается.
	m := NewModel()
	m.OnDecision(t0, timeUpDecision(), false, nil)
	m.OnBlocked(t0, ipc.BlockedPlate{Kind: ipc.BlockedSite, Title: "Сайт закрыт", Text: "x", ShowS: 10})
	if v := m.View(t0); v.Warning != nil || !v.LockShown {
		t.Fatalf("под экраном блокировки плашка %+v", v.Warning)
	}
}
