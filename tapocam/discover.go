package tapocam

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"time"
)

func localNets() []*net.IPNet {
	var out []*net.IPNet
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil || !n.IP.IsPrivate() {
				continue
			}
			ones, _ := n.Mask.Size()
			if ones < 22 {
				n = &net.IPNet{IP: n.IP.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}
			}
			out = append(out, n)
		}
	}
	return out
}

func hosts(n *net.IPNet) []net.IP {
	base := n.IP.Mask(n.Mask).To4()
	ones, bits := n.Mask.Size()
	count := 1 << (bits - ones)
	out := make([]net.IP, 0, count)
	for i := 1; i < count-1; i++ {
		ip := make(net.IP, 4)
		copy(ip, base)
		v := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
		v += uint32(i)
		ip[0], ip[1], ip[2], ip[3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
		out = append(out, ip)
	}
	return out
}

func isTapo(host string) bool {
	c, err := net.DialTimeout("tcp", net.JoinHostPort(host, "8800"), 900*time.Millisecond)
	if err != nil {
		return false
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(1500 * time.Millisecond))
	if _, err := c.Write([]byte(probe)); err != nil {
		return false
	}
	head, _ := bufio.NewReader(c).Peek(512)
	return strings.Contains(strings.ToLower(string(head)), "tp-link")
}

func Discover(wait time.Duration) []Found {
	var mu sync.Mutex
	var out []Found
	var wg sync.WaitGroup
	sem := make(chan struct{}, 128)
	seen := map[string]bool{}
	for _, n := range localNets() {
		for _, ip := range hosts(n) {
			h := ip.String()
			if seen[h] {
				continue
			}
			seen[h] = true
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				if isTapo(h) {
					mu.Lock()
					out = append(out, Found{Host: h, Model: "Tapo camera"})
					mu.Unlock()
				}
			}()
		}
	}
	wg.Wait()
	return out
}

const probe = "POST /stream HTTP/1.1\r\nContent-Type: multipart/mixed;boundary=--client-stream-boundary--\r\nContent-Length: -1\r\n\r\n"
