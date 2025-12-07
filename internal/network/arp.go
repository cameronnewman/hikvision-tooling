package network

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ARPTable maps IP addresses to MAC addresses
type ARPTable map[string]string

// HikvisionOUIs are the MAC address prefixes assigned to Hikvision
var HikvisionOUIs = []string{
	"00:0d:c5",
	"28:57:be",
	"44:19:b6",
	"54:c4:15",
	"80:cc:9c",
	"a4:14:37",
	"bc:ad:28",
	"c0:56:e3",
	"c4:2f:90",
	"e0:2f:6d",
	"f4:52:14",
	"48:40:a9",
	"8c:e7:48",
	"4c:bd:8f",
	"18:68:cb",
	"44:47:cc",
	"e4:24:6c",
}

// GetARPTable reads the system ARP table
func GetARPTable() (ARPTable, error) {
	return getARPTableOS()
}

func getARPTableOS() (ARPTable, error) {
	arpTable := make(ARPTable)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("arp", "-an")
	case "linux":
		cmd = exec.Command("arp", "-n")
	case "windows":
		cmd = exec.Command("arp", "-a")
	default:
		return arpTable, nil
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		ip, mac := ParseARPLine(line)
		if ip != "" && mac != "" {
			arpTable[ip] = mac
		}
	}

	return arpTable, nil
}

// ParseARPLine parses a single line from ARP output
func ParseARPLine(line string) (ip, mac string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", ""
	}

	// macOS pattern: ? (192.168.1.1) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]
	if strings.Contains(line, ") at ") {
		start := strings.Index(line, "(")
		end := strings.Index(line, ")")
		if start != -1 && end != -1 && end > start {
			ip = line[start+1 : end]
		}
		atIdx := strings.Index(line, ") at ")
		if atIdx != -1 {
			rest := line[atIdx+5:]
			parts := strings.Fields(rest)
			if len(parts) > 0 && strings.Contains(parts[0], ":") {
				mac = strings.ToLower(parts[0])
			}
		}
		return ip, mac
	}

	// Linux/Windows pattern
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		if net.ParseIP(fields[0]) != nil {
			ip = fields[0]
			for _, f := range fields[1:] {
				f = strings.ReplaceAll(f, "-", ":")
				if IsValidMAC(f) {
					mac = strings.ToLower(f)
					break
				}
			}
		}
	}

	return ip, mac
}

// IsValidMAC checks if a string is a valid MAC address
func IsValidMAC(s string) bool {
	s = strings.ReplaceAll(s, "-", ":")
	parts := strings.Split(s, ":")
	if len(parts) != 6 {
		return false
	}
	for _, p := range parts {
		if len(p) != 2 {
			return false
		}
		if _, err := hex.DecodeString(p); err != nil {
			return false
		}
	}
	return true
}

// IsHikvisionMAC checks if a MAC address belongs to Hikvision
func IsHikvisionMAC(mac string) bool {
	mac = strings.ToLower(mac)
	mac = strings.ReplaceAll(mac, "-", ":")

	parts := strings.Split(mac, ":")
	if len(parts) < 3 {
		return false
	}
	oui := strings.Join(parts[:3], ":")

	for _, hikOUI := range HikvisionOUIs {
		if oui == hikOUI {
			return true
		}
	}
	return false
}

// IsHostAlive checks if a host is reachable
func IsHostAlive(ip string, timeout time.Duration) bool {
	ports := []string{"80", "443", "8000", "8080", "554"}

	for _, port := range ports {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, port), timeout)
		if err == nil {
			conn.Close()
			return true
		}
	}

	return PingHost(ip, timeout)
}

// PingHost attempts to ping a host
func PingHost(ip string, timeout time.Duration) bool {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin", "linux":
		cmd = exec.Command("ping", "-c", "1", "-W", "1", ip)
	case "windows":
		cmd = exec.Command("ping", "-n", "1", "-w", "1000", ip)
	default:
		return false
	}

	// Start the command first to avoid race condition
	if err := cmd.Start(); err != nil {
		return false
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err == nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return false
	}
}

// ExpandCIDR expands a CIDR range to a list of IP addresses
func ExpandCIDR(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		if net.ParseIP(cidr) != nil {
			return []string{cidr}, nil
		}
		return nil, err
	}

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incrementIP(ip) {
		ones, bits := ipnet.Mask.Size()
		if bits-ones <= 8 {
			lastOctet := ip[len(ip)-1]
			if lastOctet == 0 || lastOctet == 255 {
				continue
			}
		}
		ipCopy := make(net.IP, len(ip))
		copy(ipCopy, ip)
		ips = append(ips, ipCopy.String())
	}

	return ips, nil
}

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
