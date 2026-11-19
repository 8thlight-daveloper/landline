package hubclient

import (
	"net"
	"testing"
)

func TestAnAddressIsThisMachinesWhenItIsLocalhostALoopbackIPOrAnInterfaceAddress(t *testing.T) {
	for address, want := range map[string]bool{
		"http://localhost:8787": true, "http://LOCALHOST": true, "http://127.0.0.1:1": true, "http://127.9.9.9": true, "http://[::1]:8787": true,
		"http://192.0.2.1:8787": false, "http://tailnet-mac.invalid:8787": false, "http://2001:db8::10": false, "": false, "://bad": false,
	} {
		if got := ThisMachines(address); got != want {
			t.Errorf("ThisMachines(%q) = %v, want %v", address, got, want)
		}
	}
	addrs, _ := net.InterfaceAddrs()
	for _, each := range addrs {
		if ip, ok := each.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() {
			if !ThisMachines("http://" + net.JoinHostPort(ip.IP.String(), "8787")) {
				t.Errorf("%s is an address of an interface", ip.IP)
			}
			return
		}
	}
}
