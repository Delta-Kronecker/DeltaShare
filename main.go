package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Config struct {
	ListenAddr string
	PublicIP   string
	Upstream   string
	Username   string
	Password   string
}

type connStats struct {
	id       uint64
	dest     string
	clientIP string
	start    time.Time
	upload   atomic.Int64
	download atomic.Int64
}

type stats struct {
	mu         sync.RWMutex
	active     map[uint64]*connStats
	totalConns uint64
	totalUp    int64
	totalDown  int64
}

var s = &stats{active: make(map[uint64]*connStats)}

const bannerLines = 16

func main() {
	cfg := Config{}

	flag.StringVar(&cfg.ListenAddr, "listen", ":7373", "Local SOCKS5 listen address")
	flag.StringVar(&cfg.PublicIP, "ip", "", "Public IP for display (auto-detect if empty)")
	flag.StringVar(&cfg.Upstream, "upstream", "127.0.0.1:10808", "Upstream SOCKS5 proxy address")
	flag.StringVar(&cfg.Username, "user", "", "Username for client auth (empty = no auth)")
	flag.StringVar(&cfg.Password, "pass", "", "Password for client auth")
	flag.Parse()

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen failed: %v\n", err)
		os.Exit(1)
	}
	defer ln.Close()

	actualAddr := ln.Addr().String()
	_, port, _ := net.SplitHostPort(actualAddr)
	if port == "" {
		port = "7373"
	}

	var displayIP string
	if cfg.PublicIP != "" {
		displayIP = cfg.PublicIP
	} else {
		displayIP = detectBestIP()
	}

	printBanner(displayIP, port, cfg)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Print("\033[?25h")
		fmt.Println("\nShutting down...")
		ln.Close()
	}()

	go statsRefreshLoop()

	var wg sync.WaitGroup
	var connID uint64

	for {
		conn, err := ln.Accept()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				break
			}
			continue
		}
		connID++
		id := connID
		wg.Add(1)
		go func() {
			defer wg.Done()
			handleConn(conn, id, cfg)
		}()
	}

	wg.Wait()
	printSummary()
	fmt.Print("\033[?25h")
}

func printBanner(ip, port string, cfg Config) {
	version := "v0.3.0"
	auth := "disabled"
	if cfg.Username != "" {
		auth = "enabled"
	}

	fmt.Print("\033[2J\033[H")
	fmt.Println()
	fmt.Println("  ╔═══════════════════════════════════════════════════╗")
	fmt.Printf("  ║             DeltaShare %-27s║\n", version)
	fmt.Println("  ╠═══════════════════════════════════════════════════╣")
	fmt.Printf("  ║  Address  : %-37s║\n", ip+":"+port)
	fmt.Printf("  ║  Auth     : %-37s║\n", auth)
	fmt.Printf("  ║  Upstream : %-37s║\n", cfg.Upstream)
	fmt.Println("  ╠═══════════════════════════════════════════════════╣")
	fmt.Println("  ║  Telegram : Settings > Proxy > SOCKS5             ║")
	fmt.Printf("  ║             %s:%-31s║\n", ip, port)
	fmt.Println("  ║  V2RayNG  : Type SOCKS5                          ║")
	fmt.Printf("  ║             %s:%-31s║\n", ip, port)
	fmt.Println("  ║  curl     : --socks5 <addr>:<port> <url>         ║")
	fmt.Println("  ╚═══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Print("\033[?25l")
}

func statsRefreshLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		refreshTable()
	}
}

func refreshTable() {
	s.mu.RLock()
	active := make([]*connStats, 0, len(s.active))
	for _, c := range s.active {
		active = append(active, c)
	}
	sort.Slice(active, func(i, j int) bool {
		return active[i].id < active[j].id
	})
	totalConns := s.totalConns
	totalUp := s.totalUp
	totalDown := s.totalDown
	s.mu.RUnlock()

	fmt.Printf("\033[%dA\033[J", bannerLines)

	fmt.Println("  ┌──────┬───────────────┬────────────────────────────┬──────────┬──────────┬──────────┐")
	fmt.Println("  │  ID  │    Client     │       Destination          │  Upload  │ Download │   Time   │")
	fmt.Println("  ├──────┼───────────────┼────────────────────────────┼──────────┼──────────┼──────────┤")

	if len(active) == 0 {
		fmt.Println("  │                     no active connections                                         │")
	} else {
		for _, c := range active {
			up := humanBytes(c.upload.Load())
			down := humanBytes(c.download.Load())
			dur := time.Since(c.start).Round(time.Second)

			dest := c.dest
			if len(dest) > 26 {
				dest = dest[:23] + "..."
			}
			clientIP := c.clientIP

			fmt.Printf("  │ #%-4d│ %-13s │ %-26s │ %8s │ %8s │ %8s │\n",
				c.id, clientIP, dest, up, down, dur)
		}
	}

	fmt.Println("  └──────┴───────────────┴────────────────────────────┴──────────┴──────────┴──────────┘")

	if totalUp > 0 || totalDown > 0 || totalConns > 0 {
		fmt.Printf("  Total: %d connections  |  ↑ %s  |  ↓ %s\n", totalConns, humanBytes(totalUp), humanBytes(totalDown))
	} else {
		fmt.Println("  Waiting for connections...")
	}
}

func printSummary() {
	s.mu.RLock()
	totalConns := s.totalConns
	totalUp := s.totalUp
	totalDown := s.totalDown
	s.mu.RUnlock()

	fmt.Print("\033[?25h")
	fmt.Println()
	fmt.Println("  ╔═══════════════════════════════════════════════════╗")
	fmt.Println("  ║               Session Summary                    ║")
	fmt.Println("  ╠═══════════════════════════════════════════════════╣")
	fmt.Printf("  ║  Connections : %-34d║\n", totalConns)
	fmt.Printf("  ║  Uploaded    : %-34s║\n", humanBytes(totalUp))
	fmt.Printf("  ║  Downloaded  : %-34s║\n", humanBytes(totalDown))
	fmt.Println("  ╚═══════════════════════════════════════════════════╝")
}

func isLinkLocal(ip net.IP) bool {
	return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func isWSL(ipStr string) bool {
	return strings.HasPrefix(ipStr, "172.16.") || strings.HasPrefix(ipStr, "172.17.") ||
		strings.HasPrefix(ipStr, "172.18.") || strings.HasPrefix(ipStr, "172.19.") ||
		strings.HasPrefix(ipStr, "172.20.") || strings.HasPrefix(ipStr, "172.21.") ||
		strings.HasPrefix(ipStr, "172.22.") || strings.HasPrefix(ipStr, "172.23.") ||
		strings.HasPrefix(ipStr, "172.24.") || strings.HasPrefix(ipStr, "172.25.") ||
		strings.HasPrefix(ipStr, "172.26.") || strings.HasPrefix(ipStr, "172.27.") ||
		strings.HasPrefix(ipStr, "172.28.") || strings.HasPrefix(ipStr, "172.29.") ||
		strings.HasPrefix(ipStr, "172.30.") || strings.HasPrefix(ipStr, "172.31.")
}

func isUsefulIP(ip net.IP) bool {
	ipStr := ip.String()
	if isLinkLocal(ip) {
		return false
	}
	if isWSL(ipStr) {
		return false
	}
	if strings.HasPrefix(ipStr, "10.") {
		return true
	}
	if strings.HasPrefix(ipStr, "192.168.") {
		return true
	}
	return true
}

func detectBestIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}

	type candidate struct {
		ip    string
		prefs int
	}
	var candidates []candidate

	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() || ipNet.IP.To4() == nil {
			continue
		}
		if !isUsefulIP(ipNet.IP) {
			continue
		}
		ipStr := ipNet.IP.String()
		prefs := 0
		if strings.HasPrefix(ipStr, "10.") {
			prefs = 3
		} else if strings.HasPrefix(ipStr, "192.168.") {
			prefs = 2
		} else {
			prefs = 1
		}
		candidates = append(candidates, candidate{ip: ipStr, prefs: prefs})
	}

	if len(candidates) == 0 {
		return "127.0.0.1"
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].prefs > candidates[j].prefs
	})

	return candidates[0].ip
}

func handleConn(client net.Conn, id uint64, cfg Config) {
	defer client.Close()

	clientAddr := client.RemoteAddr().String()
	clientHost, _, _ := net.SplitHostPort(clientAddr)

	dest, err := serverHandshake(client, cfg)
	if err != nil {
		return
	}

	cs := &connStats{id: id, dest: dest, clientIP: clientHost, start: time.Now()}
	s.mu.Lock()
	s.active[id] = cs
	s.totalConns++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.totalUp += cs.upload.Load()
		s.totalDown += cs.download.Load()
		delete(s.active, id)
		s.mu.Unlock()
	}()

	upstream, err := net.DialTimeout("tcp", cfg.Upstream, 10*time.Second)
	if err != nil {
		return
	}
	defer upstream.Close()

	if err := clientHandshake(upstream, dest, cfg); err != nil {
		return
	}

	client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	relay(client, upstream, cs)
}

func serverHandshake(conn net.Conn, cfg Config) (string, error) {
	buf := make([]byte, 258)

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return "", err
	}
	if buf[0] != 0x05 {
		return "", fmt.Errorf("version %d", buf[0])
	}
	nMethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:nMethods]); err != nil {
		return "", err
	}

	needAuth := cfg.Username != ""
	method := selectMethod(buf[:nMethods], needAuth)

	if _, err := conn.Write([]byte{0x05, method}); err != nil {
		return "", err
	}

	if method == 0x02 {
		if err := authServer(conn, cfg); err != nil {
			return "", err
		}
	}

	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return "", err
	}
	if buf[1] != 0x01 {
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return "", fmt.Errorf("cmd %d not supported", buf[1])
	}
	addr, err := readAddr(buf[3], conn)
	if err != nil {
		return "", err
	}
	return addr, nil
}

func selectMethod(methods []byte, needAuth bool) byte {
	if needAuth {
		for _, m := range methods {
			if m == 0x02 {
				return 0x02
			}
		}
		return 0xFF
	}
	for _, m := range methods {
		if m == 0x00 {
			return 0x00
		}
	}
	return 0x00
}

func authServer(conn net.Conn, cfg Config) error {
	var buf [513]byte
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return err
	}
	uLen := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:uLen]); err != nil {
		return err
	}
	username := string(buf[:uLen])
	if _, err := io.ReadFull(conn, buf[:1]); err != nil {
		return err
	}
	pLen := int(buf[0])
	if _, err := io.ReadFull(conn, buf[:pLen]); err != nil {
		return err
	}
	password := string(buf[:pLen])

	if username != cfg.Username || password != cfg.Password {
		conn.Write([]byte{0x01, 0x01})
		return fmt.Errorf("auth failed for user %q", username)
	}
	_, err := conn.Write([]byte{0x01, 0x00})
	return err
}

func readAddr(atyp byte, conn net.Conn) (string, error) {
	var host string
	switch atyp {
	case 0x01:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		host = net.IP(buf).String()
	case 0x03:
		var dLen uint8
		if err := binary.Read(conn, binary.BigEndian, &dLen); err != nil {
			return "", err
		}
		buf := make([]byte, dLen)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		host = string(buf)
	case 0x04:
		buf := make([]byte, 16)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		host = net.IP(buf).String()
	default:
		return "", fmt.Errorf("atyp %d", atyp)
	}

	var port uint16
	if err := binary.Read(conn, binary.BigEndian, &port); err != nil {
		return "", err
	}

	return net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
}

func clientHandshake(conn net.Conn, dest string, cfg Config) error {
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}

	var resp [2]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		return err
	}
	if resp[0] != 0x05 {
		return fmt.Errorf("upstream version %d", resp[1])
	}
	if resp[1] != 0x00 {
		return fmt.Errorf("upstream requested unsupported method: %d", resp[1])
	}

	host, portStr, err := net.SplitHostPort(dest)
	if err != nil {
		return err
	}
	var port uint16
	fmt.Sscanf(portStr, "%d", &port)

	req := []byte{0x05, 0x01, 0x00}

	ip := net.ParseIP(host)
	if ip4 := ip.To4(); ip4 != nil {
		req = append(req, 0x01)
		req = append(req, ip4...)
	} else if ip6 := ip.To16(); ip6 != nil {
		req = append(req, 0x04)
		req = append(req, ip6...)
	} else {
		req = append(req, 0x03)
		req = append(req, byte(len(host)))
		req = append(req, host...)
	}

	portBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(portBuf, port)
	req = append(req, portBuf...)

	if _, err := conn.Write(req); err != nil {
		return err
	}

	var reply [10]byte
	if _, err := io.ReadFull(conn, reply[:4]); err != nil {
		return err
	}
	if reply[1] != 0x00 {
		return fmt.Errorf("upstream CONNECT failed: %d", reply[1])
	}
	skipAddr(reply[3], conn)

	return nil
}

func skipAddr(atyp byte, conn net.Conn) {
	switch atyp {
	case 0x01:
		io.CopyN(io.Discard, conn, 4+2)
	case 0x03:
		var dLen uint8
		binary.Read(conn, binary.BigEndian, &dLen)
		io.CopyN(io.Discard, conn, int64(dLen)+2)
	case 0x04:
		io.CopyN(io.Discard, conn, 16+2)
	}
}

type countedConn struct {
	net.Conn
	cs   *connStats
	isUp bool
}

func (c *countedConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && c.isUp {
		c.cs.upload.Add(int64(n))
	} else if n > 0 {
		c.cs.download.Add(int64(n))
	}
	return n, err
}

func relay(a, b net.Conn, cs *connStats) (int64, int64) {
	var wg sync.WaitGroup
	wg.Add(2)

	var up, down int64

	go func() {
		defer wg.Done()
		cc := &countedConn{Conn: a, cs: cs, isUp: true}
		n, _ := io.Copy(b, cc)
		up = n
		if tc, ok := b.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		cc := &countedConn{Conn: b, cs: cs, isUp: false}
		n, _ := io.Copy(a, cc)
		down = n
		if tc, ok := a.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	wg.Wait()
	return up, down
}

func humanBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
