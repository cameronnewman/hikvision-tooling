package sadp

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Command represents a SADP command template
type Command struct {
	Name        string
	Description string
	Template    string
	NeedsMAC    bool
	NeedsPass   bool
}

// Commands is the list of available SADP commands
var Commands = map[string]Command{
	"inquiry": {
		Name:        "inquiry",
		Description: "Get device information",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><Types>inquiry</Types></Probe>`,
		NeedsMAC:    false,
		NeedsPass:   false,
	},
	"inquiry_v32": {
		Name:        "inquiry_v32",
		Description: "Get device information (v32 format)",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><Types>inquiry_v32</Types></Probe>`,
		NeedsMAC:    false,
		NeedsPass:   false,
	},
	"exchangecode": {
		Name:        "exchangecode",
		Description: "Get exchange code for password reset",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><MAC>%s</MAC><Types>exchangecode</Types><Code></Code></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   false,
	},
	"getencryptstring": {
		Name:        "getencryptstring",
		Description: "Get encryption string",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><MAC>%s</MAC><Types>getencryptstring</Types></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   false,
	},
	"getencryptstring_v31": {
		Name:        "getencryptstring_v31",
		Description: "Get encryption string (v31 format)",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><MAC>%s</MAC><Types>getencryptstring_v31</Types></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   false,
	},
	"activate": {
		Name:        "activate",
		Description: "Activate an inactive device with a new password",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><MAC>%s</MAC><Types>activate</Types><Password>%s</Password></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   true,
	},
	"update": {
		Name:        "update",
		Description: "Update device network parameters",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><Types>update</Types><PWErrorParse>true</PWErrorParse><MAC>%s</MAC><Password>%s</Password><IPv4Address>%s</IPv4Address><CommandPort>%d</CommandPort><IPv4SubnetMask>%s</IPv4SubnetMask><IPv4Gateway>%s</IPv4Gateway><DHCP>%s</DHCP></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   true,
	},
	"reboot": {
		Name:        "reboot",
		Description: "Reboot the device",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><MAC>%s</MAC><Types>reboot</Types><Password>%s</Password></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   true,
	},
	"restore": {
		Name:        "restore",
		Description: "Restore device to factory defaults",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><MAC>%s</MAC><Types>restore</Types><Password>%s</Password></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   true,
	},
	"setmailbox": {
		Name:        "setmailbox",
		Description: "Set recovery email address",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><MAC>%s</MAC><Types>SetMailBox</Types><MailBox>%s</MailBox><Password>%s</Password></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   true,
	},
	"ezvizunbind": {
		Name:        "ezvizunbind",
		Description: "Unbind device from Ezviz cloud",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><MAC>%s</MAC><Types>ezvizUnbind</Types><Password>%s</Password></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   true,
	},
	"getbindlist": {
		Name:        "getbindlist",
		Description: "Get device binding list",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><MAC>%s</MAC><Types>getBindList</Types></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   false,
	},
	"getqrcodes": {
		Name:        "getqrcodes",
		Description: "Get QR codes for device",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><MAC>%s</MAC><Types>GetQRcodes</Types></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   false,
	},
	"resetpassword": {
		Name:        "resetpassword",
		Description: "Reset password using security code",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><MAC>%s</MAC><Types>resetPassword</Types><Code>%s</Code><Password>%s</Password></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   true,
	},
	"securitycode": {
		Name:        "securitycode",
		Description: "Submit security code for password reset",
		Template:    `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><MAC>%s</MAC><Types>securityCode</Types><SecurityCode>%s</SecurityCode><Password>%s</Password></Probe>`,
		NeedsMAC:    true,
		NeedsPass:   true,
	},
}

// SendOptions contains options for sending commands
type SendOptions struct {
	TargetIP   string
	TargetMAC  string
	Password   string
	Code       string
	NewIP      string
	NewMask    string
	NewGateway string
	NewPort    int
	DHCP       bool
	Email      string
	Timeout    time.Duration
}

// BuildCommandXML builds the XML for a SADP command
func (s *Scanner) BuildCommandXML(cmdName string, opts SendOptions) (string, error) {
	cmd, ok := Commands[cmdName]
	if !ok {
		return "", fmt.Errorf("unknown command: %s", cmdName)
	}

	probeUUID := uuid.New().String()

	var xmlCmd string
	switch cmdName {
	case "inquiry", "inquiry_v32":
		xmlCmd = fmt.Sprintf(cmd.Template, probeUUID)
	case "exchangecode", "getencryptstring", "getencryptstring_v31", "getbindlist", "getqrcodes":
		if opts.TargetMAC == "" {
			return "", fmt.Errorf("MAC address required for %s command", cmdName)
		}
		xmlCmd = fmt.Sprintf(cmd.Template, probeUUID, opts.TargetMAC)
	case "activate", "reboot", "restore", "ezvizunbind":
		if opts.TargetMAC == "" {
			return "", fmt.Errorf("MAC address required for %s command", cmdName)
		}
		if opts.Password == "" {
			return "", fmt.Errorf("password required for %s command", cmdName)
		}
		xmlCmd = fmt.Sprintf(cmd.Template, probeUUID, opts.TargetMAC, opts.Password)
	case "resetpassword", "securitycode":
		if opts.TargetMAC == "" {
			return "", fmt.Errorf("MAC address required for %s command", cmdName)
		}
		if opts.Code == "" {
			return "", fmt.Errorf("security code required for %s command", cmdName)
		}
		if opts.Password == "" {
			return "", fmt.Errorf("new password required for %s command", cmdName)
		}
		xmlCmd = fmt.Sprintf(cmd.Template, probeUUID, opts.TargetMAC, opts.Code, opts.Password)
	case "setmailbox":
		if opts.TargetMAC == "" || opts.Password == "" || opts.Email == "" {
			return "", fmt.Errorf("MAC, password, and email required for setmailbox command")
		}
		xmlCmd = fmt.Sprintf(cmd.Template, probeUUID, opts.TargetMAC, opts.Email, opts.Password)
	case "update":
		if opts.TargetMAC == "" || opts.Password == "" {
			return "", fmt.Errorf("MAC and password required for update command")
		}
		dhcpStr := "false"
		if opts.DHCP {
			dhcpStr = "true"
		}
		if opts.NewIP == "" {
			opts.NewIP = opts.TargetIP
		}
		if opts.NewMask == "" {
			opts.NewMask = "255.255.255.0"
		}
		if opts.NewPort == 0 {
			opts.NewPort = 8000
		}
		xmlCmd = fmt.Sprintf(cmd.Template, probeUUID, opts.TargetMAC, opts.Password,
			opts.NewIP, opts.NewPort, opts.NewMask, opts.NewGateway, dhcpStr)
	default:
		return "", fmt.Errorf("command %s not implemented", cmdName)
	}

	return xmlCmd, nil
}

// SendCommand sends a SADP command to a device and returns the response
func (s *Scanner) SendCommand(cmdName string, opts SendOptions) (string, error) {
	xmlCmd, err := s.BuildCommandXML(cmdName, opts)
	if err != nil {
		return "", err
	}

	if opts.TargetIP == "0.0.0.0" || opts.TargetIP == "" {
		if opts.TargetMAC == "" {
			return "", fmt.Errorf("MAC address required when target IP is 0.0.0.0")
		}
		return s.sendCommandBroadcastWithMAC(xmlCmd, opts)
	}

	s.log.Debugw("Sending command", "target", opts.TargetIP, "port", Port)
	s.log.Debugw("XML command", "xml", xmlCmd)

	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{
		IP:   net.ParseIP(opts.TargetIP),
		Port: Port,
	})
	if err != nil {
		return "", fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))

	_, err = conn.Write([]byte(xmlCmd))
	if err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	buf := make([]byte, MaxPacketSize)
	n, err := conn.Read(buf)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return "", fmt.Errorf("no response (timeout)")
		}
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(buf[:n]), nil
}

func (s *Scanner) sendCommandBroadcastWithMAC(xmlCmd string, opts SendOptions) (string, error) {
	s.log.Debugw("Sending command via broadcast", "targetMAC", opts.TargetMAC)
	s.log.Debugw("XML command", "xml", xmlCmd)

	targetMAC := strings.ToUpper(strings.ReplaceAll(opts.TargetMAC, "-", ":"))

	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to get network interfaces: %w", err)
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	responseChan := make(chan string, 10)
	var wg sync.WaitGroup

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}

			wg.Add(1)
			go func(localIP net.IP, ifaceName string, ipNet *net.IPNet) {
				defer wg.Done()

				s.log.Debugw("Sending on interface", "interface", ifaceName, "ip", localIP.String())

				conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: localIP, Port: 0})
				if err != nil {
					return
				}
				defer conn.Close()

				multicastAddr := &net.UDPAddr{IP: net.ParseIP(MulticastAddr), Port: Port}
				_, _ = conn.WriteToUDP([]byte(xmlCmd), multicastAddr)

				broadcastAddr := &net.UDPAddr{IP: net.IPv4bcast, Port: Port}
				_, _ = conn.WriteToUDP([]byte(xmlCmd), broadcastAddr)

				subnetBcast := make(net.IP, 4)
				for i := 0; i < 4; i++ {
					subnetBcast[i] = localIP[i] | ^ipNet.Mask[i]
				}
				subnetBcastAddr := &net.UDPAddr{IP: subnetBcast, Port: Port}
				_, _ = conn.WriteToUDP([]byte(xmlCmd), subnetBcastAddr)

				_ = conn.SetReadDeadline(time.Now().Add(timeout))

				buf := make([]byte, MaxPacketSize)
				for {
					n, _, err := conn.ReadFromUDP(buf)
					if err != nil {
						break
					}

					response := string(buf[:n])

					if strings.Contains(strings.ToUpper(response), targetMAC) ||
						strings.Contains(strings.ToUpper(response), strings.ReplaceAll(targetMAC, ":", "-")) {
						select {
						case responseChan <- response:
						default:
						}
					}
				}
			}(ip, iface.Name, ipNet)
		}
	}

	go func() {
		wg.Wait()
		close(responseChan)
	}()

	for response := range responseChan {
		return response, nil
	}

	return "", fmt.Errorf("no response from device with MAC %s (timeout)", opts.TargetMAC)
}

// ListCommands prints the list of available commands
func ListCommands() []Command {
	order := []string{
		"inquiry", "inquiry_v32", "exchangecode", "getencryptstring",
		"getencryptstring_v31", "getqrcodes", "getbindlist",
		"resetpassword", "securitycode",
		"activate", "update", "reboot", "restore", "setmailbox", "ezvizunbind",
	}

	result := make([]Command, 0, len(order))
	for _, name := range order {
		if cmd, ok := Commands[name]; ok {
			result = append(result, cmd)
		}
	}
	return result
}
