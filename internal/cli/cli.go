package cli

import (
	"flag"
	"fmt"
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
	fmt.Println("SADP - Hikvision Device Discovery Tool")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  sadp <command> [options]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  discover <CIDR>    Discover Hikvision devices via ARP (requires subnet)")
	fmt.Println("  discover:sadp      Discover devices via SADP protocol (multicast)")
	fmt.Println("  scan <CIDR>        Discover devices using both ARP and SADP")
	fmt.Println("  probe <IP>         Check device info and status")
	fmt.Println("  send <IP> <cmd>    Send SADP XML command to a device")
	fmt.Println("  reset              Generate password reset code (firmware < 5.3.0)")
	fmt.Println("")
	fmt.Println("Environment Variables:")
	fmt.Println("  DISCOVERY_WORKERS   Number of concurrent workers (default: 100)")
	fmt.Println("  DISCOVERY_TIMEOUT   Per-host timeout (default: 1s)")
	fmt.Println("  SADP_TIMEOUT        SADP protocol timeout (default: 5s)")
	fmt.Println("  DEBUG               Enable debug output (default: false)")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  sadp discover:sadp")
	fmt.Println("  sadp discover:sadp --xml --output devices.xml")
	fmt.Println("  sadp scan 192.168.1.0/24")
	fmt.Println("  sadp send 192.168.1.64 inquiry")
	fmt.Println("  sadp reset --serial ABC123 --date 20231215")
	fmt.Println("")
	fmt.Println("Run 'sadp <command> --help' for command options.")
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
		fmt.Println("Usage: sadp discover [options] <CIDR>")
		fmt.Println("\nExamples:")
		fmt.Println("  sadp discover 192.168.1.0/24")
		fmt.Println("  sadp discover 10.0.0.0/16")
		fmt.Println("\nOptions:")
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

	fmt.Printf("\nDiscovered %d Hikvision device(s):\n", len(devices))
	fmt.Println("---------------------------------------------------")
	for _, dev := range devices {
		fmt.Printf("  IP: %-15s  MAC: %s\n", dev.IP, dev.MAC)
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
				alive := network.IsHostAlive(ip, timeout)
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
	arpTable, err := network.GetARPTable()
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

	fmt.Println("Discovering Hikvision devices via SADP protocol...")
	fmt.Println("Sending multicast probes to 239.255.255.250:37020")

	log := logger.New(*debug)
	defer func() { _ = log.Sync() }()

	scanner := sadp.NewScanner(*timeout, log)
	devices, err := scanner.Discover()
	if err != nil {
		return err
	}

	fmt.Printf("\nDiscovered %d device(s)\n", len(devices))

	var output string
	if *xmlFormat {
		output, err = scanner.ToXML(devices)
		if err != nil {
			return fmt.Errorf("error generating XML: %w", err)
		}
	} else if *csvFormat {
		output = scanner.ToCSV(devices)
	} else {
		printDeviceTable(devices)
		if *outputFile != "" {
			output, _ = scanner.ToXML(devices)
		}
	}

	if *outputFile != "" && output != "" {
		err := os.WriteFile(*outputFile, []byte(output), 0644)
		if err != nil {
			return fmt.Errorf("error writing file: %w", err)
		}
		fmt.Printf("Output written to: %s\n", *outputFile)
	} else if output != "" && (*xmlFormat || *csvFormat) {
		fmt.Println(output)
	}

	return nil
}

func printDeviceTable(devices []*sadp.Device) {
	if len(devices) == 0 {
		fmt.Println("No devices found.")
		return
	}

	fmt.Println()
	fmt.Printf("%-3s %-15s %-17s %-20s %-8s %-6s %-15s %s\n",
		"#", "IPv4 Address", "MAC Address", "Device Type", "Status", "Port", "Serial Number", "Software Version")
	fmt.Println(strings.Repeat("-", 120))

	for i, dev := range devices {
		status := "Inactive"
		if dev.Activated == "true" {
			status = "Active"
		}

		fmt.Printf("%-3d %-15s %-17s %-20s %-8s %-6d %-15s %s\n",
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
	fmt.Println()
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

	reorderedArgs := reorderArgsForFlags(args)
	_ = fs.Parse(reorderedArgs)

	if *listCmds {
		printCommandList()
		return nil
	}

	if fs.NArg() < 1 {
		fmt.Println("Usage: sadp send <IP> <command> [options]")
		fmt.Println("       sadp send --list")
		fmt.Println("")
		fmt.Println("Commands: inquiry, inquiry_v32, exchangecode, getencryptstring,")
		fmt.Println("          activate, update, reboot, restore, setmailbox, ezvizunbind")
		fmt.Println("")
		fmt.Println("Options:")
		fs.PrintDefaults()
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  sadp send 192.168.1.64 inquiry")
		fmt.Println("  sadp send 192.168.1.64 exchangecode --mac 4C:BD:8F:61:CC:5C")
		fmt.Println("  sadp send 0.0.0.0 exchangecode --mac 4C:BD:8F:61:CC:5C  (uses broadcast)")
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

	scanner := sadp.NewScanner(*timeout, log)
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
	}

	fmt.Printf("Sending '%s' command to %s...\n", command, targetIP)
	if targetIP == "0.0.0.0" {
		fmt.Printf("Using broadcast mode (target MAC: %s)\n", macAddr)
	}

	response, err := scanner.SendCommand(command, opts)
	if err != nil {
		return err
	}

	fmt.Println("\nResponse:")
	fmt.Println("---")
	fmt.Println(response)
	fmt.Println("---")

	return nil
}

func printCommandList() {
	fmt.Println("Available SADP Commands:")
	fmt.Println()
	fmt.Printf("%-20s %-12s %-12s %s\n", "Command", "Needs MAC", "Needs Pass", "Description")
	fmt.Println(strings.Repeat("-", 80))

	for _, cmd := range sadp.ListCommands() {
		mac := "No"
		if cmd.NeedsMAC {
			mac = "Yes"
		}
		pass := "No"
		if cmd.NeedsPass {
			pass = "Yes"
		}
		fmt.Printf("%-20s %-12s %-12s %s\n", cmd.Name, mac, pass, cmd.Description)
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
				if flagName != "debug" && flagName != "dhcp" && flagName != "list" {
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
			fmt.Fprintf(os.Stderr, "Warning: Could not auto-fetch device info: %v\n", err)
			fmt.Println("Please provide --serial and --date manually")
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
		fmt.Println("Hikvision Password Reset Code Generator")
		fmt.Println("========================================")
		fmt.Println("")
		fmt.Println("Usage: sadp reset --serial <SERIAL> --date <YYYYMMDD>")
		fmt.Println("       sadp reset --ip <DEVICE_IP>")
		fmt.Println("")
		fmt.Println("Options:")
		fs.PrintDefaults()
		fmt.Println("")
		fmt.Println("IMPORTANT:")
		fmt.Println("  - Serial number is CASE-SENSITIVE")
		fmt.Println("  - Remove the model prefix from the serial number")
		fmt.Println("    Example: DS-7616NI-I20123456789 -> 0123456789")
		fmt.Println("  - Date must match the device's internal clock, NOT today's date")
		fmt.Println("  - Check the 'Start Time' or 'Boot Time' in SADP to find device date")
		fmt.Println("")
		fmt.Println("Note: This only works on firmware versions < 5.3.0")
		return nil
	}

	if len(*date) != 8 {
		return fmt.Errorf("date must be in YYYYMMDD format (got: %s)", *date)
	}

	resetCode := crypto.GenerateResetCode(*serial, *date)

	fmt.Println("Hikvision Password Reset Code Generator")
	fmt.Println("========================================")
	fmt.Println("")
	fmt.Printf("Serial Number: %s\n", *serial)
	fmt.Printf("Device Date:   %s\n", *date)
	fmt.Printf("Seed:          %s%s\n", *serial, *date)
	fmt.Println("")
	fmt.Println("----------------------------------------")
	fmt.Printf("RESET CODE:    %s\n", resetCode)
	fmt.Println("----------------------------------------")
	fmt.Println("")
	fmt.Println("Instructions:")
	fmt.Println("1. Open SADP Tool and select your device")
	fmt.Println("2. Click 'Forgot Password' or enter the security code field")
	fmt.Println("3. Enter the reset code above")
	fmt.Println("4. The admin password will be reset to '12345' or '123456789abc'")
	fmt.Println("")
	fmt.Println("Note: This only works on firmware < 5.3.0")

	return nil
}

func fetchDeviceInfo(cfg *config.Config, ipAddress string, debug bool) (serial, date string, err error) {
	httpClient := network.NewHTTPClient(cfg.UserAgent, cfg.HTTPTimeout)
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
		fmt.Println("Response from /upnpdevicedesc.xml:")
		fmt.Println(bodyStr[:maxLen])
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
		fmt.Printf("Extracted model: %s\n", model)
		fmt.Printf("Extracted serial: %s\n", serial)
		fmt.Printf("Using date: %s (verify this matches device clock!)\n", date)
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
		fmt.Println("Usage: sadp scan [options] <CIDR>")
		fmt.Println("\nThis command discovers Hikvision devices using both ARP and SADP protocols.")
		fmt.Println("\nExamples:")
		fmt.Println("  sadp scan 192.168.1.0/24")
		fmt.Println("  sadp scan --workers 50 10.0.0.0/24")
		fmt.Println("\nOptions:")
		fs.PrintDefaults()
		return nil
	}

	cidr := fs.Arg(0)
	log := logger.New(*debug)
	defer func() { _ = log.Sync() }()

	fmt.Printf("Scanning %s for Hikvision devices...\n", cidr)

	// ARP Discovery
	fmt.Println("\n[1/2] ARP Discovery...")
	ips, err := network.ExpandCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}

	arpDevices := discoverDevices(ips, *workers, *timeout, log)
	fmt.Printf("      Found %d device(s) via ARP\n", len(arpDevices))

	// SADP Discovery
	fmt.Println("\n[2/2] SADP Discovery...")
	scanner := sadp.NewScanner(cfg.SADPTimeout, log)
	sadpDevices, err := scanner.Discover()
	if err != nil {
		log.Warnw("SADP discovery failed", "error", err)
	}
	fmt.Printf("      Found %d device(s) via SADP\n", len(sadpDevices))

	// Merge results (deduplicate by MAC)
	deviceMap := make(map[string]interface{})
	for _, dev := range arpDevices {
		deviceMap[strings.ToUpper(dev.MAC)] = dev
	}
	for _, dev := range sadpDevices {
		deviceMap[strings.ToUpper(dev.MAC)] = dev
	}

	fmt.Println("\n===================================================")
	fmt.Println("                   SCAN RESULTS                    ")
	fmt.Println("===================================================")
	fmt.Printf("Total unique devices: %d\n\n", len(deviceMap))

	// Print ARP results
	if len(arpDevices) > 0 {
		fmt.Println("Devices found via ARP:")
		fmt.Println("---------------------------------------------------")
		for _, dev := range arpDevices {
			fmt.Printf("  IP: %-15s  MAC: %s\n", dev.IP, dev.MAC)
		}
		fmt.Println()
	}

	// Print SADP results
	if len(sadpDevices) > 0 {
		fmt.Println("Devices found via SADP:")
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
		fmt.Println("Usage: sadp probe <IP_ADDRESS>")
		fmt.Println("\nProbes a Hikvision device to check its status and information.")
		return nil
	}

	ipAddress := fs.Arg(0)
	httpClient := network.NewHTTPClient(cfg.UserAgent, cfg.HTTPTimeout)

	fmt.Printf("Probing device at %s...\n\n", ipAddress)

	// Check common endpoints
	endpoints := []struct {
		path        string
		description string
	}{
		{"/System/deviceInfo", "Device Info (ISAPI)"},
		{"/ISAPI/System/deviceInfo", "Device Info (ISAPI v2)"},
		{"/", "Web Interface"},
	}

	fmt.Println("Checking endpoints:")
	fmt.Println("---------------------------------------------------")

	for _, ep := range endpoints {
		resp, err := httpClient.Get(ipAddress, ep.path)
		if err != nil {
			fmt.Printf("  %-25s ERROR: %v\n", ep.description, err)
			continue
		}
		fmt.Printf("  %-25s HTTP %d", ep.description, resp.StatusCode)

		if resp.StatusCode == 200 && len(resp.Body) > 0 {
			bodyStr := string(resp.Body)
			if firmware := extractFirmwareVersion(bodyStr); firmware != "" {
				fmt.Printf(" (Firmware: %s)", firmware)
			}
			if model := extractModel(bodyStr); model != "" {
				fmt.Printf(" (Model: %s)", model)
			}
		}
		fmt.Println()
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
