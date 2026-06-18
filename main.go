package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	ListenAddr string
	Upstream   string
	Username   string
	Password   string
}

func main() {
	cfg := Config{}

	flag.StringVar(&cfg.ListenAddr, "listen", ":7373", "Local SOCKS5 listen address")
	flag.StringVar(&cfg.Upstream, "upstream", "127.0.0.1:10808", "Upstream SOCKS5 proxy address")
	flag.StringVar(&cfg.Username, "user", "", "Username for client auth (empty = no auth)")
	flag.StringVar(&cfg.Password, "pass", "", "Password for client auth")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("DeltaShare starting | listen=%s upstream=%s auth=%v",
		cfg.ListenAddr, cfg.Upstream, cfg.Username != "")

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()
	log.Printf("Listening on %s", ln.Addr().String())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		ln.Close()
	}()

	var wg sync.WaitGroup
	var connID uint64

	for {
		conn, err := ln.Accept()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				break
			}
			log.Printf("accept error: %v", err)
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
	log.Println("DeltaShare stopped")
}

func handleConn(client net.Conn, id uint64, cfg Config) {
	defer client.Close()
	start := time.Now()

	dest, err := serverHandshake(client, cfg)
	if err != nil {
		log.Printf("[#%d] handshake: %v", id, err)
		return
	}
	log.Printf("[#%d] CONNECT %s", id, dest)

	upstream, err := net.DialTimeout("tcp", cfg.Upstream, 10*time.Second)
	if err != nil {
		log.Printf("[#%d] upstream dial: %v", id, err)
		return
	}
	defer upstream.Close()

	if err := clientHandshake(upstream, dest, cfg); err != nil {
		log.Printf("[#%d] upstream handshake: %v", id, err)
		return
	}

	// Send CONNECT success reply to client
	client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	up, down := relay(client, upstream)
	log.Printf("[#%d] done | ↑%s ↓%s %v",
		id, humanBytes(up), humanBytes(down), time.Since(start).Round(time.Millisecond))
}

// ─── SOCKS5 Server Handshake ───────────────────────────────────────────────

func serverHandshake(conn net.Conn, cfg Config) (string, error) {
	buf := make([]byte, 258)

	// VER | NMETHODS | METHODS
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

	// VER | METHOD
	if _, err := conn.Write([]byte{0x05, method}); err != nil {
		return "", err
	}

	if method == 0x02 {
		if err := authServer(conn, cfg); err != nil {
			return "", err
		}
	}

	// VER | CMD | RSV | ATYP | ADDR... | PORT(2)
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return "", err
	}
	if buf[1] != 0x01 { // only CONNECT
		conn.Write(socksReply(0x07, nil))
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
	return 0x00 // best effort
}

func authServer(conn net.Conn, cfg Config) error {
	var buf [513]byte
	if _, err := io.ReadFull(conn, buf[:2]); err != nil { // VER | ULEN
		return err
	}
	uLen := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:uLen]); err != nil { // UNAME
		return err
	}
	username := string(buf[:uLen])
	if _, err := io.ReadFull(conn, buf[:1]); err != nil { // PLEN
		return err
	}
	pLen := int(buf[0])
	if _, err := io.ReadFull(conn, buf[:pLen]); err != nil { // PASSWD
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
	case 0x01: // IPv4
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		host = net.IP(buf).String()
	case 0x03: // Domain
		var dLen uint8
		if err := binary.Read(conn, binary.BigEndian, &dLen); err != nil {
			return "", err
		}
		buf := make([]byte, dLen)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		host = string(buf)
	case 0x04: // IPv6
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

func socksReply(rep byte, bind net.Addr) []byte {
	reply := make([]byte, 10) // VER RSV ATYP(0x01) IP(4) PORT(2)
	reply[0] = 0x05
	reply[1] = rep
	reply[2] = 0x00
	reply[3] = 0x01 // IPv4
	// bytes 4-7 = 0.0.0.0
	if bind != nil {
		if tcpAddr, ok := bind.(*net.TCPAddr); ok {
			copy(reply[4:8], tcpAddr.IP.To4())
			binary.BigEndian.PutUint16(reply[8:10], uint16(tcpAddr.Port))
		}
	}
	return reply
}

// ─── SOCKS5 Client Handshake (connect to upstream) ────────────────────────

func clientHandshake(conn net.Conn, dest string, cfg Config) error {
	// Send: VER | NMETHODS | METHODS
	// We support no-auth (0x00) to upstream
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}

	// Read: VER | METHOD
	var resp [2]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		return err
	}
	if resp[0] != 0x05 {
		return fmt.Errorf("upstream version %d", resp[1])
	}

	// Upstream typically doesn't require auth; skip if method is 0x00
	if resp[1] != 0x00 {
		return fmt.Errorf("upstream requested unsupported method: %d", resp[1])
	}

	// Build CONNECT request
	host, portStr, err := net.SplitHostPort(dest)
	if err != nil {
		return err
	}
	var port uint16
	fmt.Sscanf(portStr, "%d", &port)

	req := []byte{0x05, 0x01, 0x00} // VER CMD RSV

	ip := net.ParseIP(host)
	if ip4 := ip.To4(); ip4 != nil {
		req = append(req, 0x01)
		req = append(req, ip4...)
	} else if ip6 := ip.To16(); ip6 != nil {
		req = append(req, 0x04)
		req = append(req, ip6...)
	} else {
		// Domain
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

	// Read reply
	var reply [10]byte
	if _, err := io.ReadFull(conn, reply[:4]); err != nil {
		return err
	}
	if reply[1] != 0x00 {
		return fmt.Errorf("upstream CONNECT failed: %d", reply[1])
	}
	// Skip the rest of the reply (address)
	skipAddr(reply[3], conn)

	return nil
}

func authClient(conn net.Conn, cfg Config) error {
	msg := []byte{0x01}
	msg = append(msg, byte(len(cfg.Username)))
	msg = append(msg, cfg.Username...)
	msg = append(msg, byte(len(cfg.Password)))
	msg = append(msg, cfg.Password...)
	if _, err := conn.Write(msg); err != nil {
		return err
	}
	var resp [2]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		return err
	}
	if resp[1] != 0x00 {
		return fmt.Errorf("upstream auth rejected")
	}
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

func relay(a, b net.Conn) (up, down int64) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		n, _ := io.Copy(b, a)
		up = n
		setCloseWrite(b)
	}()

	go func() {
		defer wg.Done()
		n, _ := io.Copy(a, b)
		down = n
		setCloseWrite(a)
	}()

	wg.Wait()
	return
}

func setCloseWrite(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.CloseWrite()
	}
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
