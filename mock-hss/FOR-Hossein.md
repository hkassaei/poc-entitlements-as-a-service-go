# Mock HSS: The Heart of SIM Authentication

## What Is This Thing?

Imagine your phone trying to prove to the network that it's really *your* phone. Your SIM card has a secret key burned into it at the factory — think of it as a fingerprint that only your carrier knows. When you connect to a cell tower, the network doesn't just take your phone's word for it. Instead, it runs a mathematical challenge-response: "If you really have the key you claim, solve this puzzle." That puzzle is the **MILENAGE algorithm**, and the **HSS (Home Subscriber Server)** is the network-side computer that generates these puzzles.

This `mock-hss/` service is our development stand-in for a real HSS. It generates authentication vectors — the puzzle + expected answer — so the main entitlements server can authenticate devices during testing.

## The Architecture: How Everything Connects

```
┌──────────────┐     POST /vectors      ┌──────────────┐
│              │  ──────────────────────>│              │
│     ECS      │   { imsi: "001..." }   │   Mock HSS   │
│  (Main App)  │  <──────────────────── │  (port 3001) │
│  port 8443   │   { rand, autn, ...}   │              │
└──────┬───────┘                        └──────┬───────┘
       │                                       │
       │         ┌──────────────┐              │
       └────────>│  PostgreSQL  │<─────────────┘
                 │  port 5432   │
                 └──────────────┘
```

The flow is:

1. A device connects to ECS and sends its IMSI (the SIM identity number)
2. ECS calls `POST /vectors` on the mock-hss with that IMSI
3. Mock-hss looks up the subscriber's encrypted keys in Postgres
4. It decrypts the keys, runs MILENAGE, and returns the authentication vectors
5. ECS uses those vectors to challenge the device

## The Codebase: File by File

### `src/milenage.ts` — The Crypto Engine

This is the crown jewel. It implements the MILENAGE algorithm from 3GPP TS 35.206 — a standardized set of cryptographic functions used in every 3G/4G/5G network worldwide. The whole thing is built on a single primitive: **AES-128-ECB** (one block, 16 bytes in, 16 bytes out).

The algorithm has seven functions (f1 through f5*), but they all share a common structure:
- Start with a subscriber key (Ki) and an operator key (OP)
- Derive OPc = AES_Ki(OP) XOR OP (this lets operators customize the algorithm)
- Compute TEMP = AES_Ki(RAND XOR OPc) — where RAND is a random challenge
- Then each function applies a different rotation + XOR + AES step to produce its output

The outputs are:
- **f1** → MAC-A: proves the network is legitimate (network authentication)
- **f2** → XRES: the expected response from the device (device authentication)
- **f3** → CK: confidentiality key (for encryption)
- **f4** → IK: integrity key (for tamper detection)
- **f5** → AK: anonymity key (hides the sequence number)

### `src/kms.ts` — Envelope Encryption

Subscriber keys (Ki and OP) are 16 bytes each and are *extremely* sensitive — if someone steals a Ki, they can clone a SIM card. So we never store them in plaintext.

The envelope encryption scheme works like a matryoshka doll:
1. Each subscriber gets their own **DEK** (Data Encryption Key) — a random 32-byte AES-256 key
2. Ki and OP are encrypted with the DEK using AES-256-GCM (authenticated encryption)
3. The DEK itself is wrapped (encrypted) with a **KEK** (Key Encryption Key) from the environment

To decrypt, you reverse the process: unwrap the DEK with the KEK, then decrypt Ki/OP with the DEK, use them, and immediately zero out the buffers.

In production, the KEK would live in a cloud KMS (AWS KMS, etc.). For development, we use a simple XOR wrap — which is fine because XOR is its own inverse and the KEK never leaves memory.

### `src/config.ts` & `src/db.ts` — Plumbing

`config.ts` reads environment variables with type safety. `db.ts` sets up a Postgres connection pool with Drizzle ORM, defining just the `subscribers` table columns that the mock-hss needs (a minimal subset of the full ECS schema).

### `src/index.ts` — The Fastify Server

Two endpoints:
- `GET /health` → `{ status: "ok" }` (for Docker health checks)
- `POST /vectors` → the main authentication vector generator

The `/vectors` handler is the pipeline that ties everything together: database lookup → key decryption → MILENAGE → buffer zeroing → response.

### `src/seed.ts` — Test Data Bootstrapper

Inserts two test subscribers using the official 3GPP TS 35.207 test vector keys. It creates the `subscribers` table if it doesn't exist (raw SQL DDL), encrypts the keys with envelope encryption, and upserts the records.

### `tests/` — The Safety Net

- `milenage.test.ts`: Validates against the official 3GPP TS 35.207 test vectors. Two full test sets covering OPc, MAC-A, RES, CK, IK, and AK.
- `kms.test.ts`: Round-trip encryption/decryption, DEK wrap/unwrap, buffer zeroing, and error cases.

## Technologies Used & Why

| Technology | Why |
|---|---|
| **Node.js `crypto`** | MILENAGE only needs AES-128-ECB and AES-256-GCM — both built into Node's OpenSSL bindings. No external crypto libs needed. |
| **Fastify + TypeBox** | Type-safe HTTP with runtime request validation. TypeBox schemas double as TypeScript types. |
| **Drizzle ORM** | Lightweight, type-safe database access. We only query one table, so a full ORM would be overkill. |
| **Vitest** | Fast ESM-native test runner. Tests complete in ~350ms. |
| **Envelope encryption** | Industry standard for protecting keys at rest. Separates the key hierarchy so you can rotate the KEK without re-encrypting every subscriber. |

## Lessons Learned

### 1. The MILENAGE f1 Bug: Read the Spec, Then Read It Again

The most significant bug was in the f1 function (MAC-A computation). The plan described the formula as:

```
OUT1 = AES_K(rotate(TEMP⊕OPc⊕C1, R1) ⊕ OPc ⊕ IN1) ⊕ OPc
```

But the actual 3GPP spec (and the reference C implementation in wpa_supplicant) does:

```
OUT1 = AES_K(rotate(IN1⊕OPc, R1) ⊕ TEMP ⊕ C1) ⊕ OPc
```

The difference is *what gets rotated*. In f1, you rotate `IN1 XOR OPc`. In f2-f5, you rotate `TEMP XOR OPc`. It's a subtle but critical distinction — getting it wrong means every MAC-A comes out wrong, and no device can ever authenticate.

Similarly, for f2-f5, the constants C2-C5 are XORed *after* rotation, not before. The reference C code does `tmp1[15] ^= 1` (XOR c2) after the rotation loop, not as part of the input to rotation.

**The lesson:** When implementing a cryptographic standard, the reference implementation is your bible. A textual description of the algorithm can be ambiguous — "apply rotation and XOR" doesn't tell you the order. The C code leaves no room for interpretation: `tmp3[(i + 8) % 16] = tmp2[i] ^ opc[i]` is unambiguous.

### 2. Test Vectors Are Non-Negotiable

The official 3GPP test vectors caught the bug immediately. Without them, we'd have had a "working" implementation that silently produced wrong output — the worst kind of bug in cryptographic code.

One wrinkle: the plan's "Test Set 2" had an incorrect expected MAC-A value. The OPc, RES, CK, IK, and AK were all correct (these don't depend on SQN/AMF), but the MAC-A was wrong because the SQN/AMF values didn't match the official spec for that test set. Test Set 1 (the canonical TS 35.207 test set) passed perfectly and validated the algorithm.

**The lesson:** Always have at least one *unimpeachable* test vector — one from the official standard, not from a secondary source.

### 3. Buffer Hygiene: Zero Your Keys

After using decrypted key material, we immediately call `zeroBuffer()` to overwrite the memory. In a garbage-collected language like JavaScript, you can't control when memory is freed, but you can ensure that once you're done with sensitive data, the buffer contents are destroyed. It's not perfect (V8 could optimize it away, copies could exist elsewhere), but it's a defense-in-depth measure that costs nothing.

### 4. Envelope Encryption: Separate Concerns

Instead of encrypting every subscriber's keys with the same KEK directly, we use a per-subscriber DEK. This means:
- Rotating the KEK only requires re-wrapping DEKs, not re-encrypting all the data
- A compromised DEK only exposes one subscriber
- The KEK never touches plaintext subscriber data — it only wraps/unwraps DEKs

### 5. TypeScript ESM Is Still Tricky

We use `"type": "module"` in `package.json` with `"module": "NodeNext"` in tsconfig. This means:
- All imports must use `.js` extensions (even for `.ts` files!) — TypeScript resolves them at compile time
- `tsx` handles this transparently during development
- The compiled output in `dist/` uses proper `.js` extensions

### 6. Fastify's Type Provider Pattern

Using `app.withTypeProvider<TypeBoxTypeProvider>()` gives you end-to-end type inference: the request body type is inferred from the TypeBox schema, so `request.body.imsi` is typed as `string` without any manual type annotations. But you need to declare response schemas for *all* status codes you send — we initially only had a `200` response schema, and sending a `404` caused a TypeScript error.

## How Good Engineers Think About This

1. **Start with the hardest part.** We built MILENAGE first, tested it against official vectors, and only then built the HTTP layer around it. If the crypto is wrong, nothing else matters.

2. **Trust nothing, verify everything.** The plan's test vectors had an error. Rather than assuming the plan was correct, we verified against the reference C implementation and the official 3GPP spec.

3. **Minimal surface area.** The mock-hss only defines the database columns it actually uses, not the full ECS schema. It has two HTTP endpoints. It uses XOR wrapping instead of a full KMS client. Every decision minimizes complexity while still being correct.

4. **Security by default.** Keys are encrypted at rest, decrypted only in memory, and zeroed after use. The separation bit in AMF (0x8000) is set for LTE/5G compatibility. These aren't afterthoughts — they're baked into the design.
