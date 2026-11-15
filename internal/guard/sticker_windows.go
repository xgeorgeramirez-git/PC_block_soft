//go:build windows

package guard

import "unsafe"

// Стикер ответа на промокод на экране блокировки (R26, D-83, RM5b-9). Кадры готовит чистый код
// (StickerFramesBGRA), здесь они один раз кладутся на фон экрана (OverBackground) в полосу памяти
// DC, а на экран выводится только прямоугольник стикера одним BitBlt: кадр не мигает фоном.

var (
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procBitBlt                 = gdi32.NewProc("BitBlt")
)

const (
	srcCopy = 0x00CC0020

	spiGetClientAreaAnimation = 0x1042

	// animTimerID — таймер кадров стикера на скрытом окне (у тика 250 мс — tickTimerID); идёт,
	// только пока кадр меняется (StickerAnimating).
	animTimerID = 8
	animEveryMs = 1000 / StickerFPS // 83 мс при 12 кадрах/с

	// wmAppStickerReady — кадры стикера разобраны в своей горутине: перестроить экран; wParam 1 — разбор
	// не удался (тогда остаётся надпись, повтора нет).
	wmAppStickerReady = wmApp + 11
)

// clientAreaAnimation — включены ли анимации Windows («Эффекты анимации» в параметрах);
// не удалось прочитать — включены.
func clientAreaAnimation() bool {
	var on int32 = 1
	r, _, _ := procSystemParametersInfo.Call(spiGetClientAreaAnimation, 0, uintptr(unsafe.Pointer(&on)), 0)
	if r == 0 {
		return true
	}
	return on != 0
}

// stickerPaint — кадры показанного стикера в памяти DC: полоса кадров слева направо.
type stickerPaint struct {
	show   bool
	rc     rect
	n      string
	size   int32
	frames int
	mem    uintptr // память DC; 0 — кадров нет
	bmp    uintptr
	oldBmp uintptr

	pending, failed stickerKey // разбор идёт в горутине / не удался (не повторять)
}

// prepare готовит кадры стикера n стороной size на фоне экрана; false — показать пока нельзя (тогда
// стоит надпись). Тот же стикер того же размера не готовится заново. Кадров нужного размера нет в
// кэше — они разбираются в своей горутине (около 100 мс; поток окон с перехватом клавиш не ждёт),
// а готовность приходит окну hwnd сообщением wmAppStickerReady.
func (s *stickerPaint) prepare(hdc, hwnd uintptr, n string, size int32) bool {
	if s.mem != 0 && s.n == n && s.size == size {
		return true
	}
	key := stickerKey{n, int(size)}
	frames, ok := stickerFramesCached(n, int(size))
	if !ok {
		if s.pending != key && s.failed != key {
			s.pending = key
			go safely(func() {
				bad := uintptr(0)
				if _, err := StickerFramesBGRA(n, int(size)); err != nil {
					bad = 1
				}
				procPostMessage.Call(hwnd, wmAppStickerReady, bad, 0)
			})
		}
		return false
	}
	s.freeBitmap()
	if len(frames) == 0 {
		return false
	}
	w := size * int32(len(frames))
	mem, _, _ := procCreateCompatibleDC.Call(hdc)
	if mem == 0 {
		return false
	}
	bmp, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(size))
	if bmp == 0 {
		procDeleteDC.Call(mem)
		return false
	}
	s.mem, s.bmp = mem, bmp
	s.oldBmp, _, _ = procSelectObject.Call(mem, bmp)
	r, g, b := bgBytes()
	for i, f := range frames {
		blitBGRA(mem, int32(i)*size, 0, size, size, OverBackground(f, r, g, b))
	}
	s.n, s.size, s.frames = n, size, len(frames)
	return true
}

// paint выводит кадр frame (за пределами — последний) в прямоугольник стикера.
func (s *stickerPaint) paint(hdc uintptr, frame int) {
	if !s.show || s.mem == 0 {
		return
	}
	frame = max(0, min(frame, s.frames-1))
	procBitBlt.Call(hdc, uintptr(s.rc.left), uintptr(s.rc.top), uintptr(s.size), uintptr(s.size),
		s.mem, uintptr(int32(frame)*s.size), 0, srcCopy)
}

// ready: горутина разбора закончила (bad — с ошибкой); экран перестроит вызывающий.
func (s *stickerPaint) ready(bad bool) {
	if bad {
		s.failed = s.pending
	}
	s.pending = stickerKey{}
}

// freeBitmap освобождает память DC и полосу кадров (отметки разбора остаются).
func (s *stickerPaint) freeBitmap() {
	if s.mem != 0 {
		procSelectObject.Call(s.mem, s.oldBmp)
		procDeleteDC.Call(s.mem)
	}
	if s.bmp != 0 {
		procDeleteObject.Call(s.bmp)
	}
	s.show, s.mem, s.bmp, s.oldBmp, s.n, s.size, s.frames = false, 0, 0, 0, "", 0, 0
}

// free освобождает полосу кадров; разбор, который ещё идёт, своё сообщение пришлёт.
func (s *stickerPaint) free() { s.freeBitmap() }
