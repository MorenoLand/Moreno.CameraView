package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"

	"moreno/blinkview/blinkcloud"
	"moreno/blinkview/localapi"
	"moreno/blinkview/model"
	"moreno/blinkview/store"
	"moreno/blinkview/tapocam"
)

type thumb struct {
	img  image.Image
	rev  int
	path string
	at   time.Time
}

type live struct {
	werr       map[string]string
	clipOff    map[string]float64
	clipT0     map[string]time.Time
	clipDur    map[string]float64
	syncSt     blinkcloud.SyncStorage
	syncStOK   bool
	mu         sync.Mutex
	cl         *blinkcloud.Client
	home       *blinkcloud.Home
	thumbs     map[string]*thumb
	status     string
	waiting    bool
	busy       bool
	in         bool
	invalidate func()
	wake       chan struct{}
	clips      []blinkcloud.Clip
	local      []*blinkcloud.LocalManifest
	localAt    time.Time
	clipStatus string
	clipMu     sync.Mutex
	settings   store.Settings
	watches    map[string]*watch
	watch      *watch
	panelWatch string
	tokens     map[string]*watch
	nextToken  uint64
	api        *localapi.Server
	apiStatus  string
	prof       blinkcloud.Profile
	notify     map[string]bool
	cfgs       map[string]map[string]any
	accAt      time.Time
	setNote    string
	tapos      []tapocam.Camera
	tapoUp     map[string]bool
	tapoErr    map[string]string
	found      []tapocam.Found
	finding    bool
	tapoNote   string
}

func newLive(invalidate func()) *live {
	l := &live{thumbs: map[string]*thumb{}, invalidate: invalidate, wake: make(chan struct{}, 1), settings: store.LoadSettings(), tokens: map[string]*watch{}, watches: map[string]*watch{}}
	l.cl = blinkcloud.New(store.LoadSession())
	l.cl.OnSave = store.SaveSession
	l.in = l.cl.HasRefresh()
	l.tapos, l.tapoUp, l.tapoErr = store.LoadTapo(), map[string]bool{}, map[string]string{}
	l.listen()
	l.applyAPI()
	go l.loop()
	return l
}

func (l *live) loop() {
	t := time.NewTicker(30 * time.Second)
	for {
		l.mu.Lock()
		in := l.in
		l.mu.Unlock()
		if in {
			l.refresh()
		}
		l.refreshTapo()
		select {
		case <-t.C:
		case <-l.wake:
		}
	}
}

func (l *live) kick() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *live) Refresh() { go l.refresh() }

func (l *live) refresh() {
	h, err := l.cl.Home()
	if err != nil {
		l.mu.Lock()
		l.status = "Error: " + err.Error()
		l.mu.Unlock()
		l.invalidate()
		return
	}
	l.mu.Lock()
	l.home, l.status = h, ""
	l.mu.Unlock()
	l.invalidate()
	l.refreshClips(h)
	var wg sync.WaitGroup
	for _, c := range h.Cameras {
		l.mu.Lock()
		t := l.thumbs[c.ID]
		stale := t == nil || t.path != c.Thumb || time.Since(t.at) > 5*time.Minute
		l.mu.Unlock()
		if !stale || c.Thumb == "" {
			continue
		}
		wg.Add(1)
		go func(c blinkcloud.Camera) {
			defer wg.Done()
			b, err := l.cl.Thumbnail(c.Thumb)
			if err != nil {
				return
			}
			img, _, err := image.Decode(bytes.NewReader(b))
			if err != nil {
				return
			}
			l.mu.Lock()
			rev := 1
			if old := l.thumbs[c.ID]; old != nil {
				rev = old.rev + 1
			}
			l.thumbs[c.ID] = &thumb{img: img, rev: rev, path: c.Thumb, at: time.Now()}
			l.mu.Unlock()
			l.invalidate()
		}(c)
	}
	wg.Wait()
}

func (l *live) refreshClips(h *blinkcloud.Home) {
	since := time.Now().AddDate(0, 0, -7)
	cloud, cerr := l.cl.Clips(since, 3)
	l.mu.Lock()
	cached, at := l.local, l.localAt
	l.mu.Unlock()
	var local []blinkcloud.Clip
	var manifests []*blinkcloud.LocalManifest
	var lerr error
	if cached == nil || time.Since(at) > 3*time.Minute {
		local, manifests, lerr = l.cl.LocalClips(h.Syncs, since)
		if lerr == nil {
			l.mu.Lock()
			l.local, l.localAt = manifests, time.Now()
			l.mu.Unlock()
		}
	} else {
		manifests, local = cached, localFrom(cached, since)
	}
	if lerr != nil {
		local = localFrom(manifests, since)
		if local == nil {
			local = localFrom(cached, since)
		}
	}
	names := map[string]string{}
	for _, n := range h.Networks {
		names[n.ID] = n.Name
	}
	for i := range local {
		if n := names[local[i].Network]; n != "" {
			local[i].Network = n
		}
	}
	seen := map[string]bool{}
	var out []blinkcloud.Clip
	for _, c := range append(local, cloud...) {
		k := c.Device + "|" + c.At.UTC().Format(time.RFC3339)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	status := ""
	if lerr != nil {
		status = "Local storage: " + lerr.Error()
	} else if cerr != nil {
		status = "Cloud clips: " + cerr.Error()
	}
	l.mu.Lock()
	l.clips, l.clipStatus = out, status
	l.mu.Unlock()
	l.invalidate()
}

func localFrom(manifests []*blinkcloud.LocalManifest, since time.Time) []blinkcloud.Clip {
	var out []blinkcloud.Clip
	for _, m := range manifests {
		for _, c := range m.Clips {
			if c.At.Before(since) {
				continue
			}
			c.Manifest, c.Sync, c.Network = m.ID, m.Sync.ID, m.Network
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

func (l *live) LoggedIn() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.in || len(l.tapos) > 0
}

func (l *live) LoginState() (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.status, l.waiting
}

func (l *live) StartLogin() {
	u := l.cl.BrowserURL()
	l.mu.Lock()
	l.waiting, l.status = true, ""
	l.mu.Unlock()
	l.invalidate()
	if err := openURL(u); err != nil {
		l.mu.Lock()
		l.waiting, l.status = false, "Error: could not open the browser"
		l.mu.Unlock()
		l.invalidate()
	}
}

func (l *live) CancelLogin() {
	l.mu.Lock()
	l.waiting, l.status = false, ""
	l.mu.Unlock()
	l.invalidate()
}

func (l *live) callback(code string) {
	l.mu.Lock()
	l.busy = true
	l.mu.Unlock()
	err := l.cl.Complete(code)
	l.mu.Lock()
	l.busy, l.waiting = false, false
	if err != nil {
		l.status = "Error: " + err.Error()
	} else {
		l.in, l.status = true, ""
	}
	l.mu.Unlock()
	l.invalidate()
	l.kick()
}

func (l *live) listen() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /code", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		if code := strings.TrimSpace(string(b)); code != "" {
			go l.callback(code)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := &http.Server{Addr: "127.0.0.1:" + callbackPort, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.ListenAndServe() }()
}

func (l *live) Clips() []model.Clip {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]model.Clip, 0, len(l.clips))
	for _, c := range l.clips {
		kind := "Motion"
		id := c.ID
		if c.Source == "local" {
			kind = "Local"
			id = "local:" + c.Manifest + ":" + c.ID + ":" + strconv.FormatInt(c.At.Unix(), 10)
		}
		out = append(out, model.Clip{ID: id, Camera: c.Device, Home: c.Network, Kind: kind, Source: c.Source, Manifest: c.Manifest, Sync: c.Sync, Network: c.Network, Size: c.Size, At: c.At.Local()})
	}
	return out
}

func (l *live) ClipStatus() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.clipStatus
}

func (l *live) OpenClip(id string) {
	u := l.WebURL()
	if u == "" {
		return
	}
	_ = openURL(u + "?clip=" + url.QueryEscape(id))
}

func (l *live) ClipFile(id string) (string, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 4 || parts[0] != "local" {
		return l.cloudClipFile(id)
	}
	manifestID, clipID := parts[1], parts[2]
	ts, _ := strconv.ParseInt(parts[3], 10, 64)
	cache, _ := os.UserCacheDir()
	dir := filepath.Join(cache, "BlinkView", "clips")
	path := filepath.Join(dir, "local-"+safeName(manifestID)+"-"+safeName(clipID)+".mp4")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
		return path, nil
	}
	l.clipMu.Lock()
	defer l.clipMu.Unlock()
	if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
		return path, nil
	}
	m, raw, err := l.localManifestFor(manifestID, clipID, ts)
	if err != nil {
		return "", err
	}
	path = filepath.Join(dir, "local-"+safeName(m.ID)+"-"+safeName(raw)+".mp4")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
		return path, nil
	}
	b, err := l.cl.LocalClip(m, raw)
	if err != nil && (strings.Contains(err.Error(), "Manifest stale") || strings.Contains(err.Error(), "not found")) {
		m, raw, err = l.localManifestFor("", clipID, ts)
		if err == nil {
			path = filepath.Join(dir, "local-"+safeName(m.ID)+"-"+safeName(raw)+".mp4")
			if fi, e := os.Stat(path); e == nil && fi.Size() > 0 {
				return path, nil
			}
			b, err = l.cl.LocalClip(m, raw)
		}
	}
	if err != nil {
		return "", err
	}
	if len(b) < 128 {
		return "", errors.New("clip download was empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "clip-*.mp4")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return "", err
	}
	return path, nil
}

func (l *live) localManifestFor(manifestID, clipID string, ts int64) (*blinkcloud.LocalManifest, string, error) {
	l.mu.Lock()
	manifests := append([]*blinkcloud.LocalManifest(nil), l.local...)
	home := l.home
	l.mu.Unlock()
	find := func() (*blinkcloud.LocalManifest, string) {
		for _, m := range manifests {
			if manifestID != "" && m.ID != manifestID {
				continue
			}
			for _, c := range m.Clips {
				if c.ID == clipID || ts > 0 && c.At.Unix() == ts {
					return m, c.ID
				}
			}
		}
		return nil, ""
	}
	if m, raw := find(); m != nil {
		return m, raw, nil
	}
	if home == nil {
		return nil, "", errors.New("not signed in")
	}
	_, fresh, err := l.cl.LocalClips(home.Syncs, time.Now().AddDate(0, 0, -30))
	if err != nil {
		return nil, "", err
	}
	manifests = fresh
	l.mu.Lock()
	l.local, l.localAt = fresh, time.Now()
	l.mu.Unlock()
	if m, raw := find(); m != nil {
		return m, raw, nil
	}
	return nil, "", errors.New("clip is no longer in local storage")
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (l *live) HomeName() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.home != nil && len(l.home.Networks) > 0 {
		return l.home.Networks[0].Name
	}
	return ""
}

func (l *live) Cameras() []model.Camera {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]model.Camera, 0, len(l.tapos)+4)
	if l.home != nil {
		for _, c := range l.home.Cameras {
			out = append(out, model.Camera{Provider: "blink", ID: c.ID, Name: c.Name, Kind: c.Kind, Online: c.Online, Enabled: c.Enabled, Battery: c.Battery, Seen: c.Updated})
		}
	}
	for _, t := range l.tapos {
		out = append(out, model.Camera{Provider: "tapo", ID: t.ID, Name: t.Name, Kind: "camera", Online: l.tapoUp[t.ID], Enabled: true})
	}
	return out
}

func (l *live) Thumb(id string) (image.Image, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t := l.thumbs[id]; t != nil {
		return t.img, t.rev
	}
	return nil, 0
}

func (l *live) Armed() (bool, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.home == nil || len(l.home.Networks) == 0 {
		return false, false
	}
	return l.home.Networks[0].Armed, true
}

func (l *live) SetArmed(on bool) {
	l.mu.Lock()
	var ids []string
	if l.home != nil {
		for i := range l.home.Networks {
			l.home.Networks[i].Armed = on
			ids = append(ids, l.home.Networks[i].ID)
		}
	}
	l.mu.Unlock()
	l.invalidate()
	go func() {
		for _, id := range ids {
			_ = l.cl.Arm(id, on)
		}
		time.Sleep(2 * time.Second)
		l.kick()
	}()
}

func (l *live) SyncStatus() (string, string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.home == nil || len(l.home.Syncs) == 0 {
		return "", "", false
	}
	s := l.home.Syncs[0]
	switch {
	case s.Full:
		return "Sync Module", "Storage Full", true
	case s.Storage == "unavailable":
		return "Sync Module", "Storage unavailable", true
	case s.Status == "online" || s.Status == "":
		return "Sync Module", "Online", false
	}
	return "Sync Module", s.Status, true
}

func (l *live) Snapshot(id string) {
	if tc, ok := l.findTapo(id); ok {
		go l.tapoShot(tc)
		return
	}
	l.mu.Lock()
	var cam blinkcloud.Camera
	if l.home != nil {
		for _, c := range l.home.Cameras {
			if c.ID == id {
				cam = c
			}
		}
	}
	l.mu.Unlock()
	if cam.ID == "" {
		return
	}
	go func() {
		_ = l.cl.Snapshot(cam)
		time.Sleep(5 * time.Second)
		l.kick()
	}()
}

func (l *live) Signout() {
	l.stopAll()
	l.cl.Forget()
	store.ForgetSession()
	l.mu.Lock()
	l.in, l.home, l.waiting, l.status = false, nil, false, ""
	l.thumbs = map[string]*thumb{}
	l.mu.Unlock()
	l.invalidate()
}

func (l *live) Quit() {
	l.stopAll()
	l.mu.Lock()
	api := l.api
	l.api = nil
	l.mu.Unlock()
	if api != nil {
		_ = api.Close()
	}
	os.Exit(0)
}

func (l *live) Settings() store.Settings {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.settings
}

func (l *live) SaveSettings(s store.Settings) {
	l.mu.Lock()
	prev := l.settings
	l.settings = s
	l.mu.Unlock()
	_ = store.SaveSettings(s)
	if apiChanged(prev, s) {
		l.applyAPI()
	}
	l.invalidate()
}

func apiChanged(a, b store.Settings) bool {
	return a.LocalAPI != b.LocalAPI || a.APIPort != b.APIPort || strings.Join(a.APIAllow, ",") != strings.Join(b.APIAllow, ",") || a.APIAuth != b.APIAuth || a.APIUser != b.APIUser || a.APIHash != b.APIHash
}

func (l *live) APIStatus() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.apiStatus
}

func (l *live) WebURL() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.settings.LocalAPI || l.api == nil {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d/", l.settings.APIPort)
}

func (l *live) OpenWeb() {
	if u := l.WebURL(); u != "" {
		_ = openURL(u)
	}
}

func (l *live) SetAPIPassword(user, pass string) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	salt := hex.EncodeToString(b)
	l.mu.Lock()
	s := l.settings
	l.mu.Unlock()
	s.APIUser, s.APISalt, s.APIHash = user, salt, store.Hash(salt, pass)
	l.SaveSettings(s)
}

func (l *live) applyAPI() {
	l.mu.Lock()
	if l.api != nil {
		_ = l.api.Close()
		l.api = nil
	}
	s := l.settings
	l.apiStatus = ""
	l.mu.Unlock()
	if !s.LocalAPI {
		l.invalidate()
		return
	}
	if s.APIAuth && s.APIHash == "" {
		l.mu.Lock()
		l.apiStatus = "Set a username and password first"
		l.mu.Unlock()
		l.invalidate()
		return
	}
	srv, err := localapi.Start(localapi.Config{Port: s.APIPort, Allow: s.APIAllow, Auth: s.APIAuth, User: s.APIUser, Salt: s.APISalt, Hash: s.APIHash}, l)
	l.mu.Lock()
	if err != nil {
		l.apiStatus = "Error: " + err.Error()
	} else {
		l.api, l.apiStatus = srv, fmt.Sprintf("Listening on port %d", s.APIPort)
	}
	l.mu.Unlock()
	l.invalidate()
}

func (l *live) Muted() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.settings.Muted
}

func (l *live) SetMuted(on bool) {
	l.mu.Lock()
	l.settings.Muted = on
	s := l.settings
	l.applyVolumeLocked()
	l.mu.Unlock()
	_ = store.SaveSettings(s)
	l.invalidate()
}

func (l *live) applyVolumeLocked() {
	for id, w := range l.watches {
		if w.player == nil {
			continue
		}
		v := float64(0)
		if w.panel && id == l.panelWatch && !l.settings.Muted {
			v = 1
		}
		w.player.SetVolume(v)
	}
}

func (l *live) findCamera(id string) (blinkcloud.Camera, bool) {
	if l.home != nil {
		for _, c := range l.home.Cameras {
			if c.ID == id {
				return c, true
			}
		}
	}
	return blinkcloud.Camera{}, false
}

func (l *live) PrimeWatch(id string) error {
	l.mu.Lock()
	run, ok := l.runner(id)
	if !ok {
		l.mu.Unlock()
		return errors.New("camera not found")
	}
	if w := l.watches[id]; w != nil && w.active {
		l.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &watch{id: id, note: "Waking camera…", active: true, cancel: cancel, done: make(chan struct{}), subs: map[int]chan []byte{}}
	l.watches[id] = w
	l.mu.Unlock()
	l.invalidate()
	go run(ctx, w)
	time.AfterFunc(30*time.Second, func() {
		l.mu.Lock()
		if l.watches[id] == w && w.refs == 0 && !w.panel && w.active && w.cancel != nil {
			w.cancel()
		}
		l.mu.Unlock()
	})
	return nil
}

func (l *live) Watch(id string) {
	l.mu.Lock()
	if l.panelWatch == id && l.watch != nil && l.watch.active && l.watch.id == id {
		l.mu.Unlock()
		l.StopWatch()
		return
	}
	run, ok := l.runner(id)
	if !ok {
		l.mu.Unlock()
		l.invalidate()
		return
	}
	if w := l.watches[id]; w != nil && w.active {
		if !w.panel {
			w.refs++
			w.panel = true
		}
		if l.watch != w {
			l.releasePanelLocked()
		}
		l.watch, l.panelWatch = w, id
		l.applyVolumeLocked()
		l.mu.Unlock()
		l.invalidate()
		return
	}
	l.releasePanelLocked()
	ctx, cancel := context.WithCancel(context.Background())
	w := &watch{id: id, note: "Waking camera…", active: true, cancel: cancel, refs: 1, panel: true, done: make(chan struct{}), subs: map[int]chan []byte{}}
	l.watches[id] = w
	l.watch, l.panelWatch = w, id
	l.mu.Unlock()
	l.invalidate()
	go run(ctx, w)
}

func (l *live) releasePanelLocked() {
	if w := l.watch; w != nil && w.panel {
		w.refs--
		w.panel = false
		if w.refs <= 0 && w.cancel != nil {
			w.cancel()
		}
	}
	l.watch = nil
	l.panelWatch = ""
	l.applyVolumeLocked()
}

func (l *live) StopWatch() {
	l.mu.Lock()
	l.releasePanelLocked()
	l.mu.Unlock()
	l.invalidate()
}

func (l *live) stopAll() {
	l.mu.Lock()
	for _, w := range l.watches {
		w.refs = 0
		w.panel = false
		if w.cancel != nil {
			w.cancel()
		}
	}
	l.watch, l.panelWatch = nil, ""
	l.mu.Unlock()
	l.invalidate()
}

func (l *live) WatchState() (string, image.Image, int, string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.watch == nil || l.panelWatch == "" {
		return "", nil, 0, "", false
	}
	return l.watch.id, l.watch.img, l.watch.rev, l.watch.note, l.watch.active
}

func (l *live) WatchStateFor(id string) (image.Image, int, string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.watches[id]
	if w == nil {
		return nil, 0, "", false
	}
	return w.img, w.rev, w.note, w.active
}

func (l *live) Watching() (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.watch != nil && l.watch.active && l.panelWatch != "" {
		return l.watch.id, true
	}
	for _, w := range l.watches {
		if w.active {
			return w.id, true
		}
	}
	return "", false
}

func (l *live) Watches() []model.Watch {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]model.Watch, 0, len(l.watches))
	for _, w := range l.watches {
		out = append(out, model.Watch{ID: w.id, Live: w.active, Note: w.note})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (l *live) OpenWatch(id string) (string, error) {
	l.mu.Lock()
	run, ok := l.runner(id)
	if !ok {
		l.mu.Unlock()
		return "", errors.New("camera not found")
	}
	w := l.watches[id]
	var ctx context.Context
	start := false
	if w == nil || !w.active {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(context.Background())
		w = &watch{id: id, note: "Waking camera…", active: true, cancel: cancel, done: make(chan struct{}), subs: map[int]chan []byte{}}
		l.watches[id] = w
		start = true
	}
	l.nextToken++
	token := fmt.Sprintf("%d", l.nextToken)
	w.refs++
	l.tokens[token] = w
	l.mu.Unlock()
	if start {
		go run(ctx, w)
	}
	l.invalidate()
	return token, nil
}

func (l *live) CloseWatch(token string) {
	l.mu.Lock()
	w := l.tokens[token]
	delete(l.tokens, token)
	if w != nil {
		w.refs--
		if w.refs <= 0 && !w.panel && w.cancel != nil {
			w.cancel()
		}
	}
	l.mu.Unlock()
	l.invalidate()
}

func (l *live) WatchMP4(ctx context.Context, id string) (io.ReadCloser, func(), error) {
	l.mu.Lock()
	w := l.watches[id]
	if w == nil || w.id != id || !w.active {
		l.mu.Unlock()
		return nil, nil, errors.New("live view is not running")
	}
	ch, pre, unsubscribe, done := l.subscribeLocked(w)
	l.mu.Unlock()
	cmd, stdin, stdout, err := startMP4(ctx)
	if err != nil {
		unsubscribe()
		return nil, nil, err
	}
	go func() {
		defer stdin.Close()
		if len(pre) > 0 {
			if _, err := stdin.Write(pre); err != nil {
				return
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case p := <-ch:
				if _, err := stdin.Write(p); err != nil {
					return
				}
			}
		}
	}()
	stop := func() {
		unsubscribe()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}
	return stdout, stop, nil
}

func (l *live) subscribeLocked(w *watch) (<-chan []byte, []byte, func(), <-chan struct{}) {
	ch := make(chan []byte, 256)
	pre := append([]byte(nil), w.preroll...)
	id := w.nextSub
	w.nextSub++
	w.subs[id] = ch
	unsub := func() {
		l.mu.Lock()
		delete(w.subs, id)
		l.mu.Unlock()
	}
	return ch, pre, unsub, w.done
}

func (l *live) broadcast(w *watch, p []byte) {
	l.mu.Lock()
	if l.watches[w.id] != w || !w.active {
		l.mu.Unlock()
		return
	}
	cp := append([]byte(nil), p...)
	if len(w.preroll) < livePreroll {
		w.preroll = append(w.preroll, cp...)
	}
	for _, ch := range w.subs {
		select {
		case ch <- cp:
		default:
		}
	}
	l.mu.Unlock()
}

func (l *live) watchNote(w *watch, s string) {
	l.mu.Lock()
	if l.watches[w.id] == w {
		w.note = s
	}
	l.mu.Unlock()
	l.invalidate()
}

func (l *live) watchFrame(w *watch, img image.Image) {
	l.mu.Lock()
	if l.watches[w.id] == w {
		w.img, w.rev, w.note = img, w.rev+1, ""
	}
	l.mu.Unlock()
	l.invalidate()
}

func (l *live) watchDone(w *watch) {
	l.mu.Lock()
	w.active = false
	w.player = nil
	if l.watches[w.id] == w {
		delete(l.watches, w.id)
	}
	if l.watch == w {
		l.watch = nil
	}
	if l.panelWatch == w.id {
		l.panelWatch = ""
	}
	l.applyVolumeLocked()
	select {
	case <-w.done:
	default:
		close(w.done)
	}
	l.mu.Unlock()
	l.invalidate()
}

func (l *live) setPlayer(w *watch, p *oto.Player) {
	l.mu.Lock()
	if l.watches[w.id] == w {
		w.player = p
		l.applyVolumeLocked()
	}
	l.mu.Unlock()
}
