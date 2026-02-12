package eapaka

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
)

// DerivedKeys holds the session keys derived from EAP-AKA authentication.
type DerivedKeys struct {
	KEncr []byte // 16 bytes — encryption key
	KAut  []byte // 16 bytes — authentication key
	MSK   []byte // 64 bytes — Master Session Key
	EMSK  []byte // 64 bytes — Extended MSK
}

// BuildIdentity builds the permanent identity string for EAP-AKA.
// Type "0" = permanent identity, followed by the IMSI.
func BuildIdentity(imsi string) string {
	return "0" + imsi
}

// DeriveMasterKey derives the Master Key (MK) per RFC 4187 Section 7.
// MK = SHA-1(Identity | IK | CK)
func DeriveMasterKey(identity string, ik, ck []byte) []byte {
	h := sha1.New()
	h.Write([]byte(identity))
	h.Write(ik)
	h.Write(ck)
	return h.Sum(nil)
}

// PrfSHA1 implements the FIPS 186-2 SHA-1-based PRF.
// Generates outputLength bytes of pseudo-random output from a 20-byte seed.
func PrfSHA1(mk []byte, outputLength int) []byte {
	output := make([]byte, outputLength)
	offset := 0

	// xkey is a 20-byte (160-bit) value, initialized from MK
	xkey := make([]byte, 20)
	copy(xkey, mk[:20])

	for offset < outputLength {
		// w = SHA-1(xkey)
		h := sha1.New()
		h.Write(xkey)
		w := h.Sum(nil)

		// Copy w into output
		toCopy := 20
		if outputLength-offset < toCopy {
			toCopy = outputLength - offset
		}
		copy(output[offset:], w[:toCopy])
		offset += toCopy

		// xkey = (xkey + w + 1) mod 2^160
		add160(xkey, w)
		increment160(xkey)
	}

	return output
}

// add160 adds two 20-byte big-endian numbers in-place: a = (a + b) mod 2^160.
func add160(a, b []byte) {
	carry := 0
	for i := 19; i >= 0; i-- {
		sum := int(a[i]) + int(b[i]) + carry
		a[i] = byte(sum & 0xff)
		carry = sum >> 8
	}
}

// increment160 increments a 20-byte big-endian number in-place: a = (a + 1) mod 2^160.
func increment160(a []byte) {
	for i := 19; i >= 0; i-- {
		val := int(a[i]) + 1
		a[i] = byte(val & 0xff)
		if val < 256 {
			break
		}
	}
}

// DeriveKeys derives all session keys from identity, IK, and CK.
func DeriveKeys(identity string, ik, ck []byte) DerivedKeys {
	mk := DeriveMasterKey(identity, ik, ck)
	prfOutput := PrfSHA1(mk, 160)

	return DerivedKeys{
		KEncr: append([]byte(nil), prfOutput[0:16]...),
		KAut:  append([]byte(nil), prfOutput[16:32]...),
		MSK:   append([]byte(nil), prfOutput[32:96]...),
		EMSK:  append([]byte(nil), prfOutput[96:160]...),
	}
}

// ComputeMAC computes the AT_MAC value for an EAP packet.
// MAC = HMAC-SHA-1(K_aut, eapPacketWithZeroedMac) truncated to 16 bytes.
func ComputeMAC(kAut, eapPacketWithZeroMAC []byte) []byte {
	mac := hmac.New(sha1.New, kAut)
	mac.Write(eapPacketWithZeroMAC)
	return mac.Sum(nil)[:16]
}

// VerifyMAC verifies the AT_MAC in a received EAP packet.
// Zeros the MAC field in a copy, recomputes, and compares with timing-safe equality.
func VerifyMAC(kAut, rawPacketBytes []byte, macOffset int, receivedMAC []byte) bool {
	// Create a copy with the MAC field zeroed
	pktCopy := make([]byte, len(rawPacketBytes))
	copy(pktCopy, rawPacketBytes)
	for i := 0; i < 16; i++ {
		pktCopy[macOffset+i] = 0
	}

	computed := ComputeMAC(kAut, pktCopy)
	return subtle.ConstantTimeCompare(computed, receivedMAC) == 1
}

// DeriveReauthKeys derives new session keys for EAP-AKA fast re-authentication.
// Per RFC 4187 Section 7:
// XKEY' = SHA-1(Identity | counter(2 bytes BE) | NONCE_S | MK)
// PRF(XKEY', 128) → MSK'(64) | EMSK'(64)
func DeriveReauthKeys(identity string, counter int, nonceS, mk []byte) (msk, emsk []byte) {
	h := sha1.New()
	h.Write([]byte(identity))
	counterBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(counterBuf, uint16(counter))
	h.Write(counterBuf)
	h.Write(nonceS)
	h.Write(mk)
	xkeyPrime := h.Sum(nil)

	prfOutput := PrfSHA1(xkeyPrime, 128)

	return append([]byte(nil), prfOutput[0:64]...), append([]byte(nil), prfOutput[64:128]...)
}
