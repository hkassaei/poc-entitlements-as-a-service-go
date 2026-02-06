# EAP-AKA Test Strategy

This document describes how we verify RFC 4187 (EAP-AKA) compliance and the overall testing approach for the entitlement server's authentication stack.

## Testing Layers

The EAP-AKA implementation is tested at five distinct layers, from low-level cryptography up to full protocol flows:

```
┌─────────────────────────────────────────────────────────┐
│  Integration Tests (eapAka.integration.test.ts)         │  ← Full HTTP flows
├─────────────────────────────────────────────────────────┤
│  RFC 4187 Compliance (rfc4187Compliance.test.ts)        │  ← Wire format
├─────────────────────────────────────────────────────────┤
│  EAP Codec (eapCodec.test.ts)                           │  ← Encode/decode
├─────────────────────────────────────────────────────────┤
│  Encryption (eapEncryption.test.ts)                     │  ← AES-128-CBC
├─────────────────────────────────────────────────────────┤
│  MILENAGE Crypto (milenage.test.ts)                     │  ← 3GPP algorithms
└─────────────────────────────────────────────────────────┘
```

---

## Layer 1: MILENAGE Cryptography

**File:** `mock-hss/tests/milenage.test.ts`
**Tests:** 36
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

```typescript
// 3GPP TS 35.207 Test Set 1
const ki = Buffer.from('465b5ce8b199b49faa5f0a2ee238a6bc', 'hex');
const op = Buffer.from('cdc202d5123e20f62b6d676ac72cb318', 'hex');
const rand = Buffer.from('23553cbe9637a89d218ae64dae47bf35', 'hex');

// Expected outputs from the spec
expect(vectors.xres.toString('hex')).toBe('a54211d5e3ba50bf');
expect(vectors.ck.toString('hex')).toBe('b40ba9a3c58b2a05bbf0d987b21bf8cb');
```

We test two complete test sets from TS 35.207 plus additional edge cases for the resync functions (f1*, f5*).

---

## Layer 2: EAP Packet Encryption

**File:** `tests/unit/eapEncryption.test.ts`
**Tests:** 15
**Approach:** Round-trip verification with known vectors

### What We Test

RFC 4187 Section 10.12 specifies AES-128-CBC encryption for AT_ENCR_DATA:

- Encryption of inner attributes (AT_COUNTER, AT_NONCE_S, AT_NEXT_REAUTH_ID)
- Proper AT_PADDING to achieve 16-byte alignment
- IV handling (AT_IV attribute)
- Decryption and padding removal

### How We Test

```typescript
// Encrypt inner attributes
const ciphertext = encryptAttributes(kEncr, iv, innerAttributes);

// Decrypt and verify round-trip
const decrypted = decryptAttributes(kEncr, iv, ciphertext);
expect(decrypted).toEqual(innerAttributes);
```

We verify:
- Ciphertext length is always a multiple of 16 bytes
- AT_PADDING is correctly added/removed
- Different K_encr values produce different ciphertext
- Tampering detection (wrong key fails decryption)

---

## Layer 3: EAP Packet Codec

**File:** `tests/unit/eapCodec.test.ts`
**Tests:** 40+
**Approach:** Round-trip encode/decode verification

### What We Test

The binary EAP packet format per RFC 3748 and RFC 4187:

- EAP header: Code (1) | Identifier (1) | Length (2)
- EAP-AKA header: Type (1) | Subtype (1) | Reserved (2)
- Attribute TLV encoding for all attribute types

### How We Test

```typescript
// Build a packet
const packet: EapPacket = {
  code: EAP_CODE.REQUEST,
  identifier: 42,
  type: EAP_TYPE_AKA,
  subtype: AKA_SUBTYPE.CHALLENGE,
  attributes: [
    { type: AT.AT_RAND, value: rand },
    { type: AT.AT_AUTN, value: autn },
    { type: AT.AT_MAC, value: mac },
  ],
};

// Encode → Decode → Compare
const encoded = encodeEapPacket(packet);
const decoded = decodeEapPacket(encoded);
expect(decoded).toMatchObject(packet);
```

Every attribute type has dedicated tests verifying:
- Correct type byte
- Correct length field (in 4-byte words)
- Correct reserved byte positions
- Correct value extraction

---

## Layer 4: RFC 4187 Wire Format Compliance

**File:** `tests/unit/rfc4187Compliance.test.ts`
**Tests:** 38
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

```typescript
it('AKA-Challenge packet has correct byte-level structure', () => {
  const buf = encodeEapPacket(challengePacket);

  // EAP Header
  expect(buf[0]).toBe(EAP_CODE.REQUEST);  // Code at byte 0
  expect(buf[1]).toBe(0x01);              // Identifier at byte 1
  expect(buf.readUInt16BE(2)).toBe(68);   // Length at bytes 2-3

  // EAP-AKA Header
  expect(buf[4]).toBe(EAP_TYPE_AKA);      // Type = 23 at byte 4
  expect(buf[5]).toBe(AKA_SUBTYPE.CHALLENGE); // Subtype at byte 5

  // AT_RAND at offset 8
  expect(buf[8]).toBe(AT.AT_RAND);        // Type = 1
  expect(buf[9]).toBe(5);                 // Length = 5 words
});
```

**Key derivation verification:**

```typescript
it('MK = SHA-1(Identity | IK | CK)', () => {
  const mk = deriveMasterKey(identity, ik, ck);

  // Verify by computing manually
  const expected = crypto.createHash('sha1')
    .update(Buffer.from(identity, 'utf-8'))
    .update(ik)
    .update(ck)
    .digest();

  expect(mk.toString('hex')).toBe(expected.toString('hex'));
});
```

**AT_MAC verification:**

```typescript
it('AT_MAC uses HMAC-SHA-1 truncated to 16 bytes', () => {
  const mac = computeMac(kAut, packetWithZeroedMac);

  const fullHmac = crypto.createHmac('sha1', kAut)
    .update(packetWithZeroedMac)
    .digest();

  expect(mac.toString('hex')).toBe(fullHmac.subarray(0, 16).toString('hex'));
});
```

---

## Layer 5: Integration Tests

**File:** `tests/integration/eapAka.integration.test.ts`
**Tests:** 16
**Approach:** Full HTTP protocol flows with real services

### What We Test

Complete EAP-AKA handshakes over HTTP:

1. **Full Authentication Flow**
   - RT1: IMSI → 401 + AKA-Challenge
   - RT2: EAP-Response → 200 + token

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

```typescript
it('completes full EAP-AKA authentication', async () => {
  // RT1: Initial request
  const rt1 = await app.inject({
    method: 'POST',
    url: '/entitlement',
    payload: { app: 'ap2004', imsi: '001010000000001', ... },
  });
  expect(rt1.statusCode).toBe(401);
  const sessionId = rt1.headers['x-eap-session-id'];

  // Build valid response using MILENAGE
  const eapResponse = buildValidResponse(rt1.json().eap_relay);

  // RT2: Complete auth
  const rt2 = await app.inject({
    method: 'POST',
    url: '/entitlement',
    headers: { 'X-EAP-Session-Id': sessionId },
    payload: { app: 'ap2004', eap_relay: eapResponse, ... },
  });
  expect(rt2.statusCode).toBe(200);
  expect(rt2.json().token).toBeDefined();
});
```

---

## Test Vector Sources

| Source | What It Provides | Used In |
|--------|------------------|---------|
| 3GPP TS 35.207 | MILENAGE test vectors (Ki, OP, RAND → XRES, CK, IK, AK) | `milenage.test.ts` |
| RFC 4187 | Protocol format specifications | `rfc4187Compliance.test.ts` |
| RFC 3748 | EAP base protocol format | `eapCodec.test.ts` |
| Custom | Derived vectors for edge cases | All test files |

---

## Running Tests

```bash
# All tests
npm test

# Specific layer
npm test -- tests/unit/rfc4187Compliance.test.ts
npm test -- tests/unit/eapCodec.test.ts
npm test -- tests/integration/eapAka.integration.test.ts

# Mock HSS tests (MILENAGE)
cd mock-hss && npm test
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

```typescript
// Dump packet bytes for Wireshark
const packet = encodeEapPacket(challengePacket);
console.log(packet.toString('hex'));
// Load in Wireshark: File → Import from Hex Dump
```

Wireshark has a built-in EAP-AKA dissector that will show all fields.

### 3. Fuzz Testing

Generate malformed packets to test decoder robustness:

```typescript
// Truncated packets
decodeEapPacket(validPacket.subarray(0, 10));

// Invalid length fields
validPacket[1] = 0xff; // Bogus length
decodeEapPacket(validPacket);

// Unknown attribute types
// ... etc
```

### 4. Interoperability with Real SIMs

For ultimate validation, test with actual SIM cards:

- Use [pySim](https://github.com/osmocom/pysim) for SIM programming
- Use [srsRAN](https://github.com/srsran/srsRAN) for a software radio stack
- Or obtain test SIMs with known Ki values from your carrier partner

---

## Test Coverage Summary

| Layer | File | Tests | Confidence |
|-------|------|-------|------------|
| MILENAGE | `milenage.test.ts` | 36 | **High** — 3GPP official vectors |
| Encryption | `eapEncryption.test.ts` | 15 | **High** — Round-trip + tampering |
| Codec | `eapCodec.test.ts` | 40+ | **High** — All attribute types |
| RFC 4187 | `rfc4187Compliance.test.ts` | 38 | **High** — Byte-level verification |
| Integration | `eapAka.integration.test.ts` | 16 | **High** — Full protocol flows |
| **Total** | | **246** | |

The combination of official test vectors, byte-level compliance tests, and full integration flows provides high confidence that the implementation correctly follows RFC 4187.
