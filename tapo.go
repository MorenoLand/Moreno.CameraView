package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"net"
	"strings"
	"time"

	"moreno/blinkview/blinkcloud"
	"moreno/blinkview/model"
	"moreno/blinkview/store"
	"moreno/blinkview/tapocam"
)

func (l *live) findTapo(id string) (tapocam.Camera, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, t := range l.tapos {
		if t.ID == id {
			return t, true
		}
	}
	return tapocam.Camera{}, false
}

func (l *live) runner(id string) (func(context.Context, *watch), bool) {
	if cid, ok := strings.CutPrefix(id, clipPrefix); ok {
		return func(ctx context.Context, w *watch) {
			l.playWith(ctx, w, func(c context.Context) error { return l.streamClip(c, w, cid) })
		}, true
	}
	for _, t := range l.tapos {
		if t.ID == id {
			tc := t
			return func(ctx context.Context, w *watch) { l.playTapo(ctx, w, tc) }, true
		}
	}
	if cam, ok := l.findCamera(id); ok {
		var c blinkcloud.Camera = cam
		return func(ctx context.Context, w *watch) { l.play(ctx, w, c) }, true
	}
	return nil, false
}

func (l *live) tapoShot(tc tapocam.Camera) {
	img, err := tapocam.Snapshot(context.Background(), tc)
	l.mu.Lock()
	if err != nil {
		l.tapoErr[tc.ID] = err.Error()
		l.mu.Unlock()
		l.invalidate()
		return
	}
	delete(l.tapoErr, tc.ID)
	rev := 1
	if old := l.thumbs[tc.ID]; old != nil {
		rev = old.rev + 1
	}
	l.thumbs[tc.ID] = &thumb{img: img, rev: rev, at: time.Now()}
	l.mu.Unlock()
	l.invalidate()
}

func (l *live) refreshTapo() {
	l.mu.Lock()
	cams := append([]tapocam.Camera(nil), l.tapos...)
	l.mu.Unlock()
	for _, t := range cams {
		up := tapocam.Reachable(t.Host)
		l.mu.Lock()
		l.tapoUp[t.ID] = up
		th := l.thumbs[t.ID]
		l.mu.Unlock()
		l.invalidate()
		if up && (th == nil || time.Since(th.at) > 3*time.Minute) {
			l.tapoShot(t)
		}
	}
}

func (l *live) TapoList() []model.TapoCam {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]model.TapoCam, 0, len(l.tapos))
	for _, t := range l.tapos {
		out = append(out, model.TapoCam{ID: t.ID, Name: t.Name, Host: t.Host, User: t.User})
	}
	return out
}

func (l *live) TapoAdd(name, host, user, pass string) error {
	host = strings.TrimSpace(host)
	if net.ParseIP(host) == nil {
		return errors.New("enter the camera's IP address")
	}
	if strings.TrimSpace(user) == "" || pass == "" {
		return errors.New("enter the username and password")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Tapo " + host
	}
	tc := tapocam.Camera{ID: "tapo:" + host, Name: name, Host: host, User: strings.TrimSpace(user), Pass: pass}
	l.mu.Lock()
	replaced := false
	for i := range l.tapos {
		if l.tapos[i].ID == tc.ID {
			l.tapos[i], replaced = tc, true
		}
	}
	if !replaced {
		l.tapos = append(l.tapos, tc)
	}
	delete(l.tapoErr, tc.ID)
	l.tapoNote = ""
	all := append([]tapocam.Camera(nil), l.tapos...)
	l.mu.Unlock()
	store.SaveTapo(all)
	l.invalidate()
	go func() {
		l.mu.Lock()
		l.tapoUp[tc.ID] = tapocam.Reachable(host)
		l.mu.Unlock()
		l.tapoShot(tc)
	}()
	return nil
}

func (l *live) TapoRemove(id string) {
	l.mu.Lock()
	var keep []tapocam.Camera
	for _, t := range l.tapos {
		if t.ID != id {
			keep = append(keep, t)
		}
	}
	l.tapos = keep
	delete(l.thumbs, id)
	delete(l.tapoUp, id)
	delete(l.tapoErr, id)
	l.mu.Unlock()
	store.SaveTapo(keep)
	l.invalidate()
}

func (l *live) TapoFind() {
	l.mu.Lock()
	if l.finding {
		l.mu.Unlock()
		return
	}
	l.finding, l.found = true, nil
	l.mu.Unlock()
	l.invalidate()
	go func() {
		f := tapocam.Discover(3 * time.Second)
		l.mu.Lock()
		l.found, l.finding = f, false
		l.mu.Unlock()
		l.invalidate()
	}()
}

func (l *live) TapoFound() ([]model.Found, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]model.Found, 0, len(l.found))
	for _, f := range l.found {
		out = append(out, model.Found{Host: f.Host, Model: f.Model})
	}
	return out, l.finding
}

func (l *live) TapoError(id string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tapoErr[id]
}

func (l *live) BlinkSignedIn() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.in
}

var _ = bytes.NewReader
var _ image.Image
