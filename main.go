package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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
	recent     []*connStats
	totalConns uint64
	totalUp    int64
	totalDown  int64
}

var s = &stats{active: make(map[uint64]*connStats)}
var startTime time.Time

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.ListenAddr, "listen", ":7373", "Local SOCKS5 listen address")
	flag.StringVar(&cfg.PublicIP, "ip", "", "Public IP for display")
	flag.StringVar(&cfg.Upstream, "upstream", "127.0.0.1:10808", "Upstream SOCKS5 proxy")
	flag.StringVar(&cfg.Username, "user", "", "Username for auth")
	flag.StringVar(&cfg.Password, "pass", "", "Password for auth")
	flag.Parse()
	startTime = time.Now()

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen failed: %v\n", err)
		os.Exit(1)
	}

	actualAddr := ln.Addr().String()
	_, port, _ := net.SplitHostPort(actualAddr)
	if port == "" {
		port = "7373"
	}

	displayIP := cfg.PublicIP
	if displayIP == "" {
		displayIP = detectBestIP()
	}

	app := tview.NewApplication()

	infoText := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)

	connTable := tview.NewTable().
		SetSelectable(false, false).
		SetFixed(1, 0)

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(infoText, 0, 1, false).
		AddItem(connTable, 0, 2, false)

	app.SetRoot(flex, true)

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			app.QueueUpdateDraw(func() {
				updateInfo(infoText, displayIP, port, cfg)
				updateTable(connTable)
			})
		}
	}()

	go func() {
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
		app.Stop()
	}()

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "UI error: %v\n", err)
		os.Exit(1)
	}

	s.mu.RLock()
	fmt.Printf("\n  Session Summary\n")
	fmt.Printf("  Connections: %d\n", s.totalConns)
	fmt.Printf("  Uploaded:    %s\n", humanBytes(s.totalUp))
	fmt.Printf("  Downloaded:  %s\n", humanBytes(s.totalDown))
	fmt.Printf("  Duration:    %s\n", formatDuration(time.Since(startTime)))
	s.mu.RUnlock()
}

func updateInfo(tv *tview.TextView, ip, port string, cfg Config) {
	auth := "[gray]off[white]"
	if cfg.Username != "" {
		auth = "[green]on[white]"
	}

	s.mu.RLock()
	totalUp := s.totalUp
	totalDown := s.totalDown
	activeCount := len(s.active)
	totalConns := s.totalConns
	s.mu.RUnlock()

	uptime := formatDuration(time.Since(startTime))

	telegramLink := fmt.Sprintf("https://t.me/socks?server=%s&port=%s", ip, port)
	v2rayLink := fmt.Sprintf("socks://%s:%s#DeltaShare", ip, port)

	tv.Clear()
	fmt.Fprintf(tv, "[yellow]■ [white]DeltaShare [gray]v0.6.2\n\n")
	fmt.Fprintf(tv, "[gray]Address    [white]%s:%s\n", ip, port)
	fmt.Fprintf(tv, "[gray]Upstream   [gray]%s\n", cfg.Upstream)
	fmt.Fprintf(tv, "[gray]Auth       %s\n", auth)
	fmt.Fprintf(tv, "[gray]Uptime     [green]%s\n\n", uptime)
	fmt.Fprintf(tv, "[gray]Telegram   [white]%s\n", telegramLink)
	fmt.Fprintf(tv, "[gray]V2RayNG    [white]%s\n\n", v2rayLink)
	fmt.Fprintf(tv, "[gray]Upload     [cyan]%s\n", humanBytes(totalUp))
	fmt.Fprintf(tv, "[gray]Download   [cyan]%s\n", humanBytes(totalDown))
	fmt.Fprintf(tv, "[gray]Active     [green]%d[gray]   Total [white]%d", activeCount, totalConns)
}

func updateTable(table *tview.Table) {
	s.mu.RLock()
	recent := make([]*connStats, 0)
	n := len(s.recent)
	if n > 10 {
		n = 10
	}
	for i := len(s.recent) - 1; i >= len(s.recent)-n; i-- {
		recent = append(recent, s.recent[i])
	}
	s.mu.RUnlock()

	table.Clear()

	headers := []string{"ID", "Destination", "Upload", "Download", "Time"}
	for c, h := range headers {
		cell := tview.NewTableCell(h).
			SetTextColor(tcellColor(tview.Styles.SecondaryTextColor)).
			SetSelectable(false).
			SetExpansion(1)
		table.SetCell(0, c, cell)
	}

	for i, cs := range recent {
		row := i + 1
		up := humanBytes(cs.upload.Load())
		down := humanBytes(cs.download.Load())
		dur := formatDuration(time.Since(cs.start))
		dest := cs.dest
		if len(dest) > 30 {
			dest = dest[:27] + "..."
		}

		table.SetCell(row, 0, tview.NewTableCell(fmt.Sprintf("#%d", cs.id)).SetExpansion(1))
		table.SetCell(row, 1, tview.NewTableCell(dest).SetExpansion(3))
		table.SetCell(row, 2, tview.NewTableCell(up).SetExpansion(1))
		table.SetCell(row, 3, tview.NewTableCell(down).SetExpansion(1))
		table.SetCell(row, 4, tview.NewTableCell(dur).SetExpansion(1))
	}
}

func tcellColor(c tcell.Color) tcell.Color {
	return c
}

func isLinkLocal(ip net.IP) bool {
	return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func isWSL(ipStr string) bool {
	return strings.HasPrefix(ipStr, "172.") && (strings.HasPrefix(ipStr, "172.16.") ||
		strings.HasPrefix(ipStr, "172.17.") || strings.HasPrefix(ipStr, "172.18.") ||
		strings.HasPrefix(ipStr, "172.19.") || strings.HasPrefix(ipStr, "172.20.") ||
		strings.HasPrefix(ipStr, "172.21.") || strings.HasPrefix(ipStr, "172.22.") ||
		strings.HasPrefix(ipStr, "172.23.") || strings.HasPrefix(ipStr, "172.24.") ||
		strings.HasPrefix(ipStr, "172.25.") || strings.HasPrefix(ipStr, "172.26.") ||
		strings.HasPrefix(ipStr, "172.27.") || strings.HasPrefix(ipStr, "172.28.") ||
		strings.HasPrefix(ipStr, "172.29.") || strings.HasPrefix(ipStr, "172.30.") ||
		strings.HasPrefix(ipStr, "172.31."))
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
		if isLinkLocal(ipNet.IP) || isWSL(ipNet.IP.String()) {
			continue
		}
		ipStr := ipNet.IP.String()
		prefs := 1
		if strings.HasPrefix(ipStr, "10.") {
			prefs = 3
		} else if strings.HasPrefix(ipStr, "192.168.") {
			prefs = 2
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
	s.recent = append(s.recent, cs)
	if len(s.recent) > 20 {
		s.recent = s.recent[len(s.recent)-20:]
	}
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

// ─── SOCKS5 Protocol ────────────────────────────────────────────────────────

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
	return readAddr(buf[3], conn)
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
		return fmt.Errorf("upstream unsupported method: %d", resp[1])
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

// ─── Relay ──────────────────────────────────────────────────────────────────

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
		n, _ := io.Copy(b, &countedConn{Conn: a, cs: cs, isUp: true})
		up = n
		if tc, ok := b.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		n, _ := io.Copy(a, &countedConn{Conn: b, cs: cs, isUp: false})
		down = n
		if tc, ok := a.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()
	wg.Wait()
	return up, down
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func humanBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.2f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
