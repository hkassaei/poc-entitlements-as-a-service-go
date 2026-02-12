// Package eapaka implements the EAP-AKA protocol (RFC 4187).
package eapaka

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
)

// EapAttribute represents a single TLV attribute in an EAP-AKA packet.
type EapAttribute struct {
	Type  int
	Value []byte
}

// EapPacket represents a decoded EAP packet.
type EapPacket struct {
	Code       int
	Identifier int
	Type       int // 0 for Success/Failure
	Subtype    int // 0 for Success/Failure
	Attributes []EapAttribute
}

// EncodeAttribute encodes an EAP attribute into its TLV binary representation.
func EncodeAttribute(attr EapAttribute) []byte {
	var valuePayload []byte

	switch attr.Type {
	case config.ATRand, config.ATAutn, config.ATMac, config.ATIV, config.ATNonceS:
		// 2 reserved bytes + 16 bytes value = 18 bytes payload, length=5 (20 bytes total)
		valuePayload = make([]byte, 18)
		copy(valuePayload[2:], attr.Value[:16])

	case config.ATRes:
		// 2-byte bit-length prefix + value, total attribute must be multiple of 4 bytes
		bitLength := len(attr.Value) * 8
		totalAttrLen := roundUp(2+2+len(attr.Value), 4)
		valuePayload = make([]byte, totalAttrLen-2) // subtract type+length bytes
		binary.BigEndian.PutUint16(valuePayload[0:2], uint16(bitLength))
		copy(valuePayload[2:], attr.Value)

	case config.ATAuts:
		// 2 reserved bytes + 14 bytes AUTS = 16 bytes payload, length=4
		valuePayload = make([]byte, 16)
		copy(valuePayload[2:], attr.Value[:14])

	case config.ATCounter:
		// 2-byte big-endian counter value, length=1 (4 bytes total)
		valuePayload = make([]byte, 2)
		binary.BigEndian.PutUint16(valuePayload, binary.BigEndian.Uint16(attr.Value[:2]))

	case config.ATCounterTooSmall:
		// 2 reserved bytes, no value, length=1 (4 bytes total)
		valuePayload = make([]byte, 2)

	case config.ATPadding:
		// attr.Value length contains the padding bytes (zeros)
		valuePayload = make([]byte, len(attr.Value))

	case config.ATEncrData:
		// 2 reserved bytes + variable-length ciphertext
		valuePayload = make([]byte, 2+len(attr.Value))
		copy(valuePayload[2:], attr.Value)

	case config.ATNextReauthID:
		// 2-byte actual-length prefix + UTF-8 identity + padding to 4-byte boundary
		actualLen := len(attr.Value)
		totalAttrLen := roundUp(2+2+actualLen, 4)
		valuePayload = make([]byte, totalAttrLen-2) // subtract type+length bytes
		binary.BigEndian.PutUint16(valuePayload[0:2], uint16(actualLen))
		copy(valuePayload[2:], attr.Value)

	default:
		// Generic: pad value to 4-byte boundary (minus 2 for type+length header)
		totalPayload := roundUp(len(attr.Value), 4)
		valuePayload = make([]byte, totalPayload)
		copy(valuePayload, attr.Value)
	}

	totalLength := 2 + len(valuePayload)
	buf := make([]byte, totalLength)
	buf[0] = byte(attr.Type)
	buf[1] = byte(totalLength / 4) // length in 4-byte words
	copy(buf[2:], valuePayload)
	return buf
}

// decodeAttribute decodes a single EAP attribute from a buffer at the given offset.
func decodeAttribute(buf []byte, offset int) (EapAttribute, int, error) {
	if offset+2 > len(buf) {
		return EapAttribute{}, 0, errors.New("truncated attribute header")
	}

	attrType := int(buf[offset])
	lengthWords := int(buf[offset+1])
	totalBytes := lengthWords * 4

	if offset+totalBytes > len(buf) {
		return EapAttribute{}, 0, fmt.Errorf("truncated attribute value: type=%d, expected %d bytes", attrType, totalBytes)
	}

	var value []byte

	switch attrType {
	case config.ATRand, config.ATAutn, config.ATMac, config.ATIV, config.ATNonceS:
		// Skip 2 reserved bytes, read 16 bytes
		value = make([]byte, 16)
		copy(value, buf[offset+4:offset+20])

	case config.ATRes:
		// 2-byte bit-length prefix after type+length header
		bitLength := binary.BigEndian.Uint16(buf[offset+2 : offset+4])
		byteLength := int(bitLength / 8)
		value = make([]byte, byteLength)
		copy(value, buf[offset+4:offset+4+byteLength])

	case config.ATAuts:
		// Skip 2 reserved bytes, read 14 bytes
		value = make([]byte, 14)
		copy(value, buf[offset+4:offset+18])

	case config.ATCounter:
		// 2-byte big-endian counter after type+length header
		value = make([]byte, 2)
		copy(value, buf[offset+2:offset+4])

	case config.ATCounterTooSmall:
		// No meaningful value
		value = make([]byte, 0)

	case config.ATPadding:
		// Padding bytes after type+length header
		value = make([]byte, totalBytes-2)
		copy(value, buf[offset+2:offset+totalBytes])

	case config.ATEncrData:
		// Skip 2 reserved bytes, rest is ciphertext
		ciphertextLen := totalBytes - 4
		value = make([]byte, ciphertextLen)
		copy(value, buf[offset+4:offset+4+ciphertextLen])

	case config.ATNextReauthID:
		// 2-byte actual-length prefix after type+length header
		actualLen := int(binary.BigEndian.Uint16(buf[offset+2 : offset+4]))
		value = make([]byte, actualLen)
		copy(value, buf[offset+4:offset+4+actualLen])

	default:
		// Generic: everything after the 2-byte header
		value = make([]byte, totalBytes-2)
		copy(value, buf[offset+2:offset+totalBytes])
	}

	return EapAttribute{Type: attrType, Value: value}, totalBytes, nil
}

// EncodeEapPacket encodes an EAP packet to bytes.
func EncodeEapPacket(packet EapPacket) []byte {
	// EAP-Success and EAP-Failure are just 4 bytes
	if packet.Code == config.EAPCodeSuccess || packet.Code == config.EAPCodeFailure {
		buf := make([]byte, 4)
		buf[0] = byte(packet.Code)
		buf[1] = byte(packet.Identifier)
		binary.BigEndian.PutUint16(buf[2:4], 4)
		return buf
	}

	// Build attribute bytes
	attrBytes := make([]byte, 0, len(packet.Attributes)*20)
	for _, attr := range packet.Attributes {
		attrBytes = append(attrBytes, EncodeAttribute(attr)...)
	}

	// Total: EAP header(4) + Type(1) + Subtype(1) + Reserved(2) + attributes
	totalLength := 8 + len(attrBytes)
	buf := make([]byte, totalLength)

	buf[0] = byte(packet.Code)
	buf[1] = byte(packet.Identifier)
	binary.BigEndian.PutUint16(buf[2:4], uint16(totalLength))

	eapType := packet.Type
	if eapType == 0 {
		eapType = config.EAPTypeAKA
	}
	buf[4] = byte(eapType)
	buf[5] = byte(packet.Subtype)
	// bytes 6-7 reserved (already zero)
	copy(buf[8:], attrBytes)

	return buf
}

// DecodeEapPacket decodes bytes into an EapPacket.
func DecodeEapPacket(buf []byte) (EapPacket, error) {
	if len(buf) < 4 {
		return EapPacket{}, errors.New("EAP packet too short")
	}

	code := int(buf[0])
	identifier := int(buf[1])
	length := int(binary.BigEndian.Uint16(buf[2:4]))

	if len(buf) < length {
		return EapPacket{}, fmt.Errorf("EAP packet truncated: header says %d, got %d", length, len(buf))
	}

	// Success/Failure: no type or attributes
	if code == config.EAPCodeSuccess || code == config.EAPCodeFailure {
		return EapPacket{Code: code, Identifier: identifier}, nil
	}

	if length < 8 {
		return EapPacket{}, errors.New("EAP-AKA packet too short for type/subtype header")
	}

	eapType := int(buf[4])
	subtype := int(buf[5])

	var attributes []EapAttribute
	offset := 8
	for offset < length {
		attr, bytesRead, err := decodeAttribute(buf, offset)
		if err != nil {
			return EapPacket{}, fmt.Errorf("decode attribute at offset %d: %w", offset, err)
		}
		attributes = append(attributes, attr)
		offset += bytesRead
	}

	return EapPacket{
		Code:       code,
		Identifier: identifier,
		Type:       eapType,
		Subtype:    subtype,
		Attributes: attributes,
	}, nil
}

// EncodeEapToBase64 encodes an EAP packet to a base64 string.
func EncodeEapToBase64(packet EapPacket) string {
	return base64.StdEncoding.EncodeToString(EncodeEapPacket(packet))
}

// DecodeEapFromBase64 decodes a base64-encoded EAP packet.
func DecodeEapFromBase64(b64 string) (EapPacket, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return EapPacket{}, fmt.Errorf("base64 decode: %w", err)
	}
	return DecodeEapPacket(data)
}

// roundUp rounds n up to the nearest multiple of m.
func roundUp(n, m int) int {
	if n%m == 0 {
		return n
	}
	return n + m - n%m
}
