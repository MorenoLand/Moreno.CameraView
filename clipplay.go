package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const clipPrefix = "clip:"

var durRe = regexp.MustCompile(`Duration: (\d+):(\d+):(\d+(?:\.\d+)?)`)

func hideCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

// cloudClipFile downloads a cloud-stored clip into the cache.
func (l *live) cloudClipFile(id string) (string, error) {
	l.mu.Lock()
	media := ""
	for _, c := range l.clips {
		if c.Source == "cloud" && c.ID == id {
			media = c.Media
		}
	}
	l.mu.Unlock()
	if media == "" {
		return "", errors.New("clip was not found")
	}
	cache, _ := os.UserCacheDir()
	dir := filepath.Join(cache, "BlinkView", "clips")
	path := filepath.Join(dir, "cloud-"+safeName(id)+".mp4")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
		return path, nil
	}
	l.clipMu.Lock()
	defer l.clipMu.Unlock()
	b, err := l.cl.Call("GET", media, nil)
	if err != nil {
		return "", err
	}
	if len(b) < 128 {
		return "", errors.New("clip download was empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp := path + ".part"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

func clipDuration(path string) float64 {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		return 0
	}
	cmd := exec.Command(ff, "-hide_banner", "-i", path)
	hideCmd(cmd)
	out, _ := cmd.CombinedOutput()
	m := durRe.FindStringSubmatch(string(out))
	if m == nil {
		return 0
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	s, _ := strconv.ParseFloat(m[3], 64)
	return float64(h*3600+mi*60) + s
}

// ClipPlay starts (or restarts) a clip at sec seconds in the panel player.
func (l *live) ClipPlay(id string, sec float64) {
	wid := clipPrefix + id
	l.mu.Lock()
	old := l.watches[wid]
	l.mu.Unlock()
	l.StopWatch()
	if old != nil {
		select {
		case <-old.done:
		case <-time.After(2 * time.Second):
		}
	}
	l.mu.Lock()
	delete(l.werr, wid)
	if l.clipOff == nil {
		l.clipOff, l.clipT0, l.clipDur = map[string]float64{}, map[string]time.Time{}, map[string]float64{}
	}
	l.clipOff[id] = max(sec, 0)
	delete(l.clipT0, id)
	l.mu.Unlock()
	l.Watch(wid)
}

// ClipPos reports the playback position and length of a clip in seconds.
func (l *live) ClipPos(id string) (pos, dur float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	dur = l.clipDur[id]
	pos = l.clipOff[id]
	if t0, ok := l.clipT0[id]; ok {
		pos += time.Since(t0).Seconds()
	}
	if dur > 0 && pos > dur {
		pos = dur
	}
	return pos, dur
}

// streamClip plays a saved clip at normal speed through the same pipeline as live view.
func (l *live) streamClip(ctx context.Context, w *watch, clipID string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	l.resetPreroll(w)
	l.watchNote(w, "Downloading clip…")
	path, err := l.ClipFile(clipID)
	if err != nil {
		return err
	}
	l.mu.Lock()
	if l.clipDur == nil {
		l.clipOff, l.clipT0, l.clipDur = map[string]float64{}, map[string]time.Time{}, map[string]float64{}
	}
	dur, off := l.clipDur[clipID], l.clipOff[clipID]
	l.mu.Unlock()
	if dur == 0 {
		dur = clipDuration(path)
		l.mu.Lock()
		l.clipDur[clipID] = dur
		l.mu.Unlock()
	}
	if dur > 0 && off >= dur-0.5 {
		off = 0
		l.mu.Lock()
		l.clipOff[clipID] = 0
		l.mu.Unlock()
	}
	l.watchNote(w, "Starting…")
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		return errors.New("ffmpeg is not installed")
	}
	cmd := exec.CommandContext(ctx, ff, "-hide_banner", "-loglevel", "error", "-re", "-ss", fmt.Sprintf("%.2f", off), "-i", path, "-map", "0:v:0", "-map", "0:a:0?", "-c:v", "copy", "-c:a", "aac", "-ar", "48000", "-ac", "2", "-b:a", "96k", "-f", "mpegts", "-flush_packets", "1", "pipe:1")
	hideCmd(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	l.mu.Lock()
	l.clipT0[clipID] = time.Now()
	l.mu.Unlock()
	defer cmd.Wait()
	defer cmd.Process.Kill()
	err = l.pipe(ctx, w, func() { _ = cmd.Process.Kill() }, func(v io.Writer, a chan<- []byte) error {
		buf := make([]byte, 188*16)
		for {
			n, rerr := io.ReadFull(out, buf)
			if n > 0 {
				n -= n % 188
				chunk := append([]byte(nil), buf[:n]...)
				l.broadcast(w, chunk)
				if a != nil {
					select {
					case a <- chunk:
					default:
					}
				}
				if _, err := v.Write(chunk); err != nil {
					return err
				}
			}
			if rerr != nil {
				return rerr
			}
		}
	})
	if err != nil && ctx.Err() == nil && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "EOF")) {
		l.mu.Lock()
		l.clipOff[clipID], l.clipT0[clipID] = dur, time.Time{}
		delete(l.clipT0, clipID)
		l.mu.Unlock()
		return nil
	}
	if ctx.Err() != nil {
		l.mu.Lock()
		l.clipOff[clipID] += time.Since(l.clipT0[clipID]).Seconds()
		delete(l.clipT0, clipID)
		l.mu.Unlock()
	}
	return err
}
