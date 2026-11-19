package app

import "net"

// routeInfo — IP и MAC интерфейса с маршрутом по умолчанию.
func routeInfo() (ip, mac string) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", ""
	}
	defer conn.Close()
	local, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || local.IP.To4() == nil {
		return "", ""
	}
	ip = local.IP.String()
	ifaces, err := net.Interfaces()
	if err != nil {
		return ip, ""
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.Equal(local.IP) {
				if m := ifc.HardwareAddr.String(); m != "" {
					return ip, m
				}
			}
		}
	}
	return ip, ""
}
