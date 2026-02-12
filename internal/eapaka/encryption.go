package eapaka

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
)

// EncryptAttributes encrypts a list of inner attributes into AT_ENCR_DATA ciphertext.
// 1. Serialize inner attributes via EncodeAttribute()
// 2. Append AT_PADDING if total isn't 16-byte aligned
// 3. AES-128-CBC encrypt with no auto-padding
func EncryptAttributes(kEncr, iv []byte, innerAttributes []EapAttribute) ([]byte, error) {
	// Serialize inner attributes
	plaintext := make([]byte, 0, len(innerAttributes)*20)
	for _, attr := range innerAttributes {
		plaintext = append(plaintext, EncodeAttribute(attr)...)
	}

	// Pad to 16-byte boundary with AT_PADDING
	remainder := len(plaintext) % 16
	if remainder != 0 {
		paddingNeeded := 16 - remainder
		paddingAttr := EncodeAttribute(EapAttribute{
			Type:  config.ATPadding,
			Value: make([]byte, paddingNeeded-2), // subtract type+length bytes
		})
		plaintext = append(plaintext, paddingAttr...)
	}

	block, err := aes.NewCipher(kEncr)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}

	ciphertext := make([]byte, len(plaintext))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, plaintext)
	return ciphertext, nil
}

// DecryptAttributes decrypts AT_ENCR_DATA ciphertext and parses inner attributes.
// Strips AT_PADDING from the result.
func DecryptAttributes(kEncr, iv, ciphertext []byte) ([]EapAttribute, error) {
	block, err := aes.NewCipher(kEncr)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}

	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	// Parse inner attributes
	var attributes []EapAttribute
	offset := 0
	for offset < len(plaintext) {
		if offset+2 > len(plaintext) {
			break
		}

		attrType := int(plaintext[offset])
		lengthWords := int(plaintext[offset+1])
		totalBytes := lengthWords * 4

		if totalBytes == 0 || offset+totalBytes > len(plaintext) {
			break
		}

		// Skip AT_PADDING
		if attrType == config.ATPadding {
			offset += totalBytes
			continue
		}

		var value []byte
		switch attrType {
		case config.ATCounter:
			value = make([]byte, 2)
			copy(value, plaintext[offset+2:offset+4])

		case config.ATCounterTooSmall:
			value = make([]byte, 0)

		case config.ATNonceS:
			value = make([]byte, 16)
			copy(value, plaintext[offset+4:offset+20])

		case config.ATNextReauthID:
			actualLen := int(binary.BigEndian.Uint16(plaintext[offset+2 : offset+4]))
			value = make([]byte, actualLen)
			copy(value, plaintext[offset+4:offset+4+actualLen])

		default:
			value = make([]byte, totalBytes-2)
			copy(value, plaintext[offset+2:offset+totalBytes])
		}

		attributes = append(attributes, EapAttribute{Type: attrType, Value: value})
		offset += totalBytes
	}

	return attributes, nil
}
