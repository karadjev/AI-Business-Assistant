package network

import (
	"net"
	"testing"
)

func TestNonPublicDestinationsDenied(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "198.18.1.1", "203.0.113.4", "240.0.0.1", "::1", "fc00::1", "fe80::1", "2001:db8::1", "64:ff9b::7f00:1", "2002:7f00:1::"} {
		if publicIP(net.ParseIP(raw)) {
			t.Errorf("unsafe destination: %s", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !publicIP(net.ParseIP(raw)) {
			t.Error("public address rejected")
		}
	}
}
