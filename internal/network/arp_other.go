//go:build !darwin && !linux && !windows

package network

import "os/exec"

func defaultArpCommand() *exec.Cmd          { return nil }
func defaultPingCommand(_ string) *exec.Cmd { return nil }
