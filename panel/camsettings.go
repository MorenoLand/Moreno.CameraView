package panel

import (
	"fmt"
	"image/color"
	"math"

	"gioui.org/f32"

	"moreno/blinkview/model"
)

const (
	viewCamSettings = iota + 100
	viewCamGeneral
	viewCamMotion
	viewCamVideo
	viewCamAudio
	viewCamPrivacy
	viewCamHelp
	viewZones
	viewSync
	viewClipPlay
)

func fnum(m map[string]any, k string) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0
}
func fbool(m map[string]any, k string) bool  { v, _ := m[k].(bool); return v }
func fstr(m map[string]any, k string) string { v, _ := m[k].(string); return v }
func fhas(m map[string]any, k string) bool   { _, ok := m[k]; return ok }

func (p *Panel) ShowCamSettings(id string) { p.view, p.sel, p.sheet = viewCamSettings, id, "" }

func (p *Panel) OpenSheet(id string) { p.sheet = id }

func (p *Panel) ShowClipPlayer(id string) { p.clipID, p.view = id, viewClipPlay }

func (p *Panel) ShowSync() { p.view, p.sel = viewSync, "" }

// backTarget is where the header back button goes from the camera settings pages.
func (p *Panel) backTarget() (int, bool) {
	switch p.view {
	case viewCamGeneral, viewCamMotion, viewCamVideo, viewCamAudio, viewCamPrivacy, viewCamHelp:
		return viewCamSettings, true
	case viewZones:
		return viewCamMotion, true
	case viewClipPlay:
		return viewClips, true
	case viewCamSettings, viewSync:
		return viewHome, true
	}
	return 0, false
}

func (p *Panel) icoMotion(cx, cy float32, c color.NRGBA) {
	p.disc(cx+2.5, cy-7, 2.2, c)
	p.line(cx+1, cy-3.5, cx-1.5, cy+2, 1.9, c)
	p.line(cx+1, cy-3.5, cx+6, cy-1, 1.9, c)
	p.line(cx+1, cy-3.5, cx-3.5, cy-1.5, 1.9, c)
	p.line(cx-1.5, cy+2, cx+2.5, cy+5, 1.9, c)
	p.line(cx+2.5, cy+5, cx+0.5, cy+9, 1.9, c)
	p.line(cx-1.5, cy+2, cx-5.5, cy+4.5, 1.9, c)
	p.line(cx-9, cy-3, cx-6.5, cy-3, 1.4, c)
	p.line(cx-10, cy+1, cx-7, cy+1, 1.4, c)
}

func (p *Panel) icoDots(cx, cy float32, c color.NRGBA) {
	for _, d := range []float32{-6, 0, 6} {
		p.disc(cx+d, cy, 1.8, c)
	}
}

func (p *Panel) icoClose(cx, cy float32, c color.NRGBA) {
	p.line(cx-5, cy-5, cx+5, cy+5, 1.8, c)
	p.line(cx+5, cy-5, cx-5, cy+5, 1.8, c)
}

func (p *Panel) icoCamera(cx, cy float32, c color.NRGBA) {
	p.strokeRR(cx-9, cy-5, 18, 13, 3, 1.6, c)
	p.ring(cx, cy+1.5, 3.4, 1.6, c)
	p.line(cx-4, cy-5, cx-2.5, cy-8, 1.6, c)
	p.line(cx-2.5, cy-8, cx+2.5, cy-8, 1.6, c)
	p.line(cx+2.5, cy-8, cx+4, cy-5, 1.6, c)
}

func (p *Panel) icoGear(cx, cy float32, c color.NRGBA) {
	p.ring(cx, cy, 3, 1.6, c)
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		p.line(cx+6*float32(math.Cos(a)), cy+6*float32(math.Sin(a)), cx+9*float32(math.Cos(a)), cy+9*float32(math.Sin(a)), 2.2, c)
	}
	p.ring(cx, cy, 6.5, 1.6, c)
}

func (p *Panel) icoSpeakerS(cx, cy float32, c color.NRGBA) { p.icoSpeaker(cx, cy, c, true) }

// camPill draws the motion, pin and menu buttons over a camera picture.
func (p *Panel) camPill(c model.Camera, x, y, w, h float32) {
	blink := c.Provider != "tapo"
	canPin := p.TogglePin != nil
	n := 1
	if blink {
		n++
	}
	if canPin {
		n++
	}
	pw := float32(n)*42 + 4
	px, py := x+w-pw-12, y+h-48
	p.fillRR(px, py, pw, 38, 19, rgba(24, 28, 40, 205))
	p.strokeRR(px, py, pw, 38, 19, 1, rgba(255, 255, 255, 18))
	bx := px + 2
	btn := func(tag string, f func(cx, cy float32)) bool {
		hv := p.hoverBtn(tag+c.ID, bx, py, 42, 38)
		p.fillRR(bx+3, py+4, 36, 30, 15, rgba(255, 255, 255, uint8(14*hv.v)))
		f(bx+21, py+19)
		bx += 42
		return hv.clicked
	}
	if blink {
		d := p.be.DeviceConfig(c.ID)
		col := cFaint
		if d.Loaded && d.Motion {
			col = cBlink
		}
		if btn("mot:", func(cx, cy float32) { p.icoMotion(cx, cy, col) }) && d.Loaded && c.Online {
			p.be.SetDeviceConfig(c.ID, "enabled", !d.Motion)
		}
	}
	if canPin {
		pinned := p.IsPinned != nil && p.IsPinned(c.ID)
		col := rgb(225, 228, 240)
		if pinned {
			col = rgb(110, 170, 255)
		}
		if btn("pin:", func(cx, cy float32) { p.icoPin(cx, cy, col, pinned) }) && c.Online {
			p.TogglePin(c.ID)
		}
	}
	if btn("dots:", func(cx, cy float32) { p.icoDots(cx, cy, rgb(225, 228, 240)) }) {
		p.sheet = c.ID
	}
}

// camSheet is the bottom sheet opened by a camera's menu button.
func (p *Panel) camSheet(cams []model.Camera) {
	t := p.ease("sheet", b2f(p.sheet != ""), 16)
	if t < 0.01 && p.sheet == "" {
		p.sheetID = ""
		return
	}
	if p.sheet != "" {
		p.sheetID = p.sheet
	}
	var c model.Camera
	found := false
	for _, x := range cams {
		if x.ID == p.sheetID {
			c, found = x, true
		}
	}
	if !found {
		p.sheet = ""
		return
	}
	blink := c.Provider != "tapo"
	sh := float32(150)
	if blink {
		sh = 264
	}
	p.WantH = min(p.MaxH, max(p.WantH, sh+60))
	p.fillRR(0, 0, p.w, p.h, 0, rgba(0, 0, 0, uint8(150*t)))
	if p.area("sheetscrim", 0, 0, p.w, p.h-sh*t).Clicked {
		p.sheet = ""
	}
	y := p.h - sh*t
	p.fillRR(10, y, p.w-20, sh+30, 20, rgb(30, 34, 46))
	p.strokeRR(10, y, p.w-20, sh+30, 20, 1, rgba(255, 255, 255, 16))
	p.area("sheetbody", 10, y, p.w-20, sh+30)
	p.textFit(30, y+18, p.w-120, c.Name, 17, cWhite, semi)
	sub := map[bool]string{true: "Online", false: "Offline"}[c.Online]
	if c.Battery != "" {
		sub += " | " + c.Battery
	}
	p.text(30, y+44, sub, 12, cDim, reg)
	cb := p.hoverBtn("sheetx", p.w-62, y+10, 40, 40)
	p.fillRR(p.w-62, y+10, 40, 40, 20, rgba(255, 255, 255, uint8(14*cb.v)))
	p.icoClose(p.w-42, y+30, rgb(225, 228, 240))
	if cb.clicked {
		p.sheet = ""
	}
	ry := y + 74
	if blink {
		d := p.be.DeviceConfig(c.ID)
		p.line(10, ry, p.w-10, ry, 1, rgba(255, 255, 255, 14))
		p.text(30, ry+16, "Motion Detection", 14.5, cWhite, semi)
		st := "Loading…"
		if d.Loaded {
			st = map[bool]string{true: "On", false: "Off"}[d.Motion]
		}
		p.text(30, ry+40, st, 12, cDim, reg)
		mb := p.hoverBtn("sheetmot", p.w-92, ry+12, 54, 54)
		on := d.Loaded && d.Motion
		p.fillRR(p.w-92, ry+12, 54, 54, 27, mix(rgb(48, 54, 72), cBlink, b2f(on)))
		p.icoMotion(p.w-65, ry+39, map[bool]color.NRGBA{true: rgb(8, 30, 22), false: rgb(210, 214, 232)}[on])
		if mb.clicked && d.Loaded && c.Online {
			p.be.SetDeviceConfig(c.ID, "enabled", !d.Motion)
		}
		ry += 78
	}
	p.line(10, ry, p.w-10, ry, 1, rgba(255, 255, 255, 14))
	rb := p.hoverBtn("sheetsnap", 10, ry, p.w-20, 56)
	p.fillRR(10, ry, p.w-20, 56, 0, rgba(255, 255, 255, uint8(8*rb.v)))
	p.icoCamera(40, ry+28, cBlink)
	p.text(66, ry+17, "Refresh Thumbnail", 14.5, cWhite, semi)
	if rb.clicked {
		if c.Online {
			p.be.Snapshot(c.ID)
		}
		p.sheet = ""
	}
	if blink {
		ry += 56
		p.line(66, ry, p.w-10, ry, 1, rgba(255, 255, 255, 14))
		db := p.hoverBtn("sheetdev", 10, ry, p.w-20, 56)
		p.fillRR(10, ry, p.w-20, 56, 0, rgba(255, 255, 255, uint8(8*db.v)))
		p.icoGear(40, ry+28, cBlink)
		p.text(66, ry+17, "Device Settings", 14.5, cWhite, semi)
		if db.clicked {
			p.ShowCamSettings(c.ID)
		}
	}
}

func (p *Panel) camByID(cams []model.Camera) (model.Camera, bool) {
	for _, c := range cams {
		if c.ID == p.sel {
			return c, true
		}
	}
	return model.Camera{}, false
}

// sliderRow draws a labelled slider; the value is sent when the drag ends.
func (p *Panel) sliderRow(tag string, y float32, label, sub string, v, lo, hi, step int, unit string, send func(int)) float32 {
	h := float32(86)
	p.glass(16, y, p.w-32, h, 16)
	p.text(32, y+12, label, 14, cWhite, semi)
	if sub != "" {
		p.textFit(32, y+32, p.w-64, sub, 11.5, cDim, reg)
	}
	tx, tw, ty := float32(32), p.w-64, y+h-24
	hit := p.area("sl:"+tag, tx-8, ty-14, tw+16, 28)
	if p.sliding == nil {
		p.sliding = map[string]int{}
	}
	cur, drag := p.sliding[tag]
	if hit.Down {
		f := clampf((hit.X-tx)/tw, 0, 1)
		n := float64(hi-lo) / float64(step)
		cur = lo + int(math.Round(float64(f)*n))*step
		p.sliding[tag] = cur
		drag = true
	}
	if !drag {
		cur = v
	}
	f := float32(cur-lo) / float32(max(hi-lo, 1))
	p.fillRR(tx, ty-2, tw, 4, 2, rgba(255, 255, 255, 40))
	p.fillRR(tx, ty-2, tw*f, 4, 2, cBlink)
	p.glow(tx+tw*f, ty, 14, cBlink, 0.4*b2f(hit.Down))
	p.disc(tx+tw*f, ty, 8+2*b2f(hit.Down), cBlink)
	p.textR(p.w-32, y+12, fmt.Sprint(cur, unit), 13, cWhite, semi)
	if drag && !hit.Down {
		delete(p.sliding, tag)
		if cur != v {
			send(cur)
		}
	}
	return y + h + 8
}

// choiceRow shows a value and runs f when clicked.
func (p *Panel) choiceRow(tag string, y float32, label, sub, value string, f func()) float32 {
	h := float32(60)
	ht := p.hoverRow("ch:"+tag, 16, y, p.w-32, h, 16)
	p.textFit(32, y+10, p.w-150, label, 14, cWhite, reg)
	p.textFit(32, y+33, p.w-150, sub, 11.5, cDim, reg)
	p.textR(p.w-32, y+20, value, 13, cDim, reg)
	if ht.Clicked && f != nil {
		f()
	}
	return y + h + 8
}

func (p *Panel) toggleCfg(y float32, id, key, label, sub string, d map[string]any) float32 {
	on := fbool(d, key)
	p.switchRow(key+id, y, label, sub, on, func() { p.be.SetDeviceConfig(id, key, !on) })
	if sub != "" {
		return y + 68
	}
	return y + 60
}

func (p *Panel) camSettingsView(cams []model.Camera) {
	if _, ok := p.camByID(cams); !ok {
		p.view = viewHome
		return
	}
	y, done := p.page("Device Settings", 6*70+20)
	defer done()
	for _, r := range []struct {
		title, sub string
		ico        func(float32, float32, color.NRGBA)
		v          int
	}{
		{"General Settings", "Device name, network status, firmware and more.", p.icoGear, viewCamGeneral},
		{"Motion Settings", "Motion detection, sensitivity, zones and more.", p.icoMotion, viewCamMotion},
		{"Video and Photo Settings", "Video quality, night vision and more.", p.icoFilm, viewCamVideo},
		{"Audio Settings", "Speaker volume and more.", p.icoSpeakerS, viewCamAudio},
		{"Privacy Settings", "Audio and video recording.", p.icoShield, viewCamPrivacy},
		{"Help", "Learn more about your device.", p.icoHelp, viewCamHelp},
	} {
		v := r.v
		p.menuRow("cs"+r.title, y, r.ico, r.title, r.sub, func() { p.view = v })
		y += 70
	}
}

// cfgPage opens a settings page and returns the camera's raw config.
func (p *Panel) cfgPage(cams []model.Camera, title string, content float32) (d map[string]any, id string, y float32, done func(), ok bool) {
	c, found := p.camByID(cams)
	if !found {
		p.view = viewHome
		return nil, "", 0, func() {}, false
	}
	p.be.LoadAccount()
	d = p.be.DeviceRaw(c.ID)
	y, done = p.page(title, content)
	if d == nil {
		p.animating = true
		p.text(34, y+4, "Loading settings…", 12, cDim, reg)
		return nil, c.ID, y, done, false
	}
	return d, c.ID, y, done, true
}

func (p *Panel) noteRow(y float32) {
	if s := p.be.SettingsNote(); s != "" {
		p.para(18, y, p.w-36, s, 11.5, cRed, reg, 3)
	}
}

func (p *Panel) camGeneralView(cams []model.Camera) {
	d, id, y, done, ok := p.cfgPage(cams, "General Settings", 300)
	defer done()
	if !ok {
		return
	}
	p.infoRow(y, "Name", fstr(d, "name"))
	y += 58
	p.infoRow(y, "Firmware", fstr(d, "fw_version"))
	y += 58
	strength := []string{"No signal", "Weak", "Fair", "Good", "Excellent"}
	w := int(fnum(d, "wifi"))
	p.infoRow(y, "Network Strength", fmt.Sprintf("%s (%d dBm)", strength[min(max(w, 0), 4)], int(fnum(d, "wifi_rssi"))))
	y += 58
	if fhas(d, "led_enabled") {
		y = p.toggleCfg(y, id, "led_enabled", "Status LED", "", d)
	}
	p.noteRow(y)
}

func (p *Panel) camMotionView(cams []model.Camera) {
	d, id, y, done, ok := p.cfgPage(cams, "Motion Settings", 470)
	defer done()
	if !ok {
		return
	}
	y = p.toggleCfg(y, id, "enabled", "Motion Detection", "Alert when movement is detected.", d)
	y = p.sliderRow("sens"+id, y, "Motion Sensitivity", "Adjust how easily motion is detected.", int(fnum(d, "motion_sensitivity")), 1, 9, 1, "", func(v int) { p.be.SetDeviceConfig(id, "motion_sensitivity", v) })
	if fhas(d, "motion_regions") {
		y = p.choiceRow("zones", y, "Motion Zones", "Customize detection to fit your home.", "", func() { p.view, p.zonesInit = viewZones, false })
	}
	y = p.sliderRow("retr"+id, y, "Retrigger Time", "Time after an event the camera will not detect motion.", int(fnum(d, "retrigger_time")), 10, 60, 1, "s", func(v int) { p.be.SetDeviceConfig(id, "retrigger_time", v) })
	if fbool(d, "early_notification_compatible") {
		y = p.toggleCfg(y, id, "early_notification", "Early Notification", "Notify as soon as motion is detected.", d)
	}
	p.noteRow(y)
}

func (p *Panel) camVideoView(cams []model.Camera) {
	d, id, y, done, ok := p.cfgPage(cams, "Video and Photo Settings", 620)
	defer done()
	if !ok {
		return
	}
	cm := max(int(fnum(d, "clip_length_max")), 10)
	y = p.sliderRow("clip"+id, y, "Motion Clip Length", "How long to record motion clips.", int(fnum(d, "clip_length")), 5, cm, 5, "s", func(v int) { p.be.SetDeviceConfig(id, "clip_length", v) })
	y = p.choiceRow("q", y, "Video Quality", "Video resolution of the camera.", fstr(d, "video_quality"), nil)
	if fbool(d, "early_termination_supported") {
		y = p.toggleCfg(y, id, "early_termination", "End Clip Early if Motion Stops", "", d)
	}
	if fbool(d, "flip_video_compatible") {
		y = p.toggleCfg(y, id, "flip_video", "Flip Video", "Enable when mounting upside down.", d)
	}
	nv := fstr(d, "illuminator_enable")
	next := map[string]string{"auto": "on", "on": "off", "off": "auto"}[nv]
	y = p.choiceRow("nv", y, "Night Vision", "Tap to change.", nv, func() {
		if next != "" {
			p.be.SetDeviceConfig(id, "illuminator_enable", next)
		}
	})
	y = p.sliderRow("ir"+id, y, "IR Intensity", "Infrared brightness for night vision.", int(fnum(d, "illuminator_intensity")), 1, 7, 1, "", func(v int) { p.be.SetDeviceConfig(id, "illuminator_intensity", v) })
	y = p.toggleCfg(y, id, "auto_update_thumbnail_enabled", "Auto-Update Thumbnail", "Update the home picture during events.", d)
	p.noteRow(y)
}

func (p *Panel) camAudioView(cams []model.Camera) {
	d, id, y, done, ok := p.cfgPage(cams, "Audio Settings", 240)
	defer done()
	if !ok {
		return
	}
	y = p.sliderRow("vol"+id, y, "Speaker Volume", "Adjust the speaker volume of your camera.", int(fnum(d, "volume_control")), 1, 10, 1, "", func(v int) { p.be.SetDeviceConfig(id, "volume_control", v) })
	if fbool(d, "chime_compatible") {
		y = p.sliderRow("chime"+id, y, "Camera as a Chime Volume", "Used when this camera chimes for a doorbell.", int(fnum(d, "chime_volume")), 1, 10, 1, "", func(v int) { p.be.SetDeviceConfig(id, "chime_volume", v) })
	}
	p.noteRow(y)
}

func (p *Panel) camPrivacyView(cams []model.Camera) {
	d, id, y, done, ok := p.cfgPage(cams, "Privacy Settings", 200)
	defer done()
	if !ok {
		return
	}
	if fbool(d, "video_recording_optional") {
		y = p.toggleCfg(y, id, "video_recording_enable", "Video Recording", "Turn off to disable video recording.", d)
	}
	y = p.toggleCfg(y, id, "record_audio_enable", "Record Audio", "Turn off to disable audio in clips.", d)
	p.noteRow(y)
}

func (p *Panel) camHelpView() {
	y, done := p.page("Help", 220)
	defer done()
	for i, t := range []string{"Learn more about your device", "Contact Support"} {
		p.menuRow(fmt.Sprint("help", i), y, p.icoHelp, t, "Opens the Blink support site", p.be.OpenHelp)
		y += 70
	}
}

// zonesView edits the 5x5 motion-zone grid stored as a bitmask.
func (p *Panel) zonesView(cams []model.Camera) {
	d, id, y, done, ok := p.cfgPage(cams, "Motion Zones", 440)
	defer done()
	if !ok {
		return
	}
	if !p.zonesInit {
		p.zones, p.zonesInit = uint32(fnum(d, "motion_regions")), true
	}
	gx, gw := float32(16), p.w-32
	gh := gw * 9 / 16
	p.fillRR(gx, y, gw, gh, 10, rgb(14, 18, 32))
	if o, ok := p.thumbOp(id); ok {
		p.cover(o, gx, y, gw, gh, 10)
	}
	cw, ch := gw/5, gh/5
	for i := 0; i < 25; i++ {
		cx, cy := gx+float32(i%5)*cw, y+float32(i/5)*ch
		if p.zones&(1<<uint(i)) == 0 {
			p.fillRR(cx, cy, cw, ch, 0, rgba(0, 200, 128, 120))
		}
		if p.area(fmt.Sprint("zone", i), cx, cy, cw, ch).Clicked {
			p.zones ^= 1 << uint(i)
		}
		p.strokeRR(cx, cy, cw, ch, 0, 1, rgba(255, 255, 255, 120))
	}
	y += gh + 16
	if p.button("zsave", 16, y, (p.w-40)/2, 44, "Save", true) {
		p.be.SetDeviceConfig(id, "motion_regions", int(p.zones))
		p.view = viewCamMotion
	}
	if p.button("zreset", 24+(p.w-40)/2, y, (p.w-40)/2, 44, "Reset zones", false) {
		p.zones = 1<<25 - 1
	}
	y += 62
	p.para(18, y, p.w-36, "Tap squares to ignore motion in that part of the picture. Green squares are ignored.", 12, cDim, reg, 3)
}

func (p *Panel) syncView() {
	p.be.LoadAccount()
	name, status, warn := p.be.SyncStatus()
	in := p.be.SyncInfo()
	y, done := p.page("Sync Module", 560)
	defer done()
	col := cBlink
	if warn {
		col = cRed
	}
	p.glass(16, y, p.w-32, 90, 16)
	p.disc(40, y+45, 6, col)
	p.text(58, y+22, name+": "+status, 16, cWhite, semi)
	p.text(58, y+48, "The hub of your system, so a good Wi-Fi link matters.", 11.5, cDim, reg)
	y += 106
	if in.Firmware != "" {
		p.infoRow(y, "Firmware Version", in.Firmware)
		y += 58
	}
	if in.Wifi > 0 {
		p.infoRow(y, "Wi-Fi Signal", fmt.Sprintf("%d of 5", min(in.Wifi, 5)))
		y += 58
	}
	if in.StorageKnown && in.StorageOn {
		p.glass(16, y, p.w-32, 96, 16)
		cx, cy := float32(58), y+48
		p.ring(cx, cy, 26, 5, rgba(255, 255, 255, 30))
		if in.StorageUsed > 0 {
			end := -math.Pi/2 + 2*math.Pi*float64(min(in.StorageUsed, 100))/100
			p.strokePoly(p.arc(float64(cx), float64(cy), 26, -math.Pi/2, end), 5, mix(cBlink, cRed, b2f(in.Full)), false)
		}
		p.textC(cx, cy-10, fmt.Sprintf("%d%%", in.StorageUsed), 15, cWhite, semi)
		p.text(104, y+22, "Storage Used", 14.5, cWhite, semi)
		p.para(104, y+46, p.w-136, "To free up space, delete clips or format the storage device.", 11.5, cDim, reg, 2)
		y += 112
		if in.BackupOn {
			v := "On"
			if !in.LastBackup.IsZero() {
				v = ago(in.LastBackup)
			}
			p.infoRow(y, "Last Backup", v)
			y += 58
		}
	} else if in.StorageState != "" {
		p.infoRow(y, "Local Storage", in.StorageState)
		y += 58
	}
	p.menuRow("synchelp", y, p.icoHelp, "Get Help", "Opens the Blink support site", p.be.OpenHelp)
}

const clipWatch = "clip:"

const (
	clipIdle = iota
	clipPlaying
	clipPaused
	clipEnded
)

func clock(sec float64) string {
	s := int(sec + 0.5)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// clipPlayView plays a saved clip inside the panel with pause and seek.
func (p *Panel) clipPlayView() {
	var c model.Clip
	found := false
	for _, x := range p.be.Clips() {
		if x.ID == p.clipID {
			c, found = x, true
		}
	}
	if !found {
		p.view = viewClips
		return
	}
	id := clipWatch + c.ID
	playing := p.watchID == id && p.watchActive
	pos, dur := p.be.ClipPos(c.ID)
	switch {
	case playing:
		p.clipState = clipPlaying
		if p.watchImg != nil {
			p.clipLast, p.clipLastRev = p.watchImg, p.watchRev
		}
	case p.clipState == clipPlaying:
		p.clipState = clipEnded
		if p.be.WatchError(id) != "" {
			p.clipState = clipIdle
		}
	}
	shown := pos
	switch p.clipState {
	case clipPaused:
		shown = p.clipPausePos
	case clipEnded:
		shown = dur
	case clipIdle:
		shown = 0
	}
	w := p.w - 32
	h := w * 9 / 16
	x, y := float32(16), float32(84)
	p.glow(x+w/2, y+h/2, w*0.75, cBlink, 0.15)
	p.fillRR(x, y, w, h, 20, rgb(14, 18, 32))
	if p.clipLast != nil {
		if o, ok := p.imageOp("clip:"+c.ID, p.clipLast, p.clipLastRev); ok {
			p.cover(o, x, y, w, h, 20)
		}
	}
	p.strokeRR(x, y, w, h, 20, 1.2, rgba(255, 255, 255, 20))
	big := func() {
		p.fillRR(x, y, w, h, 20, rgba(6, 9, 20, 110))
		p.disc(x+w/2, y+h/2, 28, rgba(0, 0, 0, 150))
		if p.clipState == clipEnded {
			p.icoRefresh(x+w/2, y+h/2, cWhite)
		} else {
			p.fillPoly([]f32.Point{p.pt(x+w/2-7, y+h/2-12), p.pt(x+w/2+13, y+h/2), p.pt(x+w/2-7, y+h/2+12)}, cWhite)
		}
	}
	switch {
	case playing && p.watchImg == nil && p.clipLast == nil:
		p.animating = true
		p.fillRR(x, y, w, h, 20, rgba(6, 9, 20, 155))
		p.textC(x+w/2, y+h/2-8, p.watchNote, 14, cWhite, semi)
	case playing && p.watchImg == nil:
		p.animating = true
	case !playing:
		big()
	}
	toggle := func() {
		switch p.clipState {
		case clipPlaying:
			p.clipPausePos = pos
			p.clipState = clipPaused
			p.be.StopWatch()
		case clipPaused:
			p.clipState = clipPlaying
			p.be.ClipPlay(c.ID, p.clipPausePos)
		case clipEnded:
			p.clipState, p.clipLast = clipPlaying, nil
			p.be.ClipPlay(c.ID, 0)
		default:
			p.clipState = clipPlaying
			p.be.ClipPlay(c.ID, 0)
		}
	}
	if p.area("clipbig", x, y, w, h).Clicked {
		toggle()
	}
	if playing && p.watchImg != nil {
		p.fillRR(x+12, y+12, 38, 30, 12, rgba(0, 0, 0, 145))
		p.icoSpeaker(x+31, y+27, cWhite, !p.muted)
		if p.area("clipmute", x+12, y+12, 38, 30).Clicked {
			p.be.SetMuted(!p.muted)
		}
	}
	// seek bar
	y += h + 18
	if dur > 0 {
		tx, tw := x+2, w-4
		hit := p.area("clipseek", tx-8, y-14, tw+16, 30)
		if hit.Down {
			p.clipSeeking = true
			p.clipSeek = float64(clampf((hit.X-tx)/tw, 0, 1)) * dur
		}
		if p.clipSeeking {
			shown = p.clipSeek
			if !hit.Down {
				p.clipSeeking = false
				p.clipState = clipPlaying
				p.be.ClipPlay(c.ID, p.clipSeek)
			}
		}
		f := float32(clampf(float32(shown/dur), 0, 1))
		p.fillRR(tx, y-2, tw, 4, 2, rgba(255, 255, 255, 40))
		p.fillRR(tx, y-2, tw*f, 4, 2, cBlink)
		p.disc(tx+tw*f, y, 7+2*b2f(hit.Down), cBlink)
		p.text(x+2, y+14, clock(shown), 11.5, cDim, reg)
		p.textR(x+w-2, y+14, clock(dur), 11.5, cDim, reg)
		y += 32
	}
	p.text(x, y+8, c.Camera, 19, cWhite, semi)
	p.text(x, y+36, c.At.Format("Monday, January 2 · 3:04 PM")+" · "+c.Kind, 12.5, cDim, reg)
	by := y + 66
	bw := (w - 8) / 2
	label := map[int]string{clipIdle: "Play", clipPlaying: "Pause", clipPaused: "Resume", clipEnded: "Replay"}[p.clipState]
	if p.button("clipplay", x, by, bw, 44, label, true) {
		toggle()
	}
	if p.button("clipweb", x+bw+8, by, bw, 44, "Open in browser", false) {
		p.be.OpenClip(c.ID)
	}
	p.WantH = min(p.MaxH, by+68)
	if e := p.be.WatchError(id); e != "" && !playing {
		p.WantH = min(p.MaxH, max(p.WantH, p.para(x, by+58, w, "Error: "+e, 12, cRed, reg, 3)+14))
	}
}
