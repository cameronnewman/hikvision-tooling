package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cameronnewman/hikvision-tooling/internal/config"
	"github.com/cameronnewman/hikvision-tooling/internal/logger"
	"github.com/cameronnewman/hikvision-tooling/internal/network"
	"github.com/cameronnewman/hikvision-tooling/internal/sadp"
)

type fakeScanner struct {
	discoverDevices []*sadp.Device
	discoverErr     error
	sendResponse    string
	sendErr         error
	xmlOutput       string
	xmlErr          error
	csvOutput       string

	lastSendCmd  string
	lastSendOpts sadp.SendOptions
}

func (f *fakeScanner) Discover() ([]*sadp.Device, error) {
	return f.discoverDevices, f.discoverErr
}

func (f *fakeScanner) SendCommand(cmdName string, opts sadp.SendOptions) (string, error) {
	f.lastSendCmd = cmdName
	f.lastSendOpts = opts
	return f.sendResponse, f.sendErr
}

func (f *fakeScanner) ToXML(_ []*sadp.Device) (string, error) {
	return f.xmlOutput, f.xmlErr
}

func (f *fakeScanner) ToCSV(_ []*sadp.Device) string { return f.csvOutput }

type fakeHTTP struct {
	responses map[string]*network.HTTPResponse
	errs      map[string]error
	lastPath  string
}

func (f *fakeHTTP) Get(_, path string) (*network.HTTPResponse, error) {
	f.lastPath = path
	if err, ok := f.errs[path]; ok {
		return nil, err
	}
	if resp, ok := f.responses[path]; ok {
		return resp, nil
	}
	return &network.HTTPResponse{StatusCode: 404, Body: []byte{}}, nil
}

func withStdout(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	orig := stdout
	stdout = buf
	t.Cleanup(func() { stdout = orig })
	return buf
}

func withStderr(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	orig := stderr
	stderr = buf
	t.Cleanup(func() { stderr = orig })
	return buf
}

func withScanner(t *testing.T, sc *fakeScanner) {
	t.Helper()
	orig := newSADPScanner
	newSADPScanner = func(time.Duration, *logger.Logger) sadpScanner { return sc }
	t.Cleanup(func() { newSADPScanner = orig })
}

func withHTTP(t *testing.T, h *fakeHTTP) {
	t.Helper()
	orig := newHTTPClient
	newHTTPClient = func(string, time.Duration) httpGetter { return h }
	t.Cleanup(func() { newHTTPClient = orig })
}

func withARP(t *testing.T, table network.ARPTable, err error) {
	t.Helper()
	orig := getARPTable
	getARPTable = func() (network.ARPTable, error) { return table, err }
	t.Cleanup(func() { getARPTable = orig })
}

func withHostAlive(t *testing.T, fn func(string, time.Duration) bool) {
	t.Helper()
	orig := isHostAlive
	isHostAlive = fn
	t.Cleanup(func() { isHostAlive = orig })
}

func TestRunDispatchesEachCommand(t *testing.T) {
	withStdout(t)
	withStderr(t)
	withScanner(t, &fakeScanner{sendResponse: "ok", discoverDevices: nil})
	withHTTP(t, &fakeHTTP{})
	withARP(t, map[string]string{}, nil)
	withHostAlive(t, func(string, time.Duration) bool { return false })

	tests := []struct {
		name string
		args []string
	}{
		{"discover no args", []string{"discover"}},
		{"discover:sadp", []string{"discover:sadp"}},
		{"scan no args", []string{"scan"}},
		{"probe no args", []string{"probe"}},
		{"send no args", []string{"send"}},
		{"reset no args", []string{"reset"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Run(tt.args); err != nil {
				t.Errorf("Run(%v) err = %v", tt.args, err)
			}
		})
	}
}

func TestDiscoverCmd_InvalidCIDR(t *testing.T) {
	withStdout(t)
	err := DiscoverCmd([]string{"not-a-cidr"})
	if err == nil || !strings.Contains(err.Error(), "invalid CIDR") {
		t.Fatalf("DiscoverCmd() err = %v, want invalid CIDR", err)
	}
}

func TestDiscoverCmd_Success(t *testing.T) {
	buf := withStdout(t)
	withARP(t, map[string]string{
		"192.168.1.1": "4C:BD:8F:11:22:33",
		"192.168.1.2": "00:11:22:33:44:55",
	}, nil)
	withHostAlive(t, func(ip string, _ time.Duration) bool {
		return ip == "192.168.1.1" || ip == "192.168.1.2"
	})

	if err := DiscoverCmd([]string{"192.168.1.0/30"}); err != nil {
		t.Fatalf("DiscoverCmd() err = %v", err)
	}
	if !strings.Contains(buf.String(), "4C:BD:8F:11:22:33") {
		t.Errorf("output missing Hikvision MAC: %s", buf.String())
	}
	if strings.Contains(buf.String(), "00:11:22:33:44:55") {
		t.Errorf("output contains non-Hikvision MAC: %s", buf.String())
	}
}

func TestDiscoverDevices_ARPError(t *testing.T) {
	withStdout(t)
	withARP(t, nil, errors.New("arp broken"))
	withHostAlive(t, func(string, time.Duration) bool { return true })

	log := logger.NewNop()
	devs := discoverDevices([]string{"192.168.1.1"}, 1, time.Millisecond, log)
	if devs != nil {
		t.Errorf("devices = %v, want nil", devs)
	}
}

func TestDiscoverSADPCmd_TableAndFileWrite(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "out.xml")

	withStdout(t)
	withScanner(t, &fakeScanner{
		discoverDevices: []*sadp.Device{
			{MAC: "AA:BB:CC:DD:EE:FF", IPv4Address: "192.168.1.64", Activated: "true", DeviceType: "Camera"},
			{MAC: "11:22:33:44:55:66", IPv4Address: "192.168.1.65", Activated: "false", DeviceType: "NVR"},
		},
		xmlOutput: "<xml/>",
	})

	if err := DiscoverSADPCmd([]string{"--output", outPath}); err != nil {
		t.Fatalf("DiscoverSADPCmd() err = %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "<xml/>" {
		t.Errorf("output file = %q, want <xml/>", string(data))
	}
}

func TestDiscoverSADPCmd_XMLOutputStdout(t *testing.T) {
	buf := withStdout(t)
	withScanner(t, &fakeScanner{xmlOutput: "<x/>"})

	if err := DiscoverSADPCmd([]string{"--xml"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(buf.String(), "<x/>") {
		t.Errorf("stdout = %q, want <x/>", buf.String())
	}
}

func TestDiscoverSADPCmd_CSVOutputStdout(t *testing.T) {
	buf := withStdout(t)
	withScanner(t, &fakeScanner{csvOutput: "id,mac\n1,AA:BB"})

	if err := DiscoverSADPCmd([]string{"--csv"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(buf.String(), "id,mac") {
		t.Errorf("stdout = %q, want csv", buf.String())
	}
}

func TestDiscoverSADPCmd_DiscoverError(t *testing.T) {
	withStdout(t)
	withScanner(t, &fakeScanner{discoverErr: errors.New("no interfaces")})

	if err := DiscoverSADPCmd(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestDiscoverSADPCmd_XMLError(t *testing.T) {
	withStdout(t)
	withScanner(t, &fakeScanner{xmlErr: errors.New("marshal broken")})

	err := DiscoverSADPCmd([]string{"--xml"})
	if err == nil || !strings.Contains(err.Error(), "error generating XML") {
		t.Fatalf("err = %v", err)
	}
}

func TestDiscoverSADPCmd_WriteFileError(t *testing.T) {
	withStdout(t)
	withScanner(t, &fakeScanner{xmlOutput: "<x/>"})

	badPath := filepath.Join(t.TempDir(), "does-not-exist", "out.xml")
	err := DiscoverSADPCmd([]string{"--output", badPath})
	if err == nil || !strings.Contains(err.Error(), "error writing file") {
		t.Fatalf("err = %v", err)
	}
}

func TestDiscoverSADPCmd_DefaultOutputFileUsesXML(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "default.xml")

	withStdout(t)
	withScanner(t, &fakeScanner{
		discoverDevices: []*sadp.Device{{MAC: "AA:BB:CC:DD:EE:FF"}},
		xmlOutput:       "<xml/>",
	})

	if err := DiscoverSADPCmd([]string{"--output", outPath}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("output not written: %v", err)
	}
}

func TestPrintDeviceTable_Empty(t *testing.T) {
	buf := withStdout(t)
	printDeviceTable(nil)
	if !strings.Contains(buf.String(), "No devices found") {
		t.Errorf("output = %q, want 'No devices found'", buf.String())
	}
}

func TestPrintCommandList(t *testing.T) {
	buf := withStdout(t)
	printCommandList()
	if !strings.Contains(buf.String(), "inquiry") {
		t.Errorf("output = %q, want 'inquiry'", buf.String())
	}
}

func TestSendCmd_List(t *testing.T) {
	buf := withStdout(t)
	if err := SendCmd([]string{"--list"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(buf.String(), "inquiry") {
		t.Errorf("output missing commands: %s", buf.String())
	}
}

func TestSendCmd_UnicastSuccess(t *testing.T) {
	buf := withStdout(t)
	sc := &fakeScanner{sendResponse: "<ok/>"}
	withScanner(t, sc)

	err := SendCmd([]string{"192.168.1.64", "inquiry", "--unicast"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !sc.lastSendOpts.Unicast {
		t.Error("expected Unicast=true")
	}
	if !strings.Contains(buf.String(), "unicast") {
		t.Errorf("output missing unicast marker: %s", buf.String())
	}
}

func TestSendCmd_MulticastWithZeroIP(t *testing.T) {
	buf := withStdout(t)
	sc := &fakeScanner{sendResponse: "<ok/>"}
	withScanner(t, sc)

	err := SendCmd([]string{"0.0.0.0", "inquiry", "--mac", "AA:BB:CC:DD:EE:FF"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(buf.String(), "multicast/broadcast (target MAC:") {
		t.Errorf("output missing MAC-based multicast marker: %s", buf.String())
	}
	if sc.lastSendOpts.TargetMAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("TargetMAC = %q, want AA:BB:CC:DD:EE:FF", sc.lastSendOpts.TargetMAC)
	}
}

func TestSendCmd_MulticastDefault(t *testing.T) {
	buf := withStdout(t)
	sc := &fakeScanner{sendResponse: "<ok/>"}
	withScanner(t, sc)

	err := SendCmd([]string{"192.168.1.64", "inquiry"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if sc.lastSendOpts.Unicast {
		t.Error("expected Unicast=false (default)")
	}
	if !strings.Contains(buf.String(), "reply correlated by MAC/IP") {
		t.Errorf("output missing multicast marker: %s", buf.String())
	}
}

func TestSendCmd_MissingCommandDefaultsInquiry(t *testing.T) {
	sc := &fakeScanner{sendResponse: "<ok/>"}
	withStdout(t)
	withScanner(t, sc)

	if err := SendCmd([]string{"192.168.1.64"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if sc.lastSendCmd != "inquiry" {
		t.Errorf("lastSendCmd = %q, want inquiry", sc.lastSendCmd)
	}
}

func TestSendCmd_ScannerError(t *testing.T) {
	withStdout(t)
	withScanner(t, &fakeScanner{sendErr: errors.New("no response")})

	err := SendCmd([]string{"192.168.1.64", "inquiry"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResetCmd_MissingSerialAndDate(t *testing.T) {
	buf := withStdout(t)
	if err := ResetCmd(nil); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(buf.String(), "Reset Code Generator") {
		t.Errorf("output missing header: %s", buf.String())
	}
}

func TestResetCmd_InvalidDate(t *testing.T) {
	withStdout(t)
	err := ResetCmd([]string{"--serial", "ABC", "--date", "2020"})
	if err == nil || !strings.Contains(err.Error(), "YYYYMMDD") {
		t.Fatalf("err = %v", err)
	}
}

func TestResetCmd_SerialAndDateSuccess(t *testing.T) {
	buf := withStdout(t)
	if err := ResetCmd([]string{"--serial", "ABC12345", "--date", "20241225"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(buf.String(), "RESET CODE:") {
		t.Errorf("output missing reset code: %s", buf.String())
	}
}

func TestResetCmd_IPAutoFetchSuccess(t *testing.T) {
	withStdout(t)
	withHTTP(t, &fakeHTTP{
		responses: map[string]*network.HTTPResponse{
			"/upnpdevicedesc.xml": {
				StatusCode: 200,
				Body: []byte(`<root>
					<modelNumber>DS-7616NI-I2</modelNumber>
					<serialNumber>DS-7616NI-I20123456789</serialNumber>
				</root>`),
			},
		},
	})

	if err := ResetCmd([]string{"--ip", "192.168.1.64"}); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestResetCmd_IPAutoFetchFailure(t *testing.T) {
	withStdout(t)
	errBuf := withStderr(t)
	withHTTP(t, &fakeHTTP{errs: map[string]error{"/upnpdevicedesc.xml": errors.New("boom")}})

	if err := ResetCmd([]string{"--ip", "192.168.1.64"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(errBuf.String(), "Could not auto-fetch") {
		t.Errorf("stderr = %q, want warning", errBuf.String())
	}
}

func TestFetchDeviceInfo_HTTPError(t *testing.T) {
	withStdout(t)
	withHTTP(t, &fakeHTTP{errs: map[string]error{"/upnpdevicedesc.xml": errors.New("dial fail")}})

	cfg := makeCfg()
	_, _, err := fetchDeviceInfo(cfg, "192.168.1.1", false)
	if err == nil || !strings.Contains(err.Error(), "failed to connect") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchDeviceInfo_Non200(t *testing.T) {
	withStdout(t)
	withHTTP(t, &fakeHTTP{responses: map[string]*network.HTTPResponse{
		"/upnpdevicedesc.xml": {StatusCode: 500, Body: []byte("err")},
	}})

	cfg := makeCfg()
	_, _, err := fetchDeviceInfo(cfg, "192.168.1.1", false)
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchDeviceInfo_NoSerial(t *testing.T) {
	withStdout(t)
	withHTTP(t, &fakeHTTP{responses: map[string]*network.HTTPResponse{
		"/upnpdevicedesc.xml": {StatusCode: 200, Body: []byte("<root></root>")},
	}})

	cfg := makeCfg()
	_, _, err := fetchDeviceInfo(cfg, "192.168.1.1", true)
	if err == nil || !strings.Contains(err.Error(), "could not find serial") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchDeviceInfo_SerialWithModelPrefix(t *testing.T) {
	withStdout(t)
	body := `<root>
		<modelNumber>DS-2CD</modelNumber>
		<serialNumber>DS-2CD1234</serialNumber>
	</root>`
	withHTTP(t, &fakeHTTP{responses: map[string]*network.HTTPResponse{
		"/upnpdevicedesc.xml": {StatusCode: 200, Body: []byte(body)},
	}})

	cfg := makeCfg()
	serial, date, err := fetchDeviceInfo(cfg, "192.168.1.1", true)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if serial != "1234" {
		t.Errorf("serial = %q, want 1234", serial)
	}
	if len(date) != 8 {
		t.Errorf("date = %q, want 8 chars", date)
	}
}

func TestFetchDeviceInfo_DebugTruncatesLongBody(t *testing.T) {
	withStdout(t)
	body := strings.Repeat("A", 1000) + "<serialNumber>SN123</serialNumber>"
	withHTTP(t, &fakeHTTP{responses: map[string]*network.HTTPResponse{
		"/upnpdevicedesc.xml": {StatusCode: 200, Body: []byte(body)},
	}})

	cfg := makeCfg()
	serial, _, err := fetchDeviceInfo(cfg, "192.168.1.1", true)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if serial != "SN123" {
		t.Errorf("serial = %q, want SN123", serial)
	}
}

func TestScanCmd_InvalidCIDR(t *testing.T) {
	withStdout(t)
	err := ScanCmd([]string{"nope"})
	if err == nil || !strings.Contains(err.Error(), "invalid CIDR") {
		t.Fatalf("err = %v", err)
	}
}

func TestScanCmd_Success(t *testing.T) {
	buf := withStdout(t)
	withScanner(t, &fakeScanner{
		discoverDevices: []*sadp.Device{
			{MAC: "AA:BB:CC:DD:EE:FF", IPv4Address: "192.168.1.64", Activated: "true"},
		},
	})
	withARP(t, map[string]string{"192.168.1.1": "4C:BD:8F:00:11:22"}, nil)
	withHostAlive(t, func(ip string, _ time.Duration) bool { return ip == "192.168.1.1" })

	if err := ScanCmd([]string{"192.168.1.0/30"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(buf.String(), "4C:BD:8F:00:11:22") {
		t.Errorf("output missing ARP MAC: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "AA:BB:CC:DD:EE:FF") {
		t.Errorf("output missing SADP MAC: %s", buf.String())
	}
}

func TestScanCmd_SADPFailureIsLogged(t *testing.T) {
	withStdout(t)
	withScanner(t, &fakeScanner{discoverErr: errors.New("sadp broke")})
	withARP(t, map[string]string{}, nil)
	withHostAlive(t, func(string, time.Duration) bool { return false })

	if err := ScanCmd([]string{"192.168.1.0/30"}); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestProbeCmd_Success(t *testing.T) {
	buf := withStdout(t)
	withHTTP(t, &fakeHTTP{
		responses: map[string]*network.HTTPResponse{
			"/System/deviceInfo": {
				StatusCode: 200,
				Body:       []byte("<deviceInfo><firmwareVersion>V5.5.0</firmwareVersion><deviceName>Cam1</deviceName></deviceInfo>"),
			},
		},
		errs: map[string]error{
			"/ISAPI/System/deviceInfo": errors.New("connection refused"),
		},
	})

	if err := ProbeCmd([]string{"192.168.1.64"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(buf.String(), "Firmware: V5.5.0") {
		t.Errorf("output missing firmware: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "ERROR:") {
		t.Errorf("output missing per-endpoint error: %s", buf.String())
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	withStdout(t)
	if err := Run([]string{"bogus"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestConfigLoadErrorFromEachCommand(t *testing.T) {
	withStdout(t)
	t.Setenv("HTTP_TIMEOUT", "nonsense")

	tests := []struct {
		name string
		fn   func([]string) error
	}{
		{"DiscoverCmd", DiscoverCmd},
		{"DiscoverSADPCmd", DiscoverSADPCmd},
		{"SendCmd", SendCmd},
		{"ResetCmd", ResetCmd},
		{"ScanCmd", ScanCmd},
		{"ProbeCmd", ProbeCmd},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn(nil)
			if err == nil || !strings.Contains(err.Error(), "failed to load config") {
				t.Fatalf("%s err = %v", tt.name, err)
			}
		})
	}
}

func makeCfg() *config.Config { return config.DefaultConfig() }

func TestMain(m *testing.M) {
	stdout = io.Discard
	stderr = io.Discard
	os.Exit(m.Run())
}
