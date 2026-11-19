package hubclient

import (
	"net"
	"net/url"
	"strings"
)

func ThisMachines(address string) bool {
	parsed, err := url.Parse(address)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	var resolved []net.IP
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return true
		}
		resolved = []net.IP{ip}
	} else {
		found, err := net.LookupIP(host)
		if err != nil {
			return false
		}
		resolved = found
	}
	interfaces, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, each := range interfaces {
		owned, ok := each.(*net.IPNet)
		if !ok {
			continue
		}
		for _, ip := range resolved {
			if owned.IP.Equal(ip) {
				return true
			}
		}
	}
	return false
}
