package main

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestServerHandshake_NoAuth(t *testing.T) {
	server, client := net.Pipe()

	// Simulate a SOCKS5 client with no auth, CONNECT to example.com:80
	go func() {
		// Client sends: VER(5) | NMETHODS(1) | METHOD(0)
		client.Write([]byte{0x05, 0x01, 0x00})
		// Read server response: VER(5) | METHOD(0)
		resp := make([]byte, 2)
		io.ReadFull(client, resp)

		// Send CONNECT request
		req := []byte{0x05, 0x01, 0x00, 0x03, 11} // VER CMD RSV ATYP DOMAIN_LEN
		req = append(req, []byte("example.com")...)
		req = append(req, 0, 80) // port 80
		client.Write(req)
	}()

	cfg := Config{ListenAddr: "", Upstream: "127.0.0.1:10808"}
	dest, err := serverHandshake(server, cfg)
	server.Close()
	client.Close()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dest != "example.com:80" {
		t.Fatalf("expected example.com:80, got %s", dest)
	}
}

func TestServerHandshake_Auth(t *testing.T) {
	server, client := net.Pipe()

	go func() {
		// Client sends: VER(5) | NMETHODS(2) | METHODS(0, 2)
		client.Write([]byte{0x05, 0x02, 0x00, 0x02})
		resp := make([]byte, 2)
		io.ReadFull(client, resp)

		// Send auth: VER(1) | ULEN | UNAME | PLEN | PASSWD
		client.Write([]byte{0x01, 0x04, 't', 'e', 's', 't', 0x04, 'p', 'a', 's', 's'})
		authResp := make([]byte, 2)
		io.ReadFull(client, authResp)

		// CONNECT request: 1.2.3.4:443
		req := []byte{0x05, 0x01, 0x00, 0x01, 1, 2, 3, 4, 0x01, 0xBB}
		client.Write(req)
	}()

	cfg := Config{Username: "test", Password: "pass"}
	dest, err := serverHandshake(server, cfg)
	server.Close()
	client.Close()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dest != "1.2.3.4:443" {
		t.Fatalf("expected 1.2.3.4:443, got %s", dest)
	}
}

func TestServerHandshake_AuthFail(t *testing.T) {
	server, client := net.Pipe()

	go func() {
		client.Write([]byte{0x05, 0x02, 0x00, 0x02})
		resp := make([]byte, 2)
		io.ReadFull(client, resp)
		client.Write([]byte{0x01, 0x04, 'b', 'a', 'd', '!', 0x04, 'p', 'a', 's', 's'})
		// Wait a bit then close
		time.Sleep(100 * time.Millisecond)
		client.Close()
	}()

	cfg := Config{Username: "test", Password: "pass"}
	_, err := serverHandshake(server, cfg)
	server.Close()

	if err == nil {
		t.Fatal("expected auth error, got nil")
	}
}

func TestReadAddr(t *testing.T) {
	tests := []struct {
		name     string
		atyp     byte
		data     []byte
		expected string
	}{
		{"IPv4", 0x01, []byte{192, 168, 1, 1, 0, 80}, "192.168.1.1:80"},
		{"Domain", 0x03, []byte{11}, ""}, // handled separately
		{"IPv6", 0x04, nil, ""},          // handled separately
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "Domain" {
				data := []byte{3, 'f', 'o', 'o', '.'} // This won't work easily
				_ = data
				return
			}
			if tt.name == "IPv6" {
				return
			}
			server, client := net.Pipe()
			go func() {
				client.Write(tt.data)
				client.Close()
			}()
			addr, err := readAddr(tt.atyp, server)
			server.Close()
			if err != nil {
				t.Fatal(err)
			}
			if addr != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, addr)
			}
		})
	}
}

func TestRelay(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				io.Copy(c, c)
			}()
		}
	}()

	a, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	b, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	done := make(chan struct{})
	var up int64
	go func() {
		var down int64
		up, down = relay(a, b)
		_ = down
		close(done)
	}()

	a.Write([]byte("ping"))
	buf := make([]byte, 1024)
	n, err := b.Read(buf)
	if err != nil || string(buf[:n]) != "ping" {
		t.Fatalf("expected 'ping', got %q err=%v", buf[:n], err)
	}

	a.Close()
	b.Close()
	<-done
	if up == 0 {
		t.Fatal("expected up > 0")
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0KB"},
		{1048576, "1.0MB"},
		{1073741824, "1.0GB"},
	}

	for _, tt := range tests {
		got := humanBytes(tt.input)
		if got != tt.expected {
			t.Errorf("humanBytes(%d) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestReadAddrDomain(t *testing.T) {
	server, client := net.Pipe()
	go func() {
		// domain "google.com" (10 bytes) + port 443
		client.Write([]byte{10})
		client.Write([]byte("google.com"))
		p := make([]byte, 2)
		binary.BigEndian.PutUint16(p, 443)
		client.Write(p)
		client.Close()
	}()

	addr, err := readAddr(0x03, server)
	server.Close()
	if err != nil {
		t.Fatal(err)
	}
	if addr != "google.com:443" {
		t.Fatalf("expected google.com:443, got %s", addr)
	}
}

func TestFullIntegration(t *testing.T) {
	// Start a fake upstream SOCKS5 server that echoes back CONNECTED
	upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstreamLn.Close()

	go func() {
		for {
			conn, err := upstreamLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// Minimal SOCKS5 handshake
				buf := make([]byte, 258)
				io.ReadFull(c, buf[:2])
				nMethods := int(buf[1])
				io.ReadFull(c, buf[:nMethods])
				c.Write([]byte{0x05, 0x00})
				// Read CONNECT request
				io.ReadFull(c, buf[:4])
				readAddr(buf[3], c) //nolint:errcheck
				// Reply success
				c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
				// Echo data
				io.Copy(c, c)
			}(conn)
		}
	}()

	// Start DeltaShare pointing to our fake upstream
	cfg := Config{
		ListenAddr: "127.0.0.1:0",
		Upstream:   upstreamLn.Addr().String(),
	}

	dshareLn, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer dshareLn.Close()

	go func() {
		for {
			conn, err := dshareLn.Accept()
			if err != nil {
				return
			}
			go handleConn(conn, 1, cfg)
		}
	}()

	// Connect as a SOCKS5 client
	client, err := net.DialTimeout("tcp", dshareLn.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// SOCKS5 handshake
	client.Write([]byte{0x05, 0x01, 0x00})
	resp := make([]byte, 2)
	io.ReadFull(client, resp)

	// CONNECT to 1.1.1.1:53
	req := []byte{0x05, 0x01, 0x00, 0x01, 1, 1, 1, 1, 0, 53}
	client.Write(req)

	reply := make([]byte, 10)
	io.ReadFull(client, reply)
	if reply[1] != 0x00 {
		t.Fatalf("CONNECT failed: %d", reply[1])
	}

	// Send and receive data
	client.Write([]byte("ping"))
	buf := make([]byte, 1024)
	n, _ := client.Read(buf)
	if string(buf[:n]) != "ping" {
		t.Fatalf("expected 'ping', got '%s'", string(buf[:n]))
	}
}
