package blinkcloud

import (
	"encoding/json"
	"time"
)

type SyncStorage struct {
	Enabled, Full, BackupOn bool
	State, LastResult       string
	Used                    int
	LastBackup              time.Time
}

// SyncStorage reads the USB local storage status of a Sync Module.
func (c *Client) SyncStorage(s Sync) (SyncStorage, error) {
	b, err := c.Call("GET", "/api/v1/accounts/"+c.Account()+"/networks/"+s.Network+"/sync_modules/"+s.ID+"/local_storage/status", nil)
	if err != nil {
		return SyncStorage{}, err
	}
	var r struct {
		Enabled    bool   `json:"local_storage_enabled"`
		State      string `json:"usb_state"`
		Used       int    `json:"usb_storage_used"`
		Full       bool   `json:"usb_storage_full"`
		BackupOn   bool   `json:"sm_backup_enabled"`
		LastBackup string `json:"last_backup_completed"`
		LastResult string `json:"last_backup_result"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return SyncStorage{}, err
	}
	t, _ := time.Parse(time.RFC3339, r.LastBackup)
	return SyncStorage{Enabled: r.Enabled, State: r.State, Used: r.Used, Full: r.Full, BackupOn: r.BackupOn, LastBackup: t, LastResult: r.LastResult}, nil
}
