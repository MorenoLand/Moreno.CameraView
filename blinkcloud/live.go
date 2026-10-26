package blinkcloud

import (
	"encoding/json"
	"fmt"
)

type Live struct {
	Server  string
	Command int64
	Poll    int
}

func (c *Client) LiveView(cam Camera) (*Live, error) {
	path := "/api/v6/accounts/" + c.Account() + "/networks/" + cam.Network + "/cameras/" + cam.ID + "/liveview"
	switch cam.Kind {
	case "mini":
		path = "/api/v2/accounts/" + c.Account() + "/networks/" + cam.Network + "/owls/" + cam.ID + "/liveview"
	case "doorbell":
		path = "/api/v2/accounts/" + c.Account() + "/networks/" + cam.Network + "/doorbells/" + cam.ID + "/liveview"
	}
	b, err := c.Call("POST", path, []byte(`{"intent":"liveview","motion_event_start_time":null}`))
	if err != nil {
		return nil, err
	}
	var m struct {
		Server  string `json:"server"`
		Command int64  `json:"command_id"`
		Poll    int    `json:"polling_interval"`
	}
	if json.Unmarshal(b, &m) != nil || m.Server == "" || m.Command == 0 {
		return nil, fmt.Errorf("live view unavailable")
	}
	return &Live{Server: m.Server, Command: m.Command, Poll: max(m.Poll, 3)}, nil
}

func (c *Client) LiveCommand(network string, id int64) (bool, error) {
	b, err := c.Call("GET", "/network/"+network+"/command/"+fmt.Sprint(id), nil)
	if err != nil {
		return false, err
	}
	var m struct {
		Complete bool `json:"complete"`
	}
	if json.Unmarshal(b, &m) != nil {
		return false, fmt.Errorf("unreadable command")
	}
	return m.Complete, nil
}

func (c *Client) EndLive(network string, id int64) {
	_, _ = c.Call("POST", "/network/"+network+"/command/"+fmt.Sprint(id)+"/done/", nil)
}
