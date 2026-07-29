package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/cameronnewman/hikvision-tooling/internal/config"
	"github.com/cameronnewman/hikvision-tooling/internal/isapi"
)

// ExitError carries a specific process exit code out to the CLI entry point.
// Handlers that need a non-1 exit (e.g. auth vs network) wrap the underlying
// error in this type; everything else just returns a plain error and maps to 1.
type ExitError struct {
	Code int
	Err  error
}

// Error returns the wrapped error's message.
func (e *ExitError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Unwrap exposes the underlying error for errors.Is / errors.As.
func (e *ExitError) Unwrap() error { return e.Err }

// isapiDeviceInfoFetcher is the subset of *isapi.Client the CLI uses; the
// interface exists so tests can inject a fake without opening a socket.
type isapiDeviceInfoFetcher interface {
	DeviceInfo(ctx context.Context) (*isapi.DeviceInfo, error)
}

// newISAPIClient constructs the real client in production; tests reassign it
// to capture the base URL and return a fake fetcher.
var newISAPIClient = func(baseURL string, opts isapi.Options) isapiDeviceInfoFetcher {
	return isapi.New(baseURL, opts)
}

// ISAPICmd dispatches `sadp isapi <subcommand>`.
func ISAPICmd(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printISAPIUsage()
		return nil
	}
	switch args[0] {
	case "info":
		return isapiInfoCmd(args[1:])
	default:
		printISAPIUsage()
		return fmt.Errorf("unknown isapi subcommand: %s", args[0])
	}
}

func printISAPIUsage() {
	out("Usage: sadp isapi <subcommand> [options]")
	out("")
	out("Subcommands:")
	out("  info <HOST>   Fetch /ISAPI/System/deviceInfo from a device")
	out("")
	out("Examples:")
	out("  sadp isapi info 192.168.1.64")
	out("  sadp isapi info https://192.168.1.64:8443 --insecure")
	out("  HIKVISION_USERNAME=admin HIKVISION_PASSWORD=secret sadp isapi info 192.168.1.64 --json")
}

func isapiInfoCmd(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fs := flag.NewFlagSet("isapi info", flag.ExitOnError)
	username := fs.String("username", cfg.HikvisionUsername, "ISAPI username (defaults to HIKVISION_USERNAME)")
	password := fs.String("password", cfg.HikvisionPassword, "ISAPI password (defaults to HIKVISION_PASSWORD)")
	insecure := fs.Bool("insecure", false, "Skip TLS certificate verification")
	timeout := fs.Duration("timeout", cfg.HTTPTimeout, "Request timeout")
	asJSON := fs.Bool("json", false, "Emit DeviceInfo as JSON")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		out("Usage: sadp isapi info [options] <HOST>")
		out("")
		out("Options:")
		fs.PrintDefaults()
		return nil
	}

	baseURL, err := normalizeISAPIHost(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("invalid host: %w", err)
	}

	if *username == "" || *password == "" {
		return &ExitError{
			Code: 2,
			Err:  errors.New("missing credentials: set HIKVISION_USERNAME and HIKVISION_PASSWORD (or --username/--password)"),
		}
	}

	client := newISAPIClient(baseURL, isapi.Options{
		Username:           *username,
		Password:           *password,
		Timeout:            *timeout,
		InsecureSkipVerify: *insecure,
	})

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	info, err := client.DeviceInfo(ctx)
	if err != nil {
		return mapISAPIError(err)
	}

	if *asJSON {
		return renderDeviceInfoJSON(info)
	}
	renderDeviceInfoPretty(info)
	return nil
}

// normalizeISAPIHost turns a bare host, an IP, or a full URL into a clean
// scheme://host[:port] base URL suitable for isapi.New.
func normalizeISAPIHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("host is empty")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
	if u.Host == "" {
		return "", errors.New("missing host")
	}
	return u.Scheme + "://" + u.Host, nil
}

// mapISAPIError classifies transport/status errors into an ExitError with a
// specific code so the CLI entry point can surface it to the shell.
func mapISAPIError(err error) error {
	switch {
	case errors.Is(err, isapi.ErrUnauthorized):
		return &ExitError{
			Code: 2,
			Err:  fmt.Errorf("authentication failed: check HIKVISION_USERNAME/HIKVISION_PASSWORD (%w)", err),
		}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return &ExitError{Code: 3, Err: fmt.Errorf("request timed out: %w", err)}
	case errors.Is(err, isapi.ErrNotFound):
		return fmt.Errorf("endpoint /ISAPI/System/deviceInfo not found on device: %w", err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &ExitError{Code: 3, Err: fmt.Errorf("network timeout: %w", err)}
	}
	return err
}

func renderDeviceInfoJSON(info *isapi.DeviceInfo) error {
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal device info: %w", err)
	}
	out(string(b))
	return nil
}

func renderDeviceInfoPretty(info *isapi.DeviceInfo) {
	rows := []struct {
		label string
		value string
	}{
		{"Device Name", info.DeviceName},
		{"Model", info.Model},
		{"Serial Number", info.SerialNumber},
		{"MAC Address", info.MacAddress},
		{"Firmware Version", info.FirmwareVersion},
		{"Firmware Date", info.FirmwareReleasedDate},
		{"Device Type", info.DeviceType},
		{"Description", info.DeviceDescription},
		{"Encoder Version", info.EncoderVersion},
		{"Device ID", info.DeviceID},
	}
	for _, r := range rows {
		if r.value == "" {
			continue
		}
		outf("%-17s %s\n", r.label+":", r.value)
	}
}
