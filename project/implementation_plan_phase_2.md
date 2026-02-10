# Phase 2: Mock HSS with MILENAGE Cryptography

## Overview
Create a separate `mock-hss/` service that implements the MILENAGE algorithm (3GPP TS 35.206), handles envelope encryption of subscriber keys, and exposes a `POST /vectors` endpoint. ~12 new files + 1 modified file.

## Implementation Steps (in dependency order)

### Step 1: Project scaffolding
**Files:** `mock-hss/package.json`, `mock-hss/tsconfig.json`, `mock-hss/.env`

- `package.json`: ESM, node>=20, deps: `fastify`, `@fastify/type-provider-typebox`, `@sinclair/typebox`, `drizzle-orm`, `pg`, `pino`, `dotenv`. Dev: `typescript`, `@types/node`, `@types/pg`, `vitest`, `tsx`.
- Scripts: `dev`, `build`, `start`, `test`, `seed`
- `tsconfig.json`: identical to root (ES2022, NodeNext, strict)
- `.env`: PORT=3001, DATABASE_URL, LOCAL_KEK_HEX (64 hex chars = 32 bytes)
- Run `npm install` in mock-hss/

### Step 2: MILENAGE algorithm
**File:** `mock-hss/src/milenage.ts`

Core implementation using only `crypto.createCipheriv('aes-128-ecb', ...)`:

- `aesEncrypt(key, input)`: Single AES-128-ECB block (16 bytes in, 16 out, `setAutoPadding(false)`)
- `xor(a, b)`: XOR two equal-length buffers
- `rotate(buf, bits)`: Circular left rotation of 128-bit buffer by N bits
- `computeOPc(ki, op)`: `OPc = AES_K(OP) XOR OP`
- `generateVectors(ki, op, sqn, amf)`: Main entry (generates random RAND)
- `generateVectorsWithRand(ki, op, rand, sqn, amf)`: Deterministic (for testing)

MILENAGE algorithm steps:
1. OPc = AES_K(OP) XOR OP
2. TEMP = AES_K(RAND XOR OPc)
3. f1: IN1 = SQN||AMF||SQN||AMF; OUT1 = AES_K(rotate(TEMP⊕OPc⊕C1, R1) ⊕ OPc ⊕ IN1) ⊕ OPc; MAC-A = OUT1[0..7]
4. f2/f5: OUT2 = AES_K(rotate(TEMP⊕OPc⊕C2, R2) ⊕ OPc) ⊕ OPc; RES=OUT2[8..15], AK=OUT2[0..5]
5. f3: OUT3 = AES_K(rotate(TEMP⊕OPc⊕C3, R3) ⊕ OPc) ⊕ OPc; CK=OUT3[0..15]
6. f4: OUT4 = AES_K(rotate(TEMP⊕OPc⊕C4, R4) ⊕ OPc) ⊕ OPc; IK=OUT4[0..15]
7. AUTN = (SQN⊕AK) || AMF || MAC-A

Constants: C1=0x00..00 R1=64, C2=0x00..01 R2=0, C3=0x00..02 R3=32, C4=0x00..04 R4=64, C5=0x00..08 R5=96

### Step 3: MILENAGE unit tests
**File:** `mock-hss/tests/milenage.test.ts`

Test against 3GPP TS 35.207 official test vectors. **Must pass before proceeding.**

**Test Set 1:**
```
Ki   = 465b5ce8 b199b49f aa5f0a2e e238a6bc
OP   = cdc202d5 123e20f6 2b6d676a c72cb318
RAND = 23553cbe 9637a89d 218ae64d ae47bf35
SQN  = ff9bb4d0 b607
AMF  = b9b9

Expected:
OPc    = cd63cb71 954a9f4e 48a5994e 37a02baf
MAC-A  = 4a9ffac3 54dfafb3
MAC-S  = 01cfaf9e c4e871e9
RES    = a54211d5 e3ba50bf
CK     = b40ba9a3 c58b2a05 bbf0d987 b21bf8cb
IK     = f769bcd7 51044604 12767271 1c6d3441
AK     = aa689c64 8370
AK(f5*)= 451e8bec a43b
```

**Test Set 2:**
```
Ki   = 0396eb31 7b6d1c36 f19c1c84 cd6ffd16
OP   = ff53bade 17df5d4e 793073ce 9d7579fa
RAND = c00d6031 03dcee52 c4478119 494202e8
SQN  = fd8eef40 df7d
AMF  = af17

Expected:
OPc  = 53c15671 c60a4b73 1c55b2a8 fdb5cfb0
MAC-A= 5df5b31e 07a08e88
MAC-S= a8c016e5 1ef4a343
RES  = f365cd68 3cd92e96
CK   = e203edb3 971574f5 a94b0d61 b816345d
IK   = 0c4524ad eac041c4 dd830d20 854fc46b
AK   = f0b9c08a d02e
AK(f5*)= 6085a86c 6f63
```

Verify: OPc, MAC-A, RES, CK, IK, AK for each set.

### Step 4: Envelope encryption (KMS)
**File:** `mock-hss/src/kms.ts`

- `KeyManager` interface: `unwrapDek(wrappedDek: Buffer): Promise<Buffer>`
- `LocalKeyManager`: XOR-based wrap/unwrap using `LOCAL_KEK_HEX` (for dev)
- `encryptWithDek(dek, plaintext)`: AES-256-GCM, output = IV[12] || ciphertext || authTag[16]
- `decryptWithDek(dek, encrypted)`: Reverse of above
- `wrapDek(kek, dek)` / `unwrapDek`: XOR-based for local dev
- `zeroBuffer(buf)`: Fill with zeros for memory hygiene

### Step 5: KMS unit tests
**File:** `mock-hss/tests/kms.test.ts`

- Round-trip encrypt/decrypt with DEK
- Round-trip wrap/unwrap DEK with KEK
- zeroBuffer clears data

### Step 6: Config + Database
**Files:** `mock-hss/src/config.ts`, `mock-hss/src/db.ts`

- `config.ts`: Typed config (port, host, databaseUrl, dbPoolSize, localKekHex, nodeEnv)
- `db.ts`: pg Pool + Drizzle, duplicate only the `subscribers` table definition (minimal subset: id, imsi, kiEncrypted, opEncrypted, kiDekWrapped, sqn)

### Step 7: Fastify server
**File:** `mock-hss/src/index.ts`

- `GET /health` → `{ status: "ok" }`
- `POST /vectors` with TypeBox validation:
  - Request: `{ imsi: string, sqn?: number }`
  - Response: `{ rand, autn, xres, ck, ik }` (base64-encoded)
  - Flow: query subscriber → unwrap DEK → decrypt Ki/OP → MILENAGE → zero buffers → respond
  - 404 if subscriber not found
- `onReady` hook: verify Postgres connection
- AMF default: `0x8000` (separation bit set for LTE/5G)

### Step 8: Seed script
**File:** `mock-hss/src/seed.ts`

Inserts 2 test subscribers using TS 35.207 test vector Ki/OP values, encrypted with envelope encryption. Creates the `subscribers` table if it doesn't exist (via raw SQL DDL).

### Step 9: Dockerfile + docker-compose update
**Files:** `mock-hss/Dockerfile`, `docker-compose.yml` (modify existing)

- `mock-hss/Dockerfile`: Multi-stage (node:20-slim), identical pattern to ECS
- `docker-compose.yml`: Add `mock-hss` service on port 3001, depends_on postgres healthy. Update `ecs` to depend on mock-hss.

## Files Created/Modified (13 total)

```
mock-hss/
  package.json          (new)
  tsconfig.json         (new)
  .env                  (new)
  Dockerfile            (new)
  src/
    index.ts            (new) — Fastify server
    config.ts           (new) — env config
    db.ts               (new) — Postgres connection
    milenage.ts         (new) — MILENAGE algorithm
    kms.ts              (new) — envelope encryption
    seed.ts             (new) — test data seeder
  tests/
    milenage.test.ts    (new) — 3GPP test vectors
    kms.test.ts         (new) — encryption round-trips

docker-compose.yml      (modify) — add mock-hss service
```

## Verification

1. `cd mock-hss && npm test` — MILENAGE test vectors pass (both test sets) + KMS round-trips pass
2. `cd mock-hss && npm run build` — TypeScript compiles cleanly
3. `docker compose up --build` — All 4 services start (postgres, redis, ecs, mock-hss)
4. `curl http://localhost:3001/health` → `200 { "status": "ok" }`
5. Run seed: insert test subscribers into Postgres
6. `curl -X POST http://localhost:3001/vectors -H 'Content-Type: application/json' -d '{"imsi":"001010000000001"}'` → returns `{ rand, autn, xres, ck, ik }` as base64
7. Ki never appears in any response, log, or error message
