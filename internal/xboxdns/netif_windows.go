//go:build windows

package xboxdns

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Нативная работа с сетевыми адаптерами Windows: перечисление через
// GetAdaptersAddresses, первичный адаптер через GetBestRoute, чтение DNS из
// реестра, применение через dnsapi!SetInterfaceDnsSettingsEx (семья-раздельный)
// с netsh-fallback. PowerShell не используется: каждый вызов powershell.exe
// стоит сотни МБ working set и сотни миллисекунд.

const (
	ifOperStatusUp = 1

	familyAFInet  = 2
	familyAFInet6 = 23

	tcpipKeyFmt = `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\%s`
	tcpip6Fmt   = `SYSTEM\CurrentControlSet\Services\Tcpip6\Parameters\Interfaces\%s`
)

// winAdapter — сведения об одном сетевом адаптере.
type winAdapter struct {
	Name     string // дружественное имя ("Ethernet") — его понимают netsh и UI
	GUID     string // "{...}" — ключ реестра Tcpip\Parameters\Interfaces
	Index    uint32
	Up       bool
	Virtual  bool
	Gateway  bool
	HasIPv4  bool
	IPv4Addr string
}

var (
	iphlpapi         = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetBestRoute = iphlpapi.NewProc("GetBestRoute")
	dnsapi           = windows.NewLazySystemDLL("dnsapi.dll")
	// Только Ex-вариант: обычная SetInterfaceDnsSettings применяет NameServer
	// к ОБОИМ стекам адаптера — наши IPv4-адреса утекали в IPv6-стек.
	procSetIfaceDnsSettsEx = dnsapi.NewProc("SetInterfaceDnsSettingsEx")
)

const dnsSettingNameserver = 0x00000001

// dnsInterfaceSettingsEx — DNS_INTERFACE_SETTINGS_EX (windns.h, Version 2):
// как v1, но с полем Family, ограничивающим применение одним стеком.
// После Family (uint16) компилятор вставляет паддинг до указателя — явный
// пропуск фиксирует раскладку для обеих разрядностей (386/amd64 совпадают).
type dnsInterfaceSettingsEx struct {
	Version                 uint32
	ChangeCount             uint32
	Flags                   uint32
	Family                  uint16
	_                       uint16
	NameServer              *uint16
	SearchList              *uint16
	RegistrationEnabled     uint32
	EnableLLMNetBIOS        uint32
	Domain                  *uint16
	DomainCompatibilityName *uint16
	EnableUpdateTest        uint32
	UseOverwrite            uint32
	ServerRanking           uint32
	MinimumServers          uint32
}

// mibIPForwardRow — MIB_IPFORWARDROW для GetBestRoute.
type mibIPForwardRow struct {
	ForwardDest    uint32
	ForwardMask    uint32
	ForwardPolicy  uint32
	ForwardNextHop uint32
	ForwardIfIndex uint32
	ForwardType    uint32
	ForwardProto   uint32
	ForwardAge     uint32
	ForwardMetric1 uint32
	ForwardMetric2 uint32
	ForwardMetric3 uint32
	ForwardMetric4 uint32
	ForwardMetric5 uint32
}

// virtualMarkers — подстроки (в нижнем регистре) имён виртуальных/служебных
// адаптеров, которые не должны получать наш DNS: предыдущие версии
// прописывали адреса на все адаптеры и ломали Tailscale MagicDNS, WSL
// (vEthernet) и виртуальные сети гипервизоров.
var virtualMarkers = []string{
	"tailscale", "vethernet", "loopback", "bluetooth", "openvpn",
	"wireguard", "wintun", "tap-", "tun-", "zerotier", "hamachi",
	"vmware", "virtualbox", "hyper-v", "vpn", "виртуальн",
	"адаптер замыкания", "wi-fi direct", "wan miniport",
	"km-test", "npcap", "zpui",
}

func isVirtualAdapterName(name string) bool {
	n := strings.ToLower(name)
	for _, m := range virtualMarkers {
		if strings.Contains(n, m) {
			return true
		}
	}
	return false
}

// listAdapters перечисляет адаптеры через GetAdaptersAddresses (AF_INET:
// работаем только с адаптерами, имеющими IPv4-стек).
func listAdapters() []winAdapter {
	const gaaIncludeGateways = 0x0080
	for _, flags := range []uint32{gaaIncludeGateways, 0} {
		adapters, err := enumAdapters(familyAFInet, flags)
		if err == nil && len(adapters) > 0 {
			return adapters
		}
	}
	return nil
}

func enumAdapters(family, flags uint32) ([]winAdapter, error) {
	size := uint32(16 << 10)
	buf := make([]byte, size)
	for i := 0; i < 3; i++ {
		err := windows.GetAdaptersAddresses(family, flags, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.ERROR_BUFFER_OVERFLOW) {
			return nil, err
		}
		buf = make([]byte, size)
	}
	var out []winAdapter
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		ad := winAdapter{
			Name:    utf16PtrToString(a.FriendlyName),
			Index:   a.IfIndex,
			Up:      a.OperStatus == ifOperStatusUp,
			Gateway: a.FirstGatewayAddress != nil,
		}
		if a.AdapterName != nil {
			ad.GUID = "{" + strings.ToUpper(strings.Trim(windows.BytePtrToString(a.AdapterName), "{}")) + "}"
		}
		ad.Virtual = isVirtualAdapterName(ad.Name)
		ua := a.FirstUnicastAddress
		for ; ua != nil; ua = ua.Next {
			if ip := ua.Address.IP(); ip != nil && ip.To4() != nil {
				ad.HasIPv4 = true
				if ad.IPv4Addr == "" {
					ad.IPv4Addr = ip.String()
				}
			}
		}
		if ad.Name != "" && ad.GUID != "" {
			out = append(out, ad)
		}
	}
	return out, nil
}

func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + uintptr(2*n))) != 0 {
		n++
	}
	return windows.UTF16ToString(unsafe.Slice(p, n))
}

// bestRouteIfIndex — индекс интерфейса с лучшим маршрутом на 0.0.0.0.
func bestRouteIfIndex() (uint32, bool) {
	var row mibIPForwardRow
	r1, _, _ := procGetBestRoute.Call(0, 0, uintptr(unsafe.Pointer(&row)))
	if r1 != 0 || row.ForwardIfIndex == 0 {
		return 0, false
	}
	return row.ForwardIfIndex, true
}

var (
	primaryMu   sync.Mutex
	primaryName string
	primaryAt   time.Time
)

// PrimaryAdapterName — физический адаптер с лучшим маршрутом по умолчанию.
// Виртуальные адаптеры исключаются. Результат кэшируется на 15 секунд;
// при сбое пересчёта возвращается прошлый результат.
func PrimaryAdapterName() string {
	primaryMu.Lock()
	defer primaryMu.Unlock()
	if primaryName != "" && time.Since(primaryAt) < 15*time.Second {
		return primaryName
	}
	if name := resolvePrimaryAdapter(); name != "" {
		primaryName = name
		primaryAt = time.Now()
	}
	return primaryName
}

func resolvePrimaryAdapter() string {
	adapters := listAdapters()
	if len(adapters) == 0 {
		return ""
	}
	byIndex := make(map[uint32]winAdapter, len(adapters))
	for _, a := range adapters {
		byIndex[a.Index] = a
	}
	if idx, ok := bestRouteIfIndex(); ok {
		if a, ok := byIndex[idx]; ok && !a.Virtual {
			return a.Name
		}
	}
	// Fallback: первый поднятый физический адаптер со шлюзом и IPv4.
	for _, a := range adapters {
		if a.Up && !a.Virtual && a.Gateway && a.HasIPv4 {
			return a.Name
		}
	}
	for _, a := range adapters {
		if a.Up && !a.Virtual && a.HasIPv4 {
			return a.Name
		}
	}
	return ""
}

// adapterGUID — GUID адаптера по имени (нужен для dnsapi и реестра).
func adapterGUID(name string) string {
	for _, a := range listAdapters() {
		if strings.EqualFold(a.Name, name) {
			return a.GUID
		}
	}
	return ""
}

// AdapterHasStaticIPv4 — у адаптера отключён DHCP для IPv4 (EnableDHCP=0 в
// реестре Tcpip\Parameters\Interfaces). Реестр языконезависим, в отличие от
// вывода netsh; это ФАКТ конфигурации системы, а не намерение из config.json.
func AdapterHasStaticIPv4(name string) bool {
	guid := adapterGUID(name)
	if guid == "" {
		return false
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, fmt.Sprintf(tcpipKeyFmt, guid), registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("EnableDHCP")
	return err == nil && v == 0
}

// AdapterStaticDNS — статические (прописанные) DNS адаптера по имени
// (IPv4+IPv6): для сверки «наши адреса — факт на адаптере» в отчётах
// и синке DNS.
func AdapterStaticDNS(name string) []string {
	guid := adapterGUID(name)
	if guid == "" {
		return nil
	}
	return adapterStaticDNS(guid)
}

// registryDNSValues читает NameServer/DhcpNameServer адаптера из реестра.
// keyFmt: tcpipKeyFmt (IPv4) или tcpip6Fmt (IPv6).
func registryDNSValues(keyFmt, guid string) (static, dhcp []string) {
	read := func(value string) []string {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, fmt.Sprintf(keyFmt, guid), registry.QUERY_VALUE)
		if err != nil {
			return nil
		}
		defer k.Close()
		if s, _, err := k.GetStringValue(value); err == nil {
			return splitDNSList(s)
		}
		if s, _, err := k.GetStringsValue(value); err == nil {
			return s
		}
		return nil
	}
	return read("NameServer"), read("DhcpNameServer")
}

// splitDNSList — реестр хранит список адресов через пробел/запятую.
func splitDNSList(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == ';' || r == '\t' }) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// adapterStaticDNS — статические DNS адаптера (IPv4+IPv6).
func adapterStaticDNS(guid string) []string {
	v4, _ := registryDNSValues(tcpipKeyFmt, guid)
	v6, _ := registryDNSValues(tcpip6Fmt, guid)
	return append(v4, v6...)
}

// adapterDNSIPs — все DNS адаптера: статические + DHCP (для UI и снимка).
func adapterDNSIPs(guid string) []string {
	v4s, v4d := registryDNSValues(tcpipKeyFmt, guid)
	v6s, v6d := registryDNSValues(tcpip6Fmt, guid)
	out := append(append([]string{}, v4s...), v4d...)
	out = append(out, v6s...)
	out = append(out, v6d...)
	seen := map[string]bool{}
	dedup := out[:0]
	for _, ip := range out {
		if !seen[ip] {
			seen[ip] = true
			dedup = append(dedup, ip)
		}
	}
	return dedup
}

// tryDnsapiSetNameservers прописывает NameServer через dnsapi
// (SetInterfaceDnsSettingsEx, Family=AF_INET) и проверяет результат в реестре.
// Пустая строка снимает статические адреса (возврат к DHCP). Нет Ex-экспорта
// (Win10 до 1709) — false, вызывающий уходит на netsh (он семьи-раздельный).
func tryDnsapiSetNameservers(guid, joined string) bool {
	if err := procSetIfaceDnsSettsEx.Find(); err != nil {
		return false
	}
	g, err := windows.GUIDFromString(guid)
	if err != nil {
		return false
	}
	nsPtr, err := windows.UTF16PtrFromString(joined)
	if err != nil {
		return false
	}
	st := dnsInterfaceSettingsEx{Version: 2, Flags: dnsSettingNameserver, Family: familyAFInet, NameServer: nsPtr}
	r1, _, _ := procSetIfaceDnsSettsEx.Call(uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&st)))
	if r1 != 0 {
		return false
	}
	got, _ := registryDNSValues(tcpipKeyFmt, guid)
	if joined == "" {
		return len(got) == 0
	}
	want := strings.ToLower(splitDNSList(joined)[0])
	found := false
	for _, addr := range got {
		if strings.ToLower(addr) == want {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	// Страховка (нормально недостижимо с Family): наши адреса в v6-ветке —
	// снимаем v6 в DHCP тем же семья-раздельным вызовом.
	if v6Leaked(guid, joined) && clearV6ViaDnsapi(&g) {
		return !v6Leaked(guid, joined)
	}
	return found
}

// v6Leaked — в IPv6-ветке адаптера стоит ровно наш список (след старой
// SetInterfaceDnsSettings, писавшей в оба стека). Любой чужой адрес
// (свои v6-DNS пользователя) делает ветку не нашей.
func v6Leaked(guid, joined string) bool {
	ours := map[string]bool{}
	for _, a := range splitDNSList(joined) {
		ours[strings.ToLower(a)] = true
	}
	if len(ours) == 0 {
		return false
	}
	v6, _ := registryDNSValues(tcpip6Fmt, guid)
	if len(v6) == 0 {
		return false
	}
	for _, a := range v6 {
		if !ours[strings.ToLower(a)] {
			return false
		}
	}
	return true
}

// clearV6ViaDnsapi снимает статические IPv6-DNS адаптера в DHCP (Ex-вызов
// с Family=AF_INET6 и пустым NameServer). Только для самолечения утечки:
// штатные операции с v6 идут через netsh.
func clearV6ViaDnsapi(g *windows.GUID) bool {
	empty, err := windows.UTF16PtrFromString("")
	if err != nil {
		return false
	}
	st := dnsInterfaceSettingsEx{Version: 2, Flags: dnsSettingNameserver, Family: familyAFInet6, NameServer: empty}
	r1, _, _ := procSetIfaceDnsSettsEx.Call(uintptr(unsafe.Pointer(g)), uintptr(unsafe.Pointer(&st)))
	return r1 == 0
}

// setStaticDNS прописывает статические DNS на адаптер: dnsapi (Ex, только
// IPv4-стек) — основной путь с проверкой в реестре, netsh — фолбэк. v6 всегда
// через netsh.
func setStaticDNS(name, guid string, v4, v6 []string) error {
	var errs []string
	if len(v4) > 0 {
		if !tryDnsapiSetNameservers(guid, strings.Join(v4, ",")) {
			if err := runNetsh("interface", "ip", "set", "dns", "name="+name, "static", v4[0]); err != nil {
				errs = append(errs, fmt.Sprintf("v4: %v", err))
			} else {
				for i, srv := range v4[1:] {
					_ = runNetsh("interface", "ip", "add", "dns", "name="+name, srv, fmt.Sprintf("index=%d", i+2))
				}
			}
		}
	}
	if len(v6) > 0 {
		if err := runNetsh("interface", "ipv6", "set", "dns", "name="+name, "static", v6[0]); err != nil {
			errs = append(errs, fmt.Sprintf("v6: %v", err))
		} else {
			for i, srv := range v6[1:] {
				_ = runNetsh("interface", "ipv6", "add", "dns", "name="+name, srv, fmt.Sprintf("index=%d", i+2))
			}
		}
	}
	// IPv6 при применении IPv4-адресов не трогаем (решение продукта):
	// ни прописываем, ни снимаем в DHCP — что было на адаптере, то и остаётся.
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// clearStaticDNS снимает статические DNS одной семьи (возврат к DHCP).
func clearStaticDNS(name, guid, family string) error {
	if family == "4" {
		if tryDnsapiSetNameservers(guid, "") {
			return nil
		}
		return runNetsh("interface", "ip", "set", "dns", "name="+name, "source=dhcp")
	}
	return runNetsh("interface", "ipv6", "set", "dns", "name="+name, "source=dhcp")
}

// resetDNSToDHCP снимает статические DNS адаптера (обе семьи).
func resetDNSToDHCP(name, guid string) error {
	// IPv6 не трогаем (решение продукта): возвращаем к DHCP только IPv4.
	return clearStaticDNS(name, guid, "4")
}

// dnsProbe — живой ли DNS-сервер: минимальный A-запрос по UDP:53, 2 попытки,
// любой ответ с совпавшим ID считается успехом. Не зависит от системного
// резолвера (который как раз настраиваем) и от локали Windows.
func dnsProbe(server string) error {
	if net.ParseIP(server) == nil {
		return errors.New("not an IP address")
	}
	addr := net.JoinHostPort(server, "53")
	id := uint16(rand.Intn(1 << 16))
	query := buildDNSQuery(id, "xbox-dns.ru")
	var lastErr error = errors.New("no response")
	for attempt := 0; attempt < 2; attempt++ {
		conn, err := net.DialTimeout("udp", addr, 1500*time.Millisecond)
		if err != nil {
			lastErr = err
			continue
		}
		conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))
		_, err = conn.Write(query)
		if err == nil {
			buf := make([]byte, 512)
			var n int
			n, err = conn.Read(buf)
			if err == nil && n >= 2 && binary.BigEndian.Uint16(buf[:2]) == id {
				conn.Close()
				return nil
			}
		}
		conn.Close()
		lastErr = err
		if err == nil {
			lastErr = errors.New("unmatched response")
		}
	}
	return lastErr
}

// buildDNSQuery — минимальный DNS-пакет (RD, один вопрос A/IN).
func buildDNSQuery(id uint16, name string) []byte {
	head := make([]byte, 12)
	binary.BigEndian.PutUint16(head[0:], id)
	binary.BigEndian.PutUint16(head[2:], 0x0100) // recursion desired
	binary.BigEndian.PutUint16(head[4:], 1)      // QDCOUNT
	buf := append([]byte{}, head...)
	for _, label := range strings.Split(name, ".") {
		if label == "" {
			continue
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0)
	tail := make([]byte, 4)
	binary.BigEndian.PutUint16(tail[0:], 1) // QTYPE=A
	binary.BigEndian.PutUint16(tail[2:], 1) // QCLASS=IN
	return append(buf, tail...)
}
