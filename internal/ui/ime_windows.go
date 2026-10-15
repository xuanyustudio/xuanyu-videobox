//go:build fyne_gui && windows

package ui

import (
	"reflect"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/theme"
)

const (
	wmIMEStartComposition = 0x010D
	wmIMEComposition      = 0x010F
	wmIMENotify           = 0x0282

	cfsPoint         = 0x0002
	cfsForcePosition = 0x0020
	cfsCandidatePos  = 0x0040
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	imm32  = syscall.NewLazyDLL("imm32.dll")

	procSetWindowLongPtr        = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProc          = user32.NewProc("CallWindowProcW")
	procImmGetContext           = imm32.NewProc("ImmGetContext")
	procImmReleaseContext       = imm32.NewProc("ImmReleaseContext")
	procImmSetCompositionWindow = imm32.NewProc("ImmSetCompositionWindow")
	procImmSetCandidateWindow   = imm32.NewProc("ImmSetCandidateWindow")
	procImmDisableTextFrame     = imm32.NewProc("ImmDisableTextFrameService")

	imeCallback = syscall.NewCallback(imeWndProc)

	imeMu       sync.Mutex
	imeHWND     uintptr
	oldWndProc  uintptr
	imeCaretX   int32
	imeCaretY   int32
	imeCaretH   int32
	imeSubclass atomic.Bool
	imeApplying atomic.Bool
)

type imePoint struct {
	x, y int32
}

type imeRect struct {
	left, top, right, bottom int32
}

type compositionForm struct {
	style uint32
	pt    imePoint
	area  imeRect
}

type candidateForm struct {
	index uint32
	style uint32
	pt    imePoint
	area  imeRect
}

func init() {
	// TSF on a custom OpenGL window ignores caret coordinates and parks the
	// candidate list at the screen origin. Legacy IMM follows the point we set.
	_, _, _ = procImmDisableTextFrame.Call(0)
}

// trackIME tells the Windows IME where the focused entry caret is, so the
// candidate window does not stay at the screen origin.
func trackIME(win fyne.Window) {
	go func() {
		ticker := time.NewTicker(32 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			fyne.Do(func() {
				ensureIMESubclass(win)
				updateIMECaret(win)
			})
		}
	}()
}

func ensureIMESubclass(win fyne.Window) {
	if imeSubclass.Load() || win == nil {
		return
	}
	nw, ok := win.(driver.NativeWindow)
	if !ok {
		return
	}
	nw.RunNative(func(ctx any) {
		wctx, ok := ctx.(driver.WindowsWindowContext)
		if !ok || wctx.HWND == 0 || imeSubclass.Load() {
			return
		}
		prev, _, _ := procSetWindowLongPtr.Call(wctx.HWND, ^uintptr(3), imeCallback)
		if prev == 0 {
			return
		}
		imeMu.Lock()
		imeHWND = wctx.HWND
		oldWndProc = prev
		imeMu.Unlock()
		imeSubclass.Store(true)
	})
}

func imeWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	defer func() { _ = recover() }()
	imeMu.Lock()
	prev := oldWndProc
	imeMu.Unlock()
	var ret uintptr
	if prev != 0 {
		ret, _, _ = procCallWindowProc.Call(prev, hwnd, msg, wparam, lparam)
	}
	switch msg {
	case wmIMEStartComposition, wmIMEComposition, wmIMENotify:
		applyIME(hwnd)
	}
	return ret
}

func updateIMECaret(win fyne.Window) {
	if win == nil || win.Canvas() == nil {
		return
	}
	focused := win.Canvas().Focused()
	target, ok := focused.(interface {
		fyne.CanvasObject
		CursorPosition() fyne.Position
	})
	if !ok || target == nil {
		return
	}
	cur := target.CursorPosition()
	scroll := entryScrollOffset(focused)
	size := target.Size()
	line := fyne.CurrentApp().Settings().Theme().Size(theme.SizeNameText)
	localX, localY := visibleCaret(cur.X, cur.Y, scroll.X, scroll.Y, size.Width, size.Height, line)
	abs := fyne.CurrentApp().Driver().AbsolutePositionForObject(target)
	px, py := win.Canvas().PixelCoordinateForPosition(abs.Add(fyne.NewPos(localX, localY)))
	_, lh := win.Canvas().PixelCoordinateForPosition(fyne.NewPos(0, line))
	if lh < 16 {
		lh = 16
	}
	imeMu.Lock()
	imeCaretX = int32(px)
	imeCaretY = int32(py)
	imeCaretH = int32(lh)
	hwnd := imeHWND
	imeMu.Unlock()
	if hwnd != 0 {
		applyIME(hwnd)
	}
}

func applyIME(hwnd uintptr) {
	if hwnd == 0 || !imeApplying.CompareAndSwap(false, true) {
		return
	}
	defer imeApplying.Store(false)

	imeMu.Lock()
	x, y, h := imeCaretX, imeCaretY, imeCaretH
	imeMu.Unlock()
	if h < 16 {
		h = 16
	}

	himc, _, _ := procImmGetContext.Call(hwnd)
	if himc == 0 {
		return
	}
	defer procImmReleaseContext.Call(hwnd, himc)

	comp := compositionForm{
		style: cfsPoint | cfsForcePosition,
		pt:    imePoint{x: x, y: y},
		area:  imeRect{left: x, top: y, right: x + 2, bottom: y + h},
	}
	_, _, _ = procImmSetCompositionWindow.Call(himc, uintptr(unsafe.Pointer(&comp)))

	cand := candidateForm{
		index: 0,
		style: cfsCandidatePos,
		pt:    imePoint{x: x, y: y + h},
		area:  imeRect{left: x, top: y, right: x + 8, bottom: y + h},
	}
	_, _, _ = procImmSetCandidateWindow.Call(himc, uintptr(unsafe.Pointer(&cand)))
}

func entryScrollOffset(focused fyne.Focusable) fyne.Position {
	rv := reflect.ValueOf(focused)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fyne.NewPos(0, 0)
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return fyne.NewPos(0, 0)
	}
	if off, ok := scrollOffsetOf(elem); ok {
		return off
	}
	if emb := elem.FieldByName("Entry"); emb.IsValid() && emb.Kind() == reflect.Struct {
		if off, ok := scrollOffsetOf(emb); ok {
			return off
		}
	}
	return fyne.NewPos(0, 0)
}

func scrollOffsetOf(elem reflect.Value) (off fyne.Position, ok bool) {
	defer func() { _ = recover() }()
	field := elem.FieldByName("scroll")
	if !field.IsValid() || field.Kind() != reflect.Pointer {
		return fyne.Position{}, false
	}
	ptr := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	if ptr.IsNil() {
		return fyne.Position{}, true
	}
	offset := ptr.Elem().FieldByName("Offset")
	if !offset.IsValid() {
		return fyne.Position{}, false
	}
	return fyne.NewPos(float32(offset.FieldByName("X").Float()), float32(offset.FieldByName("Y").Float())), true
}
