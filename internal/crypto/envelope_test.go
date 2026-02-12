package crypto

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptWithDEK(t *testing.T) {
	dek := make([]byte, 32)
	_, err := rand.Read(dek)
	require.NoError(t, err)

	plaintext := []byte("hello, world! this is a secret message")

	t.Run("round-trips plaintext", func(t *testing.T) {
		encrypted, err := EncryptWithDEK(dek, plaintext)
		require.NoError(t, err)
		assert.Greater(t, len(encrypted), len(plaintext))

		decrypted, err := DecryptWithDEK(dek, encrypted)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("produces different ciphertext each time (random IV)", func(t *testing.T) {
		ct1, err := EncryptWithDEK(dek, plaintext)
		require.NoError(t, err)
		ct2, err := EncryptWithDEK(dek, plaintext)
		require.NoError(t, err)
		assert.NotEqual(t, ct1, ct2)
	})

	t.Run("rejects tampered ciphertext", func(t *testing.T) {
		encrypted, err := EncryptWithDEK(dek, plaintext)
		require.NoError(t, err)
		encrypted[15] ^= 0xff // tamper
		_, err = DecryptWithDEK(dek, encrypted)
		assert.Error(t, err)
	})

	t.Run("rejects wrong key", func(t *testing.T) {
		encrypted, err := EncryptWithDEK(dek, plaintext)
		require.NoError(t, err)

		wrongDEK := make([]byte, 32)
		_, _ = rand.Read(wrongDEK)
		_, err = DecryptWithDEK(wrongDEK, encrypted)
		assert.Error(t, err)
	})
}

func TestWrapUnwrapDEK(t *testing.T) {
	kek := make([]byte, 32)
	dek := make([]byte, 32)
	_, _ = rand.Read(kek)
	_, _ = rand.Read(dek)

	t.Run("round-trips DEK", func(t *testing.T) {
		wrapped, err := WrapDEK(kek, dek)
		require.NoError(t, err)

		unwrapped, err := UnwrapDEK(kek, wrapped)
		require.NoError(t, err)
		assert.Equal(t, dek, unwrapped)
	})

	t.Run("rejects wrong-length KEK", func(t *testing.T) {
		_, err := WrapDEK(make([]byte, 16), dek)
		assert.Error(t, err)
	})

	t.Run("rejects wrong-length DEK", func(t *testing.T) {
		_, err := WrapDEK(kek, make([]byte, 16))
		assert.Error(t, err)
	})
}

func TestZeroSlice(t *testing.T) {
	buf := []byte{0x01, 0x02, 0x03, 0x04}
	ZeroSlice(buf)
	for _, b := range buf {
		assert.Equal(t, byte(0), b)
	}
}
