package eapaka

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"testing"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testIMSI = "001010000000001"

var (
	testIK = bytes.Repeat([]byte{0x11}, 16)
	testCK = bytes.Repeat([]byte{0x22}, 16)
)

func TestBuildIdentity(t *testing.T) {
	t.Run("prepends 0 to IMSI", func(t *testing.T) {
		assert.Equal(t, "0001010000000001", BuildIdentity("001010000000001"))
	})
}

func TestDeriveMasterKey(t *testing.T) {
	identity := BuildIdentity(testIMSI)

	t.Run("produces a 20-byte SHA-1 digest", func(t *testing.T) {
		mk := DeriveMasterKey(identity, testIK, testCK)
		assert.Len(t, mk, 20)
	})

	t.Run("produces deterministic output", func(t *testing.T) {
		mk1 := DeriveMasterKey(identity, testIK, testCK)
		mk2 := DeriveMasterKey(identity, testIK, testCK)
		assert.Equal(t, mk1, mk2)
	})

	t.Run("produces correct SHA-1", func(t *testing.T) {
		h := sha1.New()
		h.Write([]byte(identity))
		h.Write(testIK)
		h.Write(testCK)
		expected := h.Sum(nil)

		mk := DeriveMasterKey(identity, testIK, testCK)
		assert.Equal(t, expected, mk)
	})
}

func TestPrfSHA1(t *testing.T) {
	mk := bytes.Repeat([]byte{0x42}, 20)

	t.Run("produces output of requested length", func(t *testing.T) {
		output := PrfSHA1(mk, 160)
		assert.Len(t, output, 160)
	})

	t.Run("produces deterministic output", func(t *testing.T) {
		out1 := PrfSHA1(mk, 160)
		out2 := PrfSHA1(mk, 160)
		assert.Equal(t, out1, out2)
	})

	t.Run("handles partial block at end", func(t *testing.T) {
		output := PrfSHA1(mk, 10)
		assert.Len(t, output, 10)
	})
}

func TestDeriveKeys(t *testing.T) {
	identity := BuildIdentity(testIMSI)

	t.Run("splits PRF output into correct key sizes", func(t *testing.T) {
		keys := DeriveKeys(identity, testIK, testCK)
		assert.Len(t, keys.KEncr, 16)
		assert.Len(t, keys.KAut, 16)
		assert.Len(t, keys.MSK, 64)
		assert.Len(t, keys.EMSK, 64)
	})

	t.Run("produces deterministic keys", func(t *testing.T) {
		keys1 := DeriveKeys(identity, testIK, testCK)
		keys2 := DeriveKeys(identity, testIK, testCK)
		assert.Equal(t, keys1.KEncr, keys2.KEncr)
		assert.Equal(t, keys1.KAut, keys2.KAut)
		assert.Equal(t, keys1.MSK, keys2.MSK)
		assert.Equal(t, keys1.EMSK, keys2.EMSK)
	})

	t.Run("produces different keys for different inputs", func(t *testing.T) {
		keys1 := DeriveKeys(identity, testIK, testCK)
		differentCK := bytes.Repeat([]byte{0x33}, 16)
		keys2 := DeriveKeys(identity, testIK, differentCK)
		assert.NotEqual(t, keys1.KAut, keys2.KAut)
	})
}

func TestDeriveReauthKeys(t *testing.T) {
	mk := bytes.Repeat([]byte{0x42}, 20)
	nonceS := make([]byte, 16)
	_, _ = rand.Read(nonceS)
	identity := "test-reauth-identity"

	t.Run("produces MSK of 64 bytes and EMSK of 64 bytes", func(t *testing.T) {
		msk, emsk := DeriveReauthKeys(identity, 1, nonceS, mk)
		assert.Len(t, msk, 64)
		assert.Len(t, emsk, 64)
	})

	t.Run("produces deterministic output", func(t *testing.T) {
		msk1, emsk1 := DeriveReauthKeys(identity, 1, nonceS, mk)
		msk2, emsk2 := DeriveReauthKeys(identity, 1, nonceS, mk)
		assert.Equal(t, msk1, msk2)
		assert.Equal(t, emsk1, emsk2)
	})

	t.Run("produces different keys for different counters", func(t *testing.T) {
		msk1, _ := DeriveReauthKeys(identity, 1, nonceS, mk)
		msk2, _ := DeriveReauthKeys(identity, 2, nonceS, mk)
		assert.NotEqual(t, msk1, msk2)
	})

	t.Run("produces different keys for different nonces", func(t *testing.T) {
		nonceS2 := make([]byte, 16)
		_, _ = rand.Read(nonceS2)
		msk1, _ := DeriveReauthKeys(identity, 1, nonceS, mk)
		msk2, _ := DeriveReauthKeys(identity, 1, nonceS2, mk)
		assert.NotEqual(t, msk1, msk2)
	})

	t.Run("produces different keys for different identities", func(t *testing.T) {
		msk1, _ := DeriveReauthKeys(identity, 1, nonceS, mk)
		msk2, _ := DeriveReauthKeys("different-identity", 1, nonceS, mk)
		assert.NotEqual(t, msk1, msk2)
	})

	t.Run("produces different keys for different MKs", func(t *testing.T) {
		mk2 := bytes.Repeat([]byte{0x99}, 20)
		msk1, _ := DeriveReauthKeys(identity, 1, nonceS, mk)
		msk2, _ := DeriveReauthKeys(identity, 1, nonceS, mk2)
		assert.NotEqual(t, msk1, msk2)
	})
}

func TestComputeVerifyMAC(t *testing.T) {
	t.Run("produces a 16-byte MAC", func(t *testing.T) {
		kAut := bytes.Repeat([]byte{0xaa}, 16)
		data := bytes.Repeat([]byte{0xbb}, 64)
		mac := ComputeMAC(kAut, data)
		assert.Len(t, mac, 16)
	})

	t.Run("round-trips with VerifyMAC", func(t *testing.T) {
		identity := BuildIdentity(testIMSI)
		keys := DeriveKeys(identity, testIK, testCK)

		randBytes := make([]byte, 16)
		autnBytes := make([]byte, 16)
		_, _ = rand.Read(randBytes)
		_, _ = rand.Read(autnBytes)

		packet := EncodeEapPacket(EapPacket{
			Code:       config.EAPCodeRequest,
			Identifier: 1,
			Type:       config.EAPTypeAKA,
			Subtype:    config.AKASubtypeChallenge,
			Attributes: []EapAttribute{
				{Type: config.ATRand, Value: randBytes},
				{Type: config.ATAutn, Value: autnBytes},
				{Type: config.ATMac, Value: make([]byte, 16)}, // zeroed MAC
			},
		})

		// Compute MAC over packet with zeroed MAC field
		mac := ComputeMAC(keys.KAut, packet)

		// AT_MAC starts at offset: 8 (header) + 20 (AT_RAND) + 20 (AT_AUTN) = 48
		// MAC value is at offset 48 + 4 (type+length+reserved) = 52
		copy(packet[52:], mac)

		// Verify should pass
		assert.True(t, VerifyMAC(keys.KAut, packet, 52, mac))
	})

	t.Run("rejects tampered packets", func(t *testing.T) {
		identity := BuildIdentity(testIMSI)
		keys := DeriveKeys(identity, testIK, testCK)

		randBytes := make([]byte, 16)
		autnBytes := make([]byte, 16)
		_, _ = rand.Read(randBytes)
		_, _ = rand.Read(autnBytes)

		packet := EncodeEapPacket(EapPacket{
			Code:       config.EAPCodeRequest,
			Identifier: 1,
			Type:       config.EAPTypeAKA,
			Subtype:    config.AKASubtypeChallenge,
			Attributes: []EapAttribute{
				{Type: config.ATRand, Value: randBytes},
				{Type: config.ATAutn, Value: autnBytes},
				{Type: config.ATMac, Value: make([]byte, 16)},
			},
		})

		mac := ComputeMAC(keys.KAut, packet)
		copy(packet[52:], mac)

		// Tamper with the RAND value
		packet[10] = (packet[10] + 1) & 0xff

		require.False(t, VerifyMAC(keys.KAut, packet, 52, mac))
	})
}
