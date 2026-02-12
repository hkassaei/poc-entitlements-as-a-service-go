// Package crypto implements MILENAGE (3GPP TS 35.206) and envelope encryption.
package crypto

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
)

// MILENAGE constants (3GPP TS 35.206 Section 4.1)
var (
	c1 = [16]byte{} // 0x00...00
	c2 = [16]byte{15: 0x01}
	c3 = [16]byte{15: 0x02}
	c4 = [16]byte{15: 0x04}
	c5 = [16]byte{15: 0x08}

	// Resync constants
	c1Star = [16]byte{15: 0x80}
	c5Star = [16]byte{15: 0x10}
)

// Rotation constants
const (
	r1     = 64
	r2     = 0
	r3     = 32
	r4     = 64
	r1Star = 64
	r5Star = 0
)

// AuthVectors holds the authentication vectors produced by MILENAGE.
type AuthVectors struct {
	RAND []byte // 16 bytes
	AUTN []byte // 16 bytes
	XRES []byte // 8 bytes
	CK   []byte // 16 bytes
	IK   []byte // 16 bytes
	AK   []byte // 6 bytes
}

// AESEncrypt performs single AES-128-ECB block encryption (16 bytes in, 16 bytes out).
func AESEncrypt(key, input []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic("aes.NewCipher: " + err.Error())
	}
	out := make([]byte, 16)
	block.Encrypt(out, input)
	return out
}

// XOR two equal-length byte slices.
func XOR(a, b []byte) []byte {
	result := make([]byte, len(a))
	for i := range a {
		result[i] = a[i] ^ b[i]
	}
	return result
}

// Rotate performs circular left rotation of a 128-bit (16-byte) value by bits bits.
func Rotate(buf []byte, bits int) []byte {
	byteShift := (bits >> 3) % 16
	bitShift := uint(bits & 7)
	result := make([]byte, 16)

	for i := 0; i < 16; i++ {
		srcIdx := (i + byteShift) % 16
		nextIdx := (i + byteShift + 1) % 16
		if bitShift == 0 {
			result[i] = buf[srcIdx]
		} else {
			result[i] = (buf[srcIdx]<<bitShift | buf[nextIdx]>>(8-bitShift)) & 0xff
		}
	}
	return result
}

// ComputeOPc computes OPc = AES_K(OP) XOR OP.
func ComputeOPc(ki, op []byte) []byte {
	return XOR(AESEncrypt(ki, op), op)
}

// GenerateVectors generates authentication vectors with a random RAND.
func GenerateVectors(ki, op, sqn, amf []byte) (*AuthVectors, error) {
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return nil, fmt.Errorf("crypto/rand: %w", err)
	}
	return GenerateVectorsWithRAND(ki, op, randBytes, sqn, amf), nil
}

// GenerateVectorsWithRAND generates authentication vectors with a given RAND (deterministic, for testing).
func GenerateVectorsWithRAND(ki, op, randVal, sqn, amf []byte) *AuthVectors {
	opc := ComputeOPc(ki, op)

	// TEMP = AES_K(RAND XOR OPc)
	temp := AESEncrypt(ki, XOR(randVal, opc))

	// --- f1: MAC-A ---
	// IN1 = SQN || AMF || SQN || AMF (16 bytes)
	in1 := make([]byte, 16)
	copy(in1[0:], sqn[:6])
	copy(in1[6:], amf[:2])
	copy(in1[8:], sqn[:6])
	copy(in1[14:], amf[:2])

	// OUT1 = AES_K(rotate(IN1 XOR OPc, r1) XOR TEMP XOR c1) XOR OPc
	f1Input := XOR(XOR(Rotate(XOR(in1, opc), r1), temp), c1[:])
	out1 := XOR(AESEncrypt(ki, f1Input), opc)
	macA := make([]byte, 8)
	copy(macA, out1[:8])

	// --- f2 (RES) + f5 (AK) ---
	f2Input := XOR(Rotate(XOR(temp, opc), r2), c2[:])
	out2 := XOR(AESEncrypt(ki, f2Input), opc)
	xres := make([]byte, 8)
	copy(xres, out2[8:16])
	ak := make([]byte, 6)
	copy(ak, out2[:6])

	// --- f3 (CK) ---
	f3Input := XOR(Rotate(XOR(temp, opc), r3), c3[:])
	out3 := XOR(AESEncrypt(ki, f3Input), opc)
	ck := make([]byte, 16)
	copy(ck, out3[:16])

	// --- f4 (IK) ---
	f4Input := XOR(Rotate(XOR(temp, opc), r4), c4[:])
	out4 := XOR(AESEncrypt(ki, f4Input), opc)
	ik := make([]byte, 16)
	copy(ik, out4[:16])

	// AUTN = (SQN XOR AK) || AMF || MAC-A
	autn := make([]byte, 16)
	sqnXorAk := XOR(sqn[:6], ak)
	copy(autn[0:], sqnXorAk)
	copy(autn[6:], amf[:2])
	copy(autn[8:], macA)

	return &AuthVectors{
		RAND: append([]byte(nil), randVal...),
		AUTN: autn,
		XRES: xres,
		CK:   ck,
		IK:   ik,
		AK:   ak,
	}
}

// F5Star computes the Anonymity Key for Resync (3GPP TS 35.206 Section 4.1).
// Uses R5* = 0 and C5*[15] = 0x10.
func F5Star(ki, randVal, op []byte) []byte {
	opc := ComputeOPc(ki, op)
	temp := AESEncrypt(ki, XOR(randVal, opc))

	f5StarInput := XOR(Rotate(XOR(temp, opc), r5Star), c5Star[:])
	out5Star := XOR(AESEncrypt(ki, f5StarInput), opc)

	result := make([]byte, 6)
	copy(result, out5Star[:6])
	return result
}

// F1Star computes MAC-S for Resync (3GPP TS 35.206 Section 4.1).
// Uses R1* = 64 and C1*[15] = 0x80.
func F1Star(ki, randVal, sqn, amf, op []byte) []byte {
	opc := ComputeOPc(ki, op)
	temp := AESEncrypt(ki, XOR(randVal, opc))

	in1 := make([]byte, 16)
	copy(in1[0:], sqn[:6])
	copy(in1[6:], amf[:2])
	copy(in1[8:], sqn[:6])
	copy(in1[14:], amf[:2])

	f1StarInput := XOR(XOR(Rotate(XOR(in1, opc), r1Star), temp), c1Star[:])
	out1Star := XOR(AESEncrypt(ki, f1StarInput), opc)

	result := make([]byte, 8)
	copy(result, out1Star[:8])
	return result
}

// AuTsValidationResult is the result of AUTS validation.
type AuTsValidationResult struct {
	Valid bool
	SqnMs []byte // 6 bytes, only set if Valid
}

// ValidateAUTS validates AUTS and extracts SQN_MS (3GPP TS 35.206 Section 6.3.3).
func ValidateAUTS(ki, randVal, auts, op []byte) AuTsValidationResult {
	if len(auts) != 14 {
		return AuTsValidationResult{Valid: false}
	}

	concealedSqn := auts[:6]
	receivedMacS := auts[6:14]

	akStar := F5Star(ki, randVal, op)
	sqnMs := XOR(concealedSqn, akStar)

	amfZero := make([]byte, 2)
	expectedMacS := F1Star(ki, randVal, sqnMs, amfZero, op)

	if subtle.ConstantTimeCompare(receivedMacS, expectedMacS) != 1 {
		return AuTsValidationResult{Valid: false}
	}

	return AuTsValidationResult{Valid: true, SqnMs: append([]byte(nil), sqnMs...)}
}

// GenerateAUTS generates AUTS for testing — simulates a device generating AUTS
// when its SQN is out of sync with the network.
func GenerateAUTS(ki, randVal, sqnMs, op []byte) []byte {
	akStar := F5Star(ki, randVal, op)
	concealedSqn := XOR(sqnMs, akStar)

	amfZero := make([]byte, 2)
	macS := F1Star(ki, randVal, sqnMs, amfZero, op)

	auts := make([]byte, 14)
	copy(auts[:6], concealedSqn)
	copy(auts[6:], macS)
	return auts
}
