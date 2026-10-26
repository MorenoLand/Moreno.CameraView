package main

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	callbackPort = "47831"
	scheme       = "immedia-blink"
)

func openURL(u string) error {
	c := exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return c.Start()
}

func registerScheme() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+scheme, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.SetStringValue("", "URL:Blink sign-in")
	_ = k.SetStringValue("URL Protocol", "")
	c, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+scheme+`\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer c.Close()
	_ = c.SetStringValue("", `"`+exe+`" "%1"`)
}

func forwardCallback(arg string) bool {
	if !strings.HasPrefix(arg, scheme+":") {
		return false
	}
	if u, err := url.Parse(arg); err == nil {
		if code := u.Query().Get("code"); code != "" {
			if r, err := http.Post("http://127.0.0.1:"+callbackPort+"/code", "text/plain", strings.NewReader(code)); err == nil {
				r.Body.Close()
			}
		}
	}
	return true
}
