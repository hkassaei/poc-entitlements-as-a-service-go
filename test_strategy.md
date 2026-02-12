# EAP-AKA Test Strategy

This document describes how we verify RFC 4187 (EAP-AKA) compliance and the overall testing approach for the entitlement server's authentication stack.

## Testing Layers

The EAP-AKA implementation is tested at five distinct layers, from low-level cryptography up to full protocol flows:

```
┌─────────────────────────────────────────────────────────┐
│  Integration Tests (docker-compose + HTTP flows)        │  ← Full HTTP flows
├─────────────────────────────────────────────────────────┤
│  RFC 4187 Compliance (rfc4187_test.go)                  │  ← Wire format
├─────────────────────────────────────────────────────────┤
│  EAP Codec (codec_test.go)                              │  ← Encode/decode
├─────────────────────────────────────────────────────────┤
│  Encryption (encryption_test.go)                        │  ← AES-128-CBC
├─────────────────────────────────────────────────────────┤
│  MILENAGE Crypto (milenage_test.go)                     │  ← 3GPP algorithms
└─────────────────────────────────────────────────────────┘
```

---

## Layer 1: MILENAGE Cryptography

**File:** `internal/crypto/milenage_test.go`
**Approach:** Official 3GPP test vectors

### What We Test

The MILENAGE algorithm set (3GPP TS 35.206) implements the cryptographic core:

- **f1** — Network authentication (MAC-A in AUTN)
- **f2** — User authentication (RES/XRES)
- **f3** — Cipher key derivation (CK)
- **f4** — Integrity key derivation (IK)
- **f5** — Anonymity key (AK for SQN concealment)
- **f1\*** — Resync MAC (MAC-S in AUTS)
- **f5\*** — Resync anonymity key (AK* for SQN recovery)

### How We Test

We use **3GPP TS 35.207 official test vectors**. These are the gold standard — if your implementation produces the exact outputs specified in TS 35.207, it will interoperate with any compliant HSS/SIM.

```go
// 3GPP TS 35.207 Test Set 1
ki, _ := hex.DecodeString("465b5ce8b199b49faa5f0a2ee238a6bc")
op, _ := hex.DecodeString("cdc202d5123e20f62b6d676ac72cb318")
rand, _ := hex.DecodeString("23553cbe9637a89d218ae64dae47bf35")

vectors := crypto.GenerateVectorsWithRAND(ki, op, rand, sqn, amf)

// Expected outputs from the spec
assert.Equal(t, "a54211d5e3ba50bf", hex.EncodeToString(vectors.XRES))
assert.Equal(t, "b40ba9a3c58b2a05bbf0d987b21bf8cb", hex.EncodeToString(vectors.CK))
```

We test two complete test sets from TS 35.207 plus additional edge cases for the resync functions (f1*, f5*) and AUTS generation/validation round-trips.

---

## Layer 2: EAP Packet Encryption

**File:** `internal/eapaka/encryption_test.go`
**Approach:** Round-trip verification with known vectors

### What We Test

RFC 4187 Section 10.12 specifies AES-128-CBC encryption for AT_ENCR_DATA:

- Encryption of inner attributes (AT_COUNTER, AT_NONCE_S, AT_NEXT_REAUTH_ID)
- Proper AT_PADDING to achieve 16-byte alignment
- IV handling (AT_IV attribute)
- Decryption and padding removal

### How We Test

```go
// Encrypt inner attributes
ciphertext := eapaka.EncryptAttributes(kEncr, iv, innerAttributes)

// Decrypt and verify round-trip
decrypted := eapaka.DecryptAttributes(kEncr, iv, ciphertext)
assert.Equal(t, innerAttributes, decrypted)
```

We verify:
- Ciphertext length is always a multiple of 16 bytes
- AT_PADDING is correctly added/removed
- Different K_encr values produce different ciphertext
- Tampering detection (wrong key fails decryption)

---

## Layer 3: EAP Packet Codec

**File:** `internal/eapaka/codec_test.go`
**Approach:** Round-trip encode/decode verification

### What We Test

The binary EAP packet format per RFC 3748 and RFC 4187:

- EAP header: Code (1) | Identifier (1) | Length (2)
- EAP-AKA header: Type (1) | Subtype (1) | Reserved (2)
- Attribute TLV encoding for all attribute types

### How We Test

```go
// Build a packet
packet := eapaka.EapPacket{
    Code:       config.EAPCodeRequest,
    Identifier: 42,
    Subtype:    config.AKASubtypeChallenge,
    Attributes: []eapaka.EapAttribute{
        {Type: config.ATRand, Value: rand},
        {Type: config.ATAutn, Value: autn},
        {Type: config.ATMac, Value: mac},
    },
}

// Encode → Decode → Compare
encoded := eapaka.EncodeEapPacket(packet)
decoded, err := eapaka.DecodeEapPacket(encoded)
require.NoError(t, err)
assert.Equal(t, packet.Code, decoded.Code)
assert.Equal(t, packet.Identifier, decoded.Identifier)
```

Every attribute type has dedicated tests verifying:
- Correct type byte
- Correct length field (in 4-byte words)
- Correct reserved byte positions
- Correct value extraction

---

## Layer 4: RFC 4187 Wire Format Compliance

**File:** `internal/eapaka/rfc4187_test.go`
**Approach:** Byte-level verification against RFC specifications

### What We Test

This layer verifies **exact byte positions and values** as specified in RFC 4187:

| RFC Section | What It Specifies |
|-------------|-------------------|
| §8.1 | EAP-AKA message format |
| §10.6 | AT_RAND encoding |
| §10.7 | AT_AUTN encoding |
| §10.8 | AT_RES encoding (with bit-length prefix) |
| §10.9 | AT_AUTS encoding |
| §10.11 | AT_MAC encoding |
| §10.14 | AT_COUNTER encoding |
| §10.15 | AT_COUNTER_TOO_SMALL encoding |
| §10.16 | AT_NONCE_S encoding |
| §7 | Key derivation (MK, PRF, K_encr, K_aut, MSK, EMSK) |
| §10.15 | AT_MAC computation (HMAC-SHA-1-128) |
| §5.4 | Fast re-authentication |

### How We Test

**Byte-level assertions:**

```go
func TestAKAChallengePacketStructure(t *testing.T) {
    buf := eapaka.EncodeEapPacket(challengePacket)

    // EAP Header
    assert.Equal(t, byte(config.EAPCodeRequest), buf[0])  // Code at byte 0
    assert.Equal(t, byte(0x01), buf[1])                    // Identifier at byte 1
    assert.Equal(t, uint16(68), binary.BigEndian.Uint16(buf[2:4]))  // Length

    // EAP-AKA Header
    assert.Equal(t, byte(config.EAPTypeAKA), buf[4])       // Type = 23
    assert.Equal(t, byte(config.AKASubtypeChallenge), buf[5])  // Subtype

    // AT_RAND at offset 8
    assert.Equal(t, byte(config.ATRand), buf[8])           // Type = 1
    assert.Equal(t, byte(5), buf[9])                       // Length = 5 words
}
```

**Key derivation verification:**

```go
func TestMKDerivation(t *testing.T) {
    mk := eapaka.DeriveMasterKey(identity, ik, ck)

    // Verify by computing manually
    h := sha1.New()
    h.Write([]byte(identity))
    h.Write(ik)
    h.Write(ck)
    expected := h.Sum(nil)

    assert.Equal(t, hex.EncodeToString(expected), hex.EncodeToString(mk))
}
```

**AT_MAC verification:**

```go
func TestATMACComputation(t *testing.T) {
    mac := eapaka.ComputeMAC(kAut, packetWithZeroedMac)

    // HMAC-SHA-1 truncated to 16 bytes
    h := hmac.New(sha1.New, kAut)
    h.Write(packetWithZeroedMac)
    fullHmac := h.Sum(nil)

    assert.Equal(t, hex.EncodeToString(fullHmac[:16]), hex.EncodeToString(mac))
}
```

---

## Layer 5: Integration Tests

**Approach:** Full HTTP protocol flows with Docker Compose services

### What We Test

Complete EAP-AKA handshakes over HTTP:

1. **Full Authentication Flow**
   - RT1: IMSI → 401 + AKA-Challenge
   - RT2: EAP-Response → 200 + entitlement config

2. **Fast Re-authentication Flow**
   - Re-auth identity → 401 + AKA-Reauthentication
   - EAP-Response → 200 + new identity

3. **SQN Resynchronization**
   - SYNC_FAILURE + AT_AUTS → new challenge

4. **Error Cases**
   - Invalid EAP packets
   - Missing session
   - Wrong AT_RES
   - Invalid AT_MAC

### How We Test

Integration tests run against Docker Compose services (Postgres, Redis, mock-hss, ecs):

```bash
# Start services
make docker-up

# Run integration tests (once available)
go test ./tests/integration/... -count=1

# Or test manually with curl
curl -X POST http://localhost:8443/entitlement \
  -H "Content-Type: application/json" \
  -d '{"app":"ap2004","imsi":"001010000000001","terminal_id":"test"}'
```

---

## Test Vector Sources

| Source | What It Provides | Used In |
|--------|------------------|---------|
| 3GPP TS 35.207 | MILENAGE test vectors (Ki, OP, RAND → XRES, CK, IK, AK) | `milenage_test.go` |
| RFC 4187 | Protocol format specifications | `rfc4187_test.go` |
| RFC 3748 | EAP base protocol format | `codec_test.go` |
| Custom | Derived vectors for edge cases | All test files |

---

## Running Tests

```bash
# All tests
go test ./... -count=1

# With race detector
go test ./... -race -count=1

# Specific package
go test ./internal/crypto/... -v
go test ./internal/eapaka/... -v
go test ./internal/services/... -v
go test ./internal/protocol/... -v

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html

# Or via Makefile
make test
make test-race
make test-cover
```

---

## Further Testing Options

### 1. Cross-Implementation Testing

Use [wpa_supplicant's eapol_test](https://w1.fi/wpa_supplicant/devel/testing_tools.html) to act as a real EAP peer:

```bash
# Build eapol_test
git clone git://w1.fi/hostap.git
cd hostap/wpa_supplicant
cp defconfig .config
echo "CONFIG_EAPOL_TEST=y" >> .config
echo "CONFIG_EAP_AKA=y" >> .config
make eapol_test

# Test against your server
./eapol_test -c aka.conf -s secret
```

### 2. Wireshark Validation

Export encoded packets and verify parsing in Wireshark:

```go
// Dump packet bytes for Wireshark
packet := eapaka.EncodeEapPacket(challengePacket)
fmt.Println(hex.EncodeToString(packet))
// Load in Wireshark: File → Import from Hex Dump
```

Wireshark has a built-in EAP-AKA dissector that will show all fields.

### 3. Fuzz Testing

Go has built-in fuzz testing support:

```go
func FuzzDecodeEapPacket(f *testing.F) {
    f.Add(validPacketBytes)
    f.Fuzz(func(t *testing.T, data []byte) {
        // Should not panic on any input
        _, _ = eapaka.DecodeEapPacket(data)
    })
}
```

### 4. Interoperability with Real SIMs

For ultimate validation, test with actual SIM cards:

- Use [pySim](https://github.com/osmocom/pysim) for SIM programming
- Use [srsRAN](https://github.com/srsran/srsRAN) for a software radio stack
- Or obtain test SIMs with known Ki values from your carrier partner

---

## Test Coverage Summary

| Layer | File | Confidence |
|-------|------|------------|
| MILENAGE | `internal/crypto/milenage_test.go` | **High** — 3GPP official vectors |
| Envelope Encryption | `internal/crypto/envelope_test.go` | **High** — Round-trip + key validation |
| KMS | `internal/crypto/kms_test.go` | **High** — Local key manager tests |
| Encryption | `internal/eapaka/encryption_test.go` | **High** — Round-trip + tampering |
| Codec | `internal/eapaka/codec_test.go` | **High** — All attribute types |
| Key Derivation | `internal/eapaka/keys_test.go` | **High** — MK, PRF, MAC verification |
| RFC 4187 | `internal/eapaka/rfc4187_test.go` | **High** — Byte-level verification |
| Protocol Builders | `internal/protocol/*_test.go` | **High** — JSON + XML output |
| Service Handlers | `internal/services/services_test.go` | **High** — All 12 handlers, 55 tests |

The combination of official test vectors, byte-level compliance tests, and comprehensive service handler tests provides high confidence that the implementation correctly follows RFC 4187 and GSMA TS.43.
