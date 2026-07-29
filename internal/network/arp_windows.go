//go:build windows

package network

import "os/exec"

func defaultArpCommand() *exec.Cmd { return exec.Command("arp", "-a") }

func defaultPingCommand(ip string) *exec.Cmd {
	return exec.Command("ping", "-n", "1", "-w", "1000", ip) // #nosec G204 -- ip validated by net.ParseIP in PingHost
}
