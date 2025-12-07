# SADP - Hikvision Device Discovery Tool

[![CI][ci-badge]][ci-url]

[ci-badge]: https://github.com/cameronnewman/hikvision-tooling/actions/workflows/ci.yml/badge.svg
[ci-url]: https://github.com/cameronnewman/hikvision-tooling/actions/workflows/ci.yml

A cross-platform command-line tool for discovering and managing Hikvision
devices on your network using the SADP (Search Active Devices Protocol)
and ARP-based discovery methods.

## Why This Tool

Hikvision provides an official SADP Tool (SADPTool.exe) for device
discovery and configuration, but it **only runs on Windows**. You can
download it from the [Hikvision SADP Tool page][sadp-tool].

[sadp-tool]: https://www.hikvision.com/en/support/tools/desktop-tools/sadp-for-windows/

This project provides a **cross-platform alternative** that runs natively on:

- **macOS** (Intel and Apple Silicon)
- **Linux** (amd64 and arm64)
- **Windows** (amd64)

## Features

- **SADP Discovery**: Discover Hikvision devices using the official SADP
  multicast protocol
- **ARP Discovery**: Find devices by scanning IP ranges and checking MAC
  addresses
- **Combined Scanning**: Use both methods for comprehensive network discovery
- **Device Probing**: Check device status and information
- **SADP Commands**: Send SADP protocol commands to devices
- **Password Reset Code Generation**: Generate reset codes for devices with
  firmware < 5.3.0

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/cameronnewman/hikvision-tooling.git
cd hikvision-tooling

# Build
make build

# Or install to your GOBIN
make install
```

### Pre-built Binaries

Download pre-built binaries from the [Releases][releases] page.

[releases]: https://github.com/cameronnewman/hikvision-tooling/releases

## Usage

```bash
sadp <command> [options]
```

### Commands

#### `discover:sadp` - SADP Protocol Discovery

Discover devices using SADP multicast (239.255.255.250:37020):

```bash
# Basic discovery
sadp discover:sadp

# With XML output
sadp discover:sadp --xml --output devices.xml

# With CSV output
sadp discover:sadp --csv
```

#### `discover` - ARP-based Discovery

Discover devices by scanning an IP range:

```bash
sadp discover 192.168.1.0/24
sadp discover --workers 50 10.0.0.0/24
```

#### `scan` - Combined Discovery

Use both ARP and SADP for comprehensive scanning:

```bash
sadp scan 192.168.1.0/24
```

#### `probe` - Device Information

Check device status and information:

```bash
sadp probe 192.168.1.64
```

#### `send` - SADP Commands

Send SADP protocol commands to devices:

```bash
# List available commands
sadp send --list

# Send inquiry command
sadp send 192.168.1.64 inquiry

# Exchange code with a specific device
sadp send 192.168.1.64 exchangecode --mac 4C:BD:8F:61:CC:5C

# Broadcast mode
sadp send 0.0.0.0 exchangecode --mac 4C:BD:8F:61:CC:5C
```

#### `reset` - Password Reset Code Generator

Generate password reset codes for devices with firmware < 5.3.0:

```bash
# Manual input
sadp reset --serial 0123456789 --date 20231215

# Auto-fetch from device
sadp reset --ip 192.168.1.64
```

**Important Notes:**

- Serial number is case-sensitive
- Remove the model prefix from the serial number
  (e.g., DS-7616NI-I20123456789 → 0123456789)
- Date must match the device's internal clock, not today's date
- Only works on firmware versions < 5.3.0

## Configuration

Configure the tool using environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DISCOVERY_WORKERS` | 100 | Number of concurrent workers |
| `DISCOVERY_TIMEOUT` | 1s | Per-host timeout for discovery |
| `SADP_TIMEOUT` | 5s | SADP protocol timeout |
| `HTTP_TIMEOUT` | 10s | HTTP request timeout |
| `DEBUG` | false | Enable debug output |

Example:

```bash
DISCOVERY_WORKERS=50 SADP_TIMEOUT=10s sadp scan 192.168.1.0/24
```

## Development

### Prerequisites

- Go 1.21 or later
- golangci-lint (for linting)

### Building

```bash
# Build for current platform
make build

# Build for all platforms
make release

# Run tests
make test

# Run tests with coverage
make test-cover

# Run linters
make lint

# Run all checks
make check
```

### Project Structure

```text
.
├── cmd/
│   └── sadp/           # CLI entry point
├── internal/
│   ├── cli/            # CLI commands and logic
│   ├── config/         # Environment-based configuration
│   ├── crypto/         # Password reset code generation
│   ├── logger/         # Structured logging (zap)
│   ├── network/        # HTTP client, ARP table, CIDR utilities
│   └── sadp/           # SADP protocol implementation
├── Makefile
└── README.md
```

## Protocol Information

### SADP Protocol

SADP (Search Active Devices Protocol) is Hikvision's device discovery protocol:

- **Multicast Address**: 239.255.255.250
- **Port**: 37020
- **Transport**: UDP
- **Format**: XML-based messages

### Hikvision OUI (MAC Prefixes)

The tool identifies Hikvision devices by their MAC address prefixes:

- `28:57:BE`
- `4C:BD:8F`
- `54:C4:15`
- `C0:56:E3`
- `E0:50:8B`
- And more...

## License

MIT License - see [LICENSE](LICENSE) for details.

## Disclaimer

This tool is intended for legitimate network administration and security
research purposes. Only use it on networks and devices you own or have
explicit permission to test. The authors are not responsible for any
misuse of this tool.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request
