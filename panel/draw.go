package panel

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const texN = 128

var (
	cWhite     = rgb(240, 242, 250)
	cDim       = rgb(140, 148, 172)
	cFaint     = rgb(100, 108, 136)
	cAmber     = rgb(243, 185, 122)
	cAmberLine = rgb(217, 148, 74)
	cBlue      = rgb(90, 150, 255)
	cGreen     = rgb(60, 220, 110)
	cBgTop     = rgb(36, 28, 70)
	cBgBot     = rgb(8, 11, 28)
)

func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{r, g, b, 255} }

func rgba(r, g, b, a uint8) color.NRGBA { return color.NRGBA{r, g, b, a} }

func alpha(c color.NRGBA, a uint8) color.NRGBA { c.A = a; return c }

func mix(a, b color.NRGBA, t float32) color.NRGBA {
	l := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t) }
	return color.NRGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), l(a.A, b.A)}
}

func clampf(v, lo, hi float32) float32 { return max(lo, min(hi, v)) }

func loadShaper() *text.Shaper {
	var coll []font.FontFace
	for _, f := range []struct {
		file string
		w    font.Weight
	}{{"segoeuil.ttf", font.Light}, {"segoeui.ttf", font.Normal}, {"seguisb.ttf", font.SemiBold}} {
		if b, err := os.ReadFile(filepath.Join(os.Getenv("WINDIR"), "Fonts", f.file)); err == nil {
			if face, err := opentype.Parse(b); err == nil {
				coll = append(coll, font.FontFace{Font: font.Font{Typeface: "Segoe UI", Weight: f.w}, Face: face})
			}
		}
	}
	if len(coll) == 0 {
		coll = gofont.Collection()
	}
	return text.NewShaper(text.NoSystemFonts(), text.WithCollection(coll))
}

func (p *Panel) px(v float32) int { return int(v*p.s + 0.5) }

func (p *Panel) pt(x, y float32) f32.Point { return f32.Pt(x*p.s, y*p.s) }

func (p *Panel) rect(x, y, w, h float32) image.Rectangle {
	return image.Rect(p.px(x), p.px(y), p.px(x+w), p.px(y+h))
}

func (p *Panel) fillRR(x, y, w, h, r float32, c color.NRGBA) {
	paint.FillShape(p.ops, c, clip.UniformRRect(p.rect(x, y, w, h), p.px(r)).Op(p.ops))
}

func (p *Panel) gradRR(x, y, w, h, r float32, c1, c2 color.NRGBA, horiz bool) {
	st := clip.UniformRRect(p.rect(x, y, w, h), p.px(r)).Push(p.ops)
	s2 := p.pt(x, y+h)
	if horiz {
		s2 = p.pt(x+w, y)
	}
	paint.LinearGradientOp{Stop1: p.pt(x, y), Stop2: s2, Color1: c1, Color2: c2}.Add(p.ops)
	paint.PaintOp{}.Add(p.ops)
	st.Pop()
}

func (p *Panel) strokeRR(x, y, w, h, r, width float32, c color.NRGBA) {
	path := clip.UniformRRect(p.rect(x, y, w, h), p.px(r)).Path(p.ops)
	paint.FillShape(p.ops, c, clip.Stroke{Path: path, Width: width * p.s}.Op())
}

func (p *Panel) poly(pts []f32.Point) clip.PathSpec {
	var path clip.Path
	path.Begin(p.ops)
	path.MoveTo(pts[0])
	for _, q := range pts[1:] {
		path.LineTo(q)
	}
	path.Close()
	return path.End()
}

func (p *Panel) fillPoly(pts []f32.Point, c color.NRGBA) {
	paint.FillShape(p.ops, c, clip.Outline{Path: p.poly(pts)}.Op())
}

func (p *Panel) strokePoly(pts []f32.Point, width float32, c color.NRGBA, closed bool) {
	var path clip.Path
	path.Begin(p.ops)
	path.MoveTo(pts[0])
	for _, q := range pts[1:] {
		path.LineTo(q)
	}
	if closed {
		path.Close()
	}
	paint.FillShape(p.ops, c, clip.Stroke{Path: path.End(), Width: width * p.s}.Op())
}

func (p *Panel) line(x1, y1, x2, y2, w float32, c color.NRGBA) {
	p.strokePoly([]f32.Point{p.pt(x1, y1), p.pt(x2, y2)}, w, c, false)
}

func (p *Panel) arc(cx, cy, r, a0, a1 float64) []f32.Point {
	out := make([]f32.Point, 0, 25)
	for i := 0; i <= 24; i++ {
		a := a0 + (a1-a0)*float64(i)/24
		out = append(out, p.pt(float32(cx+r*math.Cos(a)), float32(cy+r*math.Sin(a))))
	}
	return out
}

func (p *Panel) ring(cx, cy, r, w float32, c color.NRGBA) {
	p.strokePoly(p.arc(float64(cx), float64(cy), float64(r), 0, 2*math.Pi), w, c, true)
}

func (p *Panel) disc(cx, cy, r float32, c color.NRGBA) { p.fillRR(cx-r, cy-r, 2*r, 2*r, r, c) }

func (p *Panel) clipTo(x, y, w, h float32) clip.Stack {
	return clip.Rect(p.rect(x, y, w, h)).Push(p.ops)
}

func (p *Panel) tex(c color.NRGBA) paint.ImageOp {
	k := uint32(c.R>>3)<<10 | uint32(c.G>>3)<<5 | uint32(c.B>>3)
	if op, ok := p.glows[k]; ok {
		return op
	}
	if len(p.glows) > 40 {
		p.glows = map[uint32]paint.ImageOp{}
	}
	img := image.NewRGBA(image.Rect(0, 0, texN, texN))
	for y := 0; y < texN; y++ {
		for x := 0; x < texN; x++ {
			d := math.Hypot(float64(x)+0.5-texN/2, float64(y)+0.5-texN/2) / (texN / 2)
			if d >= 1 {
				continue
			}
			a, o := math.Pow(1-d, 1.55), img.PixOffset(x, y)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = uint8(float64(c.R)*a), uint8(float64(c.G)*a), uint8(float64(c.B)*a), uint8(255*a)
		}
	}
	op := paint.NewImageOp(img)
	p.glows[k] = op
	return op
}

func (p *Panel) glow(cx, cy, r float32, c color.NRGBA, k float32) { p.glowE(cx, cy, r, r, c, k) }

func (p *Panel) glowE(cx, cy, rx, ry float32, c color.NRGBA, k float32) {
	if k < 0.01 || rx < 1 || ry < 1 {
		return
	}
	defer op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(2*rx*p.s/texN, 2*ry*p.s/texN)).Offset(f32.Pt((cx-rx)*p.s, (cy-ry)*p.s))).Push(p.ops).Pop()
	defer paint.PushOpacity(p.ops, min(k, 1)).Pop()
	defer clip.Rect(image.Rect(0, 0, texN, texN)).Push(p.ops).Pop()
	p.tex(c).Add(p.ops)
	paint.PaintOp{}.Add(p.ops)
}

func (p *Panel) label(x, y, maxW float32, s string, sp float32, c color.NRGBA, w font.Weight, draw bool) float32 {
	g := p.gtx
	g.Constraints = layout.Constraints{Max: image.Pt(p.px(maxW), p.px(80))}
	l := material.Label(p.th, unit.Sp(sp), s)
	l.Color, l.Font, l.MaxLines, l.Truncator = c, font.Font{Typeface: "Segoe UI", Weight: w}, 1, "…"
	if !draw {
		m := op.Record(p.ops)
		d := l.Layout(g)
		m.Stop()
		return float32(d.Size.X) / p.s
	}
	st := op.Offset(image.Pt(p.px(x), p.px(y))).Push(p.ops)
	d := l.Layout(g)
	st.Pop()
	return float32(d.Size.X) / p.s
}

func (p *Panel) text(x, y float32, s string, sp float32, c color.NRGBA, w font.Weight) float32 {
	return p.label(x, y, 380, s, sp, c, w, true)
}

func (p *Panel) textFit(x, y, maxW float32, s string, sp float32, c color.NRGBA, w font.Weight) float32 {
	return p.label(x, y, maxW, s, sp, c, w, true)
}

func (p *Panel) textW(s string, sp float32, w font.Weight) float32 {
	return p.label(0, 0, 380, s, sp, cWhite, w, false)
}

func (p *Panel) textC(cx, y float32, s string, sp float32, c color.NRGBA, w font.Weight) {
	p.text(cx-p.textW(s, sp, w)/2, y, s, sp, c, w)
}

func (p *Panel) textR(rx, y float32, s string, sp float32, c color.NRGBA, w font.Weight) {
	p.text(rx-p.textW(s, sp, w), y, s, sp, c, w)
}

type pstate struct {
	down, hover, dragged, was bool
	sx, sy, x, y              float32
	lastSend                  time.Time
	lastVal                   int
}

type hit struct {
	Hover, Down, Clicked, Dragging, Moved, Released bool
	X, Y, SX, SY                                    float32
}

func (p *Panel) area(tag any, x, y, w, h float32) hit {
	st := p.ptr[tag]
	if st == nil {
		st = &pstate{lastVal: -1}
		p.ptr[tag] = st
	}
	defer p.clipTo(x, y, w, h).Pop()
	event.Op(p.ops, tag)
	pointer.CursorPointer.Add(p.ops)
	var clicked bool
	for {
		ev, ok := p.gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Release | pointer.Drag | pointer.Enter | pointer.Leave | pointer.Cancel})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		px, py := e.Position.X/p.s, e.Position.Y/p.s
		switch e.Kind {
		case pointer.Press:
			st.down, st.dragged, st.sx, st.sy = true, false, px, py
		case pointer.Drag:
			if st.down && (math.Abs(float64(px-st.sx)) > 5 || math.Abs(float64(py-st.sy)) > 5) {
				st.dragged = true
			}
		case pointer.Release:
			clicked = clicked || (st.down && !st.dragged)
			st.down = false
		case pointer.Cancel:
			st.down = false
		case pointer.Enter:
			st.hover = true
		case pointer.Leave:
			st.hover = false
		}
		st.x, st.y = px, py
	}
	h2 := hit{Hover: st.hover, Down: st.down, Clicked: clicked, Dragging: st.down && st.dragged, Moved: st.dragged, Released: st.was && !st.down, X: st.x, Y: st.y, SX: st.sx, SY: st.sy}
	st.was = st.down
	return h2
}

func (p *Panel) wheel(tag any, x, y, w, h float32) float32 {
	defer p.clipTo(x, y, w, h).Pop()
	defer pointer.PassOp{}.Push(p.ops).Pop()
	event.Op(p.ops, tag)
	var dy float32
	for {
		ev, ok := p.gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Scroll, ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}, ScrollX: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			dy += e.Scroll.Y + e.Scroll.X
		}
	}
	return dy / p.s
}

func (p *Panel) ease(key any, target, rate float32) float32 {
	a := p.an[key]
	if a == nil {
		a = new(float32)
		*a = target
		p.an[key] = a
	}
	if p.Static {
		*a = target
		return target
	}
	*a += (target - *a) * float32(1-math.Exp(float64(-p.dt*rate)))
	if math.Abs(float64(target-*a)) > 0.002 {
		p.animating = true
	} else {
		*a = target
	}
	return *a
}

func (p *Panel) throttled(tag any, v int, final bool, f func(int)) {
	st := p.ptr[tag]
	if st == nil {
		st = &pstate{lastVal: -1}
		p.ptr[tag] = st
	}
	if v == st.lastVal && !final {
		return
	}
	if final || p.gtx.Now.Sub(st.lastSend) > 55*time.Millisecond {
		st.lastSend, st.lastVal = p.gtx.Now, v
		f(v)
	}
}

func (p *Panel) icoSun(cx, cy float32, c color.NRGBA) {
	p.ring(cx, cy, 4, 1.6, c)
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		p.line(cx+7*float32(math.Cos(a)), cy+7*float32(math.Sin(a)), cx+10*float32(math.Cos(a)), cy+10*float32(math.Sin(a)), 1.6, c)
	}
}

func (p *Panel) icoSofa(cx, cy float32, c color.NRGBA) {
	p.strokeRR(cx-9, cy-3, 18, 9, 3, 1.6, c)
	p.strokeRR(cx-6, cy-8, 12, 6, 3, 1.6, c)
	p.line(cx-6, cy+6, cx-6, cy+9, 1.6, c)
	p.line(cx+6, cy+6, cx+6, cy+9, 1.6, c)
}

func (p *Panel) icoFilm(cx, cy float32, c color.NRGBA) {
	p.strokeRR(cx-8, cy-9, 16, 18, 2.5, 1.6, c)
	p.line(cx-3, cy-9, cx-3, cy+9, 1.6, c)
	p.line(cx+3, cy-9, cx+3, cy+9, 1.6, c)
	for _, yy := range []float32{-4, 0, 4} {
		p.line(cx-8, cy+yy, cx-3, cy+yy, 1.4, c)
		p.line(cx+3, cy+yy, cx+8, cy+yy, 1.4, c)
	}
}

func (p *Panel) icoPin(cx, cy float32, c color.NRGBA, fill bool) {
	p.strokePoly([]f32.Point{p.pt(cx-4, cy-9), p.pt(cx+4, cy-9), p.pt(cx+3, cy-1), p.pt(cx+6, cy+2), p.pt(cx-6, cy+2), p.pt(cx-3, cy-1)}, 1.6, c, true)
	if fill {
		p.fillPoly([]f32.Point{p.pt(cx-4, cy-9), p.pt(cx+4, cy-9), p.pt(cx+3, cy-1), p.pt(cx+6, cy+2), p.pt(cx-6, cy+2), p.pt(cx-3, cy-1)}, c)
	}
	p.line(cx, cy+2, cx, cy+9, 1.6, c)
}

func (p *Panel) icoMoon(cx, cy float32, c color.NRGBA) {
	p.strokePoly(p.arc(float64(cx), float64(cy), 8, 0.55, 2*math.Pi-0.55), 1.6, c, false)
	p.strokePoly(p.arc(float64(cx)+3.5, float64(cy)-1.5, 6.2, 0.9, 2*math.Pi-1.7), 1.6, c, false)
}

func (p *Panel) icoDoor(cx, cy float32, c color.NRGBA) {
	p.strokeRR(cx-6, cy-9, 12, 18, 1.5, 1.6, c)
	p.line(cx-9, cy+9, cx+9, cy+9, 1.6, c)
	p.disc(cx+3, cy+1, 1.2, c)
}

func (p *Panel) icoBulb(cx, cy float32, c color.NRGBA) {
	p.strokePoly(p.arc(float64(cx), float64(cy)-2.5, 6.5, math.Pi/2+0.85, 2.5*math.Pi-0.85), 1.6, c, false)
	p.line(cx-3.5, cy+3.5, cx-3, cy+6, 1.6, c)
	p.line(cx+3.5, cy+3.5, cx+3, cy+6, 1.6, c)
	p.line(cx-3, cy+6, cx+3, cy+6, 1.6, c)
	p.line(cx-2, cy+9, cx+2, cy+9, 1.6, c)
}

func (p *Panel) icoPlug(cx, cy float32, c color.NRGBA) {
	p.strokeRR(cx-6, cy-3, 12, 9, 3, 1.6, c)
	p.line(cx-3, cy-3, cx-3, cy-8, 1.6, c)
	p.line(cx+3, cy-3, cx+3, cy-8, 1.6, c)
	p.line(cx, cy+6, cx, cy+10, 1.6, c)
}

func (p *Panel) icoSpark(cx, cy float32, c color.NRGBA) {
	p.strokePoly([]f32.Point{p.pt(cx, cy-9), p.pt(cx+2.5, cy-2.5), p.pt(cx+9, cy), p.pt(cx+2.5, cy+2.5), p.pt(cx, cy+9), p.pt(cx-2.5, cy+2.5), p.pt(cx-9, cy), p.pt(cx-2.5, cy-2.5)}, 1.6, c, true)
}

func (p *Panel) icoPower(cx, cy, r float32, c color.NRGBA) {
	p.strokePoly(p.arc(float64(cx), float64(cy), float64(r), -math.Pi/2+0.7, 1.5*math.Pi-0.7), 2, c, false)
	p.line(cx, cy-r-1, cx, cy-1, 2, c)
}

func (p *Panel) icoHome(cx, cy float32, c color.NRGBA) {
	p.strokePoly([]f32.Point{p.pt(cx-8, cy), p.pt(cx, cy-8), p.pt(cx+8, cy), p.pt(cx+8, cy+8), p.pt(cx-8, cy+8)}, 1.6, c, true)
	p.strokePoly([]f32.Point{p.pt(cx-2.5, cy+8), p.pt(cx-2.5, cy+2), p.pt(cx+2.5, cy+2), p.pt(cx+2.5, cy+8)}, 1.6, c, false)
}

func (p *Panel) icoSliders(cx, cy float32, c color.NRGBA) {
	p.line(cx-8, cy-4, cx+8, cy-4, 1.6, c)
	p.line(cx-8, cy+4, cx+8, cy+4, 1.6, c)
	p.disc(cx-3, cy-4, 2.6, c)
	p.disc(cx+3, cy+4, 2.6, c)
}

func (p *Panel) icoBack(cx, cy float32, c color.NRGBA) {
	p.strokePoly([]f32.Point{p.pt(cx+3, cy-7), p.pt(cx-3, cy), p.pt(cx+3, cy+7)}, 2, c, false)
}

func (p *Panel) icoLamp(cx, cy float32, c color.NRGBA) {
	p.strokePoly([]f32.Point{p.pt(cx-5, cy-8), p.pt(cx+5, cy-8), p.pt(cx+8, cy+1), p.pt(cx-8, cy+1)}, 1.6, c, true)
	p.line(cx, cy+1, cx, cy+8, 1.6, c)
	p.line(cx-4, cy+9, cx+4, cy+9, 1.6, c)
}

func (p *Panel) icoEye(cx, cy float32, c color.NRGBA, open bool) {
	up, lo := make([]f32.Point, 0, 21), make([]f32.Point, 0, 21)
	for i := 0; i <= 20; i++ {
		t := float32(i)/10 - 1
		k := 1 - t*t
		up = append(up, p.pt(cx+9.5*t, cy-5.5*k))
		lo = append(lo, p.pt(cx+9.5*t, cy+5.5*k))
	}
	p.strokePoly(up, 1.6, c, false)
	p.strokePoly(lo, 1.6, c, false)
	if open {
		p.disc(cx, cy, 2.8, c)
		return
	}
	p.line(cx-8, cy+8, cx+8, cy-8, 1.8, c)
}

func (p *Panel) icoRefresh(cx, cy float32, c color.NRGBA) {
	p.strokePoly(p.arc(float64(cx), float64(cy), 7, -0.35, 5.1), 1.8, c, false)
	p.strokePoly([]f32.Point{p.pt(cx+7.5, cy-7.5), p.pt(cx+7.5, cy-1.5), p.pt(cx+1.5, cy-1.5)}, 1.8, c, false)
}

func (p *Panel) icoShield(cx, cy float32, c color.NRGBA) {
	p.strokePoly([]f32.Point{p.pt(cx, cy-9), p.pt(cx+8, cy-5), p.pt(cx+7, cy+3), p.pt(cx, cy+9), p.pt(cx-7, cy+3), p.pt(cx-8, cy-5)}, 1.8, c, true)
}

func (p *Panel) icoSpeaker(cx, cy float32, c color.NRGBA, on bool) {
	p.strokePoly([]f32.Point{p.pt(cx-8, cy-3), p.pt(cx-3, cy-3), p.pt(cx+2, cy-7), p.pt(cx+2, cy+7), p.pt(cx-3, cy+3), p.pt(cx-8, cy+3)}, 1.5, c, true)
	if on {
		p.strokePoly(p.arc(float64(cx+1), float64(cy), 4.5, -0.8, 0.8), 1.5, c, false)
		p.strokePoly(p.arc(float64(cx+1), float64(cy), 7.5, -0.75, 0.75), 1.5, c, false)
		return
	}
	p.line(cx+5, cy-4, cx+11, cy+4, 1.8, c)
	p.line(cx+11, cy-4, cx+5, cy+4, 1.8, c)
}
