package model

import "time"

type Camera struct {
	Provider        string
	ID, Name, Kind  string
	Online, Enabled bool
	Battery         string
	Seen            time.Time
}

func (c Camera) KindName() string {
	if c.Kind == "doorbell" {
		return "Doorbell"
	}
	return "Camera"
}

type Clip struct {
	ID, Camera, Home, Kind          string
	Source, Manifest, Sync, Network string
	Size                            int64
	At                              time.Time
}

type Watch struct {
	ID   string `json:"id"`
	Live bool   `json:"live"`
	Note string `json:"note"`
}

type Profile struct {
	Email, Name, Phone, Country, TimeZone string
}

type DeviceConfig struct {
	Loaded                        bool
	Motion, Audio, LED            bool
	HasAudio, HasLED              bool
	Sensitivity, ClipLen, ClipMax int
	Quality, Firmware             string
}

type SyncInfo struct {
	Name, Status, Firmware     string
	Wifi                       int
	StorageOn, Full, BackupOn  bool
	StorageState, BackupResult string
	StorageUsed                int
	StorageKnown               bool
	LastBackup                 time.Time
}

type TapoCam struct {
	ID, Name, Host, User string
}

type Found struct{ Host, Model string }
