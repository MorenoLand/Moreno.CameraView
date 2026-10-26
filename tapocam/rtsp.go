package tapocam

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func RTSPURL(c Camera, stream int) string {
	u := url.URL{Scheme: "rtsp", User: url.UserPassword(c.User, c.Pass), Host: net.JoinHostPort(c.Host, "554"), Path: fmt.Sprintf("/stream%d", stream)}
	return u.String()
}

func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

func ffArgs(c Camera, stream int, extra ...string) []string {
	return append([]string{"-hide_banner", "-loglevel", "error", "-rtsp_transport", "tcp", "-timeout", "8000000", "-i", RTSPURL(c, stream)}, extra...)
}

func Reachable(host string) bool {
	c, err := net.DialTimeout("tcp", net.JoinHostPort(host, "554"), 1500*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func LastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, 10); i >= 0 {
		s = strings.TrimSpace(s[i+1:])
	}
	return s
}

func Stream(ctx context.Context, c Camera) (*exec.Cmd, *bytes.Buffer, io.Reader, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ffmpeg is not installed")
	}
	cmd := exec.CommandContext(ctx, path, ffArgs(c, 1, "-map", "0:v:0", "-map", "0:a:0?", "-c:v", "copy", "-c:a", "aac", "-ar", "48000", "-ac", "2", "-b:a", "96k", "-fflags", "+genpts", "-f", "mpegts", "-flush_packets", "1", "pipe:1")...)
	hide(cmd)
	errb := &bytes.Buffer{}
	cmd.Stderr = errb
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, err
	}
	return cmd, errb, out, nil
}

func Snapshot(ctx context.Context, c Camera) (image.Image, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg is not installed")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, ffArgs(c, 2, "-frames:v", "1", "-vf", "scale=960:-2", "-f", "image2pipe", "-c:v", "mjpeg", "-q:v", "4", "pipe:1")...)
	hide(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil || out.Len() == 0 {
		msg := LastLine(errb.String())
		if msg == "" {
			msg = "no picture from the camera"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return jpeg.Decode(&out)
}
