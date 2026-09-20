package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// DeriveKey derives a 32-byte AES-256 key from an arbitrary master secret string using SHA-256.
func DeriveKey(secret string) []byte {
	hash := sha256.Sum256([]byte(secret))
	return hash[:]
}

// Encrypt encrypts plaintext using AES-GCM-256 with a random 12-byte nonce.
// Returns base64-encoded ciphertext and hex-encoded nonce.
func Encrypt(plaintext string, secretKey []byte) (ciphertextBase64, nonceHex string, err error) {
	if len(secretKey) != 32 {
		return "", "", errors.New("secretKey must be exactly 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(secretKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", fmt.Errorf("failed to create gcm block: %w", err)
	}

	// 12-byte standard GCM nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", fmt.Errorf("failed to generate random nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), hex.EncodeToString(nonce), nil
}

// Decrypt decrypts AES-GCM-256 base64-encoded ciphertext using the hex-encoded nonce.
func Decrypt(ciphertextBase64, nonceHex string, secretKey []byte) (string, error) {
	if len(secretKey) != 32 {
		return "", errors.New("secretKey must be exactly 32 bytes for AES-256")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 ciphertext: %w", err)
	}

	nonce, err := hex.DecodeString(nonceHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode hex nonce: %w", err)
	}

	block, err := aes.NewCipher(secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create gcm block: %w", err)
	}

	if len(nonce) != gcm.NonceSize() {
		return "", fmt.Errorf("invalid nonce length: got %d, expected %d", len(nonce), gcm.NonceSize())
	}

	plaintextBytes, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed or authentication tag mismatch: %w", err)
	}

	return string(plaintextBytes), nil
}
