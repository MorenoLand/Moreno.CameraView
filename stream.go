package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ebitengine/oto/v3"
	"golang.org/x/sys/windows"

	"moreno/blinkview/blinkcloud"
)

type watch struct {
	id      string
	img     image.Image
	rev     int
	note    string
	active  bool
	cancel  context.CancelFunc
	refs    int
	panel   bool
	player  *oto.Player
	subs    map[int]chan []byte
	preroll []byte
	nextSub int
	done    chan struct{}
}

const livePreroll = 4 << 20

func (l *live) play(ctx context.Context, w *watch, cam blinkcloud.Camera) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer l.watchDone(w)
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return
		}
		last = l.stream(ctx, w, cam)
		if last == nil || ctx.Err() != nil {
			return
		}
		if attempt < 2 {
			l.watchNote(w, "Reconnecting live view…")
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(attempt+1) * 700 * time.Millisecond):
			}
		}
	}
	l.watchNote(w, "Error: "+last.Error())
}

func (l *live) stream(ctx context.Context, w *watch, cam blinkcloud.Camera) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	l.mu.Lock()
	if l.watches[w.id] == w {
		w.preroll = nil
	}
	l.mu.Unlock()
	sess, err := l.cl.LiveView(cam)
	if err != nil {
		return err
	}
	defer l.cl.EndLive(cam.Network, sess.Command)
	go l.poll(ctx, cam.Network, sess.Command, sess.Poll)
	l.watchNote(w, "Connecting…")
	conn, err := dialLive(sess.Server, cam.Serial)
	if err != nil {
		return err
	}
	defer conn.Close()
	go func() { <-ctx.Done(); conn.Close() }()
	go keepLive(ctx, conn)
	cmd, stdin, stdout, errb, err := startFFmpeg(ctx)
	if err != nil {
		return err
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()
	var audioCh chan []byte
	if actx, err := audioDevice(); err == nil {
		if acmd, ain, aout, err := startAudio(ctx); err == nil {
			player := actx.NewPlayer(aout)
			player.SetBufferSize(48000 * 2 * 2 / 10)
			player.Play()
			l.setPlayer(w, player)
			defer player.PauseAndStopReading()
			defer acmd.Wait()
			defer acmd.Process.Kill()
			defer ain.Close()
			audioCh = make(chan []byte, 128)
			go feedAudio(ctx, audioCh, ain)
		}
	}
	readErr := make(chan error, 1)
	go func() {
		readErr <- readLive(conn, stdin, audioCh, l, w)
		stdin.Close()
	}()
	got := false
	ferr := frames(ctx, stdout, func(img image.Image) {
		got = true
		l.watchFrame(w, img)
	})
	conn.Close()
	var rerr error
	select {
	case rerr = <-readErr:
	case <-time.After(500 * time.Millisecond):
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if rerr != nil {
		var ne net.Error
		switch {
		case errors.Is(rerr, net.ErrClosed):
			rerr = nil
		case errors.Is(rerr, io.EOF) || errors.Is(rerr, io.ErrUnexpectedEOF):
			if got {
				return fmt.Errorf("live stream ended")
			}
			return fmt.Errorf("live stream ended before video arrived")
		case errors.As(rerr, &ne) && ne.Timeout():
			return fmt.Errorf("live view timed out")
		default:
			return rerr
		}
	}
	if ferr != nil {
		if !got {
			return fmt.Errorf("live stream ended before video arrived")
		}
		msg := strings.TrimSpace(errb.String())
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = strings.TrimSpace(msg[:i])
		}
		if msg == "" {
			msg = ferr.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	if !got {
		return fmt.Errorf("live stream ended before video arrived")
	}
	return nil
}

func (l *live) poll(ctx context.Context, network string, id int64, every int) {
	t := time.NewTicker(time.Duration(max(every, 3)) * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			done, err := l.cl.LiveCommand(network, id)
			if err != nil || done {
				return
			}
		}
	}
}

func dialLive(server, serial string) (net.Conn, error) {
	u, err := url.Parse(server)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("bad live address")
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	cid, err := strconv.Atoi(u.Query().Get("client_id"))
	if err != nil || cid == 0 {
		return nil, fmt.Errorf("bad live client")
	}
	parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
	connID := strings.Split(parts[len(parts)-1], "__")[0]
	if connID == "" {
		return nil, fmt.Errorf("bad live session")
	}
	d := &net.Dialer{Timeout: 12 * time.Second}
	conn, err := tls.DialWithDialer(d, "tcp", net.JoinHostPort(u.Hostname(), port), &tls.Config{InsecureSkipVerify: true, ServerName: u.Hostname()})
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(liveAuth(serial, connID, cid)); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func liveAuth(serial, connID string, clientID int) []byte {
	var b bytes.Buffer
	b.Write([]byte{0, 0, 0, 0x28})
	authField(&b, serial, 16)
	_ = binary.Write(&b, binary.BigEndian, uint32(clientID))
	b.Write([]byte{1, 8})
	_ = binary.Write(&b, binary.BigEndian, uint32(64))
	b.Write(make([]byte, 64))
	authField(&b, connID, 16)
	b.Write([]byte{0, 0, 0, 1})
	return b.Bytes()
}

func authField(b *bytes.Buffer, s string, n int) {
	_ = binary.Write(b, binary.BigEndian, uint32(n))
	f := make([]byte, n)
	copy(f, s)
	b.Write(f)
}

func readLive(r net.Conn, video io.Writer, audio chan<- []byte, l *live, w *watch) error {
	br := bufio.NewReaderSize(r, 64<<10)
	buf := make([]byte, 64<<10)
	var hdr [9]byte
	for {
		_ = r.SetReadDeadline(time.Now().Add(15 * time.Second))
		if _, err := io.ReadFull(br, hdr[:]); err != nil {
			return err
		}
		n := int(binary.BigEndian.Uint32(hdr[5:9]))
		if n == 0 {
			continue
		}
		if n > 4<<20 {
			return fmt.Errorf("bad live packet")
		}
		if n > len(buf) {
			buf = make([]byte, n)
		}
		_ = r.SetReadDeadline(time.Now().Add(15 * time.Second))
		if _, err := io.ReadFull(br, buf[:n]); err != nil {
			return err
		}
		if hdr[0] == 0 && buf[0] == 0x47 {
			l.broadcast(w, buf[:n])
			if audio != nil {
				select {
				case audio <- append([]byte(nil), buf[:n]...):
				default:
				}
			}
			if _, err := video.Write(buf[:n]); err != nil {
				return err
			}
		}
	}
}

func feedAudio(ctx context.Context, ch <-chan []byte, w io.WriteCloser) {
	for {
		select {
		case <-ctx.Done():
			return
		case p := <-ch:
			if _, err := w.Write(p); err != nil {
				return
			}
		}
	}
}

var (
	audioOnce  sync.Once
	audioCtx   *oto.Context
	audioError error
)

func audioDevice() (*oto.Context, error) {
	audioOnce.Do(func() {
		ctx, ready, err := oto.NewContext(&oto.NewContextOptions{SampleRate: 48000, ChannelCount: 2, Format: oto.FormatSignedInt16LE, BufferSize: 120 * time.Millisecond, ApplicationName: "BlinkView"})
		if err != nil {
			audioError = err
			return
		}
		<-ready
		if audioError = ctx.Err(); audioError == nil {
			audioCtx = ctx
		}
	})
	return audioCtx, audioError
}

func startAudio(ctx context.Context) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ffmpeg is not installed")
	}
	cmd := exec.CommandContext(ctx, path, "-hide_banner", "-loglevel", "error", "-f", "mpegts", "-i", "pipe:0", "-map", "0:a:0", "-vn", "-ac", "2", "-ar", "48000", "-f", "s16le", "pipe:1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, err
	}
	return cmd, in, out, nil
}

func startMP4(ctx context.Context) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ffmpeg is not installed")
	}
	cmd := exec.CommandContext(ctx, path, "-hide_banner", "-loglevel", "error", "-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-map", "0:a:0?", "-c:v", "copy", "-c:a", "aac", "-b:a", "128k", "-f", "mp4", "-movflags", "+frag_keyframe+empty_moov+default_base_moof", "pipe:1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, err
	}
	return cmd, in, out, nil
}

func keepLive(ctx context.Context, conn net.Conn) {
	stats := []byte{0x12, 0, 0, 0x03, 0xe8, 0, 0, 0, 0x18, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	seq := byte(0)
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for i := 0; ; i++ {
		if i%10 == 0 {
			seq++
			if _, err := conn.Write([]byte{0x0A, 0, 0, 0, seq, 0, 0, 0, 0}); err != nil {
				return
			}
		}
		if _, err := conn.Write(stats); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func startFFmpeg(ctx context.Context) (*exec.Cmd, io.WriteCloser, io.ReadCloser, *bytes.Buffer, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("ffmpeg is not installed")
	}
	cmd := exec.CommandContext(ctx, path, "-hide_banner", "-loglevel", "error", "-f", "mpegts", "-i", "pipe:0", "-an", "-vf", "fps=15,scale=960:-2", "-f", "mjpeg", "-q:v", "6", "pipe:1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	errb := &bytes.Buffer{}
	cmd.Stderr = errb
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, nil, err
	}
	return cmd, in, out, errb, nil
}

func frames(ctx context.Context, r io.Reader, fn func(image.Image)) error {
	br := bufio.NewReaderSize(r, 1<<20)
	frame := make([]byte, 0, 512<<10)
	for {
		b, err := br.ReadByte()
		if err != nil {
			return err
		}
		if len(frame) == 0 {
			if b != 0xFF {
				continue
			}
			p, err := br.Peek(1)
			if err != nil {
				return err
			}
			if p[0] == 0xD8 {
				frame = append(frame, 0xFF, 0xD8)
				_, _ = br.Discard(1)
			}
			continue
		}
		frame = append(frame, b)
		if b == 0xFF {
			p, err := br.Peek(1)
			if err != nil {
				return err
			}
			if p[0] == 0xD9 {
				frame = append(frame, 0xD9)
				_, _ = br.Discard(1)
				if img, err := jpeg.Decode(bytes.NewReader(frame)); err == nil {
					fn(img)
				}
				frame = frame[:0]
			}
		}
	}
}
