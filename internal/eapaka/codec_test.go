package eapaka

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findAttr(attrs []EapAttribute, attrType int) *EapAttribute {
	for i := range attrs {
		if attrs[i].Type == attrType {
			return &attrs[i]
		}
	}
	return nil
}

func TestEAPSuccessFailure(t *testing.T) {
	t.Run("encodes EAP-Success to 4 bytes", func(t *testing.T) {
		pkt := EapPacket{Code: config.EAPCodeSuccess, Identifier: 42}
		buf := EncodeEapPacket(pkt)
		assert.Len(t, buf, 4)
		assert.Equal(t, byte(config.EAPCodeSuccess), buf[0])
		assert.Equal(t, byte(42), buf[1])
		assert.Equal(t, uint16(4), binary.BigEndian.Uint16(buf[2:4]))
	})

	t.Run("encodes EAP-Failure to 4 bytes", func(t *testing.T) {
		pkt := EapPacket{Code: config.EAPCodeFailure, Identifier: 7}
		buf := EncodeEapPacket(pkt)
		assert.Len(t, buf, 4)
		assert.Equal(t, byte(config.EAPCodeFailure), buf[0])
		assert.Equal(t, byte(7), buf[1])
		assert.Equal(t, uint16(4), binary.BigEndian.Uint16(buf[2:4]))
	})

	t.Run("round-trips EAP-Success", func(t *testing.T) {
		original := EapPacket{Code: config.EAPCodeSuccess, Identifier: 99}
		decoded, err := DecodeEapPacket(EncodeEapPacket(original))
		require.NoError(t, err)
		assert.Equal(t, config.EAPCodeSuccess, decoded.Code)
		assert.Equal(t, 99, decoded.Identifier)
	})

	t.Run("round-trips EAP-Failure", func(t *testing.T) {
		original := EapPacket{Code: config.EAPCodeFailure, Identifier: 1}
		decoded, err := DecodeEapPacket(EncodeEapPacket(original))
		require.NoError(t, err)
		assert.Equal(t, config.EAPCodeFailure, decoded.Code)
		assert.Equal(t, 1, decoded.Identifier)
	})
}

func TestAKAChallenge(t *testing.T) {
	rand := bytes.Repeat([]byte{0xaa}, 16)
	autn := bytes.Repeat([]byte{0xbb}, 16)
	mac := bytes.Repeat([]byte{0xcc}, 16)

	challengePacket := EapPacket{
		Code:       config.EAPCodeRequest,
		Identifier: 10,
		Type:       config.EAPTypeAKA,
		Subtype:    config.AKASubtypeChallenge,
		Attributes: []EapAttribute{
			{Type: config.ATRand, Value: rand},
			{Type: config.ATAutn, Value: autn},
			{Type: config.ATMac, Value: mac},
		},
	}

	t.Run("encodes to correct total length", func(t *testing.T) {
		buf := EncodeEapPacket(challengePacket)
		// Header(8) + AT_RAND(20) + AT_AUTN(20) + AT_MAC(20) = 68
		assert.Len(t, buf, 68)
		assert.Equal(t, uint16(68), binary.BigEndian.Uint16(buf[2:4]))
	})

	t.Run("round-trips all attributes", func(t *testing.T) {
		buf := EncodeEapPacket(challengePacket)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)

		assert.Equal(t, config.EAPCodeRequest, decoded.Code)
		assert.Equal(t, 10, decoded.Identifier)
		assert.Equal(t, config.EAPTypeAKA, decoded.Type)
		assert.Equal(t, config.AKASubtypeChallenge, decoded.Subtype)
		assert.Len(t, decoded.Attributes, 3)

		decodedRand := findAttr(decoded.Attributes, config.ATRand)
		decodedAutn := findAttr(decoded.Attributes, config.ATAutn)
		decodedMac := findAttr(decoded.Attributes, config.ATMac)

		require.NotNil(t, decodedRand)
		require.NotNil(t, decodedAutn)
		require.NotNil(t, decodedMac)

		assert.Equal(t, rand, decodedRand.Value)
		assert.Equal(t, autn, decodedAutn.Value)
		assert.Equal(t, mac, decodedMac.Value)
	})
}

func TestAKAResponse(t *testing.T) {
	xres := bytes.Repeat([]byte{0xdd}, 8) // 8 bytes = 64 bits
	mac := bytes.Repeat([]byte{0xee}, 16)

	responsePacket := EapPacket{
		Code:       config.EAPCodeResponse,
		Identifier: 10,
		Type:       config.EAPTypeAKA,
		Subtype:    config.AKASubtypeChallenge,
		Attributes: []EapAttribute{
			{Type: config.ATRes, Value: xres},
			{Type: config.ATMac, Value: mac},
		},
	}

	t.Run("encodes AT_RES with correct bit-length prefix", func(t *testing.T) {
		buf := EncodeEapPacket(responsePacket)
		// Header(8) + AT_RES(12) + AT_MAC(20) = 40
		assert.Len(t, buf, 40)
	})

	t.Run("round-trips AT_RES value", func(t *testing.T) {
		buf := EncodeEapPacket(responsePacket)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)

		decodedRes := findAttr(decoded.Attributes, config.ATRes)
		require.NotNil(t, decodedRes)
		assert.Equal(t, xres, decodedRes.Value)
		assert.Len(t, decodedRes.Value, 8)
	})

	t.Run("round-trips AT_MAC value", func(t *testing.T) {
		buf := EncodeEapPacket(responsePacket)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)

		decodedMac := findAttr(decoded.Attributes, config.ATMac)
		require.NotNil(t, decodedMac)
		assert.Equal(t, mac, decodedMac.Value)
	})
}

func TestBase64(t *testing.T) {
	t.Run("round-trips through base64", func(t *testing.T) {
		pkt := EapPacket{
			Code:       config.EAPCodeRequest,
			Identifier: 5,
			Type:       config.EAPTypeAKA,
			Subtype:    config.AKASubtypeIdentity,
			Attributes: nil,
		}

		b64 := EncodeEapToBase64(pkt)
		assert.NotEmpty(t, b64)

		decoded, err := DecodeEapFromBase64(b64)
		require.NoError(t, err)
		assert.Equal(t, config.EAPCodeRequest, decoded.Code)
		assert.Equal(t, 5, decoded.Identifier)
		assert.Equal(t, config.AKASubtypeIdentity, decoded.Subtype)
	})
}

func TestReauthentication(t *testing.T) {
	iv := bytes.Repeat([]byte{0x11}, 16)
	ciphertext := bytes.Repeat([]byte{0x22}, 32)
	mac := bytes.Repeat([]byte{0x33}, 16)

	reauthPacket := EapPacket{
		Code:       config.EAPCodeRequest,
		Identifier: 20,
		Type:       config.EAPTypeAKA,
		Subtype:    config.AKASubtypeReauthentication,
		Attributes: []EapAttribute{
			{Type: config.ATIV, Value: iv},
			{Type: config.ATEncrData, Value: ciphertext},
			{Type: config.ATMac, Value: mac},
		},
	}

	t.Run("round-trips AT_IV", func(t *testing.T) {
		buf := EncodeEapPacket(reauthPacket)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)
		decodedIv := findAttr(decoded.Attributes, config.ATIV)
		require.NotNil(t, decodedIv)
		assert.Equal(t, iv, decodedIv.Value)
		assert.Len(t, decodedIv.Value, 16)
	})

	t.Run("round-trips AT_ENCR_DATA", func(t *testing.T) {
		buf := EncodeEapPacket(reauthPacket)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)
		decodedEncr := findAttr(decoded.Attributes, config.ATEncrData)
		require.NotNil(t, decodedEncr)
		assert.Equal(t, ciphertext, decodedEncr.Value)
	})

	t.Run("round-trips AT_MAC in reauthentication packet", func(t *testing.T) {
		buf := EncodeEapPacket(reauthPacket)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)
		decodedMac := findAttr(decoded.Attributes, config.ATMac)
		require.NotNil(t, decodedMac)
		assert.Equal(t, mac, decodedMac.Value)
	})

	t.Run("uses REAUTHENTICATION subtype (13)", func(t *testing.T) {
		buf := EncodeEapPacket(reauthPacket)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)
		assert.Equal(t, config.AKASubtypeReauthentication, decoded.Subtype)
		assert.Equal(t, 13, decoded.Subtype)
	})
}

func TestATCounter(t *testing.T) {
	t.Run("round-trips counter value", func(t *testing.T) {
		counterBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(counterBuf, 1234)

		pkt := EapPacket{
			Code:       config.EAPCodeResponse,
			Identifier: 5,
			Type:       config.EAPTypeAKA,
			Subtype:    config.AKASubtypeReauthentication,
			Attributes: []EapAttribute{
				{Type: config.ATCounter, Value: counterBuf},
			},
		}

		buf := EncodeEapPacket(pkt)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)
		decodedCounter := findAttr(decoded.Attributes, config.ATCounter)
		require.NotNil(t, decodedCounter)
		assert.Equal(t, uint16(1234), binary.BigEndian.Uint16(decodedCounter.Value))
	})

	t.Run("encodes AT_COUNTER to 4 bytes total (length=1)", func(t *testing.T) {
		counterBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(counterBuf, 0)

		pkt := EapPacket{
			Code:       config.EAPCodeResponse,
			Identifier: 1,
			Type:       config.EAPTypeAKA,
			Subtype:    config.AKASubtypeReauthentication,
			Attributes: []EapAttribute{
				{Type: config.ATCounter, Value: counterBuf},
			},
		}

		buf := EncodeEapPacket(pkt)
		// Header(8) + AT_COUNTER(4) = 12
		assert.Len(t, buf, 12)
	})
}

func TestATCounterTooSmall(t *testing.T) {
	t.Run("round-trips (empty value)", func(t *testing.T) {
		pkt := EapPacket{
			Code:       config.EAPCodeResponse,
			Identifier: 5,
			Type:       config.EAPTypeAKA,
			Subtype:    config.AKASubtypeReauthentication,
			Attributes: []EapAttribute{
				{Type: config.ATCounterTooSmall, Value: []byte{}},
			},
		}

		buf := EncodeEapPacket(pkt)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)
		decodedAttr := findAttr(decoded.Attributes, config.ATCounterTooSmall)
		require.NotNil(t, decodedAttr)
		assert.Len(t, decodedAttr.Value, 0)
	})
}

func TestATNonceS(t *testing.T) {
	t.Run("round-trips 16-byte nonce", func(t *testing.T) {
		nonceS := bytes.Repeat([]byte{0x55}, 16)

		pkt := EapPacket{
			Code:       config.EAPCodeRequest,
			Identifier: 7,
			Type:       config.EAPTypeAKA,
			Subtype:    config.AKASubtypeReauthentication,
			Attributes: []EapAttribute{
				{Type: config.ATNonceS, Value: nonceS},
			},
		}

		buf := EncodeEapPacket(pkt)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)
		decodedNonce := findAttr(decoded.Attributes, config.ATNonceS)
		require.NotNil(t, decodedNonce)
		assert.Equal(t, nonceS, decodedNonce.Value)
		assert.Len(t, decodedNonce.Value, 16)
	})
}

func TestATNextReauthID(t *testing.T) {
	t.Run("round-trips UTF-8 identity string", func(t *testing.T) {
		identity := "test-reauth-id-abc123"
		identityBuf := []byte(identity)

		pkt := EapPacket{
			Code:       config.EAPCodeRequest,
			Identifier: 8,
			Type:       config.EAPTypeAKA,
			Subtype:    config.AKASubtypeReauthentication,
			Attributes: []EapAttribute{
				{Type: config.ATNextReauthID, Value: identityBuf},
			},
		}

		buf := EncodeEapPacket(pkt)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)
		decodedID := findAttr(decoded.Attributes, config.ATNextReauthID)
		require.NotNil(t, decodedID)
		assert.Equal(t, identity, string(decodedID.Value))
	})

	t.Run("handles short identity with padding correctly", func(t *testing.T) {
		identity := "a"
		identityBuf := []byte(identity)

		pkt := EapPacket{
			Code:       config.EAPCodeRequest,
			Identifier: 9,
			Type:       config.EAPTypeAKA,
			Subtype:    config.AKASubtypeReauthentication,
			Attributes: []EapAttribute{
				{Type: config.ATNextReauthID, Value: identityBuf},
			},
		}

		buf := EncodeEapPacket(pkt)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)
		decodedID := findAttr(decoded.Attributes, config.ATNextReauthID)
		require.NotNil(t, decodedID)
		assert.Equal(t, "a", string(decodedID.Value))
	})
}

func TestMalformedPackets(t *testing.T) {
	t.Run("rejects packets shorter than 4 bytes", func(t *testing.T) {
		_, err := DecodeEapPacket(make([]byte, 2))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too short")
	})

	t.Run("rejects truncated packets", func(t *testing.T) {
		buf := make([]byte, 4)
		buf[0] = byte(config.EAPCodeRequest)
		buf[1] = 1
		binary.BigEndian.PutUint16(buf[2:4], 20) // claims 20 bytes but only 4
		_, err := DecodeEapPacket(buf)
		assert.Error(t, err)
	})
}
