package cli

import (
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"empty args", []string{}, false},
		{"help", []string{"help"}, false},
		{"--help", []string{"--help"}, false},
		{"-h", []string{"-h"}, false},
		{"unknown command", []string{"unknown"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Run(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReorderArgsForFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "no flags",
			args:     []string{"192.168.1.1", "inquiry"},
			expected: []string{"192.168.1.1", "inquiry"},
		},
		{
			name:     "flags before positional",
			args:     []string{"--debug", "192.168.1.1"},
			expected: []string{"--debug", "192.168.1.1"},
		},
		{
			name:     "flags after positional",
			args:     []string{"192.168.1.1", "inquiry", "--debug"},
			expected: []string{"--debug", "192.168.1.1", "inquiry"},
		},
		{
			name:     "flag with value after positional",
			args:     []string{"192.168.1.1", "--mac", "AA:BB:CC:DD:EE:FF"},
			expected: []string{"--mac", "AA:BB:CC:DD:EE:FF", "192.168.1.1"},
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reorderArgsForFlags(tt.args)
			if len(result) != len(tt.expected) {
				t.Errorf("length mismatch: got %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("result[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestExtractFirmwareVersion(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "XML firmwareVersion tag",
			body:     "<deviceInfo><firmwareVersion>V5.5.0</firmwareVersion></deviceInfo>",
			expected: "V5.5.0",
		},
		{
			name:     "XML version tag",
			body:     "<info><version>2.0.1</version></info>",
			expected: "2.0.1",
		},
		{
			name:     "JSON firmwareVersion",
			body:     `{"deviceInfo": {"firmwareVersion": "V4.3.0"}}`,
			expected: "V4.3.0",
		},
		{
			name:     "no firmware version",
			body:     "some random content",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFirmwareVersion(tt.body)
			if result != tt.expected {
				t.Errorf("extractFirmwareVersion() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractModel(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "XML deviceName tag",
			body:     "<deviceInfo><deviceName>DS-2CD2042WD-I</deviceName></deviceInfo>",
			expected: "DS-2CD2042WD-I",
		},
		{
			name:     "XML model tag",
			body:     "<info><model>NVR-1234</model></info>",
			expected: "NVR-1234",
		},
		{
			name:     "JSON model",
			body:     `{"deviceInfo": {"model": "Camera-X100"}}`,
			expected: "Camera-X100",
		},
		{
			name:     "no model",
			body:     "some random content",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractModel(tt.body)
			if result != tt.expected {
				t.Errorf("extractModel() = %q, want %q", result, tt.expected)
			}
		})
	}
}
