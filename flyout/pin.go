package flyout

import (
	"sync/atomic"
	"unsafe"
)

var (
	free         atomic.Bool
	freeX, freeY atomic.Int32
)

// Glass styles a frameless pinned window: rounded, dark, always on top.
func Glass(hwnd uintptr) {
	ex, _, _ := pGetLongPtr.Call(hwnd, gwlExStyle)
	pSetLongPtr.Call(hwnd, gwlExStyle, (ex|wsExTool)&^wsExApp)
	round, dark, border := int32(2), int32(1), uint32(0x0040362E)
	pDwmSetAttr.Call(hwnd, 33, uintptr(unsafe.Pointer(&round)), 4)
	pDwmSetAttr.Call(hwnd, 20, uintptr(unsafe.Pointer(&dark)), 4)
	pDwmSetAttr.Call(hwnd, 34, uintptr(unsafe.Pointer(&border)), 4)
	Topmost(hwnd)
}

// CursorInside reports whether the mouse is over the window.
func CursorInside(hwnd uintptr) bool {
	var c point
	var r rect
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&c)))
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return c.X >= r.L && c.X < r.R && c.Y >= r.T && c.Y < r.B
}

// ResizeToCursor sizes the window so its bottom-right corner follows the
// mouse, keeping height = width*ratio.
func ResizeToCursor(hwnd uintptr, minW int32, ratio float64) {
	var c point
	var r rect
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&c)))
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	w := max(c.X-r.L, minW)
	h := int32(float64(w)*ratio + 0.5)
	pSetWindowPos.Call(hwnd, 0, 0, 0, uintptr(w), uintptr(h), swpNoMove|swpNoActivate|swpNoZOrder)
}

// DragOrigin returns the mouse and window positions to start a manual move.
func DragOrigin(hwnd uintptr) (cx, cy, wx, wy int32) {
	var c point
	var r rect
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&c)))
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return c.X, c.Y, r.L, r.T
}

// DragTo moves the window by the mouse travel since DragOrigin.
func DragTo(hwnd uintptr, cx, cy, wx, wy int32) {
	var c point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&c)))
	pSetWindowPos.Call(hwnd, ^uintptr(0), uintptr(wx+c.X-cx), uintptr(wy+c.Y-cy), 0, 0, swpNoSize|swpNoActivate)
}

// SetFree makes the tray panel open at x,y instead of beside the tray icon.
func SetFree(on bool, x, y int32) {
	free.Store(on)
	freeX.Store(x)
	freeY.Store(y)
}

func Free() bool { return free.Load() }

// Pos returns the window's top-left corner.
func Pos(hwnd uintptr) (x, y int32) {
	var r rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r.L, r.T
}

func clampToScreen(x, y, w, h int32) (int32, int32) {
	p := point{X: x + w/2, Y: y + h/2}
	m, _, _ := pMonitorFromPt.Call(uintptr(uint32(p.X))|uintptr(uint32(p.Y))<<32, 2)
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := pGetMonitorInfo.Call(m, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return x, y
	}
	return min(max(x, mi.Work.L), max(mi.Work.R-w, mi.Work.L)), min(max(y, mi.Work.T), max(mi.Work.B-h, mi.Work.T))
}

// Reposition moves a visible window to x,y.
func Reposition(hwnd uintptr, x, y int32) {
	pSetWindowPos.Call(hwnd, ^uintptr(0), uintptr(x), uintptr(y), 0, 0, swpNoSize|swpNoActivate)
}

// Anchor is the default spot beside the tray for the window's current size.
func Anchor(hwnd uintptr) (int32, int32) {
	var r rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return Target(r.R-r.L, r.B-r.T)
}
