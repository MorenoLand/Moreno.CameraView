package model

import "time"

type Camera struct {
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
