package eapaka

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test Vector Set 1: Derived from 3GPP TS 35.207 Test Set 1
var tv1 = struct {
	ki, op, randVal, sqn, amf []byte
	opc, xres, ck, ik, ak     []byte
	macA                      []byte
	imsi                      string
}{
	ki:      mustDecodeHex("465b5ce8b199b49faa5f0a2ee238a6bc"),
	op:      mustDecodeHex("cdc202d5123e20f62b6d676ac72cb318"),
	randVal: mustDecodeHex("23553cbe9637a89d218ae64dae47bf35"),
	sqn:     mustDecodeHex("ff9bb4d0b607"),
	amf:     mustDecodeHex("b9b9"),
	opc:     mustDecodeHex("cd63cb71954a9f4e48a5994e37a02baf"),
	xres:    mustDecodeHex("a54211d5e3ba50bf"),
	ck:      mustDecodeHex("b40ba9a3c58b2a05bbf0d987b21bf8cb"),
	ik:      mustDecodeHex("f769bcd751044604127672711c6d3441"),
	ak:      mustDecodeHex("aa689c648370"),
	macA:    mustDecodeHex("4a9ffac354dfafb3"),
	imsi:    "001010000000001",
}

func mustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// Section 8.1: Message Format
func TestRFC4187Section8_1(t *testing.T) {
	t.Run("EAP header is exactly 4 bytes", func(t *testing.T) {
		pkt := EapPacket{Code: config.EAPCodeSuccess, Identifier: 0x42}
		buf := EncodeEapPacket(pkt)

		assert.Len(t, buf, 4)
		assert.Equal(t, byte(config.EAPCodeSuccess), buf[0])
		assert.Equal(t, byte(0x42), buf[1])
		assert.Equal(t, uint16(4), binary.BigEndian.Uint16(buf[2:4]))
	})

	t.Run("EAP-AKA header adds Type+Subtype+Reserved = 8 bytes total", func(t *testing.T) {
		pkt := EapPacket{
			Code: config.EAPCodeRequest, Identifier: 1,
			Type: config.EAPTypeAKA, Subtype: config.AKASubtypeChallenge,
		}
		buf := EncodeEapPacket(pkt)

		assert.Len(t, buf, 8)
		assert.Equal(t, byte(config.EAPCodeRequest), buf[0])
		assert.Equal(t, byte(1), buf[1])
		assert.Equal(t, uint16(8), binary.BigEndian.Uint16(buf[2:4]))
		assert.Equal(t, byte(config.EAPTypeAKA), buf[4])
		assert.Equal(t, byte(config.AKASubtypeChallenge), buf[5])
		assert.Equal(t, byte(0), buf[6])
		assert.Equal(t, byte(0), buf[7])
	})

	t.Run("Type field is 23 for EAP-AKA", func(t *testing.T) {
		assert.Equal(t, 23, config.EAPTypeAKA)
	})

	t.Run("Subtype values match RFC 4187 Section 11", func(t *testing.T) {
		assert.Equal(t, 1, config.AKASubtypeChallenge)
		assert.Equal(t, 2, config.AKASubtypeAuthReject)
		assert.Equal(t, 4, config.AKASubtypeSyncFailure)
		assert.Equal(t, 5, config.AKASubtypeIdentity)
		assert.Equal(t, 13, config.AKASubtypeReauthentication)
	})
}

// Section 10: Attribute TLV Format
func TestRFC4187Section10(t *testing.T) {
	t.Run("AT_RAND: type=1, length=5, 2 reserved + 16 value", func(t *testing.T) {
		randVal := bytes.Repeat([]byte{0xaa}, 16)
		attr := EncodeAttribute(EapAttribute{Type: config.ATRand, Value: randVal})

		assert.Len(t, attr, 20)
		assert.Equal(t, byte(config.ATRand), attr[0])
		assert.Equal(t, byte(5), attr[1])
		assert.Equal(t, byte(0), attr[2])
		assert.Equal(t, byte(0), attr[3])
		assert.Equal(t, randVal, attr[4:20])
	})

	t.Run("AT_AUTN: type=2, length=5, 2 reserved + 16 value", func(t *testing.T) {
		autn := bytes.Repeat([]byte{0xbb}, 16)
		attr := EncodeAttribute(EapAttribute{Type: config.ATAutn, Value: autn})

		assert.Len(t, attr, 20)
		assert.Equal(t, byte(config.ATAutn), attr[0])
		assert.Equal(t, byte(5), attr[1])
		assert.Equal(t, byte(0), attr[2])
		assert.Equal(t, byte(0), attr[3])
		assert.Equal(t, autn, attr[4:20])
	})

	t.Run("AT_RES includes 2-byte bit-length prefix", func(t *testing.T) {
		res := tv1.xres // 8 bytes
		attr := EncodeAttribute(EapAttribute{Type: config.ATRes, Value: res})

		assert.Equal(t, byte(config.ATRes), attr[0])
		assert.Equal(t, byte(3), attr[1]) // ceil((2+2+8)/4) = 3
		assert.Equal(t, uint16(64), binary.BigEndian.Uint16(attr[2:4]))
		assert.Equal(t, res, attr[4:12])
	})

	t.Run("AT_RES bit-length is always value.length * 8", func(t *testing.T) {
		testCases := []struct {
			len          int
			expectedBits uint16
		}{
			{4, 32},
			{8, 64},
			{16, 128},
		}
		for _, tc := range testCases {
			res := bytes.Repeat([]byte{0xcc}, tc.len)
			attr := EncodeAttribute(EapAttribute{Type: config.ATRes, Value: res})
			assert.Equal(t, tc.expectedBits, binary.BigEndian.Uint16(attr[2:4]))
		}
	})

	t.Run("AT_AUTS: type=4, 2 reserved + 14 value", func(t *testing.T) {
		auts := bytes.Repeat([]byte{0xdd}, 14)
		attr := EncodeAttribute(EapAttribute{Type: config.ATAuts, Value: auts})

		assert.Len(t, attr, 18)
		assert.Equal(t, byte(config.ATAuts), attr[0])
		assert.Equal(t, byte(4), attr[1])
		assert.Equal(t, byte(0), attr[2])
		assert.Equal(t, byte(0), attr[3])
		assert.Equal(t, auts, attr[4:18])
	})

	t.Run("AT_MAC: type=11, length=5, 2 reserved + 16 MAC", func(t *testing.T) {
		mac := bytes.Repeat([]byte{0xee}, 16)
		attr := EncodeAttribute(EapAttribute{Type: config.ATMac, Value: mac})

		assert.Len(t, attr, 20)
		assert.Equal(t, byte(config.ATMac), attr[0])
		assert.Equal(t, byte(5), attr[1])
		assert.Equal(t, byte(0), attr[2])
		assert.Equal(t, byte(0), attr[3])
		assert.Equal(t, mac, attr[4:20])
	})

	t.Run("AT_COUNTER: type=19, length=1, 2-byte BE counter", func(t *testing.T) {
		counter := make([]byte, 2)
		binary.BigEndian.PutUint16(counter, 1234)
		attr := EncodeAttribute(EapAttribute{Type: config.ATCounter, Value: counter})

		assert.Len(t, attr, 4)
		assert.Equal(t, byte(config.ATCounter), attr[0])
		assert.Equal(t, byte(1), attr[1])
		assert.Equal(t, uint16(1234), binary.BigEndian.Uint16(attr[2:4]))
	})

	t.Run("AT_COUNTER_TOO_SMALL: type=20, length=1", func(t *testing.T) {
		attr := EncodeAttribute(EapAttribute{Type: config.ATCounterTooSmall, Value: []byte{}})

		assert.Len(t, attr, 4)
		assert.Equal(t, byte(config.ATCounterTooSmall), attr[0])
		assert.Equal(t, byte(1), attr[1])
		assert.Equal(t, byte(0), attr[2])
		assert.Equal(t, byte(0), attr[3])
	})

	t.Run("AT_NONCE_S: type=21, length=5, 2 reserved + 16 nonce", func(t *testing.T) {
		nonceS := make([]byte, 16)
		_, _ = rand.Read(nonceS)
		attr := EncodeAttribute(EapAttribute{Type: config.ATNonceS, Value: nonceS})

		assert.Len(t, attr, 20)
		assert.Equal(t, byte(config.ATNonceS), attr[0])
		assert.Equal(t, byte(5), attr[1])
		assert.Equal(t, nonceS, attr[4:20])
	})
}

// Section 7: Key Derivation
func TestRFC4187Section7(t *testing.T) {
	t.Run("MK = SHA-1(Identity | IK | CK)", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		mk := DeriveMasterKey(identity, tv1.ik, tv1.ck)

		h := sha1.New()
		h.Write([]byte(identity))
		h.Write(tv1.ik)
		h.Write(tv1.ck)
		expected := h.Sum(nil)

		assert.Len(t, mk, 20)
		assert.Equal(t, hex.EncodeToString(expected), hex.EncodeToString(mk))
	})

	t.Run("Identity format is 0 + IMSI", func(t *testing.T) {
		identity := BuildIdentity("001010000000001")
		assert.Equal(t, "0001010000000001", identity)
		assert.Equal(t, byte('0'), identity[0])
	})

	t.Run("PRF produces deterministic output", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		mk := DeriveMasterKey(identity, tv1.ik, tv1.ck)

		out1 := PrfSHA1(mk, 160)
		out2 := PrfSHA1(mk, 160)
		assert.Equal(t, hex.EncodeToString(out1), hex.EncodeToString(out2))
	})

	t.Run("PRF output is 160 bytes", func(t *testing.T) {
		mk := make([]byte, 20)
		_, _ = rand.Read(mk)
		output := PrfSHA1(mk, 160)
		assert.Len(t, output, 160)
	})

	t.Run("PRF handles arbitrary output lengths", func(t *testing.T) {
		mk := make([]byte, 20)
		_, _ = rand.Read(mk)
		assert.Len(t, PrfSHA1(mk, 16), 16)
		assert.Len(t, PrfSHA1(mk, 32), 32)
		assert.Len(t, PrfSHA1(mk, 64), 64)
		assert.Len(t, PrfSHA1(mk, 128), 128)
		assert.Len(t, PrfSHA1(mk, 200), 200)
	})

	t.Run("deriveKeys produces correct key sizes", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		keys := DeriveKeys(identity, tv1.ik, tv1.ck)
		assert.Len(t, keys.KEncr, 16)
		assert.Len(t, keys.KAut, 16)
		assert.Len(t, keys.MSK, 64)
		assert.Len(t, keys.EMSK, 64)
	})

	t.Run("K_encr is bytes 0-15 of PRF output", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		mk := DeriveMasterKey(identity, tv1.ik, tv1.ck)
		prf := PrfSHA1(mk, 160)
		keys := DeriveKeys(identity, tv1.ik, tv1.ck)
		assert.Equal(t, hex.EncodeToString(prf[0:16]), hex.EncodeToString(keys.KEncr))
	})

	t.Run("K_aut is bytes 16-31 of PRF output", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		mk := DeriveMasterKey(identity, tv1.ik, tv1.ck)
		prf := PrfSHA1(mk, 160)
		keys := DeriveKeys(identity, tv1.ik, tv1.ck)
		assert.Equal(t, hex.EncodeToString(prf[16:32]), hex.EncodeToString(keys.KAut))
	})

	t.Run("MSK is bytes 32-95 of PRF output", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		mk := DeriveMasterKey(identity, tv1.ik, tv1.ck)
		prf := PrfSHA1(mk, 160)
		keys := DeriveKeys(identity, tv1.ik, tv1.ck)
		assert.Equal(t, hex.EncodeToString(prf[32:96]), hex.EncodeToString(keys.MSK))
	})

	t.Run("EMSK is bytes 96-159 of PRF output", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		mk := DeriveMasterKey(identity, tv1.ik, tv1.ck)
		prf := PrfSHA1(mk, 160)
		keys := DeriveKeys(identity, tv1.ik, tv1.ck)
		assert.Equal(t, hex.EncodeToString(prf[96:160]), hex.EncodeToString(keys.EMSK))
	})

	t.Run("Re-auth key derivation", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		mk := DeriveMasterKey(identity, tv1.ik, tv1.ck)
		counter := 1
		nonceS := make([]byte, 16)
		_, _ = rand.Read(nonceS)

		msk, emsk := DeriveReauthKeys(identity, counter, nonceS, mk)
		assert.Len(t, msk, 64)
		assert.Len(t, emsk, 64)
	})

	t.Run("Different counters produce different re-auth keys", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		mk := DeriveMasterKey(identity, tv1.ik, tv1.ck)
		nonceS := make([]byte, 16)
		_, _ = rand.Read(nonceS)

		msk1, _ := DeriveReauthKeys(identity, 1, nonceS, mk)
		msk2, _ := DeriveReauthKeys(identity, 2, nonceS, mk)
		assert.NotEqual(t, hex.EncodeToString(msk1), hex.EncodeToString(msk2))
	})

	t.Run("Different NONCE_S produce different re-auth keys", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		mk := DeriveMasterKey(identity, tv1.ik, tv1.ck)

		nonceS1 := make([]byte, 16)
		nonceS2 := make([]byte, 16)
		_, _ = rand.Read(nonceS1)
		_, _ = rand.Read(nonceS2)

		msk1, _ := DeriveReauthKeys(identity, 1, nonceS1, mk)
		msk2, _ := DeriveReauthKeys(identity, 1, nonceS2, mk)
		assert.NotEqual(t, hex.EncodeToString(msk1), hex.EncodeToString(msk2))
	})
}

// Section 10.15: AT_MAC Computation
func TestRFC4187Section10_15(t *testing.T) {
	t.Run("AT_MAC uses HMAC-SHA-1 truncated to 16 bytes", func(t *testing.T) {
		kAut := make([]byte, 16)
		data := make([]byte, 100)
		_, _ = rand.Read(kAut)
		_, _ = rand.Read(data)

		mac := ComputeMAC(kAut, data)
		assert.Len(t, mac, 16)

		// Verify it's the first 16 bytes of HMAC-SHA-1
		fullHMAC := hmac.New(sha1.New, kAut)
		fullHMAC.Write(data)
		fullDigest := fullHMAC.Sum(nil)
		assert.Equal(t, hex.EncodeToString(fullDigest[:16]), hex.EncodeToString(mac))
	})

	t.Run("AT_MAC computation covers entire packet with MAC zeroed", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		keys := DeriveKeys(identity, tv1.ik, tv1.ck)

		zeroMAC := make([]byte, 16)
		pkt := EapPacket{
			Code: config.EAPCodeRequest, Identifier: 1,
			Type: config.EAPTypeAKA, Subtype: config.AKASubtypeChallenge,
			Attributes: []EapAttribute{
				{Type: config.ATRand, Value: tv1.randVal},
				{Type: config.ATAutn, Value: bytes.Repeat([]byte{0xbb}, 16)},
				{Type: config.ATMac, Value: zeroMAC},
			},
		}

		packetBytes := EncodeEapPacket(pkt)
		mac := ComputeMAC(keys.KAut, packetBytes)
		assert.Len(t, mac, 16)

		mac2 := ComputeMAC(keys.KAut, packetBytes)
		assert.Equal(t, hex.EncodeToString(mac), hex.EncodeToString(mac2))
	})

	t.Run("verifyMAC returns true for valid MAC", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		keys := DeriveKeys(identity, tv1.ik, tv1.ck)

		pkt := EapPacket{
			Code: config.EAPCodeRequest, Identifier: 1,
			Type: config.EAPTypeAKA, Subtype: config.AKASubtypeChallenge,
			Attributes: []EapAttribute{
				{Type: config.ATRand, Value: tv1.randVal},
				{Type: config.ATAutn, Value: bytes.Repeat([]byte{0xbb}, 16)},
				{Type: config.ATMac, Value: make([]byte, 16)},
			},
		}

		packetBytes := EncodeEapPacket(pkt)
		mac := ComputeMAC(keys.KAut, packetBytes)
		macOffset := findMACOffset(packetBytes)
		copy(packetBytes[macOffset+4:], mac)

		assert.True(t, VerifyMAC(keys.KAut, packetBytes, macOffset+4, mac))
	})

	t.Run("verifyMAC returns false for tampered MAC", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		keys := DeriveKeys(identity, tv1.ik, tv1.ck)

		pkt := EapPacket{
			Code: config.EAPCodeRequest, Identifier: 1,
			Type: config.EAPTypeAKA, Subtype: config.AKASubtypeChallenge,
			Attributes: []EapAttribute{
				{Type: config.ATRand, Value: tv1.randVal},
				{Type: config.ATMac, Value: make([]byte, 16)},
			},
		}

		packetBytes := EncodeEapPacket(pkt)
		mac := ComputeMAC(keys.KAut, packetBytes)
		macOffset := findMACOffset(packetBytes)
		copy(packetBytes[macOffset+4:], mac)

		tamperedMAC := make([]byte, 16)
		copy(tamperedMAC, mac)
		tamperedMAC[0] ^= 0xff

		assert.False(t, VerifyMAC(keys.KAut, packetBytes, macOffset+4, tamperedMAC))
	})

	t.Run("verifyMAC returns false for wrong K_aut", func(t *testing.T) {
		identity := BuildIdentity(tv1.imsi)
		keys := DeriveKeys(identity, tv1.ik, tv1.ck)
		wrongKAut := make([]byte, 16)
		_, _ = rand.Read(wrongKAut)

		pkt := EapPacket{
			Code: config.EAPCodeRequest, Identifier: 1,
			Type: config.EAPTypeAKA, Subtype: config.AKASubtypeChallenge,
			Attributes: []EapAttribute{
				{Type: config.ATRand, Value: tv1.randVal},
				{Type: config.ATMac, Value: make([]byte, 16)},
			},
		}

		packetBytes := EncodeEapPacket(pkt)
		mac := ComputeMAC(keys.KAut, packetBytes)
		macOffset := findMACOffset(packetBytes)
		copy(packetBytes[macOffset+4:], mac)

		assert.False(t, VerifyMAC(wrongKAut, packetBytes, macOffset+4, mac))
	})
}

// Complete Packet Wire Format
func TestCompletePacketWireFormat(t *testing.T) {
	t.Run("AKA-Challenge packet has correct byte-level structure", func(t *testing.T) {
		pkt := EapPacket{
			Code: config.EAPCodeRequest, Identifier: 0x01,
			Type: config.EAPTypeAKA, Subtype: config.AKASubtypeChallenge,
			Attributes: []EapAttribute{
				{Type: config.ATRand, Value: tv1.randVal},
				{Type: config.ATAutn, Value: bytes.Repeat([]byte{0xbb}, 16)},
				{Type: config.ATMac, Value: make([]byte, 16)},
			},
		}

		buf := EncodeEapPacket(pkt)

		// Total: Header(8) + AT_RAND(20) + AT_AUTN(20) + AT_MAC(20) = 68
		assert.Len(t, buf, 68)
		assert.Equal(t, byte(config.EAPCodeRequest), buf[0])
		assert.Equal(t, byte(0x01), buf[1])
		assert.Equal(t, uint16(68), binary.BigEndian.Uint16(buf[2:4]))
		assert.Equal(t, byte(config.EAPTypeAKA), buf[4])
		assert.Equal(t, byte(config.AKASubtypeChallenge), buf[5])
		assert.Equal(t, byte(0), buf[6])
		assert.Equal(t, byte(0), buf[7])

		// AT_RAND at offset 8
		assert.Equal(t, byte(config.ATRand), buf[8])
		assert.Equal(t, byte(5), buf[9])
		assert.Equal(t, byte(0), buf[10])
		assert.Equal(t, byte(0), buf[11])
		assert.Equal(t, tv1.randVal, buf[12:28])

		// AT_AUTN at offset 28
		assert.Equal(t, byte(config.ATAutn), buf[28])
		assert.Equal(t, byte(5), buf[29])

		// AT_MAC at offset 48
		assert.Equal(t, byte(config.ATMac), buf[48])
		assert.Equal(t, byte(5), buf[49])
	})

	t.Run("AKA-Challenge response has correct structure", func(t *testing.T) {
		pkt := EapPacket{
			Code: config.EAPCodeResponse, Identifier: 0x01,
			Type: config.EAPTypeAKA, Subtype: config.AKASubtypeChallenge,
			Attributes: []EapAttribute{
				{Type: config.ATRes, Value: tv1.xres},
				{Type: config.ATMac, Value: make([]byte, 16)},
			},
		}

		buf := EncodeEapPacket(pkt)

		// Header(8) + AT_RES(12) + AT_MAC(20) = 40
		assert.Len(t, buf, 40)
		assert.Equal(t, byte(config.EAPCodeResponse), buf[0])
		assert.Equal(t, uint16(40), binary.BigEndian.Uint16(buf[2:4]))

		// AT_RES at offset 8
		assert.Equal(t, byte(config.ATRes), buf[8])
		assert.Equal(t, uint16(64), binary.BigEndian.Uint16(buf[10:12]))
	})

	t.Run("SYNC_FAILURE packet has AT_AUTS", func(t *testing.T) {
		auts := bytes.Repeat([]byte{0xee}, 14)
		pkt := EapPacket{
			Code: config.EAPCodeResponse, Identifier: 0x01,
			Type: config.EAPTypeAKA, Subtype: config.AKASubtypeSyncFailure,
			Attributes: []EapAttribute{{Type: config.ATAuts, Value: auts}},
		}

		buf := EncodeEapPacket(pkt)

		// Header(8) + AT_AUTS(18) = 26
		assert.Len(t, buf, 26)
		assert.Equal(t, byte(config.AKASubtypeSyncFailure), buf[5])
		assert.Equal(t, byte(config.ATAuts), buf[8])
		assert.Equal(t, byte(4), buf[9])
	})

	t.Run("Decode reverses encode exactly", func(t *testing.T) {
		original := EapPacket{
			Code: config.EAPCodeRequest, Identifier: 42,
			Type: config.EAPTypeAKA, Subtype: config.AKASubtypeChallenge,
			Attributes: []EapAttribute{
				{Type: config.ATRand, Value: tv1.randVal},
				{Type: config.ATAutn, Value: bytes.Repeat([]byte{0xcc}, 16)},
				{Type: config.ATMac, Value: bytes.Repeat([]byte{0xdd}, 16)},
			},
		}

		encoded := EncodeEapPacket(original)
		decoded, err := DecodeEapPacket(encoded)
		require.NoError(t, err)

		assert.Equal(t, original.Code, decoded.Code)
		assert.Equal(t, original.Identifier, decoded.Identifier)
		assert.Equal(t, original.Type, decoded.Type)
		assert.Equal(t, original.Subtype, decoded.Subtype)
		assert.Len(t, decoded.Attributes, 3)

		decodedRand := findAttr(decoded.Attributes, config.ATRand)
		assert.Equal(t, hex.EncodeToString(tv1.randVal), hex.EncodeToString(decodedRand.Value))
	})
}

// Section 5.4: Fast Re-authentication
func TestRFC4187Section5_4(t *testing.T) {
	t.Run("Re-authentication subtype is 13", func(t *testing.T) {
		assert.Equal(t, 13, config.AKASubtypeReauthentication)
	})

	t.Run("Re-auth request includes AT_IV, AT_ENCR_DATA, AT_MAC", func(t *testing.T) {
		iv := make([]byte, 16)
		_, _ = rand.Read(iv)
		encrData := make([]byte, 32)
		_, _ = rand.Read(encrData)
		mac := make([]byte, 16)

		pkt := EapPacket{
			Code: config.EAPCodeRequest, Identifier: 1,
			Type: config.EAPTypeAKA, Subtype: config.AKASubtypeReauthentication,
			Attributes: []EapAttribute{
				{Type: config.ATIV, Value: iv},
				{Type: config.ATEncrData, Value: encrData},
				{Type: config.ATMac, Value: mac},
			},
		}

		buf := EncodeEapPacket(pkt)
		decoded, err := DecodeEapPacket(buf)
		require.NoError(t, err)

		assert.Equal(t, config.AKASubtypeReauthentication, decoded.Subtype)
		assert.NotNil(t, findAttr(decoded.Attributes, config.ATIV))
		assert.NotNil(t, findAttr(decoded.Attributes, config.ATEncrData))
		assert.NotNil(t, findAttr(decoded.Attributes, config.ATMac))
	})

	t.Run("AT_COUNTER in re-auth response is 2-byte big-endian", func(t *testing.T) {
		counter := make([]byte, 2)
		binary.BigEndian.PutUint16(counter, 12345)
		attr := EncodeAttribute(EapAttribute{Type: config.ATCounter, Value: counter})
		assert.Equal(t, uint16(12345), binary.BigEndian.Uint16(attr[2:4]))
	})
}

// findMACOffset finds the byte offset of AT_MAC in an encoded packet.
func findMACOffset(buf []byte) int {
	offset := 8
	for offset+2 <= len(buf) {
		attrType := buf[offset]
		attrLen := int(buf[offset+1]) * 4
		if int(attrType) == config.ATMac {
			return offset
		}
		offset += attrLen
	}
	return -1
}
