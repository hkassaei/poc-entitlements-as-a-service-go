package crypto

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalKeyManager(t *testing.T) {
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)

	t.Run("unwraps DEK correctly", func(t *testing.T) {
		km, err := NewLocalKeyManager(kek)
		require.NoError(t, err)

		dek := make([]byte, 32)
		_, _ = rand.Read(dek)

		wrapped, err := WrapDEK(kek, dek)
		require.NoError(t, err)

		unwrapped, err := km.UnwrapDEK(context.Background(), wrapped)
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(dek), hex.EncodeToString(unwrapped))
	})

	t.Run("rejects wrong-length KEK", func(t *testing.T) {
		_, err := NewLocalKeyManager(make([]byte, 16))
		assert.Error(t, err)
	})
}

func TestCreateKeyManager(t *testing.T) {
	t.Run("creates LocalKeyManager when KEK provided", func(t *testing.T) {
		kek := make([]byte, 32)
		_, _ = rand.Read(kek)
		km, err := CreateKeyManager(kek, "", "", "", "")
		require.NoError(t, err)
		_, ok := km.(*LocalKeyManager)
		assert.True(t, ok)
	})

	t.Run("creates CloudKMSKeyManager when GCP project provided", func(t *testing.T) {
		km, err := CreateKeyManager(nil, "my-project", "us-central1", "my-keyring", "my-key")
		require.NoError(t, err)
		_, ok := km.(*CloudKMSKeyManager)
		assert.True(t, ok)
	})

	t.Run("errors when neither KEK nor GCP project provided", func(t *testing.T) {
		_, err := CreateKeyManager(nil, "", "", "", "")
		assert.Error(t, err)
	})
}
