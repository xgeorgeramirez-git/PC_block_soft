//go:build windows

package guard

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Чтение поля адреса Brave через COM UI Automation (RM4-1, R23). Только x/sys/windows: go-ole
// напрямую не подключается. Весь COM живёт в одном потоке ОС (горутина uiaThread с
// runtime.LockOSThread, апартамент MTA): объекты UIA не переходят между потоками, а зависший
// Brave не останавливает опрос стража — Read ждёт ответа не дольше uiaCallTimeout.
//
// Текст поля адреса — только в локальных переменных: наружу Read отдаёт его вызывающему, тот
// сразу берёт домен (domainMemo.Read, sites.FromOmnibox) и текст забывает. Ни адрес, ни заголовок
// окна не хранятся.

var (
	ole32    = windows.NewLazySystemDLL("ole32.dll")
	oleaut32 = windows.NewLazySystemDLL("oleaut32.dll")

	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procSysAllocString   = oleaut32.NewProc("SysAllocString")
	procSysFreeString    = oleaut32.NewProc("SysFreeString")
	procSysStringLen     = oleaut32.NewProc("SysStringLen")
	procVariantClear     = oleaut32.NewProc("VariantClear")
)

var (
	clsidCUIAutomation            = windows.GUID{Data1: 0xff48dba4, Data2: 0x60ef, Data3: 0x4201, Data4: [8]byte{0xaa, 0x87, 0x54, 0x10, 0x3e, 0xef, 0x59, 0x4e}}
	iidIUIAutomation              = windows.GUID{Data1: 0x30cbe57d, Data2: 0xd9d0, Data3: 0x452a, Data4: [8]byte{0xab, 0x13, 0x7a, 0xc5, 0xac, 0x48, 0x25, 0xee}}
	iidIUIAutomationValue         = windows.GUID{Data1: 0xa94cd8b1, Data2: 0x0844, Data3: 0x4cd6, Data4: [8]byte{0x9d, 0x2d, 0x64, 0x05, 0x37, 0xab, 0x39, 0xe9}}
	iidIUIAutomationInvoke        = windows.GUID{Data1: 0xfb377fbe, Data2: 0x8ea6, Data3: 0x46d5, Data4: [8]byte{0x9c, 0x73, 0x64, 0x99, 0x64, 0x2d, 0x30, 0x59}}
	errUIANotFound                = errors.New("UIA: элемент не найден")
	errUIABusy                    = errors.New("UIA: прошлый вызов ещё не вернулся")
	errUIATimeout                 = errors.New("UIA: нет ответа")
	errUIAClosed                  = errors.New("UIA: поток закрыт")
	errUIANoPattern               = errors.New("UIA: у элемента нет нужного шаблона")
	uiaCallTimeout                = 2 * time.Second
	clsctxInprocServer     uint32 = 0x1
)

// Константы UIAutomationClient.h.
const (
	uiaControlTypePropertyID         = 30003
	uiaNamePropertyID                = 30005
	uiaHasKeyboardFocusPropertyID    = 30008
	uiaAutomationIDPropertyID        = 30011
	uiaClassNamePropertyID           = 30012
	uiaFrameworkIDPropertyID         = 30024
	uiaIsValuePatternAvailablePropID = 30043
	uiaValueValuePropertyID          = 30045
	uiaLegacyValuePropertyID         = 30093
	uiaSelectionItemIsSelectedPropID = 30079
	uiaInvokePatternID               = 10000
	uiaValuePatternID                = 10002
	uiaButtonControlTypeID           = 50000
	uiaEditControlTypeID             = 50004
	uiaTabItemControlTypeID          = 50019
	uiaDocumentControlTypeID         = 50030
	treeScopeChildren                = 0x2
	treeScopeDescendants             = 0x4

	vtI4   = 3
	vtBSTR = 8
	vtBool = 11
)

// omniboxClassNames — известные ClassName поля адреса (быстрый поиск FindFirst по условию; UIA не
// умеет условие «оканчивается на»): Brave и Chrome. Прочие окончания на OmniboxViewViews находит
// обход дерева (isOmniboxClass).
var omniboxClassNames = []string{"BraveOmniboxViewViews", "OmniboxViewViews"}

// Номера методов в vtable (UIAutomationClient.h, Windows SDK; IUnknown — 0 QueryInterface,
// 1 AddRef, 2 Release). Ошибку номера проба T0 покажет сразу (неверный вызов — сбой или мусор).
const (
	// IUIAutomation: 3 CompareElements, 4 CompareRuntimeIds, 5 GetRootElement, 6 ElementFromHandle,
	// 7 ElementFromPoint, 8 GetFocusedElement, 9…12 *BuildCache, 13 CreateTreeWalker,
	// 14…16 get_*ViewWalker, 17…19 get_*ViewCondition, 20 CreateCacheRequest, 21 CreateTrueCondition,
	// 22 CreateFalseCondition, 23 CreatePropertyCondition, 24 CreatePropertyConditionEx,
	// 25 CreateAndCondition.
	vtAutoElementFromHandle       = 6
	vtAutoCreateTrueCondition     = 21
	vtAutoCreatePropertyCondition = 23
	vtAutoCreateAndCondition      = 25
	// IUIAutomationElement: 3 SetFocus, 4 GetRuntimeId, 5 FindFirst, 6 FindAll, 7 FindFirstBuildCache,
	// 8 FindAllBuildCache, 9 BuildUpdatedCache, 10 GetCurrentPropertyValue, 11 …Ex,
	// 12 GetCachedPropertyValue, 13 …Ex, 14 GetCurrentPatternAs, 15 GetCachedPatternAs,
	// 16 GetCurrentPattern, 17 GetCachedPattern, 18 GetCachedParent, 19 GetCachedChildren,
	// 20 get_CurrentProcessId, 21 get_CurrentControlType, 22 get_CurrentLocalizedControlType,
	// 23 get_CurrentName, 24 get_CurrentAcceleratorKey, 25 get_CurrentAccessKey,
	// 26 get_CurrentHasKeyboardFocus, 27 get_CurrentIsKeyboardFocusable, 28 get_CurrentIsEnabled,
	// 29 get_CurrentAutomationId, 30 get_CurrentClassName.
	vtElemFindFirst               = 5
	vtElemFindAll                 = 6
	vtElemGetCurrentPropertyValue = 10
	vtElemGetCurrentPattern       = 16
	vtElemCurrentControlType      = 21
	vtElemCurrentName             = 23
	vtElemCurrentClassName        = 30
	// IUIAutomationElementArray: 3 get_Length, 4 GetElement.
	vtArrLength     = 3
	vtArrGetElement = 4
	// IUIAutomationValuePattern: 3 SetValue, 4 get_CurrentValue, 5 get_CurrentIsReadOnly.
	vtValueCurrentValue = 4
	// IUIAutomationInvokePattern: 3 Invoke.
	vtInvokeInvoke = 3
)

// variant — VARIANT для x64 (24 байта): тип, три зарезервированных слова и 16 байт значения.
type variant struct {
	vt   uint16
	_    [3]uint16
	val  uintptr
	val2 uintptr
}

// comObj — указатель на COM-объект: первое слово объекта — указатель на vtable. Типизированный
// указатель (а не uintptr), чтобы сборщик мусора и go vet видели в нём указатель.
type comObj = *comHeader

type comHeader struct{ vtbl *[64]uintptr }

func call(o comObj, method int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[method], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}

func release(o comObj) {
	if o != nil {
		call(o, 2)
	}
}

func queryInterface(o comObj, iid *windows.GUID) (comObj, error) {
	var out comObj
	if hr := call(o, 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))); hr != 0 || out == nil {
		return nil, hresultErr("QueryInterface", hr)
	}
	return out, nil
}

func hresultErr(what string, hr uintptr) error {
	return fmt.Errorf("UIA %s: 0x%08x", what, uint32(hr))
}

func bstrFrom(s string) (uintptr, error) {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return 0, err
	}
	b, _, _ := procSysAllocString.Call(uintptr(unsafe.Pointer(p)))
	if b == 0 {
		return 0, errors.New("UIA: SysAllocString")
	}
	return b, nil
}

// bstrTake читает BSTR, который вернул COM, и освобождает его.
func bstrTake(b *uint16) string {
	if b == nil {
		return ""
	}
	n, _, _ := procSysStringLen.Call(uintptr(unsafe.Pointer(b)))
	s := windows.UTF16ToString(unsafe.Slice(b, n))
	procSysFreeString.Call(uintptr(unsafe.Pointer(b)))
	return s
}

func variantClear(v *variant) { procVariantClear.Call(uintptr(unsafe.Pointer(v))) }

// uia — объект IUIAutomation; все методы — только в потоке uiaThread.
type uia struct{ auto comObj }

// propCond — CreatePropertyCondition. VARIANT по значению: в x64 структура длиннее 8 байт
// передаётся указателем на копию, поэтому здесь передаётся адрес variant.
func (u uia) propCond(prop int32, v *variant) (comObj, error) {
	var cond comObj
	if hr := call(u.auto, vtAutoCreatePropertyCondition, uintptr(prop), uintptr(unsafe.Pointer(v)), uintptr(unsafe.Pointer(&cond))); hr != 0 || cond == nil {
		return nil, hresultErr("CreatePropertyCondition", hr)
	}
	return cond, nil
}

func (u uia) strCond(prop int32, s string) (comObj, error) {
	b, err := bstrFrom(s)
	if err != nil {
		return nil, err
	}
	v := variant{vt: vtBSTR, val: b}
	defer variantClear(&v)
	return u.propCond(prop, &v)
}

func (u uia) intCond(prop int32, n int32) (comObj, error) {
	v := variant{vt: vtI4, val: uintptr(uint32(n))}
	return u.propCond(prop, &v)
}

func (u uia) boolCond(prop int32, b bool) (comObj, error) {
	v := variant{vt: vtBool}
	if b {
		v.val = 0xffff // VARIANT_TRUE
	}
	return u.propCond(prop, &v)
}

func (u uia) andCond(a, b comObj) (comObj, error) {
	var cond comObj
	if hr := call(u.auto, vtAutoCreateAndCondition, uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(&cond))); hr != 0 || cond == nil {
		return nil, hresultErr("CreateAndCondition", hr)
	}
	return cond, nil
}

func (u uia) trueCond() (comObj, error) {
	var cond comObj
	if hr := call(u.auto, vtAutoCreateTrueCondition, uintptr(unsafe.Pointer(&cond))); hr != 0 || cond == nil {
		return nil, hresultErr("CreateTrueCondition", hr)
	}
	return cond, nil
}

func (u uia) fromHandle(hwnd windows.HWND) (comObj, error) {
	var el comObj
	if hr := call(u.auto, vtAutoElementFromHandle, uintptr(hwnd), uintptr(unsafe.Pointer(&el))); hr != 0 || el == nil {
		return nil, hresultErr("ElementFromHandle", hr)
	}
	return el, nil
}

// findFirst — FindFirst; не нашлось — errUIANotFound. cond освобождает вызывающий.
func findFirst(root comObj, scope uintptr, cond comObj) (comObj, error) {
	var el comObj
	hr := call(root, vtElemFindFirst, scope, uintptr(unsafe.Pointer(cond)), uintptr(unsafe.Pointer(&el)))
	if hr != 0 {
		return nil, hresultErr("FindFirst", hr)
	}
	if el == nil {
		return nil, errUIANotFound
	}
	return el, nil
}

// findAll — FindAll: до max элементов (каждый освобождает вызывающий).
func findAll(root comObj, scope uintptr, cond comObj, max int) ([]comObj, error) {
	var arr comObj
	if hr := call(root, vtElemFindAll, scope, uintptr(unsafe.Pointer(cond)), uintptr(unsafe.Pointer(&arr))); hr != 0 || arr == nil {
		return nil, hresultErr("FindAll", hr)
	}
	defer release(arr)
	var n int32
	if hr := call(arr, vtArrLength, uintptr(unsafe.Pointer(&n))); hr != 0 {
		return nil, hresultErr("get_Length", hr)
	}
	out := make([]comObj, 0, min(int(n), max))
	for i := int32(0); i < n && len(out) < max; i++ {
		var el comObj
		if hr := call(arr, vtArrGetElement, uintptr(i), uintptr(unsafe.Pointer(&el))); hr == 0 && el != nil {
			out = append(out, el)
		}
	}
	return out, nil
}

func elemBSTR(el comObj, method int) (string, error) {
	var b *uint16
	if hr := call(el, method, uintptr(unsafe.Pointer(&b))); hr != 0 {
		return "", hresultErr("get_Current*", hr)
	}
	return bstrTake(b), nil
}

func elemName(el comObj) (string, error)      { return elemBSTR(el, vtElemCurrentName) }
func elemClassName(el comObj) (string, error) { return elemBSTR(el, vtElemCurrentClassName) }

func elemControlType(el comObj) (int32, error) {
	var ct int32
	if hr := call(el, vtElemCurrentControlType, uintptr(unsafe.Pointer(&ct))); hr != 0 {
		return 0, hresultErr("get_CurrentControlType", hr)
	}
	return ct, nil
}

// elemBool — GetCurrentPropertyValue свойства-флага (VT_BOOL).
func elemBool(el comObj, prop int32) (bool, error) {
	var v variant
	if hr := call(el, vtElemGetCurrentPropertyValue, uintptr(prop), uintptr(unsafe.Pointer(&v))); hr != 0 {
		return false, hresultErr("GetCurrentPropertyValue", hr)
	}
	defer variantClear(&v)
	return v.vt == vtBool && uint16(v.val) != 0, nil
}

// variantBSTR — тот же VARIANT, но значение — указатель на строку (VT_BSTR): так go vet видит
// указатель, а не uintptr.
type variantBSTR struct {
	vt   uint16
	_    [3]uint16
	val  *uint16
	val2 uintptr
}

// elemString — GetCurrentPropertyValue строкового свойства (VT_BSTR); другой тип — «».
func elemString(el comObj, prop int32) (string, error) {
	var v variant
	if hr := call(el, vtElemGetCurrentPropertyValue, uintptr(prop), uintptr(unsafe.Pointer(&v))); hr != 0 {
		return "", hresultErr("GetCurrentPropertyValue", hr)
	}
	defer variantClear(&v)
	if v.vt != vtBSTR {
		return "", nil
	}
	b := (*variantBSTR)(unsafe.Pointer(&v)).val
	if b == nil {
		return "", nil
	}
	n, _, _ := procSysStringLen.Call(uintptr(unsafe.Pointer(b)))
	return windows.UTF16ToString(unsafe.Slice(b, n)), nil
}

// walkTree обходит дерево UIA под root по детям (FindAll TreeScope_Children), не глубже maxDepth и
// не больше maxNodes узлов, не заходя внутрь Document (дерево страницы не обходится, NFR-5). visit
// получает элемент и глубину (1 — дети root): keep — элемент остаётся у вызывающего (он его и
// освободит), stop — обход окончен.
func walkTree(u uia, root comObj, maxDepth, maxNodes int, visit func(el comObj, depth int) (keep, stop bool)) error {
	cond, err := u.trueCond()
	if err != nil {
		return err
	}
	defer release(cond)
	count, stopped := 0, false
	var walk func(node comObj, depth int) error
	walk = func(node comObj, depth int) error {
		if count >= maxNodes {
			return nil
		}
		kids, err := findAll(node, treeScopeChildren, cond, maxNodes-count)
		if err != nil {
			return err
		}
		for _, el := range kids {
			if stopped || count >= maxNodes {
				release(el)
				continue
			}
			count++
			keep, stop := visit(el, depth)
			if stop {
				stopped = true
			} else if depth < maxDepth {
				if ct, err := elemControlType(el); err == nil && ct != uiaDocumentControlTypeID {
					_ = walk(el, depth+1) // недоступная ветка не мешает обходу соседних
				}
			}
			if !keep {
				release(el)
			}
		}
		return nil
	}
	return walk(root, 1)
}

// elemPattern — GetCurrentPattern, затем QueryInterface нужного интерфейса шаблона.
func elemPattern(el comObj, pattern int32, iid *windows.GUID) (comObj, error) {
	var unk comObj
	if hr := call(el, vtElemGetCurrentPattern, uintptr(pattern), uintptr(unsafe.Pointer(&unk))); hr != 0 {
		return nil, hresultErr("GetCurrentPattern", hr)
	}
	if unk == nil {
		return nil, errUIANoPattern
	}
	defer release(unk)
	return queryInterface(unk, iid)
}

// elemValue — ValuePattern.CurrentValue.
func elemValue(el comObj) (string, error) {
	vp, err := elemPattern(el, uiaValuePatternID, &iidIUIAutomationValue)
	if err != nil {
		return "", err
	}
	defer release(vp)
	var b *uint16
	if hr := call(vp, vtValueCurrentValue, uintptr(unsafe.Pointer(&b))); hr != 0 {
		return "", hresultErr("get_CurrentValue", hr)
	}
	return bstrTake(b), nil
}

func elemInvoke(el comObj) error {
	ip, err := elemPattern(el, uiaInvokePatternID, &iidIUIAutomationInvoke)
	if err != nil {
		return err
	}
	defer release(ip)
	if hr := call(ip, vtInvokeInvoke); hr != 0 {
		return hresultErr("Invoke", hr)
	}
	return nil
}

// uiaThread — поток ОС, в котором живёт COM. Задачи приходят по каналу; do ждёт ответа не дольше
// uiaCallTimeout. Пока задача, не уложившаяся в срок, не вернулась, новые получают errUIABusy:
// зависший Brave задерживает только чтение домена, а не опрос стража.
type uiaThread struct {
	jobs    chan func(uia)
	initErr error
	mu      sync.Mutex
	busy    bool
}

func newUIAThread() *uiaThread {
	t := &uiaThread{jobs: make(chan func(uia))}
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED)
		if err != nil && !errors.Is(err, syscall.Errno(1)) { // S_FALSE: уже инициализирован
			t.initErr = fmt.Errorf("UIA CoInitializeEx: %w", err)
			close(ready)
			return
		}
		defer windows.CoUninitialize()
		var auto comObj
		hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidCUIAutomation)), 0,
			uintptr(clsctxInprocServer), uintptr(unsafe.Pointer(&iidIUIAutomation)), uintptr(unsafe.Pointer(&auto)))
		if hr != 0 || auto == nil {
			t.initErr = hresultErr("CoCreateInstance", hr)
			close(ready)
			return
		}
		defer release(auto)
		close(ready)
		u := uia{auto: auto}
		for job := range t.jobs {
			job(u)
		}
	}()
	<-ready
	return t
}

// do выполняет f в потоке COM. Ошибка инициализации, занятый поток или нет ответа за
// uiaCallTimeout — ошибка без выполнения (или с выполнением позже, без ожидания).
func (t *uiaThread) do(f func(uia) error) error {
	if t.initErr != nil {
		return t.initErr
	}
	t.mu.Lock()
	if t.busy {
		t.mu.Unlock()
		return errUIABusy
	}
	t.busy = true
	t.mu.Unlock()
	done := make(chan error, 1)
	job := func(u uia) {
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("UIA: паника %v", r)
				}
			}()
			err = f(u)
		}()
		t.mu.Lock()
		t.busy = false
		t.mu.Unlock()
		done <- err
	}
	select {
	case t.jobs <- job:
	case <-time.After(uiaCallTimeout):
		t.mu.Lock()
		t.busy = false
		t.mu.Unlock()
		return errUIATimeout
	}
	select {
	case err := <-done:
		return err
	case <-time.After(uiaCallTimeout):
		return errUIATimeout
	}
}

// braveReader читает поле адреса окна Brave. Элемент поля адреса (или корневой Document окна без
// поля адреса) кэшируется для одного окна: поиск идёт только при смене окна или после ошибки.
// Методы — из любой горутины.
type braveReader struct {
	th *uiaThread

	// Поля ниже — только в потоке COM.
	hwnd    windows.HWND
	el      comObj
	source  string
	match   string       // по какому признаку найден элемент: braveMatch*
	foundAt time.Time    // когда найден (источник document перепроверяется, см. Read)
	woken   windows.HWND // окну уже послан WM_GETOBJECT (wakeAccessibility)
}

// Признаки, по которым найден элемент (Describe, проба).
const (
	braveMatchClass     = "ClassName (известное имя поля адреса)"
	braveMatchClassWalk = "ClassName …OmniboxViewViews (обход дерева)"
	braveMatchEdit      = "первый Edit с ValuePattern вне Document"
	braveMatchDocument  = "Document"
)

// braveDocumentRecheck — как часто окно, где нашёлся только Document, снова ищет поле адреса:
// Chromium строит дерево доступности лениво, поле адреса может появиться позже документа.
const braveDocumentRecheck = 5 * time.Second

var (
	procSendMessageTimeout = user32.NewProc("SendMessageTimeoutW")
)

const (
	wmGetObject       = 0x003D
	objidClient       = ^uintptr(3)  // OBJID_CLIENT = -4
	uiaRootObjectID   = ^uintptr(24) // UiaRootObjectId = -25
	smtoAbortIfHung   = 0x0002
	wakeTimeoutMs     = 200
	editSearchDepth   = 12
	editSearchMaxNode = 400
)

// wakeAccessibility просит окно Chromium включить дерево доступности: WM_GETOBJECT с OBJID_CLIENT
// (MSAA) и UiaRootObjectId (UIA). Chromium включает доступность лениво, по первому такому запросу.
// Ответ не нужен; зависшее окно не держит дольше wakeTimeoutMs.
func wakeAccessibility(hwnd windows.HWND) {
	var res uintptr
	for _, id := range []uintptr{objidClient, uiaRootObjectID} {
		procSendMessageTimeout.Call(uintptr(hwnd), wmGetObject, 0, id, smtoAbortIfHung, wakeTimeoutMs, uintptr(unsafe.Pointer(&res)))
	}
}

func newBraveReader() *braveReader { return &braveReader{th: newUIAThread()} }

// forget освобождает кэш (поток COM).
func (r *braveReader) forget() {
	release(r.el)
	r.el, r.hwnd, r.source, r.match = nil, 0, "", ""
}

// locate находит в окне hwnd поле адреса: сначала по известным ClassName (omniboxClassNames), затем
// обходом — ClassName, оканчивающийся на OmniboxViewViews, иначе первый
// Edit с ValuePattern вне Document (обход по детям, не глубже editSearchDepth); нет поля адреса —
// корневой Document окна (сайт-приложение, всплывающее окно). Окну один раз посылается
// WM_GETOBJECT (wakeAccessibility). Поток COM.
func (r *braveReader) locate(u uia, hwnd windows.HWND) error {
	if r.woken != hwnd {
		wakeAccessibility(hwnd)
		r.woken = hwnd
	}
	root, err := u.fromHandle(hwnd)
	if err != nil {
		return err
	}
	defer release(root)
	set := func(el comObj, source, match string) {
		r.hwnd, r.el, r.source, r.match, r.foundAt = hwnd, el, source, match, time.Now()
	}
	for _, name := range omniboxClassNames {
		cond, err := u.strCond(uiaClassNamePropertyID, name)
		if err != nil {
			return err
		}
		el, err := findFirst(root, treeScopeDescendants, cond)
		release(cond)
		if err == nil {
			set(el, braveSourceOmnibox, braveMatchClass+" "+name)
			return nil
		}
		if !errors.Is(err, errUIANotFound) {
			return err
		}
	}
	// Обход вне Document: элемент с классом …OmniboxViewViews; нет — первый Edit с ValuePattern.
	var byClass, edit comObj
	_ = walkTree(u, root, editSearchDepth, editSearchMaxNode, func(el comObj, _ int) (bool, bool) {
		if cls, err := elemClassName(el); err == nil && isOmniboxClass(cls) {
			byClass = el
			return true, true
		}
		if edit != nil {
			return false, false
		}
		ct, err := elemControlType(el)
		if err != nil || ct != uiaEditControlTypeID {
			return false, false
		}
		if ok, err := elemBool(el, uiaIsValuePatternAvailablePropID); err != nil || !ok {
			return false, false
		}
		edit = el
		return true, false
	})
	if byClass != nil {
		release(edit)
		set(byClass, braveSourceOmnibox, braveMatchClassWalk)
		return nil
	}
	if edit != nil {
		set(edit, braveSourceOmnibox, braveMatchEdit)
		return nil
	}
	dcond, err := u.intCond(uiaControlTypePropertyID, uiaDocumentControlTypeID)
	if err != nil {
		return err
	}
	el, err := findFirst(root, treeScopeDescendants, dcond)
	release(dcond)
	if err != nil {
		return err
	}
	set(el, braveSourceDocument, braveMatchDocument)
	return nil
}

// ensure — элемент окна hwnd в кэше: смена окна или пустой кэш — поиск; найден только Document —
// поиск поля адреса повторяется раз в braveDocumentRecheck. Поток COM.
func (r *braveReader) ensure(u uia, hwnd windows.HWND) error {
	if r.hwnd == hwnd && r.el != nil && !(r.source == braveSourceDocument && time.Since(r.foundAt) >= braveDocumentRecheck) {
		return nil
	}
	r.forget()
	return r.locate(u, hwnd)
}

// Read читает текст поля адреса окна hwnd. focused — поле адреса в фокусе клавиатуры (ребёнок
// печатает). source — omnibox, document (окно без поля адреса: адрес документа) или «» при ошибке.
// Ошибка — кэш сброшен, текст «» (PL-9). Итог задача пишет в свою переменную res: после
// errUIATimeout вызывающий её не читает, а поздняя задача не трогает его переменные.
func (r *braveReader) Read(hwnd windows.HWND) (text string, focused bool, source string, err error) {
	type result struct {
		text, source string
		focused      bool
	}
	res := &result{}
	err = r.th.do(func(u uia) error {
		if err := r.ensure(u, hwnd); err != nil {
			return err
		}
		v, err := elemValue(r.el)
		if err != nil {
			r.forget()
			return err
		}
		if r.source == braveSourceOmnibox {
			f, err := elemBool(r.el, uiaHasKeyboardFocusPropertyID)
			if err != nil {
				r.forget()
				return err
			}
			res.focused = f
		}
		res.text, res.source = v, r.source
		return nil
	})
	if err != nil {
		return "", false, "", err
	}
	return res.text, res.focused, res.source, nil
}

// Describe — ClassName, ControlType найденного для hwnd элемента и признак, по которому он найден
// (для пробы и журнала работы).
func (r *braveReader) Describe(hwnd windows.HWND) (string, error) {
	res := new(string)
	err := r.th.do(func(u uia) error {
		if err := r.ensure(u, hwnd); err != nil {
			return err
		}
		class, err := elemClassName(r.el)
		if err != nil {
			return err
		}
		ct, err := elemControlType(r.el)
		if err != nil {
			return err
		}
		*res = fmt.Sprintf("ClassName %q, ControlType %d, найден по признаку «%s»", class, ct, r.match)
		return nil
	})
	if err != nil {
		return "", err
	}
	return *res, nil
}

// errNoCloseButton — у выбранной вкладки не нашлось кнопки закрытия.
var errNoCloseButton = errors.New("UIA: у выбранной вкладки нет кнопки закрытия")

// closeTabButton закрывает выбранную вкладку окна Brave hwnd нажатием её кнопки закрытия через UIA
// (InvokePattern): запасной путь RM4-2, когда Ctrl+W не сработал (закреплённая вкладка). Ищется
// TabItem с SelectionItem.IsSelected в рамке окна, у него — дочерняя кнопка.
func closeTabButton(r *braveReader, hwnd windows.HWND) error {
	return r.th.do(func(u uia) error {
		root, err := u.fromHandle(hwnd)
		if err != nil {
			return err
		}
		defer release(root)
		tab, err := selectedTab(u, root)
		if err != nil {
			return err
		}
		defer release(tab)
		bcond, err := u.intCond(uiaControlTypePropertyID, uiaButtonControlTypeID)
		if err != nil {
			return err
		}
		btn, err := findFirst(tab, treeScopeDescendants, bcond)
		release(bcond)
		if errors.Is(err, errUIANotFound) {
			return errNoCloseButton
		}
		if err != nil {
			return err
		}
		defer release(btn)
		return elemInvoke(btn)
	})
}

// selectedTab — выбранная вкладка (TabItem и SelectionItem.IsSelected) в окне root.
func selectedTab(u uia, root comObj) (comObj, error) {
	tcond, err := u.intCond(uiaControlTypePropertyID, uiaTabItemControlTypeID)
	if err != nil {
		return nil, err
	}
	defer release(tcond)
	scond, err := u.boolCond(uiaSelectionItemIsSelectedPropID, true)
	if err != nil {
		return nil, err
	}
	defer release(scond)
	both, err := u.andCond(tcond, scond)
	if err != nil {
		return nil, err
	}
	defer release(both)
	return findFirst(root, treeScopeDescendants, both)
}
