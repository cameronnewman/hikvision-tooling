package sadp

import (
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cameronnewman/hikvision-tooling/internal/logger"
)

const probeMatchResponse = `<?xml version="1.0" encoding="utf-8"?>` +
	`<ProbeMatch>` +
	`<Uuid>fake-uuid</Uuid>` +
	`<MAC>AA:BB:CC:DD:EE:FF</MAC>` +
	`<IPv4Address>192.168.1.64</IPv4Address>` +
	`<DeviceType>Camera</DeviceType>` +
	`<Activated>true</Activated>` +
	`</ProbeMatch>`

type fakeTimeoutErr struct{}

func (fakeTimeoutErr) Error() string   { return "i/o timeout" }
func (fakeTimeoutErr) Timeout() bool   { return true }
func (fakeTimeoutErr) Temporary() bool { return true }

type fakePacketConn struct {
	mu           sync.Mutex
	writes       []packetWrite
	responses    [][]byte
	responseIdx  int
	writeErr     error
	readErr      error
	readDeadline time.Time
}

type packetWrite struct {
	data []byte
	addr *net.UDPAddr
}

func (f *fakePacketConn) WriteToUDP(b []byte, addr *net.UDPAddr) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	buf := make([]byte, len(b))
	copy(buf, b)
	f.writes = append(f.writes, packetWrite{data: buf, addr: addr})
	return len(b), nil
}

func (f *fakePacketConn) ReadFromUDP(b []byte) (int, *net.UDPAddr, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readErr != nil {
		return 0, nil, f.readErr
	}
	if f.responseIdx >= len(f.responses) {
		return 0, nil, fakeTimeoutErr{}
	}
	resp := f.responses[f.responseIdx]
	f.responseIdx++
	n := copy(b, resp)
	return n, &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: Port}, nil
}

func (f *fakePacketConn) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	buf := make([]byte, len(b))
	copy(buf, b)
	f.writes = append(f.writes, packetWrite{data: buf})
	return len(b), nil
}

func (f *fakePacketConn) Read(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readErr != nil {
		return 0, f.readErr
	}
	if f.responseIdx >= len(f.responses) {
		return 0, fakeTimeoutErr{}
	}
	resp := f.responses[f.responseIdx]
	f.responseIdx++
	return copy(b, resp), nil
}

func (f *fakePacketConn) SetReadDeadline(t time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readDeadline = t
	return nil
}

func (f *fakePacketConn) SetDeadline(t time.Time) error {
	return f.SetReadDeadline(t)
}

func (f *fakePacketConn) Close() error { return nil }

func fakeInterface(name string, flags net.Flags) net.Interface {
	return net.Interface{Index: 1, MTU: 1500, Name: name, Flags: flags}
}

func fakeIPNet() *net.IPNet {
	return &net.IPNet{
		IP:   net.ParseIP("192.168.1.10").To4(),
		Mask: net.IPMask(net.ParseIP("255.255.255.0").To4()),
	}
}

func newTestScanner(t *testing.T, timeout time.Duration) *Scanner {
	t.Helper()
	return NewScanner(timeout, logger.NewNop())
}

func TestDiscover_InterfacesError(t *testing.T) {
	s := newTestScanner(t, 10*time.Millisecond)
	s.interfaces = func() ([]net.Interface, error) {
		return nil, errors.New("boom")
	}

	devices, err := s.Discover()
	if err == nil || !strings.Contains(err.Error(), "failed to get network interfaces") {
		t.Fatalf("Discover() err = %v, want interfaces error", err)
	}
	if devices != nil {
		t.Errorf("Discover() devices = %v, want nil", devices)
	}
}

func TestDiscover_SkipsDownAndLoopback(t *testing.T) {
	s := newTestScanner(t, 10*time.Millisecond)
	s.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			fakeInterface("lo0", net.FlagUp|net.FlagLoopback),
			fakeInterface("eth0", 0),
		}, nil
	}
	s.addrsOf = func(_ net.Interface) ([]net.Addr, error) {
		t.Fatal("addrsOf should not be called for down or loopback interfaces")
		return nil, nil
	}

	devices, err := s.Discover()
	if err != nil {
		t.Fatalf("Discover() err = %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("Discover() devices = %d, want 0", len(devices))
	}
}

func TestDiscover_AddrsErrorContinues(t *testing.T) {
	s := newTestScanner(t, 10*time.Millisecond)
	s.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{fakeInterface("eth0", net.FlagUp)}, nil
	}
	s.addrsOf = func(_ net.Interface) ([]net.Addr, error) {
		return nil, errors.New("no addrs")
	}
	s.listenUDP = func(string, *net.UDPAddr) (packetConn, error) {
		t.Fatal("listenUDP should not run when addrsOf errors")
		return nil, nil
	}

	devices, err := s.Discover()
	if err != nil {
		t.Fatalf("Discover() err = %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("Discover() devices = %d, want 0", len(devices))
	}
}

func TestDiscover_SkipsNonIPNetAndIPv6(t *testing.T) {
	s := newTestScanner(t, 10*time.Millisecond)
	s.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{fakeInterface("eth0", net.FlagUp)}, nil
	}
	s.addrsOf = func(_ net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPAddr{IP: net.ParseIP("192.168.1.1")},
			&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
		}, nil
	}
	s.listenUDP = func(string, *net.UDPAddr) (packetConn, error) {
		t.Fatal("listenUDP should not run when only non-IPv4 addrs")
		return nil, nil
	}

	if _, err := s.Discover(); err != nil {
		t.Fatalf("Discover() err = %v", err)
	}
}

func TestDiscover_ListenFailReturnsNoDevices(t *testing.T) {
	s := newTestScanner(t, 10*time.Millisecond)
	s.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{fakeInterface("eth0", net.FlagUp)}, nil
	}
	s.addrsOf = func(_ net.Interface) ([]net.Addr, error) {
		return []net.Addr{fakeIPNet()}, nil
	}
	s.listenUDP = func(string, *net.UDPAddr) (packetConn, error) {
		return nil, errors.New("bind failed")
	}

	devices, err := s.Discover()
	if err != nil {
		t.Fatalf("Discover() err = %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("Discover() devices = %d, want 0", len(devices))
	}
}

func TestDiscover_WriteFailStillReadsResponse(t *testing.T) {
	fake := &fakePacketConn{
		responses: [][]byte{[]byte(probeMatchResponse)},
		writeErr:  errors.New("send failed"),
	}
	s := newTestScanner(t, 10*time.Millisecond)
	s.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{fakeInterface("eth0", net.FlagUp)}, nil
	}
	s.addrsOf = func(_ net.Interface) ([]net.Addr, error) {
		return []net.Addr{fakeIPNet()}, nil
	}
	s.listenUDP = func(string, *net.UDPAddr) (packetConn, error) {
		return fake, nil
	}

	devices, err := s.Discover()
	if err != nil {
		t.Fatalf("Discover() err = %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("Discover() devices = %d, want 1", len(devices))
	}
	if devices[0].MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("MAC = %q, want AA:BB:CC:DD:EE:FF", devices[0].MAC)
	}
}

func TestDiscover_DedupByMAC(t *testing.T) {
	fake := &fakePacketConn{
		responses: [][]byte{
			[]byte(probeMatchResponse),
			[]byte(probeMatchResponse),
			[]byte(`<Nothing/>`),
		},
	}
	s := newTestScanner(t, 10*time.Millisecond)
	s.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{fakeInterface("eth0", net.FlagUp)}, nil
	}
	s.addrsOf = func(_ net.Interface) ([]net.Addr, error) {
		return []net.Addr{fakeIPNet()}, nil
	}
	s.listenUDP = func(string, *net.UDPAddr) (packetConn, error) {
		return fake, nil
	}

	devices, err := s.Discover()
	if err != nil {
		t.Fatalf("Discover() err = %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("Discover() devices = %d, want 1 (dedup)", len(devices))
	}
}

func TestSendCommandUnicast_Success(t *testing.T) {
	fake := &fakePacketConn{responses: [][]byte{[]byte(probeMatchResponse)}}
	s := newTestScanner(t, 10*time.Millisecond)
	s.dialUDP = func(_ string, _, raddr *net.UDPAddr) (packetConn, error) {
		if raddr.Port != Port {
			t.Errorf("dial port = %d, want %d", raddr.Port, Port)
		}
		return fake, nil
	}

	got, err := s.SendCommand("inquiry", SendOptions{TargetIP: "192.168.1.64", Unicast: true})
	if err != nil {
		t.Fatalf("SendCommand() err = %v", err)
	}
	if !strings.Contains(got, "ProbeMatch") {
		t.Errorf("response = %q, want ProbeMatch", got)
	}
	if len(fake.writes) != 1 {
		t.Errorf("writes = %d, want 1", len(fake.writes))
	}
}

func TestSendCommandUnicast_DialFail(t *testing.T) {
	s := newTestScanner(t, 10*time.Millisecond)
	s.dialUDP = func(string, *net.UDPAddr, *net.UDPAddr) (packetConn, error) {
		return nil, errors.New("dial failed")
	}

	_, err := s.SendCommand("inquiry", SendOptions{TargetIP: "192.168.1.64", Unicast: true})
	if err == nil || !strings.Contains(err.Error(), "failed to connect") {
		t.Fatalf("SendCommand() err = %v, want connect error", err)
	}
}

func TestSendCommandUnicast_WriteFail(t *testing.T) {
	fake := &fakePacketConn{writeErr: errors.New("write broken")}
	s := newTestScanner(t, 10*time.Millisecond)
	s.dialUDP = func(string, *net.UDPAddr, *net.UDPAddr) (packetConn, error) {
		return fake, nil
	}

	_, err := s.SendCommand("inquiry", SendOptions{TargetIP: "192.168.1.64", Unicast: true})
	if err == nil || !strings.Contains(err.Error(), "failed to send command") {
		t.Fatalf("SendCommand() err = %v, want send error", err)
	}
}

func TestSendCommandUnicast_Timeout(t *testing.T) {
	fake := &fakePacketConn{readErr: fakeTimeoutErr{}}
	s := newTestScanner(t, 10*time.Millisecond)
	s.dialUDP = func(string, *net.UDPAddr, *net.UDPAddr) (packetConn, error) {
		return fake, nil
	}

	_, err := s.SendCommand("inquiry", SendOptions{TargetIP: "192.168.1.64", Unicast: true})
	if err == nil || !strings.Contains(err.Error(), "no response") {
		t.Fatalf("SendCommand() err = %v, want timeout", err)
	}
}

func TestSendCommandUnicast_ReadFail(t *testing.T) {
	fake := &fakePacketConn{readErr: errors.New("read broken")}
	s := newTestScanner(t, 10*time.Millisecond)
	s.dialUDP = func(string, *net.UDPAddr, *net.UDPAddr) (packetConn, error) {
		return fake, nil
	}

	_, err := s.SendCommand("inquiry", SendOptions{TargetIP: "192.168.1.64", Unicast: true})
	if err == nil || !strings.Contains(err.Error(), "failed to read response") {
		t.Fatalf("SendCommand() err = %v, want read error", err)
	}
}

func TestSendCommandUnicast_UsesDefaultTimeout(t *testing.T) {
	fake := &fakePacketConn{readErr: fakeTimeoutErr{}}
	s := newTestScanner(t, 0)
	s.dialUDP = func(string, *net.UDPAddr, *net.UDPAddr) (packetConn, error) {
		return fake, nil
	}

	_, _ = s.SendCommand("inquiry", SendOptions{TargetIP: "192.168.1.64", Unicast: true})
	if fake.readDeadline.IsZero() {
		t.Fatal("expected read deadline to be set")
	}
}

func TestSendCommandBroadcast_Success(t *testing.T) {
	fake := &fakePacketConn{responses: [][]byte{[]byte(probeMatchResponse)}}
	s := newTestScanner(t, 50*time.Millisecond)
	s.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{fakeInterface("eth0", net.FlagUp)}, nil
	}
	s.addrsOf = func(_ net.Interface) ([]net.Addr, error) {
		return []net.Addr{fakeIPNet()}, nil
	}
	s.listenUDP = func(string, *net.UDPAddr) (packetConn, error) {
		return fake, nil
	}

	got, err := s.SendCommand("inquiry", SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF"})
	if err != nil {
		t.Fatalf("SendCommand() err = %v", err)
	}
	if !strings.Contains(got, "AA:BB:CC:DD:EE:FF") {
		t.Errorf("response = %q, want target MAC", got)
	}
}

func TestSendCommandBroadcast_UsesDefaultTimeoutAndSkipsNonMatching(t *testing.T) {
	fake := &fakePacketConn{
		responses: [][]byte{
			[]byte(`<ProbeMatch><MAC>99:99:99:99:99:99</MAC></ProbeMatch>`),
			[]byte(probeMatchResponse),
		},
	}
	s := newTestScanner(t, 0)
	s.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{fakeInterface("eth0", net.FlagUp)}, nil
	}
	s.addrsOf = func(_ net.Interface) ([]net.Addr, error) {
		return []net.Addr{fakeIPNet()}, nil
	}
	s.listenUDP = func(string, *net.UDPAddr) (packetConn, error) {
		return fake, nil
	}

	got, err := s.SendCommand("inquiry", SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF"})
	if err != nil {
		t.Fatalf("SendCommand() err = %v", err)
	}
	if !strings.Contains(got, "AA:BB:CC:DD:EE:FF") {
		t.Errorf("response = %q, want target MAC", got)
	}
}

func TestSendCommandBroadcast_InterfacesError(t *testing.T) {
	s := newTestScanner(t, 10*time.Millisecond)
	s.interfaces = func() ([]net.Interface, error) {
		return nil, errors.New("no interfaces")
	}

	_, err := s.SendCommand("inquiry", SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF"})
	if err == nil || !strings.Contains(err.Error(), "failed to get network interfaces") {
		t.Fatalf("SendCommand() err = %v", err)
	}
}

func TestSendCommandBroadcast_SkipsDownLoopbackAndBadAddrs(t *testing.T) {
	fake := &fakePacketConn{}
	s := newTestScanner(t, 10*time.Millisecond)
	s.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			fakeInterface("lo0", net.FlagUp|net.FlagLoopback),
			fakeInterface("eth0", 0),
			fakeInterface("eth1", net.FlagUp),
			fakeInterface("eth2", net.FlagUp),
		}, nil
	}
	s.addrsOf = func(iface net.Interface) ([]net.Addr, error) {
		switch iface.Name {
		case "eth1":
			return nil, errors.New("addrs failed")
		case "eth2":
			return []net.Addr{
				&net.IPAddr{IP: net.ParseIP("10.0.0.1")},
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
			}, nil
		}
		t.Fatalf("unexpected addrsOf call for %s", iface.Name)
		return nil, nil
	}
	s.listenUDP = func(string, *net.UDPAddr) (packetConn, error) {
		return fake, nil
	}

	_, err := s.SendCommand("inquiry", SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF"})
	if err == nil || !strings.Contains(err.Error(), "no response") {
		t.Fatalf("SendCommand() err = %v, want no response", err)
	}
}

func TestSendCommandBroadcast_ListenFailStillErrors(t *testing.T) {
	s := newTestScanner(t, 10*time.Millisecond)
	s.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{fakeInterface("eth0", net.FlagUp)}, nil
	}
	s.addrsOf = func(_ net.Interface) ([]net.Addr, error) {
		return []net.Addr{fakeIPNet()}, nil
	}
	s.listenUDP = func(string, *net.UDPAddr) (packetConn, error) {
		return nil, errors.New("bind failed")
	}

	_, err := s.SendCommand("inquiry", SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF"})
	if err == nil {
		t.Fatal("SendCommand() succeeded, want error")
	}
}

func TestSendCommandBroadcast_TimeoutMessages(t *testing.T) {
	tests := []struct {
		name   string
		opts   SendOptions
		wantIn string
	}{
		{
			name:   "MAC and IP",
			opts:   SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", TargetIP: "192.168.1.64"},
			wantIn: "192.168.1.64 / AA:BB:CC:DD:EE:FF",
		},
		{
			name:   "MAC only",
			opts:   SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF"},
			wantIn: "MAC AA:BB:CC:DD:EE:FF",
		},
		{
			name:   "IP only",
			opts:   SendOptions{TargetIP: "192.168.1.64"},
			wantIn: "at 192.168.1.64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScanner(t, 10*time.Millisecond)
			s.interfaces = func() ([]net.Interface, error) { return nil, nil }
			_, err := s.SendCommand("inquiry", tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Fatalf("SendCommand() err = %v, want containing %q", err, tt.wantIn)
			}
		})
	}
}

func TestSendCommand_BuildXMLError(t *testing.T) {
	s := newTestScanner(t, 10*time.Millisecond)
	_, err := s.SendCommand("nonexistent", SendOptions{TargetIP: "192.168.1.64"})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("SendCommand() err = %v, want unknown command", err)
	}
}

func TestBuildCommandXML_UnimplementedFallback(t *testing.T) {
	const mockName = "test_unimplemented"
	Commands[mockName] = Command{Name: mockName, Description: "test", Template: "<x/>"}
	defer delete(Commands, mockName)

	s := newTestScanner(t, 10*time.Millisecond)
	_, err := s.BuildCommandXML(mockName, SendOptions{})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("BuildCommandXML() err = %v, want not implemented", err)
	}
}

func TestBuildCommandXML_ResetPasswordMissingFields(t *testing.T) {
	s := newTestScanner(t, 10*time.Millisecond)

	tests := []struct {
		name    string
		opts    SendOptions
		wantErr string
	}{
		{"missing MAC", SendOptions{Code: "X", Password: "Y"}, "MAC address required"},
		{"missing password", SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Code: "X"}, "new password required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.BuildCommandXML("resetpassword", tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("BuildCommandXML() err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestSendCommandBroadcast_TargetIPZeroTreatedAsUnset(t *testing.T) {
	s := newTestScanner(t, 5*time.Millisecond)
	s.interfaces = func() ([]net.Interface, error) { return nil, nil }

	_, err := s.SendCommand("inquiry", SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", TargetIP: "0.0.0.0"})
	if err == nil || !strings.Contains(err.Error(), "MAC AA:BB:CC:DD:EE:FF") {
		t.Fatalf("SendCommand() err = %v, want MAC-only error message (IP 0.0.0.0 dropped)", err)
	}
}

func TestNewScannerDefaultsExerciseRealNet(t *testing.T) {
	s := NewScanner(10*time.Millisecond, nil)

	ifaces, err := s.interfaces()
	if err != nil {
		t.Fatalf("interfaces() err = %v", err)
	}
	if len(ifaces) > 0 {
		if _, err := s.addrsOf(ifaces[0]); err != nil {
			t.Logf("addrsOf err = %v (non-fatal on some hosts)", err)
		}
	}

	conn, err := s.listenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listenUDP loopback err = %v", err)
	}
	udpConn := conn.(*net.UDPConn)
	localAddr := udpConn.LocalAddr().(*net.UDPAddr)
	_ = conn.Close()

	if _, err := s.listenUDP("bad-network", &net.UDPAddr{}); err == nil {
		t.Fatal("listenUDP with bad network succeeded, want error")
	}

	dialConn, err := s.dialUDP("udp4", nil, localAddr)
	if err != nil {
		t.Fatalf("dialUDP loopback err = %v", err)
	}
	_ = dialConn.Close()

	if _, err := s.dialUDP("bad-network", nil, localAddr); err == nil {
		t.Fatal("dialUDP with bad network succeeded, want error")
	}
}

func TestToXML_MarshalError(t *testing.T) {
	orig := xmlMarshalIndent
	t.Cleanup(func() { xmlMarshalIndent = orig })
	xmlMarshalIndent = func(interface{}, string, string) ([]byte, error) {
		return nil, errors.New("marshal boom")
	}

	s := newTestScanner(t, 10*time.Millisecond)
	_, err := s.ToXML([]*Device{{MAC: "AA:BB:CC:DD:EE:FF"}})
	if err == nil || !strings.Contains(err.Error(), "marshal boom") {
		t.Fatalf("ToXML() err = %v, want marshal boom", err)
	}
}
