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
}

// prepare готовит кадры стикера n стороной size на фоне экрана; false — показать нельзя (тогда
// остаётся надпись). Тот же стикер того же размера не готовится заново.
func (s *stickerPaint) prepare(hdc uintptr, n string, size int32) bool {
	if s.mem != 0 && s.n == n && s.size == size {
		return true
	}
	s.free()
	frames, err := StickerFramesBGRA(n, int(size))
	if err != nil || len(frames) == 0 {
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

// free освобождает память DC и полосу кадров.
func (s *stickerPaint) free() {
	if s.mem != 0 {
		procSelectObject.Call(s.mem, s.oldBmp)
		procDeleteDC.Call(s.mem)
	}
	if s.bmp != 0 {
		procDeleteObject.Call(s.bmp)
	}
	*s = stickerPaint{}
}
