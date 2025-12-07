package sadp

import (
	"encoding/xml"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/cameronnewman/hikvision-tooling/internal/logger"
	"github.com/google/uuid"
)

const (
	MulticastAddr  = "239.255.255.250"
	Port           = 37020
	MaxPacketSize  = 65535
	DefaultTimeout = 5 * time.Second
)

// Device represents a discovered Hikvision device via SADP protocol
type Device struct {
	XMLName           xml.Name `xml:"ProbeMatch" json:"-"`
	Uuid              string   `xml:"Uuid" json:"uuid"`
	Types             string   `xml:"Types" json:"types"`
	DeviceType        string   `xml:"DeviceType" json:"deviceType"`
	DeviceDescription string   `xml:"DeviceDescription" json:"deviceDescription"`
	DeviceSN          string   `xml:"DeviceSN" json:"serialNumber"`
	MAC               string   `xml:"MAC" json:"mac"`
	IPv4Address       string   `xml:"IPv4Address" json:"ipv4Address"`
	IPv4SubnetMask    string   `xml:"IPv4SubnetMask" json:"ipv4SubnetMask"`
	IPv4Gateway       string   `xml:"IPv4Gateway" json:"ipv4Gateway"`
	IPv6Address       string   `xml:"IPv6Address" json:"ipv6Address"`
	IPv6Gateway       string   `xml:"IPv6Gateway" json:"ipv6Gateway"`
	IPv6MaskLen       int      `xml:"IPv6MaskLen" json:"ipv6MaskLen"`
	DHCP              string   `xml:"DHCP" json:"dhcp"`
	CommandPort       int      `xml:"CommandPort" json:"commandPort"`
	HttpPort          int      `xml:"HttpPort" json:"httpPort"`
	DSPVersion        string   `xml:"DSPVersion" json:"dspVersion"`
	BootTime          string   `xml:"BootTime" json:"bootTime"`
	SoftwareVersion   string   `xml:"SoftwareVersion" json:"softwareVersion"`
	Activated         string   `xml:"Activated" json:"activated"`
	PasswordResetMode string   `xml:"PasswordResetModeSecond" json:"passwordResetMode"`
	SupportHCPlatform string   `xml:"SupportHCPlatform" json:"supportHCPlatform"`
	HCPlatformEnable  string   `xml:"HCPlatformEnable" json:"hcPlatformEnable"`
	SupportReset      string   `xml:"Support" json:"supportReset"`
	Encoder           string   `xml:"Encoder" json:"encoder"`
	OEMInfo           string   `xml:"OEMInfo" json:"oemInfo"`
	AnalogChannelNum  int      `xml:"AnalogChannelNum" json:"analogChannelNum"`
	DigitalChannelNum int      `xml:"DigitalChannelNum" json:"digitalChannelNum"`
	SDKOverTLSPort    int      `xml:"SDKOverTLSPort" json:"sdkOverTLSPort"`
	SDKServerStatus   string   `xml:"SDKServerStatus" json:"sdkServerStatus"`
	AdapterIP         string   `xml:"-" json:"adapterIP"`
	ReceivedTime      time.Time `xml:"-" json:"receivedTime"`
}

// DeviceList represents the XML output format
type DeviceList struct {
	XMLName xml.Name `xml:"SADPDeviceList"`
	Version string   `xml:"version,attr"`
	Devices []Device `xml:"Device"`
}

// Scanner handles SADP protocol discovery
type Scanner struct {
	timeout     time.Duration
	log         *logger.Logger
	devices     map[string]*Device
	deviceMutex sync.RWMutex
}

// NewScanner creates a new SADP scanner
func NewScanner(timeout time.Duration, log *logger.Logger) *Scanner {
	if log == nil {
		log = logger.NewNop()
	}
	return &Scanner{
		timeout: timeout,
		log:     log,
		devices: make(map[string]*Device),
	}
}

// Discover performs SADP multicast discovery
func (s *Scanner) Discover() ([]*Device, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

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
			go func(localIP net.IP, ifaceName string) {
				defer wg.Done()
				s.discoverOnInterface(localIP, ifaceName)
			}(ip, iface.Name)
		}
	}

	wg.Wait()

	s.deviceMutex.RLock()
	defer s.deviceMutex.RUnlock()

	result := make([]*Device, 0, len(s.devices))
	for _, dev := range s.devices {
		result = append(result, dev)
	}

	return result, nil
}

func (s *Scanner) discoverOnInterface(localIP net.IP, ifaceName string) {
	s.log.Debugw("Scanning on interface", "interface", ifaceName, "ip", localIP.String())

	localAddr := &net.UDPAddr{IP: localIP, Port: 0}
	conn, err := net.ListenUDP("udp4", localAddr)
	if err != nil {
		s.log.Debugw("Failed to bind", "ip", localIP.String(), "error", err)
		return
	}
	defer conn.Close()

	multicastAddr := &net.UDPAddr{IP: net.ParseIP(MulticastAddr), Port: Port}
	probeUUID := uuid.New().String()

	probePackets := []string{
		fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><Types>inquiry</Types></Probe>`, probeUUID),
		fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><Types>inquiry_v32</Types></Probe>`, probeUUID),
	}

	for _, probe := range probePackets {
		_, err = conn.WriteToUDP([]byte(probe), multicastAddr)
		if err != nil {
			s.log.Debugw("Failed to send probe", "ip", localIP.String(), "error", err)
		}
	}

	broadcastAddr := &net.UDPAddr{IP: net.IPv4bcast, Port: Port}
	for _, probe := range probePackets {
		_, _ = conn.WriteToUDP([]byte(probe), broadcastAddr)
	}

	_ = conn.SetReadDeadline(time.Now().Add(s.timeout))

	buf := make([]byte, MaxPacketSize)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}

		response := string(buf[:n])
		s.log.Debugw("Received response", "bytes", n, "from", remoteAddr.String())

		device := s.parseResponse(response)
		if device != nil {
			device.AdapterIP = localIP.String()
			device.ReceivedTime = time.Now()

			s.deviceMutex.Lock()
			if _, exists := s.devices[device.MAC]; !exists {
				s.devices[device.MAC] = device
				s.log.Debugw("Found device", "ip", device.IPv4Address, "mac", device.MAC, "type", device.DeviceType)
			}
			s.deviceMutex.Unlock()
		}
	}
}

func (s *Scanner) parseResponse(data string) *Device {
	if !strings.Contains(data, "<ProbeMatch") && !strings.Contains(data, "ProbeMatch>") {
		return nil
	}

	device := &Device{}
	err := xml.Unmarshal([]byte(data), device)
	if err != nil {
		s.log.Debugw("Failed to parse response", "error", err)
		return nil
	}

	device.MAC = strings.ToUpper(strings.ReplaceAll(device.MAC, "-", ":"))
	return device
}

// ToXML generates SADP-compatible XML output
func (s *Scanner) ToXML(devices []*Device) (string, error) {
	list := DeviceList{
		Version: "2.0",
		Devices: make([]Device, len(devices)),
	}

	for i, dev := range devices {
		list.Devices[i] = *dev
	}

	output, err := xml.MarshalIndent(list, "", "  ")
	if err != nil {
		return "", err
	}

	return xml.Header + string(output), nil
}

// ToCSV generates CSV output
func (s *Scanner) ToCSV(devices []*Device) string {
	var sb strings.Builder
	sb.WriteString("ID,DeviceType,Activated,IPv4Address,Port,HttpPort,SoftwareVersion,IPv4Gateway,SerialNumber,IPv4SubnetMask,MAC,ChannelNum,DSPVersion,BootTime,DHCP\n")

	for i, dev := range devices {
		channelNum := dev.AnalogChannelNum + dev.DigitalChannelNum
		sb.WriteString(fmt.Sprintf("%d,%s,%s,%s,%d,%d,%s,%s,%s,%s,%s,%d,%s,%s,%s\n",
			i+1,
			dev.DeviceType,
			dev.Activated,
			dev.IPv4Address,
			dev.CommandPort,
			dev.HttpPort,
			dev.SoftwareVersion,
			dev.IPv4Gateway,
			dev.DeviceSN,
			dev.IPv4SubnetMask,
			dev.MAC,
			channelNum,
			dev.DSPVersion,
			dev.BootTime,
			dev.DHCP,
		))
	}

	return sb.String()
}

// Truncate truncates a string to a maximum length
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
