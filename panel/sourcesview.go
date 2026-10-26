package panel

import "strings"

func (p *Panel) sourcesView() {
	list := p.be.TapoList()
	found, finding := p.be.TapoFound()
	blink := p.be.BlinkSignedIn()
	content := float32(130 + len(list)*66 + 60 + 60 + len(found)*52 + 4*58 + 130)
	y, done := p.page("Cameras and accounts", content)
	defer done()
	p.section(y, "Blink")
	y += 22
	if blink {
		p.infoRow(y, "Blink account", "Signed in")
	} else {
		status, waiting := p.be.LoginState()
		if waiting {
			p.infoRow(y, "Blink account", "Finish in your browser…")
			p.animating = true
		} else {
			p.buttonRow(y, "Sign in to Blink", cBlink, p.be.StartLogin)
		}
		if strings.HasPrefix(status, "Error") {
			p.text(20, y+54, status, 11.5, cRed, reg)
		}
	}
	y += 74
	p.section(y, "Tapo cameras")
	y += 22
	for _, t := range list {
		p.glass(16, y, p.w-32, 58, 16)
		p.textFit(32, y+10, p.w-130, t.Name, 14, cWhite, semi)
		sub := t.Host
		if e := p.be.TapoError(t.ID); e != "" {
			sub += " · " + e
		}
		col := cDim
		if p.be.TapoError(t.ID) != "" {
			col = cRed
		}
		p.textFit(32, y+33, p.w-130, sub, 11.5, col, reg)
		id := t.ID
		bx := p.w - 16 - 80
		if p.button("rm:"+id, bx, y+12, 68, 34, "Remove", false) {
			p.be.TapoRemove(id)
		}
		y += 66
	}
	if len(list) == 0 {
		p.text(20, y+2, "No Tapo cameras added yet.", 12.5, cDim, reg)
		y += 30
	}
	label := "Find Tapo cameras on this network"
	if finding {
		label = "Searching…"
		p.animating = true
	}
	if p.button("find", 16, y, p.w-32, 44, label, false) && !finding {
		p.be.TapoFind()
	}
	y += 54
	for _, f := range found {
		p.glass(16, y, p.w-32, 44, 14)
		name := f.Host
		if f.Model != "" {
			name = f.Model + " · " + f.Host
		}
		p.textFit(30, y+13, p.w-130, name, 13, cWhite, reg)
		h := f.Host
		if p.button("use:"+h, p.w-16-72, y+6, 60, 32, "Use", false) {
			p.edHost.SetText(h)
		}
		y += 52
	}
	p.section(y+4, "Add a Tapo camera")
	y += 28
	p.field(&p.edName, 16, y, p.w-32, 46, "Name (optional)", 0)
	p.field(&p.edHost, 16, y+54, p.w-32, 46, "Camera IP address", 0)
	fw := (p.w - 32 - 8) / 2
	p.field(&p.edTUser, 16, y+108, fw, 46, "Camera account", 0)
	p.field(&p.edTPass, 24+fw, y+108, fw, 46, "Password", '•')
	if p.button("tapo-add", 16, y+164, p.w-32, 46, "Add camera", true) {
		if err := p.be.TapoAdd(p.edName.Text(), p.edHost.Text(), p.edTUser.Text(), p.edTPass.Text()); err != nil {
			p.srcMsg = err.Error()
		} else {
			p.srcMsg = ""
			p.edName.SetText("")
			p.edHost.SetText("")
			p.edTPass.SetText("")
		}
	}
	if p.srcMsg != "" {
		p.textC(p.w/2, y+222, p.srcMsg, 12, cRed, reg)
	}
	p.para(18, y+246, p.w-36, "Use the Camera Account from the Tapo app (camera Settings, Advanced Settings, Camera Account). The camera must be on this network; the app talks to it directly.", 11.5, cFaint, reg, 3)
}
