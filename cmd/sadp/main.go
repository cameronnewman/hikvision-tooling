// Command sadp is the CLI entry point for the Hikvision SADP tooling.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/cameronnewman/hikvision-tooling/internal/cli"
)

func main() {
	os.Exit(mainRun(os.Args[1:], os.Stderr))
}

func mainRun(args []string, stderr io.Writer) int {
	if err := cli.Run(args); err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) && exitErr.Code != 0 {
			return exitErr.Code
		}
		return 1
	}
	return 0
}
