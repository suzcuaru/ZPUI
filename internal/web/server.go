package web

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"zpui/internal/config"
	"zpui/internal/executil"
	"zpui/internal/logger"
	"zpui/internal/monitor"
	"zpui/internal/proxy"
	"zpui/internal/zapret"
)

type Server struct {
	cfg     *config.Config
	log     *logger.Logger
	zapret  *zapret.Manager
	proxy   *proxy.SOCKS5Server
	monitor *monitor.TrafficMonitor
	webFS   embed.FS
	version string
	server  *http.Server
	readyCh chan struct{}
}

func NewServer(
	cfg *config.Config,
	log *logger.Logger,
	zapretMgr *zapret.Manager,
	proxySrv *proxy.SOCKS5Server,
	trafficMon *monitor.TrafficMonitor,
	webFS embed.FS,
	version string,
) *Server {
	return &Server{
		cfg:     cfg,
		log:     log,
		zapret:  zapretMgr,
		proxy:   proxySrv,
		monitor: trafficMon,
		webFS:   webFS,
		version: version,
		readyCh: make(chan struct{}),
	}
}

func (s *Server) GetURL() string {
	if s.server == nil {
		return ""
	}
	return fmt.Sprintf("http://%s", s.server.Addr)
}

func (s *Server) WaitReady() {
	<-s.readyCh
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	staticFS, err := fs.Sub(s.webFS, "web/dist")
	if err != nil {
		return err
	}

	fileServer := http.FileServer(http.FS(staticFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			data, err := fs.ReadFile(staticFS, "index.html")
			if err != nil {
				http.Error(w, "Not found", 404)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}

		if r.URL.Path == "/up" || r.URL.Path == "/up/" {
			data, err := fs.ReadFile(staticFS, "up/index.html")
			if err != nil {
				http.Error(w, "Not found", 404)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}

		fileServer.ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/status", s.cors(s.handleStatus))
	mux.HandleFunc("/api/zapret/start", s.cors(s.handleZapretStart))
	mux.HandleFunc("/api/zapret/stop", s.cors(s.handleZapretStop))
	mux.HandleFunc("/api/zapret/restart", s.cors(s.handleZapretRestart))
	mux.HandleFunc("/api/zapret/strategies", s.cors(s.handleStrategies))
	mux.HandleFunc("/api/zapret/set-strategy", s.cors(s.handleSetStrategy))
	mux.HandleFunc("/api/zapret/service/install", s.cors(s.handleServiceInstall))
	mux.HandleFunc("/api/zapret/service/remove", s.cors(s.handleServiceRemove))
	mux.HandleFunc("/api/zapret/service/status", s.cors(s.handleServiceStatus))
	mux.HandleFunc("/api/zapret/game-filter", s.cors(s.handleGameFilter))
	mux.HandleFunc("/api/zapret/ipset-status", s.cors(s.handleIpsetStatus))
	mux.HandleFunc("/api/zapret/ipset-toggle", s.cors(s.handleIpsetToggle))
	mux.HandleFunc("/api/zapret/auto-update-status", s.cors(s.handleAutoUpdateStatus))
	mux.HandleFunc("/api/zapret/auto-update-toggle", s.cors(s.handleAutoUpdateToggle))
	mux.HandleFunc("/api/zapret/update-ipset", s.cors(s.handleUpdateIpset))
	mux.HandleFunc("/api/zapret/update-hosts", s.cors(s.handleUpdateHosts))

	mux.HandleFunc("/api/proxy/start", s.cors(s.handleProxyStart))
	mux.HandleFunc("/api/proxy/stop", s.cors(s.handleProxyStop))
	mux.HandleFunc("/api/proxy/status", s.cors(s.handleProxyStatus))
	mux.HandleFunc("/api/proxy/connections", s.cors(s.handleProxyConnections))
	mux.HandleFunc("/api/proxy/config", s.cors(s.handleProxyConfig))
	mux.HandleFunc("/api/proxy/qrcode", s.cors(s.handleProxyQRCode))

	mux.HandleFunc("/api/monitor/traffic", s.cors(s.handleTraffic))
	mux.HandleFunc("/api/monitor/devices", s.cors(s.handleDevices))

	mux.HandleFunc("/api/update/check", s.cors(s.handleUpdateCheck))
	mux.HandleFunc("/api/update/apply", s.cors(s.handleUpdateApply))
	mux.HandleFunc("/api/update/stream", s.cors(s.handleUpdateStream))

	mux.HandleFunc("/api/strategy/auto", s.cors(s.handleAutoStrategy))
	mux.HandleFunc("/api/strategy/stream", s.cors(s.handleStrategyStream))
	mux.HandleFunc("/api/strategy/cancel", s.cors(s.handleStrategyCancel))

	mux.HandleFunc("/api/autostart/status", s.cors(s.handleAutostartStatus))
	mux.HandleFunc("/api/autostart/enable", s.cors(s.handleAutostartEnable))
	mux.HandleFunc("/api/autostart/disable", s.cors(s.handleAutostartDisable))

	mux.HandleFunc("/api/logs", s.cors(s.handleLogs))
	mux.HandleFunc("/api/logs/files", s.cors(s.handleLogFiles))
	mux.HandleFunc("/api/config", s.cors(s.handleConfig))
	mux.HandleFunc("/api/zapret/install", s.cors(s.handleZapretInstall))

	mux.HandleFunc("/api/zapret/diagnostics", s.cors(s.handleDiagnostics))
	mux.HandleFunc("/api/zapret/cache/clear", s.cors(s.handleCacheClear))
	mux.HandleFunc("/api/zapret/lists", s.cors(s.handleLists))
	mux.HandleFunc("/api/zapret/lists/save", s.cors(s.handleListsSave))
	mux.HandleFunc("/api/resource-status", s.cors(s.handleResourceStatus))
	mux.HandleFunc("/api/up/info", s.cors(s.handleUpInfo))
	mux.HandleFunc("/api/external", s.cors(s.handleOpenExternal))

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	s.server = &http.Server{
		Addr:         ln.Addr().String(),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	close(s.readyCh)
	return s.server.Serve(ln)
}

func (s *Server) Stop() {
	if s.server != nil {
		s.server.Close()
	}
}

func (s *Server) GetCachedResourcePercent() int {
	resourceCacheMu.Lock()
	if resourceCache == nil {
		resourceCacheMu.Unlock()
		return -1
	}
	data := resourceCache
	resourceCacheMu.Unlock()

	total := 0
	ok := 0
	for _, r := range data.Default {
		total++
		if status, _ := r["status"].(string); status == "ok" {
			ok++
		}
	}
	if total == 0 {
		return -1
	}
	return ok * 100 / total
}

func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(200)
			return
		}

		next(w, r)
	}
}

func getLocalIPs() []string {
	var ips []string
	interfaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range interfaces {
		nameLower := strings.ToLower(iface.Name)
		if strings.Contains(nameLower, "loopback") ||
			strings.Contains(nameLower, "vethernet") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() || ipNet.IP.To4() == nil {
				continue
			}
			ip := ipNet.IP
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			if bytes.HasPrefix(ip, []byte{172, 26}) ||
				bytes.HasPrefix(ip, []byte{172, 20}) ||
				bytes.HasPrefix(ip, []byte{172, 17}) ||
				bytes.HasPrefix(ip, []byte{172, 18}) ||
				bytes.HasPrefix(ip, []byte{172, 19}) {
				continue
			}
			ips = append(ips, ip.String())
		}
	}
	return ips
}

func getMACAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if len(iface.HardwareAddr) > 0 && !strings.HasPrefix(iface.Name, "Loopback") {
			return iface.HardwareAddr.String()
		}
	}
	return ""
}

func getHostname() string {
	cmd := executil.HiddenCmd("hostname")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func getARPTable() map[string]string {
	arp := make(map[string]string)
	cmd := executil.HiddenCmd("arp", "-a")
	output, err := cmd.Output()
	if err != nil {
		return arp
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Interface") || strings.HasPrefix(line, "---") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		host := strings.Trim(fields[0], " ")
		ipRaw := fields[0]
		mac := ""
		if len(fields) >= 2 {
			ipRaw = strings.Trim(fields[0], "()")
			if strings.Contains(fields[0], "(") {
				ipRaw = strings.Trim(fields[1], "()")
				mac = fields[3]
			} else if len(fields) >= 4 {
				mac = fields[1]
				ipRaw = strings.Trim(host, "()")
			}
		}
		if mac != "" && mac != "FF-FF-FF-FF-FF-FF" && mac != "ff-ff-ff-ff-ff-ff" {
			arp[ipRaw] = mac
		}
	}
	return arp
}

func resolveHostname(ip string) string {
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	name := names[0]
	name = strings.TrimSuffix(name, ".")
	return name
}