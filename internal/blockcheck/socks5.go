package blockcheck

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"
)

type socks5DialerImpl struct {
	proxyAddr string
	timeout   time.Duration
	username  string
	password  string
}

func makeSocks5Dialer(proxyURL *url.URL, timeout time.Duration) (*socks5DialerImpl, error) {
	d := &socks5DialerImpl{
		proxyAddr: proxyURL.Host,
		timeout:   timeout,
	}
	if proxyURL.User != nil {
		d.username = proxyURL.User.Username()
		d.password, _ = proxyURL.User.Password()
	}
	return d, nil
}

func (d *socks5DialerImpl) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", d.proxyAddr, d.timeout)
	if err != nil {
		return nil, fmt.Errorf("socks5 connect: %w", err)
	}
	if d.timeout > 0 {
		conn.SetDeadline(time.Now().Add(d.timeout))
	}
	if err := d.handshake(conn, addr); err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	return conn, nil
}

func (d *socks5DialerImpl) handshake(conn net.Conn, targetAddr string) error {
	buf := make([]byte, 0, 512)

	if d.username != "" {
		buf = append(buf, 0x05, 0x02, 0x00, 0x02)
	} else {
		buf = append(buf, 0x05, 0x01, 0x00)
	}
	if _, err := conn.Write(buf); err != nil {
		return fmt.Errorf("socks5 greeting: %w", err)
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("socks5 greeting response: %w", err)
	}
	if resp[0] != 0x05 {
		return fmt.Errorf("socks5: invalid version %d", resp[0])
	}

	if resp[1] == 0x02 {
		if d.username == "" {
			return fmt.Errorf("socks5: server requires auth but none provided")
		}
		auth := make([]byte, 0, 512)
		auth = append(auth, 0x01)
		auth = append(auth, byte(len(d.username)))
		auth = append(auth, []byte(d.username)...)
		auth = append(auth, byte(len(d.password)))
		auth = append(auth, []byte(d.password)...)
		if _, err := conn.Write(auth); err != nil {
			return fmt.Errorf("socks5 auth write: %w", err)
		}
		authResp := make([]byte, 2)
		if _, err := io.ReadFull(conn, authResp); err != nil {
			return fmt.Errorf("socks5 auth response: %w", err)
		}
		if authResp[1] != 0x00 {
			return fmt.Errorf("socks5: auth failed (code %d)", authResp[1])
		}
	} else if resp[1] != 0x00 {
		return fmt.Errorf("socks5: unsupported auth method %d", resp[1])
	}

	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return fmt.Errorf("socks5: bad target addr %q: %w", targetAddr, err)
	}
	port, _ := binaryUvarint(portStr)

	req := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req = append(req, 0x01)
			req = append(req, ip4...)
		} else {
			req = append(req, 0x04)
			req = append(req, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return fmt.Errorf("socks5: hostname too long")
		}
		req = append(req, 0x03, byte(len(host)))
		req = append(req, []byte(host)...)
	}
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	req = append(req, portBytes...)

	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("socks5 connect request: %w", err)
	}

	reply := make([]byte, 4)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return fmt.Errorf("socks5 connect response: %w", err)
	}
	if reply[1] != 0x00 {
		return fmt.Errorf("socks5: connect failed (code %d)", reply[1])
	}

	var skip int
	switch reply[3] {
	case 0x01:
		skip = 4
	case 0x03:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return fmt.Errorf("socks5: read domain length: %w", err)
		}
		skip = int(lenBuf[0])
	case 0x04:
		skip = 16
	default:
		return fmt.Errorf("socks5: unknown address type %d", reply[3])
	}
	skipBuf := make([]byte, skip+2)
	if _, err := io.ReadFull(conn, skipBuf); err != nil {
		return fmt.Errorf("socks5: read trailing addr: %w", err)
	}

	return nil
}

func binaryUvarint(s string) (uint64, error) {
	var n uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid port")
		}
		n = n*10 + uint64(c-'0')
	}
	return n, nil
}
