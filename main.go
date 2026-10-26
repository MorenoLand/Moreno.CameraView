package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/energye/systray"

	shots "moreno/blinkview/dev/shots"
	"moreno/blinkview/flyout"
	"moreno/blinkview/panel"
)

const panelW = 408

func main() {
	shot := flag.String("shot", "", "render demo screens to this folder and exit")
	show := flag.Bool("show", false, "open the panel when BlinkView starts")
	flag.Parse()
	if len(os.Args) > 1 && forwardCallback(os.Args[1]) {
		return
	}
	registerScheme()
	debug.SetGCPercent(40)
	if *shot != "" {
		shots.Run(*shot)
		return
	}
	if !flyout.Single() {
		return
	}
	var (
		wp               atomic.Pointer[app.Window]
		hwnd             atomic.Uintptr
		focused, closing atomic.Bool
		shownAt, blurAt  atomic.Int64
		lastCameras      atomic.Int64
		hNow, scale      atomic.Int32
		sizePx           atomic.Int32
		trayReady        atomic.Bool
	)
	lastCameras.Store(-1)
	scale.Store(1000)
	sizePx.Store(-1)
	sizeWake := make(chan struct{}, 1)
	go func() {
		for range sizeWake {
			for {
				px := sizePx.Swap(-1)
				if px < 0 {
					break
				}
				if h := hwnd.Load(); h != 0 {
					flyout.SetHeight(h, px)
				}
			}
		}
	}()
	setHeight := func(dp float32) {
		hNow.Store(int32(dp * 100))
		sizePx.Store(int32(dp*float32(scale.Load())/1000 + 0.5))
		select {
		case sizeWake <- struct{}{}:
		default:
		}
	}
	var lv *live
	invalidate := func() {
		if w := wp.Load(); w != nil {
			w.Invalidate()
		}
		if lv == nil || !trayReady.Load() {
			return
		}
		cams := lv.Cameras()
		on := 0
		for _, c := range cams {
			if c.Online {
				on++
			}
		}
		sig := int64(on)<<32 | int64(len(cams))
		if sig != lastCameras.Swap(sig) {
			systray.SetIcon(flyout.Icon(on > 0))
			systray.SetTooltip(fmt.Sprintf("Blink cameras: %d of %d online", on, len(cams)))
		}
	}
	hide := func() {
		h := hwnd.Load()
		if h == 0 || !flyout.Visible(h) || !closing.CompareAndSwap(false, true) {
			return
		}
		done := flyout.Close(h, 220*time.Millisecond, 10)
		closing.Store(false)
		if done {
			go func() {
				time.Sleep(600 * time.Millisecond)
				flyout.TrimMemory()
			}()
		}
	}
	open := func() {
		h := hwnd.Load()
		if h == 0 {
			return
		}
		closing.Store(false)
		focused.Store(false)
		shownAt.Store(time.Now().UnixNano())
		flyout.Show(h)
		invalidate()
	}
	toggle := func() {
		h := hwnd.Load()
		if h != 0 && flyout.Visible(h) {
			go hide()
			return
		}
		if time.Since(time.Unix(0, blurAt.Load())) < 450*time.Millisecond {
			return
		}
		open()
	}
	lv = newLive(invalidate)
	go func() {
		runtime.LockOSThread()
		systray.Run(func() {
			systray.SetIcon(flyout.Icon(false))
			systray.SetTooltip("Blink cameras")
			systray.SetOnClick(func(systray.IMenu) { go toggle() })
			systray.SetOnRClick(func(m systray.IMenu) { _ = m.ShowMenu() })
			systray.AddMenuItem("Open", "").Click(func() { go open() })
			systray.AddMenuItem("Refresh cameras", "").Click(func() { go lv.Refresh() })
			systray.AddMenuItem("Web panel", "").Click(func() { go lv.OpenWeb() })
			systray.AddSeparator()
			systray.AddMenuItem("Quit", "").Click(func() { lv.Quit() })
			trayReady.Store(true)
			invalidate()
			go func() {
				time.Sleep(800 * time.Millisecond)
				if !lv.LoggedIn() || *show {
					open()
				}
			}()
		}, nil)
	}()
	go func() {
		_, wh := flyout.WorkAreaDp()
		h := min(float32(1100), wh-24)
		if h < 420 {
			h = max(360, wh-16)
		}
		pnl := panel.New(lv)
		pnl.MaxH = h
		hNow.Store(int32(h * 100))
		w := new(app.Window)
		w.Option(app.Title("Blink cameras"), app.Size(unit.Dp(panelW), unit.Dp(h)), app.MinSize(unit.Dp(panelW), unit.Dp(200)), app.MaxSize(unit.Dp(panelW), unit.Dp(h)), app.Decorated(false))
		wp.Store(w)
		var ops op.Ops
		last := time.Now()
		for {
			switch e := w.Event().(type) {
			case app.Win32ViewEvent:
				hwnd.Store(e.HWND)
				if e.HWND != 0 {
					go flyout.Prepare(e.HWND)
				}
			case app.ConfigEvent:
				if e.Config.Focused {
					focused.Store(true)
				} else if focused.Swap(false) && time.Since(time.Unix(0, shownAt.Load())) > 300*time.Millisecond {
					if h := hwnd.Load(); h != 0 && flyout.Visible(h) {
						blurAt.Store(time.Now().UnixNano())
						go hide()
					}
				}
			case app.DestroyEvent:
				lv.Quit()
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				pnl.Layout(gtx)
				e.Frame(gtx.Ops)
				scale.Store(int32(e.Metric.PxPerDp * 1000))
				now := time.Now()
				dt := math.Min(math.Max(now.Sub(last).Seconds(), 0.001), 0.05)
				last = now
				if want, cur := pnl.WantH, float32(hNow.Load())/100; want > 0 && math.Abs(float64(want-cur)) > 0.4 {
					cur += (want - cur) * float32(1-math.Exp(-dt*16))
					if math.Abs(float64(want-cur)) < 0.4 {
						cur = want
					}
					if hwnd.Load() != 0 {
						setHeight(cur)
					}
					w.Invalidate()
				}
			}
		}
	}()
	app.Main()
}
