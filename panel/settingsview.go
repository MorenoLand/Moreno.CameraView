package panel

import (
	"fmt"
	"image/color"
	"strings"

	"gioui.org/f32"

	"moreno/blinkview/model"
)

var notifyLabels = []struct{ key, label, sub string }{
	{"motion", "Motion detected", "Alerts when a camera sees movement"},
	{"cv_motion", "Smart motion alerts", "Alerts for people and objects"},
	{"doorbell", "Doorbell presses", "Alerts when someone rings"},
	{"camera_offline", "Camera offline", "A camera lost its connection"},
	{"sync_module_offline", "Sync Module offline", "The Sync Module lost its connection"},
	{"low_battery", "Low battery", "A camera battery is running low"},
	{"battery_dead", "Battery depleted", "A camera battery has run out"},
	{"wifi", "Wi-Fi signal", "A camera has a weak Wi-Fi signal"},
	{"local_storage", "Local storage", "Sync Module storage alerts"},
	{"scheduling", "Schedules", "Arming schedule alerts"},
	{"camera_usage", "Camera usage", "Usage summaries"},
	{"temperature", "Temperature", "Temperature alerts"},
	{"bandwidth", "Bandwidth", "Network bandwidth alerts"},
	{"accessory_connected", "Accessory connected", ""},
	{"accessory_disconnected", "Accessory disconnected", ""},
	{"accessory_low_battery", "Accessory low battery", ""},
	{"general", "General news", "Product and service updates"},
}

func (p *Panel) inSettings() bool {
	switch p.view {
	case viewSettings, viewAPI, viewAccount, viewDevices, viewNotify, viewSources:
		return true
	}
	return false
}

func (p *Panel) page(title string, content float32) (float32, func()) {
	top := float32(78)
	p.text(16, top, title, 18, cWhite, semi)
	y0 := top + 38
	vh := p.h - y0
	p.WantH = min(p.MaxH, y0+content+14)
	p.scroll = clampf(p.scroll-p.wheel("pagewheel", 0, 0, p.w, p.h)*1.2, 0, max(0, content+14-vh))
	sc := p.ease("scroll", p.scroll, 18)
	pop := p.clipTo(0, y0, p.w, vh)
	return y0 - sc, pop.Pop
}

func (p *Panel) hoverRow(tag string, x, y, w, h, r float32) hit {
	ht := p.area(tag, x, y, w, h)
	hv := p.ease("rowh:"+tag, b2f(ht.Hover), 14)
	p.fillRR(x, y, w, h, r, mix(rgba(255, 255, 255, 6), rgba(255, 255, 255, 12), hv))
	p.strokeRR(x, y, w, h, r, 1, mix(rgba(255, 255, 255, 10), rgba(0, 200, 128, 70), hv))
	return ht
}

func (p *Panel) menuRow(tag string, y float32, icon func(float32, float32, color.NRGBA), title, sub string, f func()) {
	ht := p.hoverRow("menu:"+tag, 16, y, p.w-32, 62, 16)
	p.fillRR(30, y+13, 36, 36, 11, rgba(0, 200, 128, 28))
	icon(48, y+31, cBlink)
	p.textFit(80, y+11, p.w-80-48, title, 14, cWhite, semi)
	p.textFit(80, y+34, p.w-80-48, sub, 11.5, cDim, reg)
	p.strokePoly([]f32.Point{p.pt(p.w-38, y+25), p.pt(p.w-32, y+31), p.pt(p.w-38, y+37)}, 1.8, cDim, false)
	if ht.Clicked {
		f()
	}
}

func (p *Panel) switchRow(tag string, y float32, label, sub string, on bool, f func()) {
	h := float32(52)
	if sub != "" {
		h = 60
	}
	ht := p.hoverRow("sw:"+tag, 16, y, p.w-32, h, 16)
	ty := y + (h-18)/2 - 1
	if sub != "" {
		ty = y + 10
		p.textFit(32, y+33, p.w-120, sub, 11.5, cDim, reg)
	}
	p.textFit(32, ty, p.w-120, label, 14, cWhite, reg)
	t := p.ease("swt:"+tag, b2f(on), 14)
	p.fillRR(p.w-76, y+h/2-13, 44, 26, 13, mix(rgb(52, 58, 86), cBlink, t))
	p.disc(p.w-63+t*18, y+h/2, 10, cWhite)
	if ht.Clicked {
		f()
	}
}

func (p *Panel) stepRow(tag string, y float32, label, value string, dec, inc func()) {
	p.glass(16, y, p.w-32, 52, 16)
	p.text(32, y+16, label, 14, cWhite, reg)
	cx := p.w - 120
	for i, sg := range []string{"–", "+"} {
		bx := cx + float32(i)*70 - 12
		b := p.hoverBtn(fmt.Sprint("st:", tag, i), bx, y+11, 30, 30)
		p.fillRR(bx, y+11, 30, 30, 15, mix(rgba(255, 255, 255, 8), rgba(0, 200, 128, 60), b.v))
		p.textC(bx+15, y+15, sg, 15, cWhite, semi)
		if b.clicked {
			if i == 0 {
				dec()
			} else {
				inc()
			}
		}
	}
	p.textC(cx+38, y+16, value, 13.5, cWhite, semi)
}

type hb struct {
	v       float32
	clicked bool
}

func (p *Panel) hoverBtn(tag string, x, y, w, h float32) hb {
	ht := p.area(tag, x, y, w, h)
	return hb{p.ease("hb:"+tag, b2f(ht.Hover), 14), ht.Clicked}
}

func (p *Panel) infoRow(y float32, label, value string) {
	p.glass(16, y, p.w-32, 50, 16)
	p.text(32, y+15, label, 13.5, cDim, reg)
	p.textR(p.w-32, y+15, value, 13.5, cWhite, reg)
}

func (p *Panel) section(y float32, s string) {
	p.text(20, y, strings.ToUpper(s), 10.5, cFaint, semi)
}

func (p *Panel) settingsView() {
	p.be.LoadAccount()
	rows := []struct {
		tag, title, sub string
		icon            func(float32, float32, color.NRGBA)
		view            int
	}{
		{"src", "Cameras and accounts", "Blink account and Tapo cameras", p.icoFilm, viewSources},
		{"acct", "Account and Privacy", "Account details and sign out", p.icoUser, viewAccount},
		{"dev", "Device and System Settings", "Motion detection, clip length and more", p.icoSliders, viewDevices},
		{"note", "Notifications", "Choose which alerts you get", p.icoBell, viewNotify},
	}
	content := float32(len(rows))*70 + 28 + 54*4 + 70 + 110
	y, done := p.page("Settings", content)
	defer done()
	for _, r := range rows {
		v := r.view
		p.menuRow(r.tag, y, r.icon, r.title, r.sub, func() { p.view = v })
		y += 70
	}
	p.menuRow("help", y, p.icoHelp, "Help", "Customer support and other help", p.be.OpenHelp)
	y += 82
	p.section(y-4, "CameraView")
	y += 18
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
	p.buttonRow(y+162, "Quit", cWhite, p.be.Quit)
	p.para(18, y+226, p.w-36, "CameraView stays in the tray. Click the tray icon to open or hide this panel.", 11.5, cFaint, reg, 3)
}

func (p *Panel) accountView() {
	pr := p.be.Profile()
	content := float32(150 + 3*58 + 74)
	y, done := p.page("Account and Privacy", content)
	defer done()
	p.fillRR(p.w/2-30, y+4, 60, 60, 30, rgba(0, 200, 128, 28))
	p.strokeRR(p.w/2-30, y+4, 60, 60, 30, 1.4, rgba(0, 200, 128, 120))
	p.icoUser(p.w/2, y+34, cBlink)
	email := pr.Email
	if email == "" {
		email = "Loading account…"
		p.animating = true
	}
	p.textC(p.w/2, y+76, email, 15, cWhite, semi)
	if pr.Name != "" && pr.Name != pr.Email {
		p.textC(p.w/2, y+100, pr.Name, 12.5, cDim, reg)
	}
	y += 130
	p.section(y, "Account")
	y += 22
	for _, r := range [][2]string{{"Phone number", pr.Phone}, {"Country", strings.ToUpper(pr.Country)}, {"Time zone", pr.TimeZone}} {
		v := r[1]
		if v == "" {
			v = "—"
		}
		p.infoRow(y, r[0], v)
		y += 58
	}
	y += 8
	p.buttonRow(y, "Sign out", rgb(255, 150, 130), func() {
		p.be.Signout()
		p.view, p.sel = viewHome, ""
	})
}

func (p *Panel) notifyView() {
	n := p.be.Notifications()
	if len(n) == 0 {
		p.animating = true
		y, done := p.page("Notifications", 80)
		defer done()
		p.textC(p.w/2, y+30, "Loading notification settings…", 13, cDim, reg)
		return
	}
	var shown []struct{ key, label, sub string }
	for _, nl := range notifyLabels {
		if _, ok := n[nl.key]; ok {
			shown = append(shown, nl)
		}
	}
	content := float32(len(shown))*68 + 36
	y, done := p.page("Notifications", content)
	defer done()
	p.para(18, y, p.w-36, "These switches control the alerts Blink sends for your whole account.", 11.5, cFaint, reg, 2)
	y += 30
	for _, nl := range shown {
		k := nl.key
		p.switchRow("n:"+k, y, nl.label, nl.sub, n[k], func() { p.be.SetNotification(k, !n[k]) })
		y += 68
	}
	if s := p.be.SettingsNote(); s != "" {
		p.textC(p.w/2, y+4, s, 11.5, cRed, reg)
	}
}

func (p *Panel) devicesView(cams []model.Camera) {
	rows := 0
	for _, c := range cams {
		if c.Provider == "tapo" {
			continue
		}
		rows += 1
		if d := p.be.DeviceConfig(c.ID); d.Loaded {
			rows += 2
			if d.HasAudio {
				rows++
			}
			if d.HasLED {
				rows++
			}
		}
	}
	content := float32(len(cams))*52 + float32(rows)*58 + 80
	y, done := p.page("Device and System Settings", content)
	defer done()
	if len(cams) == 0 {
		p.textC(p.w/2, y+30, "No devices found", 13, cDim, reg)
		return
	}
	for _, c := range cams {
		if c.Provider == "tapo" {
			continue
		}
		d := p.be.DeviceConfig(c.ID)
		p.disc(24, y+9, 3.5, map[bool]color.NRGBA{true: cBlink, false: cFaint}[c.Online])
		p.text(34, y, c.Name, 14, cWhite, semi)
		meta := map[bool]string{true: "Online", false: "Offline"}[c.Online]
		if d.Firmware != "" {
			meta += " · firmware " + d.Firmware
		}
		p.textR(p.w-20, y+2, meta, 11.5, cDim, reg)
		y += 26
		if !d.Loaded {
			p.animating = true
			p.text(34, y+4, "Loading settings…", 12, cDim, reg)
			y += 40
			continue
		}
		id := c.ID
		p.switchRow("m:"+id, y, "Motion detection", "", d.Motion, func() { p.be.SetDeviceConfig(id, "enabled", !d.Motion) })
		y += 58
		sens := d.Sensitivity
		p.stepRow("s:"+id, y, "Motion sensitivity", fmt.Sprint(sens), func() { p.be.SetDeviceConfig(id, "motion_sensitivity", max(1, sens-1)) }, func() { p.be.SetDeviceConfig(id, "motion_sensitivity", min(9, sens+1)) })
		y += 58
		cl, cm := d.ClipLen, max(d.ClipMax, 5)
		p.stepRow("c:"+id, y, "Clip length", fmt.Sprintf("%ds", cl), func() { p.be.SetDeviceConfig(id, "clip_length", max(5, cl-5)) }, func() { p.be.SetDeviceConfig(id, "clip_length", min(cm, cl+5)) })
		y += 58
		if d.HasAudio {
			p.switchRow("a:"+id, y, "Record audio", "", d.Audio, func() { p.be.SetDeviceConfig(id, "record_audio_enable", !d.Audio) })
			y += 58
		}
		if d.HasLED {
			p.switchRow("l:"+id, y, "Status LED", "", d.LED, func() { p.be.SetDeviceConfig(id, "led_enabled", !d.LED) })
			y += 58
		}
		y += 14
	}
	if s := p.be.SettingsNote(); s != "" {
		p.para(18, y, p.w-36, s, 11.5, cRed, reg, 3)
	}
}

func (p *Panel) icoUser(cx, cy float32, c color.NRGBA) {
	p.ring(cx, cy-3.5, 4.2, 1.6, c)
	p.strokePoly(p.arc(float64(cx), float64(cy+10), 8, 3.4, 6.02), 1.6, c, false)
}

func (p *Panel) icoBell(cx, cy float32, c color.NRGBA) {
	p.strokePoly(append(p.arc(float64(cx), float64(cy-1), 6.5, 3.14159, 6.28318), p.pt(cx+6.5, cy+5), p.pt(cx+8, cy+7), p.pt(cx-8, cy+7), p.pt(cx-6.5, cy+5)), 1.6, c, true)
	p.line(cx-2.5, cy+10, cx+2.5, cy+10, 1.6, c)
}

func (p *Panel) icoHelp(cx, cy float32, c color.NRGBA) {
	p.ring(cx, cy, 8.5, 1.6, c)
	p.strokePoly(p.arc(float64(cx), float64(cy-2.5), 3.2, 3.4, 7.2), 1.6, c, false)
	p.line(cx, cy+1, cx, cy+2.6, 1.6, c)
	p.disc(cx, cy+5.4, 1.1, c)
}

func (p *Panel) ShowAccount() { p.view, p.sel = viewAccount, "" }

func (p *Panel) ShowDevices() { p.view, p.sel = viewDevices, "" }

func (p *Panel) ShowNotify() { p.view, p.sel = viewNotify, "" }

func (p *Panel) ShowSources() { p.view, p.sel = viewSources, "" }
