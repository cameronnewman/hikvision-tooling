package network

import (
	"testing"
	"time"
)

func TestParseARPLine(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		expectedIP  string
		expectedMAC string
	}{
		{
			name:        "macOS format",
			line:        "? (192.168.1.1) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]",
			expectedIP:  "192.168.1.1",
			expectedMAC: "aa:bb:cc:dd:ee:ff",
		},
		{
			name:        "Linux format",
			line:        "192.168.1.1    ether   aa:bb:cc:dd:ee:ff   C   eth0",
			expectedIP:  "192.168.1.1",
			expectedMAC: "aa:bb:cc:dd:ee:ff",
		},
		{
			name:        "Windows format",
			line:        "192.168.1.1    aa-bb-cc-dd-ee-ff    dynamic",
			expectedIP:  "192.168.1.1",
			expectedMAC: "aa:bb:cc:dd:ee:ff",
		},
		{
			name:        "empty line",
			line:        "",
			expectedIP:  "",
			expectedMAC: "",
		},
		{
			name:        "incomplete line",
			line:        "incomplete",
			expectedIP:  "",
			expectedMAC: "",
		},
		{
			name:        "macOS with incomplete marker",
			line:        "? (10.0.0.1) at 00:11:22:33:44:55 on en0",
			expectedIP:  "10.0.0.1",
			expectedMAC: "00:11:22:33:44:55",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, mac := ParseARPLine(tt.line)
			if ip != tt.expectedIP {
				t.Errorf("IP: got %q, want %q", ip, tt.expectedIP)
			}
			if mac != tt.expectedMAC {
				t.Errorf("MAC: got %q, want %q", mac, tt.expectedMAC)
			}
		})
	}
}

func TestIsValidMAC(t *testing.T) {
	tests := []struct {
		name  string
		mac   string
		valid bool
	}{
		{
			name:  "lowercase colon separated",
			mac:   "aa:bb:cc:dd:ee:ff",
			valid: true,
		},
		{
			name:  "uppercase colon separated",
			mac:   "AA:BB:CC:DD:EE:FF",
			valid: true,
		},
		{
			name:  "dash separated",
			mac:   "aa-bb-cc-dd-ee-ff",
			valid: true,
		},
		{
			name:  "numeric MAC",
			mac:   "00:11:22:33:44:55",
			valid: true,
		},
		{
			name:  "invalid string",
			mac:   "invalid",
			valid: false,
		},
		{
			name:  "too short",
			mac:   "aa:bb:cc:dd:ee",
			valid: false,
		},
		{
			name:  "too long",
			mac:   "aa:bb:cc:dd:ee:ff:00",
			valid: false,
		},
		{
			name:  "invalid hex characters",
			mac:   "gg:hh:ii:jj:kk:ll",
			valid: false,
		},
		{
			name:  "empty string",
			mac:   "",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidMAC(tt.mac)
			if result != tt.valid {
				t.Errorf("IsValidMAC(%q) = %v, want %v", tt.mac, result, tt.valid)
			}
		})
	}
}

func TestIsHikvisionMAC(t *testing.T) {
	tests := []struct {
		name     string
		mac      string
		expected bool
	}{
		{
			name:     "Hikvision OUI 00:0d:c5",
			mac:      "00:0d:c5:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI 28:57:be",
			mac:      "28:57:be:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI 44:19:b6",
			mac:      "44:19:b6:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI 54:c4:15",
			mac:      "54:c4:15:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI 80:cc:9c",
			mac:      "80:cc:9c:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI a4:14:37",
			mac:      "a4:14:37:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI bc:ad:28",
			mac:      "bc:ad:28:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI c0:56:e3",
			mac:      "c0:56:e3:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI c4:2f:90",
			mac:      "c4:2f:90:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI e0:2f:6d",
			mac:      "e0:2f:6d:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI f4:52:14",
			mac:      "f4:52:14:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI 48:40:a9",
			mac:      "48:40:a9:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI 8c:e7:48",
			mac:      "8c:e7:48:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI 4c:bd:8f",
			mac:      "4c:bd:8f:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI 18:68:cb",
			mac:      "18:68:cb:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI 44:47:cc",
			mac:      "44:47:cc:11:22:33",
			expected: true,
		},
		{
			name:     "Hikvision OUI e4:24:6c",
			mac:      "e4:24:6c:11:22:33",
			expected: true,
		},
		{
			name:     "non-Hikvision random MAC",
			mac:      "aa:bb:cc:dd:ee:ff",
			expected: false,
		},
		{
			name:     "non-Hikvision zeros",
			mac:      "00:00:00:00:00:00",
			expected: false,
		},
		{
			name:     "Hikvision with dashes",
			mac:      "00-0d-c5-11-22-33",
			expected: true,
		},
		{
			name:     "Hikvision uppercase",
			mac:      "00:0D:C5:11:22:33",
			expected: true,
		},
		{
			name:     "invalid format",
			mac:      "invalid",
			expected: false,
		},
		{
			name:     "too short",
			mac:      "aa:bb",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsHikvisionMAC(tt.mac)
			if result != tt.expected {
				t.Errorf("IsHikvisionMAC(%q) = %v, want %v", tt.mac, result, tt.expected)
			}
		})
	}
}

func TestExpandCIDR(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		minIPs  int
		maxIPs  int
		wantErr bool
	}{
		{
			name:    "small subnet /30",
			cidr:    "192.168.1.0/30",
			minIPs:  2,
			maxIPs:  3,
			wantErr: false,
		},
		{
			name:    "class C subnet /24",
			cidr:    "192.168.1.0/24",
			minIPs:  253,
			maxIPs:  254,
			wantErr: false,
		},
		{
			name:    "single IP address",
			cidr:    "192.168.1.100",
			minIPs:  1,
			maxIPs:  1,
			wantErr: false,
		},
		{
			name:    "invalid CIDR",
			cidr:    "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ips, err := ExpandCIDR(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandCIDR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(ips) < tt.minIPs || len(ips) > tt.maxIPs {
					t.Errorf("ExpandCIDR() returned %d IPs, want between %d and %d",
						len(ips), tt.minIPs, tt.maxIPs)
				}
			}
		})
	}
}

func TestHikvisionOUIs(t *testing.T) {
	tests := []struct {
		name string
		oui  string
	}{
		{name: "OUI 00:0d:c5", oui: "00:0d:c5"},
		{name: "OUI 28:57:be", oui: "28:57:be"},
		{name: "OUI 44:19:b6", oui: "44:19:b6"},
		{name: "OUI 54:c4:15", oui: "54:c4:15"},
		{name: "OUI 80:cc:9c", oui: "80:cc:9c"},
		{name: "OUI a4:14:37", oui: "a4:14:37"},
		{name: "OUI bc:ad:28", oui: "bc:ad:28"},
		{name: "OUI c0:56:e3", oui: "c0:56:e3"},
		{name: "OUI c4:2f:90", oui: "c4:2f:90"},
		{name: "OUI e0:2f:6d", oui: "e0:2f:6d"},
		{name: "OUI f4:52:14", oui: "f4:52:14"},
		{name: "OUI 48:40:a9", oui: "48:40:a9"},
		{name: "OUI 8c:e7:48", oui: "8c:e7:48"},
		{name: "OUI 4c:bd:8f", oui: "4c:bd:8f"},
		{name: "OUI 18:68:cb", oui: "18:68:cb"},
		{name: "OUI 44:47:cc", oui: "44:47:cc"},
		{name: "OUI e4:24:6c", oui: "e4:24:6c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.oui) != 8 {
				t.Errorf("OUI %q should be 8 characters", tt.oui)
			}
			if !IsValidMAC(tt.oui + ":00:00:00") {
				t.Errorf("OUI %q forms invalid MAC", tt.oui)
			}
		})
	}
}

func TestGetARPTable(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "returns valid map",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table, err := GetARPTable()
			if err != nil {
				t.Skipf("GetARPTable() not supported on this system: %v", err)
			}

			if table == nil {
				t.Error("GetARPTable() returned nil map")
			}
		})
	}
}

func TestIsHostAlive(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		timeout time.Duration
	}{
		{
			name:    "localhost best effort",
			ip:      "127.0.0.1",
			timeout: 100 * time.Millisecond,
		},
		{
			name:    "unreachable TEST-NET-1",
			ip:      "192.0.2.1",
			timeout: 100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsHostAlive(tt.ip, tt.timeout)
			if tt.ip == "192.0.2.1" && result {
				t.Log("192.0.2.1 unexpectedly reachable (this is fine in some network setups)")
			}
		})
	}
}

func TestPingHost(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		timeout time.Duration
	}{
		{
			name:    "unreachable TEST-NET-1",
			ip:      "192.0.2.1",
			timeout: 100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PingHost(tt.ip, tt.timeout)
			if result {
				t.Logf("%s unexpectedly pingable", tt.ip)
			}
		})
	}
}
