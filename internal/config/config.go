package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds all application configuration
type Config struct {
	// HTTP settings
	HTTPTimeout time.Duration `env:"HTTP_TIMEOUT" envDefault:"10s"`
	UserAgent   string        `env:"USER_AGENT" envDefault:"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"`

	// Discovery settings
	DiscoveryWorkers int           `env:"DISCOVERY_WORKERS" envDefault:"100"`
	DiscoveryTimeout time.Duration `env:"DISCOVERY_TIMEOUT" envDefault:"1s"`

	// SADP settings
	SADPTimeout time.Duration `env:"SADP_TIMEOUT" envDefault:"5s"`

	// Output settings
	OutputDir string `env:"OUTPUT_DIR" envDefault:"data"`
	Debug     bool   `env:"DEBUG" envDefault:"false"`

	// Encryption keys (these are hardcoded for Hikvision devices)
	AESKeyHex string `env:"AES_KEY_HEX" envDefault:"279977f62f6cfd2d91cd75b889ce0c9a"`
	XORKeyHex string `env:"XOR_KEY_HEX" envDefault:"738B5544"`
}

// Load parses environment variables into Config
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadWithOptions parses environment variables with custom options
func LoadWithOptions(opts env.Options) (*Config, error) {
	cfg := &Config{}
	if err := env.ParseWithOptions(cfg, opts); err != nil {
		return nil, err
	}
	return cfg, nil
}

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	return &Config{
		HTTPTimeout:      10 * time.Second,
		UserAgent:        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		DiscoveryWorkers: 100,
		DiscoveryTimeout: 1 * time.Second,
		SADPTimeout:      5 * time.Second,
		OutputDir:        "data",
		Debug:            false,
		AESKeyHex:        "279977f62f6cfd2d91cd75b889ce0c9a",
		XORKeyHex:        "738B5544",
	}
}
