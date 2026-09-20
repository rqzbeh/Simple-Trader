package auth

import (
	"testing"
)

func TestDeriveKey(t *testing.T) {
	secret := "my-secret-key-phrase"
	key := DeriveKey(secret)
	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(key))
	}

	key2 := DeriveKey(secret)
	if string(key) != string(key2) {
		t.Fatalf("expected deterministic key derivation")
	}
}

func TestAESGCMEncryptionDecryption(t *testing.T) {
	key := DeriveKey("institutional-grade-master-key-2026")
	originalText := "telegram_bot_token_secret_998877112233"

	cipherB64, nonceHex, err := Encrypt(originalText, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if cipherB64 == "" || nonceHex == "" {
		t.Fatalf("empty ciphertext or nonce")
	}

	// Verify nonce is 12 bytes = 24 hex chars
	if len(nonceHex) != 24 {
		t.Fatalf("expected 24-char hex nonce for 12 bytes, got %d chars (%s)", len(nonceHex), nonceHex)
	}

	decrypted, err := Decrypt(cipherB64, nonceHex, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != originalText {
		t.Fatalf("decrypted mismatch: expected %q, got %q", originalText, decrypted)
	}
}

func TestAESGCMTamperResistance(t *testing.T) {
	key := DeriveKey("institutional-grade-master-key-2026")
	originalText := "sensitive-financial-api-key"

	cipherB64, nonceHex, err := Encrypt(originalText, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Tampered key
	wrongKey := DeriveKey("wrong-key")
	_, err = Decrypt(cipherB64, nonceHex, wrongKey)
	if err == nil {
		t.Fatalf("expected error when decrypting with wrong key, got nil")
	}

	// Tampered ciphertext
	tamperedCipher := "A" + cipherB64[1:]
	_, err = Decrypt(tamperedCipher, nonceHex, key)
	if err == nil {
		t.Fatalf("expected error on tampered ciphertext, got nil")
	}
}
