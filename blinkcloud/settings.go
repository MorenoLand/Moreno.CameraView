package blinkcloud

import (
	"encoding/json"
	"errors"
	"strings"
)

type Profile struct {
	Email, Name, Phone, Country, TimeZone string
}

func (c *Client) User() (Profile, error) {
	b, err := c.Call("GET", "/user", nil)
	if err != nil {
		return Profile{}, err
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return Profile{}, errors.New("unreadable account")
	}
	return Profile{Email: str(m["email"]), Name: str(m["name"]), Phone: str(m["phone_number"]), Country: str(m["country_id"]), TimeZone: str(m["time_zone"])}, nil
}

func (c *Client) Notifications() (map[string]bool, error) {
	b, err := c.Call("GET", "/api/v1/accounts/"+c.Account()+"/notifications/configuration", nil)
	if err != nil {
		return nil, err
	}
	var m struct {
		Notifications map[string]bool `json:"notifications"`
	}
	if json.Unmarshal(b, &m) != nil || m.Notifications == nil {
		return nil, errors.New("unreadable notification settings")
	}
	return m.Notifications, nil
}

func (c *Client) SetNotifications(n map[string]bool) error {
	b, _ := json.Marshal(map[string]any{"notifications": n})
	_, err := c.Call("POST", "/api/v1/accounts/"+c.Account()+"/notifications/configuration", b)
	return err
}

func (c *Client) configPath(cam Camera) string {
	base := "/api/v1/accounts/" + c.Account() + "/networks/" + cam.Network
	switch cam.Kind {
	case "mini":
		return base + "/owls/" + cam.ID + "/config"
	case "doorbell":
		return base + "/doorbells/" + cam.ID + "/config"
	}
	return "/network/" + cam.Network + "/camera/" + cam.ID + "/config"
}

func (c *Client) CameraConfig(cam Camera) (map[string]any, error) {
	b, err := c.Call("GET", c.configPath(cam), nil)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil, errors.New("unreadable camera settings")
	}
	if cfg, ok := m["camera"].([]any); ok && len(cfg) > 0 {
		if o, ok := cfg[0].(map[string]any); ok {
			return o, nil
		}
	}
	return m, nil
}

func (c *Client) SetCameraConfig(cam Camera, fields map[string]any) error {
	b, _ := json.Marshal(fields)
	p := c.configPath(cam)
	if cam.Kind == "camera" {
		p = strings.TrimSuffix(p, "/config") + "/update"
	}
	_, err := c.Call("POST", p, b)
	return err
}
