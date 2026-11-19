package app

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"zpui/internal/config"
	"zpui/internal/executil"
	"zpui/internal/power"
	"zpui/internal/xboxdns"
)

// SOCKS5-прокси для домашней сети: включается мастером-тумблером
// (proxy.enabled) и требует активного статического IP — телефон ходит через
// ПК по постоянному адресу. Статика включается только вручную; сторож
// подсети лишь снимает НАШУ статику в DHCP при уходе из сети, чтобы не
// остаться без сети со старым адресом. В режиме максимальной экономии
// (уровень 3) прокси отключается, при выходе из режима — поднимается снова.

// startSocksProxy — фоновый синхронизатор состояния прокси с тумблером и
// экономным режимом. Тик 10 секунд (экономный режим растягивает) с пустым
// сравнением состояний — дешевле, чем события; прокси не дёргается лишний
// раз (Start/Stop идемпотентны).
func (a *App) startSocksProxy() {
	time.Sleep(5 * time.Second)
	a.syncSocksProxy()
	for {
		select {
		case <-a.stopCh:
			return
		case <-time.After(power.Stretch(10 * time.Second)):
		}
		a.syncSocksProxy()
	}
}

func (a *App) syncSocksProxy() {
	if a.socks == nil {
		return
	}
	if power.ServicesPaused() || !a.cfg.GetProxyEnabled() {
		if a.socks.IsRunning() {
			a.socks.Stop()
		}
		return
	}
	if !a.socks.IsRunning() {
		if err := a.socks.Start(); err != nil {
			a.log.Warn("proxy", "start: "+err.Error())
		}
	}
}

// GetSocksStatus — статус SOCKS5-прокси для вкладки «Прокси»: состояние,
// порт, адрес подключения с телефона (IP ПК из ЛВС), факт статического IP,
// подсеть и запомненная статика.
func (a *App) GetSocksStatus() map[string]interface{} {
	// static_ip — ФАКТ конфигурации адаптера (EnableDHCP из реестра), а не
	// флаг из config.json: система — источник истины, как с автозапуском.
	// Кнопка в UI обязана показывать, что реально настроено в Windows сейчас.
	staticIP := false
	subnet := ""
	if name := xboxdns.PrimaryAdapterName(); name != "" {
		staticIP = xboxdns.AdapterHasStaticIPv4(name)
		ip, mask, _, _ := currentIPv4Config(name)
		subnet = subnetKey(ip, mask)
	}
	st := map[string]interface{}{
		// Порт и LAN-IP — из системы/конфига: карточка должна показывать их
		// и при остановленном прокси, иначе поля выглядят пустыми, как при поломке.
		"running":        a.socks != nil && a.socks.IsRunning(),
		"port":           a.cfg.GetSocksProxyPort(),
		"lan_ip":         lanIPv4(),
		"static_ip":      staticIP,
		"enabled":        a.cfg.GetProxyEnabled(),
		"current_subnet": subnet,
		"static_subnets": a.cfg.GetStaticSubnets(),
		"power_paused":   power.ServicesPaused(),
	}
	if a.socks != nil {
		s := a.socks.GetStats()
		st["active"] = s["active_connections"]
		st["bytes_sent"] = s["total_bytes_sent"]
		st["bytes_recv"] = s["total_bytes_recv"]
	}
	return st
}

// SetSocksPort — порт прокси; живой сервер перезапускается на новом порту.
func (a *App) SetSocksPort(port int) map[string]interface{} {
	if port < 1 || port > 65535 {
		return errResp("порт должен быть 1..65535")
	}
	if port == a.cfg.GetSocksProxyPort() {
		return okResp()
	}
	if err := a.cfg.SetSocksProxyPort(port); err != nil {
		return errResp(err.Error())
	}
	if a.socks.IsRunning() {
		a.socks.Stop()
		if !power.ServicesPaused() && a.cfg.GetProxyEnabled() {
			if err := a.socks.Start(); err != nil {
				return errResp(err.Error())
			}
		}
	}
	return okResp()
}

// SetSocksEnabled — мастер-тумблер SOCKS (POST /api/socks/enabled {on}).
// Статика — рекомендация, а не условие: без неё прокси работает, просто
// адрес для телефона может смениться вместе с сетью. Отсутствие статики
// для текущей подсети вернёт warning=static_required (фронт предупреждает).
func (a *App) SetSocksEnabled(on bool) map[string]interface{} {
	warn := on && a.requireCurrentStatic() != nil
	if err := a.cfg.SetProxyEnabled(on); err != nil {
		return errResp(err.Error())
	}
	a.log.Info("proxy", "socks enabled: "+map[bool]string{true: "on", false: "off"}[on])
	a.syncSocksProxy()
	if warn {
		m := okResp()
		m["warning"] = "static_required"
		return m
	}
	return okResp()
}

// requireCurrentStatic — ошибка "static_required", если текущая подсеть не
// имеет запомненной статики или адаптер сейчас на DHCP.
func (a *App) requireCurrentStatic() error {
	name := xboxdns.PrimaryAdapterName()
	if name == "" {
		return fmt.Errorf("static_required")
	}
	ip, mask, _, _ := currentIPv4Config(name)
	subnet := subnetKey(ip, mask)
	if subnet == "" {
		return fmt.Errorf("static_required")
	}
	if _, ok := a.cfg.GetStaticSubnets()[subnet]; !ok {
		return fmt.Errorf("static_required")
	}
	if !xboxdns.AdapterHasStaticIPv4(name) {
		return fmt.Errorf("static_required")
	}
	return nil
}

// SetProxyStaticIP — зафиксировать текущий IPv4 ПК как статический (чтобы
// телефон всегда находил прокси по одному адресу) или вернуть DHCP.
// Включение запоминает статику в static_subnets[текущая подсеть]; выключение
// снимает её с адаптера, но запись в карте сохраняется (сеть знакома).
// Сравнение и отображение — по ФАКТУ адаптера: если статику выставили мимо
// приложения (вручную/другой программой), кнопка видит реальное состояние.
func (a *App) SetProxyStaticIP(on bool) map[string]interface{} {
	name := xboxdns.PrimaryAdapterName()
	if name == "" {
		return errResp("активный сетевой адаптер не найден")
	}
	if on == xboxdns.AdapterHasStaticIPv4(name) {
		return okResp()
	}
	if on {
		ip, mask, gw, err := currentIPv4Config(name)
		if err != nil {
			return errResp(err.Error())
		}
		subnet := subnetKey(ip, mask)
		if subnet == "" {
			return errResp("не удалось определить подсеть")
		}
		if err := netshSetStaticAddr(name, ip, mask, gw); err != nil {
			return errResp(err.Error())
		}
		if err := a.cfg.SetStaticSubnet(subnet, config.StaticIP{IP: ip, Mask: mask, GW: gw}); err != nil {
			a.log.Warn("proxy", "remember static: "+err.Error())
		}
		a.markAppliedStatic(subnet)
	} else {
		if err := netshSetDHCP(name); err != nil {
			return errResp(err.Error())
		}
		a.markAppliedStatic("")
		// netsh set address dhcp сбрасывает и DNS адаптера — вернуть
		// применённые ZPUI DNS, если функция включена.
		if a.cfg.GetXboxDnsConfig().Enabled {
			if err := a.applyDnsEnabled(true); err != nil {
				a.log.Warn("proxy", "re-apply DNS after static off: "+err.Error())
				a.ensureDnsStateHonest()
			}
		}
	}
	a.log.Info("proxy", "static ip: "+map[bool]string{true: "on", false: "off"}[on]+" ("+name+")")
	return okResp()
}

// ---------- сторож подсети ----------

// startSocksSubnetWatch — сторож подсети (тик 30с, экономный режим растягивает
// интервал и пропускает паузы): смена сети → снять НАШУ статику в DHCP.
// Статика включается только вручную — сам сторож её никогда не применяет.
func (a *App) startSocksSubnetWatch() {
	time.Sleep(15 * time.Second)
	a.detectAppliedStatic()
	seen := ""
	for {
		wait := power.Stretch(30 * time.Second)
		select {
		case <-a.stopCh:
			return
		case <-time.After(wait):
		}
		if power.ServicesPaused() {
			continue
		}
		seen = a.subnetWatchTick(seen)
	}
}

// detectAppliedStatic — на старте: если адаптер в статике и её адрес/подсеть
// совпадают с запомненной — считаем её применённой (при уходе из сети
// снимем в DHCP). Чужую статику не трогаем.
func (a *App) detectAppliedStatic() {
	name := xboxdns.PrimaryAdapterName()
	if name == "" || !xboxdns.AdapterHasStaticIPv4(name) {
		return
	}
	ip, mask, _, _ := currentIPv4Config(name)
	subnet := subnetKey(ip, mask)
	if s, ok := a.cfg.GetStaticSubnets()[subnet]; ok && s.IP == ip {
		a.markAppliedStatic(subnet)
	}
}

// subnetWatchTick — один проход сторожа; возвращает новую seen-подсеть.
func (a *App) subnetWatchTick(seen string) string {
	name := xboxdns.PrimaryAdapterName()
	if name == "" {
		return seen
	}
	ip, mask, _, _ := currentIPv4Config(name)
	subnet := subnetKey(ip, mask)
	if subnet == "" {
		return seen
	}
	if subnet == seen {
		return seen
	}

	static := xboxdns.AdapterHasStaticIPv4(name)
	// Смена сети: наша статика (применённая нами на прошлой подсети) больше
	// не нужна — возвращаем адаптер к DHCP, запись в карте сохраняется.
	if applied := a.appliedStatic(); applied != "" && applied != subnet && static {
		if err := netshSetDHCP(name); err != nil {
			a.log.Warn("proxy", "subnet change, dhcp restore: "+err.Error())
		} else {
			a.log.Info("proxy", fmt.Sprintf("subnet changed %s → %s: static released to DHCP", applied, subnet))
		}
		a.markAppliedStatic("")
		if a.cfg.GetXboxDnsConfig().Enabled {
			if err := a.applyDnsEnabled(true); err != nil {
				a.log.Warn("proxy", "re-apply DNS after subnet change: "+err.Error())
				a.ensureDnsStateHonest()
			}
		}
	}

	return subnet
}

// markAppliedStatic — подсеть, на которой статика применена нами ("" — снята).
func (a *App) markAppliedStatic(subnet string) {
	a.appliedStaticMu.Lock()
	a.appliedStaticSubnet = subnet
	a.appliedStaticMu.Unlock()
}

// appliedStatic — текущее значение (для будущих проверок).
func (a *App) appliedStatic() string {
	a.appliedStaticMu.Lock()
	defer a.appliedStaticMu.Unlock()
	return a.appliedStaticSubnet
}

// ---------- сетевые операции ----------

// currentIPv4Config — текущие IPv4, маска и шлюз первичного (заданного)
// адаптера.
func currentIPv4Config(name string) (ip, mask, gw string, err error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", "", "", fmt.Errorf("адаптер %q не найден: %w", name, err)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", "", "", err
	}
	for _, addr := range addrs {
		if ipn, ok := addr.(*net.IPNet); ok && ipn.IP.To4() != nil {
			return ipn.IP.String(), net.IP(ipn.Mask).String(), defaultGatewayIPv4(), nil
		}
	}
	return "", "", "", fmt.Errorf("у адаптера %q нет IPv4-адреса", name)
}

// subnetKey — ключ подсети "a.b.c.d/len" из IPv4 и точечной маски.
func subnetKey(ipStr, maskStr string) string {
	ip := net.ParseIP(ipStr)
	maskIP := net.ParseIP(maskStr)
	if ip == nil || maskIP == nil || ip.To4() == nil || maskIP.To4() == nil {
		return ""
	}
	m := net.IPMask(maskIP.To4())
	bits, _ := m.Size()
	if bits <= 0 || bits > 32 {
		return ""
	}
	network := ip.To4().Mask(m)
	if network == nil {
		return ""
	}
	return network.String() + "/" + strconv.Itoa(bits)
}

// netshSetStaticAddr — прописывает статический IPv4/маску/шлюз адаптеру.
func netshSetStaticAddr(name, ip, mask, gw string) error {
	if out, err := executil.HiddenCmd("netsh", "interface", "ip", "set", "address",
		"name="+name, "static", ip, mask, gw).CombinedOutput(); err != nil {
		return fmt.Errorf("netsh address: %s", strings.TrimSpace(string(out)))
	}
	// DNS: фиксируем те, что уже прописаны на адаптере (в т.ч. от Xbox DNS).
	for _, ad := range xboxdns.SystemDNS() {
		if !strings.EqualFold(ad.Name, name) || len(ad.DNS) == 0 {
			continue
		}
		if out, err := executil.HiddenCmd("netsh", "interface", "ip", "set", "dnsservers",
			"name="+name, "static", ad.DNS[0], "register=primary").CombinedOutput(); err != nil {
			return fmt.Errorf("netsh dns: %s", strings.TrimSpace(string(out)))
		}
		for i, dns := range ad.DNS[1:] {
			executil.HiddenCmd("netsh", "interface", "ip", "add", "dnsservers",
				"name="+name, dns, fmt.Sprintf("index=%d", i+2)).Run()
		}
		break
	}
	return nil
}

// netshSetDHCP — возвращает адаптер к автоматической конфигурации.
func netshSetDHCP(name string) error {
	if out, err := executil.HiddenCmd("netsh", "interface", "ip", "set", "address",
		"name="+name, "dhcp").CombinedOutput(); err != nil {
		return fmt.Errorf("netsh address dhcp: %s", strings.TrimSpace(string(out)))
	}
	executil.HiddenCmd("netsh", "interface", "ip", "set", "dnsservers",
		"name="+name, "dhcp").Run()
	return nil
}

// defaultGatewayIPv4 — шлюз по умолчанию из таблицы маршрутов (вывод
// `route print` цифровой, языконезависимый).
func defaultGatewayIPv4() string {
	out, err := executil.HiddenCmd("route", "print", "0.0.0.0").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 5 && f[0] == "0.0.0.0" && f[1] == "0.0.0.0" && net.ParseIP(f[2]) != nil {
			return f[2]
		}
	}
	return ""
}

// lanIPv4 — IPv4 активного физического адаптера (адрес для подключения
// с телефона); пустая строка, если определить не удалось.
func lanIPv4() string {
	name := xboxdns.PrimaryAdapterName()
	if name == "" {
		return ""
	}
	ip, _, _, err := currentIPv4Config(name)
	if err != nil {
		return ""
	}
	return ip
}
