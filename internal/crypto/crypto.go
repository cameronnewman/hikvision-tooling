package crypto

import (
	"bytes"
	"crypto/aes"
	"encoding/hex"
	"fmt"
)

// DecryptAES decrypts data using AES-ECB mode with the given hex key
func DecryptAES(data []byte, keyHex string) ([]byte, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid AES key hex: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	blockSize := block.BlockSize()

	// Pad data to block size if necessary
	paddedData := data
	if len(data)%blockSize != 0 {
		padding := blockSize - (len(data) % blockSize)
		paddedData = append(data, bytes.Repeat([]byte{0}, padding)...)
	}

	// Decrypt using ECB mode (block by block)
	decrypted := make([]byte, len(paddedData))
	for i := 0; i < len(paddedData); i += blockSize {
		block.Decrypt(decrypted[i:i+blockSize], paddedData[i:i+blockSize])
	}

	return decrypted, nil
}

// DecryptXOR decrypts data using XOR with the given hex key
func DecryptXOR(data []byte, keyHex string) ([]byte, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid XOR key hex: %w", err)
	}

	decoded := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		decoded[i] = data[i] ^ key[i%len(key)]
	}

	return decoded, nil
}

// GenerateResetCode generates a Hikvision password reset code
// Works on firmware versions < 5.3.0
func GenerateResetCode(serial, date string) string {
	seed := serial + date

	// Stage 1: Calculate magic number
	var magic uint64 = 0
	for i, char := range seed {
		pos := uint64(i + 1)
		charVal := uint64(char)
		magic += (pos * charVal) ^ pos
	}

	// Stage 2: Multiply by constant and convert to uint32
	const multiplier uint64 = 1751873395
	secret := uint32(magic * multiplier)

	// Stage 3: Convert to string and apply character substitution
	secretStr := fmt.Sprintf("%d", secret)

	// Substitution: "012345678" -> "QRSqrdeyz"
	substitution := map[rune]rune{
		'0': 'Q',
		'1': 'R',
		'2': 'S',
		'3': 'q',
		'4': 'r',
		'5': 'd',
		'6': 'e',
		'7': 'y',
		'8': 'z',
	}

	var result []rune
	for _, c := range secretStr {
		if sub, ok := substitution[c]; ok {
			result = append(result, sub)
		} else {
			result = append(result, c)
		}
	}

	return string(result)
}
