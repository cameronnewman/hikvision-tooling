package config

import (
	"testing"
	"time"

	"github.com/caarlos0/env/v11"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name               string
		wantHTTPTimeout    time.Duration
		wantWorkers        int
		wantDiscTimeout    time.Duration
		wantSADPTimeout    time.Duration
		wantOutputDir      string
		wantDebug          bool
		wantAESKeyHex      string
		wantXORKeyHex      string
		wantUserAgentEmpty bool
	}{
		{
			name:               "default values",
			wantHTTPTimeout:    10 * time.Second,
			wantWorkers:        100,
			wantDiscTimeout:    1 * time.Second,
			wantSADPTimeout:    5 * time.Second,
			wantOutputDir:      "data",
			wantDebug:          false,
			wantAESKeyHex:      "279977f62f6cfd2d91cd75b889ce0c9a",
			wantXORKeyHex:      "738B5544",
			wantUserAgentEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() returned error: %v", err)
			}

			if cfg == nil {
				t.Fatal("Load() returned nil config")
			}

			if cfg.HTTPTimeout != tt.wantHTTPTimeout {
				t.Errorf("HTTPTimeout = %v, want %v", cfg.HTTPTimeout, tt.wantHTTPTimeout)
			}

			if cfg.DiscoveryWorkers != tt.wantWorkers {
				t.Errorf("DiscoveryWorkers = %d, want %d", cfg.DiscoveryWorkers, tt.wantWorkers)
			}

			if cfg.DiscoveryTimeout != tt.wantDiscTimeout {
				t.Errorf("DiscoveryTimeout = %v, want %v", cfg.DiscoveryTimeout, tt.wantDiscTimeout)
			}

			if cfg.SADPTimeout != tt.wantSADPTimeout {
				t.Errorf("SADPTimeout = %v, want %v", cfg.SADPTimeout, tt.wantSADPTimeout)
			}

			if cfg.OutputDir != tt.wantOutputDir {
				t.Errorf("OutputDir = %s, want %s", cfg.OutputDir, tt.wantOutputDir)
			}

			if cfg.Debug != tt.wantDebug {
				t.Errorf("Debug = %v, want %v", cfg.Debug, tt.wantDebug)
			}

			if cfg.AESKeyHex != tt.wantAESKeyHex {
				t.Errorf("AESKeyHex = %s, want %s", cfg.AESKeyHex, tt.wantAESKeyHex)
			}

			if cfg.XORKeyHex != tt.wantXORKeyHex {
				t.Errorf("XORKeyHex = %s, want %s", cfg.XORKeyHex, tt.wantXORKeyHex)
			}

			if tt.wantUserAgentEmpty && cfg.UserAgent != "" {
				t.Errorf("UserAgent = %s, want empty", cfg.UserAgent)
			}
			if !tt.wantUserAgentEmpty && cfg.UserAgent == "" {
				t.Error("UserAgent is empty, want non-empty")
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	tests := []struct {
		name               string
		wantHTTPTimeout    time.Duration
		wantWorkers        int
		wantDiscTimeout    time.Duration
		wantSADPTimeout    time.Duration
		wantOutputDir      string
		wantDebug          bool
		wantAESKeyHex      string
		wantXORKeyHex      string
		wantUserAgentEmpty bool
	}{
		{
			name:               "default config values",
			wantHTTPTimeout:    10 * time.Second,
			wantWorkers:        100,
			wantDiscTimeout:    1 * time.Second,
			wantSADPTimeout:    5 * time.Second,
			wantOutputDir:      "data",
			wantDebug:          false,
			wantAESKeyHex:      "279977f62f6cfd2d91cd75b889ce0c9a",
			wantXORKeyHex:      "738B5544",
			wantUserAgentEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()

			if cfg == nil {
				t.Fatal("DefaultConfig() returned nil")
			}

			if cfg.HTTPTimeout != tt.wantHTTPTimeout {
				t.Errorf("HTTPTimeout = %v, want %v", cfg.HTTPTimeout, tt.wantHTTPTimeout)
			}

			if cfg.DiscoveryWorkers != tt.wantWorkers {
				t.Errorf("DiscoveryWorkers = %d, want %d", cfg.DiscoveryWorkers, tt.wantWorkers)
			}

			if cfg.DiscoveryTimeout != tt.wantDiscTimeout {
				t.Errorf("DiscoveryTimeout = %v, want %v", cfg.DiscoveryTimeout, tt.wantDiscTimeout)
			}

			if cfg.SADPTimeout != tt.wantSADPTimeout {
				t.Errorf("SADPTimeout = %v, want %v", cfg.SADPTimeout, tt.wantSADPTimeout)
			}

			if cfg.OutputDir != tt.wantOutputDir {
				t.Errorf("OutputDir = %s, want %s", cfg.OutputDir, tt.wantOutputDir)
			}

			if cfg.Debug != tt.wantDebug {
				t.Errorf("Debug = %v, want %v", cfg.Debug, tt.wantDebug)
			}

			if cfg.AESKeyHex != tt.wantAESKeyHex {
				t.Errorf("AESKeyHex = %s, want %s", cfg.AESKeyHex, tt.wantAESKeyHex)
			}

			if cfg.XORKeyHex != tt.wantXORKeyHex {
				t.Errorf("XORKeyHex = %s, want %s", cfg.XORKeyHex, tt.wantXORKeyHex)
			}

			if tt.wantUserAgentEmpty && cfg.UserAgent != "" {
				t.Errorf("UserAgent = %s, want empty", cfg.UserAgent)
			}
			if !tt.wantUserAgentEmpty && cfg.UserAgent == "" {
				t.Error("UserAgent is empty, want non-empty")
			}
		})
	}
}

func TestLoadWithEnvOverrides(t *testing.T) {
	tests := []struct {
		name            string
		envHTTPTimeout  string
		envWorkers      string
		envDebug        string
		wantHTTPTimeout time.Duration
		wantWorkers     int
		wantDebug       bool
	}{
		{
			name:            "override with environment variables",
			envHTTPTimeout:  "30s",
			envWorkers:      "50",
			envDebug:        "true",
			wantHTTPTimeout: 30 * time.Second,
			wantWorkers:     50,
			wantDebug:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HTTP_TIMEOUT", tt.envHTTPTimeout)
			t.Setenv("DISCOVERY_WORKERS", tt.envWorkers)
			t.Setenv("DEBUG", tt.envDebug)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() returned error: %v", err)
			}

			if cfg.HTTPTimeout != tt.wantHTTPTimeout {
				t.Errorf("HTTPTimeout = %v, want %v", cfg.HTTPTimeout, tt.wantHTTPTimeout)
			}

			if cfg.DiscoveryWorkers != tt.wantWorkers {
				t.Errorf("DiscoveryWorkers = %d, want %d", cfg.DiscoveryWorkers, tt.wantWorkers)
			}

			if cfg.Debug != tt.wantDebug {
				t.Errorf("Debug = %v, want %v", cfg.Debug, tt.wantDebug)
			}
		})
	}
}

func TestLoadWithOptions(t *testing.T) {
	tests := []struct {
		name          string
		wantOutputDir string
	}{
		{
			name:          "load with empty options",
			wantOutputDir: "data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadWithOptions(env.Options{})
			if err != nil {
				t.Fatalf("LoadWithOptions() returned error: %v", err)
			}

			if cfg == nil {
				t.Fatal("LoadWithOptions() returned nil config")
			}

			if cfg.OutputDir != tt.wantOutputDir {
				t.Errorf("OutputDir = %s, want %s", cfg.OutputDir, tt.wantOutputDir)
			}
		})
	}
}
