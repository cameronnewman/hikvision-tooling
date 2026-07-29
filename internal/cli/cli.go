// Package cli implements the sadp command-line interface.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/cameronnewman/hikvision-tooling/internal/config"
	"github.com/cameronnewman/hikvision-tooling/internal/crypto"
	"github.com/cameronnewman/hikvision-tooling/internal/logger"
	"github.com/cameronnewman/hikvision-tooling/internal/network"
	"github.com/cameronnewman/hikvision-tooling/internal/sadp"
)

// sadpScanner is the subset of *sadp.Scanner used by the CLI; extracted so
// tests can substitute an in-memory fake.
type sadpScanner interface {
	Discover() ([]*sadp.Device, error)
	SendCommand(cmdName string, opts sadp.SendOptions) (string, error)
	ToXML(devices []*sadp.Device) (string, error)
	ToCSV(devices []*sadp.Device) string
}

// httpGetter is the subset of *network.HTTPClient used by the CLI; extracted
// so tests can substitute an in-memory fake.
type httpGetter interface {
	Get(ipAddress, path string) (*network.HTTPResponse, error)
}

// Swappable indirections; production defaults wire to the real
// implementations. Tests reassign these to exercise handlers without
// touching real network state.
var (
	stdout         io.Writer = os.Stdout
	stderr         io.Writer = os.Stderr
	newSADPScanner           = func(t time.Duration, l *logger.Logger) sadpScanner {
		return sadp.NewScanner(t, l)
	}
	newHTTPClient = func(ua string, t time.Duration) httpGetter {
		return network.NewHTTPClient(ua, t)
	}
	getARPTable = network.GetARPTable
	isHostAlive = network.IsHostAlive
)

func out(a ...any) {
	_, _ = fmt.Fprintln(stdout, a...)
}

func outf(format string, a ...any) {
	_, _ = fmt.Fprintf(stdout, format, a...)
}

func errf(format string, a ...any) {
	_, _ = fmt.Fprintf(stderr, format, a...)
}

// Run executes the CLI with the given arguments
func Run(args []string) error {
	if len(args) < 1 {
		PrintUsage()
		return nil
	}

	switch args[0] {
	case "discover":
		return DiscoverCmd(args[1:])
	case "discover:sadp":
		return DiscoverSADPCmd(args[1:])
	case "scan":
		return ScanCmd(args[1:])
	case "probe":
		return ProbeCmd(args[1:])
	case "send":
		return SendCmd(args[1:])
	case "reset":
		return ResetCmd(args[1:])
	case "isapi":
		return ISAPICmd(args[1:])
	case "help", "--help", "-h":
		PrintUsage()
		return nil
	default:
		PrintUsage()
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

// PrintUsage prints the CLI usage information
func PrintUsage() {
	out("SADP - Hikvision Device Discovery Tool")
	out("")
	out("Usage:")
	out("  sadp <command> [options]")
	out("")
	out("Commands:")
	out("  discover <CIDR>    Discover Hikvision devices via ARP (requires subnet)")
	out("  discover:sadp      Discover devices via SADP protocol (multicast)")
	out("  scan <CIDR>        Discover devices using both ARP and SADP")
	out("  probe <IP>         Check device info and status")
	out("  send <IP> <cmd>    Send SADP XML command to a device (multicast/broadcast by default)")
	out("  reset              Generate password reset code (firmware < 5.3.0)")
	out("  isapi info <HOST>  Fetch device info via ISAPI (requires HIKVISION_USERNAME/PASSWORD)")
	out("")
	out("Environment Variables:")
	out("  DISCOVERY_WORKERS   Number of concurrent workers (default: 100)")
	out("  DISCOVERY_TIMEOUT   Per-host timeout (default: 1s)")
	out("  SADP_TIMEOUT        SADP protocol timeout (default: 5s)")
	out("  HIKVISION_USERNAME  ISAPI digest-auth username (default: empty)")
	out("  HIKVISION_PASSWORD  ISAPI digest-auth password (default: empty)")
	out("  DEBUG               Enable debug output (default: false)")
	out("")
	out("Examples:")
	out("  sadp discover:sadp")
	out("  sadp discover:sadp --xml --output devices.xml")
	out("  sadp scan 192.168.1.0/24")
	out("  sadp send 192.168.1.64 inquiry")
	out("  sadp reset --serial ABC123 --date 20231215")
	out("  sadp isapi info 192.168.1.64")
	out("")
	out("Run 'sadp <command> --help' for command options.")
}

// DiscoverCmd handles the discover command
func DiscoverCmd(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	workers := fs.Int("workers", cfg.DiscoveryWorkers, "Number of concurrent workers for scanning")
	timeout := fs.Duration("timeout", cfg.DiscoveryTimeout, "Timeout for each host probe")
	debug := fs.Bool("debug", cfg.Debug, "Enable debug output")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		out("Usage: sadp discover [options] <CIDR>")
		out("\nExamples:")
		out("  sadp discover 192.168.1.0/24")
		out("  sadp discover 10.0.0.0/16")
		out("\nOptions:")
		fs.PrintDefaults()
		return nil
	}

	cidr := fs.Arg(0)
	log := logger.New(*debug)
	defer func() { _ = log.Sync() }()

	ips, err := network.ExpandCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}

	log.Infow("Scanning IP addresses", "count", len(ips), "workers", *workers)

	devices := discoverDevices(ips, *workers, *timeout, log)

	outf("\nDiscovered %d Hikvision device(s):\n", len(devices))
	out("---------------------------------------------------")
	for _, dev := range devices {
		outf("  IP: %-15s  MAC: %s\n", dev.IP, dev.MAC)
	}

	return nil
}

type discoveredDevice struct {
	IP  string
	MAC string
}

func discoverDevices(ips []string, workers int, timeout time.Duration, log *logger.Logger) []discoveredDevice {
	type result struct {
		ip    string
		alive bool
	}

	// Channel for work distribution
	ipChan := make(chan string, len(ips))
	resultChan := make(chan result, len(ips))

	// Start workers
	for i := 0; i < workers; i++ {
		go func() {
			for ip := range ipChan {
				alive := isHostAlive(ip, timeout)
				resultChan <- result{ip: ip, alive: alive}
			}
		}()
	}

	// Send work
	for _, ip := range ips {
		ipChan <- ip
	}
	close(ipChan)

	// Collect results
	aliveHosts := make([]string, 0)
	for i := 0; i < len(ips); i++ {
		r := <-resultChan
		if r.alive {
			aliveHosts = append(aliveHosts, r.ip)
			log.Debugw("Host alive", "ip", r.ip)
		}
	}

	// Get ARP table
	arpTable, err := getARPTable()
	if err != nil {
		log.Warnw("Failed to read ARP table", "error", err)
		return nil
	}

	// Filter Hikvision devices
	var devices []discoveredDevice
	for _, ip := range aliveHosts {
		if mac, ok := arpTable[ip]; ok {
			if network.IsHikvisionMAC(mac) {
				devices = append(devices, discoveredDevice{IP: ip, MAC: mac})
			}
		}
	}

	return devices
}

// DiscoverSADPCmd handles the discover:sadp command
func DiscoverSADPCmd(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fs := flag.NewFlagSet("discover:sadp", flag.ExitOnError)
	timeout := fs.Duration("timeout", cfg.SADPTimeout, "Discovery timeout")
	outputFile := fs.String("output", "", "Output file path (default: stdout)")
	xmlFormat := fs.Bool("xml", false, "Output in XML format (SADP compatible)")
	csvFormat := fs.Bool("csv", false, "Output in CSV format")
	debug := fs.Bool("debug", cfg.Debug, "Enable debug output")
	_ = fs.Parse(args)

	out("Discovering Hikvision devices via SADP protocol...")
	out("Sending multicast probes to 239.255.255.250:37020")

	log := logger.New(*debug)
	defer func() { _ = log.Sync() }()

	scanner := newSADPScanner(*timeout, log)
	devices, err := scanner.Discover()
	if err != nil {
		return err
	}

	outf("\nDiscovered %d device(s)\n", len(devices))

	var output string
	switch {
	case *xmlFormat:
		output, err = scanner.ToXML(devices)
		if err != nil {
			return fmt.Errorf("error generating XML: %w", err)
		}
	case *csvFormat:
		output = scanner.ToCSV(devices)
	default:
		printDeviceTable(devices)
		if *outputFile != "" {
			output, _ = scanner.ToXML(devices)
		}
	}

	if *outputFile != "" && output != "" {
		err := os.WriteFile(*outputFile, []byte(output), 0600)
		if err != nil {
			return fmt.Errorf("error writing file: %w", err)
		}
		outf("Output written to: %s\n", *outputFile)
	} else if output != "" && (*xmlFormat || *csvFormat) {
		out(output)
	}

	return nil
}

func printDeviceTable(devices []*sadp.Device) {
	if len(devices) == 0 {
		out("No devices found.")
		return
	}

	out()
	outf("%-3s %-15s %-17s %-20s %-8s %-6s %-15s %s\n",
		"#", "IPv4 Address", "MAC Address", "Device Type", "Status", "Port", "Serial Number", "Software Version")
	out(strings.Repeat("-", 120))

	for i, dev := range devices {
		status := "Inactive"
		if dev.Activated == "true" {
			status = "Active"
		}

		outf("%-3d %-15s %-17s %-20s %-8s %-6d %-15s %s\n",
			i+1,
			dev.IPv4Address,
			dev.MAC,
			sadp.Truncate(dev.DeviceType, 20),
			status,
			dev.CommandPort,
			sadp.Truncate(dev.DeviceSN, 15),
			dev.SoftwareVersion,
		)
	}
	out()
}

// SendCmd handles the send command
func SendCmd(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fs := flag.NewFlagSet("send", flag.ExitOnError)
	mac := fs.String("mac", "", "Target device MAC address (required for most commands)")
	password := fs.String("password", "", "Device password")
	code := fs.String("code", "", "Security/reset code")
	newIP := fs.String("ip", "", "New IP address (for update command)")
	newMask := fs.String("mask", "255.255.255.0", "New subnet mask (for update command)")
	newGateway := fs.String("gateway", "", "New gateway (for update command)")
	newPort := fs.Int("port", 8000, "New SDK port (for update command)")
	dhcp := fs.Bool("dhcp", false, "Enable DHCP (for update command)")
	email := fs.String("email", "", "Email address (for setmailbox command)")
	timeout := fs.Duration("timeout", cfg.SADPTimeout, "Command timeout")
	debug := fs.Bool("debug", cfg.Debug, "Enable debug output")
	listCmds := fs.Bool("list", false, "List available commands")
	unicast := fs.Bool("unicast", false, "Force direct UDP send to <IP>:37020 (legacy; most devices ignore unicast SADP)")

	reorderedArgs := reorderArgsForFlags(args)
	_ = fs.Parse(reorderedArgs)

	if *listCmds {
		printCommandList()
		return nil
	}

	if fs.NArg() < 1 {
		out("Usage: sadp send <IP> <command> [options]")
		out("       sadp send --list")
		out("")
		out("Commands: inquiry, inquiry_v32, exchangecode, getencryptstring,")
		out("          activate, update, reboot, restore, setmailbox, ezvizunbind")
		out("")
		out("Transport:")
		out("  By default the request is sent to the SADP multicast group")
		out("  (239.255.255.250:37020) and to broadcast on every IPv4 interface,")
		out("  and the reply is correlated back to the target MAC or IP. This is")
		out("  how SADPTool works and is required because Hikvision devices do")
		out("  not open UDP/37020 on their unicast IP.")
		out("")
		out("  Pass 0.0.0.0 as <IP> when the device's address is unknown; a MAC")
		out("  is then required. Use --unicast only to force the legacy direct")
		out("  send (rarely works, kept as an escape hatch).")
		out("")
		out("Options:")
		fs.PrintDefaults()
		out("")
		out("Examples:")
		out("  sadp send 192.168.1.64 inquiry")
		out("  sadp send 192.168.1.64 exchangecode --mac 4C:BD:8F:61:CC:5C")
		out("  sadp send 0.0.0.0 exchangecode --mac 4C:BD:8F:61:CC:5C")
		out("  sadp send 192.168.1.64 inquiry --unicast    # legacy direct send")
		return nil
	}

	targetIP := fs.Arg(0)
	command := "inquiry"
	if fs.NArg() >= 2 {
		command = fs.Arg(1)
	}

	macAddr := strings.ToUpper(strings.ReplaceAll(*mac, "-", ":"))

	log := logger.New(*debug)
	defer func() { _ = log.Sync() }()

	scanner := newSADPScanner(*timeout, log)
	opts := sadp.SendOptions{
		TargetIP:   targetIP,
		TargetMAC:  macAddr,
		Password:   *password,
		Code:       *code,
		NewIP:      *newIP,
		NewMask:    *newMask,
		NewGateway: *newGateway,
		NewPort:    *newPort,
		DHCP:       *dhcp,
		Email:      *email,
		Timeout:    *timeout,
		Unicast:    *unicast,
	}

	outf("Sending '%s' command to %s...\n", command, targetIP)
	switch {
	case *unicast:
		out("Using unicast mode (direct UDP to target)")
	case targetIP == "0.0.0.0":
		outf("Using multicast/broadcast (target MAC: %s)\n", macAddr)
	default:
		out("Using multicast/broadcast; reply correlated by MAC/IP")
	}

	response, err := scanner.SendCommand(command, opts)
	if err != nil {
		return err
	}

	out("\nResponse:")
	out("---")
	out(response)
	out("---")

	return nil
}

func printCommandList() {
	out("Available SADP Commands:")
	out()
	outf("%-20s %-12s %-12s %s\n", "Command", "Needs MAC", "Needs Pass", "Description")
	out(strings.Repeat("-", 80))

	for _, cmd := range sadp.ListCommands() {
		mac := "No"
		if cmd.NeedsMAC {
			mac = "Yes"
		}
		pass := "No"
		if cmd.NeedsPass {
			pass = "Yes"
		}
		outf("%-20s %-12s %-12s %s\n", cmd.Name, mac, pass, cmd.Description)
	}
}

func reorderArgsForFlags(args []string) []string {
	var flags []string
	var positional []string

	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flagName := strings.TrimLeft(arg, "-")
				if flagName != "debug" && flagName != "dhcp" && flagName != "list" && flagName != "unicast" {
					i++
					flags = append(flags, args[i])
				}
			}
		} else {
			positional = append(positional, arg)
		}
		i++
	}

	return append(flags, positional...)
}

// ResetCmd handles the reset command
func ResetCmd(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fs := flag.NewFlagSet("reset", flag.ExitOnError)
	serial := fs.String("serial", "", "Device serial number (case-sensitive, without model prefix)")
	date := fs.String("date", "", "Device date in YYYYMMDD format (from device's internal clock)")
	ip := fs.String("ip", "", "Device IP to auto-fetch serial and date")
	debug := fs.Bool("debug", cfg.Debug, "Enable debug output")

	reorderedArgs := reorderArgsForFlags(args)
	_ = fs.Parse(reorderedArgs)

	if *ip != "" {
		fetchedSerial, fetchedDate, err := fetchDeviceInfo(cfg, *ip, *debug)
		if err != nil {
			errf("Warning: Could not auto-fetch device info: %v\n", err)
			out("Please provide --serial and --date manually")
		} else {
			if *serial == "" {
				*serial = fetchedSerial
			}
			if *date == "" {
				*date = fetchedDate
			}
		}
	}

	if *serial == "" || *date == "" {
		out("Hikvision Password Reset Code Generator")
		out("========================================")
		out("")
		out("Usage: sadp reset --serial <SERIAL> --date <YYYYMMDD>")
		out("       sadp reset --ip <DEVICE_IP>")
		out("")
		out("Options:")
		fs.PrintDefaults()
		out("")
		out("IMPORTANT:")
		out("  - Serial number is CASE-SENSITIVE")
		out("  - Remove the model prefix from the serial number")
		out("    Example: DS-7616NI-I20123456789 -> 0123456789")
		out("  - Date must match the device's internal clock, NOT today's date")
		out("  - Check the 'Start Time' or 'Boot Time' in SADP to find device date")
		out("")
		out("Note: This only works on firmware versions < 5.3.0")
		return nil
	}

	if len(*date) != 8 {
		return fmt.Errorf("date must be in YYYYMMDD format (got: %s)", *date)
	}

	resetCode := crypto.GenerateResetCode(*serial, *date)

	out("Hikvision Password Reset Code Generator")
	out("========================================")
	out("")
	outf("Serial Number: %s\n", *serial)
	outf("Device Date:   %s\n", *date)
	outf("Seed:          %s%s\n", *serial, *date)
	out("")
	out("----------------------------------------")
	outf("RESET CODE:    %s\n", resetCode)
	out("----------------------------------------")
	out("")
	out("Instructions:")
	out("1. Open SADP Tool and select your device")
	out("2. Click 'Forgot Password' or enter the security code field")
	out("3. Enter the reset code above")
	out("4. The admin password will be reset to '12345' or '123456789abc'")
	out("")
	out("Note: This only works on firmware < 5.3.0")

	return nil
}

func fetchDeviceInfo(cfg *config.Config, ipAddress string, debug bool) (serial, date string, err error) {
	httpClient := newHTTPClient(cfg.UserAgent, cfg.HTTPTimeout)
	resp, err := httpClient.Get(ipAddress, "/upnpdevicedesc.xml")
	if err != nil {
		return "", "", fmt.Errorf("failed to connect: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("HTTP %d response", resp.StatusCode)
	}

	bodyStr := string(resp.Body)

	if debug {
		maxLen := 500
		if len(bodyStr) < maxLen {
			maxLen = len(bodyStr)
		}
		out("Response from /upnpdevicedesc.xml:")
		out(bodyStr[:maxLen])
	}

	modelPattern := regexp.MustCompile(`<modelNumber>([^<]+)</modelNumber>`)
	modelMatch := modelPattern.FindStringSubmatch(bodyStr)
	model := ""
	if len(modelMatch) > 1 {
		model = modelMatch[1]
	}

	serialPattern := regexp.MustCompile(`<serialNumber>([^<]+)</serialNumber>`)
	serialMatch := serialPattern.FindStringSubmatch(bodyStr)
	if len(serialMatch) < 2 {
		return "", "", fmt.Errorf("could not find serial number in response")
	}
	serial = serialMatch[1]

	if model != "" && strings.HasPrefix(serial, model) {
		serial = strings.TrimPrefix(serial, model)
	}

	date = time.Now().Format("20060102")

	if debug {
		outf("Extracted model: %s\n", model)
		outf("Extracted serial: %s\n", serial)
		outf("Using date: %s (verify this matches device clock!)\n", date)
	}

	return serial, date, nil
}

// ScanCmd handles the scan command - discovers devices using both ARP and SADP
func ScanCmd(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	workers := fs.Int("workers", cfg.DiscoveryWorkers, "Number of concurrent workers for scanning")
	timeout := fs.Duration("timeout", cfg.DiscoveryTimeout, "Timeout for each host probe")
	debug := fs.Bool("debug", cfg.Debug, "Enable debug output")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		out("Usage: sadp scan [options] <CIDR>")
		out("\nThis command discovers Hikvision devices using both ARP and SADP protocols.")
		out("\nExamples:")
		out("  sadp scan 192.168.1.0/24")
		out("  sadp scan --workers 50 10.0.0.0/24")
		out("\nOptions:")
		fs.PrintDefaults()
		return nil
	}

	cidr := fs.Arg(0)
	log := logger.New(*debug)
	defer func() { _ = log.Sync() }()

	outf("Scanning %s for Hikvision devices...\n", cidr)

	// ARP Discovery
	out("\n[1/2] ARP Discovery...")
	ips, err := network.ExpandCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}

	arpDevices := discoverDevices(ips, *workers, *timeout, log)
	outf("      Found %d device(s) via ARP\n", len(arpDevices))

	// SADP Discovery
	out("\n[2/2] SADP Discovery...")
	scanner := newSADPScanner(cfg.SADPTimeout, log)
	sadpDevices, err := scanner.Discover()
	if err != nil {
		log.Warnw("SADP discovery failed", "error", err)
	}
	outf("      Found %d device(s) via SADP\n", len(sadpDevices))

	// Merge results (deduplicate by MAC)
	deviceMap := make(map[string]interface{})
	for _, dev := range arpDevices {
		deviceMap[strings.ToUpper(dev.MAC)] = dev
	}
	for _, dev := range sadpDevices {
		deviceMap[strings.ToUpper(dev.MAC)] = dev
	}

	out("\n===================================================")
	out("                   SCAN RESULTS                    ")
	out("===================================================")
	outf("Total unique devices: %d\n\n", len(deviceMap))

	// Print ARP results
	if len(arpDevices) > 0 {
		out("Devices found via ARP:")
		out("---------------------------------------------------")
		for _, dev := range arpDevices {
			outf("  IP: %-15s  MAC: %s\n", dev.IP, dev.MAC)
		}
		out()
	}

	// Print SADP results
	if len(sadpDevices) > 0 {
		out("Devices found via SADP:")
		printDeviceTable(sadpDevices)
	}

	return nil
}

// ProbeCmd handles the probe command - checks device info
func ProbeCmd(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		out("Usage: sadp probe <IP_ADDRESS>")
		out("\nProbes a Hikvision device to check its status and information.")
		return nil
	}

	ipAddress := fs.Arg(0)
	httpClient := newHTTPClient(cfg.UserAgent, cfg.HTTPTimeout)

	outf("Probing device at %s...\n\n", ipAddress)

	// Check common endpoints
	endpoints := []struct {
		path        string
		description string
	}{
		{"/System/deviceInfo", "Device Info (ISAPI)"},
		{"/ISAPI/System/deviceInfo", "Device Info (ISAPI v2)"},
		{"/", "Web Interface"},
	}

	out("Checking endpoints:")
	out("---------------------------------------------------")

	for _, ep := range endpoints {
		resp, err := httpClient.Get(ipAddress, ep.path)
		if err != nil {
			outf("  %-25s ERROR: %v\n", ep.description, err)
			continue
		}
		outf("  %-25s HTTP %d", ep.description, resp.StatusCode)

		if resp.StatusCode == 200 && len(resp.Body) > 0 {
			bodyStr := string(resp.Body)
			if firmware := extractFirmwareVersion(bodyStr); firmware != "" {
				outf(" (Firmware: %s)", firmware)
			}
			if model := extractModel(bodyStr); model != "" {
				outf(" (Model: %s)", model)
			}
		}
		out()
	}

	return nil
}

func extractFirmwareVersion(body string) string {
	patterns := []string{
		`<firmwareVersion>([^<]+)</firmwareVersion>`,
		`<version>([^<]+)</version>`,
		`"firmwareVersion"\s*:\s*"([^"]+)"`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(body); len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}

func extractModel(body string) string {
	patterns := []string{
		`<deviceName>([^<]+)</deviceName>`,
		`<model>([^<]+)</model>`,
		`"model"\s*:\s*"([^"]+)"`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(body); len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}
