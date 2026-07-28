package sadp

import (
	"strings"
	"testing"
	"time"

	"github.com/cameronnewman/hikvision-tooling/internal/logger"
)

func TestBuildCommandXML(t *testing.T) {
	log := logger.NewNop()
	scanner := NewScanner(5*time.Second, log)

	tests := []struct {
		name    string
		cmdName string
		opts    SendOptions
		wantErr bool
		check   func(string) bool
	}{
		{
			name:    "inquiry",
			cmdName: "inquiry",
			opts:    SendOptions{},
			wantErr: false,
			check: func(xml string) bool {
				return strings.Contains(xml, "inquiry") && strings.Contains(xml, "Uuid")
			},
		},
		{
			name:    "inquiry_v32",
			cmdName: "inquiry_v32",
			opts:    SendOptions{},
			wantErr: false,
			check: func(xml string) bool {
				return strings.Contains(xml, "inquiry_v32")
			},
		},
		{
			name:    "exchangecode without MAC",
			cmdName: "exchangecode",
			opts:    SendOptions{},
			wantErr: true,
		},
		{
			name:    "exchangecode with MAC",
			cmdName: "exchangecode",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF"},
			wantErr: false,
			check: func(xml string) bool {
				return strings.Contains(xml, "exchangecode") && strings.Contains(xml, "AA:BB:CC:DD:EE:FF")
			},
		},
		{
			name:    "getencryptstring with MAC",
			cmdName: "getencryptstring",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF"},
			wantErr: false,
		},
		{
			name:    "getencryptstring_v31 with MAC",
			cmdName: "getencryptstring_v31",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF"},
			wantErr: false,
		},
		{
			name:    "getbindlist with MAC",
			cmdName: "getbindlist",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF"},
			wantErr: false,
		},
		{
			name:    "getqrcodes with MAC",
			cmdName: "getqrcodes",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF"},
			wantErr: false,
		},
		{
			name:    "activate without MAC",
			cmdName: "activate",
			opts:    SendOptions{Password: "test123"},
			wantErr: true,
		},
		{
			name:    "activate without password",
			cmdName: "activate",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF"},
			wantErr: true,
		},
		{
			name:    "activate with MAC and password",
			cmdName: "activate",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Password: "test123"},
			wantErr: false,
			check: func(xml string) bool {
				return strings.Contains(xml, "activate") && strings.Contains(xml, "test123")
			},
		},
		{
			name:    "reboot with MAC and password",
			cmdName: "reboot",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Password: "admin"},
			wantErr: false,
		},
		{
			name:    "restore with MAC and password",
			cmdName: "restore",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Password: "admin"},
			wantErr: false,
		},
		{
			name:    "ezvizunbind with MAC and password",
			cmdName: "ezvizunbind",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Password: "admin"},
			wantErr: false,
		},
		{
			name:    "resetpassword without code",
			cmdName: "resetpassword",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Password: "newpass"},
			wantErr: true,
		},
		{
			name:    "resetpassword with all params",
			cmdName: "resetpassword",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Password: "newpass", Code: "ABC123"},
			wantErr: false,
		},
		{
			name:    "securitycode with all params",
			cmdName: "securitycode",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Password: "newpass", Code: "ABC123"},
			wantErr: false,
		},
		{
			name:    "setmailbox missing email",
			cmdName: "setmailbox",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Password: "admin"},
			wantErr: true,
		},
		{
			name:    "setmailbox with all params",
			cmdName: "setmailbox",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Password: "admin", Email: "test@example.com"},
			wantErr: false,
			check: func(xml string) bool {
				return strings.Contains(xml, "test@example.com")
			},
		},
		{
			name:    "update without MAC",
			cmdName: "update",
			opts:    SendOptions{Password: "admin"},
			wantErr: true,
		},
		{
			name:    "update with defaults",
			cmdName: "update",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Password: "admin", TargetIP: "192.168.1.100"},
			wantErr: false,
			check: func(xml string) bool {
				return strings.Contains(xml, "update") && strings.Contains(xml, "255.255.255.0")
			},
		},
		{
			name:    "update with DHCP",
			cmdName: "update",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Password: "admin", DHCP: true},
			wantErr: false,
			check: func(xml string) bool {
				return strings.Contains(xml, "DHCP>true")
			},
		},
		{
			name:    "update with custom params",
			cmdName: "update",
			opts: SendOptions{
				TargetMAC:  "AA:BB:CC:DD:EE:FF",
				Password:   "admin",
				NewIP:      "10.0.0.100",
				NewMask:    "255.255.0.0",
				NewGateway: "10.0.0.1",
				NewPort:    9000,
			},
			wantErr: false,
			check: func(xml string) bool {
				return strings.Contains(xml, "10.0.0.100") && strings.Contains(xml, "9000")
			},
		},
		{
			name:    "unknown command",
			cmdName: "nonexistent",
			opts:    SendOptions{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xml, err := scanner.BuildCommandXML(tt.cmdName, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildCommandXML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil && !tt.check(xml) {
				t.Errorf("BuildCommandXML() = %q, failed check", xml)
			}
		})
	}
}

func TestListCommands(t *testing.T) {
	tests := []struct {
		name         string
		expectedCmds []string
	}{
		{
			name: "contains expected commands",
			expectedCmds: []string{
				"inquiry", "inquiry_v32", "exchangecode", "activate",
				"update", "reboot", "restore",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands := ListCommands()

			if len(commands) == 0 {
				t.Fatal("ListCommands() returned empty list")
			}

			for _, expected := range tt.expectedCmds {
				found := false
				for _, cmd := range commands {
					if cmd.Name == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected command %q not found", expected)
				}
			}
		})
	}
}

func TestCommandsMap(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "all commands have required fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for name, cmd := range Commands {
				if cmd.Name == "" {
					t.Errorf("command %q has empty Name", name)
				}
				if cmd.Description == "" {
					t.Errorf("command %q has empty Description", name)
				}
				if cmd.Template == "" {
					t.Errorf("command %q has empty Template", name)
				}
			}
		})
	}
}

func TestSendOptionsStruct(t *testing.T) {
	tests := []struct {
		name       string
		targetIP   string
		targetMAC  string
		password   string
		code       string
		newIP      string
		newMask    string
		newGateway string
		newPort    int
		dhcp       bool
		email      string
		timeout    time.Duration
	}{
		{
			name:       "full options",
			targetIP:   "192.168.1.100",
			targetMAC:  "AA:BB:CC:DD:EE:FF",
			password:   "admin",
			code:       "123456",
			newIP:      "10.0.0.1",
			newMask:    "255.255.255.0",
			newGateway: "10.0.0.254",
			newPort:    8000,
			dhcp:       true,
			email:      "test@example.com",
			timeout:    10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := SendOptions{
				TargetIP:   tt.targetIP,
				TargetMAC:  tt.targetMAC,
				Password:   tt.password,
				Code:       tt.code,
				NewIP:      tt.newIP,
				NewMask:    tt.newMask,
				NewGateway: tt.newGateway,
				NewPort:    tt.newPort,
				DHCP:       tt.dhcp,
				Email:      tt.email,
				Timeout:    tt.timeout,
			}

			if opts.TargetIP != tt.targetIP {
				t.Error("TargetIP not set correctly")
			}
			if opts.Timeout != tt.timeout {
				t.Error("Timeout not set correctly")
			}
		})
	}
}

func TestSendCommandRoutingPreconditions(t *testing.T) {
	log := logger.NewNop()
	scanner := NewScanner(50*time.Millisecond, log)

	tests := []struct {
		name    string
		opts    SendOptions
		wantErr string
	}{
		{
			name:    "unicast requires an IP",
			opts:    SendOptions{TargetMAC: "AA:BB:CC:DD:EE:FF", Unicast: true},
			wantErr: "unicast mode requires a specific target IP",
		},
		{
			name:    "unicast rejects 0.0.0.0",
			opts:    SendOptions{TargetIP: "0.0.0.0", TargetMAC: "AA:BB:CC:DD:EE:FF", Unicast: true},
			wantErr: "unicast mode requires a specific target IP",
		},
		{
			name:    "broadcast requires a MAC or IP",
			opts:    SendOptions{TargetIP: "0.0.0.0"},
			wantErr: "target MAC or IP required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := scanner.SendCommand("inquiry", tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("SendCommand() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRedactXML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "activate password redacted",
			in:   `<Probe><MAC>AA:BB:CC:DD:EE:FF</MAC><Types>activate</Types><Password>hunter2</Password></Probe>`,
			want: `<Probe><MAC>AA:BB:CC:DD:EE:FF</MAC><Types>activate</Types><Password>***</Password></Probe>`,
		},
		{
			name: "resetpassword redacts Code and Password",
			in:   `<Probe><Code>ABC123</Code><Password>hunter2</Password></Probe>`,
			want: `<Probe><Code>***</Code><Password>***</Password></Probe>`,
		},
		{
			name: "securitycode redacts SecurityCode",
			in:   `<Probe><SecurityCode>XYZ789</SecurityCode><Password>hunter2</Password></Probe>`,
			want: `<Probe><SecurityCode>***</SecurityCode><Password>***</Password></Probe>`,
		},
		{
			name: "empty Code left as ***",
			in:   `<Probe><Code></Code></Probe>`,
			want: `<Probe><Code>***</Code></Probe>`,
		},
		{
			name: "inquiry has no secrets and is untouched",
			in:   `<Probe><Uuid>abc</Uuid><Types>inquiry</Types></Probe>`,
			want: `<Probe><Uuid>abc</Uuid><Types>inquiry</Types></Probe>`,
		},
		{
			name: "MAC and IP are not redacted",
			in:   `<Probe><MAC>AA:BB:CC:DD:EE:FF</MAC><IPv4Address>192.168.1.64</IPv4Address></Probe>`,
			want: `<Probe><MAC>AA:BB:CC:DD:EE:FF</MAC><IPv4Address>192.168.1.64</IPv4Address></Probe>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactXML(tt.in); got != tt.want {
				t.Errorf("redactXML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResponseMatches(t *testing.T) {
	const sampleResponse = `<?xml version="1.0" encoding="utf-8"?>` +
		`<ProbeMatch><Uuid>abc</Uuid><MAC>AA:BB:CC:DD:EE:FF</MAC>` +
		`<IPv4Address>192.168.1.64</IPv4Address></ProbeMatch>`
	const dashedMACResponse = `<?xml version="1.0" encoding="utf-8"?>` +
		`<ProbeMatch><MAC>AA-BB-CC-DD-EE-FF</MAC>` +
		`<IPv4Address>192.168.1.64</IPv4Address></ProbeMatch>`

	tests := []struct {
		name      string
		response  string
		targetMAC string
		targetIP  string
		want      bool
	}{
		{"match by MAC (colon form)", sampleResponse, "AA:BB:CC:DD:EE:FF", "", true},
		{"match by MAC (dash form)", dashedMACResponse, "AA:BB:CC:DD:EE:FF", "", true},
		{"MAC mismatch wins over IP", sampleResponse, "11:22:33:44:55:66", "192.168.1.64", false},
		{"match by IP when MAC absent", sampleResponse, "", "192.168.1.64", true},
		{"IP mismatch", sampleResponse, "", "10.0.0.1", false},
		{"neither target set is a safety-net false", sampleResponse, "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := responseMatches(tt.response, tt.targetMAC, tt.targetIP); got != tt.want {
				t.Errorf("responseMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCommandStruct(t *testing.T) {
	tests := []struct {
		name        string
		cmdName     string
		description string
		template    string
		needsMAC    bool
		needsPass   bool
	}{
		{
			name:        "command with MAC and password",
			cmdName:     "test",
			description: "Test command",
			template:    "<test/>",
			needsMAC:    true,
			needsPass:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := Command{
				Name:        tt.cmdName,
				Description: tt.description,
				Template:    tt.template,
				NeedsMAC:    tt.needsMAC,
				NeedsPass:   tt.needsPass,
			}

			if cmd.Name != tt.cmdName {
				t.Error("Name not set correctly")
			}
			if cmd.NeedsMAC != tt.needsMAC {
				t.Errorf("NeedsMAC = %v, want %v", cmd.NeedsMAC, tt.needsMAC)
			}
			if cmd.NeedsPass != tt.needsPass {
				t.Errorf("NeedsPass = %v, want %v", cmd.NeedsPass, tt.needsPass)
			}
		})
	}
}
