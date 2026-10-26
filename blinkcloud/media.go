package blinkcloud

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Clip struct {
	ID, Device, Network, Source string
	At                          time.Time
	Thumb, Media                string
	Manifest, Sync              string
	Size                        int64
}

type LocalManifest struct {
	ID, Network string
	Sync        Sync
	Clips       []Clip
}

func (c *Client) Clips(since time.Time, pages int) ([]Clip, error) {
	var out []Clip
	for page := 1; page <= pages; page++ {
		q := url.Values{"since": {since.UTC().Format("2006-01-02T15:04:05-0700")}, "page": {fmt.Sprint(page)}}
		b, err := c.Call("GET", "/api/v1/accounts/"+c.Account()+"/media/changed?"+q.Encode(), nil)
		if err != nil {
			return out, err
		}
		var m struct {
			Media []map[string]any `json:"media"`
		}
		if json.Unmarshal(b, &m) != nil || len(m.Media) == 0 {
			break
		}
		for _, o := range m.Media {
			if flag(o["deleted"]) {
				continue
			}
			out = append(out, Clip{ID: str(o["id"]), Device: str(o["device_name"]), Network: str(o["network_name"]), Source: "cloud", At: parseTime(o["created_at"]), Thumb: str(o["thumbnail"]), Media: str(o["media"])})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out, nil
}

func (c *Client) LocalClips(syncs []Sync, since time.Time) ([]Clip, []*LocalManifest, error) {
	var out []Clip
	var manifests []*LocalManifest
	for _, s := range syncs {
		st := strings.ToLower(s.Storage)
		if st != "active" && st != "ready" && st != "online" {
			continue
		}
		m, err := c.localManifest(s)
		if err != nil {
			return out, manifests, err
		}
		manifests = append(manifests, m)
		for _, cl := range m.Clips {
			if cl.At.IsZero() || cl.At.Before(since) {
				continue
			}
			out = append(out, cl)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out, manifests, nil
}

func (c *Client) localManifest(s Sync) (*LocalManifest, error) {
	base := "/api/v1/accounts/" + c.Account() + "/networks/" + s.Network + "/sync_modules/" + s.ID + "/local_storage"
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		m, err := c.manifestOnce(base, s)
		if err == nil {
			return m, nil
		}
		last = err
		if !strings.Contains(err.Error(), "Manifest stale") {
			return nil, err
		}
		time.Sleep(2 * time.Second)
	}
	return nil, last
}

func (c *Client) manifestOnce(base string, s Sync) (*LocalManifest, error) {
	var b []byte
	var err error
	for i := 0; i < 3; i++ {
		b, err = c.Call("POST", base+"/manifest/request", nil)
		if err == nil || !strings.Contains(err.Error(), "System is busy") {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, err
	}
	var r struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(b, &r) != nil || r.ID == 0 {
		return nil, fmt.Errorf("local storage manifest request failed")
	}
	if err := c.pollCommand(s.Network, r.ID); err != nil {
		return nil, err
	}
	b, err = c.Call("GET", base+"/manifest/request/"+fmt.Sprint(r.ID), nil)
	if err != nil {
		return nil, err
	}
	var m struct {
		ID    string `json:"manifest_id"`
		Clips []struct {
			ID         string `json:"id"`
			CameraName string `json:"camera_name"`
			CreatedAt  string `json:"created_at"`
			Size       string `json:"size"`
		} `json:"clips"`
	}
	if json.Unmarshal(b, &m) != nil || m.ID == "" {
		return nil, fmt.Errorf("local storage manifest is unreadable")
	}
	lm := &LocalManifest{ID: m.ID, Network: s.Network, Sync: s}
	for _, cl := range m.Clips {
		size, _ := strconv.ParseInt(cl.Size, 10, 64)
		lm.Clips = append(lm.Clips, Clip{ID: cl.ID, Device: cl.CameraName, Network: s.Network, Source: "local", At: parseTime(cl.CreatedAt), Manifest: m.ID, Sync: s.ID, Size: size * 1024})
	}
	return lm, nil
}

func (c *Client) LocalClip(m *LocalManifest, clipID string) ([]byte, error) {
	base := "/api/v1/accounts/" + c.Account() + "/networks/" + m.Network + "/sync_modules/" + m.Sync.ID + "/local_storage/manifest/" + m.ID + "/clip/request/" + clipID
	b, err := c.Call("POST", base, nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(b, &r) != nil || r.ID == 0 {
		return nil, fmt.Errorf("local clip request failed")
	}
	if err := c.pollCommand(m.Network, r.ID); err != nil {
		return nil, err
	}
	return c.Call("GET", base, nil)
}

func (c *Client) pollCommand(network string, id int64) error {
	deadline := time.Now().Add(60 * time.Second)
	for {
		b, err := c.Call("GET", "/network/"+network+"/command/"+fmt.Sprint(id), nil)
		if err != nil {
			return err
		}
		var m struct {
			Complete bool `json:"complete"`
		}
		if json.Unmarshal(b, &m) != nil {
			return fmt.Errorf("unreadable command")
		}
		if m.Complete {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("command timed out")
		}
		time.Sleep(time.Second)
	}
}
