package crypto

import (
	"context"
	"errors"
	"fmt"
)

// KeyManager abstracts key unwrapping for envelope encryption.
type KeyManager interface {
	UnwrapDEK(ctx context.Context, wrappedDEK []byte) ([]byte, error)
}

// LocalKeyManager uses a KEK from environment (dev only).
type LocalKeyManager struct {
	kek []byte
}

// NewLocalKeyManager creates a LocalKeyManager from a hex-encoded KEK.
func NewLocalKeyManager(kek []byte) (*LocalKeyManager, error) {
	if len(kek) != 32 {
		return nil, errors.New("KEK must be 32 bytes")
	}
	k := make([]byte, 32)
	copy(k, kek)
	return &LocalKeyManager{kek: k}, nil
}

// UnwrapDEK unwraps a DEK using XOR with the local KEK.
func (m *LocalKeyManager) UnwrapDEK(_ context.Context, wrappedDEK []byte) ([]byte, error) {
	return UnwrapDEK(m.kek, wrappedDEK)
}

// CloudKMSKeyManager uses Google Cloud KMS to decrypt (unwrap) DEKs.
type CloudKMSKeyManager struct {
	keyName string
}

// NewCloudKMSKeyManager creates a CloudKMSKeyManager.
func NewCloudKMSKeyManager(projectID, locationID, keyRingID, keyID string) *CloudKMSKeyManager {
	keyName := fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
		projectID, locationID, keyRingID, keyID)
	return &CloudKMSKeyManager{keyName: keyName}
}

// UnwrapDEK decrypts a wrapped DEK using Cloud KMS.
func (m *CloudKMSKeyManager) UnwrapDEK(ctx context.Context, wrappedDEK []byte) ([]byte, error) {
	// Lazy-load the KMS client to avoid requiring the dependency in dev environments.
	// This will be implemented when the cloud.google.com/go/kms dependency is added.
	return nil, errors.New("CloudKMSKeyManager: not yet implemented — add cloud.google.com/go/kms dependency")
}

// CreateKeyManager creates the appropriate key manager based on configuration.
func CreateKeyManager(localKEK []byte, gcpProjectID, kmsLocation, kmsKeyRing, kmsKeyName string) (KeyManager, error) {
	if len(localKEK) > 0 {
		return NewLocalKeyManager(localKEK)
	}
	if gcpProjectID == "" {
		return nil, errors.New("either LOCAL_KEK_HEX or GCP_PROJECT_ID must be set")
	}
	return NewCloudKMSKeyManager(gcpProjectID, kmsLocation, kmsKeyRing, kmsKeyName), nil
}
