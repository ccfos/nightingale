package sys

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func IntranetIP() (ips []string, err error) {
	ips = make([]string, 0)

	ifaces, e := net.Interfaces()
	if e != nil {
		return ips, e
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}

		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}

		// ignore docker and warden bridge
		if strings.HasPrefix(iface.Name, "docker") || strings.HasPrefix(iface.Name, "w-") {
			continue
		}

		addrs, e := iface.Addrs()
		if e != nil {
			return ips, e
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}

			ipStr := ip.String()
			if IsIntranet(ipStr) {
				ips = append(ips, ipStr)
			}
		}
	}

	return ips, nil
}

func IsIntranet(ipStr string) bool {
	if strings.HasPrefix(ipStr, "10.") {
		return true
	}

	// for didi
	if strings.HasPrefix(ipStr, "100.") {
		return true
	}

	if strings.HasPrefix(ipStr, "192.168.") {
		return true
	}

	if strings.HasPrefix(ipStr, "172.") {
		// 172.16.0.0-172.31.255.255
		arr := strings.Split(ipStr, ".")
		if len(arr) != 4 {
			return false
		}

		second, err := strconv.ParseInt(arr[1], 10, 64)
		if err != nil {
			return false
		}

		if second >= 16 && second <= 31 {
			return true
		}
	}

	return false
}

// ${sn}-${hostname}-${ip}
func LocalHostIdent() string {
	sn, _ := CmdOutTrim("/bin/bash", "-c", "dmidecode -s system-serial-number")
	if sn != "" {
		arr := strings.Fields(sn)
		sn = arr[len(arr)-1]
	} else {
		sn = "nil"
	}

	name, _ := CmdOutTrim("hostname")

	ips, _ := IntranetIP()
	ip := ""
	if ips != nil && len(ips) > 0 {
		ip = ips[0]
	}

	return fmt.Sprintf("%s-%s-%s", sn, name, ip)
}

func GetOutboundIpaddr() string {
	conn, err := net.Dial("udp4", "1.2.3.4:56")
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().String()

	if ip, _, err := net.SplitHostPort(localAddr); err != nil {
		return ""
	} else {
		return ip
	}
}
