//go:build windows

package guard

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// Стикер ответа на промокод на экране блокировки (R26, D-83…D-85, RM5b-9). Кадры и полосу-сетку на
// фоне экрана готовит чистый код в своей горутине (StickerFramesBGRA, StickerStripOver); поток окон
// кладёт готовую полосу в память DC одним вызовом и выводит кадр одним BitBlt прямо в окно — только
// прямоугольник стикера, без WM_PAINT и заливки фона: кадр не мигает. Шаг кадров ровный: его задаёт
// горутина с time.Ticker, а не WM_TIMER (тот кратен шагу системных часов 15,6 мс).

var (
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procBitBlt                 = gdi32.NewProc("BitBlt")
)

const (
	srcCopy = 0x00CC0020

	spiGetClientAreaAnimation = 0x1042

	// animStep — шаг проверки кадра: вдвое чаще кадров (60 раз в секунду при 30 кадрах/с), так кадр
	// меняется через раз и стоит на экране ровно свои 33 мс.
	animStep = time.Second / (2 * StickerFPS)

	// wmAppStickerReady — полоса кадров стикера собрана в своей горутине: перестроить экран; wParam 1 —
	// разбор не удался (тогда остаётся надпись, повтора нет).
	wmAppStickerReady = wmApp + 11
	// wmAppStickerFrame — шаг кадров стикера (скрытому окну стража): вывести кадр, если он сменился.
	wmAppStickerFrame = wmApp + 12
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

// stickerAnim — шаг кадров: горутина раз в animStep шлёт окну hwnd сообщение wmAppStickerFrame. Пока
// прошлое сообщение не разобрано (поток окон занят), новое не шлётся: очередь не копится, а кадр
// поток окон выбирает по часам, так что пропущенный шаг движение не замедляет.
type stickerAnim struct {
	stop   chan struct{}
	posted atomic.Bool // сообщение в пути
}

func startStickerAnim(hwnd uintptr) *stickerAnim {
	s := &stickerAnim{stop: make(chan struct{})}
	go safely(func() {
		t := time.NewTicker(animStep)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				if s.posted.CompareAndSwap(false, true) {
					if r, _, _ := procPostMessage.Call(hwnd, wmAppStickerFrame, 0, 0); r == 0 {
						s.posted.Store(false)
					}
				}
			}
		}
	})
	return s
}

// handled: поток окон разобрал сообщение шага.
func (s *stickerAnim) handled() { s.posted.Store(false) }

func (s *stickerAnim) close() { close(s.stop) }

// stickerStrip — собранная полоса кадров: непрозрачные BGRA w×h (StickerStripOver).
type stickerStrip struct {
	key    stickerKey
	px     []byte
	w, h   int
	frames int
}

// stickerPaint — кадры показанного стикера в памяти DC: полоса-сетка (StickerStripCell). Полоса одна:
// новая кладётся на место прошлой.
type stickerPaint struct {
	show   bool
	rc     rect
	n      string
	size   int32
	frames int
	cur    int     // кадр, выведенный на экран последним; -1 — ещё ни один
	mem    uintptr // память DC; 0 — кадров нет
	bmp    uintptr
	oldBmp uintptr

	pending, failed stickerKey // сборка идёт в горутине / не удалась (не повторять)

	mu    sync.Mutex
	built *stickerStrip // полоса, собранная горутиной и ещё не забранная потоком окон
}

// takeBuilt забирает собранную полосу для key; чужая (ответ уже сменился) выбрасывается.
func (s *stickerPaint) takeBuilt(key stickerKey) *stickerStrip {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.built
	s.built = nil
	if b == nil || b.key != key {
		return nil
	}
	return b
}

// prepare готовит кадры стикера n стороной size на фоне экрана; false — показать пока нельзя (тогда
// стоит надпись). Тот же стикер того же размера не готовится заново. Полосы нет — кадры разбираются
// и складываются в полосу в своей горутине (доли секунды; поток окон с перехватом клавиш не ждёт), а
// готовность приходит окну hwnd сообщением wmAppStickerReady: тогда полоса одним вызовом кладётся в
// память DC, а разобранные кадры и сама полоса в памяти Go больше не держатся.
func (s *stickerPaint) prepare(hdc, hwnd uintptr, n string, size int32) bool {
	if s.mem != 0 && s.n == n && s.size == size {
		return true
	}
	key := stickerKey{n, int(size)}
	strip := s.takeBuilt(key)
	if strip == nil {
		if s.pending != key && s.failed != key {
			s.pending = key
			r, g, b := bgBytes()
			go safely(func() {
				bad := uintptr(1)
				if frames, err := StickerFramesBGRA(n, int(size)); err == nil && len(frames) > 0 {
					px, w, h := StickerStripOver(frames, int(size), r, g, b)
					s.mu.Lock()
					s.built = &stickerStrip{key: key, px: px, w: w, h: h, frames: len(frames)}
					s.mu.Unlock()
					bad = 0
				}
				procPostMessage.Call(hwnd, wmAppStickerReady, bad, 0)
			})
		}
		return false
	}
	s.freeBitmap()
	ClearStickerCache() // кадры уже в полосе: второй раз в памяти не лежат
	mem, _, _ := procCreateCompatibleDC.Call(hdc)
	if mem == 0 {
		return false
	}
	bmp, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(strip.w), uintptr(strip.h))
	if bmp == 0 {
		procDeleteDC.Call(mem)
		return false
	}
	s.mem, s.bmp = mem, bmp
	s.oldBmp, _, _ = procSelectObject.Call(mem, bmp)
	blitBGRA(mem, 0, 0, int32(strip.w), int32(strip.h), strip.px)
	s.n, s.size, s.frames, s.cur = n, size, strip.frames, -1
	return true
}

// paint выводит кадр frame (за пределами — последний) в прямоугольник стикера: один BitBlt из полосы.
func (s *stickerPaint) paint(hdc uintptr, frame int) {
	if !s.show || s.mem == 0 {
		return
	}
	frame = max(0, min(frame, s.frames-1))
	col, row := StickerStripCell(frame)
	procBitBlt.Call(hdc, uintptr(s.rc.left), uintptr(s.rc.top), uintptr(s.size), uintptr(s.size),
		s.mem, uintptr(int32(col)*s.size), uintptr(int32(row)*s.size), srcCopy)
	s.cur = frame
}

// ready: горутина сборки закончила (bad — с ошибкой); экран перестроит вызывающий.
func (s *stickerPaint) ready(bad bool) {
	if bad {
		s.failed = s.pending
	}
	s.pending = stickerKey{}
}

// freeBitmap освобождает память DC и полосу кадров (отметки сборки остаются).
func (s *stickerPaint) freeBitmap() {
	if s.mem != 0 {
		procSelectObject.Call(s.mem, s.oldBmp)
		procDeleteDC.Call(s.mem)
	}
	if s.bmp != 0 {
		procDeleteObject.Call(s.bmp)
	}
	s.show, s.mem, s.bmp, s.oldBmp, s.n, s.size, s.frames, s.cur = false, 0, 0, 0, "", 0, 0, -1
}

// free освобождает полосу кадров, в том числе собранную и не забранную; сборка, которая ещё идёт,
// своё сообщение пришлёт (её полоса заберётся или выбросится следующей раскладкой).
func (s *stickerPaint) free() {
	s.freeBitmap()
	s.mu.Lock()
	s.built = nil
	s.mu.Unlock()
}
