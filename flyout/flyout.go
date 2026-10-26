package flyout

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	dwmapi           = windows.NewLazySystemDLL("dwmapi.dll")
	pShowWindow      = user32.NewProc("ShowWindow")
	pSetWindowPos    = user32.NewProc("SetWindowPos")
	pGetWindowRect   = user32.NewProc("GetWindowRect")
	pGetCursorPos    = user32.NewProc("GetCursorPos")
	pMonitorFromPt   = user32.NewProc("MonitorFromPoint")
	pMonitorFromWin  = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfo  = user32.NewProc("GetMonitorInfoW")
	pSetForeground   = user32.NewProc("SetForegroundWindow")
	pIsWindowVisible = user32.NewProc("IsWindowVisible")
	pGetLongPtr      = user32.NewProc("GetWindowLongPtrW")
	pSetLongPtr      = user32.NewProc("SetWindowLongPtrW")
	pGetDpiSystem    = user32.NewProc("GetDpiForSystem")
	pSysParams       = user32.NewProc("SystemParametersInfoW")
	pSetLayered      = user32.NewProc("SetLayeredWindowAttributes")
	pDwmSetAttr      = dwmapi.NewProc("DwmSetWindowAttribute")
	pEmptyWS         = windows.NewLazySystemDLL("kernel32.dll").NewProc("K32EmptyWorkingSet")
)

type rect struct{ L, T, R, B int32 }

type point struct{ X, Y int32 }

type monitorInfo struct {
	Size          uint32
	Monitor, Work rect
	Flags         uint32
}

const (
	gwlExStyle    = ^uintptr(19)
	wsExTool      = 0x80
	wsExApp       = 0x40000
	swHide        = 0
	swpNoSize     = 0x1
	swpNoZOrder   = 0x4
	swpNoActivate = 0x10
	swpShow       = 0x40
	swpAsync      = 0x4000
	wsExLayered   = 0x80000
	lwaAlpha      = 0x2
	margin        = 8
)

var (
	fadeMu  sync.Mutex
	fadeGen atomic.Int64
)

func Single() bool {
	n, _ := windows.UTF16PtrFromString("Local\\BlinkViewPanel")
	_, err := windows.CreateMutex(nil, false, n)
	return err != windows.ERROR_ALREADY_EXISTS
}

func WorkAreaDp() (w, h float32) {
	var r rect
	pSysParams.Call(0x30, 0, uintptr(unsafe.Pointer(&r)), 0)
	dpi, _, _ := pGetDpiSystem.Call()
	if dpi == 0 {
		dpi = 96
	}
	s := float32(dpi) / 96
	return float32(r.R-r.L) / s, float32(r.B-r.T) / s
}

func Prepare(hwnd uintptr) {
	ex, _, _ := pGetLongPtr.Call(hwnd, gwlExStyle)
	pSetLongPtr.Call(hwnd, gwlExStyle, (ex|wsExTool)&^wsExApp)
	round, dark, border := int32(2), int32(1), uint32(0x0040362E)
	pDwmSetAttr.Call(hwnd, 33, uintptr(unsafe.Pointer(&round)), 4)
	pDwmSetAttr.Call(hwnd, 20, uintptr(unsafe.Pointer(&dark)), 4)
	pDwmSetAttr.Call(hwnd, 34, uintptr(unsafe.Pointer(&border)), 4)
	Hide(hwnd)
}

func Hide(hwnd uintptr) { pShowWindow.Call(hwnd, swHide) }

func Visible(hwnd uintptr) bool {
	v, _, _ := pIsWindowVisible.Call(hwnd)
	return v != 0
}

func Target(w, h int32) (int32, int32) {
	var cur point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&cur)))
	m, _, _ := pMonitorFromPt.Call(uintptr(uint32(cur.X))|uintptr(uint32(cur.Y))<<32, 2)
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := pGetMonitorInfo.Call(m, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		pSysParams.Call(0x30, 0, uintptr(unsafe.Pointer(&mi.Work)), 0)
		mi.Monitor = mi.Work
	}
	x := min(max(cur.X-w/2, mi.Work.L+margin), mi.Work.R-w-margin)
	y := mi.Work.B - h - margin
	switch {
	case mi.Work.T > mi.Monitor.T:
		y = mi.Work.T + margin
	case mi.Work.L > mi.Monitor.L:
		x = mi.Work.L + margin
	case mi.Work.R < mi.Monitor.R:
		x = mi.Work.R - w - margin
	}
	return x, y
}

func layered(hwnd uintptr, on bool) {
	ex, _, _ := pGetLongPtr.Call(hwnd, gwlExStyle)
	if on {
		pSetLongPtr.Call(hwnd, gwlExStyle, ex|wsExLayered)
		pSetLayered.Call(hwnd, 0, 255, lwaAlpha)
	} else if ex&wsExLayered != 0 {
		pSetLongPtr.Call(hwnd, gwlExStyle, ex&^wsExLayered)
	}
}

func frame() { time.Sleep(10 * time.Millisecond) }

func Close(hwnd uintptr, dur time.Duration, drop int32) bool {
	gen := fadeGen.Add(1)
	var wr rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
	fadeMu.Lock()
	if fadeGen.Load() != gen {
		fadeMu.Unlock()
		return false
	}
	layered(hwnd, true)
	fadeMu.Unlock()
	frame()
	frame()
	start := time.Now()
	for {
		t := math.Min(float64(time.Since(start))/float64(dur), 1)
		fadeMu.Lock()
		if fadeGen.Load() != gen {
			fadeMu.Unlock()
			return false
		}
		e := t * t * t * (t*(t*6-15) + 10)
		pSetLayered.Call(hwnd, 0, uintptr(255*(1-e)+0.5), lwaAlpha)
		pSetWindowPos.Call(hwnd, 0, uintptr(wr.L), uintptr(float64(wr.T)+float64(drop)*e+0.5), 0, 0, swpNoSize|swpNoActivate|swpNoZOrder)
		if t >= 1 {
			pShowWindow.Call(hwnd, swHide)
			layered(hwnd, false)
			fadeMu.Unlock()
			return true
		}
		fadeMu.Unlock()
		frame()
	}
}

func Show(hwnd uintptr) {
	fadeGen.Add(1)
	fadeMu.Lock()
	layered(hwnd, false)
	fadeMu.Unlock()
	var wr rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
	x, y := Target(wr.R-wr.L, wr.B-wr.T)
	pSetWindowPos.Call(hwnd, ^uintptr(0), uintptr(x), uintptr(y), 0, 0, swpNoSize|swpShow)
	pSetForeground.Call(hwnd)
}

func SetAutostart(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		_ = k.DeleteValue("BlinkView")
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue("BlinkView", `"`+exe+`"`)
}

func Icon(live bool) []byte {
	const n = 32
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	body, lens := color.NRGBA{225, 232, 245, 255}, color.NRGBA{18, 22, 38, 255}
	if live {
		body = color.NRGBA{190, 255, 220, 255}
	}
	ring := color.NRGBA{80, 90, 118, 255}
	if live {
		ring = color.NRGBA{0, 200, 128, 255}
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			switch {
			case x >= 5 && x <= 26 && y >= 9 && y <= 23 && (x > 6 && x < 25 || y > 10 && y < 22):
				img.Set(x, y, body)
			case x >= 11 && x <= 20 && y >= 4 && y <= 9:
				img.Set(x, y, body)
			}
			d := math.Hypot(float64(x)-15.5, float64(y)-16)
			if d <= 6.5 {
				img.Set(x, y, lens)
			}
			if d <= 4 && d >= 2.8 {
				img.Set(x, y, ring)
			}
			if d <= 1.6 {
				img.Set(x, y, ring)
			}
		}
	}
	var pb bytes.Buffer
	_ = png.Encode(&pb, img)
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, []uint16{0, 1, 1})
	out.Write([]byte{n, n, 0, 0})
	_ = binary.Write(&out, binary.LittleEndian, []uint16{1, 32})
	_ = binary.Write(&out, binary.LittleEndian, []uint32{uint32(pb.Len()), 22})
	out.Write(pb.Bytes())
	return out.Bytes()
}

func SetHeight(hwnd uintptr, hpx int32) {
	var wr rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
	m, _, _ := pMonitorFromWin.Call(hwnd, 2)
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := pGetMonitorInfo.Call(m, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return
	}
	y := mi.Work.B - hpx - margin
	if mi.Work.T > mi.Monitor.T {
		y = mi.Work.T + margin
	}
	pSetWindowPos.Call(hwnd, ^uintptr(0), uintptr(wr.L), uintptr(y), uintptr(wr.R-wr.L), uintptr(hpx), swpNoActivate|swpAsync)
}

func TrimMemory() {
	p, _ := windows.GetCurrentProcess()
	pEmptyWS.Call(uintptr(p))
}
