package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"moreno/blinkview/blinkcloud"
	"moreno/blinkview/model"
)

func (l *live) LoadAccount() {
	l.mu.Lock()
	if l.in == false || time.Since(l.accAt) < 20*time.Second {
		l.mu.Unlock()
		return
	}
	l.accAt = time.Now()
	h := l.home
	l.mu.Unlock()
	go func() {
		if p, err := l.cl.User(); err == nil {
			l.mu.Lock()
			l.prof = p
			l.mu.Unlock()
		}
		if n, err := l.cl.Notifications(); err == nil {
			l.mu.Lock()
			l.notify = n
			l.mu.Unlock()
		}
		if h != nil && len(h.Syncs) > 0 {
			if st, err := l.cl.SyncStorage(h.Syncs[0]); err == nil {
				l.mu.Lock()
				l.syncSt, l.syncStOK = st, true
				l.mu.Unlock()
			}
		}
		if h != nil {
			for _, c := range h.Cameras {
				l.loadConfig(c)
			}
		}
		l.invalidate()
	}()
}

func (l *live) loadConfig(c blinkcloud.Camera) {
	cfg, err := l.cl.CameraConfig(c)
	if err != nil {
		return
	}
	l.mu.Lock()
	if l.cfgs == nil {
		l.cfgs = map[string]map[string]any{}
	}
	l.cfgs[c.ID] = cfg
	l.mu.Unlock()
}

func (l *live) Profile() model.Profile {
	l.mu.Lock()
	defer l.mu.Unlock()
	return model.Profile{Email: l.prof.Email, Name: l.prof.Name, Phone: l.prof.Phone, Country: l.prof.Country, TimeZone: l.prof.TimeZone}
}

func (l *live) Notifications() map[string]bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]bool, len(l.notify))
	for k, v := range l.notify {
		out[k] = v
	}
	return out
}

func (l *live) note(s string) {
	l.mu.Lock()
	l.setNote = s
	l.mu.Unlock()
	l.invalidate()
}

func (l *live) SettingsNote() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.setNote
}

func (l *live) SetNotification(key string, on bool) {
	l.mu.Lock()
	if l.notify == nil {
		l.mu.Unlock()
		return
	}
	l.notify[key] = on
	all := make(map[string]bool, len(l.notify))
	for k, v := range l.notify {
		all[k] = v
	}
	l.setNote = ""
	l.mu.Unlock()
	l.invalidate()
	go func() {
		if err := l.cl.SetNotifications(all); err != nil {
			l.note("Could not save: " + err.Error())
			l.mu.Lock()
			l.accAt = time.Time{}
			l.mu.Unlock()
			l.LoadAccount()
		}
	}()
}

func num(v any) int { f, _ := v.(float64); return int(f) }

func (l *live) DeviceConfig(id string) model.DeviceConfig {
	l.mu.Lock()
	defer l.mu.Unlock()
	m := l.cfgs[id]
	if m == nil {
		return model.DeviceConfig{}
	}
	d := model.DeviceConfig{Loaded: true, Sensitivity: num(m["motion_sensitivity"]), ClipLen: num(m["clip_length"]), ClipMax: num(m["clip_length_max"]), Quality: str(m["video_quality"]), Firmware: str(m["fw_version"])}
	d.Motion, _ = m["enabled"].(bool)
	if v, ok := m["record_audio_enable"].(bool); ok {
		d.Audio, d.HasAudio = v, true
	}
	if v, ok := m["led_enabled"].(bool); ok {
		d.LED, d.HasLED = v, true
	}
	return d
}

func str(v any) string { s, _ := v.(string); return s }

func (l *live) SetDeviceConfig(id, key string, v any) {
	c, ok := l.findCamera(id)
	if !ok {
		return
	}
	l.mu.Lock()
	if l.cfgs[id] != nil {
		l.cfgs[id][key] = v
	}
	l.setNote = ""
	l.mu.Unlock()
	l.invalidate()
	go func() {
		if err := l.cl.SetCameraConfig(c, map[string]any{key: v}); err != nil {
			msg := err.Error()
			if i := strings.Index(msg, `"message":"`); i >= 0 {
				msg = msg[i+11:]
				if j := strings.Index(msg, `"`); j >= 0 {
					msg = msg[:j]
				}
			}
			l.note(c.Name + ": " + msg)
		}
		l.loadConfig(c)
		l.invalidate()
	}()
}

func (l *live) OpenHelp() {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", "https://support.blinkforhome.com/")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
}

var _ = fmt.Sprint

func (l *live) DeviceRaw(id string) map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	m := l.cfgs[id]
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (l *live) SyncInfo() model.SyncInfo {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.home == nil || len(l.home.Syncs) == 0 {
		return model.SyncInfo{}
	}
	s := l.home.Syncs[0]
	in := model.SyncInfo{Name: "Sync Module", Status: s.Status, Firmware: s.Firmware, Wifi: s.Wifi, StorageState: s.Storage, Full: s.Full}
	if l.syncStOK {
		st := l.syncSt
		in.StorageOn, in.StorageUsed, in.StorageKnown, in.Full = st.Enabled, st.Used, true, st.Full
		in.BackupOn, in.BackupResult, in.LastBackup = st.BackupOn, st.LastResult, st.LastBackup
		if st.State != "" {
			in.StorageState = st.State
		}
	}
	return in
}
