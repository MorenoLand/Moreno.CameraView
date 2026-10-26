package blinkcloud

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	oauthBase   = "https://api.oauth.blink.com"
	tierURL     = "https://rest-prod.immedia-semi.com/api/v1/users/tier_info"
	redirectURI = "immedia-blink://applinks.blink.com/signin/callback"
	clientID    = "ios"
	scope       = "client"
	appBuild    = "ANDROID_28373244"
	webAgent    = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.1 Mobile/15E148 Safari/604.1"
	tokenAgent  = "Blink/2511191620 CFNetwork/3860.200.71 Darwin/25.1.0"
)

type Session struct {
	HardwareID   string `json:"hardware_id"`
	RefreshToken string `json:"refresh_token"`
}

type Client struct {
	mu       sync.Mutex
	http     *http.Client
	sess     Session
	access   string
	expires  time.Time
	host     string
	account  string
	verifier string
	OnSave   func(Session)
}

func New(s Session) *Client {
	jar, _ := cookiejar.New(nil)
	if _, err := uuid.Parse(s.HardwareID); err != nil {
		s.HardwareID = strings.ToUpper(uuid.NewString())
	}
	return &Client{http: &http.Client{Jar: jar, Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, sess: s}
}

func (c *Client) Session() Session { return c.sess }

func (c *Client) HasRefresh() bool { return c.sess.RefreshToken != "" }

func randB64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (c *Client) do(method, u string, form url.Values, hdr map[string]string) (*http.Response, []byte, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, nil, err
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return resp, b, nil
}

func (c *Client) webHdr() map[string]string {
	return map[string]string{"User-Agent": webAgent, "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", "Accept-Language": "en-US,en;q=0.9"}
}

func (c *Client) postHdr() map[string]string {
	h := c.webHdr()
	h["Origin"], h["Referer"] = oauthBase, oauthBase+"/oauth/v2/signin"
	return h
}

func (c *Client) BrowserURL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.verifier = randB64(48)
	sum := sha256.Sum256([]byte(c.verifier))
	q := url.Values{"app_brand": {"blink"}, "app_version": {"50.1"}, "client_id": {clientID}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "device_brand": {"Apple"}, "device_model": {"iPhone16,1"}, "device_os_version": {"26.1"}, "hardware_id": {c.sess.HardwareID}, "redirect_uri": {redirectURI}, "response_type": {"code"}, "scope": {scope}}
	return oauthBase + "/oauth/v2/authorize?" + q.Encode()
}

func (c *Client) Complete(code string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.verifier == "" {
		return errors.New("no sign-in in progress")
	}
	err := c.token(url.Values{"app_brand": {"blink"}, "client_id": {clientID}, "code": {code}, "code_verifier": {c.verifier}, "grant_type": {"authorization_code"}, "hardware_id": {c.sess.HardwareID}, "redirect_uri": {redirectURI}, "scope": {scope}})
	c.verifier = ""
	return err
}

func (c *Client) token(form url.Values) error {
	r, body, err := c.do("POST", oauthBase+"/oauth/token", form, map[string]string{"User-Agent": tokenAgent, "Accept": "*/*"})
	if err != nil {
		return err
	}
	var t struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int    `json:"expires_in"`
	}
	if r.StatusCode != 200 || json.Unmarshal(body, &t) != nil || t.Access == "" {
		return fmt.Errorf("token request failed (%d) %s", r.StatusCode, snippet(body))
	}
	c.access, c.expires = t.Access, time.Now().Add(time.Duration(max(t.Expires, 60)-30)*time.Second)
	if t.Refresh != "" {
		c.sess.RefreshToken = t.Refresh
	}
	if c.OnSave != nil {
		c.OnSave(c.sess)
	}
	return nil
}

func (c *Client) ensure() error {
	if c.access != "" && time.Now().Before(c.expires) && c.host != "" {
		return nil
	}
	if c.access == "" || !time.Now().Before(c.expires) {
		if c.sess.RefreshToken == "" {
			return errors.New("signed out")
		}
		if err := c.token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.sess.RefreshToken}, "client_id": {clientID}, "scope": {scope}, "hardware_id": {c.sess.HardwareID}}); err != nil {
			return err
		}
	}
	if c.host == "" {
		b, err := c.raw("GET", tierURL, nil)
		if err != nil {
			return err
		}
		var t struct {
			Tier    string `json:"tier"`
			Account int64  `json:"account_id"`
		}
		if json.Unmarshal(b, &t) != nil || t.Tier == "" {
			return errors.New("could not read account region")
		}
		c.host, c.account = "https://rest-"+t.Tier+".immedia-semi.com", fmt.Sprint(t.Account)
	}
	return nil
}

func (c *Client) raw(method, u string, body []byte) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		rd = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.access)
	req.Header.Set("Accept", "*/*")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode == 401 {
		c.access = ""
	}
	if resp.StatusCode >= 300 {
		return b, fmt.Errorf("blink returned %d %s", resp.StatusCode, snippet(b))
	}
	return b, nil
}

func (c *Client) Call(method, path string, body []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensure(); err != nil {
		return nil, err
	}
	return c.raw(method, c.host+path, body)
}

func (c *Client) Account() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.account
}

func (c *Client) Forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.access, c.host, c.account, c.sess.RefreshToken = "", "", "", ""
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}
