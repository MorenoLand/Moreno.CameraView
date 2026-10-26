package blinkcloud

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Camera struct {
	ID, Name, Kind, Network, Serial string
	Online, Enabled                 bool
	Battery                         string
	Thumb                           string
	Updated                         time.Time
}

type Network struct {
	ID, Name string
	Armed    bool
}

type Sync struct {
	ID, Name, Network string
	Status            string
	Storage           string
	Full              bool
}

type Home struct {
	Shape    string
	Networks []Network
	Cameras  []Camera
	Syncs    []Sync
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	}
	return ""
}

func flag(v any) bool { b, _ := v.(bool); return b }

func parseTime(v any) time.Time {
	s, _ := v.(string)
	for _, l := range []string{time.RFC3339, "2006-01-02T15:04:05-07:00", "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func list(m map[string]any, k string) []map[string]any {
	var out []map[string]any
	if a, ok := m[k].([]any); ok {
		for _, e := range a {
			if o, ok := e.(map[string]any); ok {
				out = append(out, o)
			}
		}
	}
	return out
}

func (c *Client) Home() (*Home, error) {
	c.mu.Lock()
	if err := c.ensure(); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	b, err := c.raw("GET", c.host+"/api/v3/accounts/"+c.account+"/homescreen", nil)
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil, fmt.Errorf("unreadable home screen")
	}
	h := &Home{Shape: shape(m, 0)}
	for _, n := range list(m, "networks") {
		h.Networks = append(h.Networks, Network{ID: str(n["id"]), Name: str(n["name"]), Armed: flag(n["armed"])})
	}
	for _, s := range list(m, "sync_modules") {
		sy := Sync{ID: str(s["id"]), Name: str(s["name"]), Network: str(s["network_id"]), Status: str(s["status"])}
		st := strings.ToLower(str(s["local_storage_status"]))
		sy.Storage, sy.Full = st, strings.Contains(st, "full")
		h.Syncs = append(h.Syncs, sy)
	}
	for _, kind := range []struct{ key, kind string }{{"cameras", "camera"}, {"owls", "mini"}, {"doorbells", "doorbell"}} {
		for _, o := range list(m, kind.key) {
			cam := Camera{ID: str(o["id"]), Name: str(o["name"]), Kind: kind.kind, Network: str(o["network_id"]), Serial: str(o["serial"]), Enabled: flag(o["enabled"]), Thumb: str(o["thumbnail"]), Updated: parseTime(o["updated_at"])}
			st := strings.ToLower(str(o["status"]))
			cam.Online = st == "done" || st == "online"
			if sg, ok := o["signals"].(map[string]any); ok {
				if bt := str(sg["battery"]); bt != "" {
					cam.Battery = "Battery " + bt + "/5"
				}
			}
			if bs := strings.ToLower(str(o["battery"])); bs == "low" {
				cam.Battery = "Battery low"
			}
			h.Cameras = append(h.Cameras, cam)
		}
	}
	return h, nil
}

func (c *Client) Thumbnail(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("no thumbnail")
	}
	return c.Call("GET", path+".jpg", nil)
}

func (c *Client) Arm(network string, on bool) error {
	state := "disarm"
	if on {
		state = "arm"
	}
	_, err := c.Call("POST", "/api/v1/accounts/"+c.Account()+"/networks/"+network+"/state/"+state, nil)
	return err
}

func (c *Client) Snapshot(cam Camera) error {
	base := "/api/v1/accounts/" + c.Account() + "/networks/" + cam.Network
	switch cam.Kind {
	case "mini":
		base += "/owls/" + cam.ID
	case "doorbell":
		base += "/doorbells/" + cam.ID
	default:
		base = "/network/" + cam.Network + "/camera/" + cam.ID
	}
	_, err := c.Call("POST", base+"/thumbnail", nil)
	return err
}

func shape(v any, d int) string {
	switch t := v.(type) {
	case map[string]any:
		if d > 3 {
			return "{…}"
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+":"+shape(t[k], d+1))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		if len(t) == 0 {
			return "[0]"
		}
		return fmt.Sprintf("[%d]%s", len(t), shape(t[0], d+1))
	case string:
		return "str"
	case float64:
		return "num"
	case bool:
		return "bool"
	}
	return "null"
}
