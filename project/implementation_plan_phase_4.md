# Token Rotation

## What

Add rolling token rotation: on every successful fast-auth request, revoke the old token and issue a new one. This limits the exposure window of any single token and matches the Phase 4 spec from `architecture.md`:

> "Each successful response issues a new token and revokes the old one (DEL from Redis, mark `consumed=true` in Postgres). Rolling expiry: tokens refresh on every use."

## Changes

### 1. Add `revokeToken()` to `src/auth/tokenService.ts`
- Delete from Redis (`token:{value}`)
- Set `consumed = true` in Postgres
- New export: `revokeToken(tokenValue: string): Promise<void>`

### 2. Add `rotateToken()` to `src/auth/tokenService.ts`
- Calls `revokeToken(oldToken)` + `generateToken(subscriberId, tokenType, clientIp)`
- Returns the new `TokenInfo`
- New export: `rotateToken(oldTokenValue, subscriberId, tokenType, clientIp): Promise<TokenInfo>`

### 3. Modify fast-auth path in `src/server/routes/entitlement.ts`
- POST handler, Path 1 (token present): after successful `validateToken()`, call `rotateToken()` and return the **new** token in the response instead of echoing the old one
- GET handler: read-only, no rotation (GET should be safe/idempotent)

### 4. Add unit test in `tests/unit/tokenService.test.ts`
- `revokeToken` marks token consumed in DB and removes from Redis
- `rotateToken` returns a new token different from the old one
- These are unit tests that mock DB/Redis — no Docker needed

### 5. Add integration test case in `tests/integration/eapAka.integration.test.ts`
- After EAP-AKA handshake gets a token, use it in a fast-auth POST → verify response contains a **different** token
- Use the old token again → 401 (consumed)
- Use the new token → 200

## Files Modified
```
src/auth/tokenService.ts                     — add revokeToken(), rotateToken()
src/server/routes/entitlement.ts             — call rotateToken() on fast-auth POST
tests/unit/tokenService.test.ts              — new file, unit tests for rotation
tests/integration/eapAka.integration.test.ts — add rotation integration test
```

## Verification
1. `npm run build` — compiles
2. `npm run test:unit` — new token rotation unit tests pass
3. `npm run test:integration` — rotation integration test passes (old token rejected, new token accepted)
