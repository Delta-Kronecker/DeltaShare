package main

import (
	"encoding/base64"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
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
var restartConfig *Config

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.ListenAddr, "listen", ":7373", "Local SOCKS5 listen address")
	flag.StringVar(&cfg.PublicIP, "ip", "", "Public IP for display")
	flag.StringVar(&cfg.Upstream, "upstream", "127.0.0.1:10808", "Upstream SOCKS5 proxy")
	flag.StringVar(&cfg.Username, "user", "", "Username for auth")
	flag.StringVar(&cfg.Password, "pass", "", "Password for auth")
	flag.Parse()

	for {
		if restartConfig != nil {
			cfg = *restartConfig
			restartConfig = nil
		}
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
		publicIP := cfg.PublicIP
		currentPort := &port

		app := tview.NewApplication()

		infoText := tview.NewTextView().
			SetDynamicColors(true).
			SetRegions(true)

		headerLeft := tview.NewTextView().SetDynamicColors(true).
			SetText("[#FFD93D]■ [#FFFFFF]DeltaShare [#888888]v0.8.0")
		headerLeft.SetBackgroundColor(tcell.NewRGBColor(15, 15, 25))
		headerRight := tview.NewTextView().SetDynamicColors(true).
			SetText("[#666666]press s for settings").
			SetTextAlign(tview.AlignRight)
		headerRight.SetBackgroundColor(tcell.NewRGBColor(15, 15, 25))

		headerRow := tview.NewFlex().
			AddItem(headerLeft, 0, 1, false).
			AddItem(headerRight, 0, 1, false)

		connTable := tview.NewTable().
			SetSelectable(false, false).
			SetFixed(1, 0)

		flex := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(headerRow, 1, 0, false).
			AddItem(infoText, 0, 1, false).
			AddItem(connTable, 0, 1, false)
		flex.SetTitle(" DeltaShare ").SetBorder(true).
			SetTitleColor(tcell.GetColor("#5DADE2")).
			SetBorderColor(tcell.GetColor("#5DADE2"))

		app.SetRoot(flex, true)

		if publicIP == "" {
			publicIP = "..."
			updateInfo(infoText, publicIP, *currentPort, cfg)
			go func() {
				ip := detectPublicIP()
				if ip != "" {
					publicIP = ip
				} else {
					publicIP = "unknown"
				}
				app.QueueUpdateDraw(func() {
					updateInfo(infoText, publicIP, *currentPort, cfg)
				})
			}()
		} else {
			updateInfo(infoText, publicIP, *currentPort, cfg)
		}

		app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Rune() == 's' || event.Rune() == 'S' {
				showSettings(app, flex, infoText, connTable, &cfg, currentPort, publicIP)
				return nil
			}
			if event.Rune() == 'q' || event.Rune() == 'Q' {
				app.Stop()
				return nil
			}
			return event
		})

		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				app.QueueUpdateDraw(func() {
					updateInfo(infoText, publicIP, *currentPort, cfg)
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

		ln.Close()

		if restartConfig == nil {
			break
		}
	}

	s.mu.RLock()
	fmt.Printf("\n  Session Summary\n")
	fmt.Printf("  Connections: %d\n", s.totalConns)
	fmt.Printf("  Uploaded:    %s\n", humanBytes(s.totalUp))
	fmt.Printf("  Downloaded:  %s\n", humanBytes(s.totalDown))
	fmt.Printf("  Duration:    %s\n", formatDuration(time.Since(startTime)))
	s.mu.RUnlock()
}

func showSettings(app *tview.Application, flex *tview.Flex, infoText *tview.TextView, connTable *tview.Table, cfg *Config, currentPort *string, publicIP string) {
	fields := []*tview.InputField{
		tview.NewInputField().SetLabel("Port      ").SetText(*currentPort).SetFieldWidth(5),
		tview.NewInputField().SetLabel("Username  ").SetText(cfg.Username).SetFieldWidth(20),
		tview.NewInputField().SetLabel("Password  ").SetText(cfg.Password).SetFieldWidth(20).SetMaskCharacter('*'),
		tview.NewInputField().SetLabel("Upstream  ").SetText(cfg.Upstream).SetFieldWidth(30),
	}
	fieldIdx := 0

	saveAndExit := func() {
		newCfg := *cfg
		newCfg.Username = fields[1].GetText()
		newCfg.Password = fields[2].GetText()
		if fields[3].GetText() != "" {
			newCfg.Upstream = fields[3].GetText()
		}
		newPort := fields[0].GetText()
		if newPort != "" {
			newCfg.ListenAddr = ":" + newPort
		}
		restartConfig = &newCfg
		app.Stop()
	}

	form := tview.NewForm()
	for _, f := range fields {
		form.AddFormItem(f)
	}

	form.SetTitle(" Settings [ESC] ").SetBorder(true)
	form.SetTitleColor(tcell.GetColor("#FFD93D"))
	form.SetBorderColor(tcell.GetColor("#FFD93D"))
	form.SetBackgroundColor(tcell.NewRGBColor(15, 15, 25))
	form.SetFieldBackgroundColor(tcell.NewRGBColor(30, 30, 45))
	form.SetFieldTextColor(tcell.GetColor("#FFFFFF"))
	form.SetLabelColor(tcell.GetColor("#CCCCCC"))

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyUp:
			if fieldIdx > 0 {
				fieldIdx--
				app.SetFocus(fields[fieldIdx])
			}
			return nil
		case tcell.KeyDown:
			if fieldIdx < len(fields)-1 {
				fieldIdx++
				app.SetFocus(fields[fieldIdx])
			}
			return nil
		case tcell.KeyEnter:
			saveAndExit()
			return nil
		case tcell.KeyEscape:
			c := *cfg
			restartConfig = &c
			app.Stop()
			return nil
		}
		return event
	})

	hintText := tview.NewTextView().SetDynamicColors(true).
		SetText("[#888888]Up/Down : navigate     Enter : save      ESC : cancel")

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(hintText, 1, 0, false)

	app.SetRoot(layout, true)
	app.SetFocus(fields[0])
}

func updateInfo(tv *tview.TextView, publicIP, port string, cfg Config) {
	s.mu.RLock()
	totalUp := s.totalUp
	totalDown := s.totalDown
	activeCount := len(s.active)
	totalConns := s.totalConns
	s.mu.RUnlock()

	uptime := formatDuration(time.Since(startTime))
	v2rayLink := fmt.Sprintf("socks://%s:%s#DeltaShare", publicIP, port)
	if cfg.Username != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(cfg.Username + ":" + cfg.Password))
		v2rayLink = fmt.Sprintf("socks://%s@%s:%s#DeltaShare", cred, publicIP, port)
	}

	tv.Clear()
	fmt.Fprintf(tv, "[#CCCCCC] Address    [#FFFFFF]%s:%s\n", publicIP, port)
	fmt.Fprintf(tv, "[#CCCCCC] Uptime     [#6BCB77]%s\n\n", uptime)
	fmt.Fprintf(tv, "[#FFD93D] V2RayNG\n")
	fmt.Fprintf(tv, "[#5DADE2] %s\n\n", v2rayLink)
	fmt.Fprintf(tv, "[#CCCCCC] Upload     [#FFD93D]%s\n", humanBytes(totalUp))
	fmt.Fprintf(tv, "[#CCCCCC] Download   [#FFD93D]%s\n", humanBytes(totalDown))
	fmt.Fprintf(tv, "[#CCCCCC] Active     [#6BCB77]%d[#CCCCCC]   Total [#FFFFFF]%d\n\n", activeCount, totalConns)
	fmt.Fprintf(tv, " [#5DADE2][S] Settings    [#FFFFFF][Q] Quit")
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

	headers := []string{"ID", "Upload", "Download", "Time"}
	for c, h := range headers {
		cell := tview.NewTableCell(h).
			SetTextColor(tcell.GetColor("#FFD93D")).
			SetSelectable(false).
			SetExpansion(1)
		table.SetCell(0, c, cell)
	}

	for i, cs := range recent {
		row := i + 1
		up := humanBytes(cs.upload.Load())
		down := humanBytes(cs.download.Load())
		dur := formatDuration(time.Since(cs.start))

		table.SetCell(row, 0, tview.NewTableCell(fmt.Sprintf("#%d", cs.id)).SetExpansion(1))
		table.SetCell(row, 1, tview.NewTableCell(up).SetExpansion(1))
		table.SetCell(row, 2, tview.NewTableCell(down).SetExpansion(1))
		table.SetCell(row, 3, tview.NewTableCell(dur).SetExpansion(1))
	}
}


func detectPublicIP() string {
	type result struct {
		ip  string
		err error
	}
	ch := make(chan result, 3)
	endpoints := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}
	for _, url := range endpoints {
		go func(u string) {
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Get(u)
			if err != nil {
				ch <- result{"", err}
				return
			}
			var buf [64]byte
			n, _ := resp.Body.Read(buf[:])
			resp.Body.Close()
			if n > 0 {
				ip := strings.TrimSpace(string(buf[:n]))
				if net.ParseIP(ip) != nil {
					ch <- result{ip, nil}
					return
				}
			}
			ch <- result{"", fmt.Errorf("invalid ip")}
		}(url)
	}
	for range endpoints {
		r := <-ch
		if r.err == nil && r.ip != "" {
			return r.ip
		}
	}
	return ""
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

func copyToClipboard(text string) {
	switch runtime.GOOS {
	case "windows":
		tmp, err := os.CreateTemp("", "deltashare-clip-*.txt")
		if err != nil {
			return
		}
		tmpName := tmp.Name()
		tmp.WriteString(text)
		tmp.Close()
		defer os.Remove(tmpName)
		exec.Command("cmd", "/c", "clip < \""+tmpName+"\"").Run()
	case "darwin":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		cmd.Run()
	default:
		cmd := exec.Command("xclip", "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(text)
		cmd.Run()
	}
}

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
