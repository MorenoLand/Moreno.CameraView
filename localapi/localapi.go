package localapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"io/fs"
	"mime/multipart"
	"net"
	"net/http"
	"net/netip"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"moreno/blinkview/model"
	"moreno/blinkview/store"
)

//go:embed web/*
var webAssets embed.FS

var webFS, _ = fs.Sub(webAssets, "web")

type Control interface {
	LoggedIn() bool
	HomeName() string
	Cameras() []model.Camera
	Thumb(id string) (image.Image, int)
	Clips() []model.Clip
	ClipStatus() string
	ClipFile(id string) (string, error)
	Watches() []model.Watch
	Armed() (bool, bool)
	SetArmed(bool)
	SyncStatus() (string, string, bool)
	Snapshot(id string)
	Refresh()
	OpenWatch(id string) (string, error)
	PrimeWatch(id string) error
	CloseWatch(token string)
	WatchStateFor(id string) (image.Image, int, string, bool)
	WatchMP4(ctx context.Context, id string) (io.ReadCloser, func(), error)
	Watching() (string, bool)
	Muted() bool
	SetMuted(bool)
}

type Config struct {
	Port             int
	Allow            []string
	Auth             bool
	User, Salt, Hash string
}

type Server struct {
	c     Control
	cfg   Config
	srv   *http.Server
	allow []netip.Prefix
}

type Camera struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Online   bool      `json:"online"`
	Enabled  bool      `json:"enabled"`
	Battery  string    `json:"battery"`
	Seen     time.Time `json:"seen"`
	ThumbRev int       `json:"thumbRev"`
}

type Clip struct {
	ID       string    `json:"id"`
	Camera   string    `json:"camera"`
	Home     string    `json:"home"`
	Kind     string    `json:"kind"`
	Source   string    `json:"source"`
	Manifest string    `json:"manifest,omitempty"`
	Sync     string    `json:"sync,omitempty"`
	Network  string    `json:"network,omitempty"`
	Size     int64     `json:"size,omitempty"`
	At       time.Time `json:"at"`
}

type Sync struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Warn   bool   `json:"warn"`
}

type State struct {
	LoggedIn   bool          `json:"loggedIn"`
	HomeName   string        `json:"homeName"`
	Armed      bool          `json:"armed"`
	ArmedKnown bool          `json:"armedKnown"`
	Sync       *Sync         `json:"sync"`
	Cameras    []Camera      `json:"cameras"`
	Clips      []Clip        `json:"clips"`
	ClipStatus string        `json:"clipStatus,omitempty"`
	Watches    []model.Watch `json:"watches"`
	Muted      bool          `json:"muted"`
	WatchID    string        `json:"watchId"`
	WatchLive  bool          `json:"watchLive"`
	WatchNote  string        `json:"watchNote"`
}

type problem struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, problem{http.StatusText(status), status, detail})
}

func body(w http.ResponseWriter, r *http.Request, v any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		fail(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func Start(cfg Config, c Control) (*Server, error) {
	s := &Server{c: c, cfg: cfg}
	bind := "127.0.0.1"
	for _, e := range cfg.Allow {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		pf, err := netip.ParsePrefix(e)
		if err != nil {
			a, err2 := netip.ParseAddr(e)
			if err2 != nil {
				return nil, fmt.Errorf("invalid allowlist entry %q", e)
			}
			pf = netip.PrefixFrom(a.Unmap(), a.BitLen())
		}
		s.allow = append(s.allow, pf)
		if !pf.Addr().IsLoopback() {
			bind = "0.0.0.0"
		}
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(bind, strconv.Itoa(cfg.Port)))
	if err != nil {
		return nil, err
	}
	s.srv = &http.Server{Handler: s.guard(s.routes()), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

func (s *Server) Close() error { return s.srv.Close() }

func (s *Server) permitted(ip netip.Addr) bool {
	if ip.IsLoopback() {
		return true
	}
	for _, p := range s.allow {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		ap, err := netip.ParseAddrPort(r.RemoteAddr)
		if err != nil || !s.permitted(ap.Addr().Unmap()) {
			fail(w, http.StatusForbidden, "address not in the allowlist")
			return
		}
		if s.cfg.Auth {
			u, pw, ok := r.BasicAuth()
			good := ok && subtle.ConstantTimeCompare([]byte(u), []byte(s.cfg.User)) == 1 && subtle.ConstantTimeCompare([]byte(store.Hash(s.cfg.Salt, pw)), []byte(s.cfg.Hash)) == 1
			if !good {
				w.Header().Set("WWW-Authenticate", `Basic realm="BlinkView"`)
				fail(w, http.StatusUnauthorized, "sign-in required")
				return
			}
		}
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			fail(w, http.StatusForbidden, "cross-origin request refused")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) camera(w http.ResponseWriter, id string) (model.Camera, bool) {
	for _, c := range s.c.Cameras() {
		if c.ID == id {
			return c, true
		}
	}
	fail(w, http.StatusNotFound, "camera not found")
	return model.Camera{}, false
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"name": "BlinkView", "version": "v1"})
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		st := State{LoggedIn: s.c.LoggedIn(), HomeName: s.c.HomeName(), Muted: s.c.Muted(), ClipStatus: s.c.ClipStatus()}
		st.Armed, st.ArmedKnown = s.c.Armed()
		if n, v, warn := s.c.SyncStatus(); n != "" {
			st.Sync = &Sync{Name: n, Status: v, Warn: warn}
		}
		for _, c := range s.c.Cameras() {
			_, rev := s.c.Thumb(c.ID)
			st.Cameras = append(st.Cameras, Camera{c.ID, c.Name, c.Kind, c.Online, c.Enabled, c.Battery, c.Seen, rev})
		}
		for _, c := range s.c.Clips() {
			st.Clips = append(st.Clips, Clip{ID: c.ID, Camera: c.Camera, Home: c.Home, Kind: c.Kind, Source: c.Source, Manifest: c.Manifest, Sync: c.Sync, Network: c.Network, Size: c.Size, At: c.At})
		}
		st.Watches = s.c.Watches()
		if st.Cameras == nil {
			st.Cameras = []Camera{}
		}
		if st.Clips == nil {
			st.Clips = []Clip{}
		}
		if st.Watches == nil {
			st.Watches = []model.Watch{}
		}
		st.WatchID, st.WatchLive = s.c.Watching()
		if st.WatchID != "" {
			_, _, note, _ := s.c.WatchStateFor(st.WatchID)
			st.WatchNote = note
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) {
		s.c.Refresh()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/arm", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Armed bool `json:"armed"`
		}
		if body(w, r, &in) {
			s.c.SetArmed(in.Armed)
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("POST /api/mute", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Muted bool `json:"muted"`
		}
		if body(w, r, &in) {
			s.c.SetMuted(in.Muted)
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("POST /api/cameras/{id}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.camera(w, r.PathValue("id")); !ok {
			return
		}
		s.c.Snapshot(r.PathValue("id"))
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /api/cameras/{id}/thumb", func(w http.ResponseWriter, r *http.Request) {
		img, rev := s.c.Thumb(r.PathValue("id"))
		if img == nil {
			fail(w, http.StatusNotFound, "no thumbnail")
			return
		}
		etag := `"` + strconv.Itoa(rev) + `"`
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_ = jpeg.Encode(w, img, &jpeg.Options{Quality: 84})
	})
	mux.HandleFunc("POST /api/cameras/{id}/wake", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		cam, ok := s.camera(w, id)
		if !ok {
			return
		}
		if !cam.Online {
			fail(w, http.StatusConflict, "camera is offline")
			return
		}
		if err := s.c.PrimeWatch(id); err != nil {
			fail(w, http.StatusConflict, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/cameras/{id}/live.mjpg", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		cam, ok := s.camera(w, id)
		if !ok {
			return
		}
		if !cam.Online {
			fail(w, http.StatusConflict, "camera is offline")
			return
		}
		token, err := s.c.OpenWatch(id)
		if err != nil {
			fail(w, http.StatusConflict, err.Error())
			return
		}
		defer s.c.CloseWatch(token)
		mw := multipart.NewWriter(w)
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+mw.Boundary())
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		last := -1
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-tick.C:
				img, rev, _, active := s.c.WatchStateFor(id)
				if img == nil {
					if !active {
						return
					}
					continue
				}
				if rev == last {
					if !active {
						return
					}
					continue
				}
				var buf bytes.Buffer
				if jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82}) != nil {
					continue
				}
				h := textproto.MIMEHeader{"Content-Type": {"image/jpeg"}, "Content-Length": {strconv.Itoa(buf.Len())}}
				p, err := mw.CreatePart(h)
				if err != nil {
					return
				}
				if _, err := p.Write(buf.Bytes()); err != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
				last = rev
			}
		}
	})
	mux.HandleFunc("GET /api/cameras/{id}/live.mp4", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		cam, ok := s.camera(w, id)
		if !ok {
			return
		}
		if !cam.Online {
			fail(w, http.StatusConflict, "camera is offline")
			return
		}
		token, err := s.c.OpenWatch(id)
		if err != nil {
			fail(w, http.StatusConflict, err.Error())
			return
		}
		defer s.c.CloseWatch(token)
		rc, stop, err := s.c.WatchMP4(r.Context(), id)
		if err != nil {
			fail(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		defer stop()
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Accept-Ranges", "none")
		w.WriteHeader(http.StatusOK)
		_, _ = io.CopyBuffer(w, rc, make([]byte, 64<<10))
	})
	mux.HandleFunc("GET /api/clips", func(w http.ResponseWriter, r *http.Request) {
		out := []Clip{}
		for _, c := range s.c.Clips() {
			out = append(out, Clip{ID: c.ID, Camera: c.Camera, Home: c.Home, Kind: c.Kind, Source: c.Source, Manifest: c.Manifest, Sync: c.Sync, Network: c.Network, Size: c.Size, At: c.At})
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("GET /api/clips/{id}/video", func(w http.ResponseWriter, r *http.Request) {
		path, err := s.c.ClipFile(r.PathValue("id"))
		if err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, path)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := fs.ReadFile(webFS, "index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("GET /pip", func(w http.ResponseWriter, r *http.Request) {
		b, _ := fs.ReadFile(webFS, "pip.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	})
	mux.Handle("GET /web/", http.StripPrefix("/web", noStore(http.FileServer(http.FS(webFS)))))
	return mux
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
