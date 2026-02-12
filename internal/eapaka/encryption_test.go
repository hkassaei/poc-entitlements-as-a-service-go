package eapaka

import (
	"crypto/rand"
	"encoding/binary"
	"testing"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptAttributes(t *testing.T) {
	kEncr := make([]byte, 16)
	iv := make([]byte, 16)
	_, _ = rand.Read(kEncr)
	_, _ = rand.Read(iv)

	t.Run("round-trips AT_COUNTER", func(t *testing.T) {
		counterBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(counterBuf, 42)

		innerAttrs := []EapAttribute{
			{Type: config.ATCounter, Value: counterBuf},
		}

		ciphertext := EncryptAttributes(kEncr, iv, innerAttrs)
		assert.Greater(t, len(ciphertext), 0)
		assert.Equal(t, 0, len(ciphertext)%16) // AES block-aligned

		decrypted := DecryptAttributes(kEncr, iv, ciphertext)
		require.Len(t, decrypted, 1)
		assert.Equal(t, config.ATCounter, decrypted[0].Type)
		assert.Equal(t, uint16(42), binary.BigEndian.Uint16(decrypted[0].Value))
	})

	t.Run("round-trips AT_COUNTER + AT_NONCE_S + AT_NEXT_REAUTH_ID", func(t *testing.T) {
		counterBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(counterBuf, 7)
		nonceS := make([]byte, 16)
		_, _ = rand.Read(nonceS)
		nextReauthID := "test-reauth-identity-123"

		innerAttrs := []EapAttribute{
			{Type: config.ATCounter, Value: counterBuf},
			{Type: config.ATNonceS, Value: nonceS},
			{Type: config.ATNextReauthID, Value: []byte(nextReauthID)},
		}

		ciphertext := EncryptAttributes(kEncr, iv, innerAttrs)
		decrypted := DecryptAttributes(kEncr, iv, ciphertext)

		require.Len(t, decrypted, 3)

		assert.Equal(t, config.ATCounter, decrypted[0].Type)
		assert.Equal(t, uint16(7), binary.BigEndian.Uint16(decrypted[0].Value))

		assert.Equal(t, config.ATNonceS, decrypted[1].Type)
		assert.Equal(t, nonceS, decrypted[1].Value)

		assert.Equal(t, config.ATNextReauthID, decrypted[2].Type)
		assert.Equal(t, nextReauthID, string(decrypted[2].Value))
	})

	t.Run("produces different ciphertext with different IVs", func(t *testing.T) {
		counterBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(counterBuf, 1)
		innerAttrs := []EapAttribute{
			{Type: config.ATCounter, Value: counterBuf},
		}

		iv2 := make([]byte, 16)
		_, _ = rand.Read(iv2)
		ct1 := EncryptAttributes(kEncr, iv, innerAttrs)
		ct2 := EncryptAttributes(kEncr, iv2, innerAttrs)
		assert.NotEqual(t, ct1, ct2)
	})

	t.Run("produces different ciphertext with different keys", func(t *testing.T) {
		counterBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(counterBuf, 1)
		innerAttrs := []EapAttribute{
			{Type: config.ATCounter, Value: counterBuf},
		}

		kEncr2 := make([]byte, 16)
		_, _ = rand.Read(kEncr2)
		ct1 := EncryptAttributes(kEncr, iv, innerAttrs)
		ct2 := EncryptAttributes(kEncr2, iv, innerAttrs)
		assert.NotEqual(t, ct1, ct2)
	})

	t.Run("ciphertext length is always a multiple of 16", func(t *testing.T) {
		nextReauthID := "short"
		innerAttrs := []EapAttribute{
			{Type: config.ATNextReauthID, Value: []byte(nextReauthID)},
		}

		ciphertext := EncryptAttributes(kEncr, iv, innerAttrs)
		assert.Equal(t, 0, len(ciphertext)%16)
	})

	t.Run("strips AT_PADDING from decrypted result", func(t *testing.T) {
		nextReauthID := "a"
		innerAttrs := []EapAttribute{
			{Type: config.ATNextReauthID, Value: []byte(nextReauthID)},
		}

		ciphertext := EncryptAttributes(kEncr, iv, innerAttrs)
		decrypted := DecryptAttributes(kEncr, iv, ciphertext)

		// Should not contain AT_PADDING
		for _, attr := range decrypted {
			assert.NotEqual(t, config.ATPadding, attr.Type)
		}

		require.Len(t, decrypted, 1)
		assert.Equal(t, config.ATNextReauthID, decrypted[0].Type)
		assert.Equal(t, "a", string(decrypted[0].Value))
	})

	t.Run("round-trips AT_COUNTER_TOO_SMALL", func(t *testing.T) {
		counterBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(counterBuf, 5)

		innerAttrs := []EapAttribute{
			{Type: config.ATCounter, Value: counterBuf},
			{Type: config.ATCounterTooSmall, Value: []byte{}},
		}

		ciphertext := EncryptAttributes(kEncr, iv, innerAttrs)
		decrypted := DecryptAttributes(kEncr, iv, ciphertext)

		require.Len(t, decrypted, 2)
		assert.Equal(t, config.ATCounter, decrypted[0].Type)
		assert.Equal(t, config.ATCounterTooSmall, decrypted[1].Type)
	})
}
