package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// EncryptWithDEK encrypts plaintext with a DEK using AES-256-GCM.
// Returns IV[12] || ciphertext || authTag[16].
func EncryptWithDEK(dek, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}

	iv := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("rand.Read: %w", err)
	}

	// Seal appends ciphertext+authTag to iv
	sealed := gcm.Seal(iv, iv, plaintext, nil)
	return sealed, nil
}

// DecryptWithDEK decrypts data produced by EncryptWithDEK.
func DecryptWithDEK(dek, encrypted []byte) ([]byte, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(encrypted) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	iv := encrypted[:nonceSize]
	ciphertext := encrypted[nonceSize:]

	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("gcm.Open: %w", err)
	}
	return plaintext, nil
}

// WrapDEK wraps a DEK using XOR with a KEK (simple dev-only scheme). Both must be 32 bytes.
func WrapDEK(kek, dek []byte) ([]byte, error) {
	if len(kek) != 32 || len(dek) != 32 {
		return nil, errors.New("KEK and DEK must both be 32 bytes")
	}
	wrapped := make([]byte, 32)
	for i := 0; i < 32; i++ {
		wrapped[i] = kek[i] ^ dek[i]
	}
	return wrapped, nil
}

// UnwrapDEK unwraps a DEK using XOR with a KEK.
func UnwrapDEK(kek, wrappedDEK []byte) ([]byte, error) {
	return WrapDEK(kek, wrappedDEK) // XOR is its own inverse
}

// ZeroSlice zeroes out a byte slice for memory hygiene.
func ZeroSlice(buf []byte) {
	clear(buf)
}
