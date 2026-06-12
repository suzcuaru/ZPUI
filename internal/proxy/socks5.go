package proxy

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"zpui/internal/config"
	"zpui/internal/logger"
)

type ConnectionState int

const (
	StateActive ConnectionState = iota
	StateClosed
)

type Connection struct {
	ID          int64           `json:"id"`
	ClientIP    string          `json:"client_ip"`
	ClientPort  int             `json:"client_port"`
	TargetAddr  string          `json:"target_addr"`
	TargetPort  int             `json:"target_port"`
	BytesSent   int64           `json:"bytes_sent"`
	BytesRecv   int64           `json:"bytes_recv"`
	State       ConnectionState `json:"state"`
	ConnectedAt time.Time       `json:"connected_at"`
	ClosedAt    *time.Time      `json:"closed_at,omitempty"`
	mu          sync.Mutex
}

type SOCKS5Server struct {
	cfg         *config.Config
	log         *logger.Logger
	listener    net.Listener
	connections map[int64]*Connection
	mu          sync.RWMutex
	connIDGen   atomic.Int64
	running     bool
	stopCh      chan struct{}

	totalBytesSent atomic.Int64
	totalBytesRecv atomic.Int64

	activeConns atomic.Int64
}

func NewSOCKS5(cfg *config.Config, log *logger.Logger) *SOCKS5Server {
	return &SOCKS5Server{
		cfg:         cfg,
		log:         log,
		connections: make(map[int64]*Connection),
		stopCh:      make(chan struct{}),
	}
}

func (s *SOCKS5Server) Start() error {
	s.mu.Lock()
	s.connections = make(map[int64]*Connection)
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	pcfg := s.cfg.GetProxyConfig()
	addr := fmt.Sprintf(":%d", pcfg.Port)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	s.listener = ln
	s.running = true

	s.addFirewallRule(pcfg.Port)

	s.log.Info("proxy", fmt.Sprintf("SOCKS5 proxy started on %s", addr))
	s.log.Network(fmt.Sprintf("SOCKS5 proxy listening on port %d", pcfg.Port))

	go s.acceptLoop()
	go s.cleanupLoop()
	return nil
}

func (s *SOCKS5Server) Stop() {
	if !s.running {
		return
	}
	s.running = false

	s.mu.Lock()
	if s.stopCh != nil {
		select {
		case <-s.stopCh:
		default:
			close(s.stopCh)
		}
	}
	s.mu.Unlock()

	if s.listener != nil {
		s.listener.Close()
	}

	s.removeFirewallRule()

	s.mu.Lock()
	for _, conn := range s.connections {
		conn.State = StateClosed
	}
	s.mu.Unlock()

	s.log.Info("proxy", "SOCKS5 proxy stopped")
}

func (s *SOCKS5Server) IsRunning() bool {
	return s.running
}

func (s *SOCKS5Server) acceptLoop() {
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			if !s.running {
				return
			}
			s.log.Error("proxy", fmt.Sprintf("Accept error: %v", err))
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *SOCKS5Server) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			for id, conn := range s.connections {
				if conn.State == StateClosed {
					delete(s.connections, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *SOCKS5Server) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	clientConn.SetDeadline(time.Now().Add(30 * time.Second))

	if err := s.socks5Handshake(clientConn); err != nil {
		s.log.Error("proxy", fmt.Sprintf("Handshake failed from %s: %v", clientConn.RemoteAddr(), err))
		return
	}

	targetAddr, targetPort, err := s.socks5Request(clientConn)
	if err != nil {
		s.log.Error("proxy", fmt.Sprintf("Request failed from %s: %v", clientConn.RemoteAddr(), err))
		return
	}

	clientConn.SetDeadline(time.Time{})

	target := fmt.Sprintf("%s:%d", targetAddr, targetPort)
	targetConn, err := net.DialTimeout("tcp", target, 15*time.Second)
	if err != nil {
		s.socks5Response(clientConn, 0x05, nil, 0)
		s.log.Error("proxy", fmt.Sprintf("Connect to %s failed: %v", target, err))
		return
	}
	defer targetConn.Close()

	connID := s.connIDGen.Add(1)
	clientIP, clientPort := parseAddr(clientConn.RemoteAddr().String())

	conn := &Connection{
		ID:          connID,
		ClientIP:    clientIP,
		ClientPort:  clientPort,
		TargetAddr:  targetAddr,
		TargetPort:  targetPort,
		State:       StateActive,
		ConnectedAt: time.Now(),
	}

	s.mu.Lock()
	s.connections[connID] = conn
	s.mu.Unlock()
	s.activeConns.Add(1)

	s.socks5Response(clientConn, 0x00, targetConn.LocalAddr(), 0)

	s.log.Network(fmt.Sprintf("Connection #%d: %s -> %s:%d", connID, clientIP, targetAddr, targetPort))

	done := make(chan struct{}, 2)

	go s.relay(clientConn, targetConn, conn, true, done)
	go s.relay(targetConn, clientConn, conn, false, done)

	<-done
	<-done

	conn.State = StateClosed
	now := time.Now()
	conn.ClosedAt = &now
	s.activeConns.Add(-1)

	s.log.Network(fmt.Sprintf("Connection #%d closed (sent: %d, recv: %d bytes)",
		connID, conn.BytesSent, conn.BytesRecv))
}

func (s *SOCKS5Server) socks5Handshake(conn net.Conn) error {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	if buf[0] != 0x05 {
		return fmt.Errorf("not SOCKS5: version %d", buf[0])
	}

	nMethods := int(buf[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return fmt.Errorf("read methods: %w", err)
	}

	pcfg := s.cfg.GetProxyConfig()
	needAuth := pcfg.Username != "" && pcfg.Password != ""

	if needAuth {
		if !containsByte(methods, 0x02) {
			conn.Write([]byte{0x05, 0xFF})
			return fmt.Errorf("client does not support auth")
		}
		conn.Write([]byte{0x05, 0x02})

		authBuf := make([]byte, 2)
		if _, err := io.ReadFull(conn, authBuf); err != nil {
			return fmt.Errorf("read auth version: %w", err)
		}

		userLen := int(authBuf[1])
		userBuf := make([]byte, userLen)
		if _, err := io.ReadFull(conn, userBuf); err != nil {
			return fmt.Errorf("read username: %w", err)
		}

		passLenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, passLenBuf); err != nil {
			return fmt.Errorf("read pass len: %w", err)
		}

		passBuf := make([]byte, int(passLenBuf[0]))
		if _, err := io.ReadFull(conn, passBuf); err != nil {
			return fmt.Errorf("read password: %w", err)
		}

		if string(userBuf) != pcfg.Username || string(passBuf) != pcfg.Password {
			conn.Write([]byte{0x01, 0x01})
			return fmt.Errorf("auth failed")
		}

		conn.Write([]byte{0x01, 0x00})
	} else {
		conn.Write([]byte{0x05, 0x00})
	}

	return nil
}

func (s *SOCKS5Server) socks5Request(conn net.Conn) (string, int, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", 0, fmt.Errorf("read request header: %w", err)
	}

	if header[0] != 0x05 {
		return "", 0, fmt.Errorf("invalid version in request: %d", header[0])
	}
	if header[1] != 0x01 {
		return "", 0, fmt.Errorf("unsupported command: %d", header[1])
	}

	var targetAddr string
	switch header[3] {
	case 0x01:
		ipv4 := make([]byte, 4)
		if _, err := io.ReadFull(conn, ipv4); err != nil {
			return "", 0, err
		}
		targetAddr = net.IP(ipv4).String()

	case 0x03:
		domainLen := make([]byte, 1)
		if _, err := io.ReadFull(conn, domainLen); err != nil {
			return "", 0, err
		}
		domain := make([]byte, domainLen[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", 0, err
		}
		targetAddr = string(domain)

	case 0x04:
		ipv6 := make([]byte, 16)
		if _, err := io.ReadFull(conn, ipv6); err != nil {
			return "", 0, err
		}
		targetAddr = net.IP(ipv6).String()

	default:
		return "", 0, fmt.Errorf("unsupported address type: %d", header[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", 0, err
	}
	targetPort := int(binary.BigEndian.Uint16(portBuf))

	return targetAddr, targetPort, nil
}

func (s *SOCKS5Server) socks5Response(conn net.Conn, rep byte, bindAddr net.Addr, bindPort int) {
	resp := []byte{0x05, rep, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	conn.Write(resp)
}

func (s *SOCKS5Server) relay(dst, src net.Conn, conn *Connection, isUpload bool, done chan struct{}) {
	defer func() { done <- struct{}{} }()

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		n, err := src.Read(buf)
		if n > 0 {
			conn.mu.Lock()
			if isUpload {
				conn.BytesSent += int64(n)
				s.totalBytesSent.Add(int64(n))
			} else {
				conn.BytesRecv += int64(n)
				s.totalBytesRecv.Add(int64(n))
			}
			conn.mu.Unlock()

			if _, err := dst.Write(buf[:n]); err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *SOCKS5Server) GetConnections() []*Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Connection, 0, len(s.connections))
	for _, conn := range s.connections {
		if conn.State == StateActive {
			result = append(result, conn)
		}
	}
	return result
}

func (s *SOCKS5Server) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	active := 0
	var totalSent, totalRecv int64

	for _, conn := range s.connections {
		conn.mu.Lock()
		totalSent += conn.BytesSent
		totalRecv += conn.BytesRecv
		conn.mu.Unlock()
		if conn.State == StateActive {
			active++
		}
	}

	return map[string]interface{}{
		"active_connections": active,
		"total_bytes_sent":   totalSent,
		"total_bytes_recv":   totalRecv,
		"running":            s.running,
	}
}

func (s *SOCKS5Server) GetActiveConnectionsByClient() map[string][]*Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]*Connection)
	for _, conn := range s.connections {
		if conn.State == StateActive {
			result[conn.ClientIP] = append(result[conn.ClientIP], conn)
		}
	}
	return result
}

func containsByte(s []byte, b byte) bool {
	for _, v := range s {
		if v == b {
			return true
		}
	}
	return false
}

func parseAddr(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 0
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}

func (s *SOCKS5Server) addFirewallRule(port int) {
	portStr := fmt.Sprintf("%d", port)
	exec.Command("netsh", "advfirewall", "firewall", "delete", "rule",
		"name=ZPUI SOCKS5 Proxy").Run()
	exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name=ZPUI SOCKS5 Proxy",
		"dir=in", "action=allow", "protocol=TCP",
		"localport="+portStr).Run()
	s.log.Info("proxy", fmt.Sprintf("Firewall rule added for port %d", port))
}

func (s *SOCKS5Server) removeFirewallRule() {
	exec.Command("netsh", "advfirewall", "firewall", "delete", "rule",
		"name=ZPUI SOCKS5 Proxy").Run()
	s.log.Info("proxy", "Firewall rule removed")
}
