// Package proxy — локальный SOCKS5-сервер для устройств домашней сети:
// телефон/планшет указывает в настройках proxy IP компьютера и порт —
// трафик идёт через подключение ПК. Работает всё время, пока запущено
// приложение (в максимальной экономии отключается, см. app_socks.go).
package proxy

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"zpui/internal/config"
	"zpui/internal/executil"
	"zpui/internal/logger"
	"zpui/internal/power"
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
	client      net.Conn
	target      net.Conn
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
	wg          sync.WaitGroup

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
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.connections = make(map[int64]*Connection)
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	addr := fmt.Sprintf("0.0.0.0:%d", s.cfg.GetSocksProxyPort())

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	s.mu.Lock()
	s.listener = ln
	s.running = true
	s.mu.Unlock()

	s.addFirewallRule(s.cfg.GetSocksProxyPort())

	s.log.Info("proxy", fmt.Sprintf("SOCKS5 proxy started on %s", addr))
	s.log.Network(fmt.Sprintf("SOCKS5 proxy listening on port %d", s.cfg.GetSocksProxyPort()))

	go s.acceptLoop()
	go s.cleanupLoop()
	return nil
}

func (s *SOCKS5Server) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	if s.stopCh != nil {
		select {
		case <-s.stopCh:
		default:
			close(s.stopCh)
		}
	}
	ln := s.listener
	var open []*Connection
	for _, conn := range s.connections {
		conn.mu.Lock()
		conn.State = StateClosed
		conn.mu.Unlock()
		open = append(open, conn)
	}
	s.mu.Unlock()

	if ln != nil {
		ln.Close()
	}
	// Активные релеи рвём закрытием сокетов, иначе горутины продолжают
	// качать трафик после остановки прокси.
	for _, conn := range open {
		conn.mu.Lock()
		if conn.client != nil {
			conn.client.Close()
		}
		if conn.target != nil {
			conn.target.Close()
		}
		conn.mu.Unlock()
	}
	s.wg.Wait()

	s.removeFirewallRule()

	s.log.Info("proxy", "SOCKS5 proxy stopped")
}

func (s *SOCKS5Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

func (s *SOCKS5Server) acceptLoop() {
	// Локальная копия: Stop→Start перезаписывает s.listener под мьютексом —
	// чтение поля в цикле было бы гонкой с перезаписью.
	s.mu.RLock()
	ln := s.listener
	s.mu.RUnlock()
	if ln == nil {
		return
	}
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		conn, err := ln.Accept()
		if err != nil {
			if !s.IsRunning() {
				return
			}
			s.log.Error("proxy", fmt.Sprintf("Accept error: %v", err))
			continue
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *SOCKS5Server) cleanupLoop() {
	for {
		select {
		case <-s.stopCh:
			return
		case <-time.After(power.Stretch(30 * time.Second)):
		}
		s.mu.Lock()
		for id, conn := range s.connections {
			conn.mu.Lock()
			closed := conn.State == StateClosed
			conn.mu.Unlock()
			if closed {
				delete(s.connections, id)
			}
		}
		s.mu.Unlock()
	}
}

func (s *SOCKS5Server) handleConnection(clientConn net.Conn) {
	defer s.wg.Done()
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

	target := net.JoinHostPort(targetAddr, strconv.Itoa(targetPort))
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
		client:      clientConn,
		target:      targetConn,
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

	conn.mu.Lock()
	conn.State = StateClosed
	now := time.Now()
	conn.ClosedAt = &now
	conn.mu.Unlock()
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

	// Аутентификации нет: прокси только для домашней сети, без пароля.
	nMethods := int(buf[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return fmt.Errorf("read methods: %w", err)
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return fmt.Errorf("write method: %w", err)
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

// GetStats — срез для статуса: активные соединения и суммарный трафик.
func (s *SOCKS5Server) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	active := 0
	for _, conn := range s.connections {
		conn.mu.Lock()
		isActive := conn.State == StateActive
		conn.mu.Unlock()
		if isActive {
			active++
		}
	}

	return map[string]interface{}{
		"active_connections": active,
		"total_bytes_sent":   s.totalBytesSent.Load(),
		"total_bytes_recv":   s.totalBytesRecv.Load(),
		"running":            s.running,
	}
}

// ClientsByIP — активные клиенты по IP (вкладка «Мониторинг»).
func (s *SOCKS5Server) ClientsByIP() map[string][]*Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]*Connection)
	for _, conn := range s.connections {
		conn.mu.Lock()
		isActive := conn.State == StateActive
		conn.mu.Unlock()
		if isActive {
			result[conn.ClientIP] = append(result[conn.ClientIP], conn)
		}
	}
	return result
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
	executil.HiddenCmd("netsh", "advfirewall", "firewall", "delete", "rule",
		"name=ZPUI SOCKS5 Proxy").Run()
	// remoteip=localsubnet: правило только для домашней сети — без него
	// прокси без аутентификации открыт и извне подсети.
	executil.HiddenCmd("netsh", "advfirewall", "firewall", "add", "rule",
		"name=ZPUI SOCKS5 Proxy",
		"dir=in", "action=allow", "protocol=TCP",
		"localport="+portStr, "remoteip=localsubnet").Run()
	s.log.Info("proxy", fmt.Sprintf("Firewall rule added for port %d", port))
}

func (s *SOCKS5Server) removeFirewallRule() {
	executil.HiddenCmd("netsh", "advfirewall", "firewall", "delete", "rule",
		"name=ZPUI SOCKS5 Proxy").Run()
	s.log.Info("proxy", "Firewall rule removed")
}
