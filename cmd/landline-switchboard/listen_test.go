package main

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
)

const dualStackName = "dual-stack.landline.test"

func answerWithLoopbacks(query []byte) []byte {
	if len(query) < 12 {
		return nil
	}
	end := 12
	for end < len(query) && query[end] != 0 {
		end += int(query[end]) + 1
	}
	end += 5
	if end > len(query) {
		return nil
	}
	question := query[12:end]
	kind := binary.BigEndian.Uint16(question[len(question)-4:])
	var data []byte
	switch kind {
	case 1:
		data = net.IPv4(127, 0, 0, 1).To4()
	case 28:
		data = net.IPv6loopback
	}
	reply := append([]byte{}, query[:2]...)
	reply = append(reply, 0x81, 0x80, 0, 1, 0, 0, 0, 0, 0, 0)
	if data != nil {
		reply[7] = 1
	}
	reply = append(reply, question...)
	if data != nil {
		reply = append(reply, 0xc0, 12)
		reply = binary.BigEndian.AppendUint16(reply, kind)
		reply = append(reply, 0, 1, 0, 0, 0, 60)
		reply = binary.BigEndian.AppendUint16(reply, uint16(len(data)))
		reply = append(reply, data...)
	}
	return reply
}

func nameServerForBothLoopbacks(t *testing.T) string {
	t.Helper()
	connection, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	go func() {
		buffer := make([]byte, 1500)
		for {
			n, from, err := connection.ReadFrom(buffer)
			if err != nil {
				return
			}
			if reply := answerWithLoopbacks(buffer[:n]); reply != nil {
				connection.WriteTo(reply, from)
			}
		}
	}()
	return connection.LocalAddr().String()
}

func TestANameThatResolvesToSeveralAddressesBindsTheFirstOneTheResolverReturnsEvenWhenThatIsIPv6(t *testing.T) {
	probe, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("this machine cannot bind ::1: %v", err)
	}
	probe.Close()
	server := nameServerForBothLoopbacks(t)
	original := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "udp", server)
	}}
	t.Cleanup(func() { net.DefaultResolver = original })

	resolved, err := net.LookupIP(dualStackName)
	if err != nil || len(resolved) != 2 {
		t.Fatalf("the test resolver answered %v %v, want ::1 and 127.0.0.1", resolved, err)
	}
	if resolved[0].To4() != nil {
		t.Skipf("the resolver put %v first, so this machine cannot tell the first address from the first IPv4 address", resolved)
	}

	listener, _, err := listenOn(net.JoinHostPort(dualStackName, "0"))
	if err != nil {
		t.Fatalf("listen on %s: %v", dualStackName, err)
	}
	defer listener.Close()
	bound := listener.Addr().(*net.TCPAddr).IP
	if !bound.Equal(resolved[0]) {
		t.Errorf("%s resolves to %v in that order, and the switchboard bound %v, not the first address", dualStackName, resolved, bound)
	}
}
