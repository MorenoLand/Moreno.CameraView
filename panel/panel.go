package panel

import (
	"fmt"
	"image"
	"image/color"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"moreno/blinkview/model"
	"moreno/blinkview/store"
)

const (
	viewHome = iota
	viewCamera
	viewClips
	viewSettings
	viewAPI
	semi = font.SemiBold
	reg  = font.Normal
)

var (
	cBlink  = rgb(0, 200, 128)
	cBlinkD = rgb(0, 150, 96)
	cRed    = rgb(255, 120, 110)
)

type Backend interface {
	Clips() []model.Clip
	ClipStatus() string
	OpenClip(string)
	LoggedIn() bool
	LoginState() (status string, waiting bool)
	StartLogin()
	CancelLogin()
	HomeName() string
	Cameras() []model.Camera
	Thumb(id string) (image.Image, int)
	Watch(id string)
	StopWatch()
	WatchState() (id string, img image.Image, rev int, note string, active bool)
	Armed() (armed, known bool)
	SetArmed(on bool)
	SyncStatus() (name, status string, warn bool)
	Snapshot(id string)
	Muted() bool
	SetMuted(bool)
	Refresh()
	Signout()
	Quit()
	Settings() store.Settings
	SaveSettings(store.Settings)
	SetAPIPassword(user, pass string)
	APIStatus() string
	WebURL() string
	OpenWeb()
}

type Panel struct {
	Static            bool
	MaxH, WantH       float32
	be                Backend
	th                *material.Theme
	gtx               layout.Context
	ops               *op.Ops
	s, w, h, dt       float32
	ptr               map[any]*pstate
	an                map[any]*float32
	glows             map[uint32]paint.ImageOp
	imgs              map[string]thumbOp
	last              time.Time
	animating         bool
	view              int
	prevView          int
	sel               string
	scroll, contentH  float32
	edPort, edUser    widget.Editor
	edPass, edAllow   widget.Editor
	edCode            widget.Editor
	apiInit, showPass bool
	apiMsg            string
	introT            float32
	watchID           string
	watchImg          image.Image
	watchRev          int
	watchNote         string
	watchActive       bool
	muted             bool
	settings          store.Settings
}

type thumbOp struct {
	op  paint.ImageOp
	rev int
}

func New(be Backend) *Panel {
	th := material.NewTheme()
	th.Shaper = loadShaper()
	return &Panel{be: be, th: th, ptr: map[any]*pstate{}, an: map[any]*float32{}, glows: map[uint32]paint.ImageOp{}, imgs: map[string]thumbOp{}}
}

func (p *Panel) ShowCamera(id string) { p.view, p.sel = viewCamera, id }

func (p *Panel) ShowView(v int) { p.view = v }

func (p *Panel) ShowClips() { p.view, p.sel = viewClips, "" }

func (p *Panel) ShowSettings() { p.view, p.sel = viewSettings, "" }

func (p *Panel) ShowAPI(confirm bool) { p.view, p.apiInit = viewAPI, false }

func (p *Panel) Layout(gtx layout.Context) layout.Dimensions {
	p.gtx, p.ops, p.s = gtx, gtx.Ops, gtx.Metric.PxPerDp
	p.w, p.h = float32(gtx.Constraints.Max.X)/p.s, float32(gtx.Constraints.Max.Y)/p.s
	if p.MaxH <= 0 {
		p.MaxH = p.h
	}
	p.dt = 0.016
	if !p.last.IsZero() {
		p.dt = clampf(float32(gtx.Now.Sub(p.last).Seconds()), 0, 0.05)
	}
	p.last, p.animating = gtx.Now, false
	cams := p.be.Cameras()
	p.watchID, p.watchImg, p.watchRev, p.watchNote, p.watchActive = p.be.WatchState()
	p.muted = p.be.Muted()
	p.settings = p.be.Settings()
	if p.view == viewCamera {
		found := false
		for _, c := range cams {
			if c.ID == p.sel {
				found = true
			}
		}
		if !found {
			p.view, p.sel = viewHome, ""
			p.be.StopWatch()
		}
	}
	if st, waiting := p.be.LoginState(); waiting || strings.HasPrefix(st, "Error") {
		if waiting {
			p.animating = true
		}
	}
	if p.view == viewCamera && p.watchID == p.sel && p.watchActive && p.watchImg == nil {
		p.animating = true
	}
	if p.view != p.prevView {
		p.prevView, p.scroll = p.view, 0
	}
	p.background(cams)
	p.introT = p.ease("intro", 1, 4)
	st := op.Offset(image.Pt(0, p.px((1-p.introT)*10))).Push(p.ops)
	fade := paint.PushOpacity(p.ops, p.introT)
	if !p.be.LoggedIn() {
		p.loginView()
	} else {
		p.header(cams)
		switch p.view {
		case viewCamera:
			p.cameraView(cams)
		case viewClips:
			p.clipsView()
		case viewSettings:
			p.settingsView()
		case viewAPI:
			p.apiView()
		default:
			p.homeView(cams)
		}
	}
	fade.Pop()
	st.Pop()
	if p.animating {
		gtx.Execute(op.InvalidateCmd{})
	}
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (p *Panel) background(cams []model.Camera) {
	on := 0
	for _, c := range cams {
		if c.Online {
			on++
		}
	}
	k := p.ease("bgk", float32(min(on, 4))/4, 4)
	top := mix(rgb(20, 22, 58), rgb(8, 50, 52), 0.55*k)
	p.gradRR(0, 0, p.w, p.h, 0, top, rgb(6, 9, 20), false)
	p.glow(p.w*0.8, 38, p.w*0.55, cBlink, 0.1+0.12*k)
	p.glow(p.w*0.1, p.h*0.65, p.w*0.5, rgb(90, 120, 255), 0.08)
}

func (p *Panel) glass(x, y, w, h, r float32) {
	p.fillRR(x, y, w, h, r, rgba(255, 255, 255, 6))
	p.strokeRR(x, y, w, h, r, 1, rgba(255, 255, 255, 10))
}

func (p *Panel) field(ed *widget.Editor, x, y, w, h float32, hint string, mask rune) {
	p.glass(x, y, w, h, 14)
	ed.SingleLine, ed.Mask = true, mask
	g := p.gtx
	g.Constraints = layout.Exact(image.Pt(p.px(w-24), p.px(h-14)))
	st := op.Offset(image.Pt(p.px(x+12), p.px(y+10))).Push(p.ops)
	e := material.Editor(p.th, ed, hint)
	e.Color, e.HintColor, e.TextSize = cWhite, cFaint, unit.Sp(15)
	e.Layout(g)
	st.Pop()
}

func (p *Panel) button(tag string, x, y, w, h float32, label string, primary bool) bool {
	ht := p.area("btn:"+tag, x, y, w, h)
	hv := p.ease("btn:"+tag, b2f(ht.Hover), 16)
	if primary {
		p.gradRR(x, y, w, h, 14, mix(rgb(0, 214, 138), rgb(40, 232, 160), hv), mix(rgb(0, 160, 104), rgb(0, 184, 120), hv), false)
	} else {
		p.fillRR(x, y, w, h, 14, mix(rgba(255, 255, 255, 6), rgba(255, 255, 255, 13), hv))
		p.strokeRR(x, y, w, h, 14, 1, rgba(255, 255, 255, 14))
	}
	p.textC(x+w/2, y+h/2-10, label, 13.5, cWhite, semi)
	return ht.Clicked
}

func (p *Panel) toggleRow(y float32, label string, on bool, f func()) {
	p.glass(16, y, p.w-32, 50, 16)
	p.text(32, y+14, label, 14, cWhite, reg)
	t := p.ease("tg:"+label, b2f(on), 14)
	p.fillRR(p.w-76, y+12, 44, 26, 13, mix(rgb(52, 58, 86), cBlue, t))
	p.disc(p.w-63+t*18, y+25, 10, cWhite)
	if p.area("trow:"+label, 16, y, p.w-32, 50).Clicked {
		f()
	}
}

func (p *Panel) navRow(y float32, label, value string, f func()) {
	p.glass(16, y, p.w-32, 50, 16)
	p.text(32, y+14, label, 14, cWhite, reg)
	p.strokePoly([]f32.Point{p.pt(p.w-40, y+19), p.pt(p.w-34, y+25), p.pt(p.w-40, y+31)}, 1.8, cDim, false)
	p.textR(p.w-52, y+15, value, 12.5, cDim, reg)
	if p.area("nav:"+label, 16, y, p.w-32, 50).Clicked {
		f()
	}
}

func (p *Panel) buttonRow(y float32, label string, c color.NRGBA, f func()) {
	p.glass(16, y, p.w-32, 50, 16)
	p.text(32, y+14, label, 14, c, reg)
	if p.area("brow:"+label, 16, y, p.w-32, 50).Clicked {
		f()
	}
}

func (p *Panel) header(cams []model.Camera) {
	back := p.view != viewHome
	p.glass(16, 16, 36, 36, 12)
	if back {
		p.icoBack(34, 34, cWhite)
	} else {
		p.icoHome(34, 34, rgb(220, 224, 240))
	}
	if p.area("home", 16, 16, 36, 36).Clicked {
		if p.view == viewCamera {
			p.be.StopWatch()
		}
		if p.view == viewAPI {
			p.view = viewSettings
		} else {
			p.view, p.sel = viewHome, ""
		}
	}
	name := p.be.HomeName()
	if name == "" {
		name = "My Blink"
	}
	p.textFit(62, 14, 178, name, 16, cWhite, semi)
	on, off := 0, 0
	for _, c := range cams {
		if c.Online {
			on++
		} else {
			off++
		}
	}
	sub := fmt.Sprintf("%d of %d cameras online", on, len(cams))
	if len(cams) == 0 {
		sub = "No cameras"
	}
	if off > 0 {
		sub += fmt.Sprintf(" · %d offline", off)
	}
	p.disc(67, 46, 3, map[bool]color.NRGBA{true: cBlink, false: cFaint}[on > 0])
	p.textFit(76, 36, 178, sub, 12, cDim, reg)
	cs := p.ease("hdr:clips", b2f(p.view == viewClips), 14)
	p.fillRR(262, 16, 36, 36, 12, mix(rgba(255, 255, 255, 6), rgba(90, 150, 255, 38), cs))
	p.icoFilm(280, 34, mix(rgb(210, 214, 232), cWhite, cs))
	if p.area("clips", 262, 16, 36, 36).Clicked {
		if p.view == viewCamera {
			p.be.StopWatch()
		}
		if p.view == viewClips {
			p.view = viewHome
		} else {
			p.view, p.sel = viewClips, ""
		}
	}
	gs := p.ease("hdr:gear", b2f(p.view == viewSettings || p.view == viewAPI), 14)
	p.fillRR(306, 16, 36, 36, 18, mix(rgba(255, 255, 255, 6), rgba(90, 150, 255, 40), gs))
	p.icoSliders(324, 34, mix(rgb(210, 214, 232), cWhite, gs))
	if p.area("gear", 306, 16, 36, 36).Clicked {
		if p.view == viewCamera {
			p.be.StopWatch()
		}
		if p.view == viewSettings || p.view == viewAPI {
			p.view = viewHome
		} else {
			p.view, p.sel = viewSettings, ""
		}
	}
	armed, known := p.be.Armed()
	pw := p.ease("hdr:arm", b2f(armed && known), 12)
	p.glow(372, 34, 38, cBlue, 0.8*pw)
	p.gradRR(352, 14, 40, 40, 20, mix(rgb(52, 58, 86), rgb(88, 150, 255), pw), mix(rgb(40, 45, 70), rgb(44, 96, 230), pw), false)
	p.icoShield(372, 34, map[bool]color.NRGBA{true: cWhite, false: cDim}[known])
	if p.area("arm", 352, 14, 40, 40).Clicked && known {
		p.be.SetArmed(!armed)
	}
}

func (p *Panel) syncBar(y float32) float32 {
	name, status, warn := p.be.SyncStatus()
	if name == "" {
		return y
	}
	p.glass(16, y, p.w-32, 44, 16)
	p.strokeRR(31, y+12, 16, 20, 4, 1.4, cDim)
	p.disc(39, y+26, 2.4, map[bool]color.NRGBA{true: cRed, false: cBlink}[warn])
	p.text(56, y+9, name, 13, cWhite, semi)
	tx := p.w - 32 - p.textW(status, 12, reg)
	if warn {
		tx -= 22
		p.strokePoly([]f32.Point{p.pt(tx+8, y+14), p.pt(tx+16, y+28), p.pt(tx, y+28)}, 1.6, cRed, true)
		p.line(tx+8, y+19, tx+8, y+24, 1.6, cRed)
		tx += 22
	}
	p.text(tx, y+13, status, 12, map[bool]color.NRGBA{true: cRed, false: cDim}[warn], reg)
	return y + 56
}

func (p *Panel) homeView(cams []model.Camera) {
	top := float32(76)
	y0 := p.syncBar(top)
	gap := float32(14)
	cw := p.w - 32
	ch := cw * 0.48
	p.contentH = 0
	if len(cams) > 0 {
		p.contentH = float32(len(cams)-1)*(ch+gap) + ch + 20
	}
	p.WantH = min(p.MaxH, max(260, y0+p.contentH+6))
	vh := p.h - y0
	p.scroll = clampf(p.scroll-p.wheel("homewheel", 0, 0, p.w, p.h)*1.2, 0, max(0, p.contentH-vh))
	sc := p.ease("scroll", p.scroll, 18)
	defer p.clipTo(0, y0, p.w, vh).Pop()
	for i, c := range cams {
		y := y0 + float32(i)*(ch+gap) - sc
		if y+ch > y0 && y < p.h {
			p.camTile(c, 16, y, cw, ch, i)
		}
	}
	if len(cams) == 0 {
		p.textC(p.w/2, y0+70, "No cameras found on this account", 14, cDim, reg)
	}
}

func (p *Panel) thumbOp(id string) (paint.ImageOp, bool) {
	img, rev := p.be.Thumb(id)
	if img == nil {
		return paint.ImageOp{}, false
	}
	return p.imageOp("thumb:"+id, img, rev)
}

func (p *Panel) imageOp(key string, img image.Image, rev int) (paint.ImageOp, bool) {
	if t, ok := p.imgs[key]; ok && t.rev == rev {
		return t.op, true
	}
	o := paint.NewImageOp(img)
	p.imgs[key] = thumbOp{o, rev}
	return o, true
}

func (p *Panel) cover(o paint.ImageOp, x, y, w, h, r float32) {
	sz := o.Size()
	k := max(w*p.s/float32(sz.X), h*p.s/float32(sz.Y))
	dx, dy := (w*p.s-float32(sz.X)*k)/2, (h*p.s-float32(sz.Y)*k)/2
	rr := p.px(r)
	defer clip.RRect{Rect: p.rect(x, y, w, h), NE: rr, NW: rr, SE: rr, SW: rr}.Push(p.ops).Pop()
	defer op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(k, k)).Offset(f32.Pt(x*p.s+dx, y*p.s+dy))).Push(p.ops).Pop()
	o.Add(p.ops)
	paint.PaintOp{}.Add(p.ops)
}

func ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func (p *Panel) camTile(c model.Camera, x, y, w, h float32, i int) {
	body := p.area("cam:"+c.ID, x, y, w, h)
	hv := p.ease("camh:"+c.ID, b2f(body.Hover), 14)
	on := b2f(c.Online)
	live := p.watchID == c.ID && p.watchActive
	p.glow(x+w/2, y+h/2, w*0.7, cBlink, 0.16*on*(0.7+0.3*hv))
	defer p.pressScale("cam:"+c.ID, x, y, w, h, body.Down)()
	p.fillRR(x, y, w, h, 18, rgb(14, 18, 32))
	if o, ok := p.thumbOp(c.ID); ok {
		p.cover(o, x, y, w, h, 18)
	}
	p.gradRR(x, y, w, h, 18, rgba(0, 0, 0, 62), rgba(0, 0, 0, 10), false)
	p.gradRR(x, y+h-86, w, 86, 18, rgba(0, 0, 0, 0), rgba(0, 0, 0, 175), false)
	if !c.Online && !live {
		p.fillRR(x, y, w, h, 18, rgba(6, 9, 20, 145))
	}
	if c.Online || live {
		cr := 27 + 4*hv
		p.glow(x+w/2, y+h/2, cr*2.4, cBlink, 0.2+0.12*hv)
		p.disc(x+w/2, y+h/2, cr, rgba(0, 0, 0, 125))
		p.fillPoly([]f32.Point{p.pt(x+w/2-6, y+h/2-11), p.pt(x+w/2+12, y+h/2), p.pt(x+w/2-6, y+h/2+11)}, rgb(255, 255, 255))
	} else {
		p.textC(x+w/2, y+h/2-20, c.KindName()+" is offline", 14, cWhite, semi)
		p.textC(x+w/2, y+h/2+3, "Tap for details", 12, cDim, reg)
	}
	p.strokeRR(x, y, w, h, 18, 1.2, mix(rgba(255, 255, 255, 18), alpha(cBlink, 200), hv*on))
	p.text(x+16, y+14, c.Name, 15, cWhite, semi)
	if live {
		p.fillRR(x+w-62, y+12, 50, 22, 11, rgba(0, 0, 0, 160))
		p.disc(x+w-50, y+23, 4, cRed)
		p.text(x+w-41, y+16, "LIVE", 11, cWhite, semi)
	} else {
		bw := p.textW(map[bool]string{true: "Online", false: "Offline"}[c.Online], 11, reg) + 22
		p.fillRR(x+w-14-bw, y+12, bw, 22, 11, rgba(0, 0, 0, 140))
		p.textC(x+w-14-bw/2, y+15, map[bool]string{true: "Online", false: "Offline"}[c.Online], 11, map[bool]color.NRGBA{true: cWhite, false: cRed}[c.Online], reg)
	}
	if a := ago(c.Seen); a != "" {
		bw := p.textW(a, 11.5, reg) + 16
		p.fillRR(x+14, y+h-34, bw, 22, 11, rgba(0, 0, 0, 150))
		p.text(x+22, y+h-31, a, 11.5, cWhite, reg)
	}
	if c.Battery != "" {
		p.textR(x+w-16, y+h-31, c.Battery, 11.5, cDim, reg)
	}
	if body.Clicked {
		p.view, p.sel, p.scroll = viewCamera, c.ID, 0
		p.be.Watch(c.ID)
	}
}

func (p *Panel) pressScale(tag string, x, y, w, h float32, down bool) func() {
	pr := p.ease("press:"+tag, b2f(down), 22)
	sc := 1 - 0.03*pr
	st := op.Affine(f32.Affine2D{}.Scale(f32.Pt((x+w/2)*p.s, (y+h/2)*p.s), f32.Pt(sc, sc))).Push(p.ops)
	return st.Pop
}

func (p *Panel) cameraView(cams []model.Camera) {
	var c model.Camera
	found := false
	for _, x := range cams {
		if x.ID == p.sel {
			c, found = x, true
		}
	}
	if !found {
		p.view = viewHome
		return
	}
	w := p.w - 32
	h := w * 9 / 16
	x, y := float32(16), float32(84)
	watching := p.watchID == c.ID && p.watchActive
	p.glow(x+w/2, y+h/2, w*0.75, cBlink, 0.15*b2f(c.Online))
	p.fillRR(x, y, w, h, 20, rgb(14, 18, 32))
	if p.watchID == c.ID && p.watchImg != nil {
		if o, ok := p.imageOp("live:"+p.watchID, p.watchImg, p.watchRev); ok {
			p.cover(o, x, y, w, h, 20)
		}
	} else if o, ok := p.thumbOp(c.ID); ok {
		p.cover(o, x, y, w, h, 20)
	}
	p.strokeRR(x, y, w, h, 20, 1.2, rgba(255, 255, 255, 20))
	if watching && p.watchImg == nil {
		p.fillRR(x, y, w, h, 20, rgba(6, 9, 20, 155))
		p.textC(x+w/2, y+h/2-8, p.watchNote, 14, cWhite, semi)
	}
	if watching && p.watchImg != nil {
		p.fillRR(x+w-64, y+12, 52, 22, 11, rgba(0, 0, 0, 150))
		p.disc(x+w-52, y+23, 4, cRed)
		p.text(x+w-43, y+16, "LIVE", 11, cWhite, semi)
	}
	if !c.Online && !watching {
		p.fillRR(x, y, w, h, 20, rgba(6, 9, 20, 150))
		p.textC(x+w/2, y+h/2-10, c.KindName()+" is offline", 16, cWhite, semi)
	}
	if watching {
		p.fillRR(x+12, y+12, 38, 30, 12, rgba(0, 0, 0, 145))
		p.icoSpeaker(x+31, y+27, cWhite, !p.muted)
		if p.area("mute", x+12, y+12, 38, 30).Clicked {
			p.be.SetMuted(!p.muted)
		}
	}
	y += h + 14
	p.text(x, y, c.Name, 20, cWhite, semi)
	sub := map[bool]string{true: "Online", false: "Offline"}[c.Online]
	if a := ago(c.Seen); a != "" {
		sub += " · last clip " + a
	}
	if c.Battery != "" {
		sub += " · " + c.Battery
	}
	p.text(x, y+30, sub, 12.5, cDim, reg)
	by := y + 62
	bw := (w - 8) / 2
	if p.button("snap", x, by, bw, 44, "Refresh snapshot", false) {
		if c.Online {
			p.be.Snapshot(c.ID)
		}
	}
	label := "Live view"
	if watching {
		label = "Stop live"
	}
	if p.button("live", x+bw+8, by, bw, 44, label, true) {
		if watching {
			p.be.StopWatch()
		} else {
			p.be.Watch(c.ID)
		}
	}
	p.WantH = min(p.MaxH, by+68)
	if p.watchID == c.ID && p.watchNote != "" && !(watching && p.watchImg == nil) {
		col := map[bool]color.NRGBA{true: cRed, false: cDim}[strings.HasPrefix(p.watchNote, "Error")]
		p.WantH = min(p.MaxH, max(p.WantH, p.para(x, by+58, w, p.watchNote, 12, col, reg, 3)+14))
	}
}

func (p *Panel) clipsView() {
	clips := p.be.Clips()
	w := p.w - 32
	x := float32(16)
	y0 := float32(76)
	vh := p.h - y0
	if len(clips) == 0 {
		p.textC(p.w/2, y0+70, "No clips yet", 14, cDim, reg)
		p.textC(p.w/2, y0+93, "Cloud and Sync Module clips will appear here.", 12, cFaint, reg)
		if s := p.be.ClipStatus(); s != "" {
			p.textC(p.w/2, y0+116, s, 11, cFaint, reg)
		}
		p.WantH = min(p.MaxH, 220)
		return
	}
	days := 0
	for i, c := range clips {
		if i == 0 || !sameDay(c.At, clips[i-1].At) {
			days++
		}
	}
	content := float32(len(clips))*70 + float32(days)*40 + 20
	p.contentH = content
	p.WantH = min(p.MaxH, y0+content+8)
	p.scroll = clampf(p.scroll-p.wheel("clipwheel", 0, 0, p.w, p.h)*1.2, 0, max(0, content-vh))
	sc := p.ease("scroll", p.scroll, 18)
	defer p.clipTo(0, y0, p.w, vh).Pop()
	y := y0 - sc
	for i, c := range clips {
		if i == 0 || !sameDay(c.At, clips[i-1].At) {
			n := 0
			for _, d := range clips[i:] {
				if !sameDay(d.At, c.At) {
					break
				}
				n++
			}
			p.text(x+4, y+8, c.At.Format("Monday, January 2"), 13.5, cWhite, semi)
			p.text(x+4+p.textW(c.At.Format("Monday, January 2"), 13.5, semi)+10, y+10, fmt.Sprintf("%d events", n), 12, cDim, reg)
			y += 40
		}
		if y+64 > y0 && y < p.h {
			ht := p.area("clip:"+c.ID, x, y, w, 62)
			hv := p.ease("cliph:"+c.ID, b2f(ht.Hover), 14)
			p.fillRR(x, y, w, 62, 14, mix(rgba(255, 255, 255, 5), rgba(255, 255, 255, 11), hv))
			p.strokeRR(x, y, w, 62, 14, 1, mix(rgba(255, 255, 255, 9), alpha(cBlink, 160), hv))
			p.fillRR(x+8, y+7, 76, 48, 8, rgba(0, 200, 128, 30))
			p.strokeRR(x+8, y+7, 76, 48, 8, 1, rgba(0, 200, 128, 90))
			p.fillPoly([]f32.Point{p.pt(x+38, y+23), p.pt(x+54, y+31), p.pt(x+38, y+39)}, cBlink)
			p.text(x+98, y+10, c.Camera, 14.5, cWhite, semi)
			p.text(x+98, y+33, c.Home, 12, cDim, reg)
			p.textR(x+w-14, y+10, c.At.Format("3:04 PM"), 13.5, cWhite, reg)
			bw := p.textW(c.Kind, 11.5, reg) + 22
			p.fillRR(x+w-14-bw, y+33, bw, 22, 11, rgba(255, 255, 255, 12))
			p.textC(x+w-14-bw/2, y+36, c.Kind, 11.5, cWhite, reg)
			if ht.Clicked {
				p.be.OpenClip(c.ID)
			}
		}
		y += 70
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func (p *Panel) settingsView() {
	top := float32(78)
	p.text(16, top, "Settings", 18, cWhite, semi)
	y := top + 40
	s := p.settings
	api := map[bool]string{true: fmt.Sprintf("On, port %d", s.APIPort), false: "Off"}[s.LocalAPI]
	p.navRow(y, "Local API", api, func() { p.view, p.apiInit = viewAPI, false })
	web := "Off"
	if p.be.WebURL() != "" {
		web = "Open"
	}
	p.navRow(y+54, "Web panel", web, func() {
		if p.be.WebURL() != "" {
			p.be.OpenWeb()
		} else {
			p.view, p.apiInit = viewAPI, false
		}
	})
	p.buttonRow(y+108, "Refresh cameras", cWhite, p.be.Refresh)
	p.buttonRow(y+162, "Sign out", rgb(255, 150, 130), func() {
		p.be.Signout()
		p.view, p.sel = viewHome, ""
	})
	p.buttonRow(y+216, "Quit", cWhite, p.be.Quit)
	end := p.para(18, y+286, p.w-36, "BlinkView stays in the tray. Click the tray icon to open or hide this panel.", 11.5, cFaint, reg, 3)
	p.WantH = min(p.MaxH, max(y+282, end+16))
}

func (p *Panel) apiView() {
	s := p.settings
	top := float32(78)
	p.text(16, top, "Local API", 18, cWhite, semi)
	if !p.apiInit {
		p.apiInit, p.apiMsg = true, ""
		p.edPort.SetText(strconv.Itoa(s.APIPort))
		p.edAllow.SetText(strings.Join(s.APIAllow, ", "))
		p.edUser.SetText(s.APIUser)
		p.edPass.SetText("")
	}
	y := top + 42
	p.toggleRow(y, "Enabled", s.LocalAPI, func() {
		s.LocalAPI = !s.LocalAPI
		p.be.SaveSettings(s)
	})
	if s.LocalAPI {
		st := p.be.APIStatus()
		col := cDim
		if strings.HasPrefix(st, "Error") || strings.HasPrefix(st, "Set") {
			col = cRed
		}
		p.text(18, y+58, fmt.Sprintf("http://127.0.0.1:%d/", s.APIPort), 12, cAmber, reg)
		p.text(18, y+76, st, 11.5, col, reg)
	}
	p.text(18, y+104, "Port", 12, cDim, reg)
	p.field(&p.edPort, 16, y+124, 120, 44, "7481", 0)
	by := y + 180
	if p.be.WebURL() != "" {
		if p.button("web", p.w-16-120, y+116, 120, 44, "Open web panel", false) {
			p.be.OpenWeb()
		}
	}
	p.text(18, by, "Also allow (comma separated IPs or CIDR ranges)", 12, cDim, reg)
	p.field(&p.edAllow, 16, by+20, p.w-32, 44, "192.168.0.0/24", 0)
	ay := by + 76
	p.toggleRow(ay, "Require sign-in", s.APIAuth, func() {
		s.APIAuth = !s.APIAuth
		p.be.SaveSettings(s)
	})
	cy := ay + 58
	if s.APIAuth {
		fw := (p.w - 32 - 8) / 2
		p.field(&p.edUser, 16, cy, fw, 44, "Username", 0)
		hint := "New password"
		if s.APIHash != "" {
			hint = "Password set"
		}
		p.field(&p.edPass, 24+fw, cy, fw, 44, hint, '•')
		cy += 56
	}
	if p.button("apply", 16, cy, p.w-32, 46, "Apply", true) {
		p.applyAPI(s)
	}
	if p.apiMsg != "" {
		col := cGreen
		if p.apiMsg != "Applied" {
			col = cRed
		}
		p.text(18, cy+58, p.apiMsg, 12.5, col, reg)
	}
	end := p.para(18, cy+96, p.w-36, "The API is plain HTTP. Only this PC can connect unless you add addresses. Use sign-in on networks you do not fully trust.", 11.5, cFaint, reg, 4)
	p.WantH = min(p.MaxH, max(cy+62, end+16))
}

func (p *Panel) applyAPI(s store.Settings) {
	port, err := strconv.Atoi(strings.TrimSpace(p.edPort.Text()))
	if err != nil || port < 1024 || port > 65535 {
		p.apiMsg = "Port must be between 1024 and 65535"
		return
	}
	var allow []string
	for _, e := range strings.Split(p.edAllow.Text(), ",") {
		if e = strings.TrimSpace(e); e == "" {
			continue
		}
		if _, err := netip.ParsePrefix(e); err != nil {
			if _, err := netip.ParseAddr(e); err != nil {
				p.apiMsg = "Not an IP address or CIDR: " + e
				return
			}
		}
		allow = append(allow, e)
	}
	user, pass := strings.TrimSpace(p.edUser.Text()), p.edPass.Text()
	if s.APIAuth {
		if user == "" {
			p.apiMsg = "Enter a username"
			return
		}
		if pass == "" && s.APIHash == "" {
			p.apiMsg = "Enter a password"
			return
		}
	}
	s.APIPort, s.APIAllow = port, allow
	if s.APIAuth {
		s.APIUser = user
	}
	p.be.SaveSettings(s)
	if s.APIAuth && pass != "" {
		p.be.SetAPIPassword(user, pass)
		p.edPass.SetText("")
	}
	p.apiMsg = "Applied"
}

func (p *Panel) loginView() {
	status, waiting := p.be.LoginState()
	w := min(368, p.w-40)
	x, y := (p.w-w)/2, max(80, (p.h-310)/2)
	p.glow(p.w/2, y+150, 270, cBlink, 0.12)
	p.glass(x, y, w, 300, 22)
	p.disc(x+w/2, y+46, 24, cBlink)
	p.textC(x+w/2, y+29, "b", 28, rgb(6, 30, 22), semi)
	p.textC(x+w/2, y+90, "Sign in to your Blink account", 15, cWhite, semi)
	p.textC(x+w/2, y+116, "Blink's own sign-in page opens in your browser.", 12, cDim, reg)
	if !waiting {
		if p.button("login", x+24, y+156, w-48, 48, "Sign in with Blink", true) {
			p.be.StartLogin()
		}
	} else {
		p.textC(x+w/2, y+158, "Waiting for you to finish in the browser…", 12.5, cWhite, reg)
		if p.button("cancel", x+24, y+196, w-48, 44, "Cancel", false) {
			p.be.CancelLogin()
		}
		p.animating = true
	}
	col := map[bool]color.NRGBA{true: cRed, false: cDim}[strings.HasPrefix(status, "Error")]
	p.para(x+20, y+252, w-40, status, 11.5, col, reg, 3)
	p.WantH = min(p.MaxH, 410)
}

func (p *Panel) para(x, y, maxW float32, s string, sp float32, c color.NRGBA, w font.Weight, maxLines int) float32 {
	s = strings.TrimSpace(s)
	if s == "" {
		return y
	}
	words := strings.Fields(s)
	line := ""
	for _, word := range words {
		next := word
		if line != "" {
			next = line + " " + word
		}
		if p.textW(next, sp, w) > maxW && line != "" {
			p.text(x, y, line, sp, c, w)
			y += sp * 1.25
			line = word
			maxLines--
			if maxLines <= 0 {
				return y
			}
			continue
		}
		line = next
	}
	if line != "" {
		p.textFit(x, y, maxW, line, sp, c, w)
		y += sp * 1.25
	}
	return y
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
