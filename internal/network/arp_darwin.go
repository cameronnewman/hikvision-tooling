//go:build darwin

package network

import "os/exec"

func defaultArpCommand() *exec.Cmd { return exec.Command("arp", "-an") }

func defaultPingCommand(ip string) *exec.Cmd {
	return exec.Command("ping", "-c", "1", "-W", "1", ip) // #nosec G204 -- ip validated by net.ParseIP in PingHost
}
