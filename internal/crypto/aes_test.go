package crypto

import (
	"encoding/hex"
	"testing"
)

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	plaintext := "sk-test-api-key-12345"

	encrypted, err := Encrypt(plaintext, testKey)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == plaintext {
		t.Fatal("encrypted text should differ from plaintext")
	}

	decrypted, err := Decrypt(encrypted, testKey)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	plaintext := "sk-secret-key"

	encrypted, err := Encrypt(plaintext, testKey)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	wrongKey := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	_, err = Decrypt(encrypted, wrongKey)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	plaintext := "sk-secret-key"

	encrypted, err := Encrypt(plaintext, testKey)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Tamper with the ciphertext by flipping a byte
	data, _ := hex.DecodeString(encrypted)
	data[len(data)-1] ^= 0xff
	tampered := hex.EncodeToString(data)

	_, err = Decrypt(tampered, testKey)
	if err == nil {
		t.Fatal("expected error when decrypting tampered ciphertext")
	}
}

func TestInvalidHexKey(t *testing.T) {
	_, err := Encrypt("test", "not-valid-hex")
	if err == nil {
		t.Fatal("expected error for invalid hex key")
	}

	_, err = Decrypt("aabbccdd", "not-valid-hex")
	if err == nil {
		t.Fatal("expected error for invalid hex key")
	}
}

func TestKeyWrongLength(t *testing.T) {
	shortKey := "0123456789abcdef" // 8 bytes, not 32

	_, err := Encrypt("test", shortKey)
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestValidateKey(t *testing.T) {
	tests := []struct {
		name    string
		hexKey  string
		wantErr bool
	}{
		{
			name:    "valid 64-char hex (32 bytes)",
			hexKey:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			wantErr: false,
		},
		{
			name:    "short hex (16 bytes)",
			hexKey:  "0123456789abcdef0123456789abcdef",
			wantErr: true,
		},
		{
			name:    "long hex (40 bytes)",
			hexKey:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			wantErr: true,
		},
		{
			name:    "invalid hex characters",
			hexKey:  "zzzz456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			wantErr: true,
		},
		{
			name:    "empty string",
			hexKey:  "",
			wantErr: true,
		},
		{
			name:    "odd length hex",
			hexKey:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateKey(tc.hexKey)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateKey(%q) = nil, want error", tc.hexKey)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateKey(%q) = %v, want nil", tc.hexKey, err)
			}
		})
	}
}
