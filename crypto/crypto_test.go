package crypto

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	tests := []string{
		"",
		"sk-test-key-123",
		"a-very-long-api-key-with-special-chars-!@#$%^&*()",
		"你好世界", // Unicode
	}

	for _, plain := range tests {
		enc, err := Encrypt(plain)
		if err != nil {
			t.Fatalf("Encrypt(%q) error: %v", plain, err)
		}

		dec, err := Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt(%q) error: %v", enc, err)
		}

		if dec != plain {
			t.Fatalf("round-trip failed: got %q, want %q", dec, plain)
		}
	}
}

func TestDecryptPlaintextBackwardCompat(t *testing.T) {
	// 旧明文存盘的数据不应破坏
	dec, err := Decrypt("sk-old-plaintext-key")
	if err != nil {
		t.Fatal(err)
	}
	if dec != "sk-old-plaintext-key" {
		t.Fatalf("plaintext fallback failed: got %q", dec)
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	_, err := Decrypt("ENC:!!!not-valid-base64!!!")
	if err != ErrDecryptFailed {
		t.Fatalf("expected ErrDecryptFailed, got %v", err)
	}
}
