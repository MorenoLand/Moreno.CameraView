package devshots

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"moreno/blinkview/model"
	"moreno/blinkview/panel"
	"moreno/blinkview/store"
)

type demo struct{}

func scene(top, mid, bot color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 640, 360))
	for y := 0; y < 360; y++ {
		for x := 0; x < 640; x++ {
			t := float64(y) / 360
			c := top
			switch {
			case t > 0.55:
				c = bot
			case t > 0.35:
				c = mid
			}
			n := uint8((x*7+y*13)%9) * 2
			img.SetNRGBA(x, y, color.NRGBA{c.R + n, c.G + n, c.B + n, 255})
		}
	}
	return img
}

func (demo) LoggedIn() bool             { return true }
func (demo) LoginState() (string, bool) { return "", false }
func (demo) StartLogin()                {}
func (demo) CancelLogin()               {}
func (demo) HomeName() string           { return "My Home" }
func (demo) Cameras() []model.Camera {
	now := time.Now()
	return []model.Camera{
		{ID: "a", Name: "Backyard", Kind: "camera", Online: true, Enabled: true, Battery: "Battery 4/5", Seen: now.Add(-2 * time.Minute)},
		{ID: "b", Name: "Porch", Kind: "camera", Online: true, Enabled: true, Battery: "Battery 3/5", Seen: now.Add(-12 * time.Minute)},
		{ID: "c", Name: "Frontdoor", Kind: "doorbell", Online: false, Enabled: true, Battery: "Battery 5/5", Seen: now.Add(-3 * time.Hour)},
	}
}
func (demo) Thumb(id string) (image.Image, int) {
	switch id {
	case "a":
		return scene(color.NRGBA{150, 170, 160, 255}, color.NRGBA{90, 120, 80, 255}, color.NRGBA{70, 110, 60, 255}), 1
	case "b":
		return scene(color.NRGBA{120, 130, 150, 255}, color.NRGBA{70, 80, 70, 255}, color.NRGBA{60, 90, 50, 255}), 1
	}
	return scene(color.NRGBA{60, 70, 80, 255}, color.NRGBA{40, 50, 40, 255}, color.NRGBA{30, 40, 30, 255}), 1
}
func (demo) Watch(string) {}
func (demo) StopWatch()   {}
func (demo) WatchState() (string, image.Image, int, string, bool) {
	return "", nil, 0, "", false
}
func (demo) Armed() (bool, bool)                { return true, true }
func (demo) SetArmed(bool)                      {}
func (demo) SyncStatus() (string, string, bool) { return "Sync Module", "Online", false }
func (demo) Snapshot(string)                    {}
func (demo) Muted() bool                        { return false }
func (demo) SetMuted(bool)                      {}
func (demo) Refresh()                           {}
func (demo) Signout()                           {}
func (demo) Quit()                              {}
func (demo) LoadAccount()                       {}
func (demo) TapoList() []model.TapoCam {
	return []model.TapoCam{{ID: "tapo:192.168.0.40", Name: "Office", Host: "192.168.0.40", User: "you"}}
}
func (demo) TapoAdd(string, string, string, string) error { return nil }
func (demo) TapoRemove(string)                            {}
func (demo) TapoFind()                                    {}
func (demo) TapoFound() ([]model.Found, bool) {
	return []model.Found{{Host: "192.168.0.41", Model: "Tapo C210"}}, false
}
func (demo) TapoError(string) string { return "" }
func (demo) BlinkSignedIn() bool     { return true }
func (demo) Profile() model.Profile {
	return model.Profile{Email: "you@example.com", Name: "You", Phone: "+1******0993", Country: "us", TimeZone: "America/New_York"}
}
func (demo) Notifications() map[string]bool {
	return map[string]bool{"motion": true, "cv_motion": true, "doorbell": true, "camera_offline": true, "sync_module_offline": true, "low_battery": true, "battery_dead": false, "wifi": false, "local_storage": true, "general": false}
}
func (demo) SetNotification(string, bool) {}
func (demo) DeviceConfig(id string) model.DeviceConfig {
	return model.DeviceConfig{Loaded: true, Motion: id != "b", Audio: true, HasAudio: true, LED: true, HasLED: id != "c", Sensitivity: 8, ClipLen: 30, ClipMax: 30, Quality: "best", Firmware: "11.5.2"}
}
func (demo) DeviceRaw(string) map[string]any {
	return map[string]any{"name": "Backyard", "fw_version": "9.105", "wifi": 3.0, "wifi_rssi": -67.0, "led_enabled": true, "enabled": true, "motion_sensitivity": 9.0, "motion_regions": 33554431.0, "retrigger_time": 10.0, "early_notification": true, "early_notification_compatible": true, "clip_length": 30.0, "clip_length_max": 30.0, "video_quality": "best", "early_termination": true, "early_termination_supported": true, "flip_video": false, "flip_video_compatible": true, "illuminator_enable": "auto", "illuminator_intensity": 7.0, "auto_update_thumbnail_enabled": false, "volume_control": 7.0, "chime_volume": 6.0, "chime_compatible": true, "video_recording_enable": true, "video_recording_optional": true, "record_audio_enable": true}
}
func (demo) ClipPlay(string, float64)          {}
func (demo) WatchError(string) string          { return "" }
func (demo) ClipPos(string) (float64, float64) { return 12, 30 }
func (demo) SyncInfo() model.SyncInfo {
	return model.SyncInfo{Name: "Sync Module", Status: "online", Firmware: "4.5.41", Wifi: 5, StorageOn: true, StorageKnown: true, StorageUsed: 11, StorageState: "active", BackupOn: true, BackupResult: "success", LastBackup: time.Now().Add(-9 * time.Hour)}
}
func (demo) SetDeviceConfig(string, string, any) {}
func (demo) SettingsNote() string                { return "" }
func (demo) OpenHelp()                           {}
func (demo) Settings() store.Settings {
	return store.Settings{LocalAPI: true, APIPort: store.DefaultAPIPort}
}
func (demo) SaveSettings(store.Settings)   {}
func (demo) SetAPIPassword(string, string) {}
func (demo) APIStatus() string             { return "Listening on port 7481" }
func (demo) WebURL() string                { return "http://127.0.0.1:7481/" }
func (demo) OpenWeb()                      {}
func (demo) ClipStatus() string            { return "" }
func (demo) OpenClip(string)               {}
func (demo) Clips() []model.Clip {
	now := time.Now()
	var out []model.Clip
	for i := 0; i < 9; i++ {
		out = append(out, model.Clip{ID: fmt.Sprint(i), Camera: []string{"Backyard", "Porch"}[i%2], Home: "My Home", Kind: "Motion", At: now.Add(-time.Duration(i*37) * time.Minute)})
	}
	return out
}

type login struct {
	demo
	wait bool
}

func (login) LoggedIn() bool               { return false }
func (l login) LoginState() (string, bool) { return "", l.wait }

func Run(dir string) {
	_ = os.MkdirAll(dir, 0o755)
	const s = 2
	w, h := int(408*s), int(720*s)
	cur := panel.New(demo{})
	cur.Static = true
	cur.TogglePin = func(string) {}
	for _, v := range []struct {
		name  string
		setup func()
	}{
		{"home", func() {}},
		{"camera", func() { cur.ShowCamera("a") }},
		{"clips", func() { cur.ShowClips() }},
		{"settings", func() { cur.ShowSettings() }},
		{"account", func() { cur.ShowAccount() }},
		{"sources", func() { cur.ShowSources() }},
		{"devices", func() { cur.ShowDevices() }},
		{"camset", func() { cur.ShowCamSettings("a") }},
		{"sync", func() { cur.ShowSync() }},
		{"clipplay", func() { cur.ShowClipPlayer("1") }},
		{"motion", func() { cur.ShowCamSettings("a"); cur.ShowView(102) }},
		{"video", func() { cur.ShowView(103) }},
		{"zones", func() { cur.ShowView(107) }},
		{"sheet", func() { cur.ShowView(0); cur.OpenSheet("a") }},
		{"notify", func() { cur.ShowNotify() }},
		{"api", func() { cur.ShowAPI(false) }},
		{"login", func() { cur = panel.New(login{}); cur.Static = true }},
		{"waiting", func() { cur = panel.New(login{wait: true}); cur.Static = true }},
	} {
		v.setup()
		for i := 0; i < 2; i++ {
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Constraints: layout.Exact(image.Pt(w, h)), Metric: unit.Metric{PxPerDp: s, PxPerSp: s}}
			cur.Layout(gtx)
			if i == 0 {
				continue
			}
			win, err := headless.NewWindow(w, h)
			if err != nil {
				fmt.Println(err)
				return
			}
			img := image.NewRGBA(image.Rect(0, 0, w, h))
			if err := win.Frame(&ops); err == nil {
				err = win.Screenshot(img)
			}
			win.Release()
			f, err := os.Create(filepath.Join(dir, v.name+".png"))
			if err != nil {
				fmt.Println(err)
				return
			}
			_ = png.Encode(f, img)
			f.Close()
		}
	}
}
