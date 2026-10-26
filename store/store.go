package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"moreno/blinkview/blinkcloud"
	"moreno/blinkview/tapocam"
)

const DefaultAPIPort = 7481

type Settings struct {
	LocalAPI  bool     `json:"local_api"`
	APIPort   int      `json:"api_port"`
	APIAllow  []string `json:"api_allow"`
	APIAuth   bool     `json:"api_auth"`
	APIUser   string   `json:"api_user"`
	APISalt   string   `json:"api_salt"`
	APIHash   string   `json:"api_hash"`
	Muted     bool     `json:"muted"`
	PinTray   bool     `json:"pin_tray"`
	TrayMoved bool     `json:"tray_moved"`
	TrayX     int      `json:"tray_x"`
	TrayY     int      `json:"tray_y"`
	Pinned    []string `json:"pinned"`
}

func Dir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	d := filepath.Join(base, "BlinkView")
	_ = os.MkdirAll(d, 0o700)
	return d
}

func LoadSettings() Settings {
	s := Settings{APIPort: DefaultAPIPort}
	if b, err := os.ReadFile(filepath.Join(Dir(), "settings.json")); err == nil {
		b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
		_ = json.Unmarshal(b, &s)
	}
	if s.APIPort < 1024 || s.APIPort > 65535 {
		s.APIPort = DefaultAPIPort
	}
	return s
}

func SaveSettings(s Settings) error {
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(filepath.Join(Dir(), "settings.json"), b, 0o600)
}

func Hash(salt, password string) string {
	h := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(h[:])
}

func dpapi(in []byte, protect bool) ([]byte, error) {
	if len(in) == 0 {
		return nil, errors.New("empty")
	}
	blob, out := windows.DataBlob{Size: uint32(len(in)), Data: &in[0]}, windows.DataBlob{}
	var err error
	if protect {
		err = windows.CryptProtectData(&blob, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&blob, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func LoadSession() blinkcloud.Session {
	var s blinkcloud.Session
	if b, err := os.ReadFile(filepath.Join(Dir(), "session.bin")); err == nil {
		if plain, err := dpapi(b, false); err == nil {
			_ = json.Unmarshal(plain, &s)
		}
	}
	return s
}

func SaveSession(s blinkcloud.Session) {
	b, _ := json.Marshal(s)
	if enc, err := dpapi(b, true); err == nil {
		_ = os.WriteFile(filepath.Join(Dir(), "session.bin"), enc, 0o600)
	}
}

func ForgetSession() { _ = os.Remove(filepath.Join(Dir(), "session.bin")) }

func LoadTapo() []tapocam.Camera {
	var out []tapocam.Camera
	if b, err := os.ReadFile(filepath.Join(Dir(), "tapo.bin")); err == nil {
		if plain, err := dpapi(b, false); err == nil {
			_ = json.Unmarshal(plain, &out)
		}
	}
	return out
}

func SaveTapo(c []tapocam.Camera) {
	b, _ := json.Marshal(c)
	if enc, err := dpapi(b, true); err == nil {
		_ = os.WriteFile(filepath.Join(Dir(), "tapo.bin"), enc, 0o600)
	}
}
