package sadp

import (
	"strings"
	"testing"
	"time"

	"github.com/cameronnewman/hikvision-tooling/internal/logger"
)

func TestNewScanner(t *testing.T) {
	tests := []struct {
		name        string
		timeout     time.Duration
		log         *logger.Logger
		wantTimeout time.Duration
		wantNilLog  bool
	}{
		{
			name:        "with logger",
			timeout:     5 * time.Second,
			log:         logger.NewNop(),
			wantTimeout: 5 * time.Second,
			wantNilLog:  false,
		},
		{
			name:        "nil logger uses nop",
			timeout:     5 * time.Second,
			log:         nil,
			wantTimeout: 5 * time.Second,
			wantNilLog:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := NewScanner(tt.timeout, tt.log)
			if scanner == nil {
				t.Fatal("NewScanner returned nil")
			}
			if scanner.timeout != tt.wantTimeout {
				t.Errorf("timeout = %v, want %v", scanner.timeout, tt.wantTimeout)
			}
			if (scanner.log == nil) != tt.wantNilLog {
				t.Errorf("log nil = %v, want nil = %v", scanner.log == nil, tt.wantNilLog)
			}
		})
	}
}

func TestParseResponse(t *testing.T) {
	log := logger.NewNop()
	scanner := NewScanner(5*time.Second, log)

	tests := []struct {
		name        string
		data        string
		expectNil   bool
		expectedMAC string
	}{
		{
			name:      "empty string",
			data:      "",
			expectNil: true,
		},
		{
			name:      "non-ProbeMatch",
			data:      `<?xml version="1.0"?><Other></Other>`,
			expectNil: true,
		},
		{
			name: "valid ProbeMatch",
			data: `<?xml version="1.0" encoding="utf-8"?>
<ProbeMatch>
    <Uuid>test-uuid</Uuid>
    <MAC>aa-bb-cc-dd-ee-ff</MAC>
    <IPv4Address>192.168.1.100</IPv4Address>
    <DeviceType>TestDevice</DeviceType>
    <Activated>true</Activated>
</ProbeMatch>`,
			expectNil:   false,
			expectedMAC: "AA:BB:CC:DD:EE:FF",
		},
		{
			name:      "invalid XML",
			data:      `<ProbeMatch><Invalid`,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := scanner.parseResponse(tt.data)
			if tt.expectNil && device != nil {
				t.Error("expected nil device")
			}
			if !tt.expectNil && device == nil {
				t.Error("expected non-nil device")
			}
			if !tt.expectNil && device != nil && device.MAC != tt.expectedMAC {
				t.Errorf("MAC = %q, want %q", device.MAC, tt.expectedMAC)
			}
		})
	}
}

func TestToXML(t *testing.T) {
	log := logger.NewNop()
	scanner := NewScanner(5*time.Second, log)

	tests := []struct {
		name           string
		devices        []*Device
		wantContains   []string
		wantErr        bool
	}{
		{
			name: "multiple devices",
			devices: []*Device{
				{
					Uuid:        "uuid-1",
					MAC:         "AA:BB:CC:DD:EE:FF",
					IPv4Address: "192.168.1.100",
					DeviceType:  "Camera",
					Activated:   "true",
				},
				{
					Uuid:        "uuid-2",
					MAC:         "11:22:33:44:55:66",
					IPv4Address: "192.168.1.101",
					DeviceType:  "NVR",
					Activated:   "false",
				},
			},
			wantContains: []string{"<?xml version", "SADPDeviceList", "192.168.1.100"},
			wantErr:      false,
		},
		{
			name:         "empty device list",
			devices:      []*Device{},
			wantContains: []string{"SADPDeviceList"},
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xml, err := scanner.ToXML(tt.devices)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ToXML() error = %v, wantErr %v", err, tt.wantErr)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(xml, want) {
					t.Errorf("XML should contain %q", want)
				}
			}
		})
	}
}

func TestToCSV(t *testing.T) {
	log := logger.NewNop()
	scanner := NewScanner(5*time.Second, log)

	tests := []struct {
		name         string
		devices      []*Device
		wantContains []string
	}{
		{
			name: "single device",
			devices: []*Device{
				{
					MAC:               "AA:BB:CC:DD:EE:FF",
					IPv4Address:       "192.168.1.100",
					DeviceType:        "Camera",
					Activated:         "true",
					CommandPort:       8000,
					HttpPort:          80,
					SoftwareVersion:   "V5.5.0",
					IPv4Gateway:       "192.168.1.1",
					DeviceSN:          "SN123456",
					IPv4SubnetMask:    "255.255.255.0",
					AnalogChannelNum:  0,
					DigitalChannelNum: 4,
					DSPVersion:        "1.0",
					BootTime:          "2024-01-01",
					DHCP:              "false",
				},
			},
			wantContains: []string{"ID,DeviceType", "192.168.1.100", "Camera"},
		},
		{
			name:         "empty device list",
			devices:      []*Device{},
			wantContains: []string{"ID,DeviceType"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			csv := scanner.ToCSV(tt.devices)

			for _, want := range tt.wantContains {
				if !strings.Contains(csv, want) {
					t.Errorf("CSV should contain %q", want)
				}
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string",
			input:    "short",
			maxLen:   10,
			expected: "short",
		},
		{
			name:     "exactly at limit",
			input:    "exactly10!",
			maxLen:   10,
			expected: "exactly10!",
		},
		{
			name:     "over limit",
			input:    "this is a very long string",
			maxLen:   10,
			expected: "this is...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "at limit exactly",
			input:    "abc",
			maxLen:   3,
			expected: "abc",
		},
		{
			name:     "over limit short max",
			input:    "abcd",
			maxLen:   3,
			expected: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("Truncate(%q, %d) = %q, want %q",
					tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestDeviceStruct(t *testing.T) {
	tests := []struct {
		name              string
		uuid              string
		mac               string
		ipv4Address       string
		deviceType        string
		deviceDescription string
		deviceSN          string
		ipv4SubnetMask    string
		ipv4Gateway       string
		dhcp              string
		commandPort       int
		httpPort          int
		softwareVersion   string
		activated         string
		analogChannelNum  int
		digitalChannelNum int
	}{
		{
			name:              "camera device",
			uuid:              "test-uuid",
			mac:               "AA:BB:CC:DD:EE:FF",
			ipv4Address:       "192.168.1.100",
			deviceType:        "DS-2CD2042WD-I",
			deviceDescription: "Network Camera",
			deviceSN:          "SN123456789",
			ipv4SubnetMask:    "255.255.255.0",
			ipv4Gateway:       "192.168.1.1",
			dhcp:              "false",
			commandPort:       8000,
			httpPort:          80,
			softwareVersion:   "V5.5.0 build 191126",
			activated:         "true",
			analogChannelNum:  0,
			digitalChannelNum: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := &Device{
				Uuid:              tt.uuid,
				Types:             "inquiry",
				DeviceType:        tt.deviceType,
				DeviceDescription: tt.deviceDescription,
				DeviceSN:          tt.deviceSN,
				MAC:               tt.mac,
				IPv4Address:       tt.ipv4Address,
				IPv4SubnetMask:    tt.ipv4SubnetMask,
				IPv4Gateway:       tt.ipv4Gateway,
				DHCP:              tt.dhcp,
				CommandPort:       tt.commandPort,
				HttpPort:          tt.httpPort,
				SoftwareVersion:   tt.softwareVersion,
				Activated:         tt.activated,
				AnalogChannelNum:  tt.analogChannelNum,
				DigitalChannelNum: tt.digitalChannelNum,
			}

			if device.Uuid != tt.uuid {
				t.Error("Uuid not set correctly")
			}
			if device.MAC != tt.mac {
				t.Error("MAC not set correctly")
			}
			if device.IPv4Address != tt.ipv4Address {
				t.Error("IPv4Address not set correctly")
			}
		})
	}
}

func TestConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{
			name:     "MulticastAddr",
			got:      MulticastAddr,
			expected: "239.255.255.250",
		},
		{
			name:     "Port",
			got:      Port,
			expected: 37020,
		},
		{
			name:     "MaxPacketSize",
			got:      MaxPacketSize,
			expected: 65535,
		},
		{
			name:     "DefaultTimeout",
			got:      DefaultTimeout,
			expected: 5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}
