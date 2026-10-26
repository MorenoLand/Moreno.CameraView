package main

import (
	"image"
	"image/color"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"moreno/blinkview/flyout"
)

type pinWins struct {
	lv   *live
	mu   sync.Mutex
	open map[string]chan struct{}
}

func newPinWins(lv *live) *pinWins { return &pinWins{lv: lv, open: map[string]chan struct{}{}} }

func (m *pinWins) Pinned(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.open[id]
	return ok
}

func (m *pinWins) Toggle(id string) {
	m.mu.Lock()
	stop, ok := m.open[id]
	m.mu.Unlock()
	if ok {
		close(stop)
		return
	}
	m.start(id, true)
}

func (m *pinWins) Restore() {
	for _, id := range m.lv.Settings().Pinned {
		m.start(id, false)
	}
}

func (m *pinWins) remember(id string, on bool) {
	s := m.lv.Settings()
	s.Pinned = slices.DeleteFunc(slices.Clone(s.Pinned), func(x string) bool { return x == id })
	if on {
		s.Pinned = append(s.Pinned, id)
	}
	m.lv.SaveSettings(s)
}

func (m *pinWins) start(id string, save bool) {
	stop := make(chan struct{})
	m.mu.Lock()
	if _, ok := m.open[id]; ok {
		m.mu.Unlock()
		return
	}
	m.open[id] = stop
	m.mu.Unlock()
	if save {
		m.remember(id, true)
	}
	go m.run(id, stop)
}

func (m *pinWins) title(id string) string {
	for _, c := range m.lv.Cameras() {
		if c.ID == id {
			return c.Name
		}
	}
	return "Camera"
}

func (m *pinWins) run(id string, stop chan struct{}) {
	userClosed := false
	done := make(chan struct{})
	var hwnd atomic.Uintptr
	var hovered atomic.Bool
	defer func() {
		close(done)
		m.mu.Lock()
		delete(m.open, id)
		m.mu.Unlock()
		if userClosed {
			m.remember(id, false)
		}
	}()
	w := new(app.Window)
	w.Option(app.Title(m.title(id)), app.Size(unit.Dp(384), unit.Dp(216)), app.MinSize(unit.Dp(220), unit.Dp(124)), app.Decorated(false))
	go func() {
		ticker := time.NewTicker(40 * time.Millisecond)
		defer ticker.Stop()
		token := ""
		defer func() {
			if token != "" {
				m.lv.CloseWatch(token)
			}
		}()
		var rev int
		var hov bool
		var note string
		var active bool
		var retry time.Time
		for {
			select {
			case <-stop:
				w.Perform(system.ActionClose)
				return
			case <-done:
				return
			case <-ticker.C:
			}
			_, r, n, a := m.lv.WatchStateFor(id)
			h := hwnd.Load() != 0 && flyout.CursorInside(hwnd.Load())
			if r != rev || n != note || a != active || h != hov {
				rev, note, active, hov = r, n, a, h
				hovered.Store(h)
				w.Invalidate()
			}
			if !a && time.Now().After(retry) {
				retry = time.Now().Add(5 * time.Second)
				if token != "" {
					m.lv.CloseWatch(token)
					token = ""
				}
				if t, err := m.lv.OpenWatch(id); err == nil {
					token = t
				}
			}
		}
	}()
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	var ops op.Ops
	var cache image.Image
	cacheRev := -1
	var imgOp paint.ImageOp
	var hoverA float32
	var closeTag, gripTag, muteTag, panTag int
	zoom, panX, panY := float32(1), float32(0), float32(0)
	var lastP f32.Point
	var lastClick time.Time
	var moving bool
	var o [4]int32
	moveCh := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-moveCh:
				if h := hwnd.Load(); h != 0 {
					flyout.DragTo(h, o[0], o[1], o[2], o[3])
				}
			}
		}
	}()
	ratio := 9.0 / 16
	for {
		switch e := w.Event().(type) {
		case app.Win32ViewEvent:
			hwnd.Store(e.HWND)
			if e.HWND != 0 {
				go flyout.Glass(e.HWND)
			}
		case app.DestroyEvent:
			select {
			case <-stop:
			default:
				userClosed = true
			}
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			s := gtx.Metric.PxPerDp
			W, H := float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)
			paint.FillShape(gtx.Ops, color.NRGBA{R: 12, G: 15, B: 28, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			img, rev, note, active := m.lv.WatchStateFor(id)
			if img != nil {
				if rev != cacheRev || img != cache {
					imgOp, cache, cacheRev = paint.NewImageOp(img), img, rev
				}
				sz := imgOp.Size()
				ratio = float64(sz.Y) / float64(sz.X)
				k := min(W/float32(sz.X), H/float32(sz.Y)) * zoom
				dw, dh := float32(sz.X)*k, float32(sz.Y)*k
				panX = clampPan(panX, dw, W)
				panY = clampPan(panY, dh, H)
				ox, oy := (W-dw)/2+panX, (H-dh)/2+panY
				vc := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
				st := op.Offset(image.Pt(int(ox), int(oy))).Push(gtx.Ops)
				sc := op.Affine(f32Scale(k)).Push(gtx.Ops)
				imgOp.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)
				sc.Pop()
				st.Pop()
				vc.Pop()
			}
			if img == nil || !active {
				msg := note
				if msg == "" {
					msg = "Connecting…"
				}
				layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					l := material.Body1(th, msg)
					l.Color = color.NRGBA{R: 220, G: 224, B: 240, A: 255}
					return l.Layout(gtx)
				})
			}
			// hover fade
			target := float32(0)
			if hovered.Load() {
				target = 1
			}
			hoverA += (target - hoverA) * 0.35
			if d := target - hoverA; d > 0.01 || d < -0.01 {
				w.Invalidate()
			} else {
				hoverA = target
			}
			// window drag over everything
			bar := int(46 * s)
			dr := image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, bar)}
			ds := clip.Rect(dr).Push(gtx.Ops)
			system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
			ds.Pop()
			// wheel zoom, drag pan, double-click reset
			ps := clip.Rect{Min: image.Pt(0, bar), Max: gtx.Constraints.Max}.Push(gtx.Ops)
			event.Op(gtx.Ops, &panTag)
			ps.Pop()
			for {
				ev, ok := gtx.Event(pointer.Filter{Target: &panTag, Kinds: pointer.Scroll | pointer.Press | pointer.Drag, ScrollY: pointer.ScrollRange{Min: -200, Max: 200}})
				if !ok {
					break
				}
				pe, ok := ev.(pointer.Event)
				if !ok {
					continue
				}
				switch pe.Kind {
				case pointer.Scroll:
					nz := min(max(zoom*float32(math.Exp(float64(-pe.Scroll.Y)*0.0025)), 1), 8)
					if nz != zoom {
						cx, cy := pe.Position.X-W/2, pe.Position.Y-H/2
						k := nz / zoom
						panX, panY = cx-(cx-panX)*k, cy-(cy-panY)*k
						zoom = nz
						if zoom <= 1.001 {
							zoom, panX, panY = 1, 0, 0
						}
					}
				case pointer.Press:
					lastP = pe.Position
					moving = zoom <= 1.001 || pe.Modifiers&key.ModCtrl != 0
					if h := hwnd.Load(); moving && h != 0 {
						o[0], o[1], o[2], o[3] = flyout.DragOrigin(h)
					}
					if time.Since(lastClick) < 350*time.Millisecond {
						zoom, panX, panY = 1, 0, 0
					}
					lastClick = time.Now()
				case pointer.Drag:
					if moving {
						select {
						case moveCh <- struct{}{}:
						default:
						}
					} else {
						panX += pe.Position.X - lastP.X
						panY += pe.Position.Y - lastP.Y
					}
					lastP = pe.Position
				}
				w.Invalidate()
			}
			if hoverA > 0.01 {
				a := hoverA
				top := int(46 * s)
				ts := clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, top)}.Push(gtx.Ops)
				paint.LinearGradientOp{Stop1: f32.Pt(0, 0), Stop2: f32.Pt(0, float32(top)), Color1: color.NRGBA{A: uint8(170 * a)}, Color2: color.NRGBA{}}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)
				ts.Pop()
				// close chip
				cs := int(26 * s)
				mx := gtx.Constraints.Max.X - 2*cs - int(16*s)
				my := int(10 * s)
				mr := image.Rect(mx, my, mx+cs, my+cs)
				muted := m.lv.Muted()
				mb := uint8(34 * a)
				if !muted {
					mb = uint8(70 * a)
				}
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: mb}, clip.UniformRRect(mr, cs/2).Op(gtx.Ops))
				{
					mc := func(x, y float32) f32.Point {
						return f32.Pt(float32(mx)+float32(cs)/2+x*s, float32(my)+float32(cs)/2+y*s)
					}
					var sp clip.Path
					sp.Begin(gtx.Ops)
					sp.MoveTo(mc(-5, -2))
					sp.LineTo(mc(-2, -2))
					sp.LineTo(mc(2, -5))
					sp.LineTo(mc(2, 5))
					sp.LineTo(mc(-2, 2))
					sp.LineTo(mc(-5, 2))
					sp.Close()
					wh := color.NRGBA{R: 255, G: 255, B: 255, A: uint8(240 * a)}
					paint.FillShape(gtx.Ops, wh, clip.Outline{Path: sp.End()}.Op())
					var wv clip.Path
					wv.Begin(gtx.Ops)
					if muted {
						wv.MoveTo(mc(4, -3))
						wv.LineTo(mc(8, 3))
						wv.MoveTo(mc(8, -3))
						wv.LineTo(mc(4, 3))
					} else {
						wv.MoveTo(mc(4.5, -3))
						wv.LineTo(mc(6.5, 0))
						wv.LineTo(mc(4.5, 3))
					}
					paint.FillShape(gtx.Ops, wh, clip.Stroke{Path: wv.End(), Width: 1.5 * s}.Op())
				}
				mst := clip.Rect(mr).Push(gtx.Ops)
				pointer.CursorPointer.Add(gtx.Ops)
				event.Op(gtx.Ops, &muteTag)
				mst.Pop()
				for {
					ev, ok := gtx.Event(pointer.Filter{Target: &muteTag, Kinds: pointer.Release})
					if !ok {
						break
					}
					if _, ok := ev.(pointer.Event); ok {
						m.lv.SetMuted(!muted)
					}
				}
				cx, cy := gtx.Constraints.Max.X-cs-int(10*s), int(10*s)
				cr := image.Rect(cx, cy, cx+cs, cy+cs)
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: uint8(34 * a)}, clip.UniformRRect(cr, cs/2).Op(gtx.Ops))
				p := func(x, y float32) f32.Point {
					return f32.Pt(float32(cx)+float32(cs)/2+x*s, float32(cy)+float32(cs)/2+y*s)
				}
				var path clip.Path
				path.Begin(gtx.Ops)
				path.MoveTo(p(-4, -4))
				path.LineTo(p(4, 4))
				path.MoveTo(p(4, -4))
				path.LineTo(p(-4, 4))
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: uint8(240 * a)}, clip.Stroke{Path: path.End(), Width: 1.6 * s}.Op())
				cst := cr
				cstack := clip.Rect(cst).Push(gtx.Ops)
				pointer.CursorPointer.Add(gtx.Ops)
				event.Op(gtx.Ops, &closeTag)
				cstack.Pop()
				for {
					ev, ok := gtx.Event(pointer.Filter{Target: &closeTag, Kinds: pointer.Release})
					if !ok {
						break
					}
					if _, ok := ev.(pointer.Event); ok {
						go w.Perform(system.ActionClose)
					}
				}
			}
			// resize grip (bottom-right), always active
			gs := int(22 * s)
			gr := image.Rect(gtx.Constraints.Max.X-gs, gtx.Constraints.Max.Y-gs, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
			if hoverA > 0.01 {
				var path clip.Path
				path.Begin(gtx.Ops)
				for _, o := range []float32{5, 10} {
					path.MoveTo(f32.Pt(float32(gr.Max.X)-3*s, float32(gr.Max.Y)-o*s-3*s))
					path.LineTo(f32.Pt(float32(gr.Max.X)-o*s-3*s, float32(gr.Max.Y)-3*s))
				}
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: uint8(150 * hoverA)}, clip.Stroke{Path: path.End(), Width: 1.5 * s}.Op())
			}
			gstack := clip.Rect(gr).Push(gtx.Ops)
			pointer.CursorSouthEastResize.Add(gtx.Ops)
			event.Op(gtx.Ops, &gripTag)
			gstack.Pop()
			for {
				ev, ok := gtx.Event(pointer.Filter{Target: &gripTag, Kinds: pointer.Press | pointer.Drag})
				if !ok {
					break
				}
				if _, ok := ev.(pointer.Event); ok {
					if h := hwnd.Load(); h != 0 {
						minW := int32(220 * s)
						go flyout.ResizeToCursor(h, minW, ratio)
					}
				}
			}
			// glass edge
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: uint8(26 + 30*hoverA)}, clip.Stroke{Path: clip.UniformRRect(image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y), int(10*s)).Path(gtx.Ops), Width: 1.2 * s}.Op())
			e.Frame(gtx.Ops)
		}
	}
}

func f32Scale(k float32) f32.Affine2D { return f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(k, k)) }

func clampPan(p, content, view float32) float32 {
	lim := max(content-view, 0) / 2
	return min(max(p, -lim), lim)
}
