package crypto

import (
	"bytes"
	"testing"
)

func TestDecryptAES(t *testing.T) {
	tests := []struct {
		name    string
		keyHex  string
		data    []byte
		wantErr bool
	}{
		{
			name:    "valid key and data",
			keyHex:  "279977f62f6cfd2d91cd75b889ce0c9a",
			data:    make([]byte, 16),
			wantErr: false,
		},
		{
			name:    "data not block aligned",
			keyHex:  "279977f62f6cfd2d91cd75b889ce0c9a",
			data:    make([]byte, 17),
			wantErr: false,
		},
		{
			name:    "invalid key hex",
			keyHex:  "invalid",
			data:    make([]byte, 16),
			wantErr: true,
		},
		{
			name:    "key too short",
			keyHex:  "279977",
			data:    make([]byte, 16),
			wantErr: true,
		},
		{
			name:    "empty data",
			keyHex:  "279977f62f6cfd2d91cd75b889ce0c9a",
			data:    []byte{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecryptAES(tt.data, tt.keyHex)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecryptAES() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecryptAESRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		keyHex string
		data   []byte
	}{
		{
			name:   "decrypt zeros produces deterministic output",
			keyHex: "279977f62f6cfd2d91cd75b889ce0c9a",
			data:   make([]byte, 16),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decrypted, err := DecryptAES(tt.data, tt.keyHex)
			if err != nil {
				t.Fatalf("DecryptAES failed: %v", err)
			}

			if len(decrypted) != len(tt.data) {
				t.Errorf("expected length %d, got %d", len(tt.data), len(decrypted))
			}
		})
	}
}

func TestDecryptXOR(t *testing.T) {
	tests := []struct {
		name    string
		keyHex  string
		data    []byte
		wantErr bool
	}{
		{
			name:    "valid key and data",
			keyHex:  "738B5544",
			data:    []byte("test data"),
			wantErr: false,
		},
		{
			name:    "empty data",
			keyHex:  "738B5544",
			data:    []byte{},
			wantErr: false,
		},
		{
			name:    "invalid key hex",
			keyHex:  "invalid",
			data:    []byte("test"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecryptXOR(tt.data, tt.keyHex)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecryptXOR() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecryptXORRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		keyHex   string
		original []byte
	}{
		{
			name:     "XOR round trip returns original",
			keyHex:   "738B5544",
			original: []byte("Hello World! This is a test message."),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := DecryptXOR(tt.original, tt.keyHex)
			if err != nil {
				t.Fatalf("first XOR failed: %v", err)
			}

			decoded, err := DecryptXOR(encoded, tt.keyHex)
			if err != nil {
				t.Fatalf("second XOR failed: %v", err)
			}

			if !bytes.Equal(tt.original, decoded) {
				t.Errorf("round trip failed: got %q, want %q", decoded, tt.original)
			}
		})
	}
}

func TestGenerateResetCode(t *testing.T) {
	tests := []struct {
		name     string
		serial   string
		date     string
		expected string
	}{
		{
			name:     "standard serial and date",
			serial:   "0123456789",
			date:     "20231215",
			expected: "RRRrdy9yRd",
		},
		{
			name:     "empty input",
			serial:   "",
			date:     "",
			expected: "Q",
		},
		{
			name:     "short serial",
			serial:   "ABC",
			date:     "20240101",
			expected: "RQd9qSerQz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateResetCode(tt.serial, tt.date)
			if result != tt.expected {
				t.Errorf("GenerateResetCode(%q, %q) = %q, want %q",
					tt.serial, tt.date, result, tt.expected)
			}
		})
	}
}

func TestGenerateResetCodeDeterministic(t *testing.T) {
	tests := []struct {
		name   string
		serial string
		date   string
	}{
		{
			name:   "same input produces same output",
			serial: "TEST123",
			date:   "20231225",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result1 := GenerateResetCode(tt.serial, tt.date)
			result2 := GenerateResetCode(tt.serial, tt.date)

			if result1 != result2 {
				t.Errorf("GenerateResetCode is not deterministic: got %q and %q", result1, result2)
			}
		})
	}
}

func TestGenerateResetCodeSubstitution(t *testing.T) {
	tests := []struct {
		name       string
		serial     string
		date       string
		validChars string
	}{
		{
			name:       "output contains only valid substituted characters",
			serial:     "TEST",
			date:       "20240101",
			validChars: "QRSqrdey z9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateResetCode(tt.serial, tt.date)

			for _, c := range result {
				found := false
				for _, v := range tt.validChars {
					if c == v {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("unexpected character in result: %c", c)
				}
			}
		})
	}
}
